package rules

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRequired(t *testing.T) {
	tests := []struct {
		name      string
		valParams ValidatorContext
		val       any
		wantErr   bool
	}{
		{
			name: "valid string",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(""),
				Kind: reflect.String,
			},
			val:     "hello",
			wantErr: false,
		},
		{
			name: "empty string",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(""),
				Kind: reflect.String,
			},
			val:     "",
			wantErr: true,
		},
		{
			name: "nil value",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(""),
				Kind: reflect.String,
			},
			val:     nil,
			wantErr: true,
		},
		{
			name: "valid int",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(0),
				Kind: reflect.Int,
			},
			val:     123,
			wantErr: false,
		},
		{
			name: "zero int",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(0),
				Kind: reflect.Int,
			},
			val:     0,
			wantErr: true,
		},
		{
			// Non-pointer structs are always present in Go;
			// IsRequired must not reject them even when all fields are zero.
			name: "struct with all zero-valued fields — no error",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(struct{ Value int }{}),
				Kind: reflect.Struct,
			},
			val:     struct{ Value int }{},
			wantErr: false,
		},
		{
			// Regression guard: nil must still be rejected regardless of type.
			name: "nil value — error (regression guard)",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(struct{ Value int }{}),
				Kind: reflect.Struct,
			},
			val:     nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := IsRequired(tt.valParams)
			err := validator(tt.val)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
