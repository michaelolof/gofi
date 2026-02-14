package schema

import (
	"errors"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/michaelolof/gofi/utils"
	"github.com/michaelolof/gofi/validators"
)

type CompiledSchema struct {
	Rules       SchemaRules
	Openapi     OpenapiOperationObject
	HasBody     bool
	HasResponse bool
}

type CompileOptions struct {
	CustomSpecs CustomSpecs
}

func Compile(schemaStruct any, info Info, opts *CompileOptions) (CompiledSchema, error) {
	s := CompiledSchema{
		Rules:   NewSchemaRules(),
		Openapi: InitOpenapiOperationObject(),
	}

	typ := reflect.TypeOf(schemaStruct)
	if typ.Kind() != reflect.Struct {
		return s, errors.New("schema must be a struct")
	}

	// Resolve pointer/interface
	if typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Interface {
		typ = typ.Elem()
	}

	val := reflect.ValueOf(schemaStruct)
	if val.Kind() == reflect.Pointer || val.Kind() == reflect.Interface {
		val = val.Elem()
	}

	fieldCount := typ.NumField()

	for i := 0; i < fieldCount; i++ {
		field := typ.Field(i)
		schemaInfo, ok := GetSchemaField(field.Tag.Get("schema"))
		if !ok {
			if IsResponseRule(field.Tag.Get("schema")) {
				// Handle response
				s.HasResponse = true
				if field.Type.Kind() != reflect.Struct {
					return s, errors.New("response field must be a struct")
				}

				// Process response struct
				processResponseField(&s, field, val.Field(i), opts)
				continue
			}
			continue
		}

		// Handle request parts
		processRequestField(&s, schemaInfo, field, val.Field(i), opts)
	}

	s.Openapi.Normalize(info.Method, info.Url)
	s.Openapi.Summary = info.Summary
	s.Openapi.Description = info.Description
	s.Openapi.OperationId = info.OperationId
	s.Openapi.Deprecated = &info.Deprecated
	s.Openapi.ExternalDocs = info.ExternalDocs

	return s, nil
}

func processRequestField(s *CompiledSchema, schemaInfo SchemaField, field reflect.StructField, val reflect.Value, opts *CompileOptions) {
	switch schemaInfo {
	case SchemaBody:
		s.HasBody = true
		// Body usually matches the field type directly or fields inside?
		// In gofi, SchemaBody meant the field IS the body schema (if primitive or struct).
		// If struct, it explores fields.
		// Wait, gofi `compileSchema` logic:
		// if sf.Name == string(schemaReq) { ... switch rqn { case schemaBody: ... } }
		// gofi used a nested struct strict structure: `Req struct { Body BodyType ... }`.
		// My `Compile` iterates fields of properties directly?
		// User passes `struct { Body BodyType `schema:"Body"`; Query QueryType `schema:"Query"` }`

		// So `val` is the Body value (struct or primitive wrapper).
		// But we need `getPrimitiveValFromParent` context?
		// Here `val` IS the field value from the parent instance.

		// gofi logic for body:
		// val := getPrimitiveValFromParent(obj, rqf)
		// ruleDefs := s.getFieldRuleDefs(rqf, name, val)
		// optsObj.bodySchema = s.getTypeInfo(...)

		ruleDefs := getFieldRuleDefs(field, getFieldName(field), val.Interface(), opts)
		s.Rules.SetReq(string(SchemaBody), ruleDefs)
		s.Openapi.bodySchema = getTypeInfo(field.Type, val.Interface(), getFieldName(field), ruleDefs, opts)

	case SchemaHeaders, SchemaCookies, SchemaPath, SchemaQuery:
		// These must be structs
		if field.Type.Kind() != reflect.Struct {
			return
		}

		pruleDefs := NewRuleDef(field.Type, field.Type.Kind(), string(schemaInfo), field.Name, "", nil, nil, false, nil, nil, nil, nil)
		in := schemaInfo.ReqSchemaIn()

		for _, rqff := range reflect.VisibleFields(field.Type) {
			if schemaInfo == SchemaCookies && !utils.ValidCookieType(rqff.Type) {
				continue
			}

			// val := getPrimitiveValFromParent(obj.FieldByName(rqf.Name), rqff)
			// obj.FieldByName(rqf.Name) is `val` passed to this func.
			fVal := getPrimitiveValFromParent(val, rqff)

			name := getFieldName(rqff)
			if in == "header" {
				name = strings.ToLower(name)
			}

			ruleDefs := getFieldRuleDefs(rqff, name, fVal, opts)
			pruleDefs.Attach(name, ruleDefs)

			var required *bool
			if ruleDefs.HasRule("required") {
				b := true
				required = &b
			}

			tInfo := getTypeInfo(rqff.Type, fVal, name, ruleDefs, opts)
			s.Openapi.Parameters = append(s.Openapi.Parameters, NewOpenapiParameter(in, name, required, tInfo))
		}
		s.Rules.SetReq(string(schemaInfo), pruleDefs)
	}
}

