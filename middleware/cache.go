package middleware

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/michaelolof/gofi"
	"github.com/valyala/fasthttp"
)

// CacheConfig defines the config for the Cache middleware.
//
// The cache stores only complete, buffered 200 responses that the handler
// returned without error. It bypasses responses that set cookies, carry a
// Cache-Control of private, no-store or no-cache, declare "Vary: *", or are
// streamed. Replays restore the stored Content-Type and a fixed set of safe
// end-to-end headers (Cache-Control, Content-Encoding, Content-Language,
// ETag, Expires, Last-Modified, Vary).
type CacheConfig struct {
	// Expiration is the time that the cached response will live
	// Optional. Default: 1 * time.Minute
	Expiration time.Duration

	// CacheHeader adds a public Cache-Control header to cached responses that
	// do not already declare one. An existing Cache-Control is never replaced.
	// Optional. Default: false
	CacheHeader bool

	// KeyGenerator creates a unique key per request
	// Optional. Default: method + host + path + raw query
	KeyGenerator func(c gofi.Context) string

	// Methods allowed to be cached (comma separated string)
	// Optional. Default: "GET, HEAD"
	Methods string

	// MaxEntries bounds number of cache keys retained.
	// Set <= 0 to disable entry-count cap.
	// Optional. Default: 4096
	MaxEntries int

	// MaxBytes bounds total cached response bytes (body + stored headers + key
	// length). A single response larger than MaxBytes is never stored.
	// Set <= 0 to disable byte-size cap.
	// Optional. Default: 64MB
	MaxBytes int64

	// AllowPrivateResponses enables caching requests that carry Authorization/Cookie headers.
	// Optional. Default: false
	AllowPrivateResponses bool

	// MaxConcurrentFills bounds responses being copied into the cache at
	// once. Misses beyond it run the handler but are not stored.
	// Optional. Default: 64
	MaxConcurrentFills int

	// CoalesceWait lets concurrent misses for the same key wait up to this
	// long for the request already filling it, instead of all running the
	// handler.
	// Optional. Default: 0 (no coalescing)
	CoalesceWait time.Duration

	// MaxWaiters bounds requests waiting on one key; extra requests run the
	// handler themselves.
	// Optional. Default: 32
	MaxWaiters int
}

// defaultKeyGenerator partitions entries by method, Host and path, and keeps
// the raw query string. The query is not reordered: handlers that read the
// first of repeated parameters treat "x=1&x=2" and "x=2&x=1" differently.
// The Host header is a namespace, not a trusted tenant identity.
func defaultKeyGenerator(c gofi.Context) string {
	fctx := c.Request().Context()
	key := c.Method() + " " + string(fctx.Host()) + c.Path()
	if q := fctx.QueryArgs().QueryString(); len(q) > 0 {
		key += "?" + string(q)
	}
	return key
}

// CacheConfigDefault holds the defaults applied to optional fields.
var CacheConfigDefault = CacheConfig{
	Expiration:         1 * time.Minute,
	KeyGenerator:       defaultKeyGenerator,
	Methods:            "GET, HEAD",
	MaxEntries:         4096,
	MaxBytes:           64 << 20,
	MaxConcurrentFills: 64,
	MaxWaiters:         32,
}

// replayedHeaders are the end-to-end response headers a cache hit restores.
var replayedHeaders = []string{
	"Cache-Control", "Content-Encoding", "Content-Language", "ETag", "Expires", "Last-Modified", "Vary",
}

// maxVaryHeaders bounds how many request headers one response may vary on.
const maxVaryHeaders = 8

type cachedHeader struct {
	name  string
	value string
}

type cacheEntry struct {
	body        []byte
	contentType []byte
	headers     []cachedHeader
	statusCode  int
	exp         int64
	size        int64
}

// pendingFill tracks a request filling one cache key.
type pendingFill struct {
	done    chan struct{}
	waiters int
}

// Cache validates cfg and creates a cache middleware with bounded fills and
// optional stampede control.
func Cache(cfg CacheConfig) (gofi.MiddlewareFunc, error) {
	if cfg.Expiration < 0 || cfg.MaxConcurrentFills < 0 || cfg.CoalesceWait < 0 || cfg.MaxWaiters < 0 {
		return nil, errors.New("gofi/middleware: negative CacheConfig value")
	}
	if cfg.MaxConcurrentFills == 0 {
		cfg.MaxConcurrentFills = CacheConfigDefault.MaxConcurrentFills
	}
	if cfg.MaxWaiters == 0 {
		cfg.MaxWaiters = CacheConfigDefault.MaxWaiters
	}
	return newCache(cfg), nil
}

