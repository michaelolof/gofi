package validators

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Validate validates a single value against a rule string.
// ruleString format: "required,min=10,max=20,oneof=red green blue"
func Validate(val any, rulesStr string) error {
	if rulesStr == "" {
		return nil
	}

	valType := reflect.TypeOf(val)
	valKind := reflect.Invalid
	if valType != nil {
		valKind = valType.Kind()
	}

	ruleDefs := parseRules(rulesStr)

	for _, def := range ruleDefs {
		validatorFn := NewContextValidatorFn(valType, valKind, def.Name, def.Args, nil)
		if err := validatorFn(val); err != nil {
			return err
		}
	}

	return nil
}

// ValidateStruct validates a struct's fields based on 'validate' tags.
func ValidateStruct(s any) error {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return errors.New("ValidateStruct: value must be a struct or pointer to struct")
	}

	t := v.Type()
	var errs []string

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("validate")
		if tag == "" {
			continue
		}

		fieldVal := v.Field(i).Interface()
		if err := Validate(fieldVal, tag); err != nil {
			errs = append(errs, fmt.Sprintf("field '%s': %v", field.Name, err))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

type ruleDefinition struct {
	Name string
	Args []any
}

func parseRules(ruleStr string) []ruleDefinition {
	var definitions []ruleDefinition

	// Split by comma ","
	// Note: simplistic splitting, doesn't handle escaped commas if that's a requirement (usually not for simple tags)
	parts := strings.Split(ruleStr, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		def := ruleDefinition{}

		// Check for arguments (key=value or key=val1 val2)
		if idx := strings.Index(part, "="); idx != -1 {
			def.Name = strings.TrimSpace(part[:idx])
			argsStr := part[idx+1:]

			// Handle multiple args separated by space?
			// go-playground/validator treats spaces in param as separator for 'oneof',
			// but for 'min=10' it's just one arg.
			// Let's split by space to support 'oneof=red green'
			// But careful with things that shouldn't be split?
			// Most simple rules use single arg.
			// 'oneof' is the main one using spaces.
			argParts := strings.Fields(argsStr)
			for _, arg := range argParts {
				def.Args = append(def.Args, arg)
			}
		} else {
			def.Name = part
		}

		definitions = append(definitions, def)
	}

	return definitions
}
