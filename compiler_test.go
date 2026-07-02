package gofi

import (
	"encoding/json"
	"errors"
	"log"
	"reflect"
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

	r := newRouter()
	r.RegisterSpec(&vendorSpec{})
	cs := r.compileSchema(&testSchema{}, Info{})

	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["primitive"].Type, "string", "primitive type is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["special"].Type, "string", "special type is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["special"].Format, "date-time", "special format is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["custom"].Type, "string", "custom type is correctly set")
	assert.Equal(t, cs.specs.responsesSchema["Ok"].Properties["custom"].Format, "string", "custom format is correctly set")
}

func TestCompilerTags(t *testing.T) {
	type testSchema struct {
		Ok struct {
			Body string `json:"body"`
		}
	}

	r := newRouter()
	cs := r.compileSchema(&testSchema{}, Info{
		Tags: []string{"User", "Profile"},
	})

	assert.Equal(t, cs.specs.Tags, []string{"User", "Profile"}, "tags are correctly propagated")
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

	r := newRouter()
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

	r := newRouter()
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

	r := newRouter()
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
	err = json.Unmarshal(resp.Body, &data)
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

	r := newRouter()
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

// =============================================================================
// isPromotedEmbed unit tests
// =============================================================================

func TestIsPromotedEmbed_UntaggedStruct(t *testing.T) {
	// An anonymous struct field with NO json tag should be promoted (carrier skipped).
	type Embed struct {
		Field string `json:"field"`
	}
	type Outer struct {
		Embed // anonymous, no json tag
	}

	typ := reflect.TypeOf(Outer{})
	f, _ := typ.FieldByName("Embed")
	assert.True(t, isPromotedEmbed(f), "untagged anonymous struct should be a promoted embed")
}

func TestIsPromotedEmbed_TaggedStruct(t *testing.T) {
	// An anonymous struct field WITH an explicit json name is NOT promoted.
	type Embed struct {
		Field string `json:"field"`
	}
	type Outer struct {
		Embed `json:"meta"` // tagged embed stops promotion
	}

	typ := reflect.TypeOf(Outer{})
	f, _ := typ.FieldByName("Embed")
	assert.False(t, isPromotedEmbed(f), "tagged anonymous struct should NOT be a promoted embed")
}

func TestIsPromotedEmbed_NonStructAnonymous(t *testing.T) {
	// An anonymous NON-struct (e.g. named type) is NOT a promoted embed carrier.
	type MyString string
	type Outer struct {
		MyString // anonymous, but kind is string
	}

	typ := reflect.TypeOf(Outer{})
	f, _ := typ.FieldByName("MyString")
	assert.False(t, isPromotedEmbed(f), "anonymous non-struct should NOT be a promoted embed")
}

func TestIsPromotedEmbed_RegularField(t *testing.T) {
	// A regular (non-anonymous) struct field should not be skipped.
	type Inner struct {
		Field string `json:"field"`
	}
	type Outer struct {
		Data Inner `json:"data"` // not anonymous
	}

	typ := reflect.TypeOf(Outer{})
	f, _ := typ.FieldByName("Data")
	assert.False(t, isPromotedEmbed(f), "regular named field should NOT be a promoted embed")
}

func TestIsPromotedEmbed_JsonDash(t *testing.T) {
	// An anonymous struct field with json:"-" should still be treated
	// as promoted (the carrier is skipped, children promoted).
	type Embed struct {
		Field string `json:"field"`
	}
	type Outer struct {
		Embed `json:"-"` // json:"-" means skip entirely
	}

	typ := reflect.TypeOf(Outer{})
	f, _ := typ.FieldByName("Embed")
	assert.True(t, isPromotedEmbed(f), "anonymous struct with json:\"-\" should be a promoted embed")
}

func TestIsPromotedEmbed_JsonEmptyName(t *testing.T) {
	// An anonymous struct field with json:",omitempty" (empty name) should still be promoted.
	type Embed struct {
		Field string `json:"field"`
	}
	type Outer struct {
		Embed `json:",omitempty"`
	}

	typ := reflect.TypeOf(Outer{})
	f, _ := typ.FieldByName("Embed")
	assert.True(t, isPromotedEmbed(f), "anonymous struct with empty json name should be a promoted embed")
}

// =============================================================================
// Spec-generation tests for embedded (anonymous) struct handling
// =============================================================================

func TestCompileSchema_EmbeddedHeaderFields(t *testing.T) {
	// An untagged anonymous struct in Request.Header must have its children
	// promoted as flat header params. The carrier itself must NOT appear.
	type AuthHeader struct {
		Authorization string `json:"authorization" description:"Bearer token"`
	}

	type schema struct {
		Request struct {
			Header struct {
				AuthHeader // embedded, no json tag → should be promoted
			}
		}
	}

	r := newRouter()
	cs := r.compileSchema(&schema{}, Info{})

	// Should have exactly one parameter: "authorization"
	params := cs.specs.Parameters
	assert.Len(t, params, 1, "should have exactly 1 header parameter")

	// The carrier "AuthHeader" / "authheader" must not appear
	for _, p := range params {
		assert.NotEqual(t, "AuthHeader", p.Name, "carrier field name should be skipped")
		assert.NotEqual(t, "authheader", p.Name, "lowercased carrier field name should be skipped")
	}

	// The promoted child should be there
	assert.Equal(t, "authorization", params[0].Name)
	assert.Equal(t, "header", params[0].In)
	assert.Equal(t, "string", params[0].Schema.Type)
	assert.Equal(t, "Bearer token", params[0].Schema.Description)
}

func TestCompileSchema_EmbeddedBodyFields(t *testing.T) {
	// An untagged anonymous struct in Request.Body must have its children
	// promoted as flat properties. The carrier itself must NOT appear.
	type Meta struct {
		RequestID string `json:"request_id"`
		Source    string `json:"source"`
	}

	type schema struct {
		Request struct {
			Body struct {
				Name string `json:"name" validate:"required"`
				Meta        // embedded, no json tag → should be promoted
			}
		}
	}

	r := newRouter()
	cs := r.compileSchema(&schema{}, Info{})

	body := cs.specs.bodySchema
	assert.NotNil(t, body)
	assert.Equal(t, "object", body.Type)

	// Should have 3 properties: name, request_id, source
	assert.Len(t, body.Properties, 3, "should have exactly 3 body properties")

	// The carrier "Meta" / "meta" must not appear
	_, hasMeta := body.Properties["Meta"]
	assert.False(t, hasMeta, "carrier field 'Meta' should not be a property")
	_, hasMetaLower := body.Properties["meta"]
	assert.False(t, hasMetaLower, "carrier field 'meta' should not be a property")

	// The promoted children should be there
	assert.Contains(t, body.Properties, "request_id")
	assert.Equal(t, "string", body.Properties["request_id"].Type)
	assert.Contains(t, body.Properties, "source")
	assert.Equal(t, "string", body.Properties["source"].Type)
	assert.Contains(t, body.Properties, "name")
	assert.Equal(t, "string", body.Properties["name"].Type)

	// "name" is required; the promoted fields are not
	assert.Equal(t, []string{"name"}, body.Required)
}

func TestCompileSchema_TwoUntaggedEmbeds(t *testing.T) {
	// The exact Nuvion case: two untagged embeds in Header.
	type AuthHeader struct {
		Authorization string `json:"authorization"`
	}

	type MockHeader struct {
		XMockOutcome string `json:"x-mock-outcome"`
		XMockCount   int    `json:"x-mock-count"`
	}

	type schema struct {
		Request struct {
			Header struct {
				AuthHeader  // promoted
				MockHeader  // promoted
			}
		}
	}

	r := newRouter()
	cs := r.compileSchema(&schema{}, Info{})

	params := cs.specs.Parameters

	// Should have exactly 3 params: authorization, x-mock-outcome, x-mock-count
	assert.Len(t, params, 3, "should have 3 promoted header params, no carriers")

	paramNames := make(map[string]bool)
	for _, p := range params {
		paramNames[p.Name] = true
	}
	assert.True(t, paramNames["authorization"], "authorization should be present")
	assert.True(t, paramNames["x-mock-outcome"], "x-mock-outcome should be present")
	assert.True(t, paramNames["x-mock-count"], "x-mock-count should be present")
	assert.False(t, paramNames["authheader"], "carrier should not be present")
	assert.False(t, paramNames["mockheader"], "carrier should not be present")
}

func TestCompileSchema_EmbeddedResponseBody(t *testing.T) {
	// An untagged anonymous struct in a response body must also be flattened.
	type RespInfo struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}

	type schema struct {
		Ok struct {
			Body struct {
				Message string `json:"message"`
				RespInfo        // embedded
			}
		}
	}

	r := newRouter()
	cs := r.compileSchema(&schema{}, Info{})

	okBody, ok := cs.specs.responsesSchema["Ok"]
	assert.True(t, ok)
	assert.Len(t, okBody.Properties, 3, "should have 3 properties: message, status, version")

	assert.Contains(t, okBody.Properties, "message")
	assert.Contains(t, okBody.Properties, "status")
	assert.Contains(t, okBody.Properties, "version")
	_, hasRespInfo := okBody.Properties["RespInfo"]
	assert.False(t, hasRespInfo, "carrier 'RespInfo' should not appear")
	_, hasRespInfoLower := okBody.Properties["respinfo"]
	assert.False(t, hasRespInfoLower, "carrier 'respinfo' should not appear")
}

