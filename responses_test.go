package gofi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type customStruct struct {
}

func TestSend(t *testing.T) {

	type xcamile struct {
		Ding string `json:"ding" validate:"required"`
	}

	type varmin struct {
		One   string       `json:"one,omitempty" validate:"required"`
		Two   int          `json:"two" validate:"required"`
		Three [][]xcamile  `json:"three"`
		Four  time.Time    `json:"four" validate:"required"`
		Five  customStruct `json:"five" validate:"required"`
	}

	type testSchema struct {
		Ok struct {
			Header struct {
				Une  string       `validate:"required" default:"one-in-french"`
				Duex time.Time    `json:"deux" validate:"required"`
				Tres customStruct `json:"tres" validate:"required"`
			}

			Cookie struct {
				One string       `validate:"required" default:"startings"`
				Two *http.Cookie `validate:"required" default:"two"`
			}

			Body struct {
				Two     int               `json:"two" validate:"required" default:"20"`
				One     string            `json:"one" validate:"required"`
				Casttro map[string]varmin `json:"castor" validate:"required"`
			} `validate:"required"`
		}
	}

	mux := NewRouter()

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, _ := ValidateAndBind[testSchema](c)
			// s.Ok.Body.Casttro = map[string]varmin{
			// 	"action": {One: "Unxier", Two: 344, Three: nil, Four: time.Now()},
			// }
			return c.Send(200, s.Ok)
		},
	}

	res, err := mux.Inject(InjectOptions{
		Path:    "/test",
		Method:  "GET",
		Handler: &handler,
	})

	assert.Nil(t, err)
	fmt.Println(res)
}

func TestCheckStuff(t *testing.T) {
}

func TestTypeAliasResponse(t *testing.T) {

	type TypeHint string

	type testSchema struct {
		Ok struct {
			Body struct {
				Hint TypeHint `json:"hint" validate:"required,oneof=good bad ugly"`
			}
		}
	}

	mux := NewRouter()
	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, _ := ValidateAndBind[testSchema](c)
			s.Ok.Body.Hint = "good"
			return c.Send(200, s.Ok)
		},
	}

	res, err := mux.Inject(InjectOptions{
		Path:    "/test",
		Method:  "GET",
		Handler: &handler,
	})

	assert.Equal(t, string(res.Body), `{"hint":"good"}`)
	assert.Nil(t, err)
}

// TestSend_StructRequiredWithZeroFields verifies that a response body struct tagged
// validate:"required" does not produce a validation error when its inner fields are
// zero-valued (e.g. a nil slice). The struct itself is always present — IsRequired
// must not call IsZero() on struct kinds.
func TestSend_StructRequiredWithZeroFields(t *testing.T) {

	type Item struct {
		Name string `json:"name"`
	}

	type testSchema struct {
		Ok struct {
			Body struct {
				Items []Item `json:"items"`
			} `validate:"required"`
		}
	}

	mux := NewRouter()
	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			// Send with a nil Items slice — the body struct itself is present.
			return c.Send(200, testSchema{}.Ok)
		},
	}

	res, err := mux.Inject(InjectOptions{
		Path:    "/test",
		Method:  "GET",
		Handler: &handler,
	})

	assert.Nil(t, err, "expected no validation error for a required struct with zero-valued inner fields")
	assert.Equal(t, `{"items":[]}`, string(res.Body))
}

func TestAnyValueResponse(t *testing.T) {

	type testSchema struct {
		Ok struct {
			Body struct {
				Value any `json:"value" validate:"required"`
			}
		}
	}

	mux := NewRouter()
	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, _ := ValidateAndBind[testSchema](c)
			s.Ok.Body.Value = "foo"
			return c.Send(200, s.Ok)
		},
	}

	res, err := mux.Inject(InjectOptions{
		Path:    "/test",
		Method:  "GET",
		Handler: &handler,
	})

	assert.Equal(t, string(res.Body), `{"value":"foo"}`)
	assert.Nil(t, err)
}

