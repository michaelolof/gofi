package fluid

// SchemaObject represents an OpenAPI 3.0.3 Schema Object.
//
// When Ref is set (via RefSchema), it represents a $ref reference and
// all other fields should be left at their zero values. The JSON output
// will be {"$ref": "..."}.
//
// Const is not part of the OpenAPI 3.0.3 spec (it was introduced in 3.1
// via full JSON Schema 2020-12 support) but is included here as a
// widely-supported de-facto extension for expressing single-value
// schemas without resorting to a single-element Enum.
//
// See: https://spec.openapis.org/oas/v3.0.3#schema-object
type SchemaObject struct {
	Ref                  string                  `json:"$ref,omitempty"`
	Title                string                  `json:"title,omitempty"`
	Type                 string                  `json:"type,omitempty"`
	Format               string                  `json:"format,omitempty"`
	Description          string                  `json:"description,omitempty"`
	Default              any                     `json:"default,omitempty"`
	Example              any                     `json:"example,omitempty"`
	Enum                 []any                   `json:"enum,omitempty"`
	Const                any                     `json:"const,omitempty"`
	Minimum              *float64                `json:"minimum,omitempty"`
	Maximum              *float64                `json:"maximum,omitempty"`
	ExclusiveMinimum     *float64                `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     *float64                `json:"exclusiveMaximum,omitempty"`
	MinLength            *int                    `json:"minLength,omitempty"`
	MaxLength            *int                    `json:"maxLength,omitempty"`
	MinItems             *int                    `json:"minItems,omitempty"`
	MaxItems             *int                    `json:"maxItems,omitempty"`
	MinProperties        *int                    `json:"minProperties,omitempty"`
	MaxProperties        *int                    `json:"maxProperties,omitempty"`
	Pattern              string                  `json:"pattern,omitempty"`
	MultipleOf           *float64                `json:"multipleOf,omitempty"`
	UniqueItems          bool                    `json:"uniqueItems,omitempty"`
	Required             []string                `json:"required,omitempty"`
	Properties           map[string]SchemaObject `json:"properties,omitempty"`
	AdditionalProperties *SchemaObject           `json:"additionalProperties,omitempty"`
	Items                *SchemaObject           `json:"items,omitempty"`
	OneOf                []SchemaObject          `json:"oneOf,omitempty"`
	AnyOf                []SchemaObject          `json:"anyOf,omitempty"`
	AllOf                []SchemaObject          `json:"allOf,omitempty"`
	Not                  *SchemaObject           `json:"not,omitempty"`
	Discriminator        *DiscriminatorObject    `json:"discriminator,omitempty"`
	Nullable             bool                    `json:"nullable,omitempty"`
	Deprecated           *bool                   `json:"deprecated,omitempty"`
	ReadOnly             bool                    `json:"readOnly,omitempty"`
	WriteOnly            bool                    `json:"writeOnly,omitempty"`
	XML                  *XMLObject              `json:"xml,omitempty"`
	ExternalDocs         *ExternalDocsObject     `json:"externalDocs,omitempty"`
}

// ---- Convenience Constructors ----

// StringSchema returns a SchemaObject with Type "string".
func StringSchema() SchemaObject {
	return SchemaObject{Type: "string"}
}

// IntegerSchema returns a SchemaObject with Type "integer".
func IntegerSchema() SchemaObject {
	return SchemaObject{Type: "integer"}
}

// NumberSchema returns a SchemaObject with Type "number".
func NumberSchema() SchemaObject {
	return SchemaObject{Type: "number"}
}

// BooleanSchema returns a SchemaObject with Type "boolean".
func BooleanSchema() SchemaObject {
	return SchemaObject{Type: "boolean"}
}

// ArraySchema returns a SchemaObject with Type "array".
func ArraySchema(items SchemaObject) SchemaObject {
	return SchemaObject{Type: "array", Items: &items}
}

// ObjectSchema returns a SchemaObject with Type "object" and the given properties.
// Pass nil for properties to create an object with no fixed properties
// (useful with AdditionalProperties for free-form maps).
func ObjectSchema(properties map[string]SchemaObject) SchemaObject {
	return SchemaObject{Type: "object", Properties: properties}
}

// RefSchema returns a SchemaObject representing a $ref reference.
// Example: RefSchema("#/components/schemas/User")
//
// The returned SchemaObject will marshal to {"$ref": "..."}.
// Do not chain additional builders onto a RefSchema — per the
// OpenAPI spec, $ref replaces the entire schema object.
func RefSchema(ref string) SchemaObject {
	return SchemaObject{Ref: ref}
}

// DateTimeSchema returns a SchemaObject with Type "string" and Format "date-time".
func DateTimeSchema() SchemaObject {
	return SchemaObject{Type: "string", Format: "date-time"}
}

// DateSchema returns a SchemaObject with Type "string" and Format "date".
func DateSchema() SchemaObject {
	return SchemaObject{Type: "string", Format: "date"}
}

// BinarySchema returns a SchemaObject with Type "string" and Format "binary".
func BinarySchema() SchemaObject {
	return SchemaObject{Type: "string", Format: "binary"}
}

// ByteSchema returns a SchemaObject with Type "string" and Format "byte" (base64 encoded).
func ByteSchema() SchemaObject {
	return SchemaObject{Type: "string", Format: "byte"}
}

// PasswordSchema returns a SchemaObject with Type "string" and Format "password".
func PasswordSchema() SchemaObject {
	return SchemaObject{Type: "string", Format: "password"}
}

// EmailSchema returns a SchemaObject with Type "string" and Format "email".
func EmailSchema() SchemaObject {
	return SchemaObject{Type: "string", Format: "email"}
}

// URISchema returns a SchemaObject with Type "string" and Format "uri".
func URISchema() SchemaObject {
	return SchemaObject{Type: "string", Format: "uri"}
}

// UUIDSchema returns a SchemaObject with Type "string" and Format "uuid".
func UUIDSchema() SchemaObject {
	return SchemaObject{Type: "string", Format: "uuid"}
}

// ---- Builder Methods ----

// WithDescription sets the description.
func (s SchemaObject) WithDescription(desc string) SchemaObject {
	s.Description = desc
	return s
}

// WithExample sets an example value.
func (s SchemaObject) WithExample(ex any) SchemaObject {
	s.Example = ex
	return s
}

// WithDefault sets a default value.
func (s SchemaObject) WithDefault(def any) SchemaObject {
	s.Default = def
	return s
}

// WithEnum sets the enum values.
func (s SchemaObject) WithEnum(values ...any) SchemaObject {
	s.Enum = values
	return s
}

// WithConst sets the const value, constraining the schema to a single
// literal value. See the Const field docs for spec-conformance notes.
func (s SchemaObject) WithConst(value any) SchemaObject {
	s.Const = value
	return s
}

// WithNullable marks the schema as nullable.
func (s SchemaObject) WithNullable() SchemaObject {
	s.Nullable = true
	return s
}

// WithDeprecated marks the schema as deprecated.
func (s SchemaObject) WithDeprecated(deprecated bool) SchemaObject {
	s.Deprecated = &deprecated
	return s
}

// WithMinimum sets the minimum value constraint.
func (s SchemaObject) WithMinimum(min *float64) SchemaObject {
	s.Minimum = min
	return s
}

// WithMaximum sets the maximum value constraint.
func (s SchemaObject) WithMaximum(max *float64) SchemaObject {
	s.Maximum = max
	return s
}

// WithExclusiveMinimum sets the exclusiveMinimum constraint.
func (s SchemaObject) WithExclusiveMinimum(min *float64) SchemaObject {
	s.ExclusiveMinimum = min
	return s
}

// WithExclusiveMaximum sets the exclusiveMaximum constraint.
func (s SchemaObject) WithExclusiveMaximum(max *float64) SchemaObject {
	s.ExclusiveMaximum = max
	return s
}

// WithMinLength sets the minLength constraint (for strings).
func (s SchemaObject) WithMinLength(min *int) SchemaObject {
	s.MinLength = min
	return s
}

// WithMaxLength sets the maxLength constraint (for strings).
func (s SchemaObject) WithMaxLength(max *int) SchemaObject {
	s.MaxLength = max
	return s
}

// WithMinItems sets the minItems constraint (for arrays).
func (s SchemaObject) WithMinItems(min *int) SchemaObject {
	s.MinItems = min
	return s
}

// WithMaxItems sets the maxItems constraint (for arrays).
func (s SchemaObject) WithMaxItems(max *int) SchemaObject {
	s.MaxItems = max
	return s
}

// WithPattern sets the regex pattern constraint (for strings).
func (s SchemaObject) WithPattern(pattern string) SchemaObject {
	s.Pattern = pattern
	return s
}

// WithFormat sets the format field.
func (s SchemaObject) WithFormat(format string) SchemaObject {
	s.Format = format
	return s
}

// WithRequired sets the required fields (for objects).
func (s SchemaObject) WithRequired(fields ...string) SchemaObject {
	s.Required = fields
	return s
}

// WithAdditionalProperties sets the schema for additional properties (for objects).
func (s SchemaObject) WithAdditionalProperties(schema SchemaObject) SchemaObject {
	s.AdditionalProperties = &schema
	return s
}

// WithProperties sets/replaces all properties at once (for objects).
func (s SchemaObject) WithProperties(props map[string]SchemaObject) SchemaObject {
	s.Properties = props
	return s
}

// WithReadOnly marks the schema as read-only.
func (s SchemaObject) WithReadOnly() SchemaObject {
	s.ReadOnly = true
	return s
}

// WithWriteOnly marks the schema as write-only.
func (s SchemaObject) WithWriteOnly() SchemaObject {
	s.WriteOnly = true
	return s
}

// WithUniqueItems marks array items as unique.
func (s SchemaObject) WithUniqueItems() SchemaObject {
	s.UniqueItems = true
	return s
}

// WithMultipleOf sets the multipleOf constraint (for numbers).
func (s SchemaObject) WithMultipleOf(m *float64) SchemaObject {
	s.MultipleOf = m
	return s
}

// WithMinProperties sets the minProperties constraint (for objects).
func (s SchemaObject) WithMinProperties(min *int) SchemaObject {
	s.MinProperties = min
	return s
}

// WithMaxProperties sets the maxProperties constraint (for objects).
func (s SchemaObject) WithMaxProperties(max *int) SchemaObject {
	s.MaxProperties = max
	return s
}

// WithExternalDocs attaches external documentation.
func (s SchemaObject) WithExternalDocs(docs ExternalDocsObject) SchemaObject {
	s.ExternalDocs = &docs
	return s
}

// WithXML attaches XML serialization hints.
func (s SchemaObject) WithXML(xml XMLObject) SchemaObject {
	s.XML = &xml
	return s
}

// WithDiscriminator attaches a discriminator for polymorphism.
func (s SchemaObject) WithDiscriminator(d DiscriminatorObject) SchemaObject {
	s.Discriminator = &d
	return s
}
