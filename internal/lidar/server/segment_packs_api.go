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
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
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
	// A version 2 pack linked to a label set reports the label set's counts,
	// and the capture files the label set has verified hold its packets.
	LabelSetID       string             `json:"label_set_id,omitempty"`
	LabelSetName     string             `json:"label_set_name,omitempty"`
	LabelSetRevision int                `json:"label_set_revision,omitempty"`
	LabelSetPackets  int                `json:"label_set_packets,omitempty"`
	LabelSetWindows  int                `json:"label_set_windows,omitempty"`
	LabelSetCaptures []labelset.Binding `json:"label_set_captures,omitempty"`
	// LabelSetMigratedFrom names the version 1 pack a migrated label set came
	// from; that pack's listing then names this one as MigratedTo.
	LabelSetMigratedFrom string `json:"label_set_migrated_from,omitempty"`
	MigratedTo           string `json:"migrated_to,omitempty"`
	Status               string `json:"status"`
	Error                string `json:"error,omitempty"`
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
	listed := linkMigrations(ws.packListings.list(dirs, time.Now()))
	ws.writeJSON(w, 200, map[string]any{"packs": listed, "count": len(listed)})
}

// linkMigrations marks each version 1 pack whose labels were migrated with
// the version 2 pack that now carries them, so a listing never presents the
// read-only original as the place to keep labelling.
func linkMigrations(listed []packListing) []packListing {
	successors := map[string]string{}
	for _, item := range listed {
		if item.LabelSetMigratedFrom != "" {
			successors[item.LabelSetMigratedFrom] = item.PackDir
		}
	}
	out := make([]packListing, len(listed))
	for i, item := range listed {
		if item.LabelSetID == "" {
			item.MigratedTo = successors[filepath.Base(item.PackDir)]
		}
		out[i] = item
	}
	return out
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
	if link, err := annotation.ReadLabelSetLink(dir); err != nil {
		item.Error = "label set link: " + err.Error()
		return item
	} else if link != nil {
		ls, err := labelset.Open(link.LabelSetDir(dir))
		if err != nil {
			item.Error = "label set: " + err.Error()
			return item
		}
		item.LabelSetID, item.LabelSetName, item.LabelSetRevision = ls.Manifest.LabelSetID, ls.Manifest.Name, ls.Revision()
		item.LabelSetPackets, item.LabelSetWindows = len(ls.State.Packets), len(ls.State.Windows)
		item.LabelSetCaptures = ls.Manifest.AppliesTo
		if m := ls.Manifest.MigratedFrom; m != nil {
			item.LabelSetMigratedFrom = m.PackDir
		}
		item.Objects, item.Masks = len(ls.State.Records.Objects), len(ls.State.Masks)
		for _, obj := range ls.State.Records.Objects {
			if obj.Status == annotation.StatusReviewed {
				item.ReviewedObjects++
			}
		}
		item.ReviewedMasks = len(ls.State.ReviewedMasks())
		if item.ReviewedMasks > 0 {
			item.Status = "reviewed"
		}
		return item
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
