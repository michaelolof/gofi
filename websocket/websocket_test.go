package websocket_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	fasthttpws "github.com/fasthttp/websocket"
	"github.com/michaelolof/gofi"
	gofiws "github.com/michaelolof/gofi/websocket"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func TestWebSocketUpgradeRequired(t *testing.T) {
	mux := gofi.NewRouter()
	mux.Get("/ws", gofi.RouteOptions{
		Handler: gofiws.New(func(conn *gofiws.Conn) error {
			return conn.WriteMessage(gofiws.TextMessage, []byte("ok"))
		}),
	})

	resp := mux.Test("GET", "/ws")
	if resp.StatusCode != 426 {
		t.Fatalf("expected status 426, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "websocket upgrade required") {
		t.Fatalf("expected upgrade required message, got %q", string(resp.Body))
	}
}

func TestWebSocketNewWithContext(t *testing.T) {
	mux := gofi.NewRouter()
	mux.Get("/ws/:room", gofi.RouteOptions{
		Handler: gofiws.NewWithContext(func(c gofi.Context, conn *gofiws.Conn) error {
			room := c.Param("room")
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return err
			}
			return conn.WriteMessage(mt, []byte(room+":"+string(msg)))
		}),
	})

	addr := freeAddr(t)
	go func() {
		_ = mux.Listen(addr)
	}()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws/alpha", addr)
	conn, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(fasthttpws.TextMessage, []byte("hello")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(msg) != "alpha:hello" {
		t.Fatalf("expected alpha:hello, got %s", string(msg))
	}
}

func TestWebSocketNewWithSession(t *testing.T) {
	mux := gofi.NewRouter()
	mux.Get("/ws/:room", gofi.RouteOptions{
		Handler: gofiws.NewWithSession(func(s *gofiws.Session) error {
			room := s.Context().Param("room")
			mt, msg, err := s.ReadMessage()
			if err != nil {
				return err
			}
			return s.WriteMessage(mt, []byte("["+room+"] "+string(msg)))
		}),
	})

	addr := freeAddr(t)
	go func() {
		_ = mux.Listen(addr)
	}()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws/beta", addr)
	conn, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(fasthttpws.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(msg) != "[beta] ping" {
		t.Fatalf("expected [beta] ping, got %s", string(msg))
	}
}

func TestWebSocketHandshakeSelectiveValidation(t *testing.T) {
	type handshakeSchema struct {
		Request struct {
			Header struct {
				Token string `json:"x-token" validate:"required"`
			}
		}
	}

	mux := gofi.NewRouter()
	mux.Get("/ws", gofi.RouteOptions{
		Schema: &handshakeSchema{},
		Handler: gofiws.NewWithSessionAndOptions(func(s *gofiws.Session) error {
			return s.WriteMessage(gofiws.TextMessage, []byte("ok"))
		}, gofiws.Options{
			HandshakeSelectors: []gofi.RequestSchema{gofi.Header},
		}),
	})

	addr := freeAddr(t)
	go func() {
		_ = mux.Listen(addr)
	}()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws", addr)
	if conn, _, err := fasthttpws.DefaultDialer.Dial(url, nil); err == nil {
		_ = conn.Close()
		t.Fatalf("expected dial to fail without required x-token header")
	}

	h := make(http.Header)
	h.Set("x-token", "abc123")
	conn, _, err := fasthttpws.DefaultDialer.Dial(url, h)
	if err != nil {
		t.Fatalf("dial with token failed: %v", err)
	}
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(msg) != "ok" {
		t.Fatalf("expected ok, got %s", string(msg))
	}
}

func TestWebSocketSessionJSONValidationAndErrors(t *testing.T) {
	type inbound struct {
		Message string `json:"message" validate:"required,min=2"`
	}

	type outbound struct {
		Reply string `json:"reply" validate:"required,min=2"`
	}

	type errFrame struct {
		Message string `json:"message" validate:"required"`
	}

	mux := gofi.NewRouter()
	mux.Get("/ws", gofi.RouteOptions{
		Handler: gofiws.NewWithSessionAndOptions(func(s *gofiws.Session) error {
			var req inbound
			if err := s.ReadJSON(&req); err != nil {
				_ = s.WriteError(errFrame{Message: err.Error()})
				return nil
			}

			if req.Message == "bad-out" {
				if err := s.WriteJSON(outbound{}); err != nil {
					_ = s.WriteError(errFrame{Message: err.Error()})
					return nil
				}
			}

			return s.WriteJSON(outbound{Reply: "pong"})
		}, gofiws.Options{
			InboundSchema:  inbound{},
			OutboundSchema: outbound{},
		}),
	})

	addr := freeAddr(t)
	go func() {
		_ = mux.Listen(addr)
	}()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws", addr)

	// Valid inbound/outbound flow
	conn1, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial 1 failed: %v", err)
	}
	if err := conn1.WriteMessage(fasthttpws.TextMessage, []byte(`{"message":"ping"}`)); err != nil {
		_ = conn1.Close()
		t.Fatalf("write 1 failed: %v", err)
	}
	_, msg1, err := conn1.ReadMessage()
	_ = conn1.Close()
	if err != nil {
		t.Fatalf("read 1 failed: %v", err)
	}

	var okResp outbound
	if err := json.Unmarshal(msg1, &okResp); err != nil {
		t.Fatalf("unmarshal 1 failed: %v", err)
	}
	if okResp.Reply != "pong" {
		t.Fatalf("expected pong, got %s", okResp.Reply)
	}

	// Inbound validation failure
	conn2, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial 2 failed: %v", err)
	}
	if err := conn2.WriteMessage(fasthttpws.TextMessage, []byte(`{"message":""}`)); err != nil {
		_ = conn2.Close()
		t.Fatalf("write 2 failed: %v", err)
	}
	_, msg2, err := conn2.ReadMessage()
	_ = conn2.Close()
	if err != nil {
		t.Fatalf("read 2 failed: %v", err)
	}

	var errResp errFrame
	if err := json.Unmarshal(msg2, &errResp); err != nil {
		t.Fatalf("unmarshal 2 failed: %v", err)
	}
	if errResp.Message == "" {
		t.Fatalf("unexpected inbound error frame: %+v", errResp)
	}

	// Outbound validation failure
	conn3, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial 3 failed: %v", err)
	}
	if err := conn3.WriteMessage(fasthttpws.TextMessage, []byte(`{"message":"bad-out"}`)); err != nil {
		_ = conn3.Close()
		t.Fatalf("write 3 failed: %v", err)
	}
	_, msg3, err := conn3.ReadMessage()
	_ = conn3.Close()
	if err != nil {
		t.Fatalf("read 3 failed: %v", err)
	}

	errResp = errFrame{}
	if err := json.Unmarshal(msg3, &errResp); err != nil {
		t.Fatalf("unmarshal 3 failed: %v", err)
	}
	if errResp.Message == "" {
		t.Fatalf("unexpected outbound error frame: %+v", errResp)
	}
}

