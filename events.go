package gofi

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
)

// RequestEvent describes one finished request. Every field owns its memory,
// so an event can be kept or sent to another goroutine.
type RequestEvent struct {
	Method string
	// Path is the decoded request path. Do not use it as a metrics label;
	// prefer Pattern, which has bounded cardinality.
	Path string
	// Pattern is the matched route pattern, or "" when no route matched
	// (redirects, 404, 405, rejected targets and transport errors).
	Pattern  string
	Status   int
	Duration time.Duration
	// ClientIP is gofi.ClientIP: the socket peer, or the client resolved by a
	// TrustPolicy middleware.
	ClientIP string
	// Err is the error the router's error handler rendered, if any. It may
	// hold internal detail and must not be sent to clients.
	Err error
	// Panicked reports that a panic was recovered while serving the request.
	Panicked bool
}

// OnRequestComplete registers fn to receive one RequestEvent per request the
// router serves, after the response status is final: matched routes,
// redirects, 404/405, rejected request targets, transport errors and
// recovered panics. fn runs on the request goroutine, so it must be fast; use
// an EventQueue to hand events to slow sinks. A panic in fn is recovered.
// Register before the router starts serving.
func OnRequestComplete(r Router, fn func(RequestEvent)) error {
	m, ok := r.(*serveMux)
	if !ok {
		return errors.New("gofi: OnRequestComplete supports routers created by gofi only")
	}
	m.lifecycle.mustBeOpen("OnRequestComplete")
	m.lifecycle.onComplete = fn
	return nil
}

func (s *serveMux) emitRequestEvent(ctx *fasthttp.RequestCtx, st *dispatchState) {
	fn := s.lifecycle.onComplete
	if fn == nil {
		return
	}
	ev := RequestEvent{
		Method:   string(ctx.Method()),
		Path:     string(ctx.Path()),
		Status:   ctx.Response.StatusCode(),
		Duration: time.Since(st.start),
		Panicked: st.panicked,
	}
	if c := st.ctx; c != nil {
		if st.matched {
			ev.Pattern = strings.Clone(c.opts.Pattern)
		}
		ev.ClientIP = strings.Clone(ClientIP(c))
		ev.Err = c.err
	} else {
		ev.ClientIP = ctx.RemoteIP().String()
	}
	defer func() {
		if r := recover(); r != nil {
			logRecoveredPanic("OnRequestComplete hook", r)
		}
	}()
	fn(ev)
}

// EventQueue hands RequestEvents to a consumer goroutine through a bounded
// buffer. When the buffer is full, events are dropped and counted rather than
// blocking requests.
type EventQueue struct {
	events  chan RequestEvent
	dropped atomic.Uint64
	done    chan struct{}
	once    sync.Once
}

// StartEventQueue starts a consumer goroutine that calls consume for each
// event. Pass q.Publish to OnRequestComplete, and Close the queue on shutdown.
func StartEventQueue(capacity int, consume func(RequestEvent)) (*EventQueue, error) {
	if capacity <= 0 || consume == nil {
		return nil, errors.New("gofi: StartEventQueue requires a positive capacity and a consumer")
	}
	q := &EventQueue{events: make(chan RequestEvent, capacity), done: make(chan struct{})}
	go func() {
		defer close(q.done)
		for ev := range q.events {
			func() {
				defer func() {
					if r := recover(); r != nil {
						logRecoveredPanic("EventQueue consumer", r)
					}
				}()
				consume(ev)
			}()
		}
	}()
	return q, nil
}

// Publish enqueues ev, or drops it if the queue is full.
func (q *EventQueue) Publish(ev RequestEvent) {
	select {
	case q.events <- ev:
	default:
		q.dropped.Add(1)
	}
}

// Dropped returns how many events were dropped because the queue was full.
func (q *EventQueue) Dropped() uint64 { return q.dropped.Load() }

// Close stops accepting events and waits for queued ones to be consumed.
// Publish must not be called after Close.
func (q *EventQueue) Close() {
	q.once.Do(func() { close(q.events) })
	<-q.done
}
