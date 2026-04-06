package gofi

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
				One   string       `validate:"required" default:"startings"`
				Two   *http.Cookie `validate:"required" default:"two"`
				Three customStruct `validate:"required" default:"three"`
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
