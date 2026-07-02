package fluid

// RequestBodyObject represents an OpenAPI 3.0.3 Request Body Object.
// See: https://spec.openapis.org/oas/v3.0.3#request-body-object
type RequestBodyObject struct {
	Description string                    `json:"description,omitempty"`
	Required    bool                      `json:"required,omitempty"`
	Content     map[string]MediaTypeObject `json:"content"`
}

// ---- Convenience Constructors ----

// JSONRequestBody creates a RequestBodyObject with application/json content type.
func JSONRequestBody(schema SchemaObject) RequestBodyObject {
	return RequestBodyObject{
		Content: map[string]MediaTypeObject{
			"application/json": {
				Schema: &schema,
			},
		},
	}
}

// FormURLEncodedRequestBody creates a RequestBodyObject for
// application/x-www-form-urlencoded content.
func FormURLEncodedRequestBody(schema SchemaObject) RequestBodyObject {
	return RequestBodyObject{
		Content: map[string]MediaTypeObject{
			"application/x-www-form-urlencoded": {
				Schema: &schema,
			},
		},
	}
}

// MultipartRequestBody creates a RequestBodyObject for multipart/form-data content.
func MultipartRequestBody(schema SchemaObject) RequestBodyObject {
	return RequestBodyObject{
		Content: map[string]MediaTypeObject{
			"multipart/form-data": {
				Schema: &schema,
			},
		},
	}
}

// PlainTextRequestBody creates a RequestBodyObject for text/plain content.
func PlainTextRequestBody(schema SchemaObject) RequestBodyObject {
	return RequestBodyObject{
		Content: map[string]MediaTypeObject{
			"text/plain": {
				Schema: &schema,
			},
		},
	}
}

// ---- Builder Methods ----

// WithDescription sets the description.
func (r RequestBodyObject) WithDescription(desc string) RequestBodyObject {
	r.Description = desc
	return r
}

// WithRequired marks the request body as required.
func (r RequestBodyObject) WithRequired() RequestBodyObject {
	r.Required = true
	return r
}

// WithContent sets the content map directly.
func (r RequestBodyObject) WithContent(content map[string]MediaTypeObject) RequestBodyObject {
	r.Content = content
	return r
}

// WithJSONContent is a convenience that sets application/json content.
func (r RequestBodyObject) WithJSONContent(schema SchemaObject) RequestBodyObject {
	if r.Content == nil {
		r.Content = make(map[string]MediaTypeObject)
	}
	r.Content["application/json"] = MediaTypeObject{Schema: &schema}
	return r
}

// WithFormURLEncodedContent is a convenience that sets form-urlencoded content.
func (r RequestBodyObject) WithFormURLEncodedContent(schema SchemaObject) RequestBodyObject {
	if r.Content == nil {
		r.Content = make(map[string]MediaTypeObject)
	}
	r.Content["application/x-www-form-urlencoded"] = MediaTypeObject{Schema: &schema}
	return r
}

// WithMultipartContent is a convenience that sets multipart/form-data content.
func (r RequestBodyObject) WithMultipartContent(schema SchemaObject) RequestBodyObject {
	if r.Content == nil {
		r.Content = make(map[string]MediaTypeObject)
	}
	r.Content["multipart/form-data"] = MediaTypeObject{Schema: &schema}
	return r
}
