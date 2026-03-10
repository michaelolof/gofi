package middleware

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/michaelolof/gofi"
)

// CacheConfig defines the config for the Cache middleware.
type CacheConfig struct {
	// Expiration is the time that the cached response will live
	// Optional. Default: 1 * time.Minute
	Expiration time.Duration

	// CacheHeader enables the Cache-Control header indicating how long
	// the client can cache the response.
	// Optional. Default: false
	CacheHeader bool

	// KeyGenerator creates a unique key per request
	// Optional. Default: method + path + normalized query
	KeyGenerator func(c gofi.Context) string

	// Methods allowed to be cached (comma separated string)
	// Optional. Default: "GET, HEAD"
	Methods string

	// MaxEntries bounds number of cache keys retained.
	// Set <= 0 to disable entry-count cap.
	// Optional. Default: 4096
	MaxEntries int

	// MaxBytes bounds total cached response bytes (body + content-type + key length).
	// Set <= 0 to disable byte-size cap.
	// Optional. Default: 64MB
	MaxBytes int64

	// AllowPrivateResponses enables caching requests that carry Authorization/Cookie headers.
	// Optional. Default: false
	AllowPrivateResponses bool
}

// defaultKeyGenerator uses method + path + normalized query for safer cache isolation.
func defaultKeyGenerator(c gofi.Context) string {
	rawQuery := string(c.Request().Context().QueryArgs().QueryString())
	q := normalizeQuery(rawQuery)
	if q == "" {
		return c.Method() + " " + c.Path()
	}
	return c.Method() + " " + c.Path() + "?" + q
}

func normalizeQuery(q string) string {
	if q == "" || !strings.Contains(q, "&") {
		return q
	}
	parts := strings.Split(q, "&")
	sort.Strings(parts)
	return strings.Join(parts, "&")
}

// CacheConfigDefault is the default config
var CacheConfigDefault = CacheConfig{
	Expiration:   1 * time.Minute,
	CacheHeader:  false,
	KeyGenerator: defaultKeyGenerator,
	Methods:      "GET, HEAD",
	MaxEntries:   4096,
	MaxBytes:     64 << 20,
}

type cacheEntry struct {
	body        []byte
	contentType []byte
	statusCode  int
	exp         int64
	size        int64
}

// Cache creates a new middleware handler
func Cache(config ...CacheConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := CacheConfigDefault

	if len(config) > 0 {
		cfg = config[0]

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

	// Simple in-memory thread-safe cache
	var mu sync.RWMutex
	store := make(map[string]cacheEntry)
	var totalBytes int64

	evictOne := func(now int64) bool {
		// Prefer removing expired entries first.
		for k, v := range store {
			if v.exp > 0 && v.exp < now {
				delete(store, k)
				totalBytes -= v.size
				if totalBytes < 0 {
					totalBytes = 0
				}
				return true
			}
		}

		// Fallback to deleting any one key to keep O(1) expected eviction overhead.
		for k, v := range store {
			delete(store, k)
			totalBytes -= v.size
			if totalBytes < 0 {
				totalBytes = 0
			}
			return true
		}

		return false
	}

	// Background ticker to cleanup expired entries
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			now := time.Now().UnixNano()
			mu.Lock()
			for k, v := range store {
				if v.exp > 0 && v.exp < now {
					delete(store, k)
					totalBytes -= v.size
					if totalBytes < 0 {
						totalBytes = 0
					}
				}
			}
			mu.Unlock()
		}
	}()

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

		key := cfg.KeyGenerator(c)
		now := time.Now().UnixNano()

		// 1. Check if we have a valid cache entry
		mu.RLock()
		entry, found := store[key]
		mu.RUnlock()

		if found && (entry.exp == 0 || entry.exp > now) {
			// Cache hit
			if cfg.CacheHeader {
				c.Writer().Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(cfg.Expiration.Seconds())))
			}

			// We must set the headers first before the body
			if len(entry.contentType) > 0 {
				c.Request().Context().Response.Header.SetContentTypeBytes(entry.contentType)
			}
			return c.SendBytes(entry.statusCode, entry.body)
		}

		if found && entry.exp > 0 && entry.exp <= now {
			mu.Lock()
			if stale, ok := store[key]; ok && stale.exp <= now {
				delete(store, key)
				totalBytes -= stale.size
				if totalBytes < 0 {
					totalBytes = 0
				}
			}
			mu.Unlock()
		}

		// 2. Cache miss, continue request down the chain
		err := c.Next()

		// 3. Store response into cache after handler returns
		fctx := c.Request().Context()
		statusCode := fctx.Response.StatusCode()

		// Only cache successful responses (2xx) or specific responses if needed
		// For simplicity, we cache anything under 400
		if statusCode < 400 {
			body := fctx.Response.Body()
			contentType := fctx.Response.Header.ContentType()

			// Copy bytes so we don't hold references to fasthttp's internal buffers
			bodyCopy := make([]byte, len(body))
			copy(bodyCopy, body)

			ctCopy := make([]byte, len(contentType))
			copy(ctCopy, contentType)

			size := int64(len(bodyCopy) + len(ctCopy) + len(key))

			mu.Lock()
			if prev, ok := store[key]; ok {
				totalBytes -= prev.size
				if totalBytes < 0 {
					totalBytes = 0
				}
			}

			for (cfg.MaxEntries > 0 && len(store) >= cfg.MaxEntries) || (cfg.MaxBytes > 0 && totalBytes+size > cfg.MaxBytes) {
				if !evictOne(now) {
					break
				}
			}

			store[key] = cacheEntry{
				body:        bodyCopy,
				contentType: ctCopy,
				statusCode:  statusCode,
				exp:         now + cfg.Expiration.Nanoseconds(),
				size:        size,
			}
			totalBytes += size
			mu.Unlock()

			if cfg.CacheHeader {
				c.Writer().Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(cfg.Expiration.Seconds())))
			}
		}

		return err
	}
}
