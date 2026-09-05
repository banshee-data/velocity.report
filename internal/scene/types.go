// Package scene produces static, browser-servable JSON views of a recorded
// VRLOG. The recorded VRLOG stays the source of truth for what a run analysed;
// a scene export is a derived, lossy-by-selection view of it, regenerable at
// any time from the source.
//
// The export is deliberately not a VRLOG variant. The replayer builds chunk
// paths with fmt.Sprintf("chunk_%04d.pb", …), so a renamed or recompressed
// chunk would not be found by it; rather than bend that, an export is its own
// artefact in its own directory and makes no claim to be readable by
// vrlog-analyse or the replayer.
package scene

// FormatVersion is the scene export schema version. Bump when the on-disk
// layout changes in a way an existing player cannot read.
const FormatVersion = 1

// Kind identifies what a given export carries.
type Kind string

const (
	// KindTracks carries tracked objects only: no point cloud at all. Gait and
	// body shape live in point clouds; they are absent here by construction.
	KindTracks Kind = "tracks"
	// KindClip carries foreground points plus tracks for a short window.
	KindClip Kind = "clip"
	// KindBackground carries a single static point cloud for spatial context.
	KindBackground Kind = "background"
)

// Header is written to header.json at the root of an export directory.
type Header struct {
	Version int    `json:"version"`
	Export  Kind   `json:"export"`
	Site    string `json:"site,omitempty"`
	Title   string `json:"title,omitempty"`

	SensorID   string `json:"sensor_id"`
	FrameCount int    `json:"frame_count"`

	// StartNs is the absolute capture time of the first retained frame, as a
	// decimal string. Nanosecond timestamps exceed Number.MAX_SAFE_INTEGER
	// (9.0e15), so a JSON number would silently lose about 200 ns of precision
	// in a browser. Per-frame times are microsecond offsets from this instant.
	StartNs     string  `json:"start_ns"`
	DurationSec float64 `json:"duration_sec"`

	// FrameStride is the retention interval over source frames: 2 means every
	// second source frame was kept. It is NOT a frame rate. A Pandar40P frame
	// is one sensor rotation, and rotation rate varies within a capture, so
	// playback must be driven by the recorded timestamps rather than by any
	// interval inferred from this value.
	FrameStride int `json:"frame_stride"`

	// DroppedNonMonotonic counts source frames discarded because their
	// timestamp did not advance past the previous retained frame.
	DroppedNonMonotonic int `json:"dropped_non_monotonic,omitempty"`

	ChunkEncoding string `json:"chunk_encoding"`
	ChunkFrames   int    `json:"chunk_frames"`

	CoordinateFrame CoordinateFrame `json:"coordinate_frame"`

	// Provenance. SourceVRLOGSHA256 hashes the source header.json + index.bin
	// so an export can always be traced back to the run that produced it.
	SourceVRLOGSHA256 string `json:"source_vrlog_sha256,omitempty"`
	TuningHash        string `json:"tuning_hash,omitempty"`
	BuildVersion      string `json:"build_version,omitempty"`
	GeneratedAt       string `json:"generated_at"`
}

// CoordinateFrame is the spatial reference, minus any georeferencing origin.
// Origin latitude and longitude are deliberately not carried into a published
// export.
type CoordinateFrame struct {
	FrameID        string `json:"frame_id"`
	ReferenceFrame string `json:"reference_frame"`
}

// ChunkIndex is written to index.json. Chunk start timestamps let a player
// binary-search to a chunk without fetching any frame data.
type ChunkIndex struct {
	Version int          `json:"version"`
	Chunks  []ChunkEntry `json:"chunks"`
}

// ChunkEntry describes one chunk file's time span and frame count. Times are
// microsecond offsets from the part start, matching Frame.TimeUs.
type ChunkEntry struct {
	ID      int   `json:"c"`
	Frames  int   `json:"n"`
	StartUs int64 `json:"t0"`
	EndUs   int64 `json:"t1"`
}

// Frame is one NDJSON line: a single retained frame.
//
// Field names are short on purpose. At three tracks per frame over thousands
// of frames, key names are a material share of the payload.
type Frame struct {
	Frame uint64 `json:"f"`
	// TimeUs is microseconds since the part's first retained frame. Relative
	// microseconds stay well inside Number.MAX_SAFE_INTEGER, are exact in a
	// browser, and are far finer than the ~100 ms rotation interval they
	// describe. Absolute capture time is Header.StartNs plus this offset.
	TimeUs int64        `json:"t"`
	Tracks []TrackJSON  `json:"tr,omitempty"`
	Points [][4]float64 `json:"p,omitempty"`
}

// TrackJSON is the viewer-facing projection of l9endpoints.Track. It carries
// what a renderer needs and nothing else; covariance, lifecycle counters and
// internal quality metrics are dropped.
type TrackJSON struct {
	ID      string  `json:"id"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Z       float64 `json:"z"`
	VX      float64 `json:"vx"`
	VY      float64 `json:"vy"`
	Speed   float64 `json:"spd"`
	Heading float64 `json:"hdg"`
	Length  float64 `json:"l"`
	Width   float64 `json:"w"`
	Height  float64 `json:"h"`
	BoxYaw  float64 `json:"bh"`
	Class   string  `json:"c,omitempty"`
	Conf    float64 `json:"cf,omitempty"`
}

// Manifest composes several exported parts into one logical timeline. It
// describes composition only: duration, frame count and time bounds are read
// from each part's own header, so the manifest cannot drift from its data.
type Manifest struct {
	Version int            `json:"version"`
	Site    ManifestSite   `json:"site"`
	Parts   []ManifestPart `json:"parts"`
}

// ManifestSite names the scene for display.
type ManifestSite struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// ManifestPart points at one exported part directory.
type ManifestPart struct {
	URL          string  `json:"url"`
	StartSeconds float64 `json:"start_seconds"`
	Background   string  `json:"background,omitempty"`
}
