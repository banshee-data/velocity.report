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
//
// Version 2 adds the returns: a packet table and, for every point, the return
// it was decoded from. That is what lets a label set made against one run's
// pack apply to another run's pack of the same capture. Version 1 packs stay
// readable; they carry no returns.
const PackSchemaVersion = 2

// PackSchemaVersionLegacy is the version 1 layout: points without returns.
const PackSchemaVersionLegacy = 1

// MaxPointsPerSample bounds a single frame's point count on read, so a
// corrupt or hostile manifest cannot ask for an unbounded allocation. A Hesai
// Pandar40P frame carries well under 100k returns.
const MaxPointsPerSample = 1 << 21

// File names inside a pack directory.
const (
	manifestFile = "manifest.json"
	samplesFile  = "samples.json"
	pointsFile   = "points.bin"
	// Optional: the settled background in force over the excerpt.
	backgroundsFile      = "backgrounds.json"
	backgroundPointsFile = "background.bin"
	// Version 2: the packet table and each point's return within it.
	packetsFile = "packets.bin"
	returnsFile = "returns.bin"
)

// ReturnLayout is how one sensor packet divides into returns. A return number
// within a pack is packet_index × Blocks × Channels + block × Channels +
// (channel − 1): block counted from 0, channel from 1, as the parser numbers
// them, and packet_index a position in the pack's packet table.
type ReturnLayout struct {
	Model    string `json:"model"`
	Blocks   int    `json:"blocks"`
	Channels int    `json:"channels"`
}

// PerPacket is the number of returns one packet can carry.
func (l ReturnLayout) PerPacket() uint64 { return uint64(l.Blocks) * uint64(l.Channels) }

func (l ReturnLayout) validate() error {
	if l.Model == "" || l.Blocks < 1 || l.Channels < 1 || l.Blocks > 1<<10 || l.Channels > 1<<10 {
		return fmt.Errorf("invalid return layout %+v", l)
	}
	return nil
}

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
	// HeightBand is the run's L4 vertical filter, when the exporter was told
	// it. The pack holds the points as recorded, which is before that filter
	// ran; with the band, a client can show exactly which of them the
	// clusterer never saw.
	HeightBand *HeightBand `json:"height_band,omitempty"`
}

// HeightBand is the L4 height-band filter a run was made with: the rule
// l4perception.HeightBandFilter applies, in the frame the points are in.
//
// A point is removed when RemoveGround is set and its Z is below FloorM or
// above CeilingM. Both comparisons are strict, as the filter's are.
type HeightBand struct {
	FloorM       float64 `json:"floor_m"`
	CeilingM     float64 `json:"ceiling_m"`
	RemoveGround bool    `json:"remove_ground"`
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

	// The settled background, when the recording carried it. It is context
	// for an operator, not part of the point domain: no mask can cite a
	// background point, so these digests sit beside the pack digest rather
	// than inside it.
	BackgroundCount        int    `json:"background_count,omitempty"`
	BackgroundsSHA256      string `json:"backgrounds_sha256,omitempty"`
	BackgroundPointsSHA256 string `json:"background_points_sha256,omitempty"`

	Completeness Completeness `json:"completeness"`

	// Version 2 only. The packet table holds every packet that contributed a
	// return to a sample's frame, keyed by the first 64 bits of SHA-256 over
	// its UDP payload; returns.bin holds one return number per point.
	ReturnLayout  *ReturnLayout `json:"return_layout,omitempty"`
	PacketKeyKind string        `json:"packet_key,omitempty"`
	PacketCount   int           `json:"packet_count,omitempty"`
	PacketsSHA256 string        `json:"packets_sha256,omitempty"`
	ReturnsSHA256 string        `json:"returns_sha256,omitempty"`
}

// PacketKeyKind is the only packet key derivation version 2 packs use.
const PacketKeyKind = "sha256-64:udp-payload"

