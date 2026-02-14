package gofi

import (
	"net/http"
	"net/http/httptest"
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

	// Method atss route for pattern that matches the method http method
	Method(method string, pattern string, opts RouteOptions)
	// With adds inline middlewares for an endpoint handler
	With(middlewares ...func(http.Handler) http.Handler) Router
	// Group adds a new inline router along the current routing path
	// with a fresh middleware stack for the inline router
	Group(fn func(r Router)) Router

	// Mount attaches another http.Handler or chi Router as a subrouter along a routing
	// path. It's very useful to split up a large API as many independent routers and
	// compose them as a single service using Mount
	Mount(pattern string, handler http.Handler)

	// Use appends one or more middleware onto the router stack
	Use(middlewares ...func(http.Handler) http.Handler)

	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))

	// ServeHTTP implements the http.Handler interface
	ServeHTTP(w http.ResponseWriter, r *http.Request)

	// Inject allows you to inject a request into the router and get a response. Used for testing routes requests and responses
	Inject(opts InjectOptions) (*httptest.ResponseRecorder, error)

	GlobalStore() GofiStore
	Meta() RouterMeta
	SetErrorHandler(func(err error, c Context))
	SetCustomSpecs(list map[string]CustomSchemaProps)
	SetCustomValidator(list map[string]func(c ValidatorContext) func(arg any) error)
}
