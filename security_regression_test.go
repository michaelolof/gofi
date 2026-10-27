package gofi_test

// Security regressions for the findings in gofi-docs/security-audit-2.md.
// Each test names the finding it covers. Panic cases run in a subprocess so a
// regression that terminates the process is reported as a test failure.

import (
	"bufio"
	stdcontext "context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/michaelolof/gofi"
	"github.com/michaelolof/gofi/middleware"
)

// startServer serves s on a pre-bound ephemeral listener and shuts it down
// with a bounded context, so a blocked handler cannot hang the test run.
func startServer(t *testing.T, s *gofi.FasthttpServer) string {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.ListenOnListener(ln) }()
	t.Cleanup(func() {
		ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 2*time.Second)
		defer cancel()
		_ = s.ShutdownWithContext(ctx)
		_ = ln.Close()
	})
	return ln.Addr().String()
}

// rawRequest writes req on a fresh connection and returns the full response.
func rawRequest(t *testing.T, addr, req string) string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(conn)
	return string(out)
}

// httpGet issues a GET through net/http, which decodes chunked bodies.
func httpGet(t *testing.T, addr, path string) (int, string) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + path)
	if err != nil {
		t.Errorf("GET %s: %v", path, err)
		return 0, ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func get(path string) string {
	return "GET " + path + " HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"
}

// inSubprocess runs the named test in a child process with child set, and
// fails if the child exits abnormally (for example by an unrecovered panic).
func inSubprocess(t *testing.T, child func(t *testing.T)) {
	t.Helper()
	if os.Getenv("GOFI_SECURITY_CHILD") == t.Name() {
		child(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), "GOFI_SECURITY_CHILD="+t.Name())
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--- PASS") {
		t.Fatalf("child process failed (%v):\n%s", err, out)
	}
}

// F-02, F-23: a malformed escape on a Router.Static route must not reach
// net/http as a nil request, and handler panics must not end the process.
func TestSecurityStaticMalformedEscapeDoesNotCrash(t *testing.T) {
	inSubprocess(t, func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644); err != nil {
			t.Fatal(err)
		}
		s := gofi.NewFasthttpServer()
		s.Router().Static("/static", dir)
		addr := startServer(t, s)

		if out := rawRequest(t, addr, get("/static/%zz")); !strings.HasPrefix(out, "HTTP/1.1 4") {
			t.Fatalf("malformed escape: want a 4xx, got %q", out)
		}
		if out := rawRequest(t, addr, get("/static/a.txt")); !strings.HasSuffix(out, "hi") {
			t.Fatalf("server did not keep serving after malformed escape: %q", out)
		}
	})
}

func TestSecurityHandlerAndErrorHandlerPanicsAreContained(t *testing.T) {
	inSubprocess(t, func(t *testing.T) {
		s := gofi.NewFasthttpServer()
		r := s.Router()
		r.Get("/panic", gofi.RouteOptions{Handler: func(c gofi.Context) error { panic("secret-db-password") }})
		r.Get("/error", gofi.RouteOptions{Handler: func(c gofi.Context) error { return errors.New("boom") }})
		r.Get("/ok", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})
		// The error handler itself panics for /error.
		r.UseErrorHandler(func(err error, c gofi.Context) {
			if err.Error() == "boom" {
				panic("error handler failed")
			}
			_ = c.SendString(500, "handled")
		})
		addr := startServer(t, s)

		out := rawRequest(t, addr, get("/panic"))
		if !strings.HasPrefix(out, "HTTP/1.1 500") || strings.Contains(out, "secret-db-password") {
			t.Fatalf("handler panic: want generic 500, got %q", out)
		}
		if out := rawRequest(t, addr, get("/ok")); !strings.HasSuffix(out, "ok") {
			t.Fatalf("server did not recover after handler panic: %q", out)
		}

		if out := rawRequest(t, addr, get("/error")); !strings.HasPrefix(out, "HTTP/1.1 500") {
			t.Fatalf("error-handler panic: want 500, got %q", out)
		}
		if out := rawRequest(t, addr, get("/ok")); !strings.HasSuffix(out, "ok") {
			t.Fatalf("server did not recover after error-handler panic: %q", out)
		}
	})
}

func TestSecurityStreamProducerPanicIsContained(t *testing.T) {
	inSubprocess(t, func(t *testing.T) {
		s := gofi.NewFasthttpServer()
		r := s.Router()
		r.Get("/stream", gofi.RouteOptions{Handler: func(c gofi.Context) error {
			return c.SetBodyStreamWriter(func(w *bufio.Writer) error { panic("producer failed") })
		}})
		r.Get("/ok", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})
		addr := startServer(t, s)

		_ = rawRequest(t, addr, get("/stream"))
		if out := rawRequest(t, addr, get("/ok")); !strings.HasSuffix(out, "ok") {
			t.Fatalf("server did not survive a stream producer panic: %q", out)
		}
	})
}

// F-24: middleware added through one router group must not replace middleware
// held by a retained sibling or child group.
func TestSecurityRetainedGroupsKeepTheirMiddleware(t *testing.T) {
	deny := func(c gofi.Context) error { return gofi.NewHTTPError(http.StatusUnauthorized, "denied") }
	other := func(c gofi.Context) error { return c.Next() }
	secret := gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "SECRET") }}

	t.Run("sibling Use", func(t *testing.T) {
		r := gofi.NewRouter()
		admin := r.Route("/admin", func(r gofi.Router) { r.Use(deny) })
		r.Route("/public", func(r gofi.Router) { r.Use(other) })
		admin.Get("/secret", secret)
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/admin/secret"})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("auth middleware bypassed: %d %q", resp.StatusCode, resp.BodyString())
		}
	})

	t.Run("root Use after child Use", func(t *testing.T) {
		r := gofi.NewRouter()
		api := r.Route("/api", func(r gofi.Router) { r.Use(deny) })
		r.Use(other)
		api.Get("/users", secret)
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/api/users"})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("auth middleware bypassed: %d %q", resp.StatusCode, resp.BodyString())
		}
	})

	t.Run("nested groups and With", func(t *testing.T) {
		r := gofi.NewRouter()
		var outer gofi.Router
		r.Group(func(g gofi.Router) {
			g.Use(deny)
			outer = g.With(other)
			g.Group(func(inner gofi.Router) { inner.Use(other) })
		})
		outer.Get("/x", secret)
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/x"})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("auth middleware bypassed: %d %q", resp.StatusCode, resp.BodyString())
		}
	})
}

// F-25: decoded request data must not split response headers.
func TestSecurityRedirectCaptureCannotInjectHeaders(t *testing.T) {
	s := gofi.NewFasthttpServer()
	s.Router().Use(middleware.Redirect(middleware.RedirectConfig{Rules: map[string]string{"/old/*": "/new/$1"}}))
	s.Router().Get("/x", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "x") }})
	addr := startServer(t, s)

	out := rawRequest(t, addr, get("/old/a%0d%0aSet-Cookie:%20session=attacker"))
	if strings.Contains(out, "\r\nSet-Cookie:") {
		t.Fatalf("header injected through redirect capture:\n%s", out)
	}
	if !strings.Contains(out, "Location: /new/a%0D%0ASet-Cookie:%20session=attacker") {
		t.Fatalf("expected an escaped Location header:\n%s", out)
	}

	out = rawRequest(t, addr, get("/old/%5Cevil.com"))
	if strings.Contains(out, "Location: /new/\\") {
		t.Fatalf("backslash left unescaped in Location:\n%s", out)
	}
}

func TestSecurityAdapterHeaderControlBytesFailSafely(t *testing.T) {
	r := gofi.NewRouter()
	r.Get("/h", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		c.Writer().Header().Set("X-Name", c.Query("v"))
		return c.SendString(200, "body")
	}})
	resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/h", Query: map[string]string{"v": "a%0d%0aSet-Cookie:%20x=1"}})
	if resp.StatusCode != http.StatusInternalServerError || resp.HasHeader("Set-Cookie") || resp.HasHeader("X-Name") {
		t.Fatalf("want a generic 500 without the header, got %d %v", resp.StatusCode, resp.HeaderMap)
	}
}

type headerSchema struct {
	Ok struct {
		Header struct {
			Name string `json:"x-name"`
		}
	}
}

