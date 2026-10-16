package perframeeval

import (
	"cmp"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Retention and fragmentation per reviewed mask: the geometry plan's W3
// measure (docs/plans/lidar-tracker-geometry-convergence-plan.md, sections
// 4.3 and 6), and the gate experiment l4clus0001 is scored by.
//
// For each reviewed mask of the selected episodes, frame by frame, it asks
// how many of the object's labelled returns reached an L4 cluster in a
// replay, how many clusters they fell into, and whether any of those
// clusters also held another reviewed object's returns. The first is an L3
// and L4 retention gate, the second an L4 split gate, the third D3's
// near-merge question read the other way.
//
// Matching. The replay's evidence database (lidar-state-estimation-baseline
// -evidence-dir) keeps, per accepted cluster, its population (PointsCount)
// and a retained sample of its members' coordinates (SamplePoints), capped by
// -sample-points at 1024. It keeps no point index. A pack's points and the
// sample are both the float32 of the same float64 that L4 computed, so a
// sample point is matched to a pack point by exact float32 identity of x, y
// and z within the frame whose time equals the pack sample's (to the frame
// tolerance). No distance tolerance is involved. On the kirk0 pack every
// retained sample point of a defaults replay is found among the sample's pack
// points.
//
//   - A cluster whose sample holds every member (PointsCount equal to the
//     sample size) is matched exactly: the mask's returns in it are counted.
//   - A cluster larger than the cap keeps a simple random sample of its
//     members (l4perception's content-seeded uniformSubsample). Its share of
//     the mask is estimated as the mask returns found in the sample times
//     PointsCount over the sample size, the Horvitz-Thompson estimate, which
//     is unbiased but not exact. The returns found in the sample are certain
//     members, so they are reported apart as a lower bound (confirmed).
//
// The report is approximate whenever a capped cluster contributed, and says
// so in its header and its approximate field.
//
// Limits.
//
//   - Fragment counts are a lower bound where a capped cluster is involved: a
//     capped cluster holding a handful of the object's returns may have none
//     in its sample, and is then not seen.
//   - A mask return that reached no cluster cannot be attributed: the
//     reduced evidence does not say whether L3 called it background, the
//     height band removed it, DBSCAN left it as noise or a size filter dropped
//     its cluster. The foreground-complete observation log (velocity lidar
//     replay -observations) records that per return; this measure does not
//     read it yet.
//   - The pack is foreground-only: it holds the returns the recording run's
//     L3 called foreground. A mask cannot contain a return that run called
//     background, so retention is measured against the recorded foreground,
//     not against every return the object produced.
//   - The evidence has rows for clusters, not frames, so a pack sample with no
//     evidence frame is either a frame the replay never produced (a capture
//     gap it assembled differently from the recording) or one in which it
//     clustered nothing at all. They cannot be told apart, and the first is
//     the common case on a street (static clusters put rows in every frame
//     the kirk0 replay produced), so such instants are not measured, and are
//     counted (instants_without_frame) rather than scored as misses. Samples
//     outside the evidence's frames (before warm-up ended, say) are not
//     measured either (instants_outside_window).
//   - Only certifiable masks are measured (the reference policy's scored
//     masks), and merging is counted between those. A cluster shared with an
//     unreviewed or ignored object is not a merge here.

// Retention report format.
const (
	RetentionSchema        = "velocity.report/retention-fragmentation"
	RetentionSchemaVersion = 1
)

// RetentionMethod names the matching rule the report used.
const RetentionMethod = "exact_float32_identity_with_sample_expansion"

// RetentionOptions chooses the reference, the evidence and the strata.
type RetentionOptions struct {
	Reference ReferenceOptions
	// DBPath is the replay's evidence database, opened read-only.
	DBPath string
	// SourceID selects one evidence source when the database holds several.
	SourceID string
	// FrameToleranceNanos is how far an evidence frame's time may be from a
	// pack sample's and still be that sample.
	FrameToleranceNanos int64
	// SensorXM and SensorYM are where the sensor is in the pack's frame; an
	// object's range is from here to its reference position.
	SensorXM, SensorYM float64
	// RangeBandsM are the ascending upper edges of the range bands, in
	// metres; the last band is open.
	RangeBandsM []float64
	// MinDomainShare is the least share of retained sample points that must
	// be pack points of their sample. Below it the evidence is not the pack's
	// point domain (another capture, or another transform), and the measure
	// is refused rather than reported as zero retention.
	MinDomainShare float64
}

// DefaultRetentionOptions are the defaults the tool uses: the reviewed-only
// policy, a 10 ms frame tolerance and the 20, 35 and 50 m band edges
// l4clus0001 reads.
func DefaultRetentionOptions() RetentionOptions {
	return RetentionOptions{
		Reference:           ReferenceOptions{Policy: annotation.DefaultReferencePolicy()},
		FrameToleranceNanos: 10_000_000,
		RangeBandsM:         []float64{20, 35, 50},
		MinDomainShare:      0.5,
	}
}

// Validate refuses options that cannot measure.
func (o RetentionOptions) Validate() error {
	if o.DBPath == "" {
		return fmt.Errorf("no evidence database named")
	}
	if o.FrameToleranceNanos < 0 {
		return fmt.Errorf("frame tolerance %d ns is negative", o.FrameToleranceNanos)
	}
	if len(o.RangeBandsM) == 0 {
		return fmt.Errorf("no range band edges")
	}
	for i, e := range o.RangeBandsM {
		if !(e > 0) || math.IsInf(e, 0) || (i > 0 && e <= o.RangeBandsM[i-1]) {
			return fmt.Errorf("range band edges %v: want positive, finite and ascending", o.RangeBandsM)
		}
	}
	if !(o.MinDomainShare >= 0 && o.MinDomainShare <= 1) {
		return fmt.Errorf("min domain share %g: want 0 to 1", o.MinDomainShare)
	}
	return o.Reference.Policy.Validate()
}

// RetentionMask is one measured instant: an object's reviewed returns in one
// pack sample.
type RetentionMask struct {
	SampleID    int
	TimestampNs int64
	ObjectID    string
	Class       string
	RangeM      float64
	Points      [][3]float32
	// Indices are the returns' positions in the pack sample, parallel to
	// Points: how the fate measure reaches each return's packet identity.
	Indices []int
}

// InstantCluster is one cluster an instant's returns fell into.
type InstantCluster struct {
	ObservationID string `json:"observation_id"`
	ClusterID     int64  `json:"cluster_id"`
	// SampleHits is how many of the mask's returns are in the cluster's
	// retained sample: certain members.
	SampleHits int `json:"sample_hits"`
	// Points is the mask's returns in the cluster: SampleHits when the
	// sample holds every member, the expansion when it is capped.
	Points      float64 `json:"points"`
	Capped      bool    `json:"capped"`
	PointsCount int     `json:"cluster_points"`
	SampleSize  int     `json:"sample_size"`
}

// RetentionInstant is one object in one frame.
type RetentionInstant struct {
	SampleID    int    `json:"sample_id"`
	TimestampNs int64  `json:"timestamp_ns"`
	ObjectID    string `json:"object_id"`
	Class       string `json:"class"`
	// RangeM is the horizontal distance from the sensor to the mask's
	// reference position.
	RangeM     float64 `json:"range_m"`
	Band       string  `json:"range_band"`
	MaskPoints int     `json:"mask_points"`
	// ConfirmedPoints are the mask's returns found in a retained sample.
	ConfirmedPoints int `json:"confirmed_points"`
	// ClusteredPoints are the mask's returns in any cluster, exact or
	// estimated, at most MaskPoints.
	ClusteredPoints float64 `json:"clustered_points"`
	Retention       float64 `json:"retention"`
	// MainShare is the share of the mask in the one cluster holding most
	// of it: the cluster a tracker would associate.
	MainShare  float64          `json:"main_cluster_share"`
	Fragments  int              `json:"clusters"`
	Clusters   []InstantCluster `json:"cluster_detail,omitempty"`
	MergedWith []string         `json:"merged_with,omitempty"`
	// Estimated is set when a capped cluster contributed.
	Estimated bool `json:"estimated"`
}

// RetentionMerge is one cluster holding returns of more than one reviewed
// object.
type RetentionMerge struct {
	SampleID      int                 `json:"sample_id"`
	TimestampNs   int64               `json:"timestamp_ns"`
	ObservationID string              `json:"observation_id"`
	ClusterID     int64               `json:"cluster_id"`
	PointsCount   int                 `json:"cluster_points"`
	Capped        bool                `json:"capped"`
	Objects       []RetentionMergeObj `json:"objects"`
}

// RetentionMergeObj is one object's part of a merged cluster.
type RetentionMergeObj struct {
	ObjectID   string  `json:"object_id"`
	SampleHits int     `json:"sample_hits"`
	Points     float64 `json:"points"`
}

// RetentionMergePair counts the frames in which two objects shared a cluster.
type RetentionMergePair struct {
	A      string `json:"a"`
	B      string `json:"b"`
	Frames int    `json:"frames"`
}

// RetentionSummary pools a set of instants.
type RetentionSummary struct {
	Name            string  `json:"name"`
	Class           string  `json:"class,omitempty"`
	Objects         int     `json:"objects"`
	Instants        int     `json:"instants"`
	MaskPoints      int     `json:"mask_points"`
	ConfirmedPoints int     `json:"confirmed_points"`
	ClusteredPoints float64 `json:"clustered_points"`
	// Retention is pooled: clustered over mask returns.
	Retention float64 `json:"retention"`
	// MeanRetention is the mean of the instants' own shares, so a small far
	// object weighs what a near truck does.
	MeanRetention float64 `json:"mean_retention"`
	// MainClusterShare is pooled: the returns in each instant's main
	// cluster over the mask returns.
	MainClusterShare float64 `json:"main_cluster_share"`
	// Missed instants had no return in any cluster; detected instants had
	// at least one.
	Missed   int `json:"missed_instants"`
	Detected int `json:"detected_instants"`
	// Fragmented instants fell into two or more clusters; the share is of
	// the detected instants.
	Fragmented      int     `json:"fragmented_instants"`
	FragmentedShare float64 `json:"fragmented_share"`
	MeanClusters    float64 `json:"mean_clusters"`
	MaxClusters     int     `json:"max_clusters"`
	// Merged instants shared a cluster with another reviewed object; the
	// share is of the detected instants.
	Merged            int     `json:"merged_instants"`
	MergedShare       float64 `json:"merged_share"`
	EstimatedInstants int     `json:"estimated_instants"`
}

// RetentionEvidence describes the evidence read and how well it matched.
type RetentionEvidence struct {
	// Database is the file's base name.
	Database string `json:"database"`
	SourceID string `json:"source_id"`
	// FirstFrameNs and LastFrameNs bound the source's frames.
	FirstFrameNs int64 `json:"first_frame_ns"`
	LastFrameNs  int64 `json:"last_frame_ns"`
	// Frames are the measured samples matched to an evidence frame.
	Frames         int `json:"frames"`
	Clusters       int `json:"clusters"`
	CappedClusters int `json:"capped_clusters"`
	// LargestSample is the largest retained sample read: the replay's
	// -sample-points when any cluster reached it.
	LargestSample int `json:"largest_sample"`
	// SamplePoints are the retained sample points read; InPack those equal
	// to a point of their pack sample.
	SamplePoints int     `json:"sample_points"`
	InPack       int     `json:"sample_points_in_pack"`
	DomainShare  float64 `json:"domain_share"`
	// Neither count is measured. InstantsOutsideWindow lie outside the
	// evidence's frames; InstantsWithoutFrame lie inside them at a sample
	// with no evidence frame within the tolerance.
	InstantsOutsideWindow int `json:"instants_outside_window"`
	InstantsWithoutFrame  int `json:"instants_without_frame"`
	// FramesWithoutEvidence counts the samples behind InstantsWithoutFrame.
	FramesWithoutEvidence int `json:"frames_without_evidence"`
}

// RetentionReportOpts records the measure's own choices.
type RetentionReportOpts struct {
	FrameToleranceNanos int64     `json:"frame_tolerance_ns"`
	SensorXM            float64   `json:"sensor_x_m"`
	SensorYM            float64   `json:"sensor_y_m"`
	RangeBandsM         []float64 `json:"range_band_edges_m"`
	MinDomainShare      float64   `json:"min_domain_share"`
}

// RetentionReport is the measure over one split against one replay.
type RetentionReport struct {
	Schema        string              `json:"schema"`
	SchemaVersion int                 `json:"schema_version"`
	Reference     ReferenceIdentity   `json:"reference"`
	Evidence      RetentionEvidence   `json:"evidence"`
	Options       RetentionReportOpts `json:"options"`
	Method        string              `json:"method"`
	// Approximate is set when any instant read a capped cluster, so some
	// counts are sample expansions rather than exact.
	Approximate bool                 `json:"approximate"`
	Total       RetentionSummary     `json:"total"`
	ByClass     []RetentionSummary   `json:"by_class"`
	ByRange     []RetentionSummary   `json:"by_range"`
	ByObject    []RetentionSummary   `json:"by_object"`
	MergePairs  []RetentionMergePair `json:"merge_pairs"`
	Merges      []RetentionMerge     `json:"merges"`
	Instants    []RetentionInstant   `json:"instants"`
	Caveats     []string             `json:"caveats"`
}

// MeasureRetention binds the reference as the per-frame evaluator does, reads
// the replay's clusters over the measured frames, and measures every scored
// mask of the selected episodes.
func MeasureRetention(opts RetentionOptions) (*RetentionReport, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	ref, bound, err := loadBoundReference(opts.Reference)
	if err != nil {
		return nil, err
	}
	masks, err := retentionMasks(bound, opts)
	if err != nil {
		return nil, err
	}

	database, err := sqlite.OpenReadOnly(opts.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", opts.DBPath, err)
	}
	defer database.Close()
	store := sqlite.NewObservationStore(database)
	source, err := resolveObservationSource(store, opts.SourceID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", opts.DBPath, err)
	}
	first, last := masks[0].TimestampNs, masks[len(masks)-1].TimestampNs
	clusters, err := store.ListClusterSamples(source, first-opts.FrameToleranceNanos, last+opts.FrameToleranceNanos)
	if err != nil {
		return nil, err
	}
	all, err := store.ListClusterSummariesBySource(source, sqlite.ClusterCentroid)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("source %s holds no clusters", source)
	}

	report := &RetentionReport{
		Schema: RetentionSchema, SchemaVersion: RetentionSchemaVersion, Reference: ref.Identity, Method: RetentionMethod,
		Evidence: RetentionEvidence{Database: filepath.Base(opts.DBPath), SourceID: source,
			FirstFrameNs: all[0].FrameUnixNanos, LastFrameNs: all[len(all)-1].FrameUnixNanos},
		Options: RetentionReportOpts{FrameToleranceNanos: opts.FrameToleranceNanos, SensorXM: opts.SensorXM,
			SensorYM: opts.SensorYM, RangeBandsM: slices.Clone(opts.RangeBandsM), MinDomainShare: opts.MinDomainShare},
	}
	if err := measureFrames(report, bound.pack, masks, clusters, opts); err != nil {
		return nil, err
	}
	summariseRetentionReport(report, opts.RangeBandsM)
	return report, nil
}

