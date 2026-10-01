package annotation

import (
	"fmt"
	"sort"
	"strings"
)

// Reasons an object is left out of a drafted split.
const (
	DraftSkipNotRoadUser    = "not_a_road_user"
	DraftSkipNotReviewed    = "object_not_reviewed"
	DraftSkipProposedMasks  = "has_proposed_masks"
	DraftSkipUnstatedMasks  = "has_masks_of_unstated_completeness"
	DraftSkipNoScoredInSpan = "no_scored_mask_in_the_window"
)

// DraftSplitOptions says what split to draft from a reviewed pack.
type DraftSplitOptions struct {
	// SplitName and Role name the one partition drafted. Every object that
	// qualifies goes in it, so a tuning split holds what is not held out.
	SplitName string
	Role      SplitRole
	// EpisodeID names the one episode drafted over the window.
	EpisodeID string
	// FirstSample and LastSample bound the window, inclusive. A zero
	// LastSample means the pack's last sample.
	FirstSample, LastSample int
	// Policy decides which masks score; the default policy certifies only
	// reviewed masks of reviewed road users.
	Policy ReferencePolicy
	Note   string
}

// DraftObject is an object that went into the split, with what it contributes.
type DraftObject struct {
	ObjectID string `json:"object_id"`
	Class    string `json:"class"`
	// ScoredMasks is how many of its masks inside the window the policy scores.
	ScoredMasks int `json:"scored_masks"`
	FirstSample int `json:"first_sample"`
	LastSample  int `json:"last_sample"`
}

// DraftSkipped is an object left out and why.
type DraftSkipped struct {
	ObjectID string `json:"object_id"`
	Class    string `json:"class"`
	Reason   string `json:"reason"`
}

// DraftSplitReport lists what the draft took and what it left out, so a person
// can see what is missing before freezing.
type DraftSplitReport struct {
	SidecarRevision int            `json:"sidecar_revision"`
	FirstSample     int            `json:"first_sample"`
	LastSample      int            `json:"last_sample"`
	Included        []DraftObject  `json:"included"`
	Skipped         []DraftSkipped `json:"skipped"`
	// ScoredMasks is the total over the included objects.
	ScoredMasks int `json:"scored_masks"`
}