func TestCompileSchema_MixedOwnAndEmbeddedFields(t *testing.T) {
	// Parent struct with its own fields + an untagged embed + a tagged embed.
	type Inner struct {
		A string `json:"a"`
		B string `json:"b"`
	}

	type schema struct {
		Request struct {
			Body struct {
				Own   string `json:"own" validate:"required"`
				Inner        // untagged embed → promoted
				Meta  Inner  `json:"meta"` // tagged embed → nested object
			}
		}
	}

	r := newRouter()
	cs := r.compileSchema(&schema{}, Info{})

	body := cs.specs.bodySchema

	// Should have: own, a, b, meta
	assert.Len(t, body.Properties, 4, "should have own + promoted a,b + nested meta")

	assert.Contains(t, body.Properties, "own")
	assert.Contains(t, body.Properties, "a")
	assert.Contains(t, body.Properties, "b")

	// "meta" should be a nested object (not flattened)
	metaProp, hasMeta := body.Properties["meta"]
	assert.True(t, hasMeta, "tagged embed 'meta' should be a property")
	assert.Equal(t, "object", metaProp.Type)
	assert.Contains(t, metaProp.Properties, "a")
	assert.Contains(t, metaProp.Properties, "b")
}

func TestCompileSchema_NestedEmbeds(t *testing.T) {
	// Embed A which itself embeds B (all untagged) — B's fields promoted all the way up.
	type B struct {
		FieldB string `json:"field_b"`
	}
	type A struct {
		B           // embedded in A
		FieldA string `json:"field_a"`
	}

	type schema struct {
		Request struct {
			Body A
		}
	}

	r := newRouter()
	cs := r.compileSchema(&schema{}, Info{})

	body := cs.specs.bodySchema
	assert.Len(t, body.Properties, 2, "should have field_a and field_b, no carriers")
	assert.Contains(t, body.Properties, "field_a")
	assert.Contains(t, body.Properties, "field_b")
	_, hasA := body.Properties["A"]
	assert.False(t, hasA)
	_, hasB := body.Properties["B"]
	assert.False(t, hasB)
}