func TestSecuritySchemaHeaderControlBytesRejected(t *testing.T) {
	r := gofi.NewRouter()
	r.Get("/h", gofi.RouteOptions{Schema: &headerSchema{}, Handler: func(c gofi.Context) error {
		var s headerSchema
		s.Ok.Header.Name = c.Query("v")
		return c.Send(200, s.Ok)
	}})
	resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/h", Query: map[string]string{"v": "a%0d%0aSet-Cookie:%20x=1"}})
	if resp.StatusCode == http.StatusOK || resp.HasHeader("Set-Cookie") {
		t.Fatalf("schema header with CR/LF was written: %d %v", resp.StatusCode, resp.HeaderMap)
	}
}

// F-27: a returned error or a 404/405 outcome must never become a cached 200.
func TestSecurityCacheDoesNotStoreErrors(t *testing.T) {
	r := gofi.NewRouter()
	r.Use(middleware.Cache())
	var calls atomic.Int32
	r.Get("/item", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		if calls.Add(1) == 1 {
			return gofi.NewHTTPError(http.StatusServiceUnavailable, "db unavailable")
		}
		return c.SendString(200, "real content")
	}})

	first, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/item"})
	second, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/item"})
	if first.StatusCode != http.StatusServiceUnavailable || second.StatusCode != 200 || second.BodyString() != "real content" {
		t.Fatalf("error was cached: first=%d second=%d %q", first.StatusCode, second.StatusCode, second.BodyString())
	}

	for i := 0; i < 2; i++ {
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/missing"})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("404 replayed from cache as %d on attempt %d", resp.StatusCode, i+1)
		}
	}
}

// F-28: the static helpers must not serve dotfiles or listings, and the
// middleware must not shadow routes or match on partial segments.
func TestSecurityStaticMiddlewareDefaults(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".env"), "DB_PASSWORD=hunter2")
	mustWrite(t, filepath.Join(dir, ".git", "config"), "secret")
	mustWrite(t, filepath.Join(dir, "users", "readme.txt"), "users dir")
	mustWrite(t, filepath.Join(dir, "app.js"), "js")

	r := gofi.NewRouter()
	r.Use(middleware.Static(middleware.StaticConfig{Root: dir}))
	r.Get("/", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "home") }})
	r.Get("/users", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "users-handler") }})

	cases := []struct {
		path   string
		status int
		body   string
	}{
		{"/.env", http.StatusNotFound, ""},
		{"/.git/config", http.StatusNotFound, ""},
		{"/", 200, "home"},
		{"/users", 200, "users-handler"},
		{"/app.js", 200, "js"},
	}
	for _, tc := range cases {
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: tc.path})
		if resp.StatusCode != tc.status || (tc.body != "" && resp.BodyString() != tc.body) {
			t.Errorf("GET %s: want %d %q, got %d %q", tc.path, tc.status, tc.body, resp.StatusCode, resp.BodyString())
		}
		if strings.Contains(resp.BodyString(), "hunter2") {
			t.Errorf("GET %s leaked a dotfile", tc.path)
		}
	}
}

func TestSecurityStaticMiddlewarePrefixAndRestore(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "ity"), "wrong file")
	mustWrite(t, filepath.Join(dir, "app.js"), "js")

	r := gofi.NewRouter()
	r.Use(middleware.Static(middleware.StaticConfig{Root: dir, Prefix: "/public"}))
	var seenPath string
	r.Get("/publicity", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "publicity") }})
	r.Get("/public/missing", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		seenPath = c.Path()
		return c.SendString(200, "fallback")
	}})

	if resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/publicity"}); resp.BodyString() != "publicity" {
		t.Errorf("/public matched /publicity: %q", resp.BodyString())
	}
	if resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/public/app.js"}); resp.BodyString() != "js" {
		t.Errorf("prefixed file not served: %q", resp.BodyString())
	}
	if resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/public/missing"}); resp.BodyString() != "fallback" || seenPath != "/public/missing" {
		t.Errorf("fallback did not restore the path: body=%q path=%q", resp.BodyString(), seenPath)
	}
}

func TestSecurityRouterStaticHidesDotfilesAndListings(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".env"), "SECRET=1")
	mustWrite(t, filepath.Join(dir, "app.js"), "js")
	mustWrite(t, filepath.Join(dir, "site", "index.html"), "index")

	r := gofi.NewRouter()
	r.Static("/static", dir)
	cases := []struct {
		path   string
		status int
		body   string
	}{
		{"/static/", http.StatusNotFound, ""},
		{"/static/.env", http.StatusNotFound, ""},
		{"/static/app.js", 200, "js"},
		{"/static/site/", 200, "index"},
	}
	for _, tc := range cases {
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: tc.path})
		if resp.StatusCode != tc.status || (tc.body != "" && resp.BodyString() != tc.body) {
			t.Errorf("GET %s: want %d %q, got %d %q", tc.path, tc.status, tc.body, resp.StatusCode, resp.BodyString())
		}
	}
}

// F-15: standard middleware receives a usable server-style request.
func TestSecurityWrapMiddlewareReceivesRequest(t *testing.T) {
	r := gofi.NewRouter()
	r.Use(gofi.WrapMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Header.Get("Authorization") != "Bearer token" || req.Host != "localhost" || req.URL.Path != "/" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, req)
		})
	}))
	r.Get("/", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})
	resp, err := r.Test(gofi.TestOptions{Method: "GET", Path: "/", Headers: map[string]string{"Authorization": "Bearer token"}})
	if err != nil || resp.StatusCode != 200 || resp.BodyString() != "ok" {
		t.Fatalf("want 200 ok, got %d %q err=%v", resp.StatusCode, resp.BodyString(), err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// F-17: streams are delivered incrementally and the handler returns before the
// producer finishes.
func TestSecurityLargeStreamIsDelivered(t *testing.T) {
	s := gofi.NewFasthttpServer()
	handlerReturned := make(chan struct{})
	s.Router().Get("/big", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		defer close(handlerReturned)
		return c.SetBodyStreamWriter(func(w *bufio.Writer) error {
			<-handlerReturned
			chunk := strings.Repeat("x", 1024)
			for i := 0; i < 1024; i++ {
				if _, err := w.WriteString(chunk); err != nil {
					return err
				}
			}
			return nil
		})
	}})
	addr := startServer(t, s)

	status, body := httpGet(t, addr, "/big")
	if status != 200 || len(body) != 1<<20 || strings.Count(body, "x") != 1<<20 {
		t.Fatalf("want the full 1 MiB stream, got %d with %d bytes", status, len(body))
	}
}

func TestSecuritySSEEventArrivesBeforeProducerFinishes(t *testing.T) {
	s := gofi.NewFasthttpServer()
	finish := make(chan struct{})
	s.Router().Get("/sse", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		c.Request().Context().Response.Header.SetContentType("text/event-stream")
		return c.SetBodyStreamWriter(func(w *bufio.Writer) error {
			fmt.Fprint(w, "data: first\n\n")
			if err := w.Flush(); err != nil {
				return err
			}
			<-finish
			return nil
		})
	}})
	addr := startServer(t, s)
	defer close(finish)

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	io.WriteString(conn, get("/sse"))
	br := bufio.NewReader(conn)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("first event not delivered while the producer was running: %v", err)
		}
		if strings.Contains(line, "data: first") {
			return
		}
	}
}

