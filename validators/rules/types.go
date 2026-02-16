package rules

import (
	"reflect"
)

type ValidatorFn = func(val any) error

type ValidatorContext struct {
	Type    reflect.Type
	Kind    reflect.Kind
	Options []any
}

type ContextValidator = func(c ValidatorContext) ValidatorFn
type ContextValidators map[string]ContextValidator

type ValidatorOptionType int
