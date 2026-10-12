package labelset

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// LayoutOf converts a pack's return layout.
func LayoutOf(p *annotation.Pack) (Layout, error) {
	if p.Manifest.ReturnLayout == nil {
		return Layout{}, fmt.Errorf("pack %s has no return layout", p.Dir)
	}
	l := p.Manifest.ReturnLayout
	return Layout{Model: l.Model, Blocks: l.Blocks, Channels: l.Channels}, nil
}

// DomainOf names the foreground extraction a pack holds: the run it was cut
// from, with that run's configuration and build.
func DomainOf(p *annotation.Pack) Domain {
	src := p.Manifest.Source
	return Domain{
		DomainID:     "fg-" + p.Manifest.PackDigest[7:19],
		Kind:         DomainForeground,
		RunID:        filepath.Base(src.VRLOGPath),
		RunConfigID:  src.RunConfigID,
		ConfigHash:   src.ConfigHash,
		ParamsHash:   src.ParamsHash,
		BuildVersion: src.BuildVersion,
		BuildGitSHA:  src.BuildGitSHA,
		HeightBand:   src.HeightBand,
		Note:         fmt.Sprintf("foreground of pack %s (%s)", p.Manifest.PackDigest, p.Manifest.Coverage),
	}
}

// ViewMap relates one version 2 pack, a view of one run, to a label set of
// the same capture. Returns are joined by packet key, never by coordinates.
type ViewMap struct {
	layout       Layout
	state        *State
	pack         *annotation.Pack
	windows      map[int]Window
	sampleWindow []int     // sample → window id, or -1
	offsets      [][]int64 // sample → point → offset in its window, or -1
}

// NewViewMap joins a pack to a label set state.
func NewViewMap(layout Layout, s *State, p *annotation.Pack) (*ViewMap, error) {
	pl, err := LayoutOf(p)
	if err != nil {
		return nil, err
	}
	if pl != layout {
		return nil, fmt.Errorf("pack layout %+v differs from label set layout %+v", pl, layout)
	}
	index := make(map[PacketKey]int, len(s.Packets))
	for i, k := range s.Packets {
		index[k] = i
	}
	toSet := func(viewReturn uint64) (uint64, bool) {
		vp, blk, ch := layout.Locate(viewReturn)
		li, ok := index[PacketKey(p.Packets[vp])]
		if !ok {
			return 0, false
		}
		r, _ := layout.Return(li, blk, ch)
		return r, true
	}
	byStart := append([]Window(nil), s.Windows...)
	sort.Slice(byStart, func(i, j int) bool { return byStart[i].Start < byStart[j].Start })
	windowOf := func(r uint64) (Window, bool) {
		i := sort.Search(len(byStart), func(i int) bool { return byStart[i].End > r })
		if i < len(byStart) && byStart[i].Start <= r {
			return byStart[i], true
		}
		return Window{}, false
	}
	vm := &ViewMap{
		layout: layout, state: s, pack: p,
		windows:      make(map[int]Window, len(s.Windows)),
		sampleWindow: make([]int, len(p.Samples)),
		offsets:      make([][]int64, len(p.Samples)),
	}
	for _, w := range s.Windows {
		vm.windows[w.WindowID] = w
	}
	for i, smp := range p.Samples {
		vm.sampleWindow[i] = -1
		returns, err := p.ReturnsAt(i)
		if err != nil {
			return nil, err
		}
		offs := make([]int64, len(returns))
		window := -1
		for j, vr := range returns {
			offs[j] = -1
			r, ok := toSet(vr)
			if !ok {
				continue
			}
			w, ok := windowOf(r)
			if !ok {
				continue
			}
			if window >= 0 && w.WindowID != window {
				return nil, fmt.Errorf("sample %d spans windows %d and %d: this run framed the capture differently", smp.SampleID, window, w.WindowID)
			}
			window = w.WindowID
			offs[j] = int64(r - w.Start)
		}
		vm.sampleWindow[i] = window
		vm.offsets[i] = offs
	}
	return vm, nil
}

// SampleWindow returns the window a sample shows, if the label set has one.
func (vm *ViewMap) SampleWindow(sampleID int) (int, bool) {
	if sampleID < 0 || sampleID >= len(vm.sampleWindow) || vm.sampleWindow[sampleID] < 0 {
		return 0, false
	}
	return vm.sampleWindow[sampleID], true
}

// WindowSample returns the sample that shows a window, if the pack has one.
func (vm *ViewMap) WindowSample(windowID int) (int, bool) {
	for i, w := range vm.sampleWindow {
		if w == windowID {
			return i, true
		}
	}
	return 0, false
}

// Shown returns the members of set, a window's offsets, that the sample
// showing that window displays, as point indices; and how many it does not.
func (vm *ViewMap) Shown(sampleID int, set Ranges) (indices []int, hidden uint64) {
	if sampleID < 0 || sampleID >= len(vm.offsets) {
		return nil, set.Count()
	}
	for j, off := range vm.offsets[sampleID] {
		if off >= 0 && set.Contains(uint64(off)) {
			indices = append(indices, j)
		}
	}
	return indices, set.Count() - uint64(len(indices))
}

