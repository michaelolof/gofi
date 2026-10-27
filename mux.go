package gofi

import (
	"bufio"
	"bytes"
	stdcontext "context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/michaelolof/gofi/validators/rules"
	"github.com/valyala/fasthttp"
)

// b2s converts []byte to string without allocation.
// The resulting string MUST NOT be retained beyond the lifetime of the []byte.
func b2s(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// joinPath joins a prefix and a relative path with exactly one '/' separator,
// handling all four slash-boundary combinations and empty operands.
func joinPath(prefix, path string) string {
	if prefix == "" {
		return path
	}
	if path == "" {
		return prefix
	}
	pSuffix := strings.HasSuffix(prefix, "/")
	pPrefix := strings.HasPrefix(path, "/")
	switch {
	case pSuffix && pPrefix:
		return prefix + path[1:]
	case !pSuffix && !pPrefix:
		return prefix + "/" + path
	default:
		return prefix + path
	}
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

	// lifecycle is shared by a root router and every child created through
	// With, Route or Group, so a server started on any of them is visible to
	// Shutdown on all of them.
	lifecycle *routerLifecycle
}

type routerLifecycle struct {
	mu           sync.Mutex
	activeServer *FasthttpServer
	sealed       atomic.Bool
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
		if im.prefix != "" {
			im.prefix = joinPath(im.prefix, pattern)
		} else {
			im.prefix = pattern
		}
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
	s.lifecycle.mustBeOpen("Use")
	if n := s.routeCount(); n > 0 {
		log.Printf("gofi: Use called after %d route(s) were registered; middleware applies only to routes registered afterwards", n)
	}
	s.middlewares = append(s.middlewares, middlewares...)
}

func (s *serveMux) With(middlewares ...MiddlewareFunc) Router {
	newMux := *s
	// The child owns a copy of the inherited global middleware. Sharing the
	// parent's backing array would let a later Use on one router overwrite
	// middleware another router appended at the same index.
	newMux.middlewares = make(Middlewares, len(s.middlewares))
	copy(newMux.middlewares, s.middlewares)
	newMux.inlineMiddlewares = make(Middlewares, len(s.inlineMiddlewares), len(s.inlineMiddlewares)+len(middlewares))
	copy(newMux.inlineMiddlewares, s.inlineMiddlewares)
	newMux.inlineMiddlewares = append(newMux.inlineMiddlewares, middlewares...)

	return &newMux
}

// handleFastHTTP is the main fasthttp request handler.
// Called by fasthttp.Server for each incoming connection.
func (s *serveMux) handleFastHTTP(ctx *fasthttp.RequestCtx) {
	defer recoverDispatch(ctx, "request dispatch")
	s.dispatch(ctx)
}

// dispatch routes one request. Callers own panic recovery.
func (s *serveMux) dispatch(ctx *fasthttp.RequestCtx) {
	method := b2s(ctx.Method())
	path := b2s(ctx.Path())

	if rejected := s.rejectRequest(ctx, method); rejected != nil {
		c := s.acquireContext(ctx)
		c.setContextSettings(contextOptions{}, nil, s.globalStore, s.opts)
		s.opts.errHandler(rejected, c)
		c.finishWriter()
		s.releaseContext(c)
		return
	}

	if root := s.trees[method]; root != nil {
		c := s.acquireContext(ctx)

		routeData, tsr := root.getValue(path, func() *Params {
			if cap(c.params) < int(s.maxParams) {
				c.params = make(Params, 0, s.maxParams)
			}
			return &c.params
		})

		if routeData != nil {
			c.setContextSettings(newContextOptions(routeData.pattern, method), s.routeMeta, s.globalStore, s.opts)
			c.handlers = routeData.handlers
			c.handlerIdx = -1

			// Start the middleware/handler chain
			if err := c.Next(); err != nil {
				s.opts.errHandler(err, c)
			}

			// Sync any headers set via the adapter writer to fasthttp response
			c.finishWriter()

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
				if qs := ctx.URI().QueryString(); len(qs) > 0 {
					redirectPath += "?" + b2s(qs)
				}
				// Run the global middleware chain with the redirect as its
				// terminal handler, so global policy (authentication, CORS,
				// logging) covers redirects as it covers 404 and 405.
				c.setContextSettings(newContextOptions(path, method), s.routeMeta, s.globalStore, s.opts)
				allHandlers := make([]HandlerFunc, 0, len(s.middlewares)+1)
				allHandlers = append(allHandlers, s.middlewares...)
				allHandlers = append(allHandlers, func(c Context) error {
					ctx.Redirect(redirectPath, code)
					return nil
				})
				c.handlers = allHandlers
				c.handlerIdx = -1
				if err := c.Next(); err != nil {
					s.opts.errHandler(err, c)
				}
				c.finishWriter()
				if !ctx.Hijacked() {
					s.releaseContext(c)
				}
				return
			}
		}

		// No route matched in this method's tree — run global middleware with not-found terminal.
		// First check if the path is registered under another method (405).
		if s.opts.methodNotAllowed {
			if allow := s.allowedMethods(path, method); allow != "" {
				c.setContextSettings(newContextOptions(path, method), s.routeMeta, s.globalStore, s.opts)
				allHandlers := make([]HandlerFunc, 0, len(s.middlewares)+1)
				allHandlers = append(allHandlers, s.middlewares...)
				allHandlers = append(allHandlers, func(c Context) error {
					c.Writer().Header().Set("Allow", allow)
					return NewHTTPError(http.StatusMethodNotAllowed, "405 method not allowed")
				})
				c.handlers = allHandlers
				c.handlerIdx = -1
				if err := c.Next(); err != nil {
					s.opts.errHandler(err, c)
				}
				c.finishWriter()
				if !ctx.Hijacked() {
					s.releaseContext(c)
				}
				return
			}
		}
		s.runNotFound(ctx, c, path, method)
		return
	}

	// No tree registered for this method — still run global middleware.
	c := s.acquireContext(ctx)

	// 405 check: is this path registered under a different method?
	if s.opts.methodNotAllowed {
		if allow := s.allowedMethods(path, method); allow != "" {
			c.setContextSettings(newContextOptions(path, method), s.routeMeta, s.globalStore, s.opts)
			allHandlers := make([]HandlerFunc, 0, len(s.middlewares)+1)
			allHandlers = append(allHandlers, s.middlewares...)
			allHandlers = append(allHandlers, func(c Context) error {
				c.Writer().Header().Set("Allow", allow)
				return NewHTTPError(http.StatusMethodNotAllowed, "405 method not allowed")
			})
			c.handlers = allHandlers
			c.handlerIdx = -1
			if err := c.Next(); err != nil {
				s.opts.errHandler(err, c)
			}
			c.finishWriter()
			if !ctx.Hijacked() {
				s.releaseContext(c)
			}
			return
		}
	}

	s.runNotFound(ctx, c, path, method)
}

