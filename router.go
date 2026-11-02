package gofi

import (
	stdcontext "context"
	"time"

	"github.com/valyala/fasthttp"
)

type Router interface {
	Connect(pattern string, o RouteOptions)
	Delete(pattern string, o RouteOptions)
	Get(pattern string, o RouteOptions)
	Head(pattern string, o RouteOptions)
	Options(pattern string, o RouteOptions)
	Patch(pattern string, o RouteOptions)
	Post(pattern string, o RouteOptions)
	Put(pattern string, o RouteOptions)
	Trace(pattern string, o RouteOptions)

	// Method adds a route for the pattern that matches the given HTTP method
	Method(method string, pattern string, opts RouteOptions)

	// With adds inline middlewares for an endpoint handler
	With(middlewares ...MiddlewareFunc) Router

	// Route mounts a sub-Router along a `pattern` string.
	Route(pattern string, fn func(r Router)) Router

	// Group adds a new inline router along the current routing path. The
	// inline router starts with a copy of the current middleware stack;
	// middleware it adds does not affect the parent or sibling routers.
	Group(fn func(r Router)) Router

	// Use appends one or more middleware onto the router stack
	Use(middlewares ...MiddlewareFunc)

	// UseErrorHandler sets the general error handler for the router, including
	// native request-read failures. For those failures the request may be incomplete;
	// middleware has not run and route parameters/metadata are unavailable.
	UseErrorHandler(func(err error, c Context))

	// Inject allows you to inject a request handler into the router and get a response.
	Inject(opts InjectOptions) (*InjectResponse, error)

	// Test dispatches a request through the full route tree (including middleware,
	// pre-handlers, and 404 handling) and returns the response. This is the
	// primary way to test routing behavior from external packages.
	Test(opts TestOptions) (*InjectResponse, error)

	// Listen starts the server on the given address
	Listen(addr string) error

	// ListenTLS starts an HTTPS server on the given address using the provided certificate and key files
	ListenTLS(addr, certFile, keyFile string) error

	// ListenTLSMutual starts an HTTPS server providing mutual TLS (mTLS) authentication
	ListenTLSMutual(addr, certFile, keyFile, clientCertFile string) error

	// Shutdown gracefully shuts down the server, waiting for active connections to finish.
	Shutdown() error

	// ShutdownWithContext stops accepting connections and waits for open ones
	// to close until ctx is done. If ctx ends first it returns ctx.Err();
	// handlers and hijacked connections that are still running are not stopped.
	ShutdownWithContext(ctx stdcontext.Context) error

	// Handler returns the underlying fasthttp.RequestHandler, allowing the router to be
	// embedded within custom fasthttp.Server configurations natively. The
	// caller then owns the transport: body limit (Config.BodyLimit does not
	// apply), timeouts, connection limits, ErrorHandler for request-read
	// failures, and shutdown.
	Handler() fasthttp.RequestHandler

	// GlobalStore returns the router-wide, shared key/value store. It is not
	// safe for concurrent writes: populate it entirely during setup, before
	// calling Listen/ListenTLS/Serve. Once the server is running, only read
	// from it (Get/Has/StoreGet/...) — never call Set/StoreSet from a handler,
	// middleware, or any goroutine racing with request handling. See the
	// concurrency notes on GofiStore.Set / StoreSet for why.
	GlobalStore() GofiStore
	Meta() RouterMeta

	RegisterValidator(list ...Validator)
	RegisterSpec(l ...CustomSpec)
	RegisterBodyParser(l ...BodyParser)
	Static(prefix, root string)

	// Configure sets router-level configuration, such as the body limit and the
	// server's timeouts and connection limits. Call it before the routes.
	Configure(config Config)
}

// Config defines the configuration options for a GoFi Router instance.
type Config struct {
	// MethodNotAllowed controls whether a 405 response is sent when a path matches
	// a registered route under a different HTTP method. When true (the default),
	// requests that match no route for the requested method but do match a route
	// for another method receive a 405 with an Allow header. When false, those
	// requests fall through to the 404 handler.
	//
	// Note: Set this to false if you manage OPTIONS/CORS responses yourself.
	MethodNotAllowed *bool

	// BodyLimit sets the maximum allowed size for a request body (in bytes).
	// Default: 4 * 1024 * 1024 (4MB) if zero or not provided.
	BodyLimit int

	// ReadTimeout bounds reading a full request, including the body, and also
	// closes a connection that never sends one. WriteTimeout bounds writing a
	// full response, so it must allow for the slowest download or stream.
	// IdleTimeout bounds waiting for the next request on a keep-alive connection;
	// it starts after the first request, and zero falls back to ReadTimeout.
	// Zero leaves a setting as it is, which on a plain Listen is unlimited.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// MaxConnections bounds concurrent connections. Keep it below the process's
	// open-file limit. MaxConnsPerIP bounds them per client IP; behind a proxy
	// every client shares the proxy's IP.
	MaxConnections int
	MaxConnsPerIP  int
}
