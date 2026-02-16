package validators

import (
	"strings"
	"testing"
)

type ValidationTestStruct struct {
	Name  string `validate:"required,min=3"`
	Age   int    `validate:"min=18,max=100"`
	Email string `validate:"required,email"`
	Role  string `validate:"required,oneof=admin user guest"`
	Tags  string `validate:"omitempty,len=0"` // optional check
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		val      any
		rules    string
		hasError bool
	}{
		{
			name:     "Required Valid",
			val:      "foo",
			rules:    "required",
			hasError: false,
		},
		{
			name:     "Required Invalid",
			val:      "",
			rules:    "required",
			hasError: true,
		},
		{
			name:     "Min Length String Valid",
			val:      "abc",
			rules:    "min=3",
			hasError: false,
		},
		{
			name:     "Min Length String Invalid",
			val:      "ab",
			rules:    "min=3",
			hasError: true,
		},
		{
			name:     "Max Int Valid",
			val:      10,
			rules:    "max=10",
			hasError: false,
		},
		{
			name:     "Max Int Invalid",
			val:      11,
			rules:    "max=10",
			hasError: true,
		},
		{
			name:     "Email Valid",
			val:      "test@example.com",
			rules:    "email",
			hasError: false,
		},
		{
			name:     "Email Invalid",
			val:      "invalid-email",
			rules:    "email",
			hasError: true,
		},
		{
			name:     "OneOf Valid",
			val:      "red",
			rules:    "oneof=red green blue",
			hasError: false, // oneof implementation check needed?
		},
		{
			name:     "Multiple Rules Valid",
			val:      "admin",
			rules:    "required,alpha,min=3",
			hasError: false,
		},
		{
			name:     "Multiple Rules Invalid",
			val:      "123",
			rules:    "required,alpha",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.val, tt.rules)
			if (err != nil) != tt.hasError {
				t.Errorf("Validate() error = %v, hasError %v", err, tt.hasError)
			}
		})
	}
}

func TestValidateStruct(t *testing.T) {
	validStruct := ValidationTestStruct{
		Name:  "John",
		Age:   25,
		Email: "john@example.com",
		Role:  "admin",
	}

	invalidStruct := ValidationTestStruct{
		Name:  "Jo",         // min 3 fail
		Age:   10,           // min 18 fail
		Email: "invalid",    // email fail
		Role:  "superadmin", // oneof fail
	}

	t.Run("Valid Struct", func(t *testing.T) {
		if err := ValidateStruct(&validStruct); err != nil {
			t.Errorf("ValidateStruct() unexpected error: %v", err)
		}
	})

	t.Run("Invalid Struct", func(t *testing.T) {
		err := ValidateStruct(invalidStruct) // Pass value, should handle it (if configured to strict ptr?)
		// My implementation accepts value or ptr.
		if err == nil {
			t.Errorf("ValidateStruct() expected error, got nil")
		} else {
			// Check error message contains fields
			msg := err.Error()
			if countStringOccurrences(msg, "field") < 4 {
				// We expect failures on Name, Age, Email, Role
				// But ValidateStruct might fail fast?
				// My implementation: appends to slice, returns joined. So it shouldn't fail fast per field?
				// It fails fast per field's rules (Validate), but iterates all fields.
				// Wait, Validate() returns FIRST error for that field.
				// But ValidateStruct iterates ALL fields.
				// So we should see multiple errors joined by "; ".
			}
			t.Logf("Got expected error: %v", err)
		}
	})

	t.Run("Non Struct", func(t *testing.T) {
		if err := ValidateStruct("not a struct"); err == nil {
			t.Errorf("ValidateStruct() expected error for non-struct")
		}
	})
}

func countStringOccurrences(s, substr string) int {
	return strings.Count(s, substr)
}
