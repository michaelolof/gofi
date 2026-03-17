package gofi

// HTTPError is a structured error that carries an HTTP status code.
// Handlers and middlewares can return this to let the default error handler
// preserve an explicit response status.
type HTTPError struct {
	Code    int
	Message string
}

func (e *HTTPError) Error() string {
	return e.Message
}

// NewHTTPError creates a new HTTPError.
func NewHTTPError(code int, message string) *HTTPError {
	return &HTTPError{Code: code, Message: message}
}
