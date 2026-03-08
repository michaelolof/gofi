package middleware

import (
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
}

// RecoverConfigDefault is the default config
var RecoverConfigDefault = RecoverConfig{
	EnableStackTrace: false,
	Output:           log.New(os.Stderr, "", log.LstdFlags),
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
	}

	return func(c gofi.Context) error {
		defer func() {
			if r := recover(); r != nil {
				// Log the panic
				if cfg.EnableStackTrace {
					cfg.Output.Printf("panic recovered: %v\n%s\n", r, debug.Stack())
				} else {
					cfg.Output.Printf("panic recovered: %v\n", r)
				}

				// Return 500 status code
				_ = c.SendString(500, "Internal Server Error")
			}
		}()

		return c.Next()
	}
}
