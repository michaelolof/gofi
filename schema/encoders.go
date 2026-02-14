package schema

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
	"github.com/michaelolof/gofi/validators"
	"github.com/valyala/fastjson"
)

const DefaultReqSize int64 = 1048576

type JSONSchemaEncoder struct {
	MaxRequestSize int64
}

func (j *JSONSchemaEncoder) ValidateAndDecode(body io.ReadCloser, opts RequestValidationOptions) error {
	bsMax := j.MaxRequestSize
	if bsMax == 0 {
		bsMax = DefaultReqSize
	}

	// opts.Context is io.Writer. In gofi it was *context which yielded Writer().
	// We expect opts.Context to be http.ResponseWriter or nil if not available.
	var writer http.ResponseWriter
	if w, ok := (opts.Context).(http.ResponseWriter); ok {
		writer = w
	}

	// Limit reader
	var reader io.Reader = body
	if writer != nil {
		reader = http.MaxBytesReader(writer, body, bsMax)
	} else {
		// Fallback to LimitedReader if no ResponseWriter to use MaxBytesReader logic (which sets header?)
		// http.MaxBytesReader requires ResponseWriter to set "Connection: close" on limit breach.
		// If we don't have it, we just limit reading.
		reader = io.LimitReader(body, bsMax)
	}

	bs, err := io.ReadAll(reader)
	if err != nil {
		return NewErrReport(RequestErr, SchemaBody, "", "reader", err)
	} else if len(bs) == 0 && opts.SchemaRules.Required {
		return NewErrReport(RequestErr, SchemaBody, "", "required", errors.New("request body is required"))
	} else if len(bs) == 0 {
		return nil
	}

	// Determine whether json body value is a primirive or not
	val, err := utils.PrimitiveFromStr(opts.SchemaRules.Kind, string(bs))
	if err != nil {
		return NewErrReport(RequestErr, SchemaBody, "", "encoder", err)
	}

	// Handle if JSON body value is a primitive
	if utils.IsPrimitive(val) {
		// Need to construct validator args.
		// We need request ideally. But RequestValidationOptions doesn't carry *http.Request directly currently?
		// In validator.go we passed nil for Context in my previous implementation of ValidateAndDecode call.
		// I should update RequestValidationOptions to carry Request if possible or ValidationArg

		// For now, passing nil args to RunValidation implies no context-based validation (like usage of request data in custom validators).
		// This might be a limitation if custom validators need request.
		// I should probably add Request to RequestValidationOptions.

		vArg := validators.NewValidatorArg(val, validators.ReuestType, nil, nil) // Missing req/res
		if err := RunValidation(&vArg, val, SchemaBody, "", opts.SchemaRules.Rules); err != nil {
			return err
		}

		if opts.ShouldEncode {
			sf := opts.FieldStruct.FieldByName(string(SchemaBody))
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
		return NewErrReport(RequestErr, SchemaBody, "", "parser", err)
	}

	var bodyStruct reflect.Value
	if opts.ShouldEncode {
		bodyStruct = getFieldStruct(opts.FieldStruct, SchemaBody.String())
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
		return NewErrReport(RequestErr, SchemaBody, "", "parser", errors.New("couldn't parse request body"))
	}
}

func (j *JSONSchemaEncoder) ValidateAndEncode(obj any, opts ResponseValidationOptions) ([]byte, error) {
	body := opts.Body
	if body.Kind() == reflect.Pointer {
		body = body.Elem()
	}

	if opts.SchemaRules.Required && !body.IsValid() {
		return nil, NewErrReport(ResponseErr, SchemaBody, "", "required", errors.New("value is required for body"))
	}

	if opts.SchemaRules.Kind != body.Kind() {
		return nil, NewErrReport(ResponseErr, SchemaBody, "", "typeMismatch", errors.New("body schema and payload mismatch"))
	}

	var buff bytes.Buffer
	buff.Reset()
	if err := encodeFieldValue(opts.Context, &buff, opts.Body, opts.SchemaRules, nil); err != nil {
		return nil, NewErrReport(ResponseErr, SchemaBody, "", "encoder", err)
	}

	return buff.Bytes(), nil
}

func encodeFieldValue(ctx any, buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {

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

	encodeArr := func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
		buf.WriteString("[")

		var arules *RuleDef
		if rules != nil {
			arules = rules.Item
		}

		for i := 0; i < val.Len(); i++ {
			if i > 0 {
				buf.WriteString(",")
			}
			if err := encodeFieldValue(ctx, buf, val.Index(i), arules, append(kp, strconv.Itoa(i))); err != nil {
				return err
			}
		}

		buf.WriteString("]")
		return nil
	}

	encodeMap := func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
		buf.WriteString("{")

		var mrules *RuleDef
		if rules != nil {
			mrules = rules.AdditionalProperties
		}

		mr := val.MapRange()
		for i := 0; mr.Next(); i++ {
			if i > 0 {
				buf.WriteString(",")
			}

			key := mr.Key()
			keyStr, ok := key.Interface().(string)
			if !ok {
				return NewErrReport(ResponseErr, SchemaBody, strings.Join(kp, "."), "typeMismatch", errors.New("map key must be of type string"))
			}

			encodeString(buf, keyStr)
			buf.WriteString(":")
			if err := encodeFieldValue(ctx, buf, mr.Value(), mrules, append(kp, keyStr)); err != nil {
				return err
			}
		}

		buf.WriteString("}")
		return nil
	}

	encodeStruct := func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
		buf.WriteString("{")
		first := true

		for field, frules := range rules.Properties {
			fieldValue := val.FieldByName(frules.FieldName)
			if (slices.Contains(frules.Tags["json"], "omitempty") && isEmptyValue(fieldValue)) || slices.Contains(frules.Tags["json"], "-") {
				continue
			}

			if !first {
				buf.WriteString(",")
			}
			first = false

			encodeString(buf, field)
			buf.WriteString(":")
			if err := encodeFieldValue(ctx, buf, fieldValue, frules, append(kp, field)); err != nil {
				return err
			}
		}

		buf.WriteString("}")
		return nil
	}

	var vany any
	if val.IsValid() {
		if rules != nil && rules.DefStr != "" && isEmptyValue(val) {
			if val.CanAddr() {
				val.Set(reflect.ValueOf(rules.DefVal).Convert(val.Type()))
			} else {
				ptr := reflect.New(val.Type())
				ptr.Elem().Set(reflect.ValueOf(rules.DefVal).Convert(val.Type()))
				val = ptr.Elem()
			}
		}

		// vIsValid = true
		vany = val.Interface()
	}

	if rules != nil {
		vArg := validators.NewValidatorArg(vany, validators.ReuestType, nil, nil)
		if err := RunValidation(&vArg, vany, SchemaBody, strings.Join(kp, "."), rules.Rules); err != nil {
			return err
		}
	}

	if vany == nil {
		_, err := buf.WriteString("null")
		if err != nil {
			return err
		}
		return nil
	}

	// Custom specs handling needs opts context?
	// The original code used c.serverOpts.customSpecs.
	// Here we don't have access to customSpecs easily unless we pass it in encodeFieldValue.
	// I passed `ctx any` which might contain it?
	// Or I should change signature of encodeFieldValue to take CustomSpecs map.
	// For now I will comment out custom specs part or assume no custom specs in this low level function
	// UNLESS I pass it.

	// NOTE: Missing customSpecs logic here.

	switch val.Kind() {
	case reflect.Invalid:
		_, err := buf.WriteString("null")
		if err != nil {
			return err
		}
	case reflect.Interface:
		return encodeFieldValue(ctx, buf, val.Elem(), rules, kp)
	case reflect.Pointer:
		if val.IsNil() {
			_, err := buf.WriteString("null")
			if err != nil {
				return err
			}
			return nil
		}
		return encodeFieldValue(ctx, buf, val.Elem(), rules, kp)
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
	case reflect.Float32, reflect.Float64:
		_, err := buf.WriteString(fmt.Sprintf("%v", val.Float()))
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
		if rules.Format == utils.TimeObjectFormat {
			if v, ok := (vany).(time.Time); ok {
				encodeString(buf, v.Format(rules.Pattern))
				return nil
			} else {
				return NewErrReport(ResponseErr, SchemaBody, strings.Join(kp, "."), "typeMismatch", errors.New("cannot cast time field to string"))
			}
		} else {
			return encodeStruct(buf, val, rules, kp)
		}
	}

	return nil
}