// resolveObservationSource picks the named source, or the only one.
func resolveObservationSource(store *sqlite.ObservationStore, want string) (string, error) {
	ids, err := store.ListSourceIDs()
	if err != nil {
		return "", err
	}
	if want != "" {
		if !slices.Contains(ids, want) {
			return "", fmt.Errorf("no observations from source %q (sources: %s)", want, strings.Join(ids, ", "))
		}
		return want, nil
	}
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("no observations: was the replay run with -evidence-dir?")
	case 1:
		return ids[0], nil
	}
	return "", fmt.Errorf("%d observation sources; name one (%s)", len(ids), strings.Join(ids, ", "))
}

// retentionMasks returns the scored masks of the selected episodes, each
// once, in sample then object order, with their returns and range.
func retentionMasks(b *boundReference, opts RetentionOptions) ([]RetentionMask, error) {
	scored := map[instantKey]annotation.ReferencePoint{}
	for _, p := range b.points {
		if p.Ignore == "" {
			scored[instantKey{p.SampleID, p.ObjectID}] = p
		}
	}
	var masks []RetentionMask
	seen := map[instantKey]bool{}
	for _, ep := range b.episodes {
		inEpisode := map[string]bool{}
		for _, id := range ep.ObjectIDs {
			inEpisode[id] = true
		}
		for _, m := range b.sidecar.Masks {
			k := instantKey{m.SampleID, m.ObjectID}
			rp, ok := scored[k]
			if !ok || !inEpisode[m.ObjectID] || !ep.ContainsSample(m.SampleID) || seen[k] {
				continue
			}
			seen[k] = true
			pts, err := b.pack.PointsAt(m.SampleID)
			if err != nil {
				return nil, fmt.Errorf("mask %s sample %d: %w", m.ObjectID, m.SampleID, err)
			}
			rm := RetentionMask{
				SampleID: m.SampleID, TimestampNs: b.pack.Samples[m.SampleID].TimestampNs,
				ObjectID: m.ObjectID, Class: strings.ToLower(strings.TrimSpace(rp.Class)),
				RangeM: math.Hypot(rp.X-opts.SensorXM, rp.Y-opts.SensorYM),
			}
			for _, i := range m.PointIndices {
				rm.Points = append(rm.Points, [3]float32{pts.X[i], pts.Y[i], pts.Z[i]})
				rm.Indices = append(rm.Indices, i)
			}
			masks = append(masks, rm)
		}
	}
	if len(masks) == 0 {
		return nil, fmt.Errorf("the selected episodes hold no scored mask under the %s policy: review the objects first", opts.Reference.Policy.Status)
	}
	slices.SortFunc(masks, func(a, b RetentionMask) int {
		return cmp.Or(cmp.Compare(a.SampleID, b.SampleID), strings.Compare(a.ObjectID, b.ObjectID))
	})
	return masks, nil
}

