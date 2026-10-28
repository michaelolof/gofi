package gofi

import (
	"errors"
	"fmt"
	"net/http"
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
	// UseForwarded reads the RFC 7239 Forwarded header (for=, proto=, host=)
	// instead of the X-Forwarded-* headers. A policy uses one source only.
	UseForwarded bool
	// ForwardedHostHeader carries the original host from a trusted proxy when
	// UseForwarded is false. Default: X-Forwarded-Host.
	ForwardedHostHeader string
	// AllowedHosts restricts the effective host (case-insensitive, port and
	// trailing dot ignored). The middleware answers 421 for other hosts.
	// Empty allows any syntactically valid host.
	AllowedHosts []string
}

// trustPolicy resolves the client IP, scheme and host of a request, starting
// from the socket peer.
type trustPolicy struct {
	prefixes      []netip.Prefix
	forwardedFor  string
	forwardedPro  string
	forwardedHost string
	useForwarded  bool
	allowedHosts  map[string]bool
	maxEntries    int
}

// forwardedHop is one proxy hop reported by a forwarding header.
type forwardedHop struct {
	addr  string
	proto string
	host  string
}

// TrustPolicy validates opts and returns middleware that resolves the client
// IP, scheme and host once per request, so that ClientIP, Scheme, Host and
// built-in middleware (Logger, CSRF) use them. It answers 421 when the host is
// malformed or not in AllowedHosts. Register it before middleware that reads
// them.
func TrustPolicy(opts TrustOptions) (MiddlewareFunc, error) {
	p, err := newTrustPolicy(opts)
	if err != nil {
		return nil, err
	}
	return p.middleware, nil
}

