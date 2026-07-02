# Fluid — Typed OpenAPI Components

**Package:** `github.com/michaelolof/gofi/fluid`

`fluid` is a companion package to Gofi that provides **type-safe Go structs** for
constructing **OpenAPI 3.0.3 Component Objects**. Instead of hand-crafting
brittle `map[string]any` values, you get full compile-time safety,
IDE auto-completion, and fluent builder methods for every OpenAPI component
type.

---

## Why Fluid?

Gofi's `DocsComponent` struct accepts `Schemas map[string]any`. Before `fluid`,
you had to write raw map literals:

```go
// Before — raw maps, no safety, no autocomplete
gofi.DocsComponent{
    Schemas: map[string]any{
        "Error": map[string]any{
            "type": "object",
            "required": []string{"code", "message"},
            "properties": map[string]any{
                "code":    map[string]any{"type": "integer"},
                "message": map[string]any{"type": "string"},
            },
        },
    },
}
```

With `fluid`, that same schema becomes:

```go
import "github.com/michaelolof/gofi/fluid"

gofi.DocsComponent{
    Schemas: map[string]any{
        "Error": fluid.ObjectSchema(map[string]fluid.SchemaObject{
            "code":    fluid.IntegerSchema().WithDescription("HTTP status code"),
            "message": fluid.StringSchema().WithDescription("Error message"),
        }).WithRequired("code", "message"),
    },
}
```

**Key benefits:**

- **Compile-time safety.** You can't accidentally assign a string where an
  object is expected, or misspell a JSON key.
- **IDE auto-complete.** Every field, constructor, and builder method is
  discoverable through your editor.
- **Self-documenting.** The chain of `.With*()` methods reads like natural
  English and matches the OpenAPI spec structure exactly.
- **Zero Gofi dependencies.** The `fluid` package only imports the standard
  library. You can use it standalone to generate OpenAPI JSON.

---

## Installation

```sh
go get -u github.com/michaelolof/gofi
```

The `fluid` package is bundled with Gofi. Import it as:

```go
import "github.com/michaelolof/gofi/fluid"
```

---

## Core Concepts

All `fluid` types are plain Go structs with `json` tags. When you place them
into a `gofi.DocsComponent.Schemas` map (which is `map[string]any`), Go's
`encoding/json` serializes them identically to hand-crafted maps.

The API is divided into three layers:

1. **Convenience constructors** — create common schemas in one call (e.g.
   `fluid.StringSchema()`, `fluid.ObjectSchema(props)`)
2. **Fluent builder methods** — chain `.With*()` calls to add constraints,
   descriptions, examples, and metadata
3. **Raw struct literals** — for advanced cases, compose structs directly
   for full control over every field

---

## SchemaObject — The Foundation

