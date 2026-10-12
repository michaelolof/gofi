package gofi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/michaelolof/gofi/cont"
	"github.com/michaelolof/gofi/utils"
	"github.com/valyala/fastjson"
)

type JSONBodyParser struct {
	MaxRequestSize int64
	MaxDepth       int
	MaintainOrder  bool
}

// jsonRequestOptions is the value-based walker state used by the JSON parser.
// RequestOptions keeps its pointer-based Body field for compatibility with
// custom body parsers, but the recursive JSON hot path must not take the
// address of a new reflect.Value for every visited field.
type jsonRequestOptions struct {
	SchemaRules *RuleDef
	ShouldBind  bool
	Context     ParserContext
	Body        reflect.Value
	HasBody     bool
}

type requestPathSegment struct {
	name    string
	index   int
	isIndex bool
}

// requestPath is shared by every recursive walker frame. Successful requests
// only push and pop segments; a string is materialized when an error is built.
type requestPath struct {
	segments []requestPathSegment
}

func (p *requestPath) pushField(name string) {
	p.segments = append(p.segments, requestPathSegment{name: name})
}

func (p *requestPath) pushIndex(index int) {
	p.segments = append(p.segments, requestPathSegment{index: index, isIndex: true})
}

func (p *requestPath) pop() {
	last := len(p.segments) - 1
	p.segments[last] = requestPathSegment{}
	p.segments = p.segments[:last]
}

func (p *requestPath) String() string {
	if p == nil || len(p.segments) == 0 {
		return ""
	}

	var builder strings.Builder
	for i, segment := range p.segments {
		if i > 0 {
			builder.WriteByte('.')
		}
		if segment.isIndex {
			builder.WriteString(strconv.Itoa(segment.index))
		} else {
			builder.WriteString(segment.name)
		}
	}
	return builder.String()
}

var jsonBodyPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func (j *JSONBodyParser) Match(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func (j *JSONBodyParser) ValidateAndDecodeRequest(body io.ReadCloser, opts RequestOptions) error {
	bsMax := j.MaxRequestSize
	if bsMax == 0 {
		bsMax = 1048576 // defaultReqSize
	}

	// Limit body size without depending on net/http
	limitedBody := io.NopCloser(io.LimitReader(body, bsMax))
	body = limitedBody

	buf := jsonBodyPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer jsonBodyPool.Put(buf)

	_, err := buf.ReadFrom(body)
	bs := buf.Bytes()

	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "reader", err)
	} else if len(bs) == 0 && opts.SchemaRules != nil && (opts.SchemaRules.required || opts.SchemaRules.present) {
		return newErrReport(RequestErr, schemaBody, "", "required", errors.New("request body is required"))
	} else if len(bs) == 0 {
		return nil
	}

	// Use empty rules if nil (shouldn't happen if properly initialized)
	if opts.SchemaRules == nil {
		// fallback or error? Assuming initialized.
		return errors.New("SchemaRules is nil")
	}

	// The compiled schema already identifies primitive bodies. Structured JSON
	// must not stringify the entire payload only to rediscover its schema kind.
	if utils.IsPrimitiveKind(opts.SchemaRules.kind) {
		val, err := utils.PrimitiveFromStr(opts.SchemaRules.kind, string(bs))
		if err != nil {
			return newErrReport(RequestErr, schemaBody, "", "encoder", err)
		}

		if err := runValidation(val, RequestErr, schemaBody, "", opts.SchemaRules.rules); err != nil {
			return err
		}

		if opts.ShouldBind && opts.Body != nil {
			if len(opts.SchemaRules.accessor.index) > 1 && !requestFieldParentsReady(*opts.Body, opts.SchemaRules.accessor.index) {
				return newErrReport(RequestErr, schemaBody, "", "binding", fmt.Errorf("field index %v traverses a nil or invalid parent", opts.SchemaRules.accessor.index))
			}
			sf := requestCompiledField(*opts.Body, opts.SchemaRules.accessor.index)
			switch sf.Kind() {
			case reflect.Pointer:
				sfp := reflect.New(sf.Type().Elem())
				sfp.Elem().Set(reflect.ValueOf(val).Convert(sf.Type().Elem()))
				sf.Set(sfp)
			default:
				sf.Set(reflect.ValueOf(val).Convert(sf.Type()))
			}
		}

		return nil
	}

	// Handle non primitives with FastJSON
	p := opts.Context.getParser()

	pv, err := p.ParseBytes(bs)
	if err != nil {
		return newErrReport(RequestErr, schemaBody, "", "parser", err)
	}

	walkOpts := jsonRequestOptions{
		SchemaRules: opts.SchemaRules,
		ShouldBind:  opts.ShouldBind,
		Context:     opts.Context,
	}
	if opts.ShouldBind && opts.Body != nil {
		if len(opts.SchemaRules.accessor.index) > 1 && !requestFieldParentsReady(*opts.Body, opts.SchemaRules.accessor.index) {
			return newErrReport(RequestErr, schemaBody, "", "binding", fmt.Errorf("field index %v traverses a nil or invalid parent", opts.SchemaRules.accessor.index))
		}
		walkOpts.Body = requestCompiledField(*opts.Body, opts.SchemaRules.accessor.index)
		walkOpts.HasBody = true
	}

	// Schemas identified as a common, safe shape at registration time can bind
	// directly from fastjson nodes without the generic walker's interface
	// conversions and per-node feature checks. A mismatch falls back to the
	// generic walker so supported values and errors remain unchanged.
	jsonBindMaxDepth := opts.SchemaRules.jsonBindMaxDepth
	if walkOpts.ShouldBind && walkOpts.HasBody && jsonBindMaxDepth > 0 {
		maxDepth := j.MaxDepth
		if maxDepth == 0 {
			maxDepth = 100
		}
		if int(jsonBindMaxDepth) <= maxDepth {
			if bindCompiledJSONNode(opts.SchemaRules, pv, walkOpts.Body) {
				return nil
			}
			walkOpts.Body.SetZero()
		}
	}

	var path requestPath
	status, err := j.walkStruct(pv, schemaBody, walkOpts, &path)
	if err != nil {
		return err
	}

	if status != nil && *status == walkFinished {
		return nil
	} else {
		return newErrReport(RequestErr, schemaBody, "", "parser", errors.New("couldn't parse request body"))
	}
}

