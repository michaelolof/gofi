package gofi

import (
	"bufio"
	stdcontext "context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

type transportReadyListener struct {
	net.Listener
	ready chan struct{}
	once  sync.Once
}

func (ln *transportReadyListener) Accept() (net.Conn, error) {
	ln.once.Do(func() { close(ln.ready) })
	return ln.Listener.Accept()
}

// Pre-bind the actual listener: no port reservation or startup polling is needed.
// All requests and shutdowns have deadlines, including TLS handshakes.
func startTransportServer(t *testing.T, s *FasthttpServer, tlsConfig *tls.Config) string {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if tlsConfig != nil {
		ln = tls.NewListener(ln, tlsConfig)
	}
	ready := &transportReadyListener{Listener: ln, ready: make(chan struct{})}
	s.server.Logger = &dummyLogger{}
	done := make(chan error, 1)
	go func() { done <- s.ListenOnListener(ready) }()
	t.Cleanup(func() {
		defer ln.Close()
		ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 3*time.Second)
		defer cancel()
		if err := s.ShutdownWithContext(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, net.ErrClosed) {
				t.Errorf("serve: %v", err)
			}
		case <-ctx.Done():
			t.Error("server did not stop before deadline")
		}
	})
	select {
	case <-ready.ready:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not start accepting before deadline")
	}
	return addr
}

func transportRequest(t *testing.T, addr, raw string, tlsConfig *tls.Config) (*http.Response, string) {
	t.Helper()
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	var conn net.Conn
	var err error
	if tlsConfig == nil {
		conn, err = dialer.Dial("tcp4", addr)
	} else {
		conn, err = tls.DialWithDialer(dialer, "tcp4", addr, tlsConfig)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(conn, raw); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

func transportTLSFixture(t *testing.T) (server, client *tls.Config, certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:        true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(certPEM)
	dir := t.TempDir()
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{pair}, ClientCAs: roots},
		&tls.Config{RootCAs: roots, Certificates: []tls.Certificate{pair}}, certFile, keyFile
}

const transportValidRequest = "POST /items/fresh HTTP/1.1\r\nHost: localhost\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"

func TestNativeTransportErrors(t *testing.T) {
	const head = "POST /items/private HTTP/1.1\r\nHost: localhost\r\n"
	cases := []struct {
		name string
		raw  string
		code int
	}{
		{"fixed-length-early", head + "Content-Length: 33\r\n\r\n", 413},
		{"fixed-length-body", head + "Content-Length: 33\r\n\r\n" + strings.Repeat("s", 33), 413},
		{"chunked-body", head + "Transfer-Encoding: chunked\r\n\r\n21\r\n" + strings.Repeat("s", 33) + "\r\n0\r\n\r\n", 413},
		{"malformed-header", head + "Authorization secret-without-colon\r\n\r\n", 400},
		{"oversized-header", head + "X-Secret: " + strings.Repeat("s", 1024) + "\r\n\r\n", 431},
		{"read-timeout", head + "Content-Length: 1\r\n\r\n", 408},
	}
	serverTLS, clientTLS, _, _ := transportTLSFixture(t)
	for _, mode := range []string{"tcp", "tls", "mutual-tls"} {
		t.Run(mode, func(t *testing.T) {
			m := newRouter()
			m.Configure(Config{BodyLimit: 32})
			var middleware, routes atomic.Int32
			m.Use(func(c Context) error { middleware.Add(1); return c.Next() })
			m.Post("/items/:id", RouteOptions{Handler: func(c Context) error {
				routes.Add(1)
				return c.SendString(200, "ok")
			}})
			s := m.newFasthttpServer()
			s.server.ReadBufferSize = 512
			s.server.ReadTimeout = 250 * time.Millisecond
			var serverConfig, clientConfig *tls.Config
			if mode != "tcp" {
				serverConfig, clientConfig = serverTLS.Clone(), clientTLS.Clone()
				if mode == "mutual-tls" {
					serverConfig.ClientAuth = tls.RequireAndVerifyClientCert
				}
			}
			addr := startTransportServer(t, s, serverConfig)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					resp, body := transportRequest(t, addr, tc.raw, clientConfig)
					if resp.StatusCode != tc.code || resp.Header.Get("Content-Type") != "application/json" {
						t.Fatalf("status=%d type=%q body=%q", resp.StatusCode, resp.Header.Get("Content-Type"), body)
					}
					var envelope struct {
						Status  string `json:"status"`
						Code    int    `json:"statusCode"`
						Message string `json:"message"`
					}
					if err := json.Unmarshal([]byte(body), &envelope); err != nil {
						t.Fatal(err)
					}
					if envelope.Status != "error" || envelope.Code != tc.code || envelope.Message != http.StatusText(tc.code) {
						t.Errorf("unexpected envelope: %+v", envelope)
					}
					if middleware.Load() != 0 || routes.Load() != 0 {
						t.Error("transport rejection dispatched the application chain")
					}
					t.Logf("status=%d content-type=%q body=%s", resp.StatusCode, resp.Header.Get("Content-Type"), body)
				})
			}
			resp, body := transportRequest(t, addr, transportValidRequest, clientConfig)
			if resp.StatusCode != 200 || body != "ok" || middleware.Load() != 1 || routes.Load() != 1 {
				t.Errorf("valid request: status=%d body=%q middleware=%d routes=%d", resp.StatusCode, body, middleware.Load(), routes.Load())
			}
		})
	}
}

