package gofi

import (
	"reflect"

	"github.com/michaelolof/gofi/utils"
	"github.com/valyala/fastjson"
)

// compileJSONBindMaxDepth checks the profiled common shape once during schema
// registration. A positive result enables direct binding and also records the
// maximum schema depth so the runtime path can preserve MaxDepth behavior.
//
// The binder deliberately reuses RuleDef rather than constructing a second
// metadata tree. Unsupported features keep using the generic walker.
func compileJSONBindMaxDepth(rule *RuleDef) uint8 {
	if rule == nil || rule.kind != reflect.Slice || rule.item == nil || rule.item.kind != reflect.Struct {
		return 0
	}

	depth, ok := compiledJSONBindDepth(rule)
	if !ok || depth > int(^uint8(0)) {
		return 0
	}
	return uint8(depth)
}

func compiledJSONBindDepth(rule *RuleDef) (int, bool) {
	if rule == nil || len(rule.rules) != 0 || rule.required || rule.present || rule.defVal != nil {
		return 0, false
	}
	if rule.typ == nil || rule.typ.Kind() == reflect.Pointer {
		return 0, false
	}
	if rule.format != "" && rule.format != utils.TimeObjectFormat {
		return 0, false
	}
	if rule.format == utils.TimeObjectFormat {
		return 0, true
	}

	switch rule.kind {
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.Bool:
		return 0, true

	case reflect.Struct:
		maxDepth := 0
		for _, child := range rule.orderedProps {
			// Multi-index accessors can traverse promoted pointer fields. The
			// generic walker retains responsibility for those shapes.
			if len(child.accessor.index) != 1 {
				return 0, false
			}
			childDepth, ok := compiledJSONBindDepth(child)
			if !ok {
				return 0, false
			}
			if childDepth+1 > maxDepth {
				maxDepth = childDepth + 1
			}
		}
		return maxDepth, true

	case reflect.Slice:
		if rule.item == nil {
			return 0, false
		}
		depth, ok := compiledJSONBindDepth(rule.item)
		return depth + 1, ok

	case reflect.Map:
		if rule.typ.Key() != reflect.TypeFor[string]() || rule.additionalProperties == nil {
			return 0, false
		}
		depth, ok := compiledJSONBindDepth(rule.additionalProperties)
		return depth + 1, ok

	default:
		return 0, false
	}
}

func bindCompiledJSONNode(rule *RuleDef, value *fastjson.Value, destination reflect.Value) bool {
	if value == nil || value.Type() == fastjson.TypeNull {
		return true
	}

	if rule.format == utils.TimeObjectFormat {
		bytes, err := value.StringBytes()
		if err != nil {
			return false
		}
		parsed, err := parseTime(string(bytes), rule.pattern)
		if err != nil {
			return false
		}
		destination.Set(reflect.ValueOf(parsed))
		return true
	}

	switch rule.kind {
	case reflect.String:
		bytes, err := value.StringBytes()
		if err != nil {
			return false
		}
		destination.SetString(string(bytes))
		return true

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		parsed, err := value.Int()
		if err != nil {
			return false
		}
		// Overflowing values fall back to the generic walker, which reports them.
		if destination.OverflowInt(int64(parsed)) {
			return false
		}
		destination.SetInt(int64(parsed))
		return true

	case reflect.Int64:
		parsed, err := value.Int64()
		if err != nil {
			return false
		}
		destination.SetInt(parsed)
		return true

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		parsed, err := value.Uint()
		if err != nil {
			return false
		}
		if destination.OverflowUint(uint64(parsed)) {
			return false
		}
		destination.SetUint(uint64(parsed))
		return true

	case reflect.Uint64:
		parsed, err := value.Uint64()
		if err != nil {
			return false
		}
		destination.SetUint(parsed)
		return true

	case reflect.Float32, reflect.Float64:
		parsed, err := value.Float64()
		if err != nil {
			return false
		}
		if destination.OverflowFloat(parsed) {
			return false
		}
		destination.SetFloat(parsed)
		return true

	case reflect.Bool:
		parsed, err := value.Bool()
		if err != nil {
			return false
		}
		destination.SetBool(parsed)
		return true

	case reflect.Struct:
		if value.Type() != fastjson.TypeObject {
			return false
		}
		for _, child := range rule.orderedProps {
			if !bindCompiledJSONNode(child, value.Get(child.field), destination.Field(child.accessor.index[0])) {
				return false
			}
		}
		return true

	case reflect.Slice:
		values, err := value.Array()
		if err != nil {
			return false
		}
		collection := reflect.MakeSlice(rule.typ, len(values), len(values))
		for index, item := range values {
			// The generic primitive-slice decoder skips JSON null elements.
			// Fall back so that unusual behavior remains exactly compatible.
			if item != nil && item.Type() == fastjson.TypeNull && isCompiledJSONPrimitive(rule.item) {
				return false
			}
			if !bindCompiledJSONNode(rule.item, item, collection.Index(index)) {
				return false
			}
		}
		destination.Set(collection)
		return true

	case reflect.Map:
		object, err := value.Object()
		if err != nil {
			return false
		}
		collection := reflect.MakeMapWithSize(rule.typ, object.Len())
		valid := true
		object.Visit(func(key []byte, item *fastjson.Value) {
			if !valid {
				return
			}
			element := reflect.New(rule.typ.Elem()).Elem()
			if !bindCompiledJSONNode(rule.additionalProperties, item, element) {
				valid = false
				return
			}
			collection.SetMapIndex(reflect.ValueOf(string(key)), element)
		})
		if !valid {
			return false
		}
		destination.Set(collection)
		return true
	}

	return false
}

func isCompiledJSONPrimitive(rule *RuleDef) bool {
	if rule == nil || rule.format == utils.TimeObjectFormat {
		return false
	}
	return utils.IsPrimitiveKind(rule.kind)
}
