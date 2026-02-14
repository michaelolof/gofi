package gofi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMuxWith(t *testing.T) {
	t.Run("Basic With", func(t *testing.T) {
		mux := NewServeMux()

		mw := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Test", "true")
				next.ServeHTTP(w, r)
			})
		}

		mux.With(mw).Get("/with", RouteOptions{
			Handler: func(c Context) error {
				c.Writer().WriteHeader(http.StatusOK)
				_, err := c.Writer().Write([]byte("ok"))
				return err
			},
		})

		mux.Get("/without", RouteOptions{
			Handler: func(c Context) error {
				c.Writer().WriteHeader(http.StatusOK)
				_, err := c.Writer().Write([]byte("ok"))
				return err
			},
		})

		// Test /with
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/with", nil)
		mux.ServeHTTP(w, r)

		if w.Header().Get("X-Test") != "true" {
			t.Errorf("Expected X-Test header to be true")
		}

		// Test /without
		w = httptest.NewRecorder()
		r = httptest.NewRequest("GET", "/without", nil)
		mux.ServeHTTP(w, r)

		if w.Header().Get("X-Test") == "true" {
			t.Errorf("Expected X-Test header to be empty for /without")
		}
	})

	t.Run("Chaining With", func(t *testing.T) {
		mux := NewServeMux()

		mw1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add("X-Chain", "1")
				next.ServeHTTP(w, r)
			})
		}

		mw2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add("X-Chain", "2")
				next.ServeHTTP(w, r)
			})
		}

		mux.With(mw1).With(mw2).Get("/chain", RouteOptions{
			Handler: func(c Context) error {
				c.Writer().WriteHeader(http.StatusOK)
				_, err := c.Writer().Write([]byte("ok"))
				return err
			},
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/chain", nil)
		mux.ServeHTTP(w, r)

		// Headers are added in order 1 then 2 (since mw1 wraps mw2)
		// Wait, mw1(mw2(h)). mw1 executes first, adds "1", calls mw2. mw2 adds "2", calls h.
		values := w.Header().Values("X-Chain")
		if len(values) != 2 {
			t.Fatalf("Expected 2 X-Chain headers, got %v", values)
		}
		if values[0] != "1" || values[1] != "2" {
			t.Errorf("Expected X-Chain headers to be [1, 2], got %v", values)
		}
	})

	t.Run("Isolation", func(t *testing.T) {
		mux := NewServeMux()

		mw := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Iso", "true")
				next.ServeHTTP(w, r)
			})
		}

		subResolv := mux.With(mw)
		subResolv.Get("/sub", RouteOptions{
			Handler: func(c Context) error {
				c.Writer().WriteHeader(http.StatusOK)
				_, err := c.Writer().Write([]byte("sub"))
				return err
			},
		})

		mux.Get("/main", RouteOptions{
			Handler: func(c Context) error {
				c.Writer().WriteHeader(http.StatusOK)
				_, err := c.Writer().Write([]byte("main"))
				return err
			},
		})

		// Test /sub
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/sub", nil)
		mux.ServeHTTP(w, r)
		if w.Header().Get("X-Iso") != "true" {
			t.Errorf("Expected X-Iso header on /sub")
		}

		// Test /main
		w = httptest.NewRecorder()
		r = httptest.NewRequest("GET", "/main", nil)
		mux.ServeHTTP(w, r)
		if w.Header().Get("X-Iso") != "" {
			t.Errorf("Expected no X-Iso header on /main")
		}
	})

	t.Run("Multiple Routes on With", func(t *testing.T) {
		mux := NewServeMux()
		mw := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Multi", "true")
				next.ServeHTTP(w, r)
			})
		}

		r := mux.With(mw)
		r.Get("/r1", RouteOptions{
			Handler: func(c Context) error {
				c.Writer().WriteHeader(200)
				c.Writer().Write([]byte("r1"))
				return nil
			},
		})
		r.Get("/r2", RouteOptions{
			Handler: func(c Context) error {
				c.Writer().WriteHeader(200)
				c.Writer().Write([]byte("r2"))
				return nil
			},
		})

		for _, path := range []string{"/r1", "/r2"} {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", path, nil)
			mux.ServeHTTP(w, req)
			if w.Header().Get("X-Multi") != "true" {
				t.Errorf("Expected X-Multi header on %s", path)
			}
		}
	})

	t.Run("Mount", func(t *testing.T) {
		mux := NewServeMux()

		subMux := http.NewServeMux()
		subMux.HandleFunc("/sub/test", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("mounted"))
		})

		mux.Mount("/sub/", subMux)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/sub/test", nil)
		mux.ServeHTTP(w, r)

		if w.Body.String() != "mounted" {
			t.Errorf("Expected 'mounted', got '%s'", w.Body.String())
		}
	})

	t.Run("Mount with Inline Middleware", func(t *testing.T) {
		mux := NewServeMux()

		mw := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Mount", "true")
				next.ServeHTTP(w, r)
			})
		}

		subMux := http.NewServeMux()
		subMux.HandleFunc("/sub/test", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("mounted"))
		})

		// mux.With(mw).Mount("/sub/", subMux)
		rWith := mux.With(mw)
		rWith.Mount("/sub/", subMux)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/sub/test", nil)
		mux.ServeHTTP(w, r)

		if w.Body.String() != "mounted" {
			t.Errorf("Expected 'mounted', got '%s'", w.Body.String())
		}
		if w.Header().Get("X-Mount") != "true" {
			t.Errorf("Expected X-Mount header test to be true")
		}
	})
}
