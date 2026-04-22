package gofi

import (
	stdcontext "context"

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

	// Group adds a new inline router along the current routing path
	// with a fresh middleware stack for the inline router
	Group(fn func(r Router)) Router

	// Use appends one or more middleware onto the router stack
	Use(middlewares ...MiddlewareFunc)

	// UseErrorHandler sets the general error handler for the router
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

	// ShutdownWithContext gracefully shuts down the server, forcefully closing after context is canceled.
	ShutdownWithContext(ctx stdcontext.Context) error

	// Handler returns the underlying fasthttp.RequestHandler, allowing the router to be
	// embedded within custom fasthttp.Server configurations natively.
	Handler() fasthttp.RequestHandler

	GlobalStore() GofiStore
	Meta() RouterMeta

	RegisterValidator(list ...Validator)
	RegisterSpec(l ...CustomSpec)
	RegisterBodyParser(l ...BodyParser)
	Static(prefix, root string)

	// Configure sets router-level configurations (e.g. MaxRequestBodySize)
	Configure(config Config)
}

// Config defines the configuration options for a GoFi Router instance.
type Config struct {
	// BodyLimit sets the maximum allowed size for a request body (in bytes).
	// Default: 4 * 1024 * 1024 (4MB) if zero or not provided.

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
}
