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

func IsRequired(kind reflect.Kind) func(val any) error {
	errReq := errors.New("value is required")
	errValid := errors.New("value is invalid")

	return func(val any) error {
		if val == nil {
			return errReq
		}

		switch kind {
		case reflect.String:
			if v, ok := val.(string); ok {
				if v == "" {
					return errReq
				}
			} else {
				return errValid
			}
		case reflect.Bool:
			if v, ok := val.(bool); ok {
				if !v {
					return errReq
				}
			} else {
				return errValid
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64,
			reflect.Complex64, reflect.Complex128:
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
		case reflect.Slice, reflect.Array:
			if v := reflect.ValueOf(val); v.Kind() == kind {
				if v.Len() == 0 {
					return errReq
				}
			} else {
				return errValid
			}
		case reflect.Map:
			if v := reflect.ValueOf(val); v.Kind() == kind {
				if v.Len() == 0 {
					return errReq
				}
			} else {
				return errValid
			}
		case reflect.Pointer:
			if v := reflect.ValueOf(val); v.Kind() == kind {
				if v.IsNil() {
					return errReq
				}
			} else {
				return errValid
			}
		default:
			return nil
		}

		return nil
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

func IsFileURL(kind reflect.Kind) func(val any) error {
	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
			v, ok := val.(string)
			if !ok {
				return errors.New("only string values are allowed as file url")
			}

			s := strings.ToLower(v)

			if len(s) == 0 {
				return errors.New("file url value cannot be empty")
			}

			return isFileURL(s)
		default:
			return errors.New("only string values are allowed as file url")
		}
	}
}

func IssOneOf(kind reflect.Kind, args ...any) func(val any) error {

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

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
			v, ok := val.(string)
			if !ok {
				return invalidErr
			}
			_, _, err := net.ParseCIDR(v)
			return err
		default:
			return invalidErr
		}

	}
}

func IsCIDRv4(kind reflect.Kind) func(val any) error {
	invalidErr := errors.New("invalid CIDRv4 value. value must be a string")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
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
		default:
			return invalidErr
		}

	}
}

// Validates if the value is a valid v6 CIDR address.
func IsCIDRv6(kind reflect.Kind) func(val any) error {
	invalidErr := errors.New("invalid CIDRv6 value. value must be a string")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
			v, ok := val.(string)
			if !ok {
				return invalidErr
			}

			ip, _, err := net.ParseCIDR(v)
			if ip.To4() != nil {
				return errors.New("invalid CIDRv6 value. value must be a IPv6 address")
			}

			return err
		default:
			return invalidErr
		}
	}
}

// Validates if the value is a valid data URI.
func IsDataURI(kind reflect.Kind) func(val any) error {
	invalid := errors.New("invalid data uri value")
	invalidStr := errors.New("invalid data uri value. value must be a string")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
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
		default:
			return invalidStr
		}
	}
}

func IsFQDN(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid FQDN value. value must be a string")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
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
		default:
			return invalidStr
		}
	}
}

// Checks whether a value is Hostname RFC 952
func IsHostnameRFC952(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid hostname value. value must be a string")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
			v, ok := val.(string)
			if !ok {
				return invalidStr
			}

			if HostnameRegexRFC952.MatchString(v) {
				return nil
			} else {
				return errors.New("invalid hostname value")
			}
		default:
			return invalidStr
		}
	}
}

// Validates a <dns>:<port> combination for fields typically used for socket address.
func IsHostnamePort(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid host port value. value must be a string")
	invalid := errors.New("invalid host port value")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
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
		default:
			return invalidStr
		}
	}
}

func IsHostnameRFC1123(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid host name value. value must be a string")
	invalid := errors.New("invalid host name value")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
			v, ok := val.(string)
			if !ok {
				return invalidStr
			}

			if HostnameRegexRFC1123.MatchString(v) {
				return nil
			} else {
				return invalid
			}
		default:
			return invalidStr
		}
	}
}

// Validates if a value is a valid v4 or v6 IP address.
func IsIP(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid ip value. value must be a string")
	invalid := errors.New("invalid ip value")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
			v, ok := val.(string)
			if !ok {
				return invalidStr
			}
			if net.ParseIP(v) == nil {
				return invalid
			}
			return nil
		default:
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

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
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
		default:
			return invalidStr
		}
	}
}

// Validates if a value is a valid v6 IP address.
func IsIPv6(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid ipv6 value. value must be a string")
	invalid := errors.New("invalid ipv6 value")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
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
		default:
			return invalidStr
		}
	}
}

// Validates if a value is a valid MAC address.
func IsMAC(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid MAC value. value must be a string")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
			v, ok := val.(string)
			if !ok {
				return invalidStr
			}
			_, err := net.ParseMAC(v)
			return err
		default:
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

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
			v, ok := val.(string)
			if !ok {
				return invalidStr
			}
			_, err := net.ResolveUnixAddr("unix", v)
			return err
		default:
			return invalidStr
		}
	}
}

func IsIP4Addr(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid ipv4 address. value must be a string")
	invalid := errors.New("invalid ipv4 address")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
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
		default:
			return invalidStr
		}
	}
}

func IsIP6Addr(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid ipv6 address. value must be a string")
	invalid := errors.New("invalid ipv6 address")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
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
		default:
			return invalidStr
		}
	}
}

// Validates if the current value is a valid URI.
func IsURI(kind reflect.Kind) func(val any) error {
	invalidStr := errors.New("invalid URI value. value must be a string")

	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
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
		default:
			return invalidStr
		}
	}
}

// Validates if the current value is a valid URL
func IsURL(kind reflect.Kind) func(val any) error {
	return func(val any) error {
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
			v, ok := val.(string)
			if !ok {
				return errors.New("only string values are allowed as url")
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
		default:
			return errors.New("only string values are allowed as url")
		}
	}
}

// isHttpURL is the validation function for validating if the current field's value is a valid HTTP(s) URL.
func IsHttpURL(kind reflect.Kind) func(val any) error {
	invalid := errors.New("invalid http(s) url")
	invalidStr := errors.New("invalid http(s) url. value must be a string")

	return func(val any) error {
		if err := IsURL(kind)(val); err != nil {
			return err
		}

		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		switch kind {
		case reflect.String:
			v, ok := val.(string)
			if !ok {
				return invalidStr
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
		default:
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

func IsMax(kind reflect.Kind, args ...any) func(val any) error {
	var err error
	var arg float64
	if len(args) != 1 {
		err = errors.New("validation rule 'IsMax' requires 1 argument")
	} else {
		arg, err = utils.AnyValueToFloat(args[0])
	}

	return func(val any) error {
		if err != nil {
			return err
		}

		if utils.KindIsNumber(kind) {
			v, err := utils.AnyValueToFloat(val)
			if err != nil {
				return err
			}

			if v <= arg {
				return nil
			} else {
				return fmt.Errorf("value is greater than max allowed value %f", arg)
			}
		} else if kind == reflect.String {
			if v, ok := val.(string); !ok {
				return fmt.Errorf("invalid value passed to validation rule. expected string")
			} else {
				if float64(len(v)) <= arg {
					return nil
				} else {
					return fmt.Errorf("value is greater than max allowed value %f", arg)
				}
			}
		} else {
			return errors.New("unknown value passed to validation rule")
		}
	}
}
