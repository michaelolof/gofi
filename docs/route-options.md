# Route Options & Handlers

This guide details how to define route handlers and configure them using `RouteOptions`.

## Route Handler

A route handler in Gofi is a function that accepts a `Context` and returns an `error`.

```go
type HandlerFunc func(c gofi.Context) error
```

### The Context Interface
The `Context` interface provides methods to interact with the HTTP request and response, access stores, and utilities.

| Method | Description |
| :--- | :--- |
| **`Writer() http.ResponseWriter`** | Returns the underlying `http.ResponseWriter`. |
| **`Request() *http.Request`** | Returns the underlying `*http.Request`. |
| **`Send(code int, obj any) error`** | Sends a response based on the defined schema for the given status code. |
| **`SendString(code int, s string) error`** | Helper to send a plain text response. |
| **`SendBytes(code int, b []byte) error`** | Helper to send a raw byte response. |
| **`GlobalStore() ReadOnlyStore`** | Access the global, thread-safe key-value store defined on the main router. |
| **`DataStore() GofiStore`** | Access variables stored for the lifetime of the request (e.g. by middlewares). |
| **`Meta() ContextMeta`** | Access static meta information defined on the route. |

## Handler Incoming Requests and Outgoing Responses

The core usage of a handler is to process an incoming request, validate it, perform business logic, and return a response. Gofi streamlines this using the Schema pattern.

### 1. Processing Incoming Requests

To parse and validate an incoming request, use the generic `gofi.ValidateAndBind[T](c)` function inside your handler. `T` should be your route's schema struct.

```go
func(c gofi.Context) error {
    // 1. Validate and Bind
    // This reads the Body, Query, Headers, Path, and Cookies,
    // validates them against the struct tags, and returns a populated struct.
    data, err := gofi.ValidateAndBind[UserSchema](c)
    if err != nil {
        // If validation fails, Gofi returns a structured error.
        // You can return it directly to let the global error handler manage it.
        return err
    }

    // 2. Access Data
    // Data is now type-safe and validated.
    userID := data.Request.Path.UserID
    limit := data.Request.Query.Limit

    // ... business logic ...
}
```

### 2. Sending Outgoing Responses

To send a response, use the `c.Send(code, data)` method. The `data` argument should correspond to the specific response struct defined in your schema for that status code.

You often reuse the same schema struct instance returned by `ValidateAndBind` to populate the response, but you can also create a new instance.

```go
func(c gofi.Context) error {
    data, err := gofi.ValidateAndBind[UserSchema](c)
    if err != nil {
        return err
    }

    // ... logic to find user ...
    user := findUser(data.Request.Path.UserID)

    // 1. Populate the response struct
    // 'Ok' corresponds to the 200 OK field in UserSchema
    data.Ok.Body = user
    data.Ok.Header.LastModified = time.Now().String()

    // 2. Send the response
    // Pass the status code and the specific schema field for that code.
    // Gofi validates the response data against the schema before sending.
    return c.Send(200, data.Ok)
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
    Schema: UserSchema{},
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

### 4. PreHandlers (`PreHandlers`)
`PreHandlers` are middlewares specific to this route. They run *before* the main handler.

```go
var ProtectedRoute = gofi.DefineHandler(gofi.RouteOptions{
    PreHandlers: []gofi.PreHandler{
        CheckPreHandler,
    },
    Handler: func(c gofi.Context) error { ... },
})
```

**Definition:**
A `PreHandler` wraps a `HandlerFunc`.

```go
func CheckPreHandler(next gofi.HandlerFunc) gofi.HandlerFunc {
    return func(c gofi.Context) error {
        // ... pre-processing ...
        if err := checkSomething(); err != nil {
            return err
        }
        return next(c)
    }
}
```

### 5. Handler (`Handler`)
The main business logic for the route.

```go
var MyHandler = gofi.DefineHandler(gofi.RouteOptions{
    Handler: func(c gofi.Context) error {
        return c.SendString(200, "Hello World")
    },
})
```

## Middleware vs PreHandler

Gofi supports two types of middleware, which operate at different stages of the request lifecycle.

### 1. Router Middleware (`Use`, `With`)
Router middlewares are standard Golang middlewares (similar to `go-chi` or `net/http`). They wrap the standard `http.Handler` and are executed **early** in the request lifecycle, before the Gofi `Context` is created.

- **Interface**: `func(http.Handler) http.Handler`
- **Use Case**: Logging, CORS, GZIP compression, Panic recovery.
- **Scope**: Global (`Use`) or Group-specific (`With`).

```go
// Standard router middleware
func Logger(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Println("Request started")
        next.ServeHTTP(w, r)
        log.Println("Request finished")
    })
}

r.Use(Logger)
```

### 2. PreHandlers (`RouteOptions.PreHandlers`)
PreHandlers are Gofi-specific middlewares that run **after** the Gofi `Context` has been created but **before** your main handler.

- **Interface**: `func(gofi.HandlerFunc) gofi.HandlerFunc`
- **Use Case**: Authentication, Authorization, Validation logic that needs access to the `Context` (stores, schema validation results, etc.).
- **Scope**: Per-route (defined in `RouteOptions`).

```go
func AuthPreHandler(next gofi.HandlerFunc) gofi.HandlerFunc {
    return func(c gofi.Context) error {
        // Access Context methods
        if c.Request().Header.Get("Authorization") == "" {
             return c.SendString(401, "Unauthorized")
        }
        return next(c)
    }
}
```