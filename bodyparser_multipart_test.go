package gofi

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestMultipartBodyParser(t *testing.T) {
	parser := &MultipartBodyParser{}

	t.Run("Match", func(t *testing.T) {
		if !parser.Match("multipart/form-data") {
			t.Error("expected match for multipart/form-data")
		}
	})

	t.Run("Decode File and Values", func(t *testing.T) {
		type Data struct {
			Name string                `json:"name"`
			File *multipart.FileHeader `json:"file"`
		}

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		writer.WriteField("name", "gofi")
		part, _ := writer.CreateFormFile("file", "test.txt")
		part.Write([]byte("hello world"))
		writer.Close()

		req := httptest.NewRequest("POST", "/", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		var data Data
		dataVal := reflect.ValueOf(&data).Elem()

		ruleDef := &RuleDef{
			kind: reflect.Struct,
			properties: map[string]*RuleDef{
				"name": {kind: reflect.String, fieldName: "Name"},
				"file": {kind: reflect.Ptr, fieldName: "File"},
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

		if data.Name != "gofi" {
			t.Errorf("expected Name 'gofi', got '%s'", data.Name)
		}
		if data.File == nil {
			t.Fatal("expected file to be present")
		}

		f, _ := data.File.Open()
		content, _ := io.ReadAll(f)
		if string(content) != "hello world" {
			t.Errorf("expected file content 'hello world', got '%s'", string(content))
		}
	})

	t.Run("Decode Array of Files", func(t *testing.T) {
		type Data struct {
			Files []*multipart.FileHeader `json:"files"`
		}

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		part1, _ := writer.CreateFormFile("files", "test1.txt")
		part1.Write([]byte("content1"))

		part2, _ := writer.CreateFormFile("files", "test2.txt")
		part2.Write([]byte("content2"))

		writer.Close()

		req := httptest.NewRequest("POST", "/", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		var data Data
		dataVal := reflect.ValueOf(&data).Elem()

		ruleDef := &RuleDef{
			kind: reflect.Struct,
			properties: map[string]*RuleDef{
				"files": {
					kind:      reflect.Slice,
					fieldName: "Files",
					item:      &RuleDef{kind: reflect.Ptr}, // *FileHeader
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

		if len(data.Files) != 2 {
			t.Errorf("expected 2 files, got %d", len(data.Files))
		}
	})
}
