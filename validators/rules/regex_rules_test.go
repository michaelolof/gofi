package rules

import (
	"reflect"
	"testing"
)

func TestRegexRules(t *testing.T) {
	tests := []struct {
		name     string
		rule     func(ValidatorContext) func(any) error
		valid    []any
		invalid  []any
		mustFail bool // if expected to fail
	}{
		{
			name: "IsAlpha",
			rule: IsAlpha,
			valid: []any{
				"abc", "ABC", "aBc",
			},
			invalid: []any{
				"abc1", "123", "", " ", "abc ",
			},
		},
		{
			name: "IsNumeric",
			rule: IsNumeric,
			valid: []any{
				"123", "-123", "+123", "12.34",
			},
			invalid: []any{
				"abc", "12a", "",
			},
		},
		{
			name: "IsEmail",
			rule: IsEmail,
			valid: []any{
				"test@example.com", "t.e.s.t@example.com", "test+123@example.com",
			},
			invalid: []any{
				"abc", "test@", "@example.com", "test@example", "",
			},
		},
		{
			name: "IsUUID",
			rule: IsUUID,
			valid: []any{
				"123e4567-e89b-12d3-a456-426614174000",
				"123e4567-e89b-42d3-a456-426614174000", // v4
			},
			invalid: []any{
				"123e4567e89b12d3a456426614174000",     // no hyphens
				"g23e4567-e89b-12d3-a456-426614174000", // invalid char
				"",
			},
		},
		{
			name: "IsSemver",
			rule: IsSemver,
			valid: []any{
				"1.0.0", "1.0.0-beta", "1.0.0-beta.1", "1.0.0+20130313144700",
			},
			invalid: []any{
				"1", "1.0", "v1.0.0", "",
			},
		},
		{
			name: "IsBase64",
			rule: IsBase64,
			valid: []any{
				"SGVsbG8gV29ybGQ=", "SGVsbG8gV29ybGQ=",
			},
			invalid: []any{
				"SGVsbG8gV29ybGQ==", // padding error? Regex handles some padding
				"Invalid Base64!",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := tt.rule(ValidatorContext{Kind: reflect.String}) // assuming string context for most

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
