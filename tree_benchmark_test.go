package gofi

import "testing"

var (
	benchmarkLookupData      *routeData
	benchmarkLookupTSR       bool
	benchmarkLookupParamsLen int
)

type lookupBenchmarkCase struct {
	name       string
	routes     []string
	path       string
	withParams bool
}

func BenchmarkTreeLookup(b *testing.B) {
	cases := []lookupBenchmarkCase{
		{
			name:   "static",
			routes: []string{"/search/issues"},
			path:   "/search/issues",
		},
		{
			name:       "static-beside-param",
			routes:     []string{"/users/me", "/users/:id"},
			path:       "/users/me",
			withParams: true,
		},
		{
			name:       "param-no-collision",
			routes:     []string{"/users/me", "/users/:id"},
			path:       "/users/123",
			withParams: true,
		},
		{
			name:       "deep-param",
			routes:     []string{"/repos/:owner/:repo/issues/:number"},
			path:       "/repos/open-sauce/gofi/issues/42",
			withParams: true,
		},
		{
			name:       "audit-ulid",
			routes:     []string{"/api/v1/events/filter-options", "/api/v1/events/:event"},
			path:       "/api/v1/events/01J99999999999999999999999",
			withParams: true,
		},
		{
			name:       "audit-export-download",
			routes:     []string{"/api/v1/events/:event", "/api/v1/events/exports/:export/download"},
			path:       "/api/v1/events/exports/01J99999999999999999999999/download",
			withParams: true,
		},
		{
			name:   "not-found",
			routes: []string{"/search/issues"},
			path:   "/missing",
		},
		{
			name:       "fallback-shared-byte",
			routes:     []string{"/users/me", "/users/:id"},
			path:       "/users/mike",
			withParams: true,
		},
		{
			name:       "fallback-nested",
			routes:     []string{"/users/:id/posts/latest", "/users/:id/posts/:post"},
			path:       "/users/mike/posts/lunch",
			withParams: true,
		},
		{
			name:       "fallback-not-found",
			routes:     []string{"/users/me/profile", "/users/:id/posts"},
			path:       "/users/mike/missing",
			withParams: true,
		},
		{
			name:       "fallback-tsr",
			routes:     []string{"/events/exports/", "/events/:event/records"},
			path:       "/events/exports",
			withParams: true,
		},
		{
			name:   "fallback-nil-params",
			routes: []string{"/users/me", "/users/:id"},
			path:   "/users/mike",
		},
		{
			name:       "catch-all",
			routes:     []string{"/static/*filepath"},
			path:       "/static/css/app.css",
			withParams: true,
		},
		{
			name:       "catch-all-miss",
			routes:     []string{"/static/*filepath"},
			path:       "/staticx",
			withParams: true,
		},
	}

	for _, test := range cases {
		b.Run(test.name, func(b *testing.B) {
			tree := &node{}
			for _, route := range test.routes {
				tree.addRoute(route, &routeData{pattern: route})
			}

			ps := make(Params, 0, tree.maxParams)
			params := func() *Params { return &ps }
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				ps = ps[:0]
				if test.withParams {
					benchmarkLookupData, benchmarkLookupTSR = tree.getValue(test.path, params)
				} else {
					benchmarkLookupData, benchmarkLookupTSR = tree.getValue(test.path, nil)
				}
				benchmarkLookupParamsLen = len(ps)
			}
		})
	}
}

func TestTree_BacktrackingAllocations(t *testing.T) {
	cases := []lookupBenchmarkCase{
		{
			name:       "static",
			routes:     []string{"/users/me", "/users/:id"},
			path:       "/users/me",
			withParams: true,
		},
		{
			name:       "fallback",
			routes:     []string{"/users/me", "/users/:id"},
			path:       "/users/mike",
			withParams: true,
		},
		{
			name:       "nested-fallback",
			routes:     []string{"/users/:id/posts/latest", "/users/:id/posts/:post"},
			path:       "/users/mike/posts/lunch",
			withParams: true,
		},
		{
			name:       "fallback-miss",
			routes:     []string{"/users/me/profile", "/users/:id/posts"},
			path:       "/users/mike/missing",
			withParams: true,
		},
		{
			name:   "fallback-nil-params",
			routes: []string{"/users/me", "/users/:id"},
			path:   "/users/mike",
		},
		{
			name:       "catch-all",
			routes:     []string{"/static/*filepath"},
			path:       "/static/css/app.css",
			withParams: true,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			tree := &node{}
			for _, route := range test.routes {
				tree.addRoute(route, &routeData{pattern: route})
			}

			ps := make(Params, 0, tree.maxParams)
			params := func() *Params { return &ps }
			allocs := testing.AllocsPerRun(1000, func() {
				ps = ps[:0]
				if test.withParams {
					benchmarkLookupData, benchmarkLookupTSR = tree.getValue(test.path, params)
				} else {
					benchmarkLookupData, benchmarkLookupTSR = tree.getValue(test.path, nil)
				}
				benchmarkLookupParamsLen = len(ps)
			})
			if allocs != 0 {
				t.Fatalf("lookup allocated %v times/op; want 0", allocs)
			}
		})
	}
}
