package gofi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIGeneration(t *testing.T) {
	t.Run("BasicTypes", func(t *testing.T) {
		type testSchema struct {
			Request struct {
				Query struct {
					Name     string  `json:"name" validate:"required" description:"User name"`
					Age      int     `json:"age" validate:"min=18" default:"25"`
					IsActive bool    `json:"is_active" default:"true"`
					Score    float64 `json:"score" example:"4.5"`
					ID       string  `json:"id" validate:"uuid"`
				}
			}
			Ok struct {
				Body struct {
					Status string `json:"status"`
				}
			}
		}

		r := newServeMux()
		cs := r.compileSchema(&testSchema{}, Info{
			OperationId: "getUser",
			Summary:     "Get user detail",
			Description: "Returns user information based on query params",
			Tags:        []string{"User"},
		})

		specs := cs.specs

		// Check basic info
		assert.Equal(t, "getUser", specs.OperationId)
		assert.Equal(t, "Get user detail", specs.Summary)
		assert.Equal(t, "Returns user information based on query params", specs.Description)
		assert.Contains(t, specs.Tags, "User")

		// Check parameters
		params := specs.Parameters
		assert.Len(t, params, 5)

		findParam := func(name string) *openapiParameter {
			for _, p := range params {
				if p.Name == name {
					return &p
				}
			}
			return nil
		}

		nameParam := findParam("name")
		require.NotNil(t, nameParam)
		assert.Equal(t, "query", nameParam.In)
		assert.True(t, *nameParam.Required)
		assert.Equal(t, "string", nameParam.Schema.Type)
		assert.Equal(t, "User name", nameParam.Schema.Description)

		ageParam := findParam("age")
		require.NotNil(t, ageParam)
		assert.Equal(t, "integer", ageParam.Schema.Type)
		assert.Equal(t, "int64", ageParam.Schema.Format)
		assert.Equal(t, float64(18), *ageParam.Schema.Minimum)
		assert.Equal(t, 25, ageParam.Schema.Default) // Fixed: removed int64(25) cast

		activeParam := findParam("is_active")
		require.NotNil(t, activeParam)
		assert.Equal(t, "boolean", activeParam.Schema.Type)
		assert.Equal(t, true, activeParam.Schema.Default)

		scoreParam := findParam("score")
		require.NotNil(t, scoreParam)
		assert.Equal(t, "number", scoreParam.Schema.Type)
		assert.Equal(t, 4.5, scoreParam.Schema.Example)

		idParam := findParam("id")
		require.NotNil(t, idParam)
		assert.Equal(t, "uuid", idParam.Schema.Format)
	})

	t.Run("ComplexTypes", func(t *testing.T) {
		type nested struct {
			FieldA string `json:"field_a"`
		}
		type testSchema struct {
			Request struct {
				Body struct {
					Tags     []string          `json:"tags"`
					Metadata map[string]string `json:"metadata"`
					Details  nested            `json:"details"`
					Points   []int             `json:"points"`
				}
			}
		}

		r := newServeMux()
		cs := r.compileSchema(&testSchema{}, Info{})
		body := cs.specs.bodySchema

		assert.Equal(t, "object", body.Type)
		assert.Equal(t, "array", body.Properties["tags"].Type)
		assert.Equal(t, "string", body.Properties["tags"].Items.Type)

		assert.Equal(t, "object", body.Properties["metadata"].Type)
		assert.Equal(t, "string", body.Properties["metadata"].AdditionalProperties.Type)

		assert.Equal(t, "object", body.Properties["details"].Type)
		assert.Equal(t, "string", body.Properties["details"].Properties["field_a"].Type)

		assert.Equal(t, "array", body.Properties["points"].Type)
		assert.Equal(t, "integer", body.Properties["points"].Items.Type)
	})

	t.Run("Responses", func(t *testing.T) {
		type testSchema struct {
			Ok struct {
				Header struct { // Fixed: was Headers
					XRequestID string `json:"X-Request-ID"`
				}
				Body struct {
					Message string `json:"message"`
				}
			}
			BadRequest struct {
				Body struct {
					Error string `json:"error"`
				}
			}
		}

		r := newServeMux()
		cs := r.compileSchema(&testSchema{}, Info{})

		assert.Contains(t, cs.specs.responsesSchema, "Ok")
		assert.Contains(t, cs.specs.responsesSchema, "BadRequest")

		okHeaders := cs.specs.responsesParameters["Ok"]
		require.Len(t, okHeaders, 1)
		assert.Equal(t, "header", okHeaders[0].In)
		assert.Equal(t, "X-Request-ID", okHeaders[0].Name)

		assert.Equal(t, "string", cs.specs.responsesSchema["Ok"].Properties["message"].Type)
		assert.Equal(t, "string", cs.specs.responsesSchema["BadRequest"].Properties["error"].Type)
	})

	t.Run("FormData", func(t *testing.T) {
		type testSchema struct {
			Request struct {
				Header struct {
					ContentType string `json:"Content-Type" default:"application/x-www-form-urlencoded"`
				}
				Body struct {
					Name string `json:"name"`
					Age  int    `json:"age"`
				}
			}
		}

		r := newServeMux()
		cs := r.compileSchema(&testSchema{}, Info{Method: "POST", Url: "/form"})
		cs.specs.normalize("POST", "/form")

		assert.NotNil(t, cs.specs.RequestBody)
		assert.Contains(t, cs.specs.RequestBody.Content, "application/x-www-form-urlencoded")
		schema := cs.specs.RequestBody.Content["application/x-www-form-urlencoded"].Schema
		assert.Equal(t, "object", schema.Type)
		assert.Contains(t, schema.Properties, "name")
		assert.Contains(t, schema.Properties, "age")
	})

	t.Run("CustomSpecs", func(t *testing.T) {
		r := newServeMux()
		r.RegisterSpec(&vendorSpec{})

		type testSchema struct {
			Request struct {
				Query struct {
					Custom vendorType `json:"custom" spec:"custom"`
				}
			}
		}

		cs := r.compileSchema(&testSchema{}, Info{})
		param := cs.specs.Parameters[0]
		assert.Equal(t, "string", param.Schema.Type)
		assert.Equal(t, "string", param.Schema.Format)
	})
}

