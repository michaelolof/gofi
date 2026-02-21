package gofi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/michaelolof/gofi/utils"
)

type FormBodyParser struct {
	MaxRequestSize int64
}

func (f *FormBodyParser) Match(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/x-www-form-urlencoded"
}

func (f *FormBodyParser) ValidateAndDecodeRequest(r io.ReadCloser, opts RequestOptions) error {
	req := opts.Context.Request()
	if req == nil {
		return errors.New("http request not found in context")
	}

	bsMax := f.MaxRequestSize
	if bsMax == 0 {
		bsMax = 10485760 // 10MB
	}

	// Enforce max bytes reader on body before parsing
	req.Body = http.MaxBytesReader(opts.Context.Writer(), req.Body, bsMax)

	if err := req.ParseForm(); err != nil {
		return newErrReport(RequestErr, schemaBody, "", "parser", err)
	}

	if opts.SchemaRules == nil {
		return nil
	}

	if opts.SchemaRules.kind != reflect.Struct {
		return newErrReport(RequestErr, schemaBody, "", "typeMismatch", errors.New("form body must be a struct"))
	}

	if opts.ShouldBind && opts.Body != nil {
		bodyStruct := f.getFieldStruct(*opts.Body, string(schemaBody))
		if bodyStruct.Kind() == reflect.Pointer {
			bodyStruct = bodyStruct.Elem()
		}
		for key, rule := range opts.SchemaRules.properties {
			vals, ok := req.PostForm[key]

			// Check if we have nested fields or the exact key
			hasNested := false
			if rule.kind == reflect.Slice || rule.kind == reflect.Array || rule.kind == reflect.Struct {
				prefix := key + "."
				for k := range req.PostForm {
					if strings.HasPrefix(k, prefix) {
						hasNested = true
						break
					}
				}
			}

			if !ok && !hasNested {
				if rule.required {
					return newErrReport(RequestErr, schemaBody, key, "required", errors.New("field is required"))
				}
				continue
			}

			fieldVal := bodyStruct.FieldByName(rule.fieldName)
			if !fieldVal.IsValid() {
				continue
			}

			// Helper to parse string to value
			parseVal := func(v string, r *RuleDef) (any, error) {
				if r.format == utils.TimeObjectFormat {
					return time.Parse(r.pattern, v)
				}
				return utils.PrimitiveFromStr(r.kind, v)
			}

			if rule.kind == reflect.Slice || rule.kind == reflect.Array {
				elemRule := rule.item
				if elemRule == nil {
					elemRule = &RuleDef{kind: fieldVal.Type().Elem().Kind()}
				}

				if elemRule.kind == reflect.Struct {
					// Struct array (Deep binding)
					i := 0
					var nslice reflect.Value
					sliceType := fieldVal.Type()
					if sliceType.Kind() == reflect.Pointer {
						sliceType = sliceType.Elem()
					}
					nslice = reflect.MakeSlice(sliceType, 0, 10)

					for {
						prefix := fmt.Sprintf("%s.%d.", key, i)
						prefixFound := false
						for k := range req.PostForm {
							if strings.HasPrefix(k, prefix) {
								prefixFound = true
								break
							}
						}
						if !prefixFound {
							break
						}

						istrct := reflect.New(sliceType.Elem()).Elem()
						subForm := make(map[string][]string)
						for k, v := range req.PostForm {
							if strings.HasPrefix(k, prefix) {
								subForm[strings.TrimPrefix(k, prefix)] = v
							}
						}

						if err := f.bindStruct(subForm, istrct, elemRule); err != nil {
							return err
						}
						nslice = reflect.Append(nslice, istrct)
						i++
					}

					if i == 0 && rule.required {
						return newErrReport(RequestErr, schemaBody, key, "required", errors.New("value must not be empty"))
					}

					if err := f.bindValue(fieldVal, nslice.Interface()); err != nil {
						return newErrReport(RequestErr, schemaBody, key, "typeMismatch", err)
					}
					continue
				}

				// Primitive slice
				sliceType := fieldVal.Type()
				if sliceType.Kind() == reflect.Pointer {
					sliceType = sliceType.Elem()
				}
				slice := reflect.MakeSlice(sliceType, len(vals), len(vals))
				for i, v := range vals {
					val, err := parseVal(v, elemRule)
					if err != nil {
						return newErrReport(RequestErr, schemaBody, fmt.Sprintf("%s.%d", key, i), "typeCast", err)
					}

					if err := runValidation(val, RequestErr, schemaBody, fmt.Sprintf("%s.%d", key, i), elemRule.rules); err != nil {
						return err
					}

					if err := f.bindValue(slice.Index(i), val); err != nil {
						return newErrReport(RequestErr, schemaBody, fmt.Sprintf("%s.%d", key, i), "typeMismatch", err)
					}
				}
				if err := f.bindValue(fieldVal, slice.Interface()); err != nil {
					return newErrReport(RequestErr, schemaBody, key, "typeMismatch", err)
				}

			} else if rule.kind == reflect.Struct {
				// Nested struct
				subForm := make(map[string][]string)
				prefix := key + "."
				for k, v := range req.PostForm {
					if strings.HasPrefix(k, prefix) {
						subForm[strings.TrimPrefix(k, prefix)] = v
					}
				}
				if err := f.bindStruct(subForm, fieldVal, rule); err != nil {
					return err
				}
			} else {
				// Primitive field
				if len(vals) > 0 {
					val, err := parseVal(vals[0], rule)
					if err != nil {
						return newErrReport(RequestErr, schemaBody, key, "typeCast", err)
					}

					if err := runValidation(val, RequestErr, schemaBody, key, rule.rules); err != nil {
						return err
					}

					if err := f.bindValue(fieldVal, val); err != nil {
						return newErrReport(RequestErr, schemaBody, key, "typeMismatch", err)
					}
				}
			}
		}
	}

	return nil
}

