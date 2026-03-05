package gofi

import (
	"net"
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
	m := newServeMux()
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

// ListenOnListener starts the fasthttp server on a pre-created listener.
// Useful for tests that need a random port.
func (s *FasthttpServer) ListenOnListener(ln net.Listener) error {
	return s.server.Serve(ln)
}

// Shutdown gracefully shuts down the server.
func (s *FasthttpServer) Shutdown() error {
	return s.server.Shutdown()
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