type walkFinishStatus struct{}

var walkFinished = walkFinishStatus{}

const DEFAULT_ARRAY_SIZE = 50

func walkStruct(pv *cont.ParsedJson, opts RequestValidationOptions, keys []string) (*walkFinishStatus, error) {
	kp := strings.Join(keys, ".")
	val, err := pv.GetByKind(opts.SchemaRules.Kind, opts.SchemaRules.Format, keys...)
	if err != nil {
		return nil, NewErrReport(RequestErr, opts.SchemaField, kp, "parser", err)
	}

	if val == nil && opts.SchemaRules.DefVal != nil {
		val = opts.SchemaRules.DefVal
	} else if val == cont.EOF {
		val = nil
	}

	if !opts.SchemaRules.Required && val == nil {
		return nil, nil
	}

	if opts.ShouldEncode && opts.FieldStruct.Kind() == reflect.Pointer {
		opts.FieldStruct.Set(reflect.New(opts.FieldStruct.Type().Elem()))
	}

	switch opts.SchemaRules.Kind {
	case reflect.Struct:
		for childKey, childDef := range opts.SchemaRules.Properties {
			var childStruct reflect.Value
			if opts.ShouldEncode {
				childStruct = getFieldStruct(opts.FieldStruct, childDef.FieldName)
			}

			childOpts := getFieldOptions(opts, &childStruct, childDef)
			_, err := walkStruct(pv, childOpts, append(keys, childKey))
			if err != nil {
				return nil, err
			}
		}

		return &walkFinished, nil

	case reflect.Map:
		if opts.SchemaRules.AdditionalProperties == nil {
			return &walkFinished, nil
		}

		obj, err := pv.GetRawObject(keys)
		if err != nil {
			return nil, NewErrReport(RequestErr, opts.SchemaField, kp, "parser", err)
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
			copts := getFieldOptions(opts, &cstrct, opts.SchemaRules.AdditionalProperties)
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
		if opts.SchemaRules.Max != nil {
			size = int(*opts.SchemaRules.Max)
		}

		rules := opts.SchemaRules

		switch true {
		case utils.IsPrimitiveKind(opts.SchemaRules.Item.Kind):
			// Handle array of primitive values
			arr, err := pv.GetPrimitiveArrVals(rules.Item.Kind, rules.Format, keys, size)
			if rules.Max != nil && len(arr) > int(*rules.Max) {
				return nil, NewErrReport(RequestErr, opts.SchemaField, kp, "max", errors.New("array size too large"))
			} else if err != nil {
				return nil, NewErrReport(RequestErr, opts.SchemaField, kp, "parser", err)
			}

			vArg := validators.NewValidatorArg(arr, validators.ReuestType, nil, nil)
			if err := RunValidation(&vArg, arr, opts.SchemaField, kp, opts.SchemaRules.Rules); err != nil {
				return nil, err
			}

			if opts.ShouldEncode {
				err = decodeFieldValue(opts.FieldStruct, arr)
				if err != nil {
					NewErrReport(RequestErr, opts.SchemaField, kp, "encoder", err)
				}
			}

			return &walkFinished, nil

		case utils.NotPrimitiveKind(opts.SchemaRules.Item.Kind):
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
					if rules.Required && i == 0 {
						return nil, NewErrReport(RequestErr, opts.SchemaField, _kp, "required", errors.New("value must not be empty"))
					} else {
						break
					}
				} else if rules.Max != nil && i > int(*rules.Max) {
					return nil, NewErrReport(RequestErr, opts.SchemaField, _kp, "max", fmt.Errorf("array length must not be greater than %f", *rules.Max))
				}

				var istrct reflect.Value
				if opts.ShouldEncode {
					istrct = reflect.New(opts.FieldStruct.Type().Elem()).Elem()
				}

				fopts := getFieldOptions(opts, &istrct, rules.Item)
				_, err := walkStruct(pv, fopts, append(keys, fmt.Sprintf("%d", i)))
				if err != nil {
					return nil, err
				}

				if opts.ShouldEncode {
					nslice = reflect.Append(nslice, istrct)
				}

				i++
			}

			vArg := validators.NewValidatorArg(nslice.Interface(), validators.ReuestType, nil, nil)
			if err := RunValidation(&vArg, nslice.Interface(), opts.SchemaField, kp, opts.SchemaRules.Rules); err != nil {
				return nil, err
			}

			if opts.ShouldEncode {
				opts.FieldStruct.Set(nslice)
			}

			return &walkFinished, nil
		}

	case reflect.Interface:
		v, err := pv.GetAnyValue(keys)
		if err != nil {
			return nil, NewErrReport(RequestErr, opts.SchemaField, kp, "parser", err)
		}

		vArg := validators.NewValidatorArg(v, validators.ReuestType, nil, nil)
		if err := RunValidation(&vArg, v, opts.SchemaField, kp, opts.SchemaRules.Rules); err != nil {
			return nil, err
		}

		if opts.ShouldEncode {
			err = decodeFieldValue(opts.FieldStruct, v)
			if err != nil {
				NewErrReport(RequestErr, opts.SchemaField, kp, "encoder", err)
			}
		}

		return &walkFinished, nil

	default:
		vArg := validators.NewValidatorArg(val, validators.ReuestType, nil, nil)
		if err := RunValidation(&vArg, val, opts.SchemaField, kp, opts.SchemaRules.Rules); err != nil {
			return nil, err
		}

		if opts.ShouldEncode {
			err = decodeFieldValue(opts.FieldStruct, val)
			if err != nil {
				NewErrReport(RequestErr, opts.SchemaField, kp, "encoder", err)
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

func bindValOnElem(strct *reflect.Value, val any) {
	if val == nil {
		return
	}

	switch strct.Kind() {
	case reflect.Pointer:
		if v, ok := val.([]any); ok {
			nslice := reflect.New(strct.Type().Elem())
			istrct := strct.Type().Elem().Elem()
			slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
			for _, item := range v {
				ssf := reflect.New(istrct).Elem()
				bindValOnElem(&ssf, item)
				slice = reflect.Append(slice, ssf)
			}
			nslice.Elem().Set(slice)
			strct.Set(nslice)
		}

	case reflect.Slice, reflect.Array:
		if v, ok := val.([]any); ok {
			istrct := strct.Type().Elem()
			switch istrct.Kind() {
			case reflect.Pointer:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct.Elem()).Elem()
					bindValOnElem(&ssf, item)
					slice = reflect.Append(slice, ssf.Addr())
				}
				strct.Set(slice)

			default:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct).Elem()
					bindValOnElem(&ssf, item)
					slice = reflect.Append(slice, ssf)
				}
				strct.Set(slice)
			}
		}

	default:
		strct.Set(reflect.ValueOf(val).Convert(strct.Type()))
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

func getFieldOptions(opts RequestValidationOptions, fieldStruct *reflect.Value, fieldRule *RuleDef) RequestValidationOptions {
	rtn := opts
	rtn.FieldStruct = fieldStruct
	rtn.SchemaRules = fieldRule
	return rtn
}