// rejectRequest applies the router's request-target policy before routing.
// Rejected requests reach the error handler without running middleware, as
// transport errors do.
func (s *serveMux) rejectRequest(ctx *fasthttp.RequestCtx, method string) error {
	if !s.opts.allowEncodedSeparators && hasEncodedSeparator(ctx.Request.URI().PathOriginal()) {
		return NewHTTPError(http.StatusBadRequest, "request path contains an encoded separator")
	}
	if (method == http.MethodTrace || method == http.MethodConnect) && s.trees[method] == nil {
		return NewHTTPError(http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed))
	}
	return nil
}

// hasEncodedSeparator reports whether a raw path encodes '/', '\' or NUL,
// directly or double-encoded through %25.
func hasEncodedSeparator(raw []byte) bool {
	for i := 0; i+2 < len(raw); i++ {
		if raw[i] != '%' {
			continue
		}
		hex := strings.ToLower(string(raw[i+1 : i+3]))
		switch hex {
		case "2f", "5c", "00":
			return true
		case "25":
			if i+4 < len(raw) {
				switch strings.ToLower(string(raw[i+3 : i+5])) {
				case "2f", "5c", "00":
					return true
				}
			}
		}
	}
	return false
}

// handleTransportError handles requests rejected before routing can run. The
// request may be incomplete, so only router-wide settings are initialized.
func (s *serveMux) handleTransportError(ctx *fasthttp.RequestCtx, err error) {
	code := http.StatusBadRequest
	var smallBuffer *fasthttp.ErrSmallBuffer
	var netErr net.Error
	switch {
	case errors.Is(err, fasthttp.ErrBodyTooLarge):
		code = http.StatusRequestEntityTooLarge
	case errors.As(err, &smallBuffer):
		code = http.StatusRequestHeaderFieldsTooLarge
	case errors.As(err, &netErr) && netErr.Timeout():
		code = http.StatusRequestTimeout
	}

	defer recoverDispatch(ctx, "transport error handler")

	c := s.acquireContext(ctx)
	c.setContextSettings(contextOptions{}, nil, s.globalStore, s.opts)
	// The default interceptor writes a JSON envelope via SendBytes. Set its
	// content type here; custom interceptors can override it through Context.
	ctx.Response.Header.SetContentType("application/json")
	s.opts.errHandler(&transportError{NewHTTPError(code, http.StatusText(code)), err}, c)
	c.finishWriter()
	s.releaseContext(c)
}