func TestSecurityStreamErrorsReachOnStreamError(t *testing.T) {
	r := gofi.NewRouter()
	got := make(chan error, 1)
	r.Get("/s", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		if err := gofi.OnStreamError(c, func(err error) { got <- err }); err != nil {
			return err
		}
		return c.SetBodyStreamWriter(func(w *bufio.Writer) error { return errors.New("upstream closed") })
	}})
	if _, err := r.Test(gofi.TestOptions{Method: "GET", Path: "/s"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-got:
		if err.Error() != "upstream closed" {
			t.Fatalf("unexpected stream error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream error was not delivered")
	}
}

type streamParamSchema struct {
	Request struct {
		Path struct {
			ID string `json:"id"`
		}
	}
}

// IP-05 ownership: a producer keeps its context, parameters and bound result
// after the handler returns, while other requests reuse pooled contexts.
func TestSecurityStreamProducerKeepsItsContext(t *testing.T) {
	s := gofi.NewFasthttpServer()
	r := s.Router()
	release := make(chan struct{})
	r.Get("/s/:id", gofi.RouteOptions{Schema: &streamParamSchema{}, Handler: func(c gofi.Context) error {
		bound, err := gofi.ValidateAndBind[streamParamSchema](c)
		if err != nil {
			return err
		}
		return c.SetBodyStreamWriter(func(w *bufio.Writer) error {
			<-release
			_, err := fmt.Fprintf(w, "param=%s bound=%s", c.Param("id"), bound.Request.Path.ID)
			return err
		})
	}})
	r.Get("/b/:id", gofi.RouteOptions{Schema: &streamParamSchema{}, Handler: func(c gofi.Context) error {
		bound, err := gofi.ValidateAndBind[streamParamSchema](c)
		if err != nil {
			return err
		}
		return c.SendString(200, bound.Request.Path.ID)
	}})
	addr := startServer(t, s)

	result := make(chan string, 1)
	go func() {
		_, body := httpGet(t, addr, "/s/AAAA")
		result <- body
	}()
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < 50; i++ {
		_ = rawRequest(t, addr, get(fmt.Sprintf("/b/Z%03d", i)))
	}
	close(release)
	if out := <-result; out != "param=AAAA bound=AAAA" {
		t.Fatalf("producer saw another request's state: %q", out)
	}
}

func TestSecurityCacheBypassesStreams(t *testing.T) {
	r := gofi.NewRouter()
	r.Use(middleware.Cache(), middleware.ETag())
	r.Get("/s", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		return c.SetBodyStreamWriter(func(w *bufio.Writer) error {
			_, err := w.WriteString("streamed")
			return err
		})
	}})
	for i := 0; i < 2; i++ {
		resp, err := r.Test(gofi.TestOptions{Method: "GET", Path: "/s"})
		if err != nil || resp.BodyString() != "streamed" {
			t.Fatalf("attempt %d: want streamed body, got %q err=%v", i+1, resp.BodyString(), err)
		}
	}
}

type nameSchema struct {
	Request struct {
		Body struct {
			Name string `json:"name" validate:"required"`
		}
	}
}

// F-08: parsers reject bodies over their limit instead of accepting a prefix.
func TestSecurityParserLimitsRejectOverflow(t *testing.T) {
	r := gofi.NewRouter()
	r.RegisterBodyParser(&gofi.JSONBodyParser{MaxRequestSize: 64}, &gofi.FormBodyParser{MaxRequestSize: 64})
	handler := func(c gofi.Context) error {
		s, err := gofi.ValidateAndBind[nameSchema](c)
		if err != nil {
			return err
		}
		return c.SendString(200, s.Request.Body.Name)
	}
	r.Post("/n", gofi.RouteOptions{Schema: &nameSchema{}, Handler: handler})

	post := func(contentType, body string) *gofi.InjectResponse {
		resp, _ := r.Test(gofi.TestOptions{Method: "POST", Path: "/n",
			Headers: map[string]string{"Content-Type": contentType}, Body: strings.NewReader(body)})
		return resp
	}

	exact := `{"name":"alice"}` + strings.Repeat(" ", 64-len(`{"name":"alice"}`))
	if resp := post("application/json", exact); resp.StatusCode != 200 {
		t.Errorf("JSON at the limit rejected: %d %q", resp.StatusCode, resp.BodyString())
	}
	if resp := post("application/json", exact+"X"); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("JSON one byte over the limit: want 413, got %d %q", resp.StatusCode, resp.BodyString())
	}
	if resp := post("application/x-www-form-urlencoded", "name=alice&pad="+strings.Repeat("a", 64)); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("form over the limit: want 413, got %d %q", resp.StatusCode, resp.BodyString())
	}
}

func TestSecurityMultipartLimitApplied(t *testing.T) {
	r := gofi.NewRouter()
	r.RegisterBodyParser(&gofi.MultipartBodyParser{MaxRequestSize: 64})
	r.Post("/n", gofi.RouteOptions{Schema: &nameSchema{}, Handler: func(c gofi.Context) error {
		if _, err := gofi.ValidateAndBind[nameSchema](c); err != nil {
			return err
		}
		return c.SendString(200, "ok")
	}})
	body := "--b\r\nContent-Disposition: form-data; name=\"name\"\r\n\r\n" + strings.Repeat("a", 128) + "\r\n--b--\r\n"
	resp, _ := r.Test(gofi.TestOptions{Method: "POST", Path: "/n",
		Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=b"}, Body: strings.NewReader(body)})
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("multipart over MaxRequestSize: want 413, got %d %q", resp.StatusCode, resp.BodyString())
	}
}

type numberSchema struct {
	Request struct {
		Body struct {
			Small  int8    `json:"small"`
			Count  uint    `json:"count"`
			Whole  int     `json:"whole"`
			Single float32 `json:"single"`
		}
	}
}

// IP-06: numeric binding never wraps, truncates or flips sign.
func TestSecurityNumericBindingIsExact(t *testing.T) {
	r := gofi.NewRouter()
	r.Post("/n", gofi.RouteOptions{Schema: &numberSchema{}, Handler: func(c gofi.Context) error {
		s, err := gofi.ValidateAndBind[numberSchema](c)
		if err != nil {
			return gofi.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		b := s.Request.Body
		return c.SendString(200, fmt.Sprintf("%d %d %d %v", b.Small, b.Count, b.Whole, b.Single))
	}})
	post := func(body string) *gofi.InjectResponse {
		resp, _ := r.Test(gofi.TestOptions{Method: "POST", Path: "/n",
			Headers: map[string]string{"Content-Type": "application/json"}, Body: strings.NewReader(body)})
		return resp
	}

	if resp := post(`{"small":127,"count":3,"whole":4,"single":1.5}`); resp.StatusCode != 200 || resp.BodyString() != "127 3 4 1.5" {
		t.Fatalf("valid numbers rejected: %d %q", resp.StatusCode, resp.BodyString())
	}
	for _, body := range []string{
		`{"small":300}`,
		`{"count":-1}`,
		`{"whole":1.5}`,
		`{"single":1e300}`,
	} {
		if resp := post(body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: want rejection, got %d %q", body, resp.StatusCode, resp.BodyString())
		}
	}
}

// F-05, F-06, F-07: cache isolation, eligibility, replay and size limits.
func TestSecurityCacheBoundaries(t *testing.T) {
	newRouter := func(cfg middleware.CacheConfig, handler gofi.HandlerFunc) gofi.Router {
		r := gofi.NewRouter()
		r.Use(middleware.Cache(cfg))
		r.Get("/r", gofi.RouteOptions{Handler: handler})
		return r
	}
	echo := func(c gofi.Context) error {
		return c.SendString(200, string(c.Request().Context().Host())+"|"+c.Query("x")+"|"+c.HeaderVal("Origin"))
	}
	req := func(r gofi.Router, query string, headers map[string]string) string {
		path := "/r"
		if query != "" {
			path += "?" + query
		}
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: path, Headers: headers})
		return resp.BodyString()
	}

	t.Run("host is part of the key", func(t *testing.T) {
		s := gofi.NewFasthttpServer()
		s.Router().Use(middleware.Cache())
		s.Router().Get("/r", gofi.RouteOptions{Handler: echo})
		addr := startServer(t, s)
		hostGet := func(host string) string {
			out := rawRequest(t, addr, "GET /r HTTP/1.1\r\nHost: "+host+"\r\nConnection: close\r\n\r\n")
			return out[strings.Index(out, "\r\n\r\n")+4:]
		}
		hostGet("tenant-a")
		if got := hostGet("tenant-b"); !strings.HasPrefix(got, "tenant-b|") {
			t.Fatalf("response cached across hosts: %q", got)
		}
	})

	t.Run("repeated parameters keep their order", func(t *testing.T) {
		var calls atomic.Int32
		r := newRouter(middleware.CacheConfig{}, func(c gofi.Context) error {
			calls.Add(1)
			return echo(c)
		})
		req(r, "x=first&x=second", nil)
		if got := req(r, "x=second&x=first", nil); !strings.Contains(got, "|second|") || calls.Load() != 2 {
			t.Fatalf("reordered parameters shared an entry: %q (calls=%d)", got, calls.Load())
		}
	})

	t.Run("Vary partitions entries", func(t *testing.T) {
		r := newRouter(middleware.CacheConfig{}, func(c gofi.Context) error {
			c.Writer().Header().Set("Vary", "Origin")
			return echo(c)
		})
		req(r, "", map[string]string{"Origin": "https://a.example"})
		if got := req(r, "", map[string]string{"Origin": "https://b.example"}); !strings.HasSuffix(got, "https://b.example") {
			t.Fatalf("Vary: Origin ignored: %q", got)
		}
	})

	bypass := map[string]gofi.HandlerFunc{
		"Set-Cookie": func(c gofi.Context) error {
			c.Writer().Header().Add("Set-Cookie", "session=abc")
			return c.SendString(200, "personal")
		},
		"private": func(c gofi.Context) error {
			c.Writer().Header().Set("Cache-Control", "private")
			return c.SendString(200, "personal")
		},
		"no-store": func(c gofi.Context) error {
			c.Request().Context().Response.Header.Set("Cache-Control", "no-store")
			return c.SendString(200, "personal")
		},
		"Vary *": func(c gofi.Context) error {
			c.Writer().Header().Set("Vary", "*")
			return c.SendString(200, "personal")
		},
		"redirect": func(c gofi.Context) error {
			c.Writer().Header().Set("Location", "/elsewhere")
			return c.SendString(http.StatusFound, "")
		},
	}
	for name, handler := range bypass {
		t.Run("bypass "+name, func(t *testing.T) {
			var calls atomic.Int32
			r := newRouter(middleware.CacheConfig{CacheHeader: true}, func(c gofi.Context) error {
				calls.Add(1)
				return handler(c)
			})
			req(r, "", nil)
			resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/r"})
			if calls.Load() != 2 {
				t.Fatalf("response was cached")
			}
			if cc := resp.Header("Cache-Control"); strings.Contains(cc, "public") {
				t.Fatalf("restrictive response marked public: %q", cc)
			}
		})
	}

	t.Run("replays safe headers only", func(t *testing.T) {
		r := newRouter(middleware.CacheConfig{}, func(c gofi.Context) error {
			c.Writer().Header().Set("ETag", `"v1"`)
			c.Writer().Header().Set("X-Request-Specific", "first")
			return c.SendString(200, "body")
		})
		req(r, "", nil)
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/r"})
		if resp.Header("ETag") != `"v1"` || resp.HasHeader("X-Request-Specific") {
			t.Fatalf("unexpected replayed headers: %v", resp.HeaderMap)
		}
	})

	t.Run("oversize responses are never stored", func(t *testing.T) {
		var calls atomic.Int32
		r := newRouter(middleware.CacheConfig{MaxBytes: 64}, func(c gofi.Context) error {
			calls.Add(1)
			return c.SendString(200, strings.Repeat("a", 128))
		})
		req(r, "", nil)
		req(r, "", nil)
		if calls.Load() != 2 {
			t.Fatalf("response larger than MaxBytes was cached")
		}
	})
}

