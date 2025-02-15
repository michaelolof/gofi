# gofi (In Development)

Gofi is an openapi3 schema-based http router for Golang.

Features
- Designed to be performant.
- Simple and developer friendly interface.
- Automatic request and response validation based on your schema.
- Automatic API documentation with support for StopLight, Swagger, RapidDoc, Redocly, Scalar documentations and a developer friendly interface to bring in your own custom documentation.
- General error handling
- Support for documenting, encoding and decoding custom types
<br/>
<br/>

Installation
```sh
go get -u github.com/michaelolof/gofi
```
<br/>
<br/>

Example
```go
import (
	"net/http"
	"time"

    "github.com/michaelolof/gofi"
	"github.com/rs/cors"
)



type PingOkBody struct {
	Status    string `json:"status" validate:"oneof=success,error"`
	Message   string `json:"message" validate:"required"`
	TimeStamp int    `json:"timestamp" validate:"required"`
}

type pingSchema struct {
	Request struct {
		Body struct {
			Email    string `json:"email" validate:"required,email"`
			Location string `json:"location" validate:"min=10"`
		}
	}

	Ok struct {
		Header struct {
			ContentType string `json:"content-type" default:"application/json"`
		}

		Body PingOkBody `validate:"required"`
	}
}

var PingHandler = gofi.DefineHandler(gofi.RouteOptions{

	Schema: &pingSchema{},

	Handler: func(c gofi.Context) error {
		s, err := gofi.ValidateAndBind[pingSchema](c)
		if err != nil {
			return err
		}

		// Access user email and location
		email := s.Request.Body.Email
		location := s.Request.Body.Location
		fmt.Printf("User %s pinging from %s\n", email, location)

		s.Ok.Body = PingOkBody{
			Status:    "success",
			Message:   "Awesome service up and grateful",
			TimeStamp: int(time.Now().Unix()),
		}

		return c.Send(200, s.Ok)
	},
})
	
func main() {
    
    mux := gofi.NewServeMux()

    err := gofi.ServeDocs(r, gofi.DocsOptions{
		Info: gofi.DocsInfoOptions{
			Title:       "My Awesome Service",
			Version:     "0.0.1",
			Description: "An extremely awesome service",
		},
		Views: []gofi.DocsView{
			{
				RoutePrefix: "/api-docs",
				Template:    gofi.StopLight(),
			},
		},
	})
	if err != nil {
		log.Fatalln(err)
	}

	r.Use(cors.AllowAll().Handler)

    r.GET("/api/ping", PingHandler)

	http.ListenAndServe(":4100", mux)
}   
```

Openapi3 documentation will be served on http://localhost:4200/api-docs

![Ping documentation screenshot](./docs/img/gofi_ping_doc.png)

**Please note:** gofi is still in development and APIs are subject to change. Do not use in production.