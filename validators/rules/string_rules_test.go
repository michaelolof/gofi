package rules

import (
	"reflect"
	"testing"
)

func TestStringRules(t *testing.T) {
	tests := []struct {
		name    string
		rule    func(ValidatorContext) func(any) error
		options []any
		valid   []any
		invalid []any
	}{
		{
			name:    "Contains",
			rule:    Contains,
			options: []any{"bar"},
			valid:   []any{"foobar", "bar", "barbaz"},
			invalid: []any{"foo", "baz", ""},
		},
		{
			name:    "ContainsAny",
			rule:    ContainsAny,
			options: []any{"!@#"},
			valid:   []any{"hello!", "email@", "#hashtag"},
			invalid: []any{"hello", "world"},
		},
		{
			name:    "StartsWith",
			rule:    StartsWith,
			options: []any{"https"},
			valid:   []any{"https://google.com", "https"},
			invalid: []any{"http://google.com", "www.google.com"},
		},
		{
			name:    "EndsWith",
			rule:    EndsWith,
			options: []any{".com"},
			valid:   []any{"google.com", "example.com"},
			invalid: []any{"google.org", "google.co"},
		},
		{
			name:    "IsLowercase",
			rule:    IsLowercase,
			valid:   []any{"hello", "world", "123"},
			invalid: []any{"Hello", "WORLD", "HeLLo"},
		},
		{
			name:    "Excludes",
			rule:    Excludes,
			options: []any{"admin"},
			valid:   []any{"user", "guest"},
			invalid: []any{"admin", "administrator", "superadmin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := tt.rule(ValidatorContext{Kind: reflect.String, Options: tt.options})

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
