package fluid

// ResponseObject represents an OpenAPI 3.0.3 Response Object.
// See: https://spec.openapis.org/oas/v3.0.3#response-object
type ResponseObject struct {
	Description string                    `json:"description"`
	Headers     map[string]HeaderObject   `json:"headers,omitempty"`
	Content     map[string]MediaTypeObject `json:"content,omitempty"`
	Links       map[string]any            `json:"links,omitempty"`
}

// ---- Convenience Constructors ----

// JSONResponse creates a ResponseObject with application/json content type.
func JSONResponse(description string, schema SchemaObject) ResponseObject {
	return ResponseObject{
		Description: description,
		Content: map[string]MediaTypeObject{
			"application/json": {
				Schema: &schema,
			},
		},
	}
}

// PlainTextResponse creates a ResponseObject with text/plain content type.
func PlainTextResponse(description string, schema SchemaObject) ResponseObject {
	return ResponseObject{
		Description: description,
		Content: map[string]MediaTypeObject{
			"text/plain": {
				Schema: &schema,
			},
		},
	}
}

// HTMLResponse creates a ResponseObject with text/html content type.
func HTMLResponse(description string) ResponseObject {
	return ResponseObject{
		Description: description,
		Content: map[string]MediaTypeObject{
			"text/html": {
				Schema: &SchemaObject{Type: "string"},
			},
		},
	}
}

// BinaryResponse creates a ResponseObject for binary content (application/octet-stream).
func BinaryResponse(description string) ResponseObject {
	return ResponseObject{
		Description: description,
		Content: map[string]MediaTypeObject{
			"application/octet-stream": {
				Schema: &SchemaObject{Type: "string", Format: "binary"},
			},
		},
	}
}

// ---- Builder Methods ----

// WithDescription sets the description.
func (r ResponseObject) WithDescription(desc string) ResponseObject {
	r.Description = desc
	return r
}

// WithHeaders adds headers to the response.
func (r ResponseObject) WithHeaders(headers map[string]HeaderObject) ResponseObject {
	r.Headers = headers
	return r
}

// WithContent sets the content map directly.
func (r ResponseObject) WithContent(content map[string]MediaTypeObject) ResponseObject {
	r.Content = content
	return r
}

// WithJSONContent is a convenience that sets application/json content.
func (r ResponseObject) WithJSONContent(schema SchemaObject) ResponseObject {
	if r.Content == nil {
		r.Content = make(map[string]MediaTypeObject)
	}
	r.Content["application/json"] = MediaTypeObject{Schema: &schema}
	return r
}
