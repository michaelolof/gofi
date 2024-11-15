package gofi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
)

var customValidators = make(map[string]func([]any) func(any) error)

type ServeMux struct {
	sm          *http.ServeMux
	paths       docsPaths
	errHandler  func(err error, c Context)
	middlewares Middlewares
}

func (s *ServeMux) Route(method string, path string, opts HandlerOptions) {
	var rules *schemaRules
	if opts.Schema != nil {

		rs := compileSchema(opts.Schema, opts.Info)
		rs.specs.normalize(method, path)
		if len(s.paths[path]) == 0 {
			v := map[string]openapiOperationObject{
				strings.ToLower(method): rs.specs,
			}

			if !opts.Info.Hidden {
				s.paths[path] = v
			}
		} else {
			if !opts.Info.Hidden {
				s.paths[path][strings.ToLower(method)] = rs.specs
			}
		}

		rules = &rs.rules
	}

	s.sm.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
		c := NewContext(w, r)
		c.setSchemaRules(rules)

		err := opts.Handler(c)
		if err != nil {
			s.errHandler(err, c)
			return
		}
	})
}

func (s *ServeMux) Get(path string, handler HandlerOptions) {
	s.Route("GET", path, handler)
}

func (s *ServeMux) Post(path string, handler HandlerOptions) {
	s.Route("POST", path, handler)
}

func (s *ServeMux) Put(path string, handler HandlerOptions) {
	s.Route("PUT", path, handler)
}

func (s *ServeMux) Patch(path string, handler HandlerOptions) {
	s.Route("PATCH", path, handler)
}

func (s *ServeMux) Delete(path string, handler HandlerOptions) {
	s.Route("DELETE", path, handler)
}

func (s *ServeMux) Inject(opts InjectOptions) (*httptest.ResponseRecorder, error) {
	r, err := http.NewRequest(opts.Method, opts.Path, opts.Body)
	for name, value := range opts.Paths {
		r.SetPathValue(name, value)
	}

	for _, cookie := range opts.Cookies {
		r.AddCookie(&cookie)
	}

	if err != nil {
		return nil, err
	}
	def := opts.Handler
	if def == nil {
		return nil, fmt.Errorf("gofi controller not defined for the given method '%s' and path '%s'", opts.Method, opts.Path)
	}

	rules := compileSchema(def.Schema, def.Info)
	rules.specs.normalize(opts.Method, opts.Path)

	w := httptest.NewRecorder()
	c := NewContext(w, r)
	c.setSchemaRules(&rules.rules)

	err = def.Handler(c)
	if err != nil {
		s.errHandler(err, c)
		return w, nil
	}

	return w, nil
}

// With adds inline middlewares for an endpoint handler.
func (s *ServeMux) With(middlewares ...func(http.Handler) http.Handler) *ServeMux {

	// Copy middlewares from parent inline muxs
	mws := append(middlewares, middlewares...)

	im := &ServeMux{
		sm:          http.NewServeMux(),
		paths:       map[string]map[string]openapiOperationObject{},
		errHandler:  defaultErrorHandler,
		middlewares: mws,
	}

	return im
}

func (s *ServeMux) Handle(pattern string, handler http.Handler) {
	s.sm.Handle(pattern, handler)
}

func (s *ServeMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	s.sm.HandleFunc(pattern, handler)
}

func (s *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var h http.Handler = s.sm
	middlewares := s.middlewares
	if len(middlewares) > 0 {
		h = middlewares[len(middlewares)-1](h)
		for i := len(middlewares) - 2; i >= 0; i-- {
			h = middlewares[i](h)
		}
	}

	h.ServeHTTP(w, r)
}

func (s *ServeMux) Use(middlewares ...func(http.Handler) http.Handler) {
	s.middlewares = append(s.middlewares, middlewares...)
}

func (s *ServeMux) RegisterValidators(validators map[string]func([]any) func(any) error) {
	customValidators = validators
}

func NewServeMux() *ServeMux {
	middlewares := make(Middlewares, 0, 10)
	return &ServeMux{
		sm:          http.NewServeMux(),
		paths:       map[string]map[string]openapiOperationObject{},
		errHandler:  defaultErrorHandler,
		middlewares: middlewares,
	}
}

func ListenAndServe(addr string, handler http.Handler) error {
	return http.ListenAndServe(addr, handler)
}

const docsPath = "/q/openapi"

func ServeDocs(r Router, opts DocsOptions) error {
	m, ok := r.(*ServeMux)
	if !ok {
		return errors.New("invalid server mux passed when serving docs")
	}

	if opts.Ui.RoutePrefix == "" {
		return nil
	}

	d := opts.getDocs(m)

	var cerr error

	m.sm.HandleFunc(fmt.Sprintf("GET %s", path.Join(opts.Ui.RoutePrefix, docsPath)), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		ds, err := json.Marshal(d)
		if err != nil {
			cerr = err
			return
		}

		os.WriteFile("openapi.json", ds, 0647)
		w.Write(ds)
	})

	m.sm.HandleFunc(fmt.Sprintf("GET %s", opts.Ui.RoutePrefix), func(w http.ResponseWriter, r *http.Request) {
		tmplt := opts.Ui.Template
		if tmplt == nil {
			// Make swagger the default template :(
			tmplt = SwaggerTemplate()
		}
		w.Header().Set("content-type", "text/html")
		w.Write(tmplt.HTML(path.Join(opts.Ui.RoutePrefix, docsPath)))
	})

	return cerr
}

func fallback[T comparable](v T, d T) T {
	var e T
	if v == e {
		return d
	} else {
		return v
	}
}

type defaultErrResp struct {
	Status     string `json:"status"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

func defaultErrorHandler(err error, c Context) {
	c.Writer().Header().Set("content-type", "application/json; charset-utf8")
	c.Writer().WriteHeader(500)
	bs, err := json.Marshal(defaultErrResp{
		Status:     "error",
		StatusCode: 500,
		Message:    err.Error(),
	})
	if err != nil {
		log.Fatalln(err)
	}

	c.Writer().Write(bs)
}

type InjectOptions struct {
	Path    string
	Method  string
	Paths   map[string]string
	Cookies []http.Cookie
	Body    io.Reader
	Handler *HandlerOptions
}
