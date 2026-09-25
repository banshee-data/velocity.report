package vrlog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// Hard ceilings. A writer may declare smaller limits in its manifest; a
// reader refuses a manifest declaring larger ones, so no container can make
// this package allocate beyond them. The byte values are the VRLOG plan's
// provisional test limits (§4.1); the counts sit well above any sensor this
// project runs (a Pandar40P returns at most 72,000 points per rotation, twice
// that in dual-return mode). All are to be revisited from dense-scene
// measurements, not from these convenient numbers.
const (
	DefaultTargetChunkBytes    = 8 << 20
	HardMaxChunkBytes          = 128 << 20
	HardMaxRecordBytes         = 64 << 20
	HardMaxRecordsPerChunk     = 1 << 16
	HardMaxPointsPerFrame      = 1 << 20
	HardMaxClustersPerFrame    = 1 << 16
	HardMaxStagesPerFrame      = 64
	maxCaptureFiles            = 4096
	maxMetadataObjects         = 64
	maxRequiredFeatureNameSize = 256
)

// Limits are what a writer promises about every record and chunk, declared
// in the manifest and enforced by the reader before allocation.
type Limits struct {
	// TargetChunkBytes is where the writer seals a chunk; MaxChunkBytes is a
	// bound it never crosses, rotating before a record would cross it.
	TargetChunkBytes    uint64
	MaxChunkBytes       uint64
	MaxRecordBytes      uint32
	MaxRecordsPerChunk  uint32
	MaxPointsPerFrame   uint32
	MaxClustersPerFrame uint32
	MaxStagesPerFrame   uint32
}

// DefaultLimits returns the provisional limits.
func DefaultLimits() Limits {
	return Limits{
		TargetChunkBytes:    DefaultTargetChunkBytes,
		MaxChunkBytes:       HardMaxChunkBytes,
		MaxRecordBytes:      HardMaxRecordBytes,
		MaxRecordsPerChunk:  HardMaxRecordsPerChunk,
		MaxPointsPerFrame:   HardMaxPointsPerFrame,
		MaxClustersPerFrame: HardMaxClustersPerFrame,
		MaxStagesPerFrame:   HardMaxStagesPerFrame,
	}
}

func (l Limits) validate() error {
	switch {
	case l.MaxChunkBytes == 0 || l.MaxChunkBytes > HardMaxChunkBytes:
		return fmt.Errorf("chunk bound %d outside (0, %d]", l.MaxChunkBytes, HardMaxChunkBytes)
	case l.TargetChunkBytes == 0 || l.TargetChunkBytes > l.MaxChunkBytes:
		return fmt.Errorf("target chunk size %d outside (0, %d]", l.TargetChunkBytes, l.MaxChunkBytes)
	case l.MaxRecordBytes == 0 || l.MaxRecordBytes > HardMaxRecordBytes:
		return fmt.Errorf("record bound %d outside (0, %d]", l.MaxRecordBytes, HardMaxRecordBytes)
	case uint64(l.MaxRecordBytes)+envelopeSize+chunkOverhead > l.MaxChunkBytes:
		// A record that cannot fit an empty chunk could never be written.
		return fmt.Errorf("a %d-byte record cannot fit a %d-byte chunk", l.MaxRecordBytes, l.MaxChunkBytes)
	case l.MaxRecordsPerChunk == 0 || l.MaxRecordsPerChunk > HardMaxRecordsPerChunk:
		return fmt.Errorf("records per chunk %d outside (0, %d]", l.MaxRecordsPerChunk, HardMaxRecordsPerChunk)
	case l.MaxPointsPerFrame == 0 || l.MaxPointsPerFrame > HardMaxPointsPerFrame:
		return fmt.Errorf("points per frame %d outside (0, %d]", l.MaxPointsPerFrame, HardMaxPointsPerFrame)
	case l.MaxClustersPerFrame == 0 || l.MaxClustersPerFrame > HardMaxClustersPerFrame:
		return fmt.Errorf("clusters per frame %d outside (0, %d]", l.MaxClustersPerFrame, HardMaxClustersPerFrame)
	case l.MaxStagesPerFrame == 0 || l.MaxStagesPerFrame > HardMaxStagesPerFrame:
		return fmt.Errorf("stages per frame %d outside (0, %d]", l.MaxStagesPerFrame, HardMaxStagesPerFrame)
	}
	return nil
}

// Manifest is the container's immutable root declaration.
type Manifest struct {
	// Profile is the evidence profile every record satisfies.
	Profile l4bobserve.Profile
	// RequiredFeatures are format features a reader must understand. It is
	// empty in container 1.0; a reader refuses any it does not know.
	RequiredFeatures []string
	Capture          CaptureIdentity
	Extraction       ExtractionIdentity
	Calibration      l4bobserve.Calibration
	Limits           Limits
	Provenance       Provenance
	// Metadata objects are embedded by value with a SHA-256 the reader checks.
	Metadata []MetadataObject
}

