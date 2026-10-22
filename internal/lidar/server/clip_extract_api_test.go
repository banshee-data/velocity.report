package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/dataset"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

// writeSyntheticHesai writes n Pandar40P-shaped packets 5 ms apart, 18 to a
// rotation, starting at start.
func writeSyntheticHesai(t *testing.T, path string, start time.Time, n int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := pcapgo.NewWriterNanos(f)
	if err := w.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		t.Fatal(err)
	}
	for k := range n {
		payload := make([]byte, 1262)
		for i := range 10 {
			off := i * 124
			payload[off], payload[off+1] = 0xFF, 0xEE
			binary.LittleEndian.PutUint16(payload[off+2:], uint16((k*2000+i*200)%36000))
		}
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IP{192, 168, 1, 201}, DstIP: net.IP{192, 168, 1, 100}}
		udp := &layers.UDP{SrcPort: 2369, DstPort: 2369}
		_ = udp.SetNetworkLayerForChecksum(ip)
		buf := gopacket.NewSerializeBuffer()
		if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
			eth, ip, udp, gopacket.Payload(payload)); err != nil {
			t.Fatal(err)
		}
		data := buf.Bytes()
		ci := gopacket.CaptureInfo{Timestamp: start.Add(time.Duration(k) * 5 * time.Millisecond), CaptureLength: len(data), Length: len(data)}
		if err := w.WritePacket(ci, data); err != nil {
			t.Fatal(err)
		}
	}
}

type extractFixture struct {
	ws      *Server
	store   *sqlite.ClipStore
	safeDir string
	clipID  string
}

func newExtractFixture(t *testing.T) extractFixture {
	t.Helper()
	// The replay check compares canonical paths, and macOS's temp directory
	// sits behind a symlink.
	safeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2025, 12, 5, 23, 14, 54, 0, time.UTC)
	writeSyntheticHesai(t, filepath.Join(safeDir, "kirk2.pcapng"), start, 400)
	ws := &Server{db: mustCaseDB(t), pcapSafeDir: safeDir, udpPort: 2369,
		datasetDir: filepath.Join(safeDir, "sf-street-speeds")}
	store := sqlite.NewClipStore(ws.db)
	startSecs, duration := 0.2, 1.0
	clip := &sqlite.Clip{SensorID: "hesai-pandar40p", Files: []sqlite.ClipCapture{{PCAPFile: "kirk2.pcapng"}},
		PCAPStartSecs: &startSecs, PCAPDurationSecs: &duration, Description: "Kirkham K3"}
	if err := store.InsertClip(clip); err != nil {
		t.Fatal(err)
	}
	return extractFixture{ws: ws, store: store, safeDir: safeDir, clipID: clip.ClipID}
}

func (f extractFixture) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	f.ws.handleClipByID(rec, req)
	return rec
}

// runExtract queues and runs the clip's export, as the runner would.
func (f extractFixture) runExtract(t *testing.T) sqlite.ClipExtract {
	t.Helper()
	rec := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.clipID+"/extract", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("enqueue: %d %s", rec.Code, rec.Body.String())
	}
	var queued struct {
		Job sqlite.CaptureJob `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &queued); err != nil {
		t.Fatal(err)
	}
	if again := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.clipID+"/extract", ""); again.Code != http.StatusOK {
		t.Fatalf("second enqueue: %d, want the active job", again.Code)
	}
	job := capjobs.Job{JobID: queued.Job.JobID, Kind: sqlite.JobKindCaptureExport}
	var last capjobs.Progress
	if err := f.ws.runClipExtract(context.Background(), job, func(p capjobs.Progress) { last = p }); err != nil {
		t.Fatalf("run extract: %v", err)
	}
	if !strings.Contains(last.Detail, "rotations") {
		t.Fatalf("final progress %+v", last)
	}
	if err := sqlite.NewCaptureStore(f.ws.db).FinishJob(job.JobID, sqlite.JobCompleted, ""); err != nil {
		t.Fatal(err)
	}
	extract, err := f.store.LatestClipExtract(f.clipID)
	if err != nil {
		t.Fatal(err)
	}
	return extract
}

func TestClipExtractCutsTheWindowAndReplaysIt(t *testing.T) {
	f := newExtractFixture(t)
	extract := f.runExtract(t)

	if !strings.HasPrefix(extract.RelPath, "clips/20251205T1514_clip-") || extract.FrameCount != 11 {
		t.Fatalf("extract %+v", extract)
	}
	if extract.ClosedBy != "window" || extract.RubricVersion != 1 {
		t.Fatalf("closed by %q rubric %d", extract.ClosedBy, extract.RubricVersion)
	}
	if _, err := os.Stat(filepath.Join(f.safeDir, filepath.FromSlash(extract.RelPath))); err != nil {
		t.Fatal(err)
	}
	sidecarPath := filepath.Join(f.safeDir, strings.TrimSuffix(extract.RelPath, ".pcapng")+".json")
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	var sidecar dataset.Sidecar
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		t.Fatal(err)
	}
	if sidecar.OutputSHA256 != extract.SHA256 || sidecar.SourceFiles[0].Name != "kirk2.pcapng" ||
		sidecar.Latitude != nil || sidecar.StaticExportPolicy != dataset.ExportPolicy {
		t.Fatalf("sidecar %+v", sidecar)
	}

	rec := f.do(t, http.MethodGet, "/api/lidar/clips/"+f.clipID+"/extract", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), extract.SHA256) {
		t.Fatalf("list extracts: %d %s", rec.Code, rec.Body.String())
	}

	rec = f.do(t, http.MethodPut, "/api/lidar/clips/"+f.clipID+"/publishing", `{"storage":"extract"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("publishing: %d %s", rec.Code, rec.Body.String())
	}
	var opened []string
	var cfg ReplayConfig
	prevStart, prevRun := startPCAPInternalForClip, lastRunIDForClip
	t.Cleanup(func() { startPCAPInternalForClip, lastRunIDForClip = prevStart, prevRun })
	startPCAPInternalForClip = func(_ *Server, path string, c ReplayConfig) error {
		opened, cfg = append(opened, path), c
		return nil
	}
	lastRunIDForClip = func(*Server) string { return "run-1" }
	if rec := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.clipID+"/replay", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("replay: %d %s", rec.Code, rec.Body.String())
	}
	if len(opened) != 1 || opened[0] != extract.RelPath || cfg.StartSeconds != 0 || cfg.DurationSeconds != 0 {
		t.Fatalf("replay opened %v with %+v, want the extract whole", opened, cfg)
	}
}

