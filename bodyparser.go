package gofi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
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
	// SchemaField schemaField // Needed for error reporting
	SchemaRules *RuleDef // Needed for recursion
	ShouldBind  bool
	Context     ParserContext
	SchemaPtr   any
	Body        *reflect.Value
}

type ResponseOptions struct {
	Context     ParserContext
	SchemaRules *RuleDef // Needed for validation
	Body        reflect.Value
}

type BodyParser interface {
	Match(contentType string) bool
	ValidateAndDecodeRequest(r io.ReadCloser, opts RequestOptions) error
	ValidateAndEncodeResponse(s any, opts ResponseOptions) ([]byte, error)
}

type ParserContext interface {
	Writer() http.ResponseWriter
	CustomSpecs() CustomSpecs
}

type parserContext struct {
	c *context
}

func (p *parserContext) Writer() http.ResponseWriter {
	return p.c.Writer()
}

func (p *parserContext) CustomSpecs() CustomSpecs {
	return p.c.serverOpts.customSpecs
}

type JSONBodyParser struct {
	MaxRequestSize int64
	MaxDepth       int
}

func (j *JSONBodyParser) Match(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func (j *JSONBodyParser) ValidateAndDecodeRequest(body io.ReadCloser, opts RequestOptions) error {
	bsMax := j.MaxRequestSize
	if bsMax == 0 {
		bsMax = 1048576 // defaultReqSize
	}

	body = http.MaxBytesReader(opts.Context.Writer(), body, bsMax)
	bs, err := io.ReadAll(body)
	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "reader", err)
	} else if len(bs) == 0 && opts.SchemaRules != nil && opts.SchemaRules.required {
		return newErrReport(RequestErr, schemaBody, "", "required", errors.New("request body is required"))
	} else if len(bs) == 0 {
		return nil
	}

	// Use empty rules if nil (shouldn't happen if properly initialized)
	if opts.SchemaRules == nil {
		// fallback or error? Assuming initialized.
		return errors.New("SchemaRules is nil")
	}

	// Determine whether json body value is a primitive or not
	val, err := utils.PrimitiveFromStr(opts.SchemaRules.kind, string(bs))
	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "encoder", err)
	}

	// Handle if JSON body value is a primitive
	if utils.IsPrimitive(val) {
		if err := runValidation(val, RequestErr, schemaBody, "", opts.SchemaRules.rules); err != nil {
			return err
		}

		if opts.ShouldBind && opts.Body != nil {
			sf := opts.Body.FieldByName(string(schemaBody))
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
	if opts.ShouldBind && opts.Body != nil {
		bodyStruct = j.getFieldStruct(opts.Body, schemaBody.String())
	}

	strctOpts := j.getFieldOptions(opts, &bodyStruct, opts.SchemaRules)
	status, err := j.walkStruct(pv, schemaBody, strctOpts, nil)
	if err != nil {
		return err
	}

	if status != nil && *status == walkFinished {
		return nil
	} else {
		return newErrReport(RequestErr, schemaBody, "", "parser", errors.New("couldn't parse request body"))
	}
}

func (j *JSONBodyParser) ValidateAndEncodeResponse(obj any, opts ResponseOptions) ([]byte, error) {
	body := opts.Body
	if body.Kind() == reflect.Pointer {
		body = body.Elem()
	}

	if opts.SchemaRules != nil && opts.SchemaRules.required && !body.IsValid() {
		return nil, newErrReport(ResponseErr, schemaBody, "", "required", errors.New("value is required for body"))
	}

	if opts.SchemaRules != nil && opts.SchemaRules.kind != body.Kind() {
		return nil, newErrReport(ResponseErr, schemaBody, "", "typeMismatch", errors.New("body schema and payload mismatch"))
	}

	var buff bytes.Buffer
	buff.Reset()
	if err := j.encodeFieldValue(opts.Context, &buff, opts.Body, opts.SchemaRules, nil); err != nil {
		return nil, newErrReport(ResponseErr, schemaBody, "", "encoder", err)
	}

	return buff.Bytes(), nil
}

func (j *JSONBodyParser) walkStruct(pv *cont.ParsedJson, schemaField schemaField, opts RequestOptions, keys []string) (*walkFinishStatus, error) {
	if j.MaxDepth == 0 {
		j.MaxDepth = 100
	}
	if len(keys) > j.MaxDepth {
		return nil, newErrReport(RequestErr, schemaField, strings.Join(keys, "."), "depth", errors.New("max recursion depth exceeded"))
	}

	kp := strings.Join(keys, ".")
	val, err := pv.GetByKind(opts.SchemaRules.kind, opts.SchemaRules.format, keys...)
	if err != nil {
		return nil, newErrReport(RequestErr, schemaField, kp, "parser", err)
	}

	if val == nil && opts.SchemaRules.defVal != nil {
		val = opts.SchemaRules.defVal
	} else if val == cont.EOF {
		val = nil
	}

	if !opts.SchemaRules.required && val == nil {
		return nil, nil
	}

	if opts.ShouldBind && opts.Body != nil && opts.Body.Kind() == reflect.Pointer {
		opts.Body.Set(reflect.New(opts.Body.Type().Elem()))
	}

	switch opts.SchemaRules.kind {
	case reflect.Struct:
		for childKey, childDef := range opts.SchemaRules.properties {
			var childStruct reflect.Value
			if opts.ShouldBind && opts.Body != nil {
				childStruct = j.getFieldStruct(opts.Body, childDef.fieldName)
			}

			childOpts := j.getFieldOptions(opts, &childStruct, childDef)
			_, err := j.walkStruct(pv, schemaBody, childOpts, append(keys, childKey))
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
			return nil, newErrReport(RequestErr, schemaField, kp, "parser", err)
		}

		if opts.ShouldBind && opts.Body != nil {
			opts.Body.Set(reflect.MakeMap(opts.Body.Type()))
		}

		var mapErr error
		obj.Visit(func(key []byte, v *fastjson.Value) {
			var cstrct reflect.Value
			if opts.ShouldBind && opts.Body != nil {
				cstrct = reflect.New(opts.Body.Type().Elem()).Elem()
			}

			ckey := string(key)
			copts := j.getFieldOptions(opts, &cstrct, opts.SchemaRules.additionalProperties)
			_, err := j.walkStruct(pv, schemaBody, copts, append(keys, ckey))
			if err != nil {
				mapErr = err
				return
			}

			if opts.ShouldBind && opts.Body != nil {
				opts.Body.SetMapIndex(reflect.ValueOf(ckey), cstrct)
			}

		})

		if mapErr != nil {
			return nil, mapErr
		}

		return &walkFinished, nil

	case reflect.Slice, reflect.Array:
		var size = 50 // DEFAULT_ARRAY_SIZE
		if opts.SchemaRules.max != nil {
			size = int(*opts.SchemaRules.max)
		}

		rules := opts.SchemaRules

		switch true {
		case utils.IsPrimitiveKind(opts.SchemaRules.item.kind):
			// Handle array of primitive values
			arr, err := pv.GetPrimitiveArrVals(rules.item.kind, rules.format, keys, size)
			if rules.max != nil && len(arr) > int(*rules.max) {
				return nil, newErrReport(RequestErr, schemaField, kp, "max", errors.New("array size too large"))
			} else if err != nil {
				return nil, newErrReport(RequestErr, schemaField, kp, "parser", err)
			}

			if err := runValidation(arr, RequestErr, schemaField, kp, opts.SchemaRules.rules); err != nil {
				return nil, err
			}

			if opts.ShouldBind && opts.Body != nil {
				err = j.decodeFieldValue(opts.Body, arr)
				if err != nil {
					newErrReport(RequestErr, schemaField, kp, "encoder", err)
				}
			}

			return &walkFinished, nil

		case utils.NotPrimitiveKind(opts.SchemaRules.item.kind):
			// Handle array of Non primitives
			i := 0
			var nslice reflect.Value
			if opts.ShouldBind && opts.Body != nil {
				nslice = reflect.MakeSlice(opts.Body.Type(), 0, size)
			}

			for {
				_keys := append(keys, fmt.Sprintf("%d", i))
				_kp := strings.Join(_keys, ".")
				if !pv.Exist(_keys...) {
					if rules.required && i == 0 {
						return nil, newErrReport(RequestErr, schemaField, _kp, "required", errors.New("value must not be empty"))
					} else {
						break
					}
				} else if rules.max != nil && i > int(*rules.max) {
					return nil, newErrReport(RequestErr, schemaField, _kp, "max", fmt.Errorf("array length must not be greater than %f", *rules.max))
				}

				var istrct reflect.Value
				if opts.ShouldBind && opts.Body != nil {
					istrct = reflect.New(opts.Body.Type().Elem()).Elem()
				}

				fopts := j.getFieldOptions(opts, &istrct, rules.item)
				_, err := j.walkStruct(pv, schemaBody, fopts, append(keys, fmt.Sprintf("%d", i)))
				if err != nil {
					return nil, err
				}

				if opts.ShouldBind && opts.Body != nil {
					nslice = reflect.Append(nslice, istrct)
				}

				i++
			}

			// Slice Validation
			var sliceVal any
			if opts.ShouldBind && opts.Body != nil {
				sliceVal = nslice.Interface()
			}
			// Note: validation of slice itself (e.g. min items) is tricky if logic above handles items individually.
			// But original code validates slice here:
			if err := runValidation(sliceVal, RequestErr, schemaField, kp, opts.SchemaRules.rules); err != nil {
				return nil, err
			}

			if opts.ShouldBind && opts.Body != nil {
				opts.Body.Set(nslice)
			}

			return &walkFinished, nil
		}

	case reflect.Interface:
		v, err := pv.GetAnyValue(keys)
		if err != nil {
			return nil, newErrReport(RequestErr, schemaField, kp, "parser", err)
		}

		if err := runValidation(v, RequestErr, schemaField, kp, opts.SchemaRules.rules); err != nil {
			return nil, err
		}

		if opts.ShouldBind && opts.Body != nil {
			err = j.decodeFieldValue(opts.Body, v)
			if err != nil {
				newErrReport(RequestErr, schemaField, kp, "encoder", err)
			}
		}

		return &walkFinished, nil

	default:
		if err := runValidation(val, RequestErr, schemaField, kp, opts.SchemaRules.rules); err != nil {
			return nil, err
		}

		if opts.ShouldBind && opts.Body != nil {
			err = j.decodeFieldValue(opts.Body, val)
			if err != nil {
				newErrReport(RequestErr, schemaField, kp, "encoder", err)
			}
		}

		return &walkFinished, nil
	}

	return &walkFinished, nil
}

func (j *JSONBodyParser) decodeFieldValue(field *reflect.Value, val any) error {
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
					if err := bindValOnElem(&ssf, item); err != nil {
						return err
					}
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
			v, err := utils.SafeConvert(reflect.ValueOf(val), ptype.Elem().Type())
			if err != nil {
				return err
			}
			ptype.Elem().Set(v)
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
					if err := bindValOnElem(&ssf, item); err != nil {
						return err
					}
					slice = reflect.Append(slice, ssf.Addr())
				}
				field.Set(slice)
				return nil
			default:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct).Elem()
					if err := bindValOnElem(&ssf, item); err != nil {
						return err
					}
					slice = reflect.Append(slice, ssf)
				}
				field.Set(slice)
				return nil
			}
		} else {
			return fmt.Errorf("type mismatch. expected array value got %T", val)
		}
	default:
		v, err := utils.SafeConvert(reflect.ValueOf(val), field.Type())
		if err != nil {
			return err
		}
		field.Set(v)
		return nil
	}
}

