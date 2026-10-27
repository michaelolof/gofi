package gofi

import (
	"fmt"
	"net/http"

	"github.com/valyala/fastjson"
)

// RequestLimits bounds the structure of request bodies, independently of
// their byte size. Zero fields use the defaults from DefaultRequestLimits;
// negative values are invalid.
type RequestLimits struct {
	// MaxJSONDepth bounds object/array nesting. Default: 64.
	MaxJSONDepth int
	// MaxJSONFields bounds object members across the whole body. Default: 10000.
	MaxJSONFields int
	// MaxJSONArrayLength bounds the elements of any one array. Default: 10000.
	MaxJSONArrayLength int
	// MaxFormValues bounds URL-encoded form values. Default: 1000.
	MaxFormValues int
	// MaxMultipartParts bounds multipart values plus files. Default: 100.
	MaxMultipartParts int
}

// DefaultRequestLimits returns the limits applied when none are configured.
func DefaultRequestLimits() RequestLimits {
	return RequestLimits{
		MaxJSONDepth:       64,
		MaxJSONFields:      10000,
		MaxJSONArrayLength: 10000,
		MaxFormValues:      1000,
		MaxMultipartParts:  100,
	}
}

// normalize fills zero fields with defaults and rejects negative values.
func (l RequestLimits) normalize() (RequestLimits, error) {
	d := DefaultRequestLimits()
	fields := []struct {
		name string
		v    *int
		def  int
	}{
		{"MaxJSONDepth", &l.MaxJSONDepth, d.MaxJSONDepth},
		{"MaxJSONFields", &l.MaxJSONFields, d.MaxJSONFields},
		{"MaxJSONArrayLength", &l.MaxJSONArrayLength, d.MaxJSONArrayLength},
		{"MaxFormValues", &l.MaxFormValues, d.MaxFormValues},
		{"MaxMultipartParts", &l.MaxMultipartParts, d.MaxMultipartParts},
	}
	for _, f := range fields {
		if *f.v < 0 {
			return l, fmt.Errorf("gofi: RequestLimits.%s must not be negative", f.name)
		}
		if *f.v == 0 {
			*f.v = f.def
		}
	}
	return l, nil
}

func newLimitError(what string) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, "request body exceeds the "+what+" limit")
}

// requestLimits returns the limits configured on the router behind ctx.
func requestLimits(ctx ParserContext) RequestLimits {
	if p, ok := ctx.(*parserContext); ok && p.c.serverOpts != nil {
		return p.c.serverOpts.limits
	}
	return DefaultRequestLimits()
}

// checkJSONStructure walks a parsed body once and enforces the depth, field
// and array limits before any binding work.
func checkJSONStructure(v *fastjson.Value, l RequestLimits) error {
	fields := 0
	var walk func(v *fastjson.Value, depth int) error
	walk = func(v *fastjson.Value, depth int) error {
		if depth > l.MaxJSONDepth {
			return newLimitError("nesting depth")
		}
		switch v.Type() {
		case fastjson.TypeObject:
			o, _ := v.Object()
			var err error
			o.Visit(func(_ []byte, child *fastjson.Value) {
				if err != nil {
					return
				}
				fields++
				if fields > l.MaxJSONFields {
					err = newLimitError("field count")
					return
				}
				err = walk(child, depth+1)
			})
			return err
		case fastjson.TypeArray:
			items, _ := v.Array()
			if len(items) > l.MaxJSONArrayLength {
				return newLimitError("array length")
			}
			for _, item := range items {
				if err := walk(item, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(v, 1)
}