func processResponseField(s *CompiledSchema, field reflect.StructField, val reflect.Value, opts *CompileOptions) {
	status := field.Tag.Get("schema")

	var headersParams OpenapiParameters

	// Check if response field is struct
	if field.Type.Kind() != reflect.Struct {
		return
	}

	respTyp := field.Type
	for j := 0; j < respTyp.NumField(); j++ {
		rf := respTyp.Field(j)
		sc, ok := GetSchemaField(rf.Tag.Get("schema"))

		if ok {
			switch sc {
			case SchemaHeaders:
				// headers logic similar to request headers
				if rf.Type.Kind() != reflect.Struct {
					continue
				}

				pruleDefs := NewRuleDef(rf.Type, rf.Type.Kind(), string(SchemaHeaders), rf.Name, "", nil, nil, false, nil, nil, nil, nil)

				for _, rqff := range reflect.VisibleFields(rf.Type) {
					fVal := getPrimitiveValFromParent(val.Field(j), rqff)
					name := getFieldName(rqff)
					// headers lower case?
					// name = strings.ToLower(name)

					ruleDefs := getFieldRuleDefs(rqff, name, fVal, opts)
					pruleDefs.Attach(name, ruleDefs)

					var required *bool
					if ruleDefs.HasRule("required") {
						b := true
						required = &b
					}

					tInfo := getTypeInfo(rqff.Type, fVal, name, ruleDefs, opts)
					// Collecting headers
					headersParams = append(headersParams, NewOpenapiParameter("header", name, required, tInfo))
				}
				s.Rules.SetResps(status, pruleDefs)

			case SchemaBody:
				fVal := getPrimitiveValFromParent(val, rf) // Body is field of response struct
				name := getFieldName(rf)
				ruleDefs := getFieldRuleDefs(rf, name, fVal, opts)

				s.Rules.SetResps(status, ruleDefs)
				sInfo := getTypeInfo(rf.Type, fVal, name, ruleDefs, opts)
				s.Openapi.responsesSchema[status] = sInfo
			}
		} else {
			// Implicit body check
			if rf.Name == "Body" || rf.Tag.Get("json") != "" {
				fVal := getPrimitiveValFromParent(val, rf)
				name := getFieldName(rf)
				ruleDefs := getFieldRuleDefs(rf, name, fVal, opts)

				s.Rules.SetResps(status, ruleDefs)
				sInfo := getTypeInfo(rf.Type, fVal, name, ruleDefs, opts)
				s.Openapi.responsesSchema[status] = sInfo
			}
		}
	}

	if len(headersParams) > 0 {
		s.Openapi.responsesParameters[status] = headersParams
	}
}

// Ported helper functions

