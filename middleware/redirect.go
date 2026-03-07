package middleware

import (
	"strings"

	"github.com/michaelolof/gofi"
)

// RedirectConfig defines the config for the Redirect middleware.
type RedirectConfig struct {
	// Rules defines a map of URL path conditions to their redirected variants.
	// You can use string replacement for path segments.
	// e.g., map[string]string{"/old": "/new", "/old/*": "/new/$1"}
	// Optional. Default: nil
	Rules map[string]string

	// StatusCode sets the HTTP redirect status code.
	// Optional. Default: 302 Found
	StatusCode int
}

// RedirectConfigDefault is the default config
var RedirectConfigDefault = RedirectConfig{
	Rules:      nil,
	StatusCode: 302,
}

// Redirect creates a new middleware handler
func Redirect(config ...RedirectConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := RedirectConfigDefault

	if len(config) > 0 {
		cfg = config[0]

		if cfg.StatusCode == 0 {
			cfg.StatusCode = RedirectConfigDefault.StatusCode
		}
	}

	// Fast path if no rules
	if len(cfg.Rules) == 0 {
		return func(c gofi.Context) error {
			return c.Next()
		}
	}

	// Pre-process wildcard rules internally to split exact vs wildcard matching
	exactRules := make(map[string]string)
	wildcardRules := make(map[string]string)

	for pattern, replacement := range cfg.Rules {
		if strings.HasSuffix(pattern, "/*") {
			prefix := pattern[:len(pattern)-2]
			wildcardRules[prefix] = replacement
		} else {
			exactRules[pattern] = replacement
		}
	}

	return func(c gofi.Context) error {
		path := c.Path()

		// 1. Check exact matches first
		if redirectPath, ok := exactRules[path]; ok {
			c.Writer().Header().Set("Location", redirectPath)
			return c.SendString(cfg.StatusCode, "")
		}

		// 2. Check wildcard matches
		// E.g., rule "/old/*" -> "/new/$1"
		// If path is "/old/docs", prefix matched is "/old", capture is "/docs"
		for prefix, replacement := range wildcardRules {
			if strings.HasPrefix(path, prefix) {
				// The wildcard capture starts right after the prefix
				capture := path[len(prefix):]

				// Apply capture to replacement template
				target := strings.ReplaceAll(replacement, "$1", capture)

				// Strip double slashes just in case the template was "/new$1" and capture was "/docs"
				target = strings.ReplaceAll(target, "//", "/")

				c.Writer().Header().Set("Location", target)
				return c.SendString(cfg.StatusCode, "")
			}
		}

		return c.Next()
	}
}
