package labelset

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// What a change to a label set did, for a person reading a pull request.
//
// Summarise compares two states and says, in the label set's own terms,
// which objects and masks a change added, removed or altered. It is what
// 'velocity lidar labels diff' prints: the ndjson diff Git shows is exact
// but says "line 41 of masks/000200-000299.ndjson", not "obj_7's mask in
// window 214 lost three returns".

// ObjectChange is one object added, removed or altered.
type ObjectChange struct {
	ObjectID string `json:"object_id"`
	Class    string `json:"class"`
	// Fields lists what changed on an altered object: class, subtype,
	// status, confidence, notes.
	Fields []string `json:"fields,omitempty"`
}

// MaskChange is one object's mask in one window and domain, added, removed
// or altered.
type MaskChange struct {
	ObjectID string `json:"object_id"`
	WindowID int    `json:"window_id"`
	DomainID string `json:"domain_id"`
	// Members is the mask's member count after the change; for a removed
	// mask, before it.
	Members uint64 `json:"members"`
	// Gained and Lost count members an altered mask gained and lost.
	Gained uint64 `json:"gained,omitempty"`
	Lost   uint64 `json:"lost,omitempty"`
	// Fields lists what else changed: status, completeness, visibility,
	// uncertain, pose.
	Fields []string `json:"fields,omitempty"`
}

// Summary is what a change did.
type Summary struct {
	Name           string         `json:"name,omitempty"`
	From           Head           `json:"from"`
	To             Head           `json:"to"`
	ObjectsAdded   []ObjectChange `json:"objects_added,omitempty"`
	ObjectsRemoved []ObjectChange `json:"objects_removed,omitempty"`
	ObjectsChanged []ObjectChange `json:"objects_changed,omitempty"`
	MasksAdded     []MaskChange   `json:"masks_added,omitempty"`
	MasksRemoved   []MaskChange   `json:"masks_removed,omitempty"`
	MasksChanged   []MaskChange   `json:"masks_changed,omitempty"`
	DomainsAdded   []string       `json:"domains_added,omitempty"`
	DomainsRemoved []string       `json:"domains_removed,omitempty"`
	WindowsAdded   int            `json:"windows_added,omitempty"`
	WindowsRemoved int            `json:"windows_removed,omitempty"`
	PacketsAdded   int            `json:"packets_added,omitempty"`
	PacketsRemoved int            `json:"packets_removed,omitempty"`
	// OtherRecords names record lists that changed and are not itemised:
	// correspondences, dismissed proposals.
	OtherRecords []string `json:"other_records,omitempty"`
}

// Empty reports whether nothing changed.
func (s *Summary) Empty() bool {
	return len(s.ObjectsAdded)+len(s.ObjectsRemoved)+len(s.ObjectsChanged)+
		len(s.MasksAdded)+len(s.MasksRemoved)+len(s.MasksChanged)+
		len(s.DomainsAdded)+len(s.DomainsRemoved)+len(s.OtherRecords) == 0 &&
		s.WindowsAdded+s.WindowsRemoved+s.PacketsAdded+s.PacketsRemoved == 0
}