func (j *JSONBodyParser) ValidateAndEncodeResponse(obj any, opts ResponseOptions) ([]byte, error) {
	body := opts.Body
	if body.Kind() == reflect.Pointer {
		body = body.Elem()
	}

	if opts.SchemaRules != nil && (opts.SchemaRules.required || opts.SchemaRules.present) && !body.IsValid() {
		return nil, newErrReport(ResponseErr, schemaBody, "", "required", errors.New("value is required for body"))
	}

	if opts.SchemaRules != nil && opts.SchemaRules.kind != body.Kind() {
		return nil, newErrReport(ResponseErr, schemaBody, "", "typeMismatch", errors.New("body schema and payload mismatch"))
	}

	buf := jsonBodyPool.Get().(*bytes.Buffer)
	buf.Reset()
	if err := j.encodeFieldValue(opts.Context, buf, opts.Body, opts.SchemaRules, nil); err != nil {
		jsonBodyPool.Put(buf)
		return nil, newErrReport(ResponseErr, schemaBody, "", "encoder", err)
	}

	// Copy result before returning buffer to pool
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	jsonBodyPool.Put(buf)
	return result, nil
}

func (j *JSONBodyParser) walkStruct(node *fastjson.Value, schemaField schemaField, opts jsonRequestOptions, path *requestPath) (*walkFinishStatus, error) {
	maxDepth := j.MaxDepth
	if maxDepth == 0 {
		maxDepth = 100
	}
	if len(path.segments) > maxDepth {
		return nil, newErrReport(RequestErr, schemaField, path.String(), "depth", errors.New("max recursion depth exceeded"))
	}

	val, err := cont.GetNodeByKind(node, opts.SchemaRules.kind, opts.SchemaRules.format)
	if err != nil {
		return nil, newErrReport(RequestErr, schemaField, path.String(), "parser", err)
	}

	if (val == nil || val == cont.EOF) && opts.SchemaRules.defVal != nil {
		val = opts.SchemaRules.defVal
	} else if val == cont.EOF {
		val = nil
	}

	if !opts.SchemaRules.required && !opts.SchemaRules.present && val == nil {
		return nil, nil
	}

	// Missing required or present values must fail before walking their shape.
	// In particular, calling Array, Object, or Get on a nil fastjson node can
	// panic. Only presence rules are meaningful for a missing value; running
	// length rules such as min/max against nil would panic in the validator.
	if val == nil {
		for _, rule := range opts.SchemaRules.rules {
			if rule.rule != "required" && rule.rule != "present" {
				continue
			}
			if err := rule.dator(nil); err != nil {
				return nil, newErrReport(RequestErr, schemaField, path.String(), rule.rule, err)
			}
		}
		return &walkFinished, nil
	}

	if opts.ShouldBind && opts.HasBody && opts.Body.Kind() == reflect.Pointer {
		opts.Body.Set(reflect.New(opts.Body.Type().Elem()))
	}

	if opts.SchemaRules.format == utils.TimeObjectFormat || opts.SchemaRules.format == utils.CookieObjectFormat {
		if err := runValidationLazy(val, RequestErr, schemaField, path, opts.SchemaRules.rules); err != nil {
			return nil, err
		}

		if opts.ShouldBind && opts.HasBody {
			if err = j.decodeFieldValue(opts.Body, val, opts.SchemaRules.pattern); err != nil {
				return nil, newErrReport(RequestErr, schemaField, path.String(), "decode", err)
			}
		}

		return &walkFinished, nil
	}

	// Check for custom spec types on body fields (e.g., struct fields tagged with spec:"custom")
	if spec, ok := opts.Context.CustomSpecs().Find(string(opts.SchemaRules.format)); ok {
		decoded, err := spec.Decode(val)
		if err != nil {
			return nil, newErrReport(RequestErr, schemaField, path.String(), "typeCast", err)
		}

		if err := runValidationLazy(decoded, RequestErr, schemaField, path, opts.SchemaRules.rules); err != nil {
			return nil, err
		}

		if opts.ShouldBind && opts.HasBody {
			if err = j.decodeFieldValue(opts.Body, decoded, ""); err != nil {
				return nil, newErrReport(RequestErr, schemaField, path.String(), "decode", err)
			}
		}

		return &walkFinished, nil
	}

	// json.RawMessage / json.Marshaler → verbatim JSON binding
	if opts.SchemaRules.format == utils.RawJSONFormat {
		raw, ok := val.([]byte)
		if !ok {
			return nil, newErrReport(RequestErr, schemaField, path.String(), "parser", errors.New("expected raw JSON bytes"))
		}

		if err := runValidationLazy(raw, RequestErr, schemaField, path, opts.SchemaRules.rules); err != nil {
			return nil, err
		}

		if opts.ShouldBind && opts.HasBody {
			if err := j.decodeRawJSONField(opts.Body, raw); err != nil {
				return nil, newErrReport(RequestErr, schemaField, path.String(), "decode", err)
			}
		}

		return &walkFinished, nil
	}

	// []byte (no custom marshaler) → base64 string binding
	if opts.SchemaRules.format == utils.ByteFormat {
		raw, ok := val.([]byte)
		if !ok {
			return nil, newErrReport(RequestErr, schemaField, path.String(), "parser", errors.New("expected raw JSON bytes"))
		}

		decoded, err := decodeBase64Field(raw)
		if err != nil {
			return nil, newErrReport(RequestErr, schemaField, path.String(), "decode", err)
		}

		if err := runValidationLazy(decoded, RequestErr, schemaField, path, opts.SchemaRules.rules); err != nil {
			return nil, err
		}

		if opts.ShouldBind && opts.HasBody {
			if opts.Body.Kind() == reflect.Pointer {
				ptr := reflect.New(opts.Body.Type().Elem())
				ptr.Elem().Set(reflect.ValueOf(decoded))
				opts.Body.Set(ptr)
			} else {
				opts.Body.Set(reflect.ValueOf(decoded))
			}
		}

		return &walkFinished, nil
	}

	switch opts.SchemaRules.kind {
	case reflect.Struct:
		// Fast path: use pre-ordered props slice when available
		if len(opts.SchemaRules.orderedProps) > 0 {
			for _, childDef := range opts.SchemaRules.orderedProps {
				path.pushField(childDef.field)
				childNode := node.Get(childDef.field)
				var childStruct reflect.Value
				if opts.ShouldBind && opts.HasBody {
					indexNeedsValue := len(childDef.accessor.index) > 1
					if !indexNeedsValue || childNode != nil || childDef.defVal != nil {
						if indexNeedsValue && !requestFieldParentsReady(opts.Body, childDef.accessor.index) {
							errPath := path.String()
							path.pop()
							return nil, newErrReport(RequestErr, schemaField, errPath, "binding", fmt.Errorf("field index %v traverses a nil or invalid parent", childDef.accessor.index))
						}
						childStruct = requestCompiledField(opts.Body, childDef.accessor.index)
					}
				}
				childOpts := j.getFieldOptions(opts, childStruct, childDef)
				_, err := j.walkStruct(childNode, schemaBody, childOpts, path)
				path.pop()
				if err != nil {
					return nil, err
				}
			}
		} else if j.MaintainOrder && opts.SchemaRules.typ != nil {
			for _, field := range reflect.VisibleFields(opts.SchemaRules.typ) {
				if isPromotedEmbed(field) {
					continue
				}
				jsonTags := strings.Split(field.Tag.Get("json"), ",")
				var name string
				if len(jsonTags) > 0 && jsonTags[0] != "" {
					name = jsonTags[0]
				} else {
					name = field.Name
				}
				childDef, ok := opts.SchemaRules.properties[name]
				if !ok {
					continue
				}
				path.pushField(name)
				childNode := node.Get(name)
				var childStruct reflect.Value
				if opts.ShouldBind && opts.HasBody {
					indexNeedsValue := len(childDef.accessor.index) > 1
					if !indexNeedsValue || childNode != nil || childDef.defVal != nil {
						if indexNeedsValue && !requestFieldParentsReady(opts.Body, childDef.accessor.index) {
							errPath := path.String()
							path.pop()
							return nil, newErrReport(RequestErr, schemaField, errPath, "binding", fmt.Errorf("field index %v traverses a nil or invalid parent", childDef.accessor.index))
						}
						childStruct = requestCompiledField(opts.Body, childDef.accessor.index)
					}
				}
				childOpts := j.getFieldOptions(opts, childStruct, childDef)
				_, err := j.walkStruct(childNode, schemaBody, childOpts, path)
				path.pop()
				if err != nil {
					return nil, err
				}
			}
		} else {
			for childKey, childDef := range opts.SchemaRules.properties {
				path.pushField(childKey)
				childNode := node.Get(childKey)
				var childStruct reflect.Value
				if opts.ShouldBind && opts.HasBody {
					indexNeedsValue := len(childDef.accessor.index) > 1
					if !indexNeedsValue || childNode != nil || childDef.defVal != nil {
						if indexNeedsValue && !requestFieldParentsReady(opts.Body, childDef.accessor.index) {
							errPath := path.String()
							path.pop()
							return nil, newErrReport(RequestErr, schemaField, errPath, "binding", fmt.Errorf("field index %v traverses a nil or invalid parent", childDef.accessor.index))
						}
						childStruct = requestCompiledField(opts.Body, childDef.accessor.index)
					}
				}
				childOpts := j.getFieldOptions(opts, childStruct, childDef)
				_, err := j.walkStruct(childNode, schemaBody, childOpts, path)
				path.pop()
				if err != nil {
					return nil, err
				}
			}
		}

		return &walkFinished, nil

	case reflect.Map:
		if opts.SchemaRules.additionalProperties == nil {
			return &walkFinished, nil
		}

		obj, err := node.Object()
		if err != nil {
			return nil, newErrReport(RequestErr, schemaField, path.String(), "parser", err)
		}

		if opts.ShouldBind && opts.HasBody {
			opts.Body.Set(reflect.MakeMap(opts.Body.Type()))
		}

		var mapErr error
		obj.Visit(func(key []byte, v *fastjson.Value) {
			var cstrct reflect.Value
			if opts.ShouldBind && opts.HasBody {
				cstrct = reflect.New(opts.Body.Type().Elem()).Elem()
			}

			ckey := string(key)
			path.pushField(ckey)
			copts := j.getFieldOptions(opts, cstrct, opts.SchemaRules.additionalProperties)
			_, err := j.walkStruct(v, schemaBody, copts, path)
			path.pop()
			if err != nil {
				mapErr = err
				return
			}

			if opts.ShouldBind && opts.HasBody {
				opts.Body.SetMapIndex(reflect.ValueOf(ckey), cstrct)
			}

		})

		if mapErr != nil {
			return nil, mapErr
		}

		return &walkFinished, nil

	case reflect.Slice, reflect.Array:
		rules := opts.SchemaRules

		switch true {
		case utils.IsPrimitiveKind(opts.SchemaRules.item.kind):
			// Handle array of primitive values
			arrNodes, err := node.Array()
			if err != nil {
				return nil, newErrReport(RequestErr, schemaField, path.String(), "parser", err)
			}
			arr, err := cont.GetPrimitiveArrValsFromNode(arrNodes, rules.item.kind, rules.format, len(arrNodes))
			if err != nil {
				return nil, newErrReport(RequestErr, schemaField, path.String(), "parser", err)
			}
			if err := runValidationLazy(arr, RequestErr, schemaField, path, opts.SchemaRules.rules); err != nil {
				return nil, err
			}

			if opts.ShouldBind && opts.HasBody {
				if err = j.decodeFieldValue(opts.Body, arr, ""); err != nil {
					return nil, newErrReport(RequestErr, schemaField, path.String(), "decode", err)
				}
			}

			return &walkFinished, nil

		case utils.NotPrimitiveKind(opts.SchemaRules.item.kind):
			// Handle array of Non primitives
			arrNodes, err := node.Array()
			if err != nil {
				return nil, newErrReport(RequestErr, schemaField, path.String(), "parser", err)
			}

			if len(arrNodes) == 0 && rules.required && !rules.present {
				path.pushIndex(0)
				errPath := path.String()
				path.pop()
				return nil, newErrReport(RequestErr, schemaField, errPath, "required", errors.New("value must not be empty"))
			}
			// Reject oversized input before reserving destination memory. The
			// limit is compiled once with the schema, so successful requests do
			// not pay for a second validator call or a temporary destination.
			if rules.max != nil && float64(len(arrNodes)) > *rules.max {
				for _, rule := range rules.rules {
					if rule.rule == "max" || rule.rule == "lte" {
						return nil, newErrReport(RequestErr, schemaField, path.String(), rule.rule, fmt.Errorf("value must be equal or lesser than maximum limit of %f", *rules.max))
					}
				}
			}

			var collection reflect.Value
			var collectionType reflect.Type
			if opts.ShouldBind && opts.HasBody {
				collectionType = opts.Body.Type()
				if collectionType.Kind() == reflect.Pointer {
					collectionType = collectionType.Elem()
				}
				if collectionType.Kind() == reflect.Array {
					collection = reflect.New(collectionType).Elem()
				} else {
					collection = reflect.MakeSlice(collectionType, len(arrNodes), len(arrNodes))
				}
			}

			for i, childNode := range arrNodes {
				var istrct reflect.Value
				if opts.ShouldBind && opts.HasBody {
					if i < collection.Len() {
						istrct = collection.Index(i)
					} else {
						// Fixed arrays ignore excess decoded values, but retain the
						// previous conversion and validation behavior for those items.
						istrct = reflect.New(collectionType.Elem()).Elem()
					}
				}

				path.pushIndex(i)
				fopts := j.getFieldOptions(opts, istrct, rules.item)
				_, err := j.walkStruct(childNode, schemaBody, fopts, path)
				path.pop()
				if err != nil {
					return nil, err
				}
			}

			// Validate collection-level rules without allocating the destination
			// slice when this is a validation-only request. Validators for slice
			// and array rules inspect collection length/presence, both of which are
			// represented by arrNodes.
			var sliceVal any = arrNodes
			if opts.ShouldBind && opts.HasBody {
				sliceVal = collection.Interface()
			}
			if err := runValidationLazy(sliceVal, RequestErr, schemaField, path, opts.SchemaRules.rules); err != nil {
				return nil, err
			}

			if opts.ShouldBind && opts.HasBody {
				if opts.Body.Kind() == reflect.Pointer {
					opts.Body.Elem().Set(collection)
				} else {
					opts.Body.Set(collection)
				}
			}

			return &walkFinished, nil
		}

	case reflect.Interface:
		v, err := cont.GetAnyValueFromNode(node)
		if err != nil {
			return nil, newErrReport(RequestErr, schemaField, path.String(), "parser", err)
		}

		if err := runValidationLazy(v, RequestErr, schemaField, path, opts.SchemaRules.rules); err != nil {
			return nil, err
		}

		if opts.ShouldBind && opts.HasBody {
			if err = j.decodeFieldValue(opts.Body, v, ""); err != nil {
				return nil, newErrReport(RequestErr, schemaField, path.String(), "decode", err)
			}
		}

		return &walkFinished, nil

	default:
		if err := runValidationLazy(val, RequestErr, schemaField, path, opts.SchemaRules.rules); err != nil {
			return nil, err
		}

		if opts.ShouldBind && opts.HasBody {
			if err = j.decodeFieldValue(opts.Body, val, opts.SchemaRules.pattern); err != nil {
				return nil, newErrReport(RequestErr, schemaField, path.String(), "decode", err)
			}
		}

		return &walkFinished, nil
	}

	return &walkFinished, nil
}

