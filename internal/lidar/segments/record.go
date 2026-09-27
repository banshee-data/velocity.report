package segments

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Record is selection provenance. It is written beside manifest.json, outside
// the pack digest, and cannot make a proposal into reviewed truth.
type Record struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	PackDigest    string `json:"pack_digest"`
	Role          string `json:"role"`
	Finder        string `json:"finder"`
	FinderVersion int    `json:"finder_version"`
	Parameters    Params `json:"parameters"`
	Segment       Window `json:"segment"`
}

func (r Record) Validate() error {
	if r.Schema != "velocity.report/annotation-segment" || r.SchemaVersion != 1 || r.PackDigest == "" {
		return fmt.Errorf("invalid segment record schema or pack digest")
	}
	if r.Role != "tuning" && r.Role != "held_out" {
		return fmt.Errorf("invalid pack role %q", r.Role)
	}
	if !Allowed(r.Finder, r.Role) && !(r.Finder == "manual" && r.Role == "tuning") {
		return fmt.Errorf("finder %q cannot choose a %s pack", r.Finder, r.Role)
	}
	if r.FinderVersion < 1 || r.Segment.StartNs >= r.Segment.EndNs {
		return fmt.Errorf("invalid finder version or window")
	}
	if r.Segment.Finder != "" && r.Segment.Finder != r.Finder {
		return fmt.Errorf("finder mismatch")
	}
	return r.Parameters.Validate()
}

func WriteRecord(packDir string, r Record) error {
	if err := r.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(packDir, "segment.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}
