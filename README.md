# gofi

Gofi is an openapi3 schema-based HTTP router for Golang.

## Features

- **Schema-Based Routing**: Define routes with type-safe schemas using Go structs.
- **Automatic Validation**: Request and response validation based on your schema definitions.
- **Fast Performance**: Designed to be performant with `fastjson` and optimized reflection logic.
- **Developer Friendly**: Simple, intuitive API for defining routes and handlers.
- **OpenAPI Documentation**: Automatic API documentation generation with support for multiple UI providers (StopLight, Swagger, RapidDoc, Redocly, Scalar).
- **Customizable**: Add custom validators, body parsers, and type specifications.
- **Error Handling**: Built-in error handling with customizable handlers.
- **Error Handling**: Built-in error handling with customizable handlers.
- **Middleware Support**: Easy integration with standard `http.Handler` middlewares.


## Installation

```sh
go get -u github.com/michaelolof/gofi
```

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
	r := gofi.NewServeMux()

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
	http.ListenAndServe(":8080", r)
}
```

## Router Setup

### Initialization

Create a new router instance using `NewServeMux()`:

```go
r := gofi.NewServeMux()
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

### Middleware

Add global middlewares using `Use()`:

```go
r.Use(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Println("Request received")
        next.ServeHTTP(w, r)
    })
})
```

### PreHandlers (Context-Aware Middleware)

Add global Gofi middleware using `UsePreHandler()` for logic that needs access to `gofi.Context`:

```go
r.UsePreHandler(func(next gofi.HandlerFunc) gofi.HandlerFunc {
    return func(c gofi.Context) error {
        // Access context methods
        token := c.Header().Get("Authorization")
        if token == "" {
            return c.Send(401, map[string]string{"error": "Unauthorized"})
        }
        return next(c)
    }
})
```

For a detailed comparison between Standard Middleware and PreHandlers, see the [Middleware Guide](docs/middleware.md).

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
    // Route-specific middlewares
    PreHandlers []gofi.PreHandler
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
            RoutePrefix: "/custom-docs",
            Template:    &MyCustomDocs{},
        },
    },
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

## Unit Testing

Gofi provides a convenient way to unit test your handlers without starting a full HTTP server using the `Inject` method.

### The `Inject` Method

The `Inject` method allows you to simulate an HTTP request against your router and returns a standard `httptest.ResponseRecorder`.

It is designed to test handlers in isolation. You pass the `RouteOptions` directly to `Inject`, so you don't even need to register the route on the mux to test it.

```go
func TestMyHandler(t *testing.T) {
    // Initialize a router to provide the environment (stores, validation engine)
    r := gofi.NewServeMux()

    // 1. Define your handler options
    myHandlerOpts := gofi.DefineHandler(gofi.RouteOptions{
        Schema: &MySchema{},
        Handler: func(c gofi.Context) error {
            return c.SendString(200, "success")
        },
    })

    // 2. Use Inject to test
    // Returns *httptest.ResponseRecorder
    w, err := r.Inject(gofi.InjectOptions{
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
    if w.Code != 200 {
        t.Errorf("Expected 200, got %d", w.Code)
    }
    if w.Body.String() != "\"success\"" {
        t.Errorf("Unexpected body: %s", w.Body.String())
    }
}
```

### InjectOptions

```go
type InjectOptions struct {
    Path    string              // Request Path
    Method  string              // HTTP Method
    Query   map[string]string   // Query Params
    Paths   map[string]string   // Path Params (e.g. {"id": "123"})
    Headers map[string]string   // Headers
    Cookies []http.Cookie       // Cookies
    Body    io.Reader           // Request Body
    Handler *RouteOptions       // The Handler definition to test
}
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

Gofi is benchmarked against [go-chi/chi](https://github.com/go-chi/chi) and [labstack/echo](https://github.com/labstack/echo) across micro-benchmarks, real-world API traversals, middleware scalability, and concurrency.

> Full benchmark suite and reproducible results: **[gofi-test-utils](https://github.com/michaelolof/gofi-test-utils)**
