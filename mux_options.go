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
	IsCustomType(typ reflect.Type) (*CustomSchemaProps, bool)
	CustomEncode(val any) (any, error)
	CustomDecode(obj any) (string, error)
}

type CustomSchemaProps struct {
	Type   string
	Format string
}
