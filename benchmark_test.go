package gofi

import (
	"net/http/httptest"
	"testing"
)

// BenchmarkSimpleRequest measures the allocations and performance of a simple request
// handled by the Gofi router. This serves as a baseline for context pooling optimizations.
func BenchmarkSimpleRequest(b *testing.B) {
	// Define a simple schema to force some context usage (validation/binding)
	type pingSchema struct {
		Request struct {
			Body struct {
				Msg string `json:"msg"`
			}
		}
		Ok struct {
			Body struct {
				Reply string `json:"reply"`
			}
		}
	}

	handler := DefineHandler(RouteOptions{
		Schema: &pingSchema{},
		Handler: func(c Context) error {
			// Access context methods to ensure they are exercised
			_ = c.Request()
			_ = c.Writer()

			// Simple response
			return c.SendString(200, "pong")
		},
	})

	r := NewServeMux()
	r.Post("/ping", handler)

	// Pre-create request to avoid benchmarking http.NewRequest allocations
	req := httptest.NewRequest("POST", "/ping", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Reset recorder for each iteration to simulate fresh request handling
		// Note: httptest.ResponseRecorder might accumulate data, but for allocs per op
		// regarding the router's context, this is sufficient.
		// Ideally we'd use a pooled response writer mock if we wanted to be super precise about just the router
		// but httptest is standard.

		// ServeHTTP creates the context
		r.ServeHTTP(w, req)
	}
}

// BenchmarkContextCreation isolates the context creation cost if we were to expose it,
// but since it's internal to ServeHTTP, we benchmark the minimal route possible.
func BenchmarkMinimalRoute(b *testing.B) {
	r := NewServeMux()
	r.Get("/min", RouteOptions{
		Handler: func(c Context) error {
			return nil
		},
	})

	req := httptest.NewRequest("GET", "/min", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.ServeHTTP(w, req)
	}
}
