package fluid

// SecuritySchemeObject represents an OpenAPI 3.0.3 Security Scheme Object.
// See: https://spec.openapis.org/oas/v3.0.3#security-scheme-object
type SecuritySchemeObject struct {
	Type             string            `json:"type"` // "apiKey", "http", "oauth2", "openIdConnect"
	Description      string            `json:"description,omitempty"`
	Name             string            `json:"name,omitempty"`             // required for apiKey
	In               string            `json:"in,omitempty"`               // required for apiKey: "query", "header", "cookie"
	Scheme           string            `json:"scheme,omitempty"`           // required for http: "basic", "bearer", etc.
	BearerFormat     string            `json:"bearerFormat,omitempty"`     // for http ("bearer"): "JWT", etc.
	Flows            *OAuthFlowsObject `json:"flows,omitempty"`            // required for oauth2
	OpenIDConnectURL string            `json:"openIdConnectUrl,omitempty"` // required for openIdConnect
}

// OAuthFlowsObject represents an OpenAPI 3.0.3 OAuth Flows Object.
// See: https://spec.openapis.org/oas/v3.0.3#oauth-flows-object
type OAuthFlowsObject struct {
	Implicit          *OAuthFlowObject `json:"implicit,omitempty"`
	Password          *OAuthFlowObject `json:"password,omitempty"`
	ClientCredentials *OAuthFlowObject `json:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlowObject `json:"authorizationCode,omitempty"`
}

// OAuthFlowObject represents an OpenAPI 3.0.3 OAuth Flow Object.
// See: https://spec.openapis.org/oas/v3.0.3#oauth-flow-object
type OAuthFlowObject struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	RefreshURL       string            `json:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes,omitempty"`
}

// ---- Convenience Constructors ----

// BearerAuth creates an http/bearer security scheme.
func BearerAuth(opts ...func(*SecuritySchemeObject)) SecuritySchemeObject {
	s := SecuritySchemeObject{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
	}
	for _, o := range opts {
		o(&s)
	}
	return s
}

// BasicAuth creates an http/basic security scheme.
func BasicAuth(opts ...func(*SecuritySchemeObject)) SecuritySchemeObject {
	s := SecuritySchemeObject{
		Type:   "http",
		Scheme: "basic",
	}
	for _, o := range opts {
		o(&s)
	}
	return s
}

// APIKeyAuth creates an apiKey security scheme.
func APIKeyAuth(name, in string, opts ...func(*SecuritySchemeObject)) SecuritySchemeObject {
	s := SecuritySchemeObject{
		Type: "apiKey",
		Name: name,
		In:   in,
	}
	for _, o := range opts {
		o(&s)
	}
	return s
}

// OAuth2Auth creates an oauth2 security scheme with the given flows.
func OAuth2Auth(flows OAuthFlowsObject, opts ...func(*SecuritySchemeObject)) SecuritySchemeObject {
	s := SecuritySchemeObject{
		Type:  "oauth2",
		Flows: &flows,
	}
	for _, o := range opts {
		o(&s)
	}
	return s
}

// OpenIDConnectAuth creates an openIdConnect security scheme.
func OpenIDConnectAuth(url string, opts ...func(*SecuritySchemeObject)) SecuritySchemeObject {
	s := SecuritySchemeObject{
		Type:             "openIdConnect",
		OpenIDConnectURL: url,
	}
	for _, o := range opts {
		o(&s)
	}
	return s
}

// ---- Option Helpers ----

// WithSecurityDescription sets the description on a security scheme.
func WithSecurityDescription(desc string) func(*SecuritySchemeObject) {
	return func(s *SecuritySchemeObject) {
		s.Description = desc
	}
}

// WithBearerFormat sets the bearer format.
func WithBearerFormat(format string) func(*SecuritySchemeObject) {
	return func(s *SecuritySchemeObject) {
		s.BearerFormat = format
	}
}
