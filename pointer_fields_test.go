package gofi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/michaelolof/gofi/validators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pointerColor string

// roomRule accepts strings that start with "room-".
type roomRule struct{}

func (roomRule) Name() string { return "room" }

func (roomRule) Rule(c ValidatorContext) func(val any) error {
	return func(val any) error {
		if s, ok := val.(string); ok && strings.HasPrefix(s, "room-") {
			return nil
		}
		return errors.New("value must start with room-")
	}
}

type pointerBodySchema struct {
	Request struct {
		Body struct {
			Choice  *string       `json:"choice" validate:"oneof=a b"`
			Color   *pointerColor `json:"color" validate:"oneof=red blue"`
			Age     *int          `json:"age" validate:"gte=18,lte=120"`
			Site    *string       `json:"site" validate:"url"`
			Email   *string       `json:"email" validate:"email"`
			Name    *string       `json:"name" validate:"min=2,max=5"`
			Tags    *[]string     `json:"tags" validate:"max=2"`
			Score   *float64      `json:"score" validate:"gt=0"`
			Contact *string       `json:"contact" validate:"omitempty,email"`
			Room    *string       `json:"room" validate:"room"`
		}
	}
}

func newPointerBodyRouter(t *testing.T) Router {
	t.Helper()
	r := NewRouter()
	r.RegisterValidator(roomRule{})
	r.Post("/p", RouteOptions{
		Schema: &pointerBodySchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[pointerBodySchema](c)
			if err != nil {
				return err
			}
			b := s.Request.Body
			return c.SendString(200, fmt.Sprintf("choice=%s color=%s age=%s tags=%s",
				deref(b.Choice), deref(b.Color), deref(b.Age), deref(b.Tags)))
		},
	})
	return r
}

func deref[T any](p *T) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprint(*p)
}

func TestPointerFields_JSONBodyRules(t *testing.T) {
	r := newPointerBodyRouter(t)
	cases := []struct {
		body string
		want int
	}{
		{`{}`, 200},
		{`{"choice":null,"age":null,"tags":null}`, 200},
		{`{"choice":"a"}`, 200},
		{`{"choice":"z"}`, 400},
		{`{"color":"red"}`, 200},
		{`{"color":"green"}`, 400},
		{`{"age":30}`, 200},
		{`{"age":3}`, 400},
		{`{"age":121}`, 400},
		{`{"site":"https://example.com"}`, 200},
		{`{"site":"nope"}`, 400},
		{`{"email":"a@b.co"}`, 200},
		{`{"email":"nope"}`, 400},
		{`{"name":"abc"}`, 200},
		{`{"name":"a"}`, 400},
		{`{"tags":["a","b"]}`, 200},
		{`{"tags":["a","b","c"]}`, 400},
		{`{"score":1.5}`, 200},
		{`{"score":0}`, 400},
		{`{"contact":""}`, 200},
		{`{"contact":"nope"}`, 400},
		{`{"room":"room-1"}`, 200},
		{`{"room":"lobby"}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			resp, err := r.Test(TestOptions{Method: "POST", Path: "/p", Body: strings.NewReader(tc.body)})
			require.NoError(t, err)
			assert.Equal(t, tc.want, resp.Code(), resp.BodyString())
		})
	}
}

func TestPointerFields_JSONBodyBinds(t *testing.T) {
	r := newPointerBodyRouter(t)

	resp, err := r.Test(TestOptions{Method: "POST", Path: "/p", Body: strings.NewReader(`{"choice":"b","color":"blue","age":40,"tags":["x"]}`)})
	require.NoError(t, err)
	require.Equal(t, 200, resp.Code(), resp.BodyString())
	assert.Equal(t, "choice=b color=blue age=40 tags=[x]", resp.BodyString())

	resp, err = r.Test(TestOptions{Method: "POST", Path: "/p", Body: strings.NewReader(`{}`)})
	require.NoError(t, err)
	assert.Equal(t, "choice=<nil> color=<nil> age=<nil> tags=<nil>", resp.BodyString())
}

func TestPointerFields_RequiredAndPresent(t *testing.T) {
	type schema struct {
		Request struct {
			Body struct {
				Name  *string `json:"name" validate:"required"`
				Count *int    `json:"count" validate:"present"`
			}
		}
	}
	r := NewRouter()
	r.Post("/p", RouteOptions{Schema: &schema{}, Handler: func(c Context) error {
		if _, err := ValidateAndBind[schema](c); err != nil {
			return err
		}
		return c.SendString(200, "ok")
	}})
	cases := []struct {
		body string
		want int
	}{
		{`{"name":"x","count":0}`, 200},
		{`{"count":0}`, 400},
		{`{"name":null,"count":0}`, 400},
		{`{"name":"x"}`, 400},
		{`{"name":"x","count":null}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			resp, err := r.Test(TestOptions{Method: "POST", Path: "/p", Body: strings.NewReader(tc.body)})
			require.NoError(t, err)
			assert.Equal(t, tc.want, resp.Code(), resp.BodyString())
		})
	}
}

