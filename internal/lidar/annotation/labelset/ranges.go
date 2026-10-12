package labelset

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Range is a run of consecutive return numbers, written [start, length].
type Range struct {
	Start uint64
	Len   uint64
}

// End is one past the last member.
func (r Range) End() uint64 { return r.Start + r.Len }

// MarshalJSON writes the two-element array form.
func (r Range) MarshalJSON() ([]byte, error) { return json.Marshal([2]uint64{r.Start, r.Len}) }

// UnmarshalJSON reads the two-element array form.
func (r *Range) UnmarshalJSON(b []byte) error {
	var a [2]uint64
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	r.Start, r.Len = a[0], a[1]
	return nil
}

// Ranges is a set of return numbers. Its canonical form is sorted, with every
// range non-empty and separated from the next by at least one non-member, so
// one set has exactly one encoding.
type Ranges []Range

// RangesOf builds the canonical set holding the given members, in any order
// and with repeats.
func RangesOf(members []uint64) Ranges {
	if len(members) == 0 {
		return nil
	}
	s := append([]uint64(nil), members...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	out := Ranges{{Start: s[0], Len: 1}}
	for _, m := range s[1:] {
		last := &out[len(out)-1]
		switch {
		case m < last.End():
		case m == last.End():
			last.Len++
		default:
			out = append(out, Range{Start: m, Len: 1})
		}
	}
	return out
}

// Validate refuses a set that is not in canonical form.
func (rs Ranges) Validate() error {
	for i, r := range rs {
		if r.Len == 0 {
			return fmt.Errorf("range %d is empty", i)
		}
		if r.End() < r.Start {
			return fmt.Errorf("range %d overflows", i)
		}
		if i > 0 && r.Start <= rs[i-1].End() {
			return fmt.Errorf("range %d starts at %d, not after the previous range's end %d", i, r.Start, rs[i-1].End())
		}
	}
	return nil
}

// Count is the number of members.
func (rs Ranges) Count() uint64 {
	var n uint64
	for _, r := range rs {
		n += r.Len
	}
	return n
}

// Contains reports membership.
func (rs Ranges) Contains(m uint64) bool {
	i := sort.Search(len(rs), func(i int) bool { return rs[i].End() > m })
	return i < len(rs) && rs[i].Start <= m
}

// Members lists every member in order.
func (rs Ranges) Members() []uint64 {
	out := make([]uint64, 0, rs.Count())
	for _, r := range rs {
		for m := r.Start; m < r.End(); m++ {
			out = append(out, m)
		}
	}
	return out
}

// Max is one past the largest member, or 0 for an empty set.
func (rs Ranges) Max() uint64 {
	if len(rs) == 0 {
		return 0
	}
	return rs[len(rs)-1].End()
}

// Union returns the members of either set.
func Union(a, b Ranges) Ranges { return merge(a, b, func(x, y bool) bool { return x || y }) }

// Intersect returns the members of both sets.
func Intersect(a, b Ranges) Ranges { return merge(a, b, func(x, y bool) bool { return x && y }) }

// Difference returns the members of a not in b.
func Difference(a, b Ranges) Ranges { return merge(a, b, func(x, y bool) bool { return x && !y }) }

// Equal reports whether two canonical sets are the same set.
func Equal(a, b Ranges) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// merge sweeps the boundaries of both sets and keeps the stretches where keep
// holds. Both inputs must be canonical; the result is.
func merge(a, b Ranges, keep func(inA, inB bool) bool) Ranges {
	var cuts []uint64
	for _, r := range a {
		cuts = append(cuts, r.Start, r.End())
	}
	for _, r := range b {
		cuts = append(cuts, r.Start, r.End())
	}
	if len(cuts) == 0 {
		return nil
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i] < cuts[j] })
	var out Ranges
	ia, ib := 0, 0
	for i := 0; i+1 < len(cuts); i++ {
		lo, hi := cuts[i], cuts[i+1]
		if lo == hi {
			continue
		}
		for ia < len(a) && a[ia].End() <= lo {
			ia++
		}
		for ib < len(b) && b[ib].End() <= lo {
			ib++
		}
		inA := ia < len(a) && a[ia].Start <= lo
		inB := ib < len(b) && b[ib].Start <= lo
		if !keep(inA, inB) {
			continue
		}
		if n := len(out); n > 0 && out[n-1].End() == lo {
			out[n-1].Len += hi - lo
		} else {
			out = append(out, Range{Start: lo, Len: hi - lo})
		}
	}
	return out
}