func TestWebSocketWriteErrorObject(t *testing.T) {
	type customError struct {
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Detail string `json:"detail"`
	}

	mux := gofi.NewRouter()
	mux.Get("/ws", gofi.RouteOptions{
		Handler: gofiws.NewWithSessionAndOptions(func(s *gofiws.Session) error {
			return s.WriteError(customError{
				Kind:   "ws_error",
				ID:     "invalid_room",
				Detail: "room not found",
			})
		}, gofiws.Options{}),
	})

	addr := freeAddr(t)
	go func() {
		_ = mux.Listen(addr)
	}()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws", addr)
	conn, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var resp customError
	if err := json.Unmarshal(msg, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if resp.Kind != "ws_error" || resp.ID != "invalid_room" || resp.Detail != "room not found" {
		t.Fatalf("unexpected custom error payload: %+v", resp)
	}
}

func TestDefineWebSocketSchemaDrivenContracts(t *testing.T) {
	type inbound struct {
		Message string `json:"message" validate:"required,min=2"`
	}

	type outbound struct {
		Reply string `json:"reply" validate:"required,min=2"`
	}

	type socketError struct {
		Message string `json:"message" validate:"required"`
	}

	type socketSchema struct {
		WebSocket gofi.WebSocketSchema
	}

	schema := &socketSchema{
		WebSocket: gofi.WebSocketSchema{
			Inbound: gofi.WebSocketMessageFamily{
				Variants: []gofi.WebSocketMessage{{Type: "inbound", Schema: inbound{}}},
			},
			Outbound: gofi.WebSocketMessageFamily{
				Variants: []gofi.WebSocketMessage{{Type: "outbound", Schema: outbound{}}},
			},
			Error: gofi.WebSocketMessageFamily{
				Variants: []gofi.WebSocketMessage{{Type: "error", Schema: socketError{}}},
			},
		},
	}

	mux := gofi.NewRouter()
	mux.Get("/ws", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
		Schema:    schema,
		Handshake: gofiws.HandshakePolicy{Mode: gofiws.HandshakeAuto},
		Handler: func(s *gofiws.Session) error {
			var req inbound
			if err := s.ReadJSON(&req); err != nil {
				return s.WriteError(socketError{Message: err.Error()})
			}

			if req.Message == "bad-out" {
				if err := s.WriteJSON(outbound{}); err != nil {
					return s.WriteError(socketError{Message: err.Error()})
				}
			}

			return s.WriteJSON(outbound{Reply: "pong"})
		},
	}))

	addr := freeAddr(t)
	go func() { _ = mux.Listen(addr) }()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws", addr)

	conn1, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial 1 failed: %v", err)
	}
	if err := conn1.WriteMessage(fasthttpws.TextMessage, []byte(`{"message":"ping"}`)); err != nil {
		_ = conn1.Close()
		t.Fatalf("write 1 failed: %v", err)
	}
	_, msg1, err := conn1.ReadMessage()
	_ = conn1.Close()
	if err != nil {
		t.Fatalf("read 1 failed: %v", err)
	}

	var okResp outbound
	if err := json.Unmarshal(msg1, &okResp); err != nil {
		t.Fatalf("unmarshal 1 failed: %v", err)
	}
	if okResp.Reply != "pong" {
		t.Fatalf("expected pong, got %s", okResp.Reply)
	}

	conn2, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial 2 failed: %v", err)
	}
	if err := conn2.WriteMessage(fasthttpws.TextMessage, []byte(`{"message":""}`)); err != nil {
		_ = conn2.Close()
		t.Fatalf("write 2 failed: %v", err)
	}
	_, msg2, err := conn2.ReadMessage()
	_ = conn2.Close()
	if err != nil {
		t.Fatalf("read 2 failed: %v", err)
	}

	var errResp socketError
	if err := json.Unmarshal(msg2, &errResp); err != nil {
		t.Fatalf("unmarshal 2 failed: %v", err)
	}
	if errResp.Message == "" {
		t.Fatalf("expected schema-driven error response")
	}
}

