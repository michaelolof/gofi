package rules

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsOneOf(t *testing.T) {
	tests := []struct {
		name      string
		valParams ValidatorContext
		val       any
		wantErr   bool
	}{
		{
			name: "valid oneof string",
			valParams: ValidatorContext{
				Type:    reflect.TypeOf(""),
				Kind:    reflect.String,
				Options: []any{"apple", "banana"},
			},
			val:     "apple",
			wantErr: false,
		},
		{
			name: "invalid oneof string",
			valParams: ValidatorContext{
				Type:    reflect.TypeOf(""),
				Kind:    reflect.String,
				Options: []any{"apple", "banana"},
			},
			val:     "cherry",
			wantErr: true,
		},
		{
			name: "valid oneof int",
			valParams: ValidatorContext{
				Type:    reflect.TypeOf(0),
				Kind:    reflect.Int,
				Options: []any{1, 2, 3},
			},
			val:     2,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := IsOneOf(tt.valParams)
			err := validator(tt.val)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
