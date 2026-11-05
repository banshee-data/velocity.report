package labelwork

import (
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
)

// Count is one measured quantity: Done of Total, where Total is nil when
// nothing records it. An unknown total stays unknown: it is never zero and
// never the count of what happens to exist.
type Count struct {
	Name  string `json:"name"`
	Done  int    `json:"done"`
	Total *int   `json:"total"`
	Unit  string `json:"unit"`
}

func known(n int) *int { return &n }

// Layer statuses.
const (
	StatusNotStarted  = "not_started"
	StatusInProgress  = "in_progress"
	StatusComplete    = "complete"
	StatusUnsupported = "unsupported"
	StatusUnavailable = "unavailable"
)

// Where an operation on a layer can be done today.
const (
	WhereBrowser     = "browser"
	WhereNative      = "native"
	WhereCLI         = "cli"
	WhereUnsupported = "unsupported"
)

// LayerProgress is one progress layer's measures. Layers are never combined
// into one percentage: each says what it measures.
type LayerProgress struct {
	Layer  string  `json:"layer"`
	Title  string  `json:"title"`
	Status string  `json:"status"`
	Where  string  `json:"where"`
	Counts []Count `json:"counts"`
	Note   string  `json:"note,omitempty"`
}

func status(counts []Count) string {
	if len(counts) == 0 {
		return StatusUnavailable
	}
	if counts[0].Done == 0 {
		return StatusNotStarted
	}
	for _, c := range counts {
		if c.Total == nil || c.Done < *c.Total {
			return StatusInProgress
		}
	}
	return StatusComplete
}

// ObjectSpan is one reference object's span: the windows of its first and
// last non-rejected mask.
type ObjectSpan struct {
	ObjectID    string                  `json:"object_id"`
	Class       string                  `json:"class"`
	Status      annotation.ReviewStatus `json:"status"`
	FirstWindow int                     `json:"first_window"`
	LastWindow  int                     `json:"last_window"`
	// FirstNs and LastNs are the capture times of the span's first and last
	// windows, when the label set records them; zero when it does not.
	FirstNs       int64 `json:"first_ns,string"`
	LastNs        int64 `json:"last_ns,string"`
	Masks         int   `json:"masks"`
	ReviewedMasks int   `json:"reviewed_masks"`
	ProposedMasks int   `json:"proposed_masks"`
	// SpanWindows is how many of the label set's windows lie within the
	// span, the masks it could have.
	SpanWindows int  `json:"span_windows"`
	HasSpan     bool `json:"has_span"`
}

// LabelSetProgress is the membership layers of one label set, counted once
// however many runs or packs view it.
type LabelSetProgress struct {
	Objects         int          `json:"objects"`
	RejectedObjects int          `json:"rejected_objects"`
	ReviewedObjects int          `json:"reviewed_objects"`
	ProposedObjects int          `json:"proposed_objects"`
	Masks           int          `json:"masks"`
	RejectedMasks   int          `json:"rejected_masks"`
	ReviewedMasks   int          `json:"reviewed_masks"`
	ProposedMasks   int          `json:"proposed_masks"`
	TruthMasks      int          `json:"truth_masks"`
	Windows         int          `json:"windows"`
	Spans           []ObjectSpan `json:"-"`
	// FreezeBlockers are the objects a freeze would refuse, by the rule
	// FreezeSplit applies: not reviewed, no reviewed mask, a mask still
	// proposed, or a reviewed mask without completeness. The canonical
	// check is the freeze preview; this is the same rule, counted.
	FreezeBlockers []string        `json:"freeze_blockers"`
	Layers         []LayerProgress `json:"layers"`
}

