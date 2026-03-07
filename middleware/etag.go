package middleware

import (
	"bytes"
	"fmt"
	"hash/crc32"

	"github.com/michaelolof/gofi"
)

// ETagConfig defines the config for the ETag middleware.
type ETagConfig struct {
	// Weak indicates that a weak validator should be used.
	// Weak ETags are easy to generate but are far less useful for comparisons.
	// Strong validators are ideal for comparisons but very difficult to generate.
	// Optional. Default: false
	Weak bool
}

// ETagConfigDefault is the default config
var ETagConfigDefault = ETagConfig{
	Weak: false,
}

// ETag creates a new middleware handler
func ETag(config ...ETagConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := ETagConfigDefault

	if len(config) > 0 {
		cfg = config[0]
	}

	return func(c gofi.Context) error {
		// Only check GET and HEAD methods
		method := c.Method()
		if method != "GET" && method != "HEAD" {
			return c.Next()
		}

		// Proceed to handler
		err := c.Next()

		// Get response body and compute ETag
		fctx := c.Request().Context()
		statusCode := fctx.Response.StatusCode()

		// Mostly check 200 OK since we don't need to tag errors typically
		if statusCode != 200 {
			return err
		}

		body := fctx.Response.Body()
		// If body is empty, no ETag is generated
		if len(body) == 0 {
			return err
		}

		// Calculate ETag
		// A fast hashing method like CRC32 is often used for strong ETags in web frameworks
		etag := fmt.Sprintf("\"%x-%x\"", len(body), crc32.ChecksumIEEE(body))
		if cfg.Weak {
			etag = "W/" + etag
		}

		// Set ETag header
		c.Writer().Header().Set("ETag", etag)

		// Check If-None-Match header
		clientETag := c.HeaderVal("If-None-Match")

		// To properly handle If-None-Match, it can be a comma separated list
		if bytes.Contains(c.Request().Context().Request.Header.Peek("If-None-Match"), []byte(etag)) || clientETag == "*" {
			// Serve 304 Not Modified
			c.Request().Context().Response.ResetBody()
			c.Request().Context().Response.SetStatusCode(304)
			return nil
		}

		return err
	}
}
