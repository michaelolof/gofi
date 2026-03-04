package gofi

import (
	"fmt"
	"iter"
	"reflect"
	"strings"
	"sync"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/utils"
	"github.com/michaelolof/gofi/validators"
	"github.com/michaelolof/gofi/validators/rules"
)

type ruleOpts struct {
	typ   reflect.Type
	kind  reflect.Kind
	rule  string
	args  []string
	dator rules.ValidatorFn
}

func newRuleOpts(typ reflect.Type, kind reflect.Kind, rule string, args []string, muxOpts *muxOptions) ruleOpts {
	anyArgs := make([]any, 0, len(args))
	for _, v := range args {
		anyArgs = append(anyArgs, v)
	}

	var customValidators rules.ContextValidators
	if muxOpts != nil && muxOpts.customValidators != nil {
		customValidators = muxOpts.customValidators
	}

	return ruleOpts{
		typ:   typ,
		kind:  kind,
		rule:  rule,
		args:  args,
		dator: validators.NewContextValidatorFn(typ, kind, rule, anyArgs, customValidators),
	}
}

type fieldAccessor struct {
	offset    uintptr
	index     []int
	fieldType reflect.Type
	kind      reflect.Kind
	name      string
}

type RuleDef struct {
	typ                  reflect.Type
	kind                 reflect.Kind
	format               utils.ObjectFormats
	pattern              string
	field                string
	fieldName            string
	defStr               string
	defVal               any
	rules                []ruleOpts
	item                 *RuleDef
	additionalProperties *RuleDef
	properties           map[string]*RuleDef
	orderedProps         []*RuleDef
	max                  *float64
	required             bool

	tags         map[string][]string
	accessor     fieldAccessor
	jsonKeyBytes []byte
}

func preComputeJSONKey(name string) []byte {
	if name == "" {
		return nil
	}
	buf := make([]byte, 0, len(name)+4)
	buf = append(buf, '"')
	buf = append(buf, name...)
	buf = append(buf, '"', ':')
	return buf
}

func newRuleDef(sf reflect.StructField, defStr string, defVal any, rules []ruleOpts, required bool, max *float64, properties map[string]*RuleDef, items *RuleDef, addProps *RuleDef) *RuleDef {
	props := make(map[string]*RuleDef)
	if properties != nil {
		props = properties
	}
	var ordered []*RuleDef

	// Parse JSON tag for name if needed
	var fieldName string
	var tagName string
	if tag, ok := sf.Tag.Lookup("json"); ok {
		parts := strings.Split(tag, ",")
		if len(parts) > 0 && parts[0] != "" {
			tagName = parts[0]
		}
	}
	if tagName == "" {
		tagName = sf.Name
	}
	fieldName = sf.Name

	return &RuleDef{
		typ:                  sf.Type,
		kind:                 sf.Type.Kind(),
		field:                tagName,
		fieldName:            fieldName,
		defStr:               defStr,
		defVal:               defVal,
		rules:                rules,
		required:             required,
		max:                  max,
		item:                 items,
		properties:           props,
		orderedProps:         ordered,
		additionalProperties: addProps,
		accessor: fieldAccessor{
			offset:    sf.Offset,
			index:     sf.Index,
			fieldType: sf.Type,
			kind:      sf.Type.Kind(),
			name:      sf.Name,
		},
		jsonKeyBytes: preComputeJSONKey(tagName),
	}
}

func (r *RuleDef) hasRule(rule string) bool {
	if r == nil {
		return false
	}

	for _, l := range r.rules {
		if l.rule == rule {
			return true
		}
	}
	return false
}

func (r *RuleDef) attach(name string, item *RuleDef) {
	if r == nil && item == nil {
		return
	}

	if r != nil {
		r.properties[name] = item
		r.orderedProps = append(r.orderedProps, item)
	}
}

func (r *RuleDef) append(item *RuleDef) {
	if r == nil && item == nil {
		return
	}

	if r == nil && item != nil {
		r = &RuleDef{}
	} else {
		r.item = item
	}
}

func (r *RuleDef) addProps(props *RuleDef) {
	if r == nil && props == nil {
		return
	}

	if r != nil {
		r.additionalProperties = props
	}
}

func (r *RuleDef) Walk() iter.Seq[*RuleDef] {
	return func(yield func(*RuleDef) bool) {
		if r == nil {
			return
		}

		if !yield(r) {
			return
		}

		if r.item != nil {
			for child := range r.item.Walk() {
				if !yield(child) {
					return
				}
			}
		}

		if r.additionalProperties != nil {
			for child := range r.additionalProperties.Walk() {
				if !yield(child) {
					return
				}
			}
		}

		for _, child := range r.properties {
			for subChild := range child.Walk() {
				if !yield(subChild) {
					return
				}
			}
		}
	}
}

