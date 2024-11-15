package cont

import (
	"fmt"
	"reflect"

	"github.com/michaelolof/gofi/utils"

	"github.com/valyala/fastjson"
)

var jsonparser fastjson.Parser

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
	pv, err := jsonparser.ParseBytes(bs)
	if err != nil {
		return nil, err
	}

	return &ParsedJson{pv: pv}, err
}

type JsonDefinition uint8

const (
	ArrayDefinition JsonDefinition = iota + 1
	ObjectDefinition
	MapDefinition
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
	default:
		panic(fmt.Sprintf("unsupported kind '%s' passed in GetByKind(...)", kind.String()))
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

func (p *ParsedJson) GetRawObject(keys []string) (*fastjson.Object, error) {
	obj := p.pv.Get(keys...)
	return obj.Object()
}

type arrayItem struct {
	Size int
}

func NewArrayItem(size int) arrayItem {
	return arrayItem{
		Size: size,
	}
}
