package rules

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPresent(t *testing.T) {
	tests := []struct {
		name      string
		valParams ValidatorContext
		val       any
		wantErr   bool
	}{
		{
			name: "nil — error",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(""),
				Kind: reflect.String,
			},
			val:     nil,
			wantErr: true,
		},
		{
			name: "zero int — no error",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(0),
				Kind: reflect.Int,
			},
			val:     0,
			wantErr: false,
		},
		{
			name: "false bool — no error",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(false),
				Kind: reflect.Bool,
			},
			val:     false,
			wantErr: false,
		},
		{
			name: "empty string — no error",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(""),
				Kind: reflect.String,
			},
			val:     "",
			wantErr: false,
		},
		{
			name: "empty slice — no error",
			valParams: ValidatorContext{
				Type: reflect.TypeOf([]int{}),
				Kind: reflect.Slice,
			},
			val:     []int{},
			wantErr: false,
		},
		{
			name: "empty map — no error",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(map[string]int{}),
				Kind: reflect.Map,
			},
			val:     map[string]int{},
			wantErr: false,
		},
		{
			name: "non-zero int — no error",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(0),
				Kind: reflect.Int,
			},
			val:     42,
			wantErr: false,
		},
		{
			name: "non-empty string — no error",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(""),
				Kind: reflect.String,
			},
			val:     "hello",
			wantErr: false,
		},
		{
			name: "non-empty slice — no error",
			valParams: ValidatorContext{
				Type: reflect.TypeOf([]int{}),
				Kind: reflect.Slice,
			},
			val:     []int{1, 2},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := IsPresent(tt.valParams)
			err := validator(tt.val)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
