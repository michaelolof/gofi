package gofi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCompileSchema_QueryValidation(t *testing.T) {

	type testSchema struct {
		Request struct {
			Query struct {
				Email  string `json:"email"`
				UserId string `json:"user_id" validate:"oneof=1 2 5"`
			}
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []reqestTester{
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=2",
			check: func(e error) {
				assert.Nil(t, e)
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=29",
			check: func(e error) {
				assert.NotNil(t, e)
				assert.Equal(t, e.Error(), "given value '29' not supported at request Query(user_id)")
			},
		},
	}

	runRequestValidations(cases)
}

func TestCompileSchemaBodyValidation(t *testing.T) {

	type testSchema struct {
		Schema

		Request struct {
			Body int8 `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []reqestTester{
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body:   strings.NewReader(""),
			check: func(err error) {
				assert.NotNil(t, err)
				assert.Equal(t, err.Error(), `request body is required`)
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body:   strings.NewReader(strings.TrimSpace(`400000000000000000`)),
			check: func(err error) {
				fmt.Println(err)
				assert.NotNil(t, err)
				assert.Equal(t, err.Error(), `strconv.ParseInt: parsing "400000000000000000": value out of range`)
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body:   strings.NewReader(strings.TrimSpace(`400000000000000000`)),
			check: func(err error) {
				assert.NotNil(t, err)
				assert.Equal(t, err.Error(), `strconv.ParseInt: parsing "400000000000000000": value out of range`)
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body:   strings.NewReader(strings.TrimSpace(`4`)),
			check: func(err error) {
				assert.Nil(t, err)
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body:   strings.NewReader(strings.TrimSpace(`4.56`)),
			check: func(err error) {
				assert.NotNil(t, err)
				assert.Equal(t, err.Error(), `strconv.ParseInt: parsing "4.56": invalid syntax`)
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body:   strings.NewReader(strings.TrimSpace(`johndoe`)),
			check: func(err error) {
				assert.NotNil(t, err)
				assert.Equal(t, err.Error(), `strconv.ParseInt: parsing "johndoe": invalid syntax`)
			},
		},
	}

	runRequestValidations(cases)
}

func TestCompileSchema_ReqBodyValidation(t *testing.T) {

	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				One     string `json:"one" validate:"required,oneof=1 2 5"`
				Two     int    `json:"two" default:"30"`
				IsDone  bool   `json:"is_done" validate:"required,bool"`
				Contact struct {
					FirstName string `json:"firstname" validate:"required,knownNamer"`
					LastName  string `json:"lastname" validate:"required,namer"`
					User      struct {
						Id    int    `json:"id" validate:"required,int8"`
						Email string `json:"email"`
					} `json:"user" validate:"required"`
				} `json:"contact" validate:"required"`
				Ids   []int `json:"ids" validate:"required"`
				Users []struct {
					Id        uint8  `json:"id" validate:"required"`
					FirstName string `json:"firstname" validate:"required"`
					LastName  string `json:"lastname" validate:"required"`
				} `json:"users"`
			}
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []reqestTester{
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			check: func(err error) {
				assert.Nil(t, err) // Request
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`{
				"is_done": true,
				"contact": {
					"firstname": "Josh",
					"lastname": "Langely",
					"user": {
						"id": 20,
						"email": "master@mail.com"
					}
				},
				"ids": [1, 2, 3, 4, 5, 6, 7],
				"users": [
					{ "id": 1, "firstname": "Bastein", "lastname": "Amodi" },
					{ "id": 2, "firstname": "Gasong", "lastname": "Kruger" },
					{ "id": 3, "firstname": "Fastuni", "lastname": "Turner" }
				]
			}`)),
			check: func(err error) {
				assert.NotNil(t, err)
				unerrs := err.(interface{ Unwrap() []error })
				errs := unerrs.Unwrap()
				assert.Len(t, errs, 2)
				assert.Equal(t, errs[0].Error(), "value is required at request Body(one)")
				assert.Equal(t, errs[1].Error(), "given value '<nil>' not supported at request Body(one)")
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`{
				"one": "starting",
				"two": 67,
				"is_done": false,
				"contact": {
					"firstname": "Josh",
					"lastname": "Maxwell",
					"user": {
						"id": 60789,
						"email": "join@mail.com"
					}
				},
				"ids": [8, 9, 5, 4, 3, 8]
			}`)),
			check: func(err error) {
				assert.NotNil(t, err)
				assert.Equal(t, err.Error(), "given value 'starting' not supported at request Body(one)")
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`{
				"one": "5",
				"two": 67,
				"is_done": false,
				"contact": {
					"firstname": "Josh",
					"lastname": "Maxwell",
					"user": {
						"id": 60789,
						"email": "join@mail.com"
					}
				},
				"ids": [8, 9, 5, 4, 3, 8]
			}`)),
			check: func(err error) {
				assert.Nil(t, err)
			},
		},
	}

	runRequestValidations(cases)
}

func TestCompileSchema_ReqBodyArrPrimi(t *testing.T) {

	type testSchema struct {
		Schema

		Request struct {
			Body int `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []reqestTester{
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body:   strings.NewReader(strings.TrimSpace(`michaelolof`)),
			check: func(err error) {
				fmt.Println(err)
			},
		},
	}

	runRequestValidations(cases)
}

func TestCompileSchema_ReqBodyValidation2(t *testing.T) {

	type testSchema struct {
		Schema

		Request struct {
			Body [][]struct {
				One    int      `json:"one" validate:"required"`
				Two    int      `json:"two" validate:"required,email"`
				Doer   []string `json:"doer" validate:"required"`
				Starts struct {
					Main string `json:"main" validate:"required"`
					Gain string `json:"gain" validate:"required"`
					Max  struct {
						Tax string `json:"tax" validate:"required"`
					} `json:"max" validate:"required"`
				} `json:"starts"`
				Afro struct {
					Name    string `json:"name" validate:"namer"`
					Contact string `json:"contact" validate:"required,knownNamer"`
				} `json:"afro"`
			} `validate:"required"`
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []reqestTester{
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`[
				{
					"one": 1,
					"two": "standing@mail",
					"doer": ["person", "latitude", "cost"],
					"starts": {
						"main": "justing"
					},
					"afro": {
						"name": "John",
						"contact": "Hane"
					}
				}
			]`)),
			check: func(err error) {
				assert.Equal(t, err.Error(), "request body is required")
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`[[
				{
					"one": 1,
					"two": "standing@mail",
					"doer": ["person", "latitude", "cost"],
					"starts": {
						"main": "justing"
					},
					"afro": {
						"name": "John",
						"contact": "Hane"
					}
				}
			]]`)),
			check: func(err error) {
				assertErrOf(t, err, []string{
					"value doesn't contain number; it contains string at request Body(0.0.two)",
					"value is required at request Body(0.0.starts.gain)",
					"value is required at request Body(0.0.starts.max.tax)",
				})
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`[[
				{
					"one": 1,
					"two": "standing@mail",
					"doer": ["person", "latitude", "cost"],
					"afro": {
						"name": "John",
						"contact": "Hane"
					}
				}
			]]`)),
			check: func(err error) {
				assert.NotNil(t, err)
				assert.Equal(t, err.Error(), "value doesn't contain number; it contains string at request Body(0.0.two)")
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`[[
				{
					"one": "cars",
					"two": "standing@mail",
					"afro": {
						"name": "John",
						"contact": "Hane"
					}
				}
			]]`)),
			check: func(err error) {
				assertErrOf(t, err, []string{
					"value doesn't contain number; it contains string at request Body(0.0.one)",
					"value doesn't contain number; it contains string at request Body(0.0.two)",
					"value is required at request Body(0.0.doer)",
				})
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`[[
				{
					"one": 1,
					"two": "standing@mail",
					"doer": ["person", "latitude", "cost"],
					"starts": {
						"main": "one mainer",
						"gain": "two gains"
					},
					"afro": {
						"name": "John",
						"contact": "Hane"
					}
				}
			]]`)),
			check: func(err error) {
				assertErrOf(t, err, []string{
					"value is required at request Body(0.0.starts.max.tax)",
					"value doesn't contain number; it contains string at request Body(0.0.two)",
				})
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`[[
				{
					"one": 1,
					"two": "standing@mail",
					"doer": ["person", "latitude", "cost"],
					"starts": {
						"main": "one mainer",
						"gain": "two gains",
						"max": 30
					},
					"afro": {
						"name": "John",
						"contact": "Hane"
					}
				}
			]]`)),
			check: func(err error) {
				assertErrOf(t, err, []string{
					"value doesn't contain number; it contains string at request Body(0.0.two)",
					"value is required at request Body(0.0.starts.max.tax)",
				})
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`[[
				{
					"one": 1,
					"two": 34,
					"doer": ["person", "latitude", "cost"],
					"starts": {
						"main": "one mainer",
						"gain": "two gains",
						"max": {
							"tax": "a taxer"
						}
					},
					"afro": {
						"name": "John",
						"contact": "Hane"
					}
				}
			]]`)),
			check: func(err error) {
				assert.Nil(t, err)
			},
		},
	}

	runRequestValidations(cases)
}

