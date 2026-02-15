package gofi

import (
	"net/http"
	"reflect"
)

type bindedResult struct {
	bound bool
	err   error
	val   any
}

type Context interface {
	// Returns the http writer instance for the request
	Writer() http.ResponseWriter
	// Returns the http request instance for the request
	Request() *http.Request
	// Access global store defined on the server router instance
	GlobalStore() ReadOnlyStore
	// Access route context data store. Useful for passing and retrieving during a request lifetime
	DataStore() GofiStore
	// Access static meta information defined on the route
	Meta() ContextMeta
	// Sends a schema response object for the given status code
	Send(code int, obj any) error

	SendString(code int, s string) error

	// SendBytes(code int, b []byte) error

	GetSchemaRules(pattern, method string) any
}

type contextOptions struct {
	Pattern string
	Method  string
}

func newContextOptions(patt, method string) contextOptions {
	return contextOptions{Pattern: patt, Method: method}
}

type context struct {
	w    http.ResponseWriter
	r    *http.Request
	opts contextOptions
	// rules             *schemaRules
	routeMeta         metaMap
	globalStore       ReadOnlyStore
	dataStore         GofiStore
	serverOpts        *muxOptions
	bindedCacheResult bindedResult
}

func newContext(w http.ResponseWriter, r *http.Request) *context {
	return &context{
		w:                 w,
		r:                 r,
		routeMeta:         map[string]map[string]any{},
		globalStore:       NewGlobalStore(),
		dataStore:         NewDataStore(),
		serverOpts:        defaultMuxOptions(),
		bindedCacheResult: bindedResult{bound: false},
	}
}

func (c *context) Writer() http.ResponseWriter {
	return c.w
}

func (c *context) Request() *http.Request {
	return c.r
}

func (c *context) GlobalStore() ReadOnlyStore {
	return c.globalStore
}

func (c *context) DataStore() GofiStore {
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
	c.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.w.WriteHeader(code)
	_, err := c.w.Write([]byte(s))
	return err
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

type walkFinishStatus struct{}

var walkFinished = walkFinishStatus{}

const DEFAULT_ARRAY_SIZE = 50

func bindValOnElem(strct *reflect.Value, val any) {
	if val == nil {
		return
	}

	switch strct.Kind() {
	case reflect.Pointer:
		if v, ok := val.([]any); ok {
			nslice := reflect.New(strct.Type().Elem())
			istrct := strct.Type().Elem().Elem()
			slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
			for _, item := range v {
				ssf := reflect.New(istrct).Elem()
				bindValOnElem(&ssf, item)
				slice = reflect.Append(slice, ssf)
			}
			nslice.Elem().Set(slice)
			strct.Set(nslice)
		}

	case reflect.Slice, reflect.Array:
		if v, ok := val.([]any); ok {
			istrct := strct.Type().Elem()
			switch istrct.Kind() {
			case reflect.Pointer:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct.Elem()).Elem()
					bindValOnElem(&ssf, item)
					slice = reflect.Append(slice, ssf.Addr())
				}
				strct.Set(slice)

			default:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct).Elem()
					bindValOnElem(&ssf, item)
					slice = reflect.Append(slice, ssf)
				}
				strct.Set(slice)
			}
		}

	default:
		strct.Set(reflect.ValueOf(val).Convert(strct.Type()))
	}
}
