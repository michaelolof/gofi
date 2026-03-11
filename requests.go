package gofi

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"time"

	"github.com/michaelolof/gofi/utils"
)

// RequestSchema defines supported request segments for selective validation/binding.
type RequestSchema string

const (
	Header RequestSchema = "request.header"
	Path   RequestSchema = "request.path"
	Query  RequestSchema = "request.query"
	Cookie RequestSchema = "request.cookie"
	Body   RequestSchema = "request.body"
)

type requestPartMask uint8

const (
	partHeader requestPartMask = 1 << iota
	partPath
	partQuery
	partCookie
	partBody
)

const partAll = partHeader | partPath | partQuery | partCookie | partBody

func buildRequestPartMask(selectors []RequestSchema) (requestPartMask, error) {
	if len(selectors) == 0 {
		return partAll, nil
	}

	var mask requestPartMask
	for _, selector := range selectors {
		switch selector {
		case Header:
			mask |= partHeader
		case Path:
			mask |= partPath
		case Query:
			mask |= partQuery
		case Cookie:
			mask |= partCookie
		case Body:
			mask |= partBody
		default:
			return 0, newErrReport(RequestErr, schemaReq, string(selector), "invalid_selector", errors.New("unsupported request schema selector"))
		}
	}

	return mask, nil
}

func Validate(c Context, s ...RequestSchema) error {
	ctx, ok := c.(*context)
	if !ok {
		return errors.New("unknown context object passed")
	}

	mask, err := buildRequestPartMask(s)
	if err != nil {
		return err
	}

	useCache := mask == partAll

	if useCache && ctx.bindedCacheResult.bound {
		if ctx.bindedCacheResult.err != nil {
			return ctx.bindedCacheResult.err
		} else {
			return nil
		}
	}

	_, err = validateAndOrBindRequest[any](ctx, false, mask)
	if err != nil {
		if useCache {
			ctx.bindedCacheResult = bindedResult{bound: true, err: err}
		}
		return err
	}

	if useCache {
		ctx.bindedCacheResult = bindedResult{bound: true}
	}

	return nil
}

func ValidateAndBind[T any](c Context, s ...RequestSchema) (*T, error) {
	ctx, ok := c.(*context)
	if !ok {
		return nil, errors.New("unknown context object passed")
	}

	mask, err := buildRequestPartMask(s)
	if err != nil {
		return nil, err
	}

	useCache := mask == partAll

	if useCache && ctx.bindedCacheResult.bound {
		if ctx.bindedCacheResult.err != nil {
			return nil, ctx.bindedCacheResult.err
		} else if v, ok := ctx.bindedCacheResult.val.(*T); ok {
			return v, nil
		}
	}

	schema, err := validateAndOrBindRequest[T](ctx, true, mask)
	if err != nil {
		if useCache {
			ctx.bindedCacheResult = bindedResult{bound: true, err: err}
		}
		return nil, err
	}

	if useCache {
		ctx.bindedCacheResult = bindedResult{bound: true, val: schema}
	}
	return schema, nil
}

