package fluid

import (
	"encoding/json"
	"testing"
)

func ptr[T any](v T) *T { return &v }

// ---- SchemaObject Tests ----

func TestStringSchema(t *testing.T) {
	s := StringSchema()
	assertJSON(t, s, `{"type":"string"}`)
}

func TestIntegerSchema(t *testing.T) {
	s := IntegerSchema()
	assertJSON(t, s, `{"type":"integer"}`)
}

func TestNumberSchema(t *testing.T) {
	s := NumberSchema()
	assertJSON(t, s, `{"type":"number"}`)
}

func TestBooleanSchema(t *testing.T) {
	s := BooleanSchema()
	assertJSON(t, s, `{"type":"boolean"}`)
}

func TestDateTimeSchema(t *testing.T) {
	s := DateTimeSchema()
	assertJSON(t, s, `{"type":"string","format":"date-time"}`)
}

func TestDateSchema(t *testing.T) {
	s := DateSchema()
	assertJSON(t, s, `{"type":"string","format":"date"}`)
}

func TestBinarySchema(t *testing.T) {
	s := BinarySchema()
	assertJSON(t, s, `{"type":"string","format":"binary"}`)
}

func TestEmailSchema(t *testing.T) {
	s := EmailSchema()
	assertJSON(t, s, `{"type":"string","format":"email"}`)
}

func TestURISchema(t *testing.T) {
	s := URISchema()
	assertJSON(t, s, `{"type":"string","format":"uri"}`)
}

func TestUUIDSchema(t *testing.T) {
	s := UUIDSchema()
	assertJSON(t, s, `{"type":"string","format":"uuid"}`)
}

func TestPasswordSchema(t *testing.T) {
	s := PasswordSchema()
	assertJSON(t, s, `{"type":"string","format":"password"}`)
}

func TestByteSchema(t *testing.T) {
	s := ByteSchema()
	assertJSON(t, s, `{"type":"string","format":"byte"}`)
}

func TestRefSchema(t *testing.T) {
	s := RefSchema("#/components/schemas/User")
	assertJSON(t, s, `{"$ref":"#/components/schemas/User"}`)
}

func TestArraySchema_Basic(t *testing.T) {
	items := StringSchema()
	s := ArraySchema(items)
	assertJSON(t, s, `{"type":"array","items":{"type":"string"}}`)
}

func TestArraySchema_WithConstraints(t *testing.T) {
	items := IntegerSchema()
	s := ArraySchema(items).
		WithMinItems(ptr(1)).
		WithMaxItems(ptr(10)).
		WithUniqueItems().
		WithDescription("List of IDs")
	assertJSONContains(t, s, []string{
		`"type":"array"`,
		`"minItems":1`,
		`"maxItems":10`,
		`"uniqueItems":true`,
		`"description":"List of IDs"`,
	})
}

func TestObjectSchema_Basic(t *testing.T) {
	props := map[string]SchemaObject{
		"name": StringSchema(),
		"age":  IntegerSchema(),
	}
	s := ObjectSchema(props).WithRequired("name", "age")
	assertJSONContains(t, s, []string{
		`"type":"object"`,
		`"required":["name","age"]`,
		`"name":{"type":"string"}`,
		`"age":{"type":"integer"}`,
	})
}

// ---- Builder Chaining Tests ----

func TestBuilderChaining(t *testing.T) {
	s := StringSchema().
		WithDescription("A name").
		WithPattern(`^[a-z]+$`).
		WithMinLength(ptr(1)).
		WithMaxLength(ptr(100)).
		WithDefault("default-name").
		WithExample("example-name").
		WithEnum("foo", "bar", "baz").
		WithDeprecated(true).
		WithNullable()

	assertJSONContains(t, s, []string{
		`"type":"string"`,
		`"description":"A name"`,
		`"pattern":"^[a-z]+$"`,
		`"minLength":1`,
		`"maxLength":100`,
		`"default":"default-name"`,
		`"example":"example-name"`,
		`"enum":["foo","bar","baz"]`,
		`"deprecated":true`,
		`"nullable":true`,
	})
}

