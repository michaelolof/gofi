package gofi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkTestSchemaValidation(b *testing.B) {
	b.StopTimer()

	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				Passed  bool     `json:"passed" validate:"required"`
				Stage   string   `json:"stage"  validate:"oneof=actor massive blast"`
				Shifts  []string `json:"shifts" validate:"required"`
				Contact struct {
					FirstName string `json:"firstname"`
					LastName  string `json:"lastname"`
					Age       int    `json:"age"`
				} `json:"contact"`
				Users []struct {
					Id        int    `json:"int"`
					FirstName string `json:"firstname"`
					LastName  string `json:"lastname"`
				} `json:"users"`
			}
		}
	}

	schema := &testSchema{}
	method := "GET"
	url := "/api/test"
	rs := compileSchema(schema, Info{})
	rs.specs.normalize(method, url)

	b.StartTimer()
	body := strings.NewReader(strings.TrimSpace(`{
		"passed": true,
		"stage": "actor",
		"shifts": ["one", "two", "three", "four"],
		"contact": {
			"firstname": "Judie",
			"lastname": "Akington",
			"age": 28
		},
		"users": [
			{ "id": 201, "firstname": "Joxier", "lastname": "Bennet" },
			{ "id": 202, "firstname": "Colex", "lastname": "Dinnor" },
			{ "id": 203, "firstname": "Pusher", "lastname": "Maxker" }
		]
	}`))
	r, _ := http.NewRequest(method, url, body)
	c := NewContext(httptest.NewRecorder(), r)

	c.setSchemaRules(&rs.rules)
	err := Validate(c)
	assert.Nil(b, err)
	b.StopTimer()
}

func BenchmarkTestSchemaValidationAndBind(b *testing.B) {
	b.StopTimer()

	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				Passed  bool     `json:"passed" validate:"required"`
				Stage   string   `json:"stage"  validate:"oneof=actor massive blast"`
				Shifts  []string `json:"shifts" validate:"required"`
				Contact struct {
					FirstName string `json:"firstname"`
					LastName  string `json:"lastname"`
					Age       int    `json:"age"`
				} `json:"contact"`
				Users []struct {
					Id        int    `json:"int"`
					FirstName string `json:"firstname"`
					LastName  string `json:"lastname"`
				} `json:"users"`
			}
		}
	}

	schema := &testSchema{}
	method := "GET"
	url := "/api/test"
	rs := compileSchema(schema, Info{})
	rs.specs.normalize(method, url)

	b.StartTimer()
	body := strings.NewReader(strings.TrimSpace(`{
		"passed": true,
		"stage": "actor",
		"shifts": ["one", "two", "three", "four"],
		"contact": {
			"firstname": "Judie",
			"lastname": "Akington",
			"age": 28
		},
		"users": [
			{ "id": 201, "firstname": "Joxier", "lastname": "Bennet" },
			{ "id": 202, "firstname": "Colex", "lastname": "Dinnor" },
			{ "id": 203, "firstname": "Pusher", "lastname": "Maxker" }
		]
	}`))
	r, _ := http.NewRequest(method, url, body)
	c := NewContext(httptest.NewRecorder(), r)

	c.setSchemaRules(&rs.rules)
	_, err := ValidateAndBind[testSchema](c)
	assert.Nil(b, err)
	b.StopTimer()
}