func (j *JSONBodyParser) decodeFieldValue(field reflect.Value, val any, timeLayout string) error {
	if val == nil {
		return nil
	}

	// Fast path: if the value is raw JSON bytes and the field implements json.Unmarshaler,
	// delegate to it (covers json.RawMessage and custom marshaler types in collections).
	if raw, ok := val.([]byte); ok && field.CanAddr() {
		if u, ok2 := field.Addr().Interface().(json.Unmarshaler); ok2 {
			return u.UnmarshalJSON(raw)
		}
	}

	switch field.Kind() {
	case reflect.Pointer:
		// *time.Time → parse string into time using the provided layout
		if field.Type().Elem() == utils.TimeType {
			if s, ok := val.(string); ok {
				t, err := parseTime(s, timeLayout)
				if err != nil {
					return err
				}
				field.Set(reflect.ValueOf(&t))
				return nil
			}
		}
		switch field.Type().Elem().Kind() {
		case reflect.Slice, reflect.Array:
			if v, ok := val.([]any); ok {
				collection := reflect.New(field.Type().Elem())
				if err := bindCollectionValues(collection.Elem(), v); err != nil {
					return err
				}
				field.Set(collection)
				return nil
			} else {
				return fmt.Errorf("type mismatch. expected array value got %T", val)
			}
		default:
			ptype := reflect.New(field.Type().Elem())
			if err := setConvertedReflectValue(ptype.Elem(), val); err != nil {
				return err
			}
			field.Set(ptype)
			return nil
		}
	case reflect.Slice, reflect.Array:
		if v, ok := val.([]any); ok {
			return bindCollectionValues(field, v)
		} else {
			return fmt.Errorf("type mismatch. expected array value got %T", val)
		}
	default:
		if field.Type() == utils.TimeType {
			if s, ok := val.(string); ok {
				t, err := parseTime(s, timeLayout)
				if err != nil {
					return err
				}
				field.Set(reflect.ValueOf(t))
				return nil
			}
		}

		return setConvertedReflectValue(field, val)
	}
}

