package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/gerardus"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/lanefieldrun"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const laneTestSensor = "hesai-pandar40p"

type fakeLaneGerardus struct {
	mu       sync.Mutex
	records  map[string][]gerardus.Record
	down     bool
	noFields bool
	uploads  []l8analytics.LaneFieldHeader
	asked    []string
}

func (f *fakeLaneGerardus) ServerInfo(context.Context) (gerardus.ServerInfo, error) {
	if f.down {
		return gerardus.ServerInfo{}, errors.New("connection refused")
	}
	return gerardus.ServerInfo{Version: "0.9.0-test", SupportsLaneFields: !f.noFields}, nil
}

func (f *fakeLaneGerardus) Records(_ context.Context, l16 string) ([]gerardus.Record, error) {
	if f.down {
		return nil, errors.New("connection refused")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.records[l16], nil
}

func (f *fakeLaneGerardus) UploadLaneField(_ context.Context, lf l8analytics.LaneField) (gerardus.Upload, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploads = append(f.uploads, lf.Header)
	return gerardus.Upload{Part: lf.Header.WindowPart, FieldID: "sha256:" + lf.Header.WindowPart, Cells: uint32(lf.CellCount())}, nil
}

func (f *fakeLaneGerardus) Proposals(_ context.Context, l16 string) (gerardus.ProposalCounts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, l16)
	return gerardus.ProposalCounts{Total: 5, High: 3, Medium: 2, Adopted: 1, Outstanding: 4, DeriveSecs: 0.5}, nil
}

type laneFixture struct {
	ws       *Server
	fake     *fakeLaneGerardus
	store    *sqlite.ClipStore
	located  string
	unplaced string
	l16      string
}

