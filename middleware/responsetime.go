package middleware

import (
	"strconv"
	"time"

	"github.com/michaelolof/gofi"
)

// ResponseTimeConfig defines the config for the ResponseTime middleware.
type ResponseTimeConfig struct {
	// Header is the header key for the response time.
	// Optional. Default: "X-Response-Time"
	Header string

	// ValueFormat defines the format of the header value.
	// E.g., "%v" (duration), "%vms" (milliseconds).
	// Optional. Default: "ms" suffix, i.e., just the numerical ms followed by "ms"
	Suffix string
}

// ResponseTimeConfigDefault is the default config
var ResponseTimeConfigDefault = ResponseTimeConfig{
	Header: "X-Response-Time",
	Suffix: "ms",
}

// ResponseTime creates a new middleware handler
func ResponseTime(config ...ResponseTimeConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := ResponseTimeConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		if cfg.Header == "" {
			cfg.Header = ResponseTimeConfigDefault.Header
		}
	}

	return func(c gofi.Context) error {
		start := time.Now()

		err := c.Next()

		diff := time.Since(start)

		val := strconv.FormatFloat(float64(diff.Microseconds())/1000.0, 'f', 2, 64)

		c.Writer().Header().Set(cfg.Header, val+cfg.Suffix)

		return err
	}
}
