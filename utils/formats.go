package utils

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
	"reflect"
	"time"
)

type ObjectFormats string

const (
	TimeObjectFormat   ObjectFormats = "date-time"
	CookieObjectFormat ObjectFormats = "cookie"
	CustomObjectFormat ObjectFormats = "custom-object"
	ByteFormat         ObjectFormats = "byte"
	RawJSONFormat      ObjectFormats = "raw-json"
)

var TimeType = reflect.TypeOf(time.Time{})
var CookieType = reflect.TypeOf(http.Cookie{})
var MultipartFile = reflect.TypeOf(multipart.FileHeader{})

var (
	jsonMarshalerType   = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	rawMessageType      = reflect.TypeOf(json.RawMessage(nil))
	byteSliceType       = reflect.TypeOf([]byte(nil))
)

// ImplementsJSONMarshaler reports whether T or *T implements json.Marshaler.
func ImplementsJSONMarshaler(t reflect.Type) bool {
	return t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType)
}

// ImplementsJSONUnmarshaler reports whether T or *T implements json.Unmarshaler.
func ImplementsJSONUnmarshaler(t reflect.Type) bool {
	return t.Implements(jsonUnmarshalerType) || reflect.PointerTo(t).Implements(jsonUnmarshalerType)
}

// IsByteSlice reports whether T is a []byte / [N]byte with NO custom JSON marshaler
// (i.e. encoding/json would treat it as a base64 string).
func IsByteSlice(t reflect.Type) bool {
	if ImplementsJSONMarshaler(t) || ImplementsJSONUnmarshaler(t) {
		return false // e.g. json.RawMessage is []byte but marshals verbatim, not base64
	}
	return (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) && t.Elem().Kind() == reflect.Uint8
}

// IsRawJSON reports whether T should round-trip as verbatim JSON (json.RawMessage
// or anything with custom JSON marshaling that we treat opaquely).
func IsRawJSON(t reflect.Type) bool {
	return t == rawMessageType || ImplementsJSONMarshaler(t) || ImplementsJSONUnmarshaler(t)
}
