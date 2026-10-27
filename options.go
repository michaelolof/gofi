package gofi

import (
	"fmt"
	"time"
)

// ServerOptions configures the native fasthttp transport created by
// NewFasthttpServerWithOptions. Zero values mean "use fasthttp's default",
// which for timeouts and connection limits is unlimited; negative values are
// invalid. Transport timeouts bound socket reads and writes, not handler
// execution: use ContextTimeout for handler deadlines.
type ServerOptions struct {
	// ReadTimeout bounds reading a full request, including the body.
	ReadTimeout time.Duration
	// WriteTimeout bounds writing a full response.
	WriteTimeout time.Duration
	// IdleTimeout bounds waiting for the next request on a keep-alive
	// connection. Zero falls back to ReadTimeout.
	IdleTimeout time.Duration
	// MaxRequestBodySize is the largest request body the transport buffers.
	// Router.Configure may lower it before the server starts, never raise it.
	MaxRequestBodySize int
	// ReadBufferSize also bounds the total size of request headers.
	ReadBufferSize int
	// MaxConnections bounds concurrent connections (fasthttp Concurrency).
	MaxConnections int
	// MaxConnsPerIP bounds concurrent connections from one client IP. Behind
	// a proxy every client shares the proxy's IP.
	MaxConnsPerIP int
	// MaxRequestsPerConn closes a keep-alive connection after this many requests.
	MaxRequestsPerConn int
}

// DefaultServerOptions returns a bounded starting profile for staging. The
// values are an initial experiment, not tuned production recommendations:
// measure against the intended memory budget and adjust.
func DefaultServerOptions() ServerOptions {
	return ServerOptions{
		ReadTimeout:        10 * time.Second,
		WriteTimeout:       15 * time.Second,
		IdleTimeout:        60 * time.Second,
		MaxRequestBodySize: 1 << 20,
		ReadBufferSize:     16 << 10,
		MaxConnections:     128,
		MaxRequestsPerConn: 1000,
	}
}

// ValidateServerOptions reports the first invalid setting in opts.
func ValidateServerOptions(opts ServerOptions) error {
	durations := map[string]time.Duration{
		"ReadTimeout": opts.ReadTimeout, "WriteTimeout": opts.WriteTimeout, "IdleTimeout": opts.IdleTimeout,
	}
	for name, d := range durations {
		if d < 0 {
			return fmt.Errorf("gofi: ServerOptions.%s must not be negative", name)
		}
	}
	sizes := map[string]int{
		"MaxRequestBodySize": opts.MaxRequestBodySize, "ReadBufferSize": opts.ReadBufferSize,
		"MaxConnections": opts.MaxConnections, "MaxConnsPerIP": opts.MaxConnsPerIP,
		"MaxRequestsPerConn": opts.MaxRequestsPerConn,
	}
	for name, n := range sizes {
		if n < 0 {
			return fmt.Errorf("gofi: ServerOptions.%s must not be negative", name)
		}
	}
	return nil
}

// NewFasthttpServerWithOptions creates a FasthttpServer with a new router and
// a validated transport configuration.
func NewFasthttpServerWithOptions(opts ServerOptions) (*FasthttpServer, error) {
	if err := ValidateServerOptions(opts); err != nil {
		return nil, err
	}
	m := newRouter()
	if opts.MaxRequestBodySize > 0 {
		m.opts.bodyLimit = opts.MaxRequestBodySize
	}
	s := m.newFasthttpServer()
	s.bodyCeiling = opts.MaxRequestBodySize
	srv := s.server
	srv.ReadTimeout = opts.ReadTimeout
	srv.WriteTimeout = opts.WriteTimeout
	srv.IdleTimeout = opts.IdleTimeout
	srv.ReadBufferSize = opts.ReadBufferSize
	srv.Concurrency = opts.MaxConnections
	srv.MaxConnsPerIP = opts.MaxConnsPerIP
	srv.MaxRequestsPerConn = opts.MaxRequestsPerConn
	return s, nil
}

// applyRouterConfig freezes the router's body limit into the transport right
// before serving, so Router().Configure calls made after construction take
// effect. With ServerOptions, the router may lower but not raise the ceiling.
func (s *FasthttpServer) applyRouterConfig() error {
	s.mux.lifecycle.seal()
	limit := s.mux.opts.bodyLimit
	if s.bodyCeiling > 0 && (limit <= 0 || limit > s.bodyCeiling) {
		limit = s.bodyCeiling
	}
	s.server.MaxRequestBodySize = limit
	return nil
}