// dummyLogger implements fasthttp.Logger
type dummyLogger struct{}

func (d *dummyLogger) Printf(format string, args ...interface{}) {}

func (s *serveMux) acquireContext(ctx *fasthttp.RequestCtx) *context {
	c := s.ctxPool.Get().(*context)
	c.reset(ctx)
	c.params = c.params[:0]
	c.refs.Store(1)
	return c
}

// releaseContext drops dispatch's hold on c. A stream producer registered
// during dispatch keeps the context (and its bound result) out of the pool
// until the producer returns.
func (s *serveMux) releaseContext(c *context) {
	c.releaseRef()
}

// allowedMethods returns a comma-separated Allow header value listing every
// registered HTTP method that has a matching route for path, excluding the
// method that was actually requested. Returns "" if no other method matches.
func (s *serveMux) allowedMethods(path, requestedMethod string) string {
	var buf []byte
	for m, root := range s.trees {
		if m == requestedMethod {
			continue
		}
		if data, _ := root.getValue(path, nil); data != nil {
			if len(buf) > 0 {
				buf = append(buf, ',', ' ')
			}
			buf = append(buf, m...)
		}
	}
	return string(buf)
}

// runNotFound runs the global middleware chain terminated by a 404 handler.
// This ensures middleware (e.g. CORS) always executes even when no route is matched.
func (s *serveMux) runNotFound(ctx *fasthttp.RequestCtx, c *context, path, method string) {
	c.setContextSettings(newContextOptions(path, method), s.routeMeta, s.globalStore, s.opts)

	allHandlers := make([]HandlerFunc, 0, len(s.middlewares)+1)
	allHandlers = append(allHandlers, s.middlewares...)
	allHandlers = append(allHandlers, func(c Context) error {
		return NewHTTPError(http.StatusNotFound, "404 page not found")
	})

	c.handlers = allHandlers
	c.handlerIdx = -1

	if err := c.Next(); err != nil {
		s.opts.errHandler(err, c)
	}

	c.finishWriter()

	if !ctx.Hijacked() {
		s.releaseContext(c)
	}
}

func (s *serveMux) GlobalStore() GofiStore {
	return s.globalStore
}

func (s *serveMux) Meta() RouterMeta {
	return s.routeMeta
}

