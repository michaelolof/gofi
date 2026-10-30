package gofi

import (
	"errors"
	"net/http"
	"strings"
)

// SnapshotOptions selects what SnapshotRequest copies.
type SnapshotOptions struct {
	// IncludeBody copies the request body into the snapshot.
	IncludeBody bool
	// MaxBodyBytes bounds the copied body when IncludeBody is set. A body
	// larger than this makes SnapshotRequest fail with a 413 *HTTPError.
	// Zero means 1 MiB; it must not be negative.
	MaxBodyBytes int64
}

// RequestSnapshot is an immutable copy of request data for work that outlives
// the handler, such as a background job. Every field owns its memory. It holds
// no response writer, handler chain, connection, data store or bound result.
type RequestSnapshot struct {
	Method     string
	Path       string
	RawQuery   string
	Pattern    string
	Host       string
	RemoteAddr string
	TLS        bool
	Params     Params
	Header     http.Header
	Body       []byte
}

const defaultSnapshotBodyBytes = 1 << 20

// SnapshotRequest copies the request data of c into a RequestSnapshot that is
// safe to read from any goroutine after the handler returns.
func SnapshotRequest(c Context, opts SnapshotOptions) (*RequestSnapshot, error) {
	ctx, ok := c.(*context)
	if !ok {
		return nil, errors.New("unknown context object passed")
	}
	if opts.MaxBodyBytes < 0 {
		return nil, errors.New("gofi: negative SnapshotOptions.MaxBodyBytes")
	}

	fctx := ctx.fctx
	snap := &RequestSnapshot{
		Method:     string(fctx.Method()),
		Path:       string(fctx.Path()),
		RawQuery:   string(fctx.URI().QueryString()),
		Pattern:    ctx.opts.Pattern,
		Host:       string(fctx.Host()),
		RemoteAddr: fctx.RemoteAddr().String(),
		TLS:        fctx.IsTLS(),
		Header:     make(http.Header),
	}
	// The pattern can be a view of the raw request path on 404/405 chains.
	snap.Pattern = strings.Clone(snap.Pattern)

	if len(ctx.params) > 0 {
		snap.Params = make(Params, len(ctx.params))
		for i, p := range ctx.params {
			snap.Params[i] = Param{Key: p.Key, Value: strings.Clone(p.Value)}
		}
	}
	for key, value := range fctx.Request.Header.All() {
		snap.Header.Add(string(key), string(value))
	}

	if opts.IncludeBody {
		limit := opts.MaxBodyBytes
		if limit == 0 {
			limit = defaultSnapshotBodyBytes
		}
		body := fctx.PostBody()
		if int64(len(body)) > limit {
			return nil, newBodyTooLargeError()
		}
		snap.Body = append([]byte(nil), body...)
	}
	return snap, nil
}