func TestCompileSchema_ReqBodyValidation2point1(t *testing.T) {

	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				Passed  bool     `json:"passed" validate:"required"`
				Stage   string   `json:"stage"  validate:"oneof=actor massive blast"`
				Shifts  []string `json:"shifts" validate:"required"`
				Contact struct {
					FirstName string `json:"firstname"`
					LastName  string `json:"lastname"`
					Age       int    `json:"age"`
				} `json:"contact"`
				Users []struct {
					Id        int    `json:"int"`
					FirstName string `json:"firstname"`
					LastName  string `json:"lastname"`
				} `json:"users"`
			}
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []reqestTester{
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`{
					"passed": true,
					"stage": "actor",
					"shifts": ["one", "two", "three", "four"],
					"contact": {
						"firstname": "Judie",
						"lastname": "Akington",
						"age": 28
					},
					"users": [
						{ "id": 201, "firstname": "Joxier", "lastname": "Bennet" },
						{ "id": 202, "firstname": "Colex", "lastname": "Dinnor" },
						{ "id": 203, "firstname": "Pusher", "lastname": "Maxker" }
					]
			}`)),
			check: func(e error) {
				assert.Nil(t, e)
			},
		},
	}

	runRequestValidations(cases)
}

func TestCompileSchema_ReqBodyValidation3(t *testing.T) {

	type testSchema struct {
		Schema

		Request struct {
			Body []struct {
				Passed bool     `json:"passed" validate:"required"`
				Stage  string   `json:"stage"  validate:"oneof=actor massive blast"`
				Shifts []string `json:"shifts" validate:"required"`
			}
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []reqestTester{
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`[
				{
					"passed": true,
					"stage": "actorc",
					"shifts": ["one", "two", "three", "four"]
				},
				{
					"passed": false,
					"stage": "blast",
					"shifts": ["five", "six", "seven", "nine"]
				},
				{
					"passed": false,
					"stage": 344,
					"shifts": ["ten", "eleven", "thirteen", "fifteen"]
				}
			]`)),
			check: func(err error) {
				assertErrOf(t, err, []string{
					"value doesn't contain string; it contains number at request Body(2.stage)",
					"given value 'actorc' not supported at request Body(0.stage)",
				})
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`[
				{
					"passed": true,
					"stage": "actorc",
					"shifts": ["one", "two", "three", "four"]
				},
				{
					"passed": false,
					"stage": "blast",
					"shifts": ["five", "six", "seven", "nine"]
				},
				{
					"passed": false,
					"stage": "actorc",
					"shifts": ["ten", "eleven", "thirteen", "fifteen"]
				}
			]`)),
			check: func(err error) {
				assert.NotNil(t, err)
				assertErrOf(t, err, []string{
					"given value 'actorc' not supported at request Body(2.stage)",
					"given value 'actorc' not supported at request Body(0.stage)",
				})
			},
		},
		{
			schema: &rs,
			url:    "/api/test?email=one@mail.com&user_id=5",
			body: strings.NewReader(strings.TrimSpace(`[
				{
					"passed": true,
					"stage": "actor",
					"shifts": ["one", "two", "three", "four"]
				},
				{
					"passed": false,
					"shifts": ["five", "six", "seven", "nine"]
				},
				{
					"passed": false,
					"stage": "massive",
					"shifts": ["ten", "eleven", "thirteen", "fifteen"]
				}
			]`)),
			check: func(e error) {
				assert.Nil(t, e)
			},
		},
	}

	runRequestValidations(cases)
}

