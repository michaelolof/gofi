package gofi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSpecialResponses(t *testing.T) {

	type testSchema struct {
		Ok struct {
			Body struct {
				CreatedAt time.Time `json:"created_at"`
			}
		}
	}

	rs := compileSchema(&testSchema{}, Info{})

	cases := []responseTester[testSchema]{
		{
			schema: &rs,
			handler: func(c Context, s *testSchema) *testSchema {
				s.Ok.Body.CreatedAt = time.Now()
				c.JSON(200, s.Ok)
				return s
			},
			client: func(res *http.Response, s *testSchema) {
				bs, err := io.ReadAll(res.Body)
				assert.NotNil(t, err)

				var data map[string]any
				_ = json.Unmarshal(bs, &data)
				fmt.Println("done")
			},
		},
	}

	runNewResponseValidations(t, cases)

}