func newCache(cfg CacheConfig) gofi.MiddlewareFunc {
	{
		if cfg.Expiration == 0 {
			cfg.Expiration = CacheConfigDefault.Expiration
		}
		if cfg.KeyGenerator == nil {
			cfg.KeyGenerator = CacheConfigDefault.KeyGenerator
		}
		if cfg.Methods == "" {
			cfg.Methods = CacheConfigDefault.Methods
		}
		if cfg.MaxEntries == 0 {
			cfg.MaxEntries = CacheConfigDefault.MaxEntries
		}
		if cfg.MaxBytes == 0 {
			cfg.MaxBytes = CacheConfigDefault.MaxBytes
		}
	}

	methods := strings.Split(cfg.Methods, ",")
	allowedMethods := make(map[string]bool)
	for _, m := range methods {
		allowedMethods[strings.TrimSpace(m)] = true
	}

	// Simple in-memory thread-safe cache. Expired entries are removed lazily on
	// access and during eviction, so no background goroutine is needed and a
	// discarded middleware instance leaks nothing.
	var mu sync.Mutex
	store := make(map[string]cacheEntry)
	// varySpecs records, per base key, the request headers the cached
	// response varies on.
	varySpecs := make(map[string][]string)
	var totalBytes int64
	var fills chan struct{}
	if cfg.MaxConcurrentFills > 0 {
		fills = make(chan struct{}, cfg.MaxConcurrentFills)
	}
	pending := make(map[string]*pendingFill)

	remove := func(key string) {
		if prev, ok := store[key]; ok {
			delete(store, key)
			totalBytes -= prev.size
		}
	}

	evictOne := func(now int64) bool {
		// Prefer removing expired entries first.
		for k, v := range store {
			if v.exp > 0 && v.exp < now {
				remove(k)
				return true
			}
		}
		for k := range store {
			remove(k)
			return true
		}
		return false
	}

	return func(c gofi.Context) error {
		// Only cache allowed methods
		if !allowedMethods[c.Method()] {
			return c.Next()
		}

		if !cfg.AllowPrivateResponses {
			if c.HeaderVal("Authorization") != "" || c.HeaderVal("Cookie") != "" {
				return c.Next()
			}
		}

		baseKey := cfg.KeyGenerator(c)
		now := time.Now().UnixNano()

		lookup := func() (cacheEntry, bool) {
			mu.Lock()
			defer mu.Unlock()
			key := baseKey + varySuffix(c, varySpecs[baseKey])
			entry, found := store[key]
			if found && entry.exp > 0 && entry.exp <= time.Now().UnixNano() {
				remove(key)
				found = false
			}
			return entry, found
		}

		// 1. Check if we have a valid cache entry
		entry, found := lookup()

		// Coalesce concurrent misses: wait briefly for the request already
		// filling this key, then use its entry if it was stored.
		var fill *pendingFill
		if !found && cfg.CoalesceWait > 0 {
			mu.Lock()
			if p, ok := pending[baseKey]; ok && p.waiters < cfg.MaxWaiters {
				p.waiters++
				mu.Unlock()
				timer := time.NewTimer(cfg.CoalesceWait)
				select {
				case <-p.done:
				case <-timer.C:
				}
				timer.Stop()
				entry, found = lookup()
			} else if !ok {
				fill = &pendingFill{done: make(chan struct{})}
				pending[baseKey] = fill
				mu.Unlock()
			} else {
				mu.Unlock()
			}
		}
		if fill != nil {
			defer func() {
				mu.Lock()
				delete(pending, baseKey)
				mu.Unlock()
				close(fill.done)
			}()
		}

		if found {
			fctx := c.Request().Context()
			if len(entry.contentType) > 0 {
				fctx.Response.Header.SetContentTypeBytes(entry.contentType)
			}
			for _, h := range entry.headers {
				fctx.Response.Header.Set(h.name, h.value)
			}
			if cfg.CacheHeader && !hasHeader(entry.headers, "Cache-Control") {
				fctx.Response.Header.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(cfg.Expiration.Seconds())))
			}
			return c.SendBytes(entry.statusCode, entry.body)
		}

		// 2. Cache miss, continue request down the chain
		err := c.Next()

		// A returned error has not been rendered yet: the response still holds
		// fasthttp's default 200 and an empty body. Never store it.
		if err != nil {
			return err
		}

		// 3. Store response into cache after handler returns
		fctx := c.Request().Context()
		// Streamed bodies are produced after the handler returns; reading them
		// here would block on, or buffer, the whole stream.
		if fctx.Response.IsBodyStream() {
			return nil
		}
		if fctx.Response.StatusCode() != http.StatusOK || !storable(c, fctx) {
			return nil
		}

		vary, ok := parseVary(effectiveHeader(c, fctx, "Vary"))
		if !ok {
			return nil
		}

		body := fctx.Response.Body()
		contentType := fctx.Response.Header.ContentType()
		var headers []cachedHeader
		size := int64(len(body) + len(contentType) + len(baseKey))
		for _, name := range replayedHeaders {
			if v := effectiveHeader(c, fctx, name); v != "" {
				headers = append(headers, cachedHeader{name: name, value: v})
				size += int64(len(name) + len(v))
			}
		}

		// Reject oversized responses before copying them.
		if cfg.MaxBytes > 0 && size > cfg.MaxBytes {
			return nil
		}

		// Bound concurrent copies into the cache; when full, serve uncached.
		if fills != nil {
			select {
			case fills <- struct{}{}:
				defer func() { <-fills }()
			default:
				return nil
			}
		}

		// Copy bytes so we don't hold references to fasthttp's internal buffers
		newEntry := cacheEntry{
			body:        append([]byte(nil), body...),
			contentType: append([]byte(nil), contentType...),
			headers:     headers,
			statusCode:  http.StatusOK,
			exp:         now + cfg.Expiration.Nanoseconds(),
			size:        size,
		}

		mu.Lock()
		key := ""
		if len(vary) == 0 {
			delete(varySpecs, baseKey)
		} else {
			// Bound the spec map like the store. Dropping specs only causes
			// misses, never a wrong hit, because entry keys embed the spec.
			if _, ok := varySpecs[baseKey]; !ok && cfg.MaxEntries > 0 && len(varySpecs) >= cfg.MaxEntries {
				clear(varySpecs)
			}
			varySpecs[baseKey] = vary
		}
		key = baseKey + varySuffix(c, vary)
		// Remove any entry being replaced before evicting, so its size is
		// subtracted exactly once.
		remove(key)
		for (cfg.MaxEntries > 0 && len(store) >= cfg.MaxEntries) || (cfg.MaxBytes > 0 && totalBytes+size > cfg.MaxBytes) {
			if !evictOne(now) {
				break
			}
		}
		store[key] = newEntry
		totalBytes += size
		mu.Unlock()

		if cfg.CacheHeader && !hasHeader(headers, "Cache-Control") {
			fctx.Response.Header.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(cfg.Expiration.Seconds())))
		}

		return err
	}
}

