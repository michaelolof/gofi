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

	"github.com/michaelolof/gofi/utils"
	"github.com/michaelolof/gofi/validators"
)

type headersSetter interface {
	CheckAndSetHeaders(c Context) error
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

	if ctype, ok := c.serverOpts.customSchema[string(def.format)]; ok {
		if val.IsValid() {
			v, err := ctype.CustomDecode(val.Interface())
			if err != nil {
				return newErrReport(ResponseErr, schemaBody, kp, "typeMismatch", err)
			}
			bs.appendByte('"')
			bs.appendStr(v)
			bs.appendByte('"')
			return nil
		} else {
			return newErrReport(ResponseErr, schemaBody, kp, "typeMismatch", errors.New("could not cast given time value to string"))
		}
	}

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
	bs []byte
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
