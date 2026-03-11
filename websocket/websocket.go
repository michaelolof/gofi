package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/michaelolof/gofi"
	"github.com/michaelolof/gofi/validators"
)

type Conn = websocket.Conn
type Config = websocket.FastHTTPUpgrader

// HandshakeMode controls which handshake validation policy applies before upgrade.
type HandshakeMode string

const (
	HandshakeAuto      HandshakeMode = "auto"
	HandshakeOff       HandshakeMode = "off"
	HandshakeSelective HandshakeMode = "selective"
)

// HandshakePolicy makes websocket handshake validation explicit at route definition time.
type HandshakePolicy struct {
	Mode      HandshakeMode
	Selectors []gofi.RequestSchema
}

// RuntimeOptions configures operational websocket behavior for DefineWebSocket.
type RuntimeOptions struct {
	Upgrader        *Config
	MaxMessageBytes int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	Hooks           Hooks
	Registry        *SessionRegistry
}

// Session wraps a websocket connection with the request context snapshot.
// This keeps route params, headers, and stores accessible during ws lifecycle.
type Session struct {
	conn           *Conn
	ctx            gofi.Context
	inboundSchema  any
	outboundSchema any
	readTimeout    time.Duration
	writeTimeout   time.Duration
	registry       *SessionRegistry
}

// Options configures websocket handler behavior.
type Options struct {
	Upgrader *Config
	// Limits inbound message size when > 0.
	MaxMessageBytes int
	// Per-operation read/write deadlines when > 0.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	// If true, runs full route schema validation before upgrade.
	ValidateHandshake bool
	// If set, validates only these request parts before upgrade.
	HandshakeSelectors []gofi.RequestSchema
	// Optional message schemas used by ReadJSON/WriteJSON helpers.
	InboundSchema  any
	OutboundSchema any
	// Hooks for metrics/logging/observability.
	Hooks Hooks
	// Optional registry to track and drain active sessions.
	Registry *SessionRegistry
}

// WebSocketOptions defines websocket route options and composes the transport/runtime Options.
// This is the public, websocket-specific route-definition API.
type WebSocketOptions struct {
	Info      gofi.Info
	Schema    any
	Meta      any
	Handshake HandshakePolicy
	Runtime   RuntimeOptions
	Handler   func(ws *Session) error
}

// Hooks provides optional observability callbacks for websocket lifecycle.
type Hooks struct {
	OnUpgradeAttempt func(ctx gofi.Context)
	OnUpgradeSuccess func(ctx gofi.Context)
	OnUpgradeError   func(ctx gofi.Context, err error)

	OnSessionStart func(ctx gofi.Context)
	OnSessionEnd   func(ctx gofi.Context, duration time.Duration)
	OnSessionError func(ctx gofi.Context, err error)
}

// SessionRegistry tracks active websocket sessions and supports graceful draining.
type SessionRegistry struct {
	mu       sync.Mutex
	sessions map[*Session]struct{}
	draining atomic.Bool
}

var (
	TextMessage   = websocket.TextMessage
	BinaryMessage = websocket.BinaryMessage
	CloseMessage  = websocket.CloseMessage
	PingMessage   = websocket.PingMessage
	PongMessage   = websocket.PongMessage
)

var defaultUpgrader = websocket.FastHTTPUpgrader{}

// DefineWebSocket constructs a gofi.RouteOptions using websocket-specific options.
// The provided handler receives both gofi.Context and Session for ergonomic access.
func DefineWebSocket(opts WebSocketOptions) gofi.RouteOptions {
	handlerOpts := Options{
		Upgrader:        opts.Runtime.Upgrader,
		MaxMessageBytes: opts.Runtime.MaxMessageBytes,
		ReadTimeout:     opts.Runtime.ReadTimeout,
		WriteTimeout:    opts.Runtime.WriteTimeout,
		Hooks:           opts.Runtime.Hooks,
		Registry:        opts.Runtime.Registry,
	}
	applyHandshakePolicy(&handlerOpts, opts.Handshake, opts.Schema != nil)

	return gofi.DefineHandler(gofi.RouteOptions{
		Info:   opts.Info,
		Schema: opts.Schema,
		Meta:   opts.Meta,
		Handler: NewWithSessionAndOptions(func(s *Session) error {
			if opts.Handler == nil {
				return nil
			}
			return opts.Handler(s)
		}, handlerOpts),
	})
}

