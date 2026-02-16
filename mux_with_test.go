package gofi

import (
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware(t *testing.T) {
	t.Run("Global Middleware", func(t *testing.T) {
		r := NewServeMux()

		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-logger", "1")
				next.ServeHTTP(w, r)
			})
		})

		r.Get("/test", RouteOptions{
			Handler: func(c Context) error {
				return c.SendString(200, "finished")
			},
		})

		w := httptest.NewRecorder()
		tr := httptest.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, tr)

		if w.Header().Get("x-logger") != "1" {
			t.Errorf("Expected x-logger header to be 1")
		}
	})

	t.Run("Inline Middleware", func(t *testing.T) {
		r := NewServeMux()

		logger := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-logger", "1")
				next.ServeHTTP(w, r)
			})
		}

		r.Get("/test-1", RouteOptions{
			PreHandlers: []PreHandler{
				func(next HandlerFunc) HandlerFunc {
					return func(c Context) error {
						c.Writer().Header().Set("x-test-1", "1")
						return next(c)
					}
				},
			},
			Handler: func(c Context) error {
				return c.SendString(200, "finished")
			},
		})

		// Should have inline middleware
		r.With(logger).Get("/test-2", RouteOptions{
			Handler: func(c Context) error {
				return c.SendString(200, "finished")
			},
		})

		// Should not have inline middleware
		r.Get("/test-3", RouteOptions{
			Handler: func(c Context) error {
				return c.SendString(200, "finished")
			},
		})

		w := httptest.NewRecorder()
		tr := httptest.NewRequest("GET", "/test-1", nil)
		r.ServeHTTP(w, tr)
		if w.Header().Get("x-test-1") != "1" {
			t.Errorf("Expected x-test-1 header to be 1")
		}

		w = httptest.NewRecorder()
		tr = httptest.NewRequest("GET", "/test-2", nil)
		r.ServeHTTP(w, tr)
		if w.Header().Get("x-logger") != "1" {
			t.Errorf("Expected x-logger header to be 1")
		}

		w = httptest.NewRecorder()
		tr = httptest.NewRequest("GET", "/test-3", nil)
		r.ServeHTTP(w, tr)
		if w.Header().Get("x-logger") == "1" {
			t.Errorf("Expected x-logger header to be empty")
		}
	})

	t.Run("Group Middleware", func(t *testing.T) {
		r := NewServeMux()

		logger := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-logger", "1")
				next.ServeHTTP(w, r)
			})
		}

		r.Group(func(r Router) {
			r.Use(logger)

			r.Get("/test-1", RouteOptions{
				Handler: func(c Context) error {
					c.Writer().Header().Set("x-test", "1")
					return c.SendString(200, "finished test-1")
				},
			})
		})

		r.Get("/test-2", RouteOptions{
			Handler: func(c Context) error {
				c.Writer().Header().Set("x-test", "2")
				return c.SendString(200, "finished test-2")
			},
		})

		w := httptest.NewRecorder()
		tr := httptest.NewRequest("GET", "/test-1", nil)
		r.ServeHTTP(w, tr)
		lh, th := w.Header().Get("x-logger"), w.Header().Get("x-test")
		if lh != "1" && th != "1" {
			t.Errorf("Expected x-logger header to be 1 and x-test header to be 1")
		}

		w = httptest.NewRecorder()
		tr = httptest.NewRequest("GET", "/test-2", nil)
		r.ServeHTTP(w, tr)
		lh, th = w.Header().Get("x-logger"), w.Header().Get("x-test")
		if lh != "" && th != "2" {
			t.Errorf("Expected x-logger header to be empty and x-test header to be 2")
		}
	})
}

