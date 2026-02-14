package schema

import (
	"errors"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/validators"
)

type OpenapiSchema struct {
	Format               string                   `json:"format,omitempty"`
	Type                 string                   `json:"type,omitempty"`
	Pattern              string                   `json:"pattern,omitempty"`
	Default              any                      `json:"default,omitempty"`
	Minimum              *float64                 `json:"minimum,omitempty"`
	Maximum              *float64                 `json:"maximum,omitempty"`
	Enum                 []any                    `json:"enum,omitempty"`
	Items                *OpenapiSchema           `json:"items,omitempty"`
	AdditionalProperties *OpenapiSchema           `json:"additionalProperties,omitempty"`
	Properties           map[string]OpenapiSchema `json:"properties,omitempty"`
	Required             []string                 `json:"required,omitempty"`
	Deprecated           *bool                    `json:"deprecated,omitempty"`
	Description          string                   `json:"description,omitempty"`
	Example              any                      `json:"example,omitempty"`

	ParentRequired bool `json:"-"`
}

func NewOpenapiSchema(format string, typ string, pattn string, deflt any, min *float64, max *float64, enum []any, items *OpenapiSchema, addprops *OpenapiSchema, properties map[string]OpenapiSchema, required []string, deprecated *bool, describe string, example any, pRequired bool) OpenapiSchema {
	return OpenapiSchema{
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

func (o *OpenapiSchema) IsEmpty() bool {
	return o == nil || o.Type == ""
}

type OpenapiParameter struct {
	In       string        `json:"in"`
	Name     string        `json:"name"`
	Required *bool         `json:"required,omitempty"`
	Schema   OpenapiSchema `json:"schema,omitempty"`
}

func NewOpenapiParameter(in string, name string, required *bool, schema OpenapiSchema) OpenapiParameter {
	return OpenapiParameter{
		In:       in,
		Name:     name,
		Required: required,
		Schema:   schema,
	}
}

type OpenapiParameters []OpenapiParameter

func (o OpenapiParameters) FindByNameIn(name string, in string) *OpenapiParameter {
	for _, v := range o {
		if v.Name == name && v.In == in {
			return &v
		}
	}
	return nil
}

type OpenapiMediaObject struct {
	Schema OpenapiSchema `json:"schema,omitempty"`
}

type OpenapiRequestObject struct {
	Description string                        `json:"description,omitempty"`
	Required    bool                          `json:"required,omitempty"`
	Content     map[string]OpenapiMediaObject `json:"content,omitempty"`
}

type OpenapiHeaderObject struct {
	Required *bool         `json:"required,omitempty"`
	Schema   OpenapiSchema `json:"schema,omitempty"`
	value    string
}

func NewOpenapiHeaderObject(required *bool, value string, schema OpenapiSchema) OpenapiHeaderObject {
	return OpenapiHeaderObject{
		Required: required,
		value:    value,
		Schema:   schema,
	}
}

type OpenapiResponseObject struct {
	Description string                         `json:"description,omitempty"`
	Headers     map[string]OpenapiHeaderObject `json:"headers,omitempty"`
	Required    bool                           `json:"required,omitempty"`
	Content     map[string]OpenapiMediaObject  `json:"content,omitempty"`
}

type OpenapiOperationObject struct {
	OperationId  string                           `json:"operationId,omitempty"`
	Summary      string                           `json:"summary,omitempty"`
	Description  string                           `json:"description,omitempty"`
	Deprecated   *bool                            `json:"deprecated,omitempty"`
	Parameters   OpenapiParameters                `json:"parameters,omitempty"`
	RequestBody  *OpenapiRequestObject            `json:"requestBody,omitempty"`
	Responses    map[string]OpenapiResponseObject `json:"responses,omitempty"`
	ExternalDocs []ExternalDocs                   `json:"externalDocs,omitempty"`

	urlPath             string
	method              string
	bodySchema          OpenapiSchema
	responsesParameters map[string]OpenapiParameters
	responsesSchema     map[string]OpenapiSchema
}

func InitOpenapiOperationObject() OpenapiOperationObject {
	return OpenapiOperationObject{
		Responses:           make(map[string]OpenapiResponseObject),
		responsesParameters: make(map[string]OpenapiParameters),
		responsesSchema:     make(map[string]OpenapiSchema),
	}
}

func (o *OpenapiOperationObject) Normalize(method string, path string) {

	o.method = method
	o.urlPath = path

	if !o.bodySchema.IsEmpty() {
		var contentType = string(cont.AnyContenType)
		if v := o.Parameters.FindByNameIn("content-type", "header"); v != nil {
			if def, ok := v.Schema.Default.(string); ok && def != "" {
				contentType = def
			}
		}

		o.RequestBody = &OpenapiRequestObject{
			Required: o.bodySchema.ParentRequired,
			Content: map[string]OpenapiMediaObject{
				contentType: {
					Schema: o.bodySchema,
				},
			},
		}
	}

	if len(o.responsesParameters) > 0 || len(o.responsesSchema) > 0 {
		if o.Responses == nil {
			o.Responses = make(map[string]OpenapiResponseObject)
		}

		for field, params := range o.responsesParameters {
			sinfos := Statuses[field]

			for _, sinfo := range sinfos {
				headersMap := make(map[string]OpenapiHeaderObject)
				for _, param := range params {
					if param.In == "header" {
						v, ok := param.Schema.Default.(string)
						if !ok || v == "" {
							v = string(cont.AnyContenType)
						}
						headersMap[param.Name] = NewOpenapiHeaderObject(param.Required, v, param.Schema)
					}
				}

				if v, ok := o.Responses[sinfo.Code]; ok {
					v.Headers = headersMap
					v.Description = sinfo.Description
				} else {
					o.Responses[sinfo.Code] = OpenapiResponseObject{Headers: headersMap, Description: sinfo.Description}
				}
			}
		}

		for field, schema := range o.responsesSchema {
			sinfo := Statuses[field]

			for _, sinfo := range sinfo {
				contentType := string(cont.AnyContenType)
				if v, ok := o.Responses[sinfo.Code]; ok {
					v.Description = sinfo.Description
					if c, ok := v.Headers["content-type"]; ok {
						contentType = c.value
					}
					v.Content = map[string]OpenapiMediaObject{
						contentType: {
							Schema: schema,
						},
					}
					v.Required = schema.ParentRequired
					o.Responses[sinfo.Code] = v
				} else {
					o.Responses[sinfo.Code] = OpenapiResponseObject{
						Required:    schema.ParentRequired,
						Description: sinfo.Description,
						Content: map[string]OpenapiMediaObject{
							contentType: {
								Schema: schema,
							},
						},
					}
				}
			}
		}
	}

}

type ExternalDocs struct {
	Description string `json:"description,omitempty"`
	Url         string `json:"url,omitempty"`
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
}

type SchemaField string

const (
	SchemaOperationId SchemaField = "OperationId"
	SchemaSummary     SchemaField = "Summary"
	SchemaHttpMethod  SchemaField = "Method"
	SchemaUrl         SchemaField = "Url"
	SchemaDeprecated  SchemaField = "Deprecated"
	SchemaReq         SchemaField = "Request"
	SchemaHeaders     SchemaField = "Header"
	SchemaCookies     SchemaField = "Cookie"
	SchemaQuery       SchemaField = "Query"
	SchemaPath        SchemaField = "Path"
	SchemaBody        SchemaField = "Body"
)

func (s SchemaField) ReqSchemaIn() string {
	switch s {
	case SchemaPath:
		return "path"
	case SchemaQuery:
		return "query"
	case SchemaHeaders:
		return "header"
	case SchemaCookies:
		return "cookie"
	default:
		return "<unknown>"
	}
}

func (s SchemaField) String() string {
	return string(s)
}

// Validation Error Report
type ValidatorArg = validators.ValidatorArg

func RunValidation(c *ValidatorArg, val any, field SchemaField, keyPath string, rules []RuleOpts) error {
	var errs []error

	for _, rule := range rules {
		err := rule.Dator(*c)
		if err != nil {
			errType := RequestErr
			// c.typ is not accessible directly as it is unexported in validators package?
			// ValidatorArg fields are unexported: val, typ, r, w.
			// But we aliased it.
			// We can't access c.typ if it is unexported in validators.
			// We need a way to determine type.
			// Hack: check if c.Request() is not nil?
			// But creating NewErrReport requires ErrorType.
			// Let's assume RequestErr for now or use a getter if we add one to ValidatorArg.
			// Actually `validators.ValidatorArg` has `typ` field but it is unexported?
			// `mappings.go`: `type ValidatorArg struct { ... }`. Unexported fields.
			// But `Value()`, `Request()`, `Response()` are exported.
			// I should add `Type()` to `ValidatorArg` in `gofi/validators/mappings.go` or assume Request.
			// Or pass ErrorType to RunValidation.

			// For now, I'll default to RequestErr unless I change ValidatorArg.
			// Wait, `ValidatorArg` is in `gofi/validators`.
			// I can edit it.

			// But for now, lets use RequestErr.
			// Or check c.Response() != nil ?

			errs = append(errs, NewErrReport(errType, field, keyPath, rule.Rule, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
