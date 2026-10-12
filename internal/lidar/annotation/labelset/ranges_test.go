package labelset

import (
	"encoding/json"
	"math/rand"
	"testing"
)

func setOf(rs Ranges) map[uint64]bool {
	out := map[uint64]bool{}
	for _, m := range rs.Members() {
		out[m] = true
	}
	return out
}

func randomSet(r *rand.Rand) []uint64 {
	n := r.Intn(40)
	out := make([]uint64, n)
	for i := range out {
		out[i] = uint64(r.Intn(60))
	}
	return out
}

func TestRangesOfIsCanonical(t *testing.T) {
	got := RangesOf([]uint64{5, 3, 4, 9, 4, 10, 0})
	want := Ranges{{0, 1}, {3, 3}, {9, 2}}
	if !Equal(got, want) {
		t.Fatalf("RangesOf = %v, want %v", got, want)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if got.Count() != 6 || !got.Contains(4) || got.Contains(6) || got.Max() != 11 {
		t.Fatalf("count %d contains(4) %v contains(6) %v max %d", got.Count(), got.Contains(4), got.Contains(6), got.Max())
	}
	for _, bad := range []Ranges{{{0, 0}}, {{0, 2}, {2, 1}}, {{5, 1}, {1, 1}}} {
		if bad.Validate() == nil {
			t.Fatalf("%v validated", bad)
		}
	}
}

func TestRangeAlgebraMatchesSets(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		a, b := RangesOf(randomSet(r)), RangesOf(randomSet(r))
		sa, sb := setOf(a), setOf(b)
		check := func(name string, got Ranges, keep func(x, y bool) bool) {
			t.Helper()
			if err := got.Validate(); err != nil {
				t.Fatalf("%s(%v,%v) = %v: %v", name, a, b, got, err)
			}
			var want []uint64
			for m := uint64(0); m < 64; m++ {
				if keep(sa[m], sb[m]) {
					want = append(want, m)
				}
			}
			if !Equal(got, RangesOf(want)) {
				t.Fatalf("%s(%v,%v) = %v, want %v", name, a, b, got, RangesOf(want))
			}
		}
		check("union", Union(a, b), func(x, y bool) bool { return x || y })
		check("intersect", Intersect(a, b), func(x, y bool) bool { return x && y })
		check("difference", Difference(a, b), func(x, y bool) bool { return x && !y })
	}
}

func TestRangesJSON(t *testing.T) {
	rs := Ranges{{3, 2}, {10, 1}}
	b, err := json.Marshal(rs)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[[3,2],[10,1]]" {
		t.Fatalf("encoded %s", b)
	}
	var back Ranges
	if err := json.Unmarshal(b, &back); err != nil || !Equal(back, rs) {
		t.Fatalf("round trip %v, %v", back, err)
	}
}

func TestLayoutRoundTrip(t *testing.T) {
	l := Pandar40P
	for _, c := range [][3]int{{0, 0, 1}, {0, 9, 40}, {7, 3, 17}, {123456, 5, 1}} {
		r, err := l.Return(c[0], c[1], c[2])
		if err != nil {
			t.Fatal(err)
		}
		p, b, ch := l.Locate(r)
		if p != c[0] || b != c[1] || ch != c[2] {
			t.Fatalf("Locate(Return%v) = %d,%d,%d", c, p, b, ch)
		}
	}
	if _, err := l.Return(0, 10, 1); err == nil {
		t.Fatal("block 10 accepted")
	}
	if _, err := l.Return(0, 0, 0); err == nil {
		t.Fatal("channel 0 accepted")
	}
}

func TestPacketKey(t *testing.T) {
	k := KeyOf([]byte("payload"))
	if k != KeyOf([]byte("payload")) || k == KeyOf([]byte("payloae")) {
		t.Fatal("key is not a function of the payload")
	}
	b, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	var back PacketKey
	if err := json.Unmarshal(b, &back); err != nil || back != k {
		t.Fatalf("round trip %s: %v %v", b, back, err)
	}
	if _, err := ParsePacketKey("xyz"); err == nil {
		t.Fatal("short key accepted")
	}
}
