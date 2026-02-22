package gofi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"reflect"
	"strings"
	"time"

	"github.com/michaelolof/gofi/utils"
)

type MultipartBodyParser struct {
	MaxRequestSize int64
}

func (m *MultipartBodyParser) Match(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "multipart/form-data"
}

func (m *MultipartBodyParser) ValidateAndDecodeRequest(r io.ReadCloser, opts RequestOptions) error {
	req := opts.Context.Request()
	if req == nil {
		return errors.New("http request not found in context")
	}

	bsMax := m.MaxRequestSize
	if bsMax == 0 {
		bsMax = 32 << 20 // 32MB default for multipart
	}

	// ParseMultipartForm parses up to maxMemory, storing rest in temp files.
	// It reads from req.Body.
	if err := req.ParseMultipartForm(bsMax); err != nil {
		return newErrReport(RequestErr, schemaBody, "", "parser", err)
	}

	if opts.SchemaRules == nil {
		return nil
	}

	if opts.SchemaRules.kind != reflect.Struct {
		return newErrReport(RequestErr, schemaBody, "", "typeMismatch", errors.New("multipart body must be a struct"))
	}

	if opts.ShouldBind && opts.Body != nil {
		bodyStruct := m.getFieldStruct(*opts.Body, string(schemaBody))
		if bodyStruct.Kind() == reflect.Pointer {
			bodyStruct = bodyStruct.Elem()
		}
		for key, rule := range opts.SchemaRules.properties {
			// Check form values first
			vals, ok := req.MultipartForm.Value[key]
			// If not in values, check files
			files, fileOk := req.MultipartForm.File[key]

			// Check for nested fields
			hasNested := false
			if rule.kind == reflect.Slice || rule.kind == reflect.Array || rule.kind == reflect.Struct {
				prefix := key + "."
				if req.MultipartForm != nil {
					for k := range req.MultipartForm.Value {
						if strings.HasPrefix(k, prefix) {
							hasNested = true
							break
						}
					}
					if !hasNested {
						for k := range req.MultipartForm.File {
							if strings.HasPrefix(k, prefix) {
								hasNested = true
								break
							}
						}
					}
				}
			}

			if !ok && !fileOk && !hasNested {
				if rule.required {
					return newErrReport(RequestErr, schemaBody, key, "required", errors.New("field is required"))
				}
				continue
			}

			fieldVal := bodyStruct.FieldByName(rule.fieldName)
			if !fieldVal.IsValid() {
				continue
			}

			// Handle Files
			if fileOk {
				if rule.kind == reflect.Slice || rule.kind == reflect.Array {
					elemType := fieldVal.Type().Elem()
					if elemType != utils.MultipartFile {
						return newErrReport(RequestErr, schemaBody, key, "typeMismatch", errors.New("field must be []*multipart.FileHeader"))
					}
					slice := reflect.MakeSlice(fieldVal.Type(), len(files), len(files))
					for i, file := range files {
						slice.Index(i).Set(reflect.ValueOf(file))
					}
					if err := m.bindValue(fieldVal, slice.Interface()); err != nil {
						return newErrReport(RequestErr, schemaBody, key, "typeMismatch", err)
					}
				} else {
					if len(files) > 0 {
						if fieldVal.Type() != utils.MultipartFile {
							return newErrReport(RequestErr, schemaBody, key, "typeMismatch", errors.New("field must be *multipart.FileHeader"))
						}
						if err := m.bindValue(fieldVal, files[0]); err != nil {
							return newErrReport(RequestErr, schemaBody, key, "typeMismatch", err)
						}
					}
				}
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
				if elemRule == nil {
					elemRule = &RuleDef{kind: fieldVal.Type().Elem().Kind()}
				}

				if elemRule.kind == reflect.Struct {
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
						if req.MultipartForm != nil {
							for k := range req.MultipartForm.Value {
								if strings.HasPrefix(k, prefix) {
									prefixFound = true
									break
								}
							}
							if !prefixFound {
								for k := range req.MultipartForm.File {
									if strings.HasPrefix(k, prefix) {
										prefixFound = true
										break
									}
								}
							}
						}
						if !prefixFound {
							break
						}

						istrct := reflect.New(sliceType.Elem()).Elem()
						subForm := make(map[string][]string)
						if req.MultipartForm != nil {
							for k, v := range req.MultipartForm.Value {
								if strings.HasPrefix(k, prefix) {
									subForm[strings.TrimPrefix(k, prefix)] = v
								}
							}
						}

						if err := m.bindStruct(subForm, istrct, elemRule); err != nil {
							return err
						}
						nslice = reflect.Append(nslice, istrct)
						i++
					}

					if i == 0 && rule.required {
						return newErrReport(RequestErr, schemaBody, key, "required", errors.New("value must not be empty"))
					}
					if err := m.bindValue(fieldVal, nslice.Interface()); err != nil {
						return err
					}
					continue
				}

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

					if err := m.bindValue(slice.Index(i), val); err != nil {
						return newErrReport(RequestErr, schemaBody, fmt.Sprintf("%s.%d", key, i), "typeMismatch", err)
					}
				}
				if err := m.bindValue(fieldVal, slice.Interface()); err != nil {
					return newErrReport(RequestErr, schemaBody, key, "typeMismatch", err)
				}

			} else if rule.kind == reflect.Struct {
				subForm := make(map[string][]string)
				prefix := key + "."
				if req.MultipartForm != nil {
					for k, v := range req.MultipartForm.Value {
						if strings.HasPrefix(k, prefix) {
							subForm[strings.TrimPrefix(k, prefix)] = v
						}
					}
				}
				if err := m.bindStruct(subForm, fieldVal, rule); err != nil {
					return err
				}
			} else {
				if len(vals) > 0 {
					val, err := parseVal(vals[0], rule)
					if err != nil {
						return newErrReport(RequestErr, schemaBody, key, "typeCast", err)
					}

					if err := runValidation(val, RequestErr, schemaBody, key, rule.rules); err != nil {
						return err
					}

					if err := m.bindValue(fieldVal, val); err != nil {
						return newErrReport(RequestErr, schemaBody, key, "typeMismatch", err)
					}
				}
			}
		}
	}

	return nil
}

