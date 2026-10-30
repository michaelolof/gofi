# Writing Gofi services

This is the official guide to writing applications with Gofi. It describes
one way to do each thing. Gofi accepts other ways; this guide picks one so that
every Gofi service, whoever or whatever wrote it, reads the same.

It is written for developers and for coding agents. Follow it when you write
or change code that uses Gofi.

- **MUST** and **MUST NOT** are rules. Break one only with a comment explaining why.
- **SHOULD** is the default. Deviate when you have a reason.
- Every rule has an ID (`HND-3`) so reviews can point at it.

Every code sample here is taken from the reference service in
[`gofi-test-utils/examples/reference-service`](https://github.com/michaelolof/gofi-test-utils/tree/main/examples/reference-service),
which is compiled and tested against Gofi. When this guide and that service
disagree, the service is right and this guide has a bug.

Error handling has its own document: [docs/error-handling.md](docs/error-handling.md).

---

## Contents

1. [Project layout](#1-project-layout)
2. [Handlers](#2-handlers)
3. [Schemas](#3-schemas)
4. [Struct tags](#4-struct-tags)
5. [Responses](#5-responses)
6. [Errors](#6-errors)
7. [Dependencies](#7-dependencies)
8. [Routes](#8-routes)
9. [Middleware](#9-middleware)
10. [Server setup and lifecycle](#10-server-setup-and-lifecycle)
11. [Testing](#11-testing)
12. [OpenAPI documentation](#12-openapi-documentation)
13. [Context and goroutines](#13-context-and-goroutines)
14. [Streaming and WebSockets](#14-streaming-and-websockets)
15. [Habits from other frameworks](#15-habits-from-other-frameworks)
16. [Checklist for a new endpoint](#16-checklist-for-a-new-endpoint)
17. [Using this guide with coding agents](#17-using-this-guide-with-coding-agents)

---

## 1. Project layout

Gofi does not care where files live. This is the recommended layout:

```text
cmd/
  api/main.go                 process entry: config, deps, server, signals
  openapi/main.go             writes the OpenAPI document without starting a server
internal/
  domain/                     business types and interfaces; no Gofi imports
  http/
    deps/deps.go              the Deps interface, Register, Get, MustGet
    schemas/                  shared schema blocks and response body types
    middleware/               the service's middleware and router error handler
    routes/                   RegisterXRoutes functions: URL → handler
    server/server.go          builds the router; Start, Shutdown, Router
    handlers/
      projects/               one package per OpenAPI tag
        create.go             one operation per file
        create_test.go
        get.go
        get_test.go
        support.go            helpers shared by this package's handlers
      apikeys/
        create.go
        create_test.go
  testutil/testharness/       builds the real server with test dependencies
```

- **LAY-1 (SHOULD)** Keep handlers in a dedicated `handlers` folder with one
  subfolder per resource. Large services may group resources one level deeper,
  for example `handlers/api/<resource>` and `handlers/admin/<resource>`.
- **LAY-2 (MUST)** One operation per file: `create.go`, `get.go`, `list.go`,
  `archive.go`. Its tests go in the matching `_test.go` file.
- **LAY-3 (MUST)** Name each handler package after the OpenAPI tag its
  handlers use. Tags are lowercase kebab-case; the package name is the same
  words joined: tag `projects` → package `projects`, tag `api-keys` → package
  `apikeys`. Never use `_` in package names.
- **LAY-4 (SHOULD)** Helpers shared by one package's handlers go in
  `support.go` in that package. Helpers shared across packages go in
  `schemas` (types) or a dedicated package; never import one handler package's
  helpers from another.
- **LAY-5 (MUST)** Only HTTP-layer packages (`internal/http/...`) import
  Gofi. Domain and storage code must not.

---

## 2. Handlers

A handler is an exported package-level variable created with
`gofi.DefineHandler`, followed in the same file by its schema.

```go
// GetProjectHandler returns one of the caller's projects by ID.
var GetProjectHandler = gofi.DefineHandler(gofi.RouteOptions{
	Info: gofi.Info{
		Tags:        []string{"projects"},
		Description: "Returns one of the caller's projects. A project owned by someone else answers 404.",
	},

	Schema: &getProjectSchema{},

	Handler: func(c gofi.Context) error {
		s, err := gofi.ValidateAndBind[getProjectSchema](c)
		if err != nil {
			return apperr.Wrap(1793299514311, apperr.InvalidInput, err)
		}

		caller, _ := middleware.GetPrincipal(c)
		project, err := deps.MustGet(c).Projects().Get(c.Context(), caller.UserID, s.Request.Path.ProjectID)
		if errors.Is(err, domain.ErrNotFound) {
			return apperr.Wrap(1793299514312, apperr.NotFound, err).WithMsg("project not found")
		}
		if err != nil {
			return apperr.Wrap(1793299514313, apperr.Internal, err)
		}

		s.Ok.Body.Data = toProject(project)
		return c.Send(200, s.Ok)
	},
})

type getProjectSchema struct {
	Request struct {
		Header schemas.AuthHeader
		Path   struct {
			ProjectID string `json:"project_id" validate:"required" example:"prj_01J9Z3"`
		}
	}

	Ok  schemas.SuccessSchema[schemas.Project]
	Err schemas.ErrSchema
}
```

- **HND-1 (MUST)** Define every handler with `gofi.DefineHandler` and assign it
  to an exported package-level variable. Do not write `gofi.RouteOptions{...}`
  inline in a route registration, even for small routes.
- **HND-2 (MUST)** Name the variable `<Operation>Handler`, spelled out in full:
  `GetProjectHandler`, `CreateAPIKeyHandler`, `ListProjectsHandler`. The
  package name repeats at the call site (`projects.GetProjectHandler`); that
  is deliberate, so every handler name is unique across the codebase.
- **HND-3 (MUST)** Write the `RouteOptions` fields in the order `Info`,
  `Schema`, `Meta` (if used), `Handler`, with a blank line between each.
- **HND-4 (MUST)** The schema is an unexported type named `<operation>Schema`
  (`getProjectSchema`), in the same file, directly below its handler.
- **HND-5 (MUST)** Inside the handler, in this order:
  1. `gofi.ValidateAndBind[<schema>](c)` is the first statement.
  2. Get dependencies (`deps.MustGet(c)`) and anything middleware stored.
  3. Do the work.
  4. Fill in `s.<Status>.Body`.
  5. `return c.Send(<code>, s.<Status>)`.
- **HND-6 (MUST)** Put a one-line doc comment on every handler variable.
- **HND-7 (MUST)** Only HTTP-layer code receives a `gofi.Context`. Pass
  `c.Context()` and plain values to domain and storage code.
- **HND-8 (MUST)** Read request data from the bound schema
  (`s.Request.Path.ProjectID`), not from `c.Param`, `c.Query` or
  `c.HeaderVal`. Those accessors are for middleware, which has no schema.
- **HND-9 (MUST)** Every route that takes input or returns a body has a
  schema. A route without a schema gets no validation and no documentation.

---

## 3. Schemas

A schema is a struct with a `Request` field and one field per response. Field
names are what Gofi reads; they are not optional naming choices.

| Field | Meaning |
| --- | --- |
| `Request.Path` | Path parameters (`/projects/:project_id`) |
| `Request.Query` | Query string parameters |
| `Request.Header` | Request headers |
| `Request.Cookie` | Request cookies |
| `Request.Body` | Request body |
| `Ok`, `Created`, `Accepted`, `NoContent`, `BadRequest`, `NotFound`, ... | Response for that exact status |
| `Err` | Response for any 4xx or 5xx status without its own field |

Each response field holds `Header`, `Cookie` and `Body` blocks, all optional.

### The shared schemas package

Response wrappers and blocks used by more than one route live in
`internal/http/schemas`:

```go
// AuthHeader is the request header block for authenticated routes.
type AuthHeader struct {
	Authorization string `json:"authorization" validate:"required" description:"Bearer token." example:"Bearer eyJhbGciOi..."`
}

// SuccessSchema is the response block for a successful response carrying T.
type SuccessSchema[T any] struct {
	Body SuccessBody[T]
}

// SuccessBody is the success response payload.
type SuccessBody[T any] struct {
	Status string `json:"status" validate:"required" default:"success"`
	Data   T      `json:"data" validate:"required"`
}

// ErrSchema is the response block for every error response. Declare it as
// the Err field of every route schema.
type ErrSchema struct {
	Body ErrBody
}

// ErrBody is the error response payload written by the router's error handler.
type ErrBody struct {
	Status  string `json:"status" validate:"required" default:"error"`
	ErrorID int64  `json:"error_id" description:"Unique ID of the code path that produced the error. Quote it when reporting a problem."`
	Message string `json:"message" validate:"required"`
}
```

- **SCH-1 (MUST)** Every route schema declares `Err schemas.ErrSchema`, and
  `ErrBody` matches exactly what the router's error handler writes. This keeps
  the OpenAPI document honest about error responses.
- **SCH-2 (SHOULD)** Use one generic success wrapper (`SuccessSchema[T]`) for
  every success response, so all responses share one envelope. The exact
  envelope fields are your choice.
- **SCH-3 (SHOULD)** Write blocks used by only one route inline in that
  route's schema (`Path struct { ... }`). Move a block to `schemas` once a
  second route needs it.
- **SCH-4 (SHOULD)** Response body types (`schemas.Project`) live in
  `schemas`, one file per resource (`schemas/projects.go`), and are separate
  from domain types. Convert between them in the handler package's
  `support.go`.
- **SCH-5 (MUST)** Embed shared header blocks instead of copying their
  fields: `Header struct { schemas.AuthHeader; schemas.TraceHeaders }`.
- **SCH-6 (SHOULD)** Declare `content-type` in `Request.Header` only for
  bodies that are not JSON (`application/x-www-form-urlencoded`,
  `multipart/form-data`, or your own body parser), with the value in a
  `default` tag. The declared type picks the body parser and is what the
  OpenAPI document shows. With none declared, Gofi assumes JSON and falls back
  to the request's own `Content-Type` header.

---

## 4. Struct tags

- **TAG-1 (MUST)** Every schema field has a `json` tag. It names the field
  everywhere: body property, query parameter, path parameter, header and
  cookie.
- **TAG-2 (MUST)** Header names are lowercase kebab-case:
  `json:"authorization"`, `json:"x-request-id"`, `json:"content-type"`.
- **TAG-3 (SHOULD)** Body, query and path names are lowercase snake_case:
  `json:"project_id"`, `json:"created_at"`.
- **TAG-4 (SHOULD)** Write tags in the order `json`, `validate`, `default`,
  `description`, `example`.
- **TAG-5 (SHOULD)** Give request fields a `description` when the name alone
  does not say what is expected, and an `example` for anything with a format
  (IDs, dates, enums).
- **TAG-6 (MUST)** Use only rules Gofi defines in `validate` (see
  [docs/validations.md](docs/validations.md)) or rules you registered with
  `RegisterValidator`. Gofi currently ignores unknown rule names without an
  error, so a typo or a rule borrowed from another validator silently
  validates nothing.
- **TAG-7 (MUST NOT)** Do not write `validate:"omitempty"`. Gofi does not
  have that rule yet. It already skips validation for fields the request
  leaves out and for empty query parameters; a JSON body field sent as `""`
  is still validated. `json:"name,omitempty"` is fine and works as in
  `encoding/json`.
- **TAG-8 (MUST)** Path parameters are `:name` in the route pattern and
  `json:"name"` in `Request.Path`. Never `{name}`.

---

## 5. Responses

- **RES-1 (MUST)** Send responses with `c.Send(code, s.<Status>)`, passing the
  whole response block from the bound schema (`s.Ok`, `s.Created`), not its
  `Body`.
- **RES-2 (MUST)** Fill the response by setting fields on `s.<Status>.Body`.
  Either set the payload field (`s.Ok.Body.Data = toProject(p)`) or assign a
  whole body built by a helper (`s.Ok.Body = schemas.NewSuccess(p)`). Both
  keep the compiler checking the response shape. Do not build a separate
  response value and send that.
- **RES-3 (MUST)** The status code passed to `c.Send` matches the schema
  field: `200` with `Ok`, `201` with `Created`, `202` with `Accepted`.
  Write the code as an integer literal.
- **RES-4 (MUST NOT)** Do not use `c.SendString` or `c.SendBytes` in a
  handler that has a schema. They skip response validation and the documented
  contract. They are for error handlers, health probes and similar plumbing.
- **RES-5 (SHOULD)** Fields with a fixed value (`status: "success"`) get a
  `default` tag; Gofi writes it when the field is left empty.

---

## 6. Errors

Gofi's model is simple: handlers and middleware return errors, and one error
handler, registered with `UseErrorHandler`, turns every error into a
response. Which error type or library you use is your decision.
[docs/error-handling.md](docs/error-handling.md) describes the recommended
pattern; the examples in this guide use it.

- **ERR-1 (MUST)** Handlers and middleware report failures by returning an
  error. They do not write error responses themselves.
- **ERR-2 (MUST)** Register exactly one error handler per router with
  `r.UseErrorHandler`. It decides the status, logs, and writes the body
  described by `schemas.ErrBody`. It also receives Gofi's own errors: request
  validation failures, unknown routes (404), and request-read failures
  (413, 431, 408) as `*gofi.HTTPError`.
- **ERR-3 (MUST)** Wrap the error from `ValidateAndBind` in your error type
  before returning it, so it carries the same information as every other
  error your service returns:

  ```go
  s, err := gofi.ValidateAndBind[createProjectSchema](c)
  if err != nil {
  	return apperr.Wrap(1793299514301, apperr.InvalidInput, err)
  }
  ```

- **ERR-4 (SHOULD)** Services with no error type of their own return
  `gofi.NewHTTPError(code, message)`. Gofi's default error handler sends 4xx
  messages to the client and replaces 5xx messages with the status text.
- **ERR-5 (MUST NOT)** Do not put internal error text (database errors, stack
  details) in a response. Only messages meant for clients reach them.

---

## 7. Dependencies

Handlers are package-level variables, so they get their dependencies from the
router's `GlobalStore`. A `deps` package wraps that in a typed interface.

```go
type depsKey struct{}

// Deps is everything the HTTP layer needs from the rest of the service.
type Deps interface {
	Projects() domain.ProjectStore
	APIKeys() domain.APIKeyStore
	Authenticator() domain.Authenticator
}

// Register stores d in the router's GlobalStore. Call it during setup,
// before registering routes.
func Register(r gofi.Router, d Deps) {
	gofi.StoreSet(r.GlobalStore(), depsKey{}, d)
}

// Get returns the registered Deps, or an error if Register was never called.
func Get(c gofi.Context) (Deps, error) {
	v, ok := gofi.StoreGet(c.GlobalStore(), depsKey{})
	if !ok {
		return nil, errors.New("deps: not registered; call deps.Register(r, d) during setup")
	}
	d, ok := v.(Deps)
	if !ok {
		return nil, errors.New("deps: stored value does not implement Deps")
	}
	return d, nil
}

// MustGet returns the registered Deps and panics if Register was never
// called. A missing registration is a wiring bug, not a request error.
func MustGet(c gofi.Context) Deps {
	d, err := Get(c)
	if err != nil {
		panic(err)
	}
	return d
}
```

The concrete implementation (`deps.AppDeps` in the reference service) is a
struct built in `main` and in the test harness.

- **DEP-1 (MUST)** Handlers get dependencies with `deps.MustGet(c)`. `Get` is
  for code that can carry on without them.
- **DEP-2 (MUST)** `Deps` is an interface of accessor methods, one per
  dependency, returning interfaces where the dependency has more than one
  implementation (real and fake).
- **DEP-3 (MUST)** Call `deps.Register` once, while building the router,
  before any route is registered.
- **DEP-4 (MUST)** Store keys are unexported empty struct types
  (`type depsKey struct{}`) used with `gofi.StoreSet` and `gofi.StoreGet`.
  Do not use string keys or the deprecated `Set`/`Get` store methods.
- **DEP-5 (MUST NOT)** Do not write to `GlobalStore` after the server starts.
  It is not locked; writes during serving are data races. Per-request values
  go in `c.DataStore()` (see [section 9](#9-middleware)).

---

## 8. Routes

```go
// RegisterAPIRoutes registers the product API under /api/v1. Every route
// requires a bearer token.
func RegisterAPIRoutes(r gofi.Router, d deps.Deps) {
	r.Route("/api/v1", func(r gofi.Router) {
		r.Use(middleware.Authenticate(d.Authenticator()))

		r.Post("/projects", projects.CreateProjectHandler)
		r.Get("/projects/:project_id", projects.GetProjectHandler)

		r.Post("/projects/:project_id/api-keys", apikeys.CreateAPIKeyHandler)
	})
}
```

- **RTE-1 (MUST)** All route registration lives in the `routes` package, in
  functions named `Register<Area>Routes(r gofi.Router, d deps.Deps)`, one per
  area (`RegisterAPIRoutes`, `RegisterAdminRoutes`). Handler packages never
  register their own routes.
- **RTE-2 (MUST)** Group routes that share a prefix with `r.Route(prefix, ...)`
  and call `r.Use` at the top of the block, before its routes. `Use` only
  applies to routes registered after it.
- **RTE-3 (SHOULD)** When routes under one prefix need different middleware
  (read scope vs write scope), write a separate `r.Route` block with the same
  prefix for each middleware stack, rather than wrapping single routes with
  `r.With`.
- **RTE-4 (MUST)** Register everything (routes, middleware, validators, body
  parsers, docs) before the server starts. Starting the server seals the
  router; later registration panics.
- **RTE-5 (MUST)** Use Gofi's pattern syntax: `:name` for one segment,
  `*name` for the rest of the path. Static segments win over parameters, and
  parameters win over catch-alls, whatever the registration order.
- **RTE-6 (SHOULD)** Separate groups of related routes with a blank line, and
  comment a route block when its middleware is not obvious from the code.

---

## 9. Middleware

Middleware has the same signature as a handler and calls `c.Next()` to run
the rest of the chain.

```go
type principalKey struct{}

// Authenticate verifies the bearer token and stores the caller for handlers.
func Authenticate(authn domain.Authenticator) gofi.MiddlewareFunc {
	return func(c gofi.Context) error {
		token, ok := strings.CutPrefix(c.HeaderVal("authorization"), "Bearer ")
		if !ok || token == "" {
			return apperr.New(1793299514201, apperr.Unauthorized, "missing bearer token")
		}
		p, err := authn.Authenticate(c.Context(), token)
		if err != nil {
			return apperr.Wrap(1793299514202, apperr.Unauthorized, err).WithMsg("invalid bearer token")
		}
		gofi.StoreSet(c.DataStore(), principalKey{}, p)
		return c.Next()
	}
}

// GetPrincipal returns the caller stored by Authenticate.
func GetPrincipal(c gofi.Context) (domain.Principal, bool) {
	v, _ := gofi.StoreGet(c.DataStore(), principalKey{})
	p, ok := v.(domain.Principal)
	return p, ok
}
```

### Where middleware lives

- **MDW-1 (SHOULD)** In a small service, write middleware as unexported
  functions in the routes file and mount them in one `r.Use` call:

  ```go
  func RegisterRoutes(r gofi.Router, d deps.Deps) {
  	r.Use(
  		requestLogger,
  		requireJSON,
  	)

  	r.Post("/projects", projects.CreateProjectHandler)
  	r.Get("/projects/:project_id", projects.GetProjectHandler)
  }
  ```

- **MDW-2 (SHOULD)** Once a service has several middleware, or middleware
  that needs its own tests, move them into `internal/http/middleware`, one
  file per middleware, and mount them with one `r.Use` call per line.

### Writing middleware

- **MDW-3 (MUST)** A middleware that needs dependencies is a constructor that
  takes them and returns `gofi.MiddlewareFunc` (`Authenticate(authn)`). One
  that needs none is a plain function (`RequestID`).
- **MDW-4 (MUST)** A middleware that stores a value for later code uses an
  unexported empty struct key with `c.DataStore()`, and exports a getter in
  the same file (`GetPrincipal`, `GetRequestID`). Callers never touch the key.
- **MDW-5 (MUST)** Middleware reports failures by returning an error, like a
  handler. To stop the chain, return without calling `c.Next()`.
- **MDW-6 (MUST)** Return the error from `c.Next()` unchanged unless the
  middleware's job is to change it.

### Mounting middleware

- **MDW-7 (MUST)** Mount middleware that applies to every route in
  `server.New`. Mount middleware for a group of routes inside that group's
  `r.Route` block in the `routes` package.
- **MDW-8 (SHOULD)** Mount global middleware in this order: `gofimw.Recover()`,
  request ID, security headers and CORS, logging. Authentication and
  authorization go on route groups.
- **MDW-9 (MUST)** Check the error from middleware constructors that return
  one (Gofi's `CORS`, `CSRF`, `Cache`, `Timeout`, `RateLimit`,
  `ConcurrencyLimit`, `ContextTimeout`), and fail server setup if it is set.
- **MDW-10 (SHOULD)** Import Gofi's middleware package as `gofimw` so it does
  not clash with your own `middleware` package.

---

## 10. Server setup and lifecycle

The `server` package builds the router in one function, in a fixed order.

```go
// New builds the router: dependencies, global middleware, error handler,
// routes and docs, in that order.
func New(args Args) (*Server, error) {
	cors, err := gofimw.CORS(gofimw.CORSConfig{
		AllowOrigins: args.CORSOrigins,
		AllowHeaders: "authorization, content-type",
	})
	if err != nil {
		return nil, err
	}

	r := gofi.NewRouter()
	deps.Register(r, args.Deps)

	r.Use(gofimw.Recover())
	r.Use(middleware.RequestID)
	r.Use(cors)
	r.UseErrorHandler(middleware.ErrorHandler(args.Log))

	routes.RegisterAPIRoutes(r, args.Deps)

	if err := gofi.ServeDocs(r, DocsOptions()); err != nil {
		return nil, err
	}
	return &Server{router: r, addr: args.Addr}, nil
}

// Start serves until Shutdown is called.
func (s *Server) Start() error { return s.router.Listen(s.addr) }

// Shutdown stops accepting connections and waits for in-flight requests
// until ctx is done.
func (s *Server) Shutdown(ctx context.Context) error { return s.router.ShutdownWithContext(ctx) }

// Router returns the router, for tests (Router().Test) and the spec export tool.
func (s *Server) Router() gofi.Router { return s.router }
```

`main` builds the dependencies, starts the server, and shuts it down on a
signal:

```go
	go func() {
		if err := srv.Start(); err != nil {
			log.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown failed", "error", err)
	}
```

- **SRV-1 (MUST)** Build the router in `server.New(Args) (*Server, error)` in
  this order: `gofi.NewRouter`, `Configure` (if needed), `deps.Register`,
  global `Use`, `UseErrorHandler`, `Register<Area>Routes`, docs.
- **SRV-2 (MUST)** One router per binary, built once. Tests and the spec
  export tool build it through the same `server.New`.
- **SRV-3 (MUST)** Start with `Listen` (or `ListenTLS`/`ListenTLSMutual`) and
  stop with `ShutdownWithContext` and a timeout. Do not use
  `gofi.ListenAndServe`.
- **SRV-4 (SHOULD)** Use `gofi.NewFasthttpServer` or `Handler()` only when you
  need fasthttp server settings Gofi does not expose. You then own body
  limits, timeouts and shutdown.
- **SRV-5 (SHOULD)** Set request size limits with
  `r.Configure(gofi.Config{BodyLimit: ...})` in `server.New`.

---

## 11. Testing

Handler tests send real requests through the real router, built by a test
harness with in-memory or fake dependencies.

```go
func TestGetProjectHandler_OtherOwner_Returns404(t *testing.T) {
	h := testharness.New(t)
	project, err := h.Deps.ProjectStore.Create(context.Background(), domain.Project{Name: "Payments", OwnerID: "alice"})
	require.NoError(t, err)

	resp, err := h.Test(gofi.TestOptions{
		Method:  "GET",
		Path:    "/api/v1/projects/:project_id",
		Paths:   map[string]string{"project_id": project.ID},
		Headers: testharness.AuthHeaders("token-bob"),
	})
	require.NoError(t, err)
	require.Equal(t, 404, resp.Code(), resp.BodyString())

	var out schemas.ErrBody
	require.NoError(t, resp.BindBody(&out))
	assert.Equal(t, "project not found", out.Message)
	assert.Equal(t, int64(1793299514312), out.ErrorID)
}
```

- **TST-1 (MUST)** Each operation file has a matching `_test.go` file in an
  external test package (`package projects_test`).
- **TST-2 (MUST)** Test handlers through the full router with
  `Router.Test(gofi.TestOptions{...})`, using a router built by
  `server.New`. Tests then cover routing, middleware, validation, the handler
  and the error handler together.
- **TST-3 (MUST)** Build that router in a `testharness` package with
  `New(t *testing.T)`, which supplies test dependencies and returns an
  isolated copy of the service per test.
- **TST-4 (SHOULD)** Pass the route pattern as `Path` and the parameter
  values in `Paths`, so the test names the route it exercises.
- **TST-5 (MUST)** Assert the status first, with the body as the failure
  message: `require.Equal(t, 201, resp.Code(), resp.BodyString())`. Then
  decode the body into the `schemas` types with `resp.BindBody` and assert
  fields.
- **TST-6 (SHOULD)** Name tests `Test<Handler>_<Scenario>`, for example
  `TestCreateProjectHandler_InvalidName_Returns400`. Cover the success path,
  each documented error, and authentication.
- **TST-7 (SHOULD)** Use `Router.Inject` only to test a schema or handler in
  isolation from routing and middleware.
- **TST-8 (MUST NOT)** Do not start a listener or use `net/http/httptest` to
  test handlers.

---

## 12. OpenAPI documentation

Gofi builds the OpenAPI document from your route schemas and `Info`.

- **DOC-1 (MUST)** Every handler's `Info` has exactly one tag, the lowercase
  kebab-case tag its package is named after, and a `Description`.
  `Summary` and `OperationId` are optional.
- **DOC-2 (SHOULD)** Write `Description` for API consumers: what the route
  does, who may call it, and any non-obvious behavior ("a project owned by
  someone else answers 404").
- **DOC-3 (SHOULD)** Serve the docs UI from `server.New` with
  `gofi.ServeDocs`, and keep the `DocsOptions` in one function so the export
  tool uses the same options.
- **DOC-4 (SHOULD)** Provide a command (`cmd/openapi`) that builds the server
  through `server.New` with dependencies that do no I/O and writes
  `gofi.OpenAPISpec(srv.Router(), server.DocsOptions())`. Commit its output if
  clients generate code from it.

---

## 13. Context and goroutines

Gofi reuses its `Context` objects. One is valid only until the handler
returns.

- **CTX-1 (MUST)** Pass `c.Context()` to anything that takes a
  `context.Context` (database calls, HTTP clients). It is cancelled when the
  client disconnects.
- **CTX-2 (MUST NOT)** Do not use `c.Request().Context()`. It returns the raw
  fasthttp request, not a `context.Context`.
- **CTX-3 (MUST)** Before starting a goroutine that uses request data, call
  `c.Copy()` and give the goroutine the copy. Its `Context()` is
  `context.Background()`, so the work is not cancelled when the request ends.
- **CTX-4 (MUST NOT)** Do not keep a `gofi.Context` after the handler
  returns, store it in a struct, or send it on a channel.

---

## 14. Streaming and WebSockets

The handler rules (section 2) apply to both.

- **STR-1 (MUST)** Server-sent events use `c.SendStream(code, s.<Status>, fn)`
  with a response block that declares `content-type: text/event-stream`.
  Return every write and flush error from the callback. Use
  `gofi.SendStreamWithOptions` when the stream needs a size or time limit.
- **STR-2 (MUST)** WebSocket routes are defined with
  `websocket.DefineWebSocket` and declare the handshake in `Schema.Request`
  and the messages in `Schema.WebSocket`. See
  [docs/websockets.md](docs/websockets.md).
- **STR-3 (SHOULD)** Use streaming when only the server sends; use WebSockets
  when both sides send at any time.

---

## 15. Habits from other frameworks

Gofi looks like chi, Fiber and Echo in places. These habits do not carry
over:

| Habit | In Gofi |
| --- | --- |
| `c.JSON(200, v)`, `c.Status(200).JSON(v)` | `s.Ok.Body.Data = v` then `c.Send(200, s.Ok)` |
| `c.ShouldBindJSON(&v)`, `c.BodyParser(&v)`, `c.Bind(&v)` | `gofi.ValidateAndBind[schema](c)` |
| `c.Param("id")` in a handler | `s.Request.Path.ID` |
| `/users/{id}` | `/users/:id` |
| `http.Handler`, `ServeHTTP`, `httptest.NewRecorder` | Not available; test with `Router.Test` |
| `validate:"omitempty,..."`, `dive`, `required_if` | Not Gofi rules (TAG-6, TAG-7) |
| `r.GET`, `r.POST` | `r.Get`, `r.Post` |
| `gofi.New()` | `gofi.NewRouter()` |
| `c.Request().Context()` | `c.Context()` |
| Writing an error response in the handler | Return the error (ERR-1) |
| Handler struct with dependencies as fields | Package-level handler variable and `deps.MustGet(c)` |

---

## 16. Checklist for a new endpoint

1. Pick the tag. Find or create `internal/http/handlers/<package>`.
2. Create `<operation>.go` with `<Operation>Handler` and `<operation>Schema`.
3. Declare `Request` blocks, the success response, and `Err schemas.ErrSchema`.
4. Write the handler: bind, wrap the validation error, get deps, work, fill
   `s.<Status>.Body`, `c.Send`.
5. Give every returned error a new error ID (see the error-handling doc).
6. Register the route in the right `r.Route` block in `routes`.
7. Write `<operation>_test.go`: success, each documented error, auth.
8. Run `go test ./...` and regenerate the OpenAPI document if it is committed.

---

## 17. Using this guide with coding agents

This guide ships inside the Gofi module, so the copy in your module cache
always matches the Gofi version in your `go.mod`. Paste this into your
service's `AGENTS.md` (and `CLAUDE.md`, or add `@AGENTS.md` to it):

```markdown
## Gofi

This service uses the Gofi router (github.com/michaelolof/gofi). Before
writing or changing any code that imports Gofi, read the guide for the
version in go.mod:

    $(go env GOMODCACHE)/github.com/michaelolof/gofi@<version>/GUIDELINES.md

and the error-handling pattern next to it in docs/error-handling.md.

Rules that are easy to get wrong:
- One operation per file; handler = exported `var <Operation>Handler = gofi.DefineHandler(...)`
  with its unexported `<operation>Schema` directly below.
- First statement: `s, err := gofi.ValidateAndBind[<schema>](c)`; wrap and return the error.
- Read input from `s.Request.*`; respond with `c.Send(<code>, s.<Status>)` after filling `s.<Status>.Body`.
- Every schema declares `Err schemas.ErrSchema`.
- Return errors; never write error responses in handlers.
- Dependencies via `deps.MustGet(c)`; pass `c.Context()` to domain code.
- Path params are `:name`. No `validate:"omitempty"`.
- Test through the full router with `Router.Test`, from an external `_test` package.
```
