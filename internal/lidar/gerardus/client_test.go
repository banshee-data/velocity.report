package gerardus

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	pb "github.com/banshee-data/velocity.report/internal/lidar/gerarduspb"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func field(cells int) l8analytics.LaneField {
	g := l8analytics.LaneFieldGroup{ClassGroup: "vehicle"}
	for i := range cells {
		g.IX = append(g.IX, uint32(i))
		g.IY = append(g.IY, uint32(i))
		g.Tracks = append(g.Tracks, 5)
		g.BoxTracks = append(g.BoxTracks, 6)
		g.Heading = append(g.Heading, 1, 0, 0, 0, 4, 0, 0, 0)
		g.SpeedSumMps = append(g.SpeedSumMps, 40)
	}
	return l8analytics.LaneField{
		Header: l8analytics.LaneFieldHeader{
			L16Token: "808580f47", SensorID: "hesai-pandar40p", RecordID: "gms:1", CaptureRef: "a.pcapng@2026-09-01T19:25:33Z",
			WindowStart: "2026-09-01T19:25:33Z", WindowEnd: "2026-09-01T19:35:33.5Z", WindowPart: "all",
			CellM: 0.25, Origin: [2]float64{-40, -30}, NX: 320, NY: 240, K: 5, TrackCount: 9,
		},
		Groups: []l8analytics.LaneFieldGroup{g},
	}
}

func TestUploadMessagesChunksEachGroup(t *testing.T) {
	msgs, err := UploadMessages(field(9), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("messages = %d, want a header and three chunks", len(msgs))
	}
	h := msgs[0].GetHeader()
	if h == nil || h.GetL16Token() != "808580f47" || h.GetRecordId() != "gms:1" || h.GetOrigin().GetX() != -40 ||
		h.GetWindowEnd().AsTime() != time.Date(2026, 9, 1, 19, 35, 33, 5e8, time.UTC) {
		t.Fatalf("header = %v", h)
	}
	var cells, headings int
	for _, m := range msgs[1:] {
		c := m.GetCells()
		cells += len(c.GetIx())
		headings += len(c.GetHeading())
		if len(c.GetHeading()) != 8*len(c.GetIx()) {
			t.Fatalf("chunk heading length %d for %d cells", len(c.GetHeading()), len(c.GetIx()))
		}
	}
	if cells != 9 || headings != 72 {
		t.Fatalf("cells %d headings %d", cells, headings)
	}
}

func TestUploadMessagesRefusesAnUnplacedField(t *testing.T) {
	f := field(1)
	f.Header.RecordID = ""
	if _, err := UploadMessages(f, 0); err == nil {
		t.Fatal("a field with no record was accepted")
	}
	f = field(1)
	f.Groups[0].Heading = f.Groups[0].Heading[:7]
	if _, err := UploadMessages(f, 0); err == nil {
		t.Fatal("a ragged group was accepted")
	}
}

