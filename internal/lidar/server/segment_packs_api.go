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
	// A pack linked to a label set reports the label set's counts,
	// and the capture files the label set has verified hold its packets.
	LabelSetID       string             `json:"label_set_id,omitempty"`
	LabelSetName     string             `json:"label_set_name,omitempty"`
	LabelSetRevision int                `json:"label_set_revision,omitempty"`
	LabelSetPackets  int                `json:"label_set_packets,omitempty"`
	LabelSetWindows  int                `json:"label_set_windows,omitempty"`
	LabelSetCaptures []labelset.Binding `json:"label_set_captures,omitempty"`
	Status           string             `json:"status"`
	Error            string             `json:"error,omitempty"`
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
	// Every pack position of the layout: <capture>/packs/<run>,
	// <capture>/<set>/views/<view> and clip-<job>/pack.
	dirs, err := annotation.ListPackDirs(ws.annotationPacksDir)
	if err != nil {
		ws.writeJSONError(w, 500, err.Error())
		return
	}
	if dirs == nil {
		dirs = []string{}
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
	if manifest.SchemaVersion != annotation.PackSchemaVersion {
		item.Error = fmt.Sprintf("pack schema version %d, this build reads %d only (a version 1 pack is read by the git tag annotation-v1-reader)",
			manifest.SchemaVersion, annotation.PackSchemaVersion)
		return item
	}
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
	// A pack's labels are in the label set it links; a pack with no link has
	// none yet.
	link, err := annotation.ReadLabelSetLink(dir)
	if err != nil {
		item.Error = "label set link: " + err.Error()
		return item
	}
	if link == nil {
		return item
	}
	ls, err := labelset.Open(link.LabelSetDir(dir))
	if err != nil {
		item.Error = "label set: " + err.Error()
		return item
	}
	item.LabelSetID, item.LabelSetName, item.LabelSetRevision = ls.Manifest.LabelSetID, ls.Manifest.Name, ls.Revision()
	item.LabelSetPackets, item.LabelSetWindows = len(ls.State.Packets), len(ls.State.Windows)
	item.LabelSetCaptures = ls.Manifest.AppliesTo
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
