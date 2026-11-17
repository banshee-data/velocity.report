package followingreview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

// Cache holds at most one preparation's compact geometry, never canonical rows.
// Initial verification is serialized; immutable file replacement invalidates it.
type Cache struct {
	mu            sync.Mutex
	key           string
	session       *Session
	canonicalInfo os.FileInfo
}

type Session struct {
	Metadata Metadata
	receipt  receipt
	root     string
	version  l8behaviour.InteractionVersion
	paths    []GeometryPath
}

const maxSliceBytes = 64 << 20

// Digest identifies exact immutable slice bytes in HTTP responses.
func Digest(b []byte) string { return digest(b) }

func digest(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("review file exceeds size limit")
	}
	return b, nil
}

func decode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

// contained resolves operator-managed symlinks and refuses artefacts escaping
// the completed preparation. No path from an HTTP request reaches this boundary.
func contained(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("preparation artefact path must be absolute")
	}
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("review artefact escapes registered preparation")
	}
	return p, nil
}

// Open binds a trusted registered run to its nearest completed preparation.
func (cache *Cache) Open(run *sqlite.Run) (*Session, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if run.Status != "completed" || run.VRLogPath == "" {
		return nil, fmt.Errorf("run has no completed review recording")
	}
	if !filepath.IsAbs(run.VRLogPath) {
		return nil, fmt.Errorf("registered recording path must be absolute")
	}
	vrlog, err := filepath.EvalSymlinks(run.VRLogPath)
	if err != nil {
		return nil, err
	}
	root := filepath.Dir(vrlog)
	// Preparation is an immediate parent of its vrlog. Do not discover an
	// unrelated receipt farther up a shared volume or from a sibling run.
	receiptPath, err := contained(root, filepath.Join(root, "complete.json"))
	if err != nil {
		return nil, err
	}
	b, err := readBounded(receiptPath, 1<<20)
	if err != nil {
		return nil, err
	}
	var receipt receipt
	if err = decode(b, &receipt); err != nil {
		return nil, err
	}
	if receipt.Schema != "following_review_preparation_v1" || receipt.Purpose != "annotation_review" || receipt.Rows <= 0 || len(receipt.Slices) == 0 {
		return nil, fmt.Errorf("preparation is incomplete")
	}
	registered, err := contained(root, receipt.VRLOG)
	if err != nil {
		return nil, err
	}
	if registered != vrlog {
		return nil, fmt.Errorf("receipt belongs to another recording")
	}
	canonicalPath, err := contained(root, receipt.Evidence)
	if err != nil {
		return nil, err
	}
	headerPath, err := contained(root, filepath.Join(vrlog, "header.json"))
	if err != nil {
		return nil, err
	}
	hb, err := readBounded(headerPath, 1<<20)
	if err != nil {
		return nil, err
	}
	var header recorder.LogHeader
	if err = decode(hb, &header); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(header, receipt.Header) || header.EndNs == math.MaxInt64 || header.StartNs >= header.EndNs || header.TotalFrames <= 0 || header.BuildGitSHA == "" || header.SensorID != run.SensorID {
		return nil, fmt.Errorf("recording header differs from completed receipt/run")
	}
	if run.BuildGitSHA != "" && run.BuildGitSHA != header.BuildGitSHA {
		return nil, fmt.Errorf("registered producer version differs")
	}
	if run.SourceType != "" && header.SourceType != "" && !strings.EqualFold(run.SourceType, header.SourceType) {
		return nil, fmt.Errorf("registered source kind differs from recording producer")
	}
	if run.FrameStartNs != nil && *run.FrameStartNs != header.StartNs || run.FrameEndNs != nil && *run.FrameEndNs != header.EndNs {
		return nil, fmt.Errorf("registered recording window differs")
	}
	if err = receipt.Context.Validate(); err != nil {
		return nil, err
	}
	if receipt.Context.SourceManifest == nil || receipt.Context.SourceManifest.Window == nil {
		return nil, fmt.Errorf("review source must bind replay window")
	}
	if run.ClipID != "" && run.ClipID != receipt.Context.SourceManifest.ClipID {
		return nil, fmt.Errorf("registered clip differs from pinned source")
	}
	id, err := l4bobserve.SourceID(*receipt.Context.SourceManifest)
	if err != nil || id != receipt.Context.SourceID {
		return nil, fmt.Errorf("receipt source identity differs")
	}
	if run.SourcePath != "" && !matchesCapture(run.SourcePath, receipt.Context.SourceManifest.CapturePaths) {
		return nil, fmt.Errorf("registered capture differs from pinned source")
	}
	calibration, err := l4bobserve.CalibrationID(receipt.Calibration)
	if err != nil || receipt.Calibration.SensorID != header.SensorID {
		return nil, fmt.Errorf("invalid recording calibration")
	}
	f, err := os.Open(canonicalPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	key := run.RunID + digest(b) + digest(hb) + canonicalPath + strconv.FormatInt(stat.Size(), 10) + stat.ModTime().String()
	if key == cache.key && cache.session != nil && os.SameFile(stat, cache.canonicalInfo) {
		return cache.session, nil
	}
	c, err := readCanonical(f)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(c.Context, receipt.Context) || c.Version.ParamHash != header.ParamsHash || c.Version.EstimateStage != l8behaviour.StageOnline {
		return nil, fmt.Errorf("canonical source/version differs from receipt")
	}
	s := &Session{receipt: receipt, root: root, version: c.Version, paths: c.Paths}
	s.Metadata = Metadata{Schema: "following_review_session_v1", RunID: run.RunID, EvidenceSHA256: c.SHA, HeaderSHA256: digest(hb), SourceID: id, CalibrationID: calibration, Start: header.StartNs, End: header.EndNs + 1, Binding: binding(c.Context, c.Version, c.SHA)}
	last := header.StartNs
	rows := 0
	for i, file := range receipt.Slices {
		if file.Start < last || file.Start < header.StartNs || file.End <= file.Start || file.End > header.EndNs+1 || file.Rows <= 0 {
			return nil, fmt.Errorf("invalid or overlapping slice window")
		}
		if _, err = contained(root, file.Path); err != nil {
			return nil, err
		}
		last = file.End
		rows += file.Rows
		s.Metadata.Slices = append(s.Metadata.Slices, Slice{strconv.Itoa(i), file.Start, file.End, file.Rows})
	}
	if rows != receipt.Rows {
		return nil, fmt.Errorf("slice row totals differ from receipt")
	}
	// First slice independently binds the receipt to the canonical digest,
	// header and calibration. Every subsequently requested slice is rechecked.
	if _, _, err = s.Evidence(receipt.Slices[0].Start); err != nil {
		return nil, err
	}
	cache.key, cache.session, cache.canonicalInfo = key, s, stat
	return s, nil
}

func matchesCapture(path string, declared []string) bool {
	for _, p := range declared {
		if filepath.Clean(path) == filepath.Clean(p) {
			return true
		}
		a, e1 := filepath.EvalSymlinks(path)
		b, e2 := filepath.EvalSymlinks(p)
		if e1 == nil && e2 == nil && a == b {
			return true
		}
	}
	return false
}

func (s *Session) slice(at int64) (sliceFile, error) {
	for _, f := range s.receipt.Slices {
		if at >= f.Start && at < f.End {
			return f, nil
		}
	}
	return sliceFile{}, fmt.Errorf("no evidence slice at requested capture time")
}

// Evidence returns exact original bytes, never an enriched or re-encoded slice.
func (s *Session) Evidence(at int64) ([]byte, []fieldrun.EvidenceRow, error) {
	f, err := s.slice(at)
	if err != nil {
		return nil, nil, err
	}
	path, err := contained(s.root, f.Path)
	if err != nil {
		return nil, nil, err
	}
	b, err := readBounded(path, maxSliceBytes)
	if err != nil {
		return nil, nil, err
	}
	if digest(b) != f.SHA256 {
		return nil, nil, fmt.Errorf("review slice digest differs")
	}
	var slice reviewSlice
	if err = decode(b, &slice); err != nil {
		return nil, nil, err
	}
	if slice.Schema != "following_review_slice_v1" || !reflect.DeepEqual(slice.Context, s.receipt.Context) || slice.Version != s.version || slice.Review.CanonicalSHA != s.Metadata.EvidenceSHA256 || slice.Review.HeaderSHA != s.Metadata.HeaderSHA256 || slice.Review.CalibrationID != s.Metadata.CalibrationID || slice.Review.Build != s.receipt.Header.BuildGitSHA || slice.Review.Start != f.Start || slice.Review.End != f.End || len(slice.Rows) != f.Rows {
		return nil, nil, fmt.Errorf("review slice binding differs")
	}
	seen := map[string]map[int64]bool{}
	for _, row := range slice.Rows {
		if row.FollowerTrackID == "" || row.CaptureUnixNanos < f.Start || row.CaptureUnixNanos >= f.End || row.IntervalNanos < 0 || row.CaptureUnixNanos > math.MaxInt64-row.IntervalNanos || row.CaptureUnixNanos+row.IntervalNanos > s.Metadata.End || row.Decision.FollowerTrackID != row.FollowerTrackID || row.Decision.CaptureUnixNanos != row.CaptureUnixNanos {
			return nil, nil, fmt.Errorf("invalid review sample identity or interval")
		}
		if seen[row.FollowerTrackID] == nil {
			seen[row.FollowerTrackID] = map[int64]bool{}
		}
		if seen[row.FollowerTrackID][row.CaptureUnixNanos] {
			return nil, nil, fmt.Errorf("duplicate review sample")
		}
		seen[row.FollowerTrackID][row.CaptureUnixNanos] = true
		candidates := map[string]bool{}
		for _, candidate := range row.Decision.Candidates {
			if candidate.TrackID == "" || candidate.TrackID == row.FollowerTrackID || candidates[candidate.TrackID] {
				return nil, nil, fmt.Errorf("invalid or conflicting candidate identity")
			}
			candidates[candidate.TrackID] = true
		}
		if row.Decision.LeaderTrackID != "" && !candidates[row.Decision.LeaderTrackID] {
			return nil, nil, fmt.Errorf("selected leader is absent from candidate evidence")
		}
		if row.Point != nil && (row.Point.FollowerTrackID != row.FollowerTrackID || row.Point.CaptureUnixNanos != row.CaptureUnixNanos || row.Point.LeaderTrackID != row.Decision.LeaderTrackID) {
			return nil, nil, fmt.Errorf("point identity differs from sample")
		}
		if row.DRAC.Calculation != nil && row.DRAC.Calculation.ValueMPS2 != nil {
			value := *row.DRAC.Calculation.ValueMPS2
			if row.DRAC.Status != "supported" || row.DRAC.Calculation.Status != l8behaviour.DRACSupported || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, nil, fmt.Errorf("suppressed or invalid DRAC value")
			}
		}
	}
	return b, slice.Rows, nil
}

