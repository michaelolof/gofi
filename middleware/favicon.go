package middleware

import (
	"os"

	"github.com/michaelolof/gofi"
	"github.com/valyala/fasthttp"
)

// FaviconConfig defines the config for the Favicon middleware.
type FaviconConfig struct {
	// File holds the path to an actual favicon file.
	// Optional. Default: ""
	File string

	// URL for the favicon to serve.
	// Optional. Default: "/favicon.ico"
	URL string

	// CacheControl determines the Cache-Control header.
	// Optional. Default: "public, max-age=31536000"
	CacheControl string
}

// FaviconConfigDefault is the default config
var FaviconConfigDefault = FaviconConfig{
	File:         "",
	URL:          "/favicon.ico",
	CacheControl: "public, max-age=31536000",
}

var (
	// defaultFavicon represents a transparent pixel or standard minimal valid icon
	defaultFavicon = []byte{
		0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10, 0x00, 0x00,
		0x01, 0x00, 0x18, 0x00, 0x68, 0x00, 0x00, 0x00, 0x16, 0x00,
		0x00, 0x00, 0x28, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00,
		0x20, 0x00, 0x00, 0x00, 0x01, 0x00, 0x18, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x13, 0x0b, 0x00, 0x00,
		0x13, 0x0b, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
)

// Favicon creates a new middleware handler
func Favicon(config ...FaviconConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := FaviconConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		if cfg.URL == "" {
			cfg.URL = FaviconConfigDefault.URL
		}
		if cfg.CacheControl == "" {
			cfg.CacheControl = FaviconConfigDefault.CacheControl
		}
	}

	// Read actual file if provided, otherwise serve the minimal transparent icon payload
	iconData := defaultFavicon
	if cfg.File != "" {
		if data, err := os.ReadFile(cfg.File); err == nil {
			iconData = data
		}
	}

	return func(c gofi.Context) error {
		// Only serve on configured route and specific methods
		if c.Path() != cfg.URL {
			return c.Next()
		}

		if c.Method() != "GET" && c.Method() != "HEAD" {
			return c.Next()
		}

		// It's the favicon route: Serve it
		c.Writer().Header().Set(fasthttp.HeaderContentType, "image/x-icon")
		c.Writer().Header().Set(fasthttp.HeaderCacheControl, cfg.CacheControl)

		// c.Send(statusCode, byte slice data)
		return c.SendString(fasthttp.StatusOK, string(iconData))
	}
}
