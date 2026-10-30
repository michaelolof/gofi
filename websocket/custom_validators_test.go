package websocket_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	fasthttpws "github.com/fasthttp/websocket"
	"github.com/michaelolof/gofi"
	gofiws "github.com/michaelolof/gofi/websocket"
)

// roomValidator accepts strings that start with "room-".
type roomValidator struct{}

func (roomValidator) Name() string { return "room" }

func (roomValidator) Rule(c gofi.ValidatorContext) func(val any) error {
	return func(val any) error {
		if s, ok := val.(string); ok && strings.HasPrefix(s, "room-") {
			return nil
		}
		return errors.New("value must start with room-")
	}
}

type joinMessage struct {
	Room string `json:"room" validate:"required,room"`
}

// exchange sends one message to the socket at url and returns the reply.
func exchange(t *testing.T, url, message string) string {
	t.Helper()
	conn, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(fasthttpws.TextMessage, []byte(message)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	_, reply, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	return string(reply)
}

func serve(t *testing.T, r gofi.Router) string {
	t.Helper()
	addr := freeAddr(t)
	go func() { _ = r.Listen(addr) }()
	time.Sleep(120 * time.Millisecond)
	t.Cleanup(func() { _ = r.Shutdown() })
	return fmt.Sprintf("ws://%s/ws", addr)
}

func echoRoom(s *gofiws.Session) error {
	var msg joinMessage
	if err := s.ReadJSON(&msg); err != nil {
		return s.WriteMessage(gofiws.TextMessage, []byte("rejected: "+err.Error()))
	}
	return s.WriteMessage(gofiws.TextMessage, []byte("joined "+msg.Room))
}

func TestWebSocketContract_UsesRouterCustomValidators(t *testing.T) {
	type socketSchema struct {
		WebSocket gofi.WebSocketSchema
	}

	r := gofi.NewRouter()
	r.RegisterValidator(roomValidator{})
	r.Get("/ws", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
		Schema: &socketSchema{WebSocket: gofi.WebSocketSchema{
			Inbound: gofi.WebSocketMessageFamily{
				Variants: []gofi.WebSocketMessage{{Type: "join", Schema: joinMessage{}}},
			},
		}},
		Handler: echoRoom,
	}))
	url := serve(t, r)

	if got := exchange(t, url, `{"room":"room-1"}`); got != "joined room-1" {
		t.Fatalf("valid room: got %q", got)
	}
	if got := exchange(t, url, `{"room":"lobby"}`); !strings.Contains(got, "must start with room-") {
		t.Fatalf("invalid room: got %q, want the custom rule's error", got)
	}
	if got := exchange(t, url, `{"type":"join","payload":{"room":"room-3"}}`); got != "joined room-3" {
		t.Fatalf("valid room in envelope: got %q", got)
	}
	if got := exchange(t, url, `{"type":"join","payload":{"room":"lobby"}}`); !strings.Contains(got, "must start with room-") {
		t.Fatalf("invalid room in envelope: got %q, want the custom rule's error", got)
	}
}

func TestWebSocketOptionsSchema_UsesRouterCustomValidators(t *testing.T) {
	r := gofi.NewRouter()
	r.RegisterValidator(roomValidator{})
	r.Get("/ws", gofi.RouteOptions{
		Handler: gofiws.NewWithSessionAndOptions(echoRoom, gofiws.Options{InboundSchema: joinMessage{}}),
	})
	url := serve(t, r)

	if got := exchange(t, url, `{"room":"room-2"}`); got != "joined room-2" {
		t.Fatalf("valid room: got %q", got)
	}
	if got := exchange(t, url, `{"room":"lobby"}`); !strings.Contains(got, "must start with room-") {
		t.Fatalf("invalid room: got %q, want the custom rule's error", got)
	}
}

func TestWebSocketContract_RejectsUnknownRuleAtRegistration(t *testing.T) {
	type badMessage struct {
		Items []string `json:"items" validate:"required,dive"`
	}
	type socketSchema struct {
		WebSocket gofi.WebSocketSchema
	}

	defer func() {
		got := fmt.Sprint(recover())
		if !strings.Contains(got, `unknown validation rule "dive"`) || !strings.Contains(got, "field Items") {
			t.Fatalf("got panic %q, want unknown rule panic naming the field", got)
		}
	}()
	gofi.NewRouter().Get("/ws", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
		Schema: &socketSchema{WebSocket: gofi.WebSocketSchema{
			Inbound: gofi.WebSocketMessageFamily{
				Variants: []gofi.WebSocketMessage{{Type: "bad", Schema: badMessage{}}},
			},
		}},
		Handler: echoRoom,
	}))
}
