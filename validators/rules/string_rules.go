package rules

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

func IsNotEmpty(c ValidatorContext) func(val any) error {
	return func(val any) error {
		if err, ok := isValRequired[string](val, ""); ok {
			return err
		} else if err, ok := isValRequired[int](val, 0); ok {
			return err
		} else if err, ok := isValRequired[float32](val, 0); ok {
			return err
		} else if err, ok := isValRequired[float64](val, 0); ok {
			return err
		} else if err, ok := isValRequired[int8](val, 0); ok {
			return err
		} else if err, ok := isValRequired[int16](val, 0); ok {
			return err
		} else if err, ok := isValRequired[int32](val, 0); ok {
			return err
		} else if err, ok := isValRequired[int64](val, 0); ok {
			return err
		} else if err, ok := isValRequired[uint](val, 0); ok {
			return err
		} else if err, ok := isValRequired[uint8](val, 0); ok {
			return err
		} else if _, ok := val.(bool); c.Kind == reflect.Bool && ok {
			return nil
		} else if err, ok := isValRequired[uint16](val, 0); ok {
			return err
		} else if err, ok := isValRequired[uint32](val, 0); ok {
			return err
		} else if err, ok := isValRequired[uint64](val, 0); ok {
			return err
		} else if v, ok := val.([]any); ok && len(v) == 0 {
			return errors.New("value is empty")
		} else {
			return nil
		}
	}
}

func isValRequired[T comparable](val any, empty T) (error, bool) {
	if v, ok := val.(T); ok {
		if v == empty {
			return errors.New("value is empty"), true
		} else {
			return nil, true
		}
	} else {
		return errors.New("invalid required value passed"), false
	}
}

func Contains(c ValidatorContext) func(val any) error {
	return validateString(c, func(v string) bool {
		if len(c.Options) != 1 {
			return false
		}
		sub, ok := c.Options[0].(string)
		if !ok {
			return false
		}
		return strings.Contains(v, sub)
	}, "value must contain '%s'")
}

func ContainsAny(c ValidatorContext) func(val any) error {
	return validateString(c, func(v string) bool {
		if len(c.Options) != 1 {
			return false
		}
		chars, ok := c.Options[0].(string)
		if !ok {
			return false
		}
		return strings.ContainsAny(v, chars)
	}, "value must contain any of '%s'")
}

func ContainsRune(c ValidatorContext) func(val any) error {
	return validateString(c, func(v string) bool {
		if len(c.Options) != 1 {
			return false
		}
		r, ok := c.Options[0].(rune)
		if !ok {
			// Try int32 which is alias for rune
			if i, ok := c.Options[0].(int32); ok {
				r = rune(i)
			} else {
				return false
			}
		}
		return strings.ContainsRune(v, r)
	}, "value must contain rune '%v'")
}

func Excludes(c ValidatorContext) func(val any) error {
	return validateString(c, func(v string) bool {
		if len(c.Options) != 1 {
			return false
		}
		sub, ok := c.Options[0].(string)
		if !ok {
			return false
		}
		return !strings.Contains(v, sub)
	}, "value must not contain '%s'")
}

func ExcludesAll(c ValidatorContext) func(val any) error {
	return validateString(c, func(v string) bool {
		if len(c.Options) != 1 {
			return false
		}
		chars, ok := c.Options[0].(string)
		if !ok {
			return false
		}
		return !strings.ContainsAny(v, chars)
	}, "value must not contain any of '%s'")
}

func ExcludesRune(c ValidatorContext) func(val any) error {
	return validateString(c, func(v string) bool {
		if len(c.Options) != 1 {
			return false
		}
		r, ok := c.Options[0].(rune)
		if !ok {
			if i, ok := c.Options[0].(int32); ok {
				r = rune(i)
			} else {
				return false
			}
		}
		return !strings.ContainsRune(v, r)
	}, "value must not contain rune '%v'")
}

func StartsWith(c ValidatorContext) func(val any) error {
	return validateString(c, func(v string) bool {
		if len(c.Options) != 1 {
			return false
		}
		prefix, ok := c.Options[0].(string)
		if !ok {
			return false
		}
		return strings.HasPrefix(v, prefix)
	}, "value must start with '%s'")
}

func EndsWith(c ValidatorContext) func(val any) error {
	return validateString(c, func(v string) bool {
		if len(c.Options) != 1 {
			return false
		}
		suffix, ok := c.Options[0].(string)
		if !ok {
			return false
		}
		return strings.HasSuffix(v, suffix)
	}, "value must end with '%s'")
}

func IsLowercase(c ValidatorContext) func(val any) error {
	return validateString(c, func(v string) bool {
		return v == strings.ToLower(v)
	}, "value must be lowercase")
}

func IsUppercase(c ValidatorContext) func(val any) error {
	return validateString(c, func(v string) bool {
		return v == strings.ToUpper(v)
	}, "value must be uppercase")
}

// Helper
func validateString(c ValidatorContext, check func(string) bool, errFmt string) func(val any) error {
	var arg any
	if len(c.Options) > 0 {
		arg = c.Options[0]
	}

	return func(val any) error {
		v, ok := val.(string)
		if !ok {
			return errors.New("value must be a string")
		}
		if check(v) {
			return nil
		}
		if arg != nil {
			return fmt.Errorf(errFmt, arg)
		}
		return errors.New(errFmt)
	}
}
