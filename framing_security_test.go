package gofi_test

// Raw-wire framing regressions for IP-20 in gofi-docs/security-audit-2.md.
// Each case writes bytes a well-behaved client never sends and checks that
// the native transport either rejects the request before any handler runs or
// frames it exactly one way, so a front proxy and gofi cannot disagree about
// where one request ends and the next begins.

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/michaelolof/gofi"
)

// wireRecorder is a catch-all route that records every request it handles.
type wireRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *wireRecorder) handle(c gofi.Context) error {
	body, _ := io.ReadAll(c.Request().Body)
	r.mu.Lock()
	r.seen = append(r.seen, c.Request().Method+" "+c.Request().URL.Path()+" "+string(body))
	r.mu.Unlock()
	switch c.Request().URL.Path() {
	case "/reject":
		return c.SendString(http.StatusUnauthorized, "rejected")
	case "/no-content":
		return c.SendString(http.StatusNoContent, "must not be sent")
	case "/not-modified":
		return c.SendString(http.StatusNotModified, "must not be sent")
	}
	return c.SendString(http.StatusOK, "path="+c.Request().URL.Path()+" tag="+c.Request().Header.Get("X-Tag"))
}

func (r *wireRecorder) handled() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

func (r *wireRecorder) reset() {
	r.mu.Lock()
	r.seen = nil
	r.mu.Unlock()
}

func startWireServer(t *testing.T, opts *gofi.ServerOptions) (string, *wireRecorder) {
	t.Helper()
	s := gofi.NewFasthttpServer()
	if opts != nil {
		s = gofi.NewFasthttpServer(*opts)
	}
	rec := &wireRecorder{}
	for _, register := range []func(string, gofi.RouteOptions){s.Router().Get, s.Router().Post, s.Router().Head} {
		register("/*path", gofi.RouteOptions{Handler: rec.handle})
	}
	return startServer(t, s), rec
}

// exchange writes raw on one connection and reads until the server closes it
// or the deadline passes.
func exchange(t *testing.T, addr, raw string) string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))
	if _, err := io.WriteString(conn, raw); err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(conn)
	return string(out)
}

// readResponses parses the responses in out, one per request method in
// methods, and fails if any is malformed.
func readResponses(t *testing.T, out string, methods ...string) []*http.Response {
	t.Helper()
	br := bufio.NewReader(strings.NewReader(out))
	var resps []*http.Response
	for i, method := range methods {
		resp, err := http.ReadResponse(br, &http.Request{Method: method})
		if err != nil {
			t.Fatalf("response %d (%s) is malformed: %v\nraw: %q", i+1, method, err, out)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("response %d (%s) body: %v", i+1, method, err)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resps = append(resps, resp)
	}
	if rest, _ := io.ReadAll(br); len(rest) > 0 {
		t.Fatalf("unexpected bytes after %d responses: %q", len(methods), rest)
	}
	return resps
}

func bodyOf(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestSecurityWireRejectsAmbiguousFraming(t *testing.T) {
	addr, rec := startWireServer(t, nil)
	cases := map[string]string{
		"duplicate content-length":    "POST /a HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\nContent-Length: 30\r\n\r\nabc",
		"content-length list":         "POST /a HTTP/1.1\r\nHost: x\r\nContent-Length: 3, 3\r\n\r\nabc",
		"signed content-length":       "POST /a HTTP/1.1\r\nHost: x\r\nContent-Length: +3\r\n\r\nabc",
		"negative content-length":     "POST /a HTTP/1.1\r\nHost: x\r\nContent-Length: -1\r\n\r\n",
		"overflowing content-length":  "POST /a HTTP/1.1\r\nHost: x\r\nContent-Length: 99999999999999999999\r\n\r\n",
		"duplicate transfer-encoding": "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTransfer-Encoding: identity\r\n\r\n0\r\n\r\n",
		"chunked not last":            "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked, gzip\r\n\r\n0\r\n\r\n",
		"coding before chunked":       "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: gzip, chunked\r\n\r\n0\r\n\r\n",
		"unknown transfer-encoding":   "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: xchunked\r\n\r\n0\r\n\r\n",
		"space before colon (TE)":     "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding : chunked\r\n\r\n0\r\n\r\n",
		"space before colon":          "GET /a HTTP/1.1\r\nHost: x\r\nX-A : 1\r\n\r\n",
		"chunked on HTTP/1.0":         "POST /a HTTP/1.0\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n",
		"invalid chunk size":          "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\nabc\r\n0\r\n\r\n",
		"overflowing chunk size":      "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\nffffffffffffffffff\r\nabc\r\n0\r\n\r\n",
		"bare LF in chunk framing":    "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n3\nabc\n0\n\n",
		"content-length trailer":      "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTrailer: Content-Length\r\n\r\n3\r\nabc\r\n0\r\nContent-Length: 50\r\n\r\n",
		"bare CR in header":           "GET /a HTTP/1.1\r\nHost: x\rX-B: 1\r\n\r\n",
		"NUL in header value":         "GET /a HTTP/1.1\r\nHost: x\r\nX-A: a\x00b\r\n\r\n",
		"missing host":                "GET /a HTTP/1.1\r\n\r\n",
		"duplicate host":              "GET /a HTTP/1.1\r\nHost: x\r\nHost: y\r\n\r\n",
		"HTTP/0.9 request line":       "GET /a\r\n\r\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			rec.reset()
			out := exchange(t, addr, raw)
			if !strings.HasPrefix(out, "HTTP/1.1 400 ") {
				t.Errorf("want 400, got %q", out)
			}
			if seen := rec.handled(); len(seen) > 0 {
				t.Errorf("handler ran for a rejected request: %q", seen)
			}
		})
	}
}

