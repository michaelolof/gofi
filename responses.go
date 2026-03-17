package gofi

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/michaelolof/gofi/utils"
)

func (c *context) Send(code int, obj any) error {
	if c.rules() == nil {
		return newErrReport(ResponseErr, schemaBody, "", "required", errors.New("schema not properly registered to route handler"))
	}

	_, rules, err := c.rules().getRespRulesByCode(code)
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

	contentType := c.rules().respContent(code)
	sz, err := c.serverOpts.getSerializer(contentType)
	if err != nil {
		return newErrReport(RequestErr, schemaBody, string(contentType), "required", err)
	}

	var bdef RuleDef
	if v, ok := rules[string(schemaBody)]; ok {
		bdef = v
	}

	bs, err := sz.ValidateAndEncodeResponse(obj, ResponseOptions{
		Context:     &parserContext{c: c},
		SchemaRules: &bdef,
		Body:        rv.FieldByName(string(schemaBody)),
	})
	if err != nil {
		return err
	}

	c.fctx.Response.Header.Set("Content-Type", string(contentType))
	c.fctx.Response.SetStatusCode(code)
	c.fctx.Response.SetBody(bs)

	return nil
}

type headerSetter interface {
	ValidateAndEncodeHeaders(c Context) error
}

// headerAlreadyWritten reports whether a response header with the given key has
// already been set — either directly on the fasthttp response (e.g. by middleware
// that writes to c.fctx.Response.Header) or in the ResponseWriter adapter's
// pending header map (e.g. by middleware that calls c.Writer().Header().Set).
// Both locations must be checked because the adapter's headers are not synced to
// fasthttp until after the handler returns.
func (c *context) headerAlreadyWritten(key string) bool {
	if len(c.fctx.Response.Header.Peek(key)) > 0 {
		return true
	}
	if c.rw != nil && c.rw.headerInit {
		if vals := c.rw.header.Values(key); len(vals) > 0 {
			return true
		}
	}
	return false
}

func (c *context) validateAndEncodeHeaders(rules ruleDefMap, headers reflect.Value) error {
	var ruleProps map[string]*RuleDef
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

		if strings.ToLower(key) == "content-type" {
			continue
		}

		hf := headers.FieldByName(val.fieldName)
		if !hf.IsValid() {
			if val.required {
				if c.headerAlreadyWritten(key) {
					continue
				}
				if err := runValidation(nil, ResponseErr, schemaHeaders, key, val.rules); err != nil {
					return err
				}
			}
			continue
		}

		hv := hf.Interface()
		checkAndSet := func(val string, key string, rules []ruleOpts) error {
			err := runValidation(val, ResponseErr, schemaHeaders, key, rules)
			if err != nil {
				return err
			}

			c.fctx.Response.Header.Set(key, val)
			return nil
		}

		if spec, ok := c.serverOpts.customSpecs.Find(string(val.format)); ok {
			v, err := spec.Encode(hv)
			if err != nil {
				return newErrReport(ResponseErr, schemaHeaders, key, "typeMismatch", err)
			}

			if v == "" && c.headerAlreadyWritten(key) {
				continue
			}

			if err := checkAndSet(v, key, val.rules); err != nil {
				return err
			}
		} else {
			switch true {
			case utils.IsPrimitiveKind(val.kind):
				if utils.PrimitiveKindIsEmpty(val.kind, hv) && val.defVal != nil {
					hv = val.defVal
				}
				if utils.PrimitiveKindIsEmpty(val.kind, hv) && c.headerAlreadyWritten(key) {
					continue
				}
				err := runValidation(hv, ResponseErr, schemaHeaders, key, val.rules)
				if err != nil {
					return err
				}

				c.fctx.Response.Header.Set(key, fmt.Sprintf("%v", hv))

			case val.format == utils.TimeObjectFormat:
				switch hf.Kind() {
				case reflect.Pointer:
					tv, ok := hv.(*time.Time)
					if !ok {
						return newErrReport(ResponseErr, schemaHeaders, key, "parser", errors.New("unable to parse header"))
					}

					if (tv == nil || tv.IsZero()) && c.headerAlreadyWritten(key) {
						continue
					}

					err := runValidation(tv, ResponseErr, schemaHeaders, key, val.rules)
					if err != nil {
						return err
					}

					c.fctx.Response.Header.Set(key, tv.Format(val.pattern))

				case reflect.Struct:
					tv, ok := hv.(time.Time)
					if !ok {
						return newErrReport(ResponseErr, schemaHeaders, key, "parser", errors.New("unable to parse header"))
					}

					if tv.IsZero() && c.headerAlreadyWritten(key) {
						continue
					}

					err := runValidation(tv, ResponseErr, schemaHeaders, key, val.rules)
					if err != nil {
						return err
					}

					c.fctx.Response.Header.Set(key, tv.Format(val.pattern))
				}
			}
		}
	}

	return nil
}

