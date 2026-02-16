package rules

import (
	"errors"
	"fmt"
	"reflect"
)

var errValid error = errors.New("value is invalid")

func IsRequired(c ValidatorContext) func(arg any) error {
	zeroVal := reflect.Zero(c.Type).Interface()
	isPrimitive := isPrimitiveKind(c.Type.Kind())

	return func(arg any) error {
		if arg == nil {
			return fmt.Errorf("value is required")
		}

		// Fast path for direct primitive comparisons
		if isPrimitive {
			if reflect.TypeOf(arg) == c.Type {
				if arg == zeroVal {
					return fmt.Errorf("value is required (got zero primitive)")
				}
				return nil
			}
		}

		// General reflection-based check
		v := reflect.ValueOf(arg)
		if !v.IsValid() {
			return fmt.Errorf("value is required (invalid value)")
		}

		if !v.Type().ConvertibleTo(c.Type) {
			return fmt.Errorf("value type %s is not compatible with %s",
				v.Type(), c.Type)
		}

		converted := v.Convert(c.Type)
		if converted.IsZero() {
			return fmt.Errorf("value is required (zero value for type %s)",
				c.Type.String())
		}

		return nil
	}
}

func isPrimitiveKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.String,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64:
		return true
	default:
		return false
	}
}
