package gofi

import (
	"encoding/json"
	"fmt"

	"github.com/michaelolof/gofi/cont"
)

type Docs struct {
	OpenApi string     `json:"openapi"`
	Paths   *docsPaths `json:"paths,omitempty"`
	*DocsOptions
}

func (d *DocsOptions) getDocs(m *ServeMux) Docs {

	d.Info.Title = fallback(d.Info.Title, "My Awesome API")
	d.Info.Version = fallback(d.Info.Version, "0.0.1")

	docs := Docs{
		OpenApi:     "3.0.3",
		Paths:       &m.paths,
		DocsOptions: d,
	}

	return docs
}

type DocsOptions struct {
	Info         DocsInfoOptions     `json:"info,omitempty"`
	Servers      []DocsServerOptions `json:"servers,omitempty"`
	ExternalDocs *ExternalDocs       `json:"externalDocs,omitempty"`
	Tags         *DocsInfoTag        `json:"tags,omitempty"`
	Ui           DocsUiOptions       `json:"-"`
}

type DocsInfoOptions struct {
	Title          string           `json:"title,omitempty"`
	Version        string           `json:"version,omitempty"`
	Description    string           `json:"description,omitempty"`
	Summary        string           `json:"summary,omitempty"`
	TermsOfService string           `json:"termsOfService,omitempty"`
	Contact        *DocsInfoContact `json:"contact,omitempty"`
	License        *DocsInfoLicense `json:"license,omitempty"`
}

type DocsInfoLicense struct {
	Name string `json:"name,omitempty"`
	Url  string `json:"url,omitempty"`
}