// TestResponse_PresentTag_SliceEmpty verifies that validate:"present" on a direct Body slice
// does not produce a validation error when the slice is nil/empty in the response.
func TestResponse_PresentTag_SliceEmpty(t *testing.T) {

	type Item struct {
		Name string `json:"name"`
	}

	type testSchema struct {
		Ok struct {
			Body []Item `validate:"present"`
		}
	}

	mux := NewRouter()
	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			return c.Send(200, testSchema{}.Ok)
		},
	}

	res, err := mux.Inject(InjectOptions{
		Path:    "/test",
		Method:  "GET",
		Handler: &handler,
	})

	assert.Nil(t, err, "present tag should allow nil/empty slice in response")
	assert.Equal(t, `[]`, string(res.Body))
}

// TestResponse_PresentTag_FloatZero verifies that validate:"present" on a direct Body float64
// does not produce a validation error when the value is 0 in the response.
func TestResponse_PresentTag_FloatZero(t *testing.T) {

	type testSchema struct {
		Ok struct {
			Body float64 `validate:"present"`
		}
	}

	mux := NewRouter()
	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			return c.Send(200, testSchema{}.Ok)
		},
	}

	res, err := mux.Inject(InjectOptions{
		Path:    "/test",
		Method:  "GET",
		Handler: &handler,
	})

	assert.Nil(t, err, "present tag should allow zero float in response")
	assert.Equal(t, `0`, string(res.Body))
}

// TestResponse_RequiredRegression_SliceEmpty verifies that validate:"required" on a direct Body
// slice still rejects an empty slice (existing behavior preserved). When validation fails,
// the handler returns an error which errHandler converts to a 500 HTTP response.
func TestResponse_RequiredRegression_SliceEmpty(t *testing.T) {

	type Item struct {
		Name string `json:"name"`
	}

	type testSchema struct {
		Ok struct {
			Body []Item `validate:"required"`
		}
	}

	mux := NewRouter()
	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			var ok testSchema
			ok.Ok.Body = []Item{}
			return c.Send(200, ok.Ok)
		},
	}

	res, err := mux.Inject(InjectOptions{
		Path:    "/test",
		Method:  "GET",
		Handler: &handler,
	})

	assert.Nil(t, err) // Inject only propagates panics, not handler errors
	assert.Equal(t, 500, res.StatusCode, "required should reject empty slice in response (regression guard)")
}

// =============================================================================
// Embedded (anonymous) struct response encoding tests
// =============================================================================

func TestEmbeddedStruct_EncodeResponse(t *testing.T) {
	// An untagged embedded struct in a response body should be flattened,
	// NOT emitted as a nested object named after the Go type.
	type Info struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}

	type testSchema struct {
		Ok struct {
			Body struct {
				Message string `json:"message"`
				Info           // promoted
			}
		}
	}

	mux := NewRouter()
	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			var ok testSchema
			ok.Ok.Body.Message = "hello"
			ok.Ok.Body.Status = "active"
			ok.Ok.Body.Version = 2
			return c.Send(200, ok.Ok)
		},
	}

	res, err := mux.Inject(InjectOptions{
		Path:    "/test",
		Method:  "GET",
		Handler: &handler,
	})

	assert.Nil(t, err)
	// The response should be a flat object: {"message":"hello","status":"active","version":2}
	// NOT: {"message":"hello","Info":{"status":"active","version":2}}
	assert.JSONEq(t, `{"message":"hello","status":"active","version":2}`, string(res.Body))
}

// sendSpecResponse runs a GET route with schema S and returns what fill sends.
func sendSpecResponse[S any](t *testing.T, fill func(*S) any) (*InjectResponse, error) {
	t.Helper()
	m := NewRouter()
	m.RegisterSpec(specKindSpecs()...)
	m.Get("/t", RouteOptions{Schema: new(S), Handler: func(c Context) error {
		var s S
		return c.Send(200, fill(&s))
	}})
	resp, err := m.Test(TestOptions{Method: "GET", Path: "/t"})
	require.NotNil(t, resp)
	return resp, err
}

