package l5tracks

// HeadingEpisodeState describes the most recent sustained measurement lock.
// It is separate from the legacy lifetime recovery flags. Observation timestamps
// are capture times; coasting publications must not be fed back as measurements.
type HeadingEpisodeState struct {
	Episodes           int     `json:"episodes"`
	LastStartNanos     int64   `json:"last_start_ns"`
	LastEndNanos       int64   `json:"last_end_ns"`
	LongestSeconds     float64 `json:"longest_seconds"`
	run, unlocked      int
	runStart, lastTime int64
	resolved           bool
	active             bool
}

// Observe accepts a heading decision, not a rendered copy of a previous one.
// Unknown sources and non-increasing timestamps cannot establish recovery.
func (s *HeadingEpisodeState) Observe(src HeadingSource, timestamp int64) {
	if timestamp <= s.lastTime || timestamp <= 0 || src < 0 || int(src) >= HeadingSourceCount {
		s.run, s.unlocked = 0, 0
		return
	}
	s.lastTime = timestamp
	if src.IsLocked() {
		if s.run == 0 {
			s.runStart = timestamp
		}
		s.run++
		s.unlocked = 0
		if s.run == SustainedLockFrames {
			s.Episodes++
			s.LastStartNanos = s.runStart
			s.LastEndNanos = 0
			s.resolved, s.active = false, true
		}
		if s.run >= SustainedLockFrames {
			seconds := float64(timestamp-s.runStart) / 1e9
			if seconds > s.LongestSeconds {
				s.LongestSeconds = seconds
			}
		}
		return
	}
	s.run = 0
	if s.Episodes == 0 {
		return
	}
	if s.active {
		s.LastEndNanos = timestamp
		s.active = false
	}
	s.unlocked++
	if s.unlocked >= SustainedLockFrames {
		s.resolved = true
	}
}

// Outcome distinguishes terminal recovery from a lifetime recovery event.
// Censored means there were too few valid unlocked observations to decide.
func (s HeadingEpisodeState) Outcome() string {
	if s.Episodes == 0 {
		return "none"
	}
	if s.active {
		return "unrecovered"
	}
	if !s.resolved {
		return "censored"
	}
	return "recovered"
}
