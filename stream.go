package gofi

import (
	"bufio"
	"errors"
	"reflect"
)

func (c *context) SetBodyStreamWriter(sw func(w *bufio.Writer) error) error {
	// We use channels to propagate any errors from the separated writer goroutine
	// managed internally by fasthttp back to the caller synchronosly.
	errCh := make(chan error, 1)

	c.fctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		errCh <- sw(w)
		close(errCh)
	})

	return <-errCh
}

func (c *context) SendStream(code int, obj any, sw func(w *bufio.Writer) error) error {
	if c.rules() == nil {
		return newErrReport(ResponseErr, schemaBody, "", "required", errors.New("schema not properly registered to route handler"))
	}

	_, rules, err := c.rules().getRespRulesByCode(code)
	if err != nil {
		return err
	}

	if len(rules) == 0 {
		return nil
	}

	if obj == nil {
		// TODO.  If there's is no response body defined, this should be fine
		return errors.New("undefined schema when calling the gofi Send function")
	}

	// Handle if object is a pointer
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return errors.New("bad response. invalid response type. response object must be a struct")
	}

	if err := c.validateAndEncodeHeaders(rules, rv.FieldByName(string(schemaHeaders))); err != nil {
		return err
	}

	if err := c.validateAndEncodeCookie(rules, rv.FieldByName(string(schemaCookies))); err != nil {
		return err
	}

	contentType := c.rules().respContent(code)
	c.fctx.Response.Header.Set("Content-Type", string(contentType))
	c.fctx.Response.SetStatusCode(code)
	return c.SetBodyStreamWriter(sw)
}
