package gofi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

type mockParserContext struct {
	req *http.Request
	w   http.ResponseWriter
}

func (m *mockParserContext) Writer() http.ResponseWriter {
	return m.w
}

func (m *mockParserContext) Request() *http.Request {
	return m.req
}

func (m *mockParserContext) CustomSpecs() CustomSpecs {
	return nil
}

func TestFormBodyParser(t *testing.T) {
	parser := &FormBodyParser{}

	t.Run("Match", func(t *testing.T) {
		if !parser.Match("application/x-www-form-urlencoded") {
			t.Error("expected match for application/x-www-form-urlencoded")
		}
		if parser.Match("application/json") {
			t.Error("expected no match for application/json")
		}
	})

	t.Run("Decode Simple Struct", func(t *testing.T) {
		type User struct {
			Name string `json:"name" validate:"required"`
			Age  int    `json:"age"`
		}

		formData := url.Values{}
		formData.Set("name", "John")
		formData.Set("age", "30")

		body := bytes.NewBufferString(formData.Encode())
		req := httptest.NewRequest("POST", "/", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		var user User
		userVal := reflect.ValueOf(&user).Elem()

		// Manually constructing simplified rules for test
		ruleDef := &RuleDef{
			kind: reflect.Struct,
			properties: map[string]*RuleDef{
				"name": {kind: reflect.String, fieldName: "Name", required: true},
				"age":  {kind: reflect.Int, fieldName: "Age"},
			},
		}

		err := parser.ValidateAndDecodeRequest(io.NopCloser(body), RequestOptions{
			Context:     &mockParserContext{req: req, w: httptest.NewRecorder()},
			ShouldBind:  true,
			Body:        &userVal,
			SchemaRules: ruleDef,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if user.Name != "John" {
			t.Errorf("expected Name 'John', got '%s'", user.Name)
		}
		if user.Age != 30 {
			t.Errorf("expected Age 30, got %d", user.Age)
		}
	})

	t.Run("Decode Array", func(t *testing.T) {
		type Data struct {
			Tags []string `json:"tags"`
		}

		formData := url.Values{}
		formData.Add("tags", "go")
		formData.Add("tags", "rust")

		body := bytes.NewBufferString(formData.Encode())
		req := httptest.NewRequest("POST", "/", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		var data Data
		dataVal := reflect.ValueOf(&data).Elem()

		ruleDef := &RuleDef{
			kind: reflect.Struct,
			properties: map[string]*RuleDef{
				"tags": {
					kind:      reflect.Slice,
					fieldName: "Tags",
					item:      &RuleDef{kind: reflect.String},
				},
			},
		}

		err := parser.ValidateAndDecodeRequest(io.NopCloser(body), RequestOptions{
			Context:     &mockParserContext{req: req, w: httptest.NewRecorder()},
			ShouldBind:  true,
			Body:        &dataVal,
			SchemaRules: ruleDef,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(data.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(data.Tags))
		}
		if data.Tags[0] != "go" || data.Tags[1] != "rust" {
			t.Errorf("unexpected tags values: %v", data.Tags)
		}
	})
}
