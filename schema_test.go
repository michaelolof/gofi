package gofi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompileSchema_Specs(t *testing.T) {

	type bodySchema struct {
		Name struct {
			Firstname string `json:"firstname" validate:"required,alpha"`
			Lastname  string `json:"lastname" validate:"required,alpha"`
			Contact   struct {
				First string `json:"first" validate:"required,alpha"`
				Last  string `json:"last" validate:"required,alpha"`
			} `json:"contact" validate:"required"`
		} `json:"name" validate:"required"`
		Age   int    `json:"age" validate:"numeric,min=18,max=60"`
		Email string `json:"email" validate:"email,url_encoded"`
	}

	type testSchema2 struct {
		Schema

		Request struct {
			Header struct {
				ContentType   string `json:"Content-Type" default:"application/json"`
				XFrameOptions string `json:"X-Frame-Options" default:"SAMEORIGIN"`
			}

			Cookie struct {
				AccessToken string `json:"access_token" validate:"required"`
			}

			Query struct {
				Email string `json:"email" validate:"url_encoded,email,check_if_empty,oneof=femi bayo tosin"`
				Name  string `json:"name" validate:"oneof=femi bayo tosin"`
				Age   int    `json:"age" validate:"oneof=17 19 20"`
			}

			Path struct {
				Id       int    `json:"id" validate:"required"`
				UserName string `json:"user" validate:"required,oneof=admin user operator"`
			}

			Body *bodySchema
		}

		Success struct {
			Header struct {
				ContentType   string `json:"Content-Type" default:"application/json"`
				XFrameOptions string `json:"X-Frame-Options" default:"SAMEORIGIN"`
			}

			Body struct {
				Status  string `json:"status"`
				Message string `json:"message"`
				Data    any    `json:"data"`
			}
		}
	}

	info := Info{
		OperationId: "my-other-style",
		Summary:     "A way to get things",
		Method:      "GET",
		Url:         "/api/get/vars-one-two",
		Deprecated:  true,
	}

	schema := &testSchema2{}

	rs := compileSchema(schema, info)
	rs.specs.normalize("GET", "/api/test_compile_schema")

	{
		assert.Equal(t, rs.specs.OperationId, info.OperationId)
		assert.Equal(t, rs.specs.Summary, info.Summary)
		assert.Equal(t, *rs.specs.Deprecated, info.Deprecated)
		assert.Equal(t, rs.specs.method, info.Method)
	}

	{
		pm := rs.specs.Parameters.findByName("content-type")
		assert.NotNil(t, pm)
		assert.Equal(t, pm.In, "header")
		assert.Equal(t, pm.Schema.Type, "string")
	}

	{
		pm := rs.specs.Parameters.findByName("access_token")
		assert.NotNil(t, pm)
		assert.Equal(t, pm.In, "cookie")
		assert.Equal(t, *pm.Required, true)
		assert.Equal(t, pm.Schema.Type, "string")
	}

	{
		pm := rs.specs.Parameters.findByName("email")
		assert.NotNil(t, pm)
		assert.Equal(t, pm.In, "query")
		assert.Equal(t, pm.Schema.Format, "email")
		assert.Nil(t, pm.Required)
		assert.Equal(t, pm.Schema.Type, "string")
	}

	{
		pm := rs.specs.Parameters.findByName("id")
		assert.NotNil(t, pm)
		assert.Equal(t, pm.In, "path")
		assert.Equal(t, *pm.Required, true)
		assert.Equal(t, pm.Schema.Type, "integer")
		assert.Equal(t, pm.Schema.Format, "int64")
	}

	{
		pm := rs.specs.Parameters.findByName("user")
		assert.NotNil(t, pm)
		assert.Equal(t, pm.In, "path")
		assert.Equal(t, pm.Schema.Type, "string")
	}

	{
		bs := rs.specs.bodySchema
		assert.Equal(t, bs.Type, "object")
		assert.Equal(t, bs.Required, []string{"name"})
		assert.Len(t, bs.Properties, 3)
		assert.Contains(t, bs.Properties, "age")
		assert.Len(t, bs.Properties["name"].Properties, 3)
		assert.Contains(t, bs.Properties["name"].Properties, "firstname")
		assert.Contains(t, bs.Properties["name"].Properties, "contact")
		assert.Len(t, bs.Properties["name"].Properties["contact"].Properties, 2)
	}

	{
		bs := rs.specs.responsesSchema["Success"]
		assert.Equal(t, bs.Type, "object")
		assert.Len(t, bs.Properties, 3)
		assert.Contains(t, bs.Properties, "message")
		assert.Contains(t, bs.Properties, "status")
		assert.Contains(t, bs.Properties, "data")
	}
}
