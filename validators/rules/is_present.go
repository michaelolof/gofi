package rules

import "fmt"

// IsPresent validates that a value is non-nil.
// Unlike IsRequired, zero-values (empty string, 0, false, etc.) are accepted.
func IsPresent(c ValidatorContext) func(arg any) error {
	return func(arg any) error {
		if arg == nil {
			return fmt.Errorf("value must be present")
		}
		return nil
	}
}
