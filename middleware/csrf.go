package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/michaelolof/gofi"
)

// CSRFConfig defines the config for the CSRF middleware.
type CSRFConfig struct {
	// SigningKey signs tokens. Required: at least 32 bytes, identical on
	// every instance that serves the same users. Rotate it deliberately;
	// rotation invalidates outstanding tokens.
	SigningKey []byte

	// SessionID returns the identifier of the caller's authenticated session,
	// or "" for anonymous requests. Required. A token is valid only for the
	// session it was issued to, so a token planted by another site, or
	// issued before login, is rejected.
	SessionID func(c gofi.Context) string

	// TrustedOrigins lists additional origins (scheme://host[:port]) allowed
	// to send unsafe requests. An unsafe request whose Origin header is
	// neither its own origin nor listed is rejected; requests without an
	// Origin header (non-browser clients) are checked by token only.
	TrustedOrigins []string

	// KeyLookup is a string in the form of "<source>:<key>" that is used
	// to extract token from the request.
	// Optional. Default: "header:X-CSRF-Token"
	// Possible values: "header:<name>", "query:<name>", "param:<name>", "form:<name>"
	KeyLookup string

	// CookieName is the name of the CSRF cookie.
	// Optional. Default: "csrf_"
	CookieName string

	// CookieDomain is the domain of the CSRF cookie.
	// Optional. Default: ""
	CookieDomain string

	// CookiePath is the path of the CSRF cookie.
	// Optional. Default: "/"
	CookiePath string

	// CookieSecure forces the Secure attribute. It is also set whenever the
	// request scheme (gofi.Scheme) is https.
	// Optional. Default: false
	CookieSecure bool

	// CookieHTTPOnly hides the cookie from JavaScript. Leave it false when
	// client code reads the token from the cookie to send it back.
	// Optional. Default: false
	CookieHTTPOnly bool

	// CookieSameSite is Lax, Strict or None (None requires CookieSecure).
	// Optional. Default: "Lax"
	CookieSameSite string

	// Expiration is the cookie lifetime and the default token lifetime.
	// Optional. Default: 1 * time.Hour
	Expiration time.Duration

	// TokenTTL bounds token age independently of the cookie lifetime.
	// Optional. Default: Expiration
	TokenTTL time.Duration

	// ErrorHandler is executed when a request fails the CSRF check.
	// Optional. Default: returns 403 Forbidden.
	ErrorHandler gofi.HandlerFunc
}

// CSRFConfigDefault holds the defaults applied to optional fields.
var CSRFConfigDefault = CSRFConfig{
	KeyLookup:      "header:X-CSRF-Token",
	CookieName:     "csrf_",
	CookiePath:     "/",
	CookieSameSite: "Lax",
	Expiration:     1 * time.Hour,
	ErrorHandler: func(c gofi.Context) error {
		return gofi.NewHTTPError(http.StatusForbidden, "invalid CSRF token")
	},
}

const csrfTokenVersion = "v1"

