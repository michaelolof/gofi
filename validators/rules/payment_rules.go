package rules

import (
	"errors"
	"strconv"
	"strings"
)

func IsCreditCard(c ValidatorContext) func(val any) error {
	return func(val any) error {
		v, ok := val.(string)
		if !ok {
			return errors.New("invalid credit card value. value must be a string")
		}

		s := strings.ReplaceAll(v, " ", "")
		s = strings.ReplaceAll(s, "-", "")

		if len(s) < 13 || len(s) > 19 {
			return errors.New("invalid credit card number length")
		}

		if !luhnCheck(s) {
			return errors.New("invalid credit card number (luhn check failed)")
		}

		return nil
	}
}

func HasLuhnChecksum(c ValidatorContext) func(val any) error {
	return func(val any) error { // generic luhn check
		v, ok := val.(string)
		if !ok {
			if i, ok := val.(int); ok {
				v = strconv.Itoa(i)
			} else if i, ok := val.(int64); ok {
				v = strconv.FormatInt(i, 10)
			} else {
				return errors.New("invalid luhn checksum value. value must be string or int")
			}
		}

		if !luhnCheck(v) {
			return errors.New("luhn checksum failed")
		}
		return nil
	}
}

func IsEIN(c ValidatorContext) func(val any) error {
	return func(val any) error {
		v, ok := val.(string)
		if !ok {
			return errors.New("invalid EIN value. value must be a string")
		}

		// EIN is xx-xxxxxxx (9 digits)
		// Standard format is 2 digits, hyphen, 7 digits.
		// Sometimes just 9 digits.
		// Let's support both.
		s := strings.ReplaceAll(v, "-", "")
		if len(s) != 9 {
			return errors.New("invalid EIN length")
		}

		// Check that all chars are digits
		for _, r := range s {
			if r < '0' || r > '9' {
				return errors.New("invalid EIN characters")
			}
		}

		return nil
	}
}

// luhnCheck implements the Luhn algorithm for checksum validation
func luhnCheck(s string) bool {
	sum := 0
	isSecond := false

	// Process digits from right to left
	for i := len(s) - 1; i >= 0; i-- {
		r := s[i]
		if r < '0' || r > '9' {
			return false
		}

		d := int(r - '0')

		if isSecond {
			d = d * 2
			if d > 9 {
				d = d - 9
			}
		}

		sum += d
		isSecond = !isSecond
	}

	return sum%10 == 0
}
