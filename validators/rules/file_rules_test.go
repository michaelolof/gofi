package rules

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFileRules(t *testing.T) {
	// Create temp file and dir for testing
	tempDir, err := os.MkdirTemp("", "gofi-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	tempFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(tempFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		rule    func(ValidatorContext) func(any) error
		valid   []any
		invalid []any
	}{
		{
			name:    "IsFile",
			rule:    IsFile,
			valid:   []any{tempFile},
			invalid: []any{tempDir, "non_existent_file.txt"},
		},
		{
			name:    "IsDir",
			rule:    IsDir,
			valid:   []any{tempDir},
			invalid: []any{tempFile, "non_existent_dir"},
		},
		{
			name:    "IsFilePath",
			rule:    IsFilePath,
			valid:   []any{"/bin/bash", "relative/path/to/file"},
			invalid: []any{"", string([]byte{0})},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := tt.rule(ValidatorContext{Kind: reflect.String})

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