func TestNumberConstraints(t *testing.T) {
	s := NumberSchema().
		WithMinimum(ptr(0.0)).
		WithMaximum(ptr(100.0)).
		WithExclusiveMinimum(ptr(0.0)).
		WithExclusiveMaximum(ptr(100.0)).
		WithMultipleOf(ptr(5.0))

	assertJSONContains(t, s, []string{
		`"type":"number"`,
		`"minimum":0`,
		`"maximum":100`,
		`"exclusiveMinimum":0`,
		`"exclusiveMaximum":100`,
		`"multipleOf":5`,
	})
}

func TestObjectConstraints(t *testing.T) {
	s := ObjectSchema(nil).
		WithMinProperties(ptr(1)).
		WithMaxProperties(ptr(10))

	assertJSONContains(t, s, []string{
		`"type":"object"`,
		`"minProperties":1`,
		`"maxProperties":10`,
	})
}

func TestWithDescription(t *testing.T) {
	s := IntegerSchema().WithDescription("The user ID")
	assertJSONContains(t, s, []string{`"type":"integer"`, `"description":"The user ID"`})
}

func TestWithExample(t *testing.T) {
	s := StringSchema().WithExample("hello")
	assertJSONContains(t, s, []string{`"type":"string"`, `"example":"hello"`})
}

func TestWithDefault(t *testing.T) {
	s := IntegerSchema().WithDefault(42)
	assertJSONContains(t, s, []string{`"type":"integer"`, `"default":42`})
}

func TestWithEnum(t *testing.T) {
	s := StringSchema().WithEnum("a", "b", "c")
	assertJSONContains(t, s, []string{`"type":"string"`, `"enum":["a","b","c"]`})
}

func TestWithNullable(t *testing.T) {
	s := StringSchema().WithNullable()
	assertJSONContains(t, s, []string{`"type":"string"`, `"nullable":true`})
}

func TestWithReadOnlyWriteOnly(t *testing.T) {
	s := StringSchema().WithReadOnly().WithWriteOnly()
	assertJSONContains(t, s, []string{`"readOnly":true`, `"writeOnly":true`})
}

func TestWithExternalDocs(t *testing.T) {
	s := StringSchema().WithExternalDocs(ExternalDocsObject{
		URL:         "https://example.com/docs",
		Description: "More info",
	})
	assertJSONContains(t, s, []string{
		`"externalDocs":{"description":"More info","url":"https://example.com/docs"}`,
	})
}

func TestWithXML(t *testing.T) {
	s := StringSchema().WithXML(XMLObject{
		Name:      "foo",
		Namespace: "ns",
		Wrapped:   true,
	})
	assertJSONContains(t, s, []string{
		`"xml":{"name":"foo","namespace":"ns","wrapped":true}`,
	})
}

func TestWithDiscriminator(t *testing.T) {
	s := SchemaObject{
		OneOf: []SchemaObject{
			RefSchema("#/components/schemas/Cat"),
			RefSchema("#/components/schemas/Dog"),
		},
	}.WithDiscriminator(DiscriminatorObject{
		PropertyName: "petType",
		Mapping: map[string]string{
			"cat": "#/components/schemas/Cat",
			"dog": "#/components/schemas/Dog",
		},
	})

	assertJSONContains(t, s, []string{
		`"discriminator":{"propertyName":"petType","mapping":{"cat":"#/components/schemas/Cat","dog":"#/components/schemas/Dog"}}`,
	})
}

func TestAdditionalProperties(t *testing.T) {
	s := ObjectSchema(nil).WithAdditionalProperties(StringSchema())
	assertJSONContains(t, s, []string{
		`"type":"object"`,
		`"additionalProperties":{"type":"string"}`,
	})
}

func TestWithProperties(t *testing.T) {
	s := ObjectSchema(nil).WithProperties(map[string]SchemaObject{
		"id": IntegerSchema(),
	})
	assertJSONContains(t, s, []string{
		`"type":"object"`,
		`"properties":{"id":{"type":"integer"}}`,
	})
}

func TestWithFormat(t *testing.T) {
	s := StringSchema().WithFormat("email")
	assertJSON(t, s, `{"type":"string","format":"email"}`)
}

func TestNestedObject(t *testing.T) {
	addressSchema := ObjectSchema(map[string]SchemaObject{
		"street": StringSchema(),
		"city":   StringSchema(),
	})

	userSchema := ObjectSchema(map[string]SchemaObject{
		"name":    StringSchema(),
		"address": addressSchema,
	}).WithRequired("name")

	assertJSONContains(t, userSchema, []string{
		`"type":"object"`,
		`"required":["name"]`,
		`"address":{"type":"object"`,
		`"street":{"type":"string"}`,
		`"city":{"type":"string"}`,
	})
}

