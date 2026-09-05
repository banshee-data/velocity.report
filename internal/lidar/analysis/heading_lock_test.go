package analysis

import "testing"

func srcRep(v, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func TestComputeLockStatsCountsSources(t *testing.T) {
	got := computeLockStats([]int{
		headingSourcePCA,
		headingSourceVelocity, headingSourceVelocity,
		headingSourceDisplacement,
		headingSourceLocked, headingSourceLocked, headingSourceLocked,
	}, nil)
	want := [headingSourceCount]int{1, 2, 1, 3}
	if got.sourceCounts != want {
		t.Fatalf("sourceCounts = %v, want %v", got.sourceCounts, want)
	}
	if got.lockedFrames != 3 {
		t.Fatalf("lockedFrames = %d, want 3", got.lockedFrames)
	}
}

func TestComputeLockStatsShortLockNotSustained(t *testing.T) {
	seq := append(srcRep(headingSourceVelocity, 5), srcRep(headingSourceLocked, SustainedLockFrames-1)...)
	seq = append(seq, srcRep(headingSourceVelocity, 5)...)
	got := computeLockStats(seq, nil)
	if got.sustained || got.trapped() {
		t.Fatalf("short lock reported sustained=%v trapped=%v", got.sustained, got.trapped())
	}
}

func TestComputeLockStatsTrapped(t *testing.T) {
	got := computeLockStats(append(srcRep(headingSourceVelocity, 5), srcRep(headingSourceLocked, 40)...), nil)
	if !got.sustained {
		t.Fatal("40 locked frames not sustained")
	}
	if got.released {
		t.Fatal("reported a release that never happened")
	}
	if !got.trapped() {
		t.Fatal("not reported as trapped")
	}
	if got.longestLockRun != 40 {
		t.Fatalf("longestLockRun = %d, want 40", got.longestLockRun)
	}
}

func TestComputeLockStatsSingleUnlockedFrameIsNotARelease(t *testing.T) {
	seq := srcRep(headingSourceLocked, 20)
	seq = append(seq, headingSourceVelocity)
	seq = append(seq, srcRep(headingSourceLocked, 20)...)
	got := computeLockStats(seq, nil)
	if got.released {
		t.Fatal("a single unlocked frame counted as a release")
	}
	if !got.trapped() {
		t.Fatal("interrupted lock not reported as trapped")
	}
}

func TestComputeLockStatsEmpty(t *testing.T) {
	got := computeLockStats(nil, nil)
	if got.sustained || got.released || got.trapped() || got.lockedFrames != 0 {
		t.Fatalf("empty sequence produced %+v", got)
	}
}

// Out-of-range source values must not index past the counter array.
func TestComputeLockStatsIgnoresUnknownSources(t *testing.T) {
	got := computeLockStats([]int{-1, 99, headingSourceLocked}, nil)
	if got.lockedFrames != 1 {
		t.Fatalf("lockedFrames = %d, want 1", got.lockedFrames)
	}
	var total int
	for _, n := range got.sourceCounts {
		total += n
	}
	if total != 1 {
		t.Fatalf("counted %d known sources, want 1", total)
	}
}

func TestHeadingSourceName(t *testing.T) {
	cases := map[int]string{
		headingSourcePCA:          "pca",
		headingSourceVelocity:     "velocity",
		headingSourceDisplacement: "displacement",
		headingSourceLocked:       "locked",
		42:                        "unknown",
	}
	for in, want := range cases {
		if got := headingSourceName(in); got != want {
			t.Fatalf("headingSourceName(%d) = %q, want %q", in, got, want)
		}
	}
}

// The offline reconstruction and the tracker's running counters are the two
// sides of every A/B comparison, so they must agree frame for frame. This
// mirrors l5tracks.TrackedObject.RecordHeadingSource.
func TestComputeLockStatsMatchesTrackerSemantics(t *testing.T) {
	sequences := [][]int{
		srcRep(headingSourceLocked, 40),
		srcRep(headingSourceVelocity, 40),
		append(srcRep(headingSourceLocked, 20), srcRep(headingSourceVelocity, 20)...),
		append(srcRep(headingSourceVelocity, 20), srcRep(headingSourceLocked, 20)...),
		append(append(srcRep(headingSourceLocked, 20), headingSourceVelocity), srcRep(headingSourceLocked, 20)...),
		append(srcRep(headingSourceLocked, SustainedLockFrames-1), srcRep(headingSourceVelocity, 10)...),
	}
	for i, seq := range sequences {
		got := computeLockStats(seq, nil)
		want := trackerLockReference(seq)
		if got.lockedFrames != want.lockedFrames ||
			got.longestLockRun != want.longestLockRun ||
			got.sustained != want.sustained ||
			got.released != want.released {
			t.Fatalf("sequence %d: offline %+v, tracker %+v", i, got, want)
		}
	}
}

// trackerLockReference mirrors l5tracks.TrackedObject.RecordHeadingSource. It is
// duplicated rather than imported to keep analysis free of a dependency on the
// tracker package.
func trackerLockReference(sources []int) lockStats {
	var st lockStats
	lockRun, unlockRun := 0, 0
	for _, src := range sources {
		if src == headingSourceLocked {
			st.lockedFrames++
			lockRun++
			unlockRun = 0
			if lockRun > st.longestLockRun {
				st.longestLockRun = lockRun
			}
			if lockRun >= SustainedLockFrames {
				st.sustained = true
			}
			continue
		}
		lockRun = 0
		unlockRun++
		if st.sustained && unlockRun >= SustainedLockFrames {
			st.released = true
		}
	}
	return st
}

// The ghost-frame bug. A deleted track keeps being published for the grace
// period with its state frozen, and those frames report heading source PCA
// rather than the lock the track died in. Counting them turns a track that was
// trapped for its whole life into a clean one. On the reference run this alone
// moved the trapped population from 59% of locked tracks to 11%.
func TestComputeLockStatsIgnoresDeletedGhostFrames(t *testing.T) {
	// Twenty live locked frames, then fifty ghost frames reporting PCA.
	sources := append(srcRep(headingSourceLocked, 20), srcRep(headingSourcePCA, 50)...)
	live := make([]bool, len(sources))
	for i := range live {
		live[i] = i < 20
	}

	got := computeLockStats(sources, live)
	if got.released {
		t.Fatal("ghost frames counted as a lock release")
	}
	if !got.trapped() {
		t.Fatal("track trapped for its whole live span not reported as trapped")
	}
	if got.liveFrames != 20 {
		t.Fatalf("liveFrames = %d, want 20", got.liveFrames)
	}
	if got.sourceCounts[headingSourcePCA] != 0 {
		t.Fatalf("counted %d PCA frames from ghosts, want 0", got.sourceCounts[headingSourcePCA])
	}

	// Without the live mask the same sequence looks clean, which is the bug.
	naive := computeLockStats(sources, nil)
	if !naive.released {
		t.Fatal("test precondition: unmasked sequence should look released")
	}
}

// A track shorter than the lock window cannot be classified either way and must
// not dilute the run-level ratios.
func TestLockStatsAssessable(t *testing.T) {
	short := computeLockStats(srcRep(headingSourceLocked, SustainedLockFrames-1), nil)
	if short.assessable() {
		t.Fatal("a track shorter than the lock window reported as assessable")
	}
	long := computeLockStats(srcRep(headingSourceVelocity, SustainedLockFrames), nil)
	if !long.assessable() {
		t.Fatal("a track at the lock window length reported as unassessable")
	}
}

func TestSafeRatio(t *testing.T) {
	if got := safeRatio(1, 0); got != 0 {
		t.Fatalf("safeRatio(1, 0) = %v, want 0", got)
	}
	if got := safeRatio(1, 4); got != 0.25 {
		t.Fatalf("safeRatio(1, 4) = %v, want 0.25", got)
	}
}
