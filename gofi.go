package gofi

import (
	"net/http"
	"net/http/httptest"
)

type Router interface {
	Route(method string, path string, opts HandlerOptions)
	Get(path string, handler HandlerOptions)
	Post(path string, handler HandlerOptions)
	Put(path string, handler HandlerOptions)
	Patch(path string, handler HandlerOptions)
	Delete(path string, handler HandlerOptions)
	Inject(opts InjectOptions) (*httptest.ResponseRecorder, error)
	With(middlewares ...func(http.Handler) http.Handler) *ServeMux
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	Use(middlewares ...func(http.Handler) http.Handler)
	RegisterValidators(validators map[string]func([]any) func(any) error)
	SetErrorHandler(handler func(err error, c Context))
}