func TestCompileSchema_ReqBodyValidationSpecials(t *testing.T) {
	type testSchema struct {
		Request struct {
			Query struct {
				From time.Time `json:"from" pattern:"2006-01-02" validate:"required"`
			}

			Path struct {
				Id int `json:"id" validate:"required"`
			}

			Cookie struct {
				RefreshToken string       `json:"refresh-token"`
				Token        *http.Cookie `json:"x-token"`
				XId          int          `json:"x-id"`
			}

			Body struct {
				Kickoff time.Time `json:"kickoff" pattern:"2006-01-02" validate:"required"`
			}
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []responseTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test/:id?from=2024-06-13",
			paths: map[string]string{
				"id": "20",
			},
			cookies: []http.Cookie{
				{Name: "x-token", Value: "xxx-xxx-xxx-xxx"},
				{Name: "refresh-token", Value: "yyy-yyy-yyy-yyyy"},
				{Name: "x-id", Value: "14"},
			},
			method: "POST",
			body: strings.NewReader(`{
				"kickoff": "2024-02-27"
			}`),
			handler: func(c Context, s *testSchema) *testSchema {
				assert.NotNil(t, s.Request.Query.From)
				assert.Equal(t, 2024, s.Request.Query.From.Year())
				assert.Equal(t, "June", s.Request.Query.From.Month().String())
				assert.Equal(t, 13, s.Request.Query.From.Day())
				assert.Equal(t, "February", s.Request.Body.Kickoff.Month().String())
				assert.Equal(t, 27, s.Request.Body.Kickoff.Day())
				return s
			},
		},
	}

	runNewResponseValidations(t, cases)
}

