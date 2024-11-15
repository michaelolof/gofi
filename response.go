package gofi

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/michaelolof/gofi/utils"
	"github.com/michaelolof/gofi/validators"
)

type headersSetter interface {
	CheckAndSetHeaders(c Context) error
}

type bodySetter interface {
	CheckAndBuildBody(c Context) error
}

func (c *context) checkAndSetHeaders(rules ruleDefMap, headers reflect.Value, ignore func(string) bool) error {

	var ruleProps map[string]*ruleDef
	if v, ok := rules[string(schemaHeaders)]; ok {
		ruleProps = v.properties
	}

	if len(ruleProps) == 0 {
		return nil
	}

	if !headers.IsValid() {
		return newErrReport(ResponseErr, schemaHeaders, "", "illegalValue", errors.New("headers object is invalid"))
	}

	if s, ok := headers.Interface().(headersSetter); ok {
		return s.CheckAndSetHeaders(c)
	}

	for key, val := range ruleProps {
		if ignore != nil && ignore(key) {
			continue
		}
		hv := c.w.Header().Get(key)
		if hv == "" {
			if hn := headers.FieldByName(val.name); hn.IsValid() {
				hv = fmt.Sprintf("%v", hn.Interface())
			}
		}

		if hv == "" && val.defStr != "" {
			hv = val.defStr
		}

		err := runValidation(ResponseErr, hv, schemaHeaders, key, val.rules)
		if err != nil {
			return err
		}

		if hv != "" {
			c.w.Header().Set(key, hv)
		}
	}

	return nil
}

func (c *context) checkAndBuildJson(code int, rules ruleDefMap, body reflect.Value) error {
	var bdef ruleDef
	if v, ok := rules[string(schemaBody)]; ok {
		bdef = v
	}

	if body.Kind() == reflect.Pointer {
		body = body.Elem()
	}

	if !body.IsValid() {
		if bdef.required {
			return newErrReport(ResponseErr, schemaBody, "", "required", errors.New("value is required for body"))
		} else {
			c.w.WriteHeader(code)
			return nil
		}
	}

	if bdef.kind != body.Kind() {
		return newErrReport(ResponseErr, schemaBody, "", "typeMismatch", errors.New("body schema and payload mismatch"))
	}

	if validators.IsPrimitiveKind(body.Kind()) {
		bany := body.Interface()
		err := runValidation(ResponseErr, bany, schemaBody, "", bdef.rules)
		if err != nil {
			return err
		}
		c.w.WriteHeader(code)
		return json.NewEncoder(c.w).Encode(bany)
	}

	switch bdef.kind {
	case reflect.Invalid:
		c.w.WriteHeader(code)
		return nil
	case reflect.Interface:
		bany := body.Interface()
		if bany == nil {
			if bdef.required {
				return newErrReport(ResponseErr, schemaBody, "", "required", errors.New("value is required for body"))
			} else {
				c.w.WriteHeader(code)
				return nil
			}
		} else {
			c.w.WriteHeader(code)
			return json.NewEncoder(c.w).Encode(bany)
		}
	case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
		bs := newBuildState()
		keys := make([]string, 0, 100)
		err := c.buildAndValidateJsonStruct(&bs, bdef, body, keys)
		if err != nil {
			return err
		}
		c.w.WriteHeader(code)
		_, err = c.w.Write(bs.bs)
		if err != nil {
			fmt.Println(err)
		}
	}

	return nil
}

