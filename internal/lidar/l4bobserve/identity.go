package l4bobserve

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
)

// CaptureSource identifies the exact ordered capture sequence that produced
// observations. A site name or an S2 cell is useful context, but it is not
// enough: a second visit to the same place is a different source.
type CaptureSource struct {
	ReplayCaseID   string
	CapturePaths   []string
	CaptureSHA256s []string
	// ExtractorID identifies the configuration and L4 implementation that
	// generated clusters from the raw bytes. A changed segmentation must not
	// collide with evidence from an earlier extraction.
	ExtractorID string
}

// Calibration describes the transform revision that placed sensor points in
// the site frame. The transform itself is included because a pose ID alone
// does not make an imported or hand-authored transform reproducible.
type Calibration struct {
	SensorID  string
	FromFrame string
	ToFrame   string
	Transform [16]float64
}

// SourceID returns a content-addressed, versioned identity for one ordered
// capture sequence. Reordering files deliberately changes the value: packet
// order is evidence, not presentation.
func SourceID(source CaptureSource) (string, error) {
	if strings.TrimSpace(source.ReplayCaseID) == "" {
		return "", fmt.Errorf("replay case identity is required")
	}
	if len(source.CapturePaths) == 0 || len(source.CapturePaths) != len(source.CaptureSHA256s) {
		return "", fmt.Errorf("at least one ordered capture path is required")
	}
	if strings.TrimSpace(source.ExtractorID) == "" {
		return "", fmt.Errorf("extractor identity is required")
	}
	h := sha256.New()
	writeIdentityPart(h, "observation-source-v1")
	writeIdentityPart(h, source.ReplayCaseID)
	for i, capture := range source.CapturePaths {
		if strings.TrimSpace(capture) == "" {
			return "", fmt.Errorf("capture path is required")
		}
		digest := strings.TrimSpace(source.CaptureSHA256s[i])
		if len(digest) != 64 {
			return "", fmt.Errorf("capture %q needs a SHA-256 digest", capture)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return "", fmt.Errorf("capture %q has invalid SHA-256 digest: %w", capture, err)
		}
		writeIdentityPart(h, capture)
		writeIdentityPart(h, strings.ToLower(digest))
	}
	writeIdentityPart(h, source.ExtractorID)
	return "source/v1/" + hex.EncodeToString(h.Sum(nil)), nil
}

// CalibrationID returns a content-addressed identity for the transform used
// to place an observation in the site frame. A changed transform must produce
// a different observation identity even when the raw PCAP is unchanged.
func CalibrationID(calibration Calibration) (string, error) {
	if strings.TrimSpace(calibration.SensorID) == "" || strings.TrimSpace(calibration.FromFrame) == "" || strings.TrimSpace(calibration.ToFrame) == "" {
		return "", fmt.Errorf("sensor, source frame, and site frame are required")
	}
	h := sha256.New()
	writeIdentityPart(h, "observation-calibration-v1")
	writeIdentityPart(h, calibration.SensorID)
	writeIdentityPart(h, calibration.FromFrame)
	writeIdentityPart(h, calibration.ToFrame)
	for _, value := range calibration.Transform {
		writeIdentityPart(h, fmt.Sprintf("%016x", canonicalFloat64(value)))
	}
	return "calibration/v1/" + hex.EncodeToString(h.Sum(nil)), nil
}

// ObservationID returns the identity of one cluster in one captured frame.
// It binds source and calibration identities so the same cluster reprocessed
// under a revised transform cannot overwrite the original evidence.
func ObservationID(sourceID, calibrationID string, frameUnixNanos, clusterID int64) (string, error) {
	if strings.TrimSpace(sourceID) == "" || strings.TrimSpace(calibrationID) == "" {
		return "", fmt.Errorf("source and calibration identities are required")
	}
	h := sha256.New()
	writeIdentityPart(h, "observation-v1")
	writeIdentityPart(h, sourceID)
	writeIdentityPart(h, calibrationID)
	writeIdentityPart(h, fmt.Sprintf("%d", frameUnixNanos))
	writeIdentityPart(h, fmt.Sprintf("%d", clusterID))
	return "observation/v1/" + hex.EncodeToString(h.Sum(nil)), nil
}

func writeIdentityPart(h interface{ Write([]byte) (int, error) }, value string) {
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}

func canonicalFloat64(value float64) uint64 {
	// The numeric transform is already canonical data. Formatting its IEEE-754
	// representation avoids locale and insignificant-decimal differences.
	return math.Float64bits(value)
}
