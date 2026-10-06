package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCPUProfilePath(t *testing.T) {
	if got := cpuProfilePath("", "kirk0"); got != "" {
		t.Fatalf("no directory: got %q, want no profile", got)
	}
	if got, want := cpuProfilePath("/p", "kirk0"), filepath.Join("/p", "kirk0.repeat.cpu.pprof"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestProfiledWithoutPathRunsBare(t *testing.T) {
	ran := false
	if err := profiled("", func() error { ran = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("fn did not run")
	}
}

// spin keeps a CPU busy long enough for the profiler to take samples.
func spin() error {
	deadline := time.Now().Add(50 * time.Millisecond)
	x := 0
	for time.Now().Before(deadline) {
		x++
	}
	_ = x
	return nil
}

func TestProfiledWritesAProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case.repeat.cpu.pprof")
	if err := profiled(path, spin); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A pprof profile is gzip-compressed protobuf.
	if !bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		t.Fatalf("profile is not gzip data: % x", data[:min(len(data), 4)])
	}
}

func TestProfiledRefusesAnExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case.repeat.cpu.pprof")
	if err := os.WriteFile(path, []byte("earlier"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran := false
	err := profiled(path, func() error { ran = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "cpu profile") {
		t.Fatalf("got %v, want a cpu profile error", err)
	}
	if ran {
		t.Fatal("fn ran although the profile could not be written")
	}
	if data, _ := os.ReadFile(path); string(data) != "earlier" {
		t.Fatalf("existing file was overwritten: %q", data)
	}
}

func TestProfiledReturnsTheRunError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case.repeat.cpu.pprof")
	want := errors.New("replay failed")
	if err := profiled(path, func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("profile not written on a failed run: %v", err)
	}
}

func TestProfiledRefusesWhileAnotherProfileRuns(t *testing.T) {
	dir := t.TempDir()
	outer := filepath.Join(dir, "outer.pprof")
	inner := filepath.Join(dir, "inner.pprof")
	var innerErr error
	if err := profiled(outer, func() error {
		innerErr = profiled(inner, spin)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if innerErr == nil || !strings.Contains(innerErr.Error(), "cpu profile") {
		t.Fatalf("nested profile: got %v, want a cpu profile error", innerErr)
	}
}