func TestCompileSchema_SuccessValidation(t *testing.T) {

	type bodyContact struct {
		FirstName  string `json:"firstname" validate:"required" example:"John"`
		LastName   string `json:"lastname" validate:"required" example:"Doe"`
		IsVerified bool   `json:"is_verified"`
		Bio        string `json:"bio"`
	}

	type successBody struct {
		Email   string            `json:"email" validate:"email,required" example:"johndoe@mail.com" description:"The user's email"`
		Age     float64           `json:"age" validate:"required" example:"22"`
		Contact bodyContact       `json:"contact" description:"The person we want to contact"`
		Items   []int             `json:"items" validate:"max=20"`
		Infers  map[string]string `json:"infers"`
	}

	type testSchema struct {
		Schema

		Request struct {
			Body struct {
				Infers map[string]int `json:"infers"`
			} `validate:"required"`
		}

		Created struct {
			Headers struct {
				ContentType string `json:"Content-Type" validate:"required" default:"application/json"`
				Token       string `json:"x-access-token" validate:"required"`
				PlacementId int    `json:"placement-id" validate:"required"`
			}

			Body any `validate:"required"`
		}

		Accepted struct {
			Body *float32
		}

		Ok struct {
			Body successBody
		}

		Err struct {
			Headers struct {
				ContentType string `json:"Content-Type" default:"text/html"`
			}

			Body struct {
				Status  string `json:"status"`
				Message string `json:"message"`
				Data    any    `json:"data"`
			}
		}

		MovedPermanently struct {
			Headers struct {
				Location string `json:"Location" validate:"required"`
			}
		}

		Default struct {
			Body int
		}
	}

	schema := &testSchema{}
	rs := compileSchema(schema, Info{})

	cases := []responseTester[testSchema]{
		{
			schema: &rs,
			url:    "/api/test",
			handler: func(c Context, s *testSchema) *testSchema {
				s.Created.Headers.Token = "xf12345gh"
				s.Created.Headers.PlacementId = 12345
				s.Created.Body = successBody{
					Email: "johndoe@mail.com",
					Age:   22,
					Contact: bodyContact{
						FirstName: "John",
						LastName:  "Doe",
					},
				}

				err := c.JSON(201, s.Created)
				assert.Nil(t, err)
				return s
			},
			client: func(resp *http.Response, s *testSchema) {
				assert.Equal(t, resp.StatusCode, 201)

				assert.Equal(t, resp.Header.Get("content-type"), "application/json")
				assert.Equal(t, resp.Header.Get("x-access-token"), "xf12345gh")
				assert.Equal(t, resp.Header.Get("placement-id"), "12345")

				bs, err := io.ReadAll(resp.Body)
				assert.Nil(t, err)

				var bmap map[string]any
				err = json.Unmarshal(bs, &bmap)
				assert.Nil(t, err)
				assert.Equal(t, bmap["email"], "johndoe@mail.com")
				assert.Equal(t, bmap["age"], float64(22))
				assert.Equal(t, bmap["contact"], map[string]any{"firstname": "John", "lastname": "Doe"})
			},
		},
		{
			schema: &rs,
			url:    "/api/test",
			handler: func(c Context, s *testSchema) *testSchema {
				s.Accepted.Body = nil
				err := c.JSON(202, s.Accepted)
				assert.Nil(t, err)
				return s
			},
			client: func(res *http.Response, s *testSchema) {
				assert.Equal(t, res.StatusCode, 202)
				bs, err := io.ReadAll(res.Body)
				assert.Nil(t, err)
				assert.Empty(t, bs)
			},
		},
		{
			only:   true,
			schema: &rs,
			url:    "/api/test",
			body: strings.NewReader(`{
				"infers": {
					"one": 1,
					"two": 2,
					"three": 3
				}
			}`),
			handler: func(c Context, s *testSchema) *testSchema {
				s.Ok.Body = successBody{
					Email: "done@mail.com",
					Age:   22.45,
					Items: []int{10, 14, 23, 22, 90, 44, 21},
					Infers: map[string]string{
						"10": "conditional",
						"25": "terminator",
					},
					Contact: bodyContact{
						FirstName:  "Jon",
						LastName:   "Doe",
						IsVerified: true,
						Bio: `Something is up
						and I need to know "if" this is "it" and 'now'
						"https://www.goal.com/en/gb/?name=bayo&age=21"
						`,
					},
				}
				err := c.JSON(200, s.Ok)
				assert.Nil(t, err)
				return s
			},
			client: func(res *http.Response, s *testSchema) {
				bs, err := io.ReadAll(res.Body)
				assert.Nil(t, err)

				var bval successBody
				err = json.Unmarshal(bs, &bval)
				assert.Nil(t, err)
				assert.Equal(t, bval, s.Ok.Body)
			},
		},
	}

	runNewResponseValidations(t, cases)
}

