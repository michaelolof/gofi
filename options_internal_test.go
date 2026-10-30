package gofi

import (
	stdcontext "context"
	"crypto/tls"
	"net"
	"testing"
	"time"
)

func TestServerTLSConfigPolicy(t *testing.T) {
	base := &tls.Config{MinVersion: tls.VersionTLS10, NextProtos: []string{"h2", "http/1.1"}}
	s := NewFasthttpServer(ServerOptions{TLSConfig: base})
	cfg := s.server.TLSConfig
	if cfg == base || cfg.MinVersion != tls.VersionTLS12 || len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != "http/1.1" {
		t.Fatalf("TLS policy not applied: min=%x protos=%v cloned=%v", cfg.MinVersion, cfg.NextProtos, cfg != base)
	}
	if base.MinVersion != tls.VersionTLS10 {
		t.Fatal("caller TLS config was mutated")
	}
}

// The live TLS listener negotiates HTTP/1.1 only and TLS 1.2 or later, so an
// HTTP/2-capable proxy or client never sends h2 frames to fasthttp.
func TestServerTLSListenerRefusesH2(t *testing.T) {
	serverCfg, clientCfg, certFile, keyFile := transportTLSFixture(t)
	s := NewFasthttpServer(ServerOptions{TLSConfig: &tls.Config{ClientCAs: serverCfg.ClientCAs}})
	s.Router().Get("/x", RouteOptions{Handler: func(c Context) error { return c.SendString(200, "ok") }})
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	go func() { _ = s.ListenTLS(addr, certFile, keyFile) }()
	t.Cleanup(func() {
		ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 2*time.Second)
		defer cancel()
		_ = s.ShutdownWithContext(ctx)
	})

	clientCfg = clientCfg.Clone()
	clientCfg.NextProtos = []string{"h2", "http/1.1"}
	var conn *tls.Conn
	for i := 0; i < 100; i++ {
		if conn, err = tls.Dial("tcp", addr, clientCfg); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	state := conn.ConnectionState()
	if state.NegotiatedProtocol != "http/1.1" {
		t.Fatalf("negotiated %q, want http/1.1", state.NegotiatedProtocol)
	}
	if state.Version < tls.VersionTLS12 {
		t.Fatalf("negotiated TLS version %x", state.Version)
	}

	clientCfg.MaxVersion = tls.VersionTLS11
	clientCfg.MinVersion = tls.VersionTLS10
	if old, err := tls.Dial("tcp", addr, clientCfg); err == nil {
		old.Close()
		t.Fatal("TLS 1.1 handshake accepted")
	}
}
