package gofi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/utils"
	"github.com/valyala/fastjson"
)

type Context interface {
	// Returns the http writer instance for the request
	Writer() http.ResponseWriter
	// Returns the http request instance for the request
	Request() *http.Request
	// Access global store defined on the server router instance
	GlobalStore() ReadOnlyStore
	// Access route context data store. Useful for passing and retrieving during a request lifetime
	DataStore() GofiStore
	// Access static meta information defined on the route
	Meta() ContextMeta
	// Sends a JSON response with status code.
	JSON(code int, i any) error
	// Sends a schema response object for the given status code
	Send(code int, obj any) error
}

type context struct {
	w           http.ResponseWriter
	r           *http.Request
	rules       *schemaRules
	routeMeta   metaMap
	globalStore ReadOnlyStore
	dataStore   GofiStore
	serverOpts  *muxOptions
}

func newContext(w http.ResponseWriter, r *http.Request) *context {
	return &context{
		w:           w,
		r:           r,
		routeMeta:   map[string]map[string]any{},
		globalStore: NewGlobalStore(),
		dataStore:   NewDataStore(),
		serverOpts:  defaultMuxOptions(),
	}
}

func (c *context) Writer() http.ResponseWriter {
	return c.w
}

func (c *context) Request() *http.Request {
	return c.r
}

func (c *context) GlobalStore() ReadOnlyStore {
	return c.globalStore
}

func (c *context) DataStore() GofiStore {
	return c.dataStore
}

func (c *context) Meta() ContextMeta {
	return &contextMeta{c: c}
}

func (c *context) JSON(code int, resp any) error {
	c.w.Header().Set("content-type", "application/json; charset=utf-8")
	_, rules, err := c.rules.getRespRulesByCode(code)
	if err != nil {
		return err
	}

	if len(rules) == 0 {
		return nil
	}

	if resp == nil {
		return fmt.Errorf("bad response. response value should not be empty")
	}

	rv := reflect.ValueOf(resp)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return errors.New("bad response. invalid response type passed")
	}

	err = c.checkAndSetHeaders(rules, rv.FieldByName(string(schemaHeaders)), func(s string) bool {
		return s == "content-type" || s == "-"
	})
	if err != nil {
		return err
	}

	err = c.checkAndBuildJson(code, rules, rv.FieldByName(string(schemaBody)))
	if err != nil {
		return err
	}

	return nil
}

func (c *context) setContextSettings(rules *schemaRules, routeMeta metaMap, globalStore GofiStore, serverOpts *muxOptions) {
	c.rules = rules
	c.routeMeta = routeMeta
	c.globalStore = globalStore
	c.serverOpts = serverOpts
}

