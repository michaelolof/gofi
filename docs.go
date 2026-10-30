package gofi

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"path"
	"strings"
	"sync"
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
	Tags         []DocsInfoTag       `json:"tags,omitempty"`
	// SecuritySchemes declares the authentication schemes routes can
	// require, by name. Build values with the fluid package, for example
	// fluid.BearerAuth(). They are written to components.securitySchemes in
	// every view.
	SecuritySchemes map[string]any `json:"-"`
	// Security is the requirement every documented route gets unless its
	// Info.Security overrides it.
	Security []SecurityRequirement `json:"security,omitempty"`
	Views    []DocsView            `json:"-"`
}

func (d *DocsOptions) getMatchingDocs(m *serveMux, match func(url string) bool) Docs {
	mpaths := make(docsPaths)
	for url, v := range m.paths {
		if match(url) {
			mpaths[openapiPath(url)] = v
		}
	}
	return Docs{
		OpenApi:     "3.0.3",
		Paths:       &mpaths,
		DocsOptions: d,
		Components:  d.components(DocsComponent{}),
	}
}

// openapiPath rewrites a Gofi route pattern into an OpenAPI path template:
// ":name" and "*name" become "{name}". As in the router, a parameter runs
// from its ':' or '*' to the next '/'.
func openapiPath(pattern string) string {
	if !strings.ContainsAny(pattern, ":*") {
		return pattern
	}
	var b strings.Builder
	b.Grow(len(pattern) + 2)
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch != ':' && ch != '*' {
			b.WriteByte(ch)
			continue
		}
		end := i + 1
		for end < len(pattern) && pattern[end] != '/' {
			end++
		}
		b.WriteByte('{')
		b.WriteString(pattern[i+1 : end])
		b.WriteByte('}')
		i = end - 1
	}
	return b.String()
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
	Name         string        `json:"name,omitempty"`
	Description  string        `json:"description,omitempty"`
	ExternalDocs *ExternalDocs `json:"externalDocs,omitempty"`
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
	URLMatch func(url string) bool
	// The path to the generated OpenAPI specification in JSON.
	DocsPath string
	// The components to use for the documentation.
	Components DocsComponent
}

type DocsComponent struct {
	Schemas map[string]any `json:"schemas,omitempty"`
	// SecuritySchemes adds or replaces schemes for one view, on top of
	// DocsOptions.SecuritySchemes.
	SecuritySchemes map[string]any `json:"securitySchemes,omitempty"`
}

// components returns view with d.SecuritySchemes merged in; schemes the view
// declares itself take precedence.
func (d *DocsOptions) components(view DocsComponent) DocsComponent {
	if d == nil || len(d.SecuritySchemes) == 0 {
		return view
	}
	merged := make(map[string]any, len(d.SecuritySchemes)+len(view.SecuritySchemes))
	for name, scheme := range d.SecuritySchemes {
		merged[name] = scheme
	}
	for name, scheme := range view.SecuritySchemes {
		merged[name] = scheme
	}
	view.SecuritySchemes = merged
	return view
}

