package gofi

import (
	"math/rand"
	"strings"
	"testing"
)

// Differential tests: the radix tree must agree with refRoute, a deliberately
// naive matcher that compares routes and requests segment by segment. Route sets
// are generated, registered in the generated order and in reverse, and probed
// with instances of every route plus near misses around each segment boundary.
//
// Run the fuzzer with:
//
//	go test -run '^$' -fuzz FuzzTree_MatchesReference .

type refSegKind uint8

// Ordered by match precedence: at the first position where two matching routes
// differ, the lower kind wins.
const (
	refStatic refSegKind = iota
	refParam
	refCatchAll
)

type refSeg struct {
	kind refSegKind
	text string // static text, or the wildcard name without ':' or '*'
}

type refRoute struct {
	pattern string
	segs    []refSeg
}

func parseRefRoute(pattern string) refRoute {
	parts := strings.Split(pattern[1:], "/")
	segs := make([]refSeg, len(parts))
	for i, p := range parts {
		switch {
		case strings.HasPrefix(p, ":"):
			segs[i] = refSeg{kind: refParam, text: p[1:]}
		case strings.HasPrefix(p, "*"):
			segs[i] = refSeg{kind: refCatchAll, text: p[1:]}
		default:
			segs[i] = refSeg{kind: refStatic, text: p}
		}
	}
	return refRoute{pattern: pattern, segs: segs}
}

// refConflict reports whether a and b cannot be registered together: they are
// the same route, they name a parameter differently at the same position, or one
// has a catch-all where the other has any segment at all.
func refConflict(a, b refRoute) bool {
	for i := 0; ; i++ {
		if i == len(a.segs) || i == len(b.segs) {
			return len(a.segs) == len(b.segs)
		}
		x, y := a.segs[i], b.segs[i]
		switch {
		case x.kind == refCatchAll || y.kind == refCatchAll:
			return true
		case x.kind != y.kind:
			return false
		case x.text != y.text:
			return x.kind == refParam
		}
	}
}

// match reports whether the route matches the request segments. A param needs a
// non-empty segment; a catch-all needs the '/' before it and takes the rest.
func (r refRoute) match(segs []string) (Params, bool) {
	var ps Params
	for i, s := range r.segs {
		if i >= len(segs) {
			return nil, false
		}
		switch s.kind {
		case refCatchAll:
			return append(ps, Param{Key: s.text, Value: strings.Join(segs[i:], "/")}), true
		case refParam:
			if segs[i] == "" {
				return nil, false
			}
			ps = append(ps, Param{Key: s.text, Value: segs[i]})
		default:
			if segs[i] != s.text {
				return nil, false
			}
		}
	}
	return ps, len(segs) == len(r.segs)
}

func (r refRoute) beats(other refRoute) bool {
	for i := 0; i < len(r.segs) && i < len(other.segs); i++ {
		if r.segs[i].kind != other.segs[i].kind {
			return r.segs[i].kind < other.segs[i].kind
		}
	}
	return false
}

func refLookup(routes []refRoute, path string) (string, Params) {
	segs := strings.Split(path[1:], "/")
	var best *refRoute
	var bestPs Params
	for i := range routes {
		if ps, ok := routes[i].match(segs); ok && (best == nil || routes[i].beats(*best)) {
			best, bestPs = &routes[i], ps
		}
	}
	if best == nil {
		return "", nil
	}
	return best.pattern, bestPs
}

// refChooser turns fuzzer bytes into choices, so the fuzzer mutates route sets
// and requests rather than an opaque seed. Exhausted input always chooses 0.
type refChooser []byte

func (c *refChooser) intn(n int) int {
	if len(*c) == 0 {
		return 0
	}
	v := int((*c)[0]) % n
	*c = (*c)[1:]
	return v
}

func (c *refChooser) pick(options []string) string {
	return options[c.intn(len(options))]
}

var (
	refStatics      = []string{"a", "ab", "abc", "app", "apple", "b", "me"}
	refParamNames   = []string{":id", ":name"}
	refCatchAllName = []string{"*rest", "*filepath"}
	refSegValues    = []string{"a", "ab", "app", "apple", "me", "x"}
	refRestValues   = []string{"", "x", "x/y", "a/"}
)

// genRefPattern generates a route of one to three segments, optionally ending in
// a trailing slash or a catch-all. It never generates a root catch-all, which
// the tree does not support beside other routes.
func genRefPattern(c *refChooser) string {
	if c.intn(16) == 0 {
		return "/"
	}
	var b strings.Builder
	for n := 1 + c.intn(3); n > 0; n-- {
		b.WriteByte('/')
		if c.intn(5) == 0 {
			b.WriteString(c.pick(refParamNames))
		} else {
			b.WriteString(c.pick(refStatics))
		}
	}
	switch c.intn(8) {
	case 0:
		b.WriteByte('/')
	case 1, 2:
		b.WriteString("/" + c.pick(refCatchAllName))
	}
	return b.String()
}

