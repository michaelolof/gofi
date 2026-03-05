package gofi

import (
	"net/http"
	"reflect"

	"github.com/michaelolof/gofi/utils"
	"github.com/valyala/fasthttp"
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
	// Access global store defined on the server router instance
	GlobalStore() ReadOnlyStore
	// Access route context data store. Useful for passing and retrieving during a request lifetime
	DataStore() GofiStore
	// Access static meta information defined on the route
	Meta() ContextMeta
	// Sends a schema response object for the given status code
	Send(code int, obj any) error
	SendString(code int, s string) error
	SendBytes(code int, b []byte) error
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
	// Method returns the HTTP method
	Method() string
	// QueryBytes returns the query parameter value as raw bytes (zero-copy from fasthttp)
	QueryBytes(name string) []byte
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
	opts              contextOptions
	routeMeta         metaMap
	globalStore       ReadOnlyStore
	dataStore         *gofiStore
	serverOpts        *muxOptions
	bindedCacheResult bindedResult
	params            Params
	handlers          []HandlerFunc
	handlerIdx        int
	rw                *responseWriter // cached response writer adapter
	req               *Request        // cached request adapter
}

func newContext() *context {
	return &context{
		routeMeta:         map[string]map[string]any{},
		serverOpts:        defaultMuxOptions(),
		bindedCacheResult: bindedResult{bound: false},
		params:            make(Params, 0, 10),
	}
}

func (c *context) reset(fctx *fasthttp.RequestCtx) {
	if c.bindedCacheResult.bound && c.bindedCacheResult.val != nil {
		if rules := c.rules(); rules != nil && rules.schemaPool != nil {
			// Zero out the struct before returning it to the pool
			reflect.ValueOf(c.bindedCacheResult.val).Elem().SetZero()
			rules.schemaPool.Put(c.bindedCacheResult.val)
		}
	}

	c.fctx = fctx
	c.opts = contextOptions{}
	c.routeMeta = nil
	c.dataStore = nil // Lazy: only allocate on first DataStore() access
	c.bindedCacheResult = bindedResult{bound: false}
	c.params = c.params[:0]
	c.handlers = nil
	c.handlerIdx = -1
	c.rw = nil
	c.req = nil
}

func (c *context) Writer() ResponseWriter {
	if c.rw == nil {
		c.rw = newResponseWriter(c.fctx)
	}
	return c.rw
}

func (c *context) Request() *Request {
	if c.req == nil {
		c.req = newRequest(c.fctx)
	}
	return c.req
}

func (c *context) GlobalStore() ReadOnlyStore {
	return c.globalStore
}

func (c *context) DataStore() GofiStore {
	if c.dataStore == nil {
		c.dataStore = NewDataStore()
	}
	return c.dataStore
}

func (c *context) Meta() ContextMeta {
	return &contextMeta{c: c}
}

func (c *context) GetSchemaRules(pattern, method string) any {
	rulesMap := c.serverOpts.schemaRules
	return rulesMap.GetRules(pattern, method)
}

func (c *context) SendString(code int, s string) error {
	c.fctx.Response.Header.Set("Content-Type", "text/plain; charset=utf-8")
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

func (c *context) HeaderBytes(name string) []byte {
	return c.fctx.Request.Header.Peek(name)
}

func (c *context) Path() string {
	return string(c.fctx.Path())
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

func bindValOnElem(strct *reflect.Value, val any) error {
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
				if err := bindValOnElem(&ssf, item); err != nil {
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
					if err := bindValOnElem(&ssf, item); err != nil {
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
					if err := bindValOnElem(&ssf, item); err != nil {
						return err
					}
					slice = reflect.Append(slice, ssf)
				}
				strct.Set(slice)
				return nil
			}
		}

	default:
		v, err := utils.SafeConvert(reflect.ValueOf(val), strct.Type())
		if err != nil {
			return err
		}
		strct.Set(v)
		return nil
	}

	return nil
}
