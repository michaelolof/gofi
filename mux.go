package gofi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/michaelolof/gofi/validators/rules"
)

type serveMux struct {
	trees             map[string]*node // Replaced *http.ServeMux with radix trees
	rOpts             *RouteOptions
	opts              *muxOptions
	paths             docsPaths
	routeMeta         metaMap
	globalStore       GofiStore
	middlewares       Middlewares
	inlineMiddlewares Middlewares
	prefix            string
	preHandlers       []PreHandler
	chainedHandler    http.Handler
	ctxPool           *sync.Pool
	maxParams         uint8
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
	s.buildChain()
}

// buildChain pre-builds the middleware chain so ServeHTTP doesn't
// have to reconstruct it on every request.
func (s *serveMux) buildChain() {
	var h http.Handler = http.HandlerFunc(s.serveHTTPMatched)
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		h = s.middlewares[i](h)
	}
	s.chainedHandler = h
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

func (s *serveMux) Handle(pattern string, handler http.Handler) {
	s.method("GET", pattern, RouteOptions{
		Handler: func(c Context) error {
			handler.ServeHTTP(c.Writer(), c.Request())
			return nil
		},
	})
}

func (s *serveMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	s.Handle(pattern, http.HandlerFunc(handler))
}

func (s *serveMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.chainedHandler != nil {
		s.chainedHandler.ServeHTTP(w, r)
	} else {
		s.serveHTTPMatched(w, r)
	}
}

func (s *serveMux) serveHTTPMatched(w http.ResponseWriter, r *http.Request) {
	if root := s.trees[r.Method]; root != nil {
		c := s.acquireContext(w, r)

		routeData, tsr := root.getValue(r.URL.Path, func() *Params {
			// Ensure slice capacity
			if cap(c.params) < int(s.maxParams) {
				c.params = make(Params, 0, s.maxParams)
			}
			return &c.params
		})

		if routeData != nil {
			// Found matching route
			c.setContextSettings(newContextOptions(r.URL.Path, r.Method), s.routeMeta, s.globalStore, s.opts)

			// Inject the pre-composed handler
			routeData.handler.ServeHTTP(w, r)

			s.releaseContext(c)
			return
		} else if r.Method != http.MethodConnect && r.URL.Path != "/" {
			// Try trailing slash redirect
			if tsr {
				code := http.StatusMovedPermanently
				if r.Method != http.MethodGet {
					code = http.StatusPermanentRedirect
				}
				reqURL := r.URL.Path
				if len(reqURL) > 1 && reqURL[len(reqURL)-1] == '/' {
					r.URL.Path = reqURL[:len(reqURL)-1]
				} else {
					r.URL.Path = reqURL + "/"
				}
				http.Redirect(w, r, r.URL.String(), code)
				s.releaseContext(c)
				return
			}
		}

		s.releaseContext(c)
	}

	http.NotFound(w, r)
}

func (s *serveMux) acquireContext(w http.ResponseWriter, r *http.Request) *context {
	c := s.ctxPool.Get().(*context)
	c.reset(w, r)
	// Truncate params slice, retaining capacity
	c.params = c.params[:0]
	return c
}

