package gofi_test

// Capacity regressions for IP-20 in gofi-docs/security-audit-2.md. Hostile
// traffic near every limit runs alongside valid traffic; afterwards the
// process must return to a bounded plateau of goroutines, file descriptors
// and heap. Skipped with -short.

import (
	"bufio"
	stdcontext "context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/michaelolof/gofi"
	"github.com/michaelolof/gofi/middleware"
)

// openFDs counts this process's open file descriptors, or -1 where /dev/fd
// is unavailable.
func openFDs() int {
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}

func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapInuse
}

// goroutineCounts splits live goroutines into idle fasthttp pool workers,
// which the pool keeps for MaxIdleWorkerDuration and caps at MaxConnections,
// and everything else.
func goroutineCounts() (other, idleWorkers int) {
	buf := make([]byte, 4<<20)
	stacks := strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n")
	for _, st := range stacks {
		if strings.Contains(st, "fasthttp.(*workerPool).workerFunc") && strings.Contains(st, "[chan receive") {
			idleWorkers++
		} else {
			other++
		}
	}
	return other, idleWorkers
}

type plateau struct {
	goroutines  int
	idleWorkers int
	fds         int
	heap        uint64
}

func measurePlateau() plateau {
	other, idle := goroutineCounts()
	return plateau{goroutines: other, idleWorkers: idle, fds: openFDs(), heap: heapInUse()}
}

