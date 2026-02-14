package rules

import (
	"errors"
	"reflect"
)

// Helper function to validate string with regex
func validateRegex(c ValidatorContext, regexName string, regexMatcher interface{ MatchString(string) bool }) func(val any) error {
	invalidStr := errors.New("invalid " + regexName + " value. value must be a string")
	invalid := errors.New("invalid " + regexName + " value")

	return func(val any) error {
		kind := c.Kind
		if kind == reflect.Invalid {
			kind = reflect.TypeOf(val).Kind()
		}

		if kind == reflect.String {
			v, ok := val.(string)
			if !ok {
				return invalidStr
			}
			if regexMatcher.MatchString(v) {
				return nil
			}
			return invalid
		}
		return invalidStr
	}
}

// String/Text Rules

func IsAlpha(c ValidatorContext) func(val any) error {
	return validateRegex(c, "alpha", AlphaRegex)
}

func IsAlphaNumeric(c ValidatorContext) func(val any) error {
	return validateRegex(c, "alphanumeric", AlphaNumericRegex)
}

func IsAlphaUnicode(c ValidatorContext) func(val any) error {
	return validateRegex(c, "alpha unicode", AlphaUnicodeRegex)
}

func IsAlphaUnicodeNumeric(c ValidatorContext) func(val any) error {
	return validateRegex(c, "alpha unicode numeric", AlphaUnicodeNumericRegex)
}

func IsASCII(c ValidatorContext) func(val any) error {
	return validateRegex(c, "ascii", ASCIIRegex)
}

func IsPrintableASCII(c ValidatorContext) func(val any) error {
	return validateRegex(c, "printable ascii", PrintableASCIIRegex)
}

func IsMultibyte(c ValidatorContext) func(val any) error {
	return validateRegex(c, "multibyte", MultibyteRegex)
}

// Numbers/Formatted Strings Rules

func IsNumeric(c ValidatorContext) func(val any) error {
	return validateRegex(c, "numeric", NumericRegex)
}

func IsNumber(c ValidatorContext) func(val any) error {
	return validateRegex(c, "number", NumberRegex)
}

func IsHexadecimal(c ValidatorContext) func(val any) error {
	return validateRegex(c, "hexadecimal", HexadecimalRegex)
}

func IsHexColor(c ValidatorContext) func(val any) error {
	return validateRegex(c, "hex color", HexColorRegex)
}

func IsRGB(c ValidatorContext) func(val any) error {
	return validateRegex(c, "rgb", RgbRegex)
}

func IsRGBA(c ValidatorContext) func(val any) error {
	return validateRegex(c, "rgba", RgbaRegex)
}

func IsHSL(c ValidatorContext) func(val any) error {
	return validateRegex(c, "hsl", HslRegex)
}

func IsHSLA(c ValidatorContext) func(val any) error {
	return validateRegex(c, "hsla", HslaRegex)
}

// Identity/Codes Rules

func IsEmail(c ValidatorContext) func(val any) error {
	return validateRegex(c, "email", EmailRegex)
}

func IsE164(c ValidatorContext) func(val any) error {
	return validateRegex(c, "e164", E164Regex)
}

func IsISBN10(c ValidatorContext) func(val any) error {
	return validateRegex(c, "isbn10", ISBN10Regex)
}

func IsISBN13(c ValidatorContext) func(val any) error {
	return validateRegex(c, "isbn13", ISBN13Regex)
}

func IsISSN(c ValidatorContext) func(val any) error {
	return validateRegex(c, "issn", ISSNRegex)
}

func IsUUID(c ValidatorContext) func(val any) error {
	return validateRegex(c, "uuid", UUIDRegex)
}

func IsUUID3(c ValidatorContext) func(val any) error {
	return validateRegex(c, "uuid3", UUID3Regex)
}

func IsUUID4(c ValidatorContext) func(val any) error {
	return validateRegex(c, "uuid4", UUID4Regex)
}

func IsUUID5(c ValidatorContext) func(val any) error {
	return validateRegex(c, "uuid5", UUID5Regex)
}

func IsUUIDRFC4122(c ValidatorContext) func(val any) error {
	return validateRegex(c, "uuid (rfc4122)", UUIDRFC4122Regex)
}

func IsUUID3RFC4122(c ValidatorContext) func(val any) error {
	return validateRegex(c, "uuid3 (rfc4122)", UUID3RFC4122Regex)
}

func IsUUID4RFC4122(c ValidatorContext) func(val any) error {
	return validateRegex(c, "uuid4 (rfc4122)", UUID4RFC4122Regex)
}

func IsUUID5RFC4122(c ValidatorContext) func(val any) error {
	return validateRegex(c, "uuid5 (rfc4122)", UUID5RFC4122Regex)
}

