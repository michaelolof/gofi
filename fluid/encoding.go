package fluid

// EncodingObject represents an OpenAPI 3.0.3 Encoding Object.
// Defines how a specific property is serialized when the content type
// is multipart/form-data or application/x-www-form-urlencoded.
// See: https://spec.openapis.org/oas/v3.0.3#encoding-object
type EncodingObject struct {
	ContentType   string                  `json:"contentType,omitempty"`
	Headers       map[string]HeaderObject `json:"headers,omitempty"`
	Style         string                  `json:"style,omitempty"`
	Explode       *bool                   `json:"explode,omitempty"`
	AllowReserved bool                    `json:"allowReserved,omitempty"`
}
