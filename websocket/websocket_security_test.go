package websocket_test

// Security regressions for F-29 in gofi-docs/security-audit-2.md. Both cases
// run in a subprocess so a regression that terminates the process is reported
// as a test failure.

import (
	stdcontext "context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	fasthttpws "github.com/fasthttp/websocket"
	"github.com/michaelolof/gofi"
	gofiws "github.com/michaelolof/gofi/websocket"
)

func inSubprocess(t *testing.T, child func(t *testing.T)) {
	t.Helper()
	if os.Getenv("GOFI_WS_SECURITY_CHILD") == t.Name() {
		child(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), "GOFI_WS_SECURITY_CHILD="+t.Name())
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--- PASS") {
		t.Fatalf("child process failed (%v):\n%s", err, out)
	}
}

func startWSServer(t *testing.T, s *gofi.FasthttpServer) string {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.ListenOnListener(ln) }()
	t.Cleanup(func() {
		ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 2*time.Second)
		defer cancel()
		_ = s.ShutdownWithContext(ctx)
		_ = ln.Close()
	})
	return ln.Addr().String()
}

func TestSecuritySessionPanicIsContained(t *testing.T) {
	inSubprocess(t, func(t *testing.T) {
		s := gofi.NewFasthttpServer()
		sessionErr := make(chan error, 1)
		s.Router().Get("/ws", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
			Runtime: gofiws.RuntimeOptions{Hooks: gofiws.Hooks{
				OnSessionError: func(c gofi.Context, err error) { sessionErr <- err },
			}},
			Handler: func(ws *gofiws.Session) error { panic("session failed") },
		}))
		s.Router().Get("/ok", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})
		addr := startWSServer(t, s)

		conn, _, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		select {
		case err := <-sessionErr:
			if !strings.Contains(err.Error(), "panicked") {
				t.Fatalf("unexpected session error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("session panic was not reported")
		}

		ok, _, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
		if err != nil {
			t.Fatalf("server stopped accepting upgrades after a session panic: %v", err)
		}
		ok.Close()
	})
}

func TestSecuritySessionConcurrentWritesAreSerialized(t *testing.T) {
	inSubprocess(t, func(t *testing.T) {
		const writers, perWriter = 8, 200
		s := gofi.NewFasthttpServer()
		s.Router().Get("/ws", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
			Handler: func(ws *gofiws.Session) error {
				var wg sync.WaitGroup
				for w := 0; w < writers; w++ {
					wg.Add(1)
					go func(w int) {
						defer wg.Done()
						for i := 0; i < perWriter; i++ {
							_ = ws.WriteMessage(gofiws.TextMessage, []byte(fmt.Sprintf("%d-%d", w, i)))
						}
					}(w)
				}
				wg.Wait()
				return nil
			},
		}))
		addr := startWSServer(t, s)

		conn, _, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		for n := 0; n < writers*perWriter; n++ {
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("read %d failed: %v", n, err)
			}
		}
	})
}

// F-18: JSON helpers refresh deadlines per message like the raw helpers.
func TestSecurityJSONHelpersRefreshDeadlines(t *testing.T) {
	s := gofi.NewFasthttpServer()
	received := make(chan int, 1)
	s.Router().Get("/ws", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
		Runtime: gofiws.RuntimeOptions{ReadTimeout: 200 * time.Millisecond, WriteTimeout: 200 * time.Millisecond},
		Handler: func(ws *gofiws.Session) error {
			n := 0
			for {
				var msg map[string]int
				if err := ws.ReadJSON(&msg); err != nil {
					received <- n
					return nil
				}
				n++
				if err := ws.WriteJSON(msg); err != nil {
					received <- -n
					return nil
				}
			}
		},
	}))
	addr := startWSServer(t, s)

	conn, _, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		time.Sleep(100 * time.Millisecond)
		if err := conn.WriteJSON(map[string]int{"i": i}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		var echo map[string]int
		if err := conn.ReadJSON(&echo); err != nil {
			t.Fatalf("active session expired after %d messages: %v", i, err)
		}
	}
	conn.Close()
	if n := <-received; n != 6 {
		t.Fatalf("session handled %d messages, want 6", n)
	}
}

// F-18: draining ends blocked session handlers and refuses new upgrades.
func TestSecurityDrainEndsSessions(t *testing.T) {
	registry := gofiws.NewSessionRegistry()
	s := gofi.NewFasthttpServer()
	ended := make(chan struct{})
	s.Router().Get("/ws", gofi.RouteOptions{Handler: gofiws.NewWithSessionAndOptions(func(ws *gofiws.Session) error {
		defer close(ended)
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return nil
			}
		}
	}, gofiws.Options{Registry: registry})})
	addr := startWSServer(t, s)

	conn, _, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for registry.Active() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 2*time.Second)
	defer cancel()
	if err := registry.DrainContext(ctx); err != nil {
		t.Fatalf("drain did not complete: %v", err)
	}
	select {
	case <-ended:
	default:
		t.Fatal("DrainContext returned before the session handler ended")
	}
	if _, _, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws", nil); err == nil {
		t.Fatal("upgrade accepted while draining")
	}
}

func TestSecurityCrossOriginHandshakeKeeps403(t *testing.T) {
	s := gofi.NewFasthttpServer()
	s.Router().Get("/ws", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
		Handler: func(ws *gofiws.Session) error { return nil },
	}))
	addr := startWSServer(t, s)
	_, resp, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws", map[string][]string{"Origin": {"https://evil.example"}})
	if err == nil || resp == nil || resp.StatusCode != 403 {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("cross-origin handshake: want 403, got %d (err %v)", code, err)
	}
}

// IP-18: session context ends at server shutdown; the registry caps sessions.
func TestSecuritySessionDoneOnShutdown(t *testing.T) {
	s := gofi.NewFasthttpServer()
	observed := make(chan struct{})
	s.Router().Get("/ws", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
		Handler: func(ws *gofiws.Session) error {
			if ws.Context().Context().Err() != nil {
				return nil // must not be cancelled when the upgrade request ends
			}
			<-ws.Done()
			close(observed)
			return nil
		},
	}))
	addr := startWSServer(t, s)
	conn, _, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)
	go func() {
		ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), time.Second)
		defer cancel()
		_ = s.ShutdownWithContext(ctx)
	}()
	select {
	case <-observed:
	case <-time.After(2 * time.Second):
		t.Fatal("session did not observe server shutdown")
	}
}

func TestSecuritySessionLimit(t *testing.T) {
	registry := gofiws.NewSessionRegistry(gofiws.RegistryOptions{MaxSessions: 1})
	s := gofi.NewFasthttpServer()
	s.Router().Get("/ws", gofiws.DefineWebSocket(gofiws.WebSocketOptions{
		Runtime: gofiws.RuntimeOptions{Registry: registry},
		Handler: func(ws *gofiws.Session) error {
			for {
				if _, _, err := ws.ReadMessage(); err != nil {
					return nil
				}
			}
		},
	}))
	addr := startWSServer(t, s)
	first, _, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	for registry.Active() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	_, resp, err := fasthttpws.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
	if err == nil || resp == nil || resp.StatusCode != 503 {
		t.Fatalf("second session admitted past the limit (err %v)", err)
	}
}