// pointKey is a point's exact float32 identity.
type pointKey [3]uint32

func keyOf(p [3]float32) pointKey {
	return pointKey{math.Float32bits(p[0]), math.Float32bits(p[1]), math.Float32bits(p[2])}
}

// measureFrames groups the masks by sample, finds each sample's evidence
// frame, and measures it. It refuses evidence that does not share the
// pack's point domain or does not align with its samples.
func measureFrames(report *RetentionReport, pack *annotation.Pack, masks []RetentionMask, clusters []sqlite.ClusterSample, opts RetentionOptions) error {
	var frameTimes []int64
	byFrame := map[int64][]sqlite.ClusterSample{}
	for _, c := range clusters {
		if len(byFrame[c.FrameUnixNanos]) == 0 {
			frameTimes = append(frameTimes, c.FrameUnixNanos)
		}
		byFrame[c.FrameUnixNanos] = append(byFrame[c.FrameUnixNanos], c)
	}
	ev := &report.Evidence
	tol := opts.FrameToleranceNanos
	inWindow, matched := 0, 0
	for start := 0; start < len(masks); {
		end := start
		for end < len(masks) && masks[end].SampleID == masks[start].SampleID {
			end++
		}
		group := masks[start:end]
		start = end
		ts := group[0].TimestampNs
		if ts < ev.FirstFrameNs-tol || ts > ev.LastFrameNs+tol {
			ev.InstantsOutsideWindow += len(group)
			continue
		}
		inWindow++
		frame, _, ok := nearestFrame(frameTimes, ts)
		if !ok || abs64(frame-ts) > tol {
			ev.InstantsWithoutFrame += len(group)
			ev.FramesWithoutEvidence++
			continue
		}
		matched++
		frameClusters := byFrame[frame]
		for _, c := range frameClusters {
			if len(c.Sample) == 0 && c.PointsCount > 0 {
				return fmt.Errorf("observation %s keeps no retained sample: replay with -sample-points (1024 for this measure)", c.ObservationID)
			}
		}
		pts, err := pack.PointsAt(group[0].SampleID)
		if err != nil {
			return err
		}
		domain := make(map[pointKey]bool, len(pts.X))
		for i := range pts.X {
			domain[keyOf([3]float32{pts.X[i], pts.Y[i], pts.Z[i]})] = true
		}
		instants, merges, stats := measureFrame(group, frameClusters, domain)
		report.Instants = append(report.Instants, instants...)
		report.Merges = append(report.Merges, merges...)
		ev.Frames++
		ev.Clusters += stats.clusters
		ev.CappedClusters += stats.capped
		ev.LargestSample = max(ev.LargestSample, stats.largestSample)
		ev.SamplePoints += stats.samplePoints
		ev.InPack += stats.inPack
	}
	switch {
	case inWindow == 0:
		return fmt.Errorf("no measured sample lies inside the evidence's frames (%d to %d ns): is it a replay of this pack's capture?",
			ev.FirstFrameNs, ev.LastFrameNs)
	case matched*2 < inWindow:
		return fmt.Errorf("only %d of %d measured samples inside the evidence window have an evidence frame within %d ns: "+
			"the replay's frames do not align with the pack's samples", matched, inWindow, tol)
	}
	if ev.SamplePoints > 0 {
		ev.DomainShare = float64(ev.InPack) / float64(ev.SamplePoints)
	}
	if ev.DomainShare < opts.MinDomainShare {
		return fmt.Errorf("%d of %d retained sample points (%.1f%%) are points of their pack sample, below the %.0f%% floor: "+
			"the evidence is not in this pack's point domain (another capture, or another transform)",
			ev.InPack, ev.SamplePoints, 100*ev.DomainShare, 100*opts.MinDomainShare)
	}
	return nil
}

