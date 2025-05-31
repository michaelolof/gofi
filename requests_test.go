package gofi

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/michaelolof/gofi/utils"
	"github.com/stretchr/testify/assert"
)

func TestCookieRequest(t *testing.T) {

	type invType struct {
		Name string
	}

	type testSchema struct {
		Request struct {
			Cookie struct {
				One     *string      `json:"one" validate:"required"`
				Two     int          `json:"two" validate:"required"`
				Three   http.Cookie  `json:"three" validate:"required"`
				Four    *http.Cookie `json:"four" validate:"required"`
				Invalid invType      `json:"invalid" validate:"required"`
			}
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)
			if err != nil {
				fmt.Println(err)
				return err
			}

			fmt.Println(s)
			return nil
		},
	}

	m := newServeMux()
	m.Inject(InjectOptions{
		Path: "/test",
		Cookies: []http.Cookie{
			{Name: "one", Value: "john"},
			{Name: "two", Value: "2"},
			{Name: "three", Value: "three stooges"},
			{Name: "four", Value: "four dogs"},
			{Name: "invalid", Value: "invalid cookie"},
		},
		Method:  "POST",
		Handler: &handler,
	})
}

func TestJSONEncoder_NoRequesBody(t *testing.T) {

	type testSchema struct {
		Request struct {
			Path struct {
				Id int `json:"id" validate:"required"`
			}
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)
			if err != nil {
				return err
			}

			fmt.Println(s)
			return nil
		},
	}

	m := newServeMux()
	m.Inject(InjectOptions{
		Path: "/test/:id",
		Paths: map[string]string{
			"id": "1",
		},
		Method:  "POST",
		Handler: &handler,
	})
}

func TestJSONEncoder_EmptyRequestBody(t *testing.T) {

	type testSchema struct {
		Request struct {
			Body string `validate:"required"`
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)

			assert.NotNil(t, err, "validation error should not be nil")
			assert.Nil(t, s, "schema pointer object should be nil")
			return nil
		},
	}

	m := NewServeMux()
	m.Inject(InjectOptions{
		Path:    "/test/one",
		Method:  "POST",
		Body:    bytes.NewReader([]byte{}),
		Handler: &handler,
	})

}

func TestJSONEncoder_PrimitiveRequestBody(t *testing.T) {

	type testSchema struct {
		Request struct {
			Body int8 `validate:"required,max=35"`
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)

			assert.Nil(t, err, "validation error should be nil")
			assert.NotNil(t, s, "schema pointer object should not be nil")
			assert.Equal(t, s.Request.Body, int8(30))
			return nil
		},
	}

	m := NewServeMux()
	m.Inject(InjectOptions{
		Path:    "/test/one",
		Method:  "POST",
		Body:    strings.NewReader("30"),
		Handler: &handler,
	})

}

func TestJSONEncoder_SimpleStructRequestBody(t *testing.T) {

	type testSchema struct {
		Request struct {
			Body struct {
				Fullname string   `json:"fullname" validate:"required"`
				Age      int      `json:"age" validate:"required"`
				Amount   *float32 `json:"amount" validate:"required"`
			} `validate:"required"`
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)

			assert.Nil(t, err, "validation error should be nil")
			assert.NotNil(t, s, "schema pointer object should not be nil")
			assert.Equal(t, s.Request.Body.Fullname, "John Doe")
			assert.Equal(t, s.Request.Body.Age, 25)
			assert.Equal(t, *s.Request.Body.Amount, float32(34.20))
			return nil
		},
	}

	m := NewServeMux()
	m.Inject(InjectOptions{
		Path:   "/test/one",
		Method: "POST",
		Body: utils.TryAsReader(map[string]any{
			"fullname": "John Doe",
			"age":      25,
			"amount":   34.20,
		}),
		Handler: &handler,
	})

}

func TestJSONEncode_PrimitiveArrayRequestBody(t *testing.T) {
	type testSchema struct {
		Request struct {
			Body []int `validate:"required"`
		}
	}

	list := []int{1, 2, 3, 4, 5}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)

			assert.Nil(t, err, "validation error should be nil")
			assert.NotNil(t, s, "schema pointer object should not be nil")
			assert.Equal(t, s.Request.Body, list)
			return nil
		},
	}

	m := NewServeMux()
	m.Inject(InjectOptions{
		Path:    "/test/one",
		Method:  "POST",
		Body:    utils.TryAsReader(list),
		Handler: &handler,
	})
}

