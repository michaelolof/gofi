package gofi

import (
	"encoding/json"
	"errors"
	"math"

	"github.com/michaelolof/gofi/cont"
)

type openapiSchema struct {
	Title                string                   `json:"title,omitempty"`
	Format               string                   `json:"format,omitempty"`
	Type                 string                   `json:"type,omitempty"`
	Pattern              string                   `json:"pattern,omitempty"`
	Default              any                      `json:"default,omitempty"`
	Minimum              *float64                 `json:"minimum,omitempty"`
	Maximum              *float64                 `json:"maximum,omitempty"`
	MinLength            *uint64                  `json:"minLength,omitempty"`
	MaxLength            *uint64                  `json:"maxLength,omitempty"`
	MinItems             *uint64                  `json:"minItems,omitempty"`
	MaxItems             *uint64                  `json:"maxItems,omitempty"`
	MinProperties        *uint64                  `json:"minProperties,omitempty"`
	MaxProperties        *uint64                  `json:"maxProperties,omitempty"`
	Enum                 []any                    `json:"enum,omitempty"`
	OneOf                []openapiSchema          `json:"oneOf,omitempty"`
	Discriminator        *openapiDiscriminator    `json:"discriminator,omitempty"`
	Items                *openapiSchema           `json:"items,omitempty"`
	AdditionalProperties *openapiSchema           `json:"additionalProperties,omitempty"`
	Properties           map[string]openapiSchema `json:"properties,omitempty"`
	Required             []string                 `json:"required,omitempty"`
	Deprecated           *bool                    `json:"deprecated,omitempty"`
	Description          string                   `json:"description,omitempty"`
	Example              any                      `json:"example,omitempty"`

	ParentRequired bool `json:"-"`
}

type openapiDiscriminator struct {
	PropertyName string            `json:"propertyName,omitempty"`
	Mapping      map[string]string `json:"mapping,omitempty"`
}

func newOpenapiSchema(format string, typ string, pattn string, deflt any, min *float64, max *float64, enum []any, items *openapiSchema, addprops *openapiSchema, properties map[string]openapiSchema, required []string, deprecated *bool, describe string, example any, pRequired bool) openapiSchema {
	return openapiSchema{
		Format:               format,
		Type:                 typ,
		Pattern:              pattn,
		Default:              deflt,
		Minimum:              min,
		Maximum:              max,
		Enum:                 enum,
		Items:                items,
		AdditionalProperties: addprops,
		Properties:           properties,
		Required:             required,
		Deprecated:           deprecated,
		Description:          describe,
		Example:              example,
		ParentRequired:       pRequired,
	}
}

// setBounds documents min/max rules with the keyword OpenAPI uses for the
// schema's type: minimum/maximum for numbers, minLength/maxLength for
// strings, minItems/maxItems for arrays and minProperties/maxProperties for
// maps. Gofi's runtime rules apply to the same quantities.
func (o *openapiSchema) setBounds(min, max *float64, isTime bool) {
	o.Minimum, o.Maximum = nil, nil
	switch o.Type {
	case "integer", "number":
		o.Minimum, o.Maximum = min, max
	case "string":
		if isTime || o.Format == "binary" {
			return
		}
		o.MinLength, o.MaxLength = boundCount(min, true), boundCount(max, false)
	case "array":
		o.MinItems, o.MaxItems = boundCount(min, true), boundCount(max, false)
	case "object":
		if o.AdditionalProperties != nil {
			o.MinProperties, o.MaxProperties = boundCount(min, true), boundCount(max, false)
		}
	}
}

// boundCount converts a min (rounded up) or max (rounded down) rule value to
// a non-negative count.
func boundCount(v *float64, isMin bool) *uint64 {
	if v == nil {
		return nil
	}
	f := math.Floor(*v)
	if isMin {
		f = math.Ceil(*v)
	}
	if f < 0 {
		f = 0
	}
	n := uint64(f)
	return &n
}

func (o *openapiSchema) IsEmpty() bool {
	return o == nil || o.Type == ""
}

type openapiParameter struct {
	In       string        `json:"in"`
	Name     string        `json:"name"`
	Required *bool         `json:"required,omitempty"`
	Schema   openapiSchema `json:"schema,omitempty"`
}

func newOpenapiParameter(in string, name string, required *bool, schema openapiSchema) openapiParameter {
	return openapiParameter{
		In:       in,
		Name:     name,
		Required: required,
		Schema:   schema,
	}
}