func applyHandshakePolicy(runtime *Options, policy HandshakePolicy, hasSchema bool) {
	runtime.ValidateHandshake = false
	runtime.HandshakeSelectors = nil
	if !hasSchema {
		return
	}

	switch policy.Mode {
	case HandshakeOff:
		return
	case HandshakeSelective:
		if len(policy.Selectors) == 0 {
			runtime.ValidateHandshake = true
			return
		}
		runtime.HandshakeSelectors = append([]gofi.RequestSchema(nil), policy.Selectors...)
		return
	case "", HandshakeAuto:
		runtime.ValidateHandshake = true
		return
	default:
		runtime.ValidateHandshake = true
		return
	}
}

// NewSessionRegistry creates an active-session registry for graceful shutdown/draining.
func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{sessions: make(map[*Session]struct{})}
}

// Active returns the current number of tracked active websocket sessions.
func (r *SessionRegistry) Active() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sessions)
}

// IsDraining reports whether the registry is currently draining.
func (r *SessionRegistry) IsDraining() bool {
	if r == nil {
		return false
	}
	return r.draining.Load()
}

// DrainContext marks the registry as draining, closes active sessions, and waits for all to exit.
func (r *SessionRegistry) DrainContext(ctx context.Context) error {
	if r == nil {
		return nil
	}

	r.draining.Store(true)

	r.mu.Lock()
	current := make([]*Session, 0, len(r.sessions))
	for s := range r.sessions {
		current = append(current, s)
	}
	r.mu.Unlock()

	for _, s := range current {
		_ = s.Close()
	}

	r.mu.Lock()
	r.sessions = make(map[*Session]struct{})
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (r *SessionRegistry) track(s *Session) bool {
	if r == nil {
		return true
	}
	if r.draining.Load() {
		return false
	}
	r.mu.Lock()
	r.sessions[s] = struct{}{}
	r.mu.Unlock()
	return true
}

func (r *SessionRegistry) untrack(s *Session) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.sessions, s)
	r.mu.Unlock()
}

type structFieldRule struct {
	index []int
	rule  string
	name  string
}

type cachedStructValidator struct {
	typeName string
	fields   []structFieldRule
}

var structValidatorCache sync.Map // map[reflect.Type]*cachedStructValidator

// Context returns the request context snapshot associated with the ws session.
func (s *Session) Context() gofi.Context {
	return s.ctx
}

// Conn returns the underlying websocket connection for low-level operations.
func (s *Session) Conn() *Conn {
	return s.conn
}

// ReadMessage reads a message frame from the websocket connection.
func (s *Session) ReadMessage() (int, []byte, error) {
	if s.readTimeout > 0 {
		_ = s.conn.SetReadDeadline(time.Now().Add(s.readTimeout))
	}
	return s.conn.ReadMessage()
}

