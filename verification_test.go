package gofi

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// Define a schema with a mismatched type for testing
type MismatchedTypeSchema struct {
	Request struct {
		Body struct {
			Age int `json:"age"` // Expects int, we will send string
		}
	}
}

// Test SafeConvert prevents panic on type mismatch
func TestSafeConvert_PreventsPanic(t *testing.T) {
	r := NewServeMux()
	handler := DefineHandler(RouteOptions{
		Schema: &MismatchedTypeSchema{},
		Handler: func(c Context) error {
			_, err := ValidateAndBind[MismatchedTypeSchema](c)
			return err
		},
	})
	r.Post("/test", handler) // Fixed: POST -> Post

	// Send a string "twelve" instead of an integer
	// This should not panic, but return an error (400 Bad Request usually, or internal error handled gracefully)
	w, err := r.Inject(InjectOptions{
		Method:  "POST",
		Path:    "/test",
		Body:    strings.NewReader(`{"age": "twelve"}`),
		Handler: &handler, // Fixed: Pass pointer to handler
	})

	if err != nil {
		t.Fatalf("Inject returned error: %v", err)
	}

	// We expect a non-200 code because validation/parsing should fail
	if w.Code == http.StatusOK {
		t.Errorf("Expected error status for mismatched type, got 200. Body: %s", w.Body.String())
	}
}

// Test Inject recovers from panic
func TestInject_RecoversFromPanic(t *testing.T) {
	r := NewServeMux()
	handler := DefineHandler(RouteOptions{
		Handler: func(c Context) error {
			panic("something went wrong")
		},
	})

	w, err := r.Inject(InjectOptions{
		Method:  "GET",
		Path:    "/panic",
		Handler: &handler, // Fixed: Pass pointer
	})

	// Inject should return an error if it recovers a panic
	if err == nil {
		t.Error("Expected error from panic recovery, got nil")
	}

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500 status code from panic, got %d", w.Code)
	}
}

// Test Recursion Limit
type RecursiveNode struct {
	Val  int            `json:"val"`
	Next *RecursiveNode `json:"next"`
}

type RecursiveSchema struct {
	Request struct {
		Body RecursiveNode
	}
}

func TestRecursionLimit(t *testing.T) {
	r := NewServeMux()

	// Register a body parser with a small recursion limit
	r.RegisterBodyParser(&JSONBodyParser{MaxDepth: 5})

	handler := DefineHandler(RouteOptions{
		Schema: &RecursiveSchema{},
		Handler: func(c Context) error {
			_, err := ValidateAndBind[RecursiveSchema](c)
			return err
		},
	})

	// Create a deeply nested JSON that exceeds 5 levels
	// {"next": {"next": {"next": {"next": {"next": {"next": {}}}}}}}
	deepJson := strings.Repeat(`{"next": `, 10) + "{}" + strings.Repeat("}", 10)

	w, err := r.Inject(InjectOptions{
		Method:  "POST",
		Path:    "/recursion",
		Body:    strings.NewReader(deepJson),
		Handler: &handler, // Fixed: Pass pointer
	})

	if err != nil {
		t.Fatalf("Inject returned error: %v", err)
	}

	// Should fail with recursion error
	if w.Code == http.StatusOK {
		t.Error("Expected recursion limit error, got 200 OK")
	}

	if !strings.Contains(w.Body.String(), "max recursion depth exceeded") {
		t.Errorf("Expected recursion error message, got: %s", w.Body.String())
	}
}

func TestSoftConversion(t *testing.T) {
	r := NewServeMux()
	handler := DefineHandler(RouteOptions{
		Schema: &MismatchedTypeSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[MismatchedTypeSchema](c)
			if err != nil {
				return err
			}
			if s.Request.Body.Age != 123 {
				return errors.New("value did not convert correctly")
			}
			return c.Send(200, nil)
		},
	})

	// Send a string "123" which should implicitly convert to int 123
	_, err := r.Inject(InjectOptions{ // Fixed: Ignored unused w
		Method:  "POST",
		Path:    "/soft-convert",
		Body:    strings.NewReader(`{"age": "123"}`), // String "123" should be safe-converted to int 123
		Handler: &handler,                            // Fixed: Pass pointer
	})

	if err != nil {
		t.Fatalf("Inject returned error: %v", err)
	}
}