// Encode receives the value a pointer points to, as Decode returns it, and a nil
// pointer is null without calling Encode. The spec-name spec asserts val.(specName).
func TestCustomSpecResponse_PointerField(t *testing.T) {
	name := specName("deposit")

	type body struct {
		Ok struct {
			Body struct {
				V *specName `json:"v" spec:"spec-name"`
			}
		}
	}

	t.Run("set", func(t *testing.T) {
		resp, err := sendSpecResponse(t, func(s *body) any { s.Ok.Body.V = &name; return s.Ok })
		require.NoError(t, err)
		assert.Equal(t, `{"v":"deposit"}`, string(resp.Body))
	})

	t.Run("nil is null", func(t *testing.T) {
		resp, err := sendSpecResponse(t, func(s *body) any { return s.Ok })
		require.NoError(t, err)
		assert.Equal(t, `{"v":null}`, string(resp.Body))
	})

	t.Run("nil with omitempty is left out", func(t *testing.T) {
		type omit struct {
			Ok struct {
				Body struct {
					V *specName `json:"v,omitempty" spec:"spec-name"`
				}
			}
		}
		resp, err := sendSpecResponse(t, func(s *omit) any { return s.Ok })
		require.NoError(t, err)
		assert.Equal(t, `{}`, string(resp.Body))
	})

	t.Run("nil and required is still rejected", func(t *testing.T) {
		type required struct {
			Ok struct {
				Body struct {
					V *specName `json:"v" spec:"spec-name" validate:"required"`
				}
			}
		}
		resp, _ := sendSpecResponse(t, func(s *required) any { return s.Ok })
		assert.Equal(t, 500, resp.StatusCode)
	})

	t.Run("not a pointer", func(t *testing.T) {
		type plain struct {
			Ok struct {
				Body struct {
					V specName `json:"v" spec:"spec-name"`
				}
			}
		}
		resp, err := sendSpecResponse(t, func(s *plain) any { s.Ok.Body.V = name; return s.Ok })
		require.NoError(t, err)
		assert.Equal(t, `{"v":"deposit"}`, string(resp.Body))
	})

	t.Run("top-level pointer Body", func(t *testing.T) {
		type top struct {
			Ok struct {
				Body *specName `spec:"spec-name"`
			}
		}
		resp, err := sendSpecResponse(t, func(s *top) any { s.Ok.Body = &name; return s.Ok })
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, `"deposit"`, string(resp.Body))
	})

	t.Run("header", func(t *testing.T) {
		type header struct {
			Ok struct {
				Header struct {
					V *specName `json:"x-v" spec:"spec-name"`
				}
				Body string
			}
		}
		resp, err := sendSpecResponse(t, func(s *header) any { s.Ok.Header.V = &name; return s.Ok })
		require.NoError(t, err)
		assert.Equal(t, "deposit", resp.HeaderMap.Get("x-v"))

		resp, err = sendSpecResponse(t, func(s *header) any { return s.Ok })
		require.NoError(t, err, "a nil pointer doesn't call Encode")
		assert.Equal(t, 200, resp.StatusCode)
		assert.Empty(t, resp.HeaderMap.Get("x-v"))
	})
}

func TestCustomSpecResponse_Default(t *testing.T) {
	type schema struct {
		Ok struct {
			Body struct {
				Tags specTags  `json:"tags" spec:"spec-tags" default:"a,b"`
				Ptr  *specName `json:"ptr" spec:"spec-name" default:"deposit"`
				Name specName  `json:"name" spec:"spec-name" default:"alice"`
			}
		}
	}

	t.Run("an empty field takes the default", func(t *testing.T) {
		resp, err := sendSpecResponse(t, func(s *schema) any { return s.Ok })
		require.NoError(t, err)
		assert.Equal(t, `{"tags":"a,b","ptr":"deposit","name":"alice"}`, string(resp.Body))
	})

	t.Run("a set field wins", func(t *testing.T) {
		resp, err := sendSpecResponse(t, func(s *schema) any {
			s.Ok.Body.Tags = specTags{"x"}
			s.Ok.Body.Name = "bob"
			return s.Ok
		})
		require.NoError(t, err)
		assert.Equal(t, `{"tags":"x","ptr":"deposit","name":"bob"}`, string(resp.Body))
	})

	t.Run("header", func(t *testing.T) {
		type header struct {
			Ok struct {
				Header struct {
					V specName `json:"x-v" spec:"spec-name" default:"dflt"`
				}
				Body string
			}
		}
		resp, err := sendSpecResponse(t, func(s *header) any { return s.Ok })
		require.NoError(t, err)
		assert.Equal(t, "dflt", resp.HeaderMap.Get("x-v"))
	})

	t.Run("a default tag on a plain pointer or slice doesn't panic", func(t *testing.T) {
		type plain struct {
			Ok struct {
				Body struct {
					P *string  `json:"p" default:"x"`
					S []string `json:"s" default:"x"`
				}
			}
		}
		resp, err := sendSpecResponse(t, func(s *plain) any { return s.Ok })
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, `{"p":null,"s":[]}`, string(resp.Body))
	})
}

