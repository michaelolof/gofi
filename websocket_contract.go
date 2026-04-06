package gofi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/michaelolof/gofi/validators"
)

type WebSocketDirection string

const (
	WebSocketInbound  WebSocketDirection = "inbound"
	WebSocketOutbound WebSocketDirection = "outbound"
	WebSocketError    WebSocketDirection = "error"
)

// WebSocketSchema declares the documented websocket protocol families for a route.
// It is intended for the top-level WebSocket field on a route schema.
type WebSocketSchema struct {
	Inbound  WebSocketMessageFamily `json:"inbound,omitempty"`
	Outbound WebSocketMessageFamily `json:"outbound,omitempty"`
	Error    WebSocketMessageFamily `json:"error,omitempty"`
}

// IsZero reports whether the websocket protocol declaration is empty.
func (s WebSocketSchema) IsZero() bool {
	return s.Inbound.IsZero() && s.Outbound.IsZero() && s.Error.IsZero()
}

// WebSocketMessageFamily describes one discriminated websocket message family.
type WebSocketMessageFamily struct {
	Description   string             `json:"-"`
	Discriminator string             `json:"-"`
	Variants      []WebSocketMessage `json:"-"`
}

// IsZero reports whether the message family has any variants.
func (f WebSocketMessageFamily) IsZero() bool {
	return len(f.Variants) == 0
}

// NormalizedDiscriminator returns the configured discriminator property name.
// When omitted, websocket message families default to a type field.
func (f WebSocketMessageFamily) NormalizedDiscriminator() string {
	if value := strings.TrimSpace(f.Discriminator); value != "" {
		return value
	}

	return "type"
}

// SingleSchema returns the underlying schema when the family has exactly one variant.
func (f WebSocketMessageFamily) SingleSchema() (any, bool) {
	if len(f.Variants) != 1 || f.Variants[0].Schema == nil {
		return nil, false
	}

	return f.Variants[0].Schema, true
}

// WebSocketMessage describes one discriminated websocket variant.
type WebSocketMessage struct {
	Type        string
	Schema      any
	Description string
	Example     any
}

type compiledWebSocketContract struct {
	inbound  *compiledWebSocketMessageContract
	outbound *compiledWebSocketMessageContract
	error    *compiledWebSocketMessageContract
}

type compiledWebSocketMessageContract struct {
	discriminator string
	variants      map[string]compiledWebSocketVariant
	bySchemaType  map[reflect.Type]compiledWebSocketVariant
}

type compiledWebSocketVariant struct {
	messageType string
	schema      any
	schemaType  reflect.Type
}

type websocketStructFieldRule struct {
	index []int
	rule  string
	name  string
}

type websocketCachedStructValidator struct {
	typeName string
	fields   []websocketStructFieldRule
}

var websocketStructValidatorCache sync.Map

// ExtractWebSocketSchema reads the top-level WebSocket field from a route schema when present.
func ExtractWebSocketSchema(schema any) (WebSocketSchema, bool) {
	if schema == nil {
		return WebSocketSchema{}, false
	}

	value := reflect.ValueOf(schema)
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return WebSocketSchema{}, false
		}
		value = value.Elem()
	}

	if !value.IsValid() || value.Kind() != reflect.Struct {
		return WebSocketSchema{}, false
	}

	field := value.FieldByName(string(schemaWebSocket))
	if !field.IsValid() || !field.CanInterface() {
		return WebSocketSchema{}, false
	}

	ws, ok := field.Interface().(WebSocketSchema)
	if !ok || ws.IsZero() {
		return WebSocketSchema{}, false
	}

	return ws, true
}

func compileWebSocketContract(protocol WebSocketSchema) *compiledWebSocketContract {
	contract := &compiledWebSocketContract{}
	contract.inbound = compileWebSocketMessageContract(protocol.Inbound)
	contract.outbound = compileWebSocketMessageContract(protocol.Outbound)
	contract.error = compileWebSocketMessageContract(protocol.Error)

	if contract.inbound == nil && contract.outbound == nil && contract.error == nil {
		return nil
	}

	return contract
}