func (r *RuleDef) ruleOptions(rule string) []string {
	if r == nil {
		return nil
	}

	for _, l := range r.rules {
		if l.rule == rule {
			return l.args
		}
	}
	return nil
}

func (r *RuleDef) findRules(rules []string, fallback string) string {
	if r == nil {
		return fallback
	}
	for _, l := range r.rules {
		for _, r := range rules {
			if l.rule == r {
				return l.rule
			}
		}
	}
	return fallback
}

func getItemRuleDef(typ reflect.Type) *RuleDef {
	// Create a dummy StructField to reuse newRuleDef
	sf := reflect.StructField{
		Type: typ,
		Name: "",
	}
	return newRuleDef(sf, "", nil, nil, false, nil, nil, nil, nil)
}

type ruleDefMap map[string]RuleDef

type schemaRules struct {
	req        map[string]RuleDef
	responses  map[string]map[string]RuleDef
	schemaPool *sync.Pool
	schemaType reflect.Type
}

func newSchemaRules(typ reflect.Type) schemaRules {
	var pool *sync.Pool
	if typ != nil {
		pool = &sync.Pool{
			New: func() any {
				return reflect.New(typ).Interface()
			},
		}
	}
	return schemaRules{
		req:        make(map[string]RuleDef),
		responses:  make(map[string]map[string]RuleDef),
		schemaPool: pool,
		schemaType: typ,
	}
}

func (s *schemaRules) setReq(key string, rules *RuleDef) {
	if rules == nil {
		return
	}
	s.req[key+"."+rules.field] = *rules
}

func (s *schemaRules) setResps(key string, rules *RuleDef) {
	if rules == nil {
		return
	}

	if _, ok := s.responses[key]; ok {
		s.responses[key][rules.field] = *rules
	} else {
		s.responses[key] = map[string]RuleDef{
			rules.field: *rules,
		}
	}
}

func (s *schemaRules) getReqRules(key schemaField) *RuleDef {
	if s == nil {
		return nil
	}

	prefix := string(schemaReq)
	if rtn, ok := s.req[prefix+"."+string(key)]; ok {
		return &rtn
	}
	return nil
}

func (s *schemaRules) reqContent() cont.ContentType {
	hs := s.getReqRules(schemaHeaders)
	if hs != nil {
		if v, ok := hs.properties["content-type"]; ok && len(v.defStr) > 0 {
			return cont.ContentType(v.defStr)

		} else if v, ok := hs.properties["Content-Type"]; ok && len(v.defStr) > 0 {
			return cont.ContentType(v.defStr)
		}
	}

	return cont.ApplicationJson
}

func (s *schemaRules) respContent(code int) cont.ContentType {
	_, hsc, _ := s.getRespRulesByCode(code)
	if hsc != nil {
		if hs, ok := hsc[string(schemaHeaders)]; ok {
			if v, ok := hs.properties["content-type"]; ok && len(v.defStr) > 0 {
				return cont.ContentType(v.defStr)

			} else if v, ok := hs.properties["Content-Type"]; ok && len(v.defStr) > 0 {
				return cont.ContentType(v.defStr)
			}
		}
	}
	return cont.ApplicationJson
}

func (s *schemaRules) getRespRulesByCode(code int) (string, ruleDefMap, error) {

	handleDefaults := func() (string, ruleDefMap, error) {
		// Check if falls within the range of Success, Err or Default
		if code >= 100 && code <= 199 {
			if resp, ok := s.responses[informational]; ok {
				return informational, resp, nil
			}
		} else if code >= 200 && code <= 299 { // Should have a success field
			if resp, ok := s.responses[successFieldName]; ok {
				return successFieldName, resp, nil
			}
		} else if code >= 300 && code <= 399 {
			if resp, ok := s.responses[redirectFieldName]; ok {
				return redirectFieldName, resp, nil
			}
		} else if code >= 400 && code <= 499 {
			if resp, ok := s.responses[redirectFieldName]; ok {
				return redirectFieldName, resp, nil
			} else if resp, ok := s.responses[errFieldName]; ok {
				return errFieldName, resp, nil
			}
		} else if code >= 500 && code <= 599 { // Should have an error field
			if resp, ok := s.responses[errFieldName]; ok {
				return errFieldName, resp, nil
			} else if resp, ok := s.responses[errFieldName]; ok {
				return errFieldName, resp, nil
			}
		}

		if resp, ok := s.responses[defaultFieldName]; ok {
			return defaultFieldName, resp, nil
		} else {
			return "", nil, fmt.Errorf("no matching response rules for the given status code %d", code)
		}
	}

	if info, ok := codeToStatuses[code]; ok {
		if resp, ok := s.responses[info.Field]; ok {
			return info.Field, resp, nil
		}
		return handleDefaults()
	}
	return handleDefaults()
}

type SchemaRuleDefinition struct {
	Rule    string
	Arg     any
	Message string
}
