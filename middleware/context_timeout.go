package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/michaelolof/gofi"
)

// ContextTimeout runs the rest of the chain on the same goroutine with a Go
// context deadline of d. Handlers observe it through c.Context() and should
// pass that context to database, HTTP and other blocking calls. The deadline is
// cooperative: a handler that ignores c.Context() is not interrupted. The
// derived context is also cancelled when the server shuts down.
//
// If the chain returns an error caused by the deadline, the middleware returns
// a 503 *gofi.HTTPError. When the chain returns, the previous context's deadline
// and cancellation are restored, and values set further down the chain stay
// visible, so an error handler can log them. Unlike Timeout, no goroutine or
// response copy is involved, so it is safe with streaming and WebSocket routes.
func ContextTimeout(d time.Duration) (gofi.MiddlewareFunc, error) {
	if d <= 0 {
		return nil, errors.New("gofi/middleware: ContextTimeout requires a positive duration")
	}
	return func(c gofi.Context) error {
		parent := c.Context()
		ctx, cancel := context.WithTimeout(parent, d)
		defer cancel()
		if err := gofi.SetRequestContext(c, ctx); err != nil {
			return err
		}
		defer func() { _ = gofi.SetRequestContext(c, unwoundContext{Context: parent, values: c.Context()}) }()

		err := c.Next()
		if err != nil && errors.Is(err, context.DeadlineExceeded) && ctx.Err() == context.DeadlineExceeded {
			return gofi.NewHTTPError(http.StatusServiceUnavailable, http.StatusText(http.StatusServiceUnavailable))
		}
		return err
	}, nil
}

// unwoundContext takes its deadline and cancellation from the context it embeds,
// and its values from the one the chain finished with.
type unwoundContext struct {
	context.Context
	values context.Context
}

func (c unwoundContext) Value(key any) any { return c.values.Value(key) }
