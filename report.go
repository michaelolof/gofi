package gofi

import "fmt"

type errorType string

const (
	RequestErr  errorType = "Request"
	ResponseErr errorType = "Response"
)

type errReport struct {
	typ         errorType
	err         error
	category    string
	field       schemaField
	schemaValue string
	rule        string
}

func (e *errReport) Error() string {
	if e.schemaValue != "" {
		if e.typ == RequestErr {
			return fmt.Sprintf("%s at request %s(%s)", e.err.Error(), e.field, e.schemaValue)
		} else {
			return fmt.Sprintf("%s at response %s(%s)", e.err.Error(), e.field, e.schemaValue)
		}
	} else {
		return e.err.Error()
	}
}

func (e *errReport) Type() errorType {
	return e.typ
}

func (e *errReport) Category() string {
	return e.category
}

func (e *errReport) SchemaType() schemaField {
	return e.field
}

func (e *errReport) SchemaValue() string {
	return e.schemaValue
}

func (e *errReport) Rule() string {
	return e.rule
}

func newErrReport(typ errorType, schema schemaField, schemaValue string, rule string, err error) *errReport {
	return &errReport{
		typ:         typ,
		field:       schema,
		schemaValue: schemaValue,
		category:    "gofi_validation_error",
		rule:        rule,
		err:         err,
	}
}

type ValidationError interface {
	Type() errorType
	Error() string
	Category() string
	SchemaType() schemaField
	SchemaValue() string
	Rule() string
}