func getFieldRuleDefs(sf reflect.StructField, tagName string, defVal any, opts *CompileOptions) *RuleDef {
	supportedTags := []string{
		"json",
		"validate",
		"default",
		"example",
		"deprecated",
		"description",
		"pattern",
		"spec",
	}

	tagList := make(map[string][]string)
	var defStr string
	var rules []RuleOpts
	var required bool
	var max *float64
	for _, stag := range supportedTags {
		if tag, ok := sf.Tag.Lookup(stag); ok {
			switch stag {
			case "json":
				tagList[stag] = strings.Split(tag, ",")
			case "example", "deprecated", "description", "pattern", "spec":
				tagList[stag] = []string{parseTagValue(tag, sf.Type)}
			case "default":
				defStr = parseTagValue(tag, sf.Type)
			case "validate":
				vtags := strings.Split(tag, ",")
				rules = make([]RuleOpts, 0, len(vtags))
				for _, tag := range vtags {
					tagFieldRegex := regexp.MustCompile(`([a-zA-Z0-9_]+)(?:=([^,]+)|@([^,]+))?`)
					maches := tagFieldRegex.FindStringSubmatch(tag)
					ruleName := maches[1]
					optionStr := maches[2]

					if len(maches) > 3 && len(maches[3]) != 0 {
						if ts, ok := checkTagReference(maches[3], sf.Type); ok {
							optionStr = ts
						}
					}

					var options []string
					if len(optionStr) > 0 { // Changed > 1 to > 0 to allow single char options
						options = strings.Split(optionStr, " ")
					}

					if ruleName == "required" {
						required = true
					}

					if (ruleName == "max" || ruleName == "lte") && len(options) >= 1 {
						flt, err := strconv.ParseFloat(options[0], 64)
						if err == nil {
							max = &flt
						}
					}

					// customValidators needed for RuleOpts?
					// NewRuleOpts takes ContextValidators.
					// We can pass them from opts if available or nil.
					var cv validators.ContextValidators
					if opts != nil {
						cv = opts.CustomSpecs.Validators() // Need helper?
						// CustomSpecs is map[string]Props. Validatros are separate?
						// ValidationOptions has CustomValidators.
						// CompileOptions has CustomSpecs.
						// We probably should add CustomValidators to CompileOptions if rule parsing needs them?
						// Actually NewRuleOpts uses validators to find the validator function.
						// But here we are at compile time.
						// RuleOpts stores the Dator function.
						// So we DO need validators here.
					}

					rules = append(rules, NewRuleOpts(sf.Type, sf.Type.Kind(), ruleName, options, cv))
				}
			}
		}
	}

	rtn := NewRuleDef(sf.Type, sf.Type.Kind(), tagName, sf.Name, defStr, defVal, rules, required, max, nil, nil, nil)
	rtn.Tags = tagList
	return rtn
}

