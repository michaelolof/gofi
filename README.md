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

## Examples

You can find examples of using a Gofi router [here](https://github.com/michaelolof/gofi-test-utils/tree/7a9bf615e9328d7ee30511bac6ce50677f6be274/cmd/examples) 

## Quick Start

Here is a minimal example to get you started with Gofi.

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/michaelolof/gofi"
)

// Define your request/response schema
type PingSchema struct {
	Request struct {
		Body struct {
			Message string `json:"message" validate:"required,min=5"`
		}
	}

	Ok struct {
		Body struct {
			Reply     string `json:"reply"`
			Timestamp int64  `json:"timestamp"`
		}
	}
}

func main() {
	// Initialize the router
	r := gofi.NewRouter()

	// Define the handler
	pingHandler := gofi.DefineHandler(gofi.RouteOptions{
		Schema: &PingSchema{},
		Handler: func(c gofi.Context) error {
			// Validate request and bind data to schema
			s, err := gofi.ValidateAndBind[PingSchema](c)
			if err != nil {
				return err
			}

			// Access validated data directly
			msg := s.Request.Body.Message
			fmt.Printf("Received message: %s\n", msg)

			// Populate response
			s.Ok.Body.Reply = "Pong: " + msg
			s.Ok.Body.Timestamp = time.Now().Unix()

			// Send response
			return c.Send(http.StatusOK, s.Ok)
		},
	})

	// Register the route
	r.POST("/ping", pingHandler)

	// Serve Documentation
	gofi.ServeDocs(r, gofi.DocsOptions{
		Views: []gofi.DocsView{
			{RoutePrefix: "/docs", Template: gofi.StopLight()},
		},
	})

	log.Println("Server listening on :8080")
	r.Listen(":8080")
}
```

## Router Setup

### Initialization

Create a new router instance using `NewServeMux()`:

```go
r := gofi.NewRouter()
```

### Defining a Route Schema

Schemas are defined as nested structs representing the HTTP request and response structure.

```go
type UserSchema struct {
    // Request definition
    Request struct {
        // Path parameters (e.g., /users/:id)
        Path struct {
            ID string `json:"id" validate:"required,uuid"`
        }
        // Query parameters (e.g., ?page=1)
        Query struct {
            Page int `json:"page" default:"1"`
        }
        // Headers
        Header struct {
            Authorization string `json:"Authorization" validate:"required"`
        }
        // Request Body (JSON)
        Body struct {
            Name  string `json:"name" validate:"required"`
            Email string `json:"email" validate:"required,email"`
        }
    }

    // Response definitions mapped by name (Ok, Created, Err, etc.)
    Ok struct { // 200 OK
        Body UserResponse `json:"body"`
    }
    NotFound struct { // 404 Not Found
        Body ErrorResponse `json:"body"`
    }
    // Generic error response for 400-599 status codes not implicitly matched
    Err struct {
         Body ErrorResponse `json:"body"`
    }
}
```

For a detailed guide on defining schemas, supported fields, response types and validation, please refer to the [Schema Guide](docs/schema-info.md).

### Defining a Route Handler

The `RouteOptions` struct is used to configure your route, including metadata, schema, middlewares, and the handler function itself.

```go
var UsersHandler = gofi.DefineHandler(gofi.RouteOptions{
    Info: gofi.Info{ Description: "Returns a list Users" },
    Schema: &UserListSchema{},
    Handler: func(c gofi.Context) error {
        // ... implementation ...
        return c.Send(200, response)
    },
})

r.Get("/users", UsersHandler)
```

For a comprehensive guide on Route Handlers, Context methods, and RouteOptions configuration, please refer to the [Route Options Guide](docs/route-options.md).


### Global Error Handler

You can define a custom error handler for all routes using `UseErrorHandler`:

```go
r.UseErrorHandler(func(err error, c gofi.Context) {
    // Custom error handling logic
    log.Printf("Error occurred: %v", err)
    c.Send(http.StatusInternalServerError, map[string]string{
        "error": "Internal Server Error",
        "details": err.Error(),
    })
})
```

### Plugins

You can attach shared state or plugins to the router using the `GlobalStore`, which is accessible in all route handlers.

```go
// 1. Initialize plugin
myDB := NewDatabase()

// 2. Register in GlobalStore
r.GlobalStore().Set("db", myDB)

