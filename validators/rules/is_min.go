package rules

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/michaelolof/gofi/utils"
)

// IsMin checks that a given value must be below or equal to a specific limit
func IsMin(c ValidatorContext) func(val any) error {
	kind := c.Kind
	limit := c.Options
	var err error
	var min float64
	if len(limit) != 1 {
		err = errors.New("validation rule 'IsMin' requires 1 limit argument")
	} else {
		min, err = utils.AnyValueToFloat(limit[0])
	}

	check := func(isValid bool) error {
		if isValid {
			return nil
		} else {
			return fmt.Errorf("value must be equal or greater than minimum limit of %f", min)
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
				return IsMin(tempC)(val)
			} else {
				return errors.New("invalid value passed to IsMin function")
			}
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		return func(val any) error {
			if err != nil {
				return err
			}

			v := reflect.ValueOf(val)
			return check(float64(v.Len()) >= min)
		}
	default:
		return func(val any) error {
			switch v := val.(type) {
			case int:
				return check(float64(v) >= min)
			case int8:
				return check(float64(v) >= min)
			case int16:
				return check(float64(v) >= min)
			case int32:
				return check(float64(v) >= min)
			case int64:
				return check(float64(v) >= min)
			case uint:
				return check(float64(v) >= min)
			case uint8:
				return check(float64(v) >= min)
			case uint16:
				return check(float64(v) >= min)
			case uint32:
				return check(float64(v) >= min)
			case uint64:
				return check(float64(v) >= min)
			case float32:
				return check(float64(v) >= min)
			case float64:
				return check(v >= min)
			case string:
				return check(float64(len(v)) >= min)
			default:
				return fmt.Errorf("unsupported type %T when validating minimum value", v)
			}
		}
	}
}
