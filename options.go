package gofi

import (
	"crypto/tls"
	"fmt"
	"time"
)

// ServerOptions configures the native fasthttp transport created by
// NewFasthttpServer. Zero values mean "use fasthttp's default",
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
	// TLSConfig is cloned and used by ListenTLS. The server enforces TLS 1.2
	// or later and negotiates HTTP/1.1 only; certificates passed to ListenTLS
	// are added to the clone.
	TLSConfig *tls.Config
	// Router configures the server's router.
	Router RouterOptions
}

// RouterOptions configures a router created by NewRouter, or by
// NewFasthttpServer through ServerOptions.Router.
type RouterOptions struct {
	// Limits bounds the structure of request bodies.
	Limits RequestLimits
	// AllowEncodedSeparators accepts request paths containing %2F, %5C or
	// %00. They are rejected with 400 by default, because the router matches
	// the decoded path and "/a%2Fb" would otherwise route as "/a/b".
	AllowEncodedSeparators bool
	// DisableMethodNotAllowed answers 404 instead of 405 with an Allow
	// header when a path exists only under other methods.
	DisableMethodNotAllowed bool
}

func (m *serveMux) applyRouterOptions(opts RouterOptions) error {
	limits, err := opts.Limits.normalize()
	if err != nil {
		return err
	}
	m.opts.limits = limits
	m.opts.allowEncodedSeparators = opts.AllowEncodedSeparators
	m.opts.methodNotAllowed = !opts.DisableMethodNotAllowed
	return nil
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

// ValidateServerOptions reports the first invalid setting in opts, including
// opts.Router.
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
	_, err := opts.Router.Limits.normalize()
	return err
}

// newFasthttpServerWithOptions creates a FasthttpServer with a new router and
// a validated transport configuration.
func newFasthttpServerWithOptions(opts ServerOptions) (*FasthttpServer, error) {
	if err := ValidateServerOptions(opts); err != nil {
		return nil, err
	}
	m := newRouter()
	if err := m.applyRouterOptions(opts.Router); err != nil {
		return nil, err
	}
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
	srv.TLSConfig = serverTLSConfig(opts.TLSConfig)
	return s, nil
}

// serverTLSConfig clones base (or starts empty), requires TLS 1.2 or later,
// and advertises only HTTP/1.1, the only protocol fasthttp serves.
func serverTLSConfig(base *tls.Config) *tls.Config {
	cfg := &tls.Config{}
	if base != nil {
		cfg = base.Clone()
	}
	if cfg.MinVersion < tls.VersionTLS12 {
		cfg.MinVersion = tls.VersionTLS12
	}
	cfg.NextProtos = []string{"http/1.1"}
	return cfg
}

// applyRouterConfig freezes the router's body limit into the transport right
// before serving, so Router().Configure calls made after construction take
// effect. With ServerOptions, the router may lower but not raise the ceiling.
func (s *FasthttpServer) applyRouterConfig() error {
	s.mux.lifecycle.seal()
	s.mux.lifecycle.renewIfCancelled()
	limit := s.mux.opts.bodyLimit
	if s.bodyCeiling > 0 && (limit <= 0 || limit > s.bodyCeiling) {
		limit = s.bodyCeiling
	}
	s.server.MaxRequestBodySize = limit
	return nil
}
