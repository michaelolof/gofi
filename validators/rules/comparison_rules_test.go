package rules

import (
	"reflect"
	"testing"
)

func TestComparisonRules(t *testing.T) {
	tests := []struct {
		name    string
		rule    func(ValidatorContext) func(any) error
		options []any
		kind    reflect.Kind
		valid   []any
		invalid []any
	}{
		{
			name:    "IsLen String",
			rule:    IsLen,
			options: []any{3},
			kind:    reflect.String,
			valid:   []any{"abc", "123"},
			invalid: []any{"ab", "abcd", ""},
		},
		{
			name:    "IsLen Slice",
			rule:    IsLen,
			options: []any{2},
			kind:    reflect.Slice,
			valid:   []any{[]int{1, 2}, []string{"a", "b"}},
			invalid: []any{[]int{1}, []int{1, 2, 3}, []int{}},
		},
		{
			name:    "IsGt Int",
			rule:    IsGt,
			options: []any{10},
			kind:    reflect.Int,
			valid:   []any{11, 100},
			invalid: []any{10, 9, 0, -1},
		},
		{
			name:    "IsLte Float",
			rule:    IsLte,
			options: []any{5.5},
			kind:    reflect.Float64,
			valid:   []any{5.5, 5.0, -10.0},
			invalid: []any{5.6, 10.0},
		},
		{
			name:    "IsNe String Len",
			rule:    IsNe,
			options: []any{0},
			kind:    reflect.String,
			valid:   []any{"a", " "},
			invalid: []any{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := tt.rule(ValidatorContext{Kind: tt.kind, Options: tt.options})

			for _, val := range tt.valid {
				if err := validator(val); err != nil {
					t.Errorf("%s(%v) expected valid, got error: %v", tt.name, val, err)
				}
			}

			for _, val := range tt.invalid {
				if err := validator(val); err == nil {
					t.Errorf("%s(%v) expected invalid, got nil (valid)", tt.name, val)
				}
			}
		})
	}
}
