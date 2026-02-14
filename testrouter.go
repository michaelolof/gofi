package gofi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path"
)

type TestRoute struct {
	Method  string
	Path    string
	Options *RouteOptions
}

func NewTestRouter(routes []TestRoute) TestRouter {
	s := newServeMux()

	for _, route := range routes {
		s.method(route.Method, route.Path, *route.Options)
	}

	return &testRouter{
		m: s,
		s: httptest.NewServer(s),
		r: routes,
	}
}

func TryTestRouter(router Router, routes []TestRoute) TestRouter {
	s, ok := router.(*serveMux)
	if !ok {
		panic("unknown router used. Did you call gofi.NewServeMux()")
	}

	for _, route := range routes {
		s.method(route.Method, route.Path, *route.Options)
	}

	return &testRouter{
		m: s,
		s: httptest.NewServer(s),
		r: routes,
	}
}

type InvokeOptions struct {
	Path    string
	Method  string
	Query   map[string]string
	Paths   map[string]string
	Headers map[string]string
	Cookies []http.Cookie
	Body    io.Reader
}

type TestRouter interface {
	Invoke(opts InvokeOptions) (*httptest.ResponseRecorder, error)
}

type testRouter struct {
	m *serveMux
	s *httptest.Server
	r []TestRoute
}

func (t *testRouter) Invoke(opts InvokeOptions) (*httptest.ResponseRecorder, error) {
	r, err := http.NewRequest(opts.Method, opts.Path, opts.Body)
	if err != nil {
		return nil, err
	}

	var resp *httptest.ResponseRecorder
	for _, ropts := range t.r {
		v, err := path.Match(ropts.Path, opts.Path)
		if err != nil {
			return nil, err
		}

		if ropts.Method == opts.Method && v {
			handler := applyMiddleware(ropts.Options.Handler, ropts.Options.Middlewares)

			if len(opts.Query) > 0 {
				qParams := r.URL.Query()
				for name, value := range opts.Query {
					qParams.Add(name, value)
				}
				r.URL.RawQuery = qParams.Encode()
			}

			for name, value := range opts.Paths {
				r.SetPathValue(name, value)
			}

			for _, cookie := range opts.Cookies {
				r.AddCookie(&cookie)
			}

			for name, value := range opts.Headers {
				r.Header.Add(name, value)
			}

			w := httptest.NewRecorder()
			c := newContext(w, r)
			c.setContextSettings(newContextOptions(ropts.Path, ropts.Method), t.m.routeMeta, t.m.globalStore, t.m.opts)

			err = handler(c)
			if err != nil {
				defaultErrorHandler(err, c)
				return w, nil
			}

			resp = w
		}
	}

	return resp, nil
}
