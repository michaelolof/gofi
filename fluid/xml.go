package fluid

// XMLObject represents an OpenAPI 3.0.3 XML Object.
// Provides metadata about how an XML property is serialized.
// See: https://spec.openapis.org/oas/v3.0.3#xml-object
type XMLObject struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Attribute bool   `json:"attribute,omitempty"`
	Wrapped   bool   `json:"wrapped,omitempty"`
}

// ExternalDocsObject represents an OpenAPI 3.0.3 External Documentation Object.
// See: https://spec.openapis.org/oas/v3.0.3#external-documentation-object
type ExternalDocsObject struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
}
