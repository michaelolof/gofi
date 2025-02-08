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