func (c *context) buildAndValidateJsonStruct(bs *buildState, def ruleDef, val reflect.Value, keys []string) error {
	kp := strings.Join(keys, ".")

	defer func() {
		if e := recover(); e != nil {
			newErrReport(ResponseErr, schemaBody, kp, "typeMismatch", e.(error))
		}
	}()

	switch def.kind {
	case reflect.Invalid:
		panic("it is invalid. why?")
	case reflect.Bool:
		vbool := val.Bool()
		err := runValidation(ResponseErr, vbool, schemaBody, kp, def.rules)
		if err != nil {
			return err
		}

		bs.bs = strconv.AppendBool(bs.bs, vbool)
		return nil
	case reflect.Float32, reflect.Float64:
		v := val.Float()
		err := runValidation(ResponseErr, v, schemaBody, kp, def.rules)
		if err != nil {
			return err
		}

		bits := 64
		if def.kind == reflect.Float32 {
			bits = 32
		}
		err = formatJsonFloatToString(bs, bits, v)
		if err != nil {
			return newErrReport(ResponseErr, schemaBody, kp, "typeCast", err)
		}
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		vint := val.Int()
		err := runValidation(ResponseErr, vint, schemaBody, kp, def.rules)
		if err != nil {
			return err
		}

		bs.bs = strconv.AppendInt(bs.bs, vint, 10)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		vuint := val.Uint()
		err := runValidation(ResponseErr, vuint, schemaBody, kp, def.rules)
		if err != nil {
			return err
		}

		bs.bs = strconv.AppendUint(bs.bs, vuint, 10)
		return nil
	case reflect.String:
		// Inspired implementation from std lib.
		if val.Type() == reflect.TypeFor[json.Number]() {
			numStr := val.String()
			if numStr == "" {
				numStr = def.defStr
			}
			if numStr == "" {
				numStr = "0"
			}
			if !isValidNumber(numStr) {
				return newErrReport(ResponseErr, schemaBody, kp, "typeCast", fmt.Errorf("invalid number literal %q", numStr))
			}

			bs.appendByte('"')
			bs.bs = append(bs.bs, numStr...)
			bs.appendByte('"')
			return nil
		}
		vany := valueOr(val.Interface(), def.defVal, "")
		err := runValidation(ResponseErr, vany, schemaBody, kp, def.rules)
		if err != nil {
			return err
		}
		if v, ok := vany.(string); ok {
			bs.appendBytes([]byte(fmt.Sprintf("%q", v)))
			return nil
		} else if str := val.String(); str != "" {
			bs.appendBytes([]byte(fmt.Sprintf("%q", str)))
			return nil
		} else if !def.required {
			bs.appendBytes([]byte{'"', '"'})
			return nil
		} else {
			return newErrReport(ResponseErr, schemaBody, kp, "typeMismatch", errors.New("could not cast given value to string"))
		}
	case reflect.Struct:
		switch def.format {
		case utils.TimeObjectFormat:
			if val.Kind() == reflect.Pointer {
				val = val.Elem()
			}
			if v, ok := (val.Interface()).(time.Time); ok {
				bs.appendByte('"')
				bs.appendStr(v.Format(def.pattern))
				bs.appendByte('"')
				return nil
			} else {
				return newErrReport(ResponseErr, schemaBody, kp, "typeMismatch", errors.New("could not cast given time value to string"))
			}
		default:
			bs.appendByte('{')
			l := len(def.properties)
			count := 0
			for pkey, pval := range def.properties {
				bs.appendByte('"')
				bs.appendBytes([]byte(pkey))
				bs.appendByte('"')
				bs.appendByte(':')
				err := c.buildAndValidateJsonStruct(bs, *pval, val.FieldByName(pval.name), append(keys, pkey))
				if err != nil {
					return err
				}
				count++
				if count < l {
					bs.appendByte(',')
				}
			}
			bs.appendByte('}')
			return nil
		}
	case reflect.Array, reflect.Slice:
		l := val.Len()
		if def.max != nil && *def.max < float64(l) {
			return newErrReport(ResponseErr, schemaBody, kp, "rangeError", errors.New("length of array greater than maximum"))
		}

		bs.appendByte('[')
		for i := 0; i < l; i++ {
			if i > 0 {
				bs.appendByte(',')
			}
			err := c.buildAndValidateJsonStruct(bs, *def.item, val.Index(i), append(keys, fmt.Sprintf("%d", i)))
			if err != nil {
				return err
			}
		}
		bs.appendByte(']')
	case reflect.Map:
		l := val.Len()
		if def.max != nil && *def.max < float64(l) {
			return newErrReport(ResponseErr, schemaBody, kp, "rangeError", errors.New("length of array greater than maximum"))
		}

		bs.appendByte('{')
		mr := val.MapRange()
		for i := 0; mr.Next(); i++ {
			if i > 0 {
				bs.appendByte(',')
			}

			key, ok := mr.Key().Interface().(string)
			if !ok {
				return newErrReport(ResponseErr, schemaBody, kp, "typeMismatch", errors.New("map key must be of type string"))
			}
			bs.appendStr(fmt.Sprintf("%q", key))
			bs.appendByte(':')
			err := c.buildAndValidateJsonStruct(bs, *def.additionalProperties, mr.Value(), append(keys, key))
			if err != nil {
				return err
			}
		}
		// l := len(def.additionalProperties)
		// count := 0
		// for pkey, pval := range def.properties {
		// 	bs.appendByte('"')
		// 	bs.appendBytes([]byte(pkey))
		// 	bs.appendByte('"')
		// 	bs.appendByte(':')
		// 	err := c.buildAndValidateJsonStruct(bs, *pval, val.FieldByName(pval.name), append(keys, pkey))
		// 	if err != nil {
		// 		return err
		// 	}
		// 	count++
		// 	if count < l {
		// 		bs.appendByte(',')
		// 	}
		// }
		bs.appendByte('}')
		return nil
	default:
		vany := val.Interface()
		err := runValidation(ResponseErr, vany, schemaBody, kp, def.rules)
		if err != nil {
			return err
		}
		bs.appendStr("null")
		return nil
	}

	return nil
}

