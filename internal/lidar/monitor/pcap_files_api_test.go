package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

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
	store := sqlite.NewSceneStore(testDB.DB)
	sceneRel := &sqlite.Scene{SensorID: "sensor-1", PCAPFile: "a.pcap"}
	if err := store.InsertScene(sceneRel); err != nil {
		t.Fatalf("insert relative-path scene: %v", err)
	}

	sceneAbs := &sqlite.Scene{SensorID: "sensor-1", PCAPFile: fileB}
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

func TestHandleListPCAPFiles_RespectsFileLimit(t *testing.T) {
	safeDir := t.TempDir()

	// Create more than the hard limit (500) valid PCAP files.
	for i := 0; i < 505; i++ {
		name := filepath.Join(safeDir, fmt.Sprintf("f%03d.pcap", i))
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	ws := &WebServer{pcapSafeDir: safeDir}
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

	if resp.Count != 500 {
		t.Fatalf("expected file count capped at 500, got %d", resp.Count)
	}
	if len(resp.Files) != 500 {
		t.Fatalf("expected 500 returned files, got %d", len(resp.Files))
	}
}

func TestHandleListPCAPFiles_SceneListErrorStillServesFiles(t *testing.T) {
	safeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(safeDir, "only.pcap"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write only.pcap: %v", err)
	}

	testDB := setupTestSceneAPIDB(t)
	// Close underlying DB to force ListScenes to fail.
	testDB.DB.Close()

	ws := &WebServer{
		pcapSafeDir: safeDir,
		db:          testDB,
	}

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

	if resp.Count != 1 || len(resp.Files) != 1 {
		t.Fatalf("expected exactly one listed file, got count=%d len=%d", resp.Count, len(resp.Files))
	}
	if resp.Files[0].InUse {
		t.Fatalf("expected in_use=false when scene lookup fails")
	}
}
