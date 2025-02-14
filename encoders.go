package gofi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/utils"
	"github.com/valyala/fastjson"
)

type RequestOptions struct {
	ShouldEncode      bool
	Context           *context
	SchemaField       schemaField
	SchemaPtrInstance any
	SchemaRules       *ruleDef
	FieldStruct       *reflect.Value
}

type ResponseOptions struct {
	Context     *context
	SchemaRules *ruleDef
	Body        reflect.Value
}

type SchemaEncoder interface {
	ValidateAndEncode(obj any, opts ResponseOptions) ([]byte, error)
	ValidateAndDecode(reader io.ReadCloser, opts RequestOptions) (err error)
}

const defaultReqSize int64 = 1048576

type JSONSchemaEncoder struct {
	MaxRequestSize int64
}

func (j *JSONSchemaEncoder) ValidateAndDecode(body io.ReadCloser, opts RequestOptions) error {
	bsMax := j.MaxRequestSize
	if bsMax == 0 {
		bsMax = defaultReqSize
	}

	body = http.MaxBytesReader(opts.Context.Writer(), body, 1048576)
	bs, err := io.ReadAll(body)
	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "reader", err)
	} else if len(bs) == 0 && opts.SchemaRules.required {
		return newErrReport(RequestErr, schemaBody, "", "required", errors.New("request body is required"))
	} else if len(bs) == 0 {
		return nil
	}

	// Determine whether json body value is a primirive or not
	val, err := utils.PrimitiveFromStr(opts.SchemaRules.kind, string(bs))
	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "encoder", err)
	}

	// Handle if JSON body value is a primitive
	if utils.IsPrimitive(val) {
		if err := runValidation(RequestErr, val, schemaBody, "", opts.SchemaRules.rules); err != nil {
			return err
		}

		if opts.ShouldEncode {
			sf := opts.FieldStruct.FieldByName(string(schemaBody))
			switch sf.Kind() {
			case reflect.Pointer:
				sfp := reflect.New(sf.Type().Elem())
				sfp.Elem().Set(reflect.ValueOf(val).Convert(sf.Type().Elem()))
				sf.Set(sfp)
			default:
				sf.Set(reflect.ValueOf(val).Convert(sf.Type()))
			}
		}

		return nil
	}

	// Handle non primitives with FastJSON
	pv, err := cont.PoolJsonParse(bs)
	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "parser", err)
	}

	var bodyStruct reflect.Value
	if opts.ShouldEncode {
		bodyStruct = getFieldStruct(opts.FieldStruct, schemaBody.String())
	}

	strctOpts := getFieldOptions(opts, &bodyStruct, opts.SchemaRules)
	status, err := walkStruct(pv, strctOpts, nil)
	if err != nil {
		return err
	}

	switch *status {
	case walkFinished:
		return nil
	default:
		return newErrReport(RequestErr, schemaBody, "", "parser", errors.New("couldn't parse request body"))
	}
}

func (j *JSONSchemaEncoder) ValidateAndEncode(obj any, opts ResponseOptions) ([]byte, error) {
	body := opts.Body
	if body.Kind() == reflect.Pointer {
		body = body.Elem()
	}

	if opts.SchemaRules.required && !body.IsValid() {
		return nil, newErrReport(ResponseErr, schemaBody, "", "required", errors.New("value is required for body"))
	}

	if opts.SchemaRules.kind != body.Kind() {
		return nil, newErrReport(ResponseErr, schemaBody, "", "typeMismatch", errors.New("body schema and payload mismatch"))
	}

	var buff bytes.Buffer
	buff.Reset()
	if err := encodeFieldValue(opts.Context, &buff, opts.Body, opts.SchemaRules, nil); err != nil {
		return nil, newErrReport(ResponseErr, schemaBody, "", "encoder", err)
	}

	return buff.Bytes(), nil
}