func (f *FormBodyParser) bindValue(field reflect.Value, val any) error {
	if val == nil {
		return nil
	}

	rv := reflect.ValueOf(val)

	// Try direct assignment first (important for pointers like *multipart.FileHeader)
	if rv.Type().ConvertibleTo(field.Type()) {
		field.Set(rv.Convert(field.Type()))
		return nil
	}

	if field.Kind() == reflect.Pointer {
		// If the field is a pointer, allocate memory if it's nil
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return f.bindValue(field.Elem(), val)
	}

	return fmt.Errorf("cannot convert %v to %v", rv.Type(), field.Type())
}

func (f *FormBodyParser) bindStruct(form map[string][]string, dest reflect.Value, rules *RuleDef) error {
	if dest.Kind() == reflect.Pointer {
		dest = dest.Elem()
	}
	for key, rule := range rules.properties {
		vals, ok := form[key]
		if !ok {
			// Check for nested fields even if the top-level key isn't there
			if rule.kind == reflect.Slice || rule.kind == reflect.Array || rule.kind == reflect.Struct {
				// Continue to handle nested binding
			} else {
				if rule.required {
					return newErrReport(RequestErr, schemaBody, key, "required", errors.New("field is required"))
				}
				continue
			}
		}

		fieldVal := dest.FieldByName(rule.fieldName)
		if !fieldVal.IsValid() {
			continue
		}

		parseVal := func(v string, r *RuleDef) (any, error) {
			if r.format == utils.TimeObjectFormat {
				return time.Parse(r.pattern, v)
			}
			return utils.PrimitiveFromStr(r.kind, v)
		}

		if rule.kind == reflect.Slice || rule.kind == reflect.Array {
			elemRule := rule.item
			if elemRule.kind == reflect.Struct {
				// Nested struct array
				i := 0
				var nslice reflect.Value
				sliceType := fieldVal.Type()
				if sliceType.Kind() == reflect.Pointer {
					sliceType = sliceType.Elem()
				}
				nslice = reflect.MakeSlice(sliceType, 0, 10)

				for {
					prefix := fmt.Sprintf("%s.%d.", key, i)
					found := false
					for k := range form {
						if strings.HasPrefix(k, prefix) {
							found = true
							break
						}
					}
					if !found {
						break
					}

					istrct := reflect.New(sliceType.Elem()).Elem()
					subForm := make(map[string][]string)
					for k, v := range form {
						if strings.HasPrefix(k, prefix) {
							subForm[strings.TrimPrefix(k, prefix)] = v
						}
					}

					if err := f.bindStruct(subForm, istrct, elemRule); err != nil {
						return err
					}

					nslice = reflect.Append(nslice, istrct)
					i++
				}
				if err := f.bindValue(fieldVal, nslice.Interface()); err != nil {
					return err
				}
			} else {
				// Primitive slice
				sliceType := fieldVal.Type()
				if sliceType.Kind() == reflect.Pointer {
					sliceType = sliceType.Elem()
				}
				slice := reflect.MakeSlice(sliceType, len(vals), len(vals))
				for i, v := range vals {
					val, err := parseVal(v, elemRule)
					if err != nil {
						return err
					}
					if err := f.bindValue(slice.Index(i), val); err != nil {
						return err
					}
				}
				if err := f.bindValue(fieldVal, slice.Interface()); err != nil {
					return err
				}
			}
		} else if rule.kind == reflect.Struct {
			// Nested struct
			prefix := key + "."
			subForm := make(map[string][]string)
			for k, v := range form {
				if strings.HasPrefix(k, prefix) {
					subForm[strings.TrimPrefix(k, prefix)] = v
				}
			}
			if err := f.bindStruct(subForm, fieldVal, rule); err != nil {
				return err
			}
		} else {
			if len(vals) > 0 {
				val, err := parseVal(vals[0], rule)
				if err != nil {
					return err
				}
				if err := f.bindValue(fieldVal, val); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (f *FormBodyParser) ValidateAndEncodeResponse(s any, opts ResponseOptions) ([]byte, error) {
	return nil, errors.New("form body parser does not support response encoding")
}

func (f *FormBodyParser) getFieldStruct(strct reflect.Value, fieldname string) reflect.Value {
	if strct.Kind() == reflect.Pointer {
		strct = strct.Elem()
	}
	if strct.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	return strct.FieldByName(fieldname)
}
