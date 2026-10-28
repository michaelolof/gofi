package gofi

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/michaelolof/gofi/utils"
	"github.com/valyala/fasthttp"
)

// ResponseWriter wraps fasthttp response to implement http.ResponseWriter for backward compatibility.
type ResponseWriter interface {
	http.ResponseWriter
}

// responseWriter implements ResponseWriter by wrapping fasthttp.RequestCtx.
//
// Header() is a view of the response headers: on first access it is filled
// with the headers already set on the native response, except headers the
// transport manages (Content-Length, Connection, Date, Server, Trailer,
// Transfer-Encoding) and native cookies. The view is written back once, on the
// first Write or WriteHeader or when the request finishes, as net/http
// commits headers. Only keys changed through the view are written back:
// a changed key replaces the native values of that name, a deleted key removes
// them, and Set-Cookie values are always added. Edits made through the view
// after it is written back have no effect.
type responseWriter struct {
	ctx            *fasthttp.RequestCtx
	header         http.Header
	initial        http.Header // native headers copied into header on first access
	headerSync     bool        // whether headers have been synced to fasthttp
	headerInit     bool        // whether header map was allocated
	rejectedHeader bool        // whether a header containing CR, LF or NUL was dropped
}

// transportManagedHeaders are owned by fasthttp's response framing and are
// not exposed through, or written back from, the Header() view.
var transportManagedHeaders = map[string]bool{
	"Connection":        true,
	"Content-Length":    true,
	"Date":              true,
	"Server":            true,
	"Set-Cookie":        true,
	"Trailer":           true,
	"Transfer-Encoding": true,
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
	w.rejectedHeader = false
	w.header = nil
	w.initial = nil
}

