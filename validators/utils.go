package validators

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
)

func isFileURL(path string) error {
	if !strings.HasPrefix(path, "file:/") {
		return errors.New("file url must start with file:/")
	}
	_, err := url.ParseRequestURI(path)
	return err
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

type number interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 |
		float32 | float64
}

func isValRequired[T comparable](val any, empty T) (error, bool) {
	if v, ok := val.(T); ok {
		if v == empty {
			return errors.New("value is empty"), true
		} else {
			return nil, true
		}
	} else {
		return errors.New("invalid required value passed"), false
	}
}
