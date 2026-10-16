package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// A cell is one beam (ring = channel - 1) and one azimuth bin, the same
// indexing the L3 background grid uses (l3grid returnCell): ring*azBins + bin,
// with the bin taken from the decoder's corrected azimuth.
const (
	numRings  = parse.CHANNELS_PER_BLOCK
	numAzBins = 1800 // 0.2 degrees, the background grid's bin
	numCells  = numRings * numAzBins
)

// geometry is the ray direction of every cell in the sensor frame: the bin
// centre's azimuth and the channel's calibrated elevation, through
// l2frames.SphericalToCartesian, the conversion the pipeline itself uses.
type geometry struct {
	elevationDeg [numRings]float64
	dirs         []vec3
}

func newGeometry(elevations []float64) (*geometry, error) {
	if len(elevations) != numRings {
		return nil, fmt.Errorf("need %d channel elevations, have %d", numRings, len(elevations))
	}
	g := &geometry{dirs: make([]vec3, numCells)}
	copy(g.elevationDeg[:], elevations)
	for ring := 0; ring < numRings; ring++ {
		for bin := 0; bin < numAzBins; bin++ {
			x, y, z := l2frames.SphericalToCartesian(1, binCentreDeg(bin), elevations[ring])
			g.dirs[ring*numAzBins+bin] = vec3{x, y, z}
		}
	}
	return g, nil
}

func binCentreDeg(bin int) float64 { return (float64(bin) + 0.5) * 360 / numAzBins }

// cellOf is the cell a return falls in, as l3grid's returnCell computes it.
func cellOf(channel int, azimuthDeg float64) (int, bool) {
	ring := channel - 1
	if ring < 0 || ring >= numRings {
		return 0, false
	}
	az := math.Mod(azimuthDeg, 360)
	if az < 0 {
		az += 360
	}
	bin := int(az / 360 * numAzBins)
	if bin >= numAzBins {
		bin = numAzBins - 1
	}
	return ring*numAzBins + bin, true
}

// staticParams decides which cells of a window count as static background.
type staticParams struct {
	// MinCoverage is the share of the window's rotations a cell must return
	// in at least once.
	MinCoverage float64
	// MaxIQR and MaxIQRFraction bound the interquartile range of a cell's
	// ranges: max(MaxIQR, MaxIQRFraction*median) metres.
	MaxIQR, MaxIQRFraction float64
	// MinRange excludes cells nearer than this: anything rigidly attached to
	// the sensor (mount, tripod) stays put in the sensor frame whatever the
	// sensor does, and would pull a pose fit towards zero.
	MinRange float64
	// MaxRange excludes far, sparse returns.
	MaxRange float64
}

// cellStat summarises one cell over one window.
type cellStat struct {
	Median   float64
	IQR      float64
	Coverage float64
	Samples  int
	Static   bool
}

// windowAccum collects every return of a window by cell. It applies no
// background model: each cell's ranges are kept until the window closes and
// then summarised by median and interquartile range.
type windowAccum struct {
	samples [][]float32
	lastRot []int32
	rotSeen []uint16
	rot     int32
	prevAz  int
	started bool

	packets     int
	points      int
	firstNanos  int64
	lastNanos   int64
	returnModes map[uint8]int
	motorRPM    []float64

	// series, when set, compares each return with a reference window's
	// static median as it arrives, one entry per rotation. A series keeps no
	// per-cell samples and no motor speeds: only the open rotation's |Δr|
	// (seriesAbs, reused from one rotation to the next) and a fixed-size row
	// for each rotation closed, so its memory does not grow with the span's
	// returns.
	seriesRef  []cellStat
	seriesTau  float64
	series     []rotationSample
	seriesAbs  []float64
	seriesOpen bool
}

// seriesSectors splits each series rotation into 30-degree sectors. The head
// sweeps azimuth in time, so a sector is about 8 ms of a rotation at 10 Hz and
// the sector at which the departure starts places an onset within it.
const seriesSectors = 12

// rotationSample is one rotation's raw departure from a reference window.
type rotationSample struct {
	StartNanos    int64
	Returns       int
	Changed       int
	SectorReturns [seriesSectors]int
	SectorChanged [seriesSectors]int
	// MedianAbsM is the median |Δr| of the rotation's returns on static
	// reference cells, set when the rotation closes; NaN when it had none.
	MedianAbsM float64
}

func newWindowAccum() *windowAccum {
	return &windowAccum{
		samples:     make([][]float32, numCells),
		lastRot:     make([]int32, numCells),
		rotSeen:     make([]uint16, numCells),
		returnModes: map[uint8]int{},
	}
}

