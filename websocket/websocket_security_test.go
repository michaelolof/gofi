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
