package gofi_test

import (
	"testing"

	"github.com/michaelolof/gofi"
	"github.com/valyala/fasthttp"
)

// Benchmarks for full ValidateAndBind requests (path, query, header and JSON
// body), dispatched through the router as a live request would be. IP-04
// records these before and after changing bound-result ownership.

type benchSmallSchema struct {
	Request struct {
		Path struct {
			ID string `json:"id" validate:"required"`
		}
		Query struct {
			Page int `json:"page"`
		}
		Header struct {
			Token string `json:"x-token" validate:"required"`
		}
		Body struct {
			Name  string `json:"name" validate:"required"`
			Email string `json:"email" validate:"required,email"`
		}
	}
}

type benchAddress struct {
	Street  string `json:"street" validate:"required"`
	City    string `json:"city" validate:"required"`
	Country string `json:"country" validate:"required"`
	Zip     string `json:"zip"`
}

type benchLargeSchema struct {
	Request struct {
		Path struct {
			OrgID  string `json:"org" validate:"required"`
			UserID string `json:"id" validate:"required"`
		}
		Query struct {
			Page    int    `json:"page"`
			PerPage int    `json:"per_page"`
			Sort    string `json:"sort"`
		}
		Header struct {
			Token     string `json:"x-token" validate:"required"`
			RequestID string `json:"x-request-id"`
		}
		Body struct {
			Name     string         `json:"name" validate:"required"`
			Email    string         `json:"email" validate:"required,email"`
			Age      int            `json:"age" validate:"gte=0"`
			Active   bool           `json:"active"`
			Score    float64        `json:"score"`
			Bio      string         `json:"bio"`
			Website  string         `json:"website"`
			Phone    string         `json:"phone"`
			Tags     []string       `json:"tags"`
			Roles    []string       `json:"roles"`
			Address  benchAddress   `json:"address"`
			Previous []benchAddress `json:"previous"`
		}
	}
}

const benchLargeBody = `{"name":"Ada Lovelace","email":"ada@example.com","age":36,"active":true,"score":99.5,` +
	`"bio":"Mathematician and writer","website":"https://example.com","phone":"+44 20 7946 0000",` +
	`"tags":["math","computing","poetry"],"roles":["admin","author"],` +
	`"address":{"street":"12 St James's Square","city":"London","country":"UK","zip":"SW1Y 4JH"},` +
	`"previous":[{"street":"1 Main St","city":"Leeds","country":"UK"},{"street":"2 High St","city":"York","country":"UK"}]}`

func benchmarkBind[T any](b *testing.B, pattern, uri, body string) {
	r := gofi.NewRouter()
	r.Post(pattern, gofi.RouteOptions{Schema: new(T), Handler: func(c gofi.Context) error {
		if _, err := gofi.ValidateAndBind[T](c); err != nil {
			return err
		}
		return c.SendString(200, "ok")
	}})
	handler := r.Handler()

	var fctx fasthttp.RequestCtx
	fctx.Request.Header.SetMethod("POST")
	fctx.Request.SetRequestURI(uri)
	fctx.Request.Header.SetContentType("application/json")
	fctx.Request.Header.Set("X-Token", "secret-token")
	fctx.Request.Header.Set("X-Request-Id", "req-123")
	fctx.Request.SetBodyString(body)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fctx.Response.Reset()
		handler(&fctx)
		if fctx.Response.StatusCode() != 200 {
			b.Fatalf("unexpected status %d: %s", fctx.Response.StatusCode(), fctx.Response.Body())
		}
	}
}

func BenchmarkValidateAndBindSmall(b *testing.B) {
	benchmarkBind[benchSmallSchema](b, "/users/:id", "/users/42?page=2",
		`{"name":"Ada Lovelace","email":"ada@example.com"}`)
}

func BenchmarkValidateAndBindLarge(b *testing.B) {
	benchmarkBind[benchLargeSchema](b, "/orgs/:org/users/:id", "/orgs/acme/users/42?page=2&per_page=50&sort=name",
		benchLargeBody)
}