type frameStats struct {
	clusters, capped, largestSample, samplePoints, inPack int
}

// measureFrame measures one frame's masks against its clusters. domain is
// the set of the pack sample's points, for the domain check; nil skips it.
// Clusters are matched to masks by exact float32 identity of their sample
// points; a capped cluster's count is expanded by its sampling fraction.
func measureFrame(masks []RetentionMask, clusters []sqlite.ClusterSample, domain map[pointKey]bool) ([]RetentionInstant, []RetentionMerge, frameStats) {
	owners := map[pointKey][]int{}
	for mi, m := range masks {
		for _, p := range m.Points {
			k := keyOf(p)
			if !slices.Contains(owners[k], mi) {
				owners[k] = append(owners[k], mi)
			}
		}
	}
	var stats frameStats
	// hits[mi][ci] is mask mi's returns found in cluster ci's sample.
	hits := make([]map[int]int, len(masks))
	for mi := range hits {
		hits[mi] = map[int]int{}
	}
	for ci, c := range clusters {
		stats.clusters++
		if c.PointsCount > len(c.Sample) {
			stats.capped++
		}
		stats.largestSample = max(stats.largestSample, len(c.Sample))
		for _, p := range c.Sample {
			k := keyOf(p)
			stats.samplePoints++
			if domain[k] {
				stats.inPack++
			}
			for _, mi := range owners[k] {
				hits[mi][ci]++
			}
		}
	}

	instants := make([]RetentionInstant, len(masks))
	holders := map[int][]int{} // cluster index -> masks with a return in it
	for mi, m := range masks {
		in := RetentionInstant{SampleID: m.SampleID, TimestampNs: m.TimestampNs, ObjectID: m.ObjectID,
			Class: m.Class, RangeM: m.RangeM, MaskPoints: len(m.Points)}
		cis := make([]int, 0, len(hits[mi]))
		for ci := range hits[mi] {
			cis = append(cis, ci)
		}
		slices.Sort(cis)
		var clustered, main float64
		for _, ci := range cis {
			c, h := clusters[ci], hits[mi][ci]
			ic := InstantCluster{ObservationID: c.ObservationID, ClusterID: c.ClusterID, SampleHits: h,
				Points: float64(h), Capped: c.PointsCount > len(c.Sample), PointsCount: c.PointsCount, SampleSize: len(c.Sample)}
			if ic.Capped {
				ic.Points = float64(h) * float64(c.PointsCount) / float64(len(c.Sample))
				in.Estimated = true
			}
			in.Clusters = append(in.Clusters, ic)
			in.ConfirmedPoints += h
			clustered += ic.Points
			main = max(main, ic.Points)
			holders[ci] = append(holders[ci], mi)
		}
		n := float64(in.MaskPoints)
		in.ClusteredPoints = min(clustered, n)
		in.Fragments = len(in.Clusters)
		if n > 0 {
			in.Retention = in.ClusteredPoints / n
			in.MainShare = min(main, n) / n
		}
		instants[mi] = in
	}

	var merges []RetentionMerge
	cis := make([]int, 0, len(holders))
	for ci := range holders {
		cis = append(cis, ci)
	}
	slices.Sort(cis)
	partners := make([]map[string]bool, len(masks))
	for _, ci := range cis {
		ms := holders[ci]
		if len(ms) < 2 {
			continue
		}
		c := clusters[ci]
		mg := RetentionMerge{SampleID: masks[ms[0]].SampleID, TimestampNs: masks[ms[0]].TimestampNs,
			ObservationID: c.ObservationID, ClusterID: c.ClusterID, PointsCount: c.PointsCount, Capped: c.PointsCount > len(c.Sample)}
		for _, mi := range ms {
			for _, ic := range instants[mi].Clusters {
				if ic.ObservationID == c.ObservationID {
					mg.Objects = append(mg.Objects, RetentionMergeObj{ObjectID: masks[mi].ObjectID, SampleHits: ic.SampleHits, Points: ic.Points})
				}
			}
			if partners[mi] == nil {
				partners[mi] = map[string]bool{}
			}
			for _, other := range ms {
				if other != mi {
					partners[mi][masks[other].ObjectID] = true
				}
			}
		}
		merges = append(merges, mg)
	}
	for mi := range instants {
		for id := range partners[mi] {
			instants[mi].MergedWith = append(instants[mi].MergedWith, id)
		}
		slices.Sort(instants[mi].MergedWith)
	}
	return instants, merges, stats
}

