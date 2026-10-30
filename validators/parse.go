package validators

import (
	"reflect"

	"github.com/michaelolof/gofi/validators/rules"
)

func NewContextValidatorFn(typ reflect.Type, kind reflect.Kind, rule string, args []any, vals rules.ContextValidators) rules.ValidatorFn {
	if v, ok := Validators[rule]; ok {
		return v(rules.ValidatorContext{
			Kind:    kind,
			Options: args,
			Type:    typ,
		})
	} else if v, ok := vals[rule]; ok {
		return v(rules.ValidatorContext{
			Kind:    kind,
			Options: args,
			Type:    typ,
		})
	}

	return defaultValidator

}

func defaultValidator(arg any) error {
	return nil
}

// OmitEmpty is the modifier that skips a field's other rules when its value
// is empty. It is not a validator itself.
const OmitEmpty = "omitempty"

// IsKnownRule reports whether rule is OmitEmpty, a built-in rule, or one of
// the custom rules in custom.
func IsKnownRule(rule string, custom rules.ContextValidators) bool {
	if rule == OmitEmpty {
		return true
	}
	if _, ok := Validators[rule]; ok {
		return true
	}
	_, ok := custom[rule]
	return ok
}

// IsEmpty reports whether val is empty for OmitEmpty: nil, a nil pointer or
// interface, a string, slice, map or array of length zero, or any other zero
// value.
func IsEmpty(val any) bool {
	if val == nil {
		return true
	}
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}
