package gofi

import (
	"bufio"
	"bytes"
	stdcontext "context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"unsafe"

	"github.com/michaelolof/gofi/validators/rules"
	"github.com/valyala/fasthttp"
)

// b2s converts []byte to string without allocation.
// The resulting string MUST NOT be retained beyond the lifetime of the []byte.
func b2s(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

type serveMux struct {
	trees             map[string]*node
	rOpts             *RouteOptions
	opts              *muxOptions
	paths             docsPaths
	routeMeta         metaMap
	globalStore       GofiStore
	middlewares       Middlewares
	inlineMiddlewares Middlewares
	prefix            string
	ctxPool           *sync.Pool
	maxParams         uint8

	activeServer *FasthttpServer
	serverMu     sync.Mutex
}

func NewRouter() Router {
	return newRouter()
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

// Group creates a new inline-Mux with a copy of middleware stack.
func (s *serveMux) Group(fn func(r Router)) Router {
	im := s.With()
	if fn != nil {
		fn(im)
	}
	return im
}

func (s *serveMux) Use(middlewares ...MiddlewareFunc) {
	s.middlewares = append(s.middlewares, middlewares...)
}

func (s *serveMux) With(middlewares ...MiddlewareFunc) Router {
	newMux := *s
	newMux.inlineMiddlewares = make(Middlewares, len(s.inlineMiddlewares), len(s.inlineMiddlewares)+len(middlewares))
	copy(newMux.inlineMiddlewares, s.inlineMiddlewares)
	newMux.inlineMiddlewares = append(newMux.inlineMiddlewares, middlewares...)

	return &newMux
}

// handleFastHTTP is the main fasthttp request handler.
// Called by fasthttp.Server for each incoming connection.
func (s *serveMux) handleFastHTTP(ctx *fasthttp.RequestCtx) {
	method := b2s(ctx.Method())
	path := b2s(ctx.Path())

	if root := s.trees[method]; root != nil {
		c := s.acquireContext(ctx)

		routeData, tsr := root.getValue(path, func() *Params {
			if cap(c.params) < int(s.maxParams) {
				c.params = make(Params, 0, s.maxParams)
			}
			return &c.params
		})

		if routeData != nil {
			c.setContextSettings(newContextOptions(path, method), s.routeMeta, s.globalStore, s.opts)
			c.handlers = routeData.handlers
			c.handlerIdx = -1

			// Start the middleware/handler chain
			if err := c.Next(); err != nil {
				s.opts.errHandler(err, c)
			}

			// Sync any headers set via the adapter writer to fasthttp response
			if c.rw != nil {
				c.rw.syncHeaders()
			}

			if !ctx.Hijacked() {
				s.releaseContext(c)
			}
			return
		} else if method != http.MethodConnect && path != "/" {
			if tsr {
				code := http.StatusMovedPermanently
				if method != http.MethodGet {
					code = http.StatusPermanentRedirect
				}
				redirectPath := path
				if len(redirectPath) > 1 && redirectPath[len(redirectPath)-1] == '/' {
					redirectPath = redirectPath[:len(redirectPath)-1]
				} else {
					redirectPath = redirectPath + "/"
				}
				ctx.Redirect(redirectPath, code)
				if !ctx.Hijacked() {
					s.releaseContext(c)
				}
				return
			}
		}

		if !ctx.Hijacked() {
			s.releaseContext(c)
		}
	}

	// 404
	ctx.SetStatusCode(http.StatusNotFound)
	ctx.SetBodyString("404 page not found\n")
}

// dummyLogger implements fasthttp.Logger
type dummyLogger struct{}

func (d *dummyLogger) Printf(format string, args ...interface{}) {}

// Test dispatches a request through the full route tree and returns an InjectResponse.
func (s *serveMux) Test(method, path string) *InjectResponse {
	var fctx fasthttp.RequestCtx
	fctx.Init2(nil, &dummyLogger{}, false)
	fctx.Request.Header.SetMethod(method)
	fctx.Request.SetRequestURI(path)
	fctx.Request.Header.SetHost("localhost")

	s.handleFastHTTP(&fctx)

	// Collect response headers
	headerMap := make(http.Header)
	for key, val := range fctx.Response.Header.All() {
		headerMap.Add(string(key), string(val))
	}

	return &InjectResponse{
		StatusCode: fctx.Response.StatusCode(),
		HeaderMap:  headerMap,
		Body:       fctx.Response.Body(),
	}
}

func (s *serveMux) acquireContext(ctx *fasthttp.RequestCtx) *context {
	c := s.ctxPool.Get().(*context)
	c.reset(ctx)
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
			// Bridge to net/http file server via adapter
			w := c.Writer()
			rw, ok := w.(http.ResponseWriter)
			if !ok {
				rw = newHTTPResponseWriterShim(c)
			}
			r := convertFasthttpToHTTPRequest(c)
			handler.ServeHTTP(rw, r)
			return nil
		},
	})
}