func newLaneFixture(t *testing.T) laneFixture {
	t.Helper()
	safeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(safeDir, "s2-sf-3.pcapng"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeLaneGerardus{records: map[string][]gerardus.Record{}}
	ws := &Server{db: mustCaseDB(t), pcapSafeDir: safeDir, udpPort: 2369,
		laneSt: &laneState{dial: func(string) (laneGerardus, error) { return fake, nil }}}
	store := sqlite.NewClipStore(ws.db)
	located := &sqlite.Clip{SensorID: laneTestSensor, Files: []sqlite.ClipCapture{{PCAPFile: "s2-sf-3.pcapng"}}, Description: "Columbus at Broadway"}
	unplaced := &sqlite.Clip{SensorID: laneTestSensor, Files: []sqlite.ClipCapture{{PCAPFile: "s2-sf-3.pcapng"}}, Description: "no location"}
	for _, c := range []*sqlite.Clip{located, unplaced} {
		if err := store.InsertClip(c); err != nil {
			t.Fatal(err)
		}
	}
	loc, err := store.SetClipLocation(located.ClipID, 37.7977, -122.4065, "")
	if err != nil {
		t.Fatal(err)
	}
	return laneFixture{ws: ws, fake: fake, store: store, located: located.ClipID, unplaced: unplaced.ClipID, l16: loc.L16Token}
}

func (f laneFixture) register() {
	f.fake.mu.Lock()
	f.fake.records[f.l16] = []gerardus.Record{{RecordID: "gms:cb", State: gerardus.StateRegistered, L16Token: f.l16,
		SensorID: laneTestSensor, CaptureRef: "s2-sf-3.pcapng", ReviewedAt: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}}
	f.fake.mu.Unlock()
	f.ws.lanes().forget(f.l16)
}

func (f laneFixture) do(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if strings.HasPrefix(path, "/api/lidar/runs/") {
		f.ws.handleRunTrackAPI(rec, req)
	} else if strings.HasPrefix(path, "/api/lidar/clips/") {
		f.ws.handleClipByID(rec, req)
	} else {
		f.ws.handleClips(rec, req)
	}
	return rec
}

type listedClip struct {
	ClipID       string                `json:"clip_id"`
	Registration gerardus.Registration `json:"registration"`
	Lanes        LaneStatus            `json:"lanes"`
}

func (f laneFixture) list(t *testing.T) map[string]listedClip {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/api/lidar/clips?lanes=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Clips []listedClip `json:"clips"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]listedClip{}
	for _, c := range body.Clips {
		out[c.ClipID] = c
	}
	return out
}

// straightTraffic is six cars along +Y, each seen 30 times: enough for a
// field with cells over k.
func straightTraffic() lanefieldrun.Recording {
	const t0 = int64(1_788_000_000_000_000_000)
	var tracks []l8analytics.LaneFieldTrack
	for i := range 6 {
		tr := l8analytics.LaneFieldTrack{TrackID: fmt.Sprintf("car-%d", i), Class: "car"}
		for j := range 30 {
			ts := t0 + int64(i*40+j)*100_000_000
			tr.Instants = append(tr.Instants, l8analytics.LaneFieldInstant{TimeNs: ts, X: 1.5, Y: 0.6 * float64(j), VY: 6, SpeedMps: 6,
				BoxX: 1.5, BoxY: 0.6 * float64(j), BoxHeadingRad: 1.5708, BoxLength: 4.5, BoxWidth: 1.9})
		}
		tracks = append(tracks, tr)
	}
	return lanefieldrun.Recording{Tracks: tracks, WindowStartNs: t0, WindowEndNs: t0 + 270*100_000_000, Frames: 270}
}

func TestClipListShowsRegistrationAndLanes(t *testing.T) {
	f := newLaneFixture(t)
	got := f.list(t)
	if c := got[f.unplaced]; c.Registration.State != gerardus.ClipNotLocated {
		t.Fatalf("unplaced clip: %+v", c.Registration)
	}
	if c := got[f.located]; c.Registration.State != gerardus.ClipNotRegistered || c.Lanes.Current || c.Lanes.StaleReason == "" {
		t.Fatalf("located, unregistered clip: %+v %+v", c.Registration, c.Lanes)
	}
	f.register()
	if c := f.list(t)[f.located]; !c.Registration.Placed() || c.Registration.RecordID != "gms:cb" {
		t.Fatalf("registered clip: %+v", c.Registration)
	}
	f.fake.down = true
	f.ws.lanes().forget(f.l16)
	if c := f.list(t)[f.located]; c.Registration.State != gerardus.ClipUnknown {
		t.Fatalf("Gerardus down: %+v, want unknown", c.Registration)
	}
}

func TestLaneJobUploadsThreePartsAndRecordsTheCounts(t *testing.T) {
	f := newLaneFixture(t)
	if rec := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.located+"/lane-proposals"); rec.Code != http.StatusConflict {
		t.Fatalf("unregistered clip queued: %d %s", rec.Code, rec.Body.String())
	}
	f.register()
	rec := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.located+"/lane-proposals")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("queue: %d %s", rec.Code, rec.Body.String())
	}
	var queued struct {
		Job sqlite.CaptureJob `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &queued); err != nil {
		t.Fatal(err)
	}
	if again := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.located+"/lane-proposals"); again.Code != http.StatusConflict {
		t.Fatalf("second queue: %d, want the active job", again.Code)
	}
	if c := f.list(t)[f.located]; c.Lanes.Job == nil || !c.Lanes.Job.Active() {
		t.Fatalf("queued job not shown: %+v", c.Lanes)
	}

	_, fingerprint := f.ws.activeTuning()
	var replayed []string
	ops := laneJobOps{
		replay: func(_ context.Context, clip *sqlite.Clip, paths []string, report func(string)) (string, string, func(), error) {
			replayed = paths
			report("replaying the clip: 5s")
			return "unused.vrlog", fingerprint, func() {}, nil
		},
		read: func(string) (lanefieldrun.Recording, error) { return straightTraffic(), nil },
	}
	var progress []capjobs.Progress
	job := capjobs.Job{JobID: queued.Job.JobID, Kind: sqlite.JobKindLaneProposals}
	if err := f.ws.runLaneProposalsWith(context.Background(), job, func(p capjobs.Progress) { progress = append(progress, p) }, ops); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(replayed) != 1 || filepath.Base(replayed[0]) != "s2-sf-3.pcapng" {
		t.Fatalf("replayed %v", replayed)
	}
	if len(f.fake.uploads) != 3 {
		t.Fatalf("uploads = %d, want three parts", len(f.fake.uploads))
	}
	for i, part := range []string{"all", "first_half", "second_half"} {
		h := f.fake.uploads[i]
		if h.WindowPart != part || h.L16Token != f.l16 || h.RecordID != "gms:cb" || h.SensorID != laneTestSensor ||
			!strings.HasPrefix(h.CaptureRef, "clip:"+f.located+":") || h.PipelineBuild == "" {
			t.Fatalf("upload %d header = %+v", i, h)
		}
	}
	if len(f.fake.asked) != 1 || f.fake.asked[0] != f.l16 {
		t.Fatalf("proposals asked for %v", f.fake.asked)
	}
	if last := progress[len(progress)-1]; last.Current != laneJobSteps || !strings.Contains(last.Detail, "3 high") {
		t.Fatalf("last progress = %+v", last)
	}
	if err := sqlite.NewCaptureStore(f.ws.db).FinishJob(job.JobID, sqlite.JobCompleted, ""); err != nil {
		t.Fatal(err)
	}
	c := f.list(t)[f.located]
	if c.Lanes.Latest == nil || c.Lanes.Latest.High != 3 || c.Lanes.Latest.Outstanding != 4 || len(c.Lanes.Latest.FieldIDs) != 3 || !c.Lanes.Current {
		t.Fatalf("after the job: %+v (%s)", c.Lanes.Latest, c.Lanes.StaleReason)
	}
}