func TestJSONEncode_PrimitiveArrayTypesBody(t *testing.T) {
	type testSchema struct {
		Request struct {
			Body struct {
				One   []int      `json:"one" validate:"required"`
				Two   *[]string  `json:"two" validate:"required"`
				Three []*float32 `json:"three" validate:"required"`
			} `validate:"required"`
		}
	}

	listOne := []int{1, 2, 3, 4, 5}
	listTwo := []string{"one", "two", "three"}
	listThree := []float32{1, 2, 3}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)

			assert.Nil(t, err, "validation error should be nil")
			assert.NotNil(t, s, "schema pointer object should not be nil")
			assert.Equal(t, s.Request.Body.One, listOne)
			assert.Equal(t, *s.Request.Body.Two, listTwo)
			assert.Equal(t, *s.Request.Body.Three[0], listThree[0])
			assert.Equal(t, *s.Request.Body.Three[1], listThree[1])
			assert.Equal(t, *s.Request.Body.Three[2], listThree[2])
			return nil
		},
	}

	m := NewServeMux()
	m.Inject(InjectOptions{
		Path:   "/test/one",
		Method: "POST",
		Body: utils.TryAsReader(map[string]any{
			"one":   listOne,
			"two":   listTwo,
			"three": listThree,
		}),
		Handler: &handler,
	})
}

func TestJSONEncode_StructArrayBody(t *testing.T) {

	type testBody struct {
		One   []int            `json:"one" validate:"required"`
		Two   string           `json:"two" validate:"required"`
		Three []map[string]int `json:"three" validate:"required"`
		Four  [][]int          `json:"four" validate:"required"`
	}

	type testSchema struct {
		Request struct {
			Body []testBody `validate:"required"`
		}
	}

	list := []testBody{
		{One: []int{1, 2, 3}, Two: "justin", Three: []map[string]int{{"one": 1}, {"two": 2}}, Four: [][]int{{1, 2, 3}, {4, 5, 6}}},
		{One: []int{6, 7, 8}, Two: "maxwell", Three: []map[string]int{{"three": 3}, {"four": 4}}, Four: [][]int{{4, 5, 6}, {1, 2, 3}}},
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)

			assert.Nil(t, err, "validation error should be nil")
			assert.NotNil(t, s, "schema pointer object should not be nil")
			assert.Equal(t, s.Request.Body[0], list[0])
			assert.Equal(t, s.Request.Body[1], list[1])
			return nil
		},
	}

	m := NewServeMux()
	m.Inject(InjectOptions{
		Path:    "/test/one",
		Method:  "POST",
		Body:    utils.TryAsReader(list),
		Handler: &handler,
	})
}

func TestIgnoredJSONField(t *testing.T) {
	type testBody struct {
		One int    `json:"-"`
		Two string `json:"two" validate:"required"`
	}

	type testSchema struct {
		Request struct {
			Body testBody `validate:"required"`
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)

			assert.Nil(t, err, "validation error should be nil")
			assert.NotNil(t, s, "schema pointer object should not be nil")
			// assert.Equal(t, s.Request.Body[0], list[0])
			// assert.Equal(t, s.Request.Body[1], list[1])
			return nil
		},
	}

	m := NewServeMux()
	m.Inject(InjectOptions{
		Path:   "/test/one",
		Method: "POST",
		Body: utils.TryAsReader(map[string]any{
			"-":   30,
			"two": "maxwie",
		}),
		Handler: &handler,
	})
}

func TestEncode_AnyValue(t *testing.T) {
	type testSchema struct {
		Request struct {
			Body any `validate:"required"`
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)

			assert.Nil(t, err, "validation error should be nil")
			assert.NotNil(t, s, "schema pointer object should not be nil")
			var vany any = 20
			assert.Equal(t, s.Request.Body, vany)
			return nil
		},
	}

	m := NewServeMux()
	m.Inject(InjectOptions{
		Path:    "/test/one",
		Method:  "POST",
		Body:    utils.TryAsReader(20),
		Handler: &handler,
	})
}

func TestJSONEncode_AnyValue(t *testing.T) {
	type testSchema struct {
		Request struct {
			Body struct {
				Field any `json:"field" validate:"required"`
			}
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c)

			assert.Nil(t, err, "validation error should be nil")
			assert.NotNil(t, s, "schema pointer object should not be nil")
			var vany any = "Starter"
			assert.Equal(t, s.Request.Body.Field, vany)
			return nil
		},
	}

	m := NewServeMux()
	m.Inject(InjectOptions{
		Path:    "/test/one",
		Method:  "POST",
		Body:    utils.TryAsReader(map[string]any{"field": "Starter"}),
		Handler: &handler,
	})
}
