package gofi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/michaelolof/gofi/validators"
)

type serveMux struct {
	sm          *http.ServeMux
	rOpts       *RouteOptions
	opts        *muxOptions
	paths       docsPaths
	routeMeta   metaMap
	globalStore GofiStore
	middlewares Middlewares
}

func NewServeMux() Router {
	return newServeMux()
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

func (s *serveMux) Group(o RouteOptions, r func(r Router)) {
	s.rOpts = &o
	r(s)
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

func (s *serveMux) GlobalStore() GofiStore {
	return s.globalStore
}

func (s *serveMux) Meta() RouterMeta {
	return s.routeMeta
}

func (s *serveMux) SetErrorHandler(handler func(err error, c Context)) {
	if handler != nil {
		s.opts.errHandler = handler
	}
}

func (s *serveMux) SetCustomSpecs(list map[string]CustomSchemaProps) {
	if list != nil {
		s.opts.customSpecs = list
	}
}

type ValidatorContext = validators.ValidatorContext
type ValidatorOption = validators.ValidatorArg

func (s *serveMux) SetCustomValidator(list map[string]func(c ValidatorContext) func(arg ValidatorOption) error) {
	if list != nil {
		s.opts.customValidators = list
	}
}

type InjectOptions struct {
	Path    string
	Method  string
	Query   map[string]string
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

	if len(opts.Query) > 0 {
		qParams := r.URL.Query()
		for name, value := range opts.Query {
			qParams.Add(name, value)
		}
		r.URL.RawQuery = qParams.Encode()
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

	w := httptest.NewRecorder()
	c := newContext(w, r)

	setupInjectContext := func(path, method string, ropts *RouteOptions, meta metaMap) {
		rules := s.compileSchema(ropts.Schema, ropts.Info)
		rules.specs.normalize(method, path)
		s.opts.schemaRules.SetRules(path, method, &rules.rules)
		c.setContextSettings(newContextOptions(path, method), meta, s.globalStore, s.opts)
	}

	var routeMeta metaMap
	if opts.Handler.Meta != nil {
		v := map[string]any{strings.ToLower(opts.Method): opts.Handler.Meta}
		s.routeMeta[opts.Path] = v
		routeMeta = s.routeMeta
	}
	setupInjectContext(opts.Path, opts.Method, opts.Handler, routeMeta)

	// rules := s.compileSchema(def.Schema, def.Info)
	// rules.specs.normalize(opts.Method, opts.Path)

	// var routeMeta metaMap
	// if opts.Handler.Meta != nil {
	// 	v := map[string]any{strings.ToLower(opts.Method): opts.Handler.Meta}
	// 	s.routeMeta[opts.Path] = v
	// 	routeMeta = s.routeMeta
	// }
	// c.setContextSettings(&rules.rules, routeMeta, s.globalStore, s.opts)
	handler := applyMiddleware(def.Handler, def.Middlewares)
	err = handler(c)
	if err != nil {
		s.opts.errHandler(err, c)
		return w, nil
	}

	return w, nil
}

func ListenAndServe(addr string, handler http.Handler) error {
	return http.ListenAndServe(addr, handler)
}

func (s *serveMux) route(method string, path string, opts RouteOptions) {

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

		s.opts.schemaRules.SetRules(path, method, &comps.rules)
	}

	if opts.Meta != nil {
		v := map[string]any{strings.ToLower(method): opts.Meta}
		s.routeMeta[path] = v
	}

	s.sm.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
		c := newContext(w, r)
		c.setContextSettings(newContextOptions(path, method), s.routeMeta, s.globalStore, s.opts)

		err := applyMiddleware(opts.Handler, opts.Middlewares)(c)
		if err != nil {
			s.opts.errHandler(err, c)
		}
	})
}

func newServeMux() *serveMux {
	middlewares := make(Middlewares, 0, 15)
	return serveMuxBuilder(
		http.NewServeMux(),
		map[string]map[string]openapiOperationObject{},
		map[string]map[string]any{},
		NewGlobalStore(),
		middlewares,
		defaultMuxOptions(),
	)
}

func serveMuxBuilder(sm *http.ServeMux, paths docsPaths, rm metaMap, globalStore GofiStore, m Middlewares, opts *muxOptions) *serveMux {
	return &serveMux{
		sm:          sm,
		paths:       paths,
		routeMeta:   rm,
		globalStore: globalStore,
		middlewares: m,
		opts:        opts,
		rOpts:       nil,
	}
}