func TestGRPCTarget(t *testing.T) {
	for in, want := range map[string]string{
		"http://127.0.0.1:50061": "127.0.0.1:50061", "https://g.example": "g.example:443",
		"arrow-worker:50061": "arrow-worker:50061", "http://arrow": "arrow:80",
	} {
		got, _, err := grpcTarget(in)
		if err != nil || got != want {
			t.Errorf("grpcTarget(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, _, err := grpcTarget("ftp://x"); err == nil {
		t.Error("ftp accepted")
	}
}

func TestCountProposals(t *testing.T) {
	line := func(grade string) *pb.LineFeature {
		return &pb.LineFeature{Tags: map[string]string{"confidence": grade}}
	}
	r := &pb.ListLaneProposalsResponse{ModelId: "lanes:x", Proposals: []*pb.LaneProposal{
		{Line: line("high")},
		{Line: line("high"), AdoptedBy: "mark-1"},
		{Line: line("medium"), Review: &pb.LaneProposalReview{Decision: pb.LaneProposalDecision_LANE_PROPOSAL_DECISION_REJECTED}},
		{Line: line("medium"), Review: &pb.LaneProposalReview{Decision: pb.LaneProposalDecision_LANE_PROPOSAL_DECISION_REOPENED}},
	}}
	got := CountProposals(r)
	want := ProposalCounts{Total: 4, High: 2, Medium: 2, Adopted: 1, Rejected: 1, Outstanding: 2, ModelID: "lanes:x"}
	if got != want {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
}

func TestAcceptedRecord(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	const sensor = "hesai-pandar40p"
	cases := []struct {
		name    string
		records []Record
		refs    []string
		state   string
		record  string
	}{
		{"none", nil, nil, ClipNotRegistered, ""},
		{"one", []Record{{RecordID: "r1", State: StateRegistered, SensorID: sensor, ReviewedAt: day(1)}}, nil, ClipRegistered, "r1"},
		{"manual counts", []Record{{RecordID: "r1", State: StateManuallyPlaced, ReviewedAt: day(1)}}, nil, ClipRegistered, "r1"},
		{"rejected does not", []Record{{RecordID: "r1", State: "rejected", SensorID: sensor}}, nil, ClipNotRegistered, ""},
		{"other sensor", []Record{{RecordID: "r1", State: StateRegistered, SensorID: "hesai-01"}}, nil, ClipOtherSensor, ""},
		{"capture named wins over newer", []Record{
			{RecordID: "r1", State: StateRegistered, SensorID: sensor, CaptureRef: "s2-sf-3.pcapng", ReviewedAt: day(1)},
			{RecordID: "r2", State: StateRegistered, SensorID: sensor, CaptureRef: "s2-sf-9.pcapng", ReviewedAt: day(5)},
		}, []string{"/Volumes/lidar/pcaps/s2-sf-3.pcapng"}, ClipRegistered, "r1"},
		{"two without a name refused", []Record{
			{RecordID: "r1", State: StateRegistered, SensorID: sensor, ReviewedAt: day(1)},
			{RecordID: "r2", State: StateRegistered, SensorID: sensor, ReviewedAt: day(5)},
		}, []string{"x.pcapng"}, ClipAmbiguous, ""},
		{"named without extension", []Record{
			{RecordID: "r1", State: StateRegistered, SensorID: sensor, CaptureRef: "kirk1", ReviewedAt: day(1)},
			{RecordID: "r2", State: StateRegistered, SensorID: sensor, CaptureRef: "kirk2", ReviewedAt: day(5)},
		}, []string{"/c/kirk1.pcapng"}, ClipRegistered, "r1"},
		{"superseded passed over", []Record{
			{RecordID: "r1", State: StateRegistered, SensorID: sensor, ReviewedAt: day(9)},
			{RecordID: "r2", State: StateRegistered, SensorID: sensor, Supersedes: "r1", ReviewedAt: day(5)},
		}, nil, ClipRegistered, "r2"},
		{"tie refused", []Record{
			{RecordID: "r1", State: StateRegistered, SensorID: sensor, ReviewedAt: day(5)},
			{RecordID: "r2", State: StateRegistered, SensorID: sensor, ReviewedAt: day(5)},
		}, nil, ClipAmbiguous, ""},
	}
	for _, c := range cases {
		got := AcceptedRecord(c.records, sensor, c.refs)
		if got.State != c.state || got.RecordID != c.record {
			t.Errorf("%s: got %+v, want %s %s", c.name, got, c.state, c.record)
		}
		if got.State == ClipAmbiguous && len(got.Candidates) < 2 {
			t.Errorf("%s: ambiguous without candidates: %+v", c.name, got)
		}
	}
}

type fakeRegistration struct {
	pb.UnimplementedRegistrationServiceServer
	headers []*pb.LaneFieldHeader
	cells   int
}

func (f *fakeRegistration) UploadLaneField(s grpc.ClientStreamingServer[pb.UploadLaneFieldRequest, pb.UploadLaneFieldResponse]) error {
	for {
		m, err := s.Recv()
		if err == io.EOF {
			return s.SendAndClose(&pb.UploadLaneFieldResponse{FieldId: "sha256:abc", Cells: uint32(f.cells), AlreadyStored: len(f.headers) > 1})
		}
		if err != nil {
			return err
		}
		if h := m.GetHeader(); h != nil {
			f.headers = append(f.headers, h)
			f.cells = 0
		}
		f.cells += len(m.GetCells().GetIx())
	}
}

func TestUploadLaneFieldStreams(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	fake := &fakeRegistration{}
	pb.RegisterRegistrationServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()
	conn, err := grpc.NewClient("passthrough:///buf", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{conn: conn, reg: pb.NewRegistrationServiceClient(conn)}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	up, err := c.UploadLaneField(ctx, field(UploadChunkCells+3))
	if err != nil {
		t.Fatal(err)
	}
	if up.FieldID != "sha256:abc" || up.Cells != UploadChunkCells+3 || up.Part != "all" || up.AlreadyStored {
		t.Fatalf("upload = %+v", up)
	}
	if again, err := c.UploadLaneField(ctx, field(2)); err != nil || !again.AlreadyStored {
		t.Fatalf("repeat = %+v, %v", again, err)
	}
}

func TestPinnedRecord(t *testing.T) {
	recs := []Record{
		{RecordID: "a", State: StateRegistered, SensorID: "s"},
		{RecordID: "b", State: "rejected", SensorID: "s"},
		{RecordID: "c", State: StateRegistered, SensorID: "other"},
		{RecordID: "d", State: StateManuallyPlaced, SensorID: "s", Supersedes: "e"},
		{RecordID: "e", State: StateRegistered, SensorID: "s"},
	}
	for id, want := range map[string]string{"a": ClipRegistered, "d": ClipRegistered, "b": ClipNotRegistered, "c": ClipOtherSensor, "e": ClipNotRegistered, "z": ClipNotRegistered} {
		if got := PinnedRecord(recs, "s", id); got.State != want {
			t.Errorf("PinnedRecord(%s) = %+v, want %s", id, got, want)
		}
	}
}
