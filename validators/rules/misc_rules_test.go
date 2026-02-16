package rules

import (
	"reflect"
	"testing"
)

func TestMiscRules(t *testing.T) {
	tests := []struct {
		name    string
		rule    func(ValidatorContext) func(any) error
		options []any
		kind    reflect.Kind
		valid   []any
		invalid []any
	}{
		{
			name:    "IsBoolean String",
			rule:    IsBoolean,
			kind:    reflect.String,
			valid:   []any{"true", "false", "1", "0"},
			invalid: []any{"yes", "no", 123, "TRUE"},
		},
		{
			name:    "IsBoolean Bool",
			rule:    IsBoolean,
			kind:    reflect.Bool,
			valid:   []any{true, false},
			invalid: []any{},
		},
		{
			name:    "IsJSON",
			rule:    IsJSON,
			kind:    reflect.String,
			valid:   []any{`{"foo":"bar"}`, `[1, 2, 3]`, `"string"`},
			invalid: []any{`{foo:bar}`, `{'foo':'bar'}`, `invalid`},
		},
		{
			name: "IsDefault",
			rule: IsDefault,
			// Kind not strictly required here as IsDefault uses reflect on val,
			// but let's set it to valid values' kind roughly or rely on default
			valid:   []any{0, "", false, nil, (*int)(nil)},
			invalid: []any{1, "default", true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind := tt.kind
			if kind == reflect.Invalid {
				// For IsDefault tests with mixed types, we might want to just let it be Invalid
				// or pick one. But IsDefault logic doesn't use c.Kind much if implemented right.
				// However, IsBoolean logic DOES use c.Kind.
				kind = reflect.String // Default
			}
			validator := tt.rule(ValidatorContext{Kind: kind, Options: tt.options})

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
