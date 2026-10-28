package gofi

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

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
	policy, err := newTrustPolicy(TrustOptions{TrustedProxies: []string{"127.0.0.1", "10.0.0.0/8"}})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, xff, proto string) {
		var fctx fasthttp.RequestCtx
		fctx.Init(&fasthttp.Request{}, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, nil)
		fctx.Request.Header.Set("X-Forwarded-For", xff)
		fctx.Request.Header.Set("X-Forwarded-Proto", proto)
		ip, scheme, _ := policy.resolve(&context{fctx: &fctx})
		if _, err := netip.ParseAddr(ip); err != nil {
			t.Fatalf("resolved client %q is not an IP", ip)
		}
		if scheme != "http" && scheme != "https" {
			t.Fatalf("resolved scheme %q", scheme)
		}
	})
}

// fuzzSimpleSchema has a common shape that binds through the compiled
// fastjson path.
type fuzzSimpleSchema struct {
	Request struct {
		Body struct {
			Name  string  `json:"name" validate:"required,max=64"`
			Count int8    `json:"count" validate:"min=0"`
			Ratio float32 `json:"ratio"`
			On    bool    `json:"on"`
			Tags  []string
			Inner struct {
				ID    uint16 `json:"id"`
				Items []struct {
					Key string `json:"key"`
					Val int64  `json:"val"`
				} `json:"items"`
			} `json:"inner"`
		}
	}
}

// fuzzGenericSchema uses features that force the generic walker.
type fuzzGenericSchema struct {
	Request struct {
		Body struct {
			Raw  json.RawMessage `json:"raw"`
			Map  map[string]any  `json:"map"`
			Any  any             `json:"any"`
			Ptr  *int32          `json:"ptr"`
			When time.Time       `json:"when"`
			List []*struct {
				A []float64 `json:"a"`
			} `json:"list"`
		}
	}
}

// fuzzBind injects body into a route bound with schema S and fails on a
// panic or any 5xx: hostile input must be a client error, never a crash.
func fuzzBind[S any](t *testing.T, r Router, contentType string, body []byte) {
	t.Helper()
	resp, err := r.Inject(InjectOptions{
		Method:  "POST",
		Path:    "/bind",
		Headers: map[string]string{"Content-Type": contentType},
		Body:    bytes.NewReader(body),
		Handler: &RouteOptions{
			Schema: new(S),
			Handler: func(c Context) error {
				_, err := ValidateAndBind[S](c)
				if err != nil {
					return err
				}
				return c.SendString(200, "ok")
			},
		},
	})
	if err != nil && strings.Contains(err.Error(), "panic") {
		t.Fatalf("binding panicked on %q: %v", body, err)
	}
	if resp == nil {
		t.Logf("request not built: %v", err)
		return
	}
	if resp.StatusCode >= 500 {
		t.Fatalf("binding %q answered %d: %s", body, resp.StatusCode, resp.Body)
	}
}

func FuzzBindJSONBody(f *testing.F) {
	f.Add([]byte(`{"name":"a","count":1,"ratio":0.5,"on":true,"Tags":["x"],"inner":{"id":7,"items":[{"key":"k","val":-1}]}}`))
	f.Add([]byte(`{"name":"a","count":300,"inner":{"id":70000}}`))
	f.Add([]byte(`{"raw":{"x":[1,2]},"map":{"a":{"b":null}},"any":[1,"2",{}],"ptr":-5,"when":"2026-01-02T03:04:05Z","list":[null,{"a":[1e308,-0]}]}`))
	f.Add([]byte(`[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[]]]`))
	f.Add([]byte(`{"name":"\ud800","count":"1","inner":{"items":{"key":1}}}`))
	simple := NewRouter()
	generic := NewRouter()
	f.Fuzz(func(t *testing.T, body []byte) {
		fuzzBind[fuzzSimpleSchema](t, simple, "application/json", body)
		fuzzBind[fuzzGenericSchema](t, generic, "application/json", body)
	})
}

func FuzzBindFormBody(f *testing.F) {
	f.Add([]byte("name=a&count=1&Tags=x&Tags=y&inner.id=7"))
	f.Add([]byte("name=%zz&count=-1&count=999999999999999999999&=&&&="))
	r := NewRouter()
	f.Fuzz(func(t *testing.T, body []byte) {
		fuzzBind[fuzzSimpleSchema](t, r, "application/x-www-form-urlencoded", body)
	})
}

type fuzzMultipartSchema struct {
	Request struct {
		Body struct {
			Name  string                  `json:"name" validate:"required"`
			Count int8                    `json:"count"`
			File  *multipart.FileHeader   `json:"file"`
			Files []*multipart.FileHeader `json:"files"`
		}
	}
}

func FuzzBindMultipartBody(f *testing.F) {
	var seed bytes.Buffer
	w := multipart.NewWriter(&seed)
	_ = w.SetBoundary("b")
	_ = w.WriteField("name", "a")
	_ = w.WriteField("count", "3")
	fw, _ := w.CreateFormFile("file", "a.txt")
	_, _ = fw.Write([]byte("content"))
	_ = w.Close()
	f.Add(seed.Bytes())
	f.Add([]byte("--b\r\nContent-Disposition: form-data; name=\"name\"\r\n\r\nx\r\n--b--\r\n--b\r\n"))
	f.Add([]byte("--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"../../etc/passwd\"\r\n\r\nx\r\n--b"))
	r := NewRouter()
	f.Fuzz(func(t *testing.T, body []byte) {
		fuzzBind[fuzzMultipartSchema](t, r, "multipart/form-data; boundary=b", body)
	})
}

func FuzzNormalizeHost(f *testing.F) {
	f.Add("Example.COM.:8080")
	f.Add("[::1]:443")
	f.Add("a..b")
	f.Add("ex ample.com")
	f.Fuzz(func(t *testing.T, h string) {
		name, ok := normalizeHost(h)
		if !ok {
			return
		}
		if name == "" || name != strings.ToLower(name) || strings.ContainsAny(name, " \t\r\n/\\@?#%") {
			t.Fatalf("normalizeHost(%q) = %q", h, name)
		}
		if again, ok := normalizeHost(name); !ok || again != name {
			t.Fatalf("normalizeHost is not idempotent: %q -> %q -> %q (%v)", h, name, again, ok)
		}
	})
}

func FuzzForwardedHeader(f *testing.F) {
	f.Add(`for="[2001:db8::1]:4711";proto=https;host=api.example.com, for=10.0.0.2`)
	f.Add(`for=unknown;proto=ftp, for=_hidden, for="`)
	policy, err := newTrustPolicy(TrustOptions{TrustedProxies: []string{"127.0.0.1", "10.0.0.0/8"}, UseForwarded: true, AllowedHosts: []string{"api.example.com"}})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, forwarded string) {
		var fctx fasthttp.RequestCtx
		fctx.Init(&fasthttp.Request{}, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, nil)
		fctx.Request.Header.Set("Host", "api.example.com")
		fctx.Request.Header.Set("Forwarded", forwarded)
		c := &context{fctx: &fctx}
		ip, scheme, _ := policy.resolve(c)
		if _, err := netip.ParseAddr(ip); err != nil {
			t.Fatalf("resolved client %q is not an IP", ip)
		}
		if scheme != "http" && scheme != "https" {
			t.Fatalf("resolved scheme %q", scheme)
		}
		if err := policy.middleware(c); err == nil {
			if name, ok := normalizeHost(c.host); !ok || name != "api.example.com" {
				t.Fatalf("host %q accepted outside AllowedHosts", c.host)
			}
		}
	})
}
