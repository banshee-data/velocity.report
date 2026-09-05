package l5tracks

import "testing"

// recordSources feeds a heading-source sequence into a fresh track.
func recordSources(srcs ...HeadingSource) *TrackedObject {
	tr := &TrackedObject{}
	for _, s := range srcs {
		tr.RecordHeadingSource(s)
	}
	return tr
}

func rep(s HeadingSource, n int) []HeadingSource {
	out := make([]HeadingSource, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func TestRecordHeadingSourceCountsFrames(t *testing.T) {
	tr := recordSources(
		HeadingSourcePCA,
		HeadingSourceVelocity, HeadingSourceVelocity,
		HeadingSourceDisplacement,
		HeadingSourceLocked, HeadingSourceLocked, HeadingSourceLocked,
	)
	want := [HeadingSourceCount]uint32{1, 2, 1, 3}
	if tr.HeadingSourceCounts != want {
		t.Fatalf("counts = %v, want %v", tr.HeadingSourceCounts, want)
	}
	if tr.HeadingLockedFrames != 3 {
		t.Fatalf("locked frames = %d, want 3", tr.HeadingLockedFrames)
	}
}

// A short lock is the guards doing their job on one bad cluster, not a trap.
func TestShortLockIsNotSustained(t *testing.T) {
	tr := recordSources(append(
		rep(HeadingSourceVelocity, 5),
		append(rep(HeadingSourceLocked, SustainedLockFrames-1), rep(HeadingSourceVelocity, 5)...)...,
	)...)
	if tr.EnteredSustainedLock {
		t.Fatal("a run of SustainedLockFrames-1 counted as sustained")
	}
	if tr.HeadingLockTrapped() {
		t.Fatal("short lock reported as trapped")
	}
	if tr.LongestLockRun != SustainedLockFrames-1 {
		t.Fatalf("longest run = %d, want %d", tr.LongestLockRun, SustainedLockFrames-1)
	}
}

// The failure mode: a sustained lock with no release for the rest of the track.
func TestSustainedLockWithoutReleaseIsTrapped(t *testing.T) {
	tr := recordSources(append(rep(HeadingSourceVelocity, 5), rep(HeadingSourceLocked, 40)...)...)
	if !tr.EnteredSustainedLock {
		t.Fatal("40 locked frames did not register as sustained")
	}
	if tr.ReleasedAfterLock {
		t.Fatal("reported a release that never happened")
	}
	if !tr.HeadingLockTrapped() {
		t.Fatal("sustained lock with no release is not reported as trapped")
	}
	if tr.LongestLockRun != 40 {
		t.Fatalf("longest run = %d, want 40", tr.LongestLockRun)
	}
}

func TestSustainedLockThatReleasesIsNotTrapped(t *testing.T) {
	tr := recordSources(append(
		rep(HeadingSourceLocked, 20),
		rep(HeadingSourceVelocity, SustainedLockFrames)...,
	)...)
	if !tr.EnteredSustainedLock {
		t.Fatal("20 locked frames did not register as sustained")
	}
	if !tr.ReleasedAfterLock {
		t.Fatal("a full run of unlocked frames did not count as a release")
	}
	if tr.HeadingLockTrapped() {
		t.Fatal("released lock still reported as trapped")
	}
}

// One frame slipping through between rejections is not the lock letting go.
// Without this the trapped population would be badly undercounted: Guard 3
// rejects per-frame, so a single unlocked frame is common inside a long trap.
func TestSingleUnlockedFrameIsNotARelease(t *testing.T) {
	seq := rep(HeadingSourceLocked, 20)
	seq = append(seq, HeadingSourceVelocity)
	seq = append(seq, rep(HeadingSourceLocked, 20)...)
	tr := recordSources(seq...)

	if tr.ReleasedAfterLock {
		t.Fatal("a single unlocked frame counted as a release")
	}
	if !tr.HeadingLockTrapped() {
		t.Fatal("track with one interrupted lock is not reported as trapped")
	}
	if tr.LongestLockRun != 20 {
		t.Fatalf("longest run = %d, want 20", tr.LongestLockRun)
	}
	if tr.HeadingLockedFrames != 40 {
		t.Fatalf("locked frames = %d, want 40", tr.HeadingLockedFrames)
	}
}

// Unlocked frames before any sustained lock must not pre-arm a release.
func TestUnlockedRunBeforeLockDoesNotCountAsRelease(t *testing.T) {
	tr := recordSources(append(
		rep(HeadingSourceVelocity, 30),
		rep(HeadingSourceLocked, 20)...,
	)...)
	if tr.ReleasedAfterLock {
		t.Fatal("unlocked frames preceding the lock counted as a release")
	}
	if !tr.HeadingLockTrapped() {
		t.Fatal("track should be trapped: the lock never released")
	}
}

func TestNeverLockedTrackIsClean(t *testing.T) {
	tr := recordSources(rep(HeadingSourceVelocity, 50)...)
	if tr.EnteredSustainedLock || tr.ReleasedAfterLock || tr.HeadingLockTrapped() {
		t.Fatal("a track that never locked reported lock state")
	}
	if tr.HeadingLockedFrames != 0 || tr.LongestLockRun != 0 {
		t.Fatalf("locked frames = %d, longest run = %d, want 0 and 0",
			tr.HeadingLockedFrames, tr.LongestLockRun)
	}
}

func TestGetTrackingMetricsRollsUpLockTelemetry(t *testing.T) {
	tr := NewTracker(DefaultTrackerConfig())

	trapped := &TrackedObject{TrackID: "trapped", TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed}}
	for _, s := range rep(HeadingSourceLocked, 30) {
		trapped.RecordHeadingSource(s)
	}
	healthy := &TrackedObject{TrackID: "healthy", TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed}}
	for _, s := range rep(HeadingSourceVelocity, 30) {
		healthy.RecordHeadingSource(s)
	}
	tr.Tracks["trapped"] = trapped
	tr.Tracks["healthy"] = healthy

	m := tr.GetTrackingMetrics()
	if m.SustainedLockTracks != 1 {
		t.Fatalf("sustained lock tracks = %d, want 1", m.SustainedLockTracks)
	}
	if m.LockTrappedTracks != 1 {
		t.Fatalf("trapped tracks = %d, want 1", m.LockTrappedTracks)
	}
	if m.LongestLockRunFrames != 30 {
		t.Fatalf("longest lock run = %d, want 30", m.LongestLockRunFrames)
	}
	if got := m.LockedFrameRatio; got < 0.49 || got > 0.51 {
		t.Fatalf("locked frame ratio = %v, want ~0.5", got)
	}
	if m.HeadingSourceFrames["locked"] != 30 || m.HeadingSourceFrames["velocity"] != 30 {
		t.Fatalf("source frames = %v, want 30 locked and 30 velocity", m.HeadingSourceFrames)
	}
}
