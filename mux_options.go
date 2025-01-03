package gofi

import (
	"reflect"
)

type muxOptions struct {
	errHandler       func(err error, c Context)
	customValidators map[string]func([]any) func(any) error
	customSchema     CustomSchemaTypes
}

func defaultMuxOptions() *muxOptions {
	return &muxOptions{
		errHandler:       defaultErrorHandler,
		customValidators: map[string]func([]any) func(any) error{},
		customSchema:     map[string]CustomSchemaType{},
	}
}

type CustomSchemaTypes map[string]CustomSchemaType

type CustomSchemaType interface {
	MatchType(typ reflect.Type) bool
	CustomOpenapiTypes(typ reflect.Type) CustomOpenapiTypes
	BindValueToType(val any) (any, error)
	GetPrimitiveValue(obj any) (string, error)
}

type CustomOpenapiTypes struct {
	Type   string
	Format string
}