type buildState struct {
	bs  []byte
	rtn []byte
}

func (bs *buildState) appendByte(b byte) {
	bs.bs = append(bs.bs, b)
}

func (bs *buildState) appendBytes(b []byte) {
	bs.bs = append(bs.bs, b...)
}

func (bs *buildState) appendStr(s string) {
	bs.bs = append(bs.bs, []byte(s)...)
}

func newBuildState() buildState {
	return buildState{
		bs: make([]byte, 0, 50000),
	}
}

func formatJsonFloatToString(bs *buildState, bits int, f float64) error {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return fmt.Errorf("unsupported float value %s", strconv.FormatFloat(f, 'g', -1, int(bits)))
	}

	// Convert as if by ES6 number to string conversion.
	// This matches most other JSON generators.
	// See golang.org/issue/6384 and golang.org/issue/14135.
	// Like fmt %g, but the exponent cutoffs are different
	// and exponents themselves are not padded to two digits.
	abs := math.Abs(f)
	fmt := byte('f')
	// Note: Must use float32 comparisons for underlying float32 value to get precise cutoffs right.
	if abs != 0 {
		if bits == 64 && (abs < 1e-6 || abs >= 1e21) || bits == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21) {
			fmt = 'e'
		}
	}
	bs.bs = strconv.AppendFloat(bs.bs, f, fmt, -1, int(bits))
	if fmt == 'e' {
		// clean up e-09 to e-9
		n := len(bs.bs)
		if n >= 4 && bs.bs[n-4] == 'e' && bs.bs[n-3] == '-' && bs.bs[n-2] == '0' {
			bs.bs[n-2] = bs.bs[n-1]
			bs.bs = bs.bs[:n-1]
		}
	}
	return nil
}