func (s *serveMux) UseErrorHandler(handler func(err error, c Context)) {
	s.lifecycle.mustBeOpen("UseErrorHandler")
	if handler != nil {
		s.opts.errHandler = handler
	}
}

func (s *serveMux) RegisterSpec(list ...CustomSpec) {
	s.lifecycle.mustBeOpen("RegisterSpec")
	for _, v := range list {
		s.opts.customSpecs[v.SpecID()] = v
	}
}

func (s *serveMux) RegisterBodyParser(list ...BodyParser) {
	s.lifecycle.mustBeOpen("RegisterBodyParser")
	if len(list) > 0 {
		s.opts.bodyParsers = append(list, s.opts.bodyParsers...)
	}
}

// Static serves files under root at prefix for GET and HEAD. Files are read
// through an os.Root, so symlinks cannot escape root, and are streamed with
// range and conditional request support. Paths with a segment beginning with
// "." are not served, and directories are served only through an index.html.
// If root cannot be opened, the routes answer 404 and the error is logged.
func (s *serveMux) Static(prefix, root string) {
	if s.prefix != "" {
		prefix = joinPath(s.prefix, prefix)
	}

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	handler := newStaticHandler(prefix, root)
	s.method("GET", prefix+"*filepath", RouteOptions{Handler: handler})
	s.method("HEAD", prefix+"*filepath", RouteOptions{Handler: handler})
}

func newStaticHandler(prefix, root string) HandlerFunc {
	notFound := func(c Context) error {
		return NewHTTPError(http.StatusNotFound, http.StatusText(http.StatusNotFound))
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		log.Printf("gofi: Static(%q, %q): %v; serving 404", prefix, root, err)
		return notFound
	}
	fileServer := (&fasthttp.FS{
		FS:                 dir.FS(),
		IndexNames:         []string{"index.html"},
		GenerateIndexPages: false,
		AcceptByteRange:    true,
		// Strip the route prefix but keep the leading slash.
		PathRewrite: fasthttp.NewPathPrefixStripper(len(prefix) - 1),
	}).NewRequestHandler()

	return func(c Context) error {
		name := path.Clean("/" + c.Param("filepath"))
		if strings.Contains(name, "/.") {
			return notFound(c)
		}
		fctx := c.Request().FastHTTPContext()
		fileServer(fctx)
		switch fctx.Response.StatusCode() {
		case http.StatusNotFound, http.StatusForbidden:
			// Missing files and directories without an index.
			fctx.Response.ResetBody()
			return notFound(c)
		}
		return nil
	}
}

// Listen starts the server on the given address using fasthttp.
func (s *serveMux) Listen(addr string) error {
	srv := s.newFasthttpServer()

	s.lifecycle.setActiveServer(srv)

	return srv.Listen(listenAddr(addr))
}

// ListenTLS starts an HTTPS server on the given address.
func (s *serveMux) ListenTLS(addr, certFile, keyFile string) error {
	srv := s.newFasthttpServer()

	s.lifecycle.setActiveServer(srv)

	return srv.ListenTLS(addr, certFile, keyFile)
}

// ListenTLSMutual starts an HTTPS server providing mutual TLS (mTLS) authentication.
func (s *serveMux) ListenTLSMutual(addr, certFile, keyFile, clientCertFile string) error {
	srv := s.newFasthttpServer()

	s.lifecycle.setActiveServer(srv)

	return srv.ListenTLSMutual(addr, certFile, keyFile, clientCertFile)
}

// Shutdown gracefully shuts down the server.
func (s *serveMux) Shutdown() error {
	if srv := s.lifecycle.server(); srv != nil {
		return srv.Shutdown()
	}
	return nil
}

// ShutdownWithContext gracefully shuts down the server with a timeout context.
func (s *serveMux) ShutdownWithContext(ctx stdcontext.Context) error {
	if srv := s.lifecycle.server(); srv != nil {
		return srv.ShutdownWithContext(ctx)
	}
	return nil
}

