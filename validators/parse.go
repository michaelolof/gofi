package validators

import (
	"reflect"

	"github.com/michaelolof/gofi/validators/rules"
)

func NewContextValidatorFn(typ reflect.Type, kind reflect.Kind, rule string, args []any, vals rules.ContextValidators) rules.ValidatorFn {
	if v, ok := Validators[rule]; ok {
		fn := v(rules.ValidatorContext{
			Kind:    kind,
			Options: args,
			Type:    typ,
		})
		if holdsNamedValues(typ) && NeedsPlainValues(rule) {
			return PlainValues(fn)
		}
		return fn
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

// plainTypes maps a primitive kind to its plain type.
var plainTypes = [reflect.UnsafePointer + 1]reflect.Type{
	reflect.String:  reflect.TypeFor[string](),
	reflect.Bool:    reflect.TypeFor[bool](),
	reflect.Int:     reflect.TypeFor[int](),
	reflect.Int8:    reflect.TypeFor[int8](),
	reflect.Int16:   reflect.TypeFor[int16](),
	reflect.Int32:   reflect.TypeFor[int32](),
	reflect.Int64:   reflect.TypeFor[int64](),
	reflect.Uint:    reflect.TypeFor[uint](),
	reflect.Uint8:   reflect.TypeFor[uint8](),
	reflect.Uint16:  reflect.TypeFor[uint16](),
	reflect.Uint32:  reflect.TypeFor[uint32](),
	reflect.Uint64:  reflect.TypeFor[uint64](),
	reflect.Float32: reflect.TypeFor[float32](),
	reflect.Float64: reflect.TypeFor[float64](),
}

// holdsNamedValues reports whether a field of type typ can hold a value of a
// named primitive type, such as `type Status string`. Plain fields can't, so
// they skip the conversion in PlainValues.
func holdsNamedValues(typ reflect.Type) bool {
	if typ == nil {
		return false
	}
	if typ.Kind() == reflect.Interface {
		return true
	}
	plain := plainTypes[typ.Kind()]
	return plain != nil && typ != plain
}

// PlainValues returns a built-in rule for values that may be of a named primitive
// type. The built-in rules switch on plain Go types, so such a value is converted
// to its plain type first.
func PlainValues(fn rules.ValidatorFn) rules.ValidatorFn {
	return func(val any) error {
		rv := reflect.ValueOf(val)
		if plain := plainTypes[rv.Kind()]; plain != nil && rv.Type() != plain {
			val = rv.Convert(plain).Interface()
		}
		return fn(val)
	}
}

// kindAwareRules are the built-in rules that already work on a named type. They
// are on most fields, so they skip the conversion in PlainValues.
var kindAwareRules = map[string]bool{"required": true, "present": true, "allow_zero": true, "oneof": true}

// NeedsPlainValues reports whether rule is a built-in rule that needs PlainValues
// for a value that may be of a named type.
func NeedsPlainValues(rule string) bool {
	_, builtIn := Validators[rule]
	return builtIn && !kindAwareRules[rule]
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