func TestNestedArray(t *testing.T) {
	itemSchema := ObjectSchema(map[string]SchemaObject{
		"x": NumberSchema(),
		"y": NumberSchema(),
	})

	s := ArraySchema(itemSchema).WithDescription("Coordinates")
	assertJSONContains(t, s, []string{
		`"type":"array"`,
		`"items":{"type":"object"`,
		`"x":{"type":"number"}`,
		`"y":{"type":"number"}`,
	})
}

// ---- ParameterObject Tests ----

func TestQueryParameter(t *testing.T) {
	p := QueryParameter("limit", IntegerSchema()).
		WithDescription("Max results").
		WithRequired(true)
	assertJSONContains(t, p, []string{
		`"name":"limit"`,
		`"in":"query"`,
		`"required":true`,
		`"description":"Max results"`,
	})
}

func TestPathParameter(t *testing.T) {
	p := PathParameter("userId", IntegerSchema()).
		WithDescription("User ID")
	assertJSONContains(t, p, []string{
		`"name":"userId"`,
		`"in":"path"`,
		`"required":true`, // path params always required
		`"description":"User ID"`,
	})
}

func TestHeaderParameter(t *testing.T) {
	p := HeaderParameter("X-API-Key", StringSchema()).
		WithDescription("API key")
	assertJSONContains(t, p, []string{
		`"name":"X-API-Key"`,
		`"in":"header"`,
		`"description":"API key"`,
	})
}

func TestCookieParameter(t *testing.T) {
	p := CookieParameter("session", StringSchema())
	assertJSONContains(t, p, []string{
		`"name":"session"`,
		`"in":"cookie"`,
	})
}

func TestParameter_WithStyleExplode(t *testing.T) {
	p := QueryParameter("ids", ArraySchema(IntegerSchema())).
		WithStyle("form").
		WithExplode(true).
		WithAllowEmptyValue().
		WithAllowReserved()
	assertJSONContains(t, p, []string{
		`"style":"form"`,
		`"explode":true`,
		`"allowEmptyValue":true`,
		`"allowReserved":true`,
	})
}

func TestParameter_WithExamples(t *testing.T) {
	p := QueryParameter("status", StringSchema()).
		WithExamples(map[string]ExampleObject{
			"active":   {Value: "active", Summary: "Active status"},
			"inactive": {Value: "inactive", Summary: "Inactive status"},
		})
	json := mustMarshal(t, p)
	assertContains(t, json, `"active":{"summary":"Active status","value":"active"}`)
}

func TestParameter_WithDeprecated(t *testing.T) {
	p := QueryParameter("old", StringSchema()).
		WithDeprecated(true)
	assertJSONContains(t, p, []string{`"deprecated":true`})
}

func TestParameter_WithContent(t *testing.T) {
	p := PathParameter("userId", IntegerSchema()).
		WithContent(map[string]MediaTypeObject{
			"application/json": {Schema: ptr(RefSchema("#/components/schemas/User"))},
		})
	json := mustMarshal(t, p)
	assertContains(t, json, `"application/json":{"schema":{"$ref":"#/components/schemas/User"}}`)
}

// ---- ResponseObject Tests ----

func TestJSONResponse(t *testing.T) {
	r := JSONResponse("OK", RefSchema("#/components/schemas/User"))
	assertJSONContains(t, r, []string{
		`"description":"OK"`,
		`"application/json":{"schema":{"$ref":"#/components/schemas/User"}}`,
	})
}

func TestResponse_WithJSONContent(t *testing.T) {
	r := ResponseObject{Description: "Created"}.
		WithJSONContent(ObjectSchema(map[string]SchemaObject{
			"id": IntegerSchema(),
		}))
	assertJSONContains(t, r, []string{
		`"description":"Created"`,
		`"application/json":{"schema":{"type":"object"`,
	})
}

func TestResponse_WithHeaders(t *testing.T) {
	r := JSONResponse("OK", StringSchema()).WithHeaders(map[string]HeaderObject{
		"X-Rate-Limit": {Schema: ptr(IntegerSchema()), Description: "Rate limit"},
	})
	json := mustMarshal(t, r)
	assertContains(t, json, `"X-Rate-Limit":{"description":"Rate limit","schema":{"type":"integer"}}`)
}

