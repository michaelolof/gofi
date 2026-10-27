package gofi

import (
	"crypto/tls"
	"testing"
)

func TestServerTLSConfigPolicy(t *testing.T) {
	base := &tls.Config{MinVersion: tls.VersionTLS10, NextProtos: []string{"h2", "http/1.1"}}
	s, err := NewFasthttpServerWithOptions(ServerOptions{TLSConfig: base})
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.server.TLSConfig
	if cfg == base || cfg.MinVersion != tls.VersionTLS12 || len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != "http/1.1" {
		t.Fatalf("TLS policy not applied: min=%x protos=%v cloned=%v", cfg.MinVersion, cfg.NextProtos, cfg != base)
	}
	if base.MinVersion != tls.VersionTLS10 {
		t.Fatal("caller TLS config was mutated")
	}
}
