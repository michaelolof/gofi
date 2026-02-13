package gofi

import (
	"errors"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/validators"
)

type openapiSchema struct {
	Format               string                   `json:"format,omitempty"`
	Type                 string                   `json:"type,omitempty"`
	Pattern              string                   `json:"pattern,omitempty"`
	Default              any                      `json:"default,omitempty"`
	Minimum              *float64                 `json:"minimum,omitempty"`
	Maximum              *float64                 `json:"maximum,omitempty"`
	Enum                 []any                    `json:"enum,omitempty"`
	Items                *openapiSchema           `json:"items,omitempty"`
	AdditionalProperties *openapiSchema           `json:"additionalProperties,omitempty"`
	Properties           map[string]openapiSchema `json:"properties,omitempty"`
	Required             []string                 `json:"required,omitempty"`
	Deprecated           *bool                    `json:"deprecated,omitempty"`
	Description          string                   `json:"description,omitempty"`
	Example              any                      `json:"example,omitempty"`

	ParentRequired bool `json:"-"`
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

	urlPath             string
	method              string
	bodySchema          openapiSchema
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

	if !o.bodySchema.IsEmpty() {
		var contentType = string(cont.AnyContenType)
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
				contentType := string(cont.AnyContenType)
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

type schemaField string

const (
	schemaOperationId schemaField = "OperationId"
	schemaSummary     schemaField = "Summary"
	schemaHttpMethod  schemaField = "Method"
	schemaUrl         schemaField = "Url"
	schemaDeprecated  schemaField = "Deprecated"
	schemaReq         schemaField = "Request"
	schemaHeaders     schemaField = "Header"
	schemaCookies     schemaField = "Cookie"
	schemaQuery       schemaField = "Query"
	schemaPath        schemaField = "Path"
	schemaBody        schemaField = "Body"
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

func runValidation(c *context, typ errorType, val any, schema schemaField, keypath string, rules []ruleOpts) error {
	errs := make([]error, 0, len(rules))

	optionType := validators.ReuestType
	if typ == ResponseErr {
		optionType = validators.ResponseType
	}
	for _, rule := range rules {
		err := rule.dator(validators.NewValidatorArg(val, optionType, c.r, c.w))
		if err != nil {
			errs = append(errs, newErrReport(typ, schema, keypath, rule.rule, err))
		}
	}

	return errors.Join(errs...)
}