// Offsets converts point indices of a sample to offsets in its window. An
// index whose return the label set does not number is refused.
func (vm *ViewMap) Offsets(sampleID int, indices []int) (Ranges, error) {
	if sampleID < 0 || sampleID >= len(vm.offsets) {
		return nil, fmt.Errorf("sample %d is outside the pack", sampleID)
	}
	offs := vm.offsets[sampleID]
	out := make([]uint64, 0, len(indices))
	for _, j := range indices {
		if j < 0 || j >= len(offs) {
			return nil, fmt.Errorf("point %d is outside sample %d", j, sampleID)
		}
		if offs[j] < 0 {
			return nil, fmt.Errorf("sample %d point %d is a return this label set does not number", sampleID, j)
		}
		out = append(out, uint64(offs[j]))
	}
	return RangesOf(out), nil
}

// ShownSet is every offset the sample displays.
func (vm *ViewMap) ShownSet(sampleID int) Ranges {
	var out []uint64
	for _, off := range vm.offsets[sampleID] {
		if off >= 0 {
			out = append(out, uint64(off))
		}
	}
	return RangesOf(out)
}

// Edit replaces what a set holds among the returns this sample shows,
// keeping the members it does not show. Editing through one run's view never
// drops a return another run's view labelled.
func (vm *ViewMap) Edit(sampleID int, set Ranges, shownIndices []int) (Ranges, error) {
	shown, err := vm.Offsets(sampleID, shownIndices)
	if err != nil {
		return nil, err
	}
	return Union(Difference(set, vm.ShownSet(sampleID)), shown), nil
}

// NewFromPack starts a label set from a version 2 pack: its packet table, one
// window per sample spanning the sample's frame, and the pack's foreground as
// the first domain.
func NewFromPack(p *annotation.Pack) (Layout, State, error) {
	layout, err := LayoutOf(p)
	if err != nil {
		return Layout{}, State{}, err
	}
	s := State{Packets: make([]PacketKey, len(p.Packets))}
	for i, k := range p.Packets {
		s.Packets[i] = PacketKey(k)
	}
	if _, err := AddPackDomain(layout, &s, p); err != nil {
		return Layout{}, State{}, err
	}
	return layout, s, nil
}

// AddPackDomain adds a pack's foreground to a label set of the same capture:
// packets it lacks are appended to the table, samples the label set has no
// window for become windows, and the pack's returns become a domain. A sample
// whose frame overlaps an existing window without matching it is reported
// and skipped: its run framed the capture differently.
func AddPackDomain(layout Layout, s *State, p *annotation.Pack) (skipped []int, err error) {
	pl, err := LayoutOf(p)
	if err != nil {
		return nil, err
	}
	if pl != layout {
		return nil, fmt.Errorf("pack layout %+v differs from label set layout %+v", pl, layout)
	}
	index := make(map[PacketKey]int, len(s.Packets)+len(p.Packets))
	for i, k := range s.Packets {
		index[k] = i
	}
	for _, k := range p.Packets {
		if _, ok := index[PacketKey(k)]; !ok {
			index[PacketKey(k)] = len(s.Packets)
			s.Packets = append(s.Packets, PacketKey(k))
		}
	}
	toSet := func(vr uint64) uint64 {
		vp, blk, ch := layout.Locate(vr)
		r, _ := layout.Return(index[PacketKey(p.Packets[vp])], blk, ch)
		return r
	}
	domain := DomainOf(p)
	for _, d := range s.Domains {
		if d.DomainID == domain.DomainID {
			return nil, fmt.Errorf("label set already holds pack %s's domain", p.Manifest.PackDigest)
		}
	}
	s.Domains = append(s.Domains, domain)
	nextID := 0
	for _, w := range s.Windows {
		if w.WindowID >= nextID {
			nextID = w.WindowID + 1
		}
	}
	per := layout.PerPacket()
	for i, smp := range p.Samples {
		// A frame is a window only if its packets are consecutive in the
		// label set's table, as they are in the pack's.
		firstPacket := int(smp.ReturnStart / per)
		base := index[PacketKey(p.Packets[firstPacket])]
		consecutive := true
		for vp := firstPacket; vp <= int((smp.ReturnEnd-1)/per); vp++ {
			if index[PacketKey(p.Packets[vp])] != base+(vp-firstPacket) {
				consecutive = false
				break
			}
		}
		start, last := toSet(smp.ReturnStart), toSet(smp.ReturnEnd-1)
		if !consecutive || last < start {
			skipped = append(skipped, i)
			continue
		}
		frame := Window{Start: start, End: last + 1, CaptureStartNs: smp.TimestampNs}
		var window *Window
		clash := false
		for k := range s.Windows {
			w := &s.Windows[k]
			if w.Start == frame.Start && w.End == frame.End {
				window = w
				break
			}
			if w.Start < frame.End && frame.Start < w.End {
				clash = true
			}
		}
		if window == nil && clash {
			skipped = append(skipped, i)
			continue
		}
		if window == nil {
			frame.WindowID = nextID
			nextID++
			s.Windows = append(s.Windows, frame)
			window = &s.Windows[len(s.Windows)-1]
		}
		returns, err := p.ReturnsAt(i)
		if err != nil {
			return nil, err
		}
		offs := make([]uint64, len(returns))
		for j, vr := range returns {
			r := toSet(vr)
			if r < window.Start || r >= window.End {
				return nil, fmt.Errorf("sample %d point %d falls outside its frame", i, j)
			}
			offs[j] = r - window.Start
		}
		s.DomainReturns = append(s.DomainReturns, DomainReturns{DomainID: domain.DomainID, WindowID: window.WindowID, Returns: RangesOf(offs)})
	}
	s.Canonicalise()
	return skipped, nil
}