type openapiParameters []openapiParameter

func (o openapiParameters) findByNameIn(name string, in string) *openapiParameter {
	for _, v := range o {
		if v.Name == name && v.In == in {
			return &v
		}
	}
	return nil
}

type openapiMediaObject struct {
	Schema openapiSchema `json:"schema,omitempty"`
}

type openapiRequestObject struct {
	Description string                        `json:"description,omitempty"`
	Required    bool                          `json:"required,omitempty"`
	Content     map[string]openapiMediaObject `json:"content,omitempty"`
}

type openapiHeaderObject struct {
	Required *bool         `json:"required,omitempty"`
	Schema   openapiSchema `json:"schema,omitempty"`
	value    string
}

func newOpenapiHeaderObject(required *bool, value string, schema openapiSchema) openapiHeaderObject {
	return openapiHeaderObject{
		Required: required,
		value:    value,
		Schema:   schema,
	}
}

type openapiResponseObject struct {
	Description string                         `json:"description,omitempty"`
	Headers     map[string]openapiHeaderObject `json:"headers,omitempty"`
	Required    bool                           `json:"required,omitempty"`
	Content     map[string]openapiMediaObject  `json:"content,omitempty"`
}

type openapiOperationObject struct {
	OperationId  string                           `json:"operationId,omitempty"`
	Summary      string                           `json:"summary,omitempty"`
	Description  string                           `json:"description,omitempty"`
	Deprecated   *bool                            `json:"deprecated,omitempty"`
	Parameters   openapiParameters                `json:"parameters,omitempty"`
	RequestBody  *openapiRequestObject            `json:"requestBody,omitempty"`
	Responses    map[string]openapiResponseObject `json:"responses,omitempty"`
	ExternalDocs []ExternalDocs                   `json:"externalDocs,omitempty"`
	Tags         []string                         `json:"tags,omitempty"`
	Security     *[]SecurityRequirement           `json:"security,omitempty"`

	urlPath    string
	method     string
	bodySchema openapiSchema
	// bodyDeclared records that the schema has a Request.Body field. A
	// free-form body (json.RawMessage, any) has a schema with no type, so
	// bodySchema alone cannot tell it apart from no body.
	bodyDeclared        bool
	websocketSchema     openapiSchema
	responsesParameters map[string]openapiParameters
	responsesSchema     map[string]openapiSchema
}

func initOpenapiOperationObject() openapiOperationObject {
	return openapiOperationObject{
		Responses:           make(map[string]openapiResponseObject),
		responsesParameters: make(map[string]openapiParameters),
		responsesSchema:     make(map[string]openapiSchema),
	}
}

func (o *openapiOperationObject) normalize(method string, path string) {

	o.method = method
	o.urlPath = path

	if o.bodyDeclared || !o.bodySchema.IsEmpty() {
		// Matches schemaRules.reqContent: bodies are JSON unless the schema
		// declares another content type.
		var contentType = string(cont.ApplicationJson)
		if v := o.Parameters.findByNameIn("content-type", "header"); v != nil {
			if def, ok := v.Schema.Default.(string); ok && def != "" {
				contentType = def
			}
		}

		o.RequestBody = &openapiRequestObject{
			Required: o.bodySchema.ParentRequired,
			Content: map[string]openapiMediaObject{
				contentType: {
					Schema: o.bodySchema,
				},
			},
		}
	}

	if len(o.responsesParameters) > 0 || len(o.responsesSchema) > 0 {
		if o.Responses == nil {
			o.Responses = make(map[string]openapiResponseObject)
		}

		for field, params := range o.responsesParameters {
			sinfos := statuses[field]

			for _, sinfo := range sinfos {
				headersMap := make(map[string]openapiHeaderObject)
				for _, param := range params {
					if param.In == "header" {
						v, ok := param.Schema.Default.(string)
						if !ok || v == "" {
							v = string(cont.AnyContenType)
						}
						headersMap[param.Name] = newOpenapiHeaderObject(param.Required, v, param.Schema)
					}
				}

				if v, ok := o.Responses[sinfo.Code]; ok {
					v.Headers = headersMap
					v.Description = sinfo.Description
				} else {
					o.Responses[sinfo.Code] = openapiResponseObject{Headers: headersMap, Description: sinfo.Description}
				}
			}
		}

		for field, schema := range o.responsesSchema {
			sinfo := statuses[field]

			for _, sinfo := range sinfo {
				// Matches schemaRules.respContent: responses are JSON unless
				// the schema declares another content type.
				contentType := string(cont.ApplicationJson)
				if v, ok := o.Responses[sinfo.Code]; ok {
					v.Description = sinfo.Description
					if c, ok := v.Headers["content-type"]; ok {
						contentType = c.value
					}
					v.Content = map[string]openapiMediaObject{
						contentType: {
							Schema: schema,
						},
					}
					v.Required = schema.ParentRequired
					o.Responses[sinfo.Code] = v
				} else {
					o.Responses[sinfo.Code] = openapiResponseObject{
						Required:    schema.ParentRequired,
						Description: sinfo.Description,
						Content: map[string]openapiMediaObject{
							contentType: {
								Schema: schema,
							},
						},
					}
				}
			}
		}
	}

	if !o.websocketSchema.IsEmpty() {
		if o.Responses == nil {
			o.Responses = make(map[string]openapiResponseObject)
		}

		response := o.Responses["101"]
		response.Description = "Switching Protocols Response. The schema below documents the post-upgrade websocket protocol."
		response.Required = o.websocketSchema.ParentRequired
		response.Content = map[string]openapiMediaObject{
			string(cont.ApplicationJson): {
				Schema: o.websocketSchema,
			},
		}
		o.Responses["101"] = response
	}

}

