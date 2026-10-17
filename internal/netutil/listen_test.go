package netutil

import (
	"net"
	"testing"
)

func TestListenTCPWithFallback_FreeAddressIsUsedAsGiven(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().String()
	probe.Close()

	ln, fellBack, err := ListenTCPWithFallback(addr)
	if err != nil {
		t.Fatalf("ListenTCPWithFallback(%s): %v", addr, err)
	}
	defer ln.Close()
	if fellBack {
		t.Error("fellBack = true for a free address")
	}
	if got := ln.Addr().String(); got != addr {
		t.Errorf("bound %s, want %s", got, addr)
	}
}

// A port another server holds is the case this exists for: a stale server
// still running must not stop a second one starting.
func TestListenTCPWithFallback_HeldAddressFallsBackOnTheSameHost(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	ln, fellBack, err := ListenTCPWithFallback(held.Addr().String())
	if err != nil {
		t.Fatalf("ListenTCPWithFallback on a held port: %v", err)
	}
	defer ln.Close()
	if !fellBack {
		t.Error("fellBack = false although the configured port was held")
	}
	got := ln.Addr().(*net.TCPAddr)
	if got.Port == held.Addr().(*net.TCPAddr).Port {
		t.Errorf("fell back to the held port %d", got.Port)
	}
	if !got.IP.Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("fell back to host %s, want 127.0.0.1", got.IP)
	}
}

// Failures a different port cannot fix are returned unchanged.
func TestListenTCPWithFallback_OtherErrorsDoNotFallBack(t *testing.T) {
	ln, fellBack, err := ListenTCPWithFallback("not-an-address")
	if err == nil {
		ln.Close()
		t.Fatal("expected an error for a malformed address")
	}
	if fellBack {
		t.Error("fellBack = true for a malformed address")
	}
}
