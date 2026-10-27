package gofi

import (
	"strings"
	"testing"
)

func TestTreeAddAndGet(t *testing.T) {
	tree := &node{}

	routes := []string{
		"/",
		"/cmd/:tool/:sub",
		"/cmd/:tool/",
		"/src/*filepath",
		"/search/",
		"/search/:query",
		"/user_:name",
		"/user_:name/about",
		"/files/:dir/*filepath",
		"/doc/",
		"/doc/go_faq.html",
		"/doc/go1.html",
		"/info/:user/public",
		"/info/:user/project/:project",
	}

	for _, route := range routes {
		data := &routeData{meta: route} // Use the route string as meta to verify match
		tree.addRoute(route, data)
	}

	tests := []struct {
		path   string
		match  bool
		route  string
		params map[string]string
		tsr    bool
	}{
		{"/", true, "/", nil, false},
		{"/cmd/test/", true, "/cmd/:tool/", map[string]string{"tool": "test"}, false},
		{"/cmd/test", false, "", nil, true},
		{"/cmd/test/run", true, "/cmd/:tool/:sub", map[string]string{"tool": "test", "sub": "run"}, false},
		{"/src/some/file.png", true, "/src/*filepath", map[string]string{"filepath": "some/file.png"}, false},
		{"/search/", true, "/search/", nil, false},
		{"/search", false, "", nil, true}, // TSR should be true since /search/ exists
		{"/search/somethings", true, "/search/:query", map[string]string{"query": "somethings"}, false},
		{"/user_michael", true, "/user_:name", map[string]string{"name": "michael"}, false},
		{"/user_michael/about", true, "/user_:name/about", map[string]string{"name": "michael"}, false},
		{"/files/js/inc/framework.js", true, "/files/:dir/*filepath", map[string]string{"dir": "js", "filepath": "inc/framework.js"}, false},
		{"/doc/", true, "/doc/", nil, false},
		{"/doc/go_faq.html", true, "/doc/go_faq.html", nil, false},
		{"/doc/go1.html", true, "/doc/go1.html", nil, false},
		{"/info/gordon/public", true, "/info/:user/public", map[string]string{"user": "gordon"}, false},
		{"/info/gordon/project/go", true, "/info/:user/project/:project", map[string]string{"user": "gordon", "project": "go"}, false},
	}

	for _, test := range tests {
		var ps Params
		paramsFunc := func() *Params { return &ps }

		data, tsr := tree.getValue(test.path, paramsFunc)

		if data != nil != test.match {
			t.Errorf("Path '%s' match mismatch. Expected match: %v, got %v", test.path, test.match, data != nil)
			continue
		}

		if data != nil && data.meta.(string) != test.route {
			t.Errorf("Path '%s' routed to wrong handler. Expected '%s', got '%s'", test.path, test.route, data.meta)
		}

		if test.match && test.params != nil {
			if len(ps) != len(test.params) {
				t.Errorf("Path '%s' params mismatch. Expected %d params, got %d", test.path, len(test.params), len(ps))
				continue
			}
			for k, v := range test.params {
				if ps.Get(k) != v {
					t.Errorf("Path '%s' param %s mismatch. Expected '%s', got '%s'", test.path, k, v, ps.Get(k))
				}
			}
		}

		if data == nil && tsr != test.tsr {
			t.Errorf("Path '%s' TSR mismatch. Expected %v, got %v", test.path, test.tsr, tsr)
		}
	}
}

func TestTreeDuplicateParam(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Adding duplicate param route should panic")
		}
	}()

	tree := &node{}
	tree.addRoute("/user/:id", &routeData{})
	tree.addRoute("/user/:name", &routeData{})
}

func TestTreeDuplicateWildcard(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Adding duplicate wildcard route should panic")
		}
	}()

	tree := &node{}
	tree.addRoute("/src/*filepath", &routeData{})
	tree.addRoute("/src/*path", &routeData{})
}

// treeMatchCases is a test helper that asserts every path in `cases` resolves
// to the expected meta string.
func treeMatchCases(t *testing.T, tree *node, cases map[string]string) {
	t.Helper()
	for path, wantMeta := range cases {
		var ps Params
		d, _ := tree.getValue(path, func() *Params { return &ps })
		if d == nil {
			t.Errorf("path %q: expected match %q, got nil", path, wantMeta)
			continue
		}
		if got := d.meta.(string); got != wantMeta {
			t.Errorf("path %q: got %q, want %q", path, got, wantMeta)
		}
	}
}

// --- Static-vs-param coexistence regression tests ---