func (s *serveMux) releaseContext(c *context) {
	s.ctxPool.Put(c)
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

func (s *serveMux) Static(prefix, root string) {
	if s.prefix != "" {
		if !strings.HasSuffix(s.prefix, "/") && !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		prefix = s.prefix + prefix
	}

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	fs := http.FileServer(http.Dir(root))
	handler := http.StripPrefix(prefix, fs)

	s.method("GET", prefix+"*filepath", RouteOptions{
		Handler: func(c Context) error {
			handler.ServeHTTP(c.Writer(), c.Request())
			return nil
		},
	})
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
	c := s.acquireContext(w, r)
	defer s.releaseContext(c)

	for name, value := range opts.Paths {
		c.params = append(c.params, Param{Key: name, Value: value})
	}

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

	// Pre-compose all pre-handlers at registration time (not per-request)
	allPreHandlers := make([]PreHandler, 0, len(s.preHandlers)+len(opts.PreHandlers))
	allPreHandlers = append(allPreHandlers, s.preHandlers...)
	allPreHandlers = append(allPreHandlers, opts.PreHandlers...)
	composedHandler := applyMiddleware(opts.Handler, allPreHandlers)

	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Inside the tree dispatch, context is already acquired via serveHTTPMatched.
		// So we just retrieve it from the request if we were truly doing middleware wrapping.
		// However, for performance and integration, we can pass context through.
		// For now, since chained middleware wraps the serveHTTPMatched, and serveHTTPMatched calls this directly
		// wait, serveHTTPMatched acquires context. But this handler is the routeData.handler.
		// If this is called from ServeHTTPMatched, the Context has already been acquired.
		// We'll need a mechanism to pass Context to this handler to avoid double-acquire.
		panic("Not implemented: internal handler should not be an http.Handler. See phase 1 refactoring.")
	})

	middlewares := s.inlineMiddlewares
	if len(middlewares) > 0 {
		h = middlewares[len(middlewares)-1](h)
		for i := len(middlewares) - 2; i >= 0; i-- {
			h = middlewares[i](h)
		}
	}

	// Register in the radix tree instead of standard mux
	if s.trees == nil {
		s.trees = make(map[string]*node)
	}
	root := s.trees[method]
	if root == nil {
		root = new(node)
		s.trees[method] = root
	}

	// For now, in Phase 1, we still create a closure that acquires context
	// to maintain compatibility with http.Handler middlewares.
	var finalHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Acquire inner context because http.Handler doesn't pass gofi.Context
		c := s.acquireContext(w, r)
		defer s.releaseContext(c)
		c.setContextSettings(newContextOptions(path, method), s.routeMeta, s.globalStore, s.opts)
		err := composedHandler(c)
		if err != nil {
			s.opts.errHandler(err, c)
		}
	})

	for i := len(middlewares) - 1; i >= 0; i-- {
		finalHandler = middlewares[i](finalHandler)
	}

	data := &routeData{
		handler: finalHandler,
		// rules: comps.rules (will be populated below if Schema exists)
		meta: opts.Meta,
	}

	// Add schema rules to the node data directly
	if opts.Schema != nil {
		rules := s.opts.schemaRules.GetRules(path, method)
		data.rules = rules
	}

	root.addRoute(path, data)

	// Update maxParams
	if root.maxParams > s.maxParams {
		s.maxParams = root.maxParams
	}
}

func newServeMux() *serveMux {
	middlewares := make(Middlewares, 0, 15)
	return serveMuxBuilder(
		make(map[string]*node),
		map[string]map[string]openapiOperationObject{},
		map[string]map[string]any{},
		NewGlobalStore(),
		middlewares,
		defaultMuxOptions(),
	)
}

func serveMuxBuilder(trees map[string]*node, paths docsPaths, rm metaMap, globalStore GofiStore, m Middlewares, opts *muxOptions) *serveMux {
	s := &serveMux{
		trees:             trees,
		paths:             paths,
		routeMeta:         rm,
		globalStore:       globalStore,
		middlewares:       m,
		inlineMiddlewares: make(Middlewares, 0),
		preHandlers:       make([]PreHandler, 0),
		opts:              opts,
		rOpts:             nil,
		ctxPool: &sync.Pool{
			New: func() interface{} {
				// Allocate a new context, but don't set ephemeral fields yet
				return &context{
					routeMeta:         map[string]map[string]any{},
					globalStore:       globalStore, // Shared global store reference
					serverOpts:        opts,        // Shared options reference
					bindedCacheResult: bindedResult{bound: false},
					params:            make(Params, 0, 10), // Pre-allocate param slice
				}
			},
		},
	}
	s.chainedHandler = http.HandlerFunc(s.serveHTTPMatched)
	return s
}