func newTrustPolicy(opts TrustOptions) (*trustPolicy, error) {
	p := &trustPolicy{
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
	p.useForwarded = opts.UseForwarded
	p.forwardedHost = opts.ForwardedHostHeader
	if p.forwardedHost == "" {
		p.forwardedHost = "X-Forwarded-Host"
	}
	if len(opts.AllowedHosts) > 0 {
		p.allowedHosts = map[string]bool{}
		for _, h := range opts.AllowedHosts {
			name, ok := normalizeHost(h)
			if !ok {
				return nil, fmt.Errorf("gofi: invalid allowed host %q", h)
			}
			p.allowedHosts[name] = true
		}
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

func (p *trustPolicy) trusted(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range p.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// hops returns the forwarding chain, nearest hop last.
func (p *trustPolicy) hops(c *context) []forwardedHop {
	var hops []forwardedHop
	if p.useForwarded {
		for _, value := range c.fctx.Request.Header.PeekAll("Forwarded") {
			for _, element := range strings.Split(string(value), ",") {
				var hop forwardedHop
				for _, pair := range strings.Split(element, ";") {
					k, v, _ := strings.Cut(strings.TrimSpace(pair), "=")
					v = strings.Trim(v, `"`)
					switch strings.ToLower(k) {
					case "for":
						hop.addr = v
					case "proto":
						hop.proto = v
					case "host":
						hop.host = v
					}
				}
				hops = append(hops, hop)
			}
		}
		return hops
	}
	for _, value := range c.fctx.Request.Header.PeekAll(p.forwardedFor) {
		for _, entry := range strings.Split(string(value), ",") {
			hops = append(hops, forwardedHop{addr: entry})
		}
	}
	if len(hops) > 0 {
		last := &hops[len(hops)-1]
		if v := c.fctx.Request.Header.PeekAll(p.forwardedPro); len(v) > 0 {
			last.proto = lastListValue(string(v[len(v)-1]))
		}
		if v := c.fctx.Request.Header.PeekAll(p.forwardedHost); len(v) > 0 {
			last.host = lastListValue(string(v[len(v)-1]))
		}
	} else if !p.useForwarded {
		// A proxy may send the scheme or host without a client chain.
		hop := forwardedHop{}
		if v := c.fctx.Request.Header.PeekAll(p.forwardedPro); len(v) > 0 {
			hop.proto = lastListValue(string(v[len(v)-1]))
		}
		if v := c.fctx.Request.Header.PeekAll(p.forwardedHost); len(v) > 0 {
			hop.host = lastListValue(string(v[len(v)-1]))
		}
		if hop.proto != "" || hop.host != "" {
			hops = append(hops, hop)
		}
	}
	return hops
}

func lastListValue(v string) string {
	if i := strings.LastIndexByte(v, ','); i >= 0 {
		v = v[i+1:]
	}
	return strings.TrimSpace(v)
}

// parseHopAddr parses an X-Forwarded-For entry or a Forwarded for= value,
// which may carry a port or brackets.
func parseHopAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if addr, err := netip.ParseAddr(s); err == nil {
		return addr.Unmap(), true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), true
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		if addr, err := netip.ParseAddr(s[1 : len(s)-1]); err == nil {
			return addr.Unmap(), true
		}
	}
	return netip.Addr{}, false
}

// resolve returns the client IP, scheme and host for the request in c.
func (p *trustPolicy) resolve(c *context) (string, string, string) {
	scheme := "http"
	if c.fctx.IsTLS() {
		scheme = "https"
	}
	host := string(c.fctx.Host())
	peer, ok := netip.AddrFromSlice(c.fctx.RemoteIP())
	if !ok {
		return c.fctx.RemoteIP().String(), scheme, host
	}
	peer = peer.Unmap()
	if !p.trusted(peer) {
		return peer.String(), scheme, host
	}

	// The connection comes from a trusted proxy: the nearest hop's scheme and
	// host describe the original request. Walk the client chain from the
	// right and stop at the first address not trusted.
	hops := p.hops(c)
	if len(hops) > 0 {
		nearest := hops[len(hops)-1]
		if !c.fctx.IsTLS() {
			switch strings.ToLower(nearest.proto) {
			case "https":
				scheme = "https"
			case "http":
				scheme = "http"
			}
		}
		if nearest.host != "" {
			host = nearest.host
		}
	}
	client := peer
	for i, examined := len(hops)-1, 0; i >= 0 && examined < p.maxEntries; i, examined = i-1, examined+1 {
		if hops[i].addr == "" {
			break
		}
		hop, ok := parseHopAddr(hops[i].addr)
		if !ok {
			break // malformed or obfuscated entry: keep the nearest address we can vouch for
		}
		client = hop
		if !p.trusted(client) {
			break
		}
	}
	return client.String(), scheme, host
}

// normalizeHost validates a host[:port] value and returns its lower-case
// name without port or trailing dot. Only ASCII names and IP literals are
// accepted; internationalized names must arrive in their A-label form.
func normalizeHost(h string) (string, bool) {
	h = strings.TrimSpace(h)
	if h == "" || len(h) > 255 {
		return "", false
	}
	name := h
	if strings.HasPrefix(h, "[") {
		end := strings.IndexByte(h, ']')
		if end < 0 {
			return "", false
		}
		if _, err := netip.ParseAddr(h[1:end]); err != nil {
			return "", false
		}
		name = h[:end+1]
		if rest := h[end+1:]; rest != "" && !validPort(strings.TrimPrefix(rest, ":")) {
			return "", false
		}
		return strings.ToLower(name), true
	}
	if i := strings.LastIndexByte(h, ':'); i >= 0 {
		if !validPort(h[i+1:]) {
			return "", false
		}
		name = h[:i]
	}
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	if name == "" {
		return "", false
	}
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '.') {
			return "", false
		}
	}
	return name, true
}

func validPort(p string) bool {
	if p == "" || len(p) > 5 {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] < '0' || p[i] > '9' {
			return false
		}
	}
	return true
}

func (p *trustPolicy) middleware(c Context) error {
	if cc, ok := c.(*context); ok {
		var host string
		cc.clientIP, cc.scheme, host = p.resolve(cc)
		name, valid := normalizeHost(host)
		if !valid || (p.allowedHosts != nil && !p.allowedHosts[name]) {
			return NewHTTPError(http.StatusMisdirectedRequest, http.StatusText(http.StatusMisdirectedRequest))
		}
		cc.host = host
	}
	return c.Next()
}

// ClientIP returns the request's client IP: the address resolved by a
// TrustPolicy middleware, or otherwise the socket peer. Forwarding headers are
// never used without TrustPolicy.
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

// Host returns the request's effective host: the host validated by a
// TrustPolicy middleware (from a trusted proxy's forwarding header, or the
// Host header), or otherwise the Host header as received.
func Host(c Context) string {
	if cc, ok := c.(*context); ok {
		if cc.host != "" {
			return cc.host
		}
		return string(cc.fctx.Host())
	}
	return c.Request().Host
}