// CaptureIdentity names one capture. UUID and CreatedUnixNanos are the
// ephemeral fields: a repeat replay of the same source differs in them and in
// nothing a record carries.
type CaptureIdentity struct {
	// UUID is allocated by the writer when empty.
	UUID       string
	SensorID   string
	SourceType string // "pcap", "live" or "synthetic"
	ParentUUID string
	// CreatedUnixNanos is set by the writer.
	CreatedUnixNanos int64
}

// ExtractionIdentity binds records to their ordered source and extractor.
type ExtractionIdentity struct {
	SourceID        string
	CalibrationID   string
	CoordinateFrame string
	ReplayCaseID    string
	CaptureFiles    []CaptureFile
	ExtractorID     string
	Window          SourceWindow
}

// CaptureFile is one ordered source file and its content digest.
type CaptureFile struct {
	Path   string
	SHA256 string
}

// SourceWindow is the configured replay window, relative to the capture
// sequence's first packet. A zero duration is the rest of the capture.
type SourceWindow struct {
	StartOffsetNanos int64
	WarmupNanos      int64
	DurationNanos    int64
}

// Provenance identifies the writing software and effective configuration.
type Provenance struct {
	BuildVersion        string
	BuildGitSHA         string
	Writer              string
	ParamsHash          string
	ParamsSchemaVersion string
	Experiments         []string
}

// MetadataObject is a small immutable object embedded in the manifest.
type MetadataObject struct {
	Name      string
	MediaType string
	Content   []byte
	// SHA256 is the lower-case hex digest of Content. The writer computes it;
	// the reader refuses a mismatch.
	SHA256 string
}

// knownFeatures are the required features this reader implements. Container
// 1.0 defines none; a 1.x writer that needs a reader to understand something
// new (a compressed codec, a changed field meaning) adds a name here and to
// its manifest, and every older reader refuses the container.
var knownFeatures = map[string]bool{}

// validate checks everything a manifest promises that can be checked without
// the records: a known profile whose capabilities match its declaration,
// identities that recompute from the fields beside them, and limits within
// the hard ceilings.
func (m Manifest) validate() error {
	declared, err := l4bobserve.LookupProfile(m.Profile.Name)
	if err != nil {
		return &UnsupportedError{What: fmt.Sprintf("evidence profile %q", m.Profile.Name)}
	}
	if !slices.Equal(declared.Capabilities.List(), m.Profile.Capabilities.List()) {
		return fmt.Errorf("manifest lists capabilities %v for profile %q, which declares %v",
			m.Profile.Capabilities.List(), m.Profile.Name, declared.Capabilities.List())
	}
	for _, feature := range m.RequiredFeatures {
		if !knownFeatures[feature] {
			return &UnsupportedError{What: fmt.Sprintf("required feature %q", feature)}
		}
	}
	if err := m.Limits.validate(); err != nil {
		return fmt.Errorf("limits: %w", err)
	}
	c, e := m.Capture, m.Extraction
	for name, value := range map[string]string{
		"capture UUID": c.UUID, "sensor": c.SensorID, "source type": c.SourceType,
		"source identity": e.SourceID, "calibration identity": e.CalibrationID,
		"coordinate frame": e.CoordinateFrame, "extractor identity": e.ExtractorID,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("manifest needs a %s", name)
		}
	}
	if m.Calibration.SensorID != c.SensorID {
		return fmt.Errorf("calibration is for sensor %q, capture is from %q", m.Calibration.SensorID, c.SensorID)
	}
	calibrationID, err := l4bobserve.CalibrationID(m.Calibration)
	if err != nil {
		return fmt.Errorf("calibration: %w", err)
	}
	if calibrationID != e.CalibrationID {
		return fmt.Errorf("calibration identity %s does not match the embedded transform (%s)", e.CalibrationID, calibrationID)
	}
	if len(e.CaptureFiles) > maxCaptureFiles {
		return fmt.Errorf("%d capture files exceed %d", len(e.CaptureFiles), maxCaptureFiles)
	}
	// A replayed source's identity is content-addressed from its ordered
	// files; recompute it rather than trust it. A live capture has no files.
	if e.ReplayCaseID != "" || len(e.CaptureFiles) > 0 {
		source := l4bobserve.CaptureSource{ReplayCaseID: e.ReplayCaseID, ExtractorID: e.ExtractorID}
		for _, f := range e.CaptureFiles {
			source.CapturePaths = append(source.CapturePaths, f.Path)
			source.CaptureSHA256s = append(source.CaptureSHA256s, f.SHA256)
		}
		sourceID, err := l4bobserve.SourceID(source)
		if err != nil {
			return fmt.Errorf("source: %w", err)
		}
		if sourceID != e.SourceID {
			return fmt.Errorf("source identity %s does not match its capture files and extractor (%s)", e.SourceID, sourceID)
		}
	}
	if len(m.Metadata) > maxMetadataObjects {
		return fmt.Errorf("%d metadata objects exceed %d", len(m.Metadata), maxMetadataObjects)
	}
	names := map[string]bool{}
	for _, object := range m.Metadata {
		if object.Name == "" || names[object.Name] {
			return fmt.Errorf("metadata object name %q is empty or repeated", object.Name)
		}
		names[object.Name] = true
		if sum := sha256.Sum256(object.Content); hex.EncodeToString(sum[:]) != object.SHA256 {
			return fmt.Errorf("metadata object %q does not match its SHA-256", object.Name)
		}
	}
	return nil
}

