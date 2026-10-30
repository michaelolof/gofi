# gofi

Gofi is an openapi3 schema-based HTTP router for Golang.

## Features

- **Schema-Based Routing**: Define routes with type-safe schemas using Go structs.
- **Automatic Validation**: Request and response validation based on your schema definitions.
- **Fast Performance**: Uses `valyala/fasthttp` for HTTP and `valyala/fastjson` for optimized JSON encoding.
- **Developer Friendly**: Simple, intuitive API for defining routes and handlers.
- **OpenAPI Documentation**: Automatic API documentation generation with support for multiple UI providers (StopLight, Swagger, RapidDoc, Redocly, Scalar).
- **Graceful Shutdown**: Native support for zero-downtime deployments.
- **WebSockets & Streaming**: Built-in high-performance wrappers for `fasthttp/websocket` and Server-Sent Events (SSE).
- **mTLS Support**: Define mutual-TLS authentication out of the box.
- **Customizable**: Add custom validators, body parsers, and type specifications.
- **Error Handling**: Built-in error handling with customizable handlers.
- **Middleware Support**: Context-aware middleware via `MiddlewareFunc`.


## Installation

```sh
go get -u github.com/michaelolof/gofi
```

## How to write a Gofi service

**[GUIDELINES.md](GUIDELINES.md) is the official guide to writing services with
Gofi**: project layout, handlers, schemas, errors, dependencies, routes,
middleware, testing and docs, each as a numbered rule. Read it before writing
Gofi code; coding agents should too. Error handling has its own guide in
[docs/error-handling.md](docs/error-handling.md).

