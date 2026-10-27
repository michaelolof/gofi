# Routing

Gofi uses a **radix tree** (compressed trie) for route lookup, giving O(path-length) dispatch with zero per-request allocations on the hot path. This document covers the rules that govern how routes are matched, what the priority order is, and exactly which registration combinations panic at startup.

---

## Route syntax

| Syntax | Example | Matches |
|--------|---------|---------|
| Static segment | `/users/me` | Exactly `/users/me` |
| Named parameter | `/users/:id` | `/users/alice`, `/users/123`, etc. The captured value is available via `c.PathParam("id")`. |
| Catch-all | `/files/*filepath` | `/files/a`, `/files/a/b/c`, etc. The captured value (including the leading `/`) is available via `c.PathParam("filepath")`. |

Parameter names must start with a letter and contain only letters and digits (`[A-Za-z][A-Za-z0-9]*`).

---

## Match priority

When multiple patterns could match a request path, Gofi uses the following priority order, evaluated left-to-right at each segment boundary:

1. **Static segment** — preferred over any wildcard.
2. **Named parameter** (`:name`) — matched only when no static child matches.
3. **Catch-all** (`*name`) — matched at the end of the path after all other options are exhausted.

Priority is based on a successful **complete route match**, not only the next
byte. Gofi tries a matching static branch first. If that branch later
dead-ends, lookup returns to the same segment boundary and tries the named
parameter sibling.

### Example

Given these registered routes:

```text
GET /users/me
GET /users/:id
GET /users/*rest
```

| Request path | Matched route | Captured params |
|---|---|---|
| `/users/me` | `GET /users/me` | none |
| `/users/alice` | `GET /users/:id` | `id=alice` |
| `/users/alice/posts/1` | `GET /users/*rest` | `rest=/alice/posts/1` |

---

## Static and parameter siblings

A node **may** have both a static child and a `:param` child at the same level. Registration order does not matter — you can register the static route before or after the parametric one:

```go
// Both orderings are valid and produce identical routing behaviour.
r.Get("/matches/live", liveHandler)
r.Get("/matches/:id",  detailHandler)

// or

r.Get("/matches/:id",  detailHandler)
r.Get("/matches/live", liveHandler)
```

The static segment always wins when it is an exact match. Requests for
`/matches/live` are served by `liveHandler`; all other
`/matches/<value>` requests reach `detailHandler`.

This includes values that begin with the same byte or prefix as the static
route:

| Request | Result |
|---|---|
| `/matches/live` | exact static route |
| `/matches/l` | parameter route with `id=l` |
| `/matches/liverpool` | parameter route with `id=liverpool` |

Lookup initially considers the static `live` branch for the last two
requests, then falls back to `:id` when the complete static path does not
match. Registration order does not affect the result.

---

## Trailing-slash redirect (TSR)

If a request path is missing a trailing slash and the tree contains the path with a trailing slash (or vice versa), Gofi automatically issues a redirect:

| Registered | Request | Redirect |
|---|---|---|
| `/users/` | `GET /users` | `301 /users/` |
| `/users` | `GET /users/` | `301 /users` |

Query parameters are preserved in the `Location` header:

```
GET /users?page=2  →  301 Location: /users/?page=2
```

TSR can be suppressed by registering the canonicalised form explicitly (i.e., registering both `/users` and `/users/`).

A redirect recommendation is weaker than a real route match. For example, if
`/x/static/` and `/x/:id` are registered, `/x/static` reaches the
parameter route with `id=static` instead of redirecting to `/x/static/`.
Gofi redirects only after every eligible real route has failed.

---

## Route prefix helpers

`Route()` and method helpers (`Get`, `Post`, etc.) may be nested under a path prefix using `Route()`:

```go
r.Route("/api", func(r gofi.Router) {
    r.Get("/users", listHandler)  // registers /api/users
})
```

The prefix join is slash-safe: all four combinations of trailing/leading slashes collapse to a single `/`:

| Prefix | Path | Result |
|---|---|---|
| `/api` | `/users` | `/api/users` |
| `/api/` | `/users` | `/api/users` |
| `/api` | `users` | `/api/users` |
| `/api/` | `users` | `/api/users` |

---

## What panics (and why)

The following situations panic at **startup** (route registration time), never at request time:

| Situation | Panic message | Why |
|---|---|---|
| Same static path registered twice | `a route is already registered for path '/foo' (attempted duplicate registration of '/foo')` | Two handlers on the same pattern is an unresolvable ambiguity. |
| Two `:param` names at the same position | `path segment ':name' conflicts with existing wildcard ':id' in path '/users/:name'` | The tree cannot know which name to use at lookup time; the semantics would be undefined. |
| Catch-all route followed by a static sibling under the same prefix | `new path '/files/index.html' conflicts with existing catch-all wildcard '/files/*rest'` | A catch-all by definition consumes the entire remainder of the path; a static sibling at the same level would never be reachable. |
| Catch-all not at end of path | `catch-all routes are only allowed at the end of the path in path '/a/*x/b'` | The catch-all would swallow the `/b` segment, making the pattern semantically incorrect. |
| No `/` before a catch-all | `no / before catch-all in path '...'` | Catch-all must appear after a segment boundary. |
| Wildcard without a name | `wildcards must be named with a non-empty name in path '...'` | `/users/:/posts` is not a valid route shape. |

### What does NOT panic

- Registering `/users/me` **and** `/users/:id` — this is explicitly supported (see [Static and parameter siblings](#static-and-parameter-siblings)).
- Registering routes in different registration orders — the tree tolerates any order.
- Registering a catch-all alongside a named parameter at a different depth — e.g. `/a/:id` and `/a/*rest` (the catch-all catches paths with additional segments).

---

## Method Not Allowed (405)

When `Config.MethodNotAllowed` is set to `true` (the default), a request that matches a registered path under a **different** HTTP method receives a `405 Method Not Allowed` response with an `Allow` header listing the accepted methods, instead of the generic `404 Not Found`.

```go
r := gofi.NewRouter()
// MethodNotAllowed is enabled by default; use Configure to opt out:
r.Configure(gofi.Config{MethodNotAllowed: false})

r.Get("/users", listHandler)
// POST /users → 405 Allow: GET
```

Correct same-method wildcard fallback runs before this cross-method check. A
request that previously returned an incorrect 404 or 405 because a static
branch dead-ended may now reach its registered parameter handler. A genuine
method mismatch still returns 405.

---

## Parameter value lifetime

`Param.Value` is a zero-copy slice into the raw request buffer, obtained via `c.Param(name)`. The value is valid for the **duration of the handler chain**: the next request on the same connection overwrites the bytes, so a retained string changes without warning. Do not retain it in a goroutine, queue, cache or map key that outlives the handler. If you need to keep the value, copy it:

```go
id := strings.Clone(c.Param("id")) // always safe to retain
```

The same applies to path fields bound by `gofi.ValidateAndBind`, and to the bound struct itself; see [Result lifetime](validations.md#result-lifetime).

For WebSocket handlers, which take ownership of the connection, the fasthttp request buffer remains pinned for the connection lifetime and param values remain safe to read within the WebSocket goroutine.