// RFC 9112 §6.3: with both headers, Transfer-Encoding wins and the
// connection must not be reused, so bytes after the chunked body can never be
// read as a second request.
func TestSecurityWireContentLengthWithTransferEncoding(t *testing.T) {
	addr, rec := startWireServer(t, nil)
	for name, raw := range map[string]string{
		"CL then TE": "POST /a HTTP/1.1\r\nHost: x\r\nContent-Length: 4\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\nGET /smuggled HTTP/1.1\r\nHost: x\r\n\r\n",
		"TE then CL": "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nContent-Length: 30\r\n\r\n0\r\n\r\nGET /smuggled HTTP/1.1\r\nHost: x\r\n\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			rec.reset()
			out := exchange(t, addr, raw)
			if n := strings.Count(out, "HTTP/1.1 "); n > 1 {
				t.Fatalf("want at most one response, got %d: %q", n, out)
			}
			for _, seen := range rec.handled() {
				if strings.Contains(seen, "/smuggled") {
					t.Fatalf("bytes after the chunked body were handled as a request: %q", rec.handled())
				}
				if seen != "POST /a " {
					t.Fatalf("want the chunked (empty) body, handler saw %q", seen)
				}
			}
		})
	}
}

func TestSecurityWireBareLFHeadersNotHandled(t *testing.T) {
	addr, rec := startWireServer(t, nil)
	out := exchange(t, addr, "GET /a HTTP/1.1\nHost: x\n\n")
	if strings.HasPrefix(out, "HTTP/1.1 2") || len(rec.handled()) > 0 {
		t.Fatalf("bare-LF request was handled: %q, handler saw %q", out, rec.handled())
	}
}

// A folded continuation line joins the previous value; it must never become a
// header of its own (RFC 9112 §5.2).
func TestSecurityWireObsFoldDoesNotCreateHeader(t *testing.T) {
	s := gofi.NewFasthttpServer()
	s.Router().Get("/a", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		h := c.Request().Header
		return c.SendString(200, fmt.Sprintf("admin=%q", h.Get("X-Admin")))
	}})
	addr := startServer(t, s)
	for _, fold := range []string{" ", "\t"} {
		out := exchange(t, addr, "GET /a HTTP/1.1\r\nHost: x\r\nX-A: 1\r\n"+fold+"X-Admin: true\r\nConnection: close\r\n\r\n")
		if !strings.HasPrefix(out, "HTTP/1.1 400 ") && !strings.HasSuffix(out, `admin=""`) {
			t.Fatalf("folded line became a separate header: %q", out)
		}
	}
}