// addPacket decodes one Pandar40P UDP payload with the pipeline's own parser
// and files every return under its cell.
func (w *windowAccum) addPacket(parser *parse.Pandar40PParser, unixNanos int64, payload []byte) error {
	parser.SetPacketTime(timeFromNanos(unixNanos))
	points, err := parser.ParsePacket(payload)
	if err != nil {
		return err
	}
	if w.packets == 0 {
		w.firstNanos = unixNanos
	}
	w.packets++
	w.lastNanos = unixNanos
	if len(payload) >= parse.TAIL_START+parse.TAIL_SIZE {
		w.returnModes[payload[parse.TAIL_START+14]]++
	}
	if w.seriesRef == nil {
		w.motorRPM = append(w.motorRPM, float64(parser.GetLastMotorSpeed()))
	}
	for i := range points {
		w.addPoint(points[i])
	}
	return nil
}

func (w *windowAccum) addPoint(p l2frames.PointPolar) {
	az := int(p.RawBlockAzimuth)
	if !w.started {
		w.started = true
		w.prevAz = az
		w.startRotation(p.Timestamp)
	} else if az < w.prevAz-parse.ROTATION_MAX_UNITS/2 {
		w.rot++
		w.startRotation(p.Timestamp)
	}
	w.prevAz = az
	cell, ok := cellOf(p.Channel, p.Azimuth)
	if !ok || p.Distance <= 0 {
		return
	}
	w.points++
	if w.seriesRef != nil {
		w.addSeriesReturn(cell, p.Distance)
		return
	}
	w.samples[cell] = append(w.samples[cell], float32(p.Distance))
	if w.rotSeen[cell] == 0 || w.lastRot[cell] != w.rot {
		w.rotSeen[cell]++
		w.lastRot[cell] = w.rot
	}
}

// addSeriesReturn counts one return against the open rotation when its cell
// is static in the reference.
func (w *windowAccum) addSeriesReturn(cell int, distance float64) {
	ref := w.seriesRef[cell]
	if !ref.Static {
		return
	}
	s := &w.series[len(w.series)-1]
	d := math.Abs(distance - ref.Median)
	sector := (cell % numAzBins) * seriesSectors / numAzBins
	s.Returns++
	s.SectorReturns[sector]++
	if d > w.seriesTau {
		s.Changed++
		s.SectorChanged[sector]++
	}
	w.seriesAbs = append(w.seriesAbs, d)
}

func (w *windowAccum) startRotation(unixNanos int64) {
	if w.seriesRef != nil {
		w.closeRotation()
		w.series = append(w.series, rotationSample{StartNanos: unixNanos})
		w.seriesOpen = true
	}
}

// closeRotation sets the open rotation's median |Δr| and empties the buffer
// for the next. Closing when no rotation is open does nothing.
func (w *windowAccum) closeRotation() {
	if !w.seriesOpen {
		return
	}
	w.series[len(w.series)-1].MedianAbsM = median(w.seriesAbs)
	w.seriesAbs = w.seriesAbs[:0]
	w.seriesOpen = false
}

// rotations is the number of rotations the window touched, partial ones at
// either end included.
func (w *windowAccum) rotations() int {
	if !w.started {
		return 0
	}
	return int(w.rot) + 1
}

// finish summarises every cell and releases the raw samples.
func (w *windowAccum) finish(p staticParams) []cellStat {
	out := make([]cellStat, numCells)
	rot := w.rotations()
	buf := make([]float64, 0, 1024)
	for c := range w.samples {
		s := w.samples[c]
		if len(s) == 0 {
			continue
		}
		buf = buf[:0]
		for _, v := range s {
			buf = append(buf, float64(v))
		}
		slices.Sort(buf)
		st := cellStat{
			Median:   sortedQuantile(buf, 0.5),
			IQR:      sortedQuantile(buf, 0.75) - sortedQuantile(buf, 0.25),
			Coverage: float64(w.rotSeen[c]) / float64(rot),
			Samples:  len(s),
		}
		st.Static = len(s) >= 3 && st.Coverage >= p.MinCoverage &&
			st.Median >= p.MinRange && (p.MaxRange <= 0 || st.Median <= p.MaxRange) &&
			st.IQR <= math.Max(p.MaxIQR, p.MaxIQRFraction*st.Median)
		out[c] = st
		w.samples[c] = nil
	}
	return out
}

func sortedQuantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo] + (pos-float64(lo))*(sorted[hi]-sorted[lo])
}

func countStatic(cells []cellStat) (seen, static int) {
	for _, c := range cells {
		if c.Samples > 0 {
			seen++
		}
		if c.Static {
			static++
		}
	}
	return seen, static
}
