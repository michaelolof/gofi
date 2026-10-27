package utils

import (
	"fmt"
	"math"
	"reflect"
)

// SetNumberChecked converts the numeric value src into dst. Unlike a plain
// reflect conversion it never wraps or truncates: values that overflow dst,
// negative values for an unsigned dst, and fractional values for an integer
// dst return an error and leave dst unchanged. The boolean reports whether
// both values were numeric and the conversion was attempted.
func SetNumberChecked(dst, src reflect.Value) (bool, error) {
	if !KindIsNumber(dst.Kind()) || !KindIsNumber(src.Kind()) {
		return false, nil
	}

	switch dst.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var n int64
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n = src.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			u := src.Uint()
			if u > math.MaxInt64 {
				return true, overflowError(u, dst.Type())
			}
			n = int64(u)
		default:
			f := src.Float()
			if f != math.Trunc(f) {
				return true, fmt.Errorf("value %v is not a whole number for %s", f, dst.Type())
			}
			if f < math.MinInt64 || f >= math.MaxInt64 {
				return true, overflowError(f, dst.Type())
			}
			n = int64(f)
		}
		if dst.OverflowInt(n) {
			return true, overflowError(n, dst.Type())
		}
		dst.SetInt(n)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		var u uint64
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n := src.Int()
			if n < 0 {
				return true, fmt.Errorf("negative value %d for %s", n, dst.Type())
			}
			u = uint64(n)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			u = src.Uint()
		default:
			f := src.Float()
			if f != math.Trunc(f) {
				return true, fmt.Errorf("value %v is not a whole number for %s", f, dst.Type())
			}
			if f < 0 {
				return true, fmt.Errorf("negative value %v for %s", f, dst.Type())
			}
			if f >= math.MaxUint64 {
				return true, overflowError(f, dst.Type())
			}
			u = uint64(f)
		}
		if dst.OverflowUint(u) {
			return true, overflowError(u, dst.Type())
		}
		dst.SetUint(u)

	default:
		var f float64
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			f = float64(src.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			f = float64(src.Uint())
		default:
			f = src.Float()
		}
		if dst.OverflowFloat(f) {
			return true, overflowError(f, dst.Type())
		}
		dst.SetFloat(f)
	}
	return true, nil
}

func overflowError(v any, t reflect.Type) error {
	return fmt.Errorf("value %v overflows %s", v, t)
}
