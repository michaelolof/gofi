package gofi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/michaelolof/gofi/cont"

	"github.com/stretchr/testify/assert"
)

func TestRequestBindQuery(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Query struct {
				Email    string `json:"email"`
				UserId   int    `json:"user_id" validate:"required"`
				IsPassed bool   `json:"is_passed"`
			}
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=2&is_passed=1",
			check: func(s *testSchema, e error) {
				assert.Nil(t, e)
				assert.Equal(t, s.Request.Query.Email, "one@mail.com")
				assert.Equal(t, s.Request.Query.UserId, 2)
				assert.True(t, s.Request.Query.IsPassed)
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=29",
			check: func(s *testSchema, err error) {
				fmt.Println(err)
				assert.Nil(t, err)
				assert.Equal(t, s.Request.Query.Email, "one@mail.com")
				assert.Equal(t, s.Request.Query.UserId, 29)
				assert.False(t, s.Request.Query.IsPassed)
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindPath(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Header struct {
				ContentType cont.ContentType `json:"Content-Type" default:"application/json"`
				Baser       int              `json:"Baser" default:"10"`
			}

			Path struct {
				Id     int    `json:"id" validate:"required"`
				Handle string `json:"handle" validate:"required"`
			}
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/{id}/some/{handle}",
			paths: map[string]string{
				"id":     "12345677",
				"handle": "xe1fjicdi_",
			},
			check: func(s *testSchema, err error) {
				fmt.Println(err)
				assert.Nil(t, err)
				assert.Equal(t, s.Request.Header.ContentType, cont.ApplicationJson)
				assert.Equal(t, s.Request.Header.Baser, 10)
				assert.Equal(t, s.Request.Path.Id, 12345677)
				assert.Equal(t, s.Request.Path.Handle, "xe1fjicdi_")
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindBody(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				Status  string `json:"status" validate:"required"`
				Message string `json:"message" validate:"required"`
				Stats   []int  `json:"stats" validate:"required"`
				Data    struct {
					Firstname string `json:"firstname" validate:"required"`
					Lastname  string `json:"lastname" validate:"required"`
					Age       int    `json:"age"`
				} `json:"data"`
			} `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body: strings.NewReader(strings.TrimSpace(`{
				"status": "success",
				"message": "Action completed successfully",
				"stats": [1, 2, 3, 4, 5, 6, 7, 8],
				"data": null
			}`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Equal(t, ts.Request.Body.Status, "success")
				assert.Equal(t, ts.Request.Body.Message, "Action completed successfully")
				assert.Len(t, ts.Request.Body.Stats, 8)
				assert.Equal(t, ts.Request.Body.Stats, []int{1, 2, 3, 4, 5, 6, 7, 8})
				assert.Equal(t, ts.Request.Body.Data.Firstname, "")
				assert.Equal(t, ts.Request.Body.Data.Lastname, "")
				assert.Equal(t, ts.Request.Body.Data.Age, 0)
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindBodyEmptyPointer(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				Status  string `json:"status" validate:"required"`
				Message string `json:"message" validate:"required"`
				Data    *struct {
					Firstname string `json:"firstname" validate:"required"`
					Lastname  string `json:"lastname" validate:"required"`
					Age       int    `json:"age"`
				} `json:"data"`
				Stuff int `json:"stuff"`
			} `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body: strings.NewReader(strings.TrimSpace(`{
				"status": "success",
				"message": "Action completed successfully",
				"data": null,
				"stuff": 20
			}`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Equal(t, ts.Request.Body.Status, "success")
				assert.Equal(t, ts.Request.Body.Message, "Action completed successfully")
				assert.Nil(t, ts.Request.Body.Data)
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindBodyDefinedStruct(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				Status  string `json:"status" validate:"required"`
				Message string `json:"message" validate:"required"`
				Data    struct {
					Firstname string `json:"firstname" validate:"required"`
					Lastname  string `json:"lastname" validate:"required"`
					Age       int    `json:"age"`
				} `json:"data"`
			} `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body: strings.NewReader(strings.TrimSpace(`{
				"status": "success",
				"message": "Action completed successfully",
				"data": {
					"firstname": "John",
					"lastname": "Doew",
					"age": 14
				}
			}`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Equal(t, ts.Request.Body.Status, "success")
				assert.Equal(t, ts.Request.Body.Message, "Action completed successfully")
				assert.NotNil(t, ts.Request.Body.Data)
				assert.Equal(t, ts.Request.Body.Data.Firstname, "John")
				assert.Equal(t, ts.Request.Body.Data.Lastname, "Doew")
				assert.Equal(t, ts.Request.Body.Data.Age, 14)
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestCurrentCase(t *testing.T) {
	type testSchema struct {
		Request struct {
			Body struct {
				Data *struct {
					Firstname string `json:"firstname" validate:"required"`
					Lastname  string `json:"lastname" validate:"required"`
					Age       int    `json:"age"`
				} `json:"data"`
			} `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body: strings.NewReader(strings.TrimSpace(`{
				"data": {
					"firstname": "John",
					"lastname": "Doew",
					"age": 14
				}
			}`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.NotNil(t, ts.Request.Body.Data)
				assert.Equal(t, ts.Request.Body.Data.Firstname, "John")
				assert.Equal(t, ts.Request.Body.Data.Lastname, "Doew")
				assert.Equal(t, ts.Request.Body.Data.Age, 14)
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindBodyDefinedPointer(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				Status  string `json:"status" validate:"required"`
				Message string `json:"message" validate:"required"`
				Data    *struct {
					Firstname string `json:"firstname" validate:"required"`
					Lastname  string `json:"lastname" validate:"required"`
					Age       int    `json:"age"`
				} `json:"data"`
			} `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body: strings.NewReader(strings.TrimSpace(`{
				"status": "success",
				"message": "Action completed successfully",
				"data": {
					"firstname": "John",
					"lastname": "Doew",
					"age": 14
				}
			}`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Equal(t, ts.Request.Body.Status, "success")
				assert.Equal(t, ts.Request.Body.Message, "Action completed successfully")
				assert.NotNil(t, ts.Request.Body.Data)
				assert.Equal(t, ts.Request.Body.Data.Firstname, "John")
				assert.Equal(t, ts.Request.Body.Data.Lastname, "Doew")
				assert.Equal(t, ts.Request.Body.Data.Age, 14)
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindingBodyArrayStruct(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				Status   string `json:"status" validate:"required"`
				Message  string `json:"message" validate:"required"`
				Contacts []struct {
					Firstname string `json:"firstname" validate:"required"`
					Lastname  string `json:"lastname" validate:"required"`
					Age       int    `json:"age"`
				} `json:"contacts" validate:"required"`
			} `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body: strings.NewReader(strings.TrimSpace(`{
				"status": "success",
				"message": "Action completed successfully",
				"contacts": [
					{ "firstname": "John", "lastname": "Doew", "age": 14 },
					{ "firstname": "Angie", "lastname": "Must", "age": 24 },
					{ "firstname": "Dominic", "lastname": "Carey", "age": 22 }
				]
			}`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Equal(t, ts.Request.Body.Status, "success")
				assert.Equal(t, ts.Request.Body.Message, "Action completed successfully")
				assert.Len(t, ts.Request.Body.Contacts, 3)
				assert.Equal(t, ts.Request.Body.Contacts[0].Firstname, "John")
				assert.Equal(t, ts.Request.Body.Contacts[0].Lastname, "Doew")
				assert.Equal(t, ts.Request.Body.Contacts[0].Age, 14)
				assert.Equal(t, ts.Request.Body.Contacts[1].Firstname, "Angie")
				assert.Equal(t, ts.Request.Body.Contacts[1].Lastname, "Must")
				assert.Equal(t, ts.Request.Body.Contacts[1].Age, 24)
				assert.Equal(t, ts.Request.Body.Contacts[2].Firstname, "Dominic")
				assert.Equal(t, ts.Request.Body.Contacts[2].Lastname, "Carey")
				assert.Equal(t, ts.Request.Body.Contacts[2].Age, 22)
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindingBodyArrayPointer(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				Status   string `json:"status" validate:"required"`
				Message  string `json:"message" validate:"required"`
				Contacts []*struct {
					Firstname string `json:"firstname" validate:"required"`
					Lastname  string `json:"lastname" validate:"required"`
					Age       int    `json:"age"`
				} `json:"contacts" validate:"required"`
			} `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body: strings.NewReader(strings.TrimSpace(`{
				"status": "success",
				"message": "Action completed successfully",
				"contacts": [
					{ "firstname": "John", "lastname": "Doew", "age": 14 },
					{ "firstname": "Angie", "lastname": "Must", "age": 24 },
					{ "firstname": "Dominic", "lastname": "Carey", "age": 22 }
				]
			}`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Equal(t, ts.Request.Body.Status, "success")
				assert.Equal(t, ts.Request.Body.Message, "Action completed successfully")
				assert.Len(t, ts.Request.Body.Contacts, 3)
				assert.Equal(t, ts.Request.Body.Contacts[0].Firstname, "John")
				assert.Equal(t, ts.Request.Body.Contacts[0].Lastname, "Doew")
				assert.Equal(t, ts.Request.Body.Contacts[0].Age, 14)
				assert.Equal(t, ts.Request.Body.Contacts[1].Firstname, "Angie")
				assert.Equal(t, ts.Request.Body.Contacts[1].Lastname, "Must")
				assert.Equal(t, ts.Request.Body.Contacts[1].Age, 24)
				assert.Equal(t, ts.Request.Body.Contacts[2].Firstname, "Dominic")
				assert.Equal(t, ts.Request.Body.Contacts[2].Lastname, "Carey")
				assert.Equal(t, ts.Request.Body.Contacts[2].Age, 22)
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindBodyArray(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body []struct {
				Status  string `json:"status" validate:"required"`
				Message string `json:"message" validate:"required"`
				Data    *struct {
					Firstname string `json:"firstname" validate:"required"`
					Lastname  string `json:"lastname" validate:"required"`
					Age       int    `json:"age"`
				} `json:"data"`
			} `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body: strings.NewReader(strings.TrimSpace(`[
				{
					"status": "success",
					"message": "Action completed successfully",
					"data": null
				},
				{
					"status": "ok",
					"message": "Finished task successfully",
					"data": {
						"firstname": "Wonderment",
						"lastname": "Agripunda",
						"age": 20
					}
				}
			]`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Len(t, ts.Request.Body, 2)
				assert.Equal(t, ts.Request.Body[0].Status, "success")
				assert.Equal(t, ts.Request.Body[0].Message, "Action completed successfully")
				assert.Nil(t, ts.Request.Body[0].Data)
				assert.Equal(t, ts.Request.Body[1].Status, "ok")
				assert.Equal(t, ts.Request.Body[1].Message, "Finished task successfully")
				assert.Equal(t, ts.Request.Body[1].Data.Firstname, "Wonderment")
				assert.Equal(t, ts.Request.Body[1].Data.Lastname, "Agripunda")
				assert.Equal(t, ts.Request.Body[1].Data.Age, 20)
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindBodyPrimitive(t *testing.T) {
	type testSchema struct {
		Request struct {
			Body string `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body:   strings.NewReader(strings.TrimSpace(`I see, I saw`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Equal(t, ts.Request.Body, "I see, I saw")
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindBodyPrimitivePointer(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body *int `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body:   strings.NewReader(strings.TrimSpace(`2000`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Equal(t, *ts.Request.Body, 2000)
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindBodyPrimitiveList(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body []int `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body:   strings.NewReader(strings.TrimSpace(`[1, 2, 3, 4, 5]`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Equal(t, ts.Request.Body, []int{1, 2, 3, 4, 5})
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindBodyPrimitivePointerList(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body *[]int `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body:   strings.NewReader(strings.TrimSpace(`[1, 2, 3, 4, 5]`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Equal(t, *ts.Request.Body, []int{1, 2, 3, 4, 5})
			},
		},
	}

	runRequestBinders(cases)
}

func TestRequestBindBodyPrimitiveListPointer(t *testing.T) {
	type testSchema struct {
		Schema

		Request struct {
			Body []*int `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []binderTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/1",
			method: "POST",
			body:   strings.NewReader(strings.TrimSpace(`[1, 2, 3, 4, 5]`)),
			check: func(ts *testSchema, err error) {
				assert.Nil(t, err)
				assert.Len(t, ts.Request.Body, 5)
				assert.Equal(t, *ts.Request.Body[0], 1)
				assert.Equal(t, *ts.Request.Body[1], 2)
				assert.Equal(t, *ts.Request.Body[2], 3)
				assert.Equal(t, *ts.Request.Body[3], 4)
				assert.Equal(t, *ts.Request.Body[4], 5)
			},
		},
	}

	runRequestBinders(cases)
}

func runRequestBinders[T ISchema](cases []binderTester[T]) {

	run := func(cs binderTester[T]) {
		method := "GET"
		if cs.method != "" {
			method = cs.method
		}

		r, _ := http.NewRequest(method, cs.url, cs.body)
		if len(cs.paths) > 0 {
			for name, value := range cs.paths {
				r.SetPathValue(name, value)
			}
		}

		c := NewContext(httptest.NewRecorder(), r)
		cs.schema.specs.normalize(method, cs.url)
		c.setSchemaRules(&cs.schema.rules)

		s, err := ValidateAndBind[T](c)
		if cs.check != nil {
			cs.check(s, err)
		}
	}

	var onlyCs *binderTester[T] = nil
	for _, cs := range cases {
		if cs.only {
			onlyCs = &cs
			break
		}
	}

	if onlyCs != nil {
		run(*onlyCs)
		return
	}

	for _, cs := range cases {
		if cs.ignore {
			continue
		}

		run(cs)
	}
}

type binderTester[T ISchema] struct {
	ignore bool
	only   bool
	url    string
	method string
	paths  map[string]string
	schema *routeSchema
	body   io.Reader
	check  func(*T, error)
}
