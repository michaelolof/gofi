# WebSockets

This guide explains how WebSockets work in Gofi, when to use them, how to define websocket routes, how clients connect to them, and what each public API option does.

It is written for readers who may not have implemented WebSockets in Go before.

## What a WebSocket Is

A WebSocket is a long-lived connection between a client and a server.

Unlike a normal HTTP request:

- HTTP is request then response, then the connection is usually done.
- WebSocket starts as an HTTP request, then upgrades into a persistent two-way connection.
- After the upgrade, either side can send messages at any time.

That makes WebSockets useful for:

- Chat
- Live dashboards
- Presence and typing indicators
- Multiplayer or collaborative applications
- Notifications that must arrive immediately
- Streaming application events in both directions

WebSockets are usually not the right tool for:

- Standard CRUD APIs
- Simple polling replacements where Server-Sent Events is enough
- Short request-response operations that do not need a persistent connection

If your traffic is only server-to-client streaming, Gofi's streaming support may be simpler. Use WebSockets when you need true bidirectional communication.

## How WebSockets Work in Gofi

In Gofi, a websocket route is still registered as a normal route on the router. The route usually uses `GET`, because the client first performs an HTTP upgrade request.

The high-level flow is:

1. A client sends a normal HTTP request with websocket upgrade headers.
2. Gofi runs the route and any middleware like a normal request.
3. Optional handshake validation runs before the upgrade.
4. Gofi upgrades the connection.
5. Your websocket handler starts reading and writing frames.
6. The connection stays open until the client disconnects, the server closes it, or your handler returns.

This design matters because it means:

- Middleware still works before upgrade.
- You can validate path params, query params, headers, and cookies before accepting the socket.
- You can still use `gofi.Info`, `Schema`, and `Meta` on the route.
- The websocket route remains a first-class route in the same router as your HTTP endpoints.

## Package to Import

Most websocket work is done through the dedicated package:

```go
import "github.com/michaelolof/gofi/websocket"
```

## The Main Public API

The most common entry point is:

```go
websocket.DefineWebSocket(websocket.WebSocketOptions{ ... })
```

This returns a normal `gofi.RouteOptions` value, so you register it like any other route.

## Quick Start Checklist

If you want the shortest path to a correct websocket route in Gofi, the recommended order is:

1. Define the route with `websocket.DefineWebSocket(...)`.
2. Put handshake input in `Schema.Request` when the socket depends on auth, room IDs, headers, or cookies.
3. Put message contracts in `Schema.WebSocket` when the session uses structured JSON frames.
4. Use `HandshakeAuto` or `HandshakeSelective` when invalid clients should be rejected before upgrade.
5. Use `Session.ReadJSON`, `Session.WriteJSON`, and `Session.WriteError` for JSON-based protocols.
6. Add `Runtime.MaxMessageBytes`, `ReadTimeout`, `WriteTimeout`, and optionally a `SessionRegistry` for production routes.

If your route is only pushing server-to-client updates and the client never needs to send live frames back, Gofi streaming is often simpler than WebSockets.

## Minimal Example

```go
package main

import (
	"log"

	"github.com/michaelolof/gofi"
	"github.com/michaelolof/gofi/websocket"
)

func main() {
	r := gofi.NewRouter()

	r.Get("/ws", websocket.DefineWebSocket(websocket.WebSocketOptions{
		Handler: func(s *websocket.Session) error {
			for {
				msgType, msg, err := s.ReadMessage()
				if err != nil {
					return nil
				}

				if err := s.WriteMessage(msgType, msg); err != nil {
					return err
				}
			}
		},
	}))

	log.Fatal(r.Listen(":8080"))
}
```

This is a classic echo server. It accepts websocket messages and sends the same bytes back.

## How a Client Connects

From a browser, a client can connect like this:

```js
const socket = new WebSocket("ws://localhost:8080/ws")

socket.onopen = () => {
  socket.send("hello from browser")
}

socket.onmessage = (event) => {
  console.log("received:", event.data)
}

socket.onerror = (event) => {
  console.error("websocket error", event)
}

socket.onclose = () => {
  console.log("socket closed")
}
```

Use `wss://` in production when your application is served over TLS.

## Route Definition with DefineWebSocket

The route-level configuration type is:

```go
type WebSocketOptions struct {
	Info      gofi.Info
	Schema    any
	Meta      any
	Handshake websocket.HandshakePolicy
	Runtime   websocket.RuntimeOptions
	Handler   func(ws *Session) error
}
```

Each field has a distinct purpose.

### `Info`

This is normal Gofi route metadata. Use it for summary, description, tags, and OpenAPI-related route information.

### `Schema`

This is the route schema. It is useful for:

- Handshake validation
- Path, query, header, and cookie validation before upgrade
- Keeping websocket routes documented in the same schema-driven system as the rest of your API

For websocket protocol documentation, the same route schema can also include top-level `SwitchingProtocols` and `WebSocket gofi.WebSocketSchema` fields.

### `Meta`

Attach custom route metadata if your application uses it.

### `Handshake`

This makes pre-upgrade validation explicit.

Use one of:

- `websocket.HandshakeAuto`
- `websocket.HandshakeOff`
- `websocket.HandshakeSelective`

Selective mode uses `Selectors` to choose which request parts must validate before upgrade.

### `Runtime`

This holds the operational websocket settings used by `DefineWebSocket`, such as limits, timeouts, hooks, registries, and any lower-level runtime helpers.

### `Handler`

This is your websocket session handler.

It receives `*websocket.Session`, which gives you:

- The upgraded connection
- A safe copy of the original `gofi.Context`
- Read and write helpers
- Optional JSON validation helpers
- Close helpers

This is the most ergonomic API for most applications.

`DefineWebSocket` uses `Runtime` for operational settings.

The lower-level `websocket.Options` type still exists for `NewWithContextAndOptions` and `NewWithSessionAndOptions`.

## Schema-First Protocol Contracts

The current recommended model is schema-first.

That means one websocket route can declare three different concerns in one schema:

1. `Request`: the HTTP upgrade request contract.
2. `SwitchingProtocols`: the `101` response metadata used for documentation.
3. `WebSocket`: the post-upgrade websocket protocol contract.

Example:

```go
type ChatSocketSchema struct {
	Request struct {
		Path struct {
			RoomID string `json:"room_id" validate:"required"`
		}
		Header struct {
			Authorization string `json:"Authorization" validate:"required"`
		}
	}

	SwitchingProtocols struct {
		Header struct {
			Upgrade string `json:"Upgrade" default:"websocket"`
		}
	}

	WebSocket gofi.WebSocketSchema
}
```

The route schema is the source of truth for documentation and for runtime websocket validation when you use `DefineWebSocket`.

At runtime, this gives you a clean separation:

- `Request` validates the upgrade request.
- `WebSocket` validates frames after the connection is upgraded.
- `Runtime` configures operational behavior rather than protocol shape.

That separation is important because HTTP handshake validation and websocket session validation are different phases with different failure modes.

## Using Path Params and Request Context

Because the initial upgrade is still a Gofi route, your session can access path params and other request information through `Session.Context()`.

```go
r.Get("/ws/:room_id", websocket.DefineWebSocket(websocket.WebSocketOptions{
	Handler: func(s *websocket.Session) error {
		roomID := s.Context().Param("room_id")
		_ = roomID
		return nil
	},
}))
```

Use `s.Context()` when you need:

- Path params
- Query values
- Request headers
- Cookies
- Global store access
- Route metadata

Gofi copies the request context before the socket handler starts, which makes it safe to access for the lifetime of the websocket session.

## The Session Type

`Session` is the main abstraction you work with inside handlers.

Common methods:

