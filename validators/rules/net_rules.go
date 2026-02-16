package rules

import (
	"errors"
	"net"
	"reflect"
	"strconv"
)

// Validates if the value is a valid v4 or v6 CIDR address.
func IsCIDR(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidErr := errors.New("invalid value passed. CIDR value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsCIDR(tempC)(val)
			} else {
				return invalidErr
			}
		}
	case reflect.String:
		return func(val any) error {
			v, ok := val.(string)
			if !ok {
				return invalidErr
			}
			_, _, err := net.ParseCIDR(v)
			return err
		}
	default:
		return func(val any) error {
			return invalidErr
		}
	}
}

// Validates if the value is a valid v4 CIDR address.
func IsCIDRv4(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidErr := errors.New("invalid CIDRv4 value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsCIDRv4(tempC)(val)
			} else {
				return invalidErr
			}
		}
	case reflect.String:
		return func(val any) error {
			v, ok := val.(string)
			if !ok {
				return invalidErr
			}
			ip, net, err := net.ParseCIDR(v)
			if ip.To4() == nil {
				return errors.New("invalid CIDRv4 value. value must be a IPv4 address")
			}
			if !net.IP.Equal(ip) {
				return errors.New("invalid CIDR value. ip and x values don't match")
			}
			return err
		}
	default:
		return func(val any) error {
			return invalidErr
		}
	}
}

// Validates if the value is a valid v6 CIDR address.
func IsCIDRv6(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidErr := errors.New("invalid CIDRv6 value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsCIDRv6(tempC)(val)
			} else {
				return invalidErr
			}
		}
	case reflect.String:
		return func(val any) error {
			v, ok := val.(string)
			if !ok {
				return invalidErr
			}

			ip, _, err := net.ParseCIDR(v)
			if ip.To4() != nil {
				return errors.New("invalid CIDRv6 value. value must be a IPv6 address")
			}

			return err
		}
	default:
		return func(val any) error {
			return invalidErr
		}
	}
}

// Validates if a value is a valid MAC address.
func IsMAC(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid MAC value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsMAC(tempC)(val)
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
			_, err := net.ParseMAC(v)
			return err
		}
	default:
		return func(val any) error {
			return invalidStr
		}
	}
}

// Validates if a value is a resolvable tcp4 address.
func IsTCP4AddrResolvable(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid tcp4 address. value must be a string")

	return func(val any) error {
		tempC := c
		tempC.Kind = kind
		err := IsIP4Addr(tempC)(val)
		if err != nil {
			return err
		}

		v, ok := val.(string)
		if !ok {
			return invalidStr
		}

		_, err = net.ResolveTCPAddr("tcp4", v)
		return err
	}
}

// Validates if a value is a resolvable tcp6 address.
func IsTCP6AddrResolvable(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid tcp6 address. value must be a string")

	return func(val any) error {
		tempC := c
		tempC.Kind = kind
		err := IsIP6Addr(tempC)(val)
		if err != nil {
			return err
		}

		v, ok := val.(string)
		if !ok {
			return invalidStr
		}
		_, err = net.ResolveTCPAddr("tcp6", v)

		return err
	}
}

// Validates if a value is a resolvable tcp address.
func IsTCPAddrResolvable(c ValidatorContext) func(val any) error {
	kind := c.Kind
	return func(val any) error {
		tempC := c
		tempC.Kind = kind
		if err := IsIP4Addr(tempC)(val); err != nil {
			return err
		}

		if err := IsIP6Addr(tempC)(val); err != nil {
			return err
		}

		v, ok := val.(string)
		if !ok {
			return errors.New("invalid tcp address value")
		}
		_, err := net.ResolveTCPAddr("tcp", v)

		return err
	}
}

// Validates if a value is a resolvable udp4 address.
func IsUDP4AddrResolvable(c ValidatorContext) func(val any) error {
	kind := c.Kind
	return func(val any) error {
		tempC := c
		tempC.Kind = kind
		if err := IsIP4Addr(tempC)(val); err != nil {
			return err
		}

		v, ok := val.(string)
		if !ok {
			return errors.New("invalid udp4 address")
		}

		_, err := net.ResolveUDPAddr("udp4", v)
		return err
	}
}

// Validates if a value is a resolvable udp6 address.
func IsUDP6AddrResolvable(c ValidatorContext) func(val any) error {
	kind := c.Kind
	return func(val any) error {
		tempC := c
		tempC.Kind = kind
		if err := IsIP6Addr(tempC)(val); err != nil {
			return err
		}

		v, ok := val.(string)
		if !ok {
			return errors.New("invalid udp6 address")
		}
		_, err := net.ResolveUDPAddr("udp6", v)

		return err
	}
}

// Validates if a value is a resolvable udp address.
func IsUDPAddrResolvable(c ValidatorContext) func(val any) error {
	kind := c.Kind
	return func(val any) error {
		tempC := c
		tempC.Kind = kind
		if err := IsIP4Addr(tempC)(val); err != nil {
			return err
		}

		if err := IsIP6Addr(tempC)(val); err != nil {
			return err
		}

		v, ok := val.(string)
		if !ok {
			return errors.New("invalid udp address value")
		}

		_, err := net.ResolveUDPAddr("udp", v)

		return err
	}
}

// Validates if a value is a resolvable unix address.
func IsUnixAddrResolvable(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid unix address. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsUnixAddrResolvable(tempC)(val)
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
			_, err := net.ResolveUnixAddr("unix", v)
			return err
		}
	default:
		return func(val any) error {
			return invalidStr
		}
	}
}

// Validates if the value is a valid FQDN value
func IsFQDN(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid FQDN value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsFQDN(tempC)(val)
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
			if v == "" {
				return errors.New("invalid FQDN value. value cannot be empty")
			}

			if FqdnRegexRFC1123.MatchString(v) {
				return nil
			} else {
				return invalidStr
			}
		}
	default:
		return func(val any) error {
			return invalidStr
		}
	}
}

// Checks whether a value is Hostname RFC 952
func IsHostnameRFC952(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid hostname value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsHostnameRFC952(tempC)(val)
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
			if v == "" {
				return errors.New("invalid FQDN value. value cannot be empty")
			}

			if FqdnRegexRFC1123.MatchString(v) {
				return nil
			} else {
				return invalidStr
			}
		}
	default:
		return func(val any) error {
			return invalidStr
		}
	}
}

// Validates a <dns>:<port> combination for fields typically used for socket address.
func IsHostnamePort(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid host port value. value must be a string")
	invalid := errors.New("invalid host port value")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsHostnamePort(tempC)(val)
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

			host, port, err := net.SplitHostPort(v)
			if err != nil {
				return invalid
			}
			// Port must be a iny <= 65535.
			if portNum, err := strconv.ParseInt(port, 10, 32); err != nil || portNum > 65535 || portNum < 1 {
				return invalid
			}

			// If host is specified, it should match a DNS name
			if host != "" {
				if HostnameRegexRFC1123.MatchString(host) {
					return nil
				} else {
					return invalid
				}
			}

			return nil
		}
	default:
		return func(val any) error {
			return invalidStr
		}
	}
}

func IsHostnameRFC1123(c ValidatorContext) func(val any) error {
	kind := c.Kind
	invalidStr := errors.New("invalid host name value. value must be a string")
	invalid := errors.New("invalid host name value")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				tempC := c
				tempC.Kind = kind
				return IsHostnameRFC1123(tempC)(val)
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

			if HostnameRegexRFC1123.MatchString(v) {
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
