package perframeeval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// IgnoreOutsideEpisode marks a reference point of an object the episode does
// not score. It takes precedence over the policy's own reason, because it is
// the reason that point is ignored here.
const IgnoreOutsideEpisode = "outside_episode"

// ReferenceOptions chooses the reference.
type ReferenceOptions struct {
	PackDir           string
	SplitManifestPath string
	Split             string
	// Episodes restricts scoring to the named episodes of Split. Empty scores
	// all of them.
	Episodes []string
	// AllowTuningSplit scores a split whose role is not held_out. The result
	// then carries held_out: false and says so in its caveats.
	AllowTuningSplit bool
	Policy           annotation.ReferencePolicy
}

// ReferenceIdentity is everything two arms must share for their scores to be
// compared.
type ReferenceIdentity struct {
	PackDigest          string                     `json:"pack_digest"`
	DatasetID           string                     `json:"dataset_id"`
	SidecarRevision     int                        `json:"sidecar_revision"`
	SplitManifestDigest string                     `json:"split_manifest_digest"`
	Split               string                     `json:"split"`
	SplitRole           annotation.SplitRole       `json:"split_role"`
	HeldOut             bool                       `json:"held_out"`
	Policy              annotation.ReferencePolicy `json:"policy"`
	Episodes            []string                   `json:"episodes"`
	// Digest is SHA-256 over the canonical encoding of the fields above and
	// every episode's reference series. Equal digests mean equal truth.
	Digest string `json:"digest"`
}

// EpisodeReferenceStats describes what an episode's reference holds.
type EpisodeReferenceStats struct {
	Frames int `json:"frames"`
	// FramesWithoutMasks counts scored frames in which no object has a mask.
	// Hypotheses there can only be false positives, which is right only if the
	// frame really is empty; a high count means the episode was not labelled
	// densely enough to score.
	FramesWithoutMasks int            `json:"frames_without_masks"`
	ScoredObjects      int            `json:"scored_objects"`
	ScoredPoints       int            `json:"scored_points"`
	IgnoredPoints      int            `json:"ignored_points"`
	IgnoredByReason    map[string]int `json:"ignored_by_reason"`
}

// EpisodeReference is one episode's truth.
type EpisodeReference struct {
	EpisodeID string                     `json:"episode_id"`
	ObjectIDs []string                   `json:"object_ids"`
	Intervals []annotation.FrameInterval `json:"frame_intervals"`
	// Frames are the scored sample timestamps, ascending.
	Frames []int64                   `json:"frames_ns"`
	Series []l8analytics.TrackSeries `json:"series"`
	Stats  EpisodeReferenceStats     `json:"stats"`
	spans  []timeSpan
}

type timeSpan struct{ first, last int64 }

// Reference is the loaded truth for one split.
type Reference struct {
	Identity ReferenceIdentity
	// Summary is the policy's mapping over the whole sidecar, before episodes.
	Summary  annotation.ReferenceSummary
	Episodes []EpisodeReference
}

// LoadReference opens the pack, the split manifest and the annotation
// revision it pins (or the current one), binds them together, and builds each
// selected episode's reference. Every disagreement is a refusal.
func LoadReference(opts ReferenceOptions) (*Reference, error) {
	if opts.Split == "" {
		return nil, fmt.Errorf("no split named: say which partition to score")
	}
	pack, err := annotation.OpenPack(opts.PackDir)
	if err != nil {
		return nil, fmt.Errorf("open pack: %w", err)
	}
	manifest, err := annotation.LoadSplitManifest(opts.SplitManifestPath)
	if err != nil {
		return nil, err
	}
	var sidecar *annotation.Sidecar
	if manifest.SidecarRevision > 0 {
		sidecar, err = annotation.LoadSidecarRevision(pack, manifest.SidecarRevision)
	} else {
		sidecar, err = annotation.LoadSidecar(pack)
	}
	if err != nil {
		return nil, fmt.Errorf("load annotation: %w", err)
	}
	if err := manifest.ValidateAgainst(pack, sidecar); err != nil {
		return nil, err
	}
	episodes, err := manifest.SelectEpisodes(opts.Split, opts.Episodes, !opts.AllowTuningSplit)
	if err != nil {
		return nil, err
	}
	split, _ := manifest.SplitByName(opts.Split)

	points, summary, err := annotation.BuildReference(pack, sidecar, opts.Policy)
	if err != nil {
		return nil, err
	}
	bySample := map[int][]annotation.ReferencePoint{}
	for _, p := range points {
		bySample[p.SampleID] = append(bySample[p.SampleID], p)
	}

	ref := &Reference{Summary: summary}
	for _, ep := range episodes {
		er, err := buildEpisode(pack, ep, bySample, opts.Policy)
		if err != nil {
			return nil, err
		}
		ref.Episodes = append(ref.Episodes, er)
	}

	ids := make([]string, len(ref.Episodes))
	for i, e := range ref.Episodes {
		ids[i] = e.EpisodeID
	}
	ref.Identity = ReferenceIdentity{
		PackDigest:          pack.Manifest.PackDigest,
		DatasetID:           pack.Manifest.DatasetID,
		SidecarRevision:     sidecar.Revision,
		SplitManifestDigest: manifest.Digest,
		Split:               split.Name,
		SplitRole:           split.Role,
		HeldOut:             split.Role == annotation.SplitRoleHeldOut,
		Policy:              opts.Policy,
		Episodes:            ids,
	}
	digest, err := referenceDigest(ref.Identity, ref.Episodes)
	if err != nil {
		return nil, err
	}
	ref.Identity.Digest = digest
	return ref, nil
}