func TestDefineWebSocketCustomDiscriminatorEnvelope(t *testing.T) {
	type joinPayload struct {
		Nickname string `json:"nickname" validate:"required,min=2"`
	}

	type joinedPayload struct {
		Room string `json:"room" validate:"required"`
	}

	type inboundEnvelope struct {
		Kind    string      `json:"kind" validate:"required"`
		Payload joinPayload `json:"payload" validate:"required"`
	}

	type outboundEnvelope struct {
		Kind    string        `json:"kind" validate:"required"`
		Payload joinedPayload `json:"payload" validate:"required"`
	}

	type socketSchema struct {
		WebSocket gofi.WebSocketSchema
	}

	schema := &socketSchema{
		WebSocket: gofi.WebSocketSchema{
			Inbound: gofi.WebSocketMessageFamily{
				Discriminator: "kind",
				Variants:      []gofi.WebSocketMessage{{Type: "join", Schema: joinPayload{}}},
			},
			Outbound: gofi.WebSocketMessageFamily{
				Discriminator: "kind",
				Variants:      []gofi.WebSocketMessage{{Type: "joined", Schema: joinedPayload{}}},
			},
		},
	}

	mux := gofi.NewRouter()
	mux.Get("/ws/:room", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
		Schema:    schema,
		Handshake: gofiws.HandshakePolicy{Mode: gofiws.HandshakeOff},
		Handler: func(s *gofiws.Session) error {
			var in inboundEnvelope
			if err := s.ReadJSON(&in); err != nil {
				return err
			}

			if in.Kind != "join" {
				return fmt.Errorf("unexpected discriminator %s", in.Kind)
			}

			return s.WriteJSON(outboundEnvelope{
				Kind: "joined",
				Payload: joinedPayload{
					Room: s.Context().Param("room") + ":" + in.Payload.Nickname,
				},
			})
		},
	}))

	addr := freeAddr(t)
	go func() { _ = mux.Listen(addr) }()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws/lobby", addr)
	conn, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(fasthttpws.TextMessage, []byte(`{"kind":"join","payload":{"nickname":"mike"}}`)); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var out outboundEnvelope
	if err := json.Unmarshal(msg, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if out.Kind != "joined" {
		t.Fatalf("expected joined, got %s", out.Kind)
	}

	if out.Payload.Room != "lobby:mike" {
		t.Fatalf("expected lobby:mike, got %s", out.Payload.Room)
	}
}

func TestDefineWebSocketHandshakePolicy(t *testing.T) {
	type handshakeSchema struct {
		Request struct {
			Header struct {
				Token string `json:"x-token" validate:"required"`
			}
		}
	}

	mux := gofi.NewRouter()
	mux.Get("/ws", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
		Schema: &handshakeSchema{},
		Handshake: gofiws.HandshakePolicy{
			Mode:      gofiws.HandshakeSelective,
			Selectors: []gofi.RequestSchema{gofi.Header},
		},
		Handler: func(s *gofiws.Session) error {
			return s.WriteMessage(gofiws.TextMessage, []byte("ok"))
		},
	}))

	addr := freeAddr(t)
	go func() { _ = mux.Listen(addr) }()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws", addr)
	if conn, _, err := fasthttpws.DefaultDialer.Dial(url, nil); err == nil {
		_ = conn.Close()
		t.Fatalf("expected dial to fail without required x-token header")
	}

	h := make(http.Header)
	h.Set("x-token", "abc123")
	conn, _, err := fasthttpws.DefaultDialer.Dial(url, h)
	if err != nil {
		t.Fatalf("dial with token failed: %v", err)
	}
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(msg) != "ok" {
		t.Fatalf("expected ok, got %s", string(msg))
	}
}

