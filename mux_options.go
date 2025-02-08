package gofi

import (
	"errors"
	"reflect"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/validators"
)

type muxOptions struct {
	errHandler         func(err error, c Context)
	customValidators   map[string]validators.ValidatorFnOptions
	customSchema       CustomSchemaTypes
	serializers        SerializerFn
	builtinSerializers SerializerFn
	logger             Logger
}

func defaultMuxOptions() *muxOptions {
	return &muxOptions{
		errHandler:         defaultErrorHandler,
		customValidators:   map[string]validators.ValidatorFnOptions{},
		customSchema:       map[string]CustomSchemaType{},
		serializers:        nil,
		builtinSerializers: builtinSerializer,
		logger:             &consoleLogger{},
	}
}

func (m *muxOptions) getSerializer(contentType cont.ContentType) (SchemaEncoder, error) {
	var sz SchemaEncoder
	var found bool

	if m.serializers != nil {
		sz, found = m.serializers(contentType)
	}
	if !found {
		sz, found = m.builtinSerializers(contentType)
		if !found {
			return nil, errors.New("schema serializer not defined")
		}
	}

	return sz, nil
}

type CustomSchemaTypes map[string]CustomSchemaType

// type SerializerFn map[cont.ContentType]SchemaEncoder
type SerializerFn func(cont.ContentType) (SchemaEncoder, bool)

type CustomSchemaType interface {
	IsCustomType(typ reflect.Type) (*CustomSchemaProps, bool)
	CustomEncode(val any) (any, error)
	CustomDecode(obj any) (string, error)
}

type CustomSchemaProps struct {
	Type   string
	Format string
}

func builtinSerializer(ct cont.ContentType) (SchemaEncoder, bool) {
	switch ct {
	case cont.ApplicationJson:
		return &JSONSchemaEncoder{}, true
	default:
		return nil, false
	}
}
