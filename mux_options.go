package gofi

import (
	"fmt"
	"strings"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/validators/rules"
)

type muxOptions struct {
	errHandler       func(err error, c Context)
	customValidators rules.ContextValidators
	customSpecs      CustomSpecs
	bodyParsers      []BodyParser
	logger           Logger
	schemaRules      SchemaRulesMap
}

func defaultMuxOptions() *muxOptions {
	bp := make([]BodyParser, 0, 10)
	bp = append(bp, &JSONBodyParser{})

	return &muxOptions{
		errHandler:       defaultErrorHandler,
		customValidators: make(rules.ContextValidators),
		customSpecs:      make(CustomSpecs),
		bodyParsers:      bp,
		logger:           &consoleLogger{},
		schemaRules:      make(SchemaRulesMap),
	}
}

func (m *muxOptions) getSerializer(contentType cont.ContentType) (BodyParser, error) {
	for _, bp := range m.bodyParsers {
		if bp.Match(string(contentType)) {
			return bp, nil
		}
	}
	return nil, fmt.Errorf("body parser not defined for content type '%s'", contentType)
}

// type SerializerFn map[cont.ContentType]SchemaEncoder
type SerializerFn func(cont.ContentType) (BodyParser, bool)

type CustomSpecs map[string]CustomSpec

func (c CustomSpecs) Find(specID string) (CustomSpec, bool) {
	v, ok := c[specID]
	return v, ok
}

type CustomSpec interface {
	SpecID() string
	Encode(val any) (string, error)
	Decode(val any) (any, error)
	Type() string
	Format() string
}

type SchemaRulesMap map[string]map[string]*schemaRules

func (s SchemaRulesMap) SetRules(pattern, method string, rules *schemaRules) {
	if s == nil {
		return
	}

	if s[pattern] == nil {
		s[pattern] = map[string]*schemaRules{
			strings.ToLower(method): rules,
		}
	} else {
		s[pattern][strings.ToLower(method)] = rules
	}
}

func (s SchemaRulesMap) GetRules(pattern, method string) *schemaRules {
	if s != nil {
		if x, ok := s[pattern]; ok {
			if y, ok := x[strings.ToLower(method)]; ok {
				return y
			}
		}
	}
	return nil
}

type Validator interface {
	Name() string
	Rule(c ValidatorContext) func(val any) error
}

func DefineCustomSpec(spec SpecDefinition) CustomSpec {
	return &specDefinition{
		specID: spec.SpecID,
		typ:    spec.Type,
		format: spec.Format,
		encode: spec.Encode,
		decode: spec.Decode,
	}
}

type SpecDefinition struct {
	SpecID string
	Type   string
	Format string
	Encode func(val any) (string, error)
	Decode func(val any) (any, error)
}

type specDefinition struct {
	specID string
	typ    string
	format string
	encode func(val any) (string, error)
	decode func(val any) (any, error)
}

func (s *specDefinition) SpecID() string {
	return s.specID
}

func (s *specDefinition) Type() string {
	if s.typ == "" {
		return "string"
	}
	return s.typ
}

func (s *specDefinition) Format() string {
	if s.format == "" {
		return "string"
	}
	return s.format
}

func (s *specDefinition) Encode(val any) (string, error) {
	if s.encode == nil {
		return "", fmt.Errorf("encode function not defined for spec '%s'", s.specID)
	}
	return s.encode(val)
}

func (s *specDefinition) Decode(val any) (any, error) {
	if s.decode == nil {
		return nil, fmt.Errorf("decode function not defined for spec '%s'", s.specID)
	}
	return s.decode(val)
}
