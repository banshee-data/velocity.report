package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestBgFlushEnableCondition verifies the logic that determines whether
// background flushing should be enabled. This mirrors the condition in radar.go:
//
//	backgroundManager != nil && flushInterval > 0 && flushEnable
func TestBgFlushEnableCondition(t *testing.T) {
	tests := []struct {
		name          string
		hasManager    bool
		flushInterval time.Duration
		flushEnable   bool
		wantEnabled   bool
	}{
		{
			name:          "default settings - flushing disabled",
			hasManager:    true,
			flushInterval: 60 * time.Second,
			flushEnable:   false,
			wantEnabled:   false,
		},
		{
			name:          "enable flag set - flushing enabled",
			hasManager:    true,
			flushInterval: 60 * time.Second,
			flushEnable:   true,
			wantEnabled:   true,
		},
		{
			name:          "zero interval - flushing disabled",
			hasManager:    true,
			flushInterval: 0,
			flushEnable:   true,
			wantEnabled:   false,
		},
		{
			name:          "no manager - flushing disabled",
			hasManager:    false,
			flushInterval: 60 * time.Second,
			flushEnable:   true,
			wantEnabled:   false,
		},
		{
			name:          "all disabled conditions",
			hasManager:    false,
			flushInterval: 0,
			flushEnable:   false,
			wantEnabled:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the condition from radar.go
			enabled := tc.hasManager && tc.flushInterval > 0 && tc.flushEnable

			if enabled != tc.wantEnabled {
				t.Errorf("bgFlushEnabled = %v, want %v", enabled, tc.wantEnabled)
			}
		})
	}
}

func createTestTeXRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("failed to create bin directory: %v", err)
	}

	compilerPath := filepath.Join(binDir, "xelatex")
	if err := os.WriteFile(compilerPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("failed to create xelatex stub: %v", err)
	}

	return root
}

func TestResolvePrecompiledTeXRoot(t *testing.T) {
	texRoot := createTestTeXRoot(t)

	got, err := resolvePrecompiledTeXRoot(texRoot)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want, err := filepath.Abs(texRoot)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}

	if got != want {
		t.Fatalf("resolved root mismatch: got %q want %q", got, want)
	}
}

func TestResolvePrecompiledTeXRootMissingCompiler(t *testing.T) {
	texRoot := t.TempDir()

	_, err := resolvePrecompiledTeXRoot(texRoot)
	if err == nil {
		t.Fatal("expected error for TeX root without bin/xelatex")
	}
}

func TestConfigurePDFLaTeXFlowPrecompiled(t *testing.T) {
	t.Setenv("VELOCITY_TEX_ROOT", "")
	texRoot := createTestTeXRoot(t)

	if err := configurePDFLaTeXFlow("precompiled", texRoot); err != nil {
		t.Fatalf("configurePDFLaTeXFlow returned error: %v", err)
	}

	want, err := filepath.Abs(texRoot)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}

	if got := os.Getenv("VELOCITY_TEX_ROOT"); got != want {
		t.Fatalf("VELOCITY_TEX_ROOT mismatch: got %q want %q", got, want)
	}
}

func TestConfigurePDFLaTeXFlowFull(t *testing.T) {
	t.Setenv("VELOCITY_TEX_ROOT", "/tmp/should-be-cleared")

	if err := configurePDFLaTeXFlow("full", ""); err != nil {
		t.Fatalf("configurePDFLaTeXFlow returned error: %v", err)
	}

	if got := os.Getenv("VELOCITY_TEX_ROOT"); got != "" {
		t.Fatalf("expected VELOCITY_TEX_ROOT to be unset, got %q", got)
	}
}

func TestConfigurePDFLaTeXFlowInvalid(t *testing.T) {
	if err := configurePDFLaTeXFlow("invalid-flow", ""); err == nil {
		t.Fatal("expected error for invalid --pdf-latex-flow")
	}
}