func (w *responseWriter) Header() http.Header {
	if !w.headerInit {
		w.header = make(http.Header)
		w.initial = make(http.Header)
		for key, value := range w.ctx.Response.Header.All() {
			name := http.CanonicalHeaderKey(string(key))
			if transportManagedHeaders[name] {
				continue
			}
			w.header.Add(name, string(value))
			w.initial.Add(name, string(value))
		}
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
	if !w.headerInit || w.headerSync {
		return // no headers ever set, or already flushed to fasthttp
	}
	w.headerSync = true
	native := &w.ctx.Response.Header
	for key := range w.initial {
		if _, ok := w.header[key]; !ok {
			native.Del(key)
		}
	}
	for key, vals := range w.header {
		if !validHeaderField(key) {
			w.rejectedHeader = true
			continue
		}
		cookies := http.CanonicalHeaderKey(key) == "Set-Cookie"
		if !cookies && slices.Equal(vals, w.initial[key]) {
			continue // unchanged through the view; keep later native writes
		}
		if !cookies {
			native.Del(key)
		}
		for _, val := range vals {
			if !validHeaderField(val) {
				w.rejectedHeader = true
				continue
			}
			native.Add(key, val)
		}
	}
}

// validHeaderField reports whether a header name or value is free of CR, LF
// and NUL, which would otherwise allow response splitting.
func validHeaderField(s string) bool {
	return !strings.ContainsAny(s, "\r\n\x00")
}

// Request wraps a fasthttp.RequestCtx to provide *http.Request-like access for backward compatibility.
type Request struct {
	ctx             *fasthttp.RequestCtx
	Method          string
	URL             *requestURL
	Proto           string
	ProtoMajor      int
	ProtoMinor      int
	Header          requestHeader
	Body            io.ReadCloser
	ContentLength   int64
	Host            string
	RemoteAddr      string
	RequestURI      string
	Pattern         string
	Close           bool
	Form            url.Values
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
	for k, v := range h.ctx.Request.Header.All() {
		if strings.EqualFold(string(k), key) {
			vals = append(vals, string(v))
		}
	}
	return vals
}

// requestURL provides url.URL-like access to fasthttp request URI.
type requestURL struct {
	ctx *fasthttp.RequestCtx
}

func (u *requestURL) String() string {
	return utils.BytesToString(u.ctx.RequestURI())
}

func (u *requestURL) Scheme() string {
	return utils.BytesToString(u.ctx.URI().Scheme())
}

func (u *requestURL) Host() string {
	return utils.BytesToString(u.ctx.URI().Host())
}

func (u *requestURL) Path() string {
	return utils.BytesToString(u.ctx.Path())
}

func (u *requestURL) RawQuery() string {
	return utils.BytesToString(u.ctx.QueryArgs().QueryString())
}

func (u *requestURL) Query() requestQuery {
	return requestQuery{ctx: u.ctx}
}

// requestQuery provides url.Values-like access to query params.
type requestQuery struct {
	ctx *fasthttp.RequestCtx
}

func (q requestQuery) Get(key string) string {
	return utils.BytesToString(q.ctx.QueryArgs().Peek(key))
}

func (q requestQuery) Has(key string) bool {
	return q.ctx.QueryArgs().Has(key)
}

// newRequest creates a Request adapter from a fasthttp.RequestCtx.
func newRequest(ctx *fasthttp.RequestCtx, pattern string) *Request {
	var remoteAddr string
	if ip := ctx.RemoteIP(); ip != nil {
		remoteAddr = ip.String()
	}

	// fasthttp serves HTTP/1.x only; report the protocol from the request
	// line rather than inferring it from TLS.
	proto := "HTTP/1.1"
	protoMajor, protoMinor := 1, 1
	if !ctx.Request.Header.IsHTTP11() {
		proto = "HTTP/1.0"
		protoMinor = 0
	}

	return &Request{
		ctx:           ctx,
		Header:        requestHeader{ctx: ctx},
		Method:        utils.BytesToString(ctx.Method()),
		URL:           &requestURL{ctx: ctx},
		Proto:         proto,
		ProtoMajor:    protoMajor,
		ProtoMinor:    protoMinor,
		Body:          io.NopCloser(&bodyReader{ctx: ctx}),
		ContentLength: int64(ctx.Request.Header.ContentLength()),
		Host:          utils.BytesToString(ctx.Host()),
		RemoteAddr:    remoteAddr,
		RequestURI:    utils.BytesToString(ctx.RequestURI()),
		Pattern:       pattern,
		Close:         ctx.Request.Header.ConnectionClose(),
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
	r.Form = make(url.Values)

	// Parse Query args into Form
	for key, value := range r.ctx.QueryArgs().All() {
		ks, vs := utils.BytesToString(key), utils.BytesToString(value)
		r.Form.Add(ks, vs)
	}

	// Parse POST args into PostForm and Form
	for key, value := range r.ctx.PostArgs().All() {
		ks, vs := utils.BytesToString(key), utils.BytesToString(value)
		r.PostForm.Add(ks, vs)
		r.Form.Add(ks, vs)
	}
	return nil
}

// FormValue returns the first value for the named component of the query.
// POST and PUT body parameters take precedence over URL query string values.
func (r *Request) FormValue(key string) string {
	if r.Form == nil {
		r.ParseMultipartForm(10 << 20) // 10 MB
	}
	if vs := r.Form[key]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// PostFormValue returns the first value for the named component of the POST, PATCH, or PUT request body.
func (r *Request) PostFormValue(key string) string {
	if r.PostForm == nil {
		r.ParseMultipartForm(10 << 20) // 10 MB
	}
	if vs := r.PostForm[key]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// ParseMultipartForm parses query and URL-encoded body values into Form and
// PostForm and, for multipart requests, the multipart form data as well.
//
// maxMemory is accepted for net/http compatibility but is not applied:
// fasthttp has already buffered the request body (bounded by the server body
// limit) and keeps up to 16 MiB of parts in memory before using temp files.
// It returns fasthttp.ErrNoMultipartForm for non-multipart requests, after
// populating Form and PostForm.
func (r *Request) ParseMultipartForm(maxMemory int64) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	if r.parsedMultipart {
		return nil
	}
	r.parsedMultipart = true

	form, err := r.ctx.MultipartForm()
	if err != nil {
		return err
	}
	r.MultipartForm = form
	for key, values := range form.Value {
		for _, value := range values {
			r.PostForm.Add(key, value)
			r.Form.Add(key, value)
		}
	}
	return nil
}

// WrapMiddleware converts a standard net/http middleware into a Gofi MiddlewareFunc.
//
// The wrapped middleware receives a server-style *http.Request carrying the
// method, target, headers, host, remote address and TLS state. When it calls
// the next handler, the request it passes is propagated to downstream Gofi
// handlers as follows: its context becomes c.Context() (so values attached
// with r.WithContext, such as an authenticated principal, are visible), and
// its headers replace the request headers. Changes to the method, URL, body
// or remote address are not propagated. Flusher, Hijacker and Pusher are not
// supported.
func WrapMiddleware(mw func(http.Handler) http.Handler) MiddlewareFunc {
	return func(c Context) error {
		var innerErr error
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r != nil {
				if err := SetRequestContext(c, r.Context()); err != nil {
					innerErr = err
					return
				}
				syncRequestHeaders(c, r.Header, r.Host)
			}
			innerErr = c.Next()
		})

		wrapped := mw(inner)

		// Create a temporary http.ResponseWriter + *http.Request for the net/http middleware
		w := c.Writer()
		rw, ok := w.(http.ResponseWriter)
		if !ok {
			return c.Next()
		}

		r, err := convertFasthttpToHTTPRequest(c)
		if err != nil {
			return NewHTTPError(http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
		}
		wrapped.ServeHTTP(rw, r)
		return innerErr
	}
}

// syncRequestHeaders makes the fasthttp request headers match h, as modified
// by a wrapped net/http middleware.
func syncRequestHeaders(c Context, h http.Header, host string) {
	req := &c.Request().FastHTTPContext().Request
	var stale []string
	for key := range req.Header.All() {
		name := http.CanonicalHeaderKey(string(key))
		if name != "Host" && h[name] == nil {
			stale = append(stale, name)
		}
	}
	for _, name := range stale {
		req.Header.Del(name)
	}
	for name, values := range h {
		req.Header.Del(name)
		for _, v := range values {
			if validHeaderField(name) && validHeaderField(v) {
				req.Header.Add(name, v)
			}
		}
	}
	if host != "" && validHeaderField(host) {
		req.Header.SetHost(host)
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

func (r *InjectResponse) BodyString() string {
	return string(r.Body)
}

func (r *InjectResponse) BindBody(v any) error {
	return json.Unmarshal(r.Body, v)
}

func (r *InjectResponse) Header(key string) string {
	return r.HeaderMap.Get(key)
}

func (r *InjectResponse) HasHeader(key string) bool {
	_, ok := r.HeaderMap[http.CanonicalHeaderKey(key)]
	return ok
}

func (r *InjectResponse) CookieValue(name string) string {
	for _, c := range r.Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func (r *InjectResponse) BodyJSON() map[string]any {
	var body map[string]any
	if err := json.Unmarshal(r.Body, &body); err != nil {
		return nil
	}
	return body
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
