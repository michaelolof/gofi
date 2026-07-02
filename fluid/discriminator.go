package fluid

// DiscriminatorObject represents an OpenAPI 3.0.3 Discriminator Object.
// Used with oneOf/anyOf for polymorphic schemas.
// See: https://spec.openapis.org/oas/v3.0.3#discriminator-object
type DiscriminatorObject struct {
	PropertyName string            `json:"propertyName"`
	Mapping      map[string]string `json:"mapping,omitempty"`
}