func walkStruct(pv *cont.ParsedJson, opts RequestOptions, keys []string) (*walkFinishStatus, error) {
	kp := strings.Join(keys, ".")
	val, err := pv.GetByKind(opts.SchemaRules.kind, opts.SchemaRules.format, keys...)
	if err != nil {
		return nil, newErrReport(RequestErr, opts.SchemaField, kp, "parser", err)

	}

	if val == nil && opts.SchemaRules.defVal != nil {
		val = opts.SchemaRules.defVal
	} else if val == cont.EOF {
		val = nil
	}

	if !opts.SchemaRules.required && val == nil {
		return nil, nil
	}

	if opts.ShouldEncode && opts.FieldStruct.Kind() == reflect.Pointer {
		opts.FieldStruct.Set(reflect.New(opts.FieldStruct.Type().Elem()))
	}

	switch opts.SchemaRules.kind {
	case reflect.Struct:
		for childKey, childDef := range opts.SchemaRules.properties {
			var childStruct reflect.Value
			if opts.ShouldEncode {
				childStruct = getFieldStruct(opts.FieldStruct, childDef.name)
			}

			childOpts := getFieldOptions(opts, &childStruct, childDef)
			_, err := walkStruct(pv, childOpts, append(keys, childKey))
			if err != nil {
				return nil, err
			}
		}

		return &walkFinished, nil

	case reflect.Map:
		if opts.SchemaRules.additionalProperties == nil {
			return &walkFinished, nil
		}

		obj, err := pv.GetRawObject(keys)
		if err != nil {
			return nil, newErrReport(RequestErr, opts.SchemaField, kp, "parser", err)
		}

		if opts.ShouldEncode {
			opts.FieldStruct.Set(reflect.MakeMap(opts.FieldStruct.Type()))
		}

		var mapErr error
		obj.Visit(func(key []byte, v *fastjson.Value) {
			var cstrct reflect.Value
			if opts.ShouldEncode {
				cstrct = reflect.New(opts.FieldStruct.Type().Elem()).Elem()
			}

			ckey := string(key)
			copts := getFieldOptions(opts, &cstrct, opts.SchemaRules.additionalProperties)
			_, err := walkStruct(pv, copts, append(keys, ckey))
			if err != nil {
				mapErr = err
				return
			}

			if opts.ShouldEncode {
				opts.FieldStruct.SetMapIndex(reflect.ValueOf(ckey), cstrct)
			}

		})

		if mapErr != nil {
			return nil, mapErr
		}

		return &walkFinished, nil

	case reflect.Slice, reflect.Array:
		var size = DEFAULT_ARRAY_SIZE
		if opts.SchemaRules.max != nil {
			size = int(*opts.SchemaRules.max)
		}

		rules := opts.SchemaRules

		switch true {
		case utils.IsPrimitiveKind(opts.SchemaRules.item.kind):
			// Handle array of primitive values
			arr, err := pv.GetPrimitiveArrVals(rules.item.kind, rules.format, keys, size)
			if rules.max != nil && len(arr) > int(*rules.max) {
				return nil, newErrReport(RequestErr, opts.SchemaField, kp, "max", errors.New("array size too large"))
			} else if err != nil {
				return nil, newErrReport(RequestErr, opts.SchemaField, kp, "parser", err)
			}

			if err := runValidation(RequestErr, arr, opts.SchemaField, kp, opts.SchemaRules.rules); err != nil {
				return nil, err
			}

			if opts.ShouldEncode {
				err = decodeFieldValue(opts.FieldStruct, arr)
				if err != nil {
					newErrReport(RequestErr, opts.SchemaField, kp, "encoder", err)
				}
			}

			return &walkFinished, nil

		case utils.NotPrimitiveKind(opts.SchemaRules.item.kind):
			// Handle array of Non primitives
			i := 0
			var nslice reflect.Value
			if opts.ShouldEncode {
				nslice = reflect.MakeSlice(opts.FieldStruct.Type(), 0, size)
			}

			for {
				_keys := append(keys, fmt.Sprintf("%d", i))
				_kp := strings.Join(_keys, ".")
				if !pv.Exist(_keys...) {
					if rules.required && i == 0 {
						return nil, newErrReport(RequestErr, opts.SchemaField, _kp, "required", errors.New("value must not be empty"))
					} else {
						break
					}
				} else if rules.max != nil && i > int(*rules.max) {
					return nil, newErrReport(RequestErr, opts.SchemaField, _kp, "max", fmt.Errorf("array length must not be greater than %f", *rules.max))
				}

				var istrct reflect.Value
				if opts.ShouldEncode {
					istrct = reflect.New(opts.FieldStruct.Type().Elem()).Elem()
				}

				fopts := getFieldOptions(opts, &istrct, rules.item)
				_, err := walkStruct(pv, fopts, append(keys, fmt.Sprintf("%d", i)))
				if err != nil {
					return nil, err
				}

				if opts.ShouldEncode {
					nslice = reflect.Append(nslice, istrct)
				}

				i++
			}

			if err := runValidation(RequestErr, nslice.Interface(), opts.SchemaField, kp, opts.SchemaRules.rules); err != nil {
				return nil, err
			}

			if opts.ShouldEncode {
				opts.FieldStruct.Set(nslice)
			}

			return &walkFinished, nil
		}

	default:
		if err := runValidation(RequestErr, val, opts.SchemaField, kp, opts.SchemaRules.rules); err != nil {
			return nil, err
		}

		if opts.ShouldEncode {
			err = decodeFieldValue(opts.FieldStruct, val)
			if err != nil {
				newErrReport(RequestErr, opts.SchemaField, kp, "encoder", err)
			}
		}

		return &walkFinished, nil
	}

	return &walkFinished, nil
}