// isValidNumber reports whether s is a valid JSON number literal.
func isValidNumber(s string) bool {
	// This function implements the JSON numbers grammar.
	// See https://tools.ietf.org/html/rfc7159#section-6
	// and https://www.json.org/img/number.png

	if s == "" {
		return false
	}

	// Optional -
	if s[0] == '-' {
		s = s[1:]
		if s == "" {
			return false
		}
	}

	// Digits
	switch {
	default:
		return false

	case s[0] == '0':
		s = s[1:]

	case '1' <= s[0] && s[0] <= '9':
		s = s[1:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}

	// . followed by 1 or more digits.
	if len(s) >= 2 && s[0] == '.' && '0' <= s[1] && s[1] <= '9' {
		s = s[2:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}

	// e or E followed by an optional - or + and
	// 1 or more digits.
	if len(s) >= 2 && (s[0] == 'e' || s[0] == 'E') {
		s = s[1:]
		if s[0] == '+' || s[0] == '-' {
			s = s[1:]
			if s == "" {
				return false
			}
		}
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}

	// Make sure we are at the end.
	return s == ""
}

func valueOr[T comparable](v any, or any, empty T) any {
	if _v, ok := v.(T); ok && _v == empty {
		return or
	}
	return v
}

func appendString[Bytes []byte | string](dst []byte, src Bytes, escapeHTML bool) []byte {
	const hex = "0123456789abcdef"

	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(src); {
		if b := src[i]; b < utf8.RuneSelf {
			if htmlSafeSet[b] || (!escapeHTML && safeSet[b]) {
				i++
				continue
			}
			dst = append(dst, src[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				// This encodes bytes < 0x20 except for \b, \f, \n, \r and \t.
				// If escapeHTML is set, it also escapes <, >, and &
				// because they can lead to security holes when
				// user-controlled strings are rendered into JSON
				// and served to some browsers.
				dst = append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		// TODO(https://go.dev/issue/56948): Use generic utf8 functionality.
		// For now, cast only a small portion of byte slices to a string
		// so that it can be stack allocated. This slows down []byte slightly
		// due to the extra copy, but keeps string performance roughly the same.
		n := len(src) - i
		if n > utf8.UTFMax {
			n = utf8.UTFMax
		}
		c, size := utf8.DecodeRuneInString(string(src[i : i+n]))
		if c == utf8.RuneError && size == 1 {
			dst = append(dst, src[start:i]...)
			dst = append(dst, `\ufffd`...)
			i += size
			start = i
			continue
		}
		// U+2028 is LINE SEPARATOR.
		// U+2029 is PARAGRAPH SEPARATOR.
		// They are both technically valid characters in JSON strings,
		// but don't work in JSONP, which has to be evaluated as JavaScript,
		// and can lead to security holes there. It is valid JSON to
		// escape them, so we do so unconditionally.
		// See https://en.wikipedia.org/wiki/JSON#Safety.
		if c == '\u2028' || c == '\u2029' {
			dst = append(dst, src[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, src[start:]...)
	dst = append(dst, '"')
	return dst
}

// safeSet holds the value true if the ASCII character with the given array
// position can be represented inside a JSON string without any further
// escaping.
//
// All values are true except for the ASCII control characters (0-31), the
// double quote ("), and the backslash character ("\").
var safeSet = [utf8.RuneSelf]bool{
	' ':      true,
	'!':      true,
	'"':      false,
	'#':      true,
	'$':      true,
	'%':      true,
	'&':      true,
	'\'':     true,
	'(':      true,
	')':      true,
	'*':      true,
	'+':      true,
	',':      true,
	'-':      true,
	'.':      true,
	'/':      true,
	'0':      true,
	'1':      true,
	'2':      true,
	'3':      true,
	'4':      true,
	'5':      true,
	'6':      true,
	'7':      true,
	'8':      true,
	'9':      true,
	':':      true,
	';':      true,
	'<':      true,
	'=':      true,
	'>':      true,
	'?':      true,
	'@':      true,
	'A':      true,
	'B':      true,
	'C':      true,
	'D':      true,
	'E':      true,
	'F':      true,
	'G':      true,
	'H':      true,
	'I':      true,
	'J':      true,
	'K':      true,
	'L':      true,
	'M':      true,
	'N':      true,
	'O':      true,
	'P':      true,
	'Q':      true,
	'R':      true,
	'S':      true,
	'T':      true,
	'U':      true,
	'V':      true,
	'W':      true,
	'X':      true,
	'Y':      true,
	'Z':      true,
	'[':      true,
	'\\':     false,
	']':      true,
	'^':      true,
	'_':      true,
	'`':      true,
	'a':      true,
	'b':      true,
	'c':      true,
	'd':      true,
	'e':      true,
	'f':      true,
	'g':      true,
	'h':      true,
	'i':      true,
	'j':      true,
	'k':      true,
	'l':      true,
	'm':      true,
	'n':      true,
	'o':      true,
	'p':      true,
	'q':      true,
	'r':      true,
	's':      true,
	't':      true,
	'u':      true,
	'v':      true,
	'w':      true,
	'x':      true,
	'y':      true,
	'z':      true,
	'{':      true,
	'|':      true,
	'}':      true,
	'~':      true,
	'\u007f': true,
}

// htmlSafeSet holds the value true if the ASCII character with the given
// array position can be safely represented inside a JSON string, embedded
// inside of HTML <script> tags, without any additional escaping.
//
// All values are true except for the ASCII control characters (0-31), the
// double quote ("), the backslash character ("\"), HTML opening and closing
// tags ("<" and ">"), and the ampersand ("&").
var htmlSafeSet = [utf8.RuneSelf]bool{
	' ':      true,
	'!':      true,
	'"':      false,
	'#':      true,
	'$':      true,
	'%':      true,
	'&':      false,
	'\'':     true,
	'(':      true,
	')':      true,
	'*':      true,
	'+':      true,
	',':      true,
	'-':      true,
	'.':      true,
	'/':      true,
	'0':      true,
	'1':      true,
	'2':      true,
	'3':      true,
	'4':      true,
	'5':      true,
	'6':      true,
	'7':      true,
	'8':      true,
	'9':      true,
	':':      true,
	';':      true,
	'<':      false,
	'=':      true,
	'>':      false,
	'?':      true,
	'@':      true,
	'A':      true,
	'B':      true,
	'C':      true,
	'D':      true,
	'E':      true,
	'F':      true,
	'G':      true,
	'H':      true,
	'I':      true,
	'J':      true,
	'K':      true,
	'L':      true,
	'M':      true,
	'N':      true,
	'O':      true,
	'P':      true,
	'Q':      true,
	'R':      true,
	'S':      true,
	'T':      true,
	'U':      true,
	'V':      true,
	'W':      true,
	'X':      true,
	'Y':      true,
	'Z':      true,
	'[':      true,
	'\\':     false,
	']':      true,
	'^':      true,
	'_':      true,
	'`':      true,
	'a':      true,
	'b':      true,
	'c':      true,
	'd':      true,
	'e':      true,
	'f':      true,
	'g':      true,
	'h':      true,
	'i':      true,
	'j':      true,
	'k':      true,
	'l':      true,
	'm':      true,
	'n':      true,
	'o':      true,
	'p':      true,
	'q':      true,
	'r':      true,
	's':      true,
	't':      true,
	'u':      true,
	'v':      true,
	'w':      true,
	'x':      true,
	'y':      true,
	'z':      true,
	'{':      true,
	'|':      true,
	'}':      true,
	'~':      true,
	'\u007f': true,
}
