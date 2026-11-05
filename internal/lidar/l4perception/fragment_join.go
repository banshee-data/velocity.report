package l4perception

import "math"

// FragmentHint is a recent body's predicted footprint at the current capture
// time. The composition root supplies it; L4 does not import the tracker.
type FragmentHint struct {
	ID   string
	Rect Rect
}

// JoinMotionFragments joins accepted components only when all their processed
// members have one unambiguous motion-supported footprint and a bounded gap.
// It neither recovers noise nor changes the input cap. Membership and labels
// remain coherent with the resulting detections, including through evidence
// taps. A failed shape check preserves the original components.
func JoinMotionFragments(clusters []WorldCluster, trace DBSCANTrace, params DBSCANParams, hints []FragmentHint, maxGap, margin float64) ([]WorldCluster, DBSCANTrace, int) {
	if len(clusters) < 2 || len(hints) == 0 || len(trace.Processed) != len(trace.Labels) || !finitePositive(maxGap) || !(margin >= 0) || math.IsInf(margin, 0) {
		return clusters, trace, 0
	}
	seen := make(map[string]bool)
	for _, h := range hints {
		if h.ID == "" || seen[h.ID] || !validFragmentRect(h.Rect) {
			return clusters, trace, 0
		}
		seen[h.ID] = true
	}
	index := make(map[int]int, len(clusters))
	for i, c := range clusters {
		if c.ClusterID <= 0 || c.ClusterID > int64(len(trace.Processed)) {
			return clusters, trace, 0
		}
		if _, duplicate := index[int(c.ClusterID)]; duplicate {
			return clusters, trace, 0
		}
		index[int(c.ClusterID)] = i
	}
	points := make([][]WorldPoint, len(clusters))
	for i, label := range trace.Labels {
		if label > len(trace.Processed) {
			return clusters, trace, 0
		}
		if ci, ok := index[label]; ok {
			points[ci] = append(points[ci], trace.Processed[i])
		}
	}
	owner := make([]int, len(clusters))
	parent := make([]int, len(clusters))
	for i, c := range clusters {
		owner[i], parent[i] = -1, i
		if len(points[i]) == 0 || len(points[i]) != c.PointsCount {
			continue
		}
		intersections := 0
		for j, h := range hints {
			inside := 0
			for _, p := range points[i] {
				if fragmentContains(h.Rect, p, margin) {
					inside++
				}
			}
			if inside > 0 {
				intersections++
			}
			if inside == len(points[i]) {
				owner[i] = j
			}
		}
		// Even partial membership in a second body is ambiguous. Do not let
		// an extended box swallow the neighbour's returns.
		if intersections != 1 {
			owner[i] = -1
		}
	}
	root := func(i int) int {
		for parent[i] != i {
			i = parent[i]
		}
		return i
	}
	changed := false
	for i := range clusters {
		if owner[i] < 0 {
			continue
		}
		for j := i + 1; j < len(clusters); j++ {
			if owner[j] != owner[i] || root(i) == root(j) {
				continue
			}
			if fragmentsNear(points[i], points[j], maxGap) {
				a, b := root(i), root(j)
				if clusters[a].ClusterID > clusters[b].ClusterID {
					a, b = b, a
				}
				parent[b] = a
				changed = true
			}
		}
	}
	if !changed {
		return clusters, trace, 0
	}
	labels := append([]int(nil), trace.Labels...)
	maxID := 0
	for i, label := range labels {
		if label > maxID {
			maxID = label
		}
		if ci, ok := index[label]; ok {
			labels[i] = int(clusters[root(ci)].ClusterID)
		}
	}
	trial := buildClusters(trace.Processed, labels, maxID, params)
	accepted := make(map[int64]bool, len(trial))
	for _, c := range trial {
		accepted[c.ClusterID] = true
	}
	joined := 0
	for i, label := range trace.Labels {
		ci, ok := index[label]
		if !ok {
			continue
		}
		r := root(ci)
		if !accepted[clusters[r].ClusterID] {
			labels[i] = label
		}
	}
	for i := range clusters {
		if r := root(i); r != i && accepted[clusters[r].ClusterID] {
			joined++
		}
	}
	if joined == 0 {
		return clusters, trace, 0
	}
	trace.Labels = labels
	return buildClusters(trace.Processed, labels, maxID, params), trace, joined
}

func finitePositive(v float64) bool { return v > 0 && !math.IsInf(v, 0) }

func validFragmentRect(r Rect) bool {
	for _, v := range []float64{r.X, r.Y, r.HeadingRad, r.Length, r.Width} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return r.Length > 0 && r.Width > 0
}

func fragmentContains(r Rect, p WorldPoint, margin float64) bool {
	if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsNaN(p.Z) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) || math.IsInf(p.Z, 0) {
		return false
	}
	cs, sn := math.Cos(r.HeadingRad), math.Sin(r.HeadingRad)
	dx, dy := p.X-r.X, p.Y-r.Y
	return math.Abs(dx*cs+dy*sn) <= r.Length/2+margin && math.Abs(-dx*sn+dy*cs) <= r.Width/2+margin
}

func fragmentsNear(a, b []WorldPoint, gap float64) bool {
	// Index the smaller membership and stop at the first exact match. Full
	// memberships can contain thousands of returns; comparing every pair makes
	// a refused join particularly expensive. A gap-sized grid limits each query
	// to neighbouring cells without changing the inclusive distance test.
	if len(a) < len(b) {
		a, b = b, a
	}
	type cell struct{ x, y int64 }
	key := func(p WorldPoint) cell {
		return cell{int64(math.Floor(p.X / gap)), int64(math.Floor(p.Y / gap))}
	}
	grid := make(map[cell][]WorldPoint)
	for _, p := range b {
		k := key(p)
		grid[k] = append(grid[k], p)
	}
	g2 := gap * gap
	for _, p := range a {
		k := key(p)
		for x := int64(-1); x <= 1; x++ {
			for y := int64(-1); y <= 1; y++ {
				for _, q := range grid[cell{k.x + x, k.y + y}] {
					dx, dy := p.X-q.X, p.Y-q.Y
					if dx*dx+dy*dy <= g2 {
						return true
					}
				}
			}
		}
	}
	return false
}