// rangeBand names the band a range falls in: range_0_20m, ..., range_50m_plus.
func rangeBand(r float64, edges []float64) string {
	lower := 0.0
	for _, e := range edges {
		if r < e {
			return fmt.Sprintf("range_%g_%gm", lower, e)
		}
		lower = e
	}
	return fmt.Sprintf("range_%gm_plus", lower)
}

// rangeBandNames lists every band in order, so empty bands still appear.
func rangeBandNames(edges []float64) []string {
	names := make([]string, 0, len(edges)+1)
	lower := 0.0
	for _, e := range edges {
		names = append(names, fmt.Sprintf("range_%g_%gm", lower, e))
		lower = e
	}
	return append(names, fmt.Sprintf("range_%gm_plus", lower))
}

type retentionAccum struct {
	s       RetentionSummary
	objects map[string]bool
	sumRet  float64
	sumClus int
	main    float64
}

func (a *retentionAccum) add(in RetentionInstant) {
	if a.objects == nil {
		a.objects = map[string]bool{}
	}
	a.objects[in.ObjectID] = true
	s := &a.s
	s.Instants++
	s.MaskPoints += in.MaskPoints
	s.ConfirmedPoints += in.ConfirmedPoints
	s.ClusteredPoints += in.ClusteredPoints
	a.sumRet += in.Retention
	a.main += in.MainShare * float64(in.MaskPoints)
	if in.Estimated {
		s.EstimatedInstants++
	}
	if in.Fragments == 0 {
		s.Missed++
		return
	}
	s.Detected++
	a.sumClus += in.Fragments
	s.MaxClusters = max(s.MaxClusters, in.Fragments)
	if in.Fragments >= 2 {
		s.Fragmented++
	}
	if len(in.MergedWith) > 0 {
		s.Merged++
	}
}

