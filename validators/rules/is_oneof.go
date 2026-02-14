package rules

import (
	"errors"
	"fmt"
	"reflect"
)

// Validates if a primitive value is one of the defined arguments
func IsOneOf(c ValidatorContext) func(arg any) error {
	// Pre-check if target type is comparable
	if !c.Type.Comparable() {
		err := fmt.Errorf("type %s is not comparable", c.Type)
		return func(any) error { return err }
	}

	// Check if we can use direct comparison (primitives and their aliases)
	useDirect := isPrimitiveKind(c.Type.Kind())

	// Pre-convert options during initialization
	var (
		convertedOpts []interface{}
		initErrors    []error
	)

	for i, opt := range c.Options {
		optType := reflect.TypeOf(opt)

		// Fast path for direct comparable types
		if useDirect && optType == c.Type {
			convertedOpts = append(convertedOpts, opt)
			continue
		}

		// Slow path using reflection
		optVal := reflect.ValueOf(opt)
		if !optVal.IsValid() || !optVal.Type().ConvertibleTo(c.Type) {
			initErrors = append(initErrors, fmt.Errorf(
				"option %d: %v (type %s) is not convertible to %s",
				i, opt, optType, c.Type,
			))
			continue
		}

		converted := optVal.Convert(c.Type).Interface()
		convertedOpts = append(convertedOpts, converted)
	}

	isEmpty := IsRequired(c)

	return func(arg any) error {
		// Don't validate when empty. That will be handled by the required rule.
		if err := isEmpty(arg); err != nil {
			return nil
		}

		if len(initErrors) > 0 {
			return fmt.Errorf("invalid options:\n%w", errors.Join(initErrors...))
		}

		// Fast path for direct comparable types
		if useDirect {
			if valType := reflect.TypeOf(arg); valType == c.Type {
				for _, opt := range convertedOpts {
					if arg == opt {
						return nil
					}
				}
				return fmt.Errorf("value %v not in allowed options", arg)
			}
		}

		// Slow path using reflection
		v := reflect.ValueOf(arg)
		if !v.IsValid() || !v.Type().ConvertibleTo(c.Type) {
			return fmt.Errorf(
				"value %v (type %s) is not convertible to %s",
				arg, reflect.TypeOf(arg), c.Type,
			)
		}

		convertedVal := v.Convert(c.Type).Interface()
		for _, opt := range convertedOpts {
			if convertedVal == opt {
				return nil
			}
		}

		return fmt.Errorf("value %v not in allowed options", arg)
	}
}
