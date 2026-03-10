package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/michaelolof/gofi"
)

// CSRFConfig defines the config for the CSRF middleware.
type CSRFConfig struct {
	// KeyLookup is a string in the form of "<source>:<key>" that is used
	// to extract token from the request.
	// Optional. Default: "header:X-CSRF-Token"
	// Possible values:
	// - "header:<name>"
	// - "query:<name>"
	// - "param:<name>"
	// - "form:<name>"
	KeyLookup string

	// CookieName is the name of the session cookie.
	// Optional. Default: "csrf_"
	CookieName string

	// CookieDomain is the domain of the session cookie.
	// Optional. Default: ""
	CookieDomain string

	// CookiePath is the path of the session cookie.
	// Optional. Default: ""
	CookiePath string

	// CookieSecure indicates if the session cookie is secure.
	// Optional. Default: false
	CookieSecure bool

	// CookieHTTPOnly indicates if the session cookie is HTTP only.
	// Optional. Default: false
	CookieHTTPOnly bool

	// CookieSameSite indicates the SameSite attribute of the session cookie.
	// Optional. Default: "Lax"
	CookieSameSite string

	// Expiration is the duration before the CSRF token will expire.
	// Optional. Default: 1 * time.Hour
	Expiration time.Duration

	// KeyGenerator creates a new CSRF token
	// Optional. Default: random 32 byte hex string
	KeyGenerator func() string

	// ErrorHandler is executed when an extracted CSRF token is invalid.
	// Optional. Default: returns 403 Forbidden.
	ErrorHandler gofi.HandlerFunc

	// SignTokens enables signed CSRF tokens using HMAC-SHA256.
	// Optional. Default: false
	SignTokens bool

	// SigningKey is used to sign/verify CSRF tokens when SignTokens is enabled.
	// If empty and SignTokens is true, an ephemeral key is generated at startup.
	SigningKey []byte
}

// DefaultCSRFGenerator generates a random 32-byte hex string
func DefaultCSRFGenerator() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func defaultCSRFSigningKey() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}

func signCSRFToken(raw string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(raw))
	sig := mac.Sum(nil)
	return raw + "." + hex.EncodeToString(sig)
}

func verifySignedCSRFToken(signed string, key []byte) bool {
	idx := strings.LastIndexByte(signed, '.')
	if idx <= 0 || idx == len(signed)-1 {
		return false
	}

	raw := signed[:idx]
	sigHex := signed[idx+1:]
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(raw))
	expected := mac.Sum(nil)
	return subtle.ConstantTimeCompare(sig, expected) == 1
}

// CSRFConfigDefault is the default config
var CSRFConfigDefault = CSRFConfig{
	KeyLookup:      "header:X-CSRF-Token",
	CookieName:     "csrf_",
	CookieSameSite: "Lax",
	Expiration:     1 * time.Hour,
	KeyGenerator:   DefaultCSRFGenerator,
	ErrorHandler: func(c gofi.Context) error {
		return c.SendString(http.StatusForbidden, "Invalid CSRF Token")
	},
}

// CSRF creates a new middleware handler
func CSRF(config ...CSRFConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := CSRFConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		if cfg.KeyLookup == "" {
			cfg.KeyLookup = CSRFConfigDefault.KeyLookup
		}
		if cfg.CookieName == "" {
			cfg.CookieName = CSRFConfigDefault.CookieName
		}
		if cfg.Expiration == 0 {
			cfg.Expiration = CSRFConfigDefault.Expiration
		}
		if cfg.KeyGenerator == nil {
			cfg.KeyGenerator = CSRFConfigDefault.KeyGenerator
		}
		if cfg.ErrorHandler == nil {
			cfg.ErrorHandler = CSRFConfigDefault.ErrorHandler
		}
		if cfg.CookieSameSite == "" {
			cfg.CookieSameSite = CSRFConfigDefault.CookieSameSite
		}
	}

	if cfg.SignTokens && len(cfg.SigningKey) == 0 {
		cfg.SigningKey = defaultCSRFSigningKey()
	}

	// Parse KeyLookup
	source := "header"
	key := "X-CSRF-Token"
	lookupParts := strings.SplitN(cfg.KeyLookup, ":", 2)
	if len(lookupParts) == 2 {
		source = lookupParts[0]
		key = lookupParts[1]
	}

	// Internal extractor function
	extractToken := func(c gofi.Context) string {
		switch source {
		case "header":
			return c.HeaderVal(key)
		case "query":
			return c.Query(key)
		case "param":
			return c.Param(key)
		case "form":
			_ = c.Request().ParseForm()
			return c.Request().PostForm.Get(key)
		}
		return ""
	}

	return func(c gofi.Context) error {
		// Action safe methods don't need CSRF validation, but we still generate/refresh the token
		method := c.Method()
		isSafe := method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || method == http.MethodTrace

		// 1. Check if the token already exists in the cookie
		var token string
		cookieValid := false
		cookie, err := c.Request().Cookie(cfg.CookieName)
		if err == nil && cookie != nil && cookie.Value != "" {
			token = cookie.Value
			if cfg.SignTokens {
				cookieValid = verifySignedCSRFToken(token, cfg.SigningKey)
			} else {
				cookieValid = true
			}
		}

		// 2. Generate a new token if missing/invalid
		if token == "" || !cookieValid {
			raw := cfg.KeyGenerator()
			if cfg.SignTokens {
				token = signCSRFToken(raw, cfg.SigningKey)
			} else {
				token = raw
			}
		}

		// 3. For state-changing methods, validate the token against the extracted client token
		if !isSafe {
			clientToken := extractToken(c)

			if cfg.SignTokens {
				if !cookieValid {
					return cfg.ErrorHandler(c)
				}
				if clientToken == "" || !verifySignedCSRFToken(clientToken, cfg.SigningKey) {
					return cfg.ErrorHandler(c)
				}
			}

			// Constant time comparison to prevent timing attacks
			if clientToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(clientToken)) != 1 {
				return cfg.ErrorHandler(c)
			}
		}

		// 4. Set the CSRF token in the response cookie so the client has it for future requests
		// Using fast builtin header formatting in gofi context
		cookieHeader := cfg.CookieName + "=" + token + "; Path="

		if cfg.CookiePath != "" {
			cookieHeader += cfg.CookiePath
		} else {
			cookieHeader += "/"
		}

		if cfg.CookieDomain != "" {
			cookieHeader += "; Domain=" + cfg.CookieDomain
		}

		// Set Expiration using Max-Age
		cookieHeader += "; Max-Age=" + strconv.Itoa(int(cfg.Expiration.Seconds()))

		if cfg.CookieHTTPOnly {
			cookieHeader += "; HttpOnly"
		}
		secureCookie := cfg.CookieSecure || c.Request().Context().IsTLS()
		if secureCookie {
			cookieHeader += "; Secure"
		}
		cookieHeader += "; SameSite=" + cfg.CookieSameSite

		c.Writer().Header().Add("Set-Cookie", cookieHeader)

		return c.Next()
	}
}
