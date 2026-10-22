package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capexport"
	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/dataset"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/version"
)

// Clip extracts and publishing.
//
// Routes:
// - GET  /api/lidar/clips/{clip_id}/extract    — the clip's extracts, newest first
// - POST /api/lidar/clips/{clip_id}/extract    — queue a capture_export job
// - PUT  /api/lidar/clips/{clip_id}/publishing — dataset id, north azimuth, storage
// - POST /api/lidar/clips/{clip_id}/stage      — write the extract into the dataset clone
// - GET  /api/lidar/dataset/site-index         — site-index entries for the clips marked for the dataset
//
// An extract is a standalone capture cut from the clip's window at frame
// boundaries (internal/lidar/capexport). It is written under the replay
// directory, in clipExtractDir, so the clip can replay it and the capture
// index finds it.

// clipExtractDir is where extracts are written, relative to the replay
// directory.
const clipExtractDir = "clips"

// capexportCut is the cutter. A variable so the job plumbing can be tested
// against a stand-in.
var capexportCut = capexport.Cut

func (ws *Server) handleClipExtract(w http.ResponseWriter, r *http.Request, clipID string) {
	store := sqlite.NewClipStore(ws.db)
	switch r.Method {
	case http.MethodGet:
		extracts, err := store.ClipExtracts(clipID)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ws.writeJSON(w, http.StatusOK, map[string]any{"clip_id": clipID, "extracts": extracts})
	case http.MethodPost:
		if ws.pcapSafeDir == "" {
			ws.writeJSONError(w, http.StatusServiceUnavailable, "no replay directory configured to write extracts into")
			return
		}
		clip, err := store.GetClip(clipID)
		if err != nil {
			ws.writeClipLoadError(w, err)
			return
		}
		if len(clip.Files) == 0 {
			ws.writeJSONError(w, http.StatusBadRequest, "the clip names no captures")
			return
		}
		job, created, err := store.EnqueueClipExtract(clipID, capexport.RubricVersion,
			fmt.Sprintf("queued: cut %d capture(s)", len(clip.Files)))
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		status := http.StatusAccepted
		if !created {
			status = http.StatusOK
		}
		ws.writeJSON(w, status, map[string]any{"clip_id": clipID, "job": job, "created": created})
	default:
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (ws *Server) writeClipLoadError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "not found") {
		ws.writeJSONError(w, http.StatusNotFound, "clip not found")
		return
	}
	ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
}

