package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/michaelolof/gofi"
)

// ConcurrencyLimit admits at most max requests through the rest of the chain
// at once. Requests beyond the limit are rejected immediately with 503 and
// Retry-After; there is no waiting queue. The permit is held until the chain
// returns. It does not bound body bytes the transport has already buffered,
// and a stream producer started by the handler runs after its permit is
// released.
func ConcurrencyLimit(max int) (gofi.MiddlewareFunc, error) {
	if max <= 0 {
		return nil, errors.New("gofi/middleware: ConcurrencyLimit requires a positive limit")
	}
	permits := make(chan struct{}, max)
	return func(c gofi.Context) error {
		select {
		case permits <- struct{}{}:
		default:
			c.Request().Context().Response.Header.Set("Retry-After", "1")
			return gofi.NewHTTPError(http.StatusServiceUnavailable, http.StatusText(http.StatusServiceUnavailable))
		}
		defer func() { <-permits }()
		return c.Next()
	}, nil
}

// RateLimitConfig configures RateLimit.
type RateLimitConfig struct {
	// Requests allowed per key in each window. Required.
	Requests int
	// Window length. Required.
	Window time.Duration
	// Key identifies the caller. Default: gofi.ClientIP, which uses the
	// socket peer unless a gofi.TrustPolicy middleware resolved a client
	// behind trusted proxies. Never key on an unvalidated request header.
	Key func(c gofi.Context) string
	// MaxKeys bounds tracked keys. When full, expired keys are swept; if
	// none can be removed, requests from new keys get 503. Default: 100000.
	MaxKeys int
}

// maxRateLimitKey bounds the stored length of one key.
const maxRateLimitKey = 256

type rateWindow struct {
	start time.Time
	count int
}

// RateLimit limits each key to cfg.Requests per cfg.Window (fixed windows).
// Requests over the limit get 429 with Retry-After.
func RateLimit(cfg RateLimitConfig) (gofi.MiddlewareFunc, error) {
	if cfg.Requests <= 0 || cfg.Window <= 0 {
		return nil, errors.New("gofi/middleware: RateLimit requires positive Requests and Window")
	}
	if cfg.MaxKeys < 0 {
		return nil, errors.New("gofi/middleware: negative RateLimit MaxKeys")
	}
	if cfg.MaxKeys == 0 {
		cfg.MaxKeys = 100000
	}
	if cfg.Key == nil {
		cfg.Key = gofi.ClientIP
	}

	var mu sync.Mutex
	windows := make(map[string]*rateWindow)

	return func(c gofi.Context) error {
		key := cfg.Key(c)
		if len(key) > maxRateLimitKey {
			key = key[:maxRateLimitKey]
		}
		now := time.Now()

		mu.Lock()
		w, ok := windows[key]
		if !ok {
			if len(windows) >= cfg.MaxKeys {
				for k, v := range windows {
					if now.Sub(v.start) >= cfg.Window {
						delete(windows, k)
					}
				}
			}
			if len(windows) >= cfg.MaxKeys {
				mu.Unlock()
				return gofi.NewHTTPError(http.StatusServiceUnavailable, http.StatusText(http.StatusServiceUnavailable))
			}
			w = &rateWindow{start: now}
			windows[key] = w
		}
		if now.Sub(w.start) >= cfg.Window {
			w.start, w.count = now, 0
		}
		w.count++
		over := w.count > cfg.Requests
		retry := cfg.Window - now.Sub(w.start)
		mu.Unlock()

		if over {
			seconds := int(retry.Seconds())
			if retry > time.Duration(seconds)*time.Second {
				seconds++
			}
			c.Request().Context().Response.Header.Set("Retry-After", strconv.Itoa(seconds))
			return gofi.NewHTTPError(http.StatusTooManyRequests, http.StatusText(http.StatusTooManyRequests))
		}
		return c.Next()
	}, nil
}