// checkSecurity reports a security requirement that names a scheme no view
// declares.
func (d *DocsOptions) checkSecurity(m *serveMux) error {
	views := d.Views
	if len(views) == 0 {
		views = []DocsView{{}}
	}
	for _, view := range views {
		schemes := d.components(view.Components).SecuritySchemes
		check := func(where string, reqs []SecurityRequirement) error {
			for _, req := range reqs {
				for name := range req {
					if _, ok := schemes[name]; !ok {
						return fmt.Errorf("gofi: %s requires security scheme %q, which is not declared in DocsOptions.SecuritySchemes or the view's Components.SecuritySchemes", where, name)
					}
				}
			}
			return nil
		}
		if err := check("DocsOptions.Security", d.Security); err != nil {
			return err
		}
		for url, ops := range m.paths {
			if view.URLMatch != nil && !view.URLMatch(url) {
				continue
			}
			for method, op := range ops {
				if op.Security == nil {
					continue
				}
				if err := check("route "+strings.ToUpper(method)+" "+url, *op.Security); err != nil {
					return err
				}
			}
		}
	}
	return nil
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
			<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" integrity="sha384-+yyzNgM3K92sROwsXxYCxaiLWxWJ0G+v/9A+qIZ2rgefKgkdcmJI+L601cqPD/Ut" crossorigin="anonymous" />
		</head>
		<body>
			<div id="swagger-ui"></div>
			<script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js" integrity="sha384-qn5tagrAjZi8cSmvZ+k3zk4+eDEEUcP9myuR2J6V+/H6rne++v6ChO7EeHAEzqxQ" crossorigin="anonymous"></script>
			<script>
				window.onload = () => {
					window.ui = SwaggerUIBundle({
					url: %s,
					dom_id: '#swagger-ui',
					});
				};
			</script>
		</body>
		</html>
	`
	return &uiTemplate{
		html:   html,
		escape: jsString,
	}
}

func ScalarTemplate(config *ScalarConfig) DocsUiTemplate {
	cs := fmt.Sprintf("%q", "{}")
	// Pinned with Subresource Integrity; a custom ScriptSrcLink is loaded
	// without an integrity attribute.
	srcLink := "https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.76.0/dist/browser/standalone.js"
	srcIntegrity := ` integrity="sha384-oH1C0RwqcwbVXkC+r8A+4qRoyEUQdtxAzOwumkxZtQBBcsy61FOoPt7B50OVJYYQ" crossorigin="anonymous"`
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
			srcIntegrity = ""
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
			<script src="` + html.EscapeString(srcLink) + `"` + srcIntegrity + `></script>
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
			<redoc spec-url="%s"></redoc>
			<!-- Redoc 3 bundles use import.meta and only run as a module. -->
			<script type="module" src="https://cdn.jsdelivr.net/npm/redoc@3.0.1/bundles/redoc.standalone.js" integrity="sha384-j07qRKHNNmcwU1/Y1elJc7pDLHXP/QfbQXTtSkbOJ7uYGJikqAzsFVbY2zR9FqOT" crossorigin="anonymous"></script>
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
			<script type="module" src="https://cdn.jsdelivr.net/npm/rapidoc@11.0.1/dist/rapidoc-min.js" integrity="sha384-fCYjbigC95GwGadnXpdvX5fXvlWRGMWJMOw89RxKHaGluDjZwf0Lu1KfUrE5KyfN" crossorigin="anonymous"></script>
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
			<script src="https://cdn.jsdelivr.net/npm/@stoplight/elements@9.0.29/web-components.min.js" integrity="sha384-enyjBaVmV00WFIo0D7AIVgtZ/4x0idVNWHotG8WUpvQo9HXgMSGAAW2YEOpiJTpP" crossorigin="anonymous"></script>
			<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@stoplight/elements@9.0.29/styles.min.css" integrity="sha384-iVQBHadsD+eV0M5+ubRCEVXrXEBj+BqcuwjUwPoVJc0Pb1fmrhYSAhL+BFProHdV" crossorigin="anonymous">
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
	// escape encodes the spec path for its template context; HTML
	// attribute escaping by default.
	escape func(string) string
}

func (u *uiTemplate) HTML(specPath string) []byte {
	// specPath = "https://api.apis.guru/v2/specs/github.com/1.1.4/openapi.json"
	escape := u.escape
	if escape == nil {
		escape = html.EscapeString
	}
	return []byte(fmt.Sprintf(u.html, escape(specPath)))
}

// jsString encodes s as a JavaScript string literal that is safe inside a
// <script> element: encoding/json escapes quotes, '<', '>' and '&'.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// OpenAPISpec extracts the OpenAPI 3.0.3 specification directly from the Router.
// This is useful for programmatic access, testing, or building static documentation
// files without running the server.
func OpenAPISpec(r Router, opts DocsOptions) Docs {
	m, ok := r.(*serveMux)
	if !ok {
		// Should not happen unless a mock router is passed
		return Docs{
			OpenApi:     "3.0.3",
			DocsOptions: &opts,
		}
	}

	return opts.getMatchingDocs(m, func(url string) bool { return true })
}

// Filter creates a shallow copy of Docs, retaining only the paths that
// satisfy the match callback function.
func (d Docs) Filter(match func(path string) bool) Docs {
	newPaths := make(docsPaths)

	if d.Paths != nil {
		for p, methods := range *d.Paths {
			if match == nil || match(p) {
				newPaths[p] = methods
			}
		}
	}

	filteredDocs := d
	filteredDocs.Paths = &newPaths
	return filteredDocs
}

// FilterByURL creates a shallow copy of Docs, retaining only the paths
// that start with the exact prefix string.
func (d Docs) FilterByURL(prefix string) Docs {
	return d.Filter(func(p string) bool {
		return strings.HasPrefix(p, prefix)
	})
}

// FilterByRoutePrefix returns a shallow copy of Docs dynamically tailored
// to match the specific UI View configured for routePrefix in DocsOptions.Views.
// It applies the view's custom URLMatch filter and custom Components.
func (d Docs) FilterByRoutePrefix(routePrefix string) Docs {
	if d.DocsOptions == nil {
		return d
	}

	for _, view := range d.DocsOptions.Views {
		if view.RoutePrefix == routePrefix {
			filtered := d
			if view.URLMatch != nil {
				filtered = d.Filter(view.URLMatch)
			}
			filtered.Components = d.DocsOptions.components(view.Components)
			return filtered
		}
	}

	return d
}

type docsViewState struct {
	specOnce sync.Once
	specJSON []byte

	htmlOnce sync.Once
	htmlBody []byte
}

func ServeDocs(r Router, opts DocsOptions) error {
	const docsPath = "/q/openapi"

	m, ok := r.(*serveMux)
	if !ok {
		return errors.New("invalid server mux passed when serving docs")
	}
	if err := opts.checkSecurity(m); err != nil {
		return err
	}

	for _, vopt := range opts.Views {

		if vopt.RoutePrefix == "" {
			continue
		}

		// Capture vopt for the closures
		viewOpt := vopt

		state := &docsViewState{}

		// Serve the OpenAPI spec JSON
		m.Get(path.Join(viewOpt.RoutePrefix, docsPath), RouteOptions{
			Handler: func(c Context) error {
				state.specOnce.Do(func() {
					var d Docs
					if viewOpt.URLMatch == nil {
						d = opts.getMatchingDocs(m, func(url string) bool { return true })
					} else {
						d = opts.getMatchingDocs(m, viewOpt.URLMatch)
					}

					d.Components = opts.components(viewOpt.Components)
					// In the event of a marshal error, it will just leave specJSON as []byte{}
					// which will be served as an empty response. This is acceptable for a fatal developer error.
					state.specJSON, _ = json.Marshal(d)
				})

				ctx := c.(*context)
				ctx.fctx.Response.Header.Set("Content-Type", "application/json")
				ctx.fctx.Response.SetStatusCode(200)
				ctx.fctx.Response.SetBodyRaw(state.specJSON)
				return nil
			},
		})

		// Serve the docs UI HTML
		m.Get(viewOpt.RoutePrefix, RouteOptions{
			Handler: func(c Context) error {
				state.htmlOnce.Do(func() {
					tmplt := viewOpt.Template
					if tmplt == nil {
						tmplt = SwaggerTemplate()
					}
					state.htmlBody = tmplt.HTML(path.Join(viewOpt.RoutePrefix, docsPath))
				})

				ctx := c.(*context)
				ctx.fctx.Response.Header.Set("Content-Type", "text/html")
				ctx.fctx.Response.SetStatusCode(200)
				ctx.fctx.Response.SetBodyRaw(state.htmlBody)
				return nil
			},
		})
	}

	return nil
}
