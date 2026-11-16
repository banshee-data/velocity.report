// Package motionexport writes a motion segment of a capture as sensor-frame
// point files with per-point time for offline odometry (the route SLAM
// spike's S0 step): uncompressed LAS 1.4, point format 6, with ring, azimuth
// and frame index as extra bytes, plus a manifest that states the time
// convention and the provenance of every file.
//
// The points stay in the sensor frame before any motion correction, so a
// later run can repeat the de-skew. No pose is applied here.
package motionexport

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"
)

const (
	lasHeaderSize   = 375
	vlrHeaderSize   = 54
	extraDescSize   = 192
	pdrf6Size       = 30
	extraBytesSize  = 1 + 4 + 4 // ring u8, azimuth f32, frame u32
	lasRecordLength = pdrf6Size + extraBytesSize
	lasScale        = 0.0005 // 0.5 mm grid: ±1 km range in int32
)

// LASPoint is one return in the sensor frame.
type LASPoint struct {
	X, Y, Z    float64
	Intensity  uint8
	Ring       uint8   // 1-40
	AzimuthDeg float32 // corrected azimuth, degrees
	Frame      uint32  // index of the rotation the return belongs to
	TimeS      float64 // seconds since the manifest's epoch
}

// LASWriter streams points into a LAS 1.4 file and fixes the header's counts
// and bounds on Close.
type LASWriter struct {
	f        *os.File
	buf      []byte
	n        uint64
	min, max [3]float64
	tMin     float64
	tMax     float64
	created  time.Time
	software string
}

// CreateLAS creates a new file; it refuses to overwrite an existing one.
func CreateLAS(path, software string, created time.Time) (*LASWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	w := &LASWriter{f: f, buf: make([]byte, 0, 1<<20), created: created.UTC(), software: software,
		min: [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}, max: [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)},
		tMin: math.Inf(1), tMax: math.Inf(-1)}
	hdr := make([]byte, lasHeaderSize+vlrHeaderSize+3*extraDescSize)
	if _, err := f.Write(hdr); err != nil {
		f.Close()
		return nil, err
	}
	return w, nil
}

// Count is the number of points written so far.
func (w *LASWriter) Count() uint64 { return w.n }

// TimeRange is the earliest and latest point time written.
func (w *LASWriter) TimeRange() (float64, float64) { return w.tMin, w.tMax }

// Write appends one point.
func (w *LASWriter) Write(p LASPoint) error {
	c := [3]float64{p.X, p.Y, p.Z}
	var q [3]int32
	for i, v := range c {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v/lasScale) > math.MaxInt32 {
			return fmt.Errorf("coordinate %v outside the LAS grid", v)
		}
		q[i] = int32(math.Round(v / lasScale))
		w.min[i] = math.Min(w.min[i], v)
		w.max[i] = math.Max(w.max[i], v)
	}
	if math.IsNaN(p.TimeS) || math.IsInf(p.TimeS, 0) {
		return errors.New("non-finite point time")
	}
	w.tMin, w.tMax = math.Min(w.tMin, p.TimeS), math.Max(w.tMax, p.TimeS)
	var r [lasRecordLength]byte
	le := binary.LittleEndian
	le.PutUint32(r[0:], uint32(q[0]))
	le.PutUint32(r[4:], uint32(q[1]))
	le.PutUint32(r[8:], uint32(q[2]))
	le.PutUint16(r[12:], uint16(p.Intensity)) // raw 0-255 reflectivity, not rescaled
	r[14] = 0x11                              // return 1 of 1
	// bytes 15-19: flags, classification, user data, scan angle stay zero
	le.PutUint16(r[20:], 0) // point source id
	le.PutUint64(r[22:], math.Float64bits(p.TimeS))
	r[30] = p.Ring
	le.PutUint32(r[31:], math.Float32bits(p.AzimuthDeg))
	le.PutUint32(r[35:], p.Frame)
	w.buf = append(w.buf, r[:]...)
	w.n++
	if len(w.buf) >= cap(w.buf)-lasRecordLength {
		return w.flush()
	}
	return nil
}

func (w *LASWriter) flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	_, err := w.f.Write(w.buf)
	w.buf = w.buf[:0]
	return err
}

// Close writes the remaining points and the final header.
func (w *LASWriter) Close() error {
	err := w.flush()
	if err == nil {
		_, err = w.f.WriteAt(w.header(), 0)
	}
	if cerr := w.f.Close(); err == nil {
		err = cerr
	}
	return err
}

func putString(b []byte, s string) { copy(b, s) }

