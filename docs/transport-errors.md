# Native transport errors

Native listeners (`Listen`, `ListenTLS`, `ListenTLSMutual`, and
`NewFasthttpServer`) send request parsing and read failures to the interceptor
registered through `UseErrorHandler`. The default interceptor returns a JSON error envelope with
`Content-Type: application/json` and a safe public message:

| Failure | Status | Message |
| --- | --- | --- |
| Body exceeds the configured limit | 413 | Request Entity Too Large |
| Headers exceed the native read buffer | 431 | Request Header Fields Too Large |
| Request-read network timeout | 408 | Request Timeout |
| Other parsing or read failure | 400 | Bad Request |

These failures occur before dispatch: ordinary middleware, body binding, schema
validation, and route handlers do not run. The underlying request information
may be incomplete or unavailable. `Pattern()`, route parameters, and route
metadata are unavailable, and `DataStore()` starts empty. The router's
`GlobalStore()` remains accessible. Native configuration and fasthttp's rules
still determine which reads time out and which connection failures receive a
response; this does not add timeout or header-size configuration options.

If your interceptor normally reads middleware-provided metadata such as a
request ID, supply a fallback there when the store value is absent (for example,
generate a new request ID in the interceptor). Put shared fallback settings in
`GlobalStore()` during setup. Do not assume incoming headers are complete or
trusted. The interceptor can choose status, headers, content type, and body via
the existing `Context` methods, including `Writer().Header()`.

Use `errors.As(err, &httpErr)` with `var httpErr *gofi.HTTPError` to recognize the
HTTP status. The original read failure is also available through standard
`errors.Is` / `errors.As` traversal. Its diagnostics can contain request data;
use the safe outer `err.Error()` message for public transport-error responses
instead of serializing the underlying cause. Register the interceptor before
serving requests; registration after `NewFasthttpServer()` construction is
supported.

