package gofi

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// TrustOptions configures which proxies may report the client address and
// scheme through forwarding headers.
type TrustOptions struct {
	// TrustedProxies lists proxy addresses or CIDR ranges (for example
	// "10.0.0.0/8" or "192.0.2.10"). Forwarding headers are ignored unless
	// the connection comes from one of them.
	TrustedProxies []string
	// ForwardedForHeader carries the client chain. Default: X-Forwarded-For.
	ForwardedForHeader string
	// ForwardedProtoHeader carries the client scheme. Default: X-Forwarded-Proto.
	ForwardedProtoHeader string
	// MaxForwardedEntries bounds how many chain entries are examined, from the
	// right. Default: 16.
	MaxForwardedEntries int
}

// TrustPolicy resolves the client IP and scheme of a request, starting from
// the socket peer.
type TrustPolicy struct {
	prefixes     []netip.Prefix
	forwardedFor string
	forwardedPro string
	maxEntries   int
}

// NewTrustPolicy validates opts and compiles the trusted proxy ranges.
func NewTrustPolicy(opts TrustOptions) (*TrustPolicy, error) {
	p := &TrustPolicy{
		forwardedFor: opts.ForwardedForHeader,
		forwardedPro: opts.ForwardedProtoHeader,
		maxEntries:   opts.MaxForwardedEntries,
	}
	if p.forwardedFor == "" {
		p.forwardedFor = "X-Forwarded-For"
	}
	if p.forwardedPro == "" {
		p.forwardedPro = "X-Forwarded-Proto"
	}
	if p.maxEntries < 0 {
		return nil, errors.New("gofi: negative TrustOptions.MaxForwardedEntries")
	}
	if p.maxEntries == 0 {
		p.maxEntries = 16
	}
	for _, entry := range opts.TrustedProxies {
		entry = strings.TrimSpace(entry)
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			p.prefixes = append(p.prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("gofi: invalid trusted proxy %q", entry)
		}
		addr = addr.Unmap()
		p.prefixes = append(p.prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return p, nil
}

func (p *TrustPolicy) trusted(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range p.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// resolve returns the client IP and scheme for the request in c.
func (p *TrustPolicy) resolve(c *context) (string, string) {
	scheme := "http"
	if c.fctx.IsTLS() {
		scheme = "https"
	}
	peer, ok := netip.AddrFromSlice(c.fctx.RemoteIP())
	if !ok {
		return c.fctx.RemoteIP().String(), scheme
	}
	peer = peer.Unmap()
	if !p.trusted(peer) {
		return peer.String(), scheme
	}

	// The connection comes from a trusted proxy: walk the chain from the
	// right (nearest hop) and stop at the first address not trusted.
	var entries []string
	for _, value := range c.fctx.Request.Header.PeekAll(p.forwardedFor) {
		entries = append(entries, strings.Split(string(value), ",")...)
	}
	client := peer
	for i, examined := len(entries)-1, 0; i >= 0 && examined < p.maxEntries; i, examined = i-1, examined+1 {
		hop, err := netip.ParseAddr(strings.TrimSpace(entries[i]))
		if err != nil {
			break // malformed entry: keep the nearest address we can vouch for
		}
		client = hop.Unmap()
		if !p.trusted(client) {
			break
		}
	}

	if !c.fctx.IsTLS() {
		values := c.fctx.Request.Header.PeekAll(p.forwardedPro)
		if len(values) > 0 {
			last := string(values[len(values)-1])
			if i := strings.LastIndexByte(last, ','); i >= 0 {
				last = last[i+1:]
			}
			switch strings.ToLower(strings.TrimSpace(last)) {
			case "https":
				scheme = "https"
			case "http":
				scheme = "http"
			}
		}
	}
	return client.String(), scheme
}

// Middleware resolves the client IP and scheme once per request so that
// ClientIP, Scheme and built-in middleware (Logger, CSRF) use them. Register
// it before middleware that reads them.
func (p *TrustPolicy) Middleware() MiddlewareFunc {
	return func(c Context) error {
		if cc, ok := c.(*context); ok {
			cc.clientIP, cc.scheme = p.resolve(cc)
		}
		return c.Next()
	}
}

// ClientIP returns the request's client IP: the address resolved by a
// TrustPolicy middleware, or otherwise the socket peer. Forwarding headers are
// never used without a TrustPolicy.
func ClientIP(c Context) string {
	if cc, ok := c.(*context); ok {
		if cc.clientIP != "" {
			return cc.clientIP
		}
		return cc.fctx.RemoteIP().String()
	}
	return c.Request().RemoteAddr
}

// Scheme returns "https" or "http": the scheme resolved by a TrustPolicy
// middleware, or otherwise whether the connection itself uses TLS.
func Scheme(c Context) string {
	if cc, ok := c.(*context); ok {
		if cc.scheme != "" {
			return cc.scheme
		}
		if cc.fctx.IsTLS() {
			return "https"
		}
	}
	return "http"
}
