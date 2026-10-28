package gofi_test

// Reverse-proxy integration for IP-20 in gofi-docs/security-audit-2.md. The
// net/http proxy tests always run and cover HTTP/2-to-HTTP/1.1 translation.
// The nginx tests run a pinned nginx image in Docker against a live gofi
// server and are opt-in: set GOFI_PROXY_IT=1.

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/michaelolof/gofi"
)

// nginxImage pins nginx 1.28.0-alpine by digest.
const nginxImage = "nginx@sha256:30f1c0d78e0ad60901648be663a710bdadf19e4c10ac6782c235200619158284"

// proxiedOrigin is a gofi server behind a trusted proxy. Its catch-all route
// reports what gofi resolved for the request; /reject answers 401 without
// reading the body.
type proxiedOrigin struct {
	addr string
	mu   sync.Mutex
	seen []string
}

func (o *proxiedOrigin) handled() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.seen...)
}

func startProxiedOrigin(t *testing.T, trusted []string, allowedHosts []string, opts gofi.ServerOptions) *proxiedOrigin {
	t.Helper()
	o := &proxiedOrigin{}
	s := gofi.NewFasthttpServer(opts)
	s.Router().Use(must(gofi.TrustPolicy(gofi.TrustOptions{TrustedProxies: trusted, AllowedHosts: allowedHosts})))
	handle := func(c gofi.Context) error {
		path := strings.Clone(c.Request().URL.Path()) // the path is borrowed
		o.mu.Lock()
		o.seen = append(o.seen, path)
		o.mu.Unlock()
		if path == "/reject" {
			return c.SendString(http.StatusUnauthorized, "rejected")
		}
		body, _ := io.ReadAll(c.Request().Body)
		fctx := c.Request().Context()
		return c.SendString(200, fmt.Sprintf("path=%s proto=%s ip=%s scheme=%s host=%s body=%d",
			path, fctx.Request.Header.Protocol(), gofi.ClientIP(c), gofi.Scheme(c), gofi.Host(c), len(body)))
	}
	for _, register := range []func(string, gofi.RouteOptions){s.Router().Get, s.Router().Post} {
		register("/*path", gofi.RouteOptions{Handler: handle})
	}
	o.addr = startServer(t, s)
	return o
}

// startH2Proxy fronts origin with net/http's reverse proxy over TLS, with
// HTTP/2 enabled. Rewrite with SetXForwarded replaces any client-supplied
// X-Forwarded-* headers.
func startH2Proxy(t *testing.T, origin string) *httptest.Server {
	t.Helper()
	target, _ := url.Parse("http://" + origin)
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		r.SetXForwarded()
	}}
	srv := httptest.NewUnstartedServer(proxy)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func TestSecurityProxyHTTP2ToHTTP1(t *testing.T) {
	opts := gofi.DefaultServerOptions()
	opts.MaxRequestBodySize = 1 << 10
	origin := startProxiedOrigin(t, []string{"127.0.0.1"}, []string{"127.0.0.1", "app.example"}, opts)
	proxy := startH2Proxy(t, origin.addr)
	client := proxy.Client()

	do := func(method, host string, header http.Header, body []byte) (*http.Response, string) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			// No length: HTTP/2 sends DATA frames without Content-Length.
			rd = io.MultiReader(bytes.NewReader(body))
		}
		req, _ := http.NewRequest(method, proxy.URL+"/x", rd)
		if host != "" {
			req.Host = host
		}
		for k, v := range header {
			req.Header[k] = v
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}

	resp, got := do("GET", "app.example", http.Header{
		"X-Forwarded-For":   {"6.6.6.6"},
		"X-Forwarded-Proto": {"http"},
		"X-Forwarded-Host":  {"evil.example"},
	}, nil)
	if resp.Proto != "HTTP/2.0" {
		t.Fatalf("client did not use HTTP/2: %s", resp.Proto)
	}
	if want := "path=/x proto=HTTP/1.1 ip=127.0.0.1 scheme=https host=app.example body=0"; got != want {
		t.Fatalf("client-supplied forwarding headers reached gofi:\n got %q\nwant %q", got, want)
	}

	if resp, got := do("GET", "evil.example", nil, nil); resp.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("host outside AllowedHosts through the proxy: want 421, got %d %q", resp.StatusCode, got)
	}

	if _, got := do("POST", "app.example", nil, bytes.Repeat([]byte("a"), 512)); !strings.HasSuffix(got, "body=512") {
		t.Fatalf("streamed HTTP/2 body was not delivered intact: %q", got)
	}
	if resp, _ := do("POST", "app.example", nil, bytes.Repeat([]byte("a"), 4<<10)); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized HTTP/2 body through the proxy: want 413, got %d", resp.StatusCode)
	}
	if resp, _ := do("GET", "app.example", nil, nil); resp.StatusCode != 200 {
		t.Fatalf("proxy connection unusable after a 413: %d", resp.StatusCode)
	}
}

