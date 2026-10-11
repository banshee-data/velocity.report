package l3grid

import "math"

// SupportFilterParams configures the range-image support filter (the
// beam-wise review's E4): after the per-point decision, a cell's foreground
// is kept only when enough neighbouring cells in the ring-by-azimuth index
// also hold foreground at a similar range. It is the "local angular support,
// range similarity" filter of Baumann, Vosshans and Dang (2026) on this
// grid: the window is in index space, the range tolerance grows with range,
// and the filter only ever turns foreground into background. It never
// touches a cell's background model, so a filtered return still counted as a
// rejection when its cell was updated.
//
// The ring window is in channel order, which on the Pandar40P is elevation
// order (channel 1 at +15.21 degrees down to channel 40), so VertNeighbours
// rings either side is a contiguous band of beams whatever their spacing.
type SupportFilterParams struct {
	// VertNeighbours is how many rings either side of a cell are searched.
	VertNeighbours int
	// HorNeighbours is how many azimuth bins either side are searched; the
	// search wraps at 360 degrees.
	HorNeighbours int
	// MinSupport is the number of foreground neighbour cells within the
	// range tolerance a cell needs to keep its foreground.
	MinSupport int
	// TauBaseMetres and TauSlope set the range-similarity tolerance,
	// tau = TauBaseMetres + TauSlope * range, between a cell's nearest
	// foreground return and a neighbour's.
	TauBaseMetres float64
	TauSlope      float64
}

// DefaultSupportFilterParams is the first setting used on the Pandar40P: a
// window of 5 rings by 21 bins (4.2 degrees of azimuth at 0.2-degree bins),
// four supporting cells, and the paper's example tolerance. It is a starting
// point for the replay experiment, not a tuned value.
func DefaultSupportFilterParams() SupportFilterParams {
	return SupportFilterParams{
		VertNeighbours: 2,
		HorNeighbours:  10,
		MinSupport:     4,
		TauBaseMetres:  0.65,
		TauSlope:       0.005,
	}
}

// supportFilterScratch holds the per-frame buffers the filter reuses, so a
// frame costs no allocation once the buffers have grown to the grid and the
// point count. Cells are visited through a frame stamp rather than cleared.
type supportFilterScratch struct {
	frame uint32
	stamp []uint32 // per cell: the frame that last wrote it
	rng   []float32
	head  []int32 // per cell: first foreground point index this frame
	next  []int32 // per point: next foreground point in the same cell
	cells []int32 // foreground cells touched this frame
}

func (s *supportFilterScratch) prepare(cellCount, pointCount int) {
	if len(s.stamp) != cellCount {
		s.stamp = make([]uint32, cellCount)
		s.rng = make([]float32, cellCount)
		s.head = make([]int32, cellCount)
		s.frame = 0
	}
	if cap(s.next) < pointCount {
		s.next = make([]int32, pointCount)
	}
	s.next = s.next[:pointCount]
	s.cells = s.cells[:0]
	s.frame++
	if s.frame == 0 { // wrapped: every stamp is stale, start again
		for i := range s.stamp {
			s.stamp[i] = 0
		}
		s.frame = 1
	}
}

// applySupportFilter flips the foreground points of unsupported cells to
// background and returns how many points it flipped. mask is true for
// foreground. The cell of each point is derived exactly as the decision
// derives it, so the filter sees the grid the decision used.
func (g *BackgroundGrid) applySupportFilter(p SupportFilterParams, s *supportFilterScratch, points []PointPolar, mask []bool) int {
	rings, azBins := g.Rings, g.AzimuthBins
	if rings <= 0 || azBins <= 0 || len(points) == 0 || len(mask) != len(points) {
		return 0
	}
	s.prepare(rings*azBins, len(points))

	for i, pt := range points {
		if !mask[i] {
			continue
		}
		ring := pt.Channel - 1
		if ring < 0 || ring >= rings {
			continue
		}
		az := math.Mod(pt.Azimuth, 360.0)
		if az < 0 {
			az += 360.0
		}
		cell := g.Idx(ring, int((az/360.0)*float64(azBins)))
		r := float32(pt.Distance)
		if s.stamp[cell] != s.frame {
			s.stamp[cell] = s.frame
			s.rng[cell] = r
			s.head[cell] = int32(i)
			s.next[i] = -1
			s.cells = append(s.cells, int32(cell))
			continue
		}
		if r < s.rng[cell] {
			s.rng[cell] = r
		}
		s.next[i] = s.head[cell]
		s.head[cell] = int32(i)
	}

	flipped := 0
	for _, c := range s.cells {
		cell := int(c)
		ring, az := cell/azBins, cell%azBins
		r := s.rng[cell]
		tau := float32(p.TauBaseMetres + p.TauSlope*float64(r))
		support := 0
		for dv := -p.VertNeighbours; dv <= p.VertNeighbours && support < p.MinSupport; dv++ {
			nr := ring + dv
			if nr < 0 || nr >= rings {
				continue
			}
			for dh := -p.HorNeighbours; dh <= p.HorNeighbours; dh++ {
				if dv == 0 && dh == 0 {
					continue
				}
				na := ((az+dh)%azBins + azBins) % azBins
				n := g.Idx(nr, na)
				if s.stamp[n] != s.frame {
					continue
				}
				d := s.rng[n] - r
				if d < 0 {
					d = -d
				}
				if d <= tau {
					support++
					if support >= p.MinSupport {
						break
					}
				}
			}
		}
		if support >= p.MinSupport {
			continue
		}
		for i := s.head[cell]; i >= 0; i = s.next[i] {
			mask[i] = false
			flipped++
		}
	}
	return flipped
}
