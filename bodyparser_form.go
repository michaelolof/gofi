package gofi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
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
		for key, rule := range opts.SchemaRules.properties {
			vals, ok := req.PostForm[key]

			// Check required
			if !ok {
				if rule.required {
					return newErrReport(RequestErr, schemaBody, key, "required", errors.New("field is required"))
				}
				continue
			}

			// Empty check
			if len(vals) == 0 {
				if rule.required {
					return newErrReport(RequestErr, schemaBody, key, "required", errors.New("value must not be empty"))
				}
				continue
			}

			fieldVal := opts.Body.FieldByName(rule.fieldName)
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
					// Fallback if item rule not defined (shouldn't happen for properly compiled schema)
					elemRule = &RuleDef{kind: fieldVal.Type().Elem().Kind()}
				}

				slice := reflect.MakeSlice(fieldVal.Type(), 0, len(vals))
				for i, v := range vals {
					val, err := parseVal(v, elemRule)
					if err != nil {
						return newErrReport(RequestErr, schemaBody, fmt.Sprintf("%s.%d", key, i), "typeCast", err)
					}

					if err := runValidation(val, RequestErr, schemaBody, fmt.Sprintf("%s.%d", key, i), elemRule.rules); err != nil {
						return err
					}

					rv := reflect.ValueOf(val)
					rv = rv.Convert(fieldVal.Type().Elem())
					slice = reflect.Append(slice, rv)
				}
				fieldVal.Set(slice)

			} else {
				// Single value
				val, err := parseVal(vals[0], rule)
				if err != nil {
					return newErrReport(RequestErr, schemaBody, key, "typeCast", err)
				}

				if err := runValidation(val, RequestErr, schemaBody, key, rule.rules); err != nil {
					return err
				}

				fieldVal.Set(reflect.ValueOf(val).Convert(fieldVal.Type()))
			}
		}
	}

	return nil
}

func (f *FormBodyParser) ValidateAndEncodeResponse(s any, opts ResponseOptions) ([]byte, error) {
	return nil, errors.New("form body parser does not support response encoding")
}