func (j *JSONBodyParser) getFieldStruct(strct *reflect.Value, fieldname string) reflect.Value {
	switch strct.Kind() {
	case reflect.Pointer:
		return strct.Elem().FieldByName(fieldname)
	default:
		return strct.FieldByName(fieldname)
	}
}

func (j *JSONBodyParser) getFieldOptions(opts RequestOptions, fieldStruct *reflect.Value, fieldRule *RuleDef) RequestOptions {
	rtn := opts
	rtn.Body = fieldStruct
	rtn.SchemaRules = fieldRule
	return rtn
}

func (j *JSONBodyParser) encodeFieldValue(c ParserContext, buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
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

	// We need to define encodeArr and encodeMap recursing with method call j.encodeFieldValue
	// BUT closures capturing method receiver? Yes.

	// Issue: encodeArr in encoders.go called `encodeFieldValue(c, ...)` which was a function.
	// Here `j.encodeFieldValue` is a method.
	// I'll define local helpers that call the method.

	var encodeArr func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error
	encodeArr = func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
		buf.WriteString("[")

		var arules *RuleDef
		if rules != nil {
			arules = rules.item
		}

		for i := 0; i < val.Len(); i++ {
			if i > 0 {
				buf.WriteString(",")
			}
			if err := j.encodeFieldValue(c, buf, val.Index(i), arules, append(kp, strconv.Itoa(i))); err != nil {
				return err
			}
		}

		buf.WriteString("]")
		return nil
	}

	var encodeMap func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error
	encodeMap = func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
		buf.WriteString("{")

		var mrules *RuleDef
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

			if err := j.encodeFieldValue(c, buf, key, mrules, append(kp, keyStr)); err != nil {
				return err
			}
			buf.WriteString(":")
			if err := j.encodeFieldValue(c, buf, mr.Value(), mrules, append(kp, keyStr)); err != nil {
				return err
			}
		}

		buf.WriteString("}")
		return nil
	}

	var encodeStruct func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error
	encodeStruct = func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
		buf.WriteString("{")
		first := true

		for field, frules := range rules.properties {
			fieldValue := val.FieldByName(frules.fieldName)
			if (slices.Contains(frules.tags["json"], "omitempty") && isEmptyValue(fieldValue)) || slices.Contains(frules.tags["json"], "-") {
				continue
			}

			if !first {
				buf.WriteString(",")
			}
			first = false

			buf.WriteString(fmt.Sprintf(`"%s":`, field))
			if err := j.encodeFieldValue(c, buf, fieldValue, frules, append(kp, field)); err != nil {
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
		if err := runValidation(vany, ResponseErr, schemaBody, strings.Join(kp, "."), rules.rules); err != nil {
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

	if rules != nil {
		if spec, ok := c.CustomSpecs().Find(string(rules.format)); ok {
			if vIsValid {
				v, err := spec.Encode(vany)
				if err != nil {
					return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", err)
				}
				encodeString(buf, v)
				return nil

			} else {
				return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", errors.New("could not cast given type to string"))
			}
		}
	}

	switch val.Kind() {
	case reflect.Invalid:
		_, err := buf.WriteString("null")
		if err != nil {
			return err
		}
	case reflect.Interface:
		return j.encodeFieldValue(c, buf, val.Elem(), rules, kp)
	case reflect.Pointer:
		if !val.IsValid() {
			_, err := buf.WriteString("null")
			if err != nil {
				return err
			}
			return nil
		}
		return j.encodeFieldValue(c, buf, val.Elem(), rules, kp)
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
