package middleware

import (
	"fmt"
	"time"

	"github.com/michaelolof/gofi"
	"github.com/valyala/fasthttp"
)

// TimeoutConfig defines the config for the Timeout middleware.
type TimeoutConfig struct {
	// Timeout controls how long the server should wait for a response.
	// Optional. Default: 5 * time.Second
	Timeout time.Duration

	// ErrorHandler is executed if the request times out.
	// Optional. Default: returns 408 Request Timeout.
	ErrorHandler gofi.HandlerFunc
}

// TimeoutConfigDefault is the default config
var TimeoutConfigDefault = TimeoutConfig{
	Timeout: 5 * time.Second,
	ErrorHandler: func(c gofi.Context) error {
		return c.SendString(fasthttp.StatusRequestTimeout, "Request Timeout")
	},
}

// Timeout creates a new middleware handler
func Timeout(config ...TimeoutConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := TimeoutConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		if cfg.Timeout == 0 {
			cfg.Timeout = TimeoutConfigDefault.Timeout
		}
		if cfg.ErrorHandler == nil {
			cfg.ErrorHandler = TimeoutConfigDefault.ErrorHandler
		}
	}

	return func(c gofi.Context) error {
		// Execute downstream handlers on a copied context so that if timeout fires,
		// no goroutine continues operating on pooled request state.
		cc := c.Copy()

		type runResult struct {
			err error
		}
		ch := make(chan runResult, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- runResult{err: fmt.Errorf("panic in timeout middleware: %v", r)}
				}
			}()

			ch <- runResult{err: cc.Next()}
		}()

		select {
		case result := <-ch:
			// Chain finished before timeout, copy generated response back.
			origCtx := c.Request().Context()
			copyCtx := cc.Request().Context()
			copyCtx.Response.CopyTo(&origCtx.Response)
			return result.err
		case <-time.After(cfg.Timeout):
			// Timeout response is written on the foreground request context only.
			return cfg.ErrorHandler(c)
		}
	}
}
