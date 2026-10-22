package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/pcapsplit"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Static periods as places: the settled background an operator aligns on a
// map, and the clip that carries the location once it is aligned.
//
// Routes:
// - GET  /api/lidar/capture/periods/background?period_id=  — settled background, sensor frame
// - POST /api/lidar/capture/periods/clip                  — the period's clip, created if none

// periodBackgroundFunc builds a period's background. A variable so the
// handler can be tested without libpcap.
var periodBackgroundFunc = periodBackground

// backgroundCache keeps the last few backgrounds, and lets one request build
// a period's background while others for the same period wait for it.
type backgroundCache struct {
	mu       sync.Mutex
	done     map[string]*pcapsplit.Background
	order    []string
	inflight map[string]chan struct{}
}

const backgroundCacheSize = 8

func (c *backgroundCache) get(ctx context.Context, key string,
	build func(context.Context) (*pcapsplit.Background, error)) (*pcapsplit.Background, error) {
	for {
		c.mu.Lock()
		if c.done == nil {
			c.done = map[string]*pcapsplit.Background{}
			c.inflight = map[string]chan struct{}{}
		}
		if bg, ok := c.done[key]; ok {
			c.mu.Unlock()
			return bg, nil
		}
		wait, busy := c.inflight[key]
		if !busy {
			ch := make(chan struct{})
			c.inflight[key] = ch
			c.mu.Unlock()
			bg, err := build(ctx)
			c.mu.Lock()
			delete(c.inflight, key)
			if err == nil {
				c.done[key] = bg
				c.order = append(c.order, key)
				if len(c.order) > backgroundCacheSize {
					delete(c.done, c.order[0])
					c.order = c.order[1:]
				}
			}
			close(ch)
			c.mu.Unlock()
			return bg, err
		}
		c.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// periodCaptures is a period's session captures from the first one that
// reaches the period's start, with their resolved paths and the period's start
// measured from the first of them.
type periodCaptures struct {
	period    sqlite.MotionPeriod
	files     []sqlite.CaptureFile
	paths     []string
	startSecs float64
}

func (ws *Server) loadPeriodCaptures(periodID string) (periodCaptures, int, error) {
	store, err := ws.captureStore()
	if err != nil {
		return periodCaptures{}, http.StatusServiceUnavailable, err
	}
	period, err := store.GetPeriod(periodID)
	if err != nil {
		if errors.Is(err, sqlite.ErrNotFound) {
			return periodCaptures{}, http.StatusNotFound, fmt.Errorf("no period %s", periodID)
		}
		return periodCaptures{}, http.StatusInternalServerError, err
	}
	files, err := store.SessionFiles(period.SessionID)
	if err != nil {
		return periodCaptures{}, http.StatusInternalServerError, err
	}
	sort.SliceStable(files, func(i, j int) bool { return firstPacket(files[i]) < firstPacket(files[j]) })
	var picked []sqlite.CaptureFile
	for _, f := range files {
		if f.FirstPacketNs == nil || f.LastPacketNs == nil {
			continue
		}
		if *f.LastPacketNs <= period.StartNs || *f.FirstPacketNs >= period.EndNs {
			continue
		}
		picked = append(picked, f)
	}
	if len(picked) == 0 {
		return periodCaptures{}, http.StatusConflict, fmt.Errorf("no probed capture covers period %s", periodID)
	}
	paths := make([]string, 0, len(picked))
	for _, f := range picked {
		resolved, err := ws.resolvePCAPPath(f.RelPath)
		if err != nil {
			resolved, err = ws.resolveCapturePath(f.RootID, f.RelPath)
			if err != nil {
				return periodCaptures{}, http.StatusConflict, fmt.Errorf("resolving %s: %w", f.RelPath, err)
			}
		}
		paths = append(paths, resolved)
	}
	start := math.Max(0, float64(period.StartNs-*picked[0].FirstPacketNs)/1e9)
	return periodCaptures{period: period, files: picked, paths: paths, startSecs: start}, 0, nil
}

func firstPacket(f sqlite.CaptureFile) int64 {
	if f.FirstPacketNs == nil {
		return math.MaxInt64
	}
	return *f.FirstPacketNs
}

// handleCapturePeriodBackground returns a static period's settled background
// as sensor-frame points, flattened x, y, z in metres to the centimetre.
func (ws *Server) handleCapturePeriodBackground(w http.ResponseWriter, r *http.Request) {
	periodID := r.URL.Query().Get("period_id")
	if periodID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "period_id is required")
		return
	}
	pc, status, err := ws.loadPeriodCaptures(periodID)
	if err != nil {
		ws.writeJSONError(w, status, err.Error())
		return
	}
	if pc.period.Type != sqlite.PeriodStatic {
		ws.writeJSONError(w, http.StatusBadRequest, "a settled background needs a static period")
		return
	}
	window := math.Min(float64(pc.period.DurationNs)/1e9, pcapsplit.BackgroundSettleWindow.Seconds())
	bg, err := ws.backgrounds.get(r.Context(), periodID, func(ctx context.Context) (*pcapsplit.Background, error) {
		// The request's context ends when the operator navigates away; the
		// build continues for whoever asks next, up to a bound.
		buildCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
		defer cancel()
		return periodBackgroundFunc(buildCtx, pc.paths, indexedExtents(pc.files, pc.paths, os.Stat),
			sessionUDPPort(pc.files, ws.udpPort), ws.snapshotTuningConfig(), pc.startSecs, window)
	})
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	xyz := make([]float64, 0, 3*len(bg.Points))
	for _, p := range bg.Points {
		xyz = append(xyz, centimetres(p.X), centimetres(p.Y), centimetres(p.Z))
	}
	writeCaptureJSON(w, map[string]any{
		"period_id":         periodID,
		"session_id":        pc.period.SessionID,
		"frames":            bg.Frames,
		"window_start_ns":   bg.FirstTime.UnixNano(),
		"window_end_ns":     bg.LastTime.UnixNano(),
		"settled_threshold": bg.SettledThreshold,
		"count":             len(bg.Points),
		"frame":             "sensor",
		"xyz":               xyz,
	})
}

