package schema

import (
	"errors"
	"net/http"
	"reflect"

	"github.com/michaelolof/gofi/cont"
)

func WriteResponse(w http.ResponseWriter, rules *SchemaRules, opts *ValidationOptions, code int, response any) error {
	if w == nil {
		return errors.New("response writer cannot be nil")
	}

	// 1. Get response rules for the code
	_, defs, err := rules.GetRespRulesByCode(code)
	if err != nil {
		w.WriteHeader(code) // At least default write
		return err
	}

	// 2. Validate Body
	// We need to find the rule for body in defs.
	// Typically body rule is under "Body" or root?
	// In gofi/rules.go, `responses` was map[string]map[string]ruleDef.
	// The inner map keys are typically "Body", "Headers" etc?
	// Let's check SchemaRules structure.
	// `Responses map[string]map[string]RuleDef`
	// `SetResps` sets `s.Responses[key][rules.Field] = *rules`
	// where `key` is status group (e.g. "2XX").
	// and `rules.Field` is "Body", "Headers" etc.

	var bodyDef *RuleDef
	if d, ok := defs[string(SchemaBody)]; ok {
		bodyDef = &d
	}

	// 3. Handle Headers
	if hDef, ok := defs[string(SchemaHeaders)]; ok {
		for _, prop := range hDef.Properties {
			// Response headers validation/encoding is tricky.
			// The `response` object should contain headers?
			// Or `response` IS the body?
			// In gofi, `Send` took `obj any`.
			// If `obj` is a struct that maps to body/headers etc?
			// But usually `Send(200, bodyStruct)`.
			// If we want to set headers, they must be in the `response` struct or set on `w` before?
			// `gofi` had logic to extract headers from response struct if it matched schema.
			// But here `WriteResponse` takes `response any`.

			// For now, let's assume `response` matches the body schema.
			// Headers might need to be set manually on w, or handled if `response` has header fields?
			// If the user passes a struct that has fields tagged for headers?

			// Let's defer complex header validation from struct for now, and focus on simple Body writing.
			// But we should validate if schema requires headers?

			// If headers are required in schema, and we don't provided them?
			// For minimal implementation:
			if prop.Required {
				// We can't easily validate headers unless we check `w.Header()`?
				if w.Header().Get(prop.Field) == "" && prop.DefStr != "" {
					w.Header().Set(prop.Field, prop.DefStr)
				}
			}
		}
	}

	// 4. Encode Body
	if bodyDef != nil {
		// Determine Content-Type
		// `gofi` uses `ReqContent` logic but that's for request.
		// For response, we should check `Content-Type` header in schema or set default.
		contentType := cont.ApplicationJson
		if hDef, ok := defs[string(SchemaHeaders)]; ok {
			if ct, ok := hDef.Properties["Content-Type"]; ok && ct.DefStr != "" {
				contentType = cont.ContentType(ct.DefStr)
			}
		}

		// Also check if w has Content-Type set?
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", string(contentType))
		}

		w.WriteHeader(code)

		// Get serializer
		var sz SchemaEncoder
		if opts != nil && opts.Serializers != nil {
			if s, ok := opts.Serializers(contentType); ok {
				sz = s
			}
		}

		if sz == nil {
			sz = &JSONSchemaEncoder{}
		}

		encoded, err := sz.ValidateAndEncode(response, ResponseValidationOptions{
			Context:     w, // Passing response writer as context
			SchemaRules: bodyDef,
			Body:        reflect.ValueOf(response),
		})

		if err != nil {
			return err
		}

		_, err = w.Write(encoded)
		return err

	} else {
		w.WriteHeader(code)
	}

	return nil
}

func ValidateResponse(rules *SchemaRules, code int, response any) error {
	// similar logic but without writing
	// ...
	// Placeholder logic

	_, defs, err := rules.GetRespRulesByCode(code)
	if err != nil {
		return err
	}

	var bodyDef *RuleDef
	if d, ok := defs[string(SchemaBody)]; ok {
		bodyDef = &d
	}

	if bodyDef != nil {
		// We can reuse ValidateAndEncode with a dummy context or nil
		sz := &JSONSchemaEncoder{} // Or from options if we pass them
		_, err := sz.ValidateAndEncode(response, ResponseValidationOptions{
			Context:     nil,
			SchemaRules: bodyDef,
			Body:        reflect.ValueOf(response),
		})
		return err
	}

	return nil
}

// Utility to create schema from primitive for simplistic responses?
// Maybe out of scope here.

func IsResponseRule(field string) bool {
	// Check if field is one of the response status fields
	switch field {
	case Informational, SuccessFieldName, RedirectFieldName, ErrFieldName, ServerErr, DefaultFieldName:
		return true
	}
	// Also check explicit codes?
	// "200", "404", etc.
	// Ideally we should use regex or range check but here strict names.
	return false
}

// Helper to check if a struct field tag maps to a response/request part
func GetSchemaField(tag string) (SchemaField, bool) {
	switch SchemaField(tag) {
	case SchemaHeaders:
		return SchemaHeaders, true
	case SchemaPath:
		return SchemaPath, true
	case SchemaQuery:
		return SchemaQuery, true
	case SchemaCookies:
		return SchemaCookies, true
	case SchemaBody:
		return SchemaBody, true
	default:
		return "", false
	}
}