// F-26: results and parameters retained by a handler keep their values after
// later requests, including requests on the same keep-alive connection.
func TestSecurityRetainedBoundResultIsOwned(t *testing.T) {
	r := gofi.NewRouter()
	var kept *nameSchema
	r.Post("/n", gofi.RouteOptions{Schema: &nameSchema{}, Handler: func(c gofi.Context) error {
		s, err := gofi.ValidateAndBind[nameSchema](c)
		if err != nil {
			return err
		}
		if kept == nil {
			kept = s
		}
		return c.SendString(200, s.Request.Body.Name)
	}})
	for _, name := range []string{"alice", "bob", "carol"} {
		r.Test(gofi.TestOptions{Method: "POST", Path: "/n",
			Headers: map[string]string{"Content-Type": "application/json"}, Body: strings.NewReader(`{"name":"` + name + `"}`)})
	}
	if kept.Request.Body.Name != "alice" {
		t.Fatalf("retained result was reused by a later request: %q", kept.Request.Body.Name)
	}
}

func TestSecurityRetainedValuesSurviveKeepAlive(t *testing.T) {
	s := gofi.NewFasthttpServer()
	type kept struct {
		bound  string
		copied string
		snap   string
	}
	var values []kept
	s.Router().Get("/u/:id", gofi.RouteOptions{Schema: &streamParamSchema{}, Handler: func(c gofi.Context) error {
		b, err := gofi.ValidateAndBind[streamParamSchema](c)
		if err != nil {
			return err
		}
		snap, err := gofi.SnapshotRequest(c, gofi.SnapshotOptions{})
		if err != nil {
			return err
		}
		values = append(values, kept{bound: b.Request.Path.ID, copied: c.Copy().Param("id"), snap: snap.Params.Get("id")})
		return c.SendString(200, "ok")
	}})
	addr := startServer(t, s)

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(conn)
	for _, id := range []string{"AAAA", "BBBB"} {
		fmt.Fprintf(conn, "GET /u/%s HTTP/1.1\r\nHost: x\r\n\r\n", id)
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if got := values[0]; got.bound != "AAAA" || got.copied != "AAAA" || got.snap != "AAAA" {
		t.Fatalf("values from the first request changed on keep-alive: %+v", got)
	}
}

// F-30: a copied context reports the real peer and is usable as a Go context.
func TestSecurityCopiedContextIsUsable(t *testing.T) {
	s := gofi.NewFasthttpServer()
	result := make(chan string, 1)
	s.Router().Get("/c", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		cc := c.Copy()
		go func() {
			select {
			case <-cc.Request().Context().Done():
			default:
			}
			result <- cc.Request().RemoteAddr
		}()
		return c.SendString(200, "ok")
	}})
	addr := startServer(t, s)
	rawRequest(t, addr, get("/c"))
	select {
	case ip := <-result:
		if ip != "127.0.0.1" {
			t.Fatalf("copied context lost the peer address: %q", ip)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("copied context goroutine did not finish")
	}
}

