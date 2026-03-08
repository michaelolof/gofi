package gofi

import (
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