// Rules treat a named type like its plain type, with or without a spec.
func TestSend_RulesOnNamedTypes(t *testing.T) {
	type schema struct {
		Ok struct {
			Body struct {
				Min   specName  `json:"min" validate:"min=3"`
				Email specName  `json:"email" validate:"email"`
				Num   specCents `json:"num" validate:"min=3"`
				Ptr   *specName `json:"ptr" validate:"min=3"`
			}
		}
	}
	short := specName("ab")

	t.Run("valid values pass", func(t *testing.T) {
		ok := specName("abc")
		resp, err := sendSpecResponse(t, func(s *schema) any {
			s.Ok.Body.Min, s.Ok.Body.Email, s.Ok.Body.Num, s.Ok.Body.Ptr = "abc", "a@b.co", 5, &ok
			return s.Ok
		})
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, `{"min":"abc","email":"a@b.co","num":5,"ptr":"abc"}`, string(resp.Body))
	})

	t.Run("invalid values fail", func(t *testing.T) {
		resp, _ := sendSpecResponse(t, func(s *schema) any {
			s.Ok.Body.Min, s.Ok.Body.Email, s.Ok.Body.Num, s.Ok.Body.Ptr = "abc", "a@b.co", 5, &short
			return s.Ok
		})
		assert.Equal(t, 500, resp.StatusCode, "a response that breaks its own schema is a server error")
	})
}

type zeroWhenSeven struct{ N int }

func (z zeroWhenSeven) IsZero() bool { return z.N == 7 }

type zeroWhenSevenPtr struct{ N int }

func (z *zeroWhenSevenPtr) IsZero() bool { return z != nil && z.N == 7 }

type omitZeroFields struct {
	Keep    int               `json:"keep"`
	Time    time.Time         `json:"time,omitzero"`
	PTime   *time.Time        `json:"ptime,omitzero"`
	Struct  struct{ A int }   `json:"struct,omitzero"`
	Array   [2]int            `json:"array,omitzero"`
	Int     int               `json:"int,omitzero"`
	Uint    uint              `json:"uint,omitzero"`
	Float   float64           `json:"float,omitzero"`
	Str     string            `json:"str,omitzero"`
	Bool    bool              `json:"bool,omitzero"`
	Ptr     *int              `json:"ptr,omitzero"`
	Slice   []string          `json:"slice,omitzero"`
	Map     map[string]int    `json:"map,omitzero"`
	Any     any               `json:"any,omitzero"`
	Value   zeroWhenSeven     `json:"value,omitzero"`
	Pointer zeroWhenSevenPtr  `json:"pointer,omitzero"`
	PPtr    *zeroWhenSevenPtr `json:"pptr,omitzero"`
	Both    string            `json:"both,omitempty,omitzero"`
}