// Listen starts the server on the given address using fasthttp.
func (s *serveMux) Listen(addr string) error {
	srv := &FasthttpServer{
		mux: s,
		server: &fasthttp.Server{
			Handler:            s.handleFastHTTP,
			Name:               "gofi",
			DisableKeepalive:   false,
			ReduceMemoryUsage:  false,
			MaxRequestBodySize: 4 * 1024 * 1024,
		},
	}

	s.serverMu.Lock()
	s.activeServer = srv
	s.serverMu.Unlock()

	return srv.Listen(listenAddr(addr))
}

// ListenTLS starts an HTTPS server on the given address.
func (s *serveMux) ListenTLS(addr, certFile, keyFile string) error {
	srv := &FasthttpServer{
		mux: s,
		server: &fasthttp.Server{
			Handler:            s.handleFastHTTP,
			Name:               "gofi",
			DisableKeepalive:   false,
			ReduceMemoryUsage:  false,
			MaxRequestBodySize: 4 * 1024 * 1024,
		},
	}

	s.serverMu.Lock()
	s.activeServer = srv
	s.serverMu.Unlock()

	return srv.ListenTLS(addr, certFile, keyFile)
}

// ListenTLSMutual starts an HTTPS server providing mutual TLS (mTLS) authentication.
func (s *serveMux) ListenTLSMutual(addr, certFile, keyFile, clientCertFile string) error {
	srv := &FasthttpServer{
		mux: s,
		server: &fasthttp.Server{
			Handler:            s.handleFastHTTP,
			Name:               "gofi",
			DisableKeepalive:   false,
			ReduceMemoryUsage:  false,
			MaxRequestBodySize: 4 * 1024 * 1024,
		},
	}

	s.serverMu.Lock()
	s.activeServer = srv
	s.serverMu.Unlock()

	return srv.ListenTLSMutual(addr, certFile, keyFile, clientCertFile)
}

// Shutdown gracefully shuts down the server.
func (s *serveMux) Shutdown() error {
	s.serverMu.Lock()
	defer s.serverMu.Unlock()
	if s.activeServer != nil {
		return s.activeServer.Shutdown()
	}
	return nil
}

// ShutdownWithContext gracefully shuts down the server with a timeout context.
func (s *serveMux) ShutdownWithContext(ctx stdcontext.Context) error {
	s.serverMu.Lock()
	defer s.serverMu.Unlock()
	if s.activeServer != nil {
		return s.activeServer.ShutdownWithContext(ctx)
	}
	return nil
}

// Handler returns the raw fasthttp.RequestHandler for this router.
func (s *serveMux) Handler() fasthttp.RequestHandler {
	return s.handleFastHTTP
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

func (s *serveMux) Inject(opts InjectOptions) (resp *InjectResponse, err error) {
	def := opts.Handler
	if def == nil {
		return nil, fmt.Errorf("gofi controller not defined for the given method '%s' and path '%s'", opts.Method, opts.Path)
	}

	// Build a synthetic fasthttp.RequestCtx for testing
	var reqBody []byte
	if opts.Body != nil {
		var err error
		reqBody, err = io.ReadAll(opts.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read inject body: %w", err)
		}
	}

	// Build the raw HTTP request string
	var rawReq bytes.Buffer
	path := opts.Path
	if len(opts.Query) > 0 {
		qParts := make([]string, 0, len(opts.Query))
		for k, v := range opts.Query {
			qParts = append(qParts, k+"="+v)
		}
		path += "?" + strings.Join(qParts, "&")
	}

	rawReq.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", opts.Method, path))
	rawReq.WriteString("Host: localhost\r\n")

	for name, value := range opts.Headers {
		rawReq.WriteString(fmt.Sprintf("%s: %s\r\n", name, value))
	}

	for _, cookie := range opts.Cookies {
		rawReq.WriteString(fmt.Sprintf("Cookie: %s=%s\r\n", cookie.Name, cookie.Value))
	}

	if len(reqBody) > 0 {
		rawReq.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(reqBody)))
	}

	rawReq.WriteString("\r\n")

	if len(reqBody) > 0 {
		rawReq.Write(reqBody)
	}

	// Create fasthttp context from raw request
	var fctx fasthttp.RequestCtx
	fctx.Init2(nil, &dummyLogger{}, false)
	fctx.Request.Read(bufio.NewReader(bytes.NewReader(rawReq.Bytes())))

	c := s.acquireContext(&fctx)
	defer s.releaseContext(c)

	// Set path params
	for name, value := range opts.Paths {
		c.params = append(c.params, Param{Key: name, Value: value})
	}

	// Compile schema if present
	if def.Schema != nil {
		rules := s.compileSchema(def.Schema, def.Info)
		rules.specs.normalize(opts.Method, opts.Path)
		s.opts.schemaRules.SetRules(opts.Path, opts.Method, &rules.rules)
	}

	var routeMeta metaMap
	if opts.Handler.Meta != nil {
		v := map[string]any{strings.ToLower(opts.Method): opts.Handler.Meta}
		s.routeMeta[opts.Path] = v
		routeMeta = s.routeMeta
	}
	c.setContextSettings(newContextOptions(opts.Path, opts.Method), routeMeta, s.globalStore, s.opts)

	// Build flat chain: middlewares + handler
	allHandlers := make([]HandlerFunc, 0, len(s.middlewares)+1)
	allHandlers = append(allHandlers, s.middlewares...)
	allHandlers = append(allHandlers, def.Handler)

	c.handlers = allHandlers
	c.handlerIdx = -1

	// Execute handler chain with explicit panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic recovered in Inject: %v", r)
				fctx.Response.SetStatusCode(500)
			}
		}()
		if handlerErr := c.Next(); handlerErr != nil {
			s.opts.errHandler(handlerErr, c)
		}
	}()

	// Build InjectResponse from fctx
	respHeaders := make(http.Header)
	fctx.Response.Header.VisitAll(func(key, value []byte) {
		respHeaders.Add(string(key), string(value))
	})

	// Also sync any headers that were set via the adapter
	if c.rw != nil {
		for k, vals := range c.rw.header {
			for _, v := range vals {
				respHeaders.Add(k, v)
			}
		}
	}

	resp = &InjectResponse{
		StatusCode: fctx.Response.StatusCode(),
		HeaderMap:  respHeaders,
		Body:       fctx.Response.Body(),
	}

	return resp, err
}

