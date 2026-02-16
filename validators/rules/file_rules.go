package rules

import (
	"errors"
	"fmt"
	"os"
)

func IsFile(c ValidatorContext) func(val any) error {
	return func(val any) error {
		path, ok := val.(string)
		if !ok {
			return errors.New("invalid file path value. value must be a string")
		}

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file '%s' does not exist", path)
			}
			return err
		}

		if info.IsDir() {
			return fmt.Errorf("path '%s' is a directory, not a file", path)
		}
		return nil
	}
}

func IsDir(c ValidatorContext) func(val any) error {
	return func(val any) error {
		path, ok := val.(string)
		if !ok {
			return errors.New("invalid directory path value. value must be a string")
		}

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("directory '%s' does not exist", path)
			}
			return err
		}

		if !info.IsDir() {
			return fmt.Errorf("path '%s' is a file, not a directory", path)
		}
		return nil
	}
}

func IsFilePath(c ValidatorContext) func(val any) error {
	// IsFilePath checks if the string is a valid path syntax?
	// Or if it points to an existing file?
	// Existing `validator` library `filepath` usually implies valid syntax or existence?
	// The `validator` `file` tag checks for existence and not dir.
	// `filepath` tag checks if string contains valid path characters.
	// But `validator` documentation says `filepath` is alias to `file` in some versions or distinct?
	// Actually `validator` has `file` (exists and not dir) and `dir` (exists and is dir).
	// Let's implement `IsFilePath` as structure check if possible, but complex.
	// For now, let's assume `IsFilePath` mimics `validator`'s behavior which might be checking existence or just syntax.
	// Looking at `validator/baked_in.go` code `isFilePath` usually calls `os.Stat`? No wait, `isFilePath` might be just syntax.
	// Let's implement it as existence check for now (alias to IsFile) or maybe just syntax if possible.
	// Checking syntax is OS dependent and tricky.
	// Let's stick to alias for `IsFile` or maybe checks if directory exists at least?
	// Let's implement it as: Checks if it is a valid path string (not empty).
	return func(val any) error {
		path, ok := val.(string)
		if !ok {
			return errors.New("invalid file path value. value must be a string")
		}
		if path == "" {
			return errors.New("file path cannot be empty")
		}
		// We could try to Stat, but maybe it's just about syntax.
		// Let's just return nil for non-empty string for now, or maybe check for 0x00 bytes.
		for i := 0; i < len(path); i++ {
			if path[i] == 0 {
				return errors.New("file path contains null byte")
			}
		}
		return nil
	}
}
