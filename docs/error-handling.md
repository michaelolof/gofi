# Handling errors in a Gofi service

Gofi's part of error handling is small and fixed:

1. Handlers and middleware return errors. They never write error responses.
2. One error handler, registered with `r.UseErrorHandler`, receives every
   returned error and writes the response.

What your errors look like, and which library builds them, is your decision.
This document recommends a pattern that has worked well in production Gofi
services. The examples in [GUIDELINES.md](../GUIDELINES.md) use it, and the
complete code is in the reference service
(`gofi-test-utils/examples/reference-service/internal/apperr`).

## The pattern in one example

```go
project, err := deps.MustGet(c).Projects().Get(c.Context(), caller.UserID, s.Request.Path.ProjectID)
if errors.Is(err, domain.ErrNotFound) {
	return apperr.Wrap(1793299514312, apperr.NotFound, err).WithMsg("project not found")
}
if err != nil {
	return apperr.Wrap(1793299514313, apperr.Internal, err)
}
```

Every returned error carries three things:

- **An error ID** (`1793299514312`): a number unique to this line of code.
- **A kind** (`apperr.NotFound`): what went wrong, which decides the status code.
- **A public message** (`"project not found"`), optional: what the client may see.

The client receives:

```json
{"status":"error","error_id":1793299514312,"message":"project not found"}
```

and the log line has the same ID plus everything the client must not see:
the wrapped cause, the route, the request ID.

## Timestamp error IDs

The error ID is an integer literal written by hand at the call site. Its value
is the Unix time in milliseconds at the moment you wrote the line, which makes
it unique without any registry or coordination.

```sh
# macOS and Linux
python3 -c 'import time; print(int(time.time() * 1000))'
```

Most teams bind this to an editor snippet so a new ID is one keystroke.

### Why this is worth doing

- **A report leads straight to the code.** A client quotes `error_id
  1793299514312` in a ticket; searching the codebase for that number finds the
  one line that returned it. No stack traces, no guessing which of five
  `"not found"` branches fired.
- **Logs and responses join on one value.** The same ID is in the response
  body and the log line, so support can find the server-side details of a
  client's error in one query.
- **IDs survive refactoring.** Line numbers and function names change; the
  literal moves with the code.
- **Messages stay safe.** Because the ID identifies the failure, the public
  message can stay short and generic while the log carries the detail.
- **Alerting gets precise.** You can count, graph and alert on individual
  failure sites instead of on status codes.

### Rules for IDs

- Every `New` or `Wrap` call gets its own ID. Never copy an ID to a second
  call site, and never reuse one when you change a line.
- Use the 13-digit millisecond timestamp. Do not mix it with 10-digit
  second-based IDs in the same service.
- IDs are literals, not constants: the point is that searching for the number
  finds the call site.
- Check for duplicates in CI:

  ```sh
  grep -rhoE 'apperr\.(New|Wrap)\([0-9]{13}' --include='*.go' . | sort | uniq -d
  ```

  Any output is a duplicated ID.

## The error type

A small package in your service is enough. The reference service's `apperr`:

```go
// Kind classifies an error and decides its HTTP status.
type Kind int

const (
	Internal Kind = iota
	InvalidInput
	Unauthorized
	Forbidden
	NotFound
	Conflict
)

// Error is an error with a unique ID, a Kind and an optional public message.
type Error struct {
	// ID is a unique integer literal written at the call site: the Unix time
	// in milliseconds when the line was written. Searching the codebase for
	// it finds the exact line that produced the error.
	ID   int64
	Kind Kind
	// Msg is safe to show to clients. Empty means "use the default for Kind".
	Msg string
	Err error

	status int // set only for errors converted from *gofi.HTTPError
}

// New returns an error that has no underlying cause.
func New(id int64, kind Kind, msg string) *Error

// Wrap returns err wrapped with an ID and a Kind.
func Wrap(id int64, kind Kind, err error) *Error

// WithMsg sets the message shown to clients.
func (e *Error) WithMsg(msg string) *Error
```

Two methods decide what reaches the client:

- `HTTPStatus()` maps the kind to a status code (`NotFound` → 404).
- `PublicMessage()` returns `Msg` if set, the Gofi validation message for
  `InvalidInput` errors that wrap one, and otherwise the status text
  ("Internal Server Error"). Wrapped causes never reach the client.

`From(err)` turns any error into an `*Error`, so the error handler has one
type to deal with. Errors nobody wrapped become `Internal` with ID `0`, which
is itself a signal: an `error_id` of `0` in the logs means some code path
returned a raw error and needs an ID. Gofi's own errors keep their meaning:
validation failures become `InvalidInput`, and a `*gofi.HTTPError` (404 for an
unknown route, 413 for a body over the limit) keeps its status.

## Validation errors

Wrap the error from `ValidateAndBind` like any other:

```go
s, err := gofi.ValidateAndBind[createProjectSchema](c)
if err != nil {
	return apperr.Wrap(1793299514301, apperr.InvalidInput, err)
}
```

Gofi's validation messages name the field and the rule that failed
(`invalid email value at request Body(email)`) and contain no server state, so
`PublicMessage` passes them to the client. The ID tells you which route
rejected the input.

## The error handler

```go
// ErrorHandler returns the router's error handler. Every error a handler or
// middleware returns ends up here: it is logged with its ID and written as
// a schemas.ErrBody.
func ErrorHandler(log *slog.Logger) func(err error, c gofi.Context) {
	return func(err error, c gofi.Context) {
		e := apperr.From(err)
		status := e.HTTPStatus()

		level := slog.LevelWarn
		if status >= 500 {
			level = slog.LevelError
		}
		log.Log(c.Context(), level, "request failed",
			"error_id", e.ID,
			"status", status,
			"method", c.Method(),
			"path", c.Path(),
			"request_id", GetRequestID(c),
			"error", err.Error(),
		)

		body, _ := json.Marshal(schemas.ErrBody{
			Status:  "error",
			ErrorID: e.ID,
			Message: e.PublicMessage(),
		})
		c.Writer().Header().Set("Content-Type", "application/json")
		_ = c.SendBytes(status, body)
	}
}
```

Register it once in `server.New`:

```go
r.Use(gofimw.Recover())
r.UseErrorHandler(middleware.ErrorHandler(args.Log))
```

`gofimw.Recover()` turns panics into errors so they reach this handler too and
the client gets the same error body. Without it, Gofi still recovers the panic
but answers with its own generic 500.

The handler also runs for failures that happen before any middleware: request
bodies over the limit (413), oversized headers (431) and read timeouts (408).
In that case the route, its parameters and anything middleware would have
stored are unavailable, so read them defensively (`GetRequestID` returns `""`).
See [transport-errors.md](transport-errors.md).

## Documenting errors

Declare the error body once, in your shared schemas package, and put it on
every route:

```go
type getProjectSchema struct {
	Request struct { /* ... */ }

	Ok  schemas.SuccessSchema[schemas.Project]
	Err schemas.ErrSchema
}
```

`ErrSchema` must describe exactly what the error handler writes. If you change
one, change the other.

## Not using this pattern

You do not need any of this to use Gofi. The minimum is:

```go
if err != nil {
	return gofi.NewHTTPError(404, "project not found")
}
```

With no error handler registered, Gofi sends 4xx `HTTPError` messages to the
client, replaces 5xx messages and unknown errors with the status text, logs
them, and sends validation errors as 400 with their message. Whatever you
choose, keep the two Gofi rules: return errors, and handle them in one place.
