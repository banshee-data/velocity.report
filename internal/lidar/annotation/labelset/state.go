package labelset

import (
	"fmt"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// SchemaVersion is the label set layout version.
const SchemaVersion = 2

// DomainForeground is the only kind of domain: annotation is downstream of
// foreground separation, so every mask is made on what some extraction kept.
const DomainForeground = "foreground"

// Domain names the foreground extraction a mask was made on. A return outside
// a mask's domain was never shown to the labeller and is unlabelled, never a
// negative.
type Domain struct {
	DomainID     string                 `json:"domain_id"`
	Kind         string                 `json:"kind"`
	RunID        string                 `json:"run_id,omitempty"`
	RunConfigID  string                 `json:"run_config_id,omitempty"`
	ConfigHash   string                 `json:"config_hash,omitempty"`
	ParamsHash   string                 `json:"params_hash,omitempty"`
	BuildVersion string                 `json:"build_version,omitempty"`
	BuildGitSHA  string                 `json:"build_git_sha,omitempty"`
	HeightBand   *annotation.HeightBand `json:"height_band,omitempty"`
	Note         string                 `json:"note,omitempty"`
}

// Window is one frame as the labeller saw it: the half-open range of return
// numbers [Start, End) holding every return of the frame. Masks, domain
// returns and completeness claims are all per window.
type Window struct {
	WindowID int    `json:"window_id"`
	Start    uint64 `json:"start"`
	End      uint64 `json:"end"`
	// CaptureStartNs and CaptureEndNs are capture times for people reading
	// the file; nothing joins on them.
	CaptureStartNs int64 `json:"capture_start_ns,omitempty"`
	CaptureEndNs   int64 `json:"capture_end_ns,omitempty"`
	// LegacySampleID is the version 1 sample this window was migrated from.
	LegacySampleID *int `json:"legacy_sample_id,omitempty"`
}

// Len is the number of return numbers the window spans.
func (w Window) Len() uint64 { return w.End - w.Start }

// DomainReturns is the set of returns a domain's extraction kept in one
// window, as offsets from the window's Start.
type DomainReturns struct {
	DomainID string `json:"domain_id"`
	WindowID int    `json:"window_id"`
	Returns  Ranges `json:"returns"`
}

// Mask is one object's membership within one window. Members and Uncertain
// are offsets from the window's Start, and lie inside the mask's domain.
type Mask struct {
	ObjectID     string                      `json:"object_id"`
	WindowID     int                         `json:"window_id"`
	DomainID     string                      `json:"domain_id"`
	Members      Ranges                      `json:"members"`
	Uncertain    Ranges                      `json:"uncertain,omitempty"`
	Completeness annotation.MaskCompleteness `json:"completeness"`
	Visibility   annotation.Visibility       `json:"visibility"`
	Status       annotation.ReviewStatus     `json:"status"`
	Pose         *annotation.Pose            `json:"pose,omitempty"`
	Provenance   annotation.Provenance       `json:"provenance"`
}

// Records are the label set's object-level records.
type Records struct {
	Objects            []annotation.Object              `json:"objects"`
	Correspondences    []annotation.TrackCorrespondence `json:"correspondences"`
	DismissedProposals []string                         `json:"dismissed_proposals"`
}

// State is everything a revision pins: the packet table, the records,
// domains, windows, the domains' returns and the masks.
type State struct {
	Packets       []PacketKey
	Records       Records
	Domains       []Domain
	Windows       []Window
	DomainReturns []DomainReturns
	Masks         []Mask
}

// Clone copies a state deeply enough that editing the copy cannot change the
// original.
func (s *State) Clone() State {
	c := State{
		Packets: append([]PacketKey(nil), s.Packets...),
		Records: Records{
			Objects:            append([]annotation.Object(nil), s.Records.Objects...),
			Correspondences:    append([]annotation.TrackCorrespondence(nil), s.Records.Correspondences...),
			DismissedProposals: append([]string(nil), s.Records.DismissedProposals...),
		},
		Domains:       append([]Domain(nil), s.Domains...),
		Windows:       append([]Window(nil), s.Windows...),
		DomainReturns: make([]DomainReturns, len(s.DomainReturns)),
		Masks:         make([]Mask, len(s.Masks)),
	}
	for i, d := range s.DomainReturns {
		d.Returns = append(Ranges(nil), d.Returns...)
		c.DomainReturns[i] = d
	}
	for i, m := range s.Masks {
		m.Members = append(Ranges(nil), m.Members...)
		m.Uncertain = append(Ranges(nil), m.Uncertain...)
		if m.Pose != nil {
			p := *m.Pose
			m.Pose = &p
		}
		c.Masks[i] = m
	}
	for i, w := range c.Windows {
		if w.LegacySampleID != nil {
			v := *w.LegacySampleID
			c.Windows[i].LegacySampleID = &v
		}
	}
	return c
}

// Canonicalise puts the state into its one permitted order. The packet table
// keeps its order: it is the return numbering.
func (s *State) Canonicalise() {
	sort.Slice(s.Records.Objects, func(i, j int) bool { return s.Records.Objects[i].ObjectID < s.Records.Objects[j].ObjectID })
	sort.Slice(s.Records.Correspondences, func(i, j int) bool {
		a, b := s.Records.Correspondences[i], s.Records.Correspondences[j]
		if a.ObjectID != b.ObjectID {
			return a.ObjectID < b.ObjectID
		}
		if a.RunID != b.RunID {
			return a.RunID < b.RunID
		}
		if a.StartNs != b.StartNs {
			return a.StartNs < b.StartNs
		}
		return a.EndNs < b.EndNs
	})
	sort.Strings(s.Records.DismissedProposals)
	sort.Slice(s.Domains, func(i, j int) bool { return s.Domains[i].DomainID < s.Domains[j].DomainID })
	sort.Slice(s.Windows, func(i, j int) bool { return s.Windows[i].WindowID < s.Windows[j].WindowID })
	sort.Slice(s.DomainReturns, func(i, j int) bool {
		a, b := s.DomainReturns[i], s.DomainReturns[j]
		if a.DomainID != b.DomainID {
			return a.DomainID < b.DomainID
		}
		return a.WindowID < b.WindowID
	})
	sort.Slice(s.Masks, func(i, j int) bool {
		a, b := s.Masks[i], s.Masks[j]
		if a.WindowID != b.WindowID {
			return a.WindowID < b.WindowID
		}
		return a.ObjectID < b.ObjectID
	})
	for i := range s.Masks {
		if len(s.Masks[i].Uncertain) == 0 {
			s.Masks[i].Uncertain = nil
		}
		if s.Masks[i].Members == nil {
			s.Masks[i].Members = Ranges{}
		}
	}
	for i := range s.DomainReturns {
		if s.DomainReturns[i].Returns == nil {
			s.DomainReturns[i].Returns = Ranges{}
		}
	}
	if s.Records.Objects == nil {
		s.Records.Objects = []annotation.Object{}
	}
	if s.Records.Correspondences == nil {
		s.Records.Correspondences = []annotation.TrackCorrespondence{}
	}
	if s.Records.DismissedProposals == nil {
		s.Records.DismissedProposals = []string{}
	}
}

type maskKey struct {
	object string
	window int
}

type domainWindow struct {
	domain string
	window int
}

// Validate checks the state against its own rules. It runs before every write
// and after every read: an invalid label set that loads produces plausible
// numbers, which is worse than one that refuses.
func (s *State) Validate(layout Layout) error {
	if err := layout.Validate(); err != nil {
		return err
	}
	keys := make(map[PacketKey]bool, len(s.Packets))
	for i, k := range s.Packets {
		if keys[k] {
			return fmt.Errorf("packet table lists %s twice (index %d)", k, i)
		}
		keys[k] = true
	}
	limit := uint64(len(s.Packets)) * layout.PerPacket()

	objects := make(map[string]annotation.ReviewStatus, len(s.Records.Objects))
	for _, o := range s.Records.Objects {
		if o.ObjectID == "" {
			return fmt.Errorf("an object has no id")
		}
		if _, dup := objects[o.ObjectID]; dup {
			return fmt.Errorf("object %q is declared twice", o.ObjectID)
		}
		if !validStatus(o.Status) {
			return fmt.Errorf("object %q has status %q", o.ObjectID, o.Status)
		}
		objects[o.ObjectID] = o.Status
	}
	for i, c := range s.Records.Correspondences {
		if _, ok := objects[c.ObjectID]; !ok {
			return fmt.Errorf("correspondence %d references unknown object %q", i, c.ObjectID)
		}
	}
	dismissed := make(map[string]bool, len(s.Records.DismissedProposals))
	for _, id := range s.Records.DismissedProposals {
		if id == "" || dismissed[id] {
			return fmt.Errorf("invalid or duplicate dismissed proposal %q", id)
		}
		dismissed[id] = true
	}

	domains := make(map[string]bool, len(s.Domains))
	for _, d := range s.Domains {
		if d.DomainID == "" || domains[d.DomainID] {
			return fmt.Errorf("invalid or duplicate domain %q", d.DomainID)
		}
		if d.Kind != DomainForeground {
			return fmt.Errorf("domain %q has kind %q: every domain is a foreground extraction", d.DomainID, d.Kind)
		}
		domains[d.DomainID] = true
	}

	windows := make(map[int]Window, len(s.Windows))
	byStart := append([]Window(nil), s.Windows...)
	for _, w := range s.Windows {
		if _, dup := windows[w.WindowID]; dup || w.WindowID < 0 {
			return fmt.Errorf("invalid or duplicate window %d", w.WindowID)
		}
		if w.Start >= w.End || w.End > limit {
			return fmt.Errorf("window %d spans [%d,%d), outside the %d returns the packet table numbers", w.WindowID, w.Start, w.End, limit)
		}
		windows[w.WindowID] = w
	}
	sort.Slice(byStart, func(i, j int) bool { return byStart[i].Start < byStart[j].Start })
	for i := 1; i < len(byStart); i++ {
		if byStart[i].Start < byStart[i-1].End {
			return fmt.Errorf("windows %d and %d overlap", byStart[i-1].WindowID, byStart[i].WindowID)
		}
	}

	domainReturns := make(map[domainWindow]Ranges, len(s.DomainReturns))
	for _, d := range s.DomainReturns {
		key := domainWindow{d.DomainID, d.WindowID}
		if _, dup := domainReturns[key]; dup {
			return fmt.Errorf("domain %q lists window %d twice", d.DomainID, d.WindowID)
		}
		if !domains[d.DomainID] {
			return fmt.Errorf("returns for unknown domain %q", d.DomainID)
		}
		w, ok := windows[d.WindowID]
		if !ok {
			return fmt.Errorf("domain %q returns cite unknown window %d", d.DomainID, d.WindowID)
		}
		if err := d.Returns.Validate(); err != nil {
			return fmt.Errorf("domain %q window %d: %w", d.DomainID, d.WindowID, err)
		}
		if d.Returns.Max() > w.Len() {
			return fmt.Errorf("domain %q window %d returns run past the window", d.DomainID, d.WindowID)
		}
		domainReturns[key] = d.Returns
	}

	seen := make(map[maskKey]bool, len(s.Masks))
	claimed := make(map[int]map[string]Ranges)
	for i, m := range s.Masks {
		key := maskKey{m.ObjectID, m.WindowID}
		if seen[key] {
			return fmt.Errorf("duplicate mask for object %q window %d", m.ObjectID, m.WindowID)
		}
		seen[key] = true
		if _, ok := objects[m.ObjectID]; !ok {
			return fmt.Errorf("mask %d references unknown object %q", i, m.ObjectID)
		}
		if _, ok := windows[m.WindowID]; !ok {
			return fmt.Errorf("mask %d (%s) references unknown window %d", i, m.ObjectID, m.WindowID)
		}
		if !validStatus(m.Status) || !validCompleteness(m.Completeness) || !validVisibility(m.Visibility) {
			return fmt.Errorf("mask %d (%s) has status %q, completeness %q, visibility %q", i, m.ObjectID, m.Status, m.Completeness, m.Visibility)
		}
		domain, ok := domainReturns[domainWindow{m.DomainID, m.WindowID}]
		if !ok {
			return fmt.Errorf("mask %d (%s window %d) cites domain %q, which has no returns in that window", i, m.ObjectID, m.WindowID, m.DomainID)
		}
		for name, set := range map[string]Ranges{"members": m.Members, "uncertain": m.Uncertain} {
			if err := set.Validate(); err != nil {
				return fmt.Errorf("mask %d (%s) %s: %w", i, m.ObjectID, name, err)
			}
			if outside := Difference(set, domain); len(outside) > 0 {
				return fmt.Errorf("mask %d (%s window %d) %s include return %d, which domain %q did not keep",
					i, m.ObjectID, m.WindowID, name, outside[0].Start, m.DomainID)
			}
		}
		if both := Intersect(m.Members, m.Uncertain); len(both) > 0 {
			return fmt.Errorf("mask %d (%s): return %d is both a member and uncertain", i, m.ObjectID, both[0].Start)
		}
		// A return may belong to at most one object in a window. Two objects
		// claiming the same return is a conflict for a person to resolve.
		if claimed[m.WindowID] == nil {
			claimed[m.WindowID] = make(map[string]Ranges)
		}
		for other, set := range claimed[m.WindowID] {
			if both := Intersect(set, m.Members); len(both) > 0 {
				return fmt.Errorf("window %d return %d is claimed by both %q and %q", m.WindowID, both[0].Start, other, m.ObjectID)
			}
		}
		claimed[m.WindowID][m.ObjectID] = m.Members
	}
	return nil
}

func validStatus(s annotation.ReviewStatus) bool {
	return s == annotation.StatusProposed || s == annotation.StatusReviewed || s == annotation.StatusRejected
}

func validCompleteness(c annotation.MaskCompleteness) bool {
	return c == annotation.MaskComplete || c == annotation.MaskPartial || c == annotation.MaskUnreviewed
}

func validVisibility(v annotation.Visibility) bool {
	switch v {
	case annotation.VisiblePresent, annotation.VisiblePartlyOccluded, annotation.VisibleFullyOccluded,
		annotation.VisibleOutsideView, annotation.VisibleUnknown:
		return true
	}
	return false
}

// ReviewedMasks returns only masks a person confirmed on a confirmed object,
// the accessor an evaluation must use, as in version 1.
func (s *State) ReviewedMasks() []Mask {
	status := make(map[string]annotation.ReviewStatus, len(s.Records.Objects))
	for _, o := range s.Records.Objects {
		status[o.ObjectID] = o.Status
	}
	out := make([]Mask, 0, len(s.Masks))
	for _, m := range s.Masks {
		if m.Status == annotation.StatusReviewed && status[m.ObjectID] == annotation.StatusReviewed {
			out = append(out, m)
		}
	}
	return out
}
