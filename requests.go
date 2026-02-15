package gofi

import (
	"errors"
	"net/http"
	"reflect"
	"time"

	"github.com/michaelolof/gofi/utils"
)

func Validate(c Context) error {
	ctx, ok := c.(*context)
	if !ok {
		return errors.New("unknown context object passed")
	}

	if ctx.bindedCacheResult.bound {
		if ctx.bindedCacheResult.err != nil {
			return ctx.bindedCacheResult.err
		} else {
			return nil
		}
	}

	_, err := validateAndOrBindRequest[any](ctx, false)
	if err != nil {
		ctx.bindedCacheResult = bindedResult{bound: true, err: err}
		return err
	}

	return nil
}

func ValidateAndBind[T any](c Context) (*T, error) {
	ctx, ok := c.(*context)
	if !ok {
		return nil, errors.New("unknown context object passed")
	}

	if ctx.bindedCacheResult.bound {
		if ctx.bindedCacheResult.err != nil {
			return nil, ctx.bindedCacheResult.err
		} else if v, ok := ctx.bindedCacheResult.val.(*T); ok {
			return v, nil
		}
	}

	s, err := validateAndOrBindRequest[T](ctx, true)
	if err != nil {
		ctx.bindedCacheResult = bindedResult{bound: true, err: err}
		return nil, err
	}

	ctx.bindedCacheResult = bindedResult{bound: true, val: s}
	return s, nil
}

func validateAndOrBindRequest[T any](c *context, shouldBind bool) (*T, error) {
	var schemaPtr *T
	if shouldBind {
		schemaPtr = new(T)
	}

	if c.rules() == nil {
		return nil, newErrReport(RequestErr, schemaReq, "", "required", errors.New("schema not properly registered to route handler"))
	}

	if len(c.rules().req) == 0 {
		return schemaPtr, nil
	}

	defer func() {
		if e := recover(); e != nil {
			c.serverOpts.logger.Error(newErrReport(ResponseErr, schemaBody, "", "typeMismatch", e.(error)))
		}
	}()

	var reqStruct reflect.Value
	if shouldBind {
		reqStruct = reflect.ValueOf(schemaPtr).Elem().FieldByName(string(schemaReq))
	}

	validateStrAndBind := func(field schemaField, qv string, def *RuleDef) error {
		if qv == "" && def.defStr != "" {
			qv = def.defStr
		}

		if !def.required && qv == "" {
			return nil
		}

		var val any
		var err error
		if spec, ok := c.serverOpts.customSpecs.Find(string(def.format)); ok {
			val, err = spec.Decode(qv)
			if err != nil {
				return newErrReport(RequestErr, field, def.field, "typeCast", err)
			}
		} else {
			val, err = utils.PrimitiveFromStr(def.kind, qv)
			if err != nil || utils.NotPrimitive(val) {
				if err == nil {
					err = errors.New("unsupported header type passed")
				}
				// Handle special cases.
				switch def.format {
				case utils.TimeObjectFormat:
					val, err = time.Parse(def.pattern, qv)
					if err != nil {
						return newErrReport(RequestErr, field, def.field, "typeCast", err)
					}
				default:
					return newErrReport(RequestErr, field, def.field, "typeCast", err)
				}
			}
		}

		err = runValidation(val, RequestErr, field, def.field, def.rules)
		if err != nil {
			return err
		}

		if shouldBind {
			sf := reqStruct.FieldByName(string(field)).FieldByName(def.fieldName)
			sf.Set(reflect.ValueOf(val).Convert(sf.Type()))
		}

		return nil
	}

	// Handle Headers
	pdef := c.rules().getReqRules(schemaHeaders)
	errs := make([]error, 0, len(pdef.properties))
	for _, def := range pdef.properties {
		hv := c.r.Header.Get(def.field)
		err := validateStrAndBind(schemaHeaders, hv, def)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Handle queries
	pdef = c.rules().getReqRules(schemaQuery)
	errs = make([]error, 0, len(pdef.properties))
	for _, def := range pdef.properties {
		qv := c.r.URL.Query().Get(def.field)
		err := validateStrAndBind(schemaQuery, qv, def)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Handle Paths
	pdef = c.rules().getReqRules(schemaPath)
	errs = make([]error, 0, len(pdef.properties))
	for _, def := range pdef.properties {
		pv := c.r.PathValue(def.field)
		err := validateStrAndBind(schemaPath, pv, def)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Handle Cookies
	pdef = c.rules().getReqRules(schemaCookies)
	errs = make([]error, 0, len(pdef.properties))
	for _, def := range pdef.properties {
		cv, err := c.r.Cookie(def.field)
		if def.required && err == http.ErrNoCookie {
			errs = append(errs, err)
			continue
		} else if !def.required && cv == nil {
			continue
		} else if err != nil {
			errs = append(errs, err)
			continue
		}

		switch def.format {
		case utils.CookieObjectFormat:
			err := runValidation(cv.Value, RequestErr, schemaCookies, def.field, def.rules)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if shouldBind {
				sf := reqStruct.FieldByName(string(schemaCookies)).FieldByName(def.fieldName)
				if sf.Kind() == reflect.Pointer {
					sf.Set(reflect.ValueOf(cv).Convert(sf.Type()))
				} else if cv != nil {
					sf.Set(reflect.ValueOf(*cv).Convert(sf.Type()))
				}
			}

		default:
			cvs, err := utils.PrimitiveFromStr(def.kind, cv.Value)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if utils.NotPrimitive(cvs) {
				errs = append(errs, newErrReport(RequestErr, schemaCookies, def.field, "invalid_type", errors.New("only primitives and http.Cookie types are supported")))
				continue
			}

			err = runValidation(cvs, RequestErr, schemaCookies, def.field, def.rules)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if shouldBind {
				sf := reqStruct.FieldByName(string(schemaCookies)).FieldByName(def.fieldName)
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
	pdef = c.rules().getReqRules(schemaBody)
	if pdef == nil || pdef.kind == reflect.Invalid {
		return schemaPtr, nil
	}

	body := c.r.Body
	if body == nil && pdef.required {
		return schemaPtr, newErrReport(RequestErr, schemaBody, "", "required", errors.New("request body is required"))
	} else if body == nil {
		return schemaPtr, nil
	}

	contentType := c.rules().reqContent()
	sz, err := c.serverOpts.getSerializer(contentType)
	if err != nil {
		return schemaPtr, newErrReport(RequestErr, schemaBody, string(contentType), "required", err)
	}

	err = sz.ValidateAndDecodeRequest(body, RequestOptions{
		ShouldBind:  shouldBind,
		Context:     &parserContext{c: c},
		SchemaPtr:   schemaPtr,
		Body:        &reqStruct,
		SchemaRules: pdef,
	})
	if err != nil {
		return schemaPtr, err
	}

	return schemaPtr, errors.Join(errs...)

}
