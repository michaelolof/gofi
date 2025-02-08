package oldvalidators

import (
	"reflect"

	kvalidators "github.com/michaelolof/gofi/validators"
)

type OptionValidatorFn = func([]any) func(any) error
type MappedValidators = map[string]OptionValidatorFn
type ValidatorFn = func(val any) error

type RuleFn struct {
	Rule    string
	Lator   ValidatorFn
	Options []any
}

func newLatorMp(id string, lator ValidatorFn, options []any) RuleFn {
	return RuleFn{
		Rule:    id,
		Lator:   lator,
		Options: options,
	}
}

func BuildValidators(kind reflect.Kind, rule string, options []any, vals MappedValidators) ValidatorFn {

	if len(options) > 0 {
		if v, ok := kvalidators.OptionValidators[rule]; ok {
			return v(kind, options)
		}
	} else if v, ok := kvalidators.BaseValidators[rule]; ok {
		return v(kind)
	} else if v, ok := vals[rule]; ok {
		return v(options)
	}

	return defaultValidator

}

func defaultValidator(val any) error {
	return nil
}
