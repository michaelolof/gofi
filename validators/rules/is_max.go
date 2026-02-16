package rules

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/michaelolof/gofi/utils"
)

// IsMax checks that a given value must be below or equal to a specific limit
func IsMax(c ValidatorContext) func(val any) error {
	kind := c.Kind
	limit := c.Options
	var err error
	var max float64
	if len(limit) != 1 {
		err = errors.New("validation rule 'IsMax' requires 1 limit argument")
	} else {
		max, err = utils.AnyValueToFloat(limit[0])
	}

	check := func(isValid bool) error {
		if isValid {
			return nil
		} else {
			return fmt.Errorf("value must be equal or lesser than maximum limit of %f", max)
		}
	}

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			if err != nil {
				return err
			}

			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsMax(tempC)(val)
			} else {
				return errors.New("invalid value passed to IsMax function")
			}
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		return func(val any) error {
			if err != nil {
				return err
			}

			v := reflect.ValueOf(val)
			return check(float64(v.Len()) <= max)
		}
	default:
		return func(val any) error {
			switch v := val.(type) {
			case int:
				return check(float64(v) <= max)
			case int8:
				return check(float64(v) <= max)
			case int16:
				return check(float64(v) <= max)
			case int32:
				return check(float64(v) <= max)
			case int64:
				return check(float64(v) <= max)
			case uint:
				return check(float64(v) <= max)
			case uint8:
				return check(float64(v) <= max)
			case uint16:
				return check(float64(v) <= max)
			case uint32:
				return check(float64(v) <= max)
			case uint64:
				return check(float64(v) <= max)
			case float32:
				return check(float64(v) <= max)
			case float64:
				return check(v <= max)
			case string:
				return check(float64(len(v)) <= max)
			default:
				return fmt.Errorf("unsupported type %T when validating maximum value", v)
			}
		}
	}
}
