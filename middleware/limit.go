package middleware

import (
	"context"
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

// RateLimitStore counts requests per key in fixed windows. Implement it over
// shared storage (for example Redis INCR with an expiry) so several instances
// enforce one limit.
type RateLimitStore interface {
	// Increment adds one request for key in the current window of the given
	// length and returns the window's count so far and the time until the
	// window resets.
	Increment(ctx context.Context, key string, window time.Duration) (count int, resetIn time.Duration, err error)
}

// ErrRateStoreFull is returned by the in-memory store when it tracks
// MaxKeys keys and none has expired.
var ErrRateStoreFull = errors.New("gofi/middleware: rate limit store is full")

// RateLimitConfig configures RateLimit.
type RateLimitConfig struct {
	// Requests allowed per key in each window. Required.
	Requests int
	// Window length. Required.
	Window time.Duration
	// Key identifies the caller. Default: gofi.ClientIP, which uses the
	// socket peer unless a gofi.TrustPolicy middleware resolved a client
	// behind trusted proxies. Never key on an unvalidated request header;
	// after authentication, key on the principal or tenant instead.
	Key func(c gofi.Context) string
	// RequestsFor overrides Requests per request, for example to apply each
	// tenant's quota. Values <= 0 fall back to Requests.
	RequestsFor func(c gofi.Context) int
	// Store counts requests. Default: an in-memory store bounded by MaxKeys,
	// local to this process.
	Store RateLimitStore
	// MaxKeys bounds keys in the default in-memory store. When full, expired
	// keys are swept; if none can be removed, requests from new keys get 503.
	// Default: 100000.
	MaxKeys int
	// FailOpen admits requests when the store returns an error. By default
	// such requests are rejected with 503.
	FailOpen bool
}

// maxRateLimitKey bounds the stored length of one key.
const maxRateLimitKey = 256

type rateWindow struct {
	start time.Time
	count int
}

// memoryRateStore is the default, process-local RateLimitStore.
type memoryRateStore struct {
	mu      sync.Mutex
	windows map[string]*rateWindow
	maxKeys int
}

func (m *memoryRateStore) Increment(_ context.Context, key string, window time.Duration) (int, time.Duration, error) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.windows[key]
	if !ok {
		if len(m.windows) >= m.maxKeys {
			for k, v := range m.windows {
				if now.Sub(v.start) >= window {
					delete(m.windows, k)
				}
			}
		}
		if len(m.windows) >= m.maxKeys {
			return 0, 0, ErrRateStoreFull
		}
		w = &rateWindow{start: now}
		m.windows[key] = w
	}
	if now.Sub(w.start) >= window {
		w.start, w.count = now, 0
	}
	w.count++
	return w.count, window - now.Sub(w.start), nil
}

// RateLimit limits each key to cfg.Requests (or RequestsFor) per cfg.Window.
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
	if cfg.Store == nil {
		cfg.Store = &memoryRateStore{windows: make(map[string]*rateWindow), maxKeys: cfg.MaxKeys}
	}

	return func(c gofi.Context) error {
		key := cfg.Key(c)
		if len(key) > maxRateLimitKey {
			key = key[:maxRateLimitKey]
		}
		limit := cfg.Requests
		if cfg.RequestsFor != nil {
			if n := cfg.RequestsFor(c); n > 0 {
				limit = n
			}
		}

		count, resetIn, err := cfg.Store.Increment(c.Context(), key, cfg.Window)
		if err != nil {
			if cfg.FailOpen {
				return c.Next()
			}
			return gofi.NewHTTPError(http.StatusServiceUnavailable, http.StatusText(http.StatusServiceUnavailable))
		}
		if count > limit {
			seconds := int(resetIn.Seconds())
			if resetIn > time.Duration(seconds)*time.Second {
				seconds++
			}
			c.Request().Context().Response.Header.Set("Retry-After", strconv.Itoa(seconds))
			return gofi.NewHTTPError(http.StatusTooManyRequests, http.StatusText(http.StatusTooManyRequests))
		}
		return c.Next()
	}, nil
}