// runClipExtract cuts one clip's window to a standalone capture.
func (ws *Server) runClipExtract(ctx context.Context, job capjobs.Job, report func(capjobs.Progress)) error {
	store := sqlite.NewClipStore(ws.db)
	extract, err := store.ClipExtractForJob(job.JobID)
	if err != nil {
		return fmt.Errorf("loading the extract record: %w", err)
	}
	clip, err := store.GetClip(extract.ClipID)
	if err != nil {
		return err
	}
	loc, err := store.ClipLocationOf(clip.ClipID)
	if err != nil {
		return err
	}

	sources := make([]string, 0, len(clip.Files))
	for _, f := range clip.Files {
		resolved, err := ws.resolvePCAPPath(f.PCAPFile)
		if err != nil {
			return fmt.Errorf("resolving %s: %w", f.PCAPFile, err)
		}
		sources = append(sources, resolved)
	}
	origin, err := capexport.FirstPacketTime(sources[0])
	if err != nil {
		return err
	}
	start := origin
	if clip.PCAPStartSecs != nil {
		start = origin.Add(secondsDuration(*clip.PCAPStartSecs))
	}
	var end time.Time
	if clip.PCAPDurationSecs != nil && *clip.PCAPDurationSecs > 0 {
		end = start.Add(secondsDuration(*clip.PCAPDurationSecs))
	}

	safeDir, err := absPath(ws.pcapSafeDir)
	if err != nil {
		return err
	}
	outDir := filepath.Join(safeDir, clipExtractDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", outDir, err)
	}
	// Cut to a name of the job's own, then rename once the first packet, and
	// so the stem's minute, is known.
	working := filepath.Join(outDir, "."+job.JobID+".pcapng")

	report(capjobs.Progress{Current: 0, Total: 0, Detail: fmt.Sprintf("cutting %d capture(s)", len(sources))})
	res, err := capexportCut(ctx, capexport.Request{
		Sources: sources, Start: start, End: end,
		UDPPort: ws.clipUDPPort(clip),
		Output:  working,
		Comment: "clip " + clip.ClipID,
	}, func(read, total int64) {
		report(capjobs.Progress{Current: read, Total: total,
			Detail: fmt.Sprintf("read %.1f of %.1f GB", float64(read)/1e9, float64(total)/1e9)})
	})
	if err != nil {
		return err
	}

	first := time.Unix(0, res.FirstPacketNs)
	stem, err := extractStem(clip, loc, first)
	if err != nil {
		_ = os.Remove(working)
		return err
	}
	final, err := freeName(outDir, stem, job.JobID)
	if err != nil {
		_ = os.Remove(working)
		return err
	}
	if err := os.Rename(working, filepath.Join(outDir, final+".pcapng")); err != nil {
		_ = os.Remove(working)
		return err
	}

	sidecar, err := ws.extractSidecar(clip, loc, res, final+".pcapng", start, end)
	if err != nil {
		return err
	}
	if err := dataset.WriteJSONFile(filepath.Join(outDir, final+".json"), sidecar); err != nil {
		return fmt.Errorf("writing the sidecar: %w", err)
	}
	sidecarJSON, err := json.Marshal(sidecar)
	if err != nil {
		return err
	}
	extract.RelPath = path.Join(clipExtractDir, final+".pcapng")
	extract.SHA256 = res.SHA256
	extract.SizeBytes = res.Bytes
	extract.PacketCount = res.PacketCount
	extract.LidarPacketCount = res.LidarPacketCount
	extract.FrameCount = res.FrameCount
	extract.FirstPacketNs = res.FirstPacketNs
	extract.LastPacketNs = res.LastPacketNs
	extract.ClosedBy = res.ClosedBy
	extract.RubricVersion = res.RubricVersion
	extract.Sidecar = sidecarJSON
	if err := store.CompleteClipExtract(extract); err != nil {
		return err
	}
	report(capjobs.Progress{Current: res.Bytes, Total: res.Bytes,
		Detail: fmt.Sprintf("%d rotations, %.2f GB", res.FrameCount, float64(res.Bytes)/1e9)})
	return nil
}

