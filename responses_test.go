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

	mux := NewServeMux()

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

func TestTypeHints(t *testing.T) {

	type TypeHint string

	type testSchema struct {
		Ok struct {
			Body struct {
				Hint TypeHint `json:"hint" validate:"required,oneof=good bad ugly"`
			}
		}
	}

	mux := NewServeMux()
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

	assert.Nil(t, err)
	fmt.Println(res)
}