```go
func (s *Session) Context() gofi.Context
func (s *Session) Conn() *websocket.Conn
func (s *Session) ReadMessage() (int, []byte, error)
func (s *Session) WriteMessage(mt int, data []byte) error
func (s *Session) ReadJSON(dst any) error
func (s *Session) WriteJSON(v any) error
func (s *Session) WriteError(v any) error
func (s *Session) ValidateInbound(v any) error
func (s *Session) ValidateOutbound(v any) error
func (s *Session) Close() error
func (s *Session) CloseWithReason(code int, reason string) error
```

### `Context()`

Returns the copied `gofi.Context` associated with the original HTTP upgrade request.

### `Conn()`

Returns the underlying `*fasthttp/websocket.Conn` when you need low-level websocket features not wrapped by `Session`.

### `ReadMessage()` and `WriteMessage()`

These are the low-level frame helpers. They read and write raw websocket frames.

`ReadMessage()` returns:

- Message type
- Payload bytes
- Error

The message type can be compared with exported constants such as:

- `websocket.TextMessage`
- `websocket.BinaryMessage`
- `websocket.CloseMessage`
- `websocket.PingMessage`
- `websocket.PongMessage`

### `ReadJSON()` and `WriteJSON()`

These helpers work well when your websocket messages are JSON payloads.

`ReadJSON(dst)`:

- Reads a frame
- Unmarshals JSON into `dst`
- Validates `dst` against the route schema's `WebSocket.Inbound` contract when present
- Falls back to lower-level `Options.InboundSchema` only when you built the handler directly with `NewWith...AndOptions`

`WriteJSON(v)`:

- Validates `v` against the route schema's `WebSocket.Outbound` contract when present
- Falls back to lower-level `Options.OutboundSchema` only when you built the handler directly with `NewWith...AndOptions`
- Marshals to JSON
- Sends the payload as a text frame

### `WriteError()`

`WriteError(v)` writes the object you pass in.

When the route schema declares `WebSocket.Error`, the payload is validated against that error contract before bytes are written.

That means error shape is now schema-driven instead of being forced into a built-in `{type, code, message}` envelope.

### `Close()`

Closes the socket.

### `CloseWithReason()`

Sends a websocket close control frame with a status code and reason, then closes the connection.

Use this when you want the client to know why the session is being terminated.

## `websocket.Options`: Low-Level Runtime Configuration

The low-level runtime options struct is:

```go
type Options struct {
	Upgrader *websocket.Config
	MaxMessageBytes int
	ReadTimeout time.Duration
	WriteTimeout time.Duration
	ValidateHandshake bool
	HandshakeSelectors []gofi.RequestSchema
	InboundSchema any
	OutboundSchema any
	Hooks websocket.Hooks
	Registry *websocket.SessionRegistry
}
```

`websocket.RuntimeOptions` is the route-level operational subset used by `DefineWebSocket`.

Important distinction:

- Use `WebSocketOptions.Handshake` to declare handshake policy on `DefineWebSocket`
- Use `Schema.WebSocket` to declare inbound, outbound, and error message contracts on `DefineWebSocket`
- Use `websocket.Options.ValidateHandshake` and `HandshakeSelectors` only when constructing handlers directly with the lower-level `NewWith...AndOptions` helpers
- Use `websocket.Options.InboundSchema` and `OutboundSchema` only for those lower-level direct constructors

### `Upgrader`

Custom websocket upgrader.

Use this when you need to change low-level upgrade behavior exposed by `fasthttp/websocket.FastHTTPUpgrader`, for example origin checks or compression-related settings.

If you do not set it, Gofi uses the default upgrader.

### `MaxMessageBytes`

Limits the maximum inbound message size.

Why this exists:

- Prevents a client from sending excessively large frames
- Protects memory usage
- Helps avoid abuse and accidental oversized messages

If you expect small chat or command messages, set this aggressively.

### `ReadTimeout`

Sets a per-operation read deadline.

Why this exists:

