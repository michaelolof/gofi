# Route Options & Handlers

This guide details how to define route handlers and configure them using `RouteOptions`.

## Route Handler

A route handler in Gofi is a function that accepts a `Context` and returns an `error`.

```go
type HandlerFunc func(c gofi.Context) error
```

### The Context Interface

The `Context` interface provides methods to interact with the HTTP request and response, access stores, and utilities.

- **Writer() ResponseWriter**: Returns a `ResponseWriter` for backward compatibility.
- **Request() \*Request**: Returns a `Request` adapter for backward compatibility.
- **GlobalStore() ReadOnlyStore**: Access the global store defined on the server router instance.
- **DataStore() GofiStore**: Access the route context data store for passing and retrieving data during a request lifetime.
- **Meta() ContextMeta**: Access static meta information defined on the route.
- **GetSchemaRules(pattern, method string) any**: Retrieves schema rules for a given pattern and method.
- **Next() error**: Calls the next handler in the middleware chain.
- **Param(name string) string**: Returns the named path parameter value.
- **Query(name string) string**: Returns the query parameter value.
- **HeaderVal(name string) string**: Returns the request header value.
- **HeaderBytes(name string) []byte**: Returns the request header value as raw bytes.
- **Body() []byte**: Returns the raw request body bytes.
- **Path() string**: Returns the request URL path.
- **Pattern() string**: Returns the registered route pattern that matched the request.
- **Method() string**: Returns the HTTP method.
- **QueryBytes(name string) []byte**: Returns the query parameter value as raw bytes.
- **Copy() Context**: Creates a deep copy of the `Context` that is safe to use in a background goroutine.
- **Send(code int, obj any) error**: Sends a schema response object for the given status code.
- **SendString(code int, s string) error**: Sends a string response.
- **SendBytes(code int, b []byte) error**: Sends a byte slice response.
- **SendStream(code int, obj any, sw func(w \*bufio.Writer) error) error**: SendStream simplifies SSE. Sets headers based on schema definition and takes over the connection.
- **SetBodyStreamWriter(sw func(w \*bufio.Writer) error) error**: Sets a chunked stream writer for the response body.

## Handler Incoming Requests and Outgoing Responses

The core usage of a handler is to process an incoming request, validate it, perform business logic, and return a response. Gofi streamlines this using the Schema pattern.

### 1. Processing Incoming Requests

To parse and validate an incoming request, use the generic `gofi.ValidateAndBind[T](c)` function inside your handler. `T` should be your route's schema struct.

```go
func(c gofi.Context) error {
    // 1. Validate and Bind
    // This reads the Body, Query, Headers, Path, and Cookies,
    // validates them against the struct tags, and returns a populated struct.
    s, err := gofi.ValidateAndBind[UserSchema](c)
    if err != nil {
        // If validation fails, Gofi returns a structured error.
        // You can return it directly to let the global error handler manage it.
        return err
    }

    // 2. Access Data
    // Data is now type-safe and validated.
    userID := s.Request.Path.UserID
    limit := s.Request.Query.Limit

    // ... business logic ...
}
```

### 2. Sending Outgoing Responses

To send a response, use the `c.Send(code, obj)` method. The `obj` argument should correspond to the specific response struct defined in your schema for that status code.

You often reuse the same schema struct instance returned by `ValidateAndBind` to populate the response, but you can also create a new instance.

```go
func(c gofi.Context) error {
    s, err := gofi.ValidateAndBind[UserSchema](c)
    if err != nil {
        return err
    }

    // ... logic to find user ...
    user := findUser(s.Request.Path.UserID)

    // 1. Populate the response struct
    // 'Ok' corresponds to the 200 OK field in UserSchema
    s.Ok.Body = user
    s.Ok.Header.LastModified = time.Now().String()

    // 2. Send the response
    // Pass the status code and the specific schema field for that code.
    // Gofi validates the Ok response object against the schema before sending.
    return c.Send(200, s.Ok)
}
```

## RouteOptions

`RouteOptions` is the configuration struct used when registering a route. It separates the handler logic from configuration metadata.

