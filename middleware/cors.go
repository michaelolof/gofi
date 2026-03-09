package middleware

import (
	"strconv"
	"strings"

	"github.com/michaelolof/gofi"
)

// CORSConfig defines the config for the CORS middleware.
type CORSConfig struct {
	// AllowOrigin defines a list of origins that may access the resource.
	// Optional. Default: "*"
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

// CORSConfigDefault is the default config
var CORSConfigDefault = CORSConfig{
	AllowOrigins:     "*",
	AllowMethods:     "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
	AllowHeaders:     "",
	AllowCredentials: false,
	ExposeHeaders:    "",
	MaxAge:           0,
}

// CORS creates a new middleware handler
func CORS(config ...CORSConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := CORSConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		if cfg.AllowOrigins == "" {
			cfg.AllowOrigins = CORSConfigDefault.AllowOrigins
		}

		if cfg.AllowMethods == "" {
			cfg.AllowMethods = CORSConfigDefault.AllowMethods
		}
	}

	return func(c gofi.Context) error {
		origin := c.HeaderVal("Origin")

		// If no Origin header, continue
		if origin == "" {
			return c.Next()
		}

		// Check allowed origins
		allowOrigin := ""
		if cfg.AllowOrigins == "*" {
			allowOrigin = "*"
		} else {
			origins := strings.Split(cfg.AllowOrigins, ",")
			for _, o := range origins {
				o = strings.TrimSpace(o)
				if o == origin {
					allowOrigin = origin
					break
				}
			}
		}

		// Origin not allowed
		if allowOrigin == "" {
			// We can either abort with 403 or just pass without appending headers.
			// Passing allows other handlers to decide.
			return c.Next()
		}

		c.Writer().Header().Set("Access-Control-Allow-Origin", allowOrigin)

		if cfg.AllowCredentials {
			c.Writer().Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if cfg.ExposeHeaders != "" {
			c.Writer().Header().Set("Access-Control-Expose-Headers", cfg.ExposeHeaders)
		}

		// Handle Preflight requests
		if c.Method() == "OPTIONS" {
			// Access-Control-Allow-Methods
			c.Writer().Header().Set("Access-Control-Allow-Methods", cfg.AllowMethods)

			// Access-Control-Allow-Headers
			if cfg.AllowHeaders != "" {
				c.Writer().Header().Set("Access-Control-Allow-Headers", cfg.AllowHeaders)
			} else {
				h := c.HeaderVal("Access-Control-Request-Headers")
				if h != "" {
					c.Writer().Header().Set("Access-Control-Allow-Headers", h)
				}
			}

			// Access-Control-Max-Age
			if cfg.MaxAge > 0 {
				c.Writer().Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
			}

			return c.SendString(204, "")
		}

		return c.Next()
	}
}
