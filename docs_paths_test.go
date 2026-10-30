package gofi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenapiPath(t *testing.T) {
	cases := map[string]string{
		"/projects":                          "/projects",
		"/projects/:project_id":              "/projects/{project_id}",
		"/projects/:project_id/keys/:key_id": "/projects/{project_id}/keys/{key_id}",
		"/files/*filepath":                   "/files/{filepath}",
		"/events:batch":                      "/events{batch}",
		"/projects/:project_id/api-keys":     "/projects/{project_id}/api-keys",
		"/static/:dir/*rest":                 "/static/{dir}/{rest}",
	}
	for in, want := range cases {
		assert.Equal(t, want, openapiPath(in), in)
	}
}

func TestOpenAPISpec_UsesTemplatesAndJSONMediaType(t *testing.T) {
	type schema struct {
		Request struct {
			Path struct {
				ProjectID string `json:"project_id" validate:"required"`
			}
			Body struct {
				Name string `json:"name" validate:"required"`
			}
		}
		Ok struct {
			Body struct {
				ID string `json:"id" validate:"required"`
			}
		}
	}
	r := NewRouter()
	r.Put("/projects/:project_id", RouteOptions{Schema: &schema{}, Handler: func(c Context) error { return nil }})

	raw, err := json.Marshal(OpenAPISpec(r, DocsOptions{}))
	require.NoError(t, err)

	var spec struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]any `json:"content"`
			} `json:"requestBody"`
			Responses map[string]struct {
				Content map[string]any `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(raw, &spec))

	op, ok := spec.Paths["/projects/{project_id}"]["put"]
	require.True(t, ok, "paths: %v", spec.Paths)
	assert.Contains(t, op.RequestBody.Content, "application/json")
	assert.Contains(t, op.Responses["200"].Content, "application/json")
}
