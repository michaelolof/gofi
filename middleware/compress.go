package middleware

import (
	"github.com/michaelolof/gofi"
	"github.com/valyala/fasthttp"
)

// CompressConfig defines the config for the Compress middleware.
type CompressConfig struct {
	// Level determines the compression algorithm
	// -1 : Default compression level
	// 0  : No compression
	// 1  : Best speed
	// 9  : Best compression
	// Optional. Default: -1
	Level int
}

// CompressConfigDefault is the default config
var CompressConfigDefault = CompressConfig{
	Level: -1,
}

// Compress creates a new middleware handler
func Compress(config ...CompressConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := CompressConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]
	}

	// Create fasthttp compress handler
	// Fasthttp automatically handles Accept-Encoding negotiations (gzip, deflate, brotli)
	compressHandler := fasthttp.CompressHandlerBrotliLevel(func(ctx *fasthttp.RequestCtx) {}, cfg.Level, cfg.Level)

	return func(c gofi.Context) error {
		// First pass the request down the chain
		err := c.Next()

		// Then apply compression to the response body
		// We extract the underlying fasthttp RequestCtx from the Gofi Request adapter
		fctx := c.Request().Context()

		// The fascinating thing about fasthttp.CompressHandler is that it acts as a request middleware.
		// Since we want to compress AFTER the handler runs, we can trick fasthttp into compressing
		// the existing response body by executing the handler with a no-op function.
		compressHandler(fctx)

		// Also remove Content-Length as compression changes it and fasthttp will recalculate it
		fctx.Response.Header.Del("Content-Length")

		return err
	}
}
