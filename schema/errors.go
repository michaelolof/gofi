package schema

import "fmt"

type ErrorType string

const (
	RequestErr  ErrorType = "Request"
	ResponseErr ErrorType = "Response"
)

type ErrReport struct {
	Typ         ErrorType
	Err         error
	Category    string
	Field       SchemaField
	SchemaValue string
	Rule        string
}

func (e *ErrReport) Error() string {
	if e.SchemaValue != "" {
		if e.Typ == RequestErr {
			return fmt.Sprintf("%s at request %s(%s)", e.Err.Error(), e.Field, e.SchemaValue)
		} else {
			return fmt.Sprintf("%s at response %s(%s)", e.Err.Error(), e.Field, e.SchemaValue)
		}
	} else {
		return e.Err.Error()
	}
}

func (e *ErrReport) Type() ErrorType {
	return e.Typ
}

func (e *ErrReport) ErrCategory() string {
	return e.Category
}

func (e *ErrReport) SchemaType() SchemaField {
	return e.Field
}

func (e *ErrReport) SchemaVal() string {
	return e.SchemaValue
}

func (e *ErrReport) RuleName() string {
	return e.Rule
}

func NewErrReport(typ ErrorType, schema SchemaField, schemaValue string, rule string, err error) *ErrReport {
	return &ErrReport{
		Typ:         typ,
		Field:       schema,
		SchemaValue: schemaValue,
		Category:    "gofi_validation_error",
		Rule:        rule,
		Err:         err,
	}
}

type ValidationError interface {
	Type() ErrorType
	Error() string
	ErrCategory() string
	SchemaType() SchemaField
	SchemaVal() string
	RuleName() string
}