func TestResponse_WithContent(t *testing.T) {
	r := ResponseObject{Description: "Multi"}.
		WithContent(map[string]MediaTypeObject{
			"application/json": {Schema: ptr(StringSchema())},
			"text/plain":       {Schema: ptr(StringSchema())},
		})
	assertJSONContains(t, r, []string{
		`"description":"Multi"`,
		`"application/json":{"schema":{"type":"string"}}`,
		`"text/plain":{"schema":{"type":"string"}}`,
	})
}

func TestPlainTextResponse(t *testing.T) {
	r := PlainTextResponse("OK", StringSchema())
	assertJSONContains(t, r, []string{
		`"text/plain":{"schema":{"type":"string"}}`,
	})
}

func TestHTMLResponse(t *testing.T) {
	r := HTMLResponse("OK")
	assertJSONContains(t, r, []string{
		`"text/html":{"schema":{"type":"string"}}`,
	})
}

func TestBinaryResponse(t *testing.T) {
	r := BinaryResponse("File download")
	assertJSONContains(t, r, []string{
		`"application/octet-stream":{"schema":{"type":"string","format":"binary"}}`,
	})
}

// ---- RequestBodyObject Tests ----

func TestJSONRequestBody(t *testing.T) {
	rb := JSONRequestBody(ObjectSchema(map[string]SchemaObject{
		"name": StringSchema(),
	}))
	assertJSONContains(t, rb, []string{
		`"application/json":{"schema":{"type":"object"`,
	})
}

func TestRequestBody_WithRequired(t *testing.T) {
	rb := JSONRequestBody(StringSchema()).WithRequired()
	assertJSONContains(t, rb, []string{`"required":true`})
}

func TestRequestBody_WithDescription(t *testing.T) {
	rb := JSONRequestBody(StringSchema()).WithDescription("The payload")
	assertJSONContains(t, rb, []string{`"description":"The payload"`})
}

func TestRequestBody_WithJSONContent(t *testing.T) {
	rb := RequestBodyObject{}.WithJSONContent(StringSchema())
	assertJSONContains(t, rb, []string{`"application/json":{"schema":{"type":"string"}}`})
}

func TestRequestBody_WithFormURLEncodedContent(t *testing.T) {
	rb := RequestBodyObject{}.WithFormURLEncodedContent(ObjectSchema(map[string]SchemaObject{
		"field": StringSchema(),
	}))
	assertJSONContains(t, rb, []string{`"application/x-www-form-urlencoded":{`})
}

func TestRequestBody_WithMultipartContent(t *testing.T) {
	rb := RequestBodyObject{}.WithMultipartContent(ObjectSchema(map[string]SchemaObject{
		"file": BinarySchema(),
	}))
	assertJSONContains(t, rb, []string{`"multipart/form-data":{`})
}

func TestFormURLEncodedRequestBody(t *testing.T) {
	rb := FormURLEncodedRequestBody(ObjectSchema(map[string]SchemaObject{
		"name": StringSchema(),
	}))
	assertJSONContains(t, rb, []string{`"application/x-www-form-urlencoded":{`})
}

func TestMultipartRequestBody(t *testing.T) {
	rb := MultipartRequestBody(ObjectSchema(map[string]SchemaObject{
		"file":    BinarySchema(),
		"caption": StringSchema(),
	}))
	assertJSONContains(t, rb, []string{`"multipart/form-data":{`})
}

func TestPlainTextRequestBody(t *testing.T) {
	rb := PlainTextRequestBody(StringSchema())
	assertJSONContains(t, rb, []string{`"text/plain":{`})
}

// ---- HeaderObject Tests ----

func TestHeaderObject_Basic(t *testing.T) {
	h := HeaderObject{}.
		WithDescription("Rate limit remaining").
		WithSchema(IntegerSchema()).
		WithRequired(true)
	assertJSONContains(t, h, []string{
		`"description":"Rate limit remaining"`,
		`"schema":{"type":"integer"}`,
		`"required":true`,
	})
}

func TestHeaderObject_WithDeprecated(t *testing.T) {
	h := HeaderObject{}.
		WithSchema(StringSchema()).
		WithDeprecated(true)
	assertJSONContains(t, h, []string{`"deprecated":true`})
}