func decodeFieldValue(field *reflect.Value, val any) error {
	// val can either be any primitive value or an array of primitive values
	// E.g 1, "string", []int{1, 2, 3, 4}, *[]int{1, 2, 3, 4}, []*int{1, 2, 3, 4}

	if val == nil {
		return nil
	}

	switch field.Kind() {
	case reflect.Pointer:
		switch field.Type().Elem().Kind() {
		case reflect.Slice, reflect.Array:
			if v, ok := val.([]any); ok {
				nslice := reflect.New(field.Type().Elem())
				istrct := field.Type().Elem().Elem()
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct).Elem()
					bindValOnElem(&ssf, item)
					slice = reflect.Append(slice, ssf)
				}
				nslice.Elem().Set(slice)
				field.Set(nslice)
				return nil
			} else {
				return fmt.Errorf("type mismatch. expected array value got %T", val)
			}
		default:
			ptype := reflect.New(field.Type().Elem())
			ptype.Elem().Set(reflect.ValueOf(val).Convert(ptype.Elem().Type()))
			field.Set(ptype)
			return nil
		}
	case reflect.Slice, reflect.Array:
		if v, ok := val.([]any); ok {
			istrct := field.Type().Elem()
			switch istrct.Kind() {
			case reflect.Pointer:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct.Elem()).Elem()
					bindValOnElem(&ssf, item)
					slice = reflect.Append(slice, ssf.Addr())
				}
				field.Set(slice)
				return nil
			default:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct).Elem()
					bindValOnElem(&ssf, item)
					slice = reflect.Append(slice, ssf)
				}
				field.Set(slice)
				return nil
			}
		} else {
			return fmt.Errorf("type mismatch. expected array value got %T", val)
		}
	default:
		field.Set(reflect.ValueOf(val).Convert(field.Type()))
		return nil
	}
}

func getFieldStruct(strct *reflect.Value, fieldname string) reflect.Value {
	switch strct.Kind() {
	case reflect.Pointer:
		return strct.Elem().FieldByName(fieldname)
	default:
		return strct.FieldByName(fieldname)
	}
}

func getFieldOptions(opts RequestOptions, fieldStruct *reflect.Value, fieldRule *ruleDef) RequestOptions {
	rtn := opts
	rtn.FieldStruct = fieldStruct
	rtn.SchemaRules = fieldRule
	return rtn
}