It is recommended to use `gofi.DefineHandler` to define your route options variables. This provides a clean way to organize your handlers.

```go
var GetUser = gofi.DefineHandler(gofi.RouteOptions{
    Info: gofi.Info{
        Summary: "Get User",
        Description: "Returns a user by ID",
    },
    Schema: &UserSchema{},
    Handler: func(c gofi.Context) error {
        return c.Send(200, UserResponse{})
    },
})

// Registering the handler
r.Get("/users/{id}", GetUser)
```

### 1. Route Information (`Info`)
The `Info` struct provides metadata primarily used for generating OpenAPI documentation.

```go
gofi.Info{
    Summary:     "Get User",
    Description: "Retrieves a user by their ID",
    OperationId: "getUserById",
    Deprecated:  false,
    Hidden:      false, // If true, hides from documentation
}
```

### 2. Schema (`Schema`)
Defines the request and response structure. See the [Schema Guide](schema-info.md) for full details.

### 3. Meta (`Meta`)
The `Meta` field allows you to attach static, arbitrary data to a route. This data is accessible inside the handler via `c.Meta()`. This is useful for things like permission scopes, feature flags, or any static configuration.

```go
var AdminRoute = gofi.DefineHandler(gofi.RouteOptions{
    Meta: map[string]any{
        "scope": "admin.read",
    },
    Handler: func(c gofi.Context) error {
        scope := c.Meta().Get("scope")
        // ...
        return nil
    },
})
```

### 4. Handler (`Handler`)
The main business logic for the route.

```go
var MyHandler = gofi.DefineHandler(gofi.RouteOptions{
    Handler: func(c gofi.Context) error {
        return c.SendString(200, "Hello World")
    },
})
```

### Selective Validation and Binding

You can now specify which components to validate or bind using `Validate` and `ValidateAndBind`. This is useful for optimizing performance and focusing on specific parts of the request.

```go
// Validate only Headers and Query
if err := gofi.Validate(c, gofi.Headers, gofi.Query); err != nil {
    return err
}

// Bind only Body and Cookies
s, err := gofi.ValidateAndBind[UserSchema](c, gofi.Body, gofi.Cookies)
if err != nil {
    return err
}
```

Selective processing skips unnecessary components, reducing overhead.

## Middleware

Gofi uses a unified middleware system based on `MiddlewareFunc`:

```go
type MiddlewareFunc = func(c gofi.Context) error
```

Middlewares call `c.Next()` to proceed to the next handler in the chain.

### Global Middleware (`Use`)

Apply middleware to all routes on the router:

```go
r.Use(func(c gofi.Context) error {
    log.Println("Request received:", c.Request().URL.Path)
    return c.Next()
})
```

### Inline Middleware (`With`)

Apply middleware only to specific routes:

```go
auth := func(c gofi.Context) error {
    if c.Request().Header.Get("Authorization") == "" {
        return c.SendString(401, "Unauthorized")
    }
    return c.Next()
}

r.With(auth).Get("/protected", ProtectedHandler)
```

### Using net/http Middlewares

If you have existing `net/http` compatible middlewares (e.g., from Chi, Gorilla, or third-party libraries), use `gofi.WrapMiddleware` to convert them:

```go
import "github.com/rs/cors"

corsHandler := cors.Default()
r.Use(gofi.WrapMiddleware(corsHandler.Handler))
```

### Middleware Execution Order

Middlewares execute in registration order. If a middleware calls `c.Next()`, the subsequent middlewares and the handler run. Code after `c.Next()` runs on the way back.

```go
r.Use(func(c gofi.Context) error {
    log.Println("Before handler")
    err := c.Next()
    log.Println("After handler")
    return err
})
```

If a middleware returns an error without calling `c.Next()`, the chain is short-circuited and the handler is never reached.

### ValidateAndBind in Middleware

If you call `gofi.ValidateAndBind[T]` in a middleware, the result is cached on the context. Subsequent calls in the handler return the cached result without re-validation, which is useful for performance.