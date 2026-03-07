package middleware

import (
	"log"
	"os"
	"time"

	"github.com/michaelolof/gofi"
)

// LoggerConfig defines the config for the Logger middleware.
type LoggerConfig struct {
	// Format defines the logging formatting string
	// Optional. Default: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path}\n"
	Format string

	// Output is the log destination
	// Optional. Default: os.Stdout
	Output *log.Logger
}

// LoggerConfigDefault is the default config
var LoggerConfigDefault = LoggerConfig{
	Format: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path}\n",
	Output: log.New(os.Stdout, "", 0),
}

// Logger creates a new middleware handler
func Logger(config ...LoggerConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := LoggerConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		if cfg.Format == "" {
			cfg.Format = LoggerConfigDefault.Format
		}
		if cfg.Output == nil {
			cfg.Output = LoggerConfigDefault.Output
		}
	}

	return func(c gofi.Context) error {
		start := time.Now()

		// Pass down the chain
		err := c.Next()

		latency := time.Since(start).String()

		// If no status is set yet, standard is 200
		statusCode := c.Writer().Header().Get("Status")
		if statusCode == "" {
			statusCode = "200"
		}

		ip := c.HeaderVal("X-Forwarded-For")
		if ip == "" {
			ip = "unknown"
		}
		method := c.Method()
		path := c.Path()
		timestr := time.Now().Format("2006/01/02 - 15:04:05")

		// Super simple format string replacement for performance instead of regex
		// For a full-fledged router, a buffer-writer template would be even faster
		cfg.Output.Printf("%s | %s | %s | %s | %s | %s\n",
			timestr, statusCode, latency, ip, method, path)

		return err
	}
}
