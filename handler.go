package gofi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"unicode/utf8"
)

// MiddlewareFunc is the unified middleware type for Gofi v2.
// Middlewares call c.Next() to proceed to the next handler in the chain.
type MiddlewareFunc = func(c Context) error

type Middlewares []MiddlewareFunc

type HandlerFunc = func(c Context) error

type RouteOptions struct {
	// Provide additional information about your route
	Info Info
	// Define a reference to your Schema struct
	Schema any
	// Attach meta information to your route handlers that can be accessed in using the Context or Router interface
	Meta any
	// Define the handler for your route
	Handler func(c Context) error
}

func DefineHandler(opts RouteOptions) RouteOptions {
	return opts
}

// maxPublicErrorMessage bounds the message the default error handler sends.
const maxPublicErrorMessage = 1024

// defaultErrorHandler renders err as a JSON envelope. Only messages meant for
// clients are sent: request validation errors and *HTTPError values with a 4xx
// code keep their message; every other error, including 5xx *HTTPError
// values, is sent as its generic status text and logged instead.
func defaultErrorHandler(err error, c Context) {
	code, msg := publicError(err)
	if code >= 500 {
		log.Printf("gofi: %d response for internal error: %s", code, truncateMessage(fmt.Sprint(err)))
	}

	// encoding/json escapes every control byte and replaces invalid UTF-8.
	encoded, _ := json.Marshal(msg)
	var buf []byte
	if prefix, ok := preComputedErrPrefix[code]; ok {
		buf = make([]byte, 0, len(prefix)+len(encoded)+1)
		buf = append(buf, prefix...)
	} else {
		buf = make([]byte, 0, 64+len(encoded))
		buf = append(buf, `{"status":"error","statusCode":`...)
		buf = strconv.AppendInt(buf, int64(code), 10)
		buf = append(buf, `,"message":`...)
	}
	buf = append(buf, encoded...)
	buf = append(buf, '}')

	if ctx, ok := c.(*context); ok {
		ctx.fctx.Response.Header.SetContentType("application/json")
	}
	if err := c.SendBytes(code, buf); err != nil {
		log.Println("gofi: error handler failed:", err)
	}
}

// publicError maps err to a response status and a message safe to send.
func publicError(err error) (int, string) {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr != nil && httpErr.Code > 0 {
		code := httpErr.Code
		if code < 400 || code > 599 {
			return http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)
		}
		if code < 500 {
			return code, truncateMessage(httpErr.Message)
		}
		return code, http.StatusText(code)
	}
	var report *errReport
	if errors.As(err, &report) && report.typ == RequestErr {
		return http.StatusBadRequest, truncateMessage(err.Error())
	}
	return http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)
}

func truncateMessage(msg string) string {
	if len(msg) <= maxPublicErrorMessage {
		return msg
	}
	cut := maxPublicErrorMessage
	for cut > 0 && !utf8.RuneStart(msg[cut]) {
		cut--
	}
	return msg[:cut] + "..."
}

// pre-computed error JSON templates for common status codes
var preComputedErrPrefix = func() map[int][]byte {
	m := make(map[int][]byte, 10)
	for _, code := range []int{400, 401, 403, 404, 405, 408, 409, 413, 422, 425, 500} {
		prefix := make([]byte, 0, 64)
		prefix = append(prefix, `{"status":"error","statusCode":`...)
		prefix = strconv.AppendInt(prefix, int64(code), 10)
		prefix = append(prefix, `,"message":`...)
		m[code] = prefix
	}
	return m
}()
