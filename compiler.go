package gofi

import (
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/michaelolof/gofi/utils"
)

type compiledSchema struct {
	specs openapiOperationObject
	rules schemaRules
}

func (s *serveMux) compileSchema(schema any, info Info) compiledSchema {

	var strct = reflect.TypeOf(schema)
	if strct.Kind() == reflect.Pointer || strct.Kind() == reflect.Interface {
		strct = strct.Elem()
	}

	optsObj := initOpenapiOperationObject()
	sRules := newSchemaRules()

	optsObj.OperationId = info.OperationId
	optsObj.method = info.Method
	optsObj.Summary = info.Summary
	optsObj.urlPath = info.Url
	optsObj.Description = info.Description
	optsObj.ExternalDocs = info.ExternalDocs
	if info.Deprecated {
		optsObj.Deprecated = &info.Deprecated
	}

	for _, sf := range reflect.VisibleFields(strct) {

		// Schena fields must be a struct to be valid
		if sf.Type.Kind() != reflect.Struct {
			continue
		}

		obj := reflect.ValueOf(schema).Elem().FieldByName(sf.Name)

		if sf.Name == string(schemaReq) {
			for _, rqf := range reflect.VisibleFields(sf.Type) {
				rqn := schemaField(rqf.Name)
				kind := rqf.Type.Kind()

				switch rqn {
				case schemaHeaders, schemaCookies, schemaQuery, schemaPath:
					if kind != reflect.Struct {
						continue
					}

					// ruleDefs := getFieldRuleDefs(rqf, string(rqn), nil)
					pruleDefs := newRuleDef(kind, string(rqn), rqf.Name, "", nil, nil, false, nil, nil, nil, nil)
					in := rqn.reqSchemaIn()

					for _, rqff := range reflect.VisibleFields(rqf.Type) {
						if rqn == schemaCookies && !utils.ValidCookieType(rqff.Type) {
							continue
						}

						val := getPrimitiveValFromParent(obj.FieldByName(rqf.Name), rqff)
						name := getFieldName(rqff)
						if in == "header" {
							name = strings.ToLower(name)
						}
						ruleDefs := s.getFieldRuleDefs(rqff, name, val)
						pruleDefs.attach(name, ruleDefs)
						var required *bool
						if v := ruleDefs.hasRule("required"); v {
							required = &v
						}

						tInfo := s.getTypeInfo(rqff.Type, val, name, ruleDefs)
						optsObj.Parameters = append(
							optsObj.Parameters,
							newOpenapiParameter(in, name, required, tInfo),
						)
						sRules.setReq(sf.Name, pruleDefs)
					}

				case schemaBody:
					val := getPrimitiveValFromParent(obj, rqf)
					name := getFieldName(rqf)
					ruleDefs := s.getFieldRuleDefs(rqf, name, val)
					optsObj.bodySchema = s.getTypeInfo(rqf.Type, val, name, ruleDefs)
					sRules.setReq(sf.Name, ruleDefs)
				}
			}
		} else if _, ok := statuses[sf.Name]; ok {
			for _, rqf := range reflect.VisibleFields(sf.Type) {
				rqn := schemaField(rqf.Name)
				kind := rqf.Type.Kind()
				responseParameters := make(openapiParameters, 0, 10)

				switch rqn {
				case schemaHeaders, schemaCookies:
					if kind != reflect.Struct {
						continue
					}

					// ruleDefs := getFieldRuleDefs(rqf, string(rqn), nil)
					pruleDefs := newRuleDef(kind, string(rqn), rqf.Name, "", nil, nil, false, nil, nil, nil, nil)
					in := rqn.reqSchemaIn()

					for _, rqff := range reflect.VisibleFields(rqf.Type) {

						if rqn == schemaCookies && !utils.ValidCookieType(rqff.Type) {
							continue
						}

						val := getPrimitiveValFromParent(obj.FieldByName(rqf.Name), rqff)
						name := getFieldName(rqff)
						ruleDefs := s.getFieldRuleDefs(rqff, name, val)
						pruleDefs.attach(name, ruleDefs)
						var required *bool
						if v := ruleDefs.hasRule("required"); v {
							required = &v
						}

						tInfo := s.getTypeInfo(rqff.Type, val, name, ruleDefs)
						responseParameters = append(
							responseParameters,
							newOpenapiParameter(in, name, required, tInfo),
						)
						sRules.setResps(sf.Name, pruleDefs)
					}
					optsObj.responsesParameters[sf.Name] = responseParameters

				case schemaBody:
					val := getPrimitiveValFromParent(obj, rqf)
					name := getFieldName(rqf)
					ruleDefs := s.getFieldRuleDefs(rqf, name, val)
					optsObj.responsesSchema[sf.Name] = s.getTypeInfo(rqf.Type, val, name, ruleDefs)
					sRules.setResps(sf.Name, ruleDefs)
				}
			}
		}
	}

	return compiledSchema{
		specs: optsObj,
		rules: sRules,
	}
}

