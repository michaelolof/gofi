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

// --- Phase 1 failing tests: static-vs-param coexistence (Issues #1 and #2) ---

// TestTree_StaticAndParamSiblings_StaticThenParam registers a static route
// before a param route at the same level and asserts correct dispatch for both.
// Currently FAILS — static child is silently overwritten by the param (Issue #1).
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
// Currently FAILS — panics with "conflicts with existing wildcard" (Issue #2).
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
// Currently FAILS — same root cause as Issue #1.
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
// Currently FAILS — tree state is invalid due to Issue #1.
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
// Currently FAILS — same root cause as Issue #1.
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
