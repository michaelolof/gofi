package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unsafe"
)

func Pop[T any](arr *[]T) {
	if arr == nil {
		return
	}

	if len(*arr) == 0 {
		return
	}

	var sb = *arr
	*arr = sb[:len(sb)-1]
}

func Append[T any](arr *[]T, val T) {
	*arr = append(*arr, val)
}

func Push[T comparable](arr *[]T, val T) []T {
	var d T
	if arr == nil && val != d {
		return []T{val}
	} else if arr != nil && val == d {
		return *arr
	} else {
		return append(*arr, val)
	}
}

func LastItem[T any](arr *[]T) *T {
	var t T
	if arr == nil {
		return &t
	}

	if len(*arr) == 0 {
		return &t
	}

	return &(*arr)[len(*arr)-1]
}

func UpdateItem[T any](arr *[]T, fn func(b *T)) {
	if arr == nil {
		return
	}

	if len(*arr) == 0 {
		return
	}

	fn(&(*arr)[len(*arr)-1])
}

func ToUpperFirst(s string) string {
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

func KindIsNumber(k reflect.Kind) bool {
	switch k {
	case reflect.Int,
		reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func PrimitiveKindIsEmpty(k reflect.Kind, val any) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		var empty any = 0
		return empty == val
	case reflect.String:
		var empty any = ""
		return empty == val
	default:
		return false
	}
}

func AnyValueToFloat(val any) (float64, error) {
	switch t := val.(type) {
	case int:
		return float64(t), nil
	case int8:
		return float64(t), nil
	case int16:
		return float64(t), nil
	case int32:
		return float64(t), nil
	case int64:
		return float64(t), nil
	case uint:
		return float64(t), nil
	case uint8:
		return float64(t), nil
	case uint16:
		return float64(t), nil
	case uint32:
		return float64(t), nil
	case uint64:
		return float64(t), nil
	case float32:
		return float64(t), nil
	case float64:
		return t, nil
	case string:
		return strconv.ParseFloat(t, 64)
	}

	var floatType = reflect.TypeOf(float64(0))
	var stringType = reflect.TypeOf("")
	v := reflect.ValueOf(val)
	v = reflect.Indirect(v)
	if v.Type().ConvertibleTo(floatType) {
		fv := v.Convert(floatType)
		return fv.Float(), nil
	} else if v.Type().ConvertibleTo(stringType) {
		sv := v.Convert(stringType)
		s := sv.String()
		return strconv.ParseFloat(s, 64)
	} else {
		return math.NaN(), fmt.Errorf("cannot convert %v to float64", v.Type())
	}
}

func TryAsReader(m any) io.Reader {
	bs, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(bs)
}

func SafeConvert(v reflect.Value, t reflect.Type) (reflect.Value, error) {
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("cannot convert invalid value to %v", t)
	}

	if v.Type().ConvertibleTo(t) {
		return v.Convert(t), nil
	}

	// Try soft conversions for common types
	if KindIsNumber(v.Kind()) && KindIsNumber(t.Kind()) {
		// Number to Number
		f, _ := AnyValueToFloat(v.Interface())
		switch t.Kind() {
		case reflect.Int:
			return reflect.ValueOf(int(f)).Convert(t), nil
		case reflect.Int8:
			return reflect.ValueOf(int8(f)).Convert(t), nil
		case reflect.Int16:
			return reflect.ValueOf(int16(f)).Convert(t), nil
		case reflect.Int32:
			return reflect.ValueOf(int32(f)).Convert(t), nil
		case reflect.Int64:
			return reflect.ValueOf(int64(f)).Convert(t), nil
		case reflect.Uint:
			return reflect.ValueOf(uint(f)).Convert(t), nil
		case reflect.Uint8:
			return reflect.ValueOf(uint8(f)).Convert(t), nil
		case reflect.Uint16:
			return reflect.ValueOf(uint16(f)).Convert(t), nil
		case reflect.Uint32:
			return reflect.ValueOf(uint32(f)).Convert(t), nil
		case reflect.Uint64:
			return reflect.ValueOf(uint64(f)).Convert(t), nil
		case reflect.Float32:
			return reflect.ValueOf(float32(f)).Convert(t), nil
		case reflect.Float64:
			return reflect.ValueOf(float64(f)).Convert(t), nil
		}
	}

	return reflect.Value{}, fmt.Errorf("cannot convert %v to %v", v.Type(), t)
}

// StringToBytes converts string to byte slice without a memory allocation.
func StringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// BytesToString converts byte slice to string without a memory allocation.
func BytesToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
