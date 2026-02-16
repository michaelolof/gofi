package rules

import (
	"reflect"
	"testing"
	"time"
)

func TestTimeRules(t *testing.T) {
	tests := []struct {
		name    string
		rule    func(ValidatorContext) func(any) error
		options []any
		kind    reflect.Kind
		valid   []any
		invalid []any
	}{
		{
			name:    "IsDatetime Default (RFC3339)",
			rule:    IsDatetime,
			options: nil,
			kind:    reflect.String,
			valid:   []any{"2023-10-25T12:00:00Z", "2023-10-25T12:00:00+01:00"},
			invalid: []any{"2023-10-25", "invalid", "", "12:00:00"},
		},
		{
			name:    "IsDatetime Custom Layout",
			rule:    IsDatetime,
			options: []any{time.DateOnly},
			kind:    reflect.String,
			valid:   []any{"2023-10-25"},
			invalid: []any{"2023-10-25T12:00:00Z", "invalid"},
		},
		{
			name:    "IsTimezone",
			rule:    IsTimezone,
			options: nil,
			kind:    reflect.String,
			valid:   []any{"UTC", "America/New_York", "Europe/London"},
			invalid: []any{"Invalid/Zone", "Mars/Crater", ""},
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