// TestTree_StaticAndParamSiblings_StaticThenParam registers a static route
// before a param route at the same level and asserts correct dispatch for both.
func TestTree_StaticAndParamSiblings_StaticThenParam(t *testing.T) {
	tree := &node{}
	tree.addRoute("/matches/live", &routeData{meta: "live"})
	tree.addRoute("/matches/:id", &routeData{meta: "detail"})

	cases := map[string]string{
		"/matches/live":         "live",
		"/matches/f3413-d04591": "detail",
		"/matches/abc":          "detail",
	}
	treeMatchCases(t, tree, cases)

	// Confirm the param value is captured correctly for a non-static path.
	var ps Params
	tree.getValue("/matches/f3413-d04591", func() *Params { return &ps })
	if ps.Get("id") != "f3413-d04591" {
		t.Errorf("expected param id=f3413-d04591, got %q", ps.Get("id"))
	}
}

// TestTree_StaticAndParamSiblings_ParamThenStatic registers a param route before
// a static route at the same level and asserts both routes work without panicking.
func TestTree_StaticAndParamSiblings_ParamThenStatic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("registering /matches/:id then /matches/live should not panic; got: %v", r)
		}
	}()

	tree := &node{}
	tree.addRoute("/matches/:id", &routeData{meta: "detail"})
	tree.addRoute("/matches/live", &routeData{meta: "live"})

	cases := map[string]string{
		"/matches/live":         "live",
		"/matches/f3413-d04591": "detail",
	}
	treeMatchCases(t, tree, cases)
}

// TestTree_MultipleStaticVsParam verifies that multiple static siblings alongside
// a param route all dispatch correctly regardless of registration order.
func TestTree_MultipleStaticVsParam(t *testing.T) {
	t.Run("StaticFirst", func(t *testing.T) {
		tree := &node{}
		tree.addRoute("/users/me", &routeData{meta: "me"})
		tree.addRoute("/users/admin", &routeData{meta: "admin"})
		tree.addRoute("/users/:id", &routeData{meta: "user"})

		treeMatchCases(t, tree, map[string]string{
			"/users/me":    "me",
			"/users/admin": "admin",
			"/users/123":   "user",
			"/users/other": "user",
		})
	})

	t.Run("ParamFirst", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("should not panic registering statics after param; got: %v", r)
			}
		}()

		tree := &node{}
		tree.addRoute("/users/:id", &routeData{meta: "user"})
		tree.addRoute("/users/me", &routeData{meta: "me"})
		tree.addRoute("/users/admin", &routeData{meta: "admin"})

		treeMatchCases(t, tree, map[string]string{
			"/users/me":    "me",
			"/users/admin": "admin",
			"/users/123":   "user",
		})
	})
}

// TestTree_StaticAndParam_TSR verifies that trailing-slash redirect recommendations
// (TSR) work correctly when static and param siblings coexist.
func TestTree_StaticAndParam_TSR(t *testing.T) {
	// :id/ (param with trailing slash) alongside a static /live (no trailing slash).
	tree := &node{}
	tree.addRoute("/matches/live", &routeData{meta: "live"})
	tree.addRoute("/matches/:id/", &routeData{meta: "detail-slash"})

	// Static route resolves directly.
	var ps Params
	d, _ := tree.getValue("/matches/live", func() *Params { return &ps })
	if d == nil || d.meta.(string) != "live" {
		got := "<nil>"
		if d != nil {
			got = d.meta.(string)
		}
		t.Errorf("/matches/live: want match 'live', got %q", got)
	}

	// A non-static segment without trailing slash should get TSR=true
	// (pointing toward /matches/:id/).
	ps = ps[:0]
	d2, tsr := tree.getValue("/matches/abc", func() *Params { return &ps })
	if d2 != nil {
		t.Errorf("/matches/abc: expected no direct match, got %q", d2.meta)
	}
	if !tsr {
		t.Errorf("/matches/abc: expected TSR=true for param+trailing-slash route")
	}
}

// TestTree_NestedStaticAndParam verifies that static and param siblings coexist
// correctly at non-root nesting levels.
func TestTree_NestedStaticAndParam(t *testing.T) {
	tree := &node{}
	tree.addRoute("/api/v1/users/me/posts", &routeData{meta: "my-posts"})
	tree.addRoute("/api/v1/users/:id/posts", &routeData{meta: "user-posts"})

	treeMatchCases(t, tree, map[string]string{
		"/api/v1/users/me/posts":     "my-posts",
		"/api/v1/users/gordon/posts": "user-posts",
		"/api/v1/users/123/posts":    "user-posts",
	})
}

