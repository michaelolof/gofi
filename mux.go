package gofi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/michaelolof/gofi/validators/rules"
)

type serveMux struct {
	sm                *http.ServeMux
	rOpts             *RouteOptions
	opts              *muxOptions
	paths             docsPaths
	routeMeta         metaMap
	globalStore       GofiStore
	middlewares       Middlewares
	inlineMiddlewares Middlewares
	prefix            string
	preHandlers       []PreHandler
}

func NewServeMux() Router {
	return newServeMux()
}

func (s *serveMux) Method(method string, path string, opts RouteOptions) {
	s.method(method, path, opts)
}

func (s *serveMux) Get(path string, opts RouteOptions) {
	s.method(http.MethodGet, path, opts)
}

func (s *serveMux) Post(path string, opts RouteOptions) {
	s.method(http.MethodPost, path, opts)
}

func (s *serveMux) Put(path string, opts RouteOptions) {
	s.method(http.MethodPut, path, opts)
}

func (s *serveMux) Delete(path string, opts RouteOptions) {
	s.method(http.MethodDelete, path, opts)
}

func (s *serveMux) Patch(path string, opts RouteOptions) {
	s.method(http.MethodPatch, path, opts)
}

func (s *serveMux) Head(path string, opts RouteOptions) {
	s.method(http.MethodHead, path, opts)
}

func (s *serveMux) Options(path string, opts RouteOptions) {
	s.method(http.MethodOptions, path, opts)
}

func (s *serveMux) Trace(path string, opts RouteOptions) {
	s.method(http.MethodTrace, path, opts)
}

func (s *serveMux) Connect(path string, opts RouteOptions) {
	s.method(http.MethodConnect, path, opts)
}

func (s *serveMux) Route(pattern string, fn func(r Router)) Router {
	im := s.With().(*serveMux)
	if pattern != "" {
		if im.prefix != "" && !strings.HasSuffix(im.prefix, "/") && !strings.HasPrefix(pattern, "/") {
			im.prefix += "/"
		}
		im.prefix += pattern
	}

	if fn != nil {
		fn(im)
	}
	return im
}

// Group creates a new inline-Mux with a copy of middleware stack. It's useful
// for a group of handlers along the same routing path that use an additional
// set of middlewares.
func (s *serveMux) Group(fn func(r Router)) Router {
	im := s.With()
	if fn != nil {
		fn(im)
	}
	return im
}

func (s *serveMux) Use(middlewares ...func(http.Handler) http.Handler) {
	s.middlewares = append(s.middlewares, middlewares...)
}

func (s *serveMux) With(middlewares ...func(http.Handler) http.Handler) Router {
	newMux := *s
	newMux.inlineMiddlewares = make(Middlewares, len(s.inlineMiddlewares), len(s.inlineMiddlewares)+len(middlewares))
	copy(newMux.inlineMiddlewares, s.inlineMiddlewares)
	newMux.inlineMiddlewares = append(newMux.inlineMiddlewares, middlewares...)

	// Deep copy preHandlers for isolation
	newMux.preHandlers = make([]PreHandler, len(s.preHandlers))
	copy(newMux.preHandlers, s.preHandlers)

	return &newMux
}

// func (s *serveMux) Mount(pattern string, handler http.Handler) {
// 	if pattern == "" {
// 		pattern = "/"
// 	}

// 	// Ensure pattern starts with / if not empty
// 	if pattern != "/" && !strings.HasPrefix(pattern, "/") {
// 		pattern = "/" + pattern
// 	}

// 	// If there is a prefix (from Route method), prepend it
// 	if s.prefix != "" {
// 		if !strings.HasSuffix(s.prefix, "/") && !strings.HasPrefix(pattern, "/") {
// 			pattern = "/" + pattern
// 		}
// 		pattern = s.prefix + pattern
// 	}

// 	// Mount logic: Mount usually implies a subtree, so we ensure trailing slash
// 	// for the registration pattern on the underlying ServeMux.
// 	mountPath := pattern
// 	if !strings.HasSuffix(mountPath, "/") {
// 		mountPath += "/"
// 	}

