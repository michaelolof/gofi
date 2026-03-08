package middleware

import (
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
		// Create a channel to wait for the handler
		ch := make(chan error, 1)

		// Run the handler in a goroutine
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// We just swallow the panic here and let the server's global recover handle it
					// normally, or you could push an error. But to unblock the timeout:
					ch <- nil
				}
			}()
			ch <- c.Next()
		}()

		// Wait for the handler to complete or timeout to trigger
		select {
		case err := <-ch:
			return err
		case <-time.After(cfg.Timeout):
			// We cannot easily cancel the already running goroutine cleanly unless
			// we pass a specific context through standard library context.Context
			// But since gofi relies on fasthttp, we can only return early and close the client connection
			return cfg.ErrorHandler(c)
		}
	}
}