func (w *LASWriter) header() []byte {
	le := binary.LittleEndian
	h := make([]byte, lasHeaderSize+vlrHeaderSize+3*extraDescSize)
	copy(h[0:], "LASF")
	// File source id 0; global encoding bit 4 (WKT CRS) as point format 6
	// requires. Bit 0 is clear and the time field is NOT GPS time: it is
	// seconds since the manifest's epoch on the export's stated clock.
	le.PutUint16(h[6:], 1<<4)
	h[24], h[25] = 1, 4
	putString(h[26:58], "velocity.report motion export")
	putString(h[58:90], w.software)
	le.PutUint16(h[90:], uint16(w.created.YearDay()))
	le.PutUint16(h[92:], uint16(w.created.Year()))
	le.PutUint16(h[94:], lasHeaderSize)
	le.PutUint32(h[96:], uint32(len(h)))
	le.PutUint32(h[100:], 1) // one VLR: the extra bytes descriptors
	h[104] = 6
	le.PutUint16(h[105:], lasRecordLength)
	// Legacy point counts (107-130) stay zero for point format 6.
	for i := 0; i < 3; i++ {
		le.PutUint64(h[131+8*i:], math.Float64bits(lasScale))
	}
	// Offsets (155-178) zero; bounds: max x, min x, max y, min y, max z, min z.
	mn, mx := w.min, w.max
	if w.n == 0 {
		mn, mx = [3]float64{}, [3]float64{}
	}
	for i := 0; i < 3; i++ {
		le.PutUint64(h[179+16*i:], math.Float64bits(mx[i]))
		le.PutUint64(h[187+16*i:], math.Float64bits(mn[i]))
	}
	// Waveform start (227), first EVLR (235), EVLR count (243) zero.
	le.PutUint64(h[247:], w.n)
	le.PutUint64(h[255:], w.n) // all points are first returns
	v := h[lasHeaderSize:]
	putString(v[2:18], "LASF_Spec")
	le.PutUint16(v[18:], 4)
	le.PutUint16(v[20:], 3*extraDescSize)
	putString(v[22:54], "ring, azimuth_deg, frame")
	for i, d := range []struct {
		typ  byte
		name string
		desc string
	}{{1, "ring", "laser channel 1-40"}, {9, "azimuth_deg", "corrected azimuth, degrees"}, {5, "frame", "rotation index in the segment"}} {
		e := v[vlrHeaderSize+i*extraDescSize:]
		e[2] = d.typ
		putString(e[4:36], d.name)
		putString(e[160:192], d.desc)
	}
	return h
}

// LASInfo is what ReadLASInfo recovers from a header; tests and the manifest
// check use it.
type LASInfo struct {
	Major, Minor  byte
	PointFormat   byte
	RecordLength  uint16
	PointOffset   uint32
	Points        uint64
	Min, Max      [3]float64
	ExtraNames    []string
	GlobalEncode  uint16
	HeaderSize    uint16
	NumberOfVLRs  uint32
	ScaleX        float64
	FirstTimeS    float64
	FirstRing     byte
	FirstFrame    uint32
	FirstAzimuth  float32
	FirstPointXYZ [3]float64
}

// ReadLASInfo parses the header, the extra-bytes VLR and the first point.
func ReadLASInfo(r io.ReaderAt) (LASInfo, error) {
	le := binary.LittleEndian
	h := make([]byte, lasHeaderSize)
	if _, err := r.ReadAt(h, 0); err != nil {
		return LASInfo{}, err
	}
	if string(h[:4]) != "LASF" {
		return LASInfo{}, errors.New("not a LAS file")
	}
	in := LASInfo{Major: h[24], Minor: h[25], GlobalEncode: le.Uint16(h[6:]), HeaderSize: le.Uint16(h[94:]),
		PointOffset: le.Uint32(h[96:]), NumberOfVLRs: le.Uint32(h[100:]), PointFormat: h[104],
		RecordLength: le.Uint16(h[105:]), Points: le.Uint64(h[247:]), ScaleX: math.Float64frombits(le.Uint64(h[131:]))}
	for i := 0; i < 3; i++ {
		in.Max[i] = math.Float64frombits(le.Uint64(h[179+16*i:]))
		in.Min[i] = math.Float64frombits(le.Uint64(h[187+16*i:]))
	}
	v := make([]byte, vlrHeaderSize)
	if _, err := r.ReadAt(v, lasHeaderSize); err != nil {
		return in, err
	}
	n := int(le.Uint16(v[20:])) / extraDescSize
	d := make([]byte, n*extraDescSize)
	if _, err := r.ReadAt(d, lasHeaderSize+vlrHeaderSize); err != nil {
		return in, err
	}
	for i := 0; i < n; i++ {
		name := d[i*extraDescSize+4 : i*extraDescSize+36]
		end := 0
		for end < len(name) && name[end] != 0 {
			end++
		}
		in.ExtraNames = append(in.ExtraNames, string(name[:end]))
	}
	if in.Points > 0 {
		p := make([]byte, in.RecordLength)
		if _, err := r.ReadAt(p, int64(in.PointOffset)); err != nil {
			return in, err
		}
		for i := 0; i < 3; i++ {
			in.FirstPointXYZ[i] = float64(int32(le.Uint32(p[4*i:]))) * in.ScaleX
		}
		in.FirstTimeS = math.Float64frombits(le.Uint64(p[22:]))
		in.FirstRing = p[30]
		in.FirstAzimuth = math.Float32frombits(le.Uint32(p[31:]))
		in.FirstFrame = le.Uint32(p[35:])
	}
	return in, nil
}
