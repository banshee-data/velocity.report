package main

import (
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

func record(t *testing.T, dir, id string, zs ...float64) string {
	t.Helper()
	var tracks []l8analytics.GEST1CorpusTrack
	for i, z := range zs {
		tracks = append(tracks, l8analytics.GEST1CorpusTrack{Case: id, TrackID: id + string(rune('a'+i)),
			Samples: []l8analytics.GEST1CorpusSample{{OneStep: true, Dim: 2, HasAlong: true, ZAlong: z}}})
	}
	rec := l8analytics.GEST1CorpusRecord{Case: id, Config: l8analytics.DefaultGEST1Config(2.5),
		Sources: []l8analytics.GEST1CorpusSource{{Source: l8analytics.GEST1SourceTracked, Tracks: tracks}}}
	path := filepath.Join(dir, id+".json.gz")
	if err := l8analytics.WriteGEST1CorpusRecord(path, rec); err != nil {
		t.Fatal(err)
	}
	return path
}

// Two cases pool into one read whose tracks are their sum; each case is read
// alone and left out. With z of 1 and 3 in case x and 1 in case y, the pool's
// RMS is √(11/3), x alone √5, and leaving x out leaves y's 1.
func TestPoolReadsPooledPerCaseAndLeaveOneOut(t *testing.T) {
	dir := t.TempDir()
	paths := []string{record(t, dir, "y", 1), record(t, dir, "x", 1, 3)}
	cfg := l8analytics.DefaultGEST1CorpusConfig()
	cfg.BootstrapDraws = 50
	p, err := run(paths, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Cases) != 2 || p.Cases[0] != "x" || len(p.Sources) != 1 {
		t.Fatalf("pool %+v", p)
	}
	s := p.Sources[0]
	near := func(a, b float64) bool { return a-b < 1e-12 && b-a < 1e-12 }
	if s.Pooled.Tracks != 3 || !near(s.Pooled.Along.RMS*s.Pooled.Along.RMS, 11.0/3) {
		t.Errorf("pooled %+v", s.Pooled.Along)
	}
	if len(s.PerCase) != 2 || s.PerCase[0].Case != "x" || !near(s.PerCase[0].Read.Along.RMS*s.PerCase[0].Read.Along.RMS, 5) {
		t.Errorf("per case %+v", s.PerCase)
	}
	if len(s.LeaveOneOut) != 2 || s.LeaveOneOut[0].Case != "x" || s.LeaveOneOut[0].Tracks != 1 || !near(s.LeaveOneOut[0].AlongRMS, 1) {
		t.Errorf("leave one out %+v", s.LeaveOneOut)
	}
	if _, err := run([]string{paths[0], paths[0]}, cfg); err == nil {
		t.Error("a repeated case must be refused")
	}
	// LABEL=PATH reads one record under another case name, so two segments
	// of one site pool as two cases.
	q, err := run([]string{paths[0], "y-s2=" + paths[0]}, cfg)
	if err != nil || len(q.Cases) != 2 || q.Cases[1] != "y-s2" || q.Sources[0].Pooled.Tracks != 2 {
		t.Errorf("relabelled pool %v %+v", err, q.Cases)
	}
	if lo := q.Sources[0].LeaveOneOut[0]; !near(lo.MatureAlongRMS, 0) || !near(lo.YoungAlongRMS, 1) {
		t.Errorf("leave one out age split %+v", lo)
	}
}