func assertTreeLookup(t *testing.T, tree *node, path, wantMeta string, wantTSR bool, wantParams Params) {
	t.Helper()
	ps := make(Params, 0, tree.maxParams)
	data, tsr := tree.getValue(path, func() *Params { return &ps })

	if wantMeta == "" {
		if data != nil {
			t.Fatalf("path %q: expected no route, got %q", path, data.meta)
		}
	} else {
		if data == nil {
			t.Fatalf("path %q: expected route %q, got nil", path, wantMeta)
		}
		if got := data.meta.(string); got != wantMeta {
			t.Fatalf("path %q: got route %q, want %q", path, got, wantMeta)
		}
	}
	if tsr != wantTSR {
		t.Fatalf("path %q: got TSR %v, want %v", path, tsr, wantTSR)
	}
	if len(ps) != len(wantParams) {
		t.Fatalf("path %q: got params %#v, want %#v", path, ps, wantParams)
	}
	for i := range wantParams {
		if ps[i] != wantParams[i] {
			t.Fatalf("path %q: got params %#v, want %#v", path, ps, wantParams)
		}
	}
}

func TestTree_StaticDeadEndFallsBackToParam(t *testing.T) {
	orders := []struct {
		name   string
		routes []struct {
			path string
			meta string
		}
	}{
		{
			name: "static-first",
			routes: []struct {
				path string
				meta string
			}{
				{path: "/users/me", meta: "static"},
				{path: "/users/:id", meta: "param"},
			},
		},
		{
			name: "param-first",
			routes: []struct {
				path string
				meta string
			}{
				{path: "/users/:id", meta: "param"},
				{path: "/users/me", meta: "static"},
			},
		},
	}

	for _, order := range orders {
		t.Run(order.name, func(t *testing.T) {
			tree := &node{}
			for _, route := range order.routes {
				tree.addRoute(route.path, &routeData{meta: route.meta})
			}

			assertTreeLookup(t, tree, "/users/me", "static", false, nil)
			assertTreeLookup(t, tree, "/users/mike", "param", false, Params{{Key: "id", Value: "mike"}})
			assertTreeLookup(t, tree, "/users/m", "param", false, Params{{Key: "id", Value: "m"}})
			assertTreeLookup(t, tree, "/users/123", "param", false, Params{{Key: "id", Value: "123"}})
		})
	}
}

func TestTree_StaticDeadEndAfterLongerCommonPrefixFallsBack(t *testing.T) {
	tree := &node{}
	tree.addRoute("/users/metrics", &routeData{meta: "metrics"})
	tree.addRoute("/users/:id", &routeData{meta: "param"})

	assertTreeLookup(t, tree, "/users/metric-x", "param", false, Params{{Key: "id", Value: "metric-x"}})
}

func TestTree_NestedStaticDeadEndBacktracks(t *testing.T) {
	tree := &node{}
	tree.addRoute("/users/:id/posts/latest", &routeData{meta: "latest"})
	tree.addRoute("/users/:id/posts/:post", &routeData{meta: "post"})

	assertTreeLookup(t, tree, "/users/mike/posts/latest", "latest", false, Params{{Key: "id", Value: "mike"}})
	assertTreeLookup(t, tree, "/users/mike/posts/lunch", "post", false, Params{
		{Key: "id", Value: "mike"},
		{Key: "post", Value: "lunch"},
	})
}

func TestTree_BacktracksAcrossMultipleAmbiguousNodes(t *testing.T) {
	tree := &node{}
	tree.addRoute("/a/foo/bar/static", &routeData{meta: "deep-static"})
	tree.addRoute("/a/foo/:second/end", &routeData{meta: "inner-param"})
	tree.addRoute("/a/:first/bar/else", &routeData{meta: "outer-param"})

	assertTreeLookup(t, tree, "/a/foo/bar/else", "outer-param", false, Params{
		{Key: "first", Value: "foo"},
	})
}

func TestTree_BacktrackingDiscardsAbandonedParams(t *testing.T) {
	tree := &node{}
	tree.addRoute("/a/foo/:second/static", &routeData{meta: "inner-static"})
	tree.addRoute("/a/foo/:second/:third/end", &routeData{meta: "inner-param"})
	tree.addRoute("/a/:first/bar/baz", &routeData{meta: "outer-param"})

	assertTreeLookup(t, tree, "/a/foo/bar/baz", "outer-param", false, Params{
		{Key: "first", Value: "foo"},
	})
}

