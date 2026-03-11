package gofi

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestStream(t *testing.T) {
	mux := NewRouter()

	type streamSchema struct {
		Ok struct {
			Header struct {
				ContentType  string `json:"content-type" validate:"required" default:"text/event-stream"`
				Connection   string `json:"connection" default:"keep-alive"`
				CacheControl string `json:"cache-control" default:"no-cache"`
			}
			Body string `validate:"required"`
		}
	}

	mux.Get("/events", RouteOptions{
		Schema: &streamSchema{},
		Handler: func(c Context) error {
			var s streamSchema
			return c.SendStream(200, s.Ok, func(w *bufio.Writer) error {
				for i := 0; i < 3; i++ {
					if _, err := fmt.Fprintf(w, "data: event %d\n\n", i); err != nil {
						return err
					}
					if err := w.Flush(); err != nil {
						return err
					}
					time.Sleep(10 * time.Millisecond)
				}
				return nil
			})
		},
	})

	port := 38473
	addr := fmt.Sprintf(":%d", port)

	go func() {
		_ = mux.Listen(addr)
	}()

	time.Sleep(100 * time.Millisecond) // Let server start

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/events", port))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected Content-Type text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("Expected Cache-Control no-cache, got %s", resp.Header.Get("Cache-Control"))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}

	expected := "data: event 0\n\ndata: event 1\n\ndata: event 2\n\n"
	if string(body) != expected {
		t.Errorf("Expected body '%s', got '%s'", expected, string(body))
	}

	_ = mux.Shutdown()
}