func TestHeaderObject_WithExample(t *testing.T) {
	h := HeaderObject{}.
		WithSchema(StringSchema()).
		WithExample("Bearer abc123")
	assertJSONContains(t, h, []string{`"example":"Bearer abc123"`})
}

func TestHeaderObject_WithExamples(t *testing.T) {
	h := HeaderObject{}.
		WithSchema(StringSchema()).
		WithExamples(map[string]ExampleObject{
			"jwt": {Value: "Bearer eyJ...", Summary: "JWT token"},
		})
	json := mustMarshal(t, h)
	assertContains(t, json, `"jwt":{"summary":"JWT token","value":"Bearer eyJ..."}`)
}

// ---- MediaTypeObject Tests ----

func TestMediaTypeObject_Basic(t *testing.T) {
	m := MediaTypeObject{Schema: ptr(StringSchema())}
	json := mustMarshal(t, m)
	assertContains(t, json, `"schema":{"type":"string"}`)
}

func TestMediaTypeObject_WithExample(t *testing.T) {
	m := MediaTypeObject{}.WithExample("hello")
	assertJSONContains(t, m, []string{`"example":"hello"`})
}

func TestMediaTypeObject_WithExamples(t *testing.T) {
	m := MediaTypeObject{}.WithExamples(map[string]ExampleObject{
		"short": {Value: "hi", Summary: "Short greeting"},
	})
	json := mustMarshal(t, m)
	assertContains(t, json, `"short":{"summary":"Short greeting","value":"hi"}`)
}

func TestMediaTypeObject_WithEncoding(t *testing.T) {
	m := MediaTypeObject{}.WithEncoding(map[string]EncodingObject{
		"file": {ContentType: "image/png"},
	})
	json := mustMarshal(t, m)
	assertContains(t, json, `"file":{"contentType":"image/png"}`)
}

func TestMediaTypeObject_WithSchema(t *testing.T) {
	m := MediaTypeObject{}.WithSchema(IntegerSchema())
	assertJSONContains(t, m, []string{`"schema":{"type":"integer"}`})
}

// ---- ExampleObject Tests ----

func TestExampleObject_Basic(t *testing.T) {
	e := ExampleObject{
		Summary:     "A simple example",
		Description: "Shows basic usage",
		Value:       map[string]any{"key": "value"},
	}
	json := mustMarshal(t, e)
	assertContains(t, json, `"summary":"A simple example"`)
	assertContains(t, json, `"description":"Shows basic usage"`)
	assertContains(t, json, `"key":"value"`)
}

func TestExampleObject_ExternalValue(t *testing.T) {
	e := ExampleObject{
		Summary:       "External example",
		ExternalValue: "https://example.com/examples/1",
	}
	assertJSONContains(t, e, []string{
		`"summary":"External example"`,
		`"externalValue":"https://example.com/examples/1"`,
	})
}

// ---- EncodingObject Tests ----

func TestEncodingObject_Basic(t *testing.T) {
	e := EncodingObject{
		ContentType: "image/png",
		Style:       "form",
		Explode:     ptr(true),
	}
	assertJSONContains(t, e, []string{
		`"contentType":"image/png"`,
		`"style":"form"`,
		`"explode":true`,
	})
}

// ---- SecuritySchemeObject Tests ----

func TestBearerAuth(t *testing.T) {
	s := BearerAuth(WithSecurityDescription("JWT Bearer token"))
	assertJSONContains(t, s, []string{
		`"type":"http"`,
		`"scheme":"bearer"`,
		`"bearerFormat":"JWT"`,
		`"description":"JWT Bearer token"`,
	})
}

func TestBasicAuth(t *testing.T) {
	s := BasicAuth(WithSecurityDescription("Basic auth"))
	assertJSONContains(t, s, []string{
		`"type":"http"`,
		`"scheme":"basic"`,
		`"description":"Basic auth"`,
	})
}

func TestAPIKeyAuth(t *testing.T) {
	s := APIKeyAuth("X-API-Key", "header", WithSecurityDescription("API key auth"))
	assertJSONContains(t, s, []string{
		`"type":"apiKey"`,
		`"name":"X-API-Key"`,
		`"in":"header"`,
		`"description":"API key auth"`,
	})
}

