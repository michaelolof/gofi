package middleware

import (
	"log"
	"os"
	"strconv"
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

		statusCode := strconv.Itoa(c.Request().Context().Response.StatusCode())
		if statusCode == "0" {
			statusCode = "200"
		}

		ip := c.HeaderVal("X-Forwarded-For")
		if ip == "" {
			ip = "unknown"
		}
		ip = sanitizeLogField(ip)
		method := sanitizeLogField(c.Method())
		path := sanitizeLogField(c.Path())
		timestr := time.Now().Format("2006/01/02 - 15:04:05")

		// Super simple format string replacement for performance instead of regex
		// For a full-fledged router, a buffer-writer template would be even faster
		cfg.Output.Printf("%s | %s | %s | %s | %s | %s\n",
			timestr, statusCode, latency, ip, method, path)

		return err
	}
}

func sanitizeLogField(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			buf := make([]byte, len(s))
			copy(buf, s)
			for j := i; j < len(buf); j++ {
				if buf[j] < 0x20 || buf[j] == 0x7f {
					buf[j] = ' '
				}
			}
			return string(buf)
		}
	}

	return s
}