func bindCollectionValues(field reflect.Value, values []any) error {
	collection := field
	if field.Kind() == reflect.Slice {
		collection = reflect.MakeSlice(field.Type(), len(values), len(values))
	}

	for i, item := range values {
		var elem reflect.Value
		if i < collection.Len() {
			elem = collection.Index(i)
		} else {
			// Preserve fixed-array behavior: excess values are ignored after
			// undergoing the same conversion checks as in-range values.
			elem = reflect.New(collection.Type().Elem()).Elem()
		}

		if elem.Kind() == reflect.Pointer {
			value := reflect.New(elem.Type().Elem())
			if err := bindValOnElem(value.Elem(), item); err != nil {
				return err
			}
			if i < collection.Len() {
				elem.Set(value)
			}
		} else if err := bindValOnElem(elem, item); err != nil {
			return err
		}
	}

	if field.Kind() == reflect.Slice {
		field.Set(collection)
	}
	return nil
}

// decodeRawJSONField binds raw JSON bytes into a field, supporting json.RawMessage
// and other types implementing json.Unmarshaler.
func (j *JSONBodyParser) decodeRawJSONField(field reflect.Value, raw []byte) error {
	// Try json.Unmarshaler on the addressable value (covers non-pointer json.RawMessage)
	if field.CanAddr() {
		if u, ok := field.Addr().Interface().(json.Unmarshaler); ok {
			return u.UnmarshalJSON(raw)
		}
	}
	// For pointer fields that implement json.Unmarshaler (e.g. *json.RawMessage passed directly)
	if field.Kind() == reflect.Pointer {
		// Allocate if nil
		if field.IsNil() {
			elemType := field.Type().Elem()
			// Check if *Elem implements json.Unmarshaler
			if reflect.PointerTo(elemType).Implements(reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()) {
				field.Set(reflect.New(elemType))
			}
		}
		if !field.IsNil() {
			if u, ok := field.Interface().(json.Unmarshaler); ok {
				return u.UnmarshalJSON(raw)
			}
		}
	}
	// Fallback: assign raw bytes directly for []byte-compatible fields
	if field.Kind() == reflect.Slice && field.Type().Elem().Kind() == reflect.Uint8 {
		field.Set(reflect.ValueOf(raw))
		return nil
	}
	return fmt.Errorf("cannot bind raw JSON to field of type %s", field.Type())
}

