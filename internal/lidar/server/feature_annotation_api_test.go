package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
)

func TestFeatureAPI(t *testing.T) {
	root := t.TempDir()
	p := physicalTestPack(t, root, "clip")
	ws := &Server{annotationPacksDir: root}
	path := "/api/annotations/features?pack=clip/pack&pack_digest=" + url.QueryEscape(p.Manifest.PackDigest)
	call := func(method string, b []byte) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		ws.handleFeatureAnnotations(r, httptest.NewRequest(method, path, bytes.NewReader(b)))
		return r
	}
	r := call(http.MethodGet, nil)
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	state := new(pb.FeatureState)
	if err := proto.Unmarshal(r.Body.Bytes(), state); err != nil {
		t.Fatal(err)
	}
	state.Document.Author = "operator"
	b, _ := proto.Marshal(&pb.FeatureEdit{Document: state.Document, MembershipDigest: state.MembershipDigest})
	r = call(http.MethodPost, b)
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if r = call(http.MethodPost, b); r.Code != 409 {
		t.Fatalf("conflict: %d %s", r.Code, r.Body.String())
	}
	if r = call(http.MethodPost, []byte{0xff}); r.Code != 400 {
		t.Fatal(r.Code)
	}
	if r = call(http.MethodDelete, nil); r.Code != 405 {
		t.Fatal(r.Code)
	}
	b, _ = proto.Marshal(&pb.FeatureEdit{})
	if r = call(http.MethodPost, b); r.Code != 400 {
		t.Fatal(r.Code)
	}
	path = "/api/annotations/features?pack=../outside&pack_digest=x"
	if r = call(http.MethodGet, nil); r.Code == 200 {
		t.Fatal("escaped pack root")
	}
}

func TestFeatureAPIRejectsUnknownAndOversizedRequests(t *testing.T) {
	root := t.TempDir()
	p := physicalTestPack(t, root, "clip")
	ws := &Server{annotationPacksDir: root}
	path := "/api/annotations/features?pack=clip/pack&pack_digest=" + url.QueryEscape(p.Manifest.PackDigest)
	for _, payload := range [][]byte{{0x78, 1}, bytes.Repeat([]byte("x"), maxPhysicalRequestBytes+1)} {
		r := httptest.NewRecorder()
		ws.handleFeatureAnnotations(r, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload)))
		if r.Code != 400 {
			t.Fatalf("invalid request: %d", r.Code)
		}
	}
}

func TestFeatureResponseEncodingFailure(t *testing.T) {
	ws := &Server{}
	response := httptest.NewRecorder()
	ws.writeFeatureState(response, &pb.FeatureState{PackDirectory: string([]byte{0xff})})
	if response.Code != 500 {
		t.Fatalf("invalid response: %d %s", response.Code, response.Body.String())
	}
}