func TestOAuth2Auth(t *testing.T) {
	flows := OAuthFlowsObject{
		AuthorizationCode: &OAuthFlowObject{
			AuthorizationURL: "https://auth.example.com/authorize",
			TokenURL:         "https://auth.example.com/token",
			Scopes: map[string]string{
				"read":  "Read access",
				"write": "Write access",
			},
		},
	}
	s := OAuth2Auth(flows, WithSecurityDescription("OAuth2"))
	assertJSONContains(t, s, []string{
		`"type":"oauth2"`,
		`"description":"OAuth2"`,
		`"authorizationUrl":"https://auth.example.com/authorize"`,
		`"tokenUrl":"https://auth.example.com/token"`,
	})
}

func TestOpenIDConnectAuth(t *testing.T) {
	s := OpenIDConnectAuth("https://auth.example.com/.well-known/openid-configuration")
	assertJSONContains(t, s, []string{
		`"type":"openIdConnect"`,
		`"openIdConnectUrl":"https://auth.example.com/.well-known/openid-configuration"`,
	})
}

func TestOAuthFlowsObject(t *testing.T) {
	f := OAuthFlowsObject{
		Implicit: &OAuthFlowObject{
			AuthorizationURL: "https://example.com/auth",
			Scopes:           map[string]string{"read": "Read"},
		},
		ClientCredentials: &OAuthFlowObject{
			TokenURL: "https://example.com/token",
			Scopes:   map[string]string{"admin": "Admin"},
		},
	}
	json := mustMarshal(t, f)
	assertContains(t, json, `"implicit":{`)
	assertContains(t, json, `"clientCredentials":{`)
}

// ---- Discriminator Tests ----

func TestDiscriminatorObject(t *testing.T) {
	d := DiscriminatorObject{
		PropertyName: "type",
		Mapping: map[string]string{
			"dog": "#/components/schemas/Dog",
			"cat": "#/components/schemas/Cat",
		},
	}
	assertJSONContains(t, d, []string{
		`"propertyName":"type"`,
		`"mapping":{"cat":"#/components/schemas/Cat","dog":"#/components/schemas/Dog"}`,
	})
}

// ---- XML / ExternalDocs Tests ----

func TestXMLObject(t *testing.T) {
	x := XMLObject{
		Name:      "item",
		Namespace: "http://example.com/schema",
		Prefix:    "ns",
		Attribute: true,
		Wrapped:   true,
	}
	assertJSONContains(t, x, []string{
		`"name":"item"`,
		`"namespace":"http://example.com/schema"`,
		`"prefix":"ns"`,
		`"attribute":true`,
		`"wrapped":true`,
	})
}

func TestExternalDocsObject(t *testing.T) {
	e := ExternalDocsObject{
		Description: "Find more info here",
		URL:         "https://example.com/docs",
	}
	assertJSONContains(t, e, []string{
		`"description":"Find more info here"`,
		`"url":"https://example.com/docs"`,
	})
}

// ---- Integration: Real-World Scenario Tests ----

func TestIntegration_UserSchema(t *testing.T) {
	userSchema := ObjectSchema(map[string]SchemaObject{
		"id":        IntegerSchema().WithDescription("Unique user ID"),
		"name":      StringSchema().WithDescription("Full name"),
		"email":     EmailSchema().WithDescription("Email address"),
		"role":      StringSchema().WithEnum("admin", "editor", "viewer").WithDescription("Permission level"),
		"createdAt": DateTimeSchema().WithDescription("Creation timestamp"),
	}).WithRequired("id", "name", "email")

	json := mustMarshal(t, userSchema)
	assertContains(t, json, `"type":"object"`)
	assertContains(t, json, `"required":["id","name","email"]`)
	assertContains(t, json, `"id":{"type":"integer","description":"Unique user ID"}`)
	assertContains(t, json, `"email":{"type":"string","format":"email","description":"Email address"}`)
	assertContains(t, json, `"enum":["admin","editor","viewer"]`)
}