- Prevents dead or stalled connections from sitting forever
- Helps clean up broken network sessions
- Forces reads to fail if the client stops sending data for too long

This timeout is applied when reading through the session helpers.

### `WriteTimeout`

Sets a per-operation write deadline.

Why this exists:

- Prevents blocked writes from hanging forever
- Helps when clients disconnect silently or cannot keep up

### `ValidateHandshake`

This is a lower-level option.

If true, Gofi validates the whole route request before upgrading.

That includes the request parts described by your route schema such as:

- Path params
- Query params
- Headers
- Cookies
- Request body, if relevant to the route

This is useful when you want to reject invalid websocket connections before opening a long-lived session.

### `HandshakeSelectors`

This is a lower-level option.

If you only want to validate specific parts of the request before upgrade, set selectors instead of validating everything.

For example, you may only want to validate path and header values.

When `HandshakeSelectors` is non-empty, Gofi validates only those request sections.

### `InboundSchema`

Optional schema used by `Session.ReadJSON()` and `Session.ValidateInbound()`.

Why this exists:

- Lets you validate inbound websocket messages
- Keeps message validation explicit and reusable
- Makes JSON message handling safer

### `OutboundSchema`

Optional schema used by `Session.WriteJSON()` and `Session.ValidateOutbound()`.

Why this exists:

- Prevents your server from emitting invalid message shapes
- Helps keep websocket message contracts consistent

### Schema-Driven Error Contracts

At the route level, error messages belong in `Schema.WebSocket.Error`.

Example:

```go
type SocketError struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Detail string `json:"detail"`
}

type socketSchema struct {
	WebSocket gofi.WebSocketSchema
}

schema := &socketSchema{
	WebSocket: gofi.WebSocketSchema{
		Error: gofi.WebSocketMessageFamily{
			Variants: []gofi.WebSocketMessage{
				{Type: "socket_error", Schema: SocketError{}},
			},
		},
	},
}

r.Get("/ws", websocket.DefineWebSocket(websocket.WebSocketOptions{
	Schema: schema,
	Handler: func(s *websocket.Session) error {
		return s.WriteError(SocketError{
			Kind:   "socket_error",
			ID:     "unauthorized",
			Detail: "missing token",
		})
	},
}))
```

### `Hooks`

Lifecycle callbacks for observability and metrics.

This is discussed in its own section below.

### `Registry`

Tracks active websocket sessions and supports graceful draining on shutdown.

This is important in production if you do not want to drop active sockets abruptly during deployment or shutdown.

## Handshake Validation Example

```go
type ChatSocketSchema struct {
	Request struct {
		Path struct {
			RoomID string `json:"room_id" validate:"required"`
		}
		Header struct {
			Authorization string `json:"Authorization" validate:"required"`
		}
	}
}

r.Get("/ws/:room_id", websocket.DefineWebSocket(websocket.WebSocketOptions{
	Schema: &ChatSocketSchema{},
	Handshake: websocket.HandshakePolicy{
		Mode: websocket.HandshakeAuto,
	},
	Handler: func(s *websocket.Session) error {
		return nil
	},
}))
```

In this example, invalid upgrade requests are rejected before the socket is accepted.

## Validation Lifecycle

There are two validation phases in a websocket route.

### Phase 1: Handshake Validation

This happens before the websocket upgrade completes.

It uses the normal request schema sections such as:

- `Request.Path`
- `Request.Query`
- `Request.Header`
- `Request.Cookie`
- `Request.Body`

This phase decides whether the client is allowed to establish the socket at all.

Typical reasons to validate here:

- missing auth header
- invalid room ID
- missing feature flag cookie
- invalid subscription selector

### Phase 2: Session Validation

This happens after the socket has already been upgraded.

It uses `Schema.WebSocket` when present.

Typical helpers involved:

- `ReadJSON(dst)` validates against `WebSocket.Inbound`
- `WriteJSON(v)` validates against `WebSocket.Outbound`
- `WriteError(v)` validates against `WebSocket.Error`