// Background is one settled-background snapshot carried with a pack.
//
// Snapshots are ordered by SourceOrdinal, their position in the recording, and
// the one in force at a sample is the last whose ordinal does not exceed the
// sample's. Not by timestamp, and not by sequence number: a replay that was
// settled ahead of time records its first snapshot stamped later than every
// frame that follows it, and the sequence number only moves on a grid reset.
type Background struct {
	BackgroundID     int    `json:"background_id"`
	SourceOrdinal    int    `json:"source_ordinal"`
	TimestampNs      int64  `json:"timestamp_ns"`
	SequenceNumber   uint64 `json:"sequence_number"`
	SettlingComplete bool   `json:"settling_complete"`
	PointCount       int    `json:"point_count"`
	ByteOffset       int64  `json:"byte_offset"`
}

// backgroundBytes is the encoded size of one snapshot: X, Y and Z as float32.
func backgroundBytes(n int) int64 { return int64(n) * 4 * 3 }

// encodeBackground lays a snapshot out as all X, then all Y, then all Z.
func encodeBackground(x, y, z []float32) ([]byte, error) {
	n := len(x)
	if len(y) != n || len(z) != n {
		return nil, fmt.Errorf("background coordinate arrays differ in length: %d, %d, %d", n, len(y), len(z))
	}
	buf := make([]byte, backgroundBytes(n))
	off := 0
	for _, axis := range [][]float32{x, y, z} {
		for _, v := range axis {
			binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(v))
			off += 4
		}
	}
	return buf, nil
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
	// HasIntensity records whether this sample's intensity bytes were
	// measured. The block is fixed-size and zero-fills an absent column, so
	// a zero byte cannot say which it is; only this flag can. Nil means the
	// pack predates per-sample presence and only the manifest's pack-wide
	// flag is known. Read it through Pack.SampleHasIntensity.
	HasIntensity *bool `json:"has_intensity,omitempty"`
	// ReturnStart and ReturnEnd bound, in version 2, every return of the frame
	// the sample came from, foreground or not: [start, end) in the pack's
	// return numbering. A window of a label set is that range.
	ReturnStart uint64 `json:"return_start,omitempty"`
	ReturnEnd   uint64 `json:"return_end,omitempty"`
}

// SamplesRecordIntensityPresence reports whether every sample states its own
// intensity presence. False for a pack written before the field existed, and
// for one that records it on some samples only, in which case the manifest's
// pack-wide flag is all the others have.
func SamplesRecordIntensityPresence(samples []Sample) bool {
	if len(samples) == 0 {
		return false
	}
	for _, s := range samples {
		if s.HasIntensity == nil {
			return false
		}
	}
	return true
}

// checkIntensityPresence refuses a manifest that contradicts the samples.
// The manifest flag means "any sample was measured", so when every sample
// records its own flag the two are bound: a pack-wide true over samples that
// all say false would send a reader to zero bytes as if they were readings.
// A pack that records presence on some samples only is not checked, since
// the manifest is then the only word on the rest.
func checkIntensityPresence(m Manifest, samples []Sample) error {
	if !SamplesRecordIntensityPresence(samples) {
		return nil
	}
	any := false
	for _, s := range samples {
		any = any || *s.HasIntensity
	}
	if any != m.HasIntensity {
		return fmt.Errorf("manifest has_intensity=%t but the samples' own flags say %t", m.HasIntensity, any)
	}
	return nil
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
	Dir         string
	Manifest    Manifest
	Samples     []Sample
	Backgrounds []Background

	raw           []byte // points.bin contents, verified against the manifest digest
	rawBackground []byte // background.bin contents, likewise, when present

	// Version 2 only: the packet table and returns.bin contents.
	Packets    []uint64
	rawReturns []byte
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

// EncodePoints is the canonical block encoding for one sample, for writers of
// packs other than Export: synthetic fixtures for evaluators downstream of
// this package, which cannot otherwise produce a block WritePack accepts.
func EncodePoints(p Points) ([]byte, error) { return encodePoints(p) }

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
	return WritePackWithBackground(dir, m, samples, blocks, nil, nil)
}