func (a *retentionAccum) summary(name string) RetentionSummary {
	s := a.s
	s.Name, s.Objects = name, len(a.objects)
	if s.MaskPoints > 0 {
		s.Retention = s.ClusteredPoints / float64(s.MaskPoints)
		s.MainClusterShare = a.main / float64(s.MaskPoints)
	}
	if s.Instants > 0 {
		s.MeanRetention = a.sumRet / float64(s.Instants)
	}
	if s.Detected > 0 {
		s.FragmentedShare = float64(s.Fragmented) / float64(s.Detected)
		s.MergedShare = float64(s.Merged) / float64(s.Detected)
		s.MeanClusters = float64(a.sumClus) / float64(s.Detected)
	}
	return s
}

// summariseRetentionReport pools the instants overall, by class, by range
// band and by object, counts merges per object pair, and writes the caveats.
func summariseRetentionReport(r *RetentionReport, edges []float64) {
	var total retentionAccum
	byClass, byRange, byObject := map[string]*retentionAccum{}, map[string]*retentionAccum{}, map[string]*retentionAccum{}
	classOf := map[string]string{}
	get := func(m map[string]*retentionAccum, k string) *retentionAccum {
		if m[k] == nil {
			m[k] = &retentionAccum{}
		}
		return m[k]
	}
	for i := range r.Instants {
		in := &r.Instants[i]
		in.Band = rangeBand(in.RangeM, edges)
		total.add(*in)
		get(byClass, in.Class).add(*in)
		get(byRange, in.Band).add(*in)
		get(byObject, in.ObjectID).add(*in)
		classOf[in.ObjectID] = in.Class
		r.Approximate = r.Approximate || in.Estimated
	}
	r.Total = total.summary("all")
	for _, k := range sortedMapKeys(byClass) {
		r.ByClass = append(r.ByClass, byClass[k].summary(k))
	}
	for _, k := range rangeBandNames(edges) {
		if a := byRange[k]; a != nil {
			r.ByRange = append(r.ByRange, a.summary(k))
		} else {
			r.ByRange = append(r.ByRange, RetentionSummary{Name: k})
		}
	}
	for _, k := range sortedMapKeys(byObject) {
		s := byObject[k].summary(k)
		s.Class = classOf[k]
		r.ByObject = append(r.ByObject, s)
	}

	pairs := map[[2]string]map[int]bool{}
	for _, m := range r.Merges {
		for i := range m.Objects {
			for j := i + 1; j < len(m.Objects); j++ {
				a, b := m.Objects[i].ObjectID, m.Objects[j].ObjectID
				if b < a {
					a, b = b, a
				}
				k := [2]string{a, b}
				if pairs[k] == nil {
					pairs[k] = map[int]bool{}
				}
				pairs[k][m.SampleID] = true
			}
		}
	}
	for k, frames := range pairs {
		r.MergePairs = append(r.MergePairs, RetentionMergePair{A: k[0], B: k[1], Frames: len(frames)})
	}
	slices.SortFunc(r.MergePairs, func(x, y RetentionMergePair) int {
		return cmp.Or(cmp.Compare(y.Frames, x.Frames), strings.Compare(x.A, y.A), strings.Compare(x.B, y.B))
	})
	if r.MergePairs == nil {
		r.MergePairs = []RetentionMergePair{}
	}
	if r.Merges == nil {
		r.Merges = []RetentionMerge{}
	}
	r.Caveats = retentionCaveats(r)
}

