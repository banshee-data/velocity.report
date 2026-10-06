package capindex

import (
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
)

// Probed is a capture file whose packet-time extent is known, which is what a
// file needs before it can belong to a session.
type Probed struct {
	// RelPath identifies the file within its root.
	RelPath string
	// FirstPacket and LastPacket bound the capture.
	FirstPacket time.Time
	LastPacket  time.Time
	PacketCount uint64
	SizeBytes   int64
}

// Session is a maximal run of capture files whose extents abut closely enough
// to replay as one stream.
//
// Sessions are derived, never authored: an operator does not declare that seven
// files belong together, the packet clock does. What an operator does supply is
// the label — the site the session was captured at — which is why the store
// keeps labels by session identity rather than rebuilding them.
type Session struct {
	// Files are the session's members in packet-time order.
	Files []Probed
	// Start and End bound the whole run.
	Start time.Time
	End   time.Time
	// Covered is the sum of the files' own extents; Lost is the time
	// unaccounted for at the joins between them.
	Covered time.Duration
	Lost    time.Duration
	// Worst is the weakest join in the run. A session never contains a join
	// worse than acceptable — that is what ends one.
	Worst capseq.SeamGrade
	// SizeBytes is the run's total on disk.
	SizeBytes int64
}

// Duration is the wall-clock span the session covers.
func (s Session) Duration() time.Duration { return s.End.Sub(s.Start) }

// Sessions groups probed files into runs, splitting wherever a join cannot be
// crossed.
//
// A gap wider than the tolerances means the sensor was switched off, moved, or
// the capture tool restarted, and the two sides are not one continuous
// recording however close their filenames look. An overlap means the same,
// arrived at differently.
//
// Files without a usable extent are not sessioned; a caller that wants them
// listed should list them from the index directly.
func Sessions(files []Probed, tol capseq.Tolerances) []Session {
	usable := make([]Probed, 0, len(files))
	for _, f := range files {
		if f.FirstPacket.IsZero() || f.LastPacket.IsZero() || f.LastPacket.Before(f.FirstPacket) {
			continue
		}
		usable = append(usable, f)
	}
	if len(usable) == 0 {
		return nil
	}

	sort.Slice(usable, func(i, j int) bool {
		if usable[i].FirstPacket.Equal(usable[j].FirstPacket) {
			return usable[i].RelPath < usable[j].RelPath
		}
		return usable[i].FirstPacket.Before(usable[j].FirstPacket)
	})

	var sessions []Session
	run := []Probed{usable[0]}
	flush := func() {
		if s, ok := buildSession(run, tol); ok {
			sessions = append(sessions, s)
		}
	}
	for i := 1; i < len(usable); i++ {
		gap := usable[i].FirstPacket.Sub(usable[i-1].LastPacket)
		if capseq.GradeGap(gap, tol).Replayable() {
			run = append(run, usable[i])
			continue
		}
		flush()
		run = []Probed{usable[i]}
	}
	flush()
	return sessions
}

// buildSession turns a run of files into a Session, using capseq so the grading
// and the lost-time arithmetic have exactly one definition in the system.
func buildSession(run []Probed, tol capseq.Tolerances) (Session, bool) {
	segments := make([]capseq.Segment, 0, len(run))
	var size int64
	for _, f := range run {
		segments = append(segments, capseq.Segment{
			Path:        f.RelPath,
			FirstPacket: f.FirstPacket,
			LastPacket:  f.LastPacket,
			PacketCount: f.PacketCount,
		})
		size += f.SizeBytes
	}
	seq, err := capseq.Build(segments, tol)
	if err != nil {
		// Build only rejects malformed input, which Sessions has already
		// filtered for; a run that still fails is dropped rather than
		// half-reported.
		return Session{}, false
	}
	ordered := make([]Probed, 0, len(run))
	for _, seg := range seq.Segments {
		for _, f := range run {
			if f.RelPath == seg.Path {
				ordered = append(ordered, f)
				break
			}
		}
	}
	return Session{
		Files:     ordered,
		Start:     seq.Start,
		End:       seq.End,
		Covered:   seq.Covered,
		Lost:      seq.Lost,
		Worst:     seq.Worst,
		SizeBytes: size,
	}, true
}
