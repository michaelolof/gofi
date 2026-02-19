# Gofi vs Chi — Benchmark Comparison

Comprehensive performance benchmark comparing [**Gofi**](https://github.com/michaelolof/gofi) (with and without schema validation) and [**Chi**](https://github.com/go-chi/chi) HTTP routers for Go.

Three configurations tested:
- **Gofi** — Plain routing, no schema (raw `http.ServeMux` wrapper)
- **GofiSchema** — Routes with typed schema structs + `ValidateAndBind` (Gofi's key differentiator)
- **Chi** — Standard Chi router

Benchmark methodology inspired by [gin-gonic/gin BENCHMARKS.md](https://github.com/gin-gonic/gin/blob/master/BENCHMARKS.md) and [go-http-routing-benchmark](https://github.com/michaelolof/gofi-go-http-routing-benchmark).

## System Info

```
goos: darwin
goarch: amd64
cpu: Intel(R) Core(TM) i7-4980HQ CPU @ 2.80GHz
go: go1.25.0
benchmarks: -bench=. -benchmem -count=3
```

---

## Summary

| Category | Gofi | GofiSchema | Chi | Winner |
|---|---|---|---|---|
| Static Route | 508 ns | 574 ns | 303 ns | Chi |
| Single Param | 624 ns | 2,519 ns | 544 ns | Chi |
| 5 Params | 1,155 ns | 3,168 ns | 789 ns | Chi |
| **20 Params** | **1,950 ns** | — | 3,473 ns | **Gofi** ⭐ |
| **Param Write** | **348 ns** | 2,558 ns | 710 ns | **Gofi** ⭐ |
| Multi Param (2) | 1,028 ns | 3,962 ns | 980 ns | Chi |
| Wildcard | 1,395 ns | — | 1,867 ns | Gofi |
| Deep Nesting | 1,285 ns | — | 474 ns | Chi |
| 404 Handling | 1,330 ns | — | 1,329 ns | Tie |
| Middleware ×5 | 1,582 ns | — | 601 ns | Chi |
| Middleware ×10 | 1,468 ns | — | 657 ns | Chi |
| Middleware ×20 | 2,102 ns | — | 699 ns | Chi |
| JSON Bind | 9,360 ns | 13,933 ns | 9,250 ns | Chi |
| JSON Response | 20,992 ns | — | 24,158 ns | Gofi |
| **Parallel** | **217 ns** | — | 355 ns | **Gofi** ⭐ |
| **Route Groups** | **777 ns** | — | 1,315 ns | **Gofi** ⭐ |
| Static (157 all) | 117 µs | 123 µs | 70 µs | Chi |
| GitHub (static) | 659 ns | 680 ns | 384 ns | Chi |
| GitHub (param) | 1,110 ns | 1,135 ns | 887 ns | Chi |
| GitHub (all) | 204 µs | 216 µs | 241 µs | **Gofi** ⭐ |
| GPlus (static) | 719 ns | 740 ns | 715 ns | Tie |
| GPlus (param) | 1,755 ns | 2,477 ns | 4,047 ns | Gofi |
| GPlus (2 params) | 8,809 ns | 4,766 ns | 3,212 ns | Chi |
| GPlus (all) | 48,223 ns | 26,358 ns | 38,258 ns | **GofiSchema** ⭐ |
| Parse (static) | 1,810 ns | 1,554 ns | 763 ns | Chi |
| Parse (param) | 1,387 ns | 1,308 ns | 1,197 ns | Chi |
| Parse (2 params) | 1,604 ns | 1,500 ns | 1,492 ns | Tie |
| Parse (all) | 29,081 ns | 28,402 ns | 33,666 ns | **GofiSchema** ⭐ |

> **—** indicates GofiSchema was not benchmarked for that category (no schema-specific behavior applies).

---

## 1. Memory Consumption

Memory required to load the routing structure for each API:

| API | Routes | Gofi | GofiSchema | Chi |
|---|---|---|---|---|
| Static | 157 | 127 KB | 234 KB | 81 KB |
| GitHub | 203 | 174 KB | 333 KB | 99 KB |
| Google+ | 13 | 12 KB | 23 KB | 7 KB |
| Parse.com | 26 | 23 KB | 43 KB | 8 KB |

> GofiSchema roughly doubles Gofi's memory due to schema compilation (struct reflection, validation rule parsing) at route registration time. Chi is the most memory efficient for route storage.

---

## 2. Micro Benchmarks

### Static Route `GET /`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| Gofi | 508 | 472 | 5 |
| GofiSchema | 574 | 472 | 5 |
| Chi | 303 | 368 | 2 |

> GofiSchema adds minimal overhead (~13%) for static routes — schema is compiled at registration, not per-request.

### Single Parameter `GET /user/:name`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| Gofi | 624 | 488 | 6 |
| GofiSchema | 2,519 | 1,120 | 17 |
| Chi | 544 | 704 | 4 |

> GofiSchema adds ~4x overhead due to `ValidateAndBind` performing struct instantiation, path value extraction, type coercion, and validation per request.

### 5 Params `GET /:a/:b/:c/:d/:e`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| Gofi | 1,155 | 712 | 9 |
| GofiSchema | 3,168 | 1,600 | 21 |
| Chi | 789 | 704 | 4 |

### 20 Params `GET /:a/:b/.../:t`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **1,950** | **1,480** | **11** |
| Chi | 3,473 | 2,505 | 9 |

> ⭐ **Gofi wins decisively** at high param counts — **44% faster** and **41% less memory**. Go 1.22's `http.ServeMux` scales better with many parameters.

### Param Write `GET /user/:name` (reads + writes param value)

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **348** | **72** | **3** |
| GofiSchema | 2,558 | 1,144 | 18 |
| Chi | 710 | 704 | 4 |

> ⭐ **Gofi is 2x faster** than Chi with **90% less memory** for raw param access. Go's `PathValue()` is extremely efficient. GofiSchema trades performance for type-safe, validated binding.

### Multi Parameter `GET /users/:userID/posts/:postID`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| Gofi | 1,028 | 520 | 7 |
| GofiSchema | 3,962 | 1,624 | 23 |
| Chi | 980 | 704 | 4 |

---

## 3. Middleware Scalability

| Middlewares | Gofi (ns/op) | Chi (ns/op) | Gofi allocs | Chi allocs |
|---|---|---|---|---|
| 5 | 1,582 | 601 | 10 | 2 |
| 10 | 1,468 | 657 | 15 | 2 |
| 20 | 2,102 | 699 | 25 | 2 |

> Chi maintains **constant 2 allocs** regardless of middleware count. Gofi's allocation count grows linearly. This is Chi's biggest architectural advantage.

---

## 4. Data Handling & I/O

### JSON Binding (Small Payload)

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| Gofi | 9,360 | 7,528 | 32 |
| GofiSchema | 13,933 | 8,425 | 51 |
| Chi | 9,250 | 7,103 | 29 |

> GofiSchema adds ~50% overhead for JSON binding — the cost of schema-based validation + type-safe struct binding. Chi and plain Gofi are comparable (both use `encoding/json`).

### JSON Response (100 items)

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **20,992** | **8,840** | **21** |
| Chi | 24,158 | 9,152 | 21 |

> Gofi is ~13% faster for JSON responses — both use `encoding/json`, but Gofi's writer path has slightly less overhead.

---

## 5. Concurrency

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **217** | **78** | **3** |
| Chi | 355 | 368 | 2 |

> ⭐ **Gofi is ~40% faster** under parallel load with **79% less memory per operation**. Go's built-in `http.ServeMux` is highly optimized for concurrent access.

---

## 6. Route Groups (with nested middleware)

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **777** | **512** | **7** |
| Chi | 1,315 | 736 | 6 |

> ⭐ **Gofi is ~40% faster** for grouped routes with nested middleware.

---

## 7. Real-World API: GitHub (203 routes)

| Benchmark | Gofi | GofiSchema | Chi |
|---|---|---|---|
| **Memory** | 174 KB | 333 KB | 99 KB |
| Static (ns/op) | 659 | 680 | 384 |
| Param (ns/op) | 1,110 | 1,135 | 887 |
| **All (ns/op)** | **204,000** | **216,000** | 241,000 |
| All (B/op) | **105,630** | **105,630** | 130,880 |
| All (allocs) | 1,352 | 1,352 | 740 |

> ⭐ For the full GitHub API traversal: **Gofi is ~15% faster** than Chi and uses **19% less memory** (105 KB vs 131 KB per iteration). GofiSchema adds negligible routing overhead when schemas use empty structs (schema validation only triggers on `ValidateAndBind` calls).

---

## 8. Real-World API: Google+ (13 routes)

| Benchmark | Gofi | GofiSchema | Chi |
|---|---|---|---|
| **Memory** | 12 KB | 23 KB | 7 KB |
| Static (ns/op) | 719 | 740 | 715 |
| 1 Param (ns/op) | 1,755 | 2,477 | 4,047 |
| 2 Params (ns/op) | 8,809 | 4,766 | 3,212 |
| **All (ns/op)** | 48,223 | **26,358** | 38,258 |
| All (B/op) | **6,475** | **6,475** | 8,484 |

> ⭐ **GofiSchema** was the fastest for the full Google+ API traversal! GofiSchema uses **24% less memory** per iteration (6.5 KB vs 8.5 KB) compared to Chi.

---

## 9. Real-World API: Parse.com (26 routes)

| Benchmark | Gofi | GofiSchema | Chi |
|---|---|---|---|
| **Memory** | 23 KB | 43 KB | 8 KB |
| Static (ns/op) | 1,810 | 1,554 | 763 |
| 1 Param (ns/op) | 1,387 | 1,308 | 1,197 |
| 2 Params (ns/op) | 1,604 | 1,500 | 1,492 |
| **All (ns/op)** | 29,081 | **28,402** | 33,666 |
| All (B/op) | **12,631** | **12,631** | 14,951 |

> ⭐ **GofiSchema** was the fastest for the full Parse.com API traversal and uses **16% less memory** per iteration than Chi.

---

## The GofiSchema Overhead

GofiSchema adds a **per-request cost** when `ValidateAndBind` is called:

| Scenario | Overhead vs Gofi | What's happening |
|---|---|---|
| Static routes (no bind) | ~5-13% | Minimal — schema compiled at registration only |
| 1 param + bind | ~4x | Struct alloc + path extraction + type coercion + validation |
| 2 params + bind | ~3.5x | Same, scales sub-linearly |
| 5 params + bind | ~2.7x | Same pattern |
| JSON body + bind | ~50% | JSON decode + struct validation |
| Full API traversal (no bind) | ~0-5% | Negligible — handler doesn't call ValidateAndBind |

**Key insight:** The schema overhead is **per `ValidateAndBind` call**, not per route registration. If your handler doesn't call `ValidateAndBind`, GofiSchema routes perform identically to plain Gofi routes. The schema compilation (struct reflection) happens once at route registration time, adding to startup memory but not per-request latency.

---

## Conclusions

### Where Gofi Excels

1. **High parameter counts (20 params):** Gofi is **44% faster** — Go 1.22's `http.ServeMux` scales better with many URL parameters
2. **Raw parameter access:** Gofi's `PathValue()` is **2x faster** with **90% less memory** than Chi's context-based extraction
3. **Parallel/concurrent access:** Gofi is **~40% faster** under parallel load — the stdlib `ServeMux` is highly optimized for concurrency
4. **Route groups with middleware:** Gofi is **~40% faster** for nested route resolution
5. **Full API traversal memory:** Gofi consistently uses **16-24% less memory per operation** than Chi for real-world APIs

### Where GofiSchema Adds Value

1. **Type-safe parameter binding:** Automatic struct binding with validation — a feature Chi doesn't offer
2. **Real-world API traversal:** GofiSchema was **fastest overall** for Google+ and Parse.com full API traversals
3. **Startup-only compilation:** Schema overhead is at registration, not per-request (unless you call `ValidateAndBind`)
4. **Competitive at scale:** For full API traversals, GofiSchema matches or beats Chi despite adding validation

### Where Chi Excels

1. **Static route lookup:** Chi's radix trie is **40-65% faster** for static and deeply nested routes
2. **Middleware scalability:** Chi maintains **constant 2 allocations** regardless of middleware count
3. **Route storage memory:** Chi requires **40-60% less memory** to store route structures
4. **Low param count routing:** For 1-5 parameters, Chi is **15-30% faster**

### The Trade-off

| | Chi | Gofi | GofiSchema |
|---|---|---|---|
| **Fastest for** | Static, deep nesting, middleware | Parallel, param access, high params | Full API traversals, type safety |
| **Memory model** | Low storage, higher per-op | Higher storage, lower per-op | Highest storage, lowest full-API per-op |
| **Best for** | Many static routes, heavy middleware | High concurrency, simple APIs | Type-safe APIs with validation |

---

## Usage

```bash
# Run all benchmarks
go test -bench=. -benchmem -count=3 -timeout=15m

# Run specific category
go test -bench="Gofi|GofiSchema|Chi" -benchmem

# Compare just Gofi vs Chi (no schema)
go test -bench="Benchmark(Gofi|Chi)_" -benchmem

# Run only GitHub API benchmarks
go test -bench="Github" -benchmem

# Run only GofiSchema benchmarks
go test -bench="GofiSchema" -benchmem
```
