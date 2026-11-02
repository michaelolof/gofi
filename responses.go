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

	opts := ResponseOptions{
		Context:     &parserContext{c: c},
		SchemaRules: &bdef,
		Body:        rv.FieldByName(string(schemaBody)),
	}
	var bs []byte
	if jp, ok := sz.(*JSONBodyParser); ok {
		// Encode into a pooled buffer; SetBody below copies it once.
		buf := getJSONBuffer()
		defer putJSONBuffer(buf)
		if err := jp.encodeResponse(buf, opts); err != nil {
			return err
		}
		bs = buf.Bytes()
	} else if bs, err = sz.ValidateAndEncodeResponse(obj, opts); err != nil {
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
		// A pointer writes the value it points to, and a nil one writes nothing.
		if hf.Kind() == reflect.Pointer && val.format != utils.TimeObjectFormat {
			if _, isSpec := c.serverOpts.customSpecs.Find(string(val.format)); !isSpec {
				if hf.IsNil() {
					if val.defVal == nil {
						if val.required && !c.headerAlreadyWritten(key) {
							if err := runValidation(nil, ResponseErr, schemaHeaders, key, val.rules); err != nil {
								return err
							}
						}
						continue
					}
					hv = val.defVal
				} else {
					hv = hf.Elem().Interface()
				}
			}
		}
		checkAndSet := func(val string, key string, rules []ruleOpts) error {
			err := runValidation(val, ResponseErr, schemaHeaders, key, rules)
			if err != nil {
				return err
			}
			return c.setSchemaHeader(key, val)
		}

		if spec, ok := c.serverOpts.customSpecs.Find(string(val.format)); ok {
			sv := specHeaderValue(hf, val)
			// A nil pointer has no value to encode, the same as a missing field.
			if !sv.IsValid() {
				if val.required && !c.headerAlreadyWritten(key) {
					if err := runValidation(nil, ResponseErr, schemaHeaders, key, val.rules); err != nil {
						return err
					}
				}
				continue
			}

			v, err := spec.Encode(sv.Interface())
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

				if err := c.setSchemaHeader(key, fmt.Sprintf("%v", hv)); err != nil {
					return err
				}

			case val.format == utils.TimeObjectFormat:
				var tv time.Time
				isNil := false
				switch v := hv.(type) {
				case time.Time:
					tv = v
				case *time.Time:
					if isNil = v == nil; !isNil {
						tv = *v
					}
				default:
					return newErrReport(ResponseErr, schemaHeaders, key, "parser", errors.New("unable to parse header"))
				}

				if def, ok := val.defVal.(string); ok && tv.IsZero() {
					// Registration checked that the default parses.
					if tv, _ = time.Parse(val.pattern, def); !tv.IsZero() {
						isNil = false
					}
				}

				if tv.IsZero() && c.headerAlreadyWritten(key) {
					continue
				}

				// A nil pointer writes nothing, and only the presence rules apply.
				if isNil {
					if val.required {
						if err := runValidation(nil, ResponseErr, schemaHeaders, key, val.rules); err != nil {
							return err
						}
					}
					continue
				}

				if err := runValidation(tv, ResponseErr, schemaHeaders, key, val.rules); err != nil {
					return err
				}

				if err := c.setSchemaHeader(key, tv.Format(val.pattern)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// setSchemaHeader writes a schema-defined response header, rejecting names
// and values that contain CR, LF or NUL.
func (c *context) setSchemaHeader(key, val string) error {
	if !validHeaderField(key) || !validHeaderField(val) {
		return newErrReport(ResponseErr, schemaHeaders, key, "header", errors.New("header contains CR, LF or NUL"))
	}
	c.fctx.Response.Header.Set(key, val)
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
	for key := range c.fctx.Response.Header.Cookies() {
		if string(key) == name {
			found = true
			break
		}
	}
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
		// A pointer writes the value it points to, and a nil one writes nothing.
		if cf.Kind() == reflect.Pointer && utils.IsPrimitiveKind(val.kind) {
			if cf.IsNil() {
				continue
			}
			cv = cf.Elem().Interface()
		}
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

// specHeaderValue returns what a custom spec encodes for a response header field:
// the field or its default if it is empty, with a pointer replaced by the value it
// points to. It returns an invalid Value for a nil pointer.
func specHeaderValue(field reflect.Value, rule *RuleDef) reflect.Value {
	if rule.defStr != "" && isEmptyJSONValue(field) {
		if def := rule.defaultValue(field.Type()); def.IsValid() {
			field = def
		}
	}
	for field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return reflect.Value{}
		}
		field = field.Elem()
	}
	return field
}
