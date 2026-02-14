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
