package recorder

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// The filter record: a replay recording's sidecar for retrospective
// refinement.
//
// A FrameBundle track carries the filter's posterior and its covariance, and
// nothing else of the filter. A Rauch–Tung–Striebel pass also needs the
// prior the update was made against and the interval the filter actually
// predicted across, and filter_steps.go explains why those are recorded by
// the tracker rather than recomputed from the posterior: the covariance cap,
// the miss inflation, the velocity clamp and the capture-gap sub-steps all
// shape the prior, and a second implementation of the prediction would drift
// from the first. So a replay that records a VRLOG also records the tracker's
// own filter record beside it, one gzipped NDJSON line per Update after a
// header line, and a smoother at export reads that.
//
// It is a sidecar rather than a field of the frame so the VRLOG's protobuf
// chunks, its seek index and every reader of them stay as they are, and so a
// recording without one (anything recorded before this file existed, and
// every live recording, since the live pipeline attaches no observer) reads
// exactly as before. A step's solid-body reading is not carried: it is the
// online body's beliefs, which the published trail does not revise.

// FilterRecordFile is the sidecar's name inside a VRLOG directory.
const FilterRecordFile = "filter_record.ndjson.gz"

// filterRecordVersion is bumped when the line format changes in a way an
// existing reader cannot read.
const filterRecordVersion = 1

// FilterRecordHeader is the sidecar's first line.
type FilterRecordHeader struct {
	Version  int    `json:"version"`
	SensorID string `json:"sensor_id,omitempty"`
	// EstimatorID names the online estimator whose record this is, as the
	// persisted estimates name it (cv_kf_v1), so a refined trail can say
	// which filter it refines.
	EstimatorID string `json:"estimator_id"`
}

// FilterRecordWriter records the tracker's filter record to a VRLOG
// directory's sidecar. It implements l5tracks.FilterStepObserver, so it is
// attached with SetFilterStepObserver and called under the tracker's lock:
// each frame is encoded and buffered there, and the first write error is
// kept and returned by Close.
type FilterRecordWriter struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	gz     *gzip.Writer
	buf    *bufio.Writer
	enc    *json.Encoder
	err    error
	closed bool
	frames int64
	steps  int64
}

// NewFilterRecordWriter opens the sidecar in basePath, creating the directory
// if the recorder has not yet, and writes the header line.
func NewFilterRecordWriter(basePath string, header FilterRecordHeader) (*FilterRecordWriter, error) {
	if header.EstimatorID == "" {
		return nil, errors.New("filter record: the header must name the online estimator")
	}
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("filter record: %w", err)
	}
	header.Version = filterRecordVersion
	path := filepath.Join(basePath, FilterRecordFile)
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("filter record: %w", err)
	}
	buf := bufio.NewWriterSize(file, 1<<20)
	gz := gzip.NewWriter(buf)
	w := &FilterRecordWriter{path: path, file: file, gz: gz, buf: buf, enc: json.NewEncoder(gz)}
	if err := w.enc.Encode(header); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("filter record: write header: %w", err)
	}
	return w, nil
}

// Path is the sidecar's file path.
func (w *FilterRecordWriter) Path() string { return w.path }

// ObserveFilterFrame implements l5tracks.FilterStepObserver.
func (w *FilterRecordWriter) ObserveFilterFrame(frame l5tracks.FilterFrame) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.err != nil {
		return
	}
	if err := w.enc.Encode(encodeFilterFrame(frame)); err != nil {
		w.err = fmt.Errorf("filter record: write frame at %d: %w", frame.FrameUnixNanos, err)
		return
	}
	w.frames++
	w.steps += int64(len(frame.Steps))
}

// Counts reports the frames and steps written so far.
func (w *FilterRecordWriter) Counts() (frames, steps int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.frames, w.steps
}

// Close flushes and closes the sidecar, returning the first error of the
// recording. Closing twice is a no-op.
func (w *FilterRecordWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.err
	}
	w.closed = true
	if err := w.gz.Close(); err != nil && w.err == nil {
		w.err = fmt.Errorf("filter record: %w", err)
	}
	if err := w.buf.Flush(); err != nil && w.err == nil {
		w.err = fmt.Errorf("filter record: %w", err)
	}
	if err := w.file.Close(); err != nil && w.err == nil {
		w.err = fmt.Errorf("filter record: %w", err)
	}
	return w.err
}

