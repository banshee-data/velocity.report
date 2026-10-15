package labelset

import (
	"fmt"
	"reflect"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// Change describes one save. The store supplies revision numbers and the UTC
// time; it never invents an author.
type Change struct {
	Author     string `json:"author"`
	Session    string `json:"session,omitempty"`
	Operation  string `json:"operation"`
	CreatedUTC string `json:"created_utc"`
}

// MaskFields are a mask's attributes other than its membership. A delta
// carries them whole whenever any of them changes, since they are small.
type MaskFields struct {
	DomainID     string                      `json:"domain_id"`
	Completeness annotation.MaskCompleteness `json:"completeness"`
	Visibility   annotation.Visibility       `json:"visibility"`
	Status       annotation.ReviewStatus     `json:"status"`
	Pose         *annotation.Pose            `json:"pose,omitempty"`
	Provenance   annotation.Provenance       `json:"provenance"`
}

func fieldsOf(m Mask) MaskFields {
	return MaskFields{m.DomainID, m.Completeness, m.Visibility, m.Status, m.Pose, m.Provenance}
}

// MaskDelta changes one mask. Membership moves by additions and removals, so
// a save that touches one return of one mask records one return.
type MaskDelta struct {
	ObjectID        string      `json:"object_id"`
	WindowID        int         `json:"window_id"`
	Delete          bool        `json:"delete,omitempty"`
	Fields          *MaskFields `json:"fields,omitempty"`
	MembersAdd      Ranges      `json:"members_add,omitempty"`
	MembersRemove   Ranges      `json:"members_remove,omitempty"`
	UncertainAdd    Ranges      `json:"uncertain_add,omitempty"`
	UncertainRemove Ranges      `json:"uncertain_remove,omitempty"`
}

// DomainReturnsKey names one domain's returns in one window.
type DomainReturnsKey struct {
	DomainID string `json:"domain_id"`
	WindowID int    `json:"window_id"`
}

// Delta is one journal line: the change from the parent state to State.
// Records other than masks are replaced whole when they change.
type Delta struct {
	Revision       int    `json:"revision"`
	ParentRevision int    `json:"parent_revision"`
	ParentState    string `json:"parent_state"`
	State          string `json:"state"`
	Change         Change `json:"change"`
	RestoredFrom   int    `json:"restored_from,omitempty"`
	// Base marks a journal that starts from a head it did not record, such as
	// one published without history. Its state is kept in journal-base/.
	Base bool `json:"base,omitempty"`

	PacketsAppend       []PacketKey                       `json:"packets_append,omitempty"`
	ObjectsPut          []annotation.Object               `json:"objects_put,omitempty"`
	ObjectsDelete       []string                          `json:"objects_delete,omitempty"`
	Correspondences     *[]annotation.TrackCorrespondence `json:"correspondences,omitempty"`
	DismissedAdd        []string                          `json:"dismissed_add,omitempty"`
	DismissedRemove     []string                          `json:"dismissed_remove,omitempty"`
	DomainsPut          []Domain                          `json:"domains_put,omitempty"`
	DomainsDelete       []string                          `json:"domains_delete,omitempty"`
	WindowsPut          []Window                          `json:"windows_put,omitempty"`
	WindowsDelete       []int                             `json:"windows_delete,omitempty"`
	DomainReturnsPut    []DomainReturns                   `json:"domain_returns_put,omitempty"`
	DomainReturnsDelete []DomainReturnsKey                `json:"domain_returns_delete,omitempty"`
	Masks               []MaskDelta                       `json:"masks,omitempty"`
}

// Empty reports whether the delta changes nothing.
func (d *Delta) Empty() bool {
	return len(d.PacketsAppend) == 0 && len(d.ObjectsPut) == 0 && len(d.ObjectsDelete) == 0 &&
		d.Correspondences == nil && len(d.DismissedAdd) == 0 && len(d.DismissedRemove) == 0 &&
		len(d.DomainsPut) == 0 && len(d.DomainsDelete) == 0 && len(d.WindowsPut) == 0 &&
		len(d.WindowsDelete) == 0 && len(d.DomainReturnsPut) == 0 && len(d.DomainReturnsDelete) == 0 &&
		len(d.Masks) == 0
}

// Diff computes the delta from base to next. Both must be canonical. The
// packet table may only grow at its end: return numbers are positions in it,
// so any other change would renumber every return.
func Diff(base, next *State) (*Delta, error) {
	d := &Delta{}
	if len(next.Packets) < len(base.Packets) {
		return nil, fmt.Errorf("packet table shrank from %d to %d: the table is append-only", len(base.Packets), len(next.Packets))
	}
	for i, k := range base.Packets {
		if next.Packets[i] != k {
			return nil, fmt.Errorf("packet %d changed: the table is append-only", i)
		}
	}
	d.PacketsAppend = append([]PacketKey(nil), next.Packets[len(base.Packets):]...)

	baseObjects := make(map[string]annotation.Object, len(base.Records.Objects))
	for _, o := range base.Records.Objects {
		baseObjects[o.ObjectID] = o
	}
	nextObjects := make(map[string]bool, len(next.Records.Objects))
	for _, o := range next.Records.Objects {
		nextObjects[o.ObjectID] = true
		if prev, ok := baseObjects[o.ObjectID]; !ok || prev != o {
			d.ObjectsPut = append(d.ObjectsPut, o)
		}
	}
	for _, o := range base.Records.Objects {
		if !nextObjects[o.ObjectID] {
			d.ObjectsDelete = append(d.ObjectsDelete, o.ObjectID)
		}
	}
	if !sameCorrespondences(base.Records.Correspondences, next.Records.Correspondences) {
		c := append([]annotation.TrackCorrespondence{}, next.Records.Correspondences...)
		d.Correspondences = &c
	}
	d.DismissedAdd, d.DismissedRemove = stringDiff(base.Records.DismissedProposals, next.Records.DismissedProposals)

	baseDomains := make(map[string]Domain, len(base.Domains))
	for _, x := range base.Domains {
		baseDomains[x.DomainID] = x
	}
	nextDomains := make(map[string]bool, len(next.Domains))
	for _, x := range next.Domains {
		nextDomains[x.DomainID] = true
		if prev, ok := baseDomains[x.DomainID]; !ok || !reflect.DeepEqual(prev, x) {
			d.DomainsPut = append(d.DomainsPut, x)
		}
	}
	for _, x := range base.Domains {
		if !nextDomains[x.DomainID] {
			d.DomainsDelete = append(d.DomainsDelete, x.DomainID)
		}
	}

	baseWindows := make(map[int]Window, len(base.Windows))
	for _, w := range base.Windows {
		baseWindows[w.WindowID] = w
	}
	nextWindows := make(map[int]bool, len(next.Windows))
	for _, w := range next.Windows {
		nextWindows[w.WindowID] = true
		if prev, ok := baseWindows[w.WindowID]; !ok || !reflect.DeepEqual(prev, w) {
			d.WindowsPut = append(d.WindowsPut, w)
		}
	}
	for _, w := range base.Windows {
		if !nextWindows[w.WindowID] {
			d.WindowsDelete = append(d.WindowsDelete, w.WindowID)
		}
	}

	baseReturns := make(map[DomainReturnsKey]Ranges, len(base.DomainReturns))
	for _, x := range base.DomainReturns {
		baseReturns[DomainReturnsKey{x.DomainID, x.WindowID}] = x.Returns
	}
	nextReturns := make(map[DomainReturnsKey]bool, len(next.DomainReturns))
	for _, x := range next.DomainReturns {
		key := DomainReturnsKey{x.DomainID, x.WindowID}
		nextReturns[key] = true
		if prev, ok := baseReturns[key]; !ok || !Equal(prev, x.Returns) {
			d.DomainReturnsPut = append(d.DomainReturnsPut, x)
		}
	}
	for _, x := range base.DomainReturns {
		key := DomainReturnsKey{x.DomainID, x.WindowID}
		if !nextReturns[key] {
			d.DomainReturnsDelete = append(d.DomainReturnsDelete, key)
		}
	}

	baseMasks := make(map[maskKey]Mask, len(base.Masks))
	for _, m := range base.Masks {
		baseMasks[maskKey{m.ObjectID, m.WindowID}] = m
	}
	nextMasks := make(map[maskKey]bool, len(next.Masks))
	for _, m := range next.Masks {
		key := maskKey{m.ObjectID, m.WindowID}
		nextMasks[key] = true
		prev, existed := baseMasks[key]
		md := MaskDelta{ObjectID: m.ObjectID, WindowID: m.WindowID}
		if !existed || !reflect.DeepEqual(fieldsOf(prev), fieldsOf(m)) {
			f := fieldsOf(m)
			md.Fields = &f
		}
		md.MembersAdd = Difference(m.Members, prev.Members)
		md.MembersRemove = Difference(prev.Members, m.Members)
		md.UncertainAdd = Difference(m.Uncertain, prev.Uncertain)
		md.UncertainRemove = Difference(prev.Uncertain, m.Uncertain)
		if md.Fields != nil || len(md.MembersAdd)+len(md.MembersRemove)+len(md.UncertainAdd)+len(md.UncertainRemove) > 0 {
			d.Masks = append(d.Masks, md)
		}
	}
	for _, m := range base.Masks {
		if !nextMasks[maskKey{m.ObjectID, m.WindowID}] {
			d.Masks = append(d.Masks, MaskDelta{ObjectID: m.ObjectID, WindowID: m.WindowID, Delete: true})
		}
	}
	return d, nil
}

func sameCorrespondences(a, b []annotation.TrackCorrespondence) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func stringDiff(base, next []string) (add, remove []string) {
	inBase := make(map[string]bool, len(base))
	for _, s := range base {
		inBase[s] = true
	}
	inNext := make(map[string]bool, len(next))
	for _, s := range next {
		inNext[s] = true
		if !inBase[s] {
			add = append(add, s)
		}
	}
	for _, s := range base {
		if !inNext[s] {
			remove = append(remove, s)
		}
	}
	return add, remove
}

// Apply returns base with the delta applied, canonicalised. It refuses a delta
// that does not fit: removing what is absent, adding what is present, or
// changing a mask that does not exist without its fields.
func Apply(base *State, d *Delta) (*State, error) {
	s := base.Clone()
	s.Packets = append(s.Packets, d.PacketsAppend...)

	objects := make(map[string]int, len(s.Records.Objects))
	for i, o := range s.Records.Objects {
		objects[o.ObjectID] = i
	}
	for _, id := range d.ObjectsDelete {
		if _, ok := objects[id]; !ok {
			return nil, fmt.Errorf("delta deletes unknown object %q", id)
		}
	}
	s.Records.Objects = filter(s.Records.Objects, func(o annotation.Object) bool { return !contains(d.ObjectsDelete, o.ObjectID) })
	objects = make(map[string]int, len(s.Records.Objects))
	for i, o := range s.Records.Objects {
		objects[o.ObjectID] = i
	}
	for _, o := range d.ObjectsPut {
		if i, ok := objects[o.ObjectID]; ok {
			s.Records.Objects[i] = o
		} else {
			s.Records.Objects = append(s.Records.Objects, o)
			objects[o.ObjectID] = len(s.Records.Objects) - 1
		}
	}
	if d.Correspondences != nil {
		s.Records.Correspondences = append([]annotation.TrackCorrespondence{}, (*d.Correspondences)...)
	}
	for _, id := range d.DismissedRemove {
		if !contains(s.Records.DismissedProposals, id) {
			return nil, fmt.Errorf("delta un-dismisses %q, which was not dismissed", id)
		}
	}
	s.Records.DismissedProposals = filter(s.Records.DismissedProposals, func(id string) bool { return !contains(d.DismissedRemove, id) })
	for _, id := range d.DismissedAdd {
		if contains(s.Records.DismissedProposals, id) {
			return nil, fmt.Errorf("delta dismisses %q twice", id)
		}
		s.Records.DismissedProposals = append(s.Records.DismissedProposals, id)
	}

	s.Domains = filter(s.Domains, func(x Domain) bool { return !contains(d.DomainsDelete, x.DomainID) })
	for _, x := range d.DomainsPut {
		s.Domains = filter(s.Domains, func(y Domain) bool { return y.DomainID != x.DomainID })
		s.Domains = append(s.Domains, x)
	}
	s.Windows = filter(s.Windows, func(w Window) bool { return !containsInt(d.WindowsDelete, w.WindowID) })
	for _, w := range d.WindowsPut {
		s.Windows = filter(s.Windows, func(y Window) bool { return y.WindowID != w.WindowID })
		s.Windows = append(s.Windows, w)
	}
	s.DomainReturns = filter(s.DomainReturns, func(x DomainReturns) bool {
		for _, k := range d.DomainReturnsDelete {
			if k.DomainID == x.DomainID && k.WindowID == x.WindowID {
				return false
			}
		}
		return true
	})
	for _, x := range d.DomainReturnsPut {
		s.DomainReturns = filter(s.DomainReturns, func(y DomainReturns) bool {
			return y.DomainID != x.DomainID || y.WindowID != x.WindowID
		})
		x.Returns = append(Ranges(nil), x.Returns...)
		s.DomainReturns = append(s.DomainReturns, x)
	}

	masks := make(map[maskKey]int, len(s.Masks))
	for i, m := range s.Masks {
		masks[maskKey{m.ObjectID, m.WindowID}] = i
	}
	deleted := make(map[maskKey]bool)
	for _, md := range d.Masks {
		key := maskKey{md.ObjectID, md.WindowID}
		i, ok := masks[key]
		if md.Delete {
			if !ok || deleted[key] {
				return nil, fmt.Errorf("delta deletes absent mask %s window %d", md.ObjectID, md.WindowID)
			}
			deleted[key] = true
			continue
		}
		if !ok {
			if md.Fields == nil {
				return nil, fmt.Errorf("delta creates mask %s window %d without its fields", md.ObjectID, md.WindowID)
			}
			s.Masks = append(s.Masks, Mask{ObjectID: md.ObjectID, WindowID: md.WindowID})
			i = len(s.Masks) - 1
			masks[key] = i
		}
		m := &s.Masks[i]
		if md.Fields != nil {
			f := *md.Fields
			m.DomainID, m.Completeness, m.Visibility, m.Status, m.Pose, m.Provenance = f.DomainID, f.Completeness, f.Visibility, f.Status, f.Pose, f.Provenance
		}
		var err error
		if m.Members, err = applySet(m.Members, md.MembersAdd, md.MembersRemove); err != nil {
			return nil, fmt.Errorf("mask %s window %d members: %w", md.ObjectID, md.WindowID, err)
		}
		if m.Uncertain, err = applySet(m.Uncertain, md.UncertainAdd, md.UncertainRemove); err != nil {
			return nil, fmt.Errorf("mask %s window %d uncertain: %w", md.ObjectID, md.WindowID, err)
		}
	}
	if len(deleted) > 0 {
		s.Masks = filter(s.Masks, func(m Mask) bool { return !deleted[maskKey{m.ObjectID, m.WindowID}] })
	}
	s.Canonicalise()
	return &s, nil
}

func applySet(set, add, remove Ranges) (Ranges, error) {
	if absent := Difference(remove, set); len(absent) > 0 {
		return nil, fmt.Errorf("removes return %d, which is not present", absent[0].Start)
	}
	if present := Intersect(add, set); len(present) > 0 {
		return nil, fmt.Errorf("adds return %d, which is already present", present[0].Start)
	}
	return Union(Difference(set, remove), add), nil
}

func filter[T any](xs []T, keep func(T) bool) []T {
	out := xs[:0:0]
	for _, x := range xs {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
