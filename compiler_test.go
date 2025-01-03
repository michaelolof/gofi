package gofi

import (
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCompilerHooksOpenAPISpecs(t *testing.T) {

	type testSchema struct {
		Ok struct {
			Body struct {
				Primitive string            `json:"primitive" validate:"required"`
				Special   time.Time         `json:"special" validate:"required"`
				Custom    specialStringType `json:"custom" validate:"required"`
			}
		}
	}

	r := newServeMux()
	r.SetCustomSchemaTypes(CustomSchemaTypes{"special-string-type": &compilerHook{}})
	cs := r.compileSchema(&testSchema{}, Info{})

	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["primitive"].Type, "string", "primitive type is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["special"].Type, "string", "special type is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["special"].Format, "date-time", "special format is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["custom"].Type, "string", "custom type is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["custom"].Format, "string", "custom format is correctly set")
}

func TestCompilerHooksBindedRequest(t *testing.T) {
	type testSchema struct {
		Request struct {
			Path struct {
				Primitive string            `json:"primitive" validate:"required"`
				Custom    specialStringType `json:"custom" validate:"required"`
			}
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)
			if err != nil {
				log.Fatalln(err)
			}

			assert.Equal(t, s.Request.Path.Primitive, "prime1", "primitive binded value is correct")
			assert.Equal(t, s.Request.Path.Custom.Val(), "custom1", "custom binded value is correct")
			return nil
		},
	}

	r := newServeMux()
	r.SetCustomSchemaTypes(CustomSchemaTypes{"special-string-type": &compilerHook{}})
	r.Inject(InjectOptions{
		Path:   "/test/:primitive/:custom",
		Method: "GET",
		Paths: map[string]string{
			"primitive": "prime1",
			"custom":    "custom1",
		},
		Handler: &handler,
	})
}

func TestCompilerHooksBindedResponse(t *testing.T) {
	type testSchemaBody struct {
		Primitive string            `json:"primitive" validate:"required"`
		Custom    specialStringType `json:"custom" validate:"required"`
	}

	type testSchema struct {
		Ok struct {
			Body testSchemaBody
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)
			if err != nil {
				log.Fatalln(err)
			}

			s.Ok.Body.Primitive = "john"
			s.Ok.Body.Custom = specialStringType{val: "doe"}
			return c.JSON(200, s.Ok)
		},
	}

	r := newServeMux()
	r.SetCustomSchemaTypes(CustomSchemaTypes{"special-string-type": &compilerHook{}})
	resp, err := r.Inject(InjectOptions{
		Path:    "/test/:primitive/:custom",
		Method:  "GET",
		Handler: &handler,
	})
	if err != nil {
		log.Fatalln(err)
	}

	var data testSchemaBody
	err = json.Unmarshal(resp.Body.Bytes(), &data)
	if err != nil {
		log.Fatalln(err)
	}

	assert.Equal(t, data.Primitive, "john")
	assert.Equal(t, data.Custom.Val(), "doe")
}

type specialStringType struct {
	val string
}

func (s *specialStringType) Val() string {
	return s.val
}

type compilerHook struct {
}

func (c *compilerHook) MatchType(t reflect.Type) bool {
	return t == reflect.TypeOf(specialStringType{})
}

func (c *compilerHook) CustomOpenapiTypes(t reflect.Type) CustomOpenapiTypes {
	return CustomOpenapiTypes{Type: "string", Format: "string"}
}

func (c *compilerHook) BindValueToType(val any) (any, error) {
	if v, ok := val.(string); ok {
		return specialStringType{val: v}, nil
	} else {
		return nil, errors.New("error casting special string type")
	}
}

func (c *compilerHook) GetPrimitiveValue(val any) (string, error) {
	if v, ok := val.(specialStringType); ok {
		return v.Val(), nil
	} else {
		return "", errors.New("unknown value type. unable to convert to string")
	}
}