func centimetres(v float32) float64 {
	return math.Round(float64(v)*100) / 100
}

// PeriodClipRequest names the period to cut a clip from.
type PeriodClipRequest struct {
	PeriodID string `json:"period_id"`
	SensorID string `json:"sensor_id,omitempty"`
}

// handleCapturePeriodClip returns the clip cut from a period, creating it when
// there is none: the period's captures, trimmed to the period, with the period
// and session recorded. It is how the map locates a period that has no clip.
func (ws *Server) handleCapturePeriodClip(w http.ResponseWriter, r *http.Request) {
	var req PeriodClipRequest
	if err := decodeJSONBody(r, &req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.PeriodID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "period_id is required")
		return
	}
	clips := sqlite.NewClipStore(ws.db)
	existing, err := clips.PeriodClips([]string{req.PeriodID})
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pc, ok := existing[req.PeriodID]; ok {
		ws.writePeriodClip(w, http.StatusOK, clips, pc.ClipID)
		return
	}
	pc, status, err := ws.loadPeriodCaptures(req.PeriodID)
	if err != nil {
		ws.writeJSONError(w, status, err.Error())
		return
	}
	sensorID := req.SensorID
	if sensorID == "" {
		sensorID = "hesai-pandar40p"
	}
	files := make([]sqlite.ClipCapture, 0, len(pc.files))
	for i, f := range pc.files {
		files = append(files, sqlite.ClipCapture{Ordinal: i, CaptureFileID: f.CaptureFileID, PCAPFile: f.RelPath})
	}
	start, duration := pc.startSecs, float64(pc.period.DurationNs)/1e9
	clip := &sqlite.Clip{
		SensorID: sensorID, Files: files, PCAPStartSecs: &start, PCAPDurationSecs: &duration,
		Description: strings.TrimSpace(captureFolder(pc.files[0].RelPath) + " " + pc.period.Label),
	}
	if err := clips.InsertClip(clip); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := clips.SetClipSession(clip.ClipID, pc.period.SessionID, pc.period.PeriodID); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ws.writePeriodClip(w, http.StatusCreated, clips, clip.ClipID)
}

func (ws *Server) writePeriodClip(w http.ResponseWriter, status int, clips *sqlite.ClipStore, clipID string) {
	clip, err := clips.GetClip(clipID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if loc, err := clips.ClipLocationOf(clipID); err == nil {
		clip.Location = loc
	}
	ws.writeJSON(w, status, clip)
}

// captureFolder is the directory a capture sits in, the name an operator knows
// a visit by ("kirk/kirk2.pcapng" is kirk).
func captureFolder(relPath string) string {
	if i := strings.LastIndex(relPath, "/"); i > 0 {
		return relPath[:i]
	}
	return ""
}
