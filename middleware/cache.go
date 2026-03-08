package middleware

import (
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
	// Optional. Default: request URL path
	KeyGenerator func(c gofi.Context) string

	// Methods allowed to be cached (comma separated string)
	// Optional. Default: "GET, HEAD"
	Methods string
}

// defaultKeyGenerator uses the request URL path as the cache key
func defaultKeyGenerator(c gofi.Context) string {
	return c.Path()
}

// CacheConfigDefault is the default config
var CacheConfigDefault = CacheConfig{
	Expiration:   1 * time.Minute,
	CacheHeader:  false,
	KeyGenerator: defaultKeyGenerator,
	Methods:      "GET, HEAD",
}

type cacheEntry struct {
	body        []byte
	contentType []byte
	statusCode  int
	exp         int64
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
	}

	methods := strings.Split(cfg.Methods, ",")
	allowedMethods := make(map[string]bool)
	for _, m := range methods {
		allowedMethods[strings.TrimSpace(m)] = true
	}

	// Simple in-memory thread-safe cache
	var mu sync.RWMutex
	store := make(map[string]cacheEntry)

	// Background ticker to cleanup expired entries
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			now := time.Now().UnixNano()
			mu.Lock()
			for k, v := range store {
				if v.exp > 0 && v.exp < now {
					delete(store, k)
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

			mu.Lock()
			store[key] = cacheEntry{
				body:        bodyCopy,
				contentType: ctCopy,
				statusCode:  statusCode,
				exp:         now + cfg.Expiration.Nanoseconds(),
			}
			mu.Unlock()

			if cfg.CacheHeader {
				c.Writer().Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(cfg.Expiration.Seconds())))
			}
		}

		return err
	}
}