// WritePackWithBackground is WritePack plus the settled-background snapshots
// in force over the excerpt. With none it writes exactly what WritePack
// always has: no extra files, and no extra manifest keys.
func WritePackWithBackground(
	dir string, m Manifest, samples []Sample, blocks [][]byte,
	backgrounds []Background, backgroundBlocks [][]byte,
) error {
	return writePack(dir, m, samples, blocks, nil, backgrounds, backgroundBlocks)
}

// PackReturns is what a version 2 pack adds to the points: the packet table
// and, for each sample, the return each point was decoded from, in point
// order. Returns within a sample are strictly increasing, because a frame's
// points keep the order the parser emitted them in, and they lie within the
// sample's [ReturnStart, ReturnEnd).
type PackReturns struct {
	Layout  ReturnLayout
	Packets []uint64
	Returns [][]uint64
}

// WritePackV2 writes a version 2 pack: the points, and the returns that make
// them citable by a label set independently of this run. Samples must carry
// ReturnStart and ReturnEnd.
func WritePackV2(
	dir string, m Manifest, samples []Sample, blocks [][]byte, returns PackReturns,
	backgrounds []Background, backgroundBlocks [][]byte,
) error {
	return writePack(dir, m, samples, blocks, &returns, backgrounds, backgroundBlocks)
}

func writePack(
	dir string, m Manifest, samples []Sample, blocks [][]byte, returns *PackReturns,
	backgrounds []Background, backgroundBlocks [][]byte,
) error {
	if len(backgrounds) != len(backgroundBlocks) {
		return fmt.Errorf("%d backgrounds but %d background blocks", len(backgrounds), len(backgroundBlocks))
	}
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

	if err := checkIntensityPresence(m, samples); err != nil {
		return err
	}
	samplesJSON, err := json.MarshalIndent(samples, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal samples: %w", err)
	}

	m.SchemaVersion = PackSchemaVersionLegacy
	m.SampleCount = len(samples)
	m.PointCount = 0
	for _, s := range samples {
		m.PointCount += int64(s.PointCount)
	}
	m.PointsSHA256 = sha256Hex(points)
	m.SamplesSHA256 = sha256Hex(samplesJSON)
	files := map[string][]byte{
		pointsFile:  points,
		samplesFile: append(samplesJSON, '\n'),
	}
	m.ReturnLayout, m.PacketKeyKind, m.PacketCount, m.PacketsSHA256, m.ReturnsSHA256 = nil, "", 0, "", ""
	if returns != nil {
		packetBytes, returnBytes, err := encodeReturns(samples, *returns)
		if err != nil {
			return err
		}
		layout := returns.Layout
		m.SchemaVersion = PackSchemaVersion
		m.ReturnLayout = &layout
		m.PacketKeyKind = PacketKeyKind
		m.PacketCount = len(returns.Packets)
		m.PacketsSHA256 = sha256Hex(packetBytes)
		m.ReturnsSHA256 = sha256Hex(returnBytes)
		files[packetsFile] = packetBytes
		files[returnsFile] = returnBytes
	}
	m.PackDigest = packDigest(m)
	if m.DatasetID == "" {
		m.DatasetID = "ds_" + m.PackDigest[7:23]
	}
	if len(backgrounds) > 0 {
		var backgroundPoints []byte
		for i := range backgrounds {
			backgrounds[i].BackgroundID = i
			backgrounds[i].ByteOffset = int64(len(backgroundPoints))
			if got, want := int64(len(backgroundBlocks[i])), backgroundBytes(backgrounds[i].PointCount); got != want {
				return fmt.Errorf("background %d: block is %d bytes, want %d", i, got, want)
			}
			backgroundPoints = append(backgroundPoints, backgroundBlocks[i]...)
		}
		backgroundsJSON, err := json.MarshalIndent(backgrounds, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal backgrounds: %w", err)
		}
		m.BackgroundCount = len(backgrounds)
		m.BackgroundsSHA256 = sha256Hex(backgroundsJSON)
		m.BackgroundPointsSHA256 = sha256Hex(backgroundPoints)
		files[backgroundsFile] = append(backgroundsJSON, '\n')
		files[backgroundPointsFile] = backgroundPoints
	}

	manifestJSON, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	files[manifestFile] = append(manifestJSON, '\n')

	staging := dir + ".partial"
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("clear staging directory: %w", err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(staging, name), content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("refusing to overwrite an existing pack at %s: a changed point domain is a new pack", dir)
	}
	return os.Rename(staging, dir)
}

