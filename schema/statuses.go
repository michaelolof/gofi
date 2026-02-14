package schema

import "strconv"

type StatusInfo struct {
	Code        string
	Description string
	Deprecated  bool
}

const (
	Informational     = "Informational"
	SuccessFieldName  = "Success"
	RedirectFieldName = "Redirect"
	ServerErr         = "ServerError"
	ClientErr         = "ClientError"
	ErrFieldName      = "Err"
	DefaultFieldName  = "Default"
)

var Statuses = map[string][]StatusInfo{
	Informational:     {{Code: "1XX", Description: "Informational Response"}},
	SuccessFieldName:  {{Code: "2XX", Description: "Success Response"}},
	RedirectFieldName: {{Code: "3XX", Description: "Redirect Response"}},
	ClientErr:         {{Code: "4XX", Description: "Client Error"}},
	ServerErr:         {{Code: "5XX", Description: "Server Error"}},
	ErrFieldName:      {{Code: "4XX", Description: "Client Error"}, {Code: "5XX", Description: "Server Error"}},
	DefaultFieldName:  {{Code: "default", Description: "Default Response"}},

	"Continue":                      {{Code: "100", Description: "Continue Response"}},
	"SwitchingProtocols":            {{Code: "101", Description: "Switching Protocols Respones"}},
	"Processing":                    {{Code: "102", Description: "Processing (Deprecated) Response", Deprecated: true}},
	"EarlyHints":                    {{Code: "103", Description: "Early Hints Response"}},
	"Ok":                            {{Code: "200", Description: "OK Response"}},
	"Created":                       {{Code: "201", Description: "Resource Created Response"}},
	"Accepted":                      {{Code: "202", Description: "Accepted Response"}},
	"NonAuthoritativeInformation":   {{Code: "203", Description: "Non-Authoritative Information Response"}},
	"NoContent":                     {{Code: "204", Description: "No Content Response"}},
	"ResetContent":                  {{Code: "205", Description: "Reset Content Response"}},
	"PartialContent":                {{Code: "206", Description: "Partial Content Response"}},
	"MultiStatus":                   {{Code: "207", Description: "Multi-Status Response"}},
	"AlreadyReported":               {{Code: "208", Description: "Already Reported Response"}},
	"IMUsed":                        {{Code: "226", Description: "IM Used Response"}},
	"MultipleChoices":               {{Code: "300", Description: "Mulitiple Choices Response"}},
	"MovedPermanently":              {{Code: "301", Description: "Moved Permanently Response"}},
	"Found":                         {{Code: "302", Description: "Resource Found Response"}},
	"SeeOther":                      {{Code: "303", Description: "See Other Response"}},
	"NotModified":                   {{Code: "304", Description: "Not Modified Response"}},
	"TemporaryRedirect":             {{Code: "307", Description: "Temporary Redirect Response"}},
	"PermanentRedirect":             {{Code: "308", Description: "Permanent Redirect Response"}},
	"BadRequest":                    {{Code: "400", Description: "Bad Request Error"}},
	"Unauthorized":                  {{Code: "401", Description: "Unauthorized Error"}},
	"PaymentRequired":               {{Code: "402", Description: "Payment Required Error"}},
	"Forbidden":                     {{Code: "403", Description: "Forbidden Error"}},
	"NotFound":                      {{Code: "404", Description: "Not Found Error"}},
	"MethodNotAllowed":              {{Code: "405", Description: "Method Not Allowed Error"}},
	"NotAcceptable":                 {{Code: "406", Description: "Not Acceptable Error"}},
	"ProxyAuthenticationRequired":   {{Code: "407", Description: "Proxy Authentication Required Error"}},
	"RequestTimeout":                {{Code: "408", Description: "Request Timeout Error"}},
	"Conflict":                      {{Code: "409", Description: "Conflict Error"}},
	"Gone":                          {{Code: "410", Description: "Resource Gone Error"}},
	"LengthRequired":                {{Code: "411", Description: "Length Required Error"}},
	"PreconditionFailed":            {{Code: "412", Description: "Precondition Failed Error"}},
	"ContentTooLarge":               {{Code: "413", Description: "Content Too Large Error"}},
	"URITooLong":                    {{Code: "414", Description: "URI Too Long Error"}},
	"UnsupportedMediaType":          {{Code: "415", Description: "Unsupported Media Type Error"}},
	"RangeNotSatisfiable":           {{Code: "416", Description: "Range Not Satisfiable Error"}},
	"ExpectiationFailed":            {{Code: "417", Description: "Expectation Failed Error"}},
	"ImTeamPot":                     {{Code: "418", Description: "I'm a teapot Error"}},
	"MisdirectedRequest":            {{Code: "421", Description: "Misdirected Request Error"}},
	"UnprocessableContent":          {{Code: "422", Description: "Unprocessable Content Error"}},
	"Locked":                        {{Code: "423", Description: "Locked Error"}},
	"FailedDependency":              {{Code: "424", Description: "Failed Dependency Error"}},
	"TooEarly":                      {{Code: "425", Description: "Too Early Error"}},
	"UpgradeRequired":               {{Code: "426", Description: "Upgrade Required Error"}},
	"PreconditionRequired":          {{Code: "428", Description: "Precondition Required Error"}},
	"TooManyRequests":               {{Code: "429", Description: "Too Many Requests Error"}},
	"RequestHeaderFieldsTooLarge":   {{Code: "431", Description: "Request Header Fields Too Large Error"}},
	"UnavailableForLegalReasons":    {{Code: "451", Description: "Unavailable For Legal Reasons Error"}},
	"InternalServerError":           {{Code: "500", Description: "Internal Server Error"}},
	"NotImplemented":                {{Code: "501", Description: "Not Implemented Error"}},
	"BadGateway":                    {{Code: "502", Description: "Bad Gateway Error"}},
	"ServiceUnavailable":            {{Code: "503", Description: "Service Unavailable Error"}},
	"GatewayTimeout":                {{Code: "504", Description: "Gateway Timeout Error"}},
	"HTTPVersionNotSupported":       {{Code: "505", Description: "HTTP Version Not Supported Error"}},
	"VariantAlsoNegotiates":         {{Code: "506", Description: "Variant Also Negotiates Error"}},
	"InsufficientStorage":           {{Code: "507", Description: "Insufficient Storage Error"}},
	"LoopDetected":                  {{Code: "508", Description: "Loop Detected Error"}},
	"NotExtended":                   {{Code: "510", Description: "Not Extended Error"}},
	"NetworkAuthenticationRequired": {{Code: "511", Description: "Network Authentication Required Error"}},
}

var CodeToStatuses = buildCodeToStatus()

func buildCodeToStatus() map[int]StatusFieldInfo {
	rtn := make(map[int]StatusFieldInfo)
	for name, sinfos := range Statuses {
		if len(sinfos) != 1 {
			continue
		}

		status := sinfos[0]
		code, err := strconv.ParseInt(status.Code, 10, 16)
		if err != nil {
			continue
		}
		rtn[int(code)] = StatusFieldInfo{
			Field:       name,
			Description: status.Description,
		}
	}

	return rtn
}

type StatusFieldInfo struct {
	Field       string
	Description string
}
