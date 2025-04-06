package gofi

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/michaelolof/gofi/utils"
)

func (c *context) Send(code int, obj any) error {

	_, rules, err := c.rules.getRespRulesByCode(code)
	if err != nil {
		return err
	}

	if len(rules) == 0 {
		return nil
	}

	if obj == nil {
		// TODO.  If there's is no response body defined, this should be fine
		return errors.New("undefined schema when calling the gofi Send function")
	}

	// Handle if object is a pointer
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return errors.New("bad response. invalid response type. response object must be a struct")
	}

	if err := c.validateAndEncodeHeaders(rules, rv.FieldByName(string(schemaHeaders))); err != nil {
		return err
	}

	if err := c.validateAndEncodeCookie(rules, rv.FieldByName(string(schemaCookies))); err != nil {
		return err
	}

	contentType := c.rules.reqContent()
	sz, err := c.serverOpts.getSerializer(contentType)
	if err != nil {
		return newErrReport(RequestErr, schemaBody, string(contentType), "required", err)
	}

	var bdef ruleDef
	if v, ok := rules[string(schemaBody)]; ok {
		bdef = v
	}

	bs, err := sz.ValidateAndEncode(obj, ResponseOptions{
		Context:     c,
		SchemaRules: &bdef,
		Body:        rv.FieldByName(string(schemaBody)),
	})
	if err != nil {
		return err
	}

	c.w.WriteHeader(code)
	_, err = c.w.Write(bs)
	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "writer", err)
	}

	return nil
}

type headerSetter interface {
	ValidateAndEncodeHeaders(c Context) error
}

func (c *context) validateAndEncodeHeaders(rules ruleDefMap, headers reflect.Value) error {
	var ruleProps map[string]*ruleDef
	if v, ok := rules[string(schemaHeaders)]; ok {
		ruleProps = v.properties
	}

	if len(ruleProps) == 0 {
		return nil
	}

	if !headers.IsValid() {
		return newErrReport(ResponseErr, schemaHeaders, "", "invalid_error", errors.New("headers object is invalid"))
	}

	if s, ok := headers.Interface().(headerSetter); ok {
		return s.ValidateAndEncodeHeaders(c)
	}

	for key, val := range ruleProps {

		hf := headers.FieldByName(val.fieldName)
		if !hf.IsValid() {
			continue
		}

		hv := hf.Interface()
		checkAndSet := func(val string, key string, rules []ruleOpts) error {
			err := runValidation(ResponseErr, val, schemaHeaders, key, rules)
			if err != nil {
				return err
			}

			c.w.Header().Set(key, val)
			return nil
		}

		if spec, ok := c.serverOpts.customSpecs[string(val.format)]; ok {
			if spec.Encoder != nil {
				v, err := spec.Encoder(hv)
				if err != nil {
					return newErrReport(ResponseErr, schemaHeaders, key, "typeMismatch", err)
				}

				if err := checkAndSet(v, key, val.rules); err != nil {
					return err
				}
			} else {
				switch vm := hv.(type) {
				case json.Marshaler:
					v, err := vm.MarshalJSON()
					if err != nil {
						return newErrReport(ResponseErr, schemaHeaders, key, "json-marshal", err)
					}

					if err := checkAndSet(string(v), key, val.rules); err != nil {
						return err
					}
				case encoding.TextMarshaler:
					v, err := vm.MarshalText()
					if err != nil {
						return newErrReport(ResponseErr, schemaHeaders, key, "text-marshal", err)
					}

					if err := checkAndSet(string(v), key, val.rules); err != nil {
						return err
					}
				}
			}
		} else {
			switch true {
			case utils.IsPrimitiveKind(val.kind):
				if utils.PrimitiveKindIsEmpty(val.kind, hv) && val.defVal != nil {
					hv = val.defVal
				}
				err := runValidation(ResponseErr, hv, schemaHeaders, key, val.rules)
				if err != nil {
					return err
				}

				c.w.Header().Set(key, fmt.Sprintf("%v", hv))

			case val.format == utils.TimeObjectFormat:
				switch hf.Kind() {
				case reflect.Pointer:
					tv, ok := hv.(*time.Time)
					if !ok {
						return newErrReport(ResponseErr, schemaHeaders, key, "parser", errors.New("unable to parse header"))
					}

					err := runValidation(ResponseErr, tv, schemaHeaders, key, val.rules)
					if err != nil {
						return err
					}

					c.w.Header().Set(key, tv.Format(val.pattern))
				case reflect.Struct:
					tv, ok := hv.(time.Time)
					if !ok {
						return newErrReport(ResponseErr, schemaHeaders, key, "parser", errors.New("unable to parse header"))
					}

					err := runValidation(ResponseErr, tv, schemaHeaders, key, val.rules)
					if err != nil {
						return err
					}

					c.w.Header().Set(key, tv.Format(val.pattern))
				}
			}
		}
	}

	return nil
}

type cookieSetter interface {
	ValidateAndEncodeCookies(c Context) error
}

func (c *context) validateAndEncodeCookie(rules ruleDefMap, cookies reflect.Value) error {
	// For cookies only primitives and cookie object is supported
	var ruleProps map[string]*ruleDef
	if v, ok := rules[string(schemaCookies)]; ok {
		ruleProps = v.properties
	}

	if len(ruleProps) == 0 {
		return nil
	}

	if !cookies.IsValid() {
		return newErrReport(ResponseErr, schemaCookies, "", "invalid_error", errors.New("headers object is invalid"))
	}

	if s, ok := cookies.Interface().(cookieSetter); ok {
		return s.ValidateAndEncodeCookies(c)
	}

	for key, val := range ruleProps {
		cf := cookies.FieldByName(val.fieldName)
		if !cf.IsValid() {
			continue
		}

		cv := cf.Interface()
		switch true {
		case utils.IsPrimitiveKind(val.kind):
			if utils.PrimitiveKindIsEmpty(val.kind, cv) && val.defVal != nil {
				cv = val.defVal
			}
			err := runValidation(ResponseErr, cv, schemaHeaders, key, val.rules)
			if err != nil {
				return err
			}
			http.SetCookie(c.w, &http.Cookie{Name: key, Value: fmt.Sprintf("%v", cv)})

		case val.format == utils.CookieObjectFormat:
			switch cf.Kind() {
			case reflect.Pointer:
				cook, ok := cv.(*http.Cookie)
				if !ok {
					return newErrReport(ResponseErr, schemaCookies, "", "parser", errors.New("unable to parse cookie"))
				}

				var cookV string
				if cook != nil {
					cookV = cook.Value
				}

				err := runValidation(ResponseErr, cookV, schemaHeaders, key, val.rules)
				if err != nil {
					return err
				}

				http.SetCookie(c.w, cook)
			case reflect.Struct:
				cook, ok := cv.(http.Cookie)
				if !ok {
					return newErrReport(ResponseErr, schemaCookies, "", "parser", errors.New("unable to parse cookie"))
				}

				err := runValidation(ResponseErr, cook.Value, schemaHeaders, key, val.rules)
				if err != nil {
					return err
				}

				http.SetCookie(c.w, &cook)
			}
		}
	}

	return nil
}