This phase decides whether a frame is valid for the active websocket protocol.

That distinction explains why websocket handler errors are not normal HTTP response errors. Once the connection is upgraded, you are no longer in the standard HTTP response lifecycle.

## JSON Message Validation Example

```go
type EchoInbound struct {
	Text string `json:"text" validate:"required"`
}

type EchoOutbound struct {
	Text string `json:"text" validate:"required"`
}

type ChatSocketSchema struct {
	WebSocket gofi.WebSocketSchema
}

schema := &ChatSocketSchema{
	WebSocket: gofi.WebSocketSchema{
		Inbound: gofi.WebSocketMessageFamily{
			Variants: []gofi.WebSocketMessage{{Type: "echo_in", Schema: EchoInbound{}}},
		},
		Outbound: gofi.WebSocketMessageFamily{
			Variants: []gofi.WebSocketMessage{{Type: "echo_out", Schema: EchoOutbound{}}},
		},
	},
}

r.Get("/ws/chat", websocket.DefineWebSocket(websocket.WebSocketOptions{
	Schema: schema,
	Handler: func(s *websocket.Session) error {
		for {
			var in EchoInbound
			if err := s.ReadJSON(&in); err != nil {
				return err
			}

			out := EchoOutbound{
				Text: in.Text,
			}

			if err := s.WriteJSON(out); err != nil {
				return err
			}
		}
	},
}))
```

Important detail:

With `DefineWebSocket`, message validation comes from `Schema.WebSocket`. The lower-level `Options.InboundSchema` and `Options.OutboundSchema` fields are only for direct `NewWith...AndOptions` usage.

## How OpenAPI Documents a WebSocket Route

OpenAPI is still HTTP-first, so Gofi documents websocket routes as an HTTP upgrade plus an inline protocol schema.

The practical result is:

1. The route still appears as an HTTP operation.
2. The upgrade response is documented under `101 Switching Protocols`.
3. The websocket protocol contract is emitted inline under the `101` response content.
4. Message families use `oneOf` plus `discriminator` when the route declares `gofi.WebSocketSchema`.

That means the docs can describe inbound, outbound, and error message families even though OpenAPI is not a true event-channel specification like AsyncAPI.

This is especially useful when your websocket API is schema-driven and you want the same route metadata, validation rules, and docs generation model as the rest of your HTTP API.

## Custom Discriminator Envelope Example

You are not limited to a `type` discriminator. If your wire format uses a different field such as `kind`, declare it on the message family.

```go
type JoinPayload struct {
	Nickname string `json:"nickname" validate:"required,min=2"`
}

type JoinedPayload struct {
	Room string `json:"room" validate:"required"`
}

type JoinEnvelope struct {
	Kind    string      `json:"kind" validate:"required"`
	Payload JoinPayload `json:"payload" validate:"required"`
}

type JoinedEnvelope struct {
	Kind    string        `json:"kind" validate:"required"`
	Payload JoinedPayload `json:"payload" validate:"required"`
}

type ChatSocketSchema struct {
	WebSocket gofi.WebSocketSchema
}

schema := &ChatSocketSchema{
	WebSocket: gofi.WebSocketSchema{
		Inbound: gofi.WebSocketMessageFamily{
			Discriminator: "kind",
			Variants: []gofi.WebSocketMessage{
				{Type: "join", Schema: JoinPayload{}},
			},
		},
		Outbound: gofi.WebSocketMessageFamily{
			Discriminator: "kind",
			Variants: []gofi.WebSocketMessage{
				{Type: "joined", Schema: JoinedPayload{}},
			},
		},
	},
}

r.Get("/ws/:room_id", websocket.DefineWebSocket(websocket.WebSocketOptions{
	Schema: schema,
	Handler: func(s *websocket.Session) error {
		var join JoinEnvelope
		if err := s.ReadJSON(&join); err != nil {
			return err
		}

		return s.WriteJSON(JoinedEnvelope{
			Kind: "joined",
			Payload: JoinedPayload{
				Room: s.Context().Param("room_id") + ":" + join.Payload.Nickname,
			},
		})
	},
}))
```

