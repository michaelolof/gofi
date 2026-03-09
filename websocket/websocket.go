package websocket

import (
	"github.com/fasthttp/websocket"
	"github.com/michaelolof/gofi"
)

type Conn = websocket.Conn
type Config = websocket.FastHTTPUpgrader

var (
	TextMessage   = websocket.TextMessage
	BinaryMessage = websocket.BinaryMessage
	CloseMessage  = websocket.CloseMessage
	PingMessage   = websocket.PingMessage
	PongMessage   = websocket.PongMessage
)

var defaultUpgrader = websocket.FastHTTPUpgrader{}

// New returns a new Gofi WebSocket handler.
func New(handler func(*Conn) error) gofi.HandlerFunc {
	return NewWithConfig(handler, defaultUpgrader)
}

// NewWithConfig returns a new Gofi WebSocket handler with a custom Upgrader.
func NewWithConfig(handler func(*Conn) error, upgrader Config) gofi.HandlerFunc {
	return func(c gofi.Context) error {
		return upgrader.Upgrade(c.Request().FastHTTPContext(), func(conn *websocket.Conn) {
			defer conn.Close()
			if err := handler(conn); err != nil {
				// Since the connection is already hijacked, we cannot propagate this error
				// back to the normal Gofi HTTP response cycle. Just drop or log it.
			}
		})
	}
}