// Direct requests that bypass the proxy cannot claim another client identity
// when the origin trusts only the proxy's address.
func TestSecurityProxyDirectOriginIgnoresForwarding(t *testing.T) {
	origin := startProxiedOrigin(t, []string{"10.0.0.1"}, nil, gofi.DefaultServerOptions())
	out := exchange(t, origin.addr, "GET /x HTTP/1.1\r\nHost: app.example\r\nX-Forwarded-For: 6.6.6.6\r\nX-Forwarded-Proto: https\r\nX-Forwarded-Host: other.example\r\nConnection: close\r\n\r\n")
	if !strings.HasSuffix(out, "ip=127.0.0.1 scheme=http host=app.example body=0") {
		t.Fatalf("forwarding headers trusted from a non-proxy peer: %q", out)
	}
}

// nginxProxy runs nginx in Docker in front of origin and returns its address.
func nginxProxy(t *testing.T, originAddr string) string {
	t.Helper()
	if os.Getenv("GOFI_PROXY_IT") != "1" {
		t.Skip("set GOFI_PROXY_IT=1 to run the nginx integration tests (requires Docker)")
	}
	_, port, _ := net.SplitHostPort(originAddr)
	conf := fmt.Sprintf(`events {}
http {
  access_log off;
  upstream origin {
    server host.docker.internal:%s;
    keepalive 4;
  }
  server {
    listen 8080;
    client_max_body_size 1m;
    location / {
      proxy_pass http://origin;
      proxy_http_version 1.1;
      proxy_set_header Connection "";
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-For $remote_addr;
      proxy_set_header X-Forwarded-Proto $scheme;
      proxy_set_header X-Forwarded-Host $host;
    }
  }
}
`, port)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nginx.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"--add-host=host.docker.internal:host-gateway",
		"-p", "127.0.0.1::8080",
		"-v", filepath.Join(dir, "nginx.conf")+":/etc/nginx/nginx.conf:ro",
		nginxImage).CombinedOutput()
	if err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", id).Run() })

	out, err = exec.Command("docker", "port", id, "8080/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	addr := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	deadline := time.Now().Add(15 * time.Second)
	ready, _ := http.NewRequest("GET", "http://"+addr+"/ready", nil)
	ready.Host = "app.example"
	for {
		status := 0
		resp, err := http.DefaultClient.Do(ready)
		if err == nil {
			resp.Body.Close()
			if status = resp.StatusCode; status == 200 {
				return addr
			}
		}
		if time.Now().After(deadline) {
			logs, _ := exec.Command("docker", "logs", id).CombinedOutput()
			t.Fatalf("nginx did not proxy to gofi (status %d, error %v):\n%s", status, err, logs)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func countSeen(seen []string, path string) int {
	n := 0
	for _, p := range seen {
		if p == path {
			n++
		}
	}
	return n
}

// Docker Desktop and Linux bridge networks reach the host from private
// addresses; the origin trusts those as the proxy.
var dockerNetworks = []string{"127.0.0.1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

func TestSecurityProxyNginx(t *testing.T) {
	opts := gofi.DefaultServerOptions()
	opts.MaxRequestBodySize = 1 << 10
	origin := startProxiedOrigin(t, dockerNetworks, []string{"app.example"}, opts)
	addr := nginxProxy(t, origin.addr)
	host := "Host: app.example\r\n"

	t.Run("forwarding headers", func(t *testing.T) {
		out := exchange(t, addr, "GET /x HTTP/1.1\r\n"+host+"X-Forwarded-For: 6.6.6.6\r\nX-Forwarded-Proto: https\r\nConnection: close\r\n\r\n")
		if !strings.HasPrefix(out, "HTTP/1.1 200 ") || strings.Contains(out, "6.6.6.6") || !strings.Contains(out, "scheme=http host=app.example") {
			t.Fatalf("client-supplied forwarding headers reached gofi: %q", out)
		}
		if out := exchange(t, addr, "GET /x HTTP/1.1\r\nHost: evil.example\r\nConnection: close\r\n\r\n"); !strings.HasPrefix(out, "HTTP/1.1 421 ") {
			t.Fatalf("host outside AllowedHosts through nginx: %q", firstLine(out))
		}
	})

	// A request is smuggled when gofi handles it but nginx never returned its
	// response to the client that sent it. nginx may read bytes after a body
	// as a pipelined request of its own (it accepts bare-LF chunk framing);
	// that is not smuggling, because nginx forwards and answers it.
	t.Run("ambiguous framing", func(t *testing.T) {
		for name, raw := range map[string]string{
			"CL and TE":       "POST /x HTTP/1.1\r\n" + host + "Content-Length: 4\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\nGET /smuggled HTTP/1.1\r\n" + host + "\r\n",
			"duplicate CL":    "POST /x HTTP/1.1\r\n" + host + "Content-Length: 3\r\nContent-Length: 40\r\n\r\nabcGET /smuggled HTTP/1.1\r\n" + host + "\r\n",
			"obfuscated TE":   "POST /x HTTP/1.1\r\n" + host + "Transfer-Encoding: xchunked\r\nContent-Length: 4\r\n\r\n0\r\n\r\nGET /smuggled HTTP/1.1\r\n" + host + "\r\n",
			"bare LF chunked": "POST /x HTTP/1.1\r\n" + host + "Transfer-Encoding: chunked\r\n\r\n3\nabc\n0\n\nGET /smuggled HTTP/1.1\r\n" + host + "\r\n",
		} {
			before := countSeen(origin.handled(), "/smuggled")
			out := exchange(t, addr, raw)
			handled := countSeen(origin.handled(), "/smuggled") - before
			if answered := strings.Count(out, "path=/smuggled "); handled > answered {
				t.Fatalf("%s: gofi handled /smuggled %d times but the client got %d responses", name, handled, answered)
			}
		}
	})

	t.Run("chunked body", func(t *testing.T) {
		out := exchange(t, addr, "POST /x HTTP/1.1\r\n"+host+"Transfer-Encoding: chunked\r\nConnection: close\r\n\r\n3\r\nabc\r\n2\r\nde\r\n0\r\n\r\n")
		if !strings.HasSuffix(out, "body=5") {
			t.Fatalf("chunked body not delivered intact: %q", out)
		}
	})

	// nginx reuses upstream connections. Rejected and oversized bodies must
	// leave each connection framed for the next request.
	t.Run("upstream keep-alive stays framed", func(t *testing.T) {
		before := countSeen(origin.handled(), "/smuggled")
		hidden := "GET /smuggled HTTP/1.1\r\n" + host + "\r\n"
		big := strings.Repeat("GET /smuggled HTTP/1.1\r\n"+host+"\r\n", 64)
		for i := 0; i < 20; i++ {
			out := exchange(t, addr, fmt.Sprintf("POST /reject HTTP/1.1\r\n%sContent-Length: %d\r\nConnection: close\r\n\r\n%s", host, len(hidden), hidden))
			if !strings.HasPrefix(out, "HTTP/1.1 401 ") {
				t.Fatalf("round %d: want 401, got %q", i, firstLine(out))
			}
			out = exchange(t, addr, fmt.Sprintf("POST /x HTTP/1.1\r\n%sContent-Length: %d\r\nConnection: close\r\n\r\n%s", host, len(big), big))
			if !strings.HasPrefix(out, "HTTP/1.1 413 ") {
				t.Fatalf("round %d: want 413, got %q", i, firstLine(out))
			}
			out = exchange(t, addr, fmt.Sprintf("GET /ok%d HTTP/1.1\r\n%sConnection: close\r\n\r\n", i, host))
			if !strings.Contains(out, fmt.Sprintf("path=/ok%d ", i)) {
				t.Fatalf("round %d: response desynchronised: %q", i, out)
			}
		}
		if countSeen(origin.handled(), "/smuggled") > before {
			t.Fatalf("a request hidden in a body reached gofi: %q", origin.handled())
		}
	})

	t.Run("pipelined", func(t *testing.T) {
		out := exchange(t, addr,
			"GET /p1 HTTP/1.1\r\n"+host+"\r\n"+
				"GET /p2 HTTP/1.1\r\n"+host+"\r\n"+
				"GET /p3 HTTP/1.1\r\n"+host+"Connection: close\r\n\r\n")
		resps := readResponses(t, out, "GET", "GET", "GET")
		for i, resp := range resps {
			if want := fmt.Sprintf("path=/p%d ", i+1); !strings.HasPrefix(bodyOf(resp), want) {
				t.Fatalf("pipelined response %d out of order: %q", i+1, bodyOf(resp))
			}
		}
	})
}
