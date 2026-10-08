package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

type packListing struct {
	PackDir         string                      `json:"pack_dir"`
	DatasetID       string                      `json:"dataset_id"`
	PackDigest      string                      `json:"pack_digest"`
	Source          annotation.SourceProvenance `json:"source"`
	Role            string                      `json:"role,omitempty"`
	SegmentID       string                      `json:"segment_id,omitempty"`
	Objects         int                         `json:"objects"`
	Masks           int                         `json:"masks"`
	ReviewedObjects int                         `json:"reviewed_objects"`
	ReviewedMasks   int                         `json:"reviewed_masks"`
	ProposalLayers  int                         `json:"proposal_layers"`
	Status          string                      `json:"status"`
	Error           string                      `json:"error,omitempty"`
}

func (ws *Server) handleAnnotationPacks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, 405, "method not allowed")
		return
	}
	if ws.annotationPacksDir == "" {
		ws.writeJSONError(w, 501, "annotation packs directory is not configured")
		return
	}
	entries, err := os.ReadDir(ws.annotationPacksDir)
	if os.IsNotExist(err) {
		ws.writeJSON(w, 200, map[string]any{"packs": []packListing{}, "count": 0})
		return
	}
	if err != nil {
		ws.writeJSONError(w, 500, err.Error())
		return
	}
	dirs := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		base := filepath.Join(ws.annotationPacksDir, entry.Name())
		candidate := base
		if _, err := os.Stat(filepath.Join(candidate, "manifest.json")); err != nil {
			candidate = filepath.Join(base, "pack")
		}
		if _, err := os.Stat(filepath.Join(candidate, "manifest.json")); err != nil {
			continue
		}
		dirs = append(dirs, candidate)
	}
	listed := ws.packListings.list(dirs, time.Now())
	ws.writeJSON(w, 200, map[string]any{"packs": listed, "count": len(listed)})
}

func readPackListing(dir string) packListing {
	item := packListing{PackDir: dir, Status: "packed"}
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		item.Error = err.Error()
		return item
	}
	var manifest annotation.Manifest
	if err = json.Unmarshal(b, &manifest); err != nil {
		item.Error = "manifest: " + err.Error()
		return item
	}
	item.DatasetID = manifest.DatasetID
	item.PackDigest = manifest.PackDigest
	item.Source = manifest.Source
	if b, err = os.ReadFile(filepath.Join(dir, "segment.json")); err == nil {
		var record segments.Record
		if err = json.Unmarshal(b, &record); err != nil {
			item.Error = "segment: " + err.Error()
		} else if record.PackDigest != manifest.PackDigest {
			item.Error = "segment pack digest mismatch"
		} else if err := record.Validate(); err != nil {
			item.Error = "segment: " + err.Error()
		} else {
			item.Role = record.Role
			item.SegmentID = record.Segment.ID
		}
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "proposals")); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				item.ProposalLayers++
			}
		}
		if item.ProposalLayers > 0 {
			item.Status = "proposed"
		}
	}
	if b, err = os.ReadFile(filepath.Join(dir, "annotations.json")); err == nil {
		var sidecar annotation.Sidecar
		if err = json.Unmarshal(b, &sidecar); err != nil {
			item.Error = fmt.Sprintf("sidecar: %v", err)
			return item
		}
		if sidecar.PackDigest != manifest.PackDigest {
			item.Error = "sidecar pack digest mismatch"
			return item
		}
		item.Objects = len(sidecar.Objects)
		item.Masks = len(sidecar.Masks)
		reviewed := map[string]bool{}
		for _, obj := range sidecar.Objects {
			if obj.Status == annotation.StatusReviewed {
				reviewed[obj.ObjectID] = true
				item.ReviewedObjects++
			}
		}
		for _, mask := range sidecar.Masks {
			if mask.Status == annotation.StatusReviewed && reviewed[mask.ObjectID] {
				item.ReviewedMasks++
			}
		}
		if item.ReviewedMasks > 0 {
			item.Status = "reviewed"
		}
	}
	return item
}
