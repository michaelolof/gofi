package middleware

import (
	"net/http"

	"github.com/michaelolof/gofi"
)

// EarlyDataConfig defines the config for the EarlyData middleware.
type EarlyDataConfig struct {
	// IsEarlyData returns true if the request is considered early data.
	// Optional. Default: checks if "Early-Data" header is "1".
	IsEarlyData func(c gofi.Context) bool

	// AllowEarlyData evaluates whether the early data request is safe to process.
	// Optional. Default: true for safe methods (GET, HEAD, OPTIONS, TRACE).
	AllowEarlyData func(c gofi.Context) bool

	// ErrorHandler is executed when an early data request is rejected.
	// Optional. Default: returns 425 Too Early.
	ErrorHandler gofi.HandlerFunc
}

// EarlyDataConfigDefault is the default config
var EarlyDataConfigDefault = EarlyDataConfig{
	IsEarlyData: func(c gofi.Context) bool {
		return c.HeaderVal("Early-Data") == "1"
	},
	AllowEarlyData: func(c gofi.Context) bool {
		// Safe methods according to RFC 7231
		method := c.Method()
		return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || method == http.MethodTrace
	},
	ErrorHandler: func(c gofi.Context) error {
		return gofi.NewHTTPError(http.StatusTooEarly, "too early")
	},
}

// EarlyData creates a new middleware handler
func EarlyData(config ...EarlyDataConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := EarlyDataConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		if cfg.IsEarlyData == nil {
			cfg.IsEarlyData = EarlyDataConfigDefault.IsEarlyData
		}
		if cfg.AllowEarlyData == nil {
			cfg.AllowEarlyData = EarlyDataConfigDefault.AllowEarlyData
		}
		if cfg.ErrorHandler == nil {
			cfg.ErrorHandler = EarlyDataConfigDefault.ErrorHandler
		}
	}

	return func(c gofi.Context) error {
		// If it's an early data request and we don't allow it, reject early
		if cfg.IsEarlyData(c) && !cfg.AllowEarlyData(c) {
			return cfg.ErrorHandler(c)
		}

		return c.Next()
	}
}