func TestNativeTransportCustomInterceptor(t *testing.T) {
	for _, writerPath := range []bool{false, true} {
		t.Run(fmt.Sprintf("writer=%t", writerPath), func(t *testing.T) {
			s := NewFasthttpServer()
			m := s.mux
			StoreSet(m.GlobalStore(), "app", "global-value")
			var middleware, routes, intercepted atomic.Int32
			m.Use(func(c Context) error {
				middleware.Add(1)
				if StoreHas(c.DataStore(), "failed") {
					t.Error("failed-request store leaked into middleware")
				}
				StoreSet(c.DataStore(), "middleware", true)
				return c.Next()
			})
			normalErr := NewHTTPError(409, "normal handler error")
			m.Post("/items/:id", RouteOptions{Meta: "route-meta", Handler: func(c Context) error {
				routes.Add(1)
				if c.Param("id") != "fresh" || c.Request().Pattern != "/items/:id" {
					t.Error("valid request has incorrect route state")
				}
				// Dirty the pooled adapters and datastore before another failure.
				c.Writer().Header().Set("X-Valid", "yes")
				return c.SendString(200, "ok")
			}})
			m.Post("/error", RouteOptions{Handler: func(c Context) error { return normalErr }})
			// Registered after server construction: the bridge must not capture the default.
			m.UseErrorHandler(func(err error, c Context) {
				intercepted.Add(1)
				var httpErr *HTTPError
				if !errors.As(err, &httpErr) {
					t.Error("missing HTTPError")
				} else if errors.Is(err, normalErr) {
					if httpErr.Code != 409 || !StoreHas(c.DataStore(), "middleware") {
						t.Error("ordinary error handling changed")
					}
				} else {
					if httpErr.Code != 413 || !errors.Is(err, fasthttp.ErrBodyTooLarge) {
						t.Errorf("lost status or original body error: %v", err)
					}
					if c.Pattern() != "" || c.Request().Pattern != "" || c.Param("id") != "" {
						t.Error("transport error has route state")
					}
					if _, ok := c.Meta().This(); ok {
						t.Error("transport error has route metadata")
					}
					if StoreHas(c.DataStore(), "middleware") || StoreHas(c.DataStore(), "failed") {
						t.Error("stale request datastore")
					}
					if c.HasResponseHeader("X-Valid") || c.HasResponseHeader("X-Intercepted") {
						t.Error("stale response headers")
					}
					if c.Context() == nil || c.Request().ctx != c.(*context).fctx || c.(*context).serverOpts != m.opts {
						t.Error("missing underlying context or server options")
					}
					StoreSet(c.DataStore(), "failed", true)
					// Next has no chain to execute on a transport rejection.
					if err := c.Next(); err != nil {
						t.Errorf("unexpected Next error: %v", err)
					}
				}
				if v, ok := StoreGet(c.GlobalStore(), "app"); !ok || v != "global-value" {
					t.Error("router global store unavailable")
				}
				c.Writer().Header().Set("X-Intercepted", "once")
				c.Writer().Header().Set("Content-Type", "application/problem+json")
				if writerPath {
					c.Writer().WriteHeader(422)
					_, _ = io.WriteString(c.Writer(), `{"custom":true}`)
				} else {
					_ = c.SendBytes(422, []byte(`{"custom":true}`))
				}
			})
			addr := startTransportServer(t, s, nil)
			for i := 0; i < 3; i++ {
				before := intercepted.Load()
				resp, body := transportRequest(t, addr, "POST /items/private HTTP/1.1\r\nHost: localhost\r\nContent-Length: 4194305\r\n\r\n", nil)
				if resp.StatusCode != 422 || body != `{"custom":true}` || resp.Header.Get("Content-Type") != "application/problem+json" || len(resp.Header.Values("X-Intercepted")) != 1 {
					t.Errorf("custom response: status=%d headers=%v body=%q", resp.StatusCode, resp.Header, body)
				}
				if intercepted.Load() != before+1 || middleware.Load() != int32(i) || routes.Load() != int32(i) {
					t.Error("incorrect interceptor or application invocation count")
				}
				resp, body = transportRequest(t, addr, transportValidRequest, nil)
				if resp.StatusCode != 200 || body != "ok" || resp.Header.Get("X-Intercepted") != "" || resp.Header.Get("X-Valid") != "yes" {
					t.Errorf("subsequent valid request: status=%d headers=%v body=%q", resp.StatusCode, resp.Header, body)
				}
			}
			resp, body := transportRequest(t, addr, "POST /error HTTP/1.1\r\nHost: localhost\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", nil)
			if resp.StatusCode != 422 || body != `{"custom":true}` || intercepted.Load() != 4 || middleware.Load() != 4 || routes.Load() != 3 {
				t.Error("ordinary handler error did not reach the interceptor once")
			}
		})
	}
}

