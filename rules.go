package gofi

import (
	"fmt"
	"reflect"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/utils"
	"github.com/michaelolof/gofi/validators"
)

type ruleOpts struct {
	typ   reflect.Type
	kind  reflect.Kind
	rule  string
	args  []string
	dator validators.CompiledValidatorFn
}

func newRuleOpts(typ reflect.Type, kind reflect.Kind, rule string, args []string, muxOpts *muxOptions) ruleOpts {
	anyArgs := make([]any, 0, len(args))
	for _, v := range args {
		anyArgs = append(anyArgs, v)
	}

	var customValidators validators.MappedValidators
	if muxOpts != nil && muxOpts.customValidators != nil {
		customValidators = muxOpts.customValidators
	}

	return ruleOpts{
		typ:   typ,
		kind:  kind,
		rule:  rule,
		args:  args,
		dator: validators.BuildValidators(typ, kind, rule, anyArgs, customValidators),
	}
}

type ruleDef struct {
	typ                  reflect.Type
	kind                 reflect.Kind
	format               utils.ObjectFormats
	pattern              string
	field                string
	fieldName            string
	defStr               string
	defVal               any
	rules                []ruleOpts
	item                 *ruleDef
	additionalProperties *ruleDef
	properties           map[string]*ruleDef
	max                  *float64
	required             bool

	tags map[string][]string
}

func newRuleDef(typ reflect.Type, kind reflect.Kind, field string, fleldName string, defStr string, defVal any, rules []ruleOpts, required bool, max *float64, properties map[string]*ruleDef, items *ruleDef, addProps *ruleDef) *ruleDef {
	props := make(map[string]*ruleDef)
	if properties != nil {
		props = properties
	}

	return &ruleDef{
		typ:                  typ,
		kind:                 kind,
		field:                field,
		fieldName:            fleldName,
		defStr:               defStr,
		defVal:               defVal,
		rules:                rules,
		required:             required,
		max:                  max,
		item:                 items,
		properties:           props,
		additionalProperties: addProps,
	}
}

func (r *ruleDef) hasRule(rule string) bool {
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

func (r *ruleDef) attach(name string, item *ruleDef) {
	if r == nil && item == nil {
		return
	}

	if r != nil {
		r.properties[name] = item
	}
}

func (r *ruleDef) append(item *ruleDef) {
	if r == nil && item == nil {
		return
	}

	if r == nil && item != nil {
		r = &ruleDef{}
	} else {
		r.item = item
	}
}

func (r *ruleDef) addProps(props *ruleDef) {
	if r == nil && props == nil {
		return
	}

	if r != nil {
		r.additionalProperties = props
	}
}

func (r *ruleDef) ruleOptions(rule string) []string {
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

func (r *ruleDef) findRules(rules []string, fallback string) string {
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

func getItemRuleDef(typ reflect.Type) *ruleDef {
	return newRuleDef(typ, typ.Kind(), "", "", "", nil, nil, false, nil, nil, nil, nil)
}

type ruleDefMap map[string]ruleDef

type schemaRules struct {
	req       map[string]ruleDef
	responses map[string]map[string]ruleDef
}

func newSchemaRules() schemaRules {
	return schemaRules{
		req:       make(map[string]ruleDef),
		responses: make(map[string]map[string]ruleDef),
	}
}

func (s *schemaRules) setReq(key string, rules *ruleDef) {
	if rules == nil {
		return
	}
	s.req[key+"."+rules.field] = *rules
}

func (s *schemaRules) setResps(key string, rules *ruleDef) {
	if rules == nil {
		return
	}

	if _, ok := s.responses[key]; ok {
		s.responses[key][rules.field] = *rules
	} else {
		s.responses[key] = map[string]ruleDef{
			rules.field: *rules,
		}
	}
}

func (s *schemaRules) getReqRules(key schemaField) *ruleDef {
	if s == nil {
		return nil
	}

	prefix := string(schemaReq)
	rtn := s.req[prefix+"."+string(key)]
	return &rtn
}

func (s *schemaRules) reqContent() cont.ContentType {
	hs := s.getReqRules(schemaHeaders)
	if dv, ok := hs.tags["content-type"]; ok && len(dv) > 0 {
		return cont.ContentType(dv[0])
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