func secondsDuration(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// clipUDPPort is the port the clip's captures were recorded on, as the index
// saw it, else the configured replay port.
func (ws *Server) clipUDPPort(clip *sqlite.Clip) int {
	if clip.SessionID != "" {
		if store, err := ws.captureStore(); err == nil {
			if files, err := store.SessionFiles(clip.SessionID); err == nil {
				return sessionUDPPort(files, ws.udpPort)
			}
		}
	}
	if ws.udpPort > 0 {
		return ws.udpPort
	}
	return 2369
}

// extractStem names an extract. A located clip with a dataset id takes the
// dataset's stem, so staging can link it unrenamed; anything else is named
// for its clip.
func extractStem(clip *sqlite.Clip, loc *sqlite.ClipLocation, first time.Time) (string, error) {
	if clip.DatasetID != "" && loc != nil {
		return dataset.Stem(first, loc.L13Token, clip.DatasetID)
	}
	short := clip.ClipID
	if len(short) > 8 {
		short = short[:8]
	}
	stamp, err := dataset.MinuteStamp(first)
	if err != nil {
		return "", err
	}
	return stamp + "_clip-" + short, nil
}

// freeName returns stem, or stem with the job's suffix when a file of that
// name is already there: an extract never replaces another.
func freeName(dir, stem, jobID string) (string, error) {
	for _, candidate := range []string{stem, stem + "-" + strings.TrimPrefix(jobID, "job-")[:8]} {
		_, errPCAP := os.Lstat(filepath.Join(dir, candidate+".pcapng"))
		_, errJSON := os.Lstat(filepath.Join(dir, candidate+".json"))
		if errors.Is(errPCAP, os.ErrNotExist) && errors.Is(errJSON, os.ErrNotExist) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("an extract named %s already exists twice over", stem)
}

func (ws *Server) clipWhere(clip *sqlite.Clip, loc *sqlite.ClipLocation) string {
	if loc != nil && loc.SiteID != "" {
		if site, err := sqlite.NewSiteStore(ws.db).ByToken(loc.SiteID); err == nil && site.Label != "" {
			return site.Label
		}
	}
	return clip.Description
}

func (ws *Server) extractSidecar(clip *sqlite.Clip, loc *sqlite.ClipLocation, res capexport.Result,
	outputFile string, start, end time.Time) (dataset.Sidecar, error) {
	first, err := dataset.ISOTime(time.Unix(0, res.FirstPacketNs))
	if err != nil {
		return dataset.Sidecar{}, err
	}
	last, err := dataset.ISOTime(time.Unix(0, res.LastPacketNs))
	if err != nil {
		return dataset.Sidecar{}, err
	}
	windowStart, _ := dataset.ISOTime(start)
	windowEnd := ""
	if !end.IsZero() {
		windowEnd, _ = dataset.ISOTime(end)
	}
	sidecar := dataset.Sidecar{
		SchemaVersion: 1, SiteID: clip.DatasetID, Where: ws.clipWhere(clip, loc),
		StaticExportPolicy: dataset.ExportPolicy, Start: first, End: last,
		OperatorOverrides: []any{}, SourceFiles: sourceFiles(res.Sources),
		OutputFile: outputFile, OutputSHA256: res.SHA256, OutputBytes: res.Bytes,
		PCAPSplitBuildVersion: version.Version, ClipID: clip.ClipID,
		RubricVersion: res.RubricVersion, FrameCount: res.FrameCount, PacketCount: res.PacketCount,
		ClosedBy: res.ClosedBy, WindowStart: windowStart, WindowEnd: windowEnd,
	}
	if loc != nil {
		lat, lon := loc.Lat, loc.Lon
		sidecar.Latitude, sidecar.Longitude = &lat, &lon
	}
	return sidecar, nil
}

func sourceFiles(digests []capexport.SourceDigest) []dataset.SourceFile {
	out := make([]dataset.SourceFile, 0, len(digests))
	for _, d := range digests {
		out = append(out, dataset.SourceFile{Name: filepath.Base(d.Path), SHA256: d.SHA256, Bytes: d.Bytes})
	}
	return out
}

func (ws *Server) handleClipPublishing(w http.ResponseWriter, r *http.Request, clipID string) {
	if r.Method != http.MethodPut {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req sqlite.ClipPublishing
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Storage == "" {
		req.Storage = sqlite.ClipStorageSources
	}
	store := sqlite.NewClipStore(ws.db)
	if err := store.SetClipPublishing(clipID, req); err != nil {
		switch {
		case errors.Is(err, sqlite.ErrNotFound):
			ws.writeJSONError(w, http.StatusNotFound, "clip not found")
		case errors.Is(err, sqlite.ErrDatasetIDTaken):
			ws.writeJSONError(w, http.StatusConflict, err.Error())
		default:
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	clip, err := store.GetClip(clipID)
	if err != nil {
		ws.writeClipLoadError(w, err)
		return
	}
	ws.writeJSON(w, http.StatusOK, clip)
}

// publishingSurvey assembles what the dataset needs from a clip: its location,
// its dataset id and its latest completed extract. It reports a reason a clip
// cannot be published yet rather than an error.
func (ws *Server) publishingSurvey(clip *sqlite.Clip) (dataset.Survey, sqlite.ClipExtract, string, error) {
	store := sqlite.NewClipStore(ws.db)
	if clip.DatasetID == "" {
		return dataset.Survey{}, sqlite.ClipExtract{}, "the clip has no dataset id", nil
	}
	loc, err := store.ClipLocationOf(clip.ClipID)
	if err != nil {
		return dataset.Survey{}, sqlite.ClipExtract{}, "", err
	}
	if loc == nil {
		return dataset.Survey{}, sqlite.ClipExtract{}, "the clip is not located", nil
	}
	extract, err := store.LatestClipExtract(clip.ClipID)
	if errors.Is(err, sqlite.ErrNotFound) {
		return dataset.Survey{}, sqlite.ClipExtract{}, "the clip has no completed extract", nil
	} else if err != nil {
		return dataset.Survey{}, sqlite.ClipExtract{}, "", err
	}
	var sidecar dataset.Sidecar
	if err := json.Unmarshal(extract.Sidecar, &sidecar); err != nil {
		return dataset.Survey{}, sqlite.ClipExtract{}, "", fmt.Errorf("the extract's sidecar: %w", err)
	}
	first, last := time.Unix(0, extract.FirstPacketNs), time.Unix(0, extract.LastPacketNs)
	stem, err := dataset.Stem(first, loc.L13Token, clip.DatasetID)
	if err != nil {
		return dataset.Survey{}, sqlite.ClipExtract{}, "", err
	}
	return dataset.Survey{
		ID: clip.DatasetID, ClipID: clip.ClipID, Where: ws.clipWhere(clip, loc),
		Lat: loc.Lat, Lon: loc.Lon, L10: loc.L10Token, L13: loc.L13Token,
		First: first, Last: last, NorthAzimuthDeg: clip.NorthAzimuthDeg,
		SourceFiles: sidecar.SourceFiles,
		RawPath:     path.Join("raw", "lidar", stem+".pcapng"),
		RawSHA256:   extract.SHA256, RawBytes: extract.SizeBytes,
		BuildVersion: sidecar.PCAPSplitBuildVersion, RubricVersion: extract.RubricVersion,
	}, extract, "", nil
}

// handleClipStage links the clip's extract into the dataset clone and records
// it in manifest.json. Uploading the dataset stays a person's step.
func (ws *Server) handleClipStage(w http.ResponseWriter, r *http.Request, clipID string) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if ws.datasetDir == "" {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no dataset directory configured (--lidar-dataset-dir)")
		return
	}
	store := sqlite.NewClipStore(ws.db)
	clip, err := store.GetClip(clipID)
	if err != nil {
		ws.writeClipLoadError(w, err)
		return
	}
	survey, extract, reason, err := ws.publishingSurvey(clip)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if reason != "" {
		ws.writeJSONError(w, http.StatusConflict, reason)
		return
	}
	entry, err := survey.Manifest()
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	source, err := ws.resolvePCAPPath(extract.RelPath)
	if err != nil {
		ws.writeJSONError(w, http.StatusConflict, "the extract is not on disk: "+err.Error())
		return
	}
	datasetDir, err := absPath(ws.datasetDir)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Staging adds to a dataset clone; it never starts one where none is.
	if info, err := os.Stat(datasetDir); err != nil || !info.IsDir() {
		ws.writeJSONError(w, http.StatusServiceUnavailable,
			"the dataset directory "+datasetDir+" is not there; clone the dataset first")
		return
	}
	target := filepath.Join(datasetDir, filepath.FromSlash(survey.RawPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := linkExtract(source, target, extract.SizeBytes); err != nil {
		ws.writeJSONError(w, http.StatusConflict, err.Error())
		return
	}
	if err := dataset.UpsertManifest(filepath.Join(datasetDir, "manifest.json"), entry); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, dataset.ErrSlugTaken) {
			status = http.StatusConflict
		}
		ws.writeJSONError(w, status, err.Error())
		return
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{
		"clip_id": clipID, "raw_path": survey.RawPath, "manifest_entry": entry,
	})
}

// linkExtract hard-links the extract into the dataset. A file already at the
// target is accepted only when it is the same file or the same size; the
// scene publisher checks digests, and a re-stage must not replace a
// published file silently. A cross-volume target is refused: copying
// gigabytes inside a request is the wrong place for it.
func linkExtract(source, target string, size int64) error {
	if info, err := os.Stat(target); err == nil {
		src, serr := os.Stat(source)
		if serr == nil && os.SameFile(info, src) {
			return nil
		}
		return fmt.Errorf("%s already exists and is not this extract", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Link(source, target); err != nil {
		return fmt.Errorf("linking the extract into the dataset (it must be on the same volume): %w", err)
	}
	if info, err := os.Stat(target); err != nil || info.Size() != size {
		return fmt.Errorf("the linked extract at %s is not %d bytes", target, size)
	}
	return nil
}

// handleDatasetSiteIndex returns the site-index entries for every clip marked
// for the dataset, and the reason any marked clip is left out.
// tools/s2-archive/merge-clip-sites.py merges them into site-index.json.
func (ws *Server) handleDatasetSiteIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no database configured")
		return
	}
	clips, err := sqlite.NewClipStore(ws.db).PublishingClips()
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	entries := []dataset.SiteIndexEntry{}
	skipped := map[string]string{}
	for _, clip := range clips {
		survey, _, reason, err := ws.publishingSurvey(clip)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if reason != "" {
			skipped[clip.DatasetID] = reason
			continue
		}
		entry, err := survey.SiteIndex()
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		entries = append(entries, entry)
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "skipped": skipped})
}
