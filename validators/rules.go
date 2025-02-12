package validators

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/leodido/go-urn"
	"github.com/michaelolof/gofi/utils"
)

var errValid error = errors.New("value is invalid")

func IsRequired(kind reflect.Kind) func(val any) error {
	errReq := errors.New("value is required")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsRequired(kind)(val)
			} else {
				return errValid
			}
		}
	case reflect.String:
		return func(val any) error {
			if v, ok := val.(string); ok {
				if v == "" {
					return errReq
				}
			} else {
				return errValid
			}
			return nil
		}
	case reflect.Bool:
		return func(val any) error {
			if v, ok := val.(bool); ok {
				if !v {
					return errReq
				}
			} else {
				return errValid
			}
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128:
		return func(val any) error {
			switch v := val.(type) {
			case int:
				if v == 0 {
					return errReq
				}
			case int8:
				if v == 0 {
					return errReq
				}
			case int16:
				if v == 0 {
					return errReq
				}
			case int32:
				if v == 0 {
					return errReq
				}
			case int64:
				if v == 0 {
					return errReq
				}
			case uint:
				if v == 0 {
					return errReq
				}
			case uint8:
				if v == 0 {
					return errReq
				}
			case uint16:
				if v == 0 {
					return errReq
				}
			case uint32:
				if v == 0 {
					return errReq
				}
			case uint64:
				if v == 0 {
					return errReq
				}
			case float32:
				if v == 0 {
					return errReq
				}
			case float64:
				if v == 0 {
					return errReq
				}
			case complex64:
				if v == 0 {
					return errReq
				}
			case complex128:
				if v == 0 {
					return errReq
				}
			default:
				return errValid
			}
			return nil
		}
	case reflect.Slice, reflect.Array:
		return func(val any) error {
			if v := reflect.ValueOf(val); v.Kind() == kind {
				if v.Len() == 0 {
					return errReq
				}
			} else {
				return errValid
			}
			return nil
		}
	case reflect.Map:
		return func(val any) error {
			if v := reflect.ValueOf(val); v.Kind() == kind {
				if v.Len() == 0 {
					return errReq
				}
			} else {
				return errValid
			}
			return nil
		}
	default:
		return func(val any) error {
			if v := reflect.ValueOf(val); v.Kind() == kind {
				if v.IsNil() {
					return errReq
				}
			} else {
				return errValid
			}
			return nil
		}
	}
}

