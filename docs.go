package gofi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
)

type Docs struct {
	OpenApi string     `json:"openapi"`
	Paths   *docsPaths `json:"paths,omitempty"`
	*DocsOptions
	Components DocsComponent `json:"components"`
}

type docsPaths map[string]map[string]openapiOperationObject

type DocsOptions struct {
	Info         DocsInfoOptions     `json:"info,omitempty"`
	Servers      []DocsServerOptions `json:"servers,omitempty"`
	ExternalDocs *ExternalDocs       `json:"externalDocs,omitempty"`
	Tags         *DocsInfoTag        `json:"tags,omitempty"`
	Views        []DocsView          `json:"-"`
}

func (d *DocsOptions) getMatchingDocs(m *serveMux, match func(url string) bool) Docs {
	mpaths := make(docsPaths)
	for url, v := range m.paths {
		if match(url) {
			mpaths[url] = v
		}
	}
	return Docs{
		OpenApi:     "3.0.3",
		Paths:       &mpaths,
		DocsOptions: d,
	}
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

type DocsView struct {
	// The prefix for the route where the documentation will be served.
	RoutePrefix string
	// The template to use for the documentation. Defaults to Swagger UI if none is passed
	Template DocsUiTemplate
	// Match the URL path fo be served. This is useful for serving multiple docs on the same server. (E.g admin /admin/v1/, client /v1/)
	UrlMatch func(url string) bool
	// The path to the generated OpenAPI specification in JSON.
	DocsPath string
	// The components to use for the documentation.
	Components DocsComponent
}

type DocsComponent struct {
	Schemas map[string]any `json:"schemas"`
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
			panic(err)
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

func ServeDocs(r Router, opts DocsOptions) error {
	const docsPath = "/q/openapi"

	m, ok := r.(*serveMux)
	if !ok {
		return errors.New("invalid server mux passed when serving docs")
	}

	var cerr error

	for _, vopt := range opts.Views {

		if vopt.RoutePrefix == "" {
			continue
		}

		m.sm.HandleFunc(fmt.Sprintf("GET %s", path.Join(vopt.RoutePrefix, docsPath)), func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "application/json")
			var d Docs
			if vopt.UrlMatch == nil {
				d = opts.getMatchingDocs(m, func(url string) bool { return true })
			} else {
				d = opts.getMatchingDocs(m, vopt.UrlMatch)
			}

			d.Components = vopt.Components
			ds, err := json.Marshal(d)
			if err != nil {
				cerr = err
				return
			}

			w.Write(ds)
		})

		m.sm.HandleFunc(fmt.Sprintf("GET %s", vopt.RoutePrefix), func(w http.ResponseWriter, r *http.Request) {
			tmplt := vopt.Template
			if tmplt == nil {
				// Make swagger the default template :(
				tmplt = SwaggerTemplate()
			}
			w.Header().Set("content-type", "text/html")
			w.Write(tmplt.HTML(path.Join(vopt.RoutePrefix, docsPath)))
		})

	}

	return cerr
}
