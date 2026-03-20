package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
)

// DirectBackend implements sweep.SweepBackend by calling Go methods
// in-process, bypassing HTTP serialisation. It eliminates all network
// overhead and polling loops when the sweep runner and web server live
// in the same binary.
type DirectBackend struct {
	sensorID string
	ws       *WebServer
}

// NewDirectBackend creates a backend that talks directly to the server
// internals. The caller must pass the same webserver instance that
// serves the LiDAR monitor endpoints.
func NewDirectBackend(sensorID string, ws *WebServer) *DirectBackend {
	return &DirectBackend{sensorID: sensorID, ws: ws}
}

// Compile-time check.
var _ sweep.SweepBackend = (*DirectBackend)(nil)

// SensorID returns the sensor identifier.
func (d *DirectBackend) SensorID() string { return d.sensorID }

// --- Acceptance ---

// FetchBuckets returns bucket boundary strings from the BackgroundManager.
func (d *DirectBackend) FetchBuckets() []string {
	mgr := l3grid.GetBackgroundManager(d.sensorID)
	if mgr == nil {
		opsf("WARNING: No background manager for %s (using default buckets)", d.sensorID)
		return DefaultBuckets()
	}
	metrics := mgr.GetAcceptanceMetrics()
	if metrics == nil || len(metrics.BucketsMeters) == 0 {
		return DefaultBuckets()
	}
	buckets := make([]string, 0, len(metrics.BucketsMeters))
	for _, v := range metrics.BucketsMeters {
		if v == float64(int(v)) {
			buckets = append(buckets, fmt.Sprintf("%.0f", v))
		} else {
			buckets = append(buckets, fmt.Sprintf("%.6f", v))
		}
	}
	return buckets
}

// FetchAcceptanceMetrics returns acceptance counters as a generic map,
// matching the JSON shape the HTTP client produces.
func (d *DirectBackend) FetchAcceptanceMetrics() (map[string]interface{}, error) {
	mgr := l3grid.GetBackgroundManager(d.sensorID)
	if mgr == nil {
		return nil, fmt.Errorf("no background manager for sensor %q", d.sensorID)
	}
	metrics := mgr.GetAcceptanceMetrics()
	if metrics == nil {
		metrics = &l3grid.AcceptanceMetrics{}
	}

	totals := make([]int64, len(metrics.BucketsMeters))
	rates := make([]float64, len(metrics.BucketsMeters))
	for i := range metrics.BucketsMeters {
		var a, rj int64
		if i < len(metrics.AcceptCounts) {
			a = metrics.AcceptCounts[i]
		}
		if i < len(metrics.RejectCounts) {
			rj = metrics.RejectCounts[i]
		}
		totals[i] = a + rj
		if totals[i] > 0 {
			rates[i] = float64(a) / float64(totals[i])
		}
	}

	// Build the same map shape as the HTTP handler produces.
	// The Sampler reads BucketsMeters, AcceptCounts, RejectCounts, Totals, AcceptanceRates.
	return map[string]interface{}{
		"BucketsMeters":   toInterfaceSlice(metrics.BucketsMeters),
		"AcceptCounts":    toInterfaceSliceInt64(metrics.AcceptCounts),
		"RejectCounts":    toInterfaceSliceInt64(metrics.RejectCounts),
		"Totals":          toInterfaceSliceInt64(totals),
		"AcceptanceRates": toInterfaceSliceFloat64(rates),
	}, nil
}

// ResetAcceptance zeroes the acceptance counters.
func (d *DirectBackend) ResetAcceptance() error {
	mgr := l3grid.GetBackgroundManager(d.sensorID)
	if mgr == nil {
		return fmt.Errorf("no background manager for sensor %q", d.sensorID)
	}
	return mgr.ResetAcceptanceMetrics()
}

// --- Grid ---

// FetchGridStatus returns background grid statistics.
func (d *DirectBackend) FetchGridStatus() (map[string]interface{}, error) {
	mgr := l3grid.GetBackgroundManager(d.sensorID)
	if mgr == nil {
		return nil, fmt.Errorf("no background manager for sensor %q", d.sensorID)
	}
	status := mgr.GridStatus()
	if status == nil {
		return nil, fmt.Errorf("grid status unavailable")
	}
	return status, nil
}