type DocsInfoContact struct {
	Name  string `json:"name,omitempty"`
	Url   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

type DocsInfoTag struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type ExternalDocs struct {
	Url         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
}

type DocsServerOptions struct {
	Url         string                        `json:"url,omitempty"`
	Description string                        `json:"description,omitempty"`
	Variables   map[string]DocsServerVariable `json:"variables,omitempty"`
}

type DocsServerVariable struct {
	Enum        []string `json:"enum,omitempty"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
}

type DocsUiOptions struct {
	RoutePrefix string
	Template    DocsUiTemplate
}

type DocsUiTemplate interface {
	HTML(specPath string) []byte
}

func SwaggerTemplate() DocsUiTemplate {
	html := `
		<!DOCTYPE html>
		<html lang="en">
		<head>
			<meta charset="utf-8" />
			<meta name="viewport" content="width=device-width, initial-scale=1" />
			<meta name="description" content="SwaggerUI" />
			<title>SwaggerUI</title>
			<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" />
		</head>
		<body>
			<div id="swagger-ui"></div>
			<script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js" crossorigin></script>
			<script>
				window.onload = () => {
					window.ui = SwaggerUIBundle({
					url: '%s',
					dom_id: '#swagger-ui',
					});
				};
			</script>
		</body>
		</html>
	`
	return &uiTemplate{
		html: html,
	}
}

func ScalarTemplate(config *ScalarConfig) DocsUiTemplate {
	cs := fmt.Sprintf("%q", "{}")
	srcLink := "https://cdn.jsdelivr.net/npm/@scalar/api-reference"
	additionalStyle := ""
	additionalScript := ""
	if config != nil {
		type confT struct {
			*ScalarConfig
			ShowSidebar bool `json:"showSidebar"`
		}

		conf := confT{
			ScalarConfig: config,
			ShowSidebar:  !config.HideSidebar,
		}

		csb, err := json.Marshal(conf)
		if err != nil {
			fmt.Println(err)
		} else {
			cs = fmt.Sprintf("%q", string(csb))
		}

		if config.ScriptSrcLink != "" {
			srcLink = config.ScriptSrcLink
		}

		if config.AdditionalStyle != "" {
			additionalStyle = "<style id=\"gofi-additional-styles\" type=\"text/css\">\r\t" + config.AdditionalStyle + "\r</style>"
		}

		if config.AdditionalScript != "" {
			additionalScript = "<script>\r" + config.AdditionalScript + "\r</script>"
		}
	}

	html := `
		<!doctype html>
		<html>
		<head>
			<title>API Reference</title>
			<meta charset="utf-8" />
			<meta
			name="viewport"
			content="width=device-width, initial-scale=1" />
			` + additionalStyle + `
		</head>
		<body>
			<script
				id="api-reference"
				data-url="%s">
			</script>
			<script>
				document.getElementById('api-reference').dataset.configuration = ` + cs + `
			</script>
			<script src="` + srcLink + `"></script>
			` + additionalScript + `
		</body>
		</html>`

	return &uiTemplate{
		html: html,
	}
}

type ScalarConfig struct {
	Theme              string            `json:"theme,omitempty"`
	ProxyUrl           string            `json:"proxyUrl,omitempty"`
	HideModels         bool              `json:"hideModels,omitempty"`
	HideDownloadButton bool              `json:"hideDownloadButton,omitempty"`
	CustomCSS          string            `json:"customCss,omitempty"`
	SearchHotKey       string            `json:"searchHotKey,omitempty"`
	MetaData           map[string]string `json:"metaData,omitempty"`
	WithDefaultFonts   bool              `json:"withDefaultFonts,omitempty"`
	IsEditable         bool              `json:"isEditable,omitempty"`
	HideSidebar        bool              `json:"-"` // This is because the default is true
	ScriptSrcLink      string            `json:"-"`
	AdditionalStyle    string            `json:"-"`
	AdditionalScript   string            `json:"-"`
}

func RedoclyTemplate() DocsUiTemplate {
	html := `
		<!DOCTYPE html>
		<html>
		<head>
			<title>Redoc</title>
			<!-- needed for adaptive design -->
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1">
			<link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">

			<!--
			Redoc doesn't change outer page styles
			-->
			<style>
			body {
				margin: 0;
				padding: 0;
			}
			</style>
		</head>
		<body>
			<redoc spec-url='%s'></redoc>
			<script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"> </script>
		</body>
		</html>
	`
	return &uiTemplate{html: html}
}

func RapidDoc() DocsUiTemplate {
	html := `
		<!doctype html> <!-- Important: must specify -->
		<html>
		<head>
			<meta charset="utf-8"> <!-- Important: rapi-doc uses utf8 characters -->
			<script type="module" src="https://unpkg.com/rapidoc/dist/rapidoc-min.js"></script>
		</head>
		<body>
			<rapi-doc spec-url="%s"> </rapi-doc>
		</body>
		</html>
	`
	return &uiTemplate{html: html}
}

func StopLight() DocsUiTemplate {
	html := `
		<!doctype html>
		<html lang="en">
		<head>
			<meta charset="utf-8">
			<meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
			<title>Elements in HTML</title>
			<!-- Embed elements Elements via Web Component -->
			<script src="https://unpkg.com/@stoplight/elements/web-components.min.js"></script>
			<link rel="stylesheet" href="https://unpkg.com/@stoplight/elements/styles.min.css">
		</head>
		<body>

			<elements-api
				apiDescriptionUrl="%s"
				router="hash"
				layout="sidebar"
			/>
		</body>
		</html>
	`
	return &uiTemplate{html: html}
}

type uiTemplate struct {
	html string
}

func (u *uiTemplate) HTML(specPath string) []byte {
	// specPath = "https://api.apis.guru/v2/specs/github.com/1.1.4/openapi.json"
	return []byte(fmt.Sprintf(u.html, specPath))
}

type docsPaths map[string]map[string]openapiOperationObject

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

	// generateOperationIdAndSummary := func(method string, path string) (op string, sum string) {
	// 	paths := regexp.MustCompile("[/_-]+").Split(path, -1)
	// 	rtn := strings.ToLower(method)
	// 	s := utils.ToUpperFirst(method)

	// 	for i, p := range paths {
	// 		if p == "" {
	// 			continue
	// 		}

	// 		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") && len(p) > 2 {
	// 			v := p[1 : len(p)-1]
	// 			rtn = rtn + ("By" + utils.ToUpperFirst(v))
	// 			if i == len(paths)-1 {
	// 				s = s + " by " + strings.ToLower(v)
	// 			} else {
	// 				s = s + " by " + strings.ToLower(v) + " and"
	// 			}
	// 		} else {
	// 			rtn = rtn + utils.ToUpperFirst(p)
	// 			s = s + " " + strings.ToLower(p)
	// 		}
	// 	}

	// 	return rtn, s
	// }

	o.method = method
	o.urlPath = path

	// op, sum := generateOperationIdAndSummary(method, path)
	// if o.OperationId == "" {
	// 	o.OperationId = op
	// }
	// if o.Summary == "" {
	// 	o.Summary = sum
	// }

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

type openapiParameter struct {
	In       string        `json:"in"`
	Name     string        `json:"name"`
	Required *bool         `json:"required,omitempty"`
	Schema   openapiSchema `json:"schema,omitempty"`
}

func newOpenapiParameters(in string, name string, required *bool, schema openapiSchema) openapiParameter {
	return openapiParameter{
		In:       in,
		Name:     name,
		Required: required,
		Schema:   schema,
	}
}

type openapiParameters []openapiParameter

func (o openapiParameters) findByName(name string) *openapiParameter {
	for _, v := range o {
		if v.Name == name {
			return &v
		}
	}
	return nil
}

func (o openapiParameters) findByNameIn(name string, in string) *openapiParameter {
	for _, v := range o {
		if v.Name == name && v.In == in {
			return &v
		}
	}
	return nil
}

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

func (o *openapiSchema) IsEmpty() bool {
	return o == nil || o.Type == ""
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

type openapiRequestObject struct {
	Description string                        `json:"description,omitempty"`
	Required    bool                          `json:"required,omitempty"`
	Content     map[string]openapiMediaObject `json:"content,omitempty"`
}

type openapiResponseObject struct {
	Description string                         `json:"description,omitempty"`
	Headers     map[string]openapiHeaderObject `json:"headers,omitempty"`
	Required    bool                           `json:"required,omitempty"`
	Content     map[string]openapiMediaObject  `json:"content,omitempty"`
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

type openapiMediaObject struct {
	Schema openapiSchema `json:"schema,omitempty"`
}