// genRefRequests instantiates every route, then adds near misses at each '/'
// boundary of the instance: the prefix alone, the prefix plus a character, and
// the prefix plus '/'. Those are the requests a boundary bug gets wrong.
func genRefRequests(c *refChooser, routes []refRoute) []string {
	var paths []string
	add := func(p string) {
		if !strings.Contains(p, "//") {
			paths = append(paths, p)
		}
	}

	for _, r := range routes {
		var b strings.Builder
		for _, s := range r.segs {
			b.WriteByte('/')
			switch s.kind {
			case refCatchAll:
				b.WriteString(c.pick(refRestValues))
			case refParam:
				b.WriteString(c.pick(refSegValues))
			default:
				b.WriteString(s.text)
			}
		}
		inst := b.String()

		add(inst)
		add(inst + "x")
		add(inst + "/")
		add(inst + "/x")
		for i := 1; i < len(inst); i++ {
			if inst[i] == '/' {
				add(inst[:i])
				add(inst[:i] + "x")
				add(inst[:i+1])
			}
		}
	}

	for n := c.intn(4); n > 0; n-- {
		var b strings.Builder
		for segs := 1 + c.intn(3); segs > 0; segs-- {
			b.WriteString("/" + c.pick(refSegValues))
		}
		add(b.String())
	}
	return paths
}

func addRouteRecovered(tree *node, pattern string) (panicked interface{}) {
	defer func() { panicked = recover() }()
	tree.addRoute(pattern, &routeData{pattern: pattern})
	return nil
}

func checkTreeAgainstReference(t *testing.T, input []byte) {
	t.Helper()
	c := refChooser(input)

	patterns := make([]string, 1+c.intn(6))
	for i := range patterns {
		patterns[i] = genRefPattern(&c)
	}

	tree := &node{}
	var accepted []refRoute
	for i, p := range patterns {
		route := parseRefRoute(p)
		wantConflict := false
		for _, a := range accepted {
			if refConflict(a, route) {
				wantConflict = true
				break
			}
		}

		panicked := addRouteRecovered(tree, p)
		if (panicked != nil) != wantConflict {
			t.Fatalf("registering %q after %q: panic = %v, reference conflict = %v",
				p, patterns[:i], panicked, wantConflict)
		}
		if panicked == nil {
			accepted = append(accepted, route)
			continue
		}
		// A panic can leave the tree half-modified; rebuild it without the route.
		tree = &node{}
		for _, a := range accepted {
			tree.addRoute(a.pattern, &routeData{pattern: a.pattern})
		}
	}

	// Accepted routes never conflict, so they must register in any order.
	reversed := &node{}
	for i := len(accepted) - 1; i >= 0; i-- {
		if panicked := addRouteRecovered(reversed, accepted[i].pattern); panicked != nil {
			t.Fatalf("routes %q registered in reverse: %q panicked: %v",
				refPatterns(accepted), accepted[i].pattern, panicked)
		}
	}

	for _, path := range genRefRequests(&c, accepted) {
		want, wantPs := refLookup(accepted, path)
		for _, order := range []struct {
			name string
			tree *node
		}{{"in order", tree}, {"reversed", reversed}} {
			var ps Params
			data, _ := order.tree.getValue(path, func() *Params { return &ps })
			got := ""
			if data != nil {
				got = data.pattern
			}
			if got != want || !refParamsEqual(ps, wantPs) {
				t.Fatalf("routes %q (%s): %q -> %q %v, want %q %v",
					refPatterns(accepted), order.name, path, got, ps, want, wantPs)
			}

			data, _ = order.tree.getValue(path, nil)
			got = ""
			if data != nil {
				got = data.pattern
			}
			if got != want {
				t.Fatalf("routes %q (%s, nil params): %q -> %q, want %q",
					refPatterns(accepted), order.name, path, got, want)
			}
		}
	}
}

func refPatterns(routes []refRoute) []string {
	out := make([]string, len(routes))
	for i, r := range routes {
		out[i] = r.pattern
	}
	return out
}

func refParamsEqual(a, b Params) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTree_MatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	input := make([]byte, 96)
	for i := 0; i < 20000; i++ {
		rng.Read(input)
		checkTreeAgainstReference(t, input)
	}
}

func FuzzTree_MatchesReference(f *testing.F) {
	// "/app/*rest", "/app", "/apple": a catch-all beside its bare prefix and a
	// static route that shares the prefix text.
	f.Add([]byte{2, 1, 0, 1, 3, 1, 0, 1, 0, 1, 3, 3, 1, 0, 1, 4, 3})
	// "/a/me", "/a/:id": static and param siblings that need backtracking.
	f.Add([]byte{1, 1, 1, 1, 0, 1, 6, 3, 1, 1, 1, 0, 0, 0, 3})
	// "/me/a", "/:id/*rest": backtracking into a catch-all.
	f.Add([]byte{1, 1, 1, 1, 6, 1, 0, 3, 1, 0, 0, 0, 1, 0})
	f.Fuzz(func(t *testing.T, input []byte) {
		checkTreeAgainstReference(t, input)
	})
}