func TestOpenAPIServing(t *testing.T) {
	type user struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	type getUsersSchema struct {
		Ok struct {
			Body []user
		}
	}

	r := NewServeMux()
	r.Get("/users", RouteOptions{
		Schema: &getUsersSchema{},
		Handler: func(c Context) error {
			return c.Send(200, []user{{ID: "1", Name: "John"}})
		},
	})

	err := ServeDocs(r, DocsOptions{
		Views: []DocsView{
			{
				RoutePrefix: "/docs",
			},
		},
	})
	require.NoError(t, err)

	t.Run("OpenAPIJSON", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/docs/q/openapi", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var doc Docs
		err := json.Unmarshal(w.Body.Bytes(), &doc)
		require.NoError(t, err)

		assert.Equal(t, "3.0.3", doc.OpenApi)
		assert.Contains(t, *doc.Paths, "/users")
		assert.Contains(t, (*doc.Paths)["/users"], "get")
	})

	templates := []struct {
		name     string
		template DocsUiTemplate
		contains string
	}{
		{"Swagger", SwaggerTemplate(), "SwaggerUI"},
		{"Scalar", ScalarTemplate(nil), "@scalar"}, // Fixed: was "Scalar"
		{"Redocly", RedoclyTemplate(), "Redoc"},
		{"RapidDoc", RapidDoc(), "rapi-doc"},
		{"Stoplight", StopLight(), "elements-api"},
	}

	for _, tt := range templates {
		t.Run(tt.name, func(t *testing.T) {
			r := NewServeMux()
			err := ServeDocs(r, DocsOptions{
				Views: []DocsView{
					{
						RoutePrefix: "/docs",
						Template:    tt.template,
					},
				},
			})
			require.NoError(t, err)

			req := httptest.NewRequest("GET", "/docs", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			assert.Equal(t, "text/html", w.Header().Get("Content-Type"))
			assert.Contains(t, w.Body.String(), tt.contains)
		})
	}
}