// WriteMessage writes a frame to the websocket connection.
func (s *Session) WriteMessage(mt int, data []byte) error {
	if s.writeTimeout > 0 {
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	return s.conn.WriteMessage(mt, data)
}

// ReadJSON reads a websocket frame, unmarshals JSON into dst, and validates against inbound schema if configured.
func (s *Session) ReadJSON(dst any) error {
	_, payload, err := s.conn.ReadMessage()
	if err != nil {
		return err
	}

	if handled, err := gofi.DecodeWebSocketJSON(s.ctx, gofi.WebSocketInbound, payload, dst); handled {
		return err
	}

	if err := json.Unmarshal(payload, dst); err != nil {
		return err
	}

	return s.ValidateInbound(dst)
}

// WriteJSON validates against outbound schema if configured and writes the payload as a text frame.
func (s *Session) WriteJSON(v any) error {
	if err := s.ValidateOutbound(v); err != nil {
		return err
	}

	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return s.conn.WriteMessage(TextMessage, payload)
}

// WriteError validates the payload against the route error contract when available and writes it as a text frame.
func (s *Session) WriteError(v any) error {
	if handled, err := gofi.ValidateWebSocketPayload(s.ctx, gofi.WebSocketError, v); handled {
		if err != nil {
			return err
		}
	}

	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return s.conn.WriteMessage(TextMessage, payload)
}

// ValidateInbound validates payload against configured inbound schema when set.
func (s *Session) ValidateInbound(v any) error {
	if handled, err := gofi.ValidateWebSocketPayload(s.ctx, gofi.WebSocketInbound, v); handled {
		return err
	}
	return validatePayloadSchema(v, s.inboundSchema, "inbound")
}

// ValidateOutbound validates payload against configured outbound schema when set.
func (s *Session) ValidateOutbound(v any) error {
	if handled, err := gofi.ValidateWebSocketPayload(s.ctx, gofi.WebSocketOutbound, v); handled {
		return err
	}
	return validatePayloadSchema(v, s.outboundSchema, "outbound")
}

// Close closes the underlying websocket connection.
func (s *Session) Close() error {
	return s.conn.Close()
}

// CloseWithReason writes a close control frame with code/reason and then closes the socket.
func (s *Session) CloseWithReason(code int, reason string) error {
	deadline := time.Now().Add(2 * time.Second)
	if s.writeTimeout > 0 {
		deadline = time.Now().Add(s.writeTimeout)
	}
	err := s.conn.WriteControl(CloseMessage, websocket.FormatCloseMessage(code, reason), deadline)
	closeErr := s.conn.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// New returns a new Gofi WebSocket handler.
func New(handler func(*Conn) error) gofi.HandlerFunc {
	return NewWithConfig(handler, defaultUpgrader)
}

// NewWithContext returns a new Gofi WebSocket handler that also exposes gofi.Context.
// The context passed to the handler is a copy safe for ws session lifetime usage.
func NewWithContext(handler func(gofi.Context, *Conn) error) gofi.HandlerFunc {
	return NewWithContextAndOptions(handler, Options{})
}

// NewWithSession returns a new Gofi WebSocket handler based on a Session wrapper.
func NewWithSession(handler func(*Session) error) gofi.HandlerFunc {
	return NewWithSessionAndOptions(handler, Options{})
}

// NewWithConfig returns a new Gofi WebSocket handler with a custom Upgrader.
func NewWithConfig(handler func(*Conn) error, upgrader Config) gofi.HandlerFunc {
	return NewWithContextAndOptions(func(_ gofi.Context, conn *Conn) error {
		return handler(conn)
	}, Options{Upgrader: &upgrader})
}

// NewWithContextAndConfig returns a new context-aware Gofi WebSocket handler with a custom Upgrader.
func NewWithContextAndConfig(handler func(gofi.Context, *Conn) error, upgrader Config) gofi.HandlerFunc {
	return NewWithContextAndOptions(handler, Options{Upgrader: &upgrader})
}

// NewWithContextAndOptions returns a new context-aware Gofi WebSocket handler with validation options.
func NewWithContextAndOptions(handler func(gofi.Context, *Conn) error, opts Options) gofi.HandlerFunc {
	upgrader := defaultUpgrader
	if opts.Upgrader != nil {
		upgrader = *opts.Upgrader
	}

	return func(c gofi.Context) error {
		if opts.Hooks.OnUpgradeAttempt != nil {
			opts.Hooks.OnUpgradeAttempt(c)
		}

		if opts.Registry != nil && opts.Registry.IsDraining() {
			return c.SendString(http.StatusServiceUnavailable, "websocket server draining")
		}

		if !websocket.FastHTTPIsWebSocketUpgrade(c.Request().FastHTTPContext()) {
			return c.SendString(http.StatusUpgradeRequired, "websocket upgrade required")
		}

		if opts.ValidateHandshake || len(opts.HandshakeSelectors) > 0 {
			var err error
			if len(opts.HandshakeSelectors) > 0 {
				err = gofi.Validate(c, opts.HandshakeSelectors...)
			} else {
				err = gofi.Validate(c)
			}
			if err != nil {
				return err
			}
		}

		cc := c.Copy()
		err := upgrader.Upgrade(c.Request().FastHTTPContext(), func(conn *websocket.Conn) {
			if opts.MaxMessageBytes > 0 {
				conn.SetReadLimit(int64(opts.MaxMessageBytes))
			}
			if opts.ReadTimeout > 0 {
				_ = conn.SetReadDeadline(time.Now().Add(opts.ReadTimeout))
			}
			if opts.WriteTimeout > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(opts.WriteTimeout))
			}
			session := &Session{
				conn:         conn,
				ctx:          cc,
				readTimeout:  opts.ReadTimeout,
				writeTimeout: opts.WriteTimeout,
				registry:     opts.Registry,
			}

			if opts.Registry != nil && !opts.Registry.track(session) {
				_ = session.CloseWithReason(1013, "server draining")
				return
			}

			start := time.Now()
			if opts.Hooks.OnSessionStart != nil {
				opts.Hooks.OnSessionStart(cc)
			}

			defer func() {
				if opts.Registry != nil {
					opts.Registry.untrack(session)
				}
				if opts.Hooks.OnSessionEnd != nil {
					opts.Hooks.OnSessionEnd(cc, time.Since(start))
				}
				_ = conn.Close()
			}()

			if err := handler(cc, conn); err != nil {
				// Since the connection is already hijacked, we cannot propagate this error
				// back to the normal Gofi HTTP response cycle. Just drop or log it.
				if opts.Hooks.OnSessionError != nil {
					opts.Hooks.OnSessionError(cc, err)
				}
			}
		})

		if err != nil {
			if opts.Hooks.OnUpgradeError != nil {
				opts.Hooks.OnUpgradeError(c, err)
			}
			if c.Request().FastHTTPContext().Hijacked() {
				return err
			}
			return c.SendString(http.StatusBadRequest, "websocket handshake failed")
		}

		if opts.Hooks.OnUpgradeSuccess != nil {
			opts.Hooks.OnUpgradeSuccess(c)
		}

		return nil
	}
}

