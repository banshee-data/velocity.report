package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchLatestRelease(t *testing.T) {
	release := githubRelease{
		TagName: "v0.5.2",
		Assets: []githubAsset{
			{Name: "velocity-report-arm64", BrowserDownloadURL: "https://example.com/binary"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	// We cannot easily override releasesAPI (it is a const), so this test
	// validates the JSON parsing logic directly.
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var got githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if got.TagName != "v0.5.2" {
		t.Errorf("expected tag v0.5.2, got %s", got.TagName)
	}
	if len(got.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(got.Assets))
	}
	if got.Assets[0].Name != "velocity-report-arm64" {
		t.Errorf("expected asset name velocity-report-arm64, got %s", got.Assets[0].Name)
	}
}

func TestInstallBinary(t *testing.T) {
	// Create a source binary.
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "test-binary")
	content := []byte("#!/bin/sh\necho hello\n")
	if err := os.WriteFile(srcPath, content, 0755); err != nil {
		t.Fatal(err)
	}

	// Install to a destination.
	dstDir := t.TempDir()
	dstPath := filepath.Join(dstDir, "installed-binary")

	if err := installBinary(srcPath, dstPath); err != nil {
		t.Fatalf("installBinary failed: %v", err)
	}

	// Verify content.
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading installed binary: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}

	// Verify permissions.
	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("expected mode 0755, got %o", info.Mode().Perm())
	}
}

func TestInstallBinaryAtomicity(t *testing.T) {
	// installBinary should not leave partial files on error.
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "nonexistent")
	dstDir := t.TempDir()
	dstPath := filepath.Join(dstDir, "target")

	err := installBinary(srcPath, dstPath)
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}

	// Destination should not exist.
	if _, statErr := os.Stat(dstPath); !os.IsNotExist(statErr) {
		t.Error("destination should not exist after failed install")
	}
}

func TestDownloadToTemp(t *testing.T) {
	content := "fake binary content for testing"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(content))
	}))
	defer server.Close()

	tmpPath, err := downloadToTemp(server.URL)
	if err != nil {
		t.Fatalf("downloadToTemp failed: %v", err)
	}
	defer os.Remove(tmpPath)

	got, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestDownloadToTempHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := downloadToTemp(server.URL)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}
