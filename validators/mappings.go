package validators

import "reflect"

type ValidatorFnOptions = func(kind reflect.Kind, args ...any) func(val any) error
type ValidatorFn = func(kind reflect.Kind) func(val any) error
type CompiledValidatorFn = func(val any) error
type MappedValidators = map[string]ValidatorFnOptions

var BaseValidators = map[string]ValidatorFn{
	"cidr":             IsCIDR,
	"cidrv4":           IsCIDRv4,
	"cidrv6":           IsCIDRv6,
	"datauri":          IsDataURI,
	"fileUrl":          IsFileURL,
	"fqdn":             IsFQDN,
	"hostname":         IsHostnameRFC952,
	"hostname_port":    IsHostnamePort,
	"hostname_rfc1123": IsHostnameRFC1123,
	"ip":               IsIP,
	"ip4_addr":         IsIP4AddrResolvable,
	"ip6_addr":         IsIP6AddrResolvable,
	"ip_addr":          IsIPAddrResolvable,
	"ipv4":             IsIPv4,
	"ipv6":             IsIPv6,
	"mac":              IsMAC,
	"not_empty":        IsNotEmpty,
	"required":         IsRequired,
	"tcp4_addr":        IsTCP4AddrResolvable,
	"tcp6_addr":        IsTCP6AddrResolvable,
	"tcp_addr":         IsTCPAddrResolvable,
	"udp4_addr":        IsUDP4AddrResolvable,
	"udp6_addr":        IsUDP6AddrResolvable,
	"udp_addr":         IsUDPAddrResolvable,
	"unix_addr":        IsUnixAddrResolvable,
	"uri":              IsURI,
	"url":              IsURL,
	"http_url":         IsHttpURL,
	"url_encoded":      IsURLEncoded,
	"urn_rfc2141":      IsUrnRFC2141,
}

var OptionValidators = map[string]ValidatorFnOptions{
	"max":   IsMax,
	"oneof": IssOneOf,
}