// decodeBase64Field decodes a base64-encoded JSON string value into raw bytes.
func decodeBase64Field(raw []byte) ([]byte, error) {
	// raw is the JSON representation, e.g. "SGVsbG8=" (with quotes)
	s := string(raw)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	} else {
		return nil, fmt.Errorf("expected base64-encoded JSON string, got %s", s)
	}
	return base64.StdEncoding.DecodeString(s)
}

// parseTime parses a time string using the provided layout.
// When layout is non-empty it tries that first, then falls back to RFC3339 and RFC3339Nano.
// This mirrors the encode side which uses rules.pattern for time.Format.
func parseTime(s string, layout string) (time.Time, error) {
	if layout != "" {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// requestFieldByIndex resolves a compiled struct-field index without repeating
// a name lookup for every request. FieldByIndexErr safely traverses initialized
// embedded pointers and reports nil parents instead of panicking.
func requestFieldByIndex(parent reflect.Value, index []int) (reflect.Value, error) {
	if !parent.IsValid() {
		return reflect.Value{}, errors.New("cannot access a field on an invalid value")
	}
	if len(index) == 0 {
		return reflect.Value{}, errors.New("compiled field index is empty")
	}

	for parent.Kind() == reflect.Pointer {
		if parent.IsNil() {
			return reflect.Value{}, fmt.Errorf("field index %v traverses a nil parent", index)
		}
		parent = parent.Elem()
	}
	if parent.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("field index %v traverses non-struct %s", index, parent.Kind())
	}
	currentType := parent.Type()
	for position, fieldIndex := range index {
		for currentType.Kind() == reflect.Pointer {
			currentType = currentType.Elem()
		}
		if currentType.Kind() != reflect.Struct || fieldIndex < 0 || fieldIndex >= currentType.NumField() {
			return reflect.Value{}, fmt.Errorf("field index %v is invalid at position %d", index, position)
		}
		currentType = currentType.Field(fieldIndex).Type
	}

	current, err := parent.FieldByIndexErr(index)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("invalid compiled field index %v: %w", index, err)
	}

	if !current.CanSet() {
		return reflect.Value{}, fmt.Errorf("field index %v is not settable", index)
	}
	return current, nil
}

