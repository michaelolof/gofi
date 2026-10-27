package gofi

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"

	"github.com/valyala/fasthttp"
)

// maxPanicSummary bounds the panic value written to the internal log.
const maxPanicSummary = 1024

var genericInternalErrorBody = []byte(`{"status":"error","statusCode":500,"message":"Internal Server Error"}`)

// logRecoveredPanic records a recovered panic for operators. The panic value is
// truncated and never sent to the client.
func logRecoveredPanic(where string, r any) {
	summary := fmt.Sprintf("%v", r)
	if len(summary) > maxPanicSummary {
		summary = summary[:maxPanicSummary] + "...(truncated)"
	}
	log.Printf("gofi: recovered panic in %s: %s\n%s", where, summary, debug.Stack())
}

// recoverDispatch is deferred around live dispatch. It contains a handler,
// middleware or error-handler panic, answers with a generic 500 when the
// response can still be replaced, and closes the connection because request
// state may be inconsistent. The panicking context is deliberately not
// returned to the pool.
func recoverDispatch(ctx *fasthttp.RequestCtx, where string) {
	r := recover()
	if r == nil {
		return
	}
	logRecoveredPanic(where, r)
	writeInternalServerError(ctx)
}

// writeInternalServerError replaces an uncommitted response with a generic
// 500 and closes the connection.
func writeInternalServerError(ctx *fasthttp.RequestCtx) {
	if ctx.Hijacked() {
		return
	}
	ctx.Response.Reset()
	ctx.Response.SetStatusCode(http.StatusInternalServerError)
	ctx.Response.Header.SetContentType("application/json")
	ctx.Response.SetBody(genericInternalErrorBody)
	ctx.SetConnectionClose()
}