// DraftSplitManifest drafts a version 1 split manifest over one window of a
// reviewed pack: one partition holding every object that the policy certifies
// and that would survive freezing, and one episode scoring them over the
// window. It pins the annotation revision it was drafted from, so a result
// names the labels it used. It is a draft: nothing has been frozen, and a
// frozen split pins the pack's files beyond this.
//
// An object is left out, and listed with the reason, if it is not a road user,
// is not reviewed, has a mask that is still proposed or whose completeness was
// never stated (freezing refuses those), or has no scored mask in the window.
func DraftSplitManifest(p *Pack, s *Sidecar, o DraftSplitOptions) (*SplitManifest, *DraftSplitReport, error) {
	if o.SplitName == "" {
		return nil, nil, fmt.Errorf("a draft needs a split name")
	}
	if o.Role != SplitRoleTuning && o.Role != SplitRoleHeldOut {
		return nil, nil, fmt.Errorf("split role %q: want %q or %q", o.Role, SplitRoleTuning, SplitRoleHeldOut)
	}
	if o.EpisodeID == "" {
		o.EpisodeID = fmt.Sprintf("%s-from-%d", o.SplitName, o.FirstSample)
	}
	if len(p.Samples) == 0 {
		return nil, nil, fmt.Errorf("the pack has no samples")
	}
	if o.LastSample == 0 {
		o.LastSample = len(p.Samples) - 1
	}
	if o.FirstSample < 0 || o.LastSample < o.FirstSample || o.LastSample >= len(p.Samples) {
		return nil, nil, fmt.Errorf("window [%d, %d] is not inside the pack's %d samples", o.FirstSample, o.LastSample, len(p.Samples))
	}
	if err := o.Policy.Validate(); err != nil {
		return nil, nil, err
	}
	points, _, err := BuildReference(p, s, o.Policy)
	if err != nil {
		return nil, nil, err
	}
	scored := map[string]map[int]bool{}
	for _, rp := range points {
		if rp.Ignore == "" && rp.SampleID >= o.FirstSample && rp.SampleID <= o.LastSample {
			if scored[rp.ObjectID] == nil {
				scored[rp.ObjectID] = map[int]bool{}
			}
			scored[rp.ObjectID][rp.SampleID] = true
		}
	}
	roadUser := map[string]bool{}
	for _, c := range o.Policy.RoadUserClasses {
		roadUser[strings.ToLower(strings.TrimSpace(c))] = true
	}

	report := &DraftSplitReport{SidecarRevision: s.Revision, FirstSample: o.FirstSample, LastSample: o.LastSample}
	var ids []string
	objects := append([]Object(nil), s.Objects...)
	sort.Slice(objects, func(i, j int) bool { return objects[i].ObjectID < objects[j].ObjectID })
	for _, obj := range objects {
		if obj.Status == StatusRejected {
			continue
		}
		skip := func(reason string) {
			report.Skipped = append(report.Skipped, DraftSkipped{ObjectID: obj.ObjectID, Class: obj.Class, Reason: reason})
		}
		var proposed, unstated bool
		for _, m := range s.Masks {
			if m.ObjectID != obj.ObjectID || m.Status == StatusRejected {
				continue
			}
			proposed = proposed || m.Status == StatusProposed
			unstated = unstated || (m.Status == StatusReviewed && m.Completeness == MaskUnreviewed)
		}
		switch {
		case !roadUser[strings.ToLower(strings.TrimSpace(obj.Class))]:
			skip(DraftSkipNotRoadUser)
		case obj.Status != StatusReviewed:
			skip(DraftSkipNotReviewed)
		case proposed:
			skip(DraftSkipProposedMasks)
		case unstated:
			skip(DraftSkipUnstatedMasks)
		case len(scored[obj.ObjectID]) == 0:
			skip(DraftSkipNoScoredInSpan)
		default:
			d := DraftObject{ObjectID: obj.ObjectID, Class: obj.Class, ScoredMasks: len(scored[obj.ObjectID]), FirstSample: o.LastSample, LastSample: o.FirstSample}
			for sid := range scored[obj.ObjectID] {
				d.FirstSample, d.LastSample = min(d.FirstSample, sid), max(d.LastSample, sid)
			}
			report.Included = append(report.Included, d)
			report.ScoredMasks += d.ScoredMasks
			ids = append(ids, obj.ObjectID)
		}
	}
	if len(ids) == 0 {
		return nil, report, fmt.Errorf("no object qualifies in samples %d to %d: every reviewed road user is listed as skipped with its reason", o.FirstSample, o.LastSample)
	}
	m := &SplitManifest{
		Schema: SplitSchema, SchemaVersion: SplitSchemaVersion,
		PackDigest: p.Manifest.PackDigest, DatasetID: p.Manifest.DatasetID, SidecarRevision: s.Revision, Note: o.Note,
		Splits: []Split{{Name: o.SplitName, Role: o.Role, ObjectIDs: ids}},
		Episodes: []Episode{{EpisodeID: o.EpisodeID, Split: o.SplitName, ObjectIDs: ids,
			FrameIntervals: []FrameInterval{{FirstSample: o.FirstSample, LastSample: o.LastSample}}}},
	}
	if err := m.validateStructure(); err != nil {
		return nil, report, fmt.Errorf("drafted manifest is invalid: %w", err)
	}
	if err := m.ValidateAgainst(p, s); err != nil {
		return nil, report, fmt.Errorf("drafted manifest does not bind: %w", err)
	}
	return m, report, nil
}

// FirstSampleAtOrAfter is the first sample whose capture time is at least the
// given number of seconds after the first sample's.
func FirstSampleAtOrAfter(p *Pack, seconds float64) (int, error) {
	if len(p.Samples) == 0 {
		return 0, fmt.Errorf("the pack has no samples")
	}
	if seconds < 0 {
		return 0, fmt.Errorf("%g seconds is before the first sample", seconds)
	}
	return FirstSampleAtOrAfterNs(p, p.Samples[0].TimestampNs+int64(seconds*1e9))
}

// FirstSampleAtOrAfterNs is the first sample captured at or after a time. It is
// how a window that starts where a replay's estimates start is found: a replay
// writes none before its first confirmed track, so a mask earlier than that
// reads as an object nobody found.
func FirstSampleAtOrAfterNs(p *Pack, ns int64) (int, error) {
	for i, s := range p.Samples {
		if s.TimestampNs >= ns {
			return i, nil
		}
	}
	if len(p.Samples) == 0 {
		return 0, fmt.Errorf("the pack has no samples")
	}
	return 0, fmt.Errorf("the pack ends %.1f s after its first sample, before the requested time", float64(p.Samples[len(p.Samples)-1].TimestampNs-p.Samples[0].TimestampNs)/1e9)
}
