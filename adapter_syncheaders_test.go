package gofi

import (
	"net/http"
	"testing"
)

// TestSyncHeaders_NoDoubleFlush_WriteHeaderPath verifies that when middleware sets
// a response header and the handler's error path calls WriteHeader (which triggers
// syncHeaders), the mux-level final flush does not write the header a second time.
//
// Before the fix: syncHeaders lacked a headerSync guard, so WriteHeader flushed
// the adapter header map to fasthttp AND the subsequent mux flush did the same,
// producing two identical header values.
func TestSyncHeaders_NoDoubleFlush_WriteHeaderPath(t *testing.T) {
	r := NewRouter()

	r.Use(func(c Context) error {
		c.Writer().Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		return c.Next()
	})

	r.UseErrorHandler(func(err error, c Context) {
		c.Writer().WriteHeader(http.StatusBadRequest)
		_, _ = c.Writer().Write([]byte(`{"error":"bad_request"}`))
	})

	handler := DefineHandler(RouteOptions{
		Handler: func(c Context) error {
			return NewHTTPError(http.StatusBadRequest, "bad_request")
		},
	})
	r.Get("/test", handler)

	w, err := r.Test(TestOptions{Method: "GET", Path: "/test"})
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	values := w.HeaderMap.Values("Access-Control-Allow-Origin")
	if len(values) != 1 {
		t.Errorf("expected exactly 1 Access-Control-Allow-Origin value, got %d: %v", len(values), values)
	}
}

// TestSyncHeaders_MuxFlushOnSendBytesPath verifies that when middleware sets a
// header and the handler sends the response via SendString (a direct fasthttp
// path that does not call Write on the adapter), the mux-level final flush is
// the sole sync path and still writes the header exactly once.
func TestSyncHeaders_MuxFlushOnSendBytesPath(t *testing.T) {
	r := NewRouter()

	r.Use(func(c Context) error {
		c.Writer().Header().Set("X-Request-ID", "abc-123")
		return c.Next()
	})

	handler := DefineHandler(RouteOptions{
		Handler: func(c Context) error {
			return c.SendString(200, "ok")
		},
	})
	r.Get("/test", handler)

	w, err := r.Test(TestOptions{Method: "GET", Path: "/test"})
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	values := w.HeaderMap.Values("X-Request-ID")
	if len(values) != 1 {
		t.Errorf("expected exactly 1 X-Request-ID value, got %d: %v", len(values), values)
	}
}

// TestSyncHeaders_IntentionalMultiValueHeaderPreserved verifies that intentionally
// adding two distinct Set-Cookie values via Header().Add() results in exactly two
// cookie values — not four (which would happen if syncHeaders ran twice).
func TestSyncHeaders_IntentionalMultiValueHeaderPreserved(t *testing.T) {
	r := NewRouter()

	r.Use(func(c Context) error {
		c.Writer().Header().Add("Set-Cookie", "session=abc; Path=/")
		c.Writer().Header().Add("Set-Cookie", "theme=dark; Path=/")
		return c.Next()
	})

	handler := DefineHandler(RouteOptions{
		Handler: func(c Context) error {
			c.Writer().WriteHeader(http.StatusOK)
			_, _ = c.Writer().Write([]byte(`"ok"`))
			return nil
		},
	})
	r.Get("/test", handler)

	w, err := r.Test(TestOptions{Method: "GET", Path: "/test"})
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	setCookies := w.HeaderMap.Values("Set-Cookie")
	if len(setCookies) != 2 {
		t.Errorf("expected exactly 2 Set-Cookie values, got %d: %v", len(setCookies), setCookies)
	}
}

// TestSyncHeaders_InjectNoDoubleFlush verifies that the Inject() path does not
// duplicate headers. When the handler calls WriteHeader, syncHeaders flushes
// adapter headers to fctx.Response.Header and sets headerSync=true. Inject must
// not then also read c.rw.header directly for those already-synced values.
func TestSyncHeaders_InjectNoDoubleFlush(t *testing.T) {
	r := NewRouter()

	r.Use(func(c Context) error {
		c.Writer().Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		return c.Next()
	})

	handler := DefineHandler(RouteOptions{
		Handler: func(c Context) error {
			c.Writer().WriteHeader(http.StatusOK)
			_, _ = c.Writer().Write([]byte(`"ok"`))
			return nil
		},
	})
	r.Get("/test", handler)

	w, err := r.Inject(InjectOptions{
		Method:  "GET",
		Path:    "/test",
		Handler: &handler,
	})
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	values := w.HeaderMap.Values("Access-Control-Allow-Origin")
	if len(values) != 1 {
		t.Errorf("expected exactly 1 Access-Control-Allow-Origin value, got %d: %v", len(values), values)
	}
}

// TestSyncHeaders_InjectUnsyncedHeaders verifies that when the handler uses only
// fasthttp-native paths (no adapter WriteHeader/Write), Inject still captures
// headers that were set through the adapter — i.e. the unsynced fallback branch
// in Inject() works correctly after the fix.
func TestSyncHeaders_InjectUnsyncedHeaders(t *testing.T) {
	r := NewRouter()

	r.Use(func(c Context) error {
		c.Writer().Header().Set("X-Custom", "from-middleware")
		return c.Next()
	})

	handler := DefineHandler(RouteOptions{
		Handler: func(c Context) error {
			// SendString goes through the fasthttp native path, not the adapter
			// Write/WriteHeader path, so syncHeaders is NOT called by the handler.
			// Inject's unsynced fallback must pick up the adapter header.
			return c.SendString(200, "ok")
		},
	})
	r.Get("/test", handler)

	w, err := r.Inject(InjectOptions{
		Method:  "GET",
		Path:    "/test",
		Handler: &handler,
	})
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	values := w.HeaderMap.Values("X-Custom")
	if len(values) != 1 {
		t.Errorf("expected exactly 1 X-Custom value, got %d: %v", len(values), values)
	}
}
