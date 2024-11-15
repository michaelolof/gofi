package utils

import (
	"net/http"
	"reflect"
	"time"
)

type ObjectFormats string

const (
	TimeObjectFormat   ObjectFormats = "date-time"
	CookieObjectFormat ObjectFormats = "cookie"
)

var TimeType = reflect.TypeOf(time.Time{})
var CookieType = reflect.TypeOf(http.Cookie{})
