package jobs

import (
	"fmt"
	"path"
	"strings"
)

// CaptureManifest lists the captures a job replays, in replay order, by
// content. Two hosts that each hold a file called kirk0.pcapng have the same
// input only if the bytes hash the same; the name is a label.
//
// The field names match the source manifest the state-estimation baseline
// tool writes, so a manifest it wrote validates here unchanged.
type CaptureManifest struct {
	SchemaVersion int `json:"schema_version"`
	// SensorID is the sensor the captures came from. The replay contract
	// names it too; here it says what the bytes are.
	SensorID string    `json:"sensor_id"`
	Captures []Capture `json:"captures"`
}

// Capture is one file in a manifest.
type Capture struct {
	// LogicalID is a stable human label, such as "morg0".
	LogicalID string `json:"id"`
	// Ordinal is the file's position in replay order, from zero, dense.
	Ordinal int `json:"ordinal"`
	// RelativePath is where a worker normally finds the file under its
	// configured capture root. It is a hint, never an authority: the bytes
	// are checked before a job runs.
	RelativePath string `json:"relative_path"`
	ByteSize     int64  `json:"byte_size"`
	SHA256       Digest `json:"sha256"`
	// Container is "pcap" or "pcapng"; UDPPort is the port the LiDAR packets
	// are on. Both are how the file is to be read, recorded so a replay does
	// not have to guess.
	Container string `json:"container,omitempty"`
	UDPPort   int    `json:"udp_port,omitempty"`
}

// CaptureManifestSchemaVersion is the layout this package writes and reads.
const CaptureManifestSchemaVersion = 1

// Validate checks the manifest is complete and internally consistent.
func (m CaptureManifest) Validate() error {
	if m.SchemaVersion != CaptureManifestSchemaVersion {
		return fmt.Errorf("capture manifest schema %d, this build reads %d", m.SchemaVersion, CaptureManifestSchemaVersion)
	}
	if strings.TrimSpace(m.SensorID) == "" {
		return fmt.Errorf("capture manifest has no sensor_id")
	}
	if len(m.Captures) == 0 {
		return fmt.Errorf("capture manifest lists no captures")
	}
	seen := make(map[Digest]int, len(m.Captures))
	for i, c := range m.Captures {
		if c.Ordinal != i {
			return fmt.Errorf("capture %d carries ordinal %d: ordinals must be dense and in order", i, c.Ordinal)
		}
		if strings.TrimSpace(c.LogicalID) == "" {
			return fmt.Errorf("capture %d has no id", i)
		}
		if err := validRelativePath(c.RelativePath); err != nil {
			return fmt.Errorf("capture %d (%s): %w", i, c.LogicalID, err)
		}
		if c.ByteSize <= 0 {
			return fmt.Errorf("capture %d (%s) declares %d bytes", i, c.LogicalID, c.ByteSize)
		}
		if !c.SHA256.Valid() {
			return fmt.Errorf("capture %d (%s) has no valid sha256", i, c.LogicalID)
		}
		if prev, dup := seen[c.SHA256]; dup {
			return fmt.Errorf("capture %d (%s) has the same bytes as capture %d", i, c.LogicalID, prev)
		}
		seen[c.SHA256] = i
		switch c.Container {
		case "", "pcap", "pcapng":
		default:
			return fmt.Errorf("capture %d (%s): unknown container %q", i, c.LogicalID, c.Container)
		}
		if c.UDPPort < 0 || c.UDPPort > 65535 {
			return fmt.Errorf("capture %d (%s): udp_port %d out of range", i, c.LogicalID, c.UDPPort)
		}
	}
	return nil
}

// validRelativePath admits only a clean relative path with no parent steps.
// A hint that pointed outside a worker's root would be an instruction to read
// outside it, and the worker must not take one from a manifest.
func validRelativePath(p string) error {
	if p == "" {
		return fmt.Errorf("no relative_path")
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		return fmt.Errorf("relative_path %q is absolute", p)
	}
	if strings.Contains(p, "\\") {
		return fmt.Errorf("relative_path %q uses backslashes", p)
	}
	cleaned := path.Clean(p)
	if cleaned != p || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("relative_path %q is not a clean path inside the root", p)
	}
	return nil
}

// Digest is the manifest's identity: the canonical JSON of everything but
// the path hints. Two manifests that name the same bytes in the same order
// are the same input wherever the files live.
func (m CaptureManifest) Digest() (Digest, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	type identityCapture struct {
		LogicalID string `json:"id"`
		Ordinal   int    `json:"ordinal"`
		ByteSize  int64  `json:"byte_size"`
		SHA256    Digest `json:"sha256"`
		Container string `json:"container,omitempty"`
		UDPPort   int    `json:"udp_port,omitempty"`
	}
	type identity struct {
		SchemaVersion int               `json:"schema_version"`
		SensorID      string            `json:"sensor_id"`
		Captures      []identityCapture `json:"captures"`
	}
	id := identity{SchemaVersion: m.SchemaVersion, SensorID: m.SensorID}
	for _, c := range m.Captures {
		id.Captures = append(id.Captures, identityCapture{
			LogicalID: c.LogicalID, Ordinal: c.Ordinal, ByteSize: c.ByteSize, SHA256: c.SHA256,
			Container: c.Container, UDPPort: c.UDPPort,
		})
	}
	return DigestCanonical(id)
}

// CaptureDigests is the set of content digests the manifest needs, in order:
// what a worker advertises having, and what it verifies before running.
func (m CaptureManifest) CaptureDigests() []Digest {
	out := make([]Digest, len(m.Captures))
	for i, c := range m.Captures {
		out[i] = c.SHA256
	}
	return out
}