// Summarise compares two states. Neither is changed.
func Summarise(from, to *State) *Summary {
	s := &Summary{}
	a, b := from.Clone(), to.Clone()
	a.Canonicalise()
	b.Canonicalise()

	objects := func(st *State) map[string]annotation.Object {
		out := map[string]annotation.Object{}
		for _, o := range st.Records.Objects {
			out[o.ObjectID] = o
		}
		return out
	}
	was, now := objects(&a), objects(&b)
	for _, id := range unionKeys(was, now) {
		o, inWas := was[id]
		n, inNow := now[id]
		switch {
		case !inWas:
			s.ObjectsAdded = append(s.ObjectsAdded, ObjectChange{ObjectID: id, Class: n.Class})
		case !inNow:
			s.ObjectsRemoved = append(s.ObjectsRemoved, ObjectChange{ObjectID: id, Class: o.Class})
		default:
			var fields []string
			add := func(name string, changed bool) {
				if changed {
					fields = append(fields, name)
				}
			}
			add("class", o.Class != n.Class)
			add("subtype", o.Subtype != n.Subtype)
			add("status", o.Status != n.Status)
			add("confidence", o.Confidence != n.Confidence)
			add("notes", o.Notes != n.Notes)
			if len(fields) > 0 {
				s.ObjectsChanged = append(s.ObjectsChanged, ObjectChange{ObjectID: id, Class: n.Class, Fields: fields})
			}
		}
	}

	type key struct {
		object string
		window int
		domain string
	}
	masks := func(st *State) map[key]Mask {
		out := map[key]Mask{}
		for _, m := range st.Masks {
			out[key{m.ObjectID, m.WindowID, m.DomainID}] = m
		}
		return out
	}
	mw, mn := masks(&a), masks(&b)
	keys := make([]key, 0, len(mw)+len(mn))
	for k := range mw {
		keys = append(keys, k)
	}
	for k := range mn {
		if _, ok := mw[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].window != keys[j].window {
			return keys[i].window < keys[j].window
		}
		if keys[i].object != keys[j].object {
			return keys[i].object < keys[j].object
		}
		return keys[i].domain < keys[j].domain
	})
	for _, k := range keys {
		o, inWas := mw[k]
		n, inNow := mn[k]
		c := MaskChange{ObjectID: k.object, WindowID: k.window, DomainID: k.domain}
		switch {
		case !inWas:
			c.Members = n.Members.Count()
			s.MasksAdded = append(s.MasksAdded, c)
		case !inNow:
			c.Members = o.Members.Count()
			s.MasksRemoved = append(s.MasksRemoved, c)
		default:
			c.Members = n.Members.Count()
			c.Gained = Difference(n.Members, o.Members).Count()
			c.Lost = Difference(o.Members, n.Members).Count()
			add := func(name string, changed bool) {
				if changed {
					c.Fields = append(c.Fields, name)
				}
			}
			add("status", o.Status != n.Status)
			add("completeness", o.Completeness != n.Completeness)
			add("visibility", o.Visibility != n.Visibility)
			add("uncertain", !Equal(o.Uncertain, n.Uncertain))
			add("pose", !reflect.DeepEqual(o.Pose, n.Pose))
			if c.Gained+c.Lost > 0 || len(c.Fields) > 0 {
				s.MasksChanged = append(s.MasksChanged, c)
			}
		}
	}

	domains := func(st *State) map[string]bool {
		out := map[string]bool{}
		for _, d := range st.Domains {
			out[d.DomainID] = true
		}
		return out
	}
	dw, dn := domains(&a), domains(&b)
	for _, id := range unionKeys(dw, dn) {
		switch {
		case !dw[id]:
			s.DomainsAdded = append(s.DomainsAdded, id)
		case !dn[id]:
			s.DomainsRemoved = append(s.DomainsRemoved, id)
		}
	}
	ww, wn := map[int]bool{}, map[int]bool{}
	for _, w := range a.Windows {
		ww[w.WindowID] = true
	}
	for _, w := range b.Windows {
		wn[w.WindowID] = true
		if !ww[w.WindowID] {
			s.WindowsAdded++
		}
	}
	for id := range ww {
		if !wn[id] {
			s.WindowsRemoved++
		}
	}
	pw, pn := map[PacketKey]bool{}, map[PacketKey]bool{}
	for _, p := range a.Packets {
		pw[p] = true
	}
	for _, p := range b.Packets {
		pn[p] = true
		if !pw[p] {
			s.PacketsAdded++
		}
	}
	for p := range pw {
		if !pn[p] {
			s.PacketsRemoved++
		}
	}
	if !sameCorrespondences(a.Records.Correspondences, b.Records.Correspondences) {
		s.OtherRecords = append(s.OtherRecords, "correspondences")
	}
	if !reflect.DeepEqual(a.Records.DismissedProposals, b.Records.DismissedProposals) {
		s.OtherRecords = append(s.OtherRecords, "dismissed proposals")
	}
	return s
}

func unionKeys[V any](a, b map[string]V) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string]V{a, b} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// windowSpan names a run of windows: "window 214" or "windows 200–299".
func windowSpan(first, last int) string {
	if first == last {
		return fmt.Sprintf("window %d", first)
	}
	return fmt.Sprintf("windows %d–%d", first, last)
}