func requestFieldParentsReady(parent reflect.Value, index []int) bool {
	if !parent.IsValid() || len(index) == 0 {
		return false
	}
	for parent.Kind() == reflect.Pointer {
		if parent.IsNil() {
			return false
		}
		parent = parent.Elem()
	}
	if parent.Kind() != reflect.Struct {
		return false
	}
	for _, fieldIndex := range index[:len(index)-1] {
		if parent.Kind() != reflect.Struct || fieldIndex < 0 || fieldIndex >= parent.NumField() {
			return false
		}
		parent = parent.Field(fieldIndex)
		if parent.Kind() == reflect.Pointer {
			if parent.IsNil() {
				return false
			}
			parent = parent.Elem()
		}
	}
	last := index[len(index)-1]
	return parent.Kind() == reflect.Struct && last >= 0 && last < parent.NumField()
}

func requestCompiledField(parent reflect.Value, index []int) reflect.Value {
	if parent.Kind() == reflect.Pointer {
		parent = parent.Elem()
	}
	return parent.FieldByIndex(index)
}

func (j *JSONBodyParser) getFieldOptions(opts jsonRequestOptions, fieldStruct reflect.Value, fieldRule *RuleDef) jsonRequestOptions {
	rtn := opts
	rtn.Body = fieldStruct
	rtn.HasBody = fieldStruct.IsValid()
	rtn.SchemaRules = fieldRule
	return rtn
}

