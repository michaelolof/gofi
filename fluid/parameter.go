package fluid

// ParameterObject represents an OpenAPI 3.0.3 Parameter Object.
// See: https://spec.openapis.org/oas/v3.0.3#parameter-object
type ParameterObject struct {
	Name            string                    `json:"name"`
	In              string                    `json:"in"` // "query", "header", "path", "cookie"
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

// ---- Convenience Constructors ----

// QueryParameter creates a ParameterObject in "query".
func QueryParameter(name string, schema SchemaObject) ParameterObject {
	return ParameterObject{
		Name:   name,
		In:     "query",
		Schema: &schema,
	}
}

// PathParameter creates a ParameterObject in "path".
// Path parameters are always required in OpenAPI.
func PathParameter(name string, schema SchemaObject) ParameterObject {
	return ParameterObject{
		Name:     name,
		In:       "path",
		Required: BoolPtr(true),
		Schema:   &schema,
	}
}

// HeaderParameter creates a ParameterObject in "header".
func HeaderParameter(name string, schema SchemaObject) ParameterObject {
	return ParameterObject{
		Name:   name,
		In:     "header",
		Schema: &schema,
	}
}

// CookieParameter creates a ParameterObject in "cookie".
func CookieParameter(name string, schema SchemaObject) ParameterObject {
	return ParameterObject{
		Name:   name,
		In:     "cookie",
		Schema: &schema,
	}
}

// ---- Builder Methods ----

// WithDescription sets the description.
func (p ParameterObject) WithDescription(desc string) ParameterObject {
	p.Description = desc
	return p
}

// WithRequired marks the parameter as required.
func (p ParameterObject) WithRequired(required bool) ParameterObject {
	p.Required = &required
	return p
}

// WithDeprecated marks the parameter as deprecated.
func (p ParameterObject) WithDeprecated(deprecated bool) ParameterObject {
	p.Deprecated = &deprecated
	return p
}

// WithExample sets an example value.
func (p ParameterObject) WithExample(ex any) ParameterObject {
	p.Example = ex
	return p
}

// WithStyle sets the serialization style.
func (p ParameterObject) WithStyle(style string) ParameterObject {
	p.Style = style
	return p
}

// WithExplode sets the explode flag.
func (p ParameterObject) WithExplode(explode bool) ParameterObject {
	p.Explode = &explode
	return p
}

// WithAllowEmptyValue allows empty values to be sent.
func (p ParameterObject) WithAllowEmptyValue() ParameterObject {
	p.AllowEmptyValue = true
	return p
}

// WithAllowReserved allows reserved characters without percent-encoding.
func (p ParameterObject) WithAllowReserved() ParameterObject {
	p.AllowReserved = true
	return p
}

// WithExamples sets named examples.
func (p ParameterObject) WithExamples(examples map[string]ExampleObject) ParameterObject {
	p.Examples = examples
	return p
}

// WithContent sets content for complex serialization (replaces Schema).
func (p ParameterObject) WithContent(content map[string]MediaTypeObject) ParameterObject {
	p.Content = content
	return p
}

// Ptr returns a pointer to v. Generic helper for optional OpenAPI fields.
func Ptr[T any](v T) *T {
	return &v
}

// IntPtr returns a pointer to an int. Convenience for optional OpenAPI fields.
func IntPtr(i int) *int {
	return &i
}

// FloatPtr returns a pointer to a float64. Convenience for optional OpenAPI fields.
func FloatPtr(f float64) *float64 {
	return &f
}

// BoolPtr returns a pointer to a bool. Convenience for optional OpenAPI fields.
func BoolPtr(b bool) *bool {
	return &b
}
