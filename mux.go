package gofi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

type serveMux struct {
	sm          *http.ServeMux
	opts        *MuxOptions
	paths       docsPaths
	routeMeta   metaMap
	globalStore GofiStore
	errHandler  func(err error, c Context)
	middlewares Middlewares
}

func NewServeMux() Router {
	middlewares := make(Middlewares, 0, 15)
	return serveMuxBuilder(
		http.NewServeMux(),
		map[string]map[string]openapiOperationObject{},
		map[string]map[string]any{},
		NewGlobalStore(),
		defaultErrorHandler,
		middlewares,
	)
}

func (s *serveMux) Route(method string, path string, opts RouteOptions) {
	s.route(method, path, opts)
}

func (s *serveMux) Get(path string, opts RouteOptions) {
	s.route(http.MethodGet, path, opts)
}

func (s *serveMux) Post(path string, opts RouteOptions) {
	s.route(http.MethodPost, path, opts)
}

func (s *serveMux) Put(path string, opts RouteOptions) {
	s.route(http.MethodPut, path, opts)
}

func (s *serveMux) Delete(path string, opts RouteOptions) {
	s.route(http.MethodDelete, path, opts)
}

func (s *serveMux) Patch(path string, opts RouteOptions) {
	s.route(http.MethodPatch, path, opts)
}

func (s *serveMux) Head(path string, opts RouteOptions) {
	s.route(http.MethodHead, path, opts)
}

func (s *serveMux) Options(path string, opts RouteOptions) {
	s.route(http.MethodOptions, path, opts)
}

func (s *serveMux) Trace(path string, opts RouteOptions) {
	s.route(http.MethodTrace, path, opts)
}

func (s *serveMux) Connect(path string, opts RouteOptions) {
	s.route(http.MethodConnect, path, opts)
}

func (s *serveMux) Use(middlewares ...func(http.Handler) http.Handler) {
	s.middlewares = append(s.middlewares, middlewares...)
}

func (s *serveMux) Handle(pattern string, handler http.Handler) {
	s.sm.Handle(pattern, handler)
}

func (s *serveMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	s.sm.HandleFunc(pattern, handler)
}

func (s *serveMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var h http.Handler = s.sm
	middlewares := s.middlewares
	if len(middlewares) > 0 {
		h = middlewares[len(middlewares)-1](h)
		for i := len(middlewares) - 2; i >= 0; i-- {
			h = middlewares[i](h)
		}
	}

	h.ServeHTTP(w, r)
}

type MuxOptions struct {
	ErrorHandler     func(err error, c Context)
	CustomValidators map[string]func([]any) func(any) error
	CompilerHooks    any
}

func (s *serveMux) DefineMuxOptions(opts MuxOptions) {
	s.opts = &opts
	if opts.ErrorHandler != nil {
		s.errHandler = opts.ErrorHandler
	}
}

func (s *serveMux) GlobalStore() GofiStore {
	return s.globalStore
}

func (s *serveMux) Meta() RouterMeta {
	return s.routeMeta
}

type InjectOptions struct {
	Path    string
	Method  string
	Paths   map[string]string
	Headers map[string]string
	Cookies []http.Cookie
	Body    io.Reader
	Handler *RouteOptions
}

func (s *serveMux) Inject(opts InjectOptions) (*httptest.ResponseRecorder, error) {
	r, err := http.NewRequest(opts.Method, opts.Path, opts.Body)
	if err != nil {
		return nil, err
	}
	for name, value := range opts.Paths {
		r.SetPathValue(name, value)
	}

	for _, cookie := range opts.Cookies {
		r.AddCookie(&cookie)
	}

	for name, value := range opts.Headers {
		r.Header.Add(name, value)
	}

	def := opts.Handler
	if def == nil {
		return nil, fmt.Errorf("gofi controller not defined for the given method '%s' and path '%s'", opts.Method, opts.Path)
	}

	rules := s.compileSchema(def.Schema, def.Info)
	rules.specs.normalize(opts.Method, opts.Path)

	w := httptest.NewRecorder()
	c := newContext(w, r)
	var routeMeta metaMap
	if opts.Handler.Meta != nil {
		v := map[string]any{strings.ToLower(opts.Method): opts.Handler.Meta}
		s.routeMeta[opts.Path] = v
		routeMeta = s.routeMeta
	}
	c.setContextSettings(&rules.rules, routeMeta, s.globalStore)
	handler := applyMiddleware(def.Handler, def.Middlewares)
	err = handler(c)
	if err != nil {
		s.errHandler(err, c)
		return w, nil
	}

	return w, nil
}

func ListenAndServe(addr string, handler http.Handler) error {
	return http.ListenAndServe(addr, handler)
}

func (s *serveMux) route(method string, path string, opts RouteOptions) {

	var rules *schemaRules

	if opts.Schema != nil {
		comps := s.compileSchema(opts.Schema, opts.Info)
		comps.specs.normalize(method, path)

		if len(s.paths[path]) == 0 {
			v := map[string]openapiOperationObject{
				strings.ToLower(method): comps.specs,
			}

			if !opts.Info.Hidden {
				s.paths[path] = v
			}
		} else {
			if !opts.Info.Hidden {
				s.paths[path][strings.ToLower(method)] = comps.specs
			}
		}

		rules = &comps.rules
	}

	if opts.Meta != nil {
		v := map[string]any{strings.ToLower(method): opts.Meta}
		s.routeMeta[path] = v
	}

	s.sm.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
		c := newContext(w, r)
		c.setContextSettings(rules, s.routeMeta, s.globalStore)

		err := applyMiddleware(opts.Handler, opts.Middlewares)(c)
		if err != nil {
			s.errHandler(err, c)
		}
	})
}

func serveMuxBuilder(sm *http.ServeMux, paths docsPaths, rm metaMap, globalStore GofiStore, errH func(e error, c Context), m Middlewares) *serveMux {
	return &serveMux{
		sm:          sm,
		paths:       paths,
		routeMeta:   rm,
		globalStore: globalStore,
		errHandler:  errH,
		middlewares: m,
	}
}