func (j *JSONBodyParser) encodeFieldValue(c ParserContext, buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
	isEmptyValue := func(v reflect.Value) bool {
		switch v.Kind() {
		case reflect.String:
			return v.String() == ""
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return v.Int() == 0
		case reflect.Bool:
			return !v.Bool()
		case reflect.Slice, reflect.Array, reflect.Map:
			return v.Len() == 0
		case reflect.Ptr, reflect.Interface:
			return v.IsNil()
		default:
			return false
		}
	}

	encodeString := func(b *bytes.Buffer, s string) {
		b.WriteRune('"')
		for _, r := range s {
			switch r {
			case '"':
				b.WriteString(`\"`)
			case '\\':
				b.WriteString(`\\`)
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteRune('"')
	}

	// We need to define encodeArr and encodeMap recursing with method call j.encodeFieldValue
	// BUT closures capturing method receiver? Yes.

	// Issue: encodeArr in encoders.go called `encodeFieldValue(c, ...)` which was a function.
	// Here `j.encodeFieldValue` is a method.
	// I'll define local helpers that call the method.

	var encodeArr func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error
	encodeArr = func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
		buf.WriteString("[")

		var arules *RuleDef
		if rules != nil {
			arules = rules.item
		}

		for i := 0; i < val.Len(); i++ {
			if i > 0 {
				buf.WriteString(",")
			}
			if err := j.encodeFieldValue(c, buf, val.Index(i), arules, append(kp, strconv.Itoa(i))); err != nil {
				return err
			}
		}

		buf.WriteString("]")
		return nil
	}

	var encodeMap func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error
	encodeMap = func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
		buf.WriteString("{")

		var mrules *RuleDef
		if rules != nil {
			mrules = rules.additionalProperties
		}

		mr := val.MapRange()
		for i := 0; mr.Next(); i++ {
			if i > 0 {
				buf.WriteString(",")
			}

			key := mr.Key()
			keyStr, ok := key.Interface().(string)
			if !ok {
				return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", errors.New("map key must be of type string"))
			}

			if err := j.encodeFieldValue(c, buf, key, mrules, append(kp, keyStr)); err != nil {
				return err
			}
			buf.WriteString(":")
			if err := j.encodeFieldValue(c, buf, mr.Value(), mrules, append(kp, keyStr)); err != nil {
				return err
			}
		}

		buf.WriteString("}")
		return nil
	}

	var encodeStruct func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error
	encodeStruct = func(buf *bytes.Buffer, val reflect.Value, rules *RuleDef, kp []string) error {
		buf.WriteByte('{')
		first := true

		// Fast path: schema rules are pre-compiled
		if rules != nil && len(rules.orderedProps) > 0 {
			for _, frules := range rules.orderedProps {
				var fieldValue reflect.Value
				if len(frules.accessor.index) > 0 {
					fieldValue = val.FieldByIndex(frules.accessor.index)
				} else {
					fieldValue = val.FieldByName(frules.fieldName)
				}

				if (slices.Contains(frules.tags["json"], "omitempty") && isEmptyValue(fieldValue)) || slices.Contains(frules.tags["json"], "-") {
					continue
				}

				if !first {
					buf.WriteByte(',')
				}
				first = false

				if len(frules.jsonKeyBytes) > 0 {
					buf.Write(frules.jsonKeyBytes)
				} else {
					buf.WriteByte('"')
					buf.WriteString(frules.field)
					buf.WriteString(`":`)
				}

				if err := j.encodeFieldValue(c, buf, fieldValue, frules, append(kp, frules.field)); err != nil {
					return err
				}
			}
		} else {
			// Slow path: arbitrary struct without predefined schema
			typ := val.Type()
			for _, field := range reflect.VisibleFields(typ) {
				if isPromotedEmbed(field) {
					continue
				}
				jsonTags := strings.Split(field.Tag.Get("json"), ",")
				var name string
				if len(jsonTags) > 0 && jsonTags[0] != "" {
					name = jsonTags[0]
				} else {
					name = field.Name
				}

				if slices.Contains(jsonTags, "-") || (slices.Contains(jsonTags, "omitempty") && isEmptyValue(val.FieldByName(field.Name))) {
					continue
				}

				if !first {
					buf.WriteByte(',')
				}
				first = false

				buf.WriteByte('"')
				buf.WriteString(name)
				buf.WriteString(`":`)
				fieldValue := val.FieldByName(field.Name)
				if err := j.encodeFieldValue(c, buf, fieldValue, nil, append(kp, name)); err != nil {
					return err
				}
			}
		}

		buf.WriteByte('}')
		return nil
	}

	var vIsValid bool
	var vany any
	if val.IsValid() {
		if rules != nil && rules.defStr != "" && isEmptyValue(val) {
			if val.CanAddr() {
				val.Set(reflect.ValueOf(rules.defVal).Convert(val.Type()))
			} else {
				ptr := reflect.New(val.Type())
				ptr.Elem().Set(reflect.ValueOf(rules.defVal).Convert(val.Type()))
				val = ptr.Elem()
			}
		}

		vIsValid = true
		vany = val.Interface()
	}

	if rules != nil {
		if err := runValidation(vany, ResponseErr, schemaBody, strings.Join(kp, "."), rules.rules); err != nil {
			return err
		}
	}

	if vany == nil {
		_, err := buf.WriteString("null")
		if err != nil {
			return err
		}
		return nil
	}

	if rules != nil {
		if spec, ok := c.CustomSpecs().Find(string(rules.format)); ok {
			if vIsValid {
				v, err := spec.Encode(vany)
				if err != nil {
					return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", err)
				}
				encodeString(buf, v)
				return nil

			} else {
				return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", errors.New("could not cast given type to string"))
			}
		}
	}

	// json.Marshaler (covers json.RawMessage) → write verbatim
	if vIsValid {
		if m, ok := vany.(json.Marshaler); ok {
			// For nil slices/pointers that implement json.Marshaler, emit null.
			switch val.Kind() {
			case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface:
				if val.IsNil() {
					_, err := buf.WriteString("null")
					return err
				}
			}
			raw, err := m.MarshalJSON()
			if err != nil {
				return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", err)
			}
			buf.Write(raw) // already valid JSON
			return nil
		}
	}
	// []byte (no custom marshaler) → base64 string
	if vIsValid && utils.IsByteSlice(val.Type()) {
		if val.IsNil() {
			_, err := buf.WriteString("null")
			return err
		}
		encodeString(buf, base64.StdEncoding.EncodeToString(val.Bytes()))
		return nil
	}

	switch val.Kind() {
	case reflect.Invalid:
		_, err := buf.WriteString("null")
		if err != nil {
			return err
		}
	case reflect.Interface:
		return j.encodeFieldValue(c, buf, val.Elem(), rules, kp)
	case reflect.Pointer:
		if !val.IsValid() {
			_, err := buf.WriteString("null")
			if err != nil {
				return err
			}
			return nil
		}
		return j.encodeFieldValue(c, buf, val.Elem(), rules, kp)
	case reflect.String:
		encodeString(buf, val.String())
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b := strconv.AppendInt(buf.AvailableBuffer(), val.Int(), 10)
		buf.Write(b)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b := strconv.AppendUint(buf.AvailableBuffer(), val.Uint(), 10)
		buf.Write(b)
		return nil
	case reflect.Float32, reflect.Float64:
		b := strconv.AppendFloat(buf.AvailableBuffer(), val.Float(), 'f', -1, 64)
		buf.Write(b)
		return nil
	case reflect.Bool:
		b := strconv.AppendBool(buf.AvailableBuffer(), val.Bool())
		buf.Write(b)
		return nil
	case reflect.Slice, reflect.Array:
		return encodeArr(buf, val, rules, kp)
	case reflect.Map:
		return encodeMap(buf, val, rules, kp)
	case reflect.Struct:
		if rules.format == utils.TimeObjectFormat {
			if v, ok := (vany).(time.Time); ok {
				encodeString(buf, v.Format(rules.pattern))
				return nil
			} else {
				return newErrReport(ResponseErr, schemaBody, strings.Join(kp, "."), "typeMismatch", errors.New("cannot cast time field to string"))
			}
		} else {
			return encodeStruct(buf, val, rules, kp)
		}
	}

	return nil
}