// ListenAndServe starts a fasthttp server with the given router.
func ListenAndServe(addr string, r Router) error {
	return r.Listen(addr)
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

	// Build the flat handler chain: global MW + inline MW + handler
	allHandlers := make([]HandlerFunc, 0, len(s.middlewares)+len(s.inlineMiddlewares)+1)
	allHandlers = append(allHandlers, s.middlewares...)
	allHandlers = append(allHandlers, s.inlineMiddlewares...)
	allHandlers = append(allHandlers, opts.Handler)

	// Register in the radix tree
	if s.trees == nil {
		s.trees = make(map[string]*node)
	}
	root := s.trees[method]
	if root == nil {
		root = new(node)
		s.trees[method] = root
	}

	data := &routeData{
		handlers: allHandlers,
		meta:     opts.Meta,
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

func newRouter() *serveMux {
	middlewares := make(Middlewares, 0, 15)
	return serveRouterBuilder(
		make(map[string]*node),
		map[string]map[string]openapiOperationObject{},
		map[string]map[string]any{},
		NewGlobalStore(),
		middlewares,
		defaultMuxOptions(),
	)
}

func serveRouterBuilder(trees map[string]*node, paths docsPaths, rm metaMap, globalStore GofiStore, m Middlewares, opts *muxOptions) *serveMux {
	s := &serveMux{
		trees:             trees,
		paths:             paths,
		routeMeta:         rm,
		globalStore:       globalStore,
		middlewares:       m,
		inlineMiddlewares: make(Middlewares, 0),
		opts:              opts,
		rOpts:             nil,
		ctxPool: &sync.Pool{
			New: func() interface{} {
				return &context{
					routeMeta:         map[string]map[string]any{},
					globalStore:       globalStore,
					serverOpts:        opts,
					bindedCacheResult: bindedResult{bound: false},
					params:            make(Params, 0, 10),
				}
			},
		},
	}
	return s
}

// newHTTPResponseWriterShim creates a simple http.ResponseWriter for bridging to net/http handlers.
type httpResponseWriterShim struct {
	c Context
}

func newHTTPResponseWriterShim(c Context) *httpResponseWriterShim {
	return &httpResponseWriterShim{c: c}
}

func (w *httpResponseWriterShim) Header() http.Header {
	return w.c.Writer().Header()
}

func (w *httpResponseWriterShim) Write(b []byte) (int, error) {
	return w.c.Writer().Write(b)
}

func (w *httpResponseWriterShim) WriteHeader(statusCode int) {
	w.c.Writer().WriteHeader(statusCode)
}

// convertFasthttpToHTTPRequest builds a *http.Request from the context for net/http bridge usage.
func convertFasthttpToHTTPRequest(c Context) *http.Request {
	req := c.Request()
	r, _ := http.NewRequest(req.Method, req.URL.String(), req.Body)
	return r
}
