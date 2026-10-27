package gofi

import (
	"bufio"
	stdcontext "context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/michaelolof/gofi/utils"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

// Pre-allocated content-type byte slices for zero-alloc header setting
var (
	contentTypePlain = []byte("text/plain; charset=utf-8")
)

type bindedResult struct {
	bound bool
	err   error
	val   any
}

type Context interface {
	// Writer returns a ResponseWriter for backward compatibility (implements http.ResponseWriter)
	Writer() ResponseWriter
	// Request returns a Request adapter for backward compatibility
	Request() *Request
	// GlobalStore accesses the global store defined on the server router
	// instance. It is exposed here as read-only: writes must happen once,
	// during router setup via Router.GlobalStore(), before the server starts
	// serving requests — the store is not safe for concurrent writes. See the
	// concurrency notes on GofiStore.Set / StoreSet.
	GlobalStore() ReadOnlyStore
	// Access route context data store. Useful for passing and retrieving during a request lifetime
	DataStore() GofiStore
	// Access static meta information defined on the route
	Meta() ContextMeta
	// Sends a schema response object for the given status code
	Send(code int, obj any) error
	SendString(code int, s string) error
	SendBytes(code int, b []byte) error

	// SetBodyStreamWriter registers a chunked stream writer for the response
	// body and returns without waiting for it. The writer runs on its own
	// goroutine; the context stays valid for it until it returns. Its error is
	// delivered to OnStreamError, not returned here.
	SetBodyStreamWriter(sw func(w *bufio.Writer) error) error
	// SendStream simplifies SSE. It validates and sets headers from the schema
	// definition, then registers sw as SetBodyStreamWriter does.
	SendStream(code int, s any, sw func(w *bufio.Writer) error) error

	GetSchemaRules(pattern, method string) any
	// Next calls the next handler in the middleware chain
	Next() error
	// Param returns the named path parameter value
	Param(name string) string
	// Query returns the query parameter value
	Query(name string) string
	// HeaderVal returns the request header value
	HeaderVal(name string) string
	// HeaderBytes returns the request header value as raw bytes (zero-copy from fasthttp)
	HeaderBytes(name string) []byte
	// Body returns the raw request body bytes
	Body() []byte
	// Path returns the request URL path
	Path() string
	// Pattern returns the registered route pattern that matched the request
	Pattern() string
	// Method returns the HTTP method
	Method() string
	// QueryBytes returns the query parameter value as raw bytes (zero-copy from fasthttp)
	QueryBytes(name string) []byte
	// HasResponseHeader reports whether a response header with the given key has already been
	// set — either by a middleware writing directly to the fasthttp response or via c.Writer().
	// Useful for middleware idempotency checks where a header may have been set upstream.
	HasResponseHeader(key string) bool
	// Context returns the request's Go context. On a live context it is the
	// fasthttp request context unless middleware replaced it with
	// SetRequestContext (for example ContextTimeout). The fasthttp context is
	// cancelled only when the server shuts down: it is not cancelled when the
	// client disconnects, and it carries no deadline. On a Copy()-ed context
	// it is context.Background().
	Context() stdcontext.Context
	// getParser returns the context-bound fastjson.Parser, securely isolating JSON parsing memory per request
	getParser() *fastjson.Parser

	// Copy returns a Context for use after the handler returns, for example in
	// a background goroutine. It owns copies of the request (method, URI,
	// headers, body), path parameters, route pattern and peer address, and a
	// new data store holding the same item values. It is not a deep clone:
	// values stored in the data store or global store, and a result already
	// returned by ValidateAndBind, are shared with the original. Its Go
	// context is context.Background(), and it reports no TLS state.
	Copy() Context
}

type contextOptions struct {
	Pattern string
	Method  string
}

func newContextOptions(patt, method string) contextOptions {
	return contextOptions{Pattern: patt, Method: method}
}

type context struct {
	fctx              *fasthttp.RequestCtx
	stdCtx            stdcontext.Context // explicitly managed Go context; nil on live contexts (lazily read from fctx), non-nil on Copy()-ed contexts
	opts              contextOptions
	routeMeta         metaMap
	globalStore       ReadOnlyStore
	dataStore         *gofiStore
	serverOpts        *muxOptions
	bindedCacheResult bindedResult
	params            Params
	handlers          []HandlerFunc
	handlerIdx        int
	rw                *responseWriter  // cached response writer adapter
	req               *Request         // cached request adapter
	jsonParser        *fastjson.Parser // request-bound fastjson parser

	// pool is the pool this context returns to; nil for Copy()-ed contexts.
	pool *sync.Pool
	// refs counts holders of a pooled context: dispatch plus every stream
	// producer still running. The context returns to the pool at zero.
	refs atomic.Int32
	// streamErrHandler receives terminal stream producer errors (OnStreamError).
	streamErrHandler func(err error)
}

func (c *context) reset(fctx *fasthttp.RequestCtx) {
	c.fctx = fctx
	c.stdCtx = nil // cleared on each request; live context reads from fctx lazily
	c.opts = contextOptions{}
	c.routeMeta = nil
	c.dataStore = nil // Lazy: only allocate on first DataStore() access
	c.bindedCacheResult = bindedResult{bound: false}
	c.params = c.params[:0]
	c.handlers = nil
	c.handlerIdx = -1
	c.streamErrHandler = nil

	// Reset adapters in-place instead of nil-ing to avoid re-allocation
	if c.rw != nil {
		c.rw.reset(fctx)
	}
	if c.req != nil {
		c.req = nil // Request adapter is complex; nil and lazy-init
	}
}

func (c *context) Writer() ResponseWriter {
	if c.rw == nil {
		c.rw = newResponseWriter(c.fctx)
	}
	return c.rw
}

func (c *context) Request() *Request {
	if c.req == nil {
		c.req = newRequest(c.fctx, c.opts.Pattern)
	}
	return c.req
}

func (c *context) GlobalStore() ReadOnlyStore {
	return c.globalStore
}

func (c *context) DataStore() GofiStore {
	if c.dataStore == nil {
		c.dataStore = newDataStore()
	}
	return c.dataStore
}

func (c *context) Meta() ContextMeta {
	return &contextMeta{c: c}
}

func (c *context) getParser() *fastjson.Parser {
	if c.jsonParser == nil {
		var p fastjson.Parser
		c.jsonParser = &p
	}
	return c.jsonParser
}

func (c *context) Copy() Context {
	cc := &context{
		// Method and Pattern can be views into the pooled request buffer (the
		// pattern is the raw path on 404/405 chains); the copy owns them.
		opts: contextOptions{
			Pattern: strings.Clone(c.opts.Pattern),
			Method:  strings.Clone(c.opts.Method),
		},
		routeMeta:   c.routeMeta,
		globalStore: c.globalStore,
		serverOpts:  c.serverOpts,
		// Bound results are never recycled, so sharing the pointer cannot
		// expose another request's data.
		bindedCacheResult: c.bindedCacheResult,
		handlerIdx:        c.handlerIdx,
		fctx:              new(fasthttp.RequestCtx),
	}

	// Preserve handler chain state so copied contexts can continue middleware flow.
	if len(c.handlers) > 0 {
		cc.handlers = make([]HandlerFunc, len(c.handlers))
		copy(cc.handlers, c.handlers)
	}

	// Copy path parameters. Values are views into the request path buffer,
	// which the next request on the connection overwrites, so clone them.
	if len(c.params) > 0 {
		cc.params = make(Params, len(c.params))
		for i, p := range c.params {
			cc.params[i] = Param{Key: p.Key, Value: strings.Clone(p.Value)}
		}
	}

	// Detach the request from the pooled RequestCtx. Init copies the request,
	// keeps the peer address, and attaches fasthttp's placeholder server so
	// the copy's RequestCtx is safe to use as a context.Context.
	cc.fctx.Init(&c.fctx.Request, c.fctx.RemoteAddr(), nil)

	// Detach the Go context from the connection lifecycle. The copied context
	// must not share the original connection's Done channel — the connection
	// may close (and cancel the original context) while async work is still
	// running. context.Background() is safe and non-nil for all consumers.
	cc.stdCtx = stdcontext.Background()

	// Copy datastore if initialized already
	if c.dataStore != nil {
		cc.dataStore = newDataStore()
		if len(c.dataStore.items) > 0 {
			cc.dataStore.items = make([]kv, len(c.dataStore.items))
			copy(cc.dataStore.items, c.dataStore.items)
		}
	}

	return cc
}

// releaseRef drops one hold on a pooled context and returns it to its pool
// when no dispatch or stream producer still uses it.
func (c *context) releaseRef() {
	if c.refs.Add(-1) == 0 && c.pool != nil {
		c.pool.Put(c)
	}
}

// finishWriter flushes adapter headers into the response. If a header was
// rejected for containing CR, LF or NUL, the response fails with a generic 500
// instead of being sent without it.
func (c *context) finishWriter() {
	if c.rw == nil {
		return
	}
	c.rw.syncHeaders()
	if c.rw.rejectedHeader {
		log.Println("gofi: rejected a response header containing CR, LF or NUL")
		writeInternalServerError(c.fctx)
	}
}

func (c *context) GetSchemaRules(pattern, method string) any {
	rulesMap := c.serverOpts.schemaRules
	return rulesMap.GetRules(pattern, method)
}

func (c *context) SendString(code int, s string) error {
	c.fctx.Response.Header.SetContentTypeBytes(contentTypePlain)
	c.fctx.Response.SetStatusCode(code)
	c.fctx.Response.SetBodyString(s)
	return nil
}

func (c *context) SendBytes(code int, b []byte) error {
	c.fctx.Response.SetStatusCode(code)
	c.fctx.Response.SetBody(b)
	return nil
}

func (c *context) Next() error {
	c.handlerIdx++
	if c.handlerIdx < len(c.handlers) {
		return c.handlers[c.handlerIdx](c)
	}
	return nil
}

func (c *context) Param(name string) string {
	return c.params.Get(name)
}

func (c *context) Query(name string) string {
	return string(c.fctx.QueryArgs().Peek(name))
}

func (c *context) HeaderVal(name string) string {
	return string(c.fctx.Request.Header.Peek(name))
}

func (c *context) Body() []byte {
	return c.fctx.PostBody()
}

func (c *context) QueryBytes(name string) []byte {
	return c.fctx.QueryArgs().Peek(name)
}

func (c *context) HasResponseHeader(key string) bool {
	return c.headerAlreadyWritten(key)
}

func (c *context) Context() stdcontext.Context {
	// For Copy()-ed contexts stdCtx is pre-set to context.Background();
	// SetRequestContext also sets it.
	if c.stdCtx != nil {
		return c.stdCtx
	}
	// For live contexts, delegate to the fasthttp request context, which is
	// cancelled on server shutdown only.
	return c.fctx
}

// SetRequestContext replaces the Go context that c.Context() returns for the
// rest of the request. Middleware that derives a deadline should restore the
// previous context when its downstream chain returns. It rejects a nil
// context and contexts not created by gofi.
func SetRequestContext(c Context, ctx stdcontext.Context) error {
	if ctx == nil {
		return errors.New("gofi: nil request context")
	}
	cc, ok := c.(*context)
	if !ok {
		return errors.New("unknown context object passed")
	}
	cc.stdCtx = ctx
	return nil
}

func (c *context) HeaderBytes(name string) []byte {
	return c.fctx.Request.Header.Peek(name)
}

func (c *context) Path() string {
	return string(c.fctx.Path())
}

func (c *context) Pattern() string {
	return c.opts.Pattern
}

func (c *context) Method() string {
	return string(c.fctx.Method())
}

func (c *context) setContextSettings(opts contextOptions, routeMeta metaMap, globalStore GofiStore, serverOpts *muxOptions) {
	c.opts = opts
	c.routeMeta = routeMeta
	c.globalStore = globalStore
	c.serverOpts = serverOpts
}

func (c *context) rules() *schemaRules {
	return c.serverOpts.schemaRules.GetRules(c.opts.Pattern, c.opts.Method)
}

// headerGet returns a request header value (used internally by requests.go)
func (c *context) headerGet(name string) string {
	return string(c.fctx.Request.Header.Peek(name))
}

// queryGet returns a query param value (used internally by requests.go)
func (c *context) queryGet(name string) string {
	return string(c.fctx.QueryArgs().Peek(name))
}

// cookieGet returns a request cookie (used internally by requests.go)
func (c *context) cookieGet(name string) (*http.Cookie, error) {
	val := c.fctx.Request.Header.Cookie(name)
	if len(val) == 0 {
		return nil, http.ErrNoCookie
	}
	return &http.Cookie{
		Name:  name,
		Value: string(val),
	}, nil
}

type walkFinishStatus struct{}

var walkFinished = walkFinishStatus{}

const DEFAULT_ARRAY_SIZE = 50

// setConvertedReflectValue uses typed setters for the primitive conversions
// performed on nearly every JSON field. Uncommon conversions retain the
// existing SafeConvert fallback and its error behavior.
func setConvertedReflectValue(dst reflect.Value, val any) error {
	src := reflect.ValueOf(val)
	if !src.IsValid() {
		return fmt.Errorf("cannot convert invalid value to %v", dst.Type())
	}

	switch dst.Kind() {
	case reflect.String:
		if src.Kind() == reflect.String {
			dst.SetString(src.String())
			return nil
		}
	case reflect.Bool:
		if src.Kind() == reflect.Bool {
			dst.SetBool(src.Bool())
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		// Reject overflow, sign and fractional loss instead of wrapping.
		if ok, err := utils.SetNumberChecked(dst, src); ok {
			return err
		}
	}

	v, err := utils.SafeConvert(src, dst.Type())
	if err != nil {
		return err
	}
	dst.Set(v)
	return nil
}

func bindValOnElem(strct reflect.Value, val any) error {
	if val == nil {
		return nil
	}

	switch strct.Kind() {
	case reflect.Pointer:
		if v, ok := val.([]any); ok {
			nslice := reflect.New(strct.Type().Elem())
			istrct := strct.Type().Elem().Elem()
			slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
			for _, item := range v {
				ssf := reflect.New(istrct).Elem()
				if err := bindValOnElem(ssf, item); err != nil {
					return err
				}
				slice = reflect.Append(slice, ssf)
			}
			nslice.Elem().Set(slice)
			strct.Set(nslice)
			return nil
		}

	case reflect.Slice, reflect.Array:
		if v, ok := val.([]any); ok {
			istrct := strct.Type().Elem()
			switch istrct.Kind() {
			case reflect.Pointer:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct.Elem()).Elem()
					if err := bindValOnElem(ssf, item); err != nil {
						return err
					}
					slice = reflect.Append(slice, ssf.Addr())
				}
				strct.Set(slice)
				return nil

			default:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct).Elem()
					if err := bindValOnElem(ssf, item); err != nil {
						return err
					}
					slice = reflect.Append(slice, ssf)
				}
				strct.Set(slice)
				return nil
			}
		}

	default:
		return setConvertedReflectValue(strct, val)
	}

	return nil
}