// waitForPlateau polls until goroutines (other than idle pool workers) and
// descriptors fall back within slack of base, then checks the worker pool
// bound and the heap.
func waitForPlateau(t *testing.T, base plateau, slack, maxWorkers int, heapSlack uint64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var now plateau
	for {
		now = measurePlateau()
		settled := now.goroutines <= base.goroutines+slack && (base.fds < 0 || now.fds <= base.fds+slack)
		if settled || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if now.goroutines > base.goroutines+slack {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Fatalf("goroutines did not return to plateau: %d, baseline %d\n%s", now.goroutines, base.goroutines, buf[:n])
	}
	if now.idleWorkers > maxWorkers {
		t.Fatalf("%d idle workers exceed MaxConnections %d", now.idleWorkers, maxWorkers)
	}
	// Idle workers each hold at most one closed connection's worth of state.
	if base.fds >= 0 && now.fds > base.fds+slack {
		t.Fatalf("file descriptors did not return to plateau: %d, baseline %d", now.fds, base.fds)
	}
	if now.heap > base.heap+heapSlack {
		t.Fatalf("heap did not return to plateau: %d bytes in use, baseline %d", now.heap, base.heap)
	}
	t.Logf("plateau: goroutines %d→%d (+%d idle workers), fds %d→%d, heap %d KiB→%d KiB",
		base.goroutines, now.goroutines, now.idleWorkers, base.fds, now.fds, base.heap>>10, now.heap>>10)
}

func TestCapacityHostileLoadReturnsToPlateau(t *testing.T) {
	if testing.Short() {
		t.Skip("capacity test skipped with -short")
	}
	opts := gofi.DefaultServerOptions()
	opts.ReadTimeout = 300 * time.Millisecond
	opts.WriteTimeout = time.Second
	opts.IdleTimeout = 300 * time.Millisecond
	opts.MaxRequestBodySize = 4 << 10
	opts.MaxConnections = 256
	s := gofi.NewFasthttpServer(opts)
	r := s.Router()

	var streamsEnded atomic.Int32
	r.Get("/ok", gofi.RouteOptions{Handler: func(c gofi.Context) error { return c.SendString(200, "ok") }})
	r.Post("/echo", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		body, _ := io.ReadAll(c.Request().Body)
		return c.SendString(200, fmt.Sprint(len(body)))
	}})
	// A producer that never finishes on its own: only the client going away
	// or MaxDuration ends it.
	r.Get("/stream", gofi.RouteOptions{Handler: func(c gofi.Context) error {
		return gofi.Stream(c, gofi.StreamOptions{
			MaxDuration: 2 * time.Second,
			OnError:     func(error) {},
		}, func(ctx stdcontext.Context, w *bufio.Writer) error {
			defer streamsEnded.Add(1)
			chunk := strings.Repeat("x", 1024)
			for ctx.Err() == nil {
				if _, err := w.WriteString(chunk); err != nil {
					return err
				}
				if err := w.Flush(); err != nil {
					return err
				}
			}
			return ctx.Err()
		})
	}})
	deadline := must(middleware.ContextTimeout(50 * time.Millisecond))
	r.Group(func(g gofi.Router) {
		g.Use(deadline)
		g.Get("/slow", gofi.RouteOptions{Handler: func(c gofi.Context) error {
			<-c.Context().Done()
			return c.Context().Err()
		}})
	})
	addr := startServer(t, s)

	// Warm up pools and lazily started goroutines before the baseline.
	for i := 0; i < 20; i++ {
		_ = exchange(t, addr, "GET /ok HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	}
	http.DefaultClient.CloseIdleConnections()
	base := measurePlateau()

	big := strings.Repeat("a", 64<<10)
	hostile := []func(){
		// Oversized body.
		func() {
			_ = exchange(t, addr, fmt.Sprintf("POST /echo HTTP/1.1\r\nHost: x\r\nContent-Length: %d\r\n\r\n%s", len(big), big))
		},
		// Malformed framing.
		func() {
			_ = exchange(t, addr, "POST /echo HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\nContent-Length: 30\r\n\r\nabc")
		},
		// Slow headers: never finished; ReadTimeout must reclaim the connection.
		func() {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			_, _ = io.WriteString(conn, "GET /ok HTTP/1.1\r\nHost: x\r\nX-Slow: ")
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _ = io.Copy(io.Discard, conn) // returns when the server closes it
			conn.Close()
		},
		// Body promised but abandoned mid-way.
		func() {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			_, _ = io.WriteString(conn, "POST /echo HTTP/1.1\r\nHost: x\r\nContent-Length: 4000\r\n\r\npartial")
			conn.Close()
		},
		// Stream abandoned by the client after the first bytes.
		func() {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			_, _ = io.WriteString(conn, "GET /stream HTTP/1.1\r\nHost: x\r\n\r\n")
			_, _ = conn.Read(make([]byte, 4096))
			conn.Close()
		},
		// Handler blocked until its context deadline.
		func() {
			_ = exchange(t, addr, "GET /slow HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
		},
		// Idle keep-alive connection left open.
		func() {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			_, _ = io.WriteString(conn, "GET /ok HTTP/1.1\r\nHost: x\r\n\r\n")
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _ = io.Copy(io.Discard, conn) // IdleTimeout closes it
			conn.Close()
		},
	}

	const workers, rounds = 16, 30
	var valid, validFailed atomic.Int32
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				hostile[(w+i)%len(hostile)]()
				out := exchange(t, addr, "POST /echo HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\nConnection: close\r\n\r\nhello")
				valid.Add(1)
				if !strings.HasPrefix(out, "HTTP/1.1 200 ") || !strings.HasSuffix(out, "\r\n\r\n5") {
					validFailed.Add(1)
				}
			}
		}(w)
	}
	wg.Wait()

	if n := validFailed.Load(); n > 0 {
		t.Fatalf("%d of %d valid requests failed under hostile load", n, valid.Load())
	}
	streamStarts := int32(0)
	for w := 0; w < workers; w++ {
		for i := 0; i < rounds; i++ {
			if (w+i)%len(hostile) == 4 {
				streamStarts++
			}
		}
	}
	waitForPlateau(t, base, 8, opts.MaxConnections, 8<<20)
	if got := streamsEnded.Load(); got != streamStarts {
		t.Fatalf("%d of %d abandoned stream producers ended", got, streamStarts)
	}
}
