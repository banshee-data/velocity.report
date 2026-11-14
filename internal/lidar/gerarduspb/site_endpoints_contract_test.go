package gerarduspb_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	pb "github.com/banshee-data/velocity.report/internal/lidar/gerarduspb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Exercise the vendored API over a real gRPC transport: capability discovery,
// streaming progress/result oneofs, request fields and the older-server error
// must agree. This complements Swift session tests and upstream fit tests.
type siteEndpointContract struct {
	pb.UnimplementedSiteServiceServer
}

func (siteEndpointContract) GetServerInfo(context.Context, *pb.GetServerInfoRequest) (*pb.GetServerInfoResponse, error) {
	return &pb.GetServerInfoResponse{SupportsRoadGeneration: true, RoadSurfaceVersion: 1}, nil
}

func (siteEndpointContract) PrepareSite(r *pb.PrepareSiteRequest, s grpc.ServerStreamingServer[pb.PrepareSiteResponse]) error {
	if r.L16Token != "808580aab" || r.Refresh || r.Label != "Ashbury" {
		return status.Error(codes.InvalidArgument, "unexpected preparation request")
	}
	if err := s.Send(&pb.PrepareSiteResponse{Event: &pb.PrepareSiteResponse_Progress{Progress: &pb.Progress{Stage: "reference"}}}); err != nil {
		return err
	}
	return s.Send(&pb.PrepareSiteResponse{Event: &pb.PrepareSiteResponse_Site{Site: &pb.Site{L16Token: r.L16Token}}})
}

func (siteEndpointContract) GenerateRoadGeometry(r *pb.GenerateRoadGeometryRequest, s grpc.ServerStreamingServer[pb.GenerateRoadGeometryResponse]) error {
	if r.L16Token != "808580aab" || r.RadiusM != 150 || r.HalfWidthM != 4 || !r.Refresh {
		return status.Error(codes.InvalidArgument, "unexpected road request")
	}
	if err := s.Send(&pb.GenerateRoadGeometryResponse{Event: &pb.GenerateRoadGeometryResponse_Progress{Progress: &pb.Progress{Stage: "fit"}}}); err != nil {
		return err
	}
	return s.Send(&pb.GenerateRoadGeometryResponse{Event: &pb.GenerateRoadGeometryResponse_Result{Result: &pb.GeneratedRoadGeometry{
		L16Token: r.L16Token, CorridorId: "site-" + r.L16Token, BuildHash: "synthetic-contract", PatchCount: 2,
	}}})
}

func contractClient(t *testing.T, service pb.SiteServiceServer) pb.SiteServiceClient {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	pb.RegisterSiteServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///site-contract",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewSiteServiceClient(conn)
}

func TestSitePreparationAndGenerationWireContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := contractClient(t, siteEndpointContract{})
	info, err := client.GetServerInfo(ctx, &pb.GetServerInfoRequest{})
	if err != nil || !info.GetSupportsRoadGeneration() || info.GetRoadSurfaceVersion() != 1 {
		t.Fatalf("capabilities: %v, %v", info, err)
	}
	prepare, err := client.PrepareSite(ctx, &pb.PrepareSiteRequest{L16Token: "808580aab", Label: "Ashbury"})
	if err != nil {
		t.Fatal(err)
	}
	progress, err := prepare.Recv()
	if err != nil || progress.GetProgress().GetStage() != "reference" {
		t.Fatalf("preparation progress: %v, %v", progress, err)
	}
	prepared, err := prepare.Recv()
	if err != nil || prepared.GetSite().GetL16Token() != "808580aab" {
		t.Fatalf("prepared site: %v, %v", prepared, err)
	}
	if _, err := prepare.Recv(); err != io.EOF {
		t.Fatalf("preparation completion: %v", err)
	}
	generate, err := client.GenerateRoadGeometry(ctx, &pb.GenerateRoadGeometryRequest{
		L16Token: prepared.GetSite().GetL16Token(), RadiusM: 150, HalfWidthM: 4, Refresh: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	fitting, err := generate.Recv()
	if err != nil || fitting.GetProgress().GetStage() != "fit" {
		t.Fatalf("generation progress: %v, %v", fitting, err)
	}
	generated, err := generate.Recv()
	result := generated.GetResult()
	if err != nil || result.GetL16Token() != "808580aab" || result.GetCorridorId() != "site-808580aab" || result.GetPatchCount() != 2 || result.GetBuildHash() == "" {
		t.Fatalf("generation result: %v, %v", result, err)
	}
	if _, err := generate.Recv(); err != io.EOF {
		t.Fatalf("generation completion: %v", err)
	}
}

func TestOlderSiteServerPreservesUnimplementedStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := contractClient(t, &pb.UnimplementedSiteServiceServer{})
	prepare, err := client.PrepareSite(ctx, &pb.PrepareSiteRequest{L16Token: "808580aab"})
	if err == nil {
		_, err = prepare.Recv()
	}
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("preparation error lost its status: %v", err)
	}
	generate, err := client.GenerateRoadGeometry(ctx, &pb.GenerateRoadGeometryRequest{L16Token: "808580aab"})
	if err == nil {
		_, err = generate.Recv()
	}
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("generation error lost its status: %v", err)
	}
}