type Geometry struct {
	Schema               string         `json:"schema"`
	SourceID             string         `json:"source_id"`
	CalibrationID        string         `json:"calibration_id"`
	CanonicalSHA         string         `json:"canonical_evidence_sha256"`
	Paths                []GeometryPath `json:"paths"`
	CoordinateFrameID    string         `json:"coordinate_frame_id"`
	RecordingCoordinates bool           `json:"recording_coordinates"`
}

func (s *Session) Geometry(at int64, follower string) (Geometry, error) {
	_, rows, err := s.Evidence(at)
	if err != nil {
		return Geometry{}, err
	}
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r.FollowerTrackID] = true
	}
	if follower != "" && !ids[follower] {
		return Geometry{}, fmt.Errorf("follower is absent from verified minute")
	}
	cal := s.receipt.Calibration
	identity := [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	g := Geometry{Schema: "following_review_geometry_v1", SourceID: s.Metadata.SourceID, CalibrationID: s.Metadata.CalibrationID, CanonicalSHA: s.Metadata.EvidenceSHA256, Paths: []GeometryPath{}, CoordinateFrameID: cal.ToFrame, RecordingCoordinates: cal.FromFrame == "sensor" && cal.ToFrame == "sensor" && cal.Transform == identity}
	for _, p := range s.paths {
		if ids[p.FollowerTrackID] && (follower == "" || follower == p.FollowerTrackID) {
			g.Paths = append(g.Paths, p)
		}
	}
	return g, nil
}
