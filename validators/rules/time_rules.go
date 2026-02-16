package rules

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

// IsDatetime validates if a string matches a given layout (default RFC3339).
// Option 0: layout string (optional)
func IsDatetime(c ValidatorContext) func(val any) error {
	layout := time.RFC3339
	if len(c.Options) > 0 {
		if l, ok := c.Options[0].(string); ok {
			layout = l
		}
	}

	return func(val any) error {
		kind := c.Kind
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		if kind != reflect.String {
			return errors.New("invalid datetime value. value must be a string")
		}

		v := val.(string)
		if _, err := time.Parse(layout, v); err != nil {
			return fmt.Errorf("value must match datetime layout '%s'", layout)
		}
		return nil
	}
}

// IsTimezone validates if a string is a valid timezone location.
func IsTimezone(c ValidatorContext) func(val any) error {
	return func(val any) error {
		kind := c.Kind
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		if kind != reflect.String {
			return errors.New("invalid timezone value. value must be a string")
		}

		v := val.(string)
		if v == "" {
			return errors.New("timezone value cannot be empty")
		}
		if _, err := time.LoadLocation(v); err != nil {
			return errors.New("invalid timezone location")
		}
		return nil
	}
}
