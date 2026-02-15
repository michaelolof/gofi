package gofi

import (
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOpenAPISpecsMethod(t *testing.T) {

	type testSchema struct {
		Ok struct {
			Body struct {
				Primitive string     `json:"primitive" validate:"required"`
				Special   time.Time  `json:"special" validate:"required"`
				Custom    vendorType `json:"custom" validate:"required" spec:"custom"`
			}
		}
	}

	r := newServeMux()
	r.RegisterSpec(&vendorSpec{})
	cs := r.compileSchema(&testSchema{}, Info{})

	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["primitive"].Type, "string", "primitive type is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["special"].Type, "string", "special type is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["special"].Format, "date-time", "special format is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["custom"].Type, "string", "custom type is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["custom"].Format, "string", "custom format is correctly set")
}

func TestCompilerHooksOpenAPISpecs(t *testing.T) {

	type testSchema struct {
		Ok struct {
			Body struct {
				Primitive string     `json:"primitive" validate:"required"`
				Special   time.Time  `json:"special" validate:"required"`
				Custom    vendorType `json:"custom" validate:"required" spec:"custom"`
			}
		}
	}

	r := newServeMux()
	r.RegisterSpec(&vendorSpec{})
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
				Primitive string     `json:"primitive" validate:"required"`
				Custom    vendorType `json:"custom" validate:"required" spec:"custom"`
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
	r.RegisterSpec(&vendorSpec{})
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
		Primitive string     `json:"primitive" validate:"required"`
		Custom    vendorType `json:"custom" validate:"required" spec:"custom"`
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
			s.Ok.Body.Custom = vendorType{val: "doe"}
			return c.Send(200, s.Ok)
		},
	}

	r := newServeMux()
	r.RegisterSpec(&vendorSpec{})
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

func TestDynamicStructTags(t *testing.T) {
	type testSchema struct {
		Ok struct {
			Body struct {
				Primitive string         `json:"primitive" validate:"required,oneof=june july august"`
				Special   time.Time      `json:"special" validate:"required"`
				Custom    dynamicTagType `json:"custom" validate:"required,oneof@OneOf" spec:"dynamic" description:"@MyDescription"`
			}
		}
	}

	r := newServeMux()
	r.RegisterSpec(&vendorSpec{id: "dynamic"})
	cs := r.compileSchema(&testSchema{}, Info{})

	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["primitive"].Enum, []any{"june", "july", "august"})
	assert.Contains(t, cs.specs.responsesSchema["Ok"].Required, "special")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["custom"].Enum, []any{"monday", "tuesday"})
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["custom"].Description, "this is a description")
}

type vendorType struct {
	val string
}

func (s *vendorType) Val() string {
	return s.val
}

func (u vendorType) MarshalJSON() ([]byte, error) {
	return []byte(u.val), nil
}

func (s *vendorType) UnmarshalJSON(data []byte) error {
	v, err := strconv.Unquote(string(data))
	if err == nil {
		s.val = v
	} else {
		s.val = string(data)
	}
	return nil
}

type vendorSpec struct{ id string }

func (v *vendorSpec) SpecID() string {
	if v.id != "" {
		return v.id
	}
	return "custom"
}

func (v *vendorSpec) Type() string {
	return "string"
}

func (v *vendorSpec) Format() string {
	return "string"
}

func (v *vendorSpec) Decode(val any) (any, error) {
	if v, ok := val.(string); ok {
		return vendorType{val: v}, nil
	} else {
		return nil, errors.New("unable to decode vendor")
	}
}

func (v *vendorSpec) Encode(val any) (string, error) {
	if v, ok := val.(vendorType); ok {
		return v.val, nil
	} else {
		return "", errors.New("unable to encode vendor")
	}
}

type dynamicTagType struct{}

func (d dynamicTagType) OneOf() string {
	return "monday tuesday"
}

func (d dynamicTagType) MyDescription() string {
	return "this is a description"
}