// F-16: one header model across native and Writer() access.
func TestSecurityHeaderModel(t *testing.T) {
	run := func(t *testing.T, handler gofi.HandlerFunc, mws ...gofi.MiddlewareFunc) *gofi.InjectResponse {
		t.Helper()
		r := gofi.NewRouter()
		r.Use(mws...)
		r.Get("/h", gofi.RouteOptions{Handler: handler})
		resp, err := r.Test(gofi.TestOptions{Method: "GET", Path: "/h"})
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	t.Run("view sees native headers and replaces without duplicating", func(t *testing.T) {
		resp := run(t, func(c gofi.Context) error {
			c.Request().Context().Response.Header.Set("X-Frame-Options", "DENY")
			if got := c.Writer().Header().Get("X-Frame-Options"); got != "DENY" {
				return fmt.Errorf("view did not see native header: %q", got)
			}
			c.Writer().Header().Set("X-Frame-Options", "SAMEORIGIN")
			return c.SendString(200, "ok")
		})
		if v := resp.HeaderMap.Values("X-Frame-Options"); len(v) != 1 || v[0] != "SAMEORIGIN" {
			t.Fatalf("want one SAMEORIGIN value, got %v (body %q)", v, resp.BodyString())
		}
	})

	t.Run("delete through the view removes the native header", func(t *testing.T) {
		resp := run(t, func(c gofi.Context) error {
			c.Request().Context().Response.Header.Set("X-Debug", "1")
			c.Writer().Header().Del("X-Debug")
			return c.SendString(200, "ok")
		})
		if resp.HasHeader("X-Debug") {
			t.Fatalf("deleted header still sent: %v", resp.HeaderMap)
		}
	})

	t.Run("unchanged view keys do not overwrite later native writes", func(t *testing.T) {
		resp := run(t, func(c gofi.Context) error {
			c.Request().Context().Response.Header.Set("X-Version", "1")
			_ = c.Writer().Header()
			c.Request().Context().Response.Header.Set("X-Version", "2")
			return c.SendString(200, "ok")
		})
		if v := resp.HeaderMap.Values("X-Version"); len(v) != 1 || v[0] != "2" {
			t.Fatalf("stale view value overwrote native header: %v", v)
		}
	})

	t.Run("multiple cookies are preserved", func(t *testing.T) {
		resp := run(t, func(c gofi.Context) error {
			c.Request().Context().Response.Header.Add("Set-Cookie", "native=1")
			c.Writer().Header().Add("Set-Cookie", "a=1")
			c.Writer().Header().Add("Set-Cookie", "b=2")
			return c.SendString(200, "ok")
		})
		if n := len(resp.Cookies()); n != 3 {
			t.Fatalf("want 3 cookies, got %d: %v", n, resp.HeaderMap.Values("Set-Cookie"))
		}
	})

	t.Run("ETag survives a committed view", func(t *testing.T) {
		resp := run(t, func(c gofi.Context) error {
			c.Writer().WriteHeader(200)
			_, err := c.Writer().Write([]byte("body"))
			return err
		}, middleware.ETag())
		if resp.Header("ETag") == "" {
			t.Fatalf("ETag lost after the handler wrote through Writer(): %v", resp.HeaderMap)
		}
	})
}

// F-10: public errors are safe, valid JSON with the right content type.
func TestSecurityPublicErrors(t *testing.T) {
	r := gofi.NewRouter()
	r.Use(middleware.Recover(middleware.RecoverConfig{Output: discardLogger()}))
	routes := map[string]gofi.HandlerFunc{
		"/internal":  func(c gofi.Context) error { return errors.New("pq: password=hunter2 failed") },
		"/http5xx":   func(c gofi.Context) error { return gofi.NewHTTPError(503, "redis at 10.0.0.5 down") },
		"/client":    func(c gofi.Context) error { return gofi.NewHTTPError(409, "order already paid") },
		"/control":   func(c gofi.Context) error { return gofi.NewHTTPError(400, "bad \"x\"\x01\x1b[31m\xff") },
		"/badstatus": func(c gofi.Context) error { return gofi.NewHTTPError(200, "not an error code") },
		"/panic":     func(c gofi.Context) error { panic("hunter2") },
	}
	for path, h := range routes {
		r.Get(path, gofi.RouteOptions{Handler: h})
	}
	r.Post("/validate", gofi.RouteOptions{Schema: &nameSchema{}, Handler: func(c gofi.Context) error {
		_, err := gofi.ValidateAndBind[nameSchema](c)
		return err
	}})

	cases := []struct {
		path, method string
		status       int
		message      string
	}{
		{"/internal", "GET", 500, "Internal Server Error"},
		{"/http5xx", "GET", 503, "Service Unavailable"},
		{"/client", "GET", 409, "order already paid"},
		{"/control", "GET", 400, "bad \"x\"\x01\x1b[31m�"},
		{"/badstatus", "GET", 500, "Internal Server Error"},
		{"/panic", "GET", 500, "Internal Server Error"},
		{"/validate", "POST", 400, ""},
	}
	for _, tc := range cases {
		opts := gofi.TestOptions{Method: tc.method, Path: tc.path}
		if tc.method == "POST" {
			opts.Headers = map[string]string{"Content-Type": "application/json"}
			opts.Body = strings.NewReader(`{}`)
		}
		resp, _ := r.Test(opts)
		body := resp.BodyJSON()
		if resp.StatusCode != tc.status || body == nil || resp.Header("Content-Type") != "application/json" {
			t.Errorf("%s: want %d JSON, got %d %q (%q)", tc.path, tc.status, resp.StatusCode, resp.Header("Content-Type"), resp.BodyString())
			continue
		}
		if tc.message != "" && body["message"] != tc.message {
			t.Errorf("%s: message %q, want %q", tc.path, body["message"], tc.message)
		}
		if strings.Contains(resp.BodyString(), "hunter2") || strings.Contains(resp.BodyString(), "10.0.0.5") {
			t.Errorf("%s leaked an internal detail: %q", tc.path, resp.BodyString())
		}
	}
}

func discardLogger() *log.Logger { return log.New(io.Discard, "", 0) }

// F-03, F-30: deadlines reach cooperative handlers; Timeout keeps headers and
// bounds detached work.
func TestSecurityContextTimeout(t *testing.T) {
	ct, err := middleware.ContextTimeout(20 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	r := gofi.NewRouter()
	r.Use(ct)
	r.Get("/slow", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		if _, ok := c.Context().Deadline(); !ok {
			return errors.New("no deadline on c.Context()")
		}
		select {
		case <-c.Context().Done():
			return c.Context().Err()
		case <-time.After(2 * time.Second):
			return c.SendString(200, "too slow")
		}
	}})
	start := time.Now()
	resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/slow"})
	if resp.StatusCode != http.StatusServiceUnavailable || time.Since(start) > time.Second {
		t.Fatalf("want 503 at the deadline, got %d after %v (%q)", resp.StatusCode, time.Since(start), resp.BodyString())
	}
	if _, err := middleware.ContextTimeout(0); err == nil {
		t.Fatal("zero duration accepted")
	}
}

func TestSecurityTimeoutKeepsHeaders(t *testing.T) {
	r := gofi.NewRouter()
	r.Use(func(c gofi.Context) error {
		c.Request().Context().Response.Header.Set("X-Upstream-Native", "1")
		c.Writer().Header().Set("X-Upstream-View", "1")
		return c.Next()
	})
	r.Use(middleware.Timeout(middleware.TimeoutConfig{Timeout: time.Second}))
	r.Get("/t", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		c.Writer().Header().Set("X-Downstream-View", "1")
		return c.SendString(200, c.Request().RemoteAddr)
	}})
	resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/t"})
	for _, h := range []string{"X-Upstream-Native", "X-Upstream-View", "X-Downstream-View"} {
		if resp.Header(h) != "1" {
			t.Errorf("%s lost under Timeout: %v", h, resp.HeaderMap)
		}
	}
}