func compileWebSocketMessageContract(family WebSocketMessageFamily) *compiledWebSocketMessageContract {
	if family.IsZero() {
		return nil
	}

	contract := &compiledWebSocketMessageContract{
		discriminator: family.NormalizedDiscriminator(),
		variants:      make(map[string]compiledWebSocketVariant, len(family.Variants)),
		bySchemaType:  make(map[reflect.Type]compiledWebSocketVariant, len(family.Variants)),
	}

	for _, variant := range family.Variants {
		typ, value, ok := resolveWebSocketVariantType(variant.Schema)
		if !ok {
			continue
		}

		compiled := compiledWebSocketVariant{
			messageType: variant.Type,
			schema:      value,
			schemaType:  typ,
		}
		contract.variants[variant.Type] = compiled
		contract.bySchemaType[typ] = compiled
	}

	if len(contract.variants) == 0 {
		return nil
	}

	return contract
}

func DecodeWebSocketJSON(c Context, direction WebSocketDirection, payload []byte, dst any) (bool, error) {
	family := getWebSocketMessageContract(c, direction)
	if family == nil {
		return false, nil
	}

	return true, family.decode(payload, dst, direction)
}

func ValidateWebSocketPayload(c Context, direction WebSocketDirection, payload any) (bool, error) {
	family := getWebSocketMessageContract(c, direction)
	if family == nil {
		return false, nil
	}

	return true, family.validate(payload, direction)
}

func getWebSocketMessageContract(c Context, direction WebSocketDirection) *compiledWebSocketMessageContract {
	ctx, ok := c.(*context)
	if !ok {
		return nil
	}

	rules := ctx.rules()
	if rules == nil || rules.websocket == nil {
		return nil
	}

	switch direction {
	case WebSocketInbound:
		return rules.websocket.inbound
	case WebSocketOutbound:
		return rules.websocket.outbound
	case WebSocketError:
		return rules.websocket.error
	default:
		return nil
	}
}

func (c *compiledWebSocketMessageContract) validate(payload any, direction WebSocketDirection) error {
	if payload == nil {
		return fmt.Errorf("websocket %s payload is nil", direction)
	}

	if variant, ok := c.variantForValue(payload); ok {
		return validateWebSocketSchemaValue(payload, variant.schema, string(direction))
	}

	discriminator, innerPayload, ok := c.extractEnvelopePayload(payload)
	if !ok {
		return fmt.Errorf("websocket %s payload does not match any declared websocket contract", direction)
	}

	variant, ok := c.variants[discriminator]
	if !ok {
		return fmt.Errorf("websocket %s payload type '%s' is not declared in the websocket contract", direction, discriminator)
	}

	return validateWebSocketSchemaValue(innerPayload, variant.schema, string(direction))
}

func (c *compiledWebSocketMessageContract) decode(payload []byte, dst any, direction WebSocketDirection) error {
	if dst == nil {
		return fmt.Errorf("websocket %s destination is nil", direction)
	}

	if variant, ok := c.variantForTarget(dst); ok {
		if err := json.Unmarshal(payload, dst); err == nil {
			if err := validateWebSocketSchemaValue(dst, variant.schema, string(direction)); err == nil {
				return nil
			}
		}
	}

	discriminator, rawPayload, err := c.extractWireEnvelope(payload, direction)
	if err != nil {
		return err
	}

	variant, ok := c.variants[discriminator]
	if !ok {
		return fmt.Errorf("websocket %s payload type '%s' is not declared in the websocket contract", direction, discriminator)
	}

	decoded, err := decodeWebSocketVariantPayload(rawPayload, variant.schemaType)
	if err != nil {
		return fmt.Errorf("websocket %s validation failed: %w", direction, err)
	}

	if target, ok := c.variantForTarget(dst); ok && target.schemaType == variant.schemaType {
		if err := json.Unmarshal(rawPayload, dst); err != nil {
			return err
		}
		return validateWebSocketSchemaValue(dst, variant.schema, string(direction))
	}

	if err := populateWebSocketEnvelope(dst, c.discriminator, discriminator, decoded); err != nil {
		return err
	}

	return validateWebSocketSchemaValue(decoded, variant.schema, string(direction))
}

func (c *compiledWebSocketMessageContract) variantForValue(payload any) (compiledWebSocketVariant, bool) {
	typ := reflect.TypeOf(payload)
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil {
		return compiledWebSocketVariant{}, false
	}

	variant, ok := c.bySchemaType[typ]
	return variant, ok
}

