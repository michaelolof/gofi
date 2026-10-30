package gofi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/michaelolof/gofi/fluid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func specJSON(t *testing.T, r Router, opts DocsOptions) map[string]any {
	t.Helper()
	raw, err := json.Marshal(OpenAPISpec(r, opts))
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func dig(t *testing.T, v any, keys ...string) any {
	t.Helper()
	for _, k := range keys {
		m, ok := v.(map[string]any)
		require.True(t, ok, "expected object at %q in %v", k, v)
		v, ok = m[k]
		require.True(t, ok, "missing key %q in %v", k, m)
	}
	return v
}

type securedSchema struct {
	Ok struct {
		Body struct {
			ID string `json:"id"`
		}
	}
}

func securityRouter() Router {
	r := NewRouter()
	noop := func(c Context) error { return nil }
	r.Get("/projects", RouteOptions{Schema: &securedSchema{}, Handler: noop})
	r.Post("/login", RouteOptions{
		Info:    Info{Security: NoSecurity},
		Schema:  &securedSchema{},
		Handler: noop,
	})
	r.Get("/admin/keys", RouteOptions{
		Info:    Info{Security: []SecurityRequirement{{"AdminBearerAuth": nil}}},
		Schema:  &securedSchema{},
		Handler: noop,
	})
	return r
}

func securityOptions() DocsOptions {
	return DocsOptions{
		SecuritySchemes: map[string]any{
			"BearerAuth":      fluid.BearerAuth(),
			"AdminBearerAuth": fluid.BearerAuth(fluid.WithBearerFormat("")),
		},
		Security: []SecurityRequirement{{"BearerAuth": {}}},
	}
}

func TestOpenAPISpec_Security(t *testing.T) {
	spec := specJSON(t, securityRouter(), securityOptions())

	assert.Equal(t, []any{map[string]any{"BearerAuth": []any{}}}, spec["security"])

	schemes := dig(t, spec, "components", "securitySchemes").(map[string]any)
	assert.Equal(t, "bearer", dig(t, schemes, "BearerAuth", "scheme"))
	assert.Contains(t, schemes, "AdminBearerAuth")

	_, inherits := dig(t, spec, "paths", "/projects", "get").(map[string]any)["security"]
	assert.False(t, inherits, "a route without Info.Security inherits the default")

	assert.Equal(t, []any{}, dig(t, spec, "paths", "/login", "post", "security"))
	assert.Equal(t,
		[]any{map[string]any{"AdminBearerAuth": []any{}}},
		dig(t, spec, "paths", "/admin/keys", "get", "security"),
	)
}

func TestServeDocs_RejectsUndeclaredScheme(t *testing.T) {
	opts := securityOptions()
	delete(opts.SecuritySchemes, "AdminBearerAuth")
	opts.Views = []DocsView{{RoutePrefix: "/docs"}}

	err := ServeDocs(securityRouter(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `route GET /admin/keys requires security scheme "AdminBearerAuth"`)
}

func TestServeDocs_ViewSchemesAreMerged(t *testing.T) {
	opts := securityOptions()
	delete(opts.SecuritySchemes, "AdminBearerAuth")
	opts.Views = []DocsView{{
		RoutePrefix: "/docs",
		Components:  DocsComponent{SecuritySchemes: map[string]any{"AdminBearerAuth": fluid.BearerAuth()}},
	}}
	r := securityRouter()
	require.NoError(t, ServeDocs(r, opts))

	resp, err := r.Test(TestOptions{Method: "GET", Path: "/docs/q/openapi"})
	require.NoError(t, err)
	var spec map[string]any
	require.NoError(t, json.Unmarshal(resp.Body, &spec))
	schemes := dig(t, spec, "components", "securitySchemes").(map[string]any)
	assert.Contains(t, schemes, "BearerAuth")
	assert.Contains(t, schemes, "AdminBearerAuth")
}

func TestOpenAPISpec_BoundsUseTypeKeywords(t *testing.T) {
	type schema struct {
		Request struct {
			Body struct {
				Name  string            `json:"name" validate:"min=2,max=80"`
				Tags  []string          `json:"tags" validate:"min=1,max=5"`
				Attrs map[string]string `json:"attrs" validate:"max=10"`
				Age   int               `json:"age" validate:"gte=18,lte=120"`
			}
		}
	}
	r := NewRouter()
	r.Post("/p", RouteOptions{Schema: &schema{}, Handler: func(c Context) error { return nil }})
	props := dig(t, specJSON(t, r, DocsOptions{}), "paths", "/p", "post", "requestBody", "content", "application/json", "schema", "properties").(map[string]any)

	name := props["name"].(map[string]any)
	assert.Equal(t, 2.0, name["minLength"])
	assert.Equal(t, 80.0, name["maxLength"])
	assert.NotContains(t, name, "minimum")

	tags := props["tags"].(map[string]any)
	assert.Equal(t, 1.0, tags["minItems"])
	assert.Equal(t, 5.0, tags["maxItems"])

	attrs := props["attrs"].(map[string]any)
	assert.Equal(t, 10.0, attrs["maxProperties"])

	age := props["age"].(map[string]any)
	assert.Equal(t, 18.0, age["minimum"])
	assert.Equal(t, 120.0, age["maximum"])
	assert.NotContains(t, age, "minLength")
}

type timeLayoutSchema struct {
	Request struct {
		Body struct {
			At     time.Time `json:"at"`
			Day    time.Time `json:"day" pattern:"2006-01-02"`
			Custom time.Time `json:"custom" pattern:"02/01/2006 15:04"`
			Given  time.Time `json:"given" pattern:"02/01/2006 15:04" example:"25/12/2026 09:30"`
		}
	}
	Ok struct {
		Body struct {
			Custom time.Time `json:"custom" pattern:"02/01/2006 15:04"`
		}
	}
}

func TestOpenAPISpec_TimeLayouts(t *testing.T) {
	r := NewRouter()
	r.Post("/t", RouteOptions{Schema: &timeLayoutSchema{}, Handler: func(c Context) error { return nil }})
	props := dig(t, specJSON(t, r, DocsOptions{}), "paths", "/t", "post", "requestBody", "content", "application/json", "schema", "properties").(map[string]any)

	at := props["at"].(map[string]any)
	assert.Equal(t, "date-time", at["format"])
	assert.NotContains(t, at, "pattern")

	day := props["day"].(map[string]any)
	assert.Equal(t, "date", day["format"])
	assert.NotContains(t, day, "pattern")

	custom := props["custom"].(map[string]any)
	assert.NotContains(t, custom, "format")
	assert.NotContains(t, custom, "pattern")
	assert.Equal(t, "14/03/2026 15:04", custom["example"])

	assert.Equal(t, "25/12/2026 09:30", props["given"].(map[string]any)["example"])
}

func TestTimeLayout_StillParsesAndFormatsAtRuntime(t *testing.T) {
	r := NewRouter()
	r.Post("/t", RouteOptions{
		Schema: &timeLayoutSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[timeLayoutSchema](c)
			if err != nil {
				return err
			}
			s.Ok.Body.Custom = s.Request.Body.Custom.Add(time.Hour)
			return c.Send(200, s.Ok)
		},
	})

	resp, err := r.Test(TestOptions{
		Method: "POST",
		Path:   "/t",
		Body:   strings.NewReader(`{"at":"2026-03-14T15:04:05Z","day":"2026-03-14","custom":"14/03/2026 15:04","given":"25/12/2026 09:30"}`),
	})
	require.NoError(t, err)
	require.Equal(t, 200, resp.Code(), resp.BodyString())
	assert.JSONEq(t, `{"custom":"14/03/2026 16:04"}`, resp.BodyString())
}

func TestOpenAPISpec_FreeFormRequestBodies(t *testing.T) {
	type rawSchema struct {
		Request struct {
			Body json.RawMessage `validate:"required" description:"Original provider JSON."`
		}
	}
	type anySchema struct {
		Request struct {
			Body any
		}
	}
	type noBodySchema struct {
		Request struct {
			Query struct {
				Q string `json:"q"`
			}
		}
	}
	r := NewRouter()
	noop := func(c Context) error { return nil }
	r.Post("/raw", RouteOptions{Schema: &rawSchema{}, Handler: noop})
	r.Post("/any", RouteOptions{Schema: &anySchema{}, Handler: noop})
	r.Get("/none", RouteOptions{Schema: &noBodySchema{}, Handler: noop})
	spec := specJSON(t, r, DocsOptions{})

	raw := dig(t, spec, "paths", "/raw", "post", "requestBody").(map[string]any)
	assert.Equal(t, true, raw["required"])
	assert.Equal(t,
		map[string]any{"description": "Original provider JSON."},
		dig(t, raw, "content", "application/json", "schema"),
	)

	anyBody := dig(t, spec, "paths", "/any", "post", "requestBody").(map[string]any)
	assert.Contains(t, dig(t, anyBody, "content").(map[string]any), "application/json")

	_, hasBody := dig(t, spec, "paths", "/none", "get").(map[string]any)["requestBody"]
	assert.False(t, hasBody, "a schema without Request.Body documents no request body")
}