func runRequestValidations(cases []reqestTester) {
	run := func(cs reqestTester) {
		method := "GET"
		if cs.method != "" {
			method = cs.method
		}

		r, _ := http.NewRequest(method, cs.url, cs.body)
		c := NewContext(httptest.NewRecorder(), r)
		cs.schema.specs.normalize(method, cs.url)
		c.setSchemaRules(&cs.schema.rules)

		err := Validate(c)
		if cs.check != nil {
			cs.check(err)
		}
	}

	var onlyCs *reqestTester = nil
	for _, cs := range cases {
		if cs.only {
			onlyCs = &cs
			break
		}
	}

	if onlyCs != nil {
		run(*onlyCs)
		return
	}

	for _, cs := range cases {
		if cs.ignore {
			continue
		}

		run(cs)
	}
}

func runNewResponseValidations[T ISchema](t *testing.T, cases []responseTester[T]) {
	vcases := make([]responseTester[T], 0, len(cases))
	for _, cs := range cases {
		if cs.only {
			vcases = append(vcases, cs)
		}
	}
	if len(vcases) == 0 {
		vcases = cases
	}

	for _, cs := range vcases {
		method := "GET"
		if cs.method != "" {
			method = cs.method
		}

		r, _ := http.NewRequest(method, cs.url, cs.body)
		for name, value := range cs.paths {
			r.SetPathValue(name, value)
		}

		for _, cookie := range cs.cookies {
			r.AddCookie(&cookie)
		}

		w := httptest.NewRecorder()
		c := NewContext(w, r)
		cs.schema.specs.normalize(method, cs.url)
		c.setSchemaRules(&cs.schema.rules)

		var ts *T
		if cs.handler != nil {
			s, err := ValidateAndBind[T](c)
			assert.Nil(t, err)
			ts = cs.handler(c, s)
		}

		if w != nil && cs.client != nil {
			cs.client(w.Result(), ts)
		}
	}
}

func assertErrOf(t *testing.T, err error, msgs []string) {
	assert.Contains(t, msgs, err.Error())
}

func assertErrList(t *testing.T, errs []error, msgs []string) {
	emsg := make([]string, 0, len(errs))

	for _, err := range errs {
		emsg = append(emsg, err.Error())
	}

	for i, msg := range msgs {
		assert.Contains(t, emsg, msg, fmt.Sprintf("error item #%d", i+1))
	}
}

type reqestTester struct {
	ignore bool
	only   bool
	url    string
	method string
	schema *routeSchema
	body   io.Reader
	check  func(error)
}

type errList interface {
	Unwrap() []error
}

type responseTester[T ISchema] struct {
	only    bool
	url     string
	paths   map[string]string
	cookies []http.Cookie
	method  string
	schema  *routeSchema
	body    io.Reader
	handler func(c Context, s *T) *T
	client  func(res *http.Response, s *T)
}
