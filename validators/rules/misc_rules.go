package rules

import (
	"encoding/json"
	"errors"
	"reflect"
)

func IsBoolean(c ValidatorContext) func(val any) error {
	return func(val any) error {
		kind := c.Kind
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		if kind == reflect.Bool {
			return nil
		}

		if kind == reflect.String {
			v, ok := val.(string)
			if !ok {
				return errors.New("invalid boolean value")
			}
			if v == "true" || v == "false" || v == "1" || v == "0" {
				return nil
			}
		}

		return errors.New("value must be a boolean or a string representation of boolean")
	}
}

func IsJSON(c ValidatorContext) func(val any) error {
	return func(val any) error {
		v, ok := val.(string)
		if !ok {
			return errors.New("invalid json value. value must be a string")
		}
		if v == "" {
			return errors.New("json value cannot be empty")
		}
		if !json.Valid([]byte(v)) {
			return errors.New("value must be valid json")
		}
		return nil
	}
}

// IsDefault checks if the value is the default (zero) value for its type.
// But wait, validator's `isdefault` usually checks if the value is the default value,
// often used with `excluded_if` or similar.
// If it implies "is zero value", then it passes if value IS zero value.
func IsDefault(c ValidatorContext) func(val any) error {
	return func(val any) error {
		v := reflect.ValueOf(val)
		if !v.IsValid() || v.IsZero() {
			return nil
		}
		return errors.New("value must be the default (zero) value")
	}
}
