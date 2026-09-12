// Package annotation provides the immutable source domain that reviewed point
// masks are written against.
//
// The problem it exists to solve is that a tracker's own output cannot be the
// reference for judging that tracker. Track IDs are predictions: they split,
// merge and disappear between runs, so any metric keyed on them changes when
// the population changes rather than when the estimator does. Measuring a
// heading or identity change against them compares a moving target with
// itself.
//
// A pack breaks that circularity. It is a frozen excerpt of a recording,
// content-addressed, with a point domain that cannot change underneath an
// annotation. Reviewed masks reference points inside it by position, never by
// coordinate matching, and object identities live in the pack's namespace
// rather than the tracker's. A predicted split does not split the reference.
package annotation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
)

// PackSchemaVersion is the pack layout version. A pack whose points, ordering,
// or encoding change is a new pack, not a revision of this one.
const PackSchemaVersion = 1

// MaxPointsPerSample bounds a single frame's point count on read, so a
// corrupt or hostile manifest cannot ask for an unbounded allocation. A Hesai
// Pandar40P frame carries well under 100k returns.
const MaxPointsPerSample = 1 << 21

// File names inside a pack directory.
const (
	manifestFile = "manifest.json"
	samplesFile  = "samples.json"
	pointsFile   = "points.bin"
)

// CaptureCoverage records what the source recording could see, so a limitation
// survives every export. A mask over a foreground-only recording is a mask
// over recorded foreground, not scene segmentation, and nothing downstream may
// silently promote it.
type CaptureCoverage string

const (
	CoverageFull           CaptureCoverage = "full"
	CoverageForegroundOnly CaptureCoverage = "foreground_only"
	CoverageDecimated      CaptureCoverage = "decimated"
)

// CoordinateContract states what the stored numbers mean. Without it a mask is
// a list of integers with no physical interpretation.
type CoordinateContract struct {
	Units          string `json:"units"`
	FrameID        string `json:"frame_id"`
	ReferenceFrame string `json:"reference_frame"`
	Handedness     string `json:"handedness"`
	OriginNote     string `json:"origin_note"`
	// TransformVersion identifies the sensor-to-site transform in force. An
	// empty value means points are in the sensor frame as recorded.
	TransformVersion string `json:"transform_version"`
}

// SourceProvenance identifies what the pack was cut from. Digests are over
// stored bytes: a source whose content changes cannot silently keep its
// annotations.
type SourceProvenance struct {
	VRLOGPath      string `json:"vrlog_path"`
	VRLOGHeaderSHA string `json:"vrlog_header_sha256"`
	VRLOGFramesSHA string `json:"vrlog_frames_sha256"`
	PCAPBasename   string `json:"pcap_basename,omitempty"`
	SensorID       string `json:"sensor_id"`
	RunConfigID    string `json:"run_config_id,omitempty"`
	ConfigHash     string `json:"config_hash,omitempty"`
	ParamsHash     string `json:"params_hash,omitempty"`
	BuildVersion   string `json:"build_version,omitempty"`
	BuildGitSHA    string `json:"build_git_sha,omitempty"`
}

// Completeness records what the export asked for against what it got, so a
// gap in the source is visible rather than inferred from a short file.
type Completeness struct {
	RequestedStartNs int64 `json:"requested_start_ns"`
	RequestedEndNs   int64 `json:"requested_end_ns"`
	ActualStartNs    int64 `json:"actual_start_ns"`
	ActualEndNs      int64 `json:"actual_end_ns"`
	// FramesWithoutPoints counts source frames skipped because they carried no
	// point cloud. A recording made without points yields an empty pack, and
	// that is an error at export rather than a silent success.
	FramesWithoutPoints int   `json:"frames_without_points"`
	DuplicateTimestamps int   `json:"duplicate_timestamps"`
	MaxTimestampGapNs   int64 `json:"max_timestamp_gap_ns"`
}

// Manifest is the pack's identity and contract.
type Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	DatasetID     string `json:"dataset_id"`
	CreatedNs     int64  `json:"created_ns"`

	Source     SourceProvenance   `json:"source"`
	Coordinate CoordinateContract `json:"coordinate"`
	Coverage   CaptureCoverage    `json:"coverage"`
	// CoverageNote carries the recording and export filters separately, since
	// a decimated export of a foreground-only recording is doubly limited.
	CoverageNote string `json:"coverage_note,omitempty"`

	SampleCount int   `json:"sample_count"`
	PointCount  int64 `json:"point_count"`

	// PointsSHA256 and SamplesSHA256 are digests over the stored bytes. The
	// pack digest below binds them together and is what an annotation cites.
	PointsSHA256  string `json:"points_sha256"`
	SamplesSHA256 string `json:"samples_sha256"`
	PackDigest    string `json:"pack_digest"`

	HasIntensity      bool `json:"has_intensity"`
	HasClassification bool `json:"has_classification"`

	Completeness Completeness `json:"completeness"`
}