func buildEpisode(pack *annotation.Pack, ep annotation.Episode, bySample map[int][]annotation.ReferencePoint,
	policy annotation.ReferencePolicy) (EpisodeReference, error) {

	scores := make(map[string]bool, len(ep.ObjectIDs))
	for _, id := range ep.ObjectIDs {
		scores[id] = true
	}
	out := EpisodeReference{
		EpisodeID: ep.EpisodeID,
		ObjectIDs: append([]string(nil), ep.ObjectIDs...),
		Intervals: append([]annotation.FrameInterval(nil), ep.FrameIntervals...),
		Stats:     EpisodeReferenceStats{IgnoredByReason: map[string]int{}},
	}
	series := map[string]*l8analytics.TrackSeries{}
	sampleAt := map[int64]int{}
	present, scored := map[string]bool{}, map[string]bool{}

	for _, iv := range ep.FrameIntervals {
		span := timeSpan{first: pack.Samples[iv.FirstSample].TimestampNs, last: pack.Samples[iv.FirstSample].TimestampNs}
		for sid := iv.FirstSample; sid <= iv.LastSample; sid++ {
			ts := pack.Samples[sid].TimestampNs
			// The matcher keys frames by time. Two samples sharing one would
			// be merged into one frame, silently; refuse instead.
			if other, dup := sampleAt[ts]; dup {
				return out, fmt.Errorf("episode %q: samples %d and %d share timestamp %d, and frames are matched by time",
					ep.EpisodeID, other, sid, ts)
			}
			sampleAt[ts] = sid
			span.first, span.last = min(span.first, ts), max(span.last, ts)
			out.Frames = append(out.Frames, ts)

			refs := bySample[sid]
			if len(refs) == 0 {
				out.Stats.FramesWithoutMasks++
			}
			for _, r := range refs {
				reason := string(r.Ignore)
				if !scores[r.ObjectID] {
					reason = IgnoreOutsideEpisode
				} else {
					present[r.ObjectID] = true
				}
				s := series[r.ObjectID]
				if s == nil {
					s = &l8analytics.TrackSeries{ID: r.ObjectID}
					series[r.ObjectID] = s
				}
				s.Points = append(s.Points, l8analytics.SeriesPoint{
					TimestampNanos: ts, X: float32(r.X), Y: float32(r.Y),
					Ignore: reason != "", FootprintDiagonalMetres: float32(r.FootprintDiagonal),
				})
				if reason == "" {
					out.Stats.ScoredPoints++
					scored[r.ObjectID] = true
				} else {
					out.Stats.IgnoredPoints++
					out.Stats.IgnoredByReason[reason]++
				}
			}
		}
		out.spans = append(out.spans, span)
	}
	out.Stats.Frames = len(out.Frames)
	out.Stats.ScoredObjects = len(scored)
	sort.Slice(out.Frames, func(i, j int) bool { return out.Frames[i] < out.Frames[j] })

	// An episode that names an object with no mask in its frames was frozen
	// against different labels, or mistyped.
	for _, id := range ep.ObjectIDs {
		if !present[id] {
			return out, fmt.Errorf("episode %q scores object %q, which has no mask in the episode's frames", ep.EpisodeID, id)
		}
	}
	// Nothing certifiable means nothing to score: every number would be a
	// ratio over zero, or a count of false positives against nothing.
	if out.Stats.ScoredPoints == 0 {
		return out, fmt.Errorf("episode %q has no certifiable reference point under the %s policy (%s): review its objects first",
			ep.EpisodeID, policy.Status, describeReasons(out.Stats.IgnoredByReason))
	}

	ids := make([]string, 0, len(series))
	for id := range series {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		s := series[id]
		sort.Slice(s.Points, func(i, j int) bool { return s.Points[i].TimestampNanos < s.Points[j].TimestampNanos })
		out.Series = append(out.Series, *s)
	}
	return out, nil
}

func describeReasons(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
	}
	if len(parts) == 0 {
		return "no points at all"
	}
	return "ignored: " + strings.Join(parts, ", ")
}

// referenceDigest is SHA-256 over the canonical JSON of the identity (without
// its digest) and every episode's reference. encoding/json writes struct
// fields in declaration order and map keys sorted, so the encoding is stable.
func referenceDigest(id ReferenceIdentity, episodes []EpisodeReference) (string, error) {
	id.Digest = ""
	b, err := json.Marshal(struct {
		Identity ReferenceIdentity  `json:"identity"`
		Episodes []EpisodeReference `json:"episodes"`
	}{id, episodes})
	if err != nil {
		return "", fmt.Errorf("encode reference for digest: %w", err)
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
