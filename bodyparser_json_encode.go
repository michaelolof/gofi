package gofi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/michaelolof/gofi/utils"
)

// jsonEncoder writes one response body. Field decisions (keys, omitempty,
// skipped fields) are made when the schema is compiled and type facts are
// cached, so encoding reads values without per-value lookups. An interface
// value is only taken where validation, a custom spec or a marshaler needs
// one, and the field path is only built when an error is reported.
type jsonEncoder struct {
	buf     *bytes.Buffer
	specs   CustomSpecs
	path    requestPath
	scratch []byte // reordering space for map entries
}

func (e *jsonEncoder) fail(rule string, err error) error {
	return newErrReport(ResponseErr, schemaBody, e.path.String(), rule, err)
}

func (e *jsonEncoder) encode(val reflect.Value, rules *RuleDef) error {
	if rules != nil && rules.defStr != "" && val.IsValid() && isEmptyJSONValue(val) {
		// Encode the default without writing it into the caller's value,
		// which may be shared with other requests.
		val = reflect.ValueOf(rules.defVal).Convert(val.Type())
	}

	if rules != nil && len(rules.rules) > 0 {
		var v any
		if val.IsValid() && val.CanInterface() {
			v = val.Interface()
		}
		if err := runValidationLazy(v, ResponseErr, schemaBody, &e.path, rules.rules); err != nil {
			return err
		}
	}

	if !val.IsValid() || (val.Kind() == reflect.Interface && val.IsNil()) {
		e.buf.WriteString("null")
		return nil
	}

	if rules != nil {
		if spec, ok := e.specs.Find(string(rules.format)); ok {
			s, err := spec.Encode(val.Interface())
			if err != nil {
				return e.fail("typeMismatch", err)
			}
			e.writeString(s)
			return nil
		}
	}

	t := val.Type()
	if t == timeType {
		// A schema layout (the pattern tag) decides how times are written;
		// the fast path covers the default RFC 3339 layout.
		if rules != nil && rules.pattern != "" && rules.pattern != time.RFC3339Nano {
			e.writeString(timeValue(val).Format(rules.pattern))
			return nil
		}
		return e.encodeTime(val)
	}
	// Only types with methods can implement json.Marshaler, and only []byte
	// is base64-encoded, so most values skip the type cache entirely.
	if t.NumMethod() > 0 && jsonTypeInfoFor(t).marshaler && val.CanInterface() {
		switch val.Kind() {
		case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface:
			if val.IsNil() {
				e.buf.WriteString("null")
				return nil
			}
		}
		raw, err := val.Interface().(json.Marshaler).MarshalJSON()
		if err != nil {
			return e.fail("typeMismatch", err)
		}
		// Marshaler output is spliced into the document, so it must be one
		// complete JSON value; anything else could add or end fields.
		if !json.Valid(raw) {
			return e.fail("typeMismatch", fmt.Errorf("MarshalJSON for %s returned invalid JSON", val.Type()))
		}
		e.buf.Write(raw)
		return nil
	}
	if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 && jsonTypeInfoFor(t).byteSlice {
		if val.IsNil() {
			e.buf.WriteString("null")
			return nil
		}
		b := val.Bytes()
		n := base64.StdEncoding.EncodedLen(len(b))
		e.buf.Grow(n + 2)
		e.buf.WriteByte('"')
		dst := e.buf.AvailableBuffer()[:n]
		base64.StdEncoding.Encode(dst, b)
		e.buf.Write(dst)
		e.buf.WriteByte('"')
		return nil
	}

	switch val.Kind() {
	case reflect.Interface, reflect.Pointer:
		return e.encode(val.Elem(), rules)
	case reflect.String:
		e.writeString(val.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.buf.Write(strconv.AppendInt(e.buf.AvailableBuffer(), val.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		e.buf.Write(strconv.AppendUint(e.buf.AvailableBuffer(), val.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		bits := 64
		if val.Kind() == reflect.Float32 {
			bits = 32
		}
		b, ok := appendJSONFloat(e.buf.AvailableBuffer(), val.Float(), bits)
		if !ok {
			return e.fail("typeMismatch", fmt.Errorf("%v cannot be encoded as JSON", val.Float()))
		}
		e.buf.Write(b)
	case reflect.Bool:
		e.buf.Write(strconv.AppendBool(e.buf.AvailableBuffer(), val.Bool()))
	case reflect.Slice, reflect.Array:
		return e.encodeArray(val, rules)
	case reflect.Map:
		return e.encodeMap(val, rules)
	case reflect.Struct:
		if rules != nil && rules.format == utils.TimeObjectFormat {
			v, ok := val.Interface().(time.Time)
			if !ok {
				return e.fail("typeMismatch", errors.New("cannot cast time field to string"))
			}
			e.writeString(v.Format(rules.pattern))
			return nil
		}
		return e.encodeStruct(val, rules)
	default:
		return e.fail("typeMismatch", fmt.Errorf("%s cannot be encoded as JSON", val.Type()))
	}
	return nil
}

func (e *jsonEncoder) encodeArray(val reflect.Value, rules *RuleDef) error {
	var item *RuleDef
	if rules != nil {
		item = rules.item
	}
	e.buf.WriteByte('[')
	for i, n := 0, val.Len(); i < n; i++ {
		if i > 0 {
			e.buf.WriteByte(',')
		}
		e.path.pushIndex(i)
		if err := e.encode(val.Index(i), item); err != nil {
			return err
		}
		e.path.pop()
	}
	e.buf.WriteByte(']')
	return nil
}

// encodeTime writes t as time.Time.MarshalJSON does, without boxing the
// value or allocating the result. Times RFC 3339 cannot express (years
// outside 0–9999, zone offsets of 24 hours or more) go through MarshalJSON
// for its error.
func (e *jsonEncoder) encodeTime(val reflect.Value) error {
	t := timeValue(val)
	_, offset := t.Zone()
	if y := t.Year(); y < 0 || y > 9999 || offset <= -24*3600 || offset >= 24*3600 {
		_, err := t.MarshalJSON()
		return e.fail("typeMismatch", err)
	}
	b := append(e.buf.AvailableBuffer(), '"')
	b = t.AppendFormat(b, time.RFC3339Nano)
	e.buf.Write(append(b, '"'))
	return nil
}

// timeValue reads a time.Time from val without boxing it when possible.
func timeValue(val reflect.Value) time.Time {
	if val.CanAddr() {
		return *(*time.Time)(val.Addr().UnsafePointer())
	}
	return val.Interface().(time.Time)
}

// mapEntry locates one encoded "key":value pair in the buffer.
type mapEntry struct {
	key      string
	from, to int
}

// encodeMap writes keys in sorted order so equal values encode to equal
// bytes, which ETags and caches rely on. Entries are encoded in iteration
// order through reused key and value holders, then reordered by key.
func (e *jsonEncoder) encodeMap(val reflect.Value, rules *RuleDef) error {
	n := val.Len()
	if n == 0 {
		e.buf.WriteString("{}")
		return nil
	}
	t := val.Type()
	if t.Key().Kind() != reflect.String {
		return e.fail("typeMismatch", errors.New("map key must be of type string"))
	}
	var item *RuleDef
	if rules != nil {
		item = rules.additionalProperties
	}

	var small [8]mapEntry
	entries := small[:0]
	if n > len(small) {
		entries = make([]mapEntry, 0, n)
	}
	k := reflect.New(t.Key()).Elem()
	v := reflect.New(t.Elem()).Elem()
	var it reflect.MapIter
	it.Reset(val)
	base := e.buf.Len()
	for it.Next() {
		k.SetIterKey(&it)
		v.SetIterValue(&it)
		key := k.String()
		from := e.buf.Len()
		e.writeString(key)
		e.buf.WriteByte(':')
		e.path.pushField(key)
		if err := e.encode(v, item); err != nil {
			return err
		}
		e.path.pop()
		entries = append(entries, mapEntry{key: key, from: from - base, to: e.buf.Len() - base})
	}

	// Insertion sort: maps in responses are usually small.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].key < entries[j-1].key; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	e.scratch = append(e.scratch[:0], e.buf.Bytes()[base:]...)
	e.buf.Truncate(base)
	e.buf.WriteByte('{')
	for i, en := range entries {
		if i > 0 {
			e.buf.WriteByte(',')
		}
		e.buf.Write(e.scratch[en.from:en.to])
	}
	e.buf.WriteByte('}')
	return nil
}

func (e *jsonEncoder) encodeStruct(val reflect.Value, rules *RuleDef) error {
	e.buf.WriteByte('{')
	first := true
	if rules != nil && len(rules.orderedProps) > 0 {
		for _, f := range rules.orderedProps {
			if f.skipJSON {
				continue
			}
			var fv reflect.Value
			if len(f.accessor.index) > 0 {
				fv = val.FieldByIndex(f.accessor.index)
			} else {
				fv = val.FieldByName(f.fieldName)
			}
			if f.omitEmpty && isEmptyJSONValue(fv) {
				continue
			}
			if !first {
				e.buf.WriteByte(',')
			}
			first = false
			if len(f.jsonKeyBytes) > 0 {
				e.buf.Write(f.jsonKeyBytes)
			} else {
				e.buf.Write(appendJSONString(e.buf.AvailableBuffer(), f.field))
				e.buf.WriteByte(':')
			}
			e.path.pushField(f.field)
			if err := e.encode(fv, f); err != nil {
				return err
			}
			e.path.pop()
		}
	} else {
		// A struct without schema rules, such as one held in an any field.
		for _, f := range jsonTypeInfoFor(val.Type()).fields {
			fv, err := val.FieldByIndexErr(f.index)
			if err != nil {
				continue // promoted through a nil embedded pointer
			}
			if f.omitEmpty && isEmptyJSONValue(fv) {
				continue
			}
			if !first {
				e.buf.WriteByte(',')
			}
			first = false
			e.buf.Write(f.key)
			e.path.pushField(f.name)
			if err := e.encode(fv, nil); err != nil {
				return err
			}
			e.path.pop()
		}
	}
	e.buf.WriteByte('}')
	return nil
}

func (e *jsonEncoder) writeString(s string) {
	e.buf.Write(appendJSONString(e.buf.AvailableBuffer(), s))
}

// isEmptyJSONValue reports the values that omitempty omits, as encoding/json
// defines them.
func isEmptyJSONValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}

// jsonTypeInfo caches what the encoder needs to know about a type.
type jsonTypeInfo struct {
	marshaler bool
	byteSlice bool                  // base64-encoded, as encoding/json does
	fields    []jsonSchemalessField // for structs encoded without schema rules
}

type jsonSchemalessField struct {
	index     []int
	name      string
	key       []byte // quoted name and colon
	omitEmpty bool
}

var (
	jsonTypeInfos     sync.Map // reflect.Type -> *jsonTypeInfo
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	timeType          = reflect.TypeFor[time.Time]()
)

func jsonTypeInfoFor(t reflect.Type) *jsonTypeInfo {
	if v, ok := jsonTypeInfos.Load(t); ok {
		return v.(*jsonTypeInfo)
	}
	info := &jsonTypeInfo{
		marshaler: t.Implements(jsonMarshalerType),
		byteSlice: t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 && utils.IsByteSlice(t),
	}
	if t.Kind() == reflect.Struct {
		for _, f := range reflect.VisibleFields(t) {
			if isPromotedEmbed(f) || !f.IsExported() {
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			if name == "" {
				name = f.Name
			}
			info.fields = append(info.fields, jsonSchemalessField{
				index:     f.Index,
				name:      name,
				key:       append(appendJSONString(nil, name), ':'),
				omitEmpty: slices.Contains(strings.Split(opts, ","), "omitempty"),
			})
		}
	}
	v, _ := jsonTypeInfos.LoadOrStore(t, info)
	return v.(*jsonTypeInfo)
}

const jsonHex = "0123456789abcdef"

// appendJSONString appends s as a JSON string, escaping as encoding/json does
// apart from HTML characters: control characters, invalid UTF-8 (as U+FFFD)
// and U+2028/U+2029 are escaped. Runs of safe bytes are copied at once.
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch b {
			case '"', '\\':
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
				dst = append(dst, '\\', 'u', '0', '0', jsonHex[b>>4], jsonHex[b&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, `�`...)
			i += size
			start = i
			continue
		}
		if r == ' ' || r == ' ' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', jsonHex[r&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// appendJSONFloat formats f as encoding/json does. NaN and infinities have no
// JSON form and report false.
func appendJSONFloat(dst []byte, f float64, bits int) ([]byte, bool) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return dst, false
	}
	format := byte('f')
	if abs := math.Abs(f); abs != 0 {
		if bits == 64 && (abs < 1e-6 || abs >= 1e21) || bits == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21) {
			format = 'e'
		}
	}
	dst = strconv.AppendFloat(dst, f, format, -1, bits)
	if format == 'e' {
		// Shorten e-09 to e-9.
		if n := len(dst); n >= 4 && dst[n-4] == 'e' && dst[n-3] == '-' && dst[n-2] == '0' {
			dst[n-2] = dst[n-1]
			dst = dst[:n-1]
		}
	}
	return dst, true
}
