# Gofi Schema Guide

This guide provides a detailed reference for defining schemas in Gofi, covering request fields, response statuses, and validation rules.

## Overview

A schema in Gofi is a Go struct that acts as a contract for your HTTP endpoints. It defines what input is expected (Request) and what output is returned (Response).

```go
type MyRouteSchema struct {
    Request struct { ... }
    // Response fields map directly to HTTP status codes
    Ok      struct { ... }
    Created struct { ... }
    // ...
}
```

## Request Schema

The `Request` struct defines the expected input. Gofi binds data from the HTTP request to these fields.

### Supported Fields

| Field | Source | description |
| :--- | :--- | :--- |
| **`Path`** | URL Path | Maps to path parameters (e.g., `/users/{id}`). |
| **`Query`** | URL Query | Maps to query string parameters (e.g., `?page=1`). |
| **`Header`** | HTTP Headers | Maps to request headers. |
| **`Cookie`** | HTTP Cookies | Maps to request cookies. Supports `string` or `http.Cookie`. |
| **`Body`** | Request Body | Maps to the request body (JSON, XML, etc.). |

### Example

```go
Request struct {
    // Path parameters (e.g., /users/:id)
    Path struct {
        UserID string `json:"id" validate:"required,uuid"`
    }

    // Query parameters (e.g., ?page=1&sort=asc)
    Query struct {
        Page int    `json:"page" default:"1"`
        Sort string `json:"sort" validate:"oneof=asc desc"`
    }

    // Headers
    Header struct {
        ApiKey string `json:"X-Api-Key" validate:"required"`
    }

    // Cookies
    Cookie struct {
        SessionID string `json:"session_id" validate:"required"`
    }

    // Request Body
    Body struct {
        Username string `json:"username" validate:"required"`
        Email    string `json:"email" validate:"required,email"`
    }
}
```

## Response Schema

Responses are defined by fields that match supported HTTP status codes. Each field represents a possible response and can contain `Body`, `Header`, and `Cookie` sub-fields.

### Sub-fields

- **`Body`**: The payload to send (struct, string, etc.).
- **`Header`**: Headers to set on the response.
- **`Cookie`**: Cookies to set. Can be `string` values or `http.Cookie` structs.

### Supported Response Fields

You can use any of the following fields in your root schema struct to define a response for that status code.

#### Informational (1XX)
- **`Continue`**: `100 Continue`
- **`SwitchingProtocols`**: `101 Switching Protocols`
- **`Processing`**: `102 Processing`
- **`EarlyHints`**: `103 Early Hints`

#### Success (2XX)
- **`Ok`**: `200 OK`
- **`Created`**: `201 Created`
- **`Accepted`**: `202 Accepted`
- **`NonAuthoritativeInformation`**: `203 Non-Authoritative Information`
- **`NoContent`**: `204 No Content`
- **`ResetContent`**: `205 Reset Content`
- **`PartialContent`**: `206 Partial Content`
- **`MultiStatus`**: `207 Multi-Status`
- **`AlreadyReported`**: `208 Already Reported`
- **`IMUsed`**: `226 IM Used`

#### Redirection (3XX)
- **`MultipleChoices`**: `300 Multiple Choices`
- **`MovedPermanently`**: `301 Moved Permanently`
- **`Found`**: `302 Found`
- **`SeeOther`**: `303 See Other`
- **`NotModified`**: `304 Not Modified`
- **`TemporaryRedirect`**: `307 Temporary Redirect`
- **`PermanentRedirect`**: `308 Permanent Redirect`

#### Client Errors (4XX)
- **`BadRequest`**: `400 Bad Request`
- **`Unauthorized`**: `401 Unauthorized`
- **`PaymentRequired`**: `402 Payment Required`
- **`Forbidden`**: `403 Forbidden`
- **`NotFound`**: `404 Not Found`
- **`MethodNotAllowed`**: `405 Method Not Allowed`
- **`NotAcceptable`**: `406 Not Acceptable`
- **`ProxyAuthenticationRequired`**: `407 Proxy Authentication Required`
- **`RequestTimeout`**: `408 Request Timeout`
- **`Conflict`**: `409 Conflict`
- **`Gone`**: `410 Gone`
- **`LengthRequired`**: `411 Length Required`
- **`PreconditionFailed`**: `412 Precondition Failed`
- **`ContentTooLarge`**: `413 Content Too Large`
- **`URITooLong`**: `414 URI Too Long`
- **`UnsupportedMediaType`**: `415 Unsupported Media Type`
- **`RangeNotSatisfiable`**: `416 Range Not Satisfiable`
- **`ExpectiationFailed`**: `417 Expectation Failed`
- **`ImTeamPot`**: `418 I'm a teapot`
- **`MisdirectedRequest`**: `421 Misdirected Request`
- **`UnprocessableContent`**: `422 Unprocessable Content`
- **`Locked`**: `423 Locked`
- **`FailedDependency`**: `424 Failed Dependency`
- **`TooEarly`**: `425 Too Early`
- **`UpgradeRequired`**: `426 Upgrade Required`
- **`PreconditionRequired`**: `428 Precondition Required`
- **`TooManyRequests`**: `429 Too Many Requests`
- **`RequestHeaderFieldsTooLarge`**: `431 Request Header Fields Too Large`
- **`UnavailableForLegalReasons`**: `451 Unavailable For Legal Reasons`

#### Server Errors (5XX)
- **`InternalServerError`**: `500 Internal Server Error`
- **`NotImplemented`**: `501 Not Implemented`
- **`BadGateway`**: `502 Bad Gateway`
- **`ServiceUnavailable`**: `503 Service Unavailable`
- **`GatewayTimeout`**: `504 Gateway Timeout`
- **`HTTPVersionNotSupported`**: `505 HTTP Version Not Supported`
- **`VariantAlsoNegotiates`**: `506 Variant Also Negotiates`
- **`InsufficientStorage`**: `507 Insufficient Storage`
- **`LoopDetected`**: `508 Loop Detected`
- **`NotExtended`**: `510 Not Extended`
- **`NetworkAuthenticationRequired`**: `511 Network Authentication Required`

#### Generic Fields
- **`Informational`**: Generic fallback for `1XX` codes.
- **`Success`**: Generic fallback for `2XX` codes.
- **`Redirect`**: Generic fallback for `3XX` codes.
- **`ClientError`**: Generic fallback for `4XX` codes.
- **`ServerError`**: Generic fallback for `5XX` codes.
- **`Err`**: Generic fallback for any `4XX` or `5XX` error not explicitly matched.
- **`Default`**: Generic fallback for ANY status code not explicitly matched.

## Validation Tags

Gofi supports standard validation tags (inspired by `go-playground/validator`).

- **`validate`**:
    - `required`: Field cannot be zero value.
    - `min=X`, `max=Y`: Length limits (strings, slices) or value limits (numbers).
    - `email`: Valid email format.
    - `uuid`, `uuid4`: Valid UUID.
    - `oneof=a b c`: Must match one of the values.
- **`default`**: Sets a default value if the input is missing/empty.
- **`json`**: Maps the field to the input source key (query param name, header name, JSON field, etc.).
