package middleware

import (
	"strings"

	"github.com/michaelolof/gofi"
)

// RewriteConfig defines the config for the Rewrite middleware.
type RewriteConfig struct {
	// Rules defines a map of URL path conditions to their rewritten variants.
	// E.g., map[string]string{"/old": "/new", "/old/*": "/new/$1"}
	// Optional. Default: nil
	Rules map[string]string
}

// RewriteConfigDefault is the default config
var RewriteConfigDefault = RewriteConfig{
	Rules: nil,
}

// Rewrite creates a new middleware handler.
//
// Rewrite runs after routing: it changes the path that downstream handlers
// see (c.Path()), but the route, its middleware chain and its authorization
// have already been chosen from the original path. It does not re-route the
// request to another handler.
func Rewrite(config ...RewriteConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := RewriteConfigDefault

	if len(config) > 0 {
		cfg = config[0]
	}

	// Fast path if no rules
	if len(cfg.Rules) == 0 {
		return func(c gofi.Context) error {
			return c.Next()
		}
	}

	// Pre-process wildcard rules internally
	exactRules, wildcardRules := compilePrefixRules(cfg.Rules)

	return func(c gofi.Context) error {
		path := c.Path()

		// 1. Check exact matches first
		if rewritePath, ok := exactRules[path]; ok {
			// Actually modify the request URI internally so subsequent handlers see the new route
			// fasthttp RequestCtx provides SetRequestURI to override for routing
			c.Request().Context().Request.URI().SetPath(rewritePath)
			return c.Next()
		}

		// 2. Check wildcard matches
		// Rules match on segment boundaries, longest prefix first.
		if rule, ok := matchPrefixRule(wildcardRules, path); ok {
			prefix, replacement := rule.prefix, rule.replacement
			{
				capture := path[len(prefix):]
				target := strings.ReplaceAll(replacement, "$1", capture)
				target = strings.ReplaceAll(target, "//", "/")

				// Rewrite the path internally
				c.Request().Context().Request.URI().SetPath(target)
				return c.Next()
			}
		}

		return c.Next()
	}
}