func encodeFieldValue(c *context, buf *bytes.Buffer, val reflect.Value, rules *ruleDef, kp []string) error {

	isEmptyValue := func(v reflect.Value) bool {
		switch v.Kind() {
		case reflect.String:
			return v.String() == ""
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return v.Int() == 0
		case reflect.Bool:
			return !v.Bool()
		case reflect.Slice, reflect.Array, reflect.Map:
			return v.Len() == 0
		case reflect.Ptr, reflect.Interface:
			return v.IsNil()
		default:
			return false
		}
	}

	encodeString := func(b *bytes.Buffer, s string) {
		b.WriteRune('"')
		for _, r := range s {
			switch r {
			case '"':
				b.WriteString(`\"`)
			case '\\':
				b.WriteString(`\\`)
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteRune('"')
	}

	encodeArr := func(buf *bytes.Buffer, val reflect.Value, rules *ruleDef, kp []string) error {
		buf.WriteString("[")

		var arules *ruleDef
		if rules != nil {
			arules = rules.item
		}

		for i := 0; i < val.Len(); i++ {
			if i > 0 {
				buf.WriteString(",")
			}
			if err := encodeFieldValue(c, buf, val.Index(i), arules, append(kp, strconv.Itoa(i))); err != nil {
				return err
			}
		}

		buf.WriteString("]")
		return nil
	}

	encodeMap := func(buf *bytes.Buffer, val reflect.Value, rules *ruleDef, kp []string) error {
		buf.WriteString("{")

		var mrules *ruleDef
		if rules != nil {
			mrules = rules.additionalProperties
		}

		mr := val.MapRange()
		for i := 0; mr.Next(); i++ {
			if i > 0 {
				buf.WriteString(",")
			}

			key := mr.Key()
			keyStr, ok := key.Interface().(string)
			if !ok {
				return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", errors.New("map key must be of type string"))
			}

			if err := encodeFieldValue(c, buf, key, mrules, append(kp, keyStr)); err != nil {
				return err
			}
			buf.WriteString(":")
			if err := encodeFieldValue(c, buf, mr.Value(), mrules, append(kp, keyStr)); err != nil {
				return err
			}
		}

		buf.WriteString("}")
		return nil
	}

	encodeStruct := func(buf *bytes.Buffer, val reflect.Value, rules *ruleDef, kp []string) error {
		buf.WriteString("{")
		first := true

		for field, frules := range rules.properties {
			fieldValue := val.FieldByName(frules.name)
			if (slices.Contains(frules.tags["json"], "omitempty") && isEmptyValue(fieldValue)) || slices.Contains(frules.tags["json"], "-") {
				continue
			}

			if !first {
				buf.WriteString(",")
			}
			first = false

			buf.WriteString(fmt.Sprintf(`"%s":`, field))
			if err := encodeFieldValue(c, buf, fieldValue, frules, append(kp, field)); err != nil {
				return err
			}
		}

		buf.WriteString("}")
		return nil
	}

	var vIsValid bool
	var vany any
	if val.IsValid() {
		if rules != nil && rules.defStr != "" && isEmptyValue(val) {
			if val.CanAddr() {
				val.Set(reflect.ValueOf(rules.defVal).Convert(val.Type()))
			} else {
				ptr := reflect.New(val.Type())
				ptr.Elem().Set(reflect.ValueOf(rules.defVal).Convert(val.Type()))
				val = ptr.Elem()
			}
		}

		vIsValid = true
		vany = val.Interface()
	}

	if rules != nil {
		if err := runValidation(ResponseErr, vany, schemaBody, strings.Join(kp, "."), rules.rules); err != nil {
			return err
		}
	}

	if ctype, ok := c.serverOpts.customSchema[string(rules.format)]; ok {
		if vIsValid {
			v, err := ctype.CustomEncode(vany)
			if err != nil {
				return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", err)
			}
			encodeString(buf, v)
			return nil
		} else {
			return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", errors.New("could not cast given time value to string"))
		}
	}

	switch val.Kind() {
	case reflect.Invalid:
		_, err := buf.WriteString("null")
		if err != nil {
			return err
		}
	case reflect.Pointer:
		if !val.IsValid() {
			_, err := buf.WriteString("null")
			if err != nil {
				return err
			}
			return nil
		}
		return encodeFieldValue(c, buf, val.Elem(), rules, kp)
	case reflect.String:
		encodeString(buf, val.String())
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err := buf.WriteString(fmt.Sprintf("%v", val.Int()))
		if err != nil {
			return err
		}
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, err := buf.WriteString(fmt.Sprintf("%v", val.Uint()))
		if err != nil {
			return err
		}
		return nil
	case reflect.Bool:
		_, err := buf.WriteString(fmt.Sprintf("%t", val.Bool()))
		if err != nil {
			return err
		}
		return nil
	case reflect.Slice, reflect.Array:
		return encodeArr(buf, val, rules, kp)
	case reflect.Map:
		return encodeMap(buf, val, rules, kp)
	case reflect.Struct:
		if rules.format == utils.TimeObjectFormat {
			if v, ok := (vany).(time.Time); ok {
				encodeString(buf, v.Format(rules.pattern))
				return nil
			} else {
				return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", errors.New("cannot cast time field to string"))
			}
		} else {
			return encodeStruct(buf, val, rules, kp)
		}
	}

	return nil
}
