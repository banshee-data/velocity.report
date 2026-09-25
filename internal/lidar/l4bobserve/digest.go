package l4bobserve

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"math"
)

// SemanticDigestVersion names the canonical byte stream that FrameDigest,
// GapDigest and StreamDigest hash. The digest identifies a record's values,
// not any stored bytes: protobuf output is not canonical across languages or
// library versions, so two codecs agree on evidence exactly when they agree
// on this digest. Changing any rule below changes every digest; that needs a
// new version, never an edit to this one.
//
// Rules for version 1. Integers are fixed-width little-endian. A string or
// array is its uint64 length, then its bytes or elements. Floats are their
// IEEE-754 bits, so -0 differs from +0 and a NaN keeps its payload. Fields go
// in the order FrameRecord and GapRecord declare them. Presence is an explicit
// byte, and an absent value's contents are not hashed: an unknown Count is
// unknown whatever its Value field holds, and an absent oriented box has no
// dimensions. Arrays keep their stored order. A nil and an empty column are
// the same value, because column presence is PointFields, not slice identity.
const SemanticDigestVersion = "l4bobserve.semantic/v1"

// Digest is a SHA-256 semantic digest.
type Digest [sha256.Size]byte

// String is the lower-case hex form.
func (d Digest) String() string { return hex.EncodeToString(d[:]) }

// FrameDigest returns the semantic digest of a frame record.
func FrameDigest(f FrameRecord) Digest {
	c := newCanonical("frame")
	c.u64(f.Sequence)
	c.str(f.SensorFrameID)
	c.i64(f.FrameUnixNanos)
	c.i64(f.CaptureStartUnixNanos)
	c.i64(f.CaptureEndUnixNanos)
	c.u8(uint8(f.Completeness.State))
	c.count(f.Completeness.LostPackets)
	c.u8(uint8(f.Background))
	c.u8(uint8(f.Disposition.Kind))
	c.str(string(f.Disposition.Stage))
	c.str(f.Disposition.Reason)
	c.u8(uint8(f.Payload))
	c.u64(uint64(len(f.Stages)))
	for _, s := range f.Stages {
		c.str(string(s.Stage))
		c.count(s.Input)
		c.count(s.Output)
		c.count(s.Rejected)
	}
	p := f.Points
	c.u16(uint16(p.Fields))
	c.float64s(p.X)
	c.float64s(p.Y)
	c.float64s(p.Z)
	c.int64s(p.TimeOffsetNanos)
	c.bytes(p.Intensity)
	c.uint16s(p.Channel)
	c.bytes(p.ReturnIndex)
	c.uint32s(p.SourceOrdinal)
	c.uint32s(p.PacketSequence)
	c.uint16s(p.BlockIndex)
	c.u64(uint64(len(f.Clusters)))
	for _, cluster := range f.Clusters {
		c.i64(cluster.ClusterID)
		c.uint32s(cluster.Members)
		s := cluster.Summary
		c.i64(s.FirstMemberUnixNanos)
		for _, v := range [...]float32{s.CentroidX, s.CentroidY, s.CentroidZ, s.BoundingBoxLength,
			s.BoundingBoxWidth, s.BoundingBoxHeight, s.HeightP95, s.IntensityMean} {
			c.f32(v)
		}
		c.i64(int64(s.PointsCount))
		c.boolean(s.GroundClipped)
		c.boolean(s.OBB != nil)
		if o := s.OBB; o != nil {
			for _, v := range [...]float32{o.CenterX, o.CenterY, o.CenterZ, o.Length, o.Width, o.Height, o.HeadingRad} {
				c.f32(v)
			}
		}
	}
	c.uint32s(f.Unassigned)
	c.u64(uint64(len(f.UnassignedReasons)))
	for _, r := range f.UnassignedReasons {
		c.u8(uint8(r))
	}
	return c.sum()
}

// GapDigest returns the semantic digest of a gap record.
func GapDigest(g GapRecord) Digest {
	c := newCanonical("gap")
	c.boolean(g.HasSequenceRange)
	if g.HasSequenceRange {
		c.u64(g.FirstSequence)
		c.u64(g.LastSequence)
	}
	c.count(g.MissingFrames)
	c.u8(uint8(g.Time))
	if g.Time != GapTimeUnknown {
		c.i64(g.StartUnixNanos)
		c.i64(g.EndUnixNanos)
	}
	c.str(g.Cause)
	return c.sum()
}