func old__validateAndOrBindRequest[T any](c *context, shouldBind bool) (*T, error) {
	var schemaPtr *T
	if shouldBind {
		schemaPtr = new(T)
	}

	if c.rules == nil || len(c.rules.req) == 0 {
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

	validateStrAndBind := func(field schemaField, qv string, def *ruleDef) error {
		if qv == "" && def.defStr != "" {
			qv = def.defStr
		}

		if !def.required && qv == "" {
			return nil
		}

		val, err := utils.PrimitiveFromStr(def.kind, qv)
		if err != nil || utils.NotPrimitive(val) {
			// Handle special cases.
			switch def.format {
			case utils.TimeObjectFormat:
				val, err = time.Parse(def.pattern, qv)
				if err != nil {
					return newErrReport(RequestErr, field, def.field, "typeCast", err)
				}
			default:
				if ctype, ok := c.serverOpts.customSchema[string(def.format)]; ok {
					val, err = ctype.CustomEncode(qv)
					if err != nil {
						return newErrReport(RequestErr, field, def.field, "typeCast", err)
					}
				} else {
					return newErrReport(RequestErr, field, def.field, "typeCast", err)
				}
			}
		}

		errs := make([]error, 0, len(def.rules))
		for _, l := range def.rules {
			if err := l.dator(val); err != nil {
				errs = append(errs, newErrReport(RequestErr, schemaQuery, def.field, l.rule, err))
			}
		}

		if len(errs) == 0 && shouldBind {
			sf := reqStruct.FieldByName(string(field)).FieldByName(def.name)
			sf.Set(reflect.ValueOf(val).Convert(sf.Type()))
		}

		return errors.Join(errs...)
	}

	// Handle Headers
	pdef := c.rules.getReqRules(schemaHeaders)
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
	pdef = c.rules.getReqRules(schemaQuery)
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
	pdef = c.rules.getReqRules(schemaPath)
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
	pdef = c.rules.getReqRules(schemaCookies)
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
			verrs := make([]error, 0, len(def.rules))
			for _, l := range def.rules {
				if err := l.dator(cv.Value); err != nil {
					verrs = append(verrs, newErrReport(RequestErr, schemaQuery, def.field, l.rule, err))
				}
			}
			err = errors.Join(verrs...)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if shouldBind {
				sf := reqStruct.FieldByName(string(schemaCookies)).FieldByName(def.name)
				sf.Set(reflect.ValueOf(cv).Convert(sf.Type()))
			}

		default:
			cvs, err := utils.PrimitiveFromStr(def.kind, cv.Value)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			verrs := make([]error, 0, len(def.rules))
			for _, l := range def.rules {
				if err := l.dator(cvs); err != nil {
					verrs = append(verrs, newErrReport(RequestErr, schemaQuery, def.field, l.rule, err))
				}
			}
			err = errors.Join(verrs...)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if shouldBind {
				sf := reqStruct.FieldByName(string(schemaCookies)).FieldByName(def.name)
				sf.Set(reflect.ValueOf(cvs).Convert(sf.Type()))
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Handle Body
	pdef = c.rules.getReqRules(schemaBody)
	if pdef == nil || pdef.kind == reflect.Invalid {
		return schemaPtr, errors.Join(errs...)
	}

	body := c.r.Body
	if body == nil && pdef.required {
		return schemaPtr, newErrReport(RequestErr, schemaBody, "", "required", errors.New("request body is required"))
	} else if body == nil {
		return schemaPtr, nil
	}

	contentType := c.rules.reqContent()
	switch contentType {
	case cont.ApplicationJson:
		err := validateAndOrBindJson(c, pdef, body, shouldBind, &reqStruct, schemaPtr)
		if err != nil {
			return schemaPtr, err
		}
	}

	return schemaPtr, errors.Join(errs...)
}

type walkFinishStatus struct{}

var walkFinished = walkFinishStatus{}

const DEFAULT_ARRAY_SIZE = 50

func validateAndOrBindJson(c *context, pdef *ruleDef, body io.ReadCloser, shouldBind bool, reqStruct *reflect.Value, ptr any) error {

	body = http.MaxBytesReader(c.w, body, 1048576)
	bs, err := io.ReadAll(body)
	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "reader", err)
	} else if len(bs) == 0 && pdef.required {
		return newErrReport(RequestErr, schemaBody, "", "required", errors.New("request body is required"))
	} else if len(bs) == 0 {
		return nil
	}

	val, err := utils.PrimitiveFromStr(pdef.kind, string(bs))
	if utils.IsPrimitive(val) {
		if err != nil {
			return newErrReport(RequestErr, schemaBody, "", "typeCast", err)
		}

		if err := runValidation(RequestErr, val, schemaBody, "", pdef.rules); err != nil {
			return err
		} else {
			if shouldBind {
				sf := reqStruct.FieldByName(string(schemaBody))
				switch sf.Kind() {
				case reflect.Pointer:
					sfp := reflect.New(sf.Type().Elem())
					sfp.Elem().Set(reflect.ValueOf(val).Convert(sf.Type().Elem()))
					sf.Set(sfp)
				default:
					sf.Set(reflect.ValueOf(val).Convert(sf.Type()))
				}
			}

			errs := make([]error, 0, len(pdef.rules))
			for _, rule := range pdef.rules {
				err := rule.dator(val)
				if err != nil {
					errs = append(errs, newErrReport(RequestErr, schemaBody, "", rule.rule, err))
				}
			}

			if len(errs) > 0 {
				return errors.Join(errs...)
			}

			return nil
		}
	}

	// var jsonparser fastjson.Parser
	pv, err := cont.PoolJsonParse(bs)
	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "parser", err)
	}

	// We we will walk through each entry in the array and append any validation error to the errs state object
	// If the no errors encountered surpass the error capacity, we bail out and stop parsing the JSON string
	// errCap := 30
	// TODO: remove schemaPtr from here. Just using this for easier debugging for now.
	s := jsonContentState{pv: pv, schema: schemaBody, shouldBind: shouldBind, ptr: ptr}
	var bstrct reflect.Value
	if shouldBind {
		bstrct = reqStruct.FieldByName(string(schemaBody))
	}
	rtn, err := walkJsonContent(&s, &bstrct, []string{}, pdef)
	if err != nil {
		return err
	} else if pdef.required && !s.hasVal {
		return newErrReport(RequestErr, schemaBody, "", "required", errors.New("request body is required"))
	}

	switch rtn {
	case walkFinished:
		return nil
	default:
		return newErrReport(RequestErr, schemaBody, "", "parser", errors.New("couldn't parse request body"))
	}
}