func (c *compiledWebSocketMessageContract) variantForTarget(dst any) (compiledWebSocketVariant, bool) {
	typ := reflect.TypeOf(dst)
	if typ == nil {
		return compiledWebSocketVariant{}, false
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	variant, ok := c.bySchemaType[typ]
	return variant, ok
}

func (c *compiledWebSocketMessageContract) extractWireEnvelope(payload []byte, direction WebSocketDirection) (string, json.RawMessage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", nil, err
	}

	var discriminator string
	rawType, ok := envelope[c.discriminator]
	if !ok {
		return "", nil, fmt.Errorf("websocket %s payload missing discriminator field '%s'", direction, c.discriminator)
	}
	if err := json.Unmarshal(rawType, &discriminator); err != nil {
		return "", nil, fmt.Errorf("websocket %s discriminator field '%s' must be a string", direction, c.discriminator)
	}

	rawPayload, ok := envelope["payload"]
	if !ok {
		return "", nil, fmt.Errorf("websocket %s payload missing envelope field 'payload'", direction)
	}

	return discriminator, rawPayload, nil
}

func (c *compiledWebSocketMessageContract) extractEnvelopePayload(payload any) (string, any, bool) {
	rv := reflect.ValueOf(payload)
	for rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "", nil, false
		}
		rv = rv.Elem()
	}

	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return "", nil, false
	}

	discriminatorField, ok := findStructFieldByJSONName(rv, c.discriminator)
	if !ok || discriminatorField.Kind() != reflect.String {
		return "", nil, false
	}

	payloadField, ok := findStructFieldByJSONName(rv, "payload")
	if !ok {
		return "", nil, false
	}

	return discriminatorField.String(), payloadField.Interface(), true
}

func decodeWebSocketVariantPayload(raw json.RawMessage, typ reflect.Type) (any, error) {
	value := reflect.New(typ)
	if err := json.Unmarshal(raw, value.Interface()); err != nil {
		return nil, err
	}
	return value.Elem().Interface(), nil
}

func populateWebSocketEnvelope(dst any, discriminatorField string, discriminator string, payload any) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("websocket destination must be a non-nil pointer")
	}

	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("websocket destination type mismatch: expected envelope struct")
	}

	if field, ok := findStructFieldByJSONName(rv, discriminatorField); ok && field.CanSet() && field.Kind() == reflect.String {
		field.SetString(discriminator)
	}

	payloadField, ok := findStructFieldByJSONName(rv, "payload")
	if !ok || !payloadField.CanSet() {
		return fmt.Errorf("websocket destination type mismatch: expected envelope field 'payload'")
	}

	decoded := reflect.ValueOf(payload)
	if !decoded.IsValid() {
		return fmt.Errorf("websocket destination payload is invalid")
	}

	if payloadField.Kind() == reflect.Interface {
		payloadField.Set(decoded)
		return nil
	}

	if decoded.Type().AssignableTo(payloadField.Type()) {
		payloadField.Set(decoded)
		return nil
	}

	if payloadField.Kind() == reflect.Pointer && decoded.Type().AssignableTo(payloadField.Type().Elem()) {
		ptr := reflect.New(decoded.Type())
		ptr.Elem().Set(decoded)
		payloadField.Set(ptr)
		return nil
	}

	if decoded.Type().ConvertibleTo(payloadField.Type()) {
		payloadField.Set(decoded.Convert(payloadField.Type()))
		return nil
	}

	return fmt.Errorf("websocket destination payload field cannot accept %s", decoded.Type())
}

func findStructFieldByJSONName(rv reflect.Value, jsonName string) (reflect.Value, bool) {
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}

		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" {
			name = field.Name
		}
		if name == jsonName || field.Name == jsonName {
			return rv.Field(i), true
		}
	}

	return reflect.Value{}, false
}

func validateWebSocketSchemaValue(payload any, schema any, direction string) error {
	if schema == nil {
		return nil
	}
	if payload == nil {
		return fmt.Errorf("websocket %s payload is nil", direction)
	}

	payloadType := reflect.TypeOf(payload)
	for payloadType != nil && payloadType.Kind() == reflect.Pointer {
		payloadType = payloadType.Elem()
	}

	schemaType := reflect.TypeOf(schema)
	for schemaType != nil && schemaType.Kind() == reflect.Pointer {
		schemaType = schemaType.Elem()
	}

	if payloadType == nil || schemaType == nil || payloadType != schemaType {
		return fmt.Errorf("websocket %s payload type mismatch: expected %s, got %s", direction, schemaType, payloadType)
	}

	v, err := getCachedWebSocketStructValidator(payloadType)
	if err != nil {
		return err
	}

	if err := v.validate(payload); err != nil {
		return fmt.Errorf("websocket %s validation failed: %w", direction, err)
	}

	return nil
}

