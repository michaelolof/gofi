package gofi

import (
	"errors"
	"strings"
	"testing"

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
