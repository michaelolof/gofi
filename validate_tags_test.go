package gofi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type omitEmptySchema struct {
	Request struct {
		Query struct {
			Site string `json:"site" validate:"omitempty,url"`
		}
		Body struct {
			Email string   `json:"email" validate:"omitempty,email"`
			Tags  []string `json:"tags" validate:"omitempty,min=2"`
			Age   int      `json:"age" validate:"omitempty,gte=18"`
		}
	}
}

func newOmitEmptyRouter() Router {
	r := NewRouter()
	r.Post("/p", RouteOptions{
		Schema: &omitEmptySchema{},
		Handler: func(c Context) error {
			if _, err := ValidateAndBind[omitEmptySchema](c); err != nil {
				return err
			}
			return c.SendString(200, "ok")
		},
	})
	return r
}

func TestValidate_OmitEmpty(t *testing.T) {
	r := newOmitEmptyRouter()
	cases := []struct {
		name string
		path string
		body string
		want int
	}{
		{"empty string skips rules", "/p", `{"email":""}`, 200},
		{"empty slice skips rules", "/p", `{"tags":[]}`, 200},
		{"zero number skips rules", "/p", `{"age":0}`, 200},
		{"empty query skips rules", "/p?site=", `{}`, 200},
		{"non-empty string is validated", "/p", `{"email":"nope"}`, 400},
		{"non-empty slice is validated", "/p", `{"tags":["a"]}`, 400},
		{"non-zero number is validated", "/p", `{"age":12}`, 400},
		{"non-empty query is validated", "/p?site=nope", `{}`, 400},
		{"valid values pass", "/p?site=https://example.com", `{"email":"a@b.co","tags":["a","b"],"age":30}`, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := r.Test(TestOptions{
				Method:  "POST",
				Path:    tc.path,
				Headers: map[string]string{"content-type": "application/json"},
				Body:    strings.NewReader(tc.body),
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, resp.Code(), resp.BodyString())
		})
	}
}

func TestCompile_RejectsUnknownRule(t *testing.T) {
	type schema struct {
		Request struct {
			Body struct {
				Items []string `json:"items" validate:"required,dive"`
			}
		}
	}
	r := NewRouter()
	assert.PanicsWithValue(t,
		`gofi: unknown validation rule "dive" in validate tag "required,dive" on field Items; register custom rules with RegisterValidator before the routes that use them`,
		func() { r.Post("/p", RouteOptions{Schema: &schema{}, Handler: func(c Context) error { return nil }}) },
	)
}

func TestCompile_RejectsOmitEmptyWithRequired(t *testing.T) {
	type schema struct {
		Request struct {
			Body struct {
				Name string `json:"name" validate:"required,omitempty,min=2"`
			}
		}
	}
	r := NewRouter()
	assert.Panics(t, func() {
		r.Post("/p", RouteOptions{Schema: &schema{}, Handler: func(c Context) error { return nil }})
	})
}

type evenValidator struct{}

func (evenValidator) Name() string { return "even" }

func (evenValidator) Rule(c ValidatorContext) func(val any) error {
	return func(val any) error {
		if n, ok := val.(int); ok && n%2 == 0 {
			return nil
		}
		return errors.New("value must be even")
	}
}

func TestCompile_AcceptsCustomRuleRegisteredFirst(t *testing.T) {
	type schema struct {
		Request struct {
			Body struct {
				N int `json:"n" validate:"even"`
			}
		}
	}
	r := NewRouter()
	r.RegisterValidator(evenValidator{})
	assert.NotPanics(t, func() {
		r.Post("/p", RouteOptions{
			Schema: &schema{},
			Handler: func(c Context) error {
				if _, err := ValidateAndBind[schema](c); err != nil {
					return err
				}
				return c.SendString(200, "ok")
			},
		})
	})

	resp, err := r.Test(TestOptions{Method: "POST", Path: "/p", Body: strings.NewReader(`{"n":3}`)})
	require.NoError(t, err)
	assert.Equal(t, 400, resp.Code())
}

