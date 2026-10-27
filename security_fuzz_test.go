package gofi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/valyala/fasthttp"
)

// Fuzz targets for the security boundaries in gofi-docs/security-audit-2.md.
// Run one with: go test -run '^$' -fuzz '^FuzzErrorEnvelope$' -fuzztime=60s .

func FuzzErrorEnvelope(f *testing.F) {
	f.Add(400, "bad \"input\"\x01\x1b[31m\xff")
	f.Add(503, "internal detail")
	f.Add(0, "")
	f.Fuzz(func(t *testing.T, code int, msg string) {
		var fctx fasthttp.RequestCtx
		c := &context{fctx: &fctx}
		defaultErrorHandler(NewHTTPError(code, msg), c)
		var body map[string]any
		if err := json.Unmarshal(fctx.Response.Body(), &body); err != nil {
			t.Fatalf("invalid JSON %q: %v", fctx.Response.Body(), err)
		}
		if string(fctx.Response.Header.ContentType()) != "application/json" {
			t.Fatalf("content type %q", fctx.Response.Header.ContentType())
		}
		status := fctx.Response.StatusCode()
		if status < 400 || status > 599 {
			t.Fatalf("status %d outside 4xx/5xx", status)
		}
	})
}

func FuzzValidHeaderField(f *testing.F) {
	f.Add("value")
	f.Add("a\r\nSet-Cookie: x=1")
	f.Fuzz(func(t *testing.T, s string) {
		if validHeaderField(s) == strings.ContainsAny(s, "\r\n\x00") {
			t.Fatalf("validHeaderField(%q) = %v", s, validHeaderField(s))
		}
	})
}

func FuzzReadBodyLimited(f *testing.F) {
	f.Add([]byte(`{"a":1}`), int64(7))
	f.Add([]byte("abc"), int64(0))
	f.Fuzz(func(t *testing.T, body []byte, limit int64) {
		if limit < 0 {
			limit = -limit
		}
		got, err := ReadBodyLimited(bytes.NewReader(body), limit)
		if int64(len(body)) > limit {
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) || httpErr.Code != 413 {
				t.Fatalf("body of %d bytes over limit %d: err %v", len(body), limit, err)
			}
			return
		}
		if err != nil || !bytes.Equal(got, body) {
			t.Fatalf("body within limit altered: %q -> %q (%v)", body, got, err)
		}
	})
}

func FuzzTrustPolicyResolve(f *testing.F) {
	f.Add("203.0.113.7, 10.0.0.2", "https")
	f.Add("not-an-ip,,::ffff:1.2.3.4", "HTTPS , http")
	policy, err := NewTrustPolicy(TrustOptions{TrustedProxies: []string{"127.0.0.1", "10.0.0.0/8"}})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, xff, proto string) {
		var fctx fasthttp.RequestCtx
		fctx.Init(&fasthttp.Request{}, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, nil)
		fctx.Request.Header.Set("X-Forwarded-For", xff)
		fctx.Request.Header.Set("X-Forwarded-Proto", proto)
		ip, scheme := policy.resolve(&context{fctx: &fctx})
		if _, err := netip.ParseAddr(ip); err != nil {
			t.Fatalf("resolved client %q is not an IP", ip)
		}
		if scheme != "http" && scheme != "https" {
			t.Fatalf("resolved scheme %q", scheme)
		}
	})
}