func TestIntegration_ErrorSchema(t *testing.T) {
	errorSchema := ObjectSchema(map[string]SchemaObject{
		"code":    IntegerSchema().WithDescription("HTTP status code").WithExample(422),
		"message": StringSchema().WithDescription("Error description"),
		"details": ArraySchema(
			ObjectSchema(map[string]SchemaObject{
				"field":   StringSchema().WithDescription("Field that failed validation"),
				"message": StringSchema().WithDescription("Validation message"),
			}),
		).WithDescription("Per-field details"),
	}).WithRequired("code", "message")

	json := mustMarshal(t, errorSchema)
	assertContains(t, json, `"details":{"type":"array","description":"Per-field details"`)
	assertContains(t, json, `"field":{"type":"string","description":"Field that failed validation"}`)
}

func TestIntegration_Polymorphism(t *testing.T) {
	petSchema := SchemaObject{
		OneOf: []SchemaObject{
			RefSchema("#/components/schemas/Cat"),
			RefSchema("#/components/schemas/Dog"),
		},
	}.WithDiscriminator(DiscriminatorObject{
		PropertyName: "petType",
		Mapping: map[string]string{
			"cat": "#/components/schemas/Cat",
			"dog": "#/components/schemas/Dog",
		},
	}).WithDescription("A polymorphic pet")

	json := mustMarshal(t, petSchema)
	assertContains(t, json, `"oneOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Dog"}]`)
	assertContains(t, json, `"discriminator":{`)
	assertContains(t, json, `"propertyName":"petType"`)
}

func TestIntegration_CompleteDocComponents(t *testing.T) {
	// Simulate a complete gofi.DocsComponent.Schemas construction
	schemas := map[string]any{
		"User": ObjectSchema(map[string]SchemaObject{
			"id":   IntegerSchema().WithDescription("User ID"),
			"name": StringSchema(),
		}),
		"Error": ObjectSchema(map[string]SchemaObject{
			"code":    IntegerSchema(),
			"message": StringSchema(),
		}),
		"Pet": SchemaObject{
			OneOf: []SchemaObject{
				RefSchema("#/components/schemas/Cat"),
				RefSchema("#/components/schemas/Dog"),
			},
			Discriminator: &DiscriminatorObject{
				PropertyName: "petType",
			},
		},
		"Cat": ObjectSchema(map[string]SchemaObject{
			"petType": StringSchema().WithEnum("cat"),
			"name":    StringSchema(),
		}),
		"Dog": ObjectSchema(map[string]SchemaObject{
			"petType": StringSchema().WithEnum("dog"),
			"breed":   StringSchema(),
		}),
	}

	json := mustMarshal(t, schemas)
	assertContains(t, json, `"User":{"type":"object"`)
	assertContains(t, json, `"Error":{"type":"object"`)
	assertContains(t, json, `"Pet":{"oneOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Dog"}]`)
	assertContains(t, json, `"Cat":{"type":"object"`)
	assertContains(t, json, `"Dog":{"type":"object"`)
}

// ---- Integration: Round-Trip Unmarshal ----

func TestRoundTrip_SchemaObject(t *testing.T) {
	original := ObjectSchema(map[string]SchemaObject{
		"name": StringSchema().WithDescription("Full name"),
		"age":  IntegerSchema().WithMinimum(ptr(0.0)),
	}).WithRequired("name")

	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored SchemaObject
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.Type != "object" {
		t.Errorf("expected type=object, got %s", restored.Type)
	}
	if len(restored.Required) != 1 || restored.Required[0] != "name" {
		t.Errorf("expected required=[name], got %v", restored.Required)
	}
	if restored.Properties["name"].Description != "Full name" {
		t.Errorf("expected name description, got %q", restored.Properties["name"].Description)
	}
}

// ---- Helpers ----

func assertJSON(t *testing.T, v any, expected string) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != expected {
		t.Errorf("JSON mismatch:\n  got:      %s\n  expected: %s", string(b), expected)
	}
}

func assertJSONContains(t *testing.T, v any, fragments []string) {
	t.Helper()
	json := mustMarshal(t, v)
	for _, frag := range fragments {
		assertContains(t, json, frag)
	}
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if len(needle) > 0 && len(haystack) == 0 {
		t.Fatalf("empty JSON but expected to contain %q (value might be nil)", needle)
	}
	if len(haystack) < len(needle) {
		t.Fatalf("JSON too short (%d bytes) to contain %q", len(haystack), needle)
	}
	// Use a simple contains check (not substring position) since JSON key ordering varies.
	found := false
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("JSON does not contain %q\nFull JSON: %s", needle, haystack)
	}
}