func TestWebSocketHooksFired(t *testing.T) {
	var upgradeAttempt int64
	var upgradeSuccess int64
	var sessionStart int64
	var sessionEnd int64
	var sessionErr int64

	mux := gofi.NewRouter()
	mux.Get("/ws", gofi.RouteOptions{
		Handler: gofiws.NewWithSessionAndOptions(func(s *gofiws.Session) error {
			return fmt.Errorf("boom")
		}, gofiws.Options{
			Hooks: gofiws.Hooks{
				OnUpgradeAttempt: func(ctx gofi.Context) { atomic.AddInt64(&upgradeAttempt, 1) },
				OnUpgradeSuccess: func(ctx gofi.Context) { atomic.AddInt64(&upgradeSuccess, 1) },
				OnSessionStart:   func(ctx gofi.Context) { atomic.AddInt64(&sessionStart, 1) },
				OnSessionEnd:     func(ctx gofi.Context, d time.Duration) { atomic.AddInt64(&sessionEnd, 1) },
				OnSessionError:   func(ctx gofi.Context, err error) { atomic.AddInt64(&sessionErr, 1) },
			},
		}),
	})

	addr := freeAddr(t)
	go func() { _ = mux.Listen(addr) }()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws", addr)
	conn, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	_ = conn.Close()
	time.Sleep(80 * time.Millisecond)

	if atomic.LoadInt64(&upgradeAttempt) != 1 {
		t.Fatalf("expected one upgrade attempt")
	}
	if atomic.LoadInt64(&upgradeSuccess) != 1 {
		t.Fatalf("expected one upgrade success")
	}
	if atomic.LoadInt64(&sessionStart) != 1 {
		t.Fatalf("expected one session start")
	}
	if atomic.LoadInt64(&sessionEnd) != 1 {
		t.Fatalf("expected one session end")
	}
	if atomic.LoadInt64(&sessionErr) != 1 {
		t.Fatalf("expected one session error")
	}
}

func TestWebSocketRegistryDrain(t *testing.T) {
	registry := gofiws.NewSessionRegistry()

	mux := gofi.NewRouter()
	mux.Get("/ws", gofi.RouteOptions{
		Handler: gofiws.NewWithSessionAndOptions(func(s *gofiws.Session) error {
			for {
				if _, _, err := s.ReadMessage(); err != nil {
					return nil
				}
			}
		}, gofiws.Options{Registry: registry}),
	})

	addr := freeAddr(t)
	go func() { _ = mux.Listen(addr) }()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws", addr)
	conn, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	if err := conn.WriteMessage(fasthttpws.TextMessage, []byte("hello")); err != nil {
		_ = conn.Close()
		t.Fatalf("write failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for registry.Active() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if registry.Active() < 1 {
		_ = conn.Close()
		t.Fatalf("expected active session before drain")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := registry.DrainContext(ctx); err != nil {
		_ = conn.Close()
		t.Fatalf("drain failed: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		_ = conn.Close()
		t.Fatalf("expected closed connection after drain")
	}
	_ = conn.Close()

	if registry.Active() != 0 {
		t.Fatalf("expected zero active sessions after drain")
	}
}

func TestDefineWebSocketPublicAPI(t *testing.T) {
	mux := gofi.NewRouter()
	mux.Get("/ws/:room", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
		Handshake: gofiws.HandshakePolicy{Mode: gofiws.HandshakeAuto},
		Handler: func(s *gofiws.Session) error {
			mt, msg, err := s.ReadMessage()
			if err != nil {
				return err
			}
			return s.WriteMessage(mt, []byte(s.Context().Param("room")+":"+string(msg)))
		},
		Runtime: gofiws.RuntimeOptions{ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second},
	}))

	addr := freeAddr(t)
	go func() { _ = mux.Listen(addr) }()
	time.Sleep(120 * time.Millisecond)
	defer func() { _ = mux.Shutdown() }()

	url := fmt.Sprintf("ws://%s/ws/gamma", addr)
	conn, _, err := fasthttpws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(fasthttpws.TextMessage, []byte("hello")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(msg) != "gamma:hello" {
		t.Fatalf("expected gamma:hello, got %s", string(msg))
	}
}
