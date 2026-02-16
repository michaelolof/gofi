package rules

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsMin(t *testing.T) {
	tests := []struct {
		name      string
		valParams ValidatorContext
		val       any
		wantErr   bool
	}{
		{
			name: "valid int min",
			valParams: ValidatorContext{
				Type:    reflect.TypeOf(0),
				Kind:    reflect.Int,
				Options: []any{10},
			},
			val:     15,
			wantErr: false,
		},
		{
			name: "invalid int min",
			valParams: ValidatorContext{
				Type:    reflect.TypeOf(0),
				Kind:    reflect.Int,
				Options: []any{10},
			},
			val:     5,
			wantErr: true,
		},
		{
			name: "valid string len min",
			valParams: ValidatorContext{
				Type:    reflect.TypeOf(""),
				Kind:    reflect.String,
				Options: []any{3},
			},
			val:     "hello",
			wantErr: false,
		},
		{
			name: "invalid string len min",
			valParams: ValidatorContext{
				Type:    reflect.TypeOf(""),
				Kind:    reflect.String,
				Options: []any{3},
			},
			val:     "hi",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := IsMin(tt.valParams)
			err := validator(tt.val)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