func TestCompile_RejectsCustomRuleRegisteredLate(t *testing.T) {
	type schema struct {
		Request struct {
			Body struct {
				N int `json:"n" validate:"even"`
			}
		}
	}
	r := NewRouter()
	assert.Panics(t, func() {
		r.Post("/p", RouteOptions{Schema: &schema{}, Handler: func(c Context) error { return nil }})
	})
}

// registrationPanic registers schema and returns what the registration panicked with.
func registrationPanic(specs []CustomSpec, schema any) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprint(r)
		}
	}()
	r := NewRouter()
	if len(specs) > 0 {
		r.RegisterSpec(specs...)
	}
	r.Post("/p", RouteOptions{Schema: schema, Handler: func(c Context) error { return nil }})
	return ""
}

func TestCompile_InvalidOneOfOptionPanics(t *testing.T) {
	type intField struct {
		Request struct {
			Body struct {
				V int `json:"v" validate:"oneof=abc"`
			}
		}
	}
	type smallField struct {
		Request struct {
			Body struct {
				V int8 `json:"v" validate:"oneof=1 abc"`
			}
		}
	}
	type floatField struct {
		Request struct {
			Query struct {
				V float64 `json:"v" validate:"oneof=abc"`
			}
		}
	}
	for name, tc := range map[string]struct {
		schema any
		option string
	}{
		"int":   {&intField{}, "abc"},
		"int8":  {&smallField{}, "abc"},
		"float": {&floatField{}, "abc"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, fmt.Sprintf(`gofi: invalid oneof option %q on field v; it must be a number`, tc.option), registrationPanic(nil, tc.schema))
		})
	}
}

func TestCompile_RejectsUnreadableParam(t *testing.T) {
	type bytes4 [4]byte
	type path struct {
		Request struct {
			Path struct {
				V bytes4 `json:"v"`
			}
		}
	}
	type query struct {
		Request struct {
			Query struct {
				V bytes4 `json:"v"`
			}
		}
	}
	type header struct {
		Request struct {
			Header struct {
				V bytes4 `json:"x-v"`
			}
		}
	}
	type slice struct {
		Request struct {
			Query struct {
				V []string `json:"v"`
			}
		}
	}
	type structField struct {
		Request struct {
			Query struct {
				V struct{ A string } `json:"v"`
			}
		}
	}
	type pointerToArray struct {
		Request struct {
			Query struct {
				V *bytes4 `json:"v"`
			}
		}
	}

	t.Run("panics", func(t *testing.T) {
		for name, tc := range map[string]struct {
			schema any
			want   string
		}{
			"path":             {&path{}, "gofi: path field V of type gofi.bytes4 can't be read from text"},
			"query":            {&query{}, "gofi: query field V of type gofi.bytes4 can't be read from text"},
			"header":           {&header{}, "gofi: header field V of type gofi.bytes4 can't be read from text"},
			"slice":            {&slice{}, "gofi: query field V of type []string can't be read from text"},
			"struct":           {&structField{}, "gofi: query field V of type struct { A string } can't be read from text"},
			"pointer to array": {&pointerToArray{}, "gofi: query field V of type *gofi.bytes4 can't be read from text"},
		} {
			t.Run(name, func(t *testing.T) {
				assert.Contains(t, registrationPanic(nil, tc.schema), tc.want)
			})
		}
	})

	t.Run("readable types register", func(t *testing.T) {
		type named string
		type namedInt int
		type readable struct {
			Request struct {
				Path struct {
					P string `json:"p"`
				}
				Query struct {
					S  string     `json:"s"`
					N  named      `json:"n"`
					I  int        `json:"i"`
					NI namedInt   `json:"ni"`
					U  uint64     `json:"u"`
					F  float64    `json:"f"`
					B  bool       `json:"b"`
					PS *string    `json:"ps"`
					PN *named     `json:"pn"`
					T  time.Time  `json:"t"`
					PT *time.Time `json:"pt"`
				}
				Header struct {
					H string `json:"x-h"`
				}
			}
		}
		assert.Empty(t, registrationPanic(nil, &readable{}))
	})

	t.Run("a spec makes any type readable", func(t *testing.T) {
		type withSpec struct {
			Request struct {
				Query struct {
					A specID   `json:"a" spec:"spec-id"`
					B *specID  `json:"b" spec:"spec-id"`
					C specTags `json:"c" spec:"spec-tags"`
				}
			}
		}
		assert.Empty(t, registrationPanic(specKindSpecs(), &withSpec{}))
	})

	t.Run("a field left out of the schema is ignored", func(t *testing.T) {
		type skipped struct {
			Request struct {
				Query struct {
					V    string   `json:"v"`
					Skip []string `json:"-"`
				}
			}
		}
		assert.Empty(t, registrationPanic(nil, &skipped{}))
	})
}