type Info struct {
	// Prevent path from being documented
	Hidden       bool
	OperationId  string
	Summary      string
	Deprecated   bool
	Method       string
	Url          string
	Description  string
	ExternalDocs []ExternalDocs
	Tags         []string
	// Security overrides DocsOptions.Security for this route. Leave it nil
	// to use the router-wide default; set it to NoSecurity for a route that
	// needs no credentials.
	Security []SecurityRequirement
}

// SecurityRequirement names the security schemes a route needs, mapped to
// the scopes required from each (empty for bearer, basic and API-key
// schemes). Every scheme named must be declared in DocsOptions.SecuritySchemes
// or a view's Components.SecuritySchemes.
type SecurityRequirement map[string][]string

// MarshalJSON writes a scheme with no scopes as [] rather than null, as
// OpenAPI requires.
func (r SecurityRequirement) MarshalJSON() ([]byte, error) {
	out := make(map[string][]string, len(r))
	for name, scopes := range r {
		if scopes == nil {
			scopes = []string{}
		}
		out[name] = scopes
	}
	return json.Marshal(out)
}

// NoSecurity marks a route as public in Info.Security: it is documented with
// an empty security requirement, overriding DocsOptions.Security.
var NoSecurity = []SecurityRequirement{}

type schemaField string

const (
	// schemaOperationId schemaField = "OperationId"
	// schemaSummary     schemaField = "Summary"
	// schemaHttpMethod  schemaField = "Method"
	// schemaUrl         schemaField = "Url"
	// schemaDeprecated  schemaField = "Deprecated"
	schemaReq       schemaField = "Request"
	schemaHeaders   schemaField = "Header"
	schemaCookies   schemaField = "Cookie"
	schemaQuery     schemaField = "Query"
	schemaPath      schemaField = "Path"
	schemaBody      schemaField = "Body"
	schemaWebSocket schemaField = "WebSocket"
)

func (s schemaField) reqSchemaIn() string {
	switch s {
	case schemaPath:
		return "path"
	case schemaQuery:
		return "query"
	case schemaHeaders:
		return "header"
	case schemaCookies:
		return "cookie"
	default:
		return "<unknown>"
	}
}

func (s schemaField) String() string {
	return string(s)
}

func runValidation(val any, typ errorType, schema schemaField, keypath string, rules []ruleOpts) error {
	if len(rules) == 0 {
		return nil
	}

	var errs []error
	for _, rule := range rules {
		if err := rule.dator(val); err != nil {
			errs = append(errs, newErrReport(typ, schema, keypath, rule.rule, err))
		}
	}

	return errors.Join(errs...)
}

// runValidationLazy is like runValidation but defers building the key path
// until a failure is detected. The JSON walker reuses one path stack across
// all recursive frames, so successful requests do not allocate path slices.
func runValidationLazy(val any, typ errorType, schema schemaField, path *requestPath, rules []ruleOpts) error {
	if len(rules) == 0 {
		return nil
	}

	var errs []error
	for _, rule := range rules {
		if err := rule.dator(val); err != nil {
			errs = append(errs, newErrReport(typ, schema, path.String(), rule.rule, err))
		}
	}

	return errors.Join(errs...)
}