// RFC 9112 §3.2.2: an absolute-form target's authority replaces Host, and
// AllowedHosts checks the host that routing and gofi.Host actually use.
func TestSecurityWireAbsoluteFormHost(t *testing.T) {
	s := gofi.NewFasthttpServer()
	s.Router().Use(must(gofi.TrustPolicy(gofi.TrustOptions{AllowedHosts: []string{"app.example"}})))
	s.Router().Get("/a", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		return c.SendString(200, "host="+gofi.Host(c))
	}})
	addr := startServer(t, s)

	out := exchange(t, addr, "GET http://evil.example/a HTTP/1.1\r\nHost: app.example\r\nConnection: close\r\n\r\n")
	if !strings.HasPrefix(out, "HTTP/1.1 421 ") {
		t.Fatalf("absolute-form host bypassed AllowedHosts: %q", out)
	}
	out = exchange(t, addr, "GET http://app.example/a HTTP/1.1\r\nHost: evil.example\r\nConnection: close\r\n\r\n")
	if !strings.HasSuffix(out, "host=app.example") {
		t.Fatalf("want the absolute-form host, got %q", out)
	}
}

// Pipelined requests are answered in order, and nothing from the first
// request's pooled context reaches the second.
func TestSecurityWirePipelinedRequestsAreIsolated(t *testing.T) {
	addr, rec := startWireServer(t, nil)
	out := exchange(t, addr,
		"GET /first HTTP/1.1\r\nHost: x\r\nX-Tag: secret\r\n\r\n"+
			"POST /second HTTP/1.1\r\nHost: x\r\nContent-Length: 2\r\n\r\nhi"+
			"GET /third HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	resps := readResponses(t, out, "GET", "POST", "GET")
	want := []string{"path=/first tag=secret", "path=/second tag=", "path=/third tag="}
	for i, resp := range resps {
		if got := bodyOf(resp); got != want[i] {
			t.Errorf("response %d: want %q, got %q", i+1, want[i], got)
		}
	}
	if seen := rec.handled(); len(seen) != 3 || seen[1] != "POST /second hi" {
		t.Fatalf("handler saw %q", seen)
	}
}

// A request rejected without reading its body must still consume exactly
// Content-Length bytes, so a request hidden in the body is never handled and
// the next real request on the connection is.
func TestSecurityWireEarlyRejectionConsumesBody(t *testing.T) {
	s := gofi.NewFasthttpServer()
	var mu sync.Mutex
	var seen []string
	s.Router().Use(func(c gofi.Context) error {
		mu.Lock()
		seen = append(seen, strings.Clone(c.Request().URL.Path())) // the path is borrowed
		mu.Unlock()
		if c.Request().URL.Path() == "/reject" {
			return c.SendString(http.StatusUnauthorized, "rejected") // body never read
		}
		return c.Next()
	})
	s.Router().Post("/reject", gofi.RouteOptions{Handler: func(c gofi.Context) error { return nil }})
	s.Router().Get("/after", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "after") }})
	s.Router().Get("/smuggled", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "smuggled") }})
	addr := startServer(t, s)

	hidden := "GET /smuggled HTTP/1.1\r\nHost: x\r\n\r\n"
	out := exchange(t, addr,
		fmt.Sprintf("POST /reject HTTP/1.1\r\nHost: x\r\nContent-Length: %d\r\n\r\n%s", len(hidden), hidden)+
			"GET /after HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	resps := readResponses(t, out, "POST", "GET")
	if resps[0].StatusCode != http.StatusUnauthorized || bodyOf(resps[1]) != "after" {
		t.Fatalf("want 401 then the next request, got %d / %q", resps[0].StatusCode, bodyOf(resps[1]))
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(seen, ",") != "/reject,/after" {
		t.Fatalf("handled %q; the body of a rejected request was parsed as a request", seen)
	}
}

// An over-limit body is refused and the connection closed; none of its bytes
// are parsed as a request.
func TestSecurityWireOversizedBodyNotReparsed(t *testing.T) {
	opts := gofi.DefaultServerOptions()
	opts.MaxRequestBodySize = 64
	addr, rec := startWireServer(t, &opts)

	hidden := strings.Repeat("GET /smuggled HTTP/1.1\r\nHost: x\r\n\r\n", 4)
	out := exchange(t, addr, fmt.Sprintf("POST /a HTTP/1.1\r\nHost: x\r\nContent-Length: %d\r\n\r\n%s", len(hidden), hidden))
	if !strings.HasPrefix(out, "HTTP/1.1 413 ") {
		t.Fatalf("want 413, got %q", out)
	}
	if n := strings.Count(out, "HTTP/1.1 "); n != 1 {
		t.Fatalf("want one response, got %d: %q", n, out)
	}
	if seen := rec.handled(); len(seen) > 0 {
		t.Fatalf("handler ran: %q", seen)
	}
}

// HEAD, 204 and 304 responses carry no body even when the handler writes
// one; a stray body would be read as the start of the next response.
func TestSecurityWireBodylessResponses(t *testing.T) {
	addr, _ := startWireServer(t, nil)
	out := exchange(t, addr,
		"HEAD /a HTTP/1.1\r\nHost: x\r\n\r\n"+
			"GET /no-content HTTP/1.1\r\nHost: x\r\n\r\n"+
			"GET /not-modified HTTP/1.1\r\nHost: x\r\n\r\n"+
			"GET /last HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	resps := readResponses(t, out, "HEAD", "GET", "GET", "GET")
	for i, status := range []int{200, 204, 304} {
		if resps[i].StatusCode != status {
			t.Errorf("response %d: want %d, got %d", i+1, status, resps[i].StatusCode)
		}
		if b := bodyOf(resps[i]); b != "" {
			t.Errorf("response %d carried a body: %q", i+1, b)
		}
	}
	if got := bodyOf(resps[3]); got != "path=/last tag=" {
		t.Fatalf("framing broke after bodyless responses: %q", got)
	}
}

func TestSecurityWireChunkedBodyWithTrailer(t *testing.T) {
	addr, rec := startWireServer(t, nil)
	out := exchange(t, addr, "POST /a HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTrailer: X-T\r\nConnection: close\r\n\r\n3;ext=1\r\nabc\r\n2\r\nde\r\n0\r\nX-T: 1\r\n\r\n")
	if !strings.HasPrefix(out, "HTTP/1.1 200 ") {
		t.Fatalf("want 200, got %q", out)
	}
	if seen := rec.handled(); len(seen) != 1 || seen[0] != "POST /a abcde" {
		t.Fatalf("handler saw %q", seen)
	}
}

type epilogueSchema struct {
	Request struct {
		Body struct {
			Description string `json:"description" validate:"required"`
		}
	}
}

// Bytes before the first boundary and after the closing boundary are not
// parts, even when they look like one.
func TestSecurityWireMultipartPreambleAndEpilogueIgnored(t *testing.T) {
	s := gofi.NewFasthttpServer()
	s.Router().Post("/upload", gofi.RouteOptions{
		Schema: &epilogueSchema{},
		Handler: func(c gofi.Context) error {
			req, err := gofi.ValidateAndBind[epilogueSchema](c)
			if err != nil {
				return err
			}
			return c.SendString(200, req.Request.Body.Description)
		},
	})
	addr := startServer(t, s)

	fake := "--b\r\nContent-Disposition: form-data; name=\"description\"\r\n\r\nINJECTED\r\n--b--\r\n"
	var body bytes.Buffer
	body.WriteString("preamble\r\n")
	w := multipart.NewWriter(&body)
	_ = w.SetBoundary("b")
	_ = w.WriteField("description", "real")
	_ = w.Close()
	body.WriteString("\r\n" + fake)

	out := exchange(t, addr, fmt.Sprintf(
		"POST /upload HTTP/1.1\r\nHost: x\r\nContent-Type: multipart/form-data; boundary=b\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		body.Len(), body.String()))
	resps := readResponses(t, out, "POST")
	if got := bodyOf(resps[0]); resps[0].StatusCode != 200 || got != "real" {
		t.Fatalf("want 200 \"real\", got %d %q", resps[0].StatusCode, got)
	}
}
