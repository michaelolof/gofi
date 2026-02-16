package rules

import (
	"errors"
	"net/url"
	"reflect"
	"strings"

	"github.com/leodido/go-urn"
)

// Validates if the value is a valid file url
func IsFileURL(c ValidatorContext) func(val any) error {
	kind := c.Kind
	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				// Re-wrap with correct kind, creating a temporary context
				tempC := c
				tempC.Kind = kind
				return IsFileURL(tempC)(val)
			} else {
				return errValid
			}
		}
	case reflect.String:
		return func(val any) error {
			v, ok := val.(string)
			if !ok {
				return errors.New("only string values are allowed as file url")
			}

			s := strings.ToLower(v)

			if len(s) == 0 {
				return errors.New("file url value cannot be empty")
			}

			return isFileURL(s)
		}
	default:
		return func(val any) error {
			return errors.New("only string values are allowed as file url")
		}
	}
}

// Validates if the value is a valid data URI.
func IsDataURI(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalid := errors.New("invalid data uri value")
	invalidStr := errors.New("invalid data uri value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsDataURI(tempC)(val)
			} else {
				return invalidStr
			}
		}
	case reflect.String:
		return func(val any) error {
			v, ok := val.(string)
			if !ok {
				return invalidStr
			}
			uri := strings.SplitN(v, ",", 2)
			if len(uri) != 2 {
				return invalid
			}

			if !DataURIRegex.MatchString(uri[0]) {
				return invalid
			}

			if !Base64Regex.MatchString(uri[1]) {
				return invalid
			}
			return nil
		}
	default:
		return func(val any) error {
			return invalidStr
		}
	}
}

// Validates if the current value is a valid URI.
func IsURI(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid URI value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsURI(tempC)(val)
			} else {
				return invalidStr
			}
		}
	case reflect.String:
		return func(val any) error {
			v, ok := val.(string)
			if !ok {
				return invalidStr
			}

			// checks needed as of Go 1.6 because of change https://github.com/golang/go/commit/617c93ce740c3c3cc28cdd1a0d712be183d0b328#diff-6c2d018290e298803c0c9419d8739885L195
			// emulate browser and strip the '#' suffix prior to validation. see issue-#237
			if i := strings.Index(v, "#"); i > -1 {
				v = v[:i]
			}

			if len(v) == 0 {
				return errors.New("invalid URI value. value cannot be empty")
			}

			_, err := url.ParseRequestURI(v)

			return err
		}
	default:
		return func(val any) error {
			return invalidStr
		}
	}
}

// Validates if the current value is a valid URL
func IsURL(c ValidatorContext) func(val any) error {
	kind := c.Kind

	invalidStr := errors.New("only string values are allowed as url")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsURL(tempC)(val)
			} else {
				return invalidStr
			}
		}
	case reflect.String:
		return func(val any) error {
			v, ok := val.(string)
			if !ok {
				return invalidStr
			}

			s := strings.ToLower(v)

			if len(s) == 0 {
				return errors.New("url value cannot be empty")
			}

			err := isFileURL(s)
			if err == nil {
				return nil
			}

			url, err := url.Parse(s)
			if err != nil || url.Scheme == "" {
				return errors.New("invalid url format")
			}

			if url.Host == "" && url.Fragment == "" && url.Opaque == "" {
				return errors.New("invalid url format")
			}

			return nil
		}
	default:
		return func(val any) error {
			return invalidStr
		}
	}
}

// isHttpURL is the validation function for validating if the current field's value is a valid HTTP(s) URL.
func IsHttpURL(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalid := errors.New("invalid http(s) url")
	invalidStr := errors.New("invalid http(s) url. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsHttpURL(tempC)(val)
			} else {
				return invalidStr
			}
		}
	case reflect.String:
		return func(val any) error {
			v, ok := val.(string)
			if !ok {
				return invalidStr
			}

			if err := IsURL(c)(val); err != nil {
				return err
			}

			s := strings.ToLower(v)
			url, err := url.Parse(s)
			if err != nil || url.Host == "" {
				return invalid
			}

			if url.Scheme == "http" || url.Scheme == "https" {
				return nil
			} else {
				return invalid
			}
		}
	default:
		return func(val any) error {
			return invalidStr
		}
	}
}

func IsURLEncoded(c ValidatorContext) func(val any) error {
	return func(val any) error {
		if s := val.(string); URLEncodedRegex.MatchString(s) {
			return nil
		} else {
			return errors.New("url encoding in the wrong format")
		}
	}
}

// isUrnRFC2141 is the validation function for validating if the current field's value is a valid URN as per RFC 2141.
func IsUrnRFC2141(c ValidatorContext) func(val any) error {
	return func(val any) error {
		v, ok := val.(string)
		if !ok {
			return errors.New("invalid URN value. value must be a string")
		}

		_, match := urn.Parse([]byte(v))
		if !match {
			return errors.New("invalid URN value")
		}

		return nil
	}
}

func isFileURL(path string) error {
	if !strings.HasPrefix(path, "file:/") {
		return errors.New("file url must start with file:/")
	}
	_, err := url.ParseRequestURI(path)
	return err
}