func (s *serveMux) getFieldRuleDefs(sf reflect.StructField, tagName string, defVal any) *ruleDef {
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
	var rules []ruleOpts
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
				rules = make([]ruleOpts, 0, len(vtags))
				for _, tag := range vtags {
					tagFieldRegex := regexp.MustCompile(`([a-zA-Z0-9_]+)(?:=([^,]+)|@([^,]+))?`)
					maches := tagFieldRegex.FindStringSubmatch(tag)
					ruleName := maches[1]
					optionStr := maches[2]

					if v := maches[3]; len(v) != 0 {
						if ts, ok := checkTagReference(v, sf.Type); ok {
							optionStr = ts
						}
					}

					var options []string
					if len(optionStr) > 1 {
						options = strings.Split(optionStr, " ")
					}

					if ruleName == "required" {
						required = true
					}

					if (ruleName == "max" || ruleName == "lte") && len(options) > 1 {
						flt, err := strconv.ParseFloat(options[0], 64)
						if err == nil {
							max = &flt
						}
					}

					rules = append(rules, newRuleOpts(sf.Type.Kind(), ruleName, options, s.opts))
				}
			}
		}
	}

	rtn := newRuleDef(sf.Type.Kind(), tagName, sf.Name, defStr, defVal, rules, required, max, nil, nil, nil)
	rtn.tags = tagList
	return rtn
}

func (s *serveMux) getTypeInfo(typ reflect.Type, value any, name string, ruleDefs *ruleDef) openapiSchema {

	kind := typ.Kind()

	var typeStr string
	var pattern string
	var format string
	var enum []any
	var optStr []string
	var min *float64
	var max *float64
	var items *openapiSchema
	var addProps *openapiSchema
	var example any
	var deprecated *bool
	var description string
	var specTag string
	properties := make(map[string]openapiSchema)
	requiredProps := make([]string, 0)

	var pRequired bool

	if ruleDefs != nil {
		minOpts := ruleDefs.ruleOptions("min")
		minOpts = append(minOpts, ruleDefs.ruleOptions("gte")...)
		for _, opt := range minOpts {
			i, err := strconv.ParseFloat(opt, 64)
			if err == nil {
				min = &i
				break
			}
		}

		maxOpts := ruleDefs.ruleOptions("max")
		maxOpts = append(maxOpts, ruleDefs.ruleOptions("lte")...)
		for _, opt := range maxOpts {
			i, err := strconv.ParseFloat(opt, 64)
			if err == nil {
				max = &i
				break
			}
		}

		// var items structFieldInfo
		optStr = ruleDefs.ruleOptions("oneof")
		pRequired = ruleDefs.required

		if v, ok := ruleDefs.tags["example"]; ok && len(v) > 0 {
			if v, err := utils.PrimitiveFromStr(typ.Kind(), v[0]); err == nil && utils.IsPrimitive(v) {
				example = v
			}
		}

		if v, ok := ruleDefs.tags["deprecated"]; ok && len(v) > 0 {
			if v, err := strconv.ParseBool(v[0]); err == nil && v {
				deprecated = &v
			}
		}

		if v, ok := ruleDefs.tags["description"]; ok && len(v) > 0 {
			description = v[0]
		}

		if v, ok := ruleDefs.tags["pattern"]; ok && len(v) > 0 {
			pattern = v[0]
		}

		if v, ok := ruleDefs.tags["spec"]; ok && len(v) > 0 {
			specTag = v[0]
		}
	}

	isCustom := false
	if v, ok := s.opts.customSpecs[specTag]; ok {
		enum = optsMapper(optStr, nil)
		typeStr = v.Type
		format = v.Format
		ruleDefs.format = utils.ObjectFormats(specTag)
		isCustom = true
	}

	if !isCustom {
		switch kind {
		case reflect.String:
			enum = optsMapper(optStr, nil)
			format = ruleDefs.findRules([]string{"date", "date-time", "password", "byte", "binary", "email", "uuid", "uri", "hostname", "ipv4", "ipv6"}, "")
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
			_ruleDefs := getItemRuleDef(typ.Elem())
			ruleDefs.append(_ruleDefs)
			i := s.getTypeInfo(typ.Elem(), value, name, _ruleDefs)
			items = &i

		case reflect.Map:
			typeStr = "object"
			_ruleDefs := getItemRuleDef(typ.Elem())
			ruleDefs.addProps(_ruleDefs)
			i := s.getTypeInfo(typ.Elem(), value, name, _ruleDefs)
			addProps = &i

		case reflect.Struct:
			switch typ {
			case utils.TimeType:
				enum = optsMapper(optStr, nil)
				typeStr = "string"
				format = string(utils.TimeObjectFormat)
				ruleDefs.format = utils.TimeObjectFormat
				if pattern == "" {
					pattern = time.RFC3339Nano
				}

			case utils.CookieType:
				enum = optsMapper(optStr, nil)
				typeStr = "string"
				format = string(utils.CookieObjectFormat)
				ruleDefs.format = utils.CookieObjectFormat

			default:
				typeStr = "object"
				obj := reflect.ValueOf(value)
				for _, sf := range reflect.VisibleFields(typ) {
					val := getPrimitiveValFromParent(obj, sf)
					name := getFieldName(sf)
					if name == "-" {
						continue
					}

					_ruleDefs := s.getFieldRuleDefs(sf, name, val)
					ruleDefs.attach(name, _ruleDefs)
					if _ruleDefs.hasRule("required") {
						requiredProps = append(requiredProps, name)
					}
					properties[name] = s.getTypeInfo(sf.Type, val, name, _ruleDefs)
				}

			}

		case reflect.Pointer:
			ruleDefs.kind = typ.Elem().Kind()
			return s.getTypeInfo(typ.Elem(), value, name, ruleDefs)
		}
	}

	ruleDefs.pattern = pattern

	return newOpenapiSchema(
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
		if fv.IsValid() && fv.Comparable() {
			fieldVal = fv.Interface()
			if kt := reflect.New(f.Type).Elem(); kt.IsValid() && kt.Comparable() {
				ktv := kt.Interface()
				if fieldVal != ktv {
					return fieldVal
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

type SpecialTypeIds = utils.ObjectFormats
