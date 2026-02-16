package rules

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsIPv4(t *testing.T) {
	tests := []struct {
		name      string
		valParams ValidatorContext
		val       any
		wantErr   bool
	}{
		{
			name: "valid ipv4",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(""),
				Kind: reflect.String,
			},
			val:     "192.168.1.1",
			wantErr: false,
		},
		{
			name: "invalid ipv4",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(""),
				Kind: reflect.String,
			},
			val:     "256.256.256.256",
			wantErr: true,
		},
		{
			name: "ipv6 as ipv4",
			valParams: ValidatorContext{
				Type: reflect.TypeOf(""),
				Kind: reflect.String,
			},
			val:     "::1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := IsIPv4(tt.valParams)
			err := validator(tt.val)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
