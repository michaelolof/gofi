package middleware

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/michaelolof/gofi"
)

// RecoverConfig defines the config for the Recover middleware.
type RecoverConfig struct {
	// EnableStackTrace enables handling stack trace.
	// Optional. Default: false
	EnableStackTrace bool

	// Output is the output log destination.
	// Optional. Default: os.Stderr
	Output *log.Logger

	// ErrorHandler is executed when a panic is recovered.
	// If it returns a non-nil error, that error is forwarded to the router error handler.
	// If it returns nil, the panic is considered fully handled inside the middleware.
	// Optional. Default: returns an error describing the recovered panic.
	ErrorHandler func(c gofi.Context, r any) error
}

// RecoverConfigDefault is the default config
var RecoverConfigDefault = RecoverConfig{
	EnableStackTrace: false,
	Output:           log.New(os.Stderr, "", log.LstdFlags),
	ErrorHandler: func(c gofi.Context, r any) error {
		return gofi.NewHTTPError(500, fmt.Sprintf("panic recovered: %v", r))
	},
}

// Recover creates a new middleware handler
func Recover(config ...RecoverConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := RecoverConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		// Set default values if missing
		if cfg.Output == nil {
			cfg.Output = RecoverConfigDefault.Output
		}
		if cfg.ErrorHandler == nil {
			cfg.ErrorHandler = RecoverConfigDefault.ErrorHandler
		}
	}

	return func(c gofi.Context) (retErr error) {
		defer func() {
			if r := recover(); r != nil {
				// Log the panic
				if cfg.EnableStackTrace {
					cfg.Output.Printf("panic recovered: %v\n%s\n", r, debug.Stack())
				} else {
					cfg.Output.Printf("panic recovered: %v\n", r)
				}

				retErr = cfg.ErrorHandler(c, r)
			}
		}()

		return c.Next()
	}
}