func TestLaneJobRefusesAServerWithoutLaneFields(t *testing.T) {
	f := newLaneFixture(t)
	f.register()
	job, _, err := sqlite.NewLaneProposalStore(f.ws.db).EnqueueLaneProposals(f.located, "")
	if err != nil {
		t.Fatal(err)
	}
	f.fake.noFields = true
	ops := laneJobOps{replay: func(context.Context, *sqlite.Clip, []string, func(string)) (string, string, func(), error) {
		t.Fatal("replayed against a server that takes no fields")
		return "", "", nil, nil
	}}
	err = f.ws.runLaneProposalsWith(context.Background(), capjobs.Job{JobID: job.JobID}, func(capjobs.Progress) {}, ops)
	if err == nil || !strings.Contains(err.Error(), "takes no lane fields") {
		t.Fatalf("err = %v", err)
	}
}

func TestRerunQueuesRegisteredStaleClipsOnly(t *testing.T) {
	f := newLaneFixture(t)
	f.register()
	rec := f.do(t, http.MethodPost, "/api/lidar/clips/lane-proposals/rerun?dry_run=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("dry run: %d %s", rec.Code, rec.Body.String())
	}
	var dry RerunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &dry); err != nil {
		t.Fatal(err)
	}
	if len(dry.Queued) != 1 || dry.Queued[0].ClipID != f.located || dry.Queued[0].JobID != "" || len(dry.Skipped) != 1 || dry.Skipped[0].ClipID != f.unplaced {
		t.Fatalf("dry run = %+v", dry)
	}
	if jobs, _ := sqlite.NewCaptureStore(f.ws.db).ListJobsOfKind("", sqlite.JobKindLaneProposals, 10); len(jobs) != 0 {
		t.Fatalf("a dry run queued %d jobs", len(jobs))
	}

	rec = f.do(t, http.MethodPost, "/api/lidar/clips/lane-proposals/rerun")
	var real RerunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &real); err != nil || rec.Code != http.StatusAccepted || len(real.Queued) != 1 || real.Queued[0].JobID == "" {
		t.Fatalf("rerun: %d %+v %v", rec.Code, real, err)
	}
	rec = f.do(t, http.MethodPost, "/api/lidar/clips/lane-proposals/rerun")
	var again RerunResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &again)
	if len(again.Queued) != 0 || len(again.Active) != 1 {
		t.Fatalf("a second rerun while the job is queued: %+v", again)
	}

	// Complete it as current: a rerun then skips it unless forced.
	_, fingerprint := f.ws.activeTuning()
	row, _ := sqlite.NewLaneProposalStore(f.ws.db).ForJob(real.Queued[0].JobID)
	row.RecordID, row.FieldIDs, row.PipelineBuild, row.TuningFingerprint = "gms:cb", []string{"sha256:a"}, pipelineBuild(), fingerprint
	if _, err := sqlite.NewLaneProposalStore(f.ws.db).Complete(row); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewCaptureStore(f.ws.db).FinishJob(row.JobID, sqlite.JobCompleted, ""); err != nil {
		t.Fatal(err)
	}
	rec = f.do(t, http.MethodPost, "/api/lidar/clips/lane-proposals/rerun?dry_run=1")
	var current RerunResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &current)
	if len(current.Queued) != 0 || len(current.Current) != 1 {
		t.Fatalf("current clip re-queued: %+v", current)
	}
	rec = f.do(t, http.MethodPost, "/api/lidar/clips/lane-proposals/rerun?dry_run=1&force=1")
	var forced RerunResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &forced)
	if len(forced.Queued) != 1 {
		t.Fatalf("force did not queue the current clip: %+v", forced)
	}
}

func TestLaneStatusSaysWhyItIsStale(t *testing.T) {
	done := int64(1)
	row := &sqlite.LaneProposalRun{PipelineBuild: "0.5.1+aaa", TuningFingerprint: "f1", CompletedAtNs: &done}
	if s := laneStatus(sqlite.ClipLanes{}, "0.5.2+bbb", "f1"); s.Current || s.StaleReason != "no lane proposals yet" {
		t.Fatalf("none: %+v", s)
	}
	if s := laneStatus(sqlite.ClipLanes{Latest: row}, "0.5.2+bbb", "f1"); s.Current || !strings.Contains(s.StaleReason, "0.5.1+aaa") {
		t.Fatalf("old build: %+v", s)
	}
	if s := laneStatus(sqlite.ClipLanes{Latest: row}, "0.5.1+aaa", "f2"); s.Current || !strings.Contains(s.StaleReason, "tuning f1") {
		t.Fatalf("old tuning: %+v", s)
	}
	if s := laneStatus(sqlite.ClipLanes{Latest: row}, "0.5.1+aaa", "f1"); !s.Current {
		t.Fatalf("current: %+v", s)
	}
}
