# Working on Gofi

This file is for people and coding agents changing the Gofi library itself.
If you are writing an application that uses Gofi, read
[GUIDELINES.md](GUIDELINES.md) instead.

## Repositories

Gofi is developed across three sibling checkouts:

- `gofi`: this library.
- `gofi-test-utils`: integration suites, benchmarks, examples, and the
  reference service. Its `go.mod` replaces Gofi with `../gofi`, so it always
  tests your working copy.
- `gofi-docs`: design notes, plans and audits. Not shipped.

## Checks to run

In `gofi`:

```sh
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

Fuzz targets live next to the code they cover (`func Fuzz...`). Run the one
for the area you changed, for example:

```sh
go test -run '^$' -fuzz FuzzBindJSONBody -fuzztime 30s .
```

In `gofi-test-utils`:

```sh
go test ./internal/suites/...   # integration suites against your gofi checkout
make reference                  # the reference service from GUIDELINES.md
make bench                      # routing microbenchmarks, for performance changes
```

## Keeping the documentation true

[GUIDELINES.md](GUIDELINES.md) is the contract for how applications use Gofi,
and its examples come from `gofi-test-utils/examples/reference-service`.

- A change to public behavior or API updates, in the same change:
  GUIDELINES.md, README.md, the affected file in `docs/`, and the reference
  service.
- Every Go code block in the docs must compile against the current API. Do
  not document a method without checking its signature in the code.
- `make reference` must pass.

## Conventions

- Hot paths (routing, binding, encoding) are allocation-sensitive. Keep the
  context pooling and reset patterns, and avoid adding reflection on the
  request path. Run the benchmarks before and after changing them.
- Registration-time mistakes should fail at registration (panic or error),
  not silently at request time.
- Commit messages are one lowercase line, `type: implemented <summary>`,
  for example `fix: implemented catch-all boundary fix`.