// effectiveHeader reads a response header from the native response or, if
// absent there, from headers still pending in the Writer() adapter.
func effectiveHeader(c gofi.Context, fctx *fasthttp.RequestCtx, name string) string {
	if v := fctx.Response.Header.Peek(name); len(v) > 0 {
		return string(v)
	}
	return c.Writer().Header().Get(name)
}

// storable reports whether the response may be shared between clients.
func storable(c gofi.Context, fctx *fasthttp.RequestCtx) bool {
	hasCookie := false
	fctx.Response.Header.VisitAllCookie(func(_, _ []byte) { hasCookie = true })
	if hasCookie || len(c.Writer().Header().Values("Set-Cookie")) > 0 {
		return false
	}
	cacheControl := strings.ToLower(effectiveHeader(c, fctx, "Cache-Control"))
	for _, directive := range strings.Split(cacheControl, ",") {
		switch strings.TrimSpace(strings.SplitN(directive, "=", 2)[0]) {
		case "private", "no-store", "no-cache":
			return false
		}
	}
	return true
}

// parseVary returns the normalized request header names a response varies
// on. It reports false for "Vary: *" or more than maxVaryHeaders names.
func parseVary(v string) ([]string, bool) {
	if v == "" {
		return nil, true
	}
	var names []string
	for _, name := range strings.Split(v, ",") {
		name = http.CanonicalHeaderKey(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if name == "*" {
			return nil, false
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	if len(names) > maxVaryHeaders {
		return nil, false
	}
	slices.Sort(names)
	return names, true
}

// varySuffix renders the request's values for the varied headers.
func varySuffix(c gofi.Context, vary []string) string {
	if len(vary) == 0 {
		return ""
	}
	var b strings.Builder
	for _, name := range vary {
		b.WriteString("\x00")
		b.WriteString(name)
		b.WriteString("=")
		b.WriteString(c.HeaderVal(name))
	}
	return b.String()
}

func hasHeader(headers []cachedHeader, name string) bool {
	for _, h := range headers {
		if h.name == name {
			return true
		}
	}
	return false
}
