package gofi

import (
	stdcontext "context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/valyala/fasthttp"
)

// FasthttpServer wraps a serveMux and drives it via fasthttp.Server,
// giving connection-level performance (worker pool, keepalive, zero-copy TCP)
// while preserving the existing handler chain unchanged.
type FasthttpServer struct {
	mux    *serveMux
	server *fasthttp.Server
}

// NewFasthttpServer creates a FasthttpServer backed by a new serveMux.
func NewFasthttpServer() *FasthttpServer {
	m := newRouter()
	return &FasthttpServer{
		mux: m,
		server: &fasthttp.Server{
			Handler:            m.handleFastHTTP,
			Name:               "gofi",
			DisableKeepalive:   false,
			ReduceMemoryUsage:  false,
			MaxRequestBodySize: 4 * 1024 * 1024, // 4 MB default
		},
	}
}

// Router returns the underlying Router so callers can register routes.
func (s *FasthttpServer) Router() Router {
	return s.mux
}

// Listen starts the fasthttp server on the given address (e.g. ":4000").
func (s *FasthttpServer) Listen(addr string) error {
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return err
	}
	return s.server.Serve(ln)
}

// ListenTLS starts the fasthttp server with TLS encryption.
func (s *FasthttpServer) ListenTLS(addr, certFile, keyFile string) error {
	return s.server.ListenAndServeTLS(listenAddr(addr), certFile, keyFile)
}

// ListenTLSMutual starts the fasthttp server with mutual TLS (mTLS) authentication.
func (s *FasthttpServer) ListenTLSMutual(addr, certFile, keyFile, clientCertFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("gofi: cannot load TLS certificate: %w", err)
	}

	clientCACert, err := os.ReadFile(clientCertFile)
	if err != nil {
		return fmt.Errorf("gofi: cannot load client CA certificate: %w", err)
	}

	clientCertPool := x509.NewCertPool()
	if !clientCertPool.AppendCertsFromPEM(clientCACert) {
		return fmt.Errorf("gofi: failed to append client cert to pool")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCertPool,
	}

	ln, err := net.Listen("tcp4", listenAddr(addr))
	if err != nil {
		return err
	}

	tlsListener := tls.NewListener(ln, tlsConfig)
	return s.server.Serve(tlsListener)
}

// ListenOnListener starts the fasthttp server on a pre-created listener.
// Useful for tests that need a random port.
func (s *FasthttpServer) ListenOnListener(ln net.Listener) error {
	return s.server.Serve(ln)
}

// Shutdown gracefully shuts down the server.
func (s *FasthttpServer) Shutdown() error {
	return s.server.Shutdown()
}

// ShutdownWithContext gracefully shuts down the server with a timeout context.
func (s *FasthttpServer) ShutdownWithContext(ctx stdcontext.Context) error {
	return s.server.ShutdownWithContext(ctx)
}

// listenAddr returns a Listen-ready address string.
func listenAddr(addr string) string {
	if addr == "" {
		return ":8080"
	}
	if !strings.Contains(addr, ":") {
		return ":" + addr
	}
	return addr
}