// Sample is one frame's point domain within the pack.
//
// SampleID is dense and pack-local. It is deliberately not the tracker's frame
// number: record order is not assumed to be capture order, and two source
// frames may legitimately share a timestamp.
type Sample struct {
	SampleID int `json:"sample_id"`
	// SourceOrdinal is the frame's position in the source recording, retained
	// so a reader can go back to the original without guessing.
	SourceOrdinal int    `json:"source_ordinal"`
	SourceFrameID uint64 `json:"source_frame_id"`
	TimestampNs   int64  `json:"timestamp_ns"`
	SensorID      string `json:"sensor_id"`
	PointCount    int    `json:"point_count"`
	// ByteOffset locates this sample's block in points.bin.
	ByteOffset int64 `json:"byte_offset"`
}

// Points is one sample's decoded point arrays.
type Points struct {
	X              []float32
	Y              []float32
	Z              []float32
	Intensity      []uint8
	Classification []uint8
}

// sampleBytes is the encoded size of one sample's block.
func sampleBytes(n int) int64 { return int64(n)*(4*3) + int64(n)*2 }

// Pack is an opened annotation pack.
type Pack struct {
	Dir      string
	Manifest Manifest
	Samples  []Sample

	raw []byte // points.bin contents, verified against the manifest digest
}

// sha256Hex is the digest form used throughout: "sha256:" plus lower hex, so a
// bare hash can never be mistaken for a different algorithm's.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func fileSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sha256Hex(b), nil
}

// encodePoints writes one sample's arrays in canonical little-endian order:
// all X, then all Y, then all Z as float32, then intensity and classification
// as uint8. Absent attribute arrays are written as zeros so every block has a
// fixed size derived from the point count alone.
func encodePoints(p Points) ([]byte, error) {
	n := len(p.X)
	if len(p.Y) != n || len(p.Z) != n {
		return nil, fmt.Errorf("coordinate arrays disagree: x=%d y=%d z=%d", n, len(p.Y), len(p.Z))
	}
	if n > MaxPointsPerSample {
		return nil, fmt.Errorf("sample carries %d points, above the %d limit", n, MaxPointsPerSample)
	}
	for i := 0; i < n; i++ {
		for _, v := range []float32{p.X[i], p.Y[i], p.Z[i]} {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return nil, fmt.Errorf("point %d has a non-finite coordinate", i)
			}
		}
	}

	buf := make([]byte, sampleBytes(n))
	off := 0
	for _, arr := range [][]float32{p.X, p.Y, p.Z} {
		for _, v := range arr {
			binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(v))
			off += 4
		}
	}
	for i := 0; i < n; i++ {
		if i < len(p.Intensity) {
			buf[off] = p.Intensity[i]
		}
		off++
	}
	for i := 0; i < n; i++ {
		if i < len(p.Classification) {
			buf[off] = p.Classification[i]
		}
		off++
	}
	return buf, nil
}

func decodePoints(b []byte, n int) (Points, error) {
	if int64(len(b)) != sampleBytes(n) {
		return Points{}, fmt.Errorf("block is %d bytes, want %d for %d points", len(b), sampleBytes(n), n)
	}
	p := Points{
		X: make([]float32, n), Y: make([]float32, n), Z: make([]float32, n),
		Intensity: make([]uint8, n), Classification: make([]uint8, n),
	}
	off := 0
	for _, arr := range [][]float32{p.X, p.Y, p.Z} {
		for i := range arr {
			arr[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[off:]))
			off += 4
		}
	}
	copy(p.Intensity, b[off:off+n])
	off += n
	copy(p.Classification, b[off:off+n])
	return p, nil
}