func walkJsonContent(s *jsonContentState, strct *reflect.Value, keys []string, def *ruleDef) (any, error) {
	kp := strings.Join(keys, ".")
	val, err := s.pv.GetByKind(def.kind, def.format, keys...)
	if val == nil && def.defVal != nil {
		val = def.defVal
	}

	if err != nil {
		return nil, newErrReport(RequestErr, s.schema, kp, "parser", err)
	} else if val == cont.EOF {
		val = nil
	}

	if !def.required && val == nil {
		return nil, nil
	}

	if def.kind == reflect.Struct {
		switch def.format {
		case utils.TimeObjectFormat:
			if v, ok := val.(string); ok {
				val, err = time.Parse(def.pattern, v)
				if err != nil {
					return nil, newErrReport(RequestErr, s.schema, kp, "typeCast", err)
				}
			} else {
				return nil, newErrReport(RequestErr, s.schema, kp, "typeCast", errors.New("invalid time value"))
			}

		default:
			if s.shouldBind && strct.Kind() == reflect.Pointer {
				strct.Set(reflect.New(strct.Type().Elem()))
			}

			for ckey, cdef := range def.properties {
				var cstrct reflect.Value
				if s.shouldBind {
					switch strct.Kind() {
					case reflect.Pointer:
						cstrct = strct.Elem().FieldByName(cdef.name)
					default:
						cstrct = strct.FieldByName(cdef.name)
					}
				}

				_, err := walkJsonContent(s, &cstrct, append(keys, ckey), cdef)
				if err != nil {
					return nil, err
				}
			}

			return walkFinished, nil

		}
	}

	if def.kind == reflect.Map {
		if def.additionalProperties == nil && def.required {
			return nil, newErrReport(RequestErr, schemaBody, kp, "required", errors.New("map value is required"))
		} else if def.additionalProperties == nil {
			return walkFinished, nil
		}

		obj, err := s.pv.GetRawObject(keys)
		if err != nil {
			return nil, newErrReport(RequestErr, s.schema, kp, "parser", err)
		}

		if s.shouldBind {
			strct.Set(reflect.MakeMap(strct.Type()))
		}

		var mapErr error
		obj.Visit(func(key []byte, v *fastjson.Value) {
			var cstrct reflect.Value
			if s.shouldBind {
				cstrct = reflect.New(strct.Type().Elem()).Elem()
			}

			ckey := string(key)
			_, err := walkJsonContent(s, &cstrct, append(keys, ckey), def.additionalProperties)
			if err != nil {
				mapErr = err
				return
			}

			if s.shouldBind {
				strct.SetMapIndex(reflect.ValueOf(ckey), cstrct)
			}

		})

		if mapErr != nil {
			return nil, mapErr
		}

		return walkFinished, nil
	}

	if def.kind == reflect.Slice || def.kind == reflect.Array {
		if def.item == nil {
			return walkFinished, nil
		}

		var size = DEFAULT_ARRAY_SIZE
		if def.max != nil {
			size = int(*def.max)
		}

		switch def.item.kind {
		case reflect.String,
			reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:

			arr, err := s.pv.GetPrimitiveArrVals(def.item.kind, def.format, keys, size)
			if def.max != nil && len(arr) > int(*def.max) {
				return nil, newErrReport(RequestErr, s.schema, kp, "max", errors.New("array size too large"))
			} else if err != nil {
				return nil, newErrReport(RequestErr, s.schema, kp, "parser", err)
			}

			// Treat primitive array values like primitive values
			val = arr

		default:
			i := 0

			var nslice reflect.Value
			if s.shouldBind {
				nslice = reflect.MakeSlice(strct.Type(), 0, size)
			}

			for {
				if !s.pv.Exist(append(keys, fmt.Sprintf("%d", i))...) {
					break
				} else if def.max != nil && i > int(*def.max) {
					return nil, newErrReport(RequestErr, s.schema, kp, "max", errors.New("array size too large"))
				}

				var istrct reflect.Value
				if s.shouldBind {
					istrct = reflect.New(strct.Type().Elem()).Elem()
				}

				_, err := walkJsonContent(s, &istrct, append(keys, fmt.Sprintf("%d", i)), def.item)
				if err != nil {
					return nil, err
				}

				if s.shouldBind {
					nslice = reflect.Append(nslice, istrct)
				}

				i++
			}

			// if len(vals.items) == 0 {
			// 	if def.required {
			// 		if len(s.errs) < s.errCap {
			// 			s.errs = append(s.errs, newErrReport(requestErr, s.schema, kp, "required", errors.New("array item is required")))
			// 		}
			// 		return nil
			// 	}
			// } else {
			// 	return &vals
			// }

			if s.shouldBind {
				strct.Set(nslice)
			}

			return walkFinished, nil
		}
	}

	errs := make([]error, 0, len(def.rules))
	for _, rule := range def.rules {
		err := rule.dator(val)
		if err != nil {
			errs = append(errs, newErrReport(RequestErr, s.schema, kp, rule.rule, err))
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	if s.shouldBind {
		bindValOnElem(strct, val)
	}

	s.hasVal = true
	return walkFinished, nil
}

func bindValOnElem(strct *reflect.Value, val any) {
	if val == nil {
		return
	}

	switch strct.Kind() {
	case reflect.Pointer:
		if v, ok := val.([]any); ok {
			nslice := reflect.New(strct.Type().Elem())
			istrct := strct.Type().Elem().Elem()
			slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
			for _, item := range v {
				ssf := reflect.New(istrct).Elem()
				bindValOnElem(&ssf, item)
				slice = reflect.Append(slice, ssf)
			}
			nslice.Elem().Set(slice)
			strct.Set(nslice)
		}

	case reflect.Slice, reflect.Array:
		if v, ok := val.([]any); ok {
			istrct := strct.Type().Elem()
			switch istrct.Kind() {
			case reflect.Pointer:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct.Elem()).Elem()
					bindValOnElem(&ssf, item)
					slice = reflect.Append(slice, ssf.Addr())
				}
				strct.Set(slice)

			default:
				slice := reflect.MakeSlice(reflect.SliceOf(istrct), 0, len(v))
				for _, item := range v {
					ssf := reflect.New(istrct).Elem()
					bindValOnElem(&ssf, item)
					slice = reflect.Append(slice, ssf)
				}
				strct.Set(slice)
			}
		}

	default:
		strct.Set(reflect.ValueOf(val).Convert(strct.Type()))
	}
}

type jsonContentState struct {
	pv         *cont.ParsedJson
	schema     schemaField
	shouldBind bool
	ptr        any
	hasVal     bool
}