func (m Manifest) toProto() *pb.RecordingManifest {
	caps := m.Profile.Capabilities.List()
	capabilities := make([]string, len(caps))
	for i, c := range caps {
		capabilities[i] = string(c)
	}
	files := make([]*pb.CaptureFile, len(m.Extraction.CaptureFiles))
	for i, f := range m.Extraction.CaptureFiles {
		files[i] = &pb.CaptureFile{Path: f.Path, Sha256: f.SHA256}
	}
	metadata := make([]*pb.MetadataObject, len(m.Metadata))
	for i, o := range m.Metadata {
		metadata[i] = &pb.MetadataObject{Name: o.Name, MediaType: o.MediaType, Content: o.Content, Sha256: o.SHA256}
	}
	l := m.Limits
	return &pb.RecordingManifest{
		SchemaVersion:         pb.SchemaVersion_SCHEMA_VERSION_1,
		StreamKind:            pb.StreamKind_STREAM_KIND_OBSERVATION,
		Profile:               string(m.Profile.Name),
		Capabilities:          capabilities,
		RequiredFeatures:      m.RequiredFeatures,
		SemanticDigestVersion: l4bobserve.SemanticDigestVersion,
		Capture: &pb.CaptureIdentity{
			CaptureUuid: m.Capture.UUID, SensorId: m.Capture.SensorID, SourceType: m.Capture.SourceType,
			ParentCaptureUuid: m.Capture.ParentUUID, CreatedUnixNanos: m.Capture.CreatedUnixNanos,
		},
		Extraction: &pb.ExtractionIdentity{
			SourceId: m.Extraction.SourceID, CalibrationId: m.Extraction.CalibrationID,
			CoordinateFrame: m.Extraction.CoordinateFrame, ReplayCaseId: m.Extraction.ReplayCaseID,
			CaptureFiles: files, ExtractorId: m.Extraction.ExtractorID,
			Window: &pb.SourceWindow{
				StartOffsetNanos: m.Extraction.Window.StartOffsetNanos,
				WarmupNanos:      m.Extraction.Window.WarmupNanos,
				DurationNanos:    m.Extraction.Window.DurationNanos,
			},
		},
		Calibration: &pb.Calibration{
			SensorId: m.Calibration.SensorID, FromFrame: m.Calibration.FromFrame, ToFrame: m.Calibration.ToFrame,
			Transform: m.Calibration.Transform[:],
		},
		Limits: &pb.Limits{
			TargetChunkBytes: l.TargetChunkBytes, MaxChunkBytes: l.MaxChunkBytes, MaxRecordBytes: l.MaxRecordBytes,
			MaxRecordsPerChunk: l.MaxRecordsPerChunk, MaxPointsPerFrame: l.MaxPointsPerFrame,
			MaxClustersPerFrame: l.MaxClustersPerFrame, MaxStagesPerFrame: l.MaxStagesPerFrame,
		},
		Provenance: &pb.Provenance{
			BuildVersion: m.Provenance.BuildVersion, BuildGitSha: m.Provenance.BuildGitSHA, Writer: m.Provenance.Writer,
			ParamsHash: m.Provenance.ParamsHash, ParamsSchemaVersion: m.Provenance.ParamsSchemaVersion,
			Experiments: m.Provenance.Experiments,
		},
		Metadata: metadata,
	}
}