In that setup, `ReadJSON` and `WriteJSON` both use the `kind` discriminator plus the nested `payload` contract defined on `Schema.WebSocket`.

## Schema-Driven Error Flow

When your route declares `Schema.WebSocket.Error`, you can return protocol-specific error payloads through `WriteError(v)` instead of building ad-hoc transport error frames.

```go
type ValidationError struct {
	Kind    string `json:"kind" validate:"required"`
	Message string `json:"message" validate:"required"`
}

type ChatSocketSchema struct {
	WebSocket gofi.WebSocketSchema
}

schema := &ChatSocketSchema{
	WebSocket: gofi.WebSocketSchema{
		Error: gofi.WebSocketMessageFamily{
			Discriminator: "kind",
			Variants: []gofi.WebSocketMessage{
				{Type: "validation_error", Schema: ValidationError{}},
			},
		},
	},
}

r.Get("/ws", websocket.DefineWebSocket(websocket.WebSocketOptions{
	Schema: schema,
	Handler: func(s *websocket.Session) error {
		return s.WriteError(ValidationError{
			Kind:    "validation_error",
			Message: "nickname is required",
		})
	},
}))
```

That keeps error frames aligned with the same schema-first contract model as inbound and outbound websocket messages.

## Hooks: Observability and Lifecycle Events

The hook type is:

```go
type Hooks struct {
	OnUpgradeAttempt func(ctx gofi.Context)
	OnUpgradeSuccess func(ctx gofi.Context)
	OnUpgradeError   func(ctx gofi.Context, err error)

	OnSessionStart func(ctx gofi.Context)
	OnSessionEnd   func(ctx gofi.Context, duration time.Duration)
	OnSessionError func(ctx gofi.Context, err error)
}
```

What each hook means:

### `OnUpgradeAttempt`

Runs before the server attempts the websocket upgrade.

Useful for:

- Counting incoming upgrade attempts
- Logging access attempts
- Early metrics

### `OnUpgradeSuccess`

Runs after a successful upgrade.

Useful for:

- Connection counters
- Success metrics
- Audit logging

### `OnUpgradeError`

Runs when the upgrade fails.

Useful for:

- Error metrics
- Security monitoring
- Diagnostics for rejected handshakes

### `OnSessionStart`

Runs when the websocket session begins.

Useful for:

- Tracking active users
- Initializing per-session state outside your handler

### `OnSessionEnd`

Runs when the session finishes. It receives the session duration.

Useful for:

- Session duration metrics
- Cleanup and audit logging

### `OnSessionError`

Runs when your handler returns an error.

Important detail:

After upgrade, the connection is already hijacked from the normal HTTP response lifecycle. That means Gofi cannot convert a websocket handler error into a normal HTTP error response. The error is only available to hooks and your own websocket logic.

## Session Registry and Graceful Shutdown

The session registry helps track live connections and reject new ones during shutdown.

Main API:

```go
registry := websocket.NewSessionRegistry()

count := registry.Active()
draining := registry.IsDraining()
err := registry.DrainContext(ctx)
```

Recommended shutdown flow:

1. Mark the registry as draining.
2. Close active sessions.
3. Stop accepting new websocket upgrades.
4. Shut the router down.

Example:

```go
registry := websocket.NewSessionRegistry()

r.Get("/ws", websocket.DefineWebSocket(websocket.WebSocketOptions{
	Handler: func(s *websocket.Session) error {
		for {
			_, _, err := s.ReadMessage()
			if err != nil {
				return nil
			}
		}
	},
	Runtime: websocket.RuntimeOptions{
		Registry: registry,
	},
}))

shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := registry.DrainContext(shutdownCtx); err != nil {
	return err
}

return r.Shutdown()
```