func getCachedWebSocketStructValidator(t reflect.Type) (*websocketCachedStructValidator, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("websocket validation expects struct payload, got %s", t.String())
	}

	if vv, ok := websocketStructValidatorCache.Load(t); ok {
		return vv.(*websocketCachedStructValidator), nil
	}

	built := &websocketCachedStructValidator{typeName: t.String(), fields: buildWebSocketFieldRules(t)}
	actual, _ := websocketStructValidatorCache.LoadOrStore(t, built)
	return actual.(*websocketCachedStructValidator), nil
}

func buildWebSocketFieldRules(t reflect.Type) []websocketStructFieldRule {
	rules := make([]websocketStructFieldRule, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		rule := f.Tag.Get("validate")
		if rule == "" {
			continue
		}
		rules = append(rules, websocketStructFieldRule{index: f.Index, rule: rule, name: f.Name})
	}
	return rules
}

func (v *websocketCachedStructValidator) validate(payload any) error {
	rv := reflect.ValueOf(payload)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return fmt.Errorf("nil payload for type %s", v.typeName)
		}
		rv = rv.Elem()
	}

	for _, f := range v.fields {
		fv := rv.FieldByIndex(f.index)
		if err := validators.Validate(fv.Interface(), f.rule); err != nil {
			return fmt.Errorf("field '%s': %w", f.name, err)
		}
	}

	return nil
}

func (s *serveMux) compileWebSocketSchema(protocol WebSocketSchema) openapiSchema {
	properties := make(map[string]openapiSchema, 3)
	required := make([]string, 0, 3)

	if schema := s.compileWebSocketMessageFamily(protocol.Inbound); !schema.IsEmpty() {
		properties["inbound"] = schema
		required = append(required, "inbound")
	}

	if schema := s.compileWebSocketMessageFamily(protocol.Outbound); !schema.IsEmpty() {
		properties["outbound"] = schema
		required = append(required, "outbound")
	}

	if schema := s.compileWebSocketMessageFamily(protocol.Error); !schema.IsEmpty() {
		properties["error"] = schema
		required = append(required, "error")
	}

	if len(properties) == 0 {
		return openapiSchema{}
	}

	return openapiSchema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func (s *serveMux) compileWebSocketMessageFamily(family WebSocketMessageFamily) openapiSchema {
	if family.IsZero() {
		return openapiSchema{}
	}

	discriminator := family.NormalizedDiscriminator()
	oneOf := make([]openapiSchema, 0, len(family.Variants))

	for _, variant := range family.Variants {
		payload := s.compileWebSocketVariantPayload(variant)
		branch := openapiSchema{
			Title:       variant.Type,
			Type:        "object",
			Description: variant.Description,
			Example:     variant.Example,
			Properties: map[string]openapiSchema{
				discriminator: {
					Type: "string",
					Enum: []any{variant.Type},
				},
				"payload": payload,
			},
			Required: []string{discriminator, "payload"},
		}

		oneOf = append(oneOf, branch)
	}

	if len(oneOf) == 0 {
		return openapiSchema{}
	}

	return openapiSchema{
		Type:        "object",
		Description: family.Description,
		OneOf:       oneOf,
		Discriminator: &openapiDiscriminator{
			PropertyName: discriminator,
		},
	}
}

func (s *serveMux) compileWebSocketVariantPayload(variant WebSocketMessage) openapiSchema {
	if variant.Schema == nil {
		return openapiSchema{Type: "object"}
	}

	typ, value, ok := resolveWebSocketVariantType(variant.Schema)
	if !ok {
		return openapiSchema{Type: "object"}
	}

	ruleDefs := newRuleDef(reflect.StructField{Name: variant.Type, Type: typ}, "", value, nil, false, false, nil, nil, nil, nil)
	schema := s.getTypeInfo(typ, value, variant.Type, ruleDefs)
	if schema.Title == "" {
		schema.Title = variant.Type
	}
	return schema
}

func resolveWebSocketVariantType(schema any) (reflect.Type, any, bool) {
	if schema == nil {
		return nil, nil, false
	}

	typ := reflect.TypeOf(schema)
	value := schema

	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()

		current := reflect.ValueOf(value)
		if current.IsValid() && current.Kind() == reflect.Pointer && !current.IsNil() {
			value = current.Elem().Interface()
			continue
		}

		value = reflect.New(typ).Elem().Interface()
	}

	return typ, value, true
}
