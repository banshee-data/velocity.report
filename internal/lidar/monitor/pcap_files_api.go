package monitor

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// PcapFileInfo describes a single PCAP file found in the safe directory.
type PcapFileInfo struct {
	Path       string `json:"path"`
	SizeBytes  int64  `json:"size_bytes"`
	ModifiedAt string `json:"modified_at"`
	InUse      bool   `json:"in_use"`
}

// handleListPCAPFiles scans the configured PCAP safe directory for .pcap and
// .pcapng files and returns them as JSON. Files already referenced by a scene
// are flagged as in_use.
//
// GET /api/lidar/pcap/files
func (ws *WebServer) handleListPCAPFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if ws.pcapSafeDir == "" {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "PCAP directory not configured")
		return
	}

	safeDirAbs, err := filepath.Abs(ws.pcapSafeDir)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "invalid PCAP directory configuration")
		return
	}

	// Collect scene PCAP files for in_use check.
	usedFiles := make(map[string]bool)
	if ws.db != nil {
		store := lidar.NewSceneStore(ws.db.DB)
		scenes, err := store.ListScenes("")
		if err == nil {
			for _, s := range scenes {
				usedFiles[s.PCAPFile] = true
			}
		}
	}

	const maxFiles = 500
	var files []PcapFileInfo

	_ = filepath.WalkDir(safeDirAbs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip entries we cannot read
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".pcap" && ext != ".pcapng" {
			return nil
		}

		rel, relErr := filepath.Rel(safeDirAbs, path)
		if relErr != nil {
			return nil
		}

		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}

		files = append(files, PcapFileInfo{
			Path:       rel,
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
			InUse:      usedFiles[rel],
		})

		if len(files) >= maxFiles {
			return filepath.SkipAll
		}
		return nil
	})

	if files == nil {
		files = []PcapFileInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pcap_dir": ws.pcapSafeDir,
		"files":    files,
		"count":    len(files),
	})
}
