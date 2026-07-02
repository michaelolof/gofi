package fluid

// ExampleObject represents an OpenAPI 3.0.3 Example Object.
// See: https://spec.openapis.org/oas/v3.0.3#example-object
type ExampleObject struct {
	Summary       string `json:"summary,omitempty"`
	Description   string `json:"description,omitempty"`
	Value         any    `json:"value,omitempty"`
	ExternalValue string `json:"externalValue,omitempty"`
}
