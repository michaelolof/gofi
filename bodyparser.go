package gofi

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"

	"github.com/valyala/fastjson"
)

// ReadBodyLimited reads all of r, up to limit bytes. If r holds more than
// limit bytes it returns an *HTTPError with status 413 instead of silently
// truncating, so a valid prefix is never accepted on its own. Custom body
// parsers should use it (or LimitBody) rather than io.LimitReader.
func ReadBodyLimited(r io.Reader, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	if err := LimitBody(&buf, r, limit); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LimitBody copies r into buf, failing with a 413 *HTTPError if r holds more
// than limit bytes. A negative limit is a configuration error.
func LimitBody(buf *bytes.Buffer, r io.Reader, limit int64) error {
	if limit < 0 {
		return fmt.Errorf("gofi: negative body size limit %d", limit)
	}
	probe := limit
	if probe < math.MaxInt64 {
		probe++ // read one byte past the limit to detect overflow
	}
	if _, err := buf.ReadFrom(io.LimitReader(r, probe)); err != nil {
		return err
	}
	if int64(buf.Len()) > limit {
		return newBodyTooLargeError()
	}
	return nil
}

func newBodyTooLargeError() *HTTPError {
	return NewHTTPError(http.StatusRequestEntityTooLarge, http.StatusText(http.StatusRequestEntityTooLarge))
}

type RequestOptions struct {
	SchemaRules *RuleDef
	ShouldBind  bool
	Context     ParserContext
	SchemaPtr   any
	Body        *reflect.Value
}

type ResponseOptions struct {
	Context     ParserContext
	SchemaRules *RuleDef
	Body        reflect.Value
}

type BodyParser interface {
	Match(contentType string) bool
	ValidateAndDecodeRequest(r io.ReadCloser, opts RequestOptions) error
	ValidateAndEncodeResponse(s any, opts ResponseOptions) ([]byte, error)
}

// ParserContext provides access to request/response context for body parsers.
type ParserContext interface {
	Writer() ResponseWriter
	Request() *Request
	CustomSpecs() CustomSpecs
	getParser() *fastjson.Parser
}

type parserContext struct {
	c *context
}

func (p *parserContext) Writer() ResponseWriter {
	return p.c.Writer()
}

func (p *parserContext) Request() *Request {
	return p.c.Request()
}

func (p *parserContext) CustomSpecs() CustomSpecs {
	return p.c.serverOpts.customSpecs
}

func (p *parserContext) getParser() *fastjson.Parser {
	return p.c.getParser()
}