func TestTree_BacktrackingWithNilParams(t *testing.T) {
	tree := &node{}
	tree.addRoute("/users/me", &routeData{meta: "static"})
	tree.addRoute("/users/:id", &routeData{meta: "param"})

	data, tsr := tree.getValue("/users/mike", nil)
	if data == nil || data.meta.(string) != "param" {
		t.Fatalf("expected param route with nil params callback, got %#v", data)
	}
	if tsr {
		t.Fatal("real match unexpectedly returned a TSR recommendation")
	}
}

func TestTree_BacktrackingTSRPrecedence(t *testing.T) {
	tree := &node{}
	tree.addRoute("/x/static/", &routeData{meta: "static-slash"})
	tree.addRoute("/x/:id", &routeData{meta: "param"})

	assertTreeLookup(t, tree, "/x/static", "param", false, Params{{Key: "id", Value: "static"}})
}

func TestTree_BacktrackingPreservesTSRWhenNoMatch(t *testing.T) {
	tree := &node{}
	tree.addRoute("/events/exports/", &routeData{meta: "exports-slash"})
	tree.addRoute("/events/:event/records", &routeData{meta: "records"})

	assertTreeLookup(t, tree, "/events/exports", "", true, nil)
}

func TestTree_BacktrackingUnicodeValue(t *testing.T) {
	tree := &node{}
	tree.addRoute("/users/memo", &routeData{meta: "static"})
	tree.addRoute("/users/:id", &routeData{meta: "param"})

	assertTreeLookup(t, tree, "/users/mé", "param", false, Params{{Key: "id", Value: "mé"}})
}

func buildAmbiguousDepthTree(depth int, matchingDeepestFallback bool) (*node, string) {
	tree := &node{}
	staticParts := make([]string, 0, depth)

	for i := 0; i < depth; i++ {
		prefix := ""
		if len(staticParts) > 0 {
			prefix = "/" + strings.Join(staticParts, "/")
		}
		tail := "/never"
		if matchingDeepestFallback && i == depth-1 {
			tail = "/stale"
		}
		tree.addRoute(prefix+"/:p"+string(rune('a'+i))+tail, &routeData{meta: "fallback"})
		staticParts = append(staticParts, "s"+string(rune('a'+i)))
	}

	staticPrefix := "/" + strings.Join(staticParts, "/")
	tree.addRoute(staticPrefix+"/static", &routeData{meta: "static"})
	return tree, staticPrefix + "/stale"
}

func TestTree_BacktrackingEightCheckpoints(t *testing.T) {
	tree, path := buildAmbiguousDepthTree(maxSkippedNodes, true)
	assertTreeLookup(t, tree, path, "fallback", false, Params{
		{Key: "ph", Value: "sh"},
	})
}

func TestTree_BacktrackingCheckpointOverflowIsSafe(t *testing.T) {
	tree, path := buildAmbiguousDepthTree(maxSkippedNodes+1, false)
	ps := make(Params, 0, tree.maxParams)
	data, _ := tree.getValue(path, func() *Params { return &ps })
	if data != nil && data.meta == nil {
		t.Fatal("overflow-shaped lookup returned corrupt route data")
	}
	for _, p := range ps {
		if p.Key == "" {
			t.Fatalf("overflow-shaped lookup left corrupt params: %#v", ps)
		}
	}
}

func TestRouter_WildcardBacktrackingStatusAndTSR(t *testing.T) {
	r := newRouter()
	r.Get("/users/me", RouteOptions{Handler: func(c Context) error {
		return c.SendString(200, "static")
	}})
	r.Get("/users/:id", RouteOptions{Handler: func(c Context) error {
		return c.SendString(200, "param:"+c.Param("id"))
	}})
	r.Get("/x/static/", RouteOptions{Handler: func(c Context) error {
		return c.SendString(200, "slash")
	}})
	r.Get("/x/:id", RouteOptions{Handler: func(c Context) error {
		return c.SendString(200, "dynamic:"+c.Param("id"))
	}})

	tests := []struct {
		path       string
		statusCode int
		body       string
	}{
		{path: "/users/me", statusCode: 200, body: "static"},
		{path: "/users/mike", statusCode: 200, body: "param:mike"},
		{path: "/x/static", statusCode: 200, body: "dynamic:static"},
		{path: "/missing", statusCode: 404},
	}

	for _, test := range tests {
		resp, err := r.Test(TestOptions{Method: "GET", Path: test.path})
		if err != nil {
			t.Fatalf("%s: unexpected request error: %v", test.path, err)
		}
		if resp.StatusCode != test.statusCode {
			t.Fatalf("%s: got status %d, want %d", test.path, resp.StatusCode, test.statusCode)
		}
		if test.body != "" && string(resp.Body) != test.body {
			t.Fatalf("%s: got body %q, want %q", test.path, resp.Body, test.body)
		}
	}
}

