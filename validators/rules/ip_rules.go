package rules

import (
	"errors"
	"net"
	"reflect"
	"strings"
)

// Validates if a value is a valid v4 or v6 IP address.
func IsIP(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid ip value. value must be a string")
	invalid := errors.New("invalid ip value")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsIP(tempC)(val)
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
			if net.ParseIP(v) == nil {
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

// Validates if a value is a valid v4 IP address.
func IsIPv4(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid ipv4 value. value must be a string")
	invalid := errors.New("invalid ipv4 value")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsIPv4(tempC)(val)
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

			ip := net.ParseIP(v)
			if ip != nil && ip.To4() != nil {
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

// Validates if a value is a valid v6 IP address.
func IsIPv6(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid ipv6 value. value must be a string")
	invalid := errors.New("invalid ipv6 value")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsIPv6(tempC)(val)
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

			ip := net.ParseIP(v)
			if ip != nil && ip.To4() == nil {
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

// Validates if a value is a resolvable ip4 address.
func IsIP4AddrResolvable(c ValidatorContext) func(val any) error {
	kind := c.Kind
	return func(val any) error {
		// reuse IsIPv4 logic
		tempC := c
		tempC.Kind = kind
		err := IsIPv4(tempC)(val)
		if err != nil {
			return err
		}

		v, ok := val.(string)
		if !ok {
			return errors.New("invalid ipv4 value")
		}

		_, err = net.ResolveIPAddr("ip4", v)
		return err
	}
}

// Validates if a value is a resolvable ip6 address.
func IsIP6AddrResolvable(c ValidatorContext) func(val any) error {
	kind := c.Kind
	return func(val any) error {
		tempC := c
		tempC.Kind = kind
		err := IsIPv6(tempC)(val)
		if err != nil {
			return err
		}

		v, ok := val.(string)
		if !ok {
			return errors.New("invalid ipv6 value")
		}

		_, err = net.ResolveIPAddr("ip6", v)
		return err
	}
}

// Validates if a value is a resolvable ip address.
func IsIPAddrResolvable(c ValidatorContext) func(val any) error {
	kind := c.Kind
	return func(val any) error {
		tempC := c
		tempC.Kind = kind
		err := IsIP(tempC)(val)
		if err != nil {
			return err
		}

		v, ok := val.(string)
		if !ok {
			return errors.New("invalid ip value")
		}

		_, err = net.ResolveIPAddr("ip", v)
		return err
	}
}

func IsIP4Addr(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid ipv4 address. value must be a string")
	invalid := errors.New("invalid ipv4 address")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsIP4Addr(tempC)(val)
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

			if idx := strings.LastIndex(v, ":"); idx != -1 {
				v = v[0:idx]
			}

			ip := net.ParseIP(v)

			if ip != nil && ip.To4() != nil {
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

func IsIP6Addr(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid ipv6 address. value must be a string")
	invalid := errors.New("invalid ipv6 address")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsIP6Addr(tempC)(val)
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

			if idx := strings.LastIndex(v, ":"); idx != -1 {
				if idx != 0 && v[idx-1:idx] == "]" {
					v = v[1 : idx-1]
				}
			}

			ip := net.ParseIP(v)

			if ip != nil && ip.To4() == nil {
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
