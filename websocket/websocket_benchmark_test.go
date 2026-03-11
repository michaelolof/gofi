package websocket

import (
	"encoding/json"
	"testing"
)

type benchInbound struct {
	Message string `json:"message" validate:"required,min=2,max=64"`
	Topic   string `json:"topic" validate:"required,alpha"`
	Count   int    `json:"count" validate:"gte=1,lte=1000"`
}

func BenchmarkValidatePayloadSchemaCached(b *testing.B) {
	payload := benchInbound{Message: "hello", Topic: "general", Count: 10}
	schema := benchInbound{}

	for i := 0; i < b.N; i++ {
		if err := validatePayloadSchema(payload, schema, "inbound"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidatePayloadSchemaCachedPointer(b *testing.B) {
	payload := &benchInbound{Message: "hello", Topic: "general", Count: 10}
	schema := &benchInbound{}

	for i := 0; i < b.N; i++ {
		if err := validatePayloadSchema(payload, schema, "inbound"); err != nil {
			b.Fatal(err)
		}
	}
}

type benchError struct {
	Message string `json:"message"`
}

func BenchmarkWriteErrorObjectMarshal(b *testing.B) {
	payload := benchError{Message: "field 'message' is required"}

	for i := 0; i < b.N; i++ {
		if _, err := marshalJSON(payload); err != nil {
			b.Fatal(err)
		}
	}
}

// marshalJSON is isolated to keep benchmark focused on the write path used by WriteError.
func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