func validateAndOrBindRequest[T any](c *context, shouldBind bool, mask requestPartMask) (*T, error) {
	var schemaPtr *T

	if c.rules() == nil {
		return nil, newErrReport(RequestErr, schemaReq, "", "required", errors.New("schema not properly registered to route handler"))
	}

	if shouldBind {
		if c.rules().schemaPool != nil {
			schemaPtr = c.rules().schemaPool.Get().(*T)
		} else {
			schemaPtr = new(T)
		}
	}

	if len(c.rules().req) == 0 {
		return schemaPtr, nil
	}

	var reqStruct reflect.Value
	if shouldBind {
		reqStruct = reflect.ValueOf(schemaPtr).Elem().FieldByName(string(schemaReq))
	}

	// Shared error buffer — nil-initialized, only allocates on first error
	var errs []error

	// Handle Headers
	if mask&partHeader != 0 {
		if pdef := c.rules().getReqRules(schemaHeaders); pdef != nil && len(pdef.properties) > 0 {
			for _, def := range pdef.properties {
				hv := c.headerGet(def.field)
				if err := doValidateStrAndBind(c, schemaHeaders, hv, def, shouldBind, reqStruct); err != nil {
					errs = append(errs, err)
				}
			}
			if len(errs) > 0 {
				return nil, errors.Join(errs...)
			}
		}
	}

	// Handle queries
	if mask&partQuery != 0 {
		if pdef := c.rules().getReqRules(schemaQuery); pdef != nil && len(pdef.properties) > 0 {
			for _, def := range pdef.properties {
				qv := c.queryGet(def.field)
				if err := doValidateStrAndBind(c, schemaQuery, qv, def, shouldBind, reqStruct); err != nil {
					errs = append(errs, err)
				}
			}
			if len(errs) > 0 {
				return nil, errors.Join(errs...)
			}
		}
	}

	// Handle Paths
	if mask&partPath != 0 {
		if pdef := c.rules().getReqRules(schemaPath); pdef != nil && len(pdef.properties) > 0 {
			for _, def := range pdef.properties {
				pv := c.params.Get(def.field)
				if err := doValidateStrAndBind(c, schemaPath, pv, def, shouldBind, reqStruct); err != nil {
					errs = append(errs, err)
				}
			}
			if len(errs) > 0 {
				return nil, errors.Join(errs...)
			}
		}
	}

	// Handle Cookies
	if mask&partCookie != 0 {
		if pdef := c.rules().getReqRules(schemaCookies); pdef != nil && len(pdef.properties) > 0 {
			for _, def := range pdef.properties {
				cv, err := c.cookieGet(def.field)
				if def.required && err == http.ErrNoCookie {
					errs = append(errs, newErrReport(RequestErr, schemaCookies, def.field, "required", err))
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
		}
	}

	if mask&partBody == 0 {
		if shouldBind {
			bindRequestBodyWithoutValidation(c, schemaPtr, reqStruct)
		}
		return schemaPtr, nil
	}

	// Handle Body
	pdef := c.rules().getReqRules(schemaBody)
	if pdef == nil || pdef.kind == reflect.Invalid {
		return schemaPtr, nil
	}

	bodyBytes := c.Body()
	if len(bodyBytes) == 0 && pdef.required {
		return schemaPtr, newErrReport(RequestErr, schemaBody, "", "required", errors.New("request body is required"))
	} else if len(bodyBytes) == 0 {
		return schemaPtr, nil
	}

	// Create an io.ReadCloser from the body bytes
	body := io.NopCloser(bytes.NewReader(bodyBytes))

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

	return schemaPtr, nil

}

func bindRequestBodyWithoutValidation[T any](c *context, schemaPtr *T, reqStruct reflect.Value) {
	pdef := c.rules().getReqRules(schemaBody)
	if pdef == nil || pdef.kind == reflect.Invalid {
		return
	}

	bodyBytes := c.Body()
	if len(bodyBytes) == 0 {
		return
	}

	body := io.NopCloser(bytes.NewReader(bodyBytes))
	contentType := c.rules().reqContent()
	sz, err := c.serverOpts.getSerializer(contentType)
	if err != nil {
		return
	}

	_ = sz.ValidateAndDecodeRequest(body, RequestOptions{
		ShouldBind:  true,
		Context:     &parserContext{c: c},
		SchemaPtr:   schemaPtr,
		Body:        &reqStruct,
		SchemaRules: stripValidationRules(pdef),
	})
}

func stripValidationRules(rule *RuleDef) *RuleDef {
	if rule == nil {
		return nil
	}

	clone := *rule
	clone.required = false
	clone.max = nil
	clone.rules = nil

	if rule.item != nil {
		clone.item = stripValidationRules(rule.item)
	}

	if rule.additionalProperties != nil {
		clone.additionalProperties = stripValidationRules(rule.additionalProperties)
	}

	if rule.properties != nil {
		clone.properties = make(map[string]*RuleDef, len(rule.properties))
		for key, child := range rule.properties {
			clone.properties[key] = stripValidationRules(child)
		}
	}

	if rule.orderedProps != nil {
		clone.orderedProps = make([]*RuleDef, 0, len(rule.orderedProps))
		for _, child := range rule.orderedProps {
			clone.orderedProps = append(clone.orderedProps, stripValidationRules(child))
		}
	}

	return &clone
}

// doValidateStrAndBind validates a string value against rules and optionally binds to a struct field.
// Extracted from closure to avoid per-request heap allocation.
func doValidateStrAndBind(c *context, field schemaField, qv string, def *RuleDef, shouldBind bool, reqStruct reflect.Value) error {
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
		rv := reflect.ValueOf(val)
		if rv.Type().ConvertibleTo(sf.Type()) {
			sf.Set(rv.Convert(sf.Type()))
		} else {
			slog.Error(newErrReport(ResponseErr, schemaBody, def.field, "typeMismatch",
				errors.New("cannot convert "+rv.Type().String()+" to "+sf.Type().String())).Error())
		}
	}

	return nil
}
