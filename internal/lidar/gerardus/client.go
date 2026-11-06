// Package gerardus is the Go server's client to Gerardus's alignment API
// (proto/gerardus/align/v1alpha1, vendored), for the lane proposals job
// (docs/plans/lidar-lane-field-batch-plan.md): which accepted registration
// record places a clip, uploading a clip's lane fields against it, and
// asking for the proposals Gerardus derives from them.
//
// It sends nothing but the lane field, which has no cell under k distinct
// tracks (ADR-0011), and reads nothing it stores: the proposals are
// Gerardus's (ADR-0016, D-39). The visualiser keeps its own Swift client for
// review and approval.
package gerardus

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	pb "github.com/banshee-data/velocity.report/internal/lidar/gerarduspb"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DefaultAddress is Gerardus's default listener, where gRPC and its HTTP
// API share one port; GERARDUS_URL overrides it, as for the CLI.
const DefaultAddress = "http://127.0.0.1:50061"

// UploadChunkCells is how many cells go in one upload message. Gerardus takes
// a few megabytes a message; a cell is at most about 60 bytes.
const UploadChunkCells = 4000

// Address returns the configured address, or GERARDUS_URL, or the default.
func Address(configured string) string {
	if a := strings.TrimSpace(configured); a != "" {
		return a
	}
	if a := strings.TrimSpace(os.Getenv("GERARDUS_URL")); a != "" {
		return a
	}
	return DefaultAddress
}

// Client holds one connection to a Gerardus server.
type Client struct {
	Address string
	conn    *grpc.ClientConn
	site    pb.SiteServiceClient
	reg     pb.RegistrationServiceClient
}

// Dial connects to address, an http:// or https:// URL or a bare host:port
// (plaintext). The connection is lazy: an unreachable server fails the first
// call, not Dial.
func Dial(address string) (*Client, error) {
	target, secure, err := grpcTarget(address)
	if err != nil {
		return nil, err
	}
	creds := insecure.NewCredentials()
	if secure {
		creds = credentials.NewClientTLSFromCert(nil, "")
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64<<20)))
	if err != nil {
		return nil, fmt.Errorf("gerardus %s: %w", address, err)
	}
	return &Client{Address: address, conn: conn, site: pb.NewSiteServiceClient(conn), reg: pb.NewRegistrationServiceClient(conn)}, nil
}

// Close releases the connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func grpcTarget(address string) (string, bool, error) {
	a := strings.TrimSpace(address)
	if a == "" {
		return "", false, errors.New("no Gerardus address")
	}
	if !strings.Contains(a, "://") {
		return a, false, nil
	}
	u, err := url.Parse(a)
	if err != nil {
		return "", false, fmt.Errorf("gerardus address %q: %w", address, err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return "", false, fmt.Errorf("gerardus address %q: scheme must be http or https", address)
	}
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	return host, u.Scheme == "https", nil
}

// ServerInfo is what the job needs from GetServerInfo.
type ServerInfo struct {
	Version            string `json:"server_version"`
	SupportsLaneFields bool   `json:"supports_lane_fields"`
}

// ServerInfo asks the server what it is and whether it takes lane fields.
func (c *Client) ServerInfo(ctx context.Context) (ServerInfo, error) {
	r, err := c.site.GetServerInfo(ctx, &pb.GetServerInfoRequest{})
	if err != nil {
		return ServerInfo{}, fmt.Errorf("gerardus GetServerInfo: %w", err)
	}
	return ServerInfo{Version: r.GetServerVersion(), SupportsLaneFields: r.GetSupportsLaneFields()}, nil
}

// Records lists a site's registration records, every state.
func (c *Client) Records(ctx context.Context, l16 string) ([]Record, error) {
	r, err := c.reg.ListRegistrationRecords(ctx, &pb.ListRegistrationRecordsRequest{L16Token: l16})
	if err != nil {
		return nil, fmt.Errorf("gerardus ListRegistrationRecords %s: %w", l16, err)
	}
	out := make([]Record, 0, len(r.GetRecords()))
	for _, rec := range r.GetRecords() {
		out = append(out, recordOf(rec))
	}
	return out, nil
}

