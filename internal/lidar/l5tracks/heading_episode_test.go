package l5tracks

import "testing"

func TestTerminalHeadingEpisodes(t *testing.T) {
	for _, tc := range []struct {
		seq      string
		outcome  string
		episodes int
	}{
		{"UUU", "none", 0}, {"LLLLL", "unrecovered", 1},
		{"LLLLLU", "censored", 1}, {"LLLLLUUUUU", "recovered", 1},
		{"LLLLLUUUUULLLLL", "unrecovered", 2},
		{"LLLLLUULLLLLUUUUU", "recovered", 2},
	} {
		var s HeadingEpisodeState
		for i, c := range tc.seq {
			src := HeadingSourceAxis
			if c == 'L' {
				src = HeadingSourceAmbiguous
			}
			s.Observe(src, int64(i+1)*200000000)
		}
		if s.Outcome() != tc.outcome || s.Episodes != tc.episodes {
			t.Fatalf("%s: %s/%d", tc.seq, s.Outcome(), s.Episodes)
		}
		if tc.episodes > 0 && s.LongestSeconds < .8 {
			t.Fatal("capture duration not recorded")
		}
	}
}

func TestEpisodeInvalidDecisionsDoNotRecover(t *testing.T) {
	var s HeadingEpisodeState
	for i := int64(1); i <= 8; i++ {
		s.Observe(HeadingSourceLocked, i*100)
	}
	for _, src := range []HeadingSource{-1, 99} {
		s.Observe(src, 1000)
	}
	s.Observe(HeadingSourceAxis, 0)
	s.Observe(HeadingSourceAxis, 800)
	if s.Outcome() != "unrecovered" {
		t.Fatal("invalid evidence recovered a lock")
	}
	for i := int64(9); i <= 13; i++ {
		s.Observe(HeadingSourceAxis, i*100)
	}
	if s.Outcome() != "recovered" || s.LastEndNanos != 900 {
		t.Fatal("valid recovery not recorded")
	}
	tr := TrackedObject{}
	tr.RecordHeadingSource(-1)
	if tr.HeadingEpisodes.Outcome() != "none" {
		t.Fatal("invalid source created episode")
	}
}
