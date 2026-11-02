package gofi

import (
	stdcontext "context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/valyala/fasthttp"
)

// FasthttpServer wraps a serveMux and drives it via fasthttp.Server,
// giving connection-level performance (worker pool, keepalive, zero-copy TCP)
// while preserving the existing handler chain unchanged.
type FasthttpServer struct {
	mux    *serveMux
	server *fasthttp.Server
	// bodyCeiling is the ServerOptions body limit; zero without ServerOptions.
	bodyCeiling int
}

// NewFasthttpServer creates a FasthttpServer backed by a new router. Without
// ServerOptions, fasthttp's transport defaults apply, which leave timeouts and
// connection counts unbounded. Invalid options panic; check them first with
// ValidateServerOptions when they come from configuration.
func NewFasthttpServer(opts ...ServerOptions) *FasthttpServer {
	var s *FasthttpServer
	if len(opts) == 0 {
		s = newRouter().newFasthttpServer()
	} else {
		var err error
		s, err = newFasthttpServerWithOptions(opts[0])
		if err != nil {
			panic(err)
		}
	}
	s.mux.lifecycle.mu.Lock()
	s.mux.lifecycle.owner = s
	s.mux.lifecycle.mu.Unlock()
	return s
}

// newFasthttpServer keeps all native listener paths wired identically.
func (m *serveMux) newFasthttpServer() *FasthttpServer {
	return &FasthttpServer{
		mux: m,
		server: &fasthttp.Server{
			Handler:            m.handleFastHTTP,
			ErrorHandler:       m.handleTransportError,
			Name:               "gofi",
			DisableKeepalive:   false,
			ReduceMemoryUsage:  false,
			MaxRequestBodySize: m.opts.bodyLimit,
		},
	}
}

// Router returns the underlying Router so callers can register routes.
func (s *FasthttpServer) Router() Router {
	return s.mux
}

// Listen starts the fasthttp server on the given address (e.g. ":4000").
func (s *FasthttpServer) Listen(addr string) error {
	if err := s.applyRouterConfig(); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.server.Serve(resilientListener{ln})
}

// ListenTLS starts the fasthttp server with TLS encryption.
func (s *FasthttpServer) ListenTLS(addr, certFile, keyFile string) error {
	if err := s.applyRouterConfig(); err != nil {
		return err
	}
	if s.server.TLSConfig == nil {
		s.server.TLSConfig = serverTLSConfig(nil)
	}
	ln, err := net.Listen("tcp", listenAddr(addr))
	if err != nil {
		return err
	}
	return s.server.ServeTLS(resilientListener{ln}, certFile, keyFile)
}

// ListenTLSMutual starts the fasthttp server with mutual TLS (mTLS) authentication.
func (s *FasthttpServer) ListenTLSMutual(addr, certFile, keyFile, clientCertFile string) error {
	if err := s.applyRouterConfig(); err != nil {
		return err
	}
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

	tlsConfig := serverTLSConfig(s.server.TLSConfig)
	tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	tlsConfig.ClientCAs = clientCertPool

	ln, err := net.Listen("tcp", listenAddr(addr))
	if err != nil {
		return err
	}

	tlsListener := tls.NewListener(resilientListener{ln}, tlsConfig)
	return s.server.Serve(tlsListener)
}

// ListenOnListener starts the fasthttp server on a pre-created listener.
// Useful for tests that need a random port.
func (s *FasthttpServer) ListenOnListener(ln net.Listener) error {
	if err := s.applyRouterConfig(); err != nil {
		return err
	}
	return s.server.Serve(resilientListener{ln})
}

// Shutdown gracefully shuts down the server.
func (s *FasthttpServer) Shutdown() error {
	s.mux.lifecycle.cancel()
	return s.server.Shutdown()
}

// ShutdownWithContext stops accepting connections and waits for open ones to
// close until ctx is done. If ctx ends first it returns ctx.Err(); handlers
// and hijacked connections that are still running are not stopped.
func (s *FasthttpServer) ShutdownWithContext(ctx stdcontext.Context) error {
	s.mux.lifecycle.cancel()
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

// resilientListener keeps a server running when the process runs out of file
// descriptors. fasthttp treats that Accept error as permanent and stops serving
// for good, although the connections behind it may close moments later. The
// backoff is the one net/http's Server.Serve uses.
type resilientListener struct{ net.Listener }

func (l resilientListener) Accept() (net.Conn, error) {
	var delay time.Duration
	for {
		conn, err := l.Listener.Accept()
		if err == nil || !isTemporaryAcceptError(err) {
			return conn, err
		}
		if delay == 0 {
			log.Printf("gofi: %v; retrying", err)
			delay = 5 * time.Millisecond
		} else if delay *= 2; delay > time.Second {
			delay = time.Second
		}
		time.Sleep(delay)
	}
}

func isTemporaryAcceptError(err error) bool {
	return errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) || errors.Is(err, syscall.ECONNABORTED)
}