func getTypeInfo(typ reflect.Type, value any, name string, ruleDefs *RuleDef, opts *CompileOptions) OpenapiSchema {

	kind := typ.Kind()

	var typeStr string
	var pattern string
	var format string
	var enum []any
	var optStr []string
	var min *float64
	var max *float64
	var items *OpenapiSchema
	var addProps *OpenapiSchema
	var example any
	var deprecated *bool
	var description string
	var specTag string
	properties := make(map[string]OpenapiSchema)
	requiredProps := make([]string, 0)

	var pRequired bool

	if ruleDefs != nil {
		minOpts := ruleDefs.RuleOptions("min")
		minOpts = append(minOpts, ruleDefs.RuleOptions("gte")...)
		for _, opt := range minOpts {
			i, err := strconv.ParseFloat(opt, 64)
			if err == nil {
				min = &i
				break
			}
		}

		maxOpts := ruleDefs.RuleOptions("max")
		maxOpts = append(maxOpts, ruleDefs.RuleOptions("lte")...)
		for _, opt := range maxOpts {
			i, err := strconv.ParseFloat(opt, 64)
			if err == nil {
				max = &i
				break
			}
		}

		optStr = ruleDefs.RuleOptions("oneof")
		pRequired = ruleDefs.Required

		if v, ok := ruleDefs.Tags["example"]; ok && len(v) > 0 {
			if v, err := utils.PrimitiveFromStr(typ.Kind(), v[0]); err == nil && utils.IsPrimitive(v) {
				example = v
			}
		}

		if v, ok := ruleDefs.Tags["deprecated"]; ok && len(v) > 0 {
			if v, err := strconv.ParseBool(v[0]); err == nil && v {
				deprecated = &v
			}
		}

		if v, ok := ruleDefs.Tags["description"]; ok && len(v) > 0 {
			description = v[0]
		}

		if v, ok := ruleDefs.Tags["pattern"]; ok && len(v) > 0 {
			pattern = v[0]
		}

		if v, ok := ruleDefs.Tags["spec"]; ok && len(v) > 0 {
			specTag = v[0]
		}
	}

	isCustom := false
	if opts != nil {
		if v, ok := opts.CustomSpecs[specTag]; ok {
			enum = optsMapper(optStr, nil)
			typeStr = v.Type
			format = v.Format
			ruleDefs.Format = utils.ObjectFormats(specTag)
			isCustom = true
		}
	}

	if !isCustom {
		switch kind {
		case reflect.String:
			enum = optsMapper(optStr, nil)
			format = ruleDefs.FindRules([]string{"date", "date-time", "password", "byte", "binary", "email", "uuid", "uri", "hostname", "ipv4", "ipv6"}, "")
			typeStr = "string"

		case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint8, reflect.Uint16, reflect.Uint32:
			enum = optsMapper(optStr, func(s string) any {
				v, err := strconv.Atoi(s)
				if err != nil {
					log.Fatalln("unsupported type in schema validate option 'oneof=" + s + "' at " + name)
				}
				return int32(v)
			})
			format = "int32"
			typeStr = "integer"

		case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64:
			enum = optsMapper(optStr, func(s string) any {
				v, err := strconv.Atoi(s)
				if err != nil {
					log.Fatalln("unsupported type in schema validate option 'oneof=" + s + "' at " + name)
				}
				return int64(v)
			})
			format = "int64"
			typeStr = "integer"

		case reflect.Float32, reflect.Float64:
			enum = optsMapper(optStr, func(s string) any {
				v, err := strconv.ParseFloat(s, 32)
				if err != nil {
					log.Fatalln("unsupported type in schema validate option 'oneof=" + s + "' at " + name)
				}
				return float64(v)
			})
			format = "float"
			typeStr = "number"

		case reflect.Bool:
			enum = []any{true, false}
			format = "bool"
			typeStr = "boolean"

		case reflect.Slice, reflect.Array:
			typeStr = "array"
			_ruleDefs := GetItemRuleDef(typ.Elem())
			ruleDefs.Append(_ruleDefs)
			i := getTypeInfo(typ.Elem(), value, name, _ruleDefs, opts)
			items = &i

		case reflect.Map:
			typeStr = "object"
			_ruleDefs := GetItemRuleDef(typ.Elem())
			ruleDefs.AddProps(_ruleDefs)
			i := getTypeInfo(typ.Elem(), value, name, _ruleDefs, opts)
			addProps = &i

		case reflect.Struct:
			switch typ {
			case utils.TimeType:
				enum = optsMapper(optStr, nil)
				typeStr = "string"
				format = string(utils.TimeObjectFormat)
				ruleDefs.Format = utils.TimeObjectFormat
				if pattern == "" {
					pattern = time.RFC3339Nano
				}

			case utils.CookieType:
				enum = optsMapper(optStr, nil)
				typeStr = "string"
				format = string(utils.CookieObjectFormat)
				ruleDefs.Format = utils.CookieObjectFormat

			default:
				typeStr = "object"
				obj := reflect.ValueOf(value)
				var hasObj bool
				if obj.IsValid() && obj.Kind() == reflect.Struct {
					hasObj = true
				}

				for _, sf := range reflect.VisibleFields(typ) {
					var val any
					if hasObj {
						val = getPrimitiveValFromParent(obj, sf)
					}
					name := getFieldName(sf)
					if name == "-" {
						continue
					}

					_ruleDefs := getFieldRuleDefs(sf, name, val, opts)
					ruleDefs.Attach(name, _ruleDefs)
					if _ruleDefs.HasRule("required") {
						requiredProps = append(requiredProps, name)
					}
					properties[name] = getTypeInfo(sf.Type, val, name, _ruleDefs, opts)
				}

			}

		case reflect.Pointer:
			ruleDefs.Kind = typ.Elem().Kind()
			return getTypeInfo(typ.Elem(), value, name, ruleDefs, opts)

		}
	}

	ruleDefs.Pattern = pattern

	return NewOpenapiSchema(
		format,
		typeStr,
		pattern,
		value,
		min,
		max,
		enum,
		items,
		addProps,
		properties,
		requiredProps,
		deprecated,
		description,
		example,
		pRequired,
	)
}

