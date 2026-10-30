package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/michaelolof/gofi"
)

// CORSConfig defines the config for the CORS middleware.
type CORSConfig struct {
	// AllowOrigins is a comma-separated list of origins (scheme://host[:port])
	// that may access the resource, or "*" for any origin.
	// Required.
	AllowOrigins string

	// AllowMethods defines a list methods allowed when accessing the resource.
	// This is used in response to a preflight request.
	// Optional. Default: "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS"
	AllowMethods string

	// AllowHeaders defines a list of request headers that can be used when
	// making the actual request. This is in response to a preflight request.
	// Optional. Default: ""
	AllowHeaders string

	// AllowCredentials indicates whether or not the response to the request
	// can be exposed when the credentials flag is true.
	// Optional. Default: false
	AllowCredentials bool

	// ExposeHeaders defines a whitelist headers that clients are allowed to access.
	// Optional. Default: ""
	ExposeHeaders string

	// MaxAge indicates how long (in seconds) the results of a preflight request
	// can be cached.
	// Optional. Default: 0
	MaxAge int
}

// CORSConfigDefault holds the defaults applied to optional fields.
var CORSConfigDefault = CORSConfig{
	AllowMethods: "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
}

// CORS validates cfg and creates a CORS middleware. It rejects a wildcard
// origin combined with credentials, origins that are not scheme://host[:port],
// and a negative MaxAge. Requested headers are allowed only if listed in
// AllowHeaders, and a preflight for a method not in AllowMethods receives no
// allow headers.
//
// Responses whose CORS headers depend on the request Origin carry
// "Vary: Origin", including responses to disallowed origins, so shared caches
// do not reuse one origin's decision for another. Only an OPTIONS request with
// an Access-Control-Request-Method header is answered as a preflight; other
// OPTIONS requests continue down the chain.
func CORS(cfg CORSConfig) (gofi.MiddlewareFunc, error) {
	if cfg.AllowOrigins == "" {
		return nil, errors.New("gofi/middleware: CORS AllowOrigins is required")
	}
	if cfg.AllowMethods == "" {
		cfg.AllowMethods = CORSConfigDefault.AllowMethods
	}
	if cfg.MaxAge < 0 {
		return nil, errors.New("gofi/middleware: negative CORS MaxAge")
	}
	if cfg.AllowOrigins == "*" {
		if cfg.AllowCredentials {
			return nil, errors.New("gofi/middleware: CORS cannot allow credentials for every origin")
		}
	} else {
		for _, o := range splitList(cfg.AllowOrigins) {
			u, err := url.Parse(o)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
				return nil, fmt.Errorf("gofi/middleware: invalid CORS origin %q", o)
			}
		}
	}
	return newCORSHandler(cfg), nil
}

func newCORSHandler(cfg CORSConfig) gofi.MiddlewareFunc {
	wildcard := cfg.AllowOrigins == "*"
	origins := map[string]bool{}
	if !wildcard {
		for _, o := range splitList(cfg.AllowOrigins) {
			origins[o] = true
		}
	}
	allowedMethods := map[string]bool{}
	for _, m := range splitList(cfg.AllowMethods) {
		allowedMethods[strings.ToUpper(m)] = true
	}

	return func(c gofi.Context) error {
		h := c.Writer().Header()
		if !wildcard {
			addVary(h, "Origin")
		}

		origin := c.HeaderVal("Origin")

		// If no Origin header, continue
		if origin == "" {
			return c.Next()
		}

		// Origin not allowed: pass without CORS headers. The browser withholds
		// the response; the request itself is not rejected here.
		if !wildcard && !origins[origin] {
			return c.Next()
		}
		allowOrigin := "*"
		if !wildcard {
			allowOrigin = origin
		}

		h.Set("Access-Control-Allow-Origin", allowOrigin)

		if cfg.AllowCredentials {
			h.Set("Access-Control-Allow-Credentials", "true")
		}

		if cfg.ExposeHeaders != "" {
			h.Set("Access-Control-Expose-Headers", cfg.ExposeHeaders)
		}

		requestedMethod := c.HeaderVal("Access-Control-Request-Method")
		if c.Method() != "OPTIONS" || requestedMethod == "" {
			return c.Next()
		}

		// Preflight
		addVary(h, "Access-Control-Request-Method")
		addVary(h, "Access-Control-Request-Headers")
		if allowedMethods[strings.ToUpper(requestedMethod)] {
			h.Set("Access-Control-Allow-Methods", cfg.AllowMethods)

			if cfg.AllowHeaders != "" {
				h.Set("Access-Control-Allow-Headers", cfg.AllowHeaders)
			}

			if cfg.MaxAge > 0 {
				h.Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
			}
		}

		return c.SendString(204, "")
	}
}

// addVary merges token into the Vary header without duplicating it.
func addVary(h http.Header, token string) {
	for _, v := range h.Values("Vary") {
		for _, existing := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(existing), token) {
				return
			}
		}
	}
	h.Add("Vary", token)
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
