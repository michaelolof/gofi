package schema

import (
	"fmt"
	"reflect"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/utils"
	"github.com/michaelolof/gofi/validators"
)

type RuleOpts struct {
	Typ   reflect.Type
	Kind  reflect.Kind
	Rule  string
	Args  []string
	Dator validators.ValidatorFn
}

func NewRuleOpts(typ reflect.Type, kind reflect.Kind, rule string, args []string, customValidators validators.ContextValidators) RuleOpts {
	anyArgs := make([]any, 0, len(args))
	for _, v := range args {
		anyArgs = append(anyArgs, v)
	}

	return RuleOpts{
		Typ:   typ,
		Kind:  kind,
		Rule:  rule,
		Args:  args,
		Dator: validators.NewContextValidatorFn(typ, kind, rule, anyArgs, customValidators),
	}
}

type RuleDef struct {
	Typ                  reflect.Type
	Kind                 reflect.Kind
	Format               utils.ObjectFormats
	Pattern              string
	Field                string
	FieldName            string
	DefStr               string
	DefVal               any
	Rules                []RuleOpts
	Item                 *RuleDef
	AdditionalProperties *RuleDef
	Properties           map[string]*RuleDef
	Max                  *float64
	Required             bool

	Tags map[string][]string
}

func NewRuleDef(typ reflect.Type, kind reflect.Kind, field string, fieldName string, defStr string, defVal any, rules []RuleOpts, required bool, max *float64, properties map[string]*RuleDef, items *RuleDef, addProps *RuleDef) *RuleDef {
	props := make(map[string]*RuleDef)
	if properties != nil {
		props = properties
	}

	return &RuleDef{
		Typ:                  typ,
		Kind:                 kind,
		Field:                field,
		FieldName:            fieldName,
		DefStr:               defStr,
		DefVal:               defVal,
		Rules:                rules,
		Required:             required,
		Max:                  max,
		Item:                 items,
		Properties:           props,
		AdditionalProperties: addProps,
	}
}

func (r *RuleDef) HasRule(rule string) bool {
	if r == nil {
		return false
	}

	for _, l := range r.Rules {
		if l.Rule == rule {
			return true
		}
	}
	return false
}

func (r *RuleDef) Attach(name string, item *RuleDef) {
	if r == nil && item == nil {
		return
	}

	if r != nil {
		r.Properties[name] = item
	}
}

func (r *RuleDef) Append(item *RuleDef) {
	if r == nil && item == nil {
		return
	}

	if r == nil && item != nil {
		r = &RuleDef{}
	} else {
		r.Item = item
	}
}

func (r *RuleDef) AddProps(props *RuleDef) {
	if r == nil && props == nil {
		return
	}

	if r != nil {
		r.AdditionalProperties = props
	}
}

func (r *RuleDef) RuleOptions(rule string) []string {
	if r == nil {
		return nil
	}

	for _, l := range r.Rules {
		if l.Rule == rule {
			return l.Args
		}
	}
	return nil
}

func (r *RuleDef) FindRules(rules []string, fallback string) string {
	if r == nil {
		return fallback
	}
	for _, l := range r.Rules {
		for _, r := range rules {
			if l.Rule == r {
				return l.Rule
			}
		}
	}
	return fallback
}

func GetItemRuleDef(typ reflect.Type) *RuleDef {
	return NewRuleDef(typ, typ.Kind(), "", "", "", nil, nil, false, nil, nil, nil, nil)
}

type RuleDefMap map[string]RuleDef

type SchemaRules struct {
	Req       map[string]RuleDef
	Responses map[string]map[string]RuleDef
}

func NewSchemaRules() SchemaRules {
	return SchemaRules{
		Req:       make(map[string]RuleDef),
		Responses: make(map[string]map[string]RuleDef),
	}
}

func (s *SchemaRules) SetReq(key string, rules *RuleDef) {
	if rules == nil {
		return
	}
	s.Req[key+"."+rules.Field] = *rules
}

func (s *SchemaRules) SetResps(key string, rules *RuleDef) {
	if rules == nil {
		return
	}

	if _, ok := s.Responses[key]; ok {
		s.Responses[key][rules.Field] = *rules
	} else {
		s.Responses[key] = map[string]RuleDef{
			rules.Field: *rules,
		}
	}
}

func (s *SchemaRules) GetReqRules(key SchemaField) *RuleDef {
	if s == nil {
		return nil
	}

	prefix := string(SchemaReq)
	rtn := s.Req[prefix+"."+string(key)]
	return &rtn
}

func (s *SchemaRules) ReqContent() cont.ContentType {
	hs := s.GetReqRules(SchemaHeaders)
	if dv, ok := hs.Tags["content-type"]; ok && len(dv) > 0 {
		return cont.ContentType(dv[0])
	}

	return cont.ApplicationJson
}

func (s *SchemaRules) GetRespRulesByCode(code int) (string, RuleDefMap, error) {

	handleDefaults := func() (string, RuleDefMap, error) {
		// Check if falls within the range of Success, Err or Default
		if code >= 100 && code <= 199 {
			if resp, ok := s.Responses[Informational]; ok {
				return Informational, resp, nil
			}
		} else if code >= 200 && code <= 299 { // Should have a success field
			if resp, ok := s.Responses[SuccessFieldName]; ok {
				return SuccessFieldName, resp, nil
			}
		} else if code >= 300 && code <= 399 {
			if resp, ok := s.Responses[RedirectFieldName]; ok {
				return RedirectFieldName, resp, nil
			}
		} else if code >= 400 && code <= 499 {
			if resp, ok := s.Responses[RedirectFieldName]; ok {
				return RedirectFieldName, resp, nil
			} else if resp, ok := s.Responses[ErrFieldName]; ok {
				return ErrFieldName, resp, nil
			}
		} else if code >= 500 && code <= 599 { // Should have an error field
			if resp, ok := s.Responses[ErrFieldName]; ok {
				return ErrFieldName, resp, nil
			} else if resp, ok := s.Responses[ErrFieldName]; ok {
				return ErrFieldName, resp, nil
			}
		}

		if resp, ok := s.Responses[DefaultFieldName]; ok {
			return DefaultFieldName, resp, nil
		} else {
			return "", nil, fmt.Errorf("no matching response rules for the given status code %d", code)
		}
	}

	if info, ok := CodeToStatuses[code]; ok {
		if resp, ok := s.Responses[info.Field]; ok {
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