func TestCompile_RejectsUnsupportedCookie(t *testing.T) {
	type bytes4 [4]byte
	type reqType struct {
		Request struct {
			Cookie struct {
				V bytes4 `json:"v"`
			}
		}
	}
	type reqRequired struct {
		Request struct {
			Cookie struct {
				V bytes4 `json:"v" validate:"required"`
			}
		}
	}
	type reqSpec struct {
		Request struct {
			Cookie struct {
				V string `json:"v" spec:"spec-name"`
			}
		}
	}
	type reqSpecAndType struct {
		Request struct {
			Cookie struct {
				V specID `json:"v" spec:"spec-id"`
			}
		}
	}
	type respType struct {
		Ok struct {
			Cookie struct {
				V bytes4 `json:"v"`
			}
			Body string
		}
	}
	type respSpec struct {
		Ok struct {
			Cookie struct {
				V string `json:"v" spec:"spec-name"`
			}
			Body string
		}
	}

	const badType = "gofi: unsupported type gofi.bytes4 on cookie field V; use a primitive type or http.Cookie"
	const badSpec = "gofi: spec tag on cookie field V; a cookie is a primitive type or http.Cookie, and doesn't use a spec"
	for name, tc := range map[string]struct {
		schema any
		want   string
	}{
		"request type":          {&reqType{}, badType},
		"request type required": {&reqRequired{}, badType},
		"request spec":          {&reqSpec{}, badSpec},
		"request spec and type": {&reqSpecAndType{}, badSpec},
		"response type":         {&respType{}, badType},
		"response spec":         {&respSpec{}, badSpec},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, registrationPanic(specKindSpecs(), tc.schema))
		})
	}

	t.Run("supported cookies register and bind", func(t *testing.T) {
		type named string
		type schema struct {
			Request struct {
				Cookie struct {
					S    string       `json:"s" validate:"required"`
					I    int          `json:"i"`
					N    named        `json:"n"`
					PS   *string      `json:"ps"`
					C    http.Cookie  `json:"c"`
					PC   *http.Cookie `json:"pc"`
					Skip []string     `json:"-"`
				}
			}
		}
		r := NewRouter()
		var bound *schema
		r.Post("/p", RouteOptions{Schema: &schema{}, Handler: func(c Context) error {
			s, err := ValidateAndBind[schema](c)
			bound = s
			return err
		}})
		resp, err := r.Test(TestOptions{Method: "POST", Path: "/p", Cookies: []http.Cookie{
			{Name: "s", Value: "a"}, {Name: "i", Value: "7"}, {Name: "n", Value: "b"},
			{Name: "ps", Value: "c"}, {Name: "c", Value: "d"}, {Name: "pc", Value: "e"},
		}})
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		q := bound.Request.Cookie
		assert.Equal(t, "a", q.S)
		assert.Equal(t, 7, q.I)
		assert.Equal(t, named("b"), q.N)
		assert.Equal(t, "c", *q.PS)
		assert.Equal(t, "d", q.C.Value)
		assert.Equal(t, "e", q.PC.Value)
	})
}