// ResetGrid resets the background grid, frame builder, and tracker.
func (d *DirectBackend) ResetGrid() error {
	mgr := l3grid.GetBackgroundManager(d.sensorID)
	if mgr == nil {
		return fmt.Errorf("no background manager for sensor %q", d.sensorID)
	}

	// Mirror webserver handleGridReset: frame builder → grid → tracker
	fb := l2frames.GetFrameBuilder(d.sensorID)
	if fb != nil {
		fb.Reset()
	}

	if err := mgr.ResetGrid(); err != nil {
		return err
	}

	if d.ws.tracker != nil {
		d.ws.tracker.Reset()
	}
	return nil
}

// WaitForGridSettle blocks until background_count > 0 or timeout.
func (d *DirectBackend) WaitForGridSettle(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mgr := l3grid.GetBackgroundManager(d.sensorID)
		if mgr != nil {
			status := mgr.GridStatus()
			if status != nil {
				if bc, ok := status["background_count"]; ok {
					switch v := bc.(type) {
					case float64:
						if v > 0 {
							return
						}
					case int:
						if v > 0 {
							return
						}
					case int64:
						if v > 0 {
							return
						}
					}
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// --- Tracking ---

// FetchTrackingMetrics returns velocity-trail alignment metrics as a
// generic map matching the JSON shape produced by the HTTP handler.
func (d *DirectBackend) FetchTrackingMetrics() (map[string]interface{}, error) {
	if d.ws.tracker == nil {
		return nil, fmt.Errorf("in-memory tracker not available")
	}
	metrics := d.ws.tracker.GetTrackingMetrics()

	// Round-trip through JSON to produce the same map[string]interface{}
	// that the HTTP path returns. This keeps the Sampler code unchanged.
	data, err := json.Marshal(metrics)
	if err != nil {
		return nil, fmt.Errorf("marshal tracking metrics: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal tracking metrics: %w", err)
	}
	return m, nil
}

// --- Parameters ---

// SetTuningParams applies a partial tuning config update.
// It mirrors the POST handler for /api/lidar/params.
func (d *DirectBackend) SetTuningParams(params map[string]interface{}) error {
	mgr := l3grid.GetBackgroundManager(d.sensorID)
	if mgr == nil || mgr.Grid == nil {
		return fmt.Errorf("no background manager for sensor %q", d.sensorID)
	}

	patch, err := normaliseTuningPatch(params)
	if err != nil {
		return err
	}
	if err := applyRuntimeTuningPatch(d.ws, mgr, patch); err != nil {
		return err
	}

	data, _ := json.Marshal(patch)
	diagf("[DirectBackend] Applied tuning params: %s", string(data))
	return nil
}

// --- PCAP lifecycle ---

// StartPCAPReplayWithConfig begins a PCAP replay using the WebServer's
// internal PCAP machinery, bypassing HTTP entirely.
func (d *DirectBackend) StartPCAPReplayWithConfig(cfg sweep.PCAPReplayConfig) error {
	speedMode := cfg.SpeedMode
	if speedMode == "" {
		speedMode = "analysis"
	}
	speedRatio := cfg.SpeedRatio
	if speedRatio <= 0 {
		speedRatio = 1.0
	}
	return d.ws.StartPCAPForSweep(
		cfg.PCAPFile, cfg.AnalysisMode, speedMode, speedRatio,
		cfg.StartSeconds, cfg.DurationSeconds, cfg.MaxRetries,
		cfg.DisableRecording)
}

// StopPCAPReplay cancels the running PCAP replay and restores live mode.
func (d *DirectBackend) StopPCAPReplay() error {
	return d.ws.StopPCAPForSweep()
}

// WaitForPCAPComplete blocks on the WebServer's pcapDone channel until
// the replay finishes or timeout elapses. No HTTP overhead or polling.
func (d *DirectBackend) WaitForPCAPComplete(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := d.ws.PCAPDone()
	if done == nil {
		return nil // no replay running
	}
	return sweep.WaitForPCAPDone(ctx, done)
}

// --- Data source ---

// GetLastAnalysisRunID returns the last analysis run ID.
func (d *DirectBackend) GetLastAnalysisRunID() string {
	return d.ws.LastAnalysisRunID()
}

// --- internal helpers ---

func toInterfaceSlice[T any](s []T) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func toInterfaceSliceInt64(s []int64) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func toInterfaceSliceFloat64(s []float64) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
