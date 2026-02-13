package gofi

import (
	"fmt"
	"net/http"
	"testing"
)

func Test_NewTestRouter(t *testing.T) {

	type oneSchema struct {
		Request struct {
			Header struct {
				One string `json:"one" validate:"required"`
				Two string `json:"two" validate:"required"`
			}
		}
		Ok struct {
			Header struct {
				ContentType string `json:"content-type" default:"application/json"`
			}
			Body string `validate:"required"`
		}
	}

	type twoSchema struct {
		Request struct {
			Header struct {
				Three string `json:"one" validate:"required"`
				Four  string `json:"two" validate:"required"`
			}
		}
	}

	h1 := DefineHandler(RouteOptions{
		Schema: &oneSchema{},
		Handler: func(c Context) error {
			fmt.Println("Handler 1 was called")
			s, err := ValidateAndBind[oneSchema](c)
			if err != nil {
				return err
			}

			s.Ok.Body = "Something good is happening"
			return c.Send(200, s.Ok)
		},
	})

	h2 := DefineHandler(RouteOptions{
		Schema: &twoSchema{},
		Handler: func(c Context) error {
			fmt.Println("Handler 2 was called")
			return nil
		},
	})

	svr := NewTestRouter([]TestRoute{
		{
			Path:    "/route/one/1",
			Method:  http.MethodGet,
			Options: &h1,
		},
		{
			Path:    "/route/two/1",
			Method:  http.MethodPost,
			Options: &h2,
		},
	})

	resp, err := svr.Invoke(InvokeOptions{
		Path:   "/route/one/1",
		Method: http.MethodGet,
		Headers: map[string]string{
			"one": "1",
			"two": "2",
		},
	})

	fmt.Println(">>> server", resp, err)
}