// seal marks the router as serving. Registration mutates maps and slices
// that requests read without locks, so it must finish before serving starts.
func (l *routerLifecycle) seal() {
	l.sealed.Store(true)
}

func (l *routerLifecycle) mustBeOpen(what string) {
	if l.sealed.Load() {
		panic("gofi: " + what + " called after the router started serving; register routes, middleware and configuration before Listen, ListenTLS, ListenTLSMutual or Handler")
	}
}

func (s *serveMux) routeCount() int {
	n := 0
	for _, root := range s.trees {
		if root != nil {
			n++
		}
	}
	return n
}

// Seal stops further registration on r and on every router derived from it.
// Listen, ListenTLS, ListenTLSMutual and Handler seal automatically; call
// Seal earlier to catch late registration in tests or setup code.
func Seal(r Router) error {
	m, ok := r.(*serveMux)
	if !ok {
		return errors.New("gofi: Seal supports routers created by gofi only")
	}
	m.lifecycle.seal()
	return nil
}

func (l *routerLifecycle) setActiveServer(srv *FasthttpServer) {
	l.mu.Lock()
	l.activeServer = srv
	l.mu.Unlock()
}

func (l *routerLifecycle) server() *FasthttpServer {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.activeServer
}

// Handler returns the raw fasthttp.RequestHandler for this router.
//
// Calling Handler seals the router: later route, middleware or configuration
// registration panics.
func (s *serveMux) Handler() fasthttp.RequestHandler {
	s.lifecycle.seal()
	return s.handleFastHTTP
}

type ValidatorContext = rules.ValidatorContext

func (s *serveMux) RegisterValidator(list ...Validator) {
	s.lifecycle.mustBeOpen("RegisterValidator")
	for _, v := range list {
		s.opts.customValidators[v.Name()] = v.Rule
	}
}

type TestOptions struct {
	Path    string
	Method  string
	Query   map[string]string
	Paths   map[string]string
	Headers map[string]string
	Cookies []http.Cookie
	Body    io.Reader
}

func resolveTestPath(path string, paths map[string]string) string {
	if len(paths) == 0 {
		return path
	}

	resolved := path
	for key, value := range paths {
		resolved = strings.ReplaceAll(resolved, "{"+key+"}", value)
		resolved = strings.ReplaceAll(resolved, ":"+key, value)
		resolved = strings.ReplaceAll(resolved, "*"+key, value)
	}

	return resolved
}