While draining is active, new websocket upgrades are rejected with `503 Service Unavailable`.

## WebSockets vs Streaming

Gofi supports both streaming and WebSockets, but they solve different problems.

Use streaming when:

- the server pushes updates in one direction
- the client does not need to send live frames back
- Server-Sent Events is sufficient
- HTTP semantics should stay intact for as long as possible

Use WebSockets when:

- both client and server must send messages at any time
- the client participates in a live protocol
- presence, chat, game state, collaborative editing, or bidirectional command streams are required

In short:

- streaming is simpler for server-to-client event feeds
- WebSockets are the right tool for bidirectional protocols

If you are unsure, start with streaming and only move to WebSockets when the client truly needs to talk back continuously.

## Testing WebSocket Routes

Websocket handlers should be tested as real upgraded connections rather than plain injected HTTP handlers.

The common testing pattern in Gofi is:

1. create a router
2. register the websocket route
3. listen on a free local port
4. connect with a websocket client such as `fasthttp/websocket`
5. exchange frames and assert the result

Example:

```go
func TestChatSocket(t *testing.T) {
	r := gofi.NewRouter()
	r.Get("/ws/:room_id", websocket.DefineWebSocket(websocket.WebSocketOptions{
		Handler: func(s *websocket.Session) error {
			mt, msg, err := s.ReadMessage()
			if err != nil {
				return err
			}
			return s.WriteMessage(mt, []byte(s.Context().Param("room_id")+":"+string(msg)))
		},
	}))

	addr := "127.0.0.1:8085"
	go func() { _ = r.Listen(addr) }()
	defer func() { _ = r.Shutdown() }()

	conn, _, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws/lobby", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_ = conn.WriteMessage(fasthttpws.TextMessage, []byte("hello"))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}

	if string(msg) != "lobby:hello" {
		t.Fatalf("unexpected message: %s", msg)
	}
}
```

When testing JSON protocols, prefer asserting both:

1. successful message flow
2. invalid inbound frames
3. invalid outbound frames
4. custom discriminator-envelope behavior when your protocol uses one
5. handshake rejection behavior when auth or selectors are required

## Full Production Example

This example combines route metadata, handshake validation, limits, timeouts, hooks, and shutdown draining.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/michaelolof/gofi"
	"github.com/michaelolof/gofi/websocket"
)

type ChatSocketSchema struct {
	Request struct {
		Path struct {
			RoomID string `json:"room_id" validate:"required"`
		}
	}
}

