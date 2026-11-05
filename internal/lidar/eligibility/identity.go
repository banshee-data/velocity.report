package eligibility

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Identity is an input as the decision sees it. For a capture file it is the
// SHA-256 of the file's bytes, computed here from the file and never taken
// from a client; the basename is kept to notice name collisions and for
// people reading the record. For a live source it is the sensor.
type Identity struct {
	Kind     string `json:"kind"` // "capture" or "live"
	SHA256   string `json:"sha256,omitempty"`
	Basename string `json:"basename,omitempty"`
	Bytes    int64  `json:"bytes,omitempty"`
	SensorID string `json:"sensor_id,omitempty"`
}

const (
	KindCapture = "capture"
	KindLive    = "live"
)

// Hasher hashes capture files, keeping each result against the file's path,
// size and modification time so a capture of hundreds of megabytes is read
// once per change rather than once per request.
type Hasher struct {
	mu    sync.Mutex
	cache map[string]hashEntry
}

type hashEntry struct {
	size  int64
	modNs int64
	sum   string
}

// NewHasher returns an empty hasher.
func NewHasher() *Hasher { return &Hasher{cache: map[string]hashEntry{}} }

// Capture identifies the capture at path by its bytes.
func (h *Hasher) Capture(path string) (Identity, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Identity{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Identity{}, err
	}
	if !info.Mode().IsRegular() {
		return Identity{}, fmt.Errorf("%s is not a regular file", path)
	}
	id := Identity{Kind: KindCapture, Basename: filepath.Base(abs), Bytes: info.Size()}
	h.mu.Lock()
	e, ok := h.cache[abs]
	h.mu.Unlock()
	if ok && e.size == info.Size() && e.modNs == info.ModTime().UnixNano() {
		id.SHA256 = e.sum
		return id, nil
	}
	sum, err := FileSHA256(abs)
	if err != nil {
		return Identity{}, err
	}
	// A file rewritten while it was read would be cached under the new
	// stamp with the old bytes' hash; stat again and keep it only if still
	// unchanged.
	if after, err := os.Stat(abs); err == nil && after.Size() == info.Size() && after.ModTime().Equal(info.ModTime()) {
		h.mu.Lock()
		h.cache[abs] = hashEntry{size: info.Size(), modNs: info.ModTime().UnixNano(), sum: sum}
		h.mu.Unlock()
	}
	id.SHA256 = sum
	return id, nil
}

// Live identifies a live sensor.
func Live(sensorID string) Identity { return Identity{Kind: KindLive, SensorID: sensorID} }

// FileSHA256 is a file's SHA-256 as "sha256:<hex>", streamed.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