// omitzero gives the same output as encoding/json.
func TestSend_OmitZeroMatchesEncodingJSON(t *testing.T) {
	zero := 0
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := map[string]omitZeroFields{
		"all zero": {},
		"all set": {
			Keep: 1, Time: now, PTime: &now, Struct: struct{ A int }{1}, Array: [2]int{1}, Int: 1, Uint: 1, Float: 1,
			Str: "s", Bool: true, Ptr: &zero, Slice: []string{"a"}, Map: map[string]int{"a": 1}, Any: 1,
			Value: zeroWhenSeven{1}, Pointer: zeroWhenSevenPtr{1}, PPtr: &zeroWhenSevenPtr{1}, Both: "b",
		},
		"a pointer to zero is kept":     {Ptr: &zero},
		"a pointer to zero time is out": {PTime: &time.Time{}},
		"empty but not nil is kept":     {Slice: []string{}, Map: map[string]int{}},
		"any holding zero is kept":      {Any: time.Time{}},
		"IsZero decides":                {Value: zeroWhenSeven{7}, Pointer: zeroWhenSevenPtr{7}, PPtr: &zeroWhenSevenPtr{7}},
		"IsZero says not zero":          {Value: zeroWhenSeven{0}, Pointer: zeroWhenSevenPtr{0}, PPtr: &zeroWhenSevenPtr{0}},
	}

	type typed struct {
		Ok struct{ Body omitZeroFields }
	}
	type schemaless struct {
		Ok struct {
			Body struct {
				W any `json:"w"`
			}
		}
	}

	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			want, err := json.Marshal(v)
			require.NoError(t, err)

			resp, err := sendSpecResponse(t, func(s *typed) any { s.Ok.Body = v; return s.Ok })
			require.NoError(t, err)
			assert.Equal(t, string(want), string(resp.Body), "with schema rules")

			resp, err = sendSpecResponse(t, func(s *schemaless) any { s.Ok.Body.W = v; return s.Ok })
			require.NoError(t, err)
			assert.Equal(t, `{"w":`+string(want)+`}`, string(resp.Body), "held in an any field")
		})
	}
}

// A [N]byte field is base64 on the wire, as OpenAPI documents it, so a client can
// send back what it was sent.
func TestByteArrayField_Response(t *testing.T) {
	b := [4]byte{1, 2, 3, 4}
	enc := base64.StdEncoding.EncodeToString(b[:])
	type schema struct {
		Ok struct {
			Body struct {
				V [4]byte            `json:"v"`
				P *[4]byte           `json:"p"`
				N specBytes          `json:"n"`
				L [][4]byte          `json:"l"`
				M map[string][4]byte `json:"m"`
				A any                `json:"a"`
				S []byte             `json:"s"`
				I [2]int             `json:"i"`
			}
		}
	}

	t.Run("every shape is base64", func(t *testing.T) {
		resp, err := sendSpecResponse(t, func(s *schema) any {
			o := &s.Ok.Body
			o.V, o.P, o.N, o.L, o.M, o.A, o.S, o.I = b, &b, specBytes(b), [][4]byte{b}, map[string][4]byte{"k": b}, b, b[:], [2]int{1, 2}
			return s.Ok
		})
		require.NoError(t, err)
		want := fmt.Sprintf(`{"v":%[1]q,"p":%[1]q,"n":%[1]q,"l":[%[1]q],"m":{"k":%[1]q},"a":%[1]q,"s":%[1]q,"i":[1,2]}`, enc)
		assert.Equal(t, want, string(resp.Body))
	})

	t.Run("a zero array is base64 too", func(t *testing.T) {
		resp, err := sendSpecResponse(t, func(s *schema) any { return s.Ok })
		require.NoError(t, err)
		assert.Contains(t, string(resp.Body), `"v":"AAAAAA=="`)
		assert.Contains(t, string(resp.Body), `"p":null`)
	})

	t.Run("a request binds what a response sent, and OpenAPI agrees", func(t *testing.T) {
		type echo struct {
			Request struct {
				Body struct {
					V [4]byte `json:"v"`
				}
			}
			Ok struct {
				Body struct {
					V [4]byte `json:"v"`
				}
			}
		}
		r := NewRouter()
		r.Post("/e", RouteOptions{Schema: &echo{}, Handler: func(c Context) error {
			s, err := ValidateAndBind[echo](c)
			if err != nil {
				return err
			}
			s.Ok.Body.V = s.Request.Body.V
			return c.Send(200, s.Ok)
		}})
		sent := `{"v":"` + enc + `"}`
		resp, err := r.Test(TestOptions{Method: "POST", Path: "/e", Headers: map[string]string{"Content-Type": "application/json"}, Body: strings.NewReader(sent)})
		require.NoError(t, err)
		assert.Equal(t, sent, string(resp.Body))

		spec := specJSON(t, r, DocsOptions{})
		prop := dig(t, spec, "paths", "/e", "post", "requestBody", "content", "application/json", "schema", "properties", "v").(map[string]any)
		assert.Equal(t, "string", prop["type"])
		assert.Equal(t, "byte", prop["format"])
	})
}
