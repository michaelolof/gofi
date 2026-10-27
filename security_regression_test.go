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

		if out := rawRequest(t, addr, get("/static/%zz")); !strings.HasPrefix(out, "HTTP/1.1 400") {
			t.Fatalf("malformed escape: want 400, got %q", out)
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
		addr := startServer(t, s)

		out := rawRequest(t, addr, get("/panic"))
		if !strings.HasPrefix(out, "HTTP/1.1 500") || strings.Contains(out, "secret-db-password") {
			t.Fatalf("handler panic: want generic 500, got %q", out)
		}
		if out := rawRequest(t, addr, get("/ok")); !strings.HasSuffix(out, "ok") {
			t.Fatalf("server did not recover after handler panic: %q", out)
		}

		r.UseErrorHandler(func(err error, c gofi.Context) { panic("error handler failed") })
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
		r := newRouter(middleware.CacheConfig{}, echo)
		r.Test(gofi.TestOptions{Method: "GET", Path: "/r"})
		// Test always sends Host: localhost; a second Host header overrides it.
		if got := req(r, "", map[string]string{"Host": "tenant-b"}); !strings.HasPrefix(got, "tenant-b|") {
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
