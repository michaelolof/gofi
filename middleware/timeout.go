package middleware

import (
	"errors"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/michaelolof/gofi"
	"github.com/valyala/fasthttp"
)

// TimeoutConfig defines the config for the Timeout middleware.
type TimeoutConfig struct {
	// Timeout after which the response is sent.
	// Required.
	Timeout time.Duration

	// MaxInFlight bounds handler goroutines, including handlers still running
	// after their request timed out. Requests beyond it are rejected with 503.
	// Required.
	MaxInFlight int

	// ErrorHandler is executed if the request times out.
	// Optional. Default: returns 503 Service Unavailable.
	ErrorHandler gofi.HandlerFunc
}

// TimeoutConfigDefault holds the defaults applied to optional fields.
var TimeoutConfigDefault = TimeoutConfig{
	ErrorHandler: func(c gofi.Context) error {
		return gofi.NewHTTPError(http.StatusServiceUnavailable, http.StatusText(http.StatusServiceUnavailable))
	},
}

// Timeout validates cfg and creates a response timeout middleware.
//
// Timeout runs the rest of the chain on a copied context in another goroutine
// and answers when the timer fires. It does not stop the handler: the
// goroutine keeps running, and keeps its MaxInFlight slot, until the handler
// returns. Prefer ContextTimeout, which gives handlers a cooperative deadline
// without a second goroutine. Streaming responses are rejected under Timeout.
//
// Headers set before Timeout, natively or through c.Writer(), and headers set
// by downstream handlers are kept in the response.
func Timeout(cfg TimeoutConfig) (gofi.MiddlewareFunc, error) {
	if cfg.Timeout <= 0 || cfg.MaxInFlight <= 0 {
		return nil, errors.New("gofi/middleware: Timeout requires positive Timeout and MaxInFlight")
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = TimeoutConfigDefault.ErrorHandler
	}
	return newTimeout(cfg), nil
}

func newTimeout(cfg TimeoutConfig) gofi.MiddlewareFunc {
	inFlight := make(chan struct{}, cfg.MaxInFlight)

	return func(c gofi.Context) error {
		select {
		case inFlight <- struct{}{}:
		default:
			return gofi.NewHTTPError(http.StatusServiceUnavailable, http.StatusText(http.StatusServiceUnavailable))
		}

		// Execute downstream handlers on a copied context so that if timeout fires,
		// no goroutine continues operating on pooled request state.
		cc := c.Copy()

		type runResult struct {
			err error
		}
		ch := make(chan runResult, 1)

		go func() {
			// The slot is released only when the handler really finishes.
			defer func() { <-inFlight }()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("gofi/middleware: recovered panic in handler under Timeout: %v\n%s", r, debug.Stack())
					ch <- runResult{err: gofi.NewHTTPError(500, "Internal Server Error")}
				}
			}()

			ch <- runResult{err: cc.Next()}
		}()

		timer := time.NewTimer(cfg.Timeout)
		defer timer.Stop()

		select {
		case result := <-ch:
			// Chain finished before timeout, copy generated response back.
			origCtx := c.Request().Context()
			copyCtx := cc.Request().Context()
			if copyCtx.Response.IsBodyStream() {
				// A stream cannot be copied back or bounded by this timeout.
				// Close it so the producer stops, and fail the request.
				_ = copyCtx.Response.CloseBodyStream()
				return gofi.NewHTTPError(500, "streaming responses are not supported under the Timeout middleware")
			}
			// Write the copy's Writer() headers into its response. WriteHeader
			// with the current status flushes the header view.
			cc.Writer().WriteHeader(copyCtx.Response.StatusCode())
			upstream := saveNativeHeaders(&origCtx.Response.Header)
			copyCtx.Response.CopyTo(&origCtx.Response)
			restoreMissingHeaders(&origCtx.Response.Header, upstream)
			return result.err
		case <-timer.C:
			// By default, timeout errors are returned to the router error handler.
			// A custom middleware ErrorHandler may still choose to handle the response locally.
			return cfg.ErrorHandler(c)
		}
	}
}

type savedHeader struct {
	key, value string
	cookie     bool
}

// saveNativeHeaders records headers set on the original response before the
// chain ran, so copy-back does not discard them.
func saveNativeHeaders(h *fasthttp.ResponseHeader) []savedHeader {
	var saved []savedHeader
	for key, value := range h.All() {
		name := string(key)
		switch name {
		case "Content-Length", "Content-Type", "Server", "Date", "Connection", "Set-Cookie":
			continue
		}
		saved = append(saved, savedHeader{key: name, value: string(value)})
	}
	for _, value := range h.Cookies() {
		saved = append(saved, savedHeader{key: "Set-Cookie", value: string(value), cookie: true})
	}
	return saved
}

// restoreMissingHeaders re-adds saved headers the downstream response did not
// set itself. Downstream values win for the same header name.
func restoreMissingHeaders(h *fasthttp.ResponseHeader, saved []savedHeader) {
	for _, s := range saved {
		if s.cookie {
			var cookie fasthttp.Cookie
			if cookie.Parse(s.value) != nil {
				continue
			}
			probe := fasthttp.AcquireCookie()
			probe.SetKeyBytes(cookie.Key())
			exists := h.Cookie(probe)
			fasthttp.ReleaseCookie(probe)
			if !exists {
				h.SetCookie(&cookie)
			}
			continue
		}
		if len(h.Peek(s.key)) == 0 {
			h.Add(s.key, s.value)
		}
	}
}