func getPrimitiveValFromParent(parent reflect.Value, f reflect.StructField) any {
	var fieldVal any
	if parent.IsValid() && parent.Kind() == reflect.Struct {
		fv := parent.FieldByName(f.Name)
		if fv.IsValid() {
			// Check comparability before interface()?
			// Interface() should be fine if exported or we handle unexported?
			// "VisibleFields" returns exported fields (mostly).
			// fieldVal = fv.Interface()
			// Using logic from gofi/compiler.go
			if fv.Comparable() {
				fieldVal = fv.Interface()
				// Zero value check logic?
				if kt := reflect.New(f.Type).Elem(); kt.IsValid() && kt.Comparable() {
					ktv := kt.Interface()
					if fieldVal != ktv {
						return fieldVal
					}
				}
			}
		}
	}

	tagVal := f.Tag.Get("default")
	kind := f.Type.Kind()

	switch true {
	case utils.IsPrimitiveKind(kind):
		val, err := utils.PrimitiveFromStr(kind, tagVal)
		if err != nil {
			if fieldVal != nil {
				return fieldVal
			} else {
				return nil
			}
		}
		return val
	case kind == reflect.Slice && tagVal == "[]":
		return reflect.MakeSlice(reflect.SliceOf(f.Type.Elem()), 0, 0).Interface()
	default:
		return nil
	}
}

func getFieldName(sf reflect.StructField) string {
	jsonTags := strings.Split(sf.Tag.Get("json"), ",")
	var name string
	if len(jsonTags) > 0 && jsonTags[0] != "" {
		name = jsonTags[0]
	} else {
		name = sf.Name
	}
	return name
}

func optsMapper(opts []string, fn func(string) any) []any {
	if opts == nil {
		return nil
	}

	ropts := make([]any, 0, len(opts))
	for _, opt := range opts {
		var v any
		if fn != nil {
			v = fn(opt)
		} else {
			v = opt
		}

		ropts = append(ropts, v)
	}
	return ropts
}

func parseTagValue(tag string, typ reflect.Type) string {
	if methodName, found := strings.CutPrefix(tag, "@"); found {
		if v, ok := checkTagReference(methodName, typ); ok {
			return v
		}
	}
	return tag
}

func checkTagReference(methodName string, typ reflect.Type) (string, bool) {
	method := reflect.New(typ).Elem().MethodByName(methodName)
	if method.IsValid() && !method.IsNil() {
		if results := method.Call(nil); len(results) > 0 {
			if v, ok := (results[0].Interface()).(string); ok {
				return v, true
			}
		}
	}
	return "", false
}

// Helpers for CustomSpecs to Validators map if needed
func (c CustomSpecs) Validators() validators.ContextValidators {
	// Return custom validators usage?
	// This is probably not what CustomSpecs stores.
	// CustomSpecs stores Format/Type/Decoder/Encoder.
	// Validators are passed separately.
	return nil
}
