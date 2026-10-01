package server

import (
	"bytes"
	"fmt"
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

func TestFeatureRevisionAPIIsExactAndReadOnly(t *testing.T) {
	root := t.TempDir()
	p := physicalTestPack(t, root, "clip")
	ws := &Server{annotationPacksDir: root}
	path := "/api/annotations/features?pack=clip/pack&pack_digest=" + url.QueryEscape(p.Manifest.PackDigest)
	call := func(method, suffix string, payload []byte) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		ws.handleFeatureAnnotations(r, httptest.NewRequest(method, path+suffix, bytes.NewReader(payload)))
		return r
	}
	state := new(pb.FeatureState)
	if err := proto.Unmarshal(call("GET", "", nil).Body.Bytes(), state); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		state.Document.Author = "operator"
		b, _ := proto.Marshal(&pb.FeatureEdit{Document: state.Document, BaseDigest: state.Digest, MembershipDigest: state.MembershipDigest})
		r := call("POST", "", b)
		if r.Code != 200 {
			t.Fatal(r.Body.String())
		}
		if err := proto.Unmarshal(r.Body.Bytes(), state); err != nil {
			t.Fatal(err)
		}
	}
	for _, revision := range []string{"1", "2"} {
		r := call("GET", "&revision="+revision, nil)
		if r.Code != 200 {
			t.Fatal(r.Body.String())
		}
		var retained pb.FeatureState
		if err := proto.Unmarshal(r.Body.Bytes(), &retained); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(retained.Document.Revision) != revision {
			t.Fatal("wrong retained revision")
		}
	}
	for _, suffix := range []string{"&revision=", "&revision=0", "&revision=-1", "&revision=abc", "&revision=2147483648", "&revision=1&revision=2", "&revision=99"} {
		if r := call("GET", suffix, nil); r.Code != 400 {
			t.Fatalf("%s: %d", suffix, r.Code)
		}
	}
	if r := call("POST", "&revision=1", nil); r.Code != 400 {
		t.Fatal("historical edit allowed")
	}
}
