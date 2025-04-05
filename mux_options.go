package gofi

import (
	"errors"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/validators"
)

type muxOptions struct {
	errHandler         func(err error, c Context)
	customValidators   map[string]validators.ValidatorFnOptions
	customSpecs        CustomSpecs
	serializers        SerializerFn
	builtinSerializers SerializerFn
	logger             Logger
}

func defaultMuxOptions() *muxOptions {
	return &muxOptions{
		errHandler:       defaultErrorHandler,
		customValidators: map[string]validators.ValidatorFnOptions{},
		customSpecs:      map[string]CustomSchemaProps{},

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

// type SerializerFn map[cont.ContentType]SchemaEncoder
type SerializerFn func(cont.ContentType) (SchemaEncoder, bool)

func builtinSerializer(ct cont.ContentType) (SchemaEncoder, bool) {
	switch ct {
	case cont.ApplicationJson:
		return &JSONSchemaEncoder{}, true
	default:
		return nil, false
	}
}

type CustomSpecs map[string]CustomSchemaProps
type CustomSchemaProps struct {
	// Define the openapi3 type for your custom type E.g "string", "integer", "number", 'boolean", "array" etc
	Type string
	// Define the openapi3 type for your custom type E.g "date", "date-time", "int32", 'int64", "uuie" etc
	Format string
	// Add a custom decoder. Will defer to the json.Decoder if not passed. It is advised to use the json Unmarshal method. Prefer this if you don't have access to the custom type
	Decoder func(val any) (any, error)
	// Add a custom encoder. Will defer to the json.Encode if not passed. It is advised to use the json Marshal method. Prefer this if you don't have access to the custom type
	Encoder func(val any) (string, error)
}
