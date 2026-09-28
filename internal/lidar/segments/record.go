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
	// Selector is the selector that chose the segment, as it ran. A pack cut
	// before selectors existed, or from a hand-chosen window, has none.
	Selector *SelectorProvenance `json:"selector,omitempty"`
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
	if err := r.Parameters.Validate(); err != nil {
		return err
	}
	if r.Selector != nil {
		if err := r.Selector.check(r.Finder, r.Role, r.Parameters); err != nil {
			return err
		}
	}
	if r.Finder != "manual" && (r.Segment.Version != r.FinderVersion || r.Segment.Source == "" ||
		r.Segment.Role != r.Role || r.Segment.ID != identityAtVersion(r.FinderVersion, r.Finder, r.Segment.Source, r.Role, r.Parameters, r.Segment.StartNs) ||
		r.Segment.EndNs-r.Segment.StartNs != int64(r.Parameters.WindowSeconds*1e9)) {
		return fmt.Errorf("segment identity or parameters do not match")
	}
	return nil
}

type recordOutput interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

func WriteRecord(packDir string, r Record) error {
	return writeRecordWithOpen(packDir, r, func(path string) (recordOutput, error) {
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	})
}

func writeRecordWithOpen(packDir string, r Record, open func(string) (recordOutput, error)) error {
	if err := r.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(packDir, "segment.json")
	f, err := open(path)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		_ = f.Close()
		if !complete {
			_ = os.Remove(path)
		}
	}()
	if _, err = f.Write(append(b, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}
