package validators

import (
	"net/http"
	"reflect"
)

type validatorOptionType int

const (
	ReuestType   validatorOptionType = 1
	ResponseType validatorOptionType = 2
)

func NewValidatorArg(val any, typ validatorOptionType, r *http.Request, w http.ResponseWriter) ValidatorArg {
	return ValidatorArg{val: val, typ: typ, r: r, w: w}
}

type ValidatorArg struct {
	val any
	typ validatorOptionType
	r   *http.Request
	w   http.ResponseWriter
}

func (v *ValidatorArg) Value() any {
	return v.val
}

func (v *ValidatorArg) Request() *http.Request {
	return v.r
}

func (v *ValidatorArg) Response() http.ResponseWriter {
	return v.w
}

type ValidatorFnOptions = func(kind reflect.Kind, args ...any) func(val any) error
type LegacyValidatorFn = func(kind reflect.Kind) func(val any) error
type ValidatorFn = func(val ValidatorArg) error

type ValidatorContext struct {
	Type    reflect.Type
	Kind    reflect.Kind
	Options []any
}

type ContextValidator = func(c ValidatorContext) ValidatorFn
type ContextValidators map[string]ContextValidator

var Validators = ContextValidators{
	"required": IsRequired,
	"oneof":    IsOneOf,
}

var BaseValidators = map[string]LegacyValidatorFn{
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
	"max": IsMax,
}
