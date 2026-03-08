package middleware

import "github.com/michaelolof/gofi"

// SkipConfig defines the config for the Skip middleware.
// It skips the wrapped handler if the SkipFilter evaluates to true.
type SkipConfig struct {
	// SkipFilter defines a function to skip middleware.
	// Optional. Default: nil
	SkipFilter func(c gofi.Context) bool

	// Handler is the middleware to be executed when SkipFilter returns false.
	// Required.
	Handler gofi.MiddlewareFunc
}

// Skip creates a new middleware handler
func Skip(config SkipConfig) gofi.MiddlewareFunc {
	return func(c gofi.Context) error {
		// If SkipFilter evaluates to true, bypass the wrapped Handler
		// and proceed to the next middleware/handler in the chain
		if config.SkipFilter != nil && config.SkipFilter(c) {
			return c.Next()
		}

		// Otherwise, execute the wrapped handler
		return config.Handler(c)
	}
}
