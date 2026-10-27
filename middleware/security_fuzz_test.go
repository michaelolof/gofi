package middleware

import (
	"strings"
	"testing"
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