func TestClipStageWritesTheDatasetAndSiteIndex(t *testing.T) {
	f := newExtractFixture(t)

	if rec := f.do(t, http.MethodPut, "/api/lidar/clips/"+f.clipID+"/publishing",
		`{"dataset_id":"Kirkham","storage":"sources"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad dataset id: %d", rec.Code)
	}
	if rec := f.do(t, http.MethodPut, "/api/lidar/clips/"+f.clipID+"/publishing",
		`{"dataset_id":"kirkham-9th-k3","north_azimuth_deg":131}`); rec.Code != http.StatusOK {
		t.Fatalf("publishing: %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.clipID+"/stage", ""); rec.Code != http.StatusConflict ||
		!strings.Contains(rec.Body.String(), "not located") {
		t.Fatalf("staging an unlocated clip: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := f.store.SetClipLocation(f.clipID, 37.7604, -122.4664, sqlite.GeoSourceOperator); err != nil {
		t.Fatal(err)
	}
	if rec := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.clipID+"/stage", ""); rec.Code != http.StatusConflict ||
		!strings.Contains(rec.Body.String(), "no completed extract") {
		t.Fatalf("staging before extracting: %d %s", rec.Code, rec.Body.String())
	}
	extract := f.runExtract(t)
	if !strings.HasPrefix(extract.RelPath, "clips/20251205T1514_80858-75c_kirkham-9th-k3") {
		t.Fatalf("a located clip with a dataset id takes the dataset stem: %s", extract.RelPath)
	}

	if rec := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.clipID+"/stage", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("staging into a dataset that is not cloned: %d %s", rec.Code, rec.Body.String())
	}
	if err := os.Mkdir(f.ws.datasetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.clipID+"/stage", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stage: %d %s", rec.Code, rec.Body.String())
	}
	target := filepath.Join(f.ws.datasetDir, "raw", "lidar", "20251205T1514_80858-75c_kirkham-9th-k3.pcapng")
	src, _ := os.Stat(filepath.Join(f.safeDir, filepath.FromSlash(extract.RelPath)))
	dst, err := os.Stat(target)
	if err != nil || !os.SameFile(src, dst) {
		t.Fatalf("the dataset file is not the extract: %v", err)
	}
	var manifest []dataset.ManifestEntry
	raw, _ := os.ReadFile(filepath.Join(f.ws.datasetDir, "manifest.json"))
	if err := json.Unmarshal(raw, &manifest); err != nil || len(manifest) != 1 {
		t.Fatalf("manifest %s %v", raw, err)
	}
	if manifest[0].SiteSlug != "kirkham-9th-k3" || manifest[0].RawSHA256 != extract.SHA256 ||
		manifest[0].S2L13 != "8085875c" || manifest[0].ExportProvenance.ClipID != f.clipID {
		t.Fatalf("manifest entry %+v", manifest[0])
	}
	// Staging again is a no-op on the file and replaces the entry.
	if rec := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.clipID+"/stage", ""); rec.Code != http.StatusOK {
		t.Fatalf("re-stage: %d %s", rec.Code, rec.Body.String())
	}

	idx := httptest.NewRecorder()
	f.ws.handleDatasetSiteIndex(idx, httptest.NewRequest(http.MethodGet, "/api/lidar/dataset/site-index", nil))
	var body struct {
		Entries []dataset.SiteIndexEntry `json:"entries"`
	}
	if err := json.Unmarshal(idx.Body.Bytes(), &body); err != nil || len(body.Entries) != 1 {
		t.Fatalf("site index %s %v", idx.Body.String(), err)
	}
	entry := body.Entries[0]
	if entry.ID != "kirkham-9th-k3" || entry.Where != "Kirkham K3" || entry.NorthAzimuthDeg == nil ||
		*entry.NorthAzimuthDeg != 131 || entry.ClipID != f.clipID {
		t.Fatalf("site index entry %+v", entry)
	}
	start, _ := time.Parse(time.RFC3339Nano, entry.Start)
	end, _ := time.Parse(time.RFC3339Nano, entry.End)
	if d := end.Sub(start).Seconds() - manifest[0].DurationSeconds; d > 1e-3 || d < -1e-3 {
		t.Fatalf("the site index and manifest disagree on duration by %v s", d)
	}
}

func TestClipStageNeedsADatasetDirectory(t *testing.T) {
	f := newExtractFixture(t)
	f.ws.datasetDir = ""
	if rec := f.do(t, http.MethodPost, "/api/lidar/clips/"+f.clipID+"/stage", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("stage without a dataset dir: %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/api/lidar/clips/"+f.clipID+"/stage", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET stage: %d", rec.Code)
	}
	if rec := f.do(t, http.MethodPost, "/api/lidar/clips/missing/extract", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("extract of an unknown clip: %d", rec.Code)
	}
}