// CSRF creates a CSRF middleware using session-bound, expiring, signed
// tokens (signed double-submit). The token is set in a cookie on every
// response, and unsafe requests (anything but GET, HEAD, OPTIONS, TRACE) must
// send the same token through KeyLookup, from an allowed origin. Token
// format: v1.<expiry unix seconds>.<random>.<signature>, where the HMAC-SHA256
// signature covers the version, expiry, random value and session ID.
func CSRF(cfg CSRFConfig) (gofi.MiddlewareFunc, error) {
	if len(cfg.SigningKey) < 32 {
		return nil, errors.New("gofi/middleware: CSRF SigningKey must be at least 32 bytes")
	}
	if cfg.SessionID == nil {
		return nil, errors.New("gofi/middleware: CSRF SessionID is required")
	}
	d := CSRFConfigDefault
	if cfg.KeyLookup == "" {
		cfg.KeyLookup = d.KeyLookup
	}
	if cfg.CookieName == "" {
		cfg.CookieName = d.CookieName
	}
	if cfg.CookiePath == "" {
		cfg.CookiePath = d.CookiePath
	}
	if cfg.CookieSameSite == "" {
		cfg.CookieSameSite = d.CookieSameSite
	}
	if cfg.Expiration < 0 || cfg.TokenTTL < 0 {
		return nil, errors.New("gofi/middleware: negative CSRF duration")
	}
	if cfg.Expiration == 0 {
		cfg.Expiration = d.Expiration
	}
	if cfg.TokenTTL == 0 {
		cfg.TokenTTL = cfg.Expiration
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = d.ErrorHandler
	}
	for name, value := range map[string]string{
		"CookieName": cfg.CookieName, "CookieDomain": cfg.CookieDomain,
		"CookiePath": cfg.CookiePath, "CookieSameSite": cfg.CookieSameSite,
	} {
		if strings.ContainsAny(value, ";,\r\n\x00 ") {
			return nil, errors.New("gofi/middleware: invalid CSRF " + name)
		}
	}
	var sameSite http.SameSite
	switch strings.ToLower(cfg.CookieSameSite) {
	case "lax":
		sameSite = http.SameSiteLaxMode
	case "strict":
		sameSite = http.SameSiteStrictMode
	case "none":
		if !cfg.CookieSecure {
			return nil, errors.New("gofi/middleware: CSRF SameSite=None requires CookieSecure")
		}
		sameSite = http.SameSiteNoneMode
	default:
		return nil, errors.New("gofi/middleware: CSRF CookieSameSite must be Lax, Strict or None")
	}
	source, key, ok := strings.Cut(cfg.KeyLookup, ":")
	if !ok || key == "" {
		return nil, errors.New("gofi/middleware: CSRF KeyLookup must be <source>:<key>")
	}
	switch source {
	case "header", "query", "param", "form":
	default:
		return nil, errors.New("gofi/middleware: CSRF KeyLookup source must be header, query, param or form")
	}
	trusted := map[string]bool{}
	for _, o := range cfg.TrustedOrigins {
		trusted[strings.TrimSuffix(o, "/")] = true
	}

	extract := func(c gofi.Context) string {
		switch source {
		case "header":
			return c.HeaderVal(key)
		case "query":
			return c.Query(key)
		case "param":
			return c.Param(key)
		default:
			_ = c.Request().ParseForm()
			return c.Request().PostForm.Get(key)
		}
	}
	sign := func(expiry, random, session string) string {
		mac := hmac.New(sha256.New, cfg.SigningKey)
		// Length-prefix each part so no two inputs share an encoding.
		for _, part := range []string{csrfTokenVersion, expiry, random, session} {
			mac.Write([]byte(strconv.Itoa(len(part)) + ":" + part))
		}
		return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	issue := func(session string) string {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		random := base64.RawURLEncoding.EncodeToString(b)
		expiry := strconv.FormatInt(time.Now().Add(cfg.TokenTTL).Unix(), 10)
		return strings.Join([]string{csrfTokenVersion, expiry, random, sign(expiry, random, session)}, ".")
	}
	valid := func(token, session string) bool {
		if len(token) > 256 {
			return false
		}
		parts := strings.Split(token, ".")
		if len(parts) != 4 || parts[0] != csrfTokenVersion {
			return false
		}
		expiry, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || time.Now().Unix() > expiry {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(sign(parts[1], parts[2], session)), []byte(parts[3])) == 1
	}

	return func(c gofi.Context) error {
		method := c.Method()
		safe := method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || method == http.MethodTrace
		session := cfg.SessionID(c)

		cookieToken := ""
		if cookie, err := c.Request().Cookie(cfg.CookieName); err == nil && valid(cookie.Value, session) {
			cookieToken = cookie.Value
		}

		if !safe {
			if origin := c.HeaderVal("Origin"); origin != "" {
				self := gofi.Scheme(c) + "://" + string(c.Request().Context().Host())
				if origin != self && !trusted[origin] {
					return cfg.ErrorHandler(c)
				}
			}
			clientToken := extract(c)
			if cookieToken == "" || clientToken == "" ||
				subtle.ConstantTimeCompare([]byte(cookieToken), []byte(clientToken)) != 1 {
				return cfg.ErrorHandler(c)
			}
		}

		token := cookieToken
		if token == "" {
			token = issue(session)
		}
		cookie := &http.Cookie{
			Name:     cfg.CookieName,
			Value:    token,
			Path:     cfg.CookiePath,
			Domain:   cfg.CookieDomain,
			MaxAge:   int(cfg.Expiration.Seconds()),
			HttpOnly: cfg.CookieHTTPOnly,
			Secure:   cfg.CookieSecure || gofi.Scheme(c) == "https",
			SameSite: sameSite,
		}
		c.Writer().Header().Add("Set-Cookie", cookie.String())
		return c.Next()
	}, nil
}