func TestRouteGroup(t *testing.T) {
	t.Run("Basic Group", func(t *testing.T) {
		r := NewServeMux()

		logger := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-logger", "1")
				log.Println("Request received")
				next.ServeHTTP(w, r)
			})
		}

		r.Group(func(r Router) {
			r.Use(logger)

			r.Get("/test-1", RouteOptions{
				Handler: func(c Context) error {
					c.Writer().Header().Set("x-test", "1")
					log.Println("Called test-1")
					return c.SendString(200, "finished test-1")
				},
			})

			r.Get("/test-2", RouteOptions{
				Handler: func(c Context) error {
					c.Writer().Header().Set("x-test", "2")
					log.Println("Called test-2")
					return c.SendString(200, "finished test-2")
				},
			})

			// Test /with
			w := httptest.NewRecorder()
			tr := httptest.NewRequest("GET", "/test-1", nil)
			r.ServeHTTP(w, tr)
			lh, th := w.Header().Get("x-logger"), w.Header().Get("x-test")
			if lh != "1" && th != "1" {
				t.Errorf("Expected x-logger header to be 1 and x-test header to be 1")
			}

			tr = httptest.NewRequest("GET", "/test-2", nil)
			r.ServeHTTP(w, tr)
			lh, th = w.Header().Get("x-logger"), w.Header().Get("x-test")
			if lh != "1" && th != "2" {
				t.Errorf("Expected x-logger header to be 1 and x-test header to be 2")
			}
		})

	})

	t.Run("Route Method", func(t *testing.T) {
		r := NewServeMux()

		type usersSchema struct {
			Ok struct {
				Header struct {
					Pattern string `json:"x-pattern" validate:"required"`
				}
				Body string
			}
		}

		r.Route("/api", func(r Router) {
			r.Get("/users", RouteOptions{
				Schema: &usersSchema{},
				Handler: func(c Context) error {
					log.Println("called /api/users")
					c.Writer().Header().Set("x-pattern", "/users")
					return c.SendString(200, "users list")
				},
			})

			r.Route("/v1", func(r Router) {
				r.Get("/posts", RouteOptions{
					Handler: func(c Context) error {
						log.Println("called /api/v1/posts")
						c.Writer().Header().Set("x-pattern", "/v1/posts")
						return c.SendString(200, "posts list")
					},
				})
			})
		})

		// root route should not exist
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/users", nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 || w.Header().Get("x-pattern") != "/users" {
			t.Errorf("Expected 200 for /api/users, got %d", w.Code)
		}

		// /api/v1/posts should exist
		w = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "/api/v1/posts", nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 || w.Header().Get("x-pattern") != "/v1/posts" {
			t.Errorf("Expected 200 for /api/v1/posts, got %d body %s", w.Code, w.Body.String())
		}
	})
}

// func TestMount(t *testing.T) {
// 	t.Run("Mount Sub-Handler with StripPrefix", func(t *testing.T) {
// 		mux := NewServeMux()

// 		subMux := http.NewServeMux()
// 		// Sub-handler expects "/test", not "/sub/test" because of stripping
// 		subMux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
// 			t.Logf("SubMux received request: %s %s", r.Method, r.URL.Path)
// 			w.Write([]byte("mounted"))
// 		})

// 		mux.Mount("/sub", subMux)

// 		w := httptest.NewRecorder()
// 		r := httptest.NewRequest("GET", "/sub/test", nil)
// 		mux.ServeHTTP(w, r)

// 		if w.Body.String() != "mounted" {
// 			t.Errorf("Expected 'mounted', got '%s'", w.Body.String())
// 		}
// 	})

// 	t.Run("Mount with Middlewares", func(t *testing.T) {
// 		mux := NewServeMux()

// 		mw := func(next http.Handler) http.Handler {
// 			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				w.Header().Set("X-Mount", "true")
// 				next.ServeHTTP(w, r)
// 			})
// 		}

// 		subMux := http.NewServeMux()
// 		subMux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
// 			w.Write([]byte("mounted"))
// 		})

// 		// Use With().Mount to apply middleware
// 		mux.With(mw).Mount("/sub", subMux)

// 		w := httptest.NewRecorder()
// 		r := httptest.NewRequest("GET", "/sub/test", nil)
// 		mux.ServeHTTP(w, r)

// 		if w.Body.String() != "mounted" {
// 			t.Errorf("Expected 'mounted', got '%s'", w.Body.String())
// 		}
// 		if w.Header().Get("X-Mount") != "true" {
// 			t.Errorf("Expected X-Mount header to be true")
// 		}
// 	})
// }