// Test dispatches a request through the full route tree and returns an InjectResponse.
func (s *serveMux) Test(opts TestOptions) (resp *InjectResponse, err error) {
	if opts.Method == "" {
		return nil, fmt.Errorf("gofi.Test: Method is required")
	}
	if opts.Path == "" {
		return nil, fmt.Errorf("gofi.Test: Path is required")
	}

	var reqBody []byte
	if opts.Body != nil {
		reqBody, err = io.ReadAll(opts.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read test body: %w", err)
		}
	}

	var rawReq bytes.Buffer
	path := resolveTestPath(opts.Path, opts.Paths)
	if len(opts.Query) > 0 {
		qParts := make([]string, 0, len(opts.Query))
		for key, value := range opts.Query {
			qParts = append(qParts, key+"="+value)
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

	var fctx fasthttp.RequestCtx
	fctx.Init2(nil, &dummyLogger{}, false)
	if err := fctx.Request.Read(bufio.NewReader(bytes.NewReader(rawReq.Bytes()))); err != nil {
		return nil, fmt.Errorf("failed to build test request: %w", err)
	}

	func() {
		// Mirror live dispatch: a recovered panic produces the same generic
		// response, and Test additionally reports it to the caller.
		defer func() {
			if r := recover(); r != nil {
				err = NewHTTPError(500, fmt.Sprintf("panic recovered in Test: %v", r))
				writeInternalServerError(&fctx)
			}
		}()

		s.dispatch(&fctx)
	}()

	headerMap := make(http.Header)
	for key, val := range fctx.Response.Header.All() {
		headerMap.Add(string(key), string(val))
	}

	resp = &InjectResponse{
		StatusCode: fctx.Response.StatusCode(),
		HeaderMap:  headerMap,
		Body:       fctx.Response.Body(),
	}

	return resp, err
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

	// Compile schema and metadata for this call only. Inject never writes to
	// the router's rule or metadata maps, which live routes read.
	var routeMeta metaMap
	if opts.Handler.Meta != nil {
		routeMeta = metaMap{opts.Path: {strings.ToLower(opts.Method): opts.Handler.Meta}}
	}
	c.setContextSettings(newContextOptions(opts.Path, opts.Method), routeMeta, s.globalStore, s.opts)
	if def.Schema != nil {
		compiled := s.compileSchema(def.Schema, def.Info)
		compiled.specs.normalize(opts.Method, opts.Path)
		c.rulesOverride = &compiled.rules
	}

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
				err = NewHTTPError(500, fmt.Sprintf("panic recovered in Inject: %v", r))
				s.opts.errHandler(err, c)
			}
		}()
		if handlerErr := c.Next(); handlerErr != nil {
			s.opts.errHandler(handlerErr, c)
		}
	}()

	// Write adapter headers back exactly as live dispatch does, then build
	// the InjectResponse from the native response.
	c.finishWriter()
	respHeaders := make(http.Header)
	for key, value := range fctx.Response.Header.All() {
		respHeaders.Add(string(key), string(value))
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
	s.lifecycle.mustBeOpen("route registration")
	if s.prefix != "" {
		path = joinPath(s.prefix, path)
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
		pattern:  path,
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
		newGlobalStore(),
		middlewares,
		defaultMuxOptions(),
	)
}

func (s *serveMux) Configure(config Config) {
	s.lifecycle.mustBeOpen("Configure")
	if config.BodyLimit > 0 {
		s.opts.bodyLimit = config.BodyLimit
	}
	if config.MethodNotAllowed != nil {
		s.opts.methodNotAllowed = *config.MethodNotAllowed
	}
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
		lifecycle:         &routerLifecycle{},
	}
	pool := &sync.Pool{}
	pool.New = func() interface{} {
		return &context{
			routeMeta:         map[string]map[string]any{},
			globalStore:       globalStore,
			serverOpts:        opts,
			bindedCacheResult: bindedResult{bound: false},
			params:            make(Params, 0, 10),
			pool:              pool,
		}
	}
	s.ctxPool = pool
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

// convertFasthttpToHTTPRequest builds a server-style *http.Request from the
// context for net/http bridge usage. It returns an error instead of a nil
// request when the request target cannot be represented as a net/url URL
// (for example an invalid percent escape).
func convertFasthttpToHTTPRequest(c Context) (*http.Request, error) {
	req := c.Request()
	fctx := req.FastHTTPContext()
	r, err := http.NewRequestWithContext(c.Context(), req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	for key, value := range fctx.Request.Header.All() {
		r.Header.Add(string(key), string(value))
	}
	// Match net/http server requests, which carry Host outside the header map.
	r.Header.Del("Host")
	r.Host = string(fctx.Host())
	r.RequestURI = string(fctx.RequestURI())
	r.RemoteAddr = fctx.RemoteAddr().String()
	r.ContentLength = int64(fctx.Request.Header.ContentLength())
	if r.ContentLength < 0 {
		r.ContentLength = 0
	}
	if fctx.IsTLS() {
		r.TLS = fctx.TLSConnectionState()
	}
	if !fctx.Request.Header.IsHTTP11() {
		r.Proto, r.ProtoMajor, r.ProtoMinor = "HTTP/1.0", 1, 0
	}
	return r, nil
}