// F-01: body limits configured after construction reach the transport, and
// server options are validated.
func TestSecurityServerBodyLimitTiming(t *testing.T) {
	s := gofi.NewFasthttpServer()
	s.Router().Configure(gofi.Config{BodyLimit: 16})
	s.Router().Post("/p", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})
	addr := startServer(t, s)
	body := strings.Repeat("a", 4096)
	out := rawRequest(t, addr, fmt.Sprintf("POST /p HTTP/1.1\r\nHost: x\r\nConnection: close\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
	if !strings.HasPrefix(out, "HTTP/1.1 413") {
		t.Fatalf("Configure(BodyLimit) after construction ignored: %q", firstLine(out))
	}
}

func TestSecurityServerOptions(t *testing.T) {
	if _, err := gofi.NewFasthttpServerWithOptions(gofi.ServerOptions{ReadTimeout: -1}); err == nil {
		t.Fatal("negative ReadTimeout accepted")
	}
	opts := gofi.DefaultServerOptions()
	opts.MaxRequestBodySize = 32
	s, err := gofi.NewFasthttpServerWithOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	// Configure may not raise the ceiling set by ServerOptions.
	s.Router().Configure(gofi.Config{BodyLimit: 1 << 20})
	s.Router().Post("/p", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})
	addr := startServer(t, s)
	body := strings.Repeat("a", 64)
	out := rawRequest(t, addr, fmt.Sprintf("POST /p HTTP/1.1\r\nHost: x\r\nConnection: close\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
	if !strings.HasPrefix(out, "HTTP/1.1 413") {
		t.Fatalf("Configure raised the ServerOptions body ceiling: %q", firstLine(out))
	}
}

func firstLine(s string) string {
	if i := strings.Index(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// F-09, F-19: client identity starts at the socket peer; logs carry the final
// status and bounded fields.
func TestSecurityTrustPolicy(t *testing.T) {
	newServer := func(policy *gofi.TrustPolicy) string {
		s := gofi.NewFasthttpServer()
		if policy != nil {
			s.Router().Use(policy.Middleware())
		}
		s.Router().Get("/ip", gofi.RouteOptions{Handler: func(c gofi.Context) error {
			return c.SendString(200, gofi.ClientIP(c)+" "+gofi.Scheme(c))
		}})
		return startServer(t, s)
	}
	ask := func(addr string, headers string) string {
		out := rawRequest(t, addr, "GET /ip HTTP/1.1\r\nHost: x\r\nConnection: close\r\n"+headers+"\r\n")
		return out[strings.Index(out, "\r\n\r\n")+4:]
	}

	untrusted := newServer(nil)
	if got := ask(untrusted, "X-Forwarded-For: 6.6.6.6\r\nX-Forwarded-Proto: https\r\n"); got != "127.0.0.1 http" {
		t.Errorf("forwarding headers trusted without a policy: %q", got)
	}

	policy, err := gofi.NewTrustPolicy(gofi.TrustOptions{TrustedProxies: []string{"127.0.0.1", "10.0.0.0/8"}})
	if err != nil {
		t.Fatal(err)
	}
	trusted := newServer(policy)
	cases := map[string]string{
		"X-Forwarded-For: 203.0.113.7, 10.0.0.2\r\nX-Forwarded-Proto: https\r\n": "203.0.113.7 https",
		"X-Forwarded-For: 6.6.6.6, 203.0.113.7\r\n":                              "203.0.113.7 http",
		"X-Forwarded-For: 203.0.113.7, not-an-ip\r\n":                            "127.0.0.1 http",
		"": "127.0.0.1 http",
	}
	for headers, want := range cases {
		if got := ask(trusted, headers); got != want {
			t.Errorf("headers %q: got %q, want %q", headers, got, want)
		}
	}

	if _, err := gofi.NewTrustPolicy(gofi.TrustOptions{TrustedProxies: []string{"not-a-cidr"}}); err == nil {
		t.Error("invalid proxy accepted")
	}
}

func TestSecurityLoggerAndRequestID(t *testing.T) {
	var buf strings.Builder
	r := gofi.NewRouter()
	r.Use(middleware.Logger(middleware.LoggerConfig{Format: "${status} ${ip} ${path}\n", Output: log.New(&buf, "", 0)}))
	r.Use(middleware.RequestID())
	r.Get("/fail", gofi.RouteOptions{Handler: func(c gofi.Context) error { return gofi.NewHTTPError(503, "down") }})
	r.Get("/id", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})

	r.Test(gofi.TestOptions{Method: "GET", Path: "/fail", Headers: map[string]string{"X-Forwarded-For": "6.6.6.6"}})
	if line := buf.String(); !strings.HasPrefix(line, "503 ") || strings.Contains(line, "6.6.6.6") {
		t.Errorf("logger reported %q; want the final 503 and no forwarded IP", line)
	}

	resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/id", Headers: map[string]string{"X-Request-Id": "abc<script>"}})
	if id := resp.Header("X-Request-Id"); id == "abc<script>" || len(id) != 32 {
		t.Errorf("unsafe request ID accepted: %q", id)
	}
	resp, _ = r.Test(gofi.TestOptions{Method: "GET", Path: "/id", Headers: map[string]string{"X-Request-Id": "trace-123"}})
	if id := resp.Header("X-Request-Id"); id != "trace-123" {
		t.Errorf("safe request ID replaced: %q", id)
	}
}

// F-11: CORS varies on Origin, distinguishes preflights, validates config.
func TestSecurityCORS(t *testing.T) {
	r := gofi.NewRouter()
	r.Use(middleware.CORS(middleware.CORSConfig{AllowOrigins: "https://app.example"}))
	r.Get("/x", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})
	r.Options("/x", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "options handler") }})

	for _, origin := range []string{"https://app.example", "https://evil.example"} {
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/x", Headers: map[string]string{"Origin": origin}})
		if !strings.Contains(resp.Header("Vary"), "Origin") {
			t.Errorf("origin %s: missing Vary: Origin (%v)", origin, resp.HeaderMap)
		}
	}
	resp, _ := r.Test(gofi.TestOptions{Method: "OPTIONS", Path: "/x", Headers: map[string]string{"Origin": "https://app.example"}})
	if resp.BodyString() != "options handler" {
		t.Errorf("ordinary OPTIONS treated as a preflight: %d %q", resp.StatusCode, resp.BodyString())
	}

	if _, err := middleware.NewCORS(middleware.CORSConfig{AllowOrigins: "*", AllowCredentials: true}); err == nil {
		t.Error("wildcard origin with credentials accepted")
	}
	if _, err := middleware.NewCORS(middleware.CORSConfig{AllowOrigins: "app.example"}); err == nil {
		t.Error("origin without scheme accepted")
	}
	strict, err := middleware.NewCORS(middleware.CORSConfig{AllowOrigins: "https://app.example", AllowMethods: "GET"})
	if err != nil {
		t.Fatal(err)
	}
	r2 := gofi.NewRouter()
	r2.Use(strict)
	r2.Get("/x", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})
	resp, _ = r2.Test(gofi.TestOptions{Method: "OPTIONS", Path: "/x", Headers: map[string]string{
		"Origin": "https://app.example", "Access-Control-Request-Method": "DELETE", "Access-Control-Request-Headers": "X-Secret"}})
	if resp.HasHeader("Access-Control-Allow-Methods") || resp.HasHeader("Access-Control-Allow-Headers") {
		t.Errorf("validated CORS allowed an unlisted method or reflected headers: %v", resp.HeaderMap)
	}
}

// F-12: CSRF tokens are bound to a session, expire, and check Origin.
func TestSecuritySessionBoundCSRF(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	if _, err := middleware.CSRF(middleware.CSRFConfig{SigningKey: []byte("short"), SessionID: func(gofi.Context) string { return "" }}); err == nil {
		t.Fatal("short signing key accepted")
	}
	if _, err := middleware.CSRF(middleware.CSRFConfig{SigningKey: key}); err == nil {
		t.Fatal("missing SessionID accepted")
	}
	csrf, err := middleware.CSRF(middleware.CSRFConfig{
		SigningKey: key,
		SessionID:  func(c gofi.Context) string { return c.HeaderVal("X-Session") },
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gofi.NewRouter()
	r.Use(csrf)
	r.Get("/form", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "form") }})
	r.Post("/act", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "done") }})

	tokenFor := func(session string) string {
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/form", Headers: map[string]string{"X-Session": session}})
		return resp.CookieValue("csrf_")
	}
	post := func(session, token string, extra map[string]string) int {
		headers := map[string]string{"X-Session": session, "X-CSRF-Token": token}
		for k, v := range extra {
			headers[k] = v
		}
		resp, _ := r.Test(gofi.TestOptions{Method: "POST", Path: "/act", Headers: headers,
			Cookies: []http.Cookie{{Name: "csrf_", Value: token}}})
		return resp.StatusCode
	}

	alice := tokenFor("alice")
	if code := post("alice", alice, nil); code != 200 {
		t.Fatalf("valid token rejected: %d", code)
	}
	if code := post("mallory", alice, nil); code != http.StatusForbidden {
		t.Errorf("token from another session accepted: %d", code)
	}
	if code := post("alice", alice, map[string]string{"Origin": "https://evil.example"}); code != http.StatusForbidden {
		t.Errorf("cross-origin unsafe request accepted: %d", code)
	}
	if code := post("alice", "v1.1.abc.def", nil); code != http.StatusForbidden {
		t.Errorf("forged token accepted: %d", code)
	}
}

// F-13, F-28: Router.Static is confined to its root and streams files.
func TestSecurityRouterStaticConfinementAndStreaming(t *testing.T) {
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret.txt"), "outside secret")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "app.js"), "0123456789")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "escape.txt")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}

	s := gofi.NewFasthttpServer()
	s.Router().Static("/static", dir)
	addr := startServer(t, s)

	if status, body := httpGet(t, addr, "/static/escape.txt"); status == 200 || strings.Contains(body, "outside secret") {
		t.Fatalf("symlink escaped the static root: %d %q", status, body)
	}
	out := rawRequest(t, addr, "HEAD /static/app.js HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	if !strings.HasPrefix(out, "HTTP/1.1 200") || !strings.Contains(out, "Content-Length: 10") {
		t.Fatalf("HEAD not served: %q", out)
	}
	out = rawRequest(t, addr, "GET /static/app.js HTTP/1.1\r\nHost: x\r\nRange: bytes=2-4\r\nConnection: close\r\n\r\n")
	if !strings.HasPrefix(out, "HTTP/1.1 206") || !strings.HasSuffix(out, "234") {
		t.Fatalf("range request not honoured: %q", out)
	}
}

// F-24, F-31: registration ends when serving starts; handlers cannot write
// shared router state.
func TestSecuritySealingAndSharedState(t *testing.T) {
	r := gofi.NewRouter()
	child := r.Route("/api", nil)
	r.GlobalStore().Set("config", "v1")
	r.Get("/x", gofi.RouteOptions{Meta: "meta", Handler: func(c gofi.Context) error {
		if _, ok := c.GlobalStore().(gofi.GofiStore); ok {
			return errors.New("global store is writable from a handler")
		}
		if v, _ := c.GlobalStore().Get("config"); v != "v1" {
			return fmt.Errorf("global store read failed: %v", v)
		}
		all := r.Meta().All()
		all["/x"]["get"] = "changed"
		if v, _ := r.Meta().Route("/x", "GET"); v != "meta" {
			return fmt.Errorf("route metadata changed through All(): %v", v)
		}
		return c.SendString(200, "ok")
	}})
	_ = r.Handler()

	if resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/x"}); resp.StatusCode != 200 {
		t.Fatalf("shared state check failed: %q", resp.BodyString())
	}
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s after Handler() did not panic", name)
			}
		}()
		fn()
	}
	mustPanic("Get", func() { r.Get("/late", gofi.RouteOptions{Handler: func(c gofi.Context) error { return nil }}) })
	mustPanic("child Get", func() { child.Get("/late", gofi.RouteOptions{Handler: func(c gofi.Context) error { return nil }}) })
	mustPanic("Use", func() { r.Use(func(c gofi.Context) error { return c.Next() }) })
}

