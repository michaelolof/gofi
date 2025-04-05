package gofi

import (
	"net/http"
	"net/http/httptest"
)

type Router interface {
	Route(method string, path string, opts RouteOptions)
	Get(path string, opts RouteOptions)
	Post(path string, opts RouteOptions)
	Put(path string, opts RouteOptions)
	Delete(path string, opts RouteOptions)
	Patch(path string, opts RouteOptions)
	Head(path string, opts RouteOptions)
	Options(path string, opts RouteOptions)
	Trace(path string, opts RouteOptions)
	Connect(path string, opts RouteOptions)
	Use(middlewares ...func(http.Handler) http.Handler)
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	GlobalStore() GofiStore
	Meta() RouterMeta
	SetErrorHandler(func(err error, c Context))
	Inject(opts InjectOptions) (*httptest.ResponseRecorder, error)
	SetCustomSpecs(list map[string]CustomSchemaProps)
}
