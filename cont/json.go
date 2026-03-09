package cont

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/michaelolof/gofi/utils"

	"github.com/valyala/fastjson"
)

type ParsedJson struct {
	pv *fastjson.Value
}

func NewParsedJson(pv *fastjson.Value) ParsedJson {
	return ParsedJson{pv: pv}
}

func JsonParse(p fastjson.Parser, bs []byte) (*ParsedJson, error) {
	pv, err := p.ParseBytes(bs)
	if err != nil {
		return nil, err
	}

	return &ParsedJson{pv: pv}, err
}

func PoolJsonParse(bs []byte) (*ParsedJson, error) {
	var p fastjson.Parser
	pv, err := p.ParseBytes(bs)
	if err != nil {
		return nil, err
	}

	return &ParsedJson{pv: pv}, err
}

type JsonDefinition uint8

const (
	ArrayDefinition     JsonDefinition = 1
	ObjectDefinition    JsonDefinition = 2
	MapDefinition       JsonDefinition = 3
	InterfaceDefinition JsonDefinition = 4
)

type eof struct{}

var EOF eof

func (p *ParsedJson) GetByKind(kind reflect.Kind, format utils.ObjectFormats, keys ...string) (any, error) {
	obj := p.pv.Get(keys...)
	if obj == nil || obj.Type() == fastjson.TypeNull {
		return EOF, nil
	}

	switch kind {
	case reflect.String:
		v, err := obj.StringBytes()
		if err != nil {
			return nil, err
		}
		return string(v), nil
	case reflect.Int:
		v, err := obj.Int()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Int8:
		v, err := obj.Int()
		if err != nil {
			return nil, err
		}
		return int8(v), nil
	case reflect.Int16:
		v, err := obj.Int()
		if err != nil {
			return nil, err
		}
		return int16(v), nil
	case reflect.Int32:
		v, err := obj.Int()
		if err != nil {
			return nil, err
		}
		return int32(v), nil
	case reflect.Int64:
		v, err := obj.Int64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Uint:
		v, err := obj.Uint()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Uint8:
		v, err := obj.Uint()
		if err != nil {
			return nil, err
		}
		return uint8(v), nil
	case reflect.Uint16:
		v, err := obj.Uint()
		if err != nil {
			return nil, err
		}
		return uint16(v), nil
	case reflect.Uint32:
		v, err := obj.Uint()
		if err != nil {
			return nil, err
		}
		return uint32(v), nil
	case reflect.Uint64:
		v, err := obj.Uint64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Float32:
		v, err := obj.Float64()
		if err != nil {
			return nil, err
		}
		return float32(v), nil
	case reflect.Float64:
		v, err := obj.Float64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Bool:
		v, err := obj.Bool()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Array, reflect.Slice:
		// peek into the array to see if its a list of primitives
		return ArrayDefinition, nil
	case reflect.Struct:
		switch format {
		case utils.TimeObjectFormat:
			v, err := obj.StringBytes()
			if err != nil {
				return nil, err
			}
			return string(v), nil
		default:
			// peek into the array to see if its a list of primitives
			return ObjectDefinition, nil
		}
	case reflect.Map:
		return MapDefinition, nil
	case reflect.Interface:
		return InterfaceDefinition, nil
	default:
		panic(fmt.Sprintf("unsupported kind '%s' passed in GetByKind(...)", kind.String()))
	}
}

// GetNodeByKind extracts and casts directly from a fastjson.Value node rather than looking up by path.
func GetNodeByKind(node *fastjson.Value, kind reflect.Kind, format utils.ObjectFormats) (any, error) {
	if node == nil || node.Type() == fastjson.TypeNull {
		return EOF, nil
	}

	switch kind {
	case reflect.String:
		v, err := node.StringBytes()
		if err != nil {
			return nil, err
		}
		return string(v), nil
	case reflect.Int:
		v, err := node.Int()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Int8:
		v, err := node.Int()
		if err != nil {
			return nil, err
		}
		return int8(v), nil
	case reflect.Int16:
		v, err := node.Int()
		if err != nil {
			return nil, err
		}
		return int16(v), nil
	case reflect.Int32:
		v, err := node.Int()
		if err != nil {
			return nil, err
		}
		return int32(v), nil
	case reflect.Int64:
		v, err := node.Int64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Uint:
		v, err := node.Uint()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Uint8:
		v, err := node.Uint()
		if err != nil {
			return nil, err
		}
		return uint8(v), nil
	case reflect.Uint16:
		v, err := node.Uint()
		if err != nil {
			return nil, err
		}
		return uint16(v), nil
	case reflect.Uint32:
		v, err := node.Uint()
		if err != nil {
			return nil, err
		}
		return uint32(v), nil
	case reflect.Uint64:
		v, err := node.Uint64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Float32:
		v, err := node.Float64()
		if err != nil {
			return nil, err
		}
		return float32(v), nil
	case reflect.Float64:
		v, err := node.Float64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Bool:
		v, err := node.Bool()
		if err != nil {
			return nil, err
		}
		return v, nil
	case reflect.Array, reflect.Slice:
		return ArrayDefinition, nil
	case reflect.Struct:
		switch format {
		case utils.TimeObjectFormat:
			v, err := node.StringBytes()
			if err != nil {
				return nil, err
			}
			return string(v), nil
		default:
			return ObjectDefinition, nil
		}
	case reflect.Map:
		return MapDefinition, nil
	case reflect.Interface:
		return InterfaceDefinition, nil
	default:
		panic(fmt.Sprintf("unsupported kind '%s' passed in GetNodeByKind(...)", kind.String()))
	}
}

