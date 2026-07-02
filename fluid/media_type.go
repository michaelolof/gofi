package fluid

// MediaTypeObject represents an OpenAPI 3.0.3 Media Type Object.
// Used by RequestBodyObject and ResponseObject.
// See: https://spec.openapis.org/oas/v3.0.3#media-type-object
type MediaTypeObject struct {
	Schema   *SchemaObject             `json:"schema,omitempty"`
	Example  any                       `json:"example,omitempty"`
	Examples map[string]ExampleObject  `json:"examples,omitempty"`
	Encoding map[string]EncodingObject `json:"encoding,omitempty"`
}

// ---- Builder Methods ----

// WithSchema sets the schema for this media type.
func (m MediaTypeObject) WithSchema(s SchemaObject) MediaTypeObject {
	m.Schema = &s
	return m
}

// WithExample sets an example value.
func (m MediaTypeObject) WithExample(ex any) MediaTypeObject {
	m.Example = ex
	return m
}

// WithExamples sets named examples.
func (m MediaTypeObject) WithExamples(examples map[string]ExampleObject) MediaTypeObject {
	m.Examples = examples
	return m
}

// WithEncoding sets encoding overrides for multipart/form-data properties.
func (m MediaTypeObject) WithEncoding(encoding map[string]EncodingObject) MediaTypeObject {
	m.Encoding = encoding
	return m
}