func retentionCaveats(r *RetentionReport) []string {
	c := []string{
		"Masks are matched to clusters by exact float32 identity of each retained sample point with a pack point of the same sample; no distance tolerance is used.",
		"The pack is foreground-only: retention is measured against the returns the recording run's L3 called foreground, not every return the object produced.",
		"A return that reached no cluster is not attributed: the evidence does not say whether L3, the height band, DBSCAN noise or a cluster size filter lost it.",
	}
	if r.Approximate {
		c = append(c, fmt.Sprintf("Approximate: %d of %d instants read a cluster larger than its retained sample (largest sample %d). "+
			"Their counts are Horvitz-Thompson expansions of the sample; confirmed points are the certain lower bound, "+
			"and a fragment with no return in a capped sample is not seen, so fragment counts are a lower bound there.",
			r.Total.EstimatedInstants, r.Total.Instants, r.Evidence.LargestSample))
	}
	if r.Evidence.InstantsWithoutFrame > 0 {
		c = append(c, fmt.Sprintf("%d instants at %d samples inside the evidence window have no evidence frame and are not measured: "+
			"the replay produced no frame there, or clustered nothing in it, and the evidence cannot say which.",
			r.Evidence.InstantsWithoutFrame, r.Evidence.FramesWithoutEvidence))
	}
	if r.Evidence.InstantsOutsideWindow > 0 {
		c = append(c, fmt.Sprintf("%d instants lie outside the evidence's frames (before warm-up ended, or past the replay's end) and are not measured.",
			r.Evidence.InstantsOutsideWindow))
	}
	if r.Evidence.DomainShare < 0.99 {
		c = append(c, fmt.Sprintf("Only %.1f%% of retained sample points are pack points: the replay's foreground differs from the recording's, "+
			"and its extra returns cannot belong to a mask.", 100*r.Evidence.DomainShare))
	}
	if !r.Reference.HeldOut {
		c = append(c, fmt.Sprintf("Split %q is role %q, not held out: a tuning measure, not to be quoted as held out.", r.Reference.Split, r.Reference.SplitRole))
	}
	return c
}
