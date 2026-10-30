package middleware

import (
	"strings"
	"testing"

	"github.com/michaelolof/gofi"
)

// FuzzEscapeRedirectCapture checks that a decoded path capture can never put
// control bytes, a backslash, '?' or '#' into a Location header.
func FuzzEscapeRedirectCapture(f *testing.F) {
	f.Add("/a\r\nSet-Cookie: x=1")
	f.Add("/\\evil.com")
	f.Fuzz(func(t *testing.T, capture string) {
		got := escapeRedirectCapture(capture)
		if strings.ContainsAny(got, "\r\n\x00\\?#") {
			t.Fatalf("escapeRedirectCapture(%q) = %q", capture, got)
		}
	})
}

// FuzzCSRFToken checks that no forged token passes the CSRF check, whatever
// its shape, and that parsing it never panics.
func FuzzCSRFToken(f *testing.F) {
	f.Add("v1.9999999999.AAAA.AAAA")
	f.Add("v1.-1..")
	f.Add("v1.1e9.\x00.\xff")
	f.Add(strings.Repeat("v1.", 1000))
	csrf, err := CSRF(CSRFConfig{
		SigningKey: []byte("0123456789abcdef0123456789abcdef"),
		SessionID:  func(c gofi.Context) string { return "alice" },
	})
	if err != nil {
		f.Fatal(err)
	}
	r := gofi.NewRouter()
	r.Use(csrf)
	handler := &gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }}
	f.Fuzz(func(t *testing.T, token string) {
		resp, err := r.Inject(gofi.InjectOptions{
			Method:  "POST",
			Path:    "/",
			Headers: map[string]string{"X-CSRF-Token": token, "Cookie": "csrf_=" + token},
			Handler: handler,
		})
		if err != nil && strings.Contains(err.Error(), "panic") {
			t.Fatalf("token %q panicked: %v", token, err)
		}
		if resp == nil {
			return // fasthttp refused to parse the request at all
		}
		if resp.StatusCode < 400 {
			t.Fatalf("forged token %q accepted with %d", token, resp.StatusCode)
		}
	})
}
