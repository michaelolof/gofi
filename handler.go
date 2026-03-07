package gofi

import (
	"log"
	"strconv"
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

func defaultErrorHandler(err error, c Context) {
	// Write JSON error response directly without encoding/json
	msg := err.Error()
	var buf []byte
	if prefix, ok := preComputedErrPrefix[500]; ok {
		buf = make([]byte, 0, len(prefix)+len(msg)+4)
		buf = append(buf, prefix...)
	} else {
		buf = make([]byte, 0, 128)
		buf = append(buf, `{"status":"error","statusCode":500,"message":"`...)
	}
	// Simple JSON string escape for the error message
	for i := 0; i < len(msg); i++ {
		switch msg[i] {
		case '"':
			buf = append(buf, '\\', '"')
		case '\\':
			buf = append(buf, '\\', '\\')
		case '\n':
			buf = append(buf, '\\', 'n')
		case '\r':
			buf = append(buf, '\\', 'r')
		case '\t':
			buf = append(buf, '\\', 't')
		default:
			buf = append(buf, msg[i])
		}
	}
	buf = append(buf, `"}`...)

	if err := c.SendBytes(500, buf); err != nil {
		log.Println("gofi: error handler failed:", err)
	}
}

// pre-computed error JSON templates for common status codes
var preComputedErrPrefix = func() map[int][]byte {
	m := make(map[int][]byte, 8)
	for _, code := range []int{400, 401, 403, 404, 405, 409, 422, 500} {
		prefix := make([]byte, 0, 64)
		prefix = append(prefix, `{"status":"error","statusCode":`...)
		prefix = strconv.AppendInt(prefix, int64(code), 10)
		prefix = append(prefix, `,"message":"`...)
		m[code] = prefix
	}
	return m
}()
