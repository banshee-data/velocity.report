package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
	"github.com/banshee-data/velocity.report/internal/version"
)

// Live observation capture is opt-in (--lidar-observation-dir, empty by
// default) and experimental. It commits the live pipeline's
// foreground-complete L4 frames through the durable VRLOG writer, before and
// independently of L5. The durability evidence behind a live default
// (power loss and sustained load on the target Raspberry Pi and its storage)
// does not exist yet; nothing here is enabled unless an operator asks.
//
// One container holds one extraction of one live session. The tap numbers
// frames from the start of the process, and a container's records must run
// on from sequence zero under one set of identities, so the capture ends,
// cleanly, at the first frame that would break that: the pipeline leaving
// live input (a PCAP replay), or a tuning change, which changes the
// extractor. Starting a new container at such a boundary is future work.

// lidarFrameChCapacity is the L2 frame builder's callback channel: frames
// wait there before the pipeline, and so before the observation writer.
const lidarFrameChCapacity = 32

// liveObservationPolicy is the provisional group-commit policy with the two
// live-specific declarations: frames wait in the L2 callback channel before
// the writer, and a sensor cannot be paused, so a frame the writer cannot
// admit in time is recorded as a gap rather than blocking the callback.
func liveObservationPolicy(frameChCapacity int) vrlog.CommitPolicy {
	p := vrlog.DefaultCommitPolicy()
	// A full callback channel at the slowest rotation (10 Hz) is the
	// upstream queue age: the bound this capture declares, not a measurement.
	p.UpstreamQueueAge = time.Duration(frameChCapacity) * 100 * time.Millisecond
	p.ShedAfter = 50 * time.Millisecond
	return p
}

// liveObservationCapture is the pipeline's observation frame sink for a live
// capture.
type liveObservationCapture struct {
	writer *vrlog.Writer
	// live reports whether the pipeline is fed by the sensor rather than a
	// replay.
	live  func() bool
	logf  func(string, ...any)
	mu    sync.Mutex
	ended bool
}

// liveObservationSetup is what the pipeline needs to tap for the capture.
type liveObservationSetup struct {
	capture       *liveObservationCapture
	sourceID      string
	calibrationID string
}

// startLiveObservationCapture creates a container under root for sensorID.
// tuningJSON is the effective tuning, embedded and hashed into the
// extractor identity.
func startLiveObservationCapture(root, sensorID string, tuningJSON []byte, policy vrlog.CommitPolicy,
	live func() bool, logf func(string, ...any)) (*liveObservationSetup, error) {
	captureUUID := uuid.NewString()
	// A live source's identity is its capture session (VRLOG plan §3.1); a
	// content digest of what it produced can supplement it later.
	sourceID := "source/live/v1/" + captureUUID
	// The live pipeline applies no pose: TransformToWorld with a nil pose is
	// the identity. This is the transform its coordinates were actually
	// produced with, not a guess at where the sensor is mounted.
	calibration := l4bobserve.Calibration{SensorID: sensorID, FromFrame: "sensor", ToFrame: "site/" + sensorID,
		Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	calibrationID, err := l4bobserve.CalibrationID(calibration)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(tuningJSON)
	paramsHash := "sha256:" + hex.EncodeToString(sum[:])
	name := fmt.Sprintf("%s-%s.vrlog", time.Now().UTC().Format("20060102T150405Z"), captureUUID[:8])
	w, err := vrlog.Create(filepath.Join(root, name), vrlog.Manifest{
		Capture: vrlog.CaptureIdentity{UUID: captureUUID, SensorID: sensorID, SourceType: "live"},
		Extraction: vrlog.ExtractionIdentity{SourceID: sourceID, CalibrationID: calibrationID,
			CoordinateFrame: "site/" + sensorID, ExtractorID: "l4.dbscan_xy/v1/" + paramsHash},
		Calibration: calibration,
		Commit:      policy,
		Provenance: vrlog.Provenance{BuildVersion: version.Version, BuildGitSHA: version.GitSHA, Writer: "velocity-report live",
			ParamsHash: paramsHash},
		Metadata: []vrlog.MetadataObject{vrlog.NewMetadataObject("tuning", "application/json", tuningJSON)},
	})
	if err != nil {
		return nil, fmt.Errorf("create live observation capture: %w", err)
	}
	loss := w.Policy().CrashLoss(w.Manifest().Limits)
	logf("[observations] live capture %s at %s: crash loss up to %s (process crash; power loss unproven)",
		captureUUID, w.Dir(), loss.Interval)
	return &liveObservationSetup{
		capture:  &liveObservationCapture{writer: w, live: live, logf: logf},
		sourceID: sourceID, calibrationID: calibrationID,
	}, nil
}

// ObserveFrame commits f while the pipeline is live. The first frame from
// any other source ends the capture; later frames are not recorded.
func (c *liveObservationCapture) ObserveFrame(f l4bobserve.FrameRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ended {
		return nil
	}
	if !c.live() {
		c.endLocked("the pipeline left live input")
		return nil
	}
	err := c.writer.AppendFrame(f)
	var failed *vrlog.CaptureFailedError
	if errors.As(err, &failed) {
		c.ended = true
		c.logf("[observations] live capture failed and stopped admitting frames: %v", failed)
		_, _ = c.writer.Close()
	}
	return err
}

// End closes the capture for a stated reason; it is idempotent.
func (c *liveObservationCapture) End(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.endLocked(reason)
}

func (c *liveObservationCapture) endLocked(reason string) {
	if c.ended {
		return
	}
	c.ended = true
	summary, err := c.writer.Close()
	if err != nil {
		c.logf("[observations] live capture ended (%s) with an error: %v", reason, err)
		return
	}
	c.logf("[observations] live capture closed (%s): %d frames, %d gaps, %d shed, to sequence %d",
		reason, summary.Frames, summary.Gaps, summary.ShedFrames, summary.EndSequence)
}
