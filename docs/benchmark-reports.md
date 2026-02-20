# Gofi vs Chi vs Echo — Benchmark Comparison

Comprehensive performance benchmark comparing [**Gofi**](https://github.com/michaelolof/gofi), [**Chi**](https://github.com/go-chi/chi), and [**Echo**](https://github.com/labstack/echo) HTTP routers for Go — each tested plain and with schema validation/binding.

**Six configurations tested:**
- **Gofi** — Go 1.22+ `http.ServeMux` wrapper
- **Gofi + Schema** — Gofi with typed schema structs + `ValidateAndBind`
- **Chi** — Standard Chi v5 radix trie router
- **Chi + Schema** — Chi with manual struct binding + `go-playground/validator`
- **Echo** — Echo v4 high-performance router
- **Echo + Schema** — Echo with `c.Bind()` + `c.Validate()` + validator

Full raw data: [benchmark-results.md](./benchmark-results.md)

---

## Memory Consumption

| API | Routes | Gofi | Gofi + Schema | Chi | Echo |
|---|---|---|---|---|---|
| Static | 157 | 91 KB | 346 KB | **78 KB** | 88 KB |
| GitHub | 203 | 135 KB | 423 KB | **91 KB** | 114 KB |
| Google+ | 13 | 10 KB | 30 KB | **6 KB** | 10 KB |
| Parse.com | 26 | 17 KB | 51 KB | **8 KB** | 13 KB |

> 🥇 **Chi** — consistently lowest memory for route storage
> 🥈 **Echo** — moderate footprint
> 🥉 **Gofi** — slightly higher than Echo. Gofi + Schema uses ~3.5x more due to schema compilation at registration

---

## Micro Benchmarks

### Static Route — `GET /`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Chi** | **319** | **368** | **2** |
| Chi + Schema | 389 | 370 | 3 |
| Gofi | 468 | 416 | 3 |
| Gofi + Schema | 614 | 432 | 5 |
| Echo + Schema | 13,892 | 424 | 4 |
| Echo | 14,220 | 424 | 4 |

> 🥇 **Chi** — 319 ns (32% faster than Gofi)
> 🥈 **Gofi** — 468 ns
> 🥉 **Echo** — 14,220 ns (higher single-route overhead, but excels in full API traversals)

### Single Param — `GET /user/:name`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **568** | **432** | **4** |
| Chi | 781 | 704 | 4 |
| Chi + Schema | 938 | 722 | 6 |
| Gofi + Schema | 2,515 | 1,504 | 19 |

> 🥇 **Gofi** — 568 ns (27% faster than Chi, 39% less memory)
> 🥈 **Chi** — 781 ns
> 🥉 **Chi + Schema** — 938 ns (only 20% overhead vs plain Chi)

### 5 Params — `GET /:a/:b/:c/:d/:e`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **979** | 656 | 7 |
| Chi | 1,110 | **704** | **4** |
| Chi + Schema | 1,575 | 786 | 6 |
| Gofi + Schema | 4,817 | 1,936 | 27 |

> 🥇 **Gofi** — 979 ns (12% faster than Chi)
> 🥈 **Chi** — 1,110 ns (constant 4 allocs regardless of param count)
> 🥉 **Chi + Schema** — 1,575 ns

### 20 Params — `GET /:a/:b/.../:t`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **2,075** | 1,424 | 9 |
| Chi | 3,583 | 2,504 | 9 |

> 🥇 **Gofi** — 2,075 ns (42% faster, 43% less memory)
> 🥈 **Chi** — 3,583 ns

### Param Write — `GET /user/:name` (reads param + writes to response)

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **252** | **16** | **1** |
| Chi | 746 | 704 | 4 |
| Chi + Schema | 905 | 720 | 5 |
| Gofi + Schema | 2,754 | 1,088 | 16 |

> 🥇 **Gofi** — 252 ns / 16 B / 1 alloc (**3x faster** than Chi, **98% less memory**)
> 🥈 **Chi** — 746 ns / 704 B
> 🥉 **Chi + Schema** — 905 ns

### Multi Param — `GET /users/:userID/posts/:postID`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **707** | **464** | 5 |
| Chi | 878 | 704 | **4** |
| Chi + Schema | 1,058 | 738 | 6 |
| Gofi + Schema | 2,953 | 1,568 | 21 |

> 🥇 **Gofi** — 707 ns (19% faster, 34% less memory)
> 🥈 **Chi** — 878 ns
> 🥉 **Chi + Schema** — 1,058 ns

### Wildcard — `GET /files/*`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Chi** | **836** | 704 | **4** |
| Gofi | 1,413 | **504** | 8 |

> 🥇 **Chi** — 836 ns (41% faster)
> 🥈 **Gofi** — 1,413 ns (28% less memory)

### Deep Nesting — `GET /v1/api/deep/nested/resource/action`

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Chi** | **388** | **368** | **2** |
| Gofi | 738 | 416 | 3 |

> 🥇 **Chi** — 388 ns (1.9x faster via single trie traversal)
> 🥈 **Gofi** — 738 ns

### 404 Handling

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Gofi** | **803** | **464** | **6** |
| Chi | 977 | 816 | 7 |
| Echo | 3,441 | 896 | 10 |

> 🥇 **Gofi** — 803 ns (18% faster, 43% less memory than Chi)
> 🥈 **Chi** — 977 ns
> 🥉 **Echo** — 3,441 ns

---

## Middleware Scalability

| Middlewares | Gofi | Chi | Echo | Gofi allocs | Chi allocs | Echo allocs |
|---|---|---|---|---|---|---|
| 5 | 644 | **628** | 869 | 3 | **2** | 9 |
| 10 | 1,265 | **699** | 909 | 3 | **2** | 14 |
| 20 | 912 | **600** | 1,274 | 3 | **2** | 24 |

> 🥇 **Chi** — constant 2 allocs, fastest at all counts
> 🥈 **Gofi** — constant 3 allocs, competitive at ×5
> 🥉 **Echo** — allocations grow linearly (9 → 14 → 24)

---

## Data Handling

### JSON Binding (Small Payload)

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| Echo | 5,752 | 7,478 | 31 |
| Chi + Schema | 6,126 | 7,105 | **29** |
| Echo + Schema | 6,875 | 7,482 | 31 |
| Chi | 6,905 | **7,101** | **29** |
| Gofi | 7,570 | 7,469 | 30 |
| Gofi + Schema | 11,574 | 8,367 | 49 |

> 🥇 **Echo** — 5,752 ns
> 🥈 **Chi + Schema** — 6,126 ns
> 🥉 **Echo + Schema** — 6,875 ns
>
> All within ~20% — bottleneck is `encoding/json`. Gofi + Schema has the most overhead (~53%) due to its richer binding pipeline.

### JSON Response (100 items)

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Echo** | **15,643** | 8,824 | 20 |
| Gofi | 18,740 | **8,777** | **19** |
| Chi | 21,590 | 9,147 | 21 |

> 🥇 **Echo** — 15,643 ns (16% faster than Gofi)
> 🥈 **Gofi** — 18,740 ns (least memory per op)
> 🥉 **Chi** — 21,590 ns

---

## Concurrency

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Echo** | **62** | **14** | **1** |
| Gofi | 141 | 22 | 1 |
| Chi | 363 | 368 | 2 |

> 🥇 **Echo** — 62 ns (2.3x faster than Gofi, 5.9x faster than Chi)
> 🥈 **Gofi** — 141 ns (single-allocation concurrency)
> 🥉 **Chi** — 363 ns

---

## Route Groups (nested middleware)

| Router | ns/op | B/op | allocs/op |
|---|---|---|---|
| **Echo** | **729** | 472 | 7 |
| Gofi | 917 | **432** | **4** |
| Chi | 1,644 | 736 | 6 |

> 🥇 **Echo** — 729 ns (20% faster than Gofi)
> 🥈 **Gofi** — 917 ns (least memory and allocs)
> 🥉 **Chi** — 1,644 ns

---

## Real-World APIs

### GitHub API (203 routes)

| Benchmark | Gofi | Gofi + Schema | Chi | Echo | Echo + Schema |
|---|---|---|---|---|---|
| Memory | 135 KB | 423 KB | **91 KB** | 114 KB | 115 KB |
| Static (ns/op) | 814 | 673 | **465** | 592 | 654 |
| Param (ns/op) | 916 | 868 | 937 | **867** | 994 |
| **All (ns/op)** | 276,279 | 229,135 | 257,763 | **124,922** | 590,463 |
| All (B/op) | 94,232 | 94,233 | 130,866 | **86,105** | **86,105** |
| All (allocs) | 946 | 946 | **740** | 812 | 812 |

> 🥇 **Echo** — 124,922 ns full traversal (2x faster, least memory per iteration at 86 KB)
> 🥈 **Gofi + Schema** — 229,135 ns (faster than both plain Gofi and Chi)
> 🥉 **Chi** — 257,763 ns (fewest allocs at 740, fastest for static lookups)

### Google+ API (13 routes)

| Benchmark | Gofi | Gofi + Schema | Chi | Echo | Echo + Schema |
|---|---|---|---|---|---|
| Memory | 10 KB | 30 KB | **6 KB** | 10 KB | 10 KB |
| Static (ns/op) | 1,043 | 1,208 | 1,407 | 1,082 | **1,036** |
| 1 Param (ns/op) | 2,208 | 3,784 | 3,329 | 1,862 | **1,122** |
| 2 Params (ns/op) | 5,054 | 4,932 | 4,638 | 1,726 | **1,187** |
| **All (ns/op)** | 64,994 | 110,089 | 101,901 | **7,577** | 11,608 |
| All (B/op) | 5,746 | 5,746 | 8,483 | **5,514** | **5,514** |

> 🥇 **Echo** — 7,577 ns full traversal (8.6x faster than Gofi, least memory per iteration)
> 🥈 **Echo + Schema** — 11,608 ns (fastest for individual param lookups)
> 🥉 **Gofi** — 64,994 ns

### Parse.com API (26 routes)

| Benchmark | Gofi | Gofi + Schema | Chi | Echo | Echo + Schema |
|---|---|---|---|---|---|
| Memory | 17 KB | 51 KB | **8 KB** | 13 KB | 13 KB |
| Static (ns/op) | 5,484 | 6,207 | 4,696 | **1,045** | 1,239 |
| 1 Param (ns/op) | 8,362 | 8,256 | 8,774 | 1,095 | **755** |
| 2 Params (ns/op) | 12,052 | 29,341 | 19,587 | 1,239 | **601** |
| **All (ns/op)** | 358,621 | 210,491 | 233,078 | 24,826 | **16,729** |
| All (B/op) | 11,173 | 11,173 | 14,949 | **11,028** | **11,028** |

> 🥇 **Echo + Schema** — 16,729 ns full traversal (14x faster than Gofi)
> 🥈 **Echo** — 24,826 ns
> 🥉 **Gofi + Schema** — 210,491 ns

---

## Schema Overhead

The cost of adding schema validation/binding to each router:

| Scenario | Gofi | Gofi + Schema | Chi | Chi + Schema | Echo | Echo + Schema |
|---|---|---|---|---|---|---|
| Static | 468 ns | 614 ns (1.3x) | 319 ns | 389 ns (1.2x) | 14,220 ns | 13,892 ns (1.0x) |
| 1 param | 568 ns | 2,515 ns (**4.4x**) | 781 ns | 938 ns (1.2x) | 15,288 ns | 38,383 ns (2.5x) |
| 5 params | 979 ns | 4,817 ns (**4.9x**) | 1,110 ns | 1,575 ns (1.4x) | 4,379 ns | 23,150 ns (**5.3x**) |
| JSON bind | 7,570 ns | 11,574 ns (1.5x) | 6,905 ns | 6,126 ns (0.9x) | 5,752 ns | 6,875 ns (1.2x) |

> 🥇 **Chi + Schema** — lowest overhead (~1.2x) via direct struct assignment without reflection
> 🥈 **Echo + Schema** — moderate overhead (~1.2x for JSON, higher for params)
> 🥉 **Gofi + Schema** — highest per-request overhead (~4-5x for params) — the cost of **automatic, type-safe binding**

---

## Key Takeaways

### Gofi excels at:
- **Parameterized routing** — 🥇 for 1-20 params in isolated benchmarks (27-42% faster than Chi)
- **Raw param access** — 🥇 at 252 ns / 16 B / 1 alloc (3x faster than Chi)
- **404 handling** — 🥇 fastest unmatched route resolution
- **Low per-operation memory** — uses 25-34% less memory than Chi per request
- **Gofi + Schema** — automatic `ValidateAndBind` with type safety (unique feature)

### Chi excels at:
- **Static & deep nested routes** — 🥇 fastest trie lookup (32-47% faster than Gofi)
- **Middleware scalability** — 🥇 constant 2 allocs (best in class)
- **Route storage memory** — 🥇 40-60% less than Gofi at registration
- **Schema overhead** — 🥇 Chi + Schema adds only ~20% (direct struct assignment)

### Echo excels at:
- **Concurrency** — 🥇 2.3x faster than Gofi, 5.9x faster than Chi
- **Full API traversal** — 🥇 dominates GitHub, Google+, Parse.com (2-14x faster)
- **JSON handling** — 🥇 fastest for both bind and response
- **Route groups** — 🥇 20% faster than Gofi

### The trade-off:

| | Gofi | Chi | Echo |
|---|---|---|---|
| **Fastest for** | Params, raw access, 404s | Static, deep nesting, middleware | API traversal, concurrency, JSON |
| **Memory model** | Low per-request, moderate storage | Low storage, higher per-request | Variable per-request, moderate storage |
| **Schema cost** | ~4.5x (full auto ValidateAndBind) | ~1.2x (manual struct binding) | ~2.5x (built-in Bind + Validate) |
| **Best for** | Type-safe APIs with validation | Many static routes, heavy middleware | High-throughput API servers |

---

## Running Benchmarks

```bash
# Run all benchmarks and generate benchmark-results.md
go run ./cmd/report/

# Run benchmarks manually
go test -bench=. -benchmem -count=3 -timeout=20m

# Run specific category
go test -bench="Github" -benchmem

# Run only schema benchmarks
go test -bench="GofiS|ChiS|EchoS" -benchmem
```
