package fluid

// HeaderObject represents an OpenAPI 3.0.3 Header Object.
// Follows the same structure as ParameterObject but without "name" and "in".
// See: https://spec.openapis.org/oas/v3.0.3#header-object
type HeaderObject struct {
	Description     string                    `json:"description,omitempty"`
	Required        *bool                     `json:"required,omitempty"`
	Deprecated      *bool                     `json:"deprecated,omitempty"`
	AllowEmptyValue bool                      `json:"allowEmptyValue,omitempty"`
	Style           string                    `json:"style,omitempty"`
	Explode         *bool                     `json:"explode,omitempty"`
	AllowReserved   bool                      `json:"allowReserved,omitempty"`
	Schema          *SchemaObject             `json:"schema,omitempty"`
	Example         any                       `json:"example,omitempty"`
	Examples        map[string]ExampleObject  `json:"examples,omitempty"`
	Content         map[string]MediaTypeObject `json:"content,omitempty"`
}

// ---- Builder Methods ----

// WithDescription sets the description.
func (h HeaderObject) WithDescription(desc string) HeaderObject {
	h.Description = desc
	return h
}

// WithRequired marks the header as required.
func (h HeaderObject) WithRequired(required bool) HeaderObject {
	h.Required = &required
	return h
}

// WithDeprecated marks the header as deprecated.
func (h HeaderObject) WithDeprecated(deprecated bool) HeaderObject {
	h.Deprecated = &deprecated
	return h
}

// WithSchema sets the schema for this header.
func (h HeaderObject) WithSchema(schema SchemaObject) HeaderObject {
	h.Schema = &schema
	return h
}

// WithExample sets an example value.
func (h HeaderObject) WithExample(ex any) HeaderObject {
	h.Example = ex
	return h
}

// WithExamples sets named examples.
func (h HeaderObject) WithExamples(examples map[string]ExampleObject) HeaderObject {
	h.Examples = examples
	return h
}