func TestTransportErrorClassificationAndReset(t *testing.T) {
	small := &fasthttp.ErrSmallBuffer{}
	timeout := &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}
	parse := errors.New("secret parser diagnostic")
	for _, tc := range []struct {
		name  string
		cause error
		code  int
	}{
		{"body", fasthttp.ErrBodyTooLarge, 413},
		{"header", small, 431},
		{"timeout", timeout, 408},
		{"parse", parse, 400},
		{"non-timeout-network-error", &net.OpError{Op: "read", Err: io.ErrUnexpectedEOF}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newRouter()
			var calls, dispatched int
			// A fresh dirty fixture on every pool miss makes reset assertions
			// deterministic even when sync.Pool discards entries (e.g. under -race).
			m.ctxPool = &sync.Pool{New: func() any {
				old := new(fasthttp.RequestCtx)
				return &context{
					fctx: old, opts: newContextOptions("/old", "POST"),
					routeMeta:  metaMap{"/old": {"post": "old"}},
					serverOpts: m.opts, dataStore: newDataStore(),
					params:     Params{{Key: "id", Value: "old"}},
					handlers:   []HandlerFunc{func(c Context) error { dispatched++; return nil }},
					handlerIdx: -1, bindedCacheResult: bindedResult{bound: true},
					req: newRequest(old, "/old"), rw: newResponseWriter(old),
				}
			}}
			var fctx fasthttp.RequestCtx
			wrapped := fmt.Errorf("private read details: %w", tc.cause)
			m.UseErrorHandler(func(err error, c Context) {
				calls++
				var httpErr *HTTPError
				if !errors.As(err, &httpErr) || httpErr.Code != tc.code || err.Error() != http.StatusText(tc.code) || !errors.Is(err, wrapped) || !errors.Is(err, tc.cause) {
					t.Errorf("incorrect classified error: %v", err)
				}
				if tc.cause == small {
					var original *fasthttp.ErrSmallBuffer
					if !errors.As(err, &original) || original != small {
						t.Error("lost original small-buffer error")
					}
				}
				if tc.cause == timeout {
					var original *net.OpError
					if !errors.As(err, &original) || original != timeout {
						t.Error("lost original timeout error")
					}
				}
				cc := c.(*context)
				if cc.fctx != &fctx || cc.serverOpts != m.opts || cc.globalStore != m.globalStore || cc.bindedCacheResult.bound || cc.req != nil || cc.dataStore != nil || len(cc.params) != 0 || cc.routeMeta != nil || cc.opts != (contextOptions{}) {
					t.Error("pooled context state was not reset and initialized")
				}
				_ = c.Next()
			})
			m.handleTransportError(&fctx, wrapped)
			if calls != 1 || dispatched != 0 {
				t.Errorf("interceptor=%d dispatched=%d", calls, dispatched)
			}
		})
	}
}

func TestNativeListenerConstructionWiring(t *testing.T) {
	_, _, cert, key := transportTLSFixture(t)
	for _, mode := range []string{"http", "tls", "mutual-tls"} {
		t.Run(mode, func(t *testing.T) {
			m := newRouter()
			m.Configure(Config{BodyLimit: 32})
			// Fail binding deterministically, then inspect the constructed server.
			// Transport behavior itself is exercised on pre-bound TCP/TLS listeners above.
			var err error
			switch mode {
			case "http":
				err = m.Listen("127.0.0.1:invalid-port")
			case "tls":
				err = m.ListenTLS("127.0.0.1:invalid-port", cert, key)
			case "mutual-tls":
				err = m.ListenTLSMutual("127.0.0.1:invalid-port", cert, key, cert)
			}
			if err == nil {
				t.Fatal("expected invalid address to fail")
			}
			s := m.lifecycle.server()
			if s == nil || s.server.ErrorHandler == nil || s.server.MaxRequestBodySize != 32 {
				t.Fatal("native listener is missing shared server configuration")
			}
			var intercepted int
			m.UseErrorHandler(func(err error, c Context) { intercepted++ })
			s.server.ErrorHandler(new(fasthttp.RequestCtx), fasthttp.ErrBodyTooLarge)
			if intercepted != 1 {
				t.Error("native listener did not use the current interceptor")
			}
		})
	}
}