func IsULID(c ValidatorContext) func(val any) error {
	return validateRegex(c, "ulid", ULIDRegex)
}

func IsSSN(c ValidatorContext) func(val any) error {
	return validateRegex(c, "ssn", SSNRegex)
}

func IsBIC(c ValidatorContext) func(val any) error {
	return validateRegex(c, "bic", BicRegex)
}

func IsSemver(c ValidatorContext) func(val any) error {
	return validateRegex(c, "semver", SemverRegex)
}

func IsCVE(c ValidatorContext) func(val any) error {
	return validateRegex(c, "cve", CveRegex)
}

// Encoding/Hash Rules

func IsBase32(c ValidatorContext) func(val any) error {
	return validateRegex(c, "base32", Base32Regex)
}

func IsBase64(c ValidatorContext) func(val any) error {
	return validateRegex(c, "base64", Base64Regex)
}

func IsBase64URL(c ValidatorContext) func(val any) error {
	return validateRegex(c, "base64url", Base64URLRegex)
}

func IsBase64RawURL(c ValidatorContext) func(val any) error {
	return validateRegex(c, "base64rawurl", Base64RawURLRegex)
}

func IsMD4(c ValidatorContext) func(val any) error {
	return validateRegex(c, "md4", Md4Regex)
}

func IsMD5(c ValidatorContext) func(val any) error {
	return validateRegex(c, "md5", Md5Regex)
}

func IsSHA256(c ValidatorContext) func(val any) error {
	return validateRegex(c, "sha256", Sha256Regex)
}

func IsSHA384(c ValidatorContext) func(val any) error {
	return validateRegex(c, "sha384", Sha384Regex)
}

func IsSHA512(c ValidatorContext) func(val any) error {
	return validateRegex(c, "sha512", Sha512Regex)
}

func IsRIPEMD128(c ValidatorContext) func(val any) error {
	return validateRegex(c, "ripemd128", Ripemd128Regex)
}

func IsRIPEMD160(c ValidatorContext) func(val any) error {
	return validateRegex(c, "ripemd160", Ripemd160Regex)
}

func IsTiger128(c ValidatorContext) func(val any) error {
	return validateRegex(c, "tiger128", Tiger128Regex)
}

func IsTiger160(c ValidatorContext) func(val any) error {
	return validateRegex(c, "tiger160", Tiger160Regex)
}

func IsTiger192(c ValidatorContext) func(val any) error {
	return validateRegex(c, "tiger192", Tiger192Regex)
}

func IsHTMLEncoded(c ValidatorContext) func(val any) error {
	return validateRegex(c, "html encoded", HTMLEncodedRegex)
}

func IsJWT(c ValidatorContext) func(val any) error {
	return validateRegex(c, "jwt", JWTRegex)
}

// Network/Web (Extensions) Rules

func IsHTML(c ValidatorContext) func(val any) error {
	return validateRegex(c, "html", HTMLRegex)
}

func IsCron(c ValidatorContext) func(val any) error {
	return validateRegex(c, "cron", CronRegex)
}

func IsSpiceDBID(c ValidatorContext) func(val any) error {
	return validateRegex(c, "spicedb id", SpicedbIDRegex)
}

func IsSpiceDBPermission(c ValidatorContext) func(val any) error {
	return validateRegex(c, "spicedb permission", SpicedbPermissionRegex)
}

func IsSpiceDBType(c ValidatorContext) func(val any) error {
	return validateRegex(c, "spicedb type", SpicedbTypeRegex)
}

// Geo/Crypto/DB Rules

func IsLatitude(c ValidatorContext) func(val any) error {
	return validateRegex(c, "latitude", LatitudeRegex)
}

func IsLongitude(c ValidatorContext) func(val any) error {
	return validateRegex(c, "longitude", LongitudeRegex)
}

func IsBTCAddress(c ValidatorContext) func(val any) error {
	return validateRegex(c, "bitcoin address", BtcAddressRegex)
}

func IsBTCAddressBech32(c ValidatorContext) func(val any) error {
	return func(val any) error {
		// Try both upper and lower
		errUpper := validateRegex(c, "bitcoin bech32 address", BtcUpperAddressRegexBech32)(val)
		if errUpper == nil {
			return nil
		}
		return validateRegex(c, "bitcoin bech32 address", BtcLowerAddressRegexBech32)(val)
	}
}

func IsETHAddress(c ValidatorContext) func(val any) error {
	return validateRegex(c, "ethereum address", EthAddressRegex)
}

func IsMongoDB(c ValidatorContext) func(val any) error {
	return validateRegex(c, "mongodb objectid", MongodbRegex)
}
