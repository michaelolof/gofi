package gofi

import (
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/valyala/fasthttp"
)

// ResponseWriter wraps fasthttp response to implement http.ResponseWriter for backward compatibility.
type ResponseWriter interface {
	http.ResponseWriter
}

// responseWriter implements ResponseWriter by wrapping fasthttp.RequestCtx
type responseWriter struct {
	ctx        *fasthttp.RequestCtx
	header     http.Header
	headerSync bool // whether headers have been synced to fasthttp
	headerInit bool // whether header map was allocated
}

func newResponseWriter(ctx *fasthttp.RequestCtx) *responseWriter {
	return &responseWriter{
		ctx: ctx,
	}
}

// reset reuses the responseWriter with a new fasthttp context, avoiding struct re-allocation.
func (w *responseWriter) reset(ctx *fasthttp.RequestCtx) {
	w.ctx = ctx
	w.headerSync = false
	w.headerInit = false
	w.header = nil
}

func (w *responseWriter) Header() http.Header {
	if !w.headerInit {
		w.header = make(http.Header)
		w.headerInit = true
	}
	return w.header
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.headerSync {
		w.syncHeaders()
	}
	return w.ctx.Write(b)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	if !w.headerSync {
		w.syncHeaders()
	}
	w.ctx.Response.SetStatusCode(statusCode)
}

func (w *responseWriter) syncHeaders() {
	if !w.headerInit {
		return // no headers were ever set via the adapter
	}
	w.headerSync = true
	for key, vals := range w.header {
		for _, val := range vals {
			w.ctx.Response.Header.Add(key, val)
		}
	}
}

// Request wraps a fasthttp.RequestCtx to provide *http.Request-like access for backward compatibility.
type Request struct {
	ctx             *fasthttp.RequestCtx
	Header          requestHeader
	Method          string
	URL             *requestURL
	Body            io.ReadCloser
	PostForm        url.Values
	MultipartForm   *multipart.Form
	parsedForm      bool
	parsedMultipart bool
}

// requestHeader provides http.Header-like access to fasthttp request headers.
type requestHeader struct {
	ctx *fasthttp.RequestCtx
}

func (h requestHeader) Get(key string) string {
	return string(h.ctx.Request.Header.Peek(key))
}

func (h requestHeader) Set(key, value string) {
	h.ctx.Request.Header.Set(key, value)
}

func (h requestHeader) Add(key, value string) {
	h.ctx.Request.Header.Add(key, value)
}

func (h requestHeader) Del(key string) {
	h.ctx.Request.Header.Del(key)
}

func (h requestHeader) Values(key string) []string {
	var vals []string
	h.ctx.Request.Header.VisitAll(func(k, v []byte) {
		if strings.EqualFold(string(k), key) {
			vals = append(vals, string(v))
		}
	})
	return vals
}

// requestURL provides url.URL-like access to fasthttp request URI.
type requestURL struct {
	ctx *fasthttp.RequestCtx
}

func (u *requestURL) String() string {
	return string(u.ctx.RequestURI())
}

func (u *requestURL) Path() string {
	return string(u.ctx.Path())
}

func (u *requestURL) RawQuery() string {
	return string(u.ctx.QueryArgs().QueryString())
}

func (u *requestURL) Query() requestQuery {
	return requestQuery{ctx: u.ctx}
}

// requestQuery provides url.Values-like access to query params.
type requestQuery struct {
	ctx *fasthttp.RequestCtx
}

func (q requestQuery) Get(key string) string {
	return string(q.ctx.QueryArgs().Peek(key))
}

func (q requestQuery) Has(key string) bool {
	return q.ctx.QueryArgs().Has(key)
}

// newRequest creates a Request adapter from a fasthttp.RequestCtx.
func newRequest(ctx *fasthttp.RequestCtx) *Request {
	return &Request{
		ctx:    ctx,
		Header: requestHeader{ctx: ctx},
		Method: string(ctx.Method()),
		URL:    &requestURL{ctx: ctx},
		Body:   io.NopCloser(&bodyReader{ctx: ctx}),
	}
}

// bodyReader reads the request body from fasthttp.
type bodyReader struct {
	ctx  *fasthttp.RequestCtx
	read bool
	data []byte
	pos  int
}

func (r *bodyReader) Read(p []byte) (int, error) {
	if !r.read {
		r.data = r.ctx.PostBody()
		r.read = true
	}
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// Cookie returns the named cookie from the request.
func (r *Request) Cookie(name string) (*http.Cookie, error) {
	val := r.ctx.Request.Header.Cookie(name)
	if len(val) == 0 {
		return nil, http.ErrNoCookie
	}
	return &http.Cookie{
		Name:  name,
		Value: string(val),
	}, nil
}

// Context returns the underlying fasthttp.RequestCtx.
func (r *Request) Context() *fasthttp.RequestCtx {
	return r.ctx
}

// FastHTTPContext returns the underlying fasthttp.RequestCtx explicitely.
// Useful for accessing advanced, low-level server primitives natively.
func (r *Request) FastHTTPContext() *fasthttp.RequestCtx {
	return r.ctx
}

// ParseForm parses the URL query parameters and POST body as form data.
func (r *Request) ParseForm() error {
	if r.parsedForm {
		return nil
	}
	r.parsedForm = true
	r.PostForm = make(url.Values)
	// Parse POST args from fasthttp
	for key, value := range r.ctx.PostArgs().All() {
		r.PostForm.Add(string(key), string(value))
	}
	return nil
}

// ParseMultipartForm parses the multipart form data from the request body.
func (r *Request) ParseMultipartForm(maxMemory int64) error {
	if r.parsedMultipart {
		return nil
	}
	r.parsedMultipart = true

	form, err := r.ctx.MultipartForm()
	if err != nil {
		return err
	}
	r.MultipartForm = form
	return nil
}

// WrapMiddleware converts a standard net/http middleware into a Gofi MiddlewareFunc.
// This enables backward compatibility with existing net/http middleware libraries.
func WrapMiddleware(mw func(http.Handler) http.Handler) MiddlewareFunc {
	return func(c Context) error {
		var innerErr error
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			innerErr = c.Next()
		})

		wrapped := mw(inner)

		// Create a temporary http.ResponseWriter + *http.Request for the net/http middleware
		w := c.Writer()
		rw, ok := w.(http.ResponseWriter)
		if !ok {
			return c.Next()
		}

		wrapped.ServeHTTP(rw, nil)
		return innerErr
	}
}

// InjectResponse represents the result of an Inject() call, replacing httptest.ResponseRecorder.
type InjectResponse struct {
	StatusCode int
	HeaderMap  http.Header
	Body       []byte
}

func (r *InjectResponse) Code() int {
	return r.StatusCode
}

func (r *InjectResponse) Result() *InjectResponse {
	return r
}

// Cookies parses the Set-Cookie headers and returns the cookies.
func (r *InjectResponse) Cookies() []*http.Cookie {
	var cookies []*http.Cookie
	for _, v := range r.HeaderMap.Values("Set-Cookie") {
		header := http.Header{}
		header.Add("Set-Cookie", v)
		resp := &http.Response{Header: header}
		cookies = append(cookies, resp.Cookies()...)
	}
	return cookies
}
