package gofi

import (
	"encoding/json"
	"log"
	"net/http"
)

type Middlewares []func(http.Handler) http.Handler

type HandlerFunc = func(c Context) error

type PreHandler = func(next HandlerFunc) HandlerFunc

type RouteOptions struct {
	// Provide additional information about your route
	Info Info
	// Define a reference to your Schema struct
	Schema any
	// Attach meta information to your route handlers that can be accessed in using the Context or Router interface
	Meta any
	// Register middleware functions for your route
	PreHandlers []PreHandler
	// Define the handler for your route
	Handler func(c Context) error
}

func DefineHandler(opts RouteOptions) RouteOptions {
	return opts
}

func applyMiddleware(handler HandlerFunc, middleware []PreHandler) HandlerFunc {
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}

type defaultErrResp struct {
	Status     string `json:"status"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

func defaultErrorHandler(err error, c Context) {
	c.Writer().Header().Set("content-type", "application/json; charset-utf8")
	c.Writer().WriteHeader(500)
	bs, err := json.Marshal(defaultErrResp{
		Status:     "error",
		StatusCode: 500,
		Message:    err.Error(),
	})
	if err != nil {
		log.Fatalln(err)
	}

	c.Writer().Write(bs)
}
