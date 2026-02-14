package rules

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/michaelolof/gofi/utils"
)

// Helper to get float64 value from limit option
func getLimit(opts []any) (float64, error) {
	if len(opts) != 1 {
		return 0, errors.New("validation rule requires 1 limit argument")
	}
	return utils.AnyValueToFloat(opts[0])
}

// Helper to evaluate comparison
func evaluateComparison(c ValidatorContext, check func(float64, float64) bool, errFmt string) func(val any) error {
	limit, err := getLimit(c.Options)

	return func(val any) error {
		if err != nil {
			return err
		}

		kind := c.Kind
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		var currentVal float64

		switch kind {
		case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
			v := reflect.ValueOf(val)
			currentVal = float64(v.Len())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			currentVal = float64(reflect.ValueOf(val).Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			currentVal = float64(reflect.ValueOf(val).Uint())
		case reflect.Float32, reflect.Float64:
			currentVal = reflect.ValueOf(val).Float()
		default:
			return fmt.Errorf("unsupported type %v for comparison", kind)
		}

		if check(currentVal, limit) {
			return nil
		}
		return fmt.Errorf(errFmt, limit)
	}
}

func IsLen(c ValidatorContext) func(val any) error {
	return evaluateComparison(c, func(v, l float64) bool { return v == l }, "value length must be %f")
}

func IsEq(c ValidatorContext) func(val any) error {
	// For strings/collections IsEq is alias for IsLen, for numbers it's value equality
	// The evaluateComparison handles both by checking Len() for collections and Value for numbers
	return evaluateComparison(c, func(v, l float64) bool { return v == l }, "value must be equal to %f")
}

func IsNe(c ValidatorContext) func(val any) error {
	return evaluateComparison(c, func(v, l float64) bool { return v != l }, "value must not be equal to %f")
}

func IsLt(c ValidatorContext) func(val any) error {
	return evaluateComparison(c, func(v, l float64) bool { return v < l }, "value must be less than %f")
}

func IsGt(c ValidatorContext) func(val any) error {
	return evaluateComparison(c, func(v, l float64) bool { return v > l }, "value must be greater than %f")
}

func IsLte(c ValidatorContext) func(val any) error {
	return evaluateComparison(c, func(v, l float64) bool { return v <= l }, "value must be less than or equal to %f")
}

func IsGte(c ValidatorContext) func(val any) error {
	return evaluateComparison(c, func(v, l float64) bool { return v >= l }, "value must be greater than or equal to %f")
}
