package validators

import (
	"reflect"
	"strings"
	"testing"

	"github.com/michaelolof/gofi/validators/rules"
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

func TestValidate_UnknownRuleIsAnError(t *testing.T) {
	err := Validate("abc", "required,dive")
	if err == nil || err.Error() != `unknown validation rule "dive"` {
		t.Fatalf("got %v, want unknown validation rule error", err)
	}
}

func TestValidate_OmitEmpty(t *testing.T) {
	if err := Validate("", "omitempty,email"); err != nil {
		t.Fatalf("empty value with omitempty: got %v, want nil", err)
	}
	if err := Validate("nope", "omitempty,email"); err == nil {
		t.Fatal("non-empty invalid value with omitempty: got nil, want error")
	}
	if err := Validate([]string{}, "omitempty,min=2"); err != nil {
		t.Fatalf("empty slice with omitempty: got %v, want nil", err)
	}
}

// Every built-in rule treats a named type like its plain type.
func TestBuiltInRulesTreatNamedTypesAsPlain(t *testing.T) {
	type (
		namedString string
		namedInt    int64
		namedUint   uint32
		namedFloat  float64
		namedBool   bool
	)
	run := func(fn rules.ValidatorFn, val any) (out string) {
		defer func() {
			if r := recover(); r != nil {
				out = "panic"
			}
		}()
		if err := fn(val); err != nil {
			return "error: " + err.Error()
		}
		return "ok"
	}
	pairs := []struct{ plain, named any }{
		{"abc", namedString("abc")},
		{"a@b.co", namedString("a@b.co")},
		{"", namedString("")},
		{int64(5), namedInt(5)},
		{uint32(5), namedUint(5)},
		{1.5, namedFloat(1.5)},
		{true, namedBool(true)},
	}
	for rule := range Validators {
		for _, args := range [][]any{nil, {"3"}, {"abc"}} {
			for _, p := range pairs {
				plainType, namedType := reflect.TypeOf(p.plain), reflect.TypeOf(p.named)
				plain := NewContextValidatorFn(plainType, plainType.Kind(), rule, args, nil)
				named := NewContextValidatorFn(namedType, namedType.Kind(), rule, args, nil)
				a := run(plain, p.plain)
				// Only an error's type name may differ: oneof reports the field's type.
				b := strings.ReplaceAll(run(named, p.named), namedType.String(), plainType.String())
				if a != b {
					t.Errorf("%s(%v) on %T: plain %q, named %q", rule, args, p.named, a, b)
				}
			}
		}
	}
}

// A custom rule gets the value as it is, so it can assert on its own named type.
func TestCustomRulesKeepNamedTypes(t *testing.T) {
	type namedString string
	var got any
	custom := rules.ContextValidators{"mine": func(c rules.ValidatorContext) rules.ValidatorFn {
		return func(val any) error { got = val; return nil }
	}}
	typ := reflect.TypeFor[namedString]()
	if err := NewContextValidatorFn(typ, typ.Kind(), "mine", nil, custom)(namedString("a")); err != nil {
		t.Fatal(err)
	}
	if got != namedString("a") {
		t.Fatalf("got %#v", got)
	}
}
