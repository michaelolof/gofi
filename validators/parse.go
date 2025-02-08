package validators

import (
	"reflect"
)

type RuleFn struct {
	Kind      reflect.Kind
	Rule      string
	Lator     ValidatorFn
	Arguments []string
}

func newLatorMp(id string, lator ValidatorFn, options []string) RuleFn {
	return RuleFn{
		Rule:      id,
		Lator:     lator,
		Arguments: options,
	}
}

func BuildValidators(kind reflect.Kind, rule string, args []any, vals MappedValidators) CompiledValidatorFn {

	if len(args) > 0 {
		if v, ok := OptionValidators[rule]; ok {
			return v(kind, args...)
		}
	} else if v, ok := BaseValidators[rule]; ok {
		return v(kind)
	} else if v, ok := vals[rule]; ok {
		return v(kind, args...)
	}

	return defaultValidator

}

func defaultValidator(val any) error {
	return nil
}
