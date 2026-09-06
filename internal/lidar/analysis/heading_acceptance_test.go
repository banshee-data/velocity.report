package analysis

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// Held share is not comparable between the guard path and the axis path,
// because they label their accepted frames differently. Acceptance is, and
// these two sequences are the same result stated in each path's vocabulary.
func TestAcceptanceIsComparableAcrossHeadingPaths(t *testing.T) {
	guard := computeLockStats(append(
		srcRep(headingSourceVelocity, 30),
		srcRep(headingSourceLocked, 10)...,
	), nil)
	axis := computeLockStats(append(
		srcRep(int(l5tracks.HeadingSourceAxis), 30),
		srcRep(int(l5tracks.HeadingSourceAxisSquare), 10)...,
	), nil)

	if guard.lockedFrames != axis.lockedFrames {
		t.Fatalf("held frames = %d guard, %d axis: the same behaviour must count the same",
			guard.lockedFrames, axis.lockedFrames)
	}
	if guard.liveFrames-guard.lockedFrames != 30 {
		t.Fatalf("guard accepted %d frames, want 30", guard.liveFrames-guard.lockedFrames)
	}
	if axis.liveFrames-axis.lockedFrames != 30 {
		t.Fatalf("axis accepted %d frames, want 30", axis.liveFrames-axis.lockedFrames)
	}
}

// Each abstention reason must survive into the report separately: they are
// the evidence for which of them to act on.
func TestAbstentionReasonsAreCountedSeparately(t *testing.T) {
	got := computeLockStats([]int{
		int(l5tracks.HeadingSourceAxis),
		int(l5tracks.HeadingSourceAxisSquare),
		int(l5tracks.HeadingSourceAxisNoFit), int(l5tracks.HeadingSourceAxisNoFit),
		int(l5tracks.HeadingSourceAmbiguous),
		int(l5tracks.HeadingSourceInsufficient),
	}, nil)

	for src, want := range map[l5tracks.HeadingSource]int{
		l5tracks.HeadingSourceAxis:         1,
		l5tracks.HeadingSourceAxisSquare:   1,
		l5tracks.HeadingSourceAxisNoFit:    2,
		l5tracks.HeadingSourceAmbiguous:    1,
		l5tracks.HeadingSourceInsufficient: 1,
	} {
		if got.sourceCounts[src] != want {
			t.Fatalf("%q counted %d, want %d", src, got.sourceCounts[src], want)
		}
	}
	if got.lockedFrames != 5 {
		t.Fatalf("held frames = %d, want 5", got.lockedFrames)
	}
}

// A forced release on either path is an event in the recording, not a counter,
// so both must be counted when the recording is read back.
func TestBothForcedReleaseSourcesCount(t *testing.T) {
	for _, src := range []int{headingSourceReleased, int(l5tracks.HeadingSourceAxisReleased)} {
		seq := append(srcRep(headingSourceLocked, 20), src)
		seq = append(seq, srcRep(headingSourceVelocity, 10)...)
		got := computeLockStats(seq, nil)
		if got.releases != 1 {
			t.Fatalf("source %q: releases = %d, want 1", headingSourceName(src), got.releases)
		}
		if l5tracks.HeadingSource(src).IsLocked() {
			t.Fatalf("source %q counted as held", headingSourceName(src))
		}
	}
}

// The report reads source names from l5tracks so the two cannot drift apart.
func TestEveryHeadingSourceIsNamed(t *testing.T) {
	for src := 0; src < headingSourceCount; src++ {
		if name := headingSourceName(src); name == "unknown" {
			t.Fatalf("source %d has no name", src)
		}
	}
	for _, src := range []int{-1, headingSourceCount} {
		if headingSourceName(src) != "unknown" {
			t.Fatalf("out-of-range source %d was named", src)
		}
	}
}
