package rules

// IsAllowZero is a modifier validator that always passes.
// It signals that a field tagged with "required" should also accept zero-values.
// The actual semantics are enforced at the compiler level via the allow_zero tag.
func IsAllowZero(c ValidatorContext) func(arg any) error {
	return func(arg any) error {
		return nil
	}
}
