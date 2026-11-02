package gofi

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
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
	bsMax := f.MaxRequestSize
	if bsMax == 0 {
		bsMax = 10485760 // 10MB
	}

	// Read body and parse form values manually, rejecting bodies over the limit
	bodyBytes, err := ReadBodyLimited(r, bsMax)
	var tooLarge *HTTPError
	if errors.As(err, &tooLarge) {
		return err
	} else if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "parser", err)
	}

	formValues, err := url.ParseQuery(string(bodyBytes))
	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "parser", err)
	}
	count := 0
	for _, vals := range formValues {
		count += len(vals)
	}
	if count > requestLimits(opts.Context).MaxFormValues {
		return newLimitError("form value count")
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
		// Capture custom specs for use in parseVal closures
		customSpecs := opts.Context.CustomSpecs()

		for key, rule := range opts.SchemaRules.properties {
			vals, ok := formValues[key]

			// Check if we have nested fields or the exact key
			hasNested := false
			if rule.kind == reflect.Slice || rule.kind == reflect.Array || rule.kind == reflect.Struct {
				prefix := key + "."
				for k := range formValues {
					if strings.HasPrefix(k, prefix) {
						hasNested = true
						break
					}
				}
			}

			if !ok && !hasNested {
				if rule.required || rule.present {
					return newErrReport(RequestErr, schemaBody, key, "required", errors.New("field is required"))
				}
				continue
			}

			fieldVal, err := formBindField(bodyStruct, rule, key)
			if err != nil {
				return err
			}
			if !fieldVal.IsValid() {
				continue
			}

			// Helper to parse string to value, respecting custom specs
			parseVal := func(v string, r *RuleDef) (any, error) {
				if r.format == utils.TimeObjectFormat {
					return time.Parse(r.pattern, v)
				}
				// Check custom spec first
				if spec, ok := customSpecs.Find(string(r.format)); ok {
					return decodeSpecValue(spec, v)
				}
				return utils.PrimitiveFromStr(r.kind, v)
			}

			// A spec field is one value, whatever its kind; it is bound as a scalar below.
			_, isSpecField := customSpecs.Find(string(rule.format))
			if (rule.kind == reflect.Slice || rule.kind == reflect.Array) && !isSpecField {
				elemRule := rule.item
				if elemRule == nil {
					elemRule = &RuleDef{kind: fieldVal.Type().Elem().Kind()}
				}

				if elemRule.kind == reflect.Struct && elemRule.format != utils.TimeObjectFormat {
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
						for k := range formValues {
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
						for k, v := range formValues {
							if strings.HasPrefix(k, prefix) {
								subForm[strings.TrimPrefix(k, prefix)] = v
							}
						}

						if err := f.bindStruct(subForm, istrct, elemRule, customSpecs, fmt.Sprintf("%s.%d", key, i)); err != nil {
							return err
						}
						nslice = reflect.Append(nslice, istrct)
						i++
					}

					if i == 0 && rule.required && !rule.present {
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
				if handled, err := bindFormBytes(fieldVal, sliceType, vals, rule, key); handled {
					if err != nil {
						return err
					}
					continue
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

			} else if rule.kind == reflect.Struct || isSpecField {
				// If this struct-like field has a custom spec format, treat it as a scalar
				if _, isCustom := customSpecs.Find(string(rule.format)); isCustom || rule.format == utils.TimeObjectFormat {
					if len(vals) > 0 {
						val, err := parseVal(vals[0], rule)
						if err != nil {
							return newErrReport(RequestErr, schemaBody, key, "typeCast", err)
						}

						if err := runSpecValidation(val, RequestErr, schemaBody, key, rule.rules); err != nil {
							return err
						}

						if err := f.bindValue(fieldVal, val); err != nil {
							return newErrReport(RequestErr, schemaBody, key, "typeMismatch", err)
						}
					} else if rule.required || rule.present {
						return newErrReport(RequestErr, schemaBody, key, "required", errors.New("field is required"))
					}
					continue
				}

				// Nested struct
				subForm := make(map[string][]string)
				prefix := key + "."
				for k, v := range formValues {
					if strings.HasPrefix(k, prefix) {
						subForm[strings.TrimPrefix(k, prefix)] = v
					}
				}
				if err := f.bindStruct(subForm, fieldVal, rule, customSpecs, key); err != nil {
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

	return setParamField(field, val)
}

// bindStruct binds the nested fields under path. Errors and validation mirror
// the top level, reported against the full dotted field name.
func (f *FormBodyParser) bindStruct(form map[string][]string, dest reflect.Value, rules *RuleDef, customSpecs CustomSpecs, path string) error {
	if dest.Kind() == reflect.Pointer {
		dest = dest.Elem()
	}
	for key, rule := range rules.properties {
		name := path + "." + key
		vals, ok := form[key]
		if !ok {
			// Check for nested fields even if the top-level key isn't there
			if rule.kind == reflect.Slice || rule.kind == reflect.Array || rule.kind == reflect.Struct {
				// Continue to handle nested binding
			} else {
				if rule.required || rule.present {
					return newErrReport(RequestErr, schemaBody, name, "required", errors.New("field is required"))
				}
				continue
			}
		}

		fieldVal, err := formBindField(dest, rule, name)
		if err != nil {
			return err
		}
		if !fieldVal.IsValid() {
			continue
		}

		parseVal := func(v string, r *RuleDef) (any, error) {
			if r.format == utils.TimeObjectFormat {
				return time.Parse(r.pattern, v)
			}
			// Check custom spec first
			if spec, ok := customSpecs.Find(string(r.format)); ok {
				return decodeSpecValue(spec, v)
			}
			return utils.PrimitiveFromStr(r.kind, v)
		}

		// bindScalar parses, validates and binds one value.
		bindScalar := func(field reflect.Value, v string, r *RuleDef, name string) error {
			val, err := parseVal(v, r)
			if err != nil {
				return newErrReport(RequestErr, schemaBody, name, "typeCast", err)
			}
			validate := runValidation
			if _, isSpec := customSpecs.Find(string(r.format)); isSpec {
				validate = runSpecValidation
			}
			if err := validate(val, RequestErr, schemaBody, name, r.rules); err != nil {
				return err
			}
			if err := f.bindValue(field, val); err != nil {
				return newErrReport(RequestErr, schemaBody, name, "typeMismatch", err)
			}
			return nil
		}

		// A spec field is one value, whatever its kind; it is bound as a scalar below.
		_, isSpecField := customSpecs.Find(string(rule.format))
		if (rule.kind == reflect.Slice || rule.kind == reflect.Array) && !isSpecField {
			elemRule := rule.item
			if elemRule == nil {
				elemRule = &RuleDef{kind: fieldVal.Type().Elem().Kind()}
			}
			if elemRule.kind == reflect.Struct && elemRule.format != utils.TimeObjectFormat {
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

					if err := f.bindStruct(subForm, istrct, elemRule, customSpecs, fmt.Sprintf("%s.%d", name, i)); err != nil {
						return err
					}

					nslice = reflect.Append(nslice, istrct)
					i++
				}
				if err := f.bindValue(fieldVal, nslice.Interface()); err != nil {
					return newErrReport(RequestErr, schemaBody, name, "typeMismatch", err)
				}
			} else {
				// Primitive slice
				sliceType := fieldVal.Type()
				if sliceType.Kind() == reflect.Pointer {
					sliceType = sliceType.Elem()
				}
				if handled, err := bindFormBytes(fieldVal, sliceType, vals, rule, name); handled {
					if err != nil {
						return err
					}
					continue
				}
				slice := reflect.MakeSlice(sliceType, len(vals), len(vals))
				for i, v := range vals {
					if err := bindScalar(slice.Index(i), v, elemRule, fmt.Sprintf("%s.%d", name, i)); err != nil {
						return err
					}
				}
				if err := f.bindValue(fieldVal, slice.Interface()); err != nil {
					return newErrReport(RequestErr, schemaBody, name, "typeMismatch", err)
				}
			}
		} else if rule.kind == reflect.Struct || isSpecField {
			// If this struct-like field has a custom spec format, treat it as a scalar
			if _, isCustom := customSpecs.Find(string(rule.format)); isCustom || rule.format == utils.TimeObjectFormat {
				if len(vals) > 0 {
					if err := bindScalar(fieldVal, vals[0], rule, name); err != nil {
						return err
					}
				} else if rule.required || rule.present {
					return newErrReport(RequestErr, schemaBody, name, "required", errors.New("field is required"))
				}
				continue
			}

			// Nested struct
			prefix := key + "."
			subForm := make(map[string][]string)
			for k, v := range form {
				if strings.HasPrefix(k, prefix) {
					subForm[strings.TrimPrefix(k, prefix)] = v
				}
			}
			if err := f.bindStruct(subForm, fieldVal, rule, customSpecs, name); err != nil {
				return err
			}
		} else {
			if len(vals) > 0 {
				if err := bindScalar(fieldVal, vals[0], rule, name); err != nil {
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

// formBindField returns the field a form value binds to. A field promoted through
// an embedded pointer allocates it, as a JSON body does, so call this only when
// there is a value to bind.
func formBindField(parent reflect.Value, rule *RuleDef, key string) (reflect.Value, error) {
	index := rule.accessor.index
	if len(index) <= 1 {
		return parent.FieldByName(rule.fieldName), nil
	}
	if !requestFieldParentsReady(parent, index) {
		return reflect.Value{}, newErrReport(RequestErr, schemaBody, key, "binding", fmt.Errorf("field index %v traverses a nil or invalid parent", index))
	}
	return requestCompiledField(parent, index), nil
}

// bindFormBytes handles []byte and [N]byte fields in form and multipart bodies:
// their text is base64, as it is in JSON. Any other fixed-size array can't be
// bound from a form. It reports whether it handled the field.
func bindFormBytes(field reflect.Value, t reflect.Type, vals []string, rule *RuleDef, key string) (bool, error) {
	isBytes := t.Elem().Kind() == reflect.Uint8
	if t.Kind() != reflect.Array && !isBytes {
		return false, nil
	}
	if len(vals) == 0 {
		if rule.required || rule.present {
			return true, newErrReport(RequestErr, schemaBody, key, "required", errors.New("field is required"))
		}
		return true, nil
	}
	if !isBytes {
		return true, newErrReport(RequestErr, schemaBody, key, "typeMismatch", fmt.Errorf("a form value can't fill %s", t))
	}
	if len(vals) != 1 {
		return true, newErrReport(RequestErr, schemaBody, key, "typeMismatch", errors.New("expected one value"))
	}
	decoded, err := base64.StdEncoding.DecodeString(vals[0])
	if err == nil {
		err = checkByteArrayLen(t, decoded)
	}
	if err != nil {
		return true, newErrReport(RequestErr, schemaBody, key, "typeCast", err)
	}
	if err := runValidation(decoded, RequestErr, schemaBody, key, rule.rules); err != nil {
		return true, err
	}
	bindDecodedBytes(field, decoded)
	return true, nil
}