The [reference service](https://github.com/michaelolof/gofi-test-utils/tree/main/examples/reference-service)
follows every rule in the guide and is compiled and tested against Gofi. More
focused examples (JSON, form data, multipart, streaming, WebSockets) are in
[gofi-test-utils/cmd/examples](https://github.com/michaelolof/gofi-test-utils/tree/main/cmd/examples).

## Quick Start

A complete single-file program. A real service splits this across packages as
described in [GUIDELINES.md](GUIDELINES.md#1-project-layout).

```go
package main

import (
	"log"

	"github.com/michaelolof/gofi"
)

// PingHandler replies to a message.
var PingHandler = gofi.DefineHandler(gofi.RouteOptions{
	Info: gofi.Info{
		Tags:        []string{"ping"},
		Description: "Replies to a message.",
	},

	Schema: &pingSchema{},

	Handler: func(c gofi.Context) error {
		s, err := gofi.ValidateAndBind[pingSchema](c)
		if err != nil {
			return err
		}

		s.Ok.Body.Reply = "pong: " + s.Request.Body.Message
		return c.Send(200, s.Ok)
	},
})

type pingSchema struct {
	Request struct {
		Body struct {
			Message string `json:"message" validate:"required,min=5"`
		}
	}

	Ok struct {
		Body struct {
			Reply string `json:"reply" validate:"required"`
		}
	}
}

func main() {
	r := gofi.NewRouter()

	r.Post("/ping", PingHandler)

	if err := gofi.ServeDocs(r, gofi.DocsOptions{
		Views: []gofi.DocsView{
			{RoutePrefix: "/docs", Template: gofi.StopLight()},
		},
	}); err != nil {
		log.Fatal(err)
	}

	log.Println("Server listening on :8080")
	log.Fatal(r.Listen(":8080"))
}
```

## Router Setup

### Initialization

Create a new router instance using `NewRouter()`:

```go
r := gofi.NewRouter()
```

### Defining a Route Schema

Schemas are defined as nested structs representing the HTTP request and response structure.

```go
type updateUserSchema struct {
    Request struct {
        // Path parameters (/users/:user_id)
        Path struct {
            UserID string `json:"user_id" validate:"required,uuid"`
        }
        // Query parameters (?notify=true)
        Query struct {
            Notify bool `json:"notify" default:"false"`
        }
        // Headers: names are lowercase
        Header struct {
            Authorization string `json:"authorization" validate:"required"`
        }
        // Request body (JSON unless the Header block declares another content-type)
        Body struct {
            Name  string `json:"name" validate:"required"`
            Email string `json:"email" validate:"required,email"`
        }
    }

    // Response fields are named after the status they answer (Ok = 200)
    Ok struct {
        Body UserResponse
    }
    // Err covers every 4xx and 5xx status without its own field
    Err struct {
        Body ErrorResponse
    }
}
```

For a detailed guide on defining schemas, supported fields, response types and validation, please refer to the [Schema Guide](docs/schema-info.md).

### Defining a Route Handler

A handler is an exported package-level variable built with `DefineHandler`,
with its schema directly below it:

```go
// UpdateUserHandler updates a user's name and email.
var UpdateUserHandler = gofi.DefineHandler(gofi.RouteOptions{
    Info: gofi.Info{
        Tags:        []string{"users"},
        Description: "Updates a user's name and email.",
    },

    Schema: &updateUserSchema{},

    Handler: func(c gofi.Context) error {
        s, err := gofi.ValidateAndBind[updateUserSchema](c)
        if err != nil {
            return err
        }

        user, err := updateUser(c.Context(), s.Request.Path.UserID, s.Request.Body.Name, s.Request.Body.Email)
        if err != nil {
            return err
        }

        s.Ok.Body = toUserResponse(user)
        return c.Send(200, s.Ok)
    },
})

r.Put("/users/:user_id", UpdateUserHandler)
```

See [GUIDELINES.md](GUIDELINES.md#2-handlers) for the full set of handler rules,
and the [Route Options Guide](docs/route-options.md) for every `Context` method
and `RouteOptions` field.


### Global Error Handler

Handlers and middleware return errors; one error handler, registered with
`UseErrorHandler`, turns them into responses:

```go
r.UseErrorHandler(func(err error, c gofi.Context) {
    // Log the full error; send clients only what is safe for them to see.
    log.Printf("request failed: %v", err)
    _ = c.SendString(gofi.StatusForError(err), "request failed")
})
```

[docs/error-handling.md](docs/error-handling.md) describes the recommended
pattern: errors that carry a unique ID, a kind that decides the status, and a
message that is safe to show.

Without a custom handler, Gofi responds with a JSON envelope (`{"status":"error","statusCode":...,"message":...}`, `Content-Type: application/json`):

- request validation errors return `400` with the validation message;
- a `gofi.NewHTTPError` with a `4xx` code returns that code and its message;
- any other error, including a `5xx` `HTTPError`, returns its status with the generic status text, and the full error is logged.

A panic in a handler, middleware or error handler is recovered by the router: it is logged and the client receives a generic `500`, even without the `Recover` middleware.

### Registration happens before serving

Register routes, middleware, validators, body parsers and configuration before the server starts. `Listen`, `ListenTLS`, `ListenTLSMutual` and `Handler()` seal the router (and every router derived from it with `Route`, `Group` or `With`); later registration panics, because requests read these tables without locks. `gofi.Seal(r)` seals earlier. `Use` applies only to routes registered after it.

### Plugins

You can attach shared state or plugins to the router using the `GlobalStore`, which is accessible in all route handlers. The same applies to the per-request `DataStore()`.

For a service's dependencies (stores, clients, config), wrap the `GlobalStore` in a small typed `deps` package with `Register` and `MustGet`, as described in [GUIDELINES.md](GUIDELINES.md#7-dependencies). The example below shows the underlying store API.

Store keys may be **any comparable value**. The recommended convention is a distinct, empty `struct{}` type per key: it is collision-proof (two different named types can never clash) and the fastest option — boxing a zero-sized value into the store allocates nothing, and lookups short-circuit on the type descriptor. Use the generic `StoreSet` / `StoreGet` helpers to write and read; `StoreGet` returns the stored value as `any` — converting it to the appropriate type is the caller's responsibility, same as reading from a `map[any]any`.

```go
// 1. Define a private, collision-proof key type
type dbKey struct{}

// 2. Initialize plugin
myDB := NewDatabase()

// 3. Register in GlobalStore
gofi.StoreSet(r.GlobalStore(), dbKey{}, myDB)

// 4. Access in Handler — assert to the expected type yourself
gofi.DefineHandler(gofi.RouteOptions{
    Handler: func(c gofi.Context) error {
        if v, found := gofi.StoreGet(c.GlobalStore(), dbKey{}); found {
            db := v.(*Database)
            db.Query("...")
        }
        return nil
    },
})
```

Keys of non-comparable types (slices, maps, funcs) will panic, exactly as with the standard library's `context.WithValue`.

> **Deprecation:** the string-keyed methods `Set` / `Get` / `Has` / `TryGet` / `All` on the store are deprecated in favour of the generic `StoreSet` / `StoreGet` / `StoreHas` / `StoreTryGet` / `StoreAll` helpers and will be removed in a future release. Existing string keys keep working — pass a `string` as the key to the generic helpers.

#### GlobalStore concurrency: write once, at setup

`GlobalStore` is **not internally synchronized** — it is a single instance shared by every in-flight request, and it is deliberately unlocked for performance. This mirrors how the standard library's `context.WithValue` gets away without a mutex: safety comes from never mutating shared state once other goroutines can observe it, not from locking around every access.

In practice:

- ✅ Call `r.GlobalStore().Set(...)` / `gofi.StoreSet(r.GlobalStore(), ...)` during router setup, **before** `Listen` / `ListenTLS` / `Serve`.
- ✅ Reading from `GlobalStore` inside handlers and middleware (`Get`, `Has`, `StoreGet`, ...) is always safe, as long as all writes finished before the server started.
- ❌ Never call `Set` / `StoreSet` on a `GlobalStore` from inside a request handler, middleware, or any goroutine that runs concurrently with a live server — this is a genuine data race (`append` can reallocate the backing slice mid-read), not just a logical inconsistency, and will show up under `go test -race`.

If you need state that's populated per-request at runtime, use `c.DataStore()` instead — it's scoped to a single request (or its `Context.Copy()`) and is never shared across goroutines, so no synchronization is needed.

### Goroutines and Concurrency

Gofi heavily utilizes `sync.Pool` under the hood to ensure zero-allocation routing. Because of this, the `gofi.Context` object is **immediately recycled** back to the framework as soon as your HTTP handler returns. 

If you are passing `gofi.Context` to other functions where you **cannot strictly guarantee** they won't spawn background goroutines or outlive the handler scope, you **cannot** pass the original context to them. Doing so will result in a data race and memory corruption when a subsequent request overwrites your context memory pools.

Instead, you should proactively use `c.Copy()` to safely detach and clone the necessary request paths, parameters, datastores, and headers:

```go
// ProcessHandler starts a background job and answers immediately.
var ProcessHandler = gofi.DefineHandler(gofi.RouteOptions{
    Info: gofi.Info{
        Tags:        []string{"jobs"},
        Description: "Starts a background job.",
    },

    Schema: &processSchema{},

    Handler: func(c gofi.Context) error {
        s, err := gofi.ValidateAndBind[processSchema](c)
        if err != nil {
            return err
        }

        // Clone the context safely before leaving the handler chain
        detachedCtx := c.Copy()

        go func(cc gofi.Context) {
            // Safe to access path, method, headers, and datastore
            log.Printf("Background processing for: %s", cc.Path())
        }(detachedCtx)

        return c.Send(202, s.Accepted)
    },
})

type processSchema struct {
    Accepted struct {
        Body struct {
            Status string `json:"status" default:"processing"`
        }
    }
    Err struct {
        Body ErrorResponse
    }
}
```

#### Standard Library context.Context

Use `c.Context()` to obtain a standard library `context.Context`. This is the **recommended, safe entry point** for passing contexts to external libraries (database drivers, HTTP clients, timeouts, etc.).

- On a **live handler context** it delegates to the fasthttp connection context, so cancellation signals (e.g. client disconnect) propagate naturally.
- On a **`c.Copy()`-ed context** it returns a detached `context.Background()`, ensuring async work is never cancelled by the original connection closing.

Because both cases return a non-nil `context.Context`, `c.Context()` works correctly regardless of whether the consuming code runs synchronously or asynchronously:

```go
func(c gofi.Context) error {
    // Safe for both sync calls and goroutines:
    ctx := c.Context()

    // Wrap with a timeout (works on live or copied context)
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    // Synchronous use
    result, err := myDatabase.QueryContext(ctx, "SELECT ...")

    // Async use — copy first, then take its context
    cc := c.Copy()
    go func() {
        bgCtx := cc.Context() // context.Background(), never nil
        myQueue.Push(bgCtx, payload)
    }()

    return c.SendString(200, "ok")
}
```

> **Avoid `c.Request().Context()`** for standard-library context usage. That method returns the raw `*fasthttp.RequestCtx`, which is not a `context.Context`. On a `c.Copy()`-ed context the fasthttp struct is freshly allocated and its Go context is `nil`, causing a panic in anything that calls `Done()`, `Deadline()`, or `Value()` on it.

#### Passing values to domain code

Middleware that needs to hand a value to code below the handler (the
authenticated caller, the tenant, a trace span) derives a context from
`c.Context()` and installs it with `gofi.SetRequestContext`. Every later
`c.Context()` call in the request returns it:

```go
p, err := authn.Authenticate(c.Context(), token)
if err != nil {
    return err
}
if err := gofi.SetRequestContext(c, domain.WithPrincipal(c.Context(), p)); err != nil {
    return err
}
return c.Next()
```

Do not store values with `SetUserValue` on `c.Request().Context()`:
`c.Context()` does not see them. Releases up to v0.8.11 did, so code that
relies on it breaks on upgrade. Values only HTTP-layer code reads can go in
`c.DataStore()` instead. A goroutine that must outlive the request but needs
these values takes `context.WithoutCancel(c.Context())` before the handler
returns; `c.Copy().Context()` carries no values. See
[GUIDELINES.md](GUIDELINES.md#13-context-and-goroutines).

### Middleware

Add global middlewares using `Use()`:

```go
r.Use(func(c gofi.Context) error {
    log.Println("Request received")
    return c.Next()
})
```

### Using net/http Middlewares

If you have existing `net/http` compatible middlewares (e.g., from Chi, Gorilla, or third-party libraries), you can use `gofi.WrapMiddleware` to convert them:

```go
import "github.com/rs/cors"

corsHandler := cors.Default()
r.Use(gofi.WrapMiddleware(corsHandler.Handler))
```

### Route Groups

Group routes with shared middlewares using `Group()`:

```go
r.Group(func(r gofi.Router) {
    r.Use(AuthMiddleware)
    r.Get("/profile", ProfileHandler)
})
```
### Route Grouping & Versioning

The `Route` method allows you to create sub-routers for grouping related endpoints or handling API versioning. This is similar to mounting, but more integrated.

```go
r.Route("/api", func(r gofi.Router) {
    // API v1
    r.Route("/v1", func(r gofi.Router) {
        r.Get("/users", UserListHandler)
        r.Get("/posts", PostListHandler)
    })

    // API v2
    r.Route("/v2", func(r gofi.Router) {
        r.Get("/users", UserListHandlerV2)
    })
})
```

This structure keeps your routing logic clean and hierarchical. Middleware applied within a `Route` block will only affect routes within that block.

## Route Patterns

Gofi supports three kinds of path segments:

| Syntax | Example | Description |
|--------|---------|-------------|
| Static | `/users/me` | Matched exactly. |
| Named parameter | `/users/:id` | Matches any single segment. Bound to the `Request.Path` field tagged `json:"id"`; middleware can read it with `c.Param("id")`. |
| Catch-all | `/files/*filepath` | Matches `/files/` and everything below it (including sub-segments), never `/filesx`. Bound to the `Request.Path` field tagged `json:"filepath"`, without the leading `/`. |

### Match priority

When multiple patterns could match the same request, **static segments always win** over wildcards:

```go
r.Get("/users/:id",  detailHandler)
r.Get("/users/me",   meHandler)    // registration order does not matter

// GET /users/me   → meHandler (static wins)
// GET /users/123  → detailHandler (:id matches)
```

Named parameters (`:name`) take priority over catch-alls (`*name`).

### Trailing-slash redirect

When a request path differs from the registered pattern only by a trailing slash, Gofi issues an automatic redirect (`301`/`308`) and preserves query parameters:

```text
GET /users?page=2  with  /users/  registered  →  301 Location: /users/?page=2
```

For a full reference — including what registration combinations panic at startup — see [docs/routing.md](docs/routing.md).

## Defining Route Options

Routes are defined using `DefineHandler` which takes `RouteOptions`.

### RouteOptions

```go
type RouteOptions struct {
    // OpenAPI Info (Summary, Description, Tags, etc.)
    Info gofi.Info
    // Schema definition struct instance
    Schema any
    // Custom metadata accessible in handlers
    Meta any
    // The handler function
    Handler func(c gofi.Context) error
}
```

## Schema Validations

Gofi validates requests based on struct tags in your schema.

```go
type UserSchema struct {
    Request struct {
        Body struct {
            Email string `json:"email" validate:"required,email"`
            Age   int    `json:"age" validate:"gte=18"`
        }
    }
}
```

Use `gofi.ValidateAndBind` in your handler to perform validation:

```go
s, err := gofi.ValidateAndBind[UserSchema](c)
if err != nil {
    return err // Returns structured validation error
}
```

Gofi supports a wide range of validators (`required`, `min`, `max`, `uuid`, `ip`, etc.) and allows you to define custom validators.

For a complete list of supported validators and a guide on creating custom ones, refer to the [Schema Validations Guide](docs/validations.md).

## Serving OpenAPI Documentation

Gofi can automatically serve OpenAPI 3.0 documentation generated from your schemas.

```go
err := gofi.ServeDocs(r, gofi.DocsOptions{
    Info: gofi.DocsInfoOptions{
        Title:       "My API",
        Version:     "1.0.0",
        Description: "API Documentation",
    },
    Views: []gofi.DocsView{
        {
            RoutePrefix: "/docs/swagger",
            Template:    gofi.SwaggerTemplate(),
            URLMatch: func(path string) bool {
                // Serve only for routes that begin with /api/v1/
                return strings.HasPrefix(path, "/api/v1/")
            },
        },
        {
            RoutePrefix: "/docs/scalar",
            Template:    gofi.ScalarTemplate(&gofi.ScalarConfig{
                Theme: "purple",
            }),
        },
        {
            RoutePrefix: "/docs/redoc",
            Template:    gofi.RedoclyTemplate(),
        },
    },
})
```

Supported UI templates:
- `SwaggerTemplate()`
- `ScalarTemplate(config)`
- `RedoclyTemplate()`
- `RapidDoc()`
- `StopLight()`

### Documenting authentication

OpenAPI ignores `Authorization` header parameters, so declare how clients
authenticate in `DocsOptions`. Every route inherits `Security`; a route that
needs no credentials sets `Info.Security: gofi.NoSecurity`.

```go
gofi.ServeDocs(r, gofi.DocsOptions{
    Info: gofi.DocsInfoOptions{Title: "My API", Version: "1.0.0"},
    SecuritySchemes: map[string]any{
        "BearerAuth": fluid.BearerAuth(fluid.WithSecurityDescription("Access token issued to the caller.")),
    },
    Security: []gofi.SecurityRequirement{{"BearerAuth": nil}},
    Views:    []gofi.DocsView{{RoutePrefix: "/docs", Template: gofi.StopLight()}},
})
```

`ServeDocs` returns an error if a requirement names an undeclared scheme. See
[GUIDELINES.md](GUIDELINES.md#security) for the rules.

### Enriching Docs with Fluid (Typed OpenAPI Components)

Use the [`fluid`](docs/fluid-components.md) companion package to build reusable
OpenAPI components with full type safety — no more raw `map[string]any`:

```go
import "github.com/michaelolof/gofi/fluid"

gofi.DocsComponent{
    Schemas: map[string]any{
        "Error": fluid.ObjectSchema(map[string]fluid.SchemaObject{
            "code":    fluid.IntegerSchema().WithDescription("HTTP status code"),
            "message": fluid.StringSchema().WithDescription("Error message"),
        }).WithRequired("code", "message"),
        "User":  fluid.ObjectSchema(map[string]fluid.SchemaObject{
            "id":   fluid.IntegerSchema(),
            "name": fluid.StringSchema(),
        }).WithRequired("id", "name"),
    },
}
```

Gofi accepts fluid schemas in `DocsComponent.Schemas` and fluid security
schemes in `DocsOptions.SecuritySchemes`; fluid also has typed builders for
the other OpenAPI component types.
See the [Fluid guide](docs/fluid-components.md) for the complete reference.

### Custom Documentation UI

You can serve your own custom documentation UI by implementing the `DocsUiTemplate` interface:

```go
type MyCustomDocs struct {}

func (m *MyCustomDocs) HTML(specPath string) []byte {
    return []byte(fmt.Sprintf(`
        <html>
            <body>
                <h1>My Docs</h1>
                <script>initDocs("%s")</script>
            </body>
        </html>
    `, specPath))
}

// Usage
gofi.ServeDocs(r, gofi.DocsOptions{
    Views: []gofi.DocsView{
        {
            RoutePrefix: "/docs",
            Template:    &MyCustomDocs{},
        },
    },
})
```

### Exporting OpenAPI Specification

If you need to programmatically access the OpenAPI specification to generate a static file for CI/CD, feed code generators (like `oapi-codegen`), or assert against it in tests, you can use `gofi.OpenAPISpec()`.

This function extracts the `gofi.Docs` struct directly from your router without needing to start the server:

```go
func main() {
    r := gofi.NewRouter()
    // ... register all your routes and middlewares ...

    // Extract the full OpenAPI 3.0.3 specification
    opts := gofi.DocsOptions{
        Info: gofi.DocsInfoOptions{Title: "My API", Version: "1.0"},
    }
    masterSpec := gofi.OpenAPISpec(r, opts)

    // Marshal to JSON
    bytes, _ := json.MarshalIndent(masterSpec, "", "  ")

    // Write to file
    os.WriteFile("openapi.json", bytes, 0644)
}
```

#### Slicing Documentation (Filtering)

When you have multiple documentation views configured via `gofi.DocsOptions.Views` (e.g. one for internal admin panels and one for public clients), you may want to export those restricted subsets to JSON as well. 

The `gofi.Docs` type provides native filtering methods to extract slices:

**Option 1: Extract by View Route Prefix (Recommended)**
Extract the exact specification that `gofi.ServeDocs` would have served for a given view (automatically applies its `URLMatch` rules and component scoping).
```go
opts := gofi.DocsOptions{
    Views: []gofi.DocsView{
        { RoutePrefix: "/docs/admin", URLMatch: func(p string) bool { return strings.HasPrefix(p, "/admin") } },
        { RoutePrefix: "/docs/public" },
    },
}
masterSpec := gofi.OpenAPISpec(r, opts)

// Pull out the spec dynamically bound to the /docs/admin view rules
adminSpec := masterSpec.FilterByRoutePrefix("/docs/admin")
```

**Option 2: Filter by URL Prefix**
```go
publicSpec := masterSpec.FilterByURL("/public")
```

**Option 3: Custom Callback Filtering**
```go
customSpec := masterSpec.Filter(func(path string) bool {
    return !strings.HasPrefix(path, "/legacy")
})
```

## Handling Form Data and File Uploads

Gofi supports `application/x-www-form-urlencoded` and `multipart/form-data` requests out of the box.

### Form Data
Define your schema using standard struct tags. Gofi will automatically parse the form data into your struct.

```go
type LoginSchema struct {
    Request struct {
        Body struct {
            Username string `json:"username" validate:"required"`
            Password string `json:"password" validate:"required"`
        }
    }
}
```

### File Uploads
For multipart file uploads, use `*multipart.FileHeader` (or `[]*multipart.FileHeader` for multiple files) in your schema.

```go
type UploadSchema struct {
    Request struct {
        Body struct {
            Title string                `json:"title"`
            File  *multipart.FileHeader `json:"file" validate:"required"`
            Docs  []*multipart.FileHeader `json:"docs"`
        }
    }
}
```

## Serving Static Files

You can serve static files from a directory using the `Static` method:

```go
// Serves files from "./public" directory at "/assets" route
// e.g. GET /assets/style.css -> ./public/style.css
r.Static("/assets", "./public")
```

## Graceful Shutdown

Gofi seamlessly integrates with `valyala/fasthttp`'s shutdown primitives, ensuring active network connections finish processing before terminating the server.

```go
func main() {
    r := gofi.NewRouter()
    
    // Start server in a background goroutine
    go func() {
        if err := r.Listen(":8080"); err != nil {
            log.Fatalf("Server error: %v", err)
        }
    }()

    // Wait for interrupt signal to gracefully shutdown the server
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
    <-quit
    log.Println("Shutting down server...")

    // The shutdown blocks until all active requests have completed
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    if err := r.ShutdownWithContext(ctx); err != nil {
        log.Fatalf("Server forced to shutdown: %v", err)
    }

    log.Println("Server exiting")
}
```

## Streaming (Server-Sent Events)

Gofi provides an ergonomic `SendStream` helper that takes the status code and schema definition, validates the response headers, and streams each flushed chunk to the client as the callback writes it.

Use streaming when:

- the server is pushing updates in one direction
- the client does not need to send live frames back
- SSE semantics are sufficient
- you want a simpler HTTP-native real-time transport

```go
// StreamHandler sends ten events, one per second.
var StreamHandler = gofi.DefineHandler(gofi.RouteOptions{
    Info: gofi.Info{
        Tags:        []string{"events"},
        Description: "Streams ten server-sent events.",
    },

    Schema: &streamSchema{},

    Handler: func(c gofi.Context) error {
        var s streamSchema
        return c.SendStream(200, s.Ok, func(w *bufio.Writer) error {
            for i := 0; i < 10; i++ {
                if _, err := fmt.Fprintf(w, "data: Chunk %d\n\n", i+1); err != nil {
                    return err
                }
                if err := w.Flush(); err != nil {
                    return err
                }
                time.Sleep(1 * time.Second)
            }
            return nil
        })
    },
})

type streamSchema struct {
    Ok struct {
        Header struct {
            ContentType string `json:"content-type" default:"text/event-stream"`
        }
        Body string `validate:"required"`
    }
}

r.Get("/stream", StreamHandler)
```

This ensures the status code and schema are set before streaming begins. By default, if you do not set a status code, the default (usually 200 OK) is used.

Operational notes:

- `SendStream` validates response headers and cookies before the stream body starts.
- The stream writer is responsible for writing event frames and calling `Flush()`.
- `SendStream` returns as soon as the stream is registered; the callback runs on its own goroutine after the handler returns. The context, path parameters and bound request data stay valid for the callback until it returns.
- Write and disconnect errors should be returned from the callback. Register `gofi.OnStreamError(c, fn)` before `SendStream` to receive them; otherwise they are logged.
- Streamed responses bypass the `Cache` and `ETag` middleware and are rejected under `Timeout`.
- For SSE, the response schema should normally declare `content-type: text/event-stream`.

Recommended build order for a streaming route:

- define the streaming response schema for the status code you plan to send
- set `content-type: text/event-stream` on the response schema when building SSE
- call `SendStream(code, s.Ok, callback)` from the handler, passing the response block for that status
- write valid SSE frames such as `data: ...\n\n`
- flush after each event or batch that should reach the client immediately
- return callback errors so disconnects and write failures are visible

## WebSockets

Gofi provides context-aware websocket handlers, handshake validation, JSON message helpers, lifecycle hooks, and active-session draining for production workloads.

If you are new to WebSockets or want the full explanation of how they fit into Gofi's router model, read the dedicated guide: [WebSockets Guide](docs/websockets.md).

That guide covers:

- When to use WebSockets instead of HTTP or SSE
- How upgrade requests work in Gofi
- How to define websocket routes with `websocket.DefineWebSocket(...)`
- What `Session`, `Options`, `Hooks`, and `SessionRegistry` do
- How handshake validation, message validation, and graceful shutdown work
- How browser clients connect and exchange messages

There is also a runnable companion example in the examples repository at `gofi-test-utils/cmd/examples/websockets/main.go`.

Recommended build order for a websocket route:

- define handshake input in `Schema.Request`
- define message contracts in `Schema.WebSocket`
- choose `HandshakeAuto`, `HandshakeSelective`, or `HandshakeOff`
- implement the session using `ReadJSON`, `WriteJSON`, and `WriteError` when the protocol is JSON-based
- add limits, timeouts, hooks, and optionally a `SessionRegistry` before shipping to production

```go
import (
    "context"
    "log"
    "time"

    "github.com/michaelolof/gofi"
    "github.com/michaelolof/gofi/websocket"
)

var registry = websocket.NewSessionRegistry()

// ChatHandler echoes every message back to the room.
var ChatHandler = websocket.DefineWebSocket(websocket.WebSocketOptions{
    Info: gofi.Info{
        Tags:        []string{"chat"},
        Description: "Joins a chat room over WebSocket.",
    },

    Handshake: websocket.HandshakePolicy{Mode: websocket.HandshakeAuto},
    Handler: func(s *websocket.Session) error {
        roomID := s.Context().Param("room_id")
        for {
            mt, msg, err := s.ReadMessage()
            if err != nil {
                return nil
            }
            if err := s.WriteMessage(mt, []byte("["+roomID+"] "+string(msg))); err != nil {
                return err
            }
        }
    },
    Runtime: websocket.RuntimeOptions{
        Registry:        registry,
        MaxMessageBytes: 1 << 20,
        ReadTimeout:     30 * time.Second,
        WriteTimeout:    10 * time.Second,
        Hooks: websocket.Hooks{
            OnUpgradeError: func(ctx gofi.Context, err error) {
                log.Printf("upgrade failed path=%s err=%v", ctx.Path(), err)
            },
            OnSessionError: func(ctx gofi.Context, err error) {
                log.Printf("session error room=%s err=%v", ctx.Param("room_id"), err)
            },
        },
    },
})

r.Get("/ws/:room_id", ChatHandler)

// On shutdown, drain active sessions before router shutdown.
drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
_ = registry.DrainContext(drainCtx)
_ = r.Shutdown()
```

Handshake validation is configured at the route layer with `websocket.HandshakePolicy{Mode: websocket.HandshakeAuto}` or `websocket.HandshakePolicy{Mode: websocket.HandshakeSelective, Selectors: ...}`.

`websocket.DefineWebSocket(...)` is the documented public way to define websocket route options. For the full walkthrough, API reference, and production guidance, see [docs/websockets.md](docs/websockets.md).

Use WebSockets when the client and server must both send messages at any time. If the server is only pushing one-way updates, Gofi streaming is often a simpler fit.

At a high level:

- use streaming for outbound event feeds and SSE
- use WebSockets for bidirectional session protocols

## Testing

Test handlers by sending requests through the full router with `Test`. Build
the router the same way production does, with test dependencies, so a test
covers routing, middleware, validation, the handler and the error handler
together. No listener is started.

```go
func TestGetProjectHandler_ReturnsCallersProject(t *testing.T) {
    h := testharness.New(t) // builds the real router with in-memory deps
    project, err := h.Deps.ProjectStore.Create(context.Background(), domain.Project{Name: "Payments", OwnerID: "alice"})
    require.NoError(t, err)

    resp, err := h.Test(gofi.TestOptions{
        Method:  "GET",
        Path:    "/api/v1/projects/:project_id",
        Paths:   map[string]string{"project_id": project.ID},
        Headers: testharness.AuthHeaders("token-alice"),
    })
    require.NoError(t, err)
    require.Equal(t, 200, resp.Code(), resp.BodyString())

    var out schemas.SuccessBody[schemas.Project]
    require.NoError(t, resp.BindBody(&out))
    assert.Equal(t, project.ID, out.Data.ID)
}
```

[GUIDELINES.md](GUIDELINES.md#11-testing) describes the test harness and the
testing rules; the reference service has a complete one.

### The `Inject` Method

`Inject` runs one `RouteOptions` without registering it, skipping routing and
route-group middleware. Use it to test a schema or handler in isolation.

```go
func TestPingSchema_RejectsShortMessage(t *testing.T) {
    r := gofi.NewRouter()

    resp, err := r.Inject(gofi.InjectOptions{
        Method:  "POST",
        Path:    "/ping",
        Handler: &PingHandler, // no need to register it
        Body:    strings.NewReader(`{"message": "hi"}`),
    })
    require.NoError(t, err)
    assert.Equal(t, 400, resp.Code())
}
```

You can also pass headers, query params, cookies, and a body:

```go
resp, err := r.Test(gofi.TestOptions{
    Method:  "POST",
    Path:    "/users",
    Headers: map[string]string{"Authorization": "Bearer token"},
    Query:   map[string]string{"page": "1"},
    Body:    strings.NewReader(`{"name": "Alice"}`),
})
```

If the handler panics, `Test` returns the same generic `500` response a live server sends, alongside a non-nil error describing the panic.

If the request cannot be parsed (for example, a header beyond the server's limits), `Test` and `Inject` return an error and the handler does not run, matching a live server that rejects the request.

### TestOptions, InjectOptions & InjectResponse

```go
// TestOptions is used with r.Test() — dispatches through the full route tree.
type TestOptions struct {
    Path    string            // Request path
    Method  string            // HTTP method (required)
    Query   map[string]string // Query parameters
    Paths   map[string]string // Path parameter overrides (e.g. {"id": "123"})
    Headers map[string]string // Request headers
    Cookies []http.Cookie     // Request cookies
    Body    io.Reader         // Request body
}

// InjectOptions is used with r.Inject() — bypasses the route tree and calls the handler directly.
type InjectOptions struct {
    Path    string              // Request Path
    Method  string              // HTTP Method
    Query   map[string]string   // Query Params
    Paths   map[string]string   // Path Params (e.g. {"id": "123"})
    Headers map[string]string   // Headers
    Cookies []http.Cookie       // Cookies
    Body    io.Reader           // Request Body
    Handler *RouteOptions       // The Handler definition to test (required)
}

type InjectResponse struct {
    StatusCode int
    HeaderMap  http.Header
    Body       []byte
}

// Helper methods on InjectResponse:
func (r *InjectResponse) BodyString() string          // Body as a string
func (r *InjectResponse) BindBody(v any) error        // Unmarshal JSON body into v
func (r *InjectResponse) BodyJSON() map[string]any    // Unmarshal body into a map (nil on error)
func (r *InjectResponse) Header(key string) string    // Get a response header value
func (r *InjectResponse) HasHeader(key string) bool   // Check if a header is present
func (r *InjectResponse) CookieValue(name string) string // Get a Set-Cookie value by name
func (r *InjectResponse) Cookies() []*http.Cookie     // Parse all Set-Cookie headers
```

## Custom Types/Specs
Gofi schema supports the basic GoLang types for encoding and decoding (e.g struct, array, int, string etc.) as well as `time.Time` and `http.Cookie`
To support aditional types in your Gofi Schema (e.g google's `uuid.UUID`), you can define them as custom `specs` and register them like below

### Custom Specs (Vendor Types)
You can define custom encoding/decoding behavior for specific types, useful for vendor types or custom scalars.

1. Implement `CustomSpec` or use `DefineCustomSpec`.
2. Register it with the router.

```go
type MyCustomID string

// Define the spec
var customIDSpec = gofi.DefineCustomSpec(gofi.SpecDefinition{
    SpecID: "custom-id",
    Type:   "string",
    Format: "uuid", // OpenAPI format
    Encode: func(val any) (string, error) {
        return string(val.(MyCustomID)), nil
    },
    Decode: func(val any) (any, error) {
        return MyCustomID(val.(string)), nil
    },
})

// Register
r.RegisterSpec(customIDSpec)

// Use in schema
type Schema struct {
    Request struct {
        Body struct {
            ID MyCustomID `json:"id" spec:"custom-id"` // You need to add the spec struct tag to notify gofi that this is a custom spec
        }
    }
}
```



## Decoding Requests and Encoding Responses
Incoming requests are decoded when you call the `gofi.ValidateAndBind[T](c gofi.Context) error` method and Outgoing responses are encoded when you call the `c.Send(code int, obj any) error` method.

These methods rely on the ContentType defined in the Header of the Schema to determine which BodyParser to use. The Gofi library comes already with a built-in JSON BodyParser

You can implement the `BodyParser` interface to handle different content types (e.g., XML, YAML, MsgPack) and register it.

```go
type MyXMLParser struct {}

func (p *MyXMLParser) Match(contentType string) bool {
    return contentType == "application/xml"
}

func (p *MyXMLParser) ValidateAndDecodeRequest(r io.ReadCloser, opts gofi.RequestOptions) error {
    // Implement decoding logic
    return nil
}

func (p *MyXMLParser) ValidateAndEncodeResponse(s any, opts gofi.ResponseOptions) ([]byte, error) {
    // Implement encoding logic
    return nil, nil
}

// Register
r.RegisterBodyParser(&MyXMLParser{})
```

## Benchmarks

Gofi has been heavily optimized around `fasthttp` to provide maximum throughput and zero-allocation critical paths where possible, dominating benchmark results across micro-benchmarks, real-world API traversals, middleware chains, and concurrency scaling.

### HTTP Load Test Results (`bombardier`, 125 concurrent connections, 5s)

Gofi is benchmarked against [Chi](https://github.com/go-chi/chi), [Echo](https://github.com/labstack/echo), [Gin](https://github.com/gin-gonic/gin), and [Fiber](https://github.com/gofiber/fiber).

> 📊 Full benchmark suite (including memory allocation profiles) and reproducible runner scripts are available at: **[gofi-test-utils](https://github.com/michaelolof/gofi-test-utils)**
