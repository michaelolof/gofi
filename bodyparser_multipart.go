package gofi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"reflect"
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
		for key, rule := range opts.SchemaRules.properties {
			// Check form values first
			vals, ok := req.MultipartForm.Value[key]

			// If not in values, check files
			files, fileOk := req.MultipartForm.File[key]

			if !ok && !fileOk {
				if rule.required {
					return newErrReport(RequestErr, schemaBody, key, "required", errors.New("field is required"))
				}
				continue
			}

			fieldVal := opts.Body.FieldByName(rule.fieldName)
			if !fieldVal.IsValid() {
				continue
			}

			// Handle Files
			if fileOk {
				// Check expected type
				// Either *multipart.FileHeader or []*multipart.FileHeader

				if rule.kind == reflect.Slice || rule.kind == reflect.Array {
					// Array of files
					elemType := fieldVal.Type().Elem() // *multipart.FileHeader
					if elemType != reflect.TypeOf(&multipart.FileHeader{}) {
						return newErrReport(RequestErr, schemaBody, key, "typeMismatch", errors.New("field must be []*multipart.FileHeader"))
					}

					slice := reflect.MakeSlice(fieldVal.Type(), len(files), len(files))
					for i, file := range files {
						slice.Index(i).Set(reflect.ValueOf(file))
					}
					fieldVal.Set(slice)
				} else {
					// Single file
					if len(files) == 0 {
						if rule.required {
							return newErrReport(RequestErr, schemaBody, key, "required", errors.New("file is required"))
						}
						continue
					}

					if fieldVal.Type() != reflect.TypeOf(&multipart.FileHeader{}) {
						return newErrReport(RequestErr, schemaBody, key, "typeMismatch", errors.New("field must be *multipart.FileHeader"))
					}
					fieldVal.Set(reflect.ValueOf(files[0]))
				}

				continue
			}

			// Handle Values (similar to FormBodyParser)
			if len(vals) == 0 {
				if rule.required {
					return newErrReport(RequestErr, schemaBody, key, "required", errors.New("value must not be empty"))
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

func (m *MultipartBodyParser) ValidateAndEncodeResponse(s any, opts ResponseOptions) ([]byte, error) {
	return nil, errors.New("multipart body parser does not support response encoding")
}
