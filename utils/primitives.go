package utils

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

type nonPrimitiveType struct{}

var nonPrimitive = &nonPrimitiveType{}

func PrimitiveFromStr(kind reflect.Kind, val string) (any, error) {

	switch kind {
	case reflect.String:
		if val == "" {
			return nil, nil
		} else {
			return val, nil
		}
	case reflect.Bool:
		return strconv.ParseBool(val)
	case reflect.Int:
		return toIntX(val, 0)
	case reflect.Int8:
		return toIntX(val, 8)
	case reflect.Int16:
		return toIntX(val, 16)
	case reflect.Int32:
		return toIntX(val, 32)
	case reflect.Int64:
		return toIntX(val, 64)
	case reflect.Uint:
		return toUintX(val, 0)
	case reflect.Uint8:
		return toUintX(val, 8)
	case reflect.Uint16:
		return toUintX(val, 16)
	case reflect.Uint32:
		return toUintX(val, 32)
	case reflect.Uint64:
		return toUintX(val, 64)
	case reflect.Float32:
		v, err := strconv.ParseFloat(val, 32)
		if err != nil {
			return nil, err
		}
		return float32(v), nil
	case reflect.Float64:
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return nil, err
		}
		return float64(v), err
	default:
		return nonPrimitive, nil
	}
}

type primitives interface {
	int | int8 | int16 | int32 | int64 | uint | uint8 | uint16 | uint32 | uint64 | float32 | float64 | string | bool
}

// func PrimitiveFromAny[T primitives](kind reflect.Kind, val any) (T, error) {
// 	switch kind {
// 	case reflect.Int:
// 		return
// 	}
// }

func IsPrimitiveKind(kind reflect.Kind) bool {
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

func NotPrimitiveKind(kind reflect.Kind) bool {
	return !IsPrimitiveKind(kind)
}

func toIntX(val string, bitSize int) (any, error) {
	v, err := strconv.ParseInt(val, 10, bitSize)
	if err != nil {
		return nil, errors.New("error converting value to int type")
	}
	switch bitSize {
	case 0:
		return int(v), nil
	case 8:
		return int8(v), nil
	case 16:
		return int16(v), nil
	case 32:
		return int32(v), nil
	case 64:
		return int64(v), nil
	}
	return nil, fmt.Errorf("unknown int type 'int%d' passed as value", bitSize)
}

func toUintX(val string, bitSize int) (any, error) {
	v, err := strconv.ParseUint(val, 10, bitSize)
	if err != nil {
		return nil, err
	}
	switch bitSize {
	case 0:
		return uint(v), nil
	case 8:
		return uint8(v), nil
	case 16:
		return uint16(v), nil
	case 32:
		return uint32(v), nil
	case 64:
		return uint64(v), nil
	}
	return nil, fmt.Errorf("unknown int type 'int%d' passed as value", bitSize)
}

func IsPrimitive(val any) bool {
	return val != nonPrimitive
}

func NotPrimitive(val any) bool {
	return val == nonPrimitive
}

func ValidCookieType(typ reflect.Type) bool {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return IsPrimitiveKind(typ.Kind()) || typ == CookieType
}