func recordOf(rec *pb.RegistrationRecord) Record {
	out := Record{
		RecordID: rec.GetRecordId(), State: stateName(rec.GetState()), L16Token: rec.GetL16Token(),
		ScanID: rec.GetScanId(), CaptureRef: rec.GetCaptureRef(), SensorID: rec.GetSensorId(),
		Supersedes: rec.GetSupersedesRecordId(),
	}
	if t := rec.GetReviewedAt(); t != nil {
		out.ReviewedAt = t.AsTime()
	}
	return out
}

func stateName(s pb.RegistrationState) string {
	switch s {
	case pb.RegistrationState_REGISTRATION_STATE_REGISTERED:
		return StateRegistered
	case pb.RegistrationState_REGISTRATION_STATE_MANUALLY_PLACED:
		return StateManuallyPlaced
	case pb.RegistrationState_REGISTRATION_STATE_REJECTED:
		return "rejected"
	case pb.RegistrationState_REGISTRATION_STATE_CANDIDATE:
		return "candidate"
	case pb.RegistrationState_REGISTRATION_STATE_FAILED:
		return "failed"
	case pb.RegistrationState_REGISTRATION_STATE_RUNNING:
		return "running"
	default:
		return "unspecified"
	}
}

// Upload is what one UploadLaneField answered.
type Upload struct {
	Part          string `json:"part"`
	FieldID       string `json:"field_id"`
	Cells         uint32 `json:"cells"`
	AlreadyStored bool   `json:"already_stored"`
}

// UploadLaneField streams one part of a lane field: the header, then its
// cells in chunks. The field's header must already name the site and the
// record. A repeat of a stored field answers AlreadyStored with the same id.
func (c *Client) UploadLaneField(ctx context.Context, f l8analytics.LaneField) (Upload, error) {
	msgs, err := UploadMessages(f, UploadChunkCells)
	if err != nil {
		return Upload{}, err
	}
	stream, err := c.reg.UploadLaneField(ctx)
	if err != nil {
		return Upload{}, fmt.Errorf("gerardus UploadLaneField: %w", err)
	}
	for _, m := range msgs {
		if err := stream.Send(m); err != nil {
			// The server's reason arrives on CloseAndRecv.
			if _, rerr := stream.CloseAndRecv(); rerr != nil {
				return Upload{}, fmt.Errorf("gerardus UploadLaneField %s: %w", f.Header.WindowPart, rerr)
			}
			return Upload{}, fmt.Errorf("gerardus UploadLaneField %s: %w", f.Header.WindowPart, err)
		}
	}
	r, err := stream.CloseAndRecv()
	if err != nil {
		return Upload{}, fmt.Errorf("gerardus UploadLaneField %s: %w", f.Header.WindowPart, err)
	}
	return Upload{Part: f.Header.WindowPart, FieldID: r.GetFieldId(), Cells: r.GetCells(), AlreadyStored: r.GetAlreadyStored()}, nil
}

