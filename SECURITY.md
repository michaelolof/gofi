# Security policy

## Reporting a vulnerability

Report vulnerabilities privately through GitHub: open the repository's **Security** tab and choose **Report a vulnerability**. Please do not open a public issue for a suspected vulnerability.

Include the gofi version or commit, the Go version, a minimal reproduction, and the impact you observed.

## Supported versions

Security fixes target the latest release. gofi requires Go 1.26 or later and builds against fasthttp v1.77.0; keep your Go toolchain on a current patch release, because several fixes ship in the standard library.

## Scope

gofi covers routing, request parsing and validation, response handling and its bundled middleware. Authentication, authorization, tenant isolation, outbound requests (SSRF) and business rules remain the application's responsibility; schema validation is not authorization. The maintained threat model and remediation record is in `gofi-docs/security-audit-2.md`.

## Checks

```sh
go vet ./...
go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
go test -run '^$' -fuzz '^FuzzErrorEnvelope$' -fuzztime=60s .
```