// HasFilterRecord reports whether a VRLOG directory carries a filter record.
func HasFilterRecord(basePath string) bool {
	info, err := os.Stat(filepath.Join(basePath, FilterRecordFile))
	return err == nil && !info.IsDir()
}

// ReadFilterRecord reads a VRLOG directory's filter record, handing each
// frame to visit in recorded order, and returns the header. A visit error
// stops the read and is returned.
func ReadFilterRecord(basePath string, visit func(l5tracks.FilterFrame) error) (FilterRecordHeader, error) {
	var header FilterRecordHeader
	file, err := os.Open(filepath.Join(basePath, FilterRecordFile))
	if err != nil {
		return header, fmt.Errorf("filter record: %w", err)
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(bufio.NewReaderSize(file, 1<<20))
	if err != nil {
		return header, fmt.Errorf("filter record: %w", err)
	}
	defer func() { _ = gz.Close() }()
	dec := json.NewDecoder(gz)
	if err := dec.Decode(&header); err != nil {
		return header, fmt.Errorf("filter record: read header: %w", err)
	}
	if header.Version != filterRecordVersion {
		return header, fmt.Errorf("filter record: version %d, this reader knows %d", header.Version, filterRecordVersion)
	}
	if header.EstimatorID == "" {
		return header, errors.New("filter record: the header names no estimator")
	}
	for line := int64(2); ; line++ {
		var rec filterFrameRecord
		if err := dec.Decode(&rec); err != nil {
			if errors.Is(err, io.EOF) {
				return header, nil
			}
			return header, fmt.Errorf("filter record: line %d: %w", line, err)
		}
		if err := visit(decodeFilterFrame(rec)); err != nil {
			return header, err
		}
	}
}

// --- wire format ------------------------------------------------------------
//
// Keys are short on purpose: a forty-minute replay records a step for every
// live track on every rotation. Numbers are float32 as the filter stored
// them, written at their shortest exact decimal, so a read-back is bit for
// bit the record the tracker made.

type filterFrameRecord struct {
	FrameUnixNanos int64                  `json:"t"`
	Steps          []filterStepRecord     `json:"s,omitempty"`
	Ended          []filterChainEndRecord `json:"e,omitempty"`
}

// filterMomentsRecord is x, y, vx, vy followed by the row-major 4×4 P.
type filterMomentsRecord [20]float32

type filterStepRecord struct {
	TrackID          string  `json:"id"`
	CreationSequence int64   `json:"seq"`
	FrameUnixNanos   int64   `json:"f"`
	StateUnixNanos   int64   `json:"st"`
	PredictedSecs    float32 `json:"dt,omitempty"`
	Linked           bool    `json:"ln,omitempty"`
	Observed         bool    `json:"ob,omitempty"`
	UpdateRefused    bool    `json:"ur,omitempty"`
	Confirmed        bool    `json:"cf,omitempty"`
	// Prior is omitted when it is the posterior: an unobserved step, or the
	// founding one, stores the same numbers for both (filter_steps.go).
	Prior       *filterMomentsRecord     `json:"pr,omitempty"`
	Posterior   filterMomentsRecord      `json:"po"`
	Observation *filterObservationRecord `json:"o,omitempty"`
	Reference   uint8                    `json:"ref,omitempty"`
	Support     uint8                    `json:"sup,omitempty"`
}

type filterObservationRecord struct {
	ClusterID            int64      `json:"c"`
	MeasurementUnixNanos int64      `json:"mt,omitempty"`
	Source               string     `json:"src,omitempty"`
	X                    float32    `json:"x"`
	Y                    float32    `json:"y"`
	InnovationX          float32    `json:"ix,omitempty"`
	InnovationY          float32    `json:"iy,omitempty"`
	NIS                  float32    `json:"nis,omitempty"`
	HasInnovation        bool       `json:"hi,omitempty"`
	GeometryCovariance   [3]float32 `json:"g,omitempty"`
	Disposition          string     `json:"d,omitempty"`
	Reason               string     `json:"r,omitempty"`
}

type filterChainEndRecord struct {
	TrackID          string `json:"id"`
	CreationSequence int64  `json:"seq"`
	FrameUnixNanos   int64  `json:"f"`
	Reason           string `json:"why"`
}

func encodeMoments(m l5tracks.FilterMoments) filterMomentsRecord {
	var out filterMomentsRecord
	out[0], out[1], out[2], out[3] = m.X, m.Y, m.VX, m.VY
	copy(out[4:], m.P[:])
	return out
}

func decodeMoments(r filterMomentsRecord) l5tracks.FilterMoments {
	m := l5tracks.FilterMoments{X: r[0], Y: r[1], VX: r[2], VY: r[3]}
	copy(m.P[:], r[4:])
	return m
}

func encodeFilterFrame(frame l5tracks.FilterFrame) filterFrameRecord {
	out := filterFrameRecord{FrameUnixNanos: frame.FrameUnixNanos}
	for _, step := range frame.Steps {
		rec := filterStepRecord{
			TrackID: step.TrackID, CreationSequence: step.CreationSequence,
			FrameUnixNanos: step.FrameUnixNanos, StateUnixNanos: step.StateUnixNanos,
			PredictedSecs: step.PredictedSecs, Linked: step.Linked, Observed: step.Observed,
			UpdateRefused: step.UpdateRefused, Confirmed: step.Confirmed,
			Posterior: encodeMoments(step.Posterior),
			Reference: uint8(step.Reference), Support: uint8(step.Support),
		}
		if step.Prior != step.Posterior {
			prior := encodeMoments(step.Prior)
			rec.Prior = &prior
		}
		if step.Observed || step.Observation != (l5tracks.FilterObservation{}) {
			o := step.Observation
			rec.Observation = &filterObservationRecord{
				ClusterID: o.ClusterID, MeasurementUnixNanos: o.MeasurementUnixNanos, Source: string(o.Source),
				X: o.X, Y: o.Y, InnovationX: o.InnovationX, InnovationY: o.InnovationY, NIS: o.NIS,
				HasInnovation:      o.HasInnovation,
				GeometryCovariance: [3]float32{o.GeometryCovariance.XX, o.GeometryCovariance.XY, o.GeometryCovariance.YY},
				Disposition:        o.Disposition, Reason: o.Reason,
			}
		}
		out.Steps = append(out.Steps, rec)
	}
	for _, end := range frame.Ended {
		out.Ended = append(out.Ended, filterChainEndRecord{
			TrackID: end.TrackID, CreationSequence: end.CreationSequence,
			FrameUnixNanos: end.FrameUnixNanos, Reason: string(end.Reason),
		})
	}
	return out
}

func decodeFilterFrame(rec filterFrameRecord) l5tracks.FilterFrame {
	frame := l5tracks.FilterFrame{FrameUnixNanos: rec.FrameUnixNanos}
	for _, s := range rec.Steps {
		step := l5tracks.FilterStep{
			TrackID: s.TrackID, CreationSequence: s.CreationSequence,
			FrameUnixNanos: s.FrameUnixNanos, StateUnixNanos: s.StateUnixNanos,
			PredictedSecs: s.PredictedSecs, Linked: s.Linked, Observed: s.Observed,
			UpdateRefused: s.UpdateRefused, Confirmed: s.Confirmed,
			Posterior: decodeMoments(s.Posterior),
			Reference: l5tracks.ReferencePoint(s.Reference), Support: l5tracks.ObservationSupport(s.Support),
		}
		step.Prior = step.Posterior
		if s.Prior != nil {
			step.Prior = decodeMoments(*s.Prior)
		}
		if o := s.Observation; o != nil {
			step.Observation = l5tracks.FilterObservation{
				ClusterID: o.ClusterID, MeasurementUnixNanos: o.MeasurementUnixNanos,
				Source: l5tracks.MeasurementSource(o.Source), X: o.X, Y: o.Y,
				InnovationX: o.InnovationX, InnovationY: o.InnovationY, NIS: o.NIS, HasInnovation: o.HasInnovation,
				GeometryCovariance: l5tracks.MeasurementCovariance{XX: o.GeometryCovariance[0], XY: o.GeometryCovariance[1], YY: o.GeometryCovariance[2]},
				Disposition:        o.Disposition, Reason: o.Reason,
			}
		}
		frame.Steps = append(frame.Steps, step)
	}
	for _, e := range rec.Ended {
		frame.Ended = append(frame.Ended, l5tracks.FilterChainEnd{
			TrackID: e.TrackID, CreationSequence: e.CreationSequence,
			FrameUnixNanos: e.FrameUnixNanos, Reason: l5tracks.ChainEndReason(e.Reason),
		})
	}
	return frame
}
