package rules

import (
	"reflect"
	"testing"
)

func TestPaymentRules(t *testing.T) {
	tests := []struct {
		name    string
		rule    func(ValidatorContext) func(any) error
		valid   []any
		invalid []any
	}{
		{
			name:    "IsCreditCard",
			rule:    IsCreditCard,
			valid:   []any{"4242424242424242", "4242 4242 4242 4242"}, // Test generic valid luhn
			invalid: []any{"4242424242424241", "123"},
		},
		{
			name:    "IsEIN",
			rule:    IsEIN,
			valid:   []any{"123456789", "12-3456789"},
			invalid: []any{"12345678", "1234567890", "12-345678", "abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := tt.rule(ValidatorContext{Kind: reflect.String}) // Default context

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
