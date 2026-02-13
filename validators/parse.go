package validators

import (
	"reflect"
)

type RuleFn struct {
	Kind      reflect.Kind
	Rule      string
	Lator     LegacyValidatorFn
	Arguments []string
}

func NewContextValidatorFn(typ reflect.Type, kind reflect.Kind, rule string, args []any, vals ContextValidators) ValidatorFn {
	if v, ok := Validators[rule]; ok {
		return v(ValidatorContext{
			Kind:    kind,
			Options: args,
			Type:    typ,
		})
	} else if v, ok := vals[rule]; ok {
		return v(ValidatorContext{
			Kind:    kind,
			Options: args,
			Type:    typ,
		})
	}

	return defaultValidator

}

func defaultValidator(arg ValidatorArg) error {
	return nil
}