// UploadMessages turns a lane field into the upload stream: one header
// message, then each class group's cells in chunks of at most chunk cells.
// A group with no cells sends nothing.
func UploadMessages(f l8analytics.LaneField, chunk int) ([]*pb.UploadLaneFieldRequest, error) {
	h := f.Header
	switch {
	case h.L16Token == "":
		return nil, errors.New("lane field header names no site")
	case h.RecordID == "":
		return nil, errors.New("lane field header names no accepted record")
	case h.SensorID == "":
		return nil, errors.New("lane field header names no sensor")
	case h.CaptureRef == "":
		return nil, errors.New("lane field header names no capture")
	}
	if chunk <= 0 {
		chunk = UploadChunkCells
	}
	header := &pb.LaneFieldHeader{
		L16Token: h.L16Token, SensorId: h.SensorID, RecordId: h.RecordID, CaptureRef: h.CaptureRef,
		WindowPart: h.WindowPart, CellM: float32(h.CellM), Origin: &pb.Vector3{X: h.Origin[0], Y: h.Origin[1]},
		Nx: h.NX, Ny: h.NY, K: h.K, PipelineBuild: h.PipelineBuild, TuningFingerprint: h.TuningFingerprint,
		TrackCount: h.TrackCount,
	}
	var err error
	if header.WindowStart, err = timestampOf(h.WindowStart); err != nil {
		return nil, fmt.Errorf("lane field window_start: %w", err)
	}
	if header.WindowEnd, err = timestampOf(h.WindowEnd); err != nil {
		return nil, fmt.Errorf("lane field window_end: %w", err)
	}
	out := []*pb.UploadLaneFieldRequest{{Part: &pb.UploadLaneFieldRequest_Header{Header: header}}}
	for _, g := range f.Groups {
		n := len(g.IX)
		if len(g.IY) != n || len(g.Tracks) != n || len(g.BoxTracks) != n || len(g.SpeedSumMps) != n || len(g.Heading) != 8*n {
			return nil, fmt.Errorf("lane field group %s has ragged channels", g.ClassGroup)
		}
		for lo := 0; lo < n; lo += chunk {
			hi := min(lo+chunk, n)
			out = append(out, &pb.UploadLaneFieldRequest{Part: &pb.UploadLaneFieldRequest_Cells{Cells: &pb.LaneFieldCells{
				ClassGroup: g.ClassGroup, Ix: g.IX[lo:hi], Iy: g.IY[lo:hi], Tracks: g.Tracks[lo:hi],
				BoxTracks: g.BoxTracks[lo:hi], Heading: g.Heading[8*lo : 8*hi], SpeedSumMps: g.SpeedSumMps[lo:hi],
			}}})
		}
	}
	return out, nil
}

func timestampOf(s string) (*timestamppb.Timestamp, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil, err
	}
	return timestamppb.New(t), nil
}

// ProposalCounts is a site's proposals counted the way the painter reads
// them: by the grade tag, by whether an authored lane adopted them, and by
// whether their latest review is a rejection. Outstanding is neither adopted
// nor rejected.
type ProposalCounts struct {
	Total       int     `json:"total"`
	High        int     `json:"high"`
	Medium      int     `json:"medium"`
	Adopted     int     `json:"adopted"`
	Rejected    int     `json:"rejected"`
	Outstanding int     `json:"outstanding"`
	ModelID     string  `json:"model_id,omitempty"`
	DeriveSecs  float64 `json:"derive_secs"`
}

// Proposals asks for a site's proposals, which derives them first when the
// fields changed, and counts them. DeriveSecs is the call's wall time.
func (c *Client) Proposals(ctx context.Context, l16 string) (ProposalCounts, error) {
	start := time.Now()
	r, err := c.site.ListLaneProposals(ctx, &pb.ListLaneProposalsRequest{L16Token: l16})
	if err != nil {
		return ProposalCounts{}, fmt.Errorf("gerardus ListLaneProposals %s: %w", l16, err)
	}
	out := CountProposals(r)
	out.DeriveSecs = time.Since(start).Seconds()
	return out, nil
}

// CountProposals counts a ListLaneProposals response.
func CountProposals(r *pb.ListLaneProposalsResponse) ProposalCounts {
	out := ProposalCounts{ModelID: r.GetModelId()}
	for _, p := range r.GetProposals() {
		out.Total++
		switch strings.ToLower(p.GetLine().GetTags()["confidence"]) {
		case "high":
			out.High++
		case "medium":
			out.Medium++
		}
		adopted := p.GetAdoptedBy() != ""
		rejected := p.GetReview().GetDecision() == pb.LaneProposalDecision_LANE_PROPOSAL_DECISION_REJECTED
		switch {
		case adopted:
			out.Adopted++
		case rejected:
			out.Rejected++
		default:
			out.Outstanding++
		}
	}
	return out
}
