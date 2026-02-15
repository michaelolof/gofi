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

### Global Error Handler

You can define a custom error handler for all routes using `SetErrorHandler`:

```go
r.SetErrorHandler(func(err error, c gofi.Context) {
    // Custom error handling logic
    log.Printf("Error occurred: %v", err)
    c.Send(http.StatusInternalServerError, map[string]string{
        "error": "Internal Server Error",
        "details": err.Error(),
    })
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

### Mounting Sub-Routers

Mount another `http.Handler` or router under a specific path using `Mount()`:

```go
apiV1 := gofi.NewServeMux()
// ... define v1 routes ...
r.Mount("/v1", apiV1)
```

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

### Defining Schema

Schemas are defined as nested structs representing the HTTP request and response structure.

```go
type UserSchema struct {
    // Request definition
    Request struct {
        // Path parameters (e.g., /users/:id)
        Params struct {
            ID string `json:"id" validate:"required,uuid"`
        }
        // Query parameters (e.g., ?page=1)
        Query struct {
            Page int `json:"page" default:"1"`
        }
        // Headers
        Headers struct {
            Authorization string `json:"Authorization" validate:"required"`
        }
        // Request Body (JSON)
        Body struct {
            Name  string `json:"name" validate:"required"`
            Email string `json:"email" validate:"required,email"`
        }
    }

    // Response definitions mapped by name (Ok, Created, Error, etc.)
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

### Validation Tags

Gofi uses an internal validator for structs that is mostly compatible with the [go-playground/validator](https://github.com/go-playground/validator) library.

Supported validators include:
- `required`
- `min`, `max`, `len`
- `email`, `uuid`
- `oneof`
- And many more standard validations.

Example: `validate:"required,min=5,max=20"`

## Validating and Binding Requests

Inside your handler, use `ValidateAndBind` to validate the request against your schema and bind the data to a typed struct.

```go
s, err := gofi.ValidateAndBind[UserSchema](c)
if err != nil {
    // Automatically handles validation errors
    return err
}

// Access validated data
userID := s.Request.Params.ID
page := s.Request.Query.Page
```

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

## Advanced Usage

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
r.RegisterSpecs(customIDSpec)

// Use in schema
type Schema struct {
    Request struct {
        Body struct {
            ID MyCustomID `json:"id" spec:"custom-id"` // You need to add the spec struct tag to notify gofi that this is a custom spec
        }
    }
}
```

### Custom Validators

Add your own validation logic for specific tags.

```go
r.SetCustomValidator(map[string]func(c gofi.ValidatorContext) func(arg any) error{
    "is-cool": func(c gofi.ValidatorContext) func(arg any) error {
        return func(arg any) error {
            s, ok := arg.(string)
            if !ok || s != "cool" {
                return errors.New("must be cool")
            }
            return nil
        }
    },
})

// Use in schema: `validate:"is-cool"`
```

### Custom Body Parsers

Implement the `BodyParser` interface to handle different content types (e.g., XML, YAML, MsgPack).

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