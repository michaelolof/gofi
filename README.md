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
    Schema: UserListSchema{},
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
    Middlewares []gofi.MiddlewareFunc
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