package gofi

import (
	"io"
	"net/http"
	"reflect"
)

type RequestOptions struct {
	// SchemaField schemaField // Needed for error reporting
	SchemaRules *RuleDef // Needed for recursion
	ShouldBind  bool
	Context     ParserContext
	SchemaPtr   any
	Body        *reflect.Value
}

type ResponseOptions struct {
	Context     ParserContext
	SchemaRules *RuleDef // Needed for validation
	Body        reflect.Value
}

type BodyParser interface {
	Match(contentType string) bool
	ValidateAndDecodeRequest(r io.ReadCloser, opts RequestOptions) error
	ValidateAndEncodeResponse(s any, opts ResponseOptions) ([]byte, error)
}

type ParserContext interface {
	Writer() http.ResponseWriter
	Request() *http.Request
	CustomSpecs() CustomSpecs
}

type parserContext struct {
	c *context
}

func (p *parserContext) Writer() http.ResponseWriter {
	return p.c.Writer()
}

func (p *parserContext) Request() *http.Request {
	return p.c.Request()
}

func (p *parserContext) CustomSpecs() CustomSpecs {
	return p.c.serverOpts.customSpecs
}