// MembershipProgress reads a label set's state.
func MembershipProgress(s *labelset.State) LabelSetProgress {
	p := LabelSetProgress{Windows: len(s.Windows), FreezeBlockers: []string{}}
	spans := map[string]*ObjectSpan{}
	var order []string
	for _, o := range s.Records.Objects {
		if o.Status == annotation.StatusRejected {
			p.RejectedObjects++
			continue
		}
		p.Objects++
		switch o.Status {
		case annotation.StatusReviewed:
			p.ReviewedObjects++
		case annotation.StatusProposed:
			p.ProposedObjects++
		}
		spans[o.ObjectID] = &ObjectSpan{ObjectID: o.ObjectID, Class: o.Class, Status: o.Status}
		order = append(order, o.ObjectID)
	}
	windowIDs := make([]int, 0, len(s.Windows))
	windows := make(map[int]labelset.Window, len(s.Windows))
	for _, w := range s.Windows {
		windowIDs = append(windowIDs, w.WindowID)
		windows[w.WindowID] = w
	}
	sort.Ints(windowIDs)
	occlusionStated, occluded, incomplete := 0, 0, map[string]bool{}
	for _, m := range s.Masks {
		if m.Status == annotation.StatusRejected {
			p.RejectedMasks++
			continue
		}
		p.Masks++
		switch m.Status {
		case annotation.StatusReviewed:
			p.ReviewedMasks++
			if m.Completeness == "" {
				incomplete[m.ObjectID] = true
			}
		case annotation.StatusProposed:
			p.ProposedMasks++
		}
		if m.Visibility != "" && m.Visibility != annotation.VisibleUnknown {
			occlusionStated++
		}
		if m.Visibility == annotation.VisiblePartlyOccluded || m.Visibility == annotation.VisibleFullyOccluded {
			occluded++
		}
		sp := spans[m.ObjectID]
		if sp == nil {
			continue
		}
		if !sp.HasSpan || m.WindowID < sp.FirstWindow {
			sp.FirstWindow = m.WindowID
		}
		if !sp.HasSpan || m.WindowID > sp.LastWindow {
			sp.LastWindow = m.WindowID
		}
		sp.HasSpan = true
		sp.Masks++
		switch m.Status {
		case annotation.StatusReviewed:
			sp.ReviewedMasks++
		case annotation.StatusProposed:
			sp.ProposedMasks++
		}
	}
	p.TruthMasks = len(s.ReviewedMasks())
	withSpan, classed, spanWindows, masksInSpan := 0, 0, 0, 0
	for _, id := range order {
		sp := spans[id]
		if sp.Class != "" {
			classed++
		}
		if sp.HasSpan {
			withSpan++
			lo := sort.SearchInts(windowIDs, sp.FirstWindow)
			hi := sort.SearchInts(windowIDs, sp.LastWindow+1)
			sp.SpanWindows = hi - lo
			sp.FirstNs, sp.LastNs = windows[sp.FirstWindow].CaptureStartNs, windows[sp.LastWindow].CaptureEndNs
			spanWindows += sp.SpanWindows
			masksInSpan += sp.Masks
		}
		if sp.Status != annotation.StatusReviewed || sp.ReviewedMasks == 0 || sp.ProposedMasks > 0 || incomplete[id] {
			p.FreezeBlockers = append(p.FreezeBlockers, id)
		}
		p.Spans = append(p.Spans, *sp)
	}

	spansCounts := []Count{
		{Name: "road users recorded", Done: p.Objects, Total: nil, Unit: "objects"},
		{Name: "with a span", Done: withSpan, Total: known(p.Objects), Unit: "objects"},
		{Name: "class stated", Done: classed, Total: known(p.Objects), Unit: "objects"},
	}
	review := []Count{{Name: "objects reviewed", Done: p.ReviewedObjects, Total: known(p.Objects), Unit: "objects"}}
	masks := []Count{
		{Name: "masks recorded", Done: p.Masks, Total: nil, Unit: "masks"},
		{Name: "frames masked within spans", Done: masksInSpan, Total: known(spanWindows), Unit: "frames"},
	}
	maskReview := []Count{
		{Name: "masks reviewed", Done: p.ReviewedMasks, Total: known(p.Masks), Unit: "masks"},
		{Name: "usable as truth (reviewed mask on a reviewed object)", Done: p.TruthMasks, Total: known(p.Masks), Unit: "masks"},
	}
	occlusion := []Count{
		{Name: "visibility stated", Done: occlusionStated, Total: known(p.Masks), Unit: "masks"},
		{Name: "occluded", Done: occluded, Total: known(p.Masks), Unit: "masks"},
	}
	p.Layers = []LayerProgress{
		{Layer: "spans", Title: "Road-user spans, class and identity", Status: status(spansCounts), Where: WhereNative, Counts: spansCounts,
			Note: "The window's total road users is not recorded anywhere, so span coverage cannot be complete from the data: the operator says when every road user has a record."},
		{Layer: "object_review", Title: "Object review", Status: status(review), Where: WhereBrowser, Counts: review,
			Note: "Reviewing an object reviews its span, class and identity. It does not review its masks or any physical reference."},
		{Layer: "masks", Title: "Point-mask membership", Status: status(masks), Where: WhereNative, Counts: masks,
			Note: "A frame inside a span without a mask may be an occlusion rather than a gap."},
		{Layer: "mask_review", Title: "Mask review", Status: status(maskReview), Where: WhereNative, Counts: maskReview},
		{Layer: "occlusion", Title: "Occlusion and manoeuvre", Status: status(occlusion), Where: WhereNative, Counts: occlusion,
			Note: "Occlusion is the mask's visibility. Manoeuvre annotations are not supported by the label-set format."},
	}
	return p
}

