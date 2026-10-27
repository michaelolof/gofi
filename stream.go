package gofi

import (
	"bufio"
	"errors"
	"log"
	"reflect"
)

var errStreamPanicked = errors.New("gofi: stream producer panicked")

// SetBodyStreamWriter registers sw as the response body producer and returns
// without waiting for it. fasthttp runs sw on its own goroutine and streams
// what it writes to the client as it is flushed. The context, including path
// parameters and any bound result, stays valid for sw until sw returns; do not
// use it after that. Because sw finishes after the handler has returned, its
// error cannot be returned here: register OnStreamError to receive it,
// otherwise it is logged.
func (c *context) SetBodyStreamWriter(sw func(w *bufio.Writer) error) error {
	if sw == nil {
		return errors.New("gofi: nil stream writer")
	}

	onErr := c.streamErrHandler
	c.refs.Add(1)
	c.fctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		defer c.releaseRef()
		if err := runStreamProducer(sw, w); err != nil {
			reportStreamError(onErr, err)
		}
	})
	return nil
}

// runStreamProducer runs sw, converting a panic into errStreamPanicked. The
// producer runs outside the dispatch recovery boundary.
func runStreamProducer(sw func(w *bufio.Writer) error, w *bufio.Writer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logRecoveredPanic("stream producer", r)
			err = errStreamPanicked
		}
	}()
	return sw(w)
}

func reportStreamError(onErr func(error), err error) {
	if onErr == nil {
		log.Printf("gofi: stream producer ended with error: %v", err)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logRecoveredPanic("stream error handler", r)
		}
	}()
	onErr(err)
}

// OnStreamError registers fn to receive the terminal error of stream
// producers that c starts afterwards with SetBodyStreamWriter or SendStream.
// Producers finish after the handler has returned, so their errors (including
// a client disconnect or a recovered panic) cannot be returned from it.
func OnStreamError(c Context, fn func(err error)) error {
	ctx, ok := c.(*context)
	if !ok {
		return errors.New("unknown context object passed")
	}
	ctx.streamErrHandler = fn
	return nil
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