// packDigest binds the digests an annotation depends on. Version 2 adds the
// returns, since a label set cites points through them.
func packDigest(m Manifest) string {
	if m.SchemaVersion == PackSchemaVersionLegacy {
		return sha256Hex([]byte(m.PointsSHA256 + "\n" + m.SamplesSHA256))
	}
	return sha256Hex([]byte(m.PointsSHA256 + "\n" + m.SamplesSHA256 + "\n" + m.PacketsSHA256 + "\n" + m.ReturnsSHA256))
}

// encodeReturns lays out packets.bin and returns.bin and checks the returns
// against the samples: one per point, strictly increasing within a sample, and
// inside the sample's frame range, which lies inside the packet table.
func encodeReturns(samples []Sample, r PackReturns) ([]byte, []byte, error) {
	if err := r.Layout.validate(); err != nil {
		return nil, nil, err
	}
	if len(r.Returns) != len(samples) {
		return nil, nil, fmt.Errorf("%d samples but returns for %d", len(samples), len(r.Returns))
	}
	seen := make(map[uint64]bool, len(r.Packets))
	packetBytes := make([]byte, 8*len(r.Packets))
	for i, k := range r.Packets {
		if seen[k] {
			return nil, nil, fmt.Errorf("packet table lists key %016x twice", k)
		}
		seen[k] = true
		binary.LittleEndian.PutUint64(packetBytes[8*i:], k)
	}
	limit := uint64(len(r.Packets)) * r.Layout.PerPacket()
	var returnBytes []byte
	for i, s := range samples {
		if err := checkSampleReturns(i, s, r.Returns[i], limit); err != nil {
			return nil, nil, err
		}
		for _, v := range r.Returns[i] {
			returnBytes = binary.LittleEndian.AppendUint64(returnBytes, v)
		}
	}
	return packetBytes, returnBytes, nil
}

