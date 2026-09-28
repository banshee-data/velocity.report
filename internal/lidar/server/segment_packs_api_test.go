package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

func TestPackInventoryKeepsCorruptReviewDataVisibleAsErrors(t *testing.T) {
	ws, _ := segmentServer(t)
	if response := callSegment(t, ws, "POST", "/api/annotations/packs", nil, ws.handleAnnotationPacks); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("inventory method: %d", response.Code)
	}
	dir := t.TempDir()
	if item := readPackListing(dir); item.Error == "" {
		t.Fatal("missing manifest went unnoticed")
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("bad JSON"), 0644); err != nil {
		t.Fatal(err)
	}
	if item := readPackListing(dir); item.Error == "" {
		t.Fatal("malformed manifest went unnoticed")
	}
	if err := os.WriteFile(manifestPath, []byte(`{"dataset_id":"dataset","pack_digest":"sha256:pack"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if item := readPackListing(dir); item.Status != "packed" || item.Error != "" {
		t.Fatalf("empty review state: %+v", item)
	}
	proposalDir := filepath.Join(dir, "proposals")
	if err := os.Mkdir(proposalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proposalDir, "cluster_chain@1.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if item := readPackListing(dir); item.ProposalLayers != 1 || item.Status != "proposed" {
		t.Fatalf("proposal state: %+v", item)
	}
	sidecarPath := filepath.Join(dir, "annotations.json")
	if err := os.WriteFile(sidecarPath, []byte("broken"), 0644); err != nil {
		t.Fatal(err)
	}
	if item := readPackListing(dir); item.Error == "" {
		t.Fatal("malformed sidecar went unnoticed")
	}
	if err := os.WriteFile(sidecarPath, []byte(`{"pack_digest":"sha256:other"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if item := readPackListing(dir); item.Error != "sidecar pack digest mismatch" {
		t.Fatalf("wrong sidecar binding: %+v", item)
	}
	if err := os.Remove(sidecarPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "segment.json"), []byte(`{"pack_digest":"sha256:other"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if item := readPackListing(dir); item.Error != "segment pack digest mismatch" {
		t.Fatalf("wrong segment binding: %+v", item)
	}
	if err := os.WriteFile(filepath.Join(dir, "segment.json"), []byte("broken"), 0644); err != nil {
		t.Fatal(err)
	}
	if item := readPackListing(dir); item.Error == "" {
		t.Fatal("malformed segment JSON went unnoticed")
	}
	params := segments.DefaultParams()
	window := segments.Window{Finder: "following", Version: segments.Version, Source: "run", Role: "tuning", StartNs: 1, EndNs: 10_000_000_001}
	window.ID = segments.Identity(window.Finder, window.Source, window.Role, params, window.StartNs)
	record := segments.Record{Schema: "velocity.report/annotation-segment", SchemaVersion: 1, PackDigest: "sha256:pack", Role: "tuning", Finder: "following", FinderVersion: segments.Version, Parameters: params, Segment: window}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "segment.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if item := readPackListing(dir); item.Error != "" || item.SegmentID != window.ID {
		t.Fatalf("valid segment provenance: %+v", item)
	}
}

func TestPackInventoryHandlesMissingAndNestedPackDirectories(t *testing.T) {
	ws, _ := segmentServer(t)
	ws.annotationPacksDir = filepath.Join(t.TempDir(), "missing")
	if response := callSegment(t, ws, "GET", "/api/annotations/packs", nil, ws.handleAnnotationPacks); response.Code != 200 {
		t.Fatalf("missing pack directory: %d", response.Code)
	}
	root := t.TempDir()
	ws.annotationPacksDir = root
	if err := os.WriteFile(filepath.Join(root, "ordinary-file"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "clip", "pack")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "manifest.json"), []byte(`{"dataset_id":"dataset","pack_digest":"sha256:pack"}`), 0644); err != nil {
		t.Fatal(err)
	}
	response := callSegment(t, ws, "GET", "/api/annotations/packs", nil, ws.handleAnnotationPacks)
	if response.Code != 200 || !strings.Contains(response.Body.String(), nested) {
		t.Fatalf("nested inventory: %d %s", response.Code, response.Body.String())
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	ws.annotationPacksDir = file
	if response := callSegment(t, ws, "GET", "/api/annotations/packs", nil, ws.handleAnnotationPacks); response.Code != 500 {
		t.Fatalf("unreadable pack root: %d", response.Code)
	}
}