func IsNotEmpty(kind reflect.Kind) func(val any) error {
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
		} else if _, ok := val.(bool); kind == reflect.Bool && ok {
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

// Validates if the value is a valid file url
func IsFileURL(kind reflect.Kind) func(val any) error {
	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsFileURL(kind)(val)
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

// Validates if a primitive value is one of the defined arguments
func IsOneOf(kind reflect.Kind, args ...any) func(val any) error {

	if !isPrimitiveKind(kind) {
		return func(val any) error {
			return errors.New("cannot compare non primitive values with oneOf")
		}
	}

	return func(val any) error {
		for i := 0; i < len(args); i++ {
			if args[i] == val {
				return nil
			}
		}
		return errors.New("given value '" + fmt.Sprintf("%v", val) + "' not supported")
	}
}

// Validates if the value is a valid v4 or v6 CIDR address.
func IsCIDR(kind reflect.Kind) func(val any) error {
	invalidErr := errors.New("invalid value passed. CIDR value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsCIDR(kind)(val)
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
func IsCIDRv4(kind reflect.Kind) func(val any) error {
	invalidErr := errors.New("invalid CIDRv4 value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsCIDRv4(kind)(val)
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
func IsCIDRv6(kind reflect.Kind) func(val any) error {
	invalidErr := errors.New("invalid CIDRv6 value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsCIDRv6(kind)(val)
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

// Validates if the value is a valid data URI.
func IsDataURI(kind reflect.Kind) func(val any) error {
	invalid := errors.New("invalid data uri value")
	invalidStr := errors.New("invalid data uri value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsDataURI(kind)(val)
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

// Validates if the value is a valid FQDN value
func IsFQDN(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid FQDN value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsFQDN(kind)(val)
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
func IsHostnameRFC952(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid hostname value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsHostnameRFC952(kind)(val)
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
func IsHostnamePort(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid host port value. value must be a string")
	invalid := errors.New("invalid host port value")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsHostnamePort(kind)(val)
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

func IsHostnameRFC1123(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid host name value. value must be a string")
	invalid := errors.New("invalid host name value")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsHostnameRFC1123(kind)(val)
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

// Validates if a value is a valid v4 or v6 IP address.
func IsIP(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid ip value. value must be a string")
	invalid := errors.New("invalid ip value")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsIP(kind)(val)
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

// Validates if a value is a resolvable ip4 address.
func IsIP4AddrResolvable(kind reflect.Kind) func(val any) error {
	return func(val any) error {
		err := IsIPv4(kind)(val)
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
func IsIP6AddrResolvable(kind reflect.Kind) func(val any) error {
	return func(val any) error {
		err := IsIPv6(kind)(val)
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
func IsIPAddrResolvable(kind reflect.Kind) func(val any) error {
	return func(val any) error {
		err := IsIP(kind)(val)
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

// Validates if a value is a valid v4 IP address.
func IsIPv4(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid ipv4 value. value must be a string")
	invalid := errors.New("invalid ipv4 value")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsIPv4(kind)(val)
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
func IsIPv6(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid ipv6 value. value must be a string")
	invalid := errors.New("invalid ipv6 value")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsIPv6(kind)(val)
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

// Validates if a value is a valid MAC address.
func IsMAC(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid MAC value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsMAC(kind)(val)
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
func IsTCP4AddrResolvable(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid tcp4 address. value must be a string")

	return func(val any) error {
		err := IsIP4Addr(kind)(val)
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
func IsTCP6AddrResolvable(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid tcp6 address. value must be a string")

	return func(val any) error {
		err := IsIP6Addr(kind)(val)
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
func IsTCPAddrResolvable(kind reflect.Kind) func(val any) error {
	return func(val any) error {
		if err := IsIP4Addr(kind)(val); err != nil {
			return err
		}

		if err := IsIP6Addr(kind)(val); err != nil {
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
func IsUDP4AddrResolvable(kind reflect.Kind) func(val any) error {
	return func(val any) error {
		if err := IsIP4Addr(kind)(val); err != nil {
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
func IsUDP6AddrResolvable(kind reflect.Kind) func(val any) error {
	return func(val any) error {
		if err := IsIP6Addr(kind)(val); err != nil {
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
func IsUDPAddrResolvable(kind reflect.Kind) func(val any) error {
	return func(val any) error {
		if err := IsIP4Addr(kind)(val); err != nil {
			return err
		}

		if err := IsIP6Addr(kind)(val); err != nil {
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
func IsUnixAddrResolvable(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid unix address. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsUnixAddrResolvable(kind)(val)
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

func IsIP4Addr(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid ipv4 address. value must be a string")
	invalid := errors.New("invalid ipv4 address")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsIP4Addr(kind)(val)
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

func IsIP6Addr(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid ipv6 address. value must be a string")
	invalid := errors.New("invalid ipv6 address")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsIP6Addr(kind)(val)
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

// Validates if the current value is a valid URI.
func IsURI(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid URI value. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsURI(kind)(val)
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
func IsURL(kind reflect.Kind) func(val any) error {

	invalidStr := errors.New("only string values are allowed as url")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsURL(kind)(val)
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
func IsHttpURL(kind reflect.Kind) func(val any) error {
	invalid := errors.New("invalid http(s) url")
	invalidStr := errors.New("invalid http(s) url. value must be a string")

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsHttpURL(kind)(val)
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

			if err := IsURL(kind)(val); err != nil {
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

func IsURLEncoded(kind reflect.Kind) func(val any) error {
	return func(val any) error {
		if s := val.(string); URLEncodedRegex.MatchString(s) {
			return nil
		} else {
			return errors.New("url encoding in the wrong format")
		}
	}
}

// isUrnRFC2141 is the validation function for validating if the current field's value is a valid URN as per RFC 2141.
func IsUrnRFC2141(kind reflect.Kind) func(val any) error {
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

// IsMax checks that a given value must be below or equal to a specific limit
func IsMax(kind reflect.Kind, limit ...any) func(val any) error {
	var err error
	var max float64
	if len(limit) != 1 {
		err = errors.New("validation rule 'IsMax' requires 1 limit argument")
	} else {
		max, err = utils.AnyValueToFloat(limit[0])
	}

	check := func(isValid bool) error {
		if isValid {
			return nil
		} else {
			return fmt.Errorf("value must be equal or lesser than maximum limit of %f", max)
		}
	}

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			if err != nil {
				return err
			}

			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsMax(kind)(val)
			} else {
				return errors.New("invalid value passed to IsMax function")
			}
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		return func(val any) error {
			if err != nil {
				return err
			}

			v := reflect.ValueOf(val)
			return check(float64(v.Len()) <= max)
		}
	default:
		return func(val any) error {
			switch v := val.(type) {
			case int:
				return check(float64(v) <= max)
			case int8:
				return check(float64(v) <= max)
			case int16:
				return check(float64(v) <= max)
			case int32:
				return check(float64(v) <= max)
			case int64:
				return check(float64(v) <= max)
			case uint:
				return check(float64(v) <= max)
			case uint8:
				return check(float64(v) <= max)
			case uint16:
				return check(float64(v) <= max)
			case uint32:
				return check(float64(v) <= max)
			case uint64:
				return check(float64(v) <= max)
			case float32:
				return check(float64(v) <= max)
			case float64:
				return check(v <= max)
			case string:
				return check(float64(len(v)) <= max)
			default:
				return fmt.Errorf("unsupported type %T when validating maximum value", v)
			}
		}
	}
}

// IsMin checks that a given value must be below or equal to a specific limit
func IsMin(kind reflect.Kind, limit ...any) func(val any) error {
	var err error
	var min float64
	if len(limit) != 1 {
		err = errors.New("validation rule 'IsMin' requires 1 limit argument")
	} else {
		min, err = utils.AnyValueToFloat(limit[0])
	}

	check := func(isValid bool) error {
		if isValid {
			return nil
		} else {
			return fmt.Errorf("value must be equal or greater than minimum limit of %f", min)
		}
	}

	switch kind {
	case reflect.Invalid:
		return func(val any) error {
			if err != nil {
				return err
			}

			kind = reflect.TypeOf(val).Kind()
			if kind != reflect.Invalid {
				return IsMin(kind)(val)
			} else {
				return errors.New("invalid value passed to IsMin function")
			}
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		return func(val any) error {
			if err != nil {
				return err
			}

			v := reflect.ValueOf(val)
			return check(float64(v.Len()) >= min)
		}
	default:
		return func(val any) error {
			switch v := val.(type) {
			case int:
				return check(float64(v) >= min)
			case int8:
				return check(float64(v) >= min)
			case int16:
				return check(float64(v) >= min)
			case int32:
				return check(float64(v) >= min)
			case int64:
				return check(float64(v) >= min)
			case uint:
				return check(float64(v) >= min)
			case uint8:
				return check(float64(v) >= min)
			case uint16:
				return check(float64(v) >= min)
			case uint32:
				return check(float64(v) >= min)
			case uint64:
				return check(float64(v) >= min)
			case float32:
				return check(float64(v) >= min)
			case float64:
				return check(v >= min)
			case string:
				return check(float64(len(v)) >= min)
			default:
				return fmt.Errorf("unsupported type %T when validating minimum value", v)
			}
		}
	}
}
