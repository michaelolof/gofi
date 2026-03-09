package gofi

import (
	"io"
	"reflect"

	"github.com/valyala/fastjson"
)

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
