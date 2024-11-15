package gofi

import "net/http"

type Middlewares []func(http.Handler) http.Handler

func (s *ServeMux) SetErrorHandler(handler func(err error, c Context)) {
	s.errHandler = handler
}