func main() {
	r := gofi.NewRouter()
	registry := websocket.NewSessionRegistry()

	r.Get("/ws/:room_id", websocket.DefineWebSocket(websocket.WebSocketOptions{
		Info: gofi.Info{
			Summary:     "Chat WebSocket",
			Description: "Upgrades the request to a bidirectional room socket.",
		},
		Schema: &ChatSocketSchema{},
		Handshake: websocket.HandshakePolicy{
			Mode: websocket.HandshakeAuto,
		},
		Handler: func(s *websocket.Session) error {
			roomID := s.Context().Param("room_id")

			for {
				msgType, msg, err := s.ReadMessage()
				if err != nil {
					return nil
				}

				payload := []byte(fmt.Sprintf("[%s] %s", roomID, string(msg)))
				if err := s.WriteMessage(msgType, payload); err != nil {
					return err
				}
			}
		},
		Runtime: websocket.RuntimeOptions{
			Registry:          registry,
			MaxMessageBytes:   1 << 20,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      10 * time.Second,
			Hooks: websocket.Hooks{
				OnUpgradeError: func(ctx gofi.Context, err error) {
					log.Printf("upgrade failed path=%s err=%v", ctx.Path(), err)
				},
				OnSessionStart: func(ctx gofi.Context) {
					log.Printf("session start room=%s active=%d", ctx.Param("room_id"), registry.Active())
				},
				OnSessionEnd: func(ctx gofi.Context, d time.Duration) {
					log.Printf("session end room=%s duration=%s active=%d", ctx.Param("room_id"), d, registry.Active())
				},
				OnSessionError: func(ctx gofi.Context, err error) {
					log.Printf("session error room=%s err=%v", ctx.Param("room_id"), err)
				},
			},
		},
	}))

	go func() {
		if err := gofi.ListenAndServe(":8080", r); err != nil {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelDrain()

	if err := registry.DrainContext(drainCtx); err != nil {
		log.Printf("drain failed: %v", err)
	}

	if err := r.Shutdown(); err != nil {
		log.Printf("shutdown failed: %v", err)
	}
}
```

## Alternative Constructors

For advanced cases, the package also exposes lower-level constructors.

### `websocket.New`

Use this when you only want the raw websocket connection.

```go
handler := websocket.New(func(conn *websocket.Conn) error {
	return nil
})
```

### `websocket.NewWithContext`

Use this when you want the raw connection plus `gofi.Context`.

```go
handler := websocket.NewWithContext(func(ctx gofi.Context, conn *websocket.Conn) error {
	_ = ctx
	_ = conn
	return nil
})
```

### `websocket.NewWithSession`

Use this when you want the `Session` abstraction but are not using the route-level `DefineWebSocket` wrapper.

```go
handler := websocket.NewWithSession(func(s *websocket.Session) error {
	return nil
})
```

### `websocket.NewWithContextAndOptions` and `websocket.NewWithSessionAndOptions`

Use these when you want explicit control over runtime `Options` without going through `DefineWebSocket`.

For most users, `DefineWebSocket` is the clearest and best documented API.

## When to Use Which Entry Point

Use `DefineWebSocket` when:

- You are defining a route on the router
- You want schema, metadata, and route-level ergonomics
- You want the most readable public API

Use `NewWithSession` or `NewWithSessionAndOptions` when:

- You already have a normal route definition and only need the handler function
- You want direct handler construction

Use `New` when:

- You only want raw websocket frames
- You do not need `gofi.Context`
- You do not want the `Session` helper methods

## Best Practices

- Validate the handshake for authenticated or parameterized sockets.
- Set `MaxMessageBytes` for every public-facing websocket route.
- Set read and write timeouts in production.
- Use `Session` for most application code instead of raw `Conn`.
- Use `WriteJSON` and `ReadJSON` if your protocol is JSON-based.
- Use hooks for metrics, logs, and operational visibility.
- Use a `SessionRegistry` if your application needs graceful shutdowns.
- Return `nil` on expected disconnects to avoid treating normal closes as application failures.
- Send explicit close reasons when rejecting a session after upgrade.

## Common Mistakes

- Treating a websocket like a normal HTTP endpoint after upgrade. Once upgraded, you are exchanging frames, not HTTP responses.
- Forgetting to bound message size.
- Forgetting to validate auth or required params before accepting a long-lived connection.
- Assuming `OnSessionError` sends something to the client. It does not. It is for observability.
- Using WebSockets when SSE or normal HTTP would be simpler.

## Practical Build Order

When implementing a new websocket endpoint with Gofi, the least error-prone order is:

1. model the handshake contract in `Schema.Request`
2. model inbound, outbound, and error frame shapes in `Schema.WebSocket`
3. write a `DefineWebSocket` route using `Handshake` plus `Runtime`
4. implement the handler with `Session.ReadJSON`, `Session.WriteJSON`, and `Session.WriteError`
5. add a test for handshake rejection and a test for frame validation
6. add limits, timeouts, hooks, and a session registry before shipping to production

## Runnable Example

For a complete runnable example, see the websocket example in the companion examples repository:

- `gofi-test-utils/cmd/examples/websockets/main.go`

That example demonstrates:

- Route definition with `DefineWebSocket`
- Accessing path params from `Session.Context()`
- Message echoing
- Hooks
- Session registry usage
- Graceful shutdown draining
