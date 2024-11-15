package gofi

import (
	"encoding/json"
	"testing"

	"github.com/michaelolof/gofi/cont"

	"github.com/stretchr/testify/assert"
)

func TestMuxSchema(t *testing.T) {

	type testSchemaOne struct {
		Schema

		Req struct {
			Path struct {
				Id int `json:"id" validate:"required"`
			}

			Query struct {
				Name string `json:"name"`
			}
		}
	}

	type testSchemaTwo struct {
		Schema

		Req struct {
			Headers struct {
				ContentType cont.ContentType `json:"Content-Type" default:"text/html"`
			}

			Body struct {
				Email    string `json:"email" example:"johndoe@mail.com"`
				Age      int    `json:"age" default:"22"`
				Password string `json:"password" example:"xxxxxxx"`
			} `validate:"required" description:"This is something i would like to say about the struct i'm using"`
		}

		Resp struct {
			Success struct {
			}
		}
	}

	mux := NewServeMux()

	mux.Get("/path/one/{id}/scan-live-games/{age}/bus_it/{file}", HandlerOptions{
		Schema: &testSchemaOne{},
		Handler: func(c Context) error {
			return nil
		},
	})

	mux.Post("/path/two", HandlerOptions{

		Info: Info{
			OperationId: "goingAgainstSomething",
		},

		Schema: &testSchemaTwo{},

		Handler: func(c Context) error {
			return nil
		},
	})

	mux.Delete("/path/two", HandlerOptions{
		Schema: &testSchemaTwo{},
		Handler: func(c Context) error {
			return nil
		},
	})

	dopts := DocsOptions{
		Ui: DocsUiOptions{
			RoutePrefix: "/api-docs/test-service",
		},
	}

	dopts.getDocs(mux)
}

func TestMuxSchema2(t *testing.T) {

	type apiSuccess[T any] struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Data    T      `json:"data"`
	}

	type testSchema struct {
		Schema

		Success struct {
			Body apiSuccess[bool]
		}
	}

	mux := NewServeMux()

	controller := DefineHandler(HandlerOptions{
		Schema: &testSchema{},

		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)
			if err != nil {
				return err
			}

			s.Success.Body = apiSuccess[bool]{
				Status:  "ok",
				Message: "Message sent successfully",
				Data:    true,
			}

			return c.JSON(200, s.Success)
		},
	})

	mux.Get("/api/ping", controller)

	reply, err := mux.Inject(InjectOptions{
		Path:    "/api/ping",
		Method:  "GET",
		Handler: &controller,
	})

	assert.Nil(t, err)
	assert.Equal(t, reply.Code, 200)

	var resp apiSuccess[bool]
	err = json.NewDecoder(reply.Body).Decode(&resp)
	assert.Nil(t, err)
	assert.Equal(t, resp.Status, "ok")
	assert.Equal(t, resp.Message, "Message sent successfully")
	assert.Equal(t, resp.Data, true)
}
