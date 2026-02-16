package rules

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/michaelolof/gofi/utils"
)

var errValid error = errors.New("value is invalid")

func IsRequired(c ValidatorContext) func(arg any) error {
	return func(arg any) error {
		if arg == nil {
			return fmt.Errorf("value is required")
		}

		v := reflect.ValueOf(arg)
		if !v.IsValid() {
			return fmt.Errorf("value is required (invalid value)")
		}

		// Check basic "zero" conditions for common types regardless of strict type match
		// This handles []any -> []int conversions
		switch v.Kind() {
		case reflect.Slice, reflect.Map, reflect.String, reflect.Array:
			if v.Len() == 0 {
				return fmt.Errorf("value is required (empty)")
			}
			return nil
		}

		if utils.KindIsNumber(v.Kind()) {
			f, _ := utils.AnyValueToFloat(arg)
			if f == 0 {
				return fmt.Errorf("value is required (zero)")
			}
			return nil
		}

		if v.Kind() == reflect.Bool {
			if !v.Bool() {
				// Assuming required bool must be true?
				// Actually, required just means present. But passing false is "zero".
				// Standard `required` usually means non-zero.
				return fmt.Errorf("value is required (false)")
			}
			return nil
		}

		// General reflection-based check for other types
		if !v.Type().ConvertibleTo(c.Type) {
			// If we can't convert, but it passed the checks above (meaning it's not empty slice/map/string/number),
			// then it IS present. So strict type check here might be redundant if the Parser handles conversion later.
			// However, if the Parser fails, that's a parser error.
			// Validator's job is to check validity of value.

			// We return nil here to allow Parser to attempt conversion.
			// If Parser fails, it returns its own error.
			// If we return error here, we block valid convertible values.
			return nil
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