// 	// Apply middlewares
// 	middlewares := s.inlineMiddlewares
// 	if len(middlewares) > 0 {
// 		handler = middlewares[len(middlewares)-1](handler)
// 		for i := len(middlewares) - 2; i >= 0; i-- {
// 			handler = middlewares[i](handler)
// 		}
// 	}

// 	// Register with StripPrefix to ensure the sub-handler sees relative paths
// 	stripPath := strings.TrimSuffix(mountPath, "/")
// 	s.sm.Handle(mountPath, http.StripPrefix(stripPath, handler))
// }

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

func (s *serveMux) UseErrorHandler(handler func(err error, c Context)) {
	if handler != nil {
		s.opts.errHandler = handler
	}
}

func (s *serveMux) UsePreHandler(h ...func(h HandlerFunc) HandlerFunc) {
	s.preHandlers = append(s.preHandlers, h...)
}

func (s *serveMux) RegisterSpec(list ...CustomSpec) {
	for _, v := range list {
		s.opts.customSpecs[v.SpecID()] = v
	}
}

func (s *serveMux) RegisterBodyParser(list ...BodyParser) {
	if len(list) > 0 {
		s.opts.bodyParsers = append(list, s.opts.bodyParsers...)
	}
}

type ValidatorContext = rules.ValidatorContext

func (s *serveMux) RegisterValidator(list ...Validator) {
	for _, v := range list {
		s.opts.customValidators[v.Name()] = v.Rule
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

func (s *serveMux) Inject(opts InjectOptions) (rec *httptest.ResponseRecorder, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered in Inject: %v", r)
			if rec == nil {
				rec = httptest.NewRecorder()
			}
			rec.WriteHeader(http.StatusInternalServerError)
		}
	}()

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
		if ropts.Schema != nil {
			rules := s.compileSchema(ropts.Schema, ropts.Info)
			rules.specs.normalize(method, path)
			s.opts.schemaRules.SetRules(path, method, &rules.rules)
		}

		c.setContextSettings(newContextOptions(path, method), meta, s.globalStore, s.opts)
	}

	var routeMeta metaMap
	if opts.Handler.Meta != nil {
		v := map[string]any{strings.ToLower(opts.Method): opts.Handler.Meta}
		s.routeMeta[opts.Path] = v
		routeMeta = s.routeMeta
	}
	setupInjectContext(opts.Path, opts.Method, opts.Handler, routeMeta)

	// Combine global and route-specific pre-handlers
	allPreHandlers := make([]PreHandler, 0, len(s.preHandlers)+len(def.PreHandlers))
	allPreHandlers = append(allPreHandlers, s.preHandlers...)
	allPreHandlers = append(allPreHandlers, def.PreHandlers...)

	handler := applyMiddleware(def.Handler, allPreHandlers)
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

func (s *serveMux) method(method string, path string, opts RouteOptions) {
	if s.prefix != "" {
		if !strings.HasSuffix(s.prefix, "/") && !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		path = s.prefix + path
	}

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

	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := newContext(w, r)
		c.setContextSettings(newContextOptions(path, method), s.routeMeta, s.globalStore, s.opts)

		// Combine global and route-specific pre-handlers
		// Note: We create a new slice to avoid modifying the backing array
		allPreHandlers := make([]PreHandler, 0, len(s.preHandlers)+len(opts.PreHandlers))
		allPreHandlers = append(allPreHandlers, s.preHandlers...)
		allPreHandlers = append(allPreHandlers, opts.PreHandlers...)

		err := applyMiddleware(opts.Handler, allPreHandlers)(c)
		if err != nil {
			s.opts.errHandler(err, c)
		}
	})

	middlewares := s.inlineMiddlewares
	if len(middlewares) > 0 {
		h = middlewares[len(middlewares)-1](h)
		for i := len(middlewares) - 2; i >= 0; i-- {
			h = middlewares[i](h)
		}
	}

	s.sm.Handle(method+" "+path, h)
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
		sm:                sm,
		paths:             paths,
		routeMeta:         rm,
		globalStore:       globalStore,
		middlewares:       m,
		inlineMiddlewares: make(Middlewares, 0),
		preHandlers:       make([]PreHandler, 0),
		opts:              opts,
		rOpts:             nil,
	}
}