func TestSecurityInjectDoesNotReplaceLiveRules(t *testing.T) {
	r := gofi.NewRouter()
	r.Post("/n", gofi.RouteOptions{Schema: &nameSchema{}, Handler: func(c gofi.Context) error {
		if _, err := gofi.ValidateAndBind[nameSchema](c); err != nil {
			return err
		}
		return c.SendString(200, "ok")
	}})
	loose := gofi.RouteOptions{Schema: &struct{}{}, Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }}
	if _, err := r.Inject(gofi.InjectOptions{Method: "POST", Path: "/n", Handler: &loose}); err != nil {
		t.Fatal(err)
	}
	resp, _ := r.Test(gofi.TestOptions{Method: "POST", Path: "/n",
		Headers: map[string]string{"Content-Type": "application/json"}, Body: strings.NewReader(`{}`)})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Inject replaced the live route's validation: %d %q", resp.StatusCode, resp.BodyString())
	}
}

// F-14: global middleware covers trailing-slash redirects.
func TestSecurityRedirectRunsGlobalMiddleware(t *testing.T) {
	r := gofi.NewRouter()
	r.Use(func(c gofi.Context) error {
		c.Writer().Header().Set("X-Global", "1")
		if c.HeaderVal("Authorization") == "" {
			return gofi.NewHTTPError(http.StatusUnauthorized, "login required")
		}
		return c.Next()
	})
	r.Get("/users/", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "users") }})

	resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/users"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("redirect bypassed global middleware: %d %v", resp.StatusCode, resp.HeaderMap)
	}
	resp, _ = r.Test(gofi.TestOptions{Method: "GET", Path: "/users", Headers: map[string]string{"Authorization": "Bearer x"}})
	if resp.StatusCode != http.StatusMovedPermanently || resp.Header("X-Global") != "1" {
		t.Fatalf("authorized redirect: want 301 with global headers, got %d %v", resp.StatusCode, resp.HeaderMap)
	}
}

// F-13: wildcard rules match on segment boundaries, longest prefix first.
func TestSecurityRedirectRulesAreDeterministic(t *testing.T) {
	r := gofi.NewRouter()
	r.Use(middleware.Redirect(middleware.RedirectConfig{Rules: map[string]string{
		"/docs/*":     "/manual/$1",
		"/docs/api/*": "/reference/$1",
	}}))
	r.Get("/docsearch", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "search") }})

	for i := 0; i < 20; i++ {
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/docs/api/users"})
		if loc := resp.Header("Location"); loc != "/reference/users" {
			t.Fatalf("overlapping rules resolved to %q", loc)
		}
	}
	if resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/docsearch"}); resp.BodyString() != "search" {
		t.Fatalf("/docs/* matched /docsearch: %d %v", resp.StatusCode, resp.HeaderMap)
	}
}

type principalKey struct{}

// F-15: decisions made by standard middleware reach the gofi handler.
func TestSecurityWrapMiddlewarePropagatesContext(t *testing.T) {
	r := gofi.NewRouter()
	r.Use(gofi.WrapMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req.Header.Set("X-User", "alice")
			req.Header.Del("Authorization")
			next.ServeHTTP(w, req.WithContext(stdcontext.WithValue(req.Context(), principalKey{}, "alice")))
		})
	}))
	r.Get("/", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		principal, _ := c.Context().Value(principalKey{}).(string)
		return c.SendString(200, principal+"|"+c.HeaderVal("X-User")+"|"+c.HeaderVal("Authorization")+"|"+c.Request().Proto)
	}})
	resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/", Headers: map[string]string{"Authorization": "Bearer t"}})
	if resp.BodyString() != "alice|alice||HTTP/1.1" {
		t.Fatalf("bridge did not propagate context/headers: %q", resp.BodyString())
	}
}

// IP-15: admission control rejects predictably and bounds its bookkeeping.
func TestSecurityAdmissionControl(t *testing.T) {
	limit, err := middleware.ConcurrencyLimit(1)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	entered := make(chan struct{})
	r := gofi.NewRouter()
	r.Use(limit)
	r.Get("/slow", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		close(entered)
		<-release
		return c.SendString(200, "ok")
	}})
	done := make(chan int)
	go func() {
		resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/slow"})
		done <- resp.StatusCode
	}()
	<-entered
	if resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/slow"}); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("second concurrent request admitted: %d", resp.StatusCode)
	}
	close(release)
	if code := <-done; code != 200 {
		t.Errorf("first request failed: %d", code)
	}

	rl, err := middleware.RateLimit(middleware.RateLimitConfig{Requests: 2, Window: time.Minute, MaxKeys: 2,
		Key: func(c gofi.Context) string { return c.HeaderVal("X-Client") }})
	if err != nil {
		t.Fatal(err)
	}
	r2 := gofi.NewRouter()
	r2.Use(rl)
	r2.Get("/x", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})
	call := func(client string) *gofi.InjectResponse {
		resp, _ := r2.Test(gofi.TestOptions{Method: "GET", Path: "/x", Headers: map[string]string{"X-Client": client}})
		return resp
	}
	call("a")
	call("a")
	if resp := call("a"); resp.StatusCode != http.StatusTooManyRequests || resp.Header("Retry-After") == "" {
		t.Errorf("third request in window: want 429 with Retry-After, got %d %v", resp.StatusCode, resp.HeaderMap)
	}
	call("b")
	if resp := call("c"); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("key table grew past MaxKeys: %d", resp.StatusCode)
	}
	if _, err := middleware.RateLimit(middleware.RateLimitConfig{}); err == nil {
		t.Error("zero rate limit accepted")
	}
}

// F-22: documentation UIs load pinned assets with SRI and escape the spec path.
func TestSecurityDocsTemplates(t *testing.T) {
	hostile := `/q/openapi'"></redoc><script>alert(1)</script>`
	templates := map[string]gofi.DocsUiTemplate{
		"swagger":   gofi.SwaggerTemplate(),
		"scalar":    gofi.ScalarTemplate(nil),
		"redoc":     gofi.RedoclyTemplate(),
		"rapidoc":   gofi.RapidDoc(),
		"stoplight": gofi.StopLight(),
	}
	for name, tpl := range templates {
		page := string(tpl.HTML(hostile))
		if strings.Contains(page, "<script>alert(1)") {
			t.Errorf("%s: spec path broke out of its context", name)
		}
		if strings.Contains(page, "/latest/") || !strings.Contains(page, `integrity="sha384-`) {
			t.Errorf("%s: assets are not version-pinned with SRI", name)
		}
	}
}