type pointerParamSchema struct {
	Request struct {
		Path struct {
			ID *int `json:"id" validate:"gte=1"`
		}
		Query struct {
			Q *int `json:"q" validate:"gte=18"`
		}
		Header struct {
			Mode *string `json:"x-mode" validate:"oneof=fast slow"`
		}
		Cookie struct {
			Limit *int `json:"limit" validate:"gte=1"`
		}
	}
}

func TestPointerFields_Parameters(t *testing.T) {
	r := NewRouter()
	r.Get("/items/:id", RouteOptions{
		Schema: &pointerParamSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[pointerParamSchema](c)
			if err != nil {
				return err
			}
			req := s.Request
			return c.SendString(200, fmt.Sprintf("id=%s q=%s mode=%s limit=%s",
				deref(req.Path.ID), deref(req.Query.Q), deref(req.Header.Mode), deref(req.Cookie.Limit)))
		},
	})

	get := func(path string, headers map[string]string, cookies ...http.Cookie) (int, string) {
		t.Helper()
		resp, err := r.Test(TestOptions{Method: "GET", Path: path, Headers: headers, Cookies: cookies})
		require.NoError(t, err)
		return resp.Code(), resp.BodyString()
	}

	code, body := get("/items/5?q=30", map[string]string{"x-mode": "fast"}, http.Cookie{Name: "limit", Value: "3"})
	require.Equal(t, 200, code, body)
	assert.Equal(t, "id=5 q=30 mode=fast limit=3", body)

	code, body = get("/items/5", nil)
	require.Equal(t, 200, code, body)
	assert.Equal(t, "id=5 q=<nil> mode=<nil> limit=<nil>", body)

	code, _ = get("/items/0", nil)
	assert.Equal(t, 400, code, "path rule")
	code, _ = get("/items/5?q=3", nil)
	assert.Equal(t, 400, code, "query rule")
	code, _ = get("/items/5", map[string]string{"x-mode": "turbo"})
	assert.Equal(t, 400, code, "header rule")
	code, _ = get("/items/5", nil, http.Cookie{Name: "limit", Value: "0"})
	assert.Equal(t, 400, code, "cookie rule")
}

func TestPointerFields_FormBody(t *testing.T) {
	type schema struct {
		Request struct {
			Header struct {
				ContentType string `json:"content-type" default:"application/x-www-form-urlencoded"`
			}
			Body struct {
				Age    *int    `json:"age" validate:"gte=18"`
				Choice *string `json:"choice" validate:"oneof=a b"`
			}
		}
	}
	r := NewRouter()
	r.Post("/f", RouteOptions{Schema: &schema{}, Handler: func(c Context) error {
		s, err := ValidateAndBind[schema](c)
		if err != nil {
			return err
		}
		return c.SendString(200, fmt.Sprintf("age=%s choice=%s", deref(s.Request.Body.Age), deref(s.Request.Body.Choice)))
	}})
	post := func(form string) (int, string) {
		t.Helper()
		resp, err := r.Test(TestOptions{
			Method:  "POST",
			Path:    "/f",
			Headers: map[string]string{"content-type": "application/x-www-form-urlencoded"},
			Body:    strings.NewReader(form),
		})
		require.NoError(t, err)
		return resp.Code(), resp.BodyString()
	}

	code, body := post("age=30&choice=a")
	require.Equal(t, 200, code, body)
	assert.Equal(t, "age=30 choice=a", body)

	code, _ = post("age=3")
	assert.Equal(t, 400, code)
	code, _ = post("choice=z")
	assert.Equal(t, 400, code)
}

func TestPointerFields_ResponseRules(t *testing.T) {
	type schema struct {
		Ok struct {
			Body struct {
				Choice *string `json:"choice" validate:"oneof=a b"`
				Age    *int    `json:"age" validate:"gte=18"`
			}
		}
	}
	send := func(choice string, age int) int {
		t.Helper()
		r := NewRouter()
		r.Get("/r", RouteOptions{Schema: &schema{}, Handler: func(c Context) error {
			var s schema
			s.Ok.Body.Choice = &choice
			s.Ok.Body.Age = &age
			return c.Send(200, s.Ok)
		}})
		resp, err := r.Test(TestOptions{Method: "GET", Path: "/r"})
		require.NoError(t, err)
		return resp.Code()
	}

	assert.Equal(t, 200, send("a", 30))
	assert.Equal(t, 500, send("z", 30), "response rule on *string")
	assert.Equal(t, 500, send("a", 3), "response rule on *int")
}

func TestPointerFields_StandaloneValidate(t *testing.T) {
	a, z, age := "a", "z", 30
	assert.NoError(t, validators.Validate(&a, "oneof=a b"))
	assert.Error(t, validators.Validate(&z, "oneof=a b"))
	assert.NoError(t, validators.Validate(&age, "gte=18"))
	assert.Error(t, validators.Validate((*string)(nil), "required"))
	assert.NoError(t, validators.Validate((*string)(nil), "email"))
}
