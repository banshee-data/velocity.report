package pipeline

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// The pipeline's half of L3's track guard (l3grid.TrackGuardParams): it
// hands L3 the confirmed tracks' boxes after each tracker update, for the
// frames that follow, and gives the published mask its tracked objects'
// edges. L3 cannot import L5, so the boxes cross as l3grid.GuardBox values.

// guardBox is a track's published box as the guard reads it, carried by the
// track's velocity.
func guardBox(t *l5tracks.TrackedObject) (l3grid.GuardBox, bool) {
	b := t.PublishedBox()
	if !(b.Length > 0 && b.Width > 0 && b.Height > 0) {
		return l3grid.GuardBox{}, false
	}
	return l3grid.GuardBox{
		X: float64(b.X), Y: float64(b.Y), Z: float64(b.Z),
		Length: float64(b.Length), Width: float64(b.Width), Height: float64(b.Height),
		HeadingRad: float64(b.HeadingRad),
		VX:         float64(t.VX), VY: float64(t.VY),
	}, true
}

// holdBoxes are the boxes of every confirmed track moving at minSpeed or
// more, coasting ones included: a coasting track's object is still there,
// hidden or missed.
func holdBoxes(tracks []*l5tracks.TrackedObject, minSpeed float64) []l3grid.GuardBox {
	boxes := make([]l3grid.GuardBox, 0, len(tracks))
	for _, t := range tracks {
		if float64(t.Speed()) < minSpeed {
			continue
		}
		if b, ok := guardBox(t); ok {
			boxes = append(boxes, b)
		}
	}
	return boxes
}

// edgeBoxes are the boxes whose returns are published with their object's
// class this frame: each confirmed track observed in the frame, and each
// cluster the tracker associated with one of them. A cluster's box holds the
// returns the track's box may not yet reach.
func edgeBoxes(tracks []*l5tracks.TrackedObject, clusters []l4perception.WorldCluster, associations []string) []l3grid.GuardBox {
	observed := make(map[string]bool, len(tracks))
	boxes := make([]l3grid.GuardBox, 0, 2*len(tracks))
	for _, t := range tracks {
		if t.Misses != 0 {
			continue
		}
		observed[t.TrackID] = true
		if b, ok := guardBox(t); ok {
			boxes = append(boxes, b)
		}
	}
	for i := range clusters {
		if i >= len(associations) || !observed[associations[i]] || clusters[i].OBB == nil {
			continue
		}
		o := clusters[i].OBB
		if !(o.Length > 0 && o.Width > 0 && o.Height > 0) {
			continue
		}
		// The OBB's CenterZ is its lowest point, not its centre.
		boxes = append(boxes, l3grid.GuardBox{
			X: float64(o.CenterX), Y: float64(o.CenterY), Z: float64(o.CenterZ + o.Height/2),
			Length: float64(o.Length), Width: float64(o.Width), Height: float64(o.Height),
			HeadingRad: float64(o.HeadingRad),
		})
	}
	return boxes
}

// sameBacking reports whether two masks share their first element, which is
// how the pipeline tells L3's own mask from the published copy.
func sameBacking(a, b []bool) bool {
	return len(a) > 0 && len(b) > 0 && &a[0] == &b[0]
}