func (m *MultipartBodyParser) bindValue(field reflect.Value, val any) error {
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
		return m.bindValue(field.Elem(), val)
	}

	return fmt.Errorf("cannot convert %v to %v", rv.Type(), field.Type())
}

func (m *MultipartBodyParser) bindStruct(form map[string][]string, dest reflect.Value, rules *RuleDef) error {
	if dest.Kind() == reflect.Pointer {
		dest = dest.Elem()
	}
	for key, rule := range rules.properties {
		vals, ok := form[key]
		if !ok {
			if rule.kind == reflect.Slice || rule.kind == reflect.Array || rule.kind == reflect.Struct {
				// Continue
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

					if err := m.bindStruct(subForm, istrct, elemRule); err != nil {
						return err
					}

					nslice = reflect.Append(nslice, istrct)
					i++
				}
				if err := m.bindValue(fieldVal, nslice.Interface()); err != nil {
					return err
				}
			} else {
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
					if err := m.bindValue(slice.Index(i), val); err != nil {
						return err
					}
				}
				if err := m.bindValue(fieldVal, slice.Interface()); err != nil {
					return err
				}
			}
		} else if rule.kind == reflect.Struct {
			prefix := key + "."
			subForm := make(map[string][]string)
			for k, v := range form {
				if strings.HasPrefix(k, prefix) {
					subForm[strings.TrimPrefix(k, prefix)] = v
				}
			}
			if err := m.bindStruct(subForm, fieldVal, rule); err != nil {
				return err
			}
		} else {
			if len(vals) > 0 {
				val, err := parseVal(vals[0], rule)
				if err != nil {
					return err
				}
				if err := m.bindValue(fieldVal, val); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m *MultipartBodyParser) ValidateAndEncodeResponse(s any, opts ResponseOptions) ([]byte, error) {
	return nil, errors.New("multipart body parser does not support response encoding")
}

func (m *MultipartBodyParser) getFieldStruct(strct reflect.Value, fieldname string) reflect.Value {
	if strct.Kind() == reflect.Pointer {
		strct = strct.Elem()
	}
	if strct.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	return strct.FieldByName(fieldname)
}
