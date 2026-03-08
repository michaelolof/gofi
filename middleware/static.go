package middleware

import (
	"strings"

	"github.com/michaelolof/gofi"
	"github.com/valyala/fasthttp"
)

// StaticConfig defines the config for the Static middleware.
type StaticConfig struct {
	// Root is the root directory where static assets are located.
	// Required.
	Root string

	// Prefix to strip from the request URL before serving the file.
	// Optional. Default: ""
	Prefix string

	// Index file for serving a directory.
	// Optional. Default: "index.html"
	Index string

	// Compress specifies whether to compress responses.
	// Optional. Default: false
	Compress bool
}

// StaticConfigDefault is the default config
var StaticConfigDefault = StaticConfig{
	Root:     ".",
	Prefix:   "",
	Index:    "index.html",
	Compress: false,
}

// Static creates a new middleware handler for serving static files
func Static(config ...StaticConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := StaticConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		if cfg.Root == "" {
			cfg.Root = StaticConfigDefault.Root
		}
		if cfg.Index == "" {
			cfg.Index = StaticConfigDefault.Index
		}
	}

	// Internal fasthttp FS handler setup
	fs := &fasthttp.FS{
		Root:               cfg.Root,
		IndexNames:         []string{cfg.Index},
		GenerateIndexPages: false,
		Compress:           cfg.Compress,
		AcceptByteRange:    true,
	}

	fsHandler := fs.NewRequestHandler()

	return func(c gofi.Context) error {
		// Only serve on GET and HEAD methods
		method := c.Method()
		if method != "GET" && method != "HEAD" {
			return c.Next()
		}

		path := c.Path()

		// If a Prefix is configured, only serve files if the path starts with it
		if cfg.Prefix != "" {
			if !strings.HasPrefix(path, cfg.Prefix) {
				return c.Next()
			}

			// Strip the prefix for the actual file lookup
			path = strings.TrimPrefix(path, cfg.Prefix)
			// Ensure it still starts with a slash
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}

			// Temporarily modify the request context so fasthttp.FS sees the stripped path
			c.Request().Context().Request.URI().SetPath(path)
		}

		// Delegate to fasthttp.FS handler
		fsHandler(c.Request().Context())

		// If fasthttp setting status to 404 meaning file not found, we proceed to next route/middleware
		// instead of hard stopping so the router can try to match other things
		if c.Request().Context().Response.StatusCode() == fasthttp.StatusNotFound {
			// Restore the original path so downstream handlers see the correct path
			if cfg.Prefix != "" {
				c.Request().Context().Request.URI().SetPath(c.Path())
			}

			// Reset the response body since the FS handler would have written "404 Not Found"
			c.Request().Context().Response.ResetBody()

			return c.Next()
		}

		return nil
	}
}