// maskLines groups mask changes by object and describes each group: how
// many masks, over which windows, and the returns gained and lost.
func maskLines(changes []MaskChange, verb string) []string {
	type group struct {
		object       string
		first, last  int
		n            int
		gained, lost uint64
		members      uint64
		fields       map[string]bool
	}
	byObject := map[string]*group{}
	var order []string
	for _, c := range changes {
		g := byObject[c.ObjectID]
		if g == nil {
			g = &group{object: c.ObjectID, first: c.WindowID, last: c.WindowID, fields: map[string]bool{}}
			byObject[c.ObjectID] = g
			order = append(order, c.ObjectID)
		}
		if c.WindowID < g.first {
			g.first = c.WindowID
		}
		if c.WindowID > g.last {
			g.last = c.WindowID
		}
		g.n++
		g.gained += c.Gained
		g.lost += c.Lost
		g.members += c.Members
		for _, f := range c.Fields {
			g.fields[f] = true
		}
	}
	sort.Strings(order)
	var out []string
	for _, id := range order {
		g := byObject[id]
		line := fmt.Sprintf("%s: %d mask%s %s, %s", g.object, g.n, plural(g.n), verb, windowSpan(g.first, g.last))
		switch verb {
		case "changed":
			line += fmt.Sprintf(" (+%d −%d returns", g.gained, g.lost)
			if len(g.fields) > 0 {
				line += "; " + strings.Join(sortedKeys(g.fields), ", ")
			}
			line += ")"
		default:
			line += fmt.Sprintf(" (%d returns)", g.members)
		}
		out = append(out, line)
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func objectLine(o ObjectChange) string {
	if len(o.Fields) > 0 {
		return fmt.Sprintf("%s (%s): %s", o.ObjectID, o.Class, strings.Join(o.Fields, ", "))
	}
	return fmt.Sprintf("%s (%s)", o.ObjectID, o.Class)
}

// sections are the summary's headed lists, in reading order.
func (s *Summary) sections() []struct {
	title string
	lines []string
} {
	var out []struct {
		title string
		lines []string
	}
	add := func(title string, lines []string) {
		if len(lines) > 0 {
			out = append(out, struct {
				title string
				lines []string
			}{title, lines})
		}
	}
	objects := func(cs []ObjectChange) []string {
		var lines []string
		for _, c := range cs {
			lines = append(lines, objectLine(c))
		}
		return lines
	}
	add("Objects added", objects(s.ObjectsAdded))
	add("Objects removed", objects(s.ObjectsRemoved))
	add("Objects changed", objects(s.ObjectsChanged))
	add("Masks added", maskLines(s.MasksAdded, "added"))
	add("Masks removed", maskLines(s.MasksRemoved, "removed"))
	add("Masks changed", maskLines(s.MasksChanged, "changed"))
	var other []string
	for _, d := range s.DomainsAdded {
		other = append(other, "domain added: "+d)
	}
	for _, d := range s.DomainsRemoved {
		other = append(other, "domain removed: "+d)
	}
	if s.WindowsAdded+s.WindowsRemoved > 0 {
		other = append(other, fmt.Sprintf("windows: +%d −%d", s.WindowsAdded, s.WindowsRemoved))
	}
	if s.PacketsAdded+s.PacketsRemoved > 0 {
		other = append(other, fmt.Sprintf("packets: +%d −%d", s.PacketsAdded, s.PacketsRemoved))
	}
	for _, r := range s.OtherRecords {
		other = append(other, r+" changed")
	}
	add("Also", other)
	return out
}

func (s *Summary) heading() string {
	name := s.Name
	if name == "" {
		name = "label set"
	}
	return fmt.Sprintf("%s: revision %d → %d", name, s.From.Revision, s.To.Revision)
}

// Text is the summary for a terminal.
func (s *Summary) Text() string {
	var b strings.Builder
	b.WriteString(s.heading() + "\n")
	if s.Empty() {
		b.WriteString("no label changed\n")
		return b.String()
	}
	for _, sec := range s.sections() {
		b.WriteString("\n" + sec.title + ":\n")
		for _, l := range sec.lines {
			b.WriteString("  " + l + "\n")
		}
	}
	return b.String()
}

// Markdown is the summary for a pull request description.
func (s *Summary) Markdown() string {
	var b strings.Builder
	b.WriteString("### " + s.heading() + "\n\n")
	if s.Empty() {
		b.WriteString("No label changed.\n")
		return b.String()
	}
	b.WriteString("|         | Added | Removed | Changed |\n| ------- | ----: | ------: | ------: |\n")
	fmt.Fprintf(&b, "| Objects | %d | %d | %d |\n", len(s.ObjectsAdded), len(s.ObjectsRemoved), len(s.ObjectsChanged))
	fmt.Fprintf(&b, "| Masks   | %d | %d | %d |\n", len(s.MasksAdded), len(s.MasksRemoved), len(s.MasksChanged))
	for _, sec := range s.sections() {
		b.WriteString("\n**" + sec.title + "**\n\n")
		for _, l := range sec.lines {
			b.WriteString("- " + l + "\n")
		}
	}
	return b.String()
}
