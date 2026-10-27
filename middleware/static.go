package middleware

import (
	"log"
	"os"
	"path"
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

// Static creates a new middleware handler for serving static files.
//
// Paths with any segment beginning with "." (such as /.env or /.git/config)
// are never served. Directory requests that have no index file fall through to
// the next handler instead of answering with a redirect or 403. Configure Root
// explicitly: without it the working directory is served.
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

	if len(config) == 0 || config[0].Root == "" {
		log.Printf("gofi/middleware: Static has no Root and serves the working directory; set StaticConfig.Root to a directory that contains only public files")
	}

	// Match the prefix on segment boundaries: "/public" serves "/public" and
	// "/public/...", never "/publicity".
	prefix := strings.TrimSuffix(cfg.Prefix, "/")

	// Read files through an os.Root so symlinks cannot escape Root. With a
	// custom filesystem fasthttp also keeps compressed copies in memory rather
	// than writing them into the served directory.
	dir, err := os.OpenRoot(cfg.Root)
	if err != nil {
		log.Printf("gofi/middleware: Static cannot open Root %q: %v; passing requests through", cfg.Root, err)
		return func(c gofi.Context) error { return c.Next() }
	}
	fs := &fasthttp.FS{
		FS:                 dir.FS(),
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

		// Explicit path traversal sanitization before prefix matching
		path := pathClean(c.Path())

		if prefix != "" {
			if path != prefix && !strings.HasPrefix(path, prefix+"/") {
				return c.Next()
			}
			path = strings.TrimPrefix(path, prefix)
			if path == "" {
				path = "/"
			}
		}

		// pathClean has removed "." and ".." segments, so any remaining "/."
		// starts a hidden file or directory name.
		if strings.Contains(path, "/.") {
			return c.Next()
		}

		fctx := c.Request().Context()
		uri := fctx.Request.URI()
		var originalPath []byte
		if prefix != "" {
			// Save the raw path before rewriting so the fallback restores the
			// request exactly; reading c.Path() afterwards would see the rewrite.
			originalPath = append(originalPath, uri.PathOriginal()...)
			uri.SetPath(path)
		}

		// Delegate to fasthttp.FS handler
		fsHandler(fctx)

		// Missing files and directory outcomes (redirect to a trailing slash,
		// or a forbidden directory index) fall through to the next handler so
		// routes are not shadowed by files or directories.
		switch fctx.Response.StatusCode() {
		case fasthttp.StatusNotFound, fasthttp.StatusForbidden, fasthttp.StatusFound, fasthttp.StatusMovedPermanently:
			if prefix != "" {
				uri.SetPathBytes(originalPath)
			}
			fctx.Response.ResetBody()
			fctx.Response.Header.Del("Location")
			fctx.Response.SetStatusCode(fasthttp.StatusOK)
			return c.Next()
		}

		return nil
	}
}

// pathClean cleans the path, eliminating traversal anomalies.
// Adapted from standard library path.Clean
func pathClean(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	cp := path.Clean(p)
	if cp == "." {
		return "/"
	}
	return cp
}