func TestCompileSchema_JsonDashField(t *testing.T) {
	// json:"-" fields should be excluded from parameters.
	type schema struct {
		Request struct {
			Header struct {
				Visible string `json:"visible"`
				Hidden  string `json:"-"`
			}
		}
	}

	r := newRouter()
	cs := r.compileSchema(&schema{}, Info{})

	params := cs.specs.Parameters
	assert.Len(t, params, 1, "only visible should appear")
	assert.Equal(t, "visible", params[0].Name)
}

func TestCompileSchema_EmbeddedJsonDashField(t *testing.T) {
	// An anonymous struct field with json:"-" should have its carrier skipped.
	// The carrier's children still have their own valid json tags and will
	// appear as promoted entries (matching what reflect.VisibleFields returns).
	// Full child suppression under json:"-" embeds is deferred (§5.3).
	type DashInfo struct {
		Field string `json:"field"`
	}

	type schema struct {
		Request struct {
			Header struct {
				DashInfo `json:"-"` // json:"-", carrier skipped
			}
		}
	}

	r := newRouter()
	cs := r.compileSchema(&schema{}, Info{})

	// The carrier ("DashInfo" / "dashinfo") must not appear
	params := cs.specs.Parameters
	for _, p := range params {
		assert.NotEqual(t, "DashInfo", p.Name, "carrier name should not appear")
		assert.NotEqual(t, "dashinfo", p.Name, "lowercased carrier name should not appear")
	}
}
