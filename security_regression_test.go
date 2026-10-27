package gofi_test

// Security regressions for the findings in gofi-docs/security-audit-2.md.
// Each test names the finding it covers. Panic cases run in a subprocess so a
// regression that terminates the process is reported as a test failure.

import (
	"bufio"
	stdcontext "context"
	"errors"
	"io"
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
