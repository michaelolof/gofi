package gofi

import (
	"bufio"
	stdcontext "context"
	"errors"
	"log"
	"reflect"
	"time"
)

var errStreamPanicked = errors.New("gofi: stream producer panicked")

// errNoStreamBody signals a schema with no body for the status code; the
// stream is not started, matching SendStream's existing behaviour.
var errNoStreamBody = errors.New("gofi: schema defines no stream body")

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
	if err := c.prepareStream(code, obj); err != nil {
		if errors.Is(err, errNoStreamBody) {
			return nil
		}
		return err
	}
	return c.SetBodyStreamWriter(sw)
}

// prepareStream validates the schema for code and sets the response
// headers, cookies, content type and status before a stream starts. It
// returns errNoStreamBody when the schema defines no body for code.
func (c *context) prepareStream(code int, obj any) error {
	if c.rules() == nil {
		return newErrReport(ResponseErr, schemaBody, "", "required", errors.New("schema not properly registered to route handler"))
	}

	_, rules, err := c.rules().getRespRulesByCode(code)
	if err != nil {
		return err
	}

	if len(rules) == 0 {
		return errNoStreamBody
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
	return nil
}

// StreamOptions bounds a stream started with Stream or SendStreamWithOptions.
type StreamOptions struct {
	// MaxBytes stops the producer with ErrStreamLimit once it has written
	// this many bytes. Zero means unlimited.
	MaxBytes int64
	// MaxDuration cancels the producer's context after this long. Zero means
	// no limit beyond server shutdown.
	MaxDuration time.Duration
	// OnError receives the producer's terminal error, including client
	// disconnects and ErrStreamLimit. Without it, errors are logged.
	OnError func(err error)
}

// ErrStreamLimit is returned to a producer that writes past MaxBytes.
var ErrStreamLimit = errors.New("gofi: stream byte limit reached")

// Stream registers sw as the response body producer with bounded options.
// sw receives a context that is cancelled when the server shuts down or
// MaxDuration passes; producers should stop when it is done. As with
// SetBodyStreamWriter, Stream returns once the producer is registered.
func Stream(c Context, opts StreamOptions, sw func(ctx stdcontext.Context, w *bufio.Writer) error) error {
	if opts.MaxBytes < 0 || opts.MaxDuration < 0 {
		return errors.New("gofi: negative StreamOptions value")
	}
	if sw == nil {
		return errors.New("gofi: nil stream writer")
	}
	ctx, ok := c.(*context)
	if !ok {
		return errors.New("unknown context object passed")
	}
	if opts.OnError != nil {
		ctx.streamErrHandler = opts.OnError
	}
	parent := ctx.Context()
	return ctx.SetBodyStreamWriter(func(w *bufio.Writer) error {
		streamCtx, cancel := stdcontext.WithCancel(parent)
		if opts.MaxDuration > 0 {
			streamCtx, cancel = stdcontext.WithTimeout(parent, opts.MaxDuration)
		}
		defer cancel()
		if opts.MaxBytes == 0 {
			return sw(streamCtx, w)
		}
		limited := bufio.NewWriterSize(&streamLimitWriter{w: w, remaining: opts.MaxBytes}, w.Size())
		if err := sw(streamCtx, limited); err != nil {
			return err
		}
		return limited.Flush()
	})
}

// SendStreamWithOptions validates and sets schema headers for code, as
// SendStream does, then starts sw with Stream.
func SendStreamWithOptions(c Context, code int, obj any, opts StreamOptions, sw func(ctx stdcontext.Context, w *bufio.Writer) error) error {
	ctx, ok := c.(*context)
	if !ok {
		return errors.New("unknown context object passed")
	}
	if err := ctx.prepareStream(code, obj); err != nil {
		if errors.Is(err, errNoStreamBody) {
			return nil
		}
		return err
	}
	return Stream(c, opts, sw)
}

// streamLimitWriter forwards writes to the response and flushes them, failing
// once the byte budget is spent.
type streamLimitWriter struct {
	w         *bufio.Writer
	remaining int64
}

func (l *streamLimitWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > l.remaining {
		n, _ := l.w.Write(p[:l.remaining])
		l.remaining = 0
		_ = l.w.Flush()
		return n, ErrStreamLimit
	}
	n, err := l.w.Write(p)
	l.remaining -= int64(n)
	if err != nil {
		return n, err
	}
	return n, l.w.Flush()
}