func checkSampleReturns(i int, s Sample, returns []uint64, limit uint64) error {
	if len(returns) != s.PointCount {
		return fmt.Errorf("sample %d has %d points but %d returns", i, s.PointCount, len(returns))
	}
	if s.ReturnStart >= s.ReturnEnd || s.ReturnEnd > limit {
		return fmt.Errorf("sample %d frame range [%d,%d) is empty or outside the %d returns the packet table numbers", i, s.ReturnStart, s.ReturnEnd, limit)
	}
	for j, v := range returns {
		if v < s.ReturnStart || v >= s.ReturnEnd {
			return fmt.Errorf("sample %d point %d is return %d, outside its frame [%d,%d)", i, j, v, s.ReturnStart, s.ReturnEnd)
		}
		if j > 0 && v <= returns[j-1] {
			return fmt.Errorf("sample %d point %d: returns must increase with point order, %d follows %d", i, j, v, returns[j-1])
		}
	}
	return nil
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
	if m.SchemaVersion != PackSchemaVersion && m.SchemaVersion != PackSchemaVersionLegacy {
		return nil, fmt.Errorf("pack schema version %d, this build reads %d and %d", m.SchemaVersion, PackSchemaVersionLegacy, PackSchemaVersion)
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
	if got := packDigest(m); got != m.PackDigest {
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
	// The manifest is outside the pack digest, so an edit to its flag would
	// otherwise pass every digest check above.
	if err := checkIntensityPresence(m, samples); err != nil {
		return nil, err
	}

	pack := &Pack{Dir: dir, Manifest: m, Samples: samples, raw: raw}
	if m.SchemaVersion == PackSchemaVersion {
		if err := pack.openReturns(); err != nil {
			return nil, err
		}
	}
	if m.BackgroundCount > 0 {
		if err := pack.openBackground(); err != nil {
			return nil, err
		}
	}
	return pack, nil
}

// openReturns reads and verifies a version 2 pack's packet table and returns.
func (p *Pack) openReturns() error {
	m := p.Manifest
	if m.ReturnLayout == nil {
		return fmt.Errorf("version 2 pack has no return layout")
	}
	if err := m.ReturnLayout.validate(); err != nil {
		return err
	}
	if m.PacketKeyKind != PacketKeyKind {
		return fmt.Errorf("packet key %q, this build reads %q", m.PacketKeyKind, PacketKeyKind)
	}
	packetBytes, err := os.ReadFile(filepath.Join(p.Dir, packetsFile))
	if err != nil {
		return fmt.Errorf("read packets: %w", err)
	}
	if got := sha256Hex(packetBytes); got != m.PacketsSHA256 {
		return fmt.Errorf("packets digest mismatch: file is %s, manifest says %s", got, m.PacketsSHA256)
	}
	returnBytes, err := os.ReadFile(filepath.Join(p.Dir, returnsFile))
	if err != nil {
		return fmt.Errorf("read returns: %w", err)
	}
	if got := sha256Hex(returnBytes); got != m.ReturnsSHA256 {
		return fmt.Errorf("returns digest mismatch: file is %s, manifest says %s", got, m.ReturnsSHA256)
	}
	if len(packetBytes)%8 != 0 || len(packetBytes)/8 != m.PacketCount {
		return fmt.Errorf("packets.bin is %d bytes for %d declared packets", len(packetBytes), m.PacketCount)
	}
	if int64(len(returnBytes)) != 8*m.PointCount {
		return fmt.Errorf("returns.bin is %d bytes for %d points", len(returnBytes), m.PointCount)
	}
	packets := make([]uint64, m.PacketCount)
	seen := make(map[uint64]bool, m.PacketCount)
	for i := range packets {
		packets[i] = binary.LittleEndian.Uint64(packetBytes[8*i:])
		if seen[packets[i]] {
			return fmt.Errorf("packet table lists key %016x twice", packets[i])
		}
		seen[packets[i]] = true
	}
	limit := uint64(m.PacketCount) * m.ReturnLayout.PerPacket()
	var off int64
	for i, s := range p.Samples {
		returns := decodeReturns(returnBytes[off : off+8*int64(s.PointCount)])
		if err := checkSampleReturns(i, s, returns, limit); err != nil {
			return err
		}
		off += 8 * int64(s.PointCount)
	}
	p.Packets = packets
	p.rawReturns = returnBytes
	return nil
}

func decodeReturns(b []byte) []uint64 {
	out := make([]uint64, len(b)/8)
	for i := range out {
		out[i] = binary.LittleEndian.Uint64(b[8*i:])
	}
	return out
}

// HasReturns reports whether the pack is version 2 and so can be cited by a
// label set.
func (p *Pack) HasReturns() bool { return p.rawReturns != nil }

// ReturnsAt returns one sample's return numbers, one per point in point order.
func (p *Pack) ReturnsAt(sampleID int) ([]uint64, error) {
	if !p.HasReturns() {
		return nil, fmt.Errorf("version %d pack carries no returns", p.Manifest.SchemaVersion)
	}
	if sampleID < 0 || sampleID >= len(p.Samples) {
		return nil, fmt.Errorf("sample %d is outside the pack's %d samples", sampleID, len(p.Samples))
	}
	var off int64
	for _, s := range p.Samples[:sampleID] {
		off += 8 * int64(s.PointCount)
	}
	return decodeReturns(p.rawReturns[off : off+8*int64(p.Samples[sampleID].PointCount)]), nil
}

// openBackground reads and verifies the background context. A pack that
// declares one and cannot produce it is refused like any other mismatch: an
// operator shown the wrong background would label against a scene that was
// not there.
func (p *Pack) openBackground() error {
	m := p.Manifest
	index, err := os.ReadFile(filepath.Join(p.Dir, backgroundsFile))
	if err != nil {
		return fmt.Errorf("read backgrounds: %w", err)
	}
	if got := sha256Hex(trimTrailingNewline(index)); got != m.BackgroundsSHA256 {
		return fmt.Errorf("backgrounds digest mismatch: file is %s, manifest says %s", got, m.BackgroundsSHA256)
	}
	var backgrounds []Background
	if err := json.Unmarshal(index, &backgrounds); err != nil {
		return fmt.Errorf("parse backgrounds: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(p.Dir, backgroundPointsFile))
	if err != nil {
		return fmt.Errorf("read background points: %w", err)
	}
	if got := sha256Hex(raw); got != m.BackgroundPointsSHA256 {
		return fmt.Errorf("background points digest mismatch: file is %s, manifest says %s", got, m.BackgroundPointsSHA256)
	}
	if len(backgrounds) != m.BackgroundCount {
		return fmt.Errorf("manifest declares %d backgrounds, file holds %d", m.BackgroundCount, len(backgrounds))
	}
	for i, b := range backgrounds {
		if b.BackgroundID != i {
			return fmt.Errorf("background %d carries id %d: ids must be dense and ordered", i, b.BackgroundID)
		}
		if b.PointCount < 0 || b.PointCount > MaxPointsPerSample {
			return fmt.Errorf("background %d declares %d points", i, b.PointCount)
		}
		if i > 0 && b.SourceOrdinal < backgrounds[i-1].SourceOrdinal {
			return fmt.Errorf("background %d is out of recording order", i)
		}
		end := b.ByteOffset + backgroundBytes(b.PointCount)
		if b.ByteOffset < 0 || end > int64(len(raw)) {
			return fmt.Errorf("background %d spans bytes [%d,%d) of a %d-byte file", i, b.ByteOffset, end, len(raw))
		}
	}
	p.Backgrounds = backgrounds
	p.rawBackground = raw
	return nil
}

// BackgroundInForce returns the snapshot an operator should see behind a
// sample: the last one recorded at or before it. ok is false when the pack
// carries none, or none that early.
func (p *Pack) BackgroundInForce(sampleID int) (Background, bool) {
	if sampleID < 0 || sampleID >= len(p.Samples) {
		return Background{}, false
	}
	var found Background
	ok := false
	for _, b := range p.Backgrounds {
		if b.SourceOrdinal > p.Samples[sampleID].SourceOrdinal {
			break
		}
		found, ok = b, true
	}
	return found, ok
}

// SampleHasIntensity reports whether a sample's intensity bytes are
// measurements: the sample's own flag when it recorded one, else the
// manifest's pack-wide flag, which is the most a legacy pack can say. It is
// never inferred from the bytes. Out-of-range samples are reported absent.
func (p *Pack) SampleHasIntensity(sampleID int) bool {
	if sampleID < 0 || sampleID >= len(p.Samples) {
		return false
	}
	if flag := p.Samples[sampleID].HasIntensity; flag != nil {
		return *flag
	}
	return p.Manifest.HasIntensity
}

// IntensityPresenceIsPerSample reports whether SampleHasIntensity is answering
// from each sample's own record rather than falling back to the pack-wide
// flag for any of them. A reader that needs to know every byte it reads was
// measured, not merely that some sample's were, checks this first.
func (p *Pack) IntensityPresenceIsPerSample() bool {
	return SamplesRecordIntensityPresence(p.Samples)
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