// manifestFromProto maps the stored manifest and refuses what this reader
// cannot interpret: an unknown schema version, stream kind or digest version.
// Field-level promises are then checked by validate.
func manifestFromProto(p *pb.RecordingManifest) (Manifest, error) {
	if p.GetSchemaVersion() != pb.SchemaVersion_SCHEMA_VERSION_1 {
		return Manifest{}, &UnsupportedError{What: fmt.Sprintf("recording schema version %d", p.GetSchemaVersion())}
	}
	if p.GetStreamKind() != pb.StreamKind_STREAM_KIND_OBSERVATION {
		return Manifest{}, &UnsupportedError{What: fmt.Sprintf("stream kind %d", p.GetStreamKind())}
	}
	if p.GetSemanticDigestVersion() != l4bobserve.SemanticDigestVersion {
		return Manifest{}, &UnsupportedError{What: fmt.Sprintf("semantic digest version %q", p.GetSemanticDigestVersion())}
	}
	for _, feature := range p.GetRequiredFeatures() {
		if len(feature) > maxRequiredFeatureNameSize {
			return Manifest{}, fmt.Errorf("required feature name of %d bytes", len(feature))
		}
	}
	caps := make([]l4bobserve.Capability, len(p.GetCapabilities()))
	for i, c := range p.GetCapabilities() {
		caps[i] = l4bobserve.Capability(c)
	}
	transform := p.GetCalibration().GetTransform()
	if len(transform) != 16 {
		return Manifest{}, fmt.Errorf("calibration transform has %d values, not 16", len(transform))
	}
	m := Manifest{
		Profile:          l4bobserve.Profile{Name: l4bobserve.ProfileName(p.GetProfile()), Capabilities: l4bobserve.NewCapabilitySet(caps...)},
		RequiredFeatures: p.GetRequiredFeatures(),
		Capture: CaptureIdentity{
			UUID: p.GetCapture().GetCaptureUuid(), SensorID: p.GetCapture().GetSensorId(),
			SourceType: p.GetCapture().GetSourceType(), ParentUUID: p.GetCapture().GetParentCaptureUuid(),
			CreatedUnixNanos: p.GetCapture().GetCreatedUnixNanos(),
		},
		Extraction: ExtractionIdentity{
			SourceID: p.GetExtraction().GetSourceId(), CalibrationID: p.GetExtraction().GetCalibrationId(),
			CoordinateFrame: p.GetExtraction().GetCoordinateFrame(), ReplayCaseID: p.GetExtraction().GetReplayCaseId(),
			ExtractorID: p.GetExtraction().GetExtractorId(),
			Window: SourceWindow{
				StartOffsetNanos: p.GetExtraction().GetWindow().GetStartOffsetNanos(),
				WarmupNanos:      p.GetExtraction().GetWindow().GetWarmupNanos(),
				DurationNanos:    p.GetExtraction().GetWindow().GetDurationNanos(),
			},
		},
		Calibration: l4bobserve.Calibration{
			SensorID: p.GetCalibration().GetSensorId(), FromFrame: p.GetCalibration().GetFromFrame(),
			ToFrame: p.GetCalibration().GetToFrame(), Transform: [16]float64(transform),
		},
		Limits: Limits{
			TargetChunkBytes: p.GetLimits().GetTargetChunkBytes(), MaxChunkBytes: p.GetLimits().GetMaxChunkBytes(),
			MaxRecordBytes: p.GetLimits().GetMaxRecordBytes(), MaxRecordsPerChunk: p.GetLimits().GetMaxRecordsPerChunk(),
			MaxPointsPerFrame: p.GetLimits().GetMaxPointsPerFrame(), MaxClustersPerFrame: p.GetLimits().GetMaxClustersPerFrame(),
			MaxStagesPerFrame: p.GetLimits().GetMaxStagesPerFrame(),
		},
		Provenance: Provenance{
			BuildVersion: p.GetProvenance().GetBuildVersion(), BuildGitSHA: p.GetProvenance().GetBuildGitSha(),
			Writer: p.GetProvenance().GetWriter(), ParamsHash: p.GetProvenance().GetParamsHash(),
			ParamsSchemaVersion: p.GetProvenance().GetParamsSchemaVersion(), Experiments: p.GetProvenance().GetExperiments(),
		},
	}
	for _, f := range p.GetExtraction().GetCaptureFiles() {
		m.Extraction.CaptureFiles = append(m.Extraction.CaptureFiles, CaptureFile{Path: f.GetPath(), SHA256: f.GetSha256()})
	}
	for _, o := range p.GetMetadata() {
		m.Metadata = append(m.Metadata, MetadataObject{Name: o.GetName(), MediaType: o.GetMediaType(), Content: o.GetContent(), SHA256: o.GetSha256()})
	}
	return m, nil
}

// NewMetadataObject returns an embedded object with its digest filled in.
func NewMetadataObject(name, mediaType string, content []byte) MetadataObject {
	sum := sha256.Sum256(content)
	return MetadataObject{Name: name, MediaType: mediaType, Content: slices.Clone(content), SHA256: hex.EncodeToString(sum[:])}
}

// MetadataObject returns the embedded object with the given name.
func (m Manifest) MetadataObject(name string) (MetadataObject, bool) {
	for _, o := range m.Metadata {
		if o.Name == name {
			return o, true
		}
	}
	return MetadataObject{}, false
}
