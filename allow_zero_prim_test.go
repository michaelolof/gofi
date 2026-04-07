package gofi

import (
	"testing"

	"github.com/michaelolof/gofi/utils"
	"github.com/stretchr/testify/assert"
)

// TestRequest_AllowZero_PrimitiveSlices confirms allow_zero/present work on
// primitive slice types ([]string, []int) and top-level slice bodies.
func TestRequest_AllowZero_PrimitiveSlices(t *testing.T) {
	t.Run("[]string field empty with allow_zero", func(t *testing.T) {
		type testSchema struct {
			Request struct {
				Body struct {
					Tags []string `json:"tags" validate:"required,allow_zero"`
				} `validate:"required"`
			}
		}
		m := NewRouter()
		m.Inject(InjectOptions{
			Path:   "/test",
			Method: "POST",
			Body:   utils.TryAsReader(map[string]any{"tags": []any{}}),
			Handler: &RouteOptions{
				Schema: &testSchema{},
				Handler: func(c Context) error {
					s, err := ValidateAndBind[testSchema](c)
					assert.Nil(t, err, "empty []string with allow_zero should not error: %v", err)
					assert.NotNil(t, s)
					return nil
				},
			},
		})
	})

	t.Run("[]int field empty with allow_zero", func(t *testing.T) {
		type testSchema struct {
			Request struct {
				Body struct {
					Nums []int `json:"nums" validate:"required,allow_zero"`
				} `validate:"required"`
			}
		}
		m := NewRouter()
		m.Inject(InjectOptions{
			Path:   "/test",
			Method: "POST",
			Body:   utils.TryAsReader(map[string]any{"nums": []any{}}),
			Handler: &RouteOptions{
				Schema: &testSchema{},
				Handler: func(c Context) error {
					s, err := ValidateAndBind[testSchema](c)
					assert.Nil(t, err, "empty []int with allow_zero should not error: %v", err)
					assert.NotNil(t, s)
					return nil
				},
			},
		})
	})

	t.Run("[]string field empty with present", func(t *testing.T) {
		type testSchema struct {
			Request struct {
				Body struct {
					Tags []string `json:"tags" validate:"present"`
				} `validate:"required"`
			}
		}
		m := NewRouter()
		m.Inject(InjectOptions{
			Path:   "/test",
			Method: "POST",
			Body:   utils.TryAsReader(map[string]any{"tags": []any{}}),
			Handler: &RouteOptions{
				Schema: &testSchema{},
				Handler: func(c Context) error {
					s, err := ValidateAndBind[testSchema](c)
					assert.Nil(t, err, "empty []string with present should not error: %v", err)
					assert.NotNil(t, s)
					return nil
				},
			},
		})
	})

	// allow_zero does NOT suppress explicit min/gte rules.
	// This test confirms min=1 still rejects empty slices even with allow_zero.
	t.Run("[]string empty with allow_zero AND min=1 still errors", func(t *testing.T) {
		type testSchema struct {
			Request struct {
				Body struct {
					Tags []string `json:"tags" validate:"required,min=1,allow_zero"`
				} `validate:"required"`
			}
		}
		m := NewRouter()
		m.Inject(InjectOptions{
			Path:   "/test",
			Method: "POST",
			Body:   utils.TryAsReader(map[string]any{"tags": []any{}}),
			Handler: &RouteOptions{
				Schema: &testSchema{},
				Handler: func(c Context) error {
					_, err := ValidateAndBind[testSchema](c)
					// min=1 fires even with allow_zero — to allow empty, remove min=1
					assert.NotNil(t, err, "min=1 should still fire for empty slice")
					return nil
				},
			},
		})
	})
}

func TestResponse_AllowZero_Slices(t *testing.T) {
	t.Run("[]string field empty with allow_zero", func(t *testing.T) {
		type testSchema struct {
			Ok struct {
				Body struct {
					Tags []string `json:"tags" validate:"required,allow_zero"`
				} `validate:"required"`
			}
		}
		m := NewRouter()
		rec, err := m.Inject(InjectOptions{
			Path:   "/test",
			Method: "POST",
			Body:   utils.TryAsReader(map[string]any{"tags": []any{}}),
			Handler: &RouteOptions{
				Schema: &testSchema{},
				Handler: func(c Context) error {
					var s testSchema
					r := c.Send(200, s.Ok)
					assert.NoError(t, r)
					return r
				},
			},
		})

		assert.NoError(t, err)
		assert.Contains(t, rec.BodyString(), `"tags":[]`)
	})
}
