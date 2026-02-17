package gofi

import (
	"errors"
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

func TestUsePreHandler(t *testing.T) {
	r := NewServeMux()

	// Global PreHandler 1
	r.UsePreHandler(func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			c.Writer().Header().Add("X-PreHandler-1", "executed")
			return next(c)
		}
	})

	// Global PreHandler 2
	r.UsePreHandler(func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			c.Writer().Header().Add("X-PreHandler-2", "executed")
			return next(c)
		}
	})

	handler := DefineHandler(RouteOptions{
		Handler: func(c Context) error {
			return c.SendString(200, "OK")
		},
	})

	r.Get("/test", handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Header().Get("X-PreHandler-1") != "executed" {
		t.Error("Expected X-PreHandler-1 to be executed")
	}
	if w.Header().Get("X-PreHandler-2") != "executed" {
		t.Error("Expected X-PreHandler-2 to be executed")
	}
}

func TestUsePreHandler_GroupIsolation(t *testing.T) {
	r := NewServeMux()

	// Base handler
	r.UsePreHandler(func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			c.Writer().Header().Add("X-Base", "true")
			return next(c)
		}
	})

	// Group 1
	r.Group(func(sub Router) {
		sub.UsePreHandler(func(next HandlerFunc) HandlerFunc {
			return func(c Context) error {
				c.Writer().Header().Add("X-Group-1", "true")
				return next(c)
			}
		})

		sub.Get("/group1", DefineHandler(RouteOptions{
			Handler: func(c Context) error { return c.SendString(200, "G1") },
		}))
	})

	// Group 2 (Should NOT have X-Group-1)
	r.Group(func(sub Router) {
		sub.Get("/group2", DefineHandler(RouteOptions{
			Handler: func(c Context) error { return c.SendString(200, "G2") },
		}))
	})

	// Test Group 1
	w1 := httptest.NewRecorder()
	r1, _ := http.NewRequest("GET", "/group1", nil)
	r.ServeHTTP(w1, r1)

	if w1.Header().Get("X-Base") != "true" {
		t.Error("Group 1 should have base prehandler")
	}
	if w1.Header().Get("X-Group-1") != "true" {
		t.Error("Group 1 should have group prehandler")
	}

	// Test Group 2
	w2 := httptest.NewRecorder()
	r2, _ := http.NewRequest("GET", "/group2", nil)
	r.ServeHTTP(w2, r2)

	if w2.Header().Get("X-Base") != "true" {
		t.Error("Group 2 should have base prehandler")
	}
	if w2.Header().Get("X-Group-1") == "true" {
		t.Error("Group 2 should NOT have group 1 prehandler (Leak detected!)")
	}
}

func TestUsePreHandler_Inject(t *testing.T) {
	r := NewServeMux()

	r.UsePreHandler(func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			c.Writer().Header().Add("X-Injected", "true")
			return next(c)
		}
	})

	handler := DefineHandler(RouteOptions{
		Handler: func(c Context) error {
			return c.SendString(200, "OK")
		},
	})

	w, err := r.Inject(InjectOptions{
		Method:  "GET",
		Path:    "/inject",
		Handler: &handler,
	})

	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	if w.Header().Get("X-Injected") != "true" {
		t.Error("Inject should execute global prehandlers")
	}
}

func TestUsePreHandler_ExecutionOrder(t *testing.T) {
	r := NewServeMux()
	var order []string

	r.UsePreHandler(func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			order = append(order, "global-start")
			err := next(c)
			order = append(order, "global-end")
			return err
		}
	})

	handler := DefineHandler(RouteOptions{
		PreHandlers: []PreHandler{
			func(next HandlerFunc) HandlerFunc {
				return func(c Context) error {
					order = append(order, "route-start")
					err := next(c)
					order = append(order, "route-end")
					return err
				}
			},
		},
		Handler: func(c Context) error {
			order = append(order, "handler")
			return nil
		},
	})

	r.Get("/order", handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/order", nil)
	r.ServeHTTP(w, req)

	expected := []string{"global-start", "route-start", "handler", "route-end", "global-end"}

	if len(order) != len(expected) {
		t.Fatalf("Expected order length %d, got %d: %v", len(expected), len(order), order)
	}

	for i, v := range expected {
		if order[i] != v {
			t.Errorf("Expected order[%d] to be %s, got %s", i, v, order[i])
		}
	}
}

func TestUsePreHandler_ErrorShortCircuit(t *testing.T) {
	r := NewServeMux()

	r.UsePreHandler(func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			return errors.New("auth failed")
		}
	})

	handler := DefineHandler(RouteOptions{
		Handler: func(c Context) error {
			t.Error("Handler should not be called")
			return nil
		},
	})

	r.Get("/short-circuit", handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/short-circuit", nil)
	r.ServeHTTP(w, req)

	// Default error handler should catch the error and return 500 (or whatever default is)
	// We just want to ensure handler wasn't called (checked above)
	if w.Code != 500 {
		t.Logf("Expected code 500 for unhandled error, got %d", w.Code)
	}
}
