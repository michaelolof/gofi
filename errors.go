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

// transportError exposes a safe HTTP error while retaining the read failure.
// Keep HTTPError unchanged so existing constructors and struct literals work.
type transportError struct {
	*HTTPError
	cause error
}

func (e *transportError) Unwrap() []error {
	return []error{e.HTTPError, e.cause}
}