type cookieSetter interface {
	ValidateAndEncodeCookies(c Context) error
}

// cookieAlreadyWritten reports whether a Set-Cookie header for the given cookie
// name has already been written to the response — either directly on the fasthttp
// response or in the ResponseWriter adapter's pending header map.
func (c *context) cookieAlreadyWritten(name string) bool {
	found := false
	c.fctx.Response.Header.VisitAllCookie(func(key, _ []byte) {
		if string(key) == name {
			found = true
		}
	})
	if found {
		return true
	}
	if c.rw != nil && c.rw.headerInit {
		for _, cookieStr := range c.rw.header.Values("Set-Cookie") {
			if eqIdx := strings.IndexByte(cookieStr, '='); eqIdx > 0 {
				if cookieStr[:eqIdx] == name {
					return true
				}
			}
		}
	}
	return false
}

func (c *context) validateAndEncodeCookie(rules ruleDefMap, cookies reflect.Value) error {
	// For cookies only primitives and cookie object is supported
	var ruleProps map[string]*RuleDef
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
			if val.required {
				if c.cookieAlreadyWritten(key) {
					continue
				}
				if err := runValidation(nil, ResponseErr, schemaCookies, key, val.rules); err != nil {
					return err
				}
			}
			continue
		}

		cv := cf.Interface()
		switch true {
		case utils.IsPrimitiveKind(val.kind):
			if utils.PrimitiveKindIsEmpty(val.kind, cv) && val.defVal != nil {
				cv = val.defVal
			}
			if utils.PrimitiveKindIsEmpty(val.kind, cv) && c.cookieAlreadyWritten(key) {
				continue
			}
			err := runValidation(cv, ResponseErr, schemaCookies, key, val.rules)
			if err != nil {
				return err
			}
			cookie := &http.Cookie{Name: key, Value: fmt.Sprintf("%v", cv)}
			c.fctx.Response.Header.Add("Set-Cookie", cookie.String())

		case val.format == utils.CookieObjectFormat:
			switch cf.Kind() {
			case reflect.Pointer:
				cook, ok := cv.(*http.Cookie)
				if !ok {
					return newErrReport(ResponseErr, schemaCookies, "", "parser", errors.New("unable to parse cookie"))
				}

				if cf.IsNil() && c.cookieAlreadyWritten(key) {
					continue
				}

				var cookV string
				if cook != nil {
					cookV = cook.Value
				}

				err := runValidation(cookV, ResponseErr, schemaCookies, key, val.rules)
				if err != nil {
					return err
				}

				c.fctx.Response.Header.Add("Set-Cookie", cook.String())
			case reflect.Struct:
				cook, ok := cv.(http.Cookie)
				if !ok {
					return newErrReport(ResponseErr, schemaCookies, "", "parser", errors.New("unable to parse cookie"))
				}

				if cook.Value == "" && c.cookieAlreadyWritten(key) {
					continue
				}

				err := runValidation(cook.Value, ResponseErr, schemaCookies, key, val.rules)
				if err != nil {
					return err
				}

				c.fctx.Response.Header.Add("Set-Cookie", cook.String())
			}
		}
	}

	return nil
}