func TestRouter_WildcardBacktrackingPrecedesMethodNotAllowed(t *testing.T) {
	r := newRouter()
	r.Get("/events/exports/:export", RouteOptions{Handler: func(c Context) error {
		return c.SendString(200, "export:"+c.Param("export"))
	}})
	r.Get("/events/:event", RouteOptions{Handler: func(c Context) error {
		return c.SendString(200, "event:"+c.Param("event"))
	}})
	r.Post("/events/exports", RouteOptions{Handler: func(c Context) error {
		return c.SendString(201, "created")
	}})

	resp, err := r.Test(TestOptions{Method: "GET", Path: "/events/exports"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("got status %d, want 200", resp.StatusCode)
	}
	if got := string(resp.Body); got != "event:exports" {
		t.Fatalf("got body %q, want %q", got, "event:exports")
	}
}

// --- Regression guards: panics that MUST still fire after the fix ---

// TestTree_DuplicateStatic_StillPanics ensures that registering the same
// static path twice still panics after the coexistence fix lands.
func TestTree_DuplicateStatic_StillPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("registering a duplicate static path should panic")
		}
	}()

	tree := &node{}
	tree.addRoute("/users/profile", &routeData{})
	tree.addRoute("/users/profile", &routeData{})
}

// TestTree_DuplicateParam_StillPanics ensures that two distinct param names
// at the same position (e.g. :id vs :slug) still panic after the fix.
func TestTree_DuplicateParam_StillPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("registering two different param names at the same position should panic")
		}
	}()

	tree := &node{}
	tree.addRoute("/items/:id", &routeData{})
	tree.addRoute("/items/:slug", &routeData{})
}

