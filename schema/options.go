package schema

import (
	"io"
	"reflect"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/validators"
)

type ValidationOptions struct {
	ErrHandler       func(err error, r any) // r can be *http.Request or Context
	CustomValidators validators.ContextValidators
	CustomSpecs      CustomSpecs
	Serializers      SerializerFn
	// Logger             Logger // Need to define Logger interface or use standard log
}

// RequestValidationOptions holds context for request validation/decoding
type RequestValidationOptions struct {
	ShouldEncode      bool
	Context           io.Writer // Used for writing errors if needed, or MaxBytesReader. In http case: http.ResponseWriter
	SchemaField       SchemaField
	SchemaPtrInstance any
	SchemaRules       *RuleDef
	FieldStruct       *reflect.Value
}

// ResponseValidationOptions holds context for response validation/encoding
type ResponseValidationOptions struct {
	Context     any // Can be used for custom logic
	SchemaRules *RuleDef
	Body        reflect.Value
}

type SchemaEncoder interface {
	ValidateAndEncode(obj any, opts ResponseValidationOptions) ([]byte, error)
	ValidateAndDecode(reader io.ReadCloser, opts RequestValidationOptions) (err error)
}

type SerializerFn func(cont.ContentType) (SchemaEncoder, bool)

type CustomSpecs map[string]CustomSchemaProps

type CustomSchemaProps struct {
	// Add a custom decoder. Will defer to the json.Decoder if not passed. It is advised to use the json Unmarshal method. Prefer this if you don't have access to the custom type
	Decoder func(val any) (any, error) `json:"-"`
	// Add a custom encoder. Will defer to the json.Encode if not passed. It is advised to use the json Marshal method. Prefer this if you don't have access to the custom type
	Encoder func(val any) (string, error) `json:"-"`
	// Define the openapi3 type for your custom type E.g "string", "integer", "number", 'boolean", "array" etc
	Type string `json:"type,omitempty"`
	// Define the openapi3 type for your custom type E.g "date", "date-time", "int32", 'int64", "uuie" etc
	Format string `json:"format,omitempty"`
}

type SchemaRulesMap map[string]map[string]*SchemaRules
