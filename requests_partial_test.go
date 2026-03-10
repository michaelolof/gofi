package gofi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/michaelolof/gofi/utils"
	"github.com/stretchr/testify/assert"
)

func TestValidateAndBind_PartialBodyAndCookie(t *testing.T) {
	type testSchema struct {
		Request struct {
			Header struct {
				Token string `json:"x-token" validate:"required"`
			}
			Cookie struct {
				Session string `json:"session" validate:"required"`
			}
			Body struct {
				Name string `json:"name" validate:"required"`
			} `validate:"required"`
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			s, err := ValidateAndBind[testSchema](c, Cookie)
			assert.NoError(t, err)
			if err != nil {
				return nil
			}

			assert.Equal(t, "abc123", s.Request.Cookie.Session)
			assert.Equal(t, "Jane", s.Request.Body.Name)
			assert.Equal(t, "", s.Request.Header.Token)
			return nil
		},
	}

	m := newRouter()
	_, err := m.Inject(InjectOptions{
		Path:   "/partial",
		Method: http.MethodPost,
		Headers: map[string]string{
			"x-token": "xxxxxxx",
		},
		Cookies: []http.Cookie{
			{Name: "session", Value: "abc123"},
		},
		Body:    utils.TryAsReader(map[string]any{"name": "Jane"}),
		Handler: &handler,
	})
	assert.NoError(t, err)
}

func TestValidateAndBind_PartialDoesNotReuseFullCache(t *testing.T) {
	type testSchema struct {
		Request struct {
			Header struct {
				Token string `json:"x-token" validate:"required"`
			}
			Body struct {
				Name string `json:"name" validate:"required"`
			} `validate:"required"`
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			_, err := ValidateAndBind[testSchema](c, Body)
			assert.NoError(t, err)

			_, err = ValidateAndBind[testSchema](c)
			assert.Error(t, err)
			return nil
		},
	}

	m := newRouter()
	_, err := m.Inject(InjectOptions{
		Path:    "/cache",
		Method:  http.MethodPost,
		Body:    utils.TryAsReader(map[string]any{"name": "Jane"}),
		Handler: &handler,
	})
	assert.NoError(t, err)
}

func TestValidate_InvalidSelector(t *testing.T) {
	type testSchema struct {
		Request struct {
			Body map[string]any `validate:"required"`
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			err := Validate(c, RequestSchema("request.unknown"))
			assert.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), "unsupported request schema selector"))
			return nil
		},
	}

	m := newRouter()
	_, err := m.Inject(InjectOptions{
		Path:    "/invalid-selector",
		Method:  http.MethodPost,
		Body:    strings.NewReader(`{"ok":true}`),
		Handler: &handler,
	})
	assert.NoError(t, err)
}

func TestValidate_DuplicateSelectors(t *testing.T) {
	type testSchema struct {
		Request struct {
			Body struct {
				Count int `json:"count" validate:"required"`
			} `validate:"required"`
		}
	}

	handler := RouteOptions{
		Schema: &testSchema{},
		Handler: func(c Context) error {
			err := Validate(c, Body, Body)
			assert.NoError(t, err)
			return nil
		},
	}

	m := newRouter()
	_, err := m.Inject(InjectOptions{
		Path:    "/dupes",
		Method:  http.MethodPost,
		Body:    utils.TryAsReader(map[string]any{"count": 2}),
		Handler: &handler,
	})
	assert.NoError(t, err)
}