// TestRouter_RoutePrefix_NoDoubleSlash verifies that Route("/api/") + Get("/users")
// registers the pattern as "/api/users" and not "/api//users".
// Currently FAILS — Issue #3.
func TestRouter_RoutePrefix_NoDoubleSlash(t *testing.T) {
	r := newRouter()
	r.Route("/api/", func(r Router) {
		r.Get("/users", RouteOptions{Handler: func(c Context) error { return nil }})
	})

	resp, err := r.Test(TestOptions{Method: "GET", Path: "/api/users"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /api/users: want 200, got %d (likely double-slash prefix bug)", resp.StatusCode)
	}
}

// TestRouter_RoutePrefix_AllSlashCombinations table-tests all four slash
// arrangements of prefix·path and asserts each resolves to "/api/users".
// Currently FAILS for the "both have slash" case — Issue #3.
func TestRouter_RoutePrefix_AllSlashCombinations(t *testing.T) {
	combinations := []struct {
		prefix string
		path   string
	}{
		{"/api", "users"},   // neither side has a joining slash
		{"/api", "/users"},  // only path has a leading slash
		{"/api/", "users"},  // only prefix has a trailing slash
		{"/api/", "/users"}, // both have a slash — the bug case
	}

	for _, c := range combinations {
		c := c
		t.Run(c.prefix+"+"+c.path, func(t *testing.T) {
			r := newRouter()
			r.Route(c.prefix, func(r Router) {
				r.Get(c.path, RouteOptions{Handler: func(c Context) error { return nil }})
			})
			resp, err := r.Test(TestOptions{Method: "GET", Path: "/api/users"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != 200 {
				t.Errorf("prefix=%q path=%q → GET /api/users: want 200, got %d", c.prefix, c.path, resp.StatusCode)
			}
		})
	}
}

// TestRouter_TSR_PreservesQueryString verifies that a trailing-slash redirect
// preserves the full query string in the Location header.
// Currently FAILS — Issue #4.
func TestRouter_TSR_PreservesQueryString(t *testing.T) {
	r := newRouter()
	r.Get("/users/", RouteOptions{Handler: func(c Context) error { return nil }})

	resp, err := r.Test(TestOptions{
		Method: "GET",
		Path:   "/users",
		Query:  map[string]string{"id": "5", "page": "2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 301 {
		t.Fatalf("expected 301 redirect, got %d", resp.StatusCode)
	}

	loc := resp.HeaderMap.Get("Location")
	if loc == "" {
		t.Fatal("Location header is empty")
	}
	if !strings.Contains(loc, "id=5") || !strings.Contains(loc, "page=2") {
		t.Errorf("Location %q is missing query params; want both id=5 and page=2", loc)
	}
}

// TestRouter_TSR_NoQuery_Unchanged verifies that a TSR redirect with no query
// string produces a clean Location header with no trailing '?'.
func TestRouter_TSR_NoQuery_Unchanged(t *testing.T) {
	r := newRouter()
	r.Get("/users/", RouteOptions{Handler: func(c Context) error { return nil }})

	resp, err := r.Test(TestOptions{Method: "GET", Path: "/users"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 301 {
		t.Fatalf("expected 301 redirect, got %d", resp.StatusCode)
	}

	loc := resp.HeaderMap.Get("Location")
	if loc == "" {
		t.Fatal("Location header is empty")
	}
	if strings.Contains(loc, "?") {
		t.Errorf("Location %q should not contain '?' when there is no query string", loc)
	}
}

func TestRepro_StaticBeforeParam(t *testing.T) {
	tree := &node{}
	tree.addRoute("/matches/live", &routeData{meta: "live"})
	tree.addRoute("/matches/:id", &routeData{meta: "detail"})

	cases := map[string]string{
		"/matches/live":         "live",
		"/matches/f3413-d04591": "detail",
	}
	for p, want := range cases {
		var ps Params
		d, _ := tree.getValue(p, func() *Params { return &ps })
		got := "<nil>"
		if d != nil {
			got = d.meta.(string)
		}
		t.Logf("path=%s -> %s (params=%v)", p, got, ps)
		if got != want {
			t.Errorf("path %s: got %q want %q", p, got, want)
		}
	}
}

func TestRepro_ParamBeforeStatic(t *testing.T) {
	tree := &node{}
	tree.addRoute("/matches/:id", &routeData{meta: "detail"})
	tree.addRoute("/matches/live", &routeData{meta: "live"})

	cases := map[string]string{
		"/matches/live":         "live",
		"/matches/f3413-d04591": "detail",
	}
	for p, want := range cases {
		var ps Params
		d, _ := tree.getValue(p, func() *Params { return &ps })
		got := "<nil>"
		if d != nil {
			got = d.meta.(string)
		}
		t.Logf("path=%s -> %s (params=%v)", p, got, ps)
		if got != want {
			t.Errorf("path %s: got %q want %q", p, got, want)
		}
	}
}

func TestRepro_CatchAllVsStatic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("PANIC catchall-vs-static: %v", r)
		}
	}()
	tree := &node{}
	tree.addRoute("/files/static.txt", &routeData{meta: "static"})
	tree.addRoute("/files/*path", &routeData{meta: "catchall"})
	var ps Params
	d, _ := tree.getValue("/files/static.txt", func() *Params { return &ps })
	if d != nil {
		t.Logf("/files/static.txt -> %v", d.meta)
	} else {
		t.Logf("/files/static.txt -> nil")
	}
}

func TestRepro_DoublePrefix(t *testing.T) {
	r := newRouter()
	r.Route("/api/", func(r Router) {
		r.Get("/users", RouteOptions{Handler: func(c Context) error { return nil }})
	})
	for path := range r.paths {
		t.Logf("registered path: %q", path)
	}
}

func TestRepro_TSRDropsQuery(t *testing.T) {
	var r Router = newRouter()
	r.Get("/users/", RouteOptions{Handler: func(c Context) error { return nil }})
	resp, err := r.Test(TestOptions{Method: "GET", Path: "/users", Query: map[string]string{"id": "5"}})
	if err != nil {
		t.Logf("err: %v", err)
	}
	if resp != nil {
		t.Logf("status=%d location=%q", resp.StatusCode, resp.HeaderMap.Get("Location"))
	}
}

func TestRepro_DoublePrefix2(t *testing.T) {
	var r Router = newRouter()
	r.Route("/api/", func(r Router) {
		r.Get("/users", RouteOptions{Handler: func(c Context) error { return nil }})
	})
	mux := r.(*serveMux)
	for method, root := range mux.trees {
		walkPrint(t, method, root, "")
	}
}

// TestTree_CatchAllAndStaticSibling documents that registering a static route
// before a catch-all under the same prefix panics. This is a hard constraint of
// the httprouter radix tree algorithm: when the tree splits to accommodate the
// static child, the '/' separator is consumed into the parent node's path, leaving
// insertChild unable to find it when the catch-all is added later.
func TestTree_CatchAllAndStaticSibling(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("registering a static child then a catch-all under the same prefix should panic")
		}
	}()

	tree := &node{}
	tree.addRoute("/files/static.txt", &routeData{meta: "static"})
	tree.addRoute("/files/*path", &routeData{meta: "catchall"})
}

func walkPrint(t *testing.T, method string, n *node, prefix string) {
	p := prefix + n.path
	if n.data != nil {
		t.Logf("%s %q (pattern=%q)", method, p, n.data.pattern)
	}
	for _, c := range n.children {
		walkPrint(t, method, c, p)
	}
}

// TestRouter_405_MethodNotAllowed verifies that a request matching a registered path
// under a different HTTP method returns 405 with an Allow header.
func TestRouter_405_MethodNotAllowed(t *testing.T) {
	r := newRouter()
	r.Get("/users", RouteOptions{Handler: func(c Context) error { return c.SendString(200, "ok") }})
	r.Post("/items", RouteOptions{Handler: func(c Context) error { return c.SendString(200, "ok") }})

	t.Run("POST to GET-only route → 405", func(t *testing.T) {
		resp, err := r.Test(TestOptions{Method: "POST", Path: "/users"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 405 {
			t.Errorf("expected 405, got %d", resp.StatusCode)
		}
		allow := resp.HeaderMap.Get("Allow")
		if allow == "" {
			t.Error("expected Allow header, got empty string")
		}
		if !strings.Contains(allow, "GET") {
			t.Errorf("Allow header %q does not contain GET", allow)
		}
	})

	t.Run("GET to POST-only route → 405", func(t *testing.T) {
		resp, err := r.Test(TestOptions{Method: "GET", Path: "/items"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 405 {
			t.Errorf("expected 405, got %d", resp.StatusCode)
		}
		allow := resp.HeaderMap.Get("Allow")
		if !strings.Contains(allow, "POST") {
			t.Errorf("Allow header %q does not contain POST", allow)
		}
	})

	t.Run("path not registered at all → 404", func(t *testing.T) {
		resp, err := r.Test(TestOptions{Method: "GET", Path: "/nonexistent"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 404 {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("MethodNotAllowed disabled → 404 instead of 405", func(t *testing.T) {
		r2 := newRouter()
		f := false
		r2.Configure(Config{MethodNotAllowed: &f})
		r2.Get("/ping", RouteOptions{Handler: func(c Context) error { return c.SendString(200, "ok") }})
		resp, err := r2.Test(TestOptions{Method: "POST", Path: "/ping"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 404 {
			t.Errorf("expected 404 when MethodNotAllowed disabled, got %d", resp.StatusCode)
		}
	})
}

// --- Catch-all segment boundary ---
//
// A catch-all "/<prefix>/*name" covers "/<prefix>/" and everything below it,
// never a request that merely starts with the prefix text ("/<prefix>foo").

func TestTree_CatchAllStopsAtSegmentBoundary(t *testing.T) {
	tree := &node{}
	tree.addRoute("/src/*filepath", &routeData{meta: "catchall"})

	assertTreeLookup(t, tree, "/src/a/b", "catchall", false, Params{{Key: "filepath", Value: "a/b"}})
	assertTreeLookup(t, tree, "/src/", "catchall", false, Params{{Key: "filepath", Value: ""}})
	assertTreeLookup(t, tree, "/srcfoo", "", false, nil)
	assertTreeLookup(t, tree, "/srcx/y", "", false, nil)
	assertTreeLookup(t, tree, "/src", "", false, nil)
}

func TestTree_CatchAllBesideBarePrefix(t *testing.T) {
	orders := map[string][]string{
		"bare-first":     {"/app", "/app/*filepath"},
		"catchall-first": {"/app/*filepath", "/app"},
	}

	for name, routes := range orders {
		t.Run(name, func(t *testing.T) {
			tree := &node{}
			for _, route := range routes {
				tree.addRoute(route, &routeData{meta: route})
			}

			assertTreeLookup(t, tree, "/app", "/app", false, nil)
			assertTreeLookup(t, tree, "/app/x", "/app/*filepath", false, Params{{Key: "filepath", Value: "x"}})
			assertTreeLookup(t, tree, "/application", "", false, nil)
		})
	}
}

func TestTree_CatchAllBesideStaticSharingPrefixText(t *testing.T) {
	orders := map[string][]string{
		"catchall-first": {"/app/*filepath", "/apple"},
		"static-first":   {"/apple", "/app/*filepath"},
	}

	for name, routes := range orders {
		t.Run(name, func(t *testing.T) {
			tree := &node{}
			for _, route := range routes {
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Fatalf("registering %q panicked: %v", route, r)
						}
					}()
					tree.addRoute(route, &routeData{meta: route})
				}()
			}

			assertTreeLookup(t, tree, "/apple", "/apple", false, nil)
			assertTreeLookup(t, tree, "/app/x", "/app/*filepath", false, Params{{Key: "filepath", Value: "x"}})
			assertTreeLookup(t, tree, "/appx", "", false, nil)
		})
	}
}

func TestTree_CatchAllAfterParamUnchanged(t *testing.T) {
	tree := &node{}
	tree.addRoute("/files/:dir/*filepath", &routeData{meta: "catchall"})

	assertTreeLookup(t, tree, "/files/js/a.js", "catchall", false, Params{
		{Key: "dir", Value: "js"},
		{Key: "filepath", Value: "a.js"},
	})

	var ps Params
	if d, _ := tree.getValue("/files/js", func() *Params { return &ps }); d != nil {
		t.Errorf("path %q: expected no route, got %q", "/files/js", d.meta)
	}
}

func TestTree_BacktrackingIntoCatchAllUnchanged(t *testing.T) {
	tree := &node{}
	tree.addRoute("/users/me/settings", &routeData{meta: "static"})
	tree.addRoute("/users/:id/*rest", &routeData{meta: "catchall"})

	assertTreeLookup(t, tree, "/users/me/other", "catchall", false, Params{
		{Key: "id", Value: "me"},
		{Key: "rest", Value: "other"},
	})
	assertTreeLookup(t, tree, "/users/me/settings", "static", false, nil)
}

func TestTree_CatchAllConflictsStillPanic(t *testing.T) {
	cases := [][2]string{
		{"/app/*filepath", "/app/foo"},
		{"/app/*filepath", "/app/:id"},
		{"/app/*filepath", "/app/*other"},
		{"/files/static.txt", "/files/*path"},
	}

	for _, routes := range cases {
		t.Run(routes[0]+" then "+routes[1], func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("registering %q after %q should panic", routes[1], routes[0])
				}
			}()

			tree := &node{}
			tree.addRoute(routes[0], &routeData{})
			tree.addRoute(routes[1], &routeData{})
		})
	}
}

func TestRouter_CatchAllNearMissIsNotFound(t *testing.T) {
	t.Run("no 405 from another method's catch-all", func(t *testing.T) {
		r := newRouter()
		ok := func(c Context) error { return c.SendString(200, "ok") }
		r.Get("/app", RouteOptions{Handler: ok})
		r.Get("/app/*filepath", RouteOptions{Handler: ok})
		r.Head("/app/*filepath", RouteOptions{Handler: ok})

		resp, err := r.Test(TestOptions{Method: "GET", Path: "/application"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 404 {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		if allow := resp.HeaderMap.Get("Allow"); allow != "" {
			t.Errorf("expected no Allow header, got %q", allow)
		}
	})

	t.Run("catch-all handler does not run", func(t *testing.T) {
		r := newRouter()
		called := false
		r.Get("/assets/*filepath", RouteOptions{Handler: func(c Context) error {
			called = true
			return c.SendString(200, "asset")
		}})

		resp, err := r.Test(TestOptions{Method: "GET", Path: "/assetsfoo"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 404 {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		if called {
			t.Error("catch-all handler ran for /assetsfoo")
		}
	})
}

// --- A miss leaves params untouched ---

func TestTree_MissLeavesParamsUntouched(t *testing.T) {
	cases := []struct {
		routes []string
		path   string
	}{
		{routes: []string{"/users/:id/posts"}, path: "/users/42/comments"},
		{routes: []string{"/users/:id/posts/:post"}, path: "/users/42/posts/7/x"},
		{routes: []string{"/users/:id/posts"}, path: "/users/42/"},
		{routes: []string{"/users/me/settings", "/users/:id/posts"}, path: "/users/me/other"},
	}

	for _, test := range cases {
		t.Run(test.path, func(t *testing.T) {
			tree := &node{}
			for _, route := range test.routes {
				tree.addRoute(route, &routeData{meta: route})
			}

			ps := Params{{Key: "outer", Value: "kept"}}
			if d, _ := tree.getValue(test.path, func() *Params { return &ps }); d != nil {
				t.Fatalf("path %q: expected no route, got %q", test.path, d.meta)
			}
			if len(ps) != 1 || ps[0] != (Param{Key: "outer", Value: "kept"}) {
				t.Errorf("path %q: params after miss = %#v, want only the caller's", test.path, ps)
			}
		})
	}
}

func TestRouter_NotFoundMiddlewareSeesNoStaleParams(t *testing.T) {
	r := newRouter()
	seen := "unset"
	r.Use(func(c Context) error {
		seen = c.Param("id")
		return c.Next()
	})
	r.Get("/users/:id/posts", RouteOptions{Handler: func(c Context) error {
		return c.SendString(200, "ok")
	}})

	resp, err := r.Test(TestOptions{Method: "GET", Path: "/users/42/comments"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	if seen != "" {
		t.Errorf("middleware on a 404 saw id=%q, want empty", seen)
	}
}