// WritePack writes a pack directory atomically: content is staged beside the
// destination and moved into place, so an interrupted export cannot leave a
// half-written domain that annotations would then cite.
func WritePack(dir string, m Manifest, samples []Sample, blocks [][]byte) error {
	if len(samples) != len(blocks) {
		return fmt.Errorf("%d samples but %d point blocks", len(samples), len(blocks))
	}
	if len(samples) == 0 {
		return fmt.Errorf("refusing to write an empty pack: nothing could be annotated in it")
	}

	var points []byte
	for i := range samples {
		samples[i].SampleID = i
		samples[i].ByteOffset = int64(len(points))
		if got, want := int64(len(blocks[i])), sampleBytes(samples[i].PointCount); got != want {
			return fmt.Errorf("sample %d: block is %d bytes, want %d", i, got, want)
		}
		points = append(points, blocks[i]...)
	}

	samplesJSON, err := json.MarshalIndent(samples, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal samples: %w", err)
	}

	m.SchemaVersion = PackSchemaVersion
	m.SampleCount = len(samples)
	m.PointCount = 0
	for _, s := range samples {
		m.PointCount += int64(s.PointCount)
	}
	m.PointsSHA256 = sha256Hex(points)
	m.SamplesSHA256 = sha256Hex(samplesJSON)
	m.PackDigest = sha256Hex([]byte(m.PointsSHA256 + "\n" + m.SamplesSHA256))
	if m.DatasetID == "" {
		m.DatasetID = "ds_" + m.PackDigest[7:23]
	}

	manifestJSON, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	staging := dir + ".partial"
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("clear staging directory: %w", err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	for name, content := range map[string][]byte{
		pointsFile:   points,
		samplesFile:  append(samplesJSON, '\n'),
		manifestFile: append(manifestJSON, '\n'),
	} {
		if err := os.WriteFile(filepath.Join(staging, name), content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("refusing to overwrite an existing pack at %s: a changed point domain is a new pack", dir)
	}
	return os.Rename(staging, dir)
}

// OpenPack reads and verifies a pack. It fails closed: a digest that does not
// match its content means annotations written against this pack can no longer
// be trusted to reference the same points, and guessing is worse than
// refusing.
func OpenPack(dir string) (*Pack, error) {
	manifestBytes, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.SchemaVersion != PackSchemaVersion {
		return nil, fmt.Errorf("pack schema version %d, this build reads %d", m.SchemaVersion, PackSchemaVersion)
	}

	samplesBytes, err := os.ReadFile(filepath.Join(dir, samplesFile))
	if err != nil {
		return nil, fmt.Errorf("read samples: %w", err)
	}
	// Compare against the trimmed bytes the writer digested, not the file,
	// which carries a trailing newline.
	if got := sha256Hex(trimTrailingNewline(samplesBytes)); got != m.SamplesSHA256 {
		return nil, fmt.Errorf("samples digest mismatch: file is %s, manifest says %s", got, m.SamplesSHA256)
	}
	var samples []Sample
	if err := json.Unmarshal(samplesBytes, &samples); err != nil {
		return nil, fmt.Errorf("parse samples: %w", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, pointsFile))
	if err != nil {
		return nil, fmt.Errorf("read points: %w", err)
	}
	if got := sha256Hex(raw); got != m.PointsSHA256 {
		return nil, fmt.Errorf("points digest mismatch: file is %s, manifest says %s", got, m.PointsSHA256)
	}
	if got := sha256Hex([]byte(m.PointsSHA256 + "\n" + m.SamplesSHA256)); got != m.PackDigest {
		return nil, fmt.Errorf("pack digest mismatch: computed %s, manifest says %s", got, m.PackDigest)
	}

	if len(samples) != m.SampleCount {
		return nil, fmt.Errorf("manifest declares %d samples, file holds %d", m.SampleCount, len(samples))
	}
	for i, s := range samples {
		if s.SampleID != i {
			return nil, fmt.Errorf("sample %d carries id %d: pack-local ids must be dense and ordered", i, s.SampleID)
		}
		if s.PointCount < 0 || s.PointCount > MaxPointsPerSample {
			return nil, fmt.Errorf("sample %d declares %d points", i, s.PointCount)
		}
		end := s.ByteOffset + sampleBytes(s.PointCount)
		if s.ByteOffset < 0 || end > int64(len(raw)) {
			return nil, fmt.Errorf("sample %d spans bytes [%d,%d) of a %d-byte file", i, s.ByteOffset, end, len(raw))
		}
	}

	return &Pack{Dir: dir, Manifest: m, Samples: samples, raw: raw}, nil
}

func trimTrailingNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}

// PointsAt decodes one sample's arrays.
func (p *Pack) PointsAt(sampleID int) (Points, error) {
	if sampleID < 0 || sampleID >= len(p.Samples) {
		return Points{}, fmt.Errorf("sample %d is outside the pack's %d samples", sampleID, len(p.Samples))
	}
	s := p.Samples[sampleID]
	return decodePoints(p.raw[s.ByteOffset:s.ByteOffset+sampleBytes(s.PointCount)], s.PointCount)
}

// CanonicalIndices returns the one encoding a membership set is allowed to
// have: sorted ascending with duplicates removed. Two masks over the same
// points must compare equal byte for byte, or a round-trip cannot be checked.
func CanonicalIndices(idx []int) []int {
	if len(idx) == 0 {
		return []int{}
	}
	out := append([]int(nil), idx...)
	sort.Ints(out)
	n := 0
	for i, v := range out {
		if i == 0 || v != out[i-1] {
			out[n] = v
			n++
		}
	}
	return out[:n]
}

// ValidateIndices checks a membership set against a sample's point domain.
// Negative, duplicate, out-of-range and unsorted references are all rejected:
// each one means the mask was written against a different domain than the one
// being read.
func (p *Pack) ValidateIndices(sampleID int, idx []int) error {
	if sampleID < 0 || sampleID >= len(p.Samples) {
		return fmt.Errorf("sample %d is outside the pack's %d samples", sampleID, len(p.Samples))
	}
	n := p.Samples[sampleID].PointCount
	for i, v := range idx {
		if v < 0 || v >= n {
			return fmt.Errorf("point index %d is outside sample %d's %d points", v, sampleID, n)
		}
		if i > 0 && v <= idx[i-1] {
			return fmt.Errorf("point indices must be sorted and unique: %d follows %d", v, idx[i-1])
		}
	}
	return nil
}

// dirSHA256 digests a directory's files by name and content, so a frames
// directory that gains, loses or alters a chunk gets a different digest.
func dirSHA256(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return "", err
		}
		h.Write([]byte(n))
		h.Write(b)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
