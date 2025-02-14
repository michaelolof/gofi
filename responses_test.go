package gofi

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type customStruct struct {
}

func (c customStruct) IsCustomType(typ reflect.Type) (*CustomSchemaProps, bool) {
	if reflect.TypeOf(customStruct{}) == typ {
		return &CustomSchemaProps{Type: "string", Format: ""}, true
	} else {
		return nil, false
	}
}

func (c customStruct) CustomDecode(val any) (string, error) {
	return "custom-type", nil
}

func (c customStruct) CustomEncode(val any) (any, error) {
	return customStruct{}, nil
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
			// Header struct {
			// 	Une  string       `validate:"required" default:"one-in-french"`
			// 	Duex time.Time    `json:"deux" validate:"required"`
			// 	Tres customStruct `json:"tres" validate:"required"`
			// }

			// Cookie struct {
			// 	One   string       `validate:"required" default:"startings"`
			// 	Two   *http.Cookie `validate:"required" default:"two"`
			// 	Three customStruct `validate:"required" default:"three"`
			// }

			Body struct {
				Two int `json:"two" validate:"required" default:"20"`
				// One     string            `json:"one" validate:"required"`
				// Casttro map[string]varmin `json:"castor" validate:"required"`
			} `validate:"required"`
		}
	}

	mux := NewServeMux()
	mux.SetCustomSchemaTypes(map[string]CustomSchemaType{
		"custom_struct": customStruct{},
	})

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
