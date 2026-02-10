package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

func TestHandleListPCAPFiles_MethodNotAllowed(t *testing.T) {
	ws := &WebServer{pcapSafeDir: t.TempDir()}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/files", nil)
	w := httptest.NewRecorder()

	ws.handleListPCAPFiles(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleListPCAPFiles_DirectoryNotConfigured(t *testing.T) {
	ws := &WebServer{}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/pcap/files", nil)
	w := httptest.NewRecorder()

	ws.handleListPCAPFiles(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHandleListPCAPFiles_EmptyDirectory(t *testing.T) {
	ws := &WebServer{pcapSafeDir: t.TempDir()}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/pcap/files", nil)
	w := httptest.NewRecorder()

	ws.handleListPCAPFiles(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		Files []PcapFileInfo `json:"files"`
		Count int            `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Count != 0 {
		t.Fatalf("expected count 0, got %d", resp.Count)
	}
	if resp.Files == nil {
		t.Fatal("expected empty files array, got nil")
	}
}

func TestHandleListPCAPFiles_SuccessAndInUseFlags(t *testing.T) {
	safeDir := t.TempDir()
	nestedDir := filepath.Join(safeDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	fileA := filepath.Join(safeDir, "a.pcap")
	fileB := filepath.Join(nestedDir, "b.pcapng")
	fileIgnored := filepath.Join(safeDir, "ignored.txt")
	if err := os.WriteFile(fileA, []byte("a"), 0o644); err != nil {
		t.Fatalf("write fileA: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("b"), 0o644); err != nil {
		t.Fatalf("write fileB: %v", err)
	}
	if err := os.WriteFile(fileIgnored, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fileIgnored: %v", err)
	}

	testDB := setupTestSceneAPIDB(t)
	defer testDB.DB.Close()

	// Mark both PCAP files as in use: one via relative path, one via absolute path.
	store := lidar.NewSceneStore(testDB.DB)
	sceneRel := &lidar.Scene{SensorID: "sensor-1", PCAPFile: "a.pcap"}
	if err := store.InsertScene(sceneRel); err != nil {
		t.Fatalf("insert relative-path scene: %v", err)
	}

	sceneAbs := &lidar.Scene{SensorID: "sensor-1", PCAPFile: fileB}
	if err := store.InsertScene(sceneAbs); err != nil {
		t.Fatalf("insert absolute-path scene: %v", err)
	}

	ws := &WebServer{
		pcapSafeDir: safeDir,
		db:          testDB,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/pcap/files", nil)
	w := httptest.NewRecorder()
	ws.handleListPCAPFiles(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Files []PcapFileInfo `json:"files"`
		Count int            `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Count != 2 {
		t.Fatalf("expected count 2, got %d", resp.Count)
	}

	filesByPath := make(map[string]PcapFileInfo, len(resp.Files))
	for _, fi := range resp.Files {
		filesByPath[fi.Path] = fi
	}

	if !filesByPath["a.pcap"].InUse {
		t.Fatalf("expected a.pcap to be marked in_use")
	}
	if !filesByPath[filepath.Join("nested", "b.pcapng")].InUse {
		t.Fatalf("expected nested/b.pcapng to be marked in_use")
	}
}