func (p *ParsedJson) Exist(keys ...string) bool {
	return p.pv.Exists(keys...)
}

type ArrayValues uint8

const (
	PrimitiveArrVal ArrayValues = iota + 1
	ArrayArrVal
	ObjectArrVal
	UnknownArrVal
)

func (p *ParsedJson) GetPrimitiveArrVals(kind reflect.Kind, format utils.ObjectFormats, keys []string, size int) ([]any, error) {
	arr := make([]any, 0, size)

	i := 0
	v, err := p.GetByKind(kind, format, append(keys, fmt.Sprintf("%d", i))...)
	if err != nil {
		return nil, err
	} else if v == EOF {
		return nil, nil
	}

	arr = append(arr, v)

	for {
		i++
		v, err := p.GetByKind(kind, format, append(keys, fmt.Sprintf("%d", i))...)
		if err != nil {
			return nil, err
		} else if v == EOF {
			break
		}

		arr = append(arr, v)
	}

	return arr, nil
}

// GetPrimitiveArrValsFromNode extracts primitive array values directly from a *fastjson.Value array block.
func GetPrimitiveArrValsFromNode(nodes []*fastjson.Value, kind reflect.Kind, format utils.ObjectFormats, size int) ([]any, error) {
	arr := make([]any, 0, len(nodes))

	for _, node := range nodes {
		v, err := GetNodeByKind(node, kind, format)
		if err != nil {
			return nil, err
		} else if v == EOF {
			// Fastjson nodes array usually won't contain literal missing EOF instances but skips mapped nils,
			// if it does we skip or break based on reqs.
			continue
		}
		arr = append(arr, v)
	}

	return arr, nil
}

func (p *ParsedJson) GetRawObject(keys []string) (*fastjson.Object, error) {
	obj := p.pv.Get(keys...)
	return obj.Object()
}

func (p *ParsedJson) GetRawValue() *fastjson.Value {
	return p.pv
}

func (p *ParsedJson) GetAnyValue(keys []string) (any, error) {
	v := p.pv
	if len(keys) > 0 {
		v = p.pv.Get(keys...)
	}

	if v == nil {
		return nil, errors.New("error getting any value")
	}

	switch v.Type() {
	case fastjson.TypeString:
		s := v.String()
		if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
			return strconv.Unquote(s)
		}
		return s, nil
	case fastjson.TypeTrue:
		return true, nil
	case fastjson.TypeFalse:
		return false, nil
	case fastjson.TypeNull:
		return nil, nil
	case fastjson.TypeArray, fastjson.TypeObject:
		var vany any
		if err := json.Unmarshal(v.GetStringBytes(), &vany); err != nil {
			return nil, err
		}
		return vany, nil
	case fastjson.TypeNumber:
		iv, err := v.Int()
		if err == nil {
			return iv, nil
		}

		fv, err := v.Float64()
		if err != nil {
			return nil, err
		}

		return fv, nil
	}

	return nil, errors.New("unknown value passed")
}

// GetAnyValueFromNode resolves an arbitrary type dynamically straight from a specific *fastjson.Value node.
func GetAnyValueFromNode(node *fastjson.Value) (any, error) {
	if node == nil {
		return nil, errors.New("error getting any value string from node")
	}

	switch node.Type() {
	case fastjson.TypeString:
		s := node.String()
		if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
			return strconv.Unquote(s)
		}
		return s, nil
	case fastjson.TypeTrue:
		return true, nil
	case fastjson.TypeFalse:
		return false, nil
	case fastjson.TypeNull:
		return nil, nil
	case fastjson.TypeArray, fastjson.TypeObject:
		var vany any
		if err := json.Unmarshal(node.GetStringBytes(), &vany); err != nil {
			return nil, err
		}
		return vany, nil
	case fastjson.TypeNumber:
		iv, err := node.Int()
		if err == nil {
			return iv, nil
		}

		fv, err := node.Float64()
		if err != nil {
			return nil, err
		}

		return fv, nil
	}

	return nil, errors.New("unknown value passed to GetAnyValueFromNode")
}

type arrayItem struct {
	Size int
}

func NewArrayItem(size int) arrayItem {
	return arrayItem{
		Size: size,
	}
}
