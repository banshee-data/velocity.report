package followingreview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

// GeometryPath is a projection, not a new fit. The exact canonical LocalPath
// is retained; quadratic lists of unrelated actors are deliberately omitted.
type GeometryPath struct {
	FollowerTrackID string                      `json:"follower_track_id"`
	MethodID        string                      `json:"method_id"`
	First           int64                       `json:"window_first_unix_nanos"`
	Last            int64                       `json:"window_last_unix_nanos"`
	Conditions      []l8behaviour.PathCondition `json:"conditions,omitempty"`
	Path            *l8behaviour.LocalPath      `json:"path,omitempty"`
}

type canonical struct {
	Context fieldrun.EvidenceContext
	Version l8behaviour.InteractionVersion
	Paths   []GeometryPath
	SHA     string
}

const maxGeometryBytes = 64 << 20

// readCanonical verifies the entire byte stream once without retaining raw
// trajectories, row arrays or pairwise non-membership lists in memory.
func readCanonical(r io.Reader) (canonical, error) {
	return readCanonicalLimit(r, maxGeometryBytes)
}

func readCanonicalLimit(r io.Reader, limit int) (canonical, error) {
	var c canonical
	h := sha256.New()
	d := json.NewDecoder(io.TeeReader(r, h))
	d.UseNumber()
	seen := map[string]bool{}
	err := object(d, func(key string) error {
		seen[key] = true
		switch key {
		case "schema":
			var schema string
			if err := d.Decode(&schema); err != nil {
				return err
			}
			if schema != fieldrun.EvidenceSchema {
				return fmt.Errorf("unsupported canonical schema")
			}
		case "context":
			return d.Decode(&c.Context)
		case "analysis_version":
			return d.Decode(&c.Version)
		case "analysis":
			return object(d, func(key string) error {
				if key != "paths" {
					return skip(d, 0)
				}
				t, err := d.Token()
				if err != nil {
					return err
				}
				if t != json.Delim('[') {
					return fmt.Errorf("canonical paths must be array")
				}
				ids := map[string]bool{}
				size := 0
				for d.More() {
					var p l8behaviour.LocalPathResult
					if err := d.Decode(&p); err != nil {
						return err
					}
					if p.FollowerTrackID == "" || ids[p.FollowerTrackID] {
						return fmt.Errorf("duplicate or missing path follower")
					}
					ids[p.FollowerTrackID] = true
					if p.Path != nil {
						if err := p.Path.Validate(); err != nil {
							return err
						}
					}
					g := GeometryPath{p.FollowerTrackID, p.MethodID, p.WindowFirstUnixNanos, p.WindowLastUnixNanos, p.Conditions, p.Path}
					// Decoding JSON into these concrete fields already excludes
					// non-finite numbers and unsupported serialisation types.
					b, _ := json.Marshal(g)
					size += len(b)
					if size > limit {
						return fmt.Errorf("canonical geometry exceeds bounded review cache")
					}
					c.Paths = append(c.Paths, g)
				}
				_, err = d.Token()
				return err
			})
		default:
			return skip(d, 0)
		}
		return nil
	})
	if err != nil {
		return c, err
	}
	if _, err := d.Token(); err != io.EOF {
		return c, fmt.Errorf("canonical trailing JSON: %v", err)
	}
	if !seen["schema"] || !seen["context"] || !seen["analysis_version"] || !seen["analysis"] || c.Version.MethodID == "" {
		return c, fmt.Errorf("canonical identity is incomplete")
	}
	if err := c.Version.Validate(); err != nil {
		return c, err
	}
	c.SHA = "sha256:" + hex.EncodeToString(h.Sum(nil))
	return c, nil
}

func object(d *json.Decoder, visit func(string) error) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	if t != json.Delim('{') {
		return fmt.Errorf("expected JSON object")
	}
	seen := map[string]bool{}
	for d.More() {
		t, err = d.Token()
		if err != nil {
			return err
		}
		// Token guarantees string keys inside a decoded object.
		key := t.(string)
		if seen[key] {
			return fmt.Errorf("duplicate JSON key %s", key)
		}
		seen[key] = true
		if err = visit(key); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}

func skip(d *json.Decoder, depth int) error {
	if depth > 128 {
		return fmt.Errorf("JSON nesting exceeds review limit")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); ok {
		for d.More() {
			if delim == '{' {
				if _, err = d.Token(); err != nil {
					return err
				}
			}
			if err = skip(d, depth+1); err != nil {
				return err
			}
		}
		_, err = d.Token()
	}
	return err
}
