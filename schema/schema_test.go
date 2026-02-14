package schema_test

import (
	"testing"

	"github.com/michaelolof/gofi/schema"
)

type TestBody struct {
	Name string `json:"name" validate:"required"`
	Age  int    `json:"age" validate:"min=18"`
}

type TestRequest struct {
	Body TestBody `schema:"Body"`
}

type TestResponse struct {
	Body TestBody `schema:"Body"`
}

func TestCompile(t *testing.T) {
	req := TestRequest{}

	s, err := schema.Compile(req, schema.Info{
		Method: "POST",
		Url:    "/test",
	}, nil)

	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if !s.HasBody {
		t.Error("Expected HasBody to be true")
	}

	rule := s.Rules.GetReqRules("Body")
	if rule == nil {
		t.Error("Expected Body rules")
	}
}

func TestValidation(t *testing.T) {
	// This requires setting up a request with body.
	// skipped for compilation check primarily
}

func TestTypes(t *testing.T) {
	// Check if types are accessible
	_ = schema.ValidationOptions{}
	_ = schema.RuleDef{}
	_ = schema.SchemaRules{}
	_ = schema.JSONSchemaEncoder{}
}