// PhysicalProgress is one pack's physical-reference and following layers.
// Physical references live in the pack, not the label set, so they are
// counted per pack.
type PhysicalProgress struct {
	Exists            bool            `json:"exists"`
	Revision          int             `json:"revision"`
	Digest            string          `json:"digest,omitempty"`
	Bodies            int             `json:"bodies"`
	ReviewedBodies    int             `json:"reviewed_bodies"`
	IndependentBodies int             `json:"independent_bodies"`
	Keyframes         int             `json:"keyframes"`
	ReviewedKeyframes int             `json:"reviewed_keyframes"`
	ProposedRecords   []RecordRef     `json:"proposed_records"`
	Following         int             `json:"following"`
	ReviewedFollowing int             `json:"reviewed_following"`
	Stale             int             `json:"stale"`
	Layers            []LayerProgress `json:"layers"`
}

// RecordRef names one physical record a reviewer could review.
type RecordRef struct {
	Kind     string `json:"kind"`
	ObjectID string `json:"object_id,omitempty"`
	RecordID string `json:"record_id"`
}

// PhysicalReferenceProgress reads a pack's physical-reference document; doc
// is nil for a pack without one. reviewedObjects is the label set's reviewed
// objects, the bodies a reference could exist for.
func PhysicalReferenceProgress(doc *annotation.PhysicalReferenceSet, reviewedObjects int) PhysicalProgress {
	p := PhysicalProgress{ProposedRecords: []RecordRef{}}
	if doc != nil && doc.Digest() != "" {
		p.Exists, p.Revision, p.Digest = true, doc.Revision, doc.Digest()
		p.Stale = len(doc.Stale())
		for _, o := range doc.Objects {
			if o.Body != nil {
				p.Bodies++
				switch o.Body.Review.Status {
				case annotation.StatusReviewed:
					p.ReviewedBodies++
					if o.Body.Review.Origin == annotation.OriginIndependent {
						p.IndependentBodies++
					}
				case annotation.StatusProposed:
					p.ProposedRecords = append(p.ProposedRecords, RecordRef{Kind: "body", ObjectID: o.ObjectID, RecordID: o.Body.BodyID})
				}
			}
			for _, k := range o.Keyframes {
				p.Keyframes++
				switch k.Review.Status {
				case annotation.StatusReviewed:
					p.ReviewedKeyframes++
				case annotation.StatusProposed:
					p.ProposedRecords = append(p.ProposedRecords, RecordRef{Kind: "keyframe", ObjectID: o.ObjectID, RecordID: k.KeyframeID})
				}
			}
		}
		for _, f := range doc.Following {
			p.Following++
			switch f.Review.Status {
			case annotation.StatusReviewed:
				p.ReviewedFollowing++
			case annotation.StatusProposed:
				p.ProposedRecords = append(p.ProposedRecords, RecordRef{Kind: "following", RecordID: f.FollowingID})
			}
		}
	}
	physical := []Count{
		{Name: "reviewed bodies", Done: p.ReviewedBodies, Total: known(reviewedObjects), Unit: "reviewed objects"},
		{Name: "independent of the tracker", Done: p.IndependentBodies, Total: known(p.ReviewedBodies), Unit: "bodies"},
		{Name: "keyframes reviewed", Done: p.ReviewedKeyframes, Total: known(p.Keyframes), Unit: "keyframes"},
	}
	following := []Count{
		{Name: "following records", Done: p.Following, Total: nil, Unit: "records"},
		{Name: "reviewed", Done: p.ReviewedFollowing, Total: known(p.Following), Unit: "records"},
	}
	physicalNote := "Reviewing masks does not review physical geometry: each body and keyframe has its own review."
	if p.Stale > 0 {
		physicalNote += " Some records no longer hold against the membership and must be repaired before the next save."
	}
	p.Layers = []LayerProgress{
		{Layer: "physical", Title: "Physical body and pose references", Status: status(physical), Where: WhereNative, Counts: physical, Note: physicalNote},
		{Layer: "following", Title: "Following relationships", Status: status(following), Where: WhereNative, Counts: following,
			Note: "How many follower intervals need a record is not recorded until the operator drafts them, so the total is unknown."},
	}
	return p
}
