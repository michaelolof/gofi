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

// CSRFSessionOptions configures the session-bound CSRF token design used by
// NewCSRF.
type CSRFSessionOptions struct {
	// SigningKey signs tokens. It must be at least 32 bytes and identical on
	// every instance that serves the same users; rotate it deliberately.
	SigningKey []byte
	// SessionID returns the identifier of the caller's authenticated session,
	// or "" for anonymous requests. A token is only valid for the session it
	// was issued to, so a token planted by another site, or issued before
	// login, is rejected.
	SessionID func(c gofi.Context) string
	// TokenTTL bounds token age independently of the cookie lifetime.
	// Default: the CSRFConfig Expiration.
	TokenTTL time.Duration
	// TrustedOrigins lists additional origins (scheme://host[:port]) allowed
	// to send unsafe requests. Requests with an Origin header that is neither
	// the request's own origin nor listed are rejected; requests without an
	// Origin header (non-browser clients) are checked by token only.
	TrustedOrigins []string
}

const csrfTokenVersion = "v1"

// NewCSRF creates a CSRF middleware whose tokens are bound to a session,
// carry an expiry, and are signed with a required, shared key. It validates
// the configuration instead of generating per-process secrets. Token format:
// v1.<expiry unix seconds>.<random>.<signature>, where the signature covers
// the version, expiry, random value and session ID.
func NewCSRF(cfg CSRFConfig, opts CSRFSessionOptions) (gofi.MiddlewareFunc, error) {
	if len(opts.SigningKey) < 32 {
		return nil, errors.New("gofi/middleware: CSRF SigningKey must be at least 32 bytes")
	}
	if opts.SessionID == nil {
		return nil, errors.New("gofi/middleware: CSRF SessionID is required")
	}
	if cfg.KeyLookup == "" {
		cfg.KeyLookup = CSRFConfigDefault.KeyLookup
	}
	if cfg.CookieName == "" {
		cfg.CookieName = CSRFConfigDefault.CookieName
	}
	if cfg.Expiration <= 0 {
		cfg.Expiration = CSRFConfigDefault.Expiration
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = CSRFConfigDefault.ErrorHandler
	}
	if cfg.CookieSameSite == "" {
		cfg.CookieSameSite = CSRFConfigDefault.CookieSameSite
	}
	ttl := opts.TokenTTL
	if ttl <= 0 {
		ttl = cfg.Expiration
	}
	for name, value := range map[string]string{
		"CookieName": cfg.CookieName, "CookieDomain": cfg.CookieDomain,
		"CookiePath": cfg.CookiePath, "CookieSameSite": cfg.CookieSameSite,
	} {
		if strings.ContainsAny(value, ";,\r\n\x00 ") {
			return nil, errors.New("gofi/middleware: invalid CSRF " + name)
		}
	}
	switch strings.ToLower(cfg.CookieSameSite) {
	case "lax", "strict", "none":
	default:
		return nil, errors.New("gofi/middleware: CSRF CookieSameSite must be Lax, Strict or None")
	}
	if strings.EqualFold(cfg.CookieSameSite, "none") && !cfg.CookieSecure {
		return nil, errors.New("gofi/middleware: CSRF SameSite=None requires CookieSecure")
	}
	trusted := map[string]bool{}
	for _, o := range opts.TrustedOrigins {
		trusted[strings.TrimSuffix(o, "/")] = true
	}

	source, key, _ := strings.Cut(cfg.KeyLookup, ":")
	extract := func(c gofi.Context) string {
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

	sign := func(expiry, random, session string) string {
		mac := hmac.New(sha256.New, opts.SigningKey)
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
		expiry := strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)
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
		want := sign(parts[1], parts[2], session)
		return subtle.ConstantTimeCompare([]byte(want), []byte(parts[3])) == 1
	}

	return func(c gofi.Context) error {
		method := c.Method()
		safe := method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || method == http.MethodTrace
		session := opts.SessionID(c)

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
		}
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		switch strings.ToLower(cfg.CookieSameSite) {
		case "strict":
			cookie.SameSite = http.SameSiteStrictMode
		case "none":
			cookie.SameSite = http.SameSiteNoneMode
		default:
			cookie.SameSite = http.SameSiteLaxMode
		}
		c.Writer().Header().Add("Set-Cookie", cookie.String())
		return c.Next()
	}, nil
}