// Record kinds folded into a StreamDigest. They tag each digest so a frame
// and a gap can never be confused, however their bytes compare.
const (
	streamFrame byte = 1
	streamGap   byte = 2
)

// StreamDigest folds record digests in stream order. Two streams with equal
// digests hold the same records, in the same order, value for value; it is
// how an entire extraction is compared across runs, codecs or chunkings.
type StreamDigest struct{ h hash.Hash }

// NewStreamDigest starts an empty stream.
func NewStreamDigest() *StreamDigest {
	c := newCanonical("stream")
	c.flush()
	return &StreamDigest{h: c.h}
}

// AddFrame folds the next frame's digest.
func (s *StreamDigest) AddFrame(d Digest) { s.add(streamFrame, d) }

// AddGap folds the next gap's digest.
func (s *StreamDigest) AddGap(d Digest) { s.add(streamGap, d) }

func (s *StreamDigest) add(kind byte, d Digest) {
	_, _ = s.h.Write([]byte{kind})
	_, _ = s.h.Write(d[:])
}

// Sum returns the digest of the records added so far; adding may continue.
func (s *StreamDigest) Sum() Digest {
	var d Digest
	s.h.Sum(d[:0])
	return d
}

// canonical writes the version-1 byte stream through a small buffer, so the
// point columns are hashed in bulk rather than one Write per value.
type canonical struct {
	h   hash.Hash
	buf []byte
}

func newCanonical(domain string) *canonical {
	c := &canonical{h: sha256.New(), buf: make([]byte, 0, 8192)}
	c.str(SemanticDigestVersion)
	c.str(domain)
	return c
}

func (c *canonical) reserve(n int) []byte {
	if len(c.buf)+n > cap(c.buf) {
		c.flush()
	}
	start := len(c.buf)
	c.buf = c.buf[:start+n]
	return c.buf[start:]
}

func (c *canonical) flush() {
	_, _ = c.h.Write(c.buf)
	c.buf = c.buf[:0]
}

func (c *canonical) sum() Digest {
	c.flush()
	var d Digest
	c.h.Sum(d[:0])
	return d
}

func (c *canonical) u8(v uint8)   { c.reserve(1)[0] = v }
func (c *canonical) u16(v uint16) { binary.LittleEndian.PutUint16(c.reserve(2), v) }
func (c *canonical) u32(v uint32) { binary.LittleEndian.PutUint32(c.reserve(4), v) }
func (c *canonical) u64(v uint64) { binary.LittleEndian.PutUint64(c.reserve(8), v) }
func (c *canonical) i64(v int64)  { c.u64(uint64(v)) }
func (c *canonical) f32(v float32) {
	c.u32(math.Float32bits(v))
}

func (c *canonical) boolean(v bool) {
	if v {
		c.u8(1)
		return
	}
	c.u8(0)
}

// count hashes presence, then the value only when it is known.
func (c *canonical) count(v Count) {
	c.boolean(v.Known)
	if v.Known {
		c.u64(v.Value)
	}
}

func (c *canonical) str(s string) {
	c.u64(uint64(len(s)))
	c.flush()
	_, _ = c.h.Write([]byte(s))
}

func (c *canonical) bytes(b []byte) {
	c.u64(uint64(len(b)))
	c.flush()
	_, _ = c.h.Write(b)
}

func (c *canonical) float64s(values []float64) {
	c.u64(uint64(len(values)))
	for _, v := range values {
		c.u64(math.Float64bits(v))
	}
}

func (c *canonical) int64s(values []int64) {
	c.u64(uint64(len(values)))
	for _, v := range values {
		c.i64(v)
	}
}

func (c *canonical) uint32s(values []uint32) {
	c.u64(uint64(len(values)))
	for _, v := range values {
		c.u32(v)
	}
}

func (c *canonical) uint16s(values []uint16) {
	c.u64(uint64(len(values)))
	for _, v := range values {
		c.u16(v)
	}
}
