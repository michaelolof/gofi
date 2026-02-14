package schema

import (
	"encoding"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"time"

	"github.com/michaelolof/gofi/utils"
	"github.com/michaelolof/gofi/validators"
)

func Validate(r *http.Request, rules *SchemaRules, opts *ValidationOptions) error {
	_, err := validateAndOrBindRequest[any](r, rules, opts, false)
	return err
}

func Bind[T any](r *http.Request, rules *SchemaRules, opts *ValidationOptions, target *T) error {
	// If target is nil, we can't bind, but we can return a new instance if we change signature.
	// But standard Bind usually takes a target.
	// The original ValidateAndBind returned *T.
	// We matched signature: Bind(r, rules, target, opts)

	if target == nil {
		return errors.New("target cannot be nil")
	}

	val, err := validateAndOrBindRequest[T](r, rules, opts, true)
	if err != nil {
		return err
	}

	if val != nil {
		*target = *val
	}
	return nil
}

func validateAndOrBindRequest[T any](r *http.Request, rules *SchemaRules, opts *ValidationOptions, shouldBind bool) (*T, error) {
	var schemaPtr *T
	if shouldBind {
		schemaPtr = new(T)
	}

	if rules == nil || len(rules.Req) == 0 {
		return schemaPtr, nil
	}

	// Logger handling?
	// defer func() {
	// 	if e := recover(); e != nil {
	// 		// Log error
	//      if opts != nil && opts.Logger != nil { ... }
	// 	}
	// }()

	var reqStruct reflect.Value
	if shouldBind {
		reqStruct = reflect.ValueOf(schemaPtr).Elem().FieldByName(string(SchemaReq))
	}

	validateStrAndBind := func(field SchemaField, qv string, def *RuleDef) error {
		if qv == "" && def.DefStr != "" {
			qv = def.DefStr
		}

		if !def.Required && qv == "" {
			return nil
		}

		var val any
		var err error

		var customSpecs CustomSpecs
		if opts != nil {
			customSpecs = opts.CustomSpecs
		}

		if spec, ok := customSpecs[string(def.Format)]; ok {
			if spec.Decoder != nil {
				val, err = spec.Decoder(qv)
				if err != nil {
					return NewErrReport(RequestErr, field, def.Field, "typeCast", err)
				}
			} else {
				sf := reqStruct.FieldByName(string(field)).FieldByName(def.FieldName)
				// Only support structs cause pointers will default to nil which is not possible to mutate
				if sf.Kind() != reflect.Pointer {
					sfp := reflect.New(sf.Type())
					if sfp.Type().NumMethod() > 0 && sfp.CanInterface() {
						switch v := (sfp.Interface()).(type) {
						case json.Unmarshaler:
							if err := v.UnmarshalJSON([]byte(qv)); err != nil {
								return NewErrReport(RequestErr, field, def.Field, "json-unmarshal", err)
							}

							sf.Set(reflect.ValueOf(v).Elem().Convert(sf.Type()))
							return nil
						case encoding.TextUnmarshaler:
							if err := v.UnmarshalText([]byte(qv)); err != nil {
								return NewErrReport(RequestErr, field, def.Field, "text-unmarshal", err)
							}

							sf.Set(reflect.ValueOf(v).Elem().Convert(sf.Type()))
							return nil
						}
					}
				}
			}
		} else {
			val, err = utils.PrimitiveFromStr(def.Kind, qv)
			if err != nil || utils.NotPrimitive(val) {
				if err == nil {
					err = errors.New("unsupported header type passed")
				}
				// Handle special cases.
				switch def.Format {
				case utils.TimeObjectFormat:
					val, err = time.Parse(def.Pattern, qv)
					if err != nil {
						return NewErrReport(RequestErr, field, def.Field, "typeCast", err)
					}
				default:
					return NewErrReport(RequestErr, field, def.Field, "typeCast", err)
				}
			}
		}

		// Create validator args
		// ValidatorArg needs http.request and responseWriter.
		// We only have request here. ResponseWriter might be nil or we need to pass it in opts.
		// For request validation, ResponseWriter is usually not needed unless we want to access it?
		// validators.NewValidatorArg(val, validators.ReuestType, r, nil)

		vArg := validators.NewValidatorArg(val, validators.ReuestType, r, nil)

		err = RunValidation(&vArg, val, field, def.Field, def.Rules)
		if err != nil {
			return err
		}

		if shouldBind {
			sf := reqStruct.FieldByName(string(field)).FieldByName(def.FieldName)
			sf.Set(reflect.ValueOf(val).Convert(sf.Type()))
		}

		return nil
	}

	// Handle Headers
	pdef := rules.GetReqRules(SchemaHeaders)
	errs := make([]error, 0, len(pdef.Properties))
	for _, def := range pdef.Properties {
		hv := r.Header.Get(def.Field)
		err := validateStrAndBind(SchemaHeaders, hv, def)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Handle queries
	pdef = rules.GetReqRules(SchemaQuery)
	errs = make([]error, 0, len(pdef.Properties))
	for _, def := range pdef.Properties {
		qv := r.URL.Query().Get(def.Field)
		err := validateStrAndBind(SchemaQuery, qv, def)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Handle Paths
	pdef = rules.GetReqRules(SchemaPath)
	errs = make([]error, 0, len(pdef.Properties))
	for _, def := range pdef.Properties {
		pv := r.PathValue(def.Field)
		err := validateStrAndBind(SchemaPath, pv, def)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Handle Cookies
	pdef = rules.GetReqRules(SchemaCookies)
	errs = make([]error, 0, len(pdef.Properties))
	for _, def := range pdef.Properties {
		cv, err := r.Cookie(def.Field)
		if def.Required && err == http.ErrNoCookie {
			errs = append(errs, err)
			continue
		} else if !def.Required && cv == nil {
			continue
		} else if err != nil {
			errs = append(errs, err)
			continue
		}

		switch def.Format {
		case utils.CookieObjectFormat:
			vArg := validators.NewValidatorArg(cv.Value, validators.ReuestType, r, nil)
			err := RunValidation(&vArg, cv.Value, SchemaCookies, def.Field, def.Rules)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if shouldBind {
				sf := reqStruct.FieldByName(string(SchemaCookies)).FieldByName(def.FieldName)
				if sf.Kind() == reflect.Pointer {
					sf.Set(reflect.ValueOf(cv).Convert(sf.Type()))
				} else if cv != nil {
					sf.Set(reflect.ValueOf(*cv).Convert(sf.Type()))
				}
			}

		default:
			cvs, err := utils.PrimitiveFromStr(def.Kind, cv.Value)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if utils.NotPrimitive(cvs) {
				errs = append(errs, NewErrReport(RequestErr, SchemaCookies, def.Field, "invalid_type", errors.New("only primitives and http.Cookie types are supported")))
				continue
			}

			vArg := validators.NewValidatorArg(cvs, validators.ReuestType, r, nil)
			err = RunValidation(&vArg, cvs, SchemaCookies, def.Field, def.Rules)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if shouldBind {
				sf := reqStruct.FieldByName(string(SchemaCookies)).FieldByName(def.FieldName)
				if sf.Kind() == reflect.Pointer {
					sf.Elem().Set(reflect.ValueOf(cvs).Convert(sf.Elem().Type()))
				} else {
					sf.Set(reflect.ValueOf(cvs).Convert(sf.Type()))
				}
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Handle Body
	pdef = rules.GetReqRules(SchemaBody)
	if pdef == nil || pdef.Kind == reflect.Invalid {
		return schemaPtr, nil
	}

	body := r.Body
	if body == nil && pdef.Required {
		return schemaPtr, NewErrReport(RequestErr, SchemaBody, "", "required", errors.New("request body is required"))
	} else if body == nil {
		return schemaPtr, nil
	}

	contentType := rules.ReqContent()

	// Get serializer
	var sz SchemaEncoder
	if opts != nil && opts.Serializers != nil {
		if s, ok := opts.Serializers(contentType); ok {
			sz = s
		}
	}

	// Fallback to builtin?
	// I should move builtinSerializer to schema package or export it?
	// I haven't moved JSONSchemaEncoder yet.
	if sz == nil {
		// Use default JSON encoder
		// I need to implement JSONSchemaEncoder in schema package or reuse
		sz = &JSONSchemaEncoder{} // Assuming I will implement it
	}

	err := sz.ValidateAndDecode(body, RequestValidationOptions{
		ShouldEncode:      shouldBind,
		Context:           nil, // Passed nil for now or pass w if available?
		SchemaPtrInstance: schemaPtr,
		SchemaRules:       pdef,
		FieldStruct:       &reqStruct,
		SchemaField:       SchemaBody,
	})
	if err != nil {
		return schemaPtr, err
	}

	return schemaPtr, errors.Join(errs...)

}