`SchemaObject` is the most important type. It represents the [OpenAPI 3.0.3
Schema Object](https://spec.openapis.org/oas/v3.0.3#schema-object) and
supports all JSON Schema keywords.

### Primitive Constructors

```go
fluid.StringSchema()    // {"type": "string"}
fluid.IntegerSchema()   // {"type": "integer"}
fluid.NumberSchema()    // {"type": "number"}
fluid.BooleanSchema()   // {"type": "boolean"}
```

### Format-Specific Constructors

These create `"type": "string"` schemas with the appropriate `format`:

```go
fluid.DateTimeSchema()   // string, format: date-time    (RFC 3339)
fluid.DateSchema()       // string, format: date          (full-date)
fluid.EmailSchema()      // string, format: email
fluid.URISchema()        // string, format: uri
fluid.UUIDSchema()       // string, format: uuid
fluid.BinarySchema()     // string, format: binary
fluid.ByteSchema()       // string, format: byte          (base64)
fluid.PasswordSchema()   // string, format: password      (UI hint)
```

### Array & Object

```go
// Array of strings
tags := fluid.ArraySchema(fluid.StringSchema())

// Object with typed properties
user := fluid.ObjectSchema(map[string]fluid.SchemaObject{
    "name": fluid.StringSchema(),
    "age":  fluid.IntegerSchema(),
})
```

Pass `nil` to `ObjectSchema` to create a free-form object (useful with
`WithAdditionalProperties`):

```go
// Key-value map of string → integer
scores := fluid.ObjectSchema(nil).
    WithAdditionalProperties(fluid.IntegerSchema())
```

### $ref References

```go
profile := fluid.RefSchema("#/components/schemas/UserProfile")
```

> **Important:** A `RefSchema` must not be chained with any builder methods.
> Per the OpenAPI spec, `$ref` replaces the entire schema object.

### Builder Methods

Every builder returns a new `SchemaObject`, so you can chain them fluently:

```go
fluid.StringSchema().
    WithDescription("Full name").
    WithMinLength(fluid.IntPtr(1)).
    WithMaxLength(fluid.IntPtr(100)).
    WithPattern(`^[a-zA-Z\s]+$`).
    WithExample("Jane Doe").
    WithNullable()
```

| Constraint | Builder | Works On |
|---|---|---|
| Description | `.WithDescription(string)` | Any |
| Example | `.WithExample(any)` | Any |
| Default | `.WithDefault(any)` | Any |
| Enum values | `.WithEnum(values ...any)` | Any |
| Nullable | `.WithNullable()` | Any |
| Deprecated | `.WithDeprecated(bool)` | Any |
| Read-only | `.WithReadOnly()` | Any |
| Write-only | `.WithWriteOnly()` | Any |
| Minimum | `.WithMinimum(*float64)` | Number, Integer |
| Maximum | `.WithMaximum(*float64)` | Number, Integer |
| Exclusive min | `.WithExclusiveMinimum(*float64)` | Number, Integer |
| Exclusive max | `.WithExclusiveMaximum(*float64)` | Number, Integer |
| Multiple of | `.WithMultipleOf(*float64)` | Number, Integer |
| Min length | `.WithMinLength(*int)` | String |
| Max length | `.WithMaxLength(*int)` | String |
| Pattern (regex) | `.WithPattern(string)` | String |
| Format | `.WithFormat(string)` | String |
| Min items | `.WithMinItems(*int)` | Array |
| Max items | `.WithMaxItems(*int)` | Array |
| Unique items | `.WithUniqueItems()` | Array |
| Required fields | `.WithRequired(fields ...string)` | Object |
| Additional properties | `.WithAdditionalProperties(SchemaObject)` | Object |
| Min properties | `.WithMinProperties(*int)` | Object |
| Max properties | `.WithMaxProperties(*int)` | Object |
| Discriminator | `.WithDiscriminator(DiscriminatorObject)` | Any |
| External docs | `.WithExternalDocs(ExternalDocsObject)` | Any |
| XML hints | `.WithXML(XMLObject)` | Any |

### Pointer Helpers

Since OpenAPI optional numeric fields use `*float64` and `*int`, `fluid`
provides helper functions:

```go
fluid.FloatPtr(0.0)   // *float64
fluid.IntPtr(10)       // *int
fluid.BoolPtr(true)    // *bool
```

---

## Practical Examples

### Enums

```go
fluid.StringSchema().
    WithEnum("admin", "editor", "viewer").
    WithDescription("User permission level")
```

```json
{
  "type": "string",
  "enum": ["admin", "editor", "viewer"],
  "description": "User permission level"
}
```

### Constrained Numbers

```go
fluid.IntegerSchema().
    WithMinimum(fluid.FloatPtr(0)).
    WithMaximum(fluid.FloatPtr(100)).
    WithMultipleOf(fluid.FloatPtr(5)).
    WithDescription("Score in increments of 5")
```

### Nested Objects

```go
fluid.ObjectSchema(map[string]fluid.SchemaObject{
    "street":  fluid.StringSchema().WithDescription("Street address"),
    "city":    fluid.StringSchema(),
    "zipCode": fluid.StringSchema().
        WithPattern(`^\d{5}(-\d{4})?$`).
        WithDescription("US ZIP code"),
}).WithRequired("street", "city")
```

### Polymorphism (oneOf + Discriminator)

```go
fluid.SchemaObject{
    OneOf: []fluid.SchemaObject{
        fluid.RefSchema("#/components/schemas/Cat"),
        fluid.RefSchema("#/components/schemas/Dog"),
    },
}.WithDiscriminator(fluid.DiscriminatorObject{
    PropertyName: "petType",
    Mapping: map[string]string{
        "cat": "#/components/schemas/Cat",
        "dog": "#/components/schemas/Dog",
    },
}).WithDescription("A polymorphic pet")
```

### Deprecated & Nullable Fields

```go
fluid.ObjectSchema(map[string]fluid.SchemaObject{
    "email":      fluid.EmailSchema(),
    "phone":      fluid.StringSchema().
        WithDeprecated(true).
        WithDescription("Use contactMethods instead"),
    "middleName": fluid.StringSchema().
        WithNullable().
        WithDescription("Optional middle name"),
}).WithRequired("email")
```

---

## Complete Wiring Example

Here is a realistic Gofi server with `fluid`-powered documentation:

```go
package main

import (
    "github.com/michaelolof/gofi"
    "github.com/michaelolof/gofi/fluid"
)

func main() {
    app := gofi.New()

    // ... register routes ...

    gofi.ServeDocs(app, gofi.DocsOptions{
        Info: gofi.DocsInfoOptions{
            Title:   "My API",
            Version: "1.0.0",
        },
        Views: []gofi.DocsView{
            {
                RoutePrefix: "/docs",
                Components: gofi.DocsComponent{
                    Schemas: map[string]any{
                        // ---- Domain schemas ----
                        "User": fluid.ObjectSchema(map[string]fluid.SchemaObject{
                            "id":        fluid.IntegerSchema().WithDescription("Unique user ID"),
                            "name":      fluid.StringSchema().WithDescription("Full name"),
                            "email":     fluid.EmailSchema(),
                            "role":      fluid.StringSchema().
                                WithEnum("admin", "editor", "viewer"),
                            "createdAt": fluid.DateTimeSchema(),
                        }).WithRequired("id", "name", "email"),

                        "Address": fluid.ObjectSchema(map[string]fluid.SchemaObject{
                            "street":  fluid.StringSchema(),
                            "city":    fluid.StringSchema(),
                            "country": fluid.StringSchema().
                                WithMinLength(fluid.IntPtr(2)).
                                WithMaxLength(fluid.IntPtr(2)),
                        }).WithRequired("street", "city", "country"),

                        // ---- Error schemas ----
                        "Error": fluid.ObjectSchema(map[string]fluid.SchemaObject{
                            "code":    fluid.IntegerSchema().WithExample(500),
                            "message": fluid.StringSchema().WithExample("Internal error"),
                        }).WithRequired("code", "message"),

                        "ValidationError": fluid.ObjectSchema(map[string]fluid.SchemaObject{
                            "code":    fluid.IntegerSchema().WithExample(422),
                            "message": fluid.StringSchema().WithExample("Validation failed"),
                            "details": fluid.ArraySchema(
                                fluid.ObjectSchema(map[string]fluid.SchemaObject{
                                    "field":   fluid.StringSchema().WithDescription("Field name"),
                                    "message": fluid.StringSchema().WithDescription("Error description"),
                                }),
                            ).WithDescription("Per‑field validation errors"),
                        }).WithRequired("code", "message"),
                    },
                },
            },
        },
    })

    app.Listen(":3000")
}
```

---

## Beyond Schemas — Other Component Types

While `DocsComponent.Schemas` is the primary way to enrich your OpenAPI spec
today, `fluid` also provides typed structs for every OpenAPI component type.
These are ready for future Gofi versions that may support global
`parameters`, `responses`, `requestBodies`, `headers`, `examples`, and
`securitySchemes` in `DocsComponent`.

### ParameterObject

Create reusable parameter definitions:

```go
// Query parameter
pageSize := fluid.QueryParameter("pageSize", fluid.IntegerSchema().
    WithMinimum(fluid.FloatPtr(1)).
    WithMaximum(fluid.FloatPtr(100)).
    WithDefault(20).
    WithDescription("Number of results per page"))

// Path parameter (always required)
userId := fluid.PathParameter("userId", fluid.IntegerSchema().
    WithDescription("The user's unique ID"))

// Header parameter
apiKey := fluid.HeaderParameter("X-API-Key", fluid.StringSchema().
    WithDescription("API key for authentication"))

// Cookie parameter
sessionId := fluid.CookieParameter("SID", fluid.StringSchema())
```

```go
pageSize.WithRequired(true).WithStyle("form").WithExplode(true)
```

### ResponseObject

Define reusable response structures:

```go
notFound := fluid.JSONResponse("Resource not found",
    fluid.RefSchema("#/components/schemas/Error"))

// Multi-content response
response := fluid.ResponseObject{Description: "OK"}.
    WithJSONContent(fluid.StringSchema()).
    WithContent(map[string]fluid.MediaTypeObject{
        "text/html": {Schema: fluid.Ptr(fluid.StringSchema())},
    })

// With headers
response.WithHeaders(map[string]fluid.HeaderObject{
    "X-Rate-Limit": {
        Schema:      fluid.Ptr(fluid.IntegerSchema()),
        Description: "Requests remaining",
    },
})
```

### RequestBodyObject

Describe request payloads:

```go
createUser := fluid.JSONRequestBody(
    fluid.ObjectSchema(map[string]fluid.SchemaObject{
        "name":  fluid.StringSchema(),
        "email": fluid.EmailSchema(),
    }).WithRequired("name", "email"),
).WithRequired().WithDescription("User to create")

// Form-encoded
login := fluid.FormURLEncodedRequestBody(
    fluid.ObjectSchema(map[string]fluid.SchemaObject{
        "username": fluid.StringSchema(),
        "password": fluid.PasswordSchema(),
    }),
)

// Multipart
upload := fluid.MultipartRequestBody(
    fluid.ObjectSchema(map[string]fluid.SchemaObject{
        "file":    fluid.BinarySchema(),
        "caption": fluid.StringSchema(),
    }),
)
```

### SecuritySchemeObject

Declare authentication methods:

```go
bearer := fluid.BearerAuth(
    fluid.WithSecurityDescription("Enter your JWT token"),
)

basic := fluid.BasicAuth()

apiKey := fluid.APIKeyAuth("X-API-Key", "header",
    fluid.WithSecurityDescription("Project API key"),
)

oauth := fluid.OAuth2Auth(fluid.OAuthFlowsObject{
    AuthorizationCode: &fluid.OAuthFlowObject{
        AuthorizationURL: "https://auth.example.com/authorize",
        TokenURL:         "https://auth.example.com/token",
        Scopes: map[string]string{
            "read:users":  "Read user data",
            "write:users": "Create or modify users",
        },
    },
})
```

### HeaderObject, ExampleObject, EncodingObject

```go
// Response header
rateLimit := fluid.HeaderObject{}.
    WithDescription("Remaining requests").
    WithSchema(fluid.IntegerSchema()).
    WithRequired(true)

// Named example
activeExample := fluid.ExampleObject{
    Summary: "Active user",
    Value:   map[string]any{"name": "Alice", "status": "active"},
}

// Encoding override (for multipart/form-data properties)
fileEncoding := fluid.EncodingObject{
    ContentType: "image/png",
    Style:       "form",
    Explode:     fluid.BoolPtr(true),
}
```

---

## Full API Reference

### SchemaObject

```go
// Simple constructors
fluid.StringSchema()        fluid.IntegerSchema()
fluid.NumberSchema()        fluid.BooleanSchema()

// Format constructors
fluid.DateTimeSchema()      fluid.DateSchema()
fluid.BinarySchema()        fluid.ByteSchema()
fluid.PasswordSchema()      fluid.EmailSchema()
fluid.URISchema()           fluid.UUIDSchema()

// Composite constructors
fluid.ArraySchema(items SchemaObject) SchemaObject
fluid.ObjectSchema(properties map[string]SchemaObject) SchemaObject
fluid.RefSchema(ref string) SchemaObject

// Builder methods (return SchemaObject, chainable)
.WithDescription(string)         .WithExample(any)
.WithDefault(any)                .WithEnum(...any)
.WithNullable()                  .WithDeprecated(bool)
.WithReadOnly()                  .WithWriteOnly()
.WithMinimum(*float64)           .WithMaximum(*float64)
.WithExclusiveMinimum(*float64)  .WithExclusiveMaximum(*float64)
.WithMultipleOf(*float64)        .WithMinLength(*int)
.WithMaxLength(*int)             .WithPattern(string)
.WithFormat(string)              .WithMinItems(*int)
.WithMaxItems(*int)              .WithUniqueItems()
.WithRequired(...string)         .WithAdditionalProperties(SchemaObject)
.WithProperties(map[string]SchemaObject)
.WithMinProperties(*int)         .WithMaxProperties(*int)
.WithExternalDocs(ExternalDocsObject)
.WithXML(XMLObject)              .WithDiscriminator(DiscriminatorObject)
```

### ParameterObject

```go
fluid.QueryParameter(name, schema)   fluid.PathParameter(name, schema)
fluid.HeaderParameter(name, schema)  fluid.CookieParameter(name, schema)

.WithDescription(string)  .WithRequired(bool)   .WithDeprecated(bool)
.WithExample(any)         .WithExamples(map)     .WithContent(map)
.WithStyle(string)        .WithExplode(bool)
.WithAllowEmptyValue()    .WithAllowReserved()
```

### ResponseObject

```go
fluid.JSONResponse(description, schema)     fluid.PlainTextResponse(description, schema)
fluid.HTMLResponse(description)             fluid.BinaryResponse(description)

.WithDescription(string)  .WithHeaders(map)  .WithContent(map)
.WithJSONContent(schema)
```

### RequestBodyObject

```go
fluid.JSONRequestBody(schema)            fluid.FormURLEncodedRequestBody(schema)
fluid.MultipartRequestBody(schema)       fluid.PlainTextRequestBody(schema)

.WithDescription(string)  .WithRequired()      .WithContent(map)
.WithJSONContent(schema)  .WithFormURLEncodedContent(schema)
.WithMultipartContent(schema)
```

### SecuritySchemeObject

```go
fluid.BearerAuth(opts...func(*SecuritySchemeObject))
fluid.BasicAuth(opts...func(*SecuritySchemeObject))
fluid.APIKeyAuth(name, in, opts...func(*SecuritySchemeObject))
fluid.OAuth2Auth(flows OAuthFlowsObject, opts...func(*SecuritySchemeObject))
fluid.OpenIDConnectAuth(url, opts...func(*SecuritySchemeObject))

// Option functions
fluid.WithSecurityDescription(string) func(*SecuritySchemeObject)
fluid.WithBearerFormat(string)        func(*SecuritySchemeObject)
```

### Other Types

```go
fluid.HeaderObject{}.WithDescription(string).WithRequired(bool).
    WithDeprecated(bool).WithSchema(SchemaObject).WithExample(any).
    WithExamples(map[string]ExampleObject)

fluid.MediaTypeObject{}.WithSchema(SchemaObject).WithExample(any).
    WithExamples(map[string]ExampleObject).WithEncoding(map[string]EncodingObject)

fluid.ExampleObject{Summary, Description, Value, ExternalValue}
fluid.EncodingObject{ContentType, Headers, Style, Explode, AllowReserved}
fluid.DiscriminatorObject{PropertyName, Mapping map[string]string}
fluid.XMLObject{Name, Namespace, Prefix, Attribute, Wrapped}
fluid.ExternalDocsObject{Description, URL}

// Pointer helpers
fluid.IntPtr(i int)          *int
fluid.FloatPtr(f float64)    *float64
fluid.BoolPtr(b bool)        *bool
fluid.Ptr[T any](v T)        *T
```

---

## Migrating from Raw Maps

If you already use `map[string]any`, migration is straightforward:

```go
// Before
"User": map[string]any{
    "type": "object",
    "required": []string{"id", "name"},
    "properties": map[string]any{
        "id":   map[string]any{"type": "integer"},
        "name": map[string]any{"type": "string"},
    },
},

// After — identical JSON output with full type safety
"User": fluid.ObjectSchema(map[string]fluid.SchemaObject{
    "id":   fluid.IntegerSchema(),
    "name": fluid.StringSchema(),
}).WithRequired("id", "name"),
```

The old `map[string]any` approach continues to work — `fluid` is an
opt-in improvement, not a breaking change.

---

## Integration with Gofi Docs

`fluid` works anywhere Gofi accepts raw OpenAPI JSON. The primary integration
point is `DocsComponent.Schemas`:

```go
func(c gofi.Context) error {
    docs := gofi.DocsComponent{
        Schemas: map[string]any{
            "Error": ErrorSchema(),  // returns fluid.SchemaObject
        },
    }
    // ...
}
```

Because `DocsComponent.Schemas` is `map[string]any`, you can mix `fluid`
types with raw maps in the same dictionary — useful during incremental
migration.

---

## Further Reading

- [OpenAPI 3.0.3 Specification — Schema Object](https://spec.openapis.org/oas/v3.0.3#schema-object)
- [Gofi README — Serving OpenAPI Documentation](../README.md#serving-openapi-documentation)
- [Fluid package source](https://github.com/michaelolof/gofi/tree/main/fluid)