// 3. Access in Handler
gofi.DefineHandler(gofi.RouteOptions{
    Handler: func(c gofi.Context) error {
        // Retrieve from GlobalStore (read-only access)
        if db, found := c.GlobalStore().Get("db"); found {
            // Use the plugin
            db.(*Database).Query("...")
        }
        return nil
    },
})
```

### Goroutines and Concurrency

Gofi heavily utilizes `sync.Pool` under the hood to ensure zero-allocation routing. Because of this, the `gofi.Context` object is **immediately recycled** back to the framework as soon as your HTTP handler returns. 

If you are passing `gofi.Context` to other functions where you **cannot strictly guarantee** they won't spawn background goroutines or outlive the handler scope, you **cannot** pass the original context to them. Doing so will result in a data race and memory corruption when a subsequent request overwrites your context memory pools.

Instead, you should proactively use `c.Copy()` to safely detach and clone the necessary request paths, parameters, datastores, and headers:

```go
r.GET("/process", gofi.DefineHandler(gofi.RouteOptions{
    Handler: func(c gofi.Context) error {
        // Clone the context safely before leaving the handler chain
        detachedCtx := c.Copy()

        go func(cc gofi.Context) {
            // Safe to access path, method, headers, and datastore
            log.Printf("Background processing for: %s", cc.Path())
        }(detachedCtx)

        return c.SendString(202, "Processing in background")
    },
}))
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
    r.GET("/profile", ProfileHandler)
})
```
### Route Grouping & Versioning

The `Route` method allows you to create sub-routers for grouping related endpoints or handling API versioning. This is similar to mounting, but more integrated.

```go
r.Route("/api", func(r gofi.Router) {
    // API v1
    r.Route("/v1", func(r gofi.Router) {
        r.GET("/users", UserListHandler)
        r.GET("/posts", PostListHandler)
    })

    // API v2
    r.Route("/v2", func(r gofi.Router) {
        r.GET("/users", UserListHandlerV2)
    })
})
```

This structure keeps your routing logic clean and hierarchical. Middleware applied within a `Route` block will only affect routes within that block.

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
            Match: func(path string) bool {
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
    r := gofi.New()
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

Gofi provides an ergonomic `SendStream` helper that takes the status code, schema definition and securely propagates network disconnects up to the global HTTP error handler.

Use streaming when:

- the server is pushing updates in one direction
- the client does not need to send live frames back
- SSE semantics are sufficient
- you want a simpler HTTP-native real-time transport

```go
// Define the schema for the streaming response
type streamSchema struct {
    Ok struct {
        Header struct {
            ContentType string `json:"content-type" default:"text/event-stream"`
        }
        Body string `validate:"required" pattern:"event-stream"`
    }
}

r.GET("/stream", gofi.DefineHandler(gofi.RouteOptions{
    Schema: &streamSchema{},
    Handler: func(c gofi.Context) error {
        var s streamSchema
        return c.SendStream(200, s, func(w *bufio.Writer) error {
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
}))
```

This ensures the status code and schema are set before streaming begins. By default, if you do not set a status code, the default (usually 200 OK) is used.

Operational notes:

- `SendStream` validates response headers and cookies before the stream body starts.
- The stream writer is responsible for writing event frames and calling `Flush()`.
- Write and disconnect errors should be returned from the callback so the caller can observe stream failure.
- For SSE, the response schema should normally declare `content-type: text/event-stream`.

Recommended build order for a streaming route:

- define the streaming response schema for the status code you plan to send
- set `content-type: text/event-stream` on the response schema when building SSE
- call `SendStream(code, schemaValue, callback)` from the handler
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

registry := websocket.NewSessionRegistry()

r.Get("/ws/:room_id", websocket.DefineWebSocket(websocket.WebSocketOptions{
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
}))

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

## Unit Testing

Gofi provides a convenient way to unit test your handlers without starting a full HTTP server using the `Inject` method.

### The `Inject` Method

The `Inject` method allows you to simulate an HTTP request against your router and returns an `*InjectResponse`.

It is designed to test handlers in isolation. You pass the `RouteOptions` directly to `Inject`, so you don't even need to register the route on the mux to test it.

```go
func TestMyHandler(t *testing.T) {
    // Initialize a router to provide the environment (stores, validation engine)
    r := gofi.NewRouter()

    // 1. Define your handler options
    myHandlerOpts := gofi.DefineHandler(gofi.RouteOptions{
        Schema: &MySchema{},
        Handler: func(c gofi.Context) error {
            return c.SendString(200, "success")
        },
    })

    // 2. Use Inject to test
    // Returns *InjectResponse
    resp, err := r.Inject(gofi.InjectOptions{
        Method: "GET",
        Path:   "/test-path",
        Handler: &myHandlerOpts, // Pass the RouteOptions directly (no need to register)
        
        // Optional inputs:
        Query:   map[string]string{"foo": "bar"},
        Headers: map[string]string{"Authorization": "Bearer token"},
        Body:    strings.NewReader(`{"name": "test"}`),
    })

    if err != nil {
        t.Fatalf("Inject failed: %v", err)
    }

    // 3. Assert results
    if resp.StatusCode != 200 {
        t.Errorf("Expected 200, got %d", resp.StatusCode)
    }
}
```

### Lightweight Testing with `Test()`

For quick tests on registered routes, use the `Test()` shorthand. Unlike `Inject`, `Test` dispatches through the full registered route tree (middleware, 404 handling, redirects) and returns both a response and an error.

```go
func TestPing(t *testing.T) {
    r := gofi.NewRouter()
    r.Get("/ping", gofi.RouteOptions{
        Handler: func(c gofi.Context) error {
            return c.SendString(200, "pong")
        },
    })

    resp, err := r.Test(gofi.TestOptions{
        Method: "GET",
        Path:   "/ping",
    })
    assert.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode)
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

If the handler panics, `Test` recovers, invokes the configured error handler, and returns a `500` response alongside a non-nil error.

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