// NewWithSessionAndConfig returns a new Session-based Gofi WebSocket handler with a custom Upgrader.
func NewWithSessionAndConfig(handler func(*Session) error, upgrader Config) gofi.HandlerFunc {
	return NewWithSessionAndOptions(handler, Options{Upgrader: &upgrader})
}

// NewWithSessionAndOptions returns a new Session-based Gofi WebSocket handler with validation options.
func NewWithSessionAndOptions(handler func(*Session) error, opts Options) gofi.HandlerFunc {
	return NewWithContextAndOptions(func(c gofi.Context, conn *Conn) error {
		return handler(&Session{
			conn:           conn,
			ctx:            c,
			inboundSchema:  opts.InboundSchema,
			outboundSchema: opts.OutboundSchema,
			readTimeout:    opts.ReadTimeout,
			writeTimeout:   opts.WriteTimeout,
			registry:       opts.Registry,
		})
	}, opts)
}

func validatePayloadSchema(payload any, schema any, direction string) error {
	if schema == nil {
		return nil
	}
	if payload == nil {
		return fmt.Errorf("websocket %s payload is nil", direction)
	}

	payloadType := reflect.TypeOf(payload)
	if payloadType.Kind() == reflect.Pointer {
		payloadType = payloadType.Elem()
	}

	schemaType := reflect.TypeOf(schema)
	if schemaType.Kind() == reflect.Pointer {
		schemaType = schemaType.Elem()
	}

	if payloadType != schemaType {
		return fmt.Errorf("websocket %s payload type mismatch: expected %s, got %s", direction, schemaType.String(), payloadType.String())
	}

	v, err := getCachedStructValidator(payloadType)
	if err != nil {
		return err
	}

	if err := v.validate(payload); err != nil {
		return fmt.Errorf("websocket %s validation failed: %w", direction, err)
	}

	return nil
}

func getCachedStructValidator(t reflect.Type) (*cachedStructValidator, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("websocket validation expects struct payload, got %s", t.String())
	}

	if vv, ok := structValidatorCache.Load(t); ok {
		return vv.(*cachedStructValidator), nil
	}

	built := &cachedStructValidator{typeName: t.String(), fields: buildFieldRules(t)}
	actual, _ := structValidatorCache.LoadOrStore(t, built)
	return actual.(*cachedStructValidator), nil
}

func buildFieldRules(t reflect.Type) []structFieldRule {
	rules := make([]structFieldRule, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		rule := f.Tag.Get("validate")
		if rule == "" {
			continue
		}
		rules = append(rules, structFieldRule{index: f.Index, rule: rule, name: f.Name})
	}
	return rules
}

func (v *cachedStructValidator) validate(payload any) error {
	rv := reflect.ValueOf(payload)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return fmt.Errorf("nil payload for type %s", v.typeName)
		}
		rv = rv.Elem()
	}

	for _, f := range v.fields {
		fv := rv.FieldByIndex(f.index)
		if err := validators.Validate(fv.Interface(), f.rule); err != nil {
			return fmt.Errorf("field '%s': %w", f.name, err)
		}
	}

	return nil
}