// IP-06: structural limits and content-type policy.
func TestSecurityRequestStructureLimits(t *testing.T) {
	r := gofi.NewRouter()
	r.Post("/n", gofi.RouteOptions{Schema: &nameSchema{}, Handler: func(c gofi.Context) error {
		if _, err := gofi.ValidateAndBind[nameSchema](c); err != nil {
			return err
		}
		return c.SendString(200, "ok")
	}})
	post := func(contentType, body string, headers map[string]string) int {
		h := map[string]string{"Content-Type": contentType}
		for k, v := range headers {
			h[k] = v
		}
		resp, _ := r.Test(gofi.TestOptions{Method: "POST", Path: "/n", Headers: h, Body: strings.NewReader(body)})
		return resp.StatusCode
	}

	deep := `{"name":"a","x":` + strings.Repeat("[", 70) + strings.Repeat("]", 70) + `}`
	wide := `{"name":"a","x":[` + strings.TrimSuffix(strings.Repeat("1,", 10001), ",") + `]}`
	many := "name=a" + strings.Repeat("&k=v", 1000)
	cases := []struct {
		name, ct, body string
		headers        map[string]string
		want           int
	}{
		{"deep JSON", "application/json", deep, nil, http.StatusBadRequest},
		{"long array", "application/json", wide, nil, http.StatusBadRequest},
		{"many form values", "application/x-www-form-urlencoded", many, nil, http.StatusBadRequest},
		{"unsupported media type", "application/xml", "<a/>", nil, http.StatusUnsupportedMediaType},
		{"compressed body", "application/json", `{"name":"a"}`, map[string]string{"Content-Encoding": "gzip"}, http.StatusUnsupportedMediaType},
		{"within limits", "application/json", `{"name":"a","x":[1,2,3]}`, nil, 200},
	}
	for _, tc := range cases {
		if got := post(tc.ct, tc.body, tc.headers); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

// IP-08, IP-11: router options and request-target policy.
func TestSecurityRouterOptionsAndTargetPolicy(t *testing.T) {
	ok := gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, c.Param("id")) }}
	r := gofi.NewRouter()
	r.Get("/files/:id", ok)
	for _, path := range []string{"/files/a%2Fb", "/files/a%5Cb", "/files/a%00b", "/files/a%252Fb"} {
		if resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: path}); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d %q", path, resp.StatusCode, resp.BodyString())
		}
	}
	if resp, _ := r.Test(gofi.TestOptions{Method: "TRACE", Path: "/files/x"}); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("TRACE without a route: want 405, got %d", resp.StatusCode)
	}

	if _, err := gofi.NewRouterWithOptions(gofi.RouterOptions{Limits: gofi.RequestLimits{MaxJSONDepth: -1}}); err == nil {
		t.Fatal("negative limit accepted")
	}
	r2, err := gofi.NewRouterWithOptions(gofi.RouterOptions{AllowEncodedSeparators: true, Limits: gofi.RequestLimits{MaxJSONArrayLength: 2}})
	if err != nil {
		t.Fatal(err)
	}
	r2.Get("/files/:id", ok)
	r2.Trace("/files/:id", ok)
	r2.Post("/n", gofi.RouteOptions{Schema: &nameSchema{}, Handler: func(c gofi.Context) error {
		_, err := gofi.ValidateAndBind[nameSchema](c)
		return err
	}})
	// Allowed through the policy; the router then matches the decoded path
	// "/files/a/b", which is why the policy rejects it by default.
	if resp, _ := r2.Test(gofi.TestOptions{Method: "GET", Path: "/files/a%2Fb"}); resp.StatusCode == http.StatusBadRequest {
		t.Errorf("AllowEncodedSeparators ignored: %d", resp.StatusCode)
	}
	if resp, _ := r2.Test(gofi.TestOptions{Method: "TRACE", Path: "/files/x"}); resp.StatusCode != 200 {
		t.Errorf("registered TRACE route rejected: %d", resp.StatusCode)
	}
	resp, _ := r2.Test(gofi.TestOptions{Method: "POST", Path: "/n", Headers: map[string]string{"Content-Type": "application/json"},
		Body: strings.NewReader(`{"name":"a","x":[1,2,3]}`)})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("configured array limit ignored: %d", resp.StatusCode)
	}
}

// IP-09: the request context ends with the request; NewTimeout is validated
// and bounds detached work.
func TestSecurityRequestContextLifetime(t *testing.T) {
	r := gofi.NewRouter()
	got := make(chan stdcontext.Context, 1)
	r.Get("/c", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		if c.Context().Err() != nil {
			return errors.New("request context cancelled during the request")
		}
		got <- c.Context()
		return c.SendString(200, "ok")
	}})
	if resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/c"}); resp.StatusCode != 200 {
		t.Fatalf("request failed: %q", resp.BodyString())
	}
	select {
	case <-(<-got).Done():
	case <-time.After(time.Second):
		t.Fatal("request context not cancelled after the request finished")
	}
}

func TestSecurityNewTimeout(t *testing.T) {
	if _, err := middleware.NewTimeout(middleware.TimeoutOptions{Timeout: time.Second}); err == nil {
		t.Fatal("missing MaxInFlight accepted")
	}
	to, err := middleware.NewTimeout(middleware.TimeoutOptions{Timeout: 20 * time.Millisecond, MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	defer close(release)
	r := gofi.NewRouter()
	r.Use(to)
	r.Get("/stuck", gofi.RouteOptions{Handler: func(c gofi.Context) error { <-release; return nil }})
	first, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/stuck"})
	second, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/stuck"})
	if first.StatusCode != http.StatusServiceUnavailable || second.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503 for the timeout and for the full in-flight limit, got %d and %d", first.StatusCode, second.StatusCode)
	}
}

// IP-05: bounded streams.
func TestSecurityStreamOptions(t *testing.T) {
	s := gofi.NewFasthttpServer()
	errs := make(chan error, 2)
	s.Router().Get("/bytes", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		return gofi.Stream(c, gofi.StreamOptions{MaxBytes: 10, OnError: func(err error) { errs <- err }},
			func(ctx stdcontext.Context, w *bufio.Writer) error {
				for {
					if _, err := w.WriteString("0123456789"); err != nil {
						return err
					}
					if err := w.Flush(); err != nil {
						return err
					}
				}
			})
	}})
	s.Router().Get("/duration", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		return gofi.Stream(c, gofi.StreamOptions{MaxDuration: 50 * time.Millisecond, OnError: func(err error) { errs <- err }},
			func(ctx stdcontext.Context, w *bufio.Writer) error {
				<-ctx.Done()
				return ctx.Err()
			})
	}})
	addr := startServer(t, s)

	if _, body := httpGet(t, addr, "/bytes"); len(body) != 10 {
		t.Errorf("MaxBytes: want 10 bytes, got %d", len(body))
	}
	if err := <-errs; !errors.Is(err, gofi.ErrStreamLimit) {
		t.Errorf("MaxBytes: want ErrStreamLimit, got %v", err)
	}
	start := time.Now()
	httpGet(t, addr, "/duration")
	if err := <-errs; !errors.Is(err, stdcontext.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Errorf("MaxDuration: want deadline exceeded promptly, got %v after %v", err, time.Since(start))
	}
	if err := gofi.Stream(nil, gofi.StreamOptions{MaxBytes: -1}, nil); err == nil {
		t.Error("negative MaxBytes accepted")
	}
}

// IP-07: stampede control and bounded fills.
func TestSecurityNewCacheCoalescesAndBoundsFills(t *testing.T) {
	if _, err := middleware.NewCache(middleware.CacheOptions{MaxWaiters: -1}); err == nil {
		t.Fatal("negative option accepted")
	}
	cache, err := middleware.NewCache(middleware.CacheOptions{CoalesceWait: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	r := gofi.NewRouter()
	r.Use(cache)
	r.Get("/slow", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return c.SendString(200, "value")
	}})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if resp, _ := r.Test(gofi.TestOptions{Method: "GET", Path: "/slow"}); resp.BodyString() != "value" {
				t.Errorf("unexpected body %q", resp.BodyString())
			}
		}()
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Errorf("10 concurrent misses ran the handler %d times, want 1", n)
	}
}

// IP-10: RFC 7239 Forwarded and validated hosts.
func TestSecurityForwardedAndHosts(t *testing.T) {
	policy, err := gofi.NewTrustPolicy(gofi.TrustOptions{
		TrustedProxies: []string{"127.0.0.1"},
		UseForwarded:   true,
		AllowedHosts:   []string{"api.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := gofi.NewFasthttpServer()
	s.Router().Use(policy.Middleware())
	s.Router().Get("/w", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		return c.SendString(200, gofi.ClientIP(c)+" "+gofi.Scheme(c)+" "+gofi.Host(c))
	}})
	addr := startServer(t, s)
	ask := func(host, headers string) string {
		out := rawRequest(t, addr, "GET /w HTTP/1.1\r\nHost: "+host+"\r\nConnection: close\r\n"+headers+"\r\n")
		return firstLine(out) + " | " + out[strings.Index(out, "\r\n\r\n")+4:]
	}

	cases := []struct{ host, headers, want string }{
		{"internal:8080", `Forwarded: for="[2001:db8::1]:4711";proto=https;host=api.example.com` + "\r\n", "HTTP/1.1 200 OK | 2001:db8::1 https api.example.com"},
		{"api.example.com", "Forwarded: for=6.6.6.6, for=198.51.100.9\r\n", "HTTP/1.1 200 OK | 198.51.100.9 http api.example.com"},
		{"api.example.com", "X-Forwarded-For: 6.6.6.6\r\n", "HTTP/1.1 200 OK | 127.0.0.1 http api.example.com"},
		{"evil.example", "", "HTTP/1.1 421 Misdirected Request"},
		{"api.example.com", "Forwarded: host=\"bad host\"\r\n", "HTTP/1.1 421 Misdirected Request"},
	}
	for _, tc := range cases {
		if got := ask(tc.host, tc.headers); !strings.HasPrefix(got, tc.want) {
			t.Errorf("Host %q, %q: got %q, want prefix %q", tc.host, tc.headers, got, tc.want)
		}
	}
}
