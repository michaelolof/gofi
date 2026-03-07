package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/michaelolof/gofi"
)

// RequestIDConfig defines the config for the RequestID middleware.
type RequestIDConfig struct {
	// Header is the header key where to get/set the request ID.
	// Optional. Default: "X-Request-Id"
	Header string

	// Generator is the function that handles generating the request ID
	// Optional. Default: fast 16-byte hex generator
	Generator func() string
}

// DefaultRequestIDGenerator generates a random 16-bye hex string (32 characters)
func DefaultRequestIDGenerator() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RequestIDConfigDefault is the default config
var RequestIDConfigDefault = RequestIDConfig{
	Header:    "X-Request-Id",
	Generator: DefaultRequestIDGenerator,
}

// RequestID creates a new middleware handler
func RequestID(config ...RequestIDConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := RequestIDConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		if cfg.Header == "" {
			cfg.Header = RequestIDConfigDefault.Header
		}
		if cfg.Generator == nil {
			cfg.Generator = RequestIDConfigDefault.Generator
		}
	}

	return func(c gofi.Context) error {
		// Get ID from request
		rid := c.HeaderVal(cfg.Header)

		// Create new ID if empty
		if rid == "" {
			rid = cfg.Generator()
		}

		// Set the ID to the response header
		c.Writer().Header().Set(cfg.Header, rid)

		return c.Next()
	}
}
