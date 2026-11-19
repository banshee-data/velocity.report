package followingreview

import (
	"bytes"
	"encoding/json"
	"testing"
)

type countedCanonicalReader struct {
	*bytes.Reader
	calls   int
	largest int
}

func (r *countedCanonicalReader) Read(p []byte) (int, error) {
	r.calls++
	r.largest = max(r.largest, len(p))
	return r.Reader.Read(p)
}

func TestCanonicalReadBatchingPreservesExactDigest(t *testing.T) {
	f := newFixture(t)
	f.doc["rows"] = make([]int, 1<<18)
	data, err := json.Marshal(f.doc)
	if err != nil {
		t.Fatal(err)
	}
	reader := &countedCanonicalReader{Reader: bytes.NewReader(data)}
	canonical, err := readCanonical(reader)
	if err != nil || canonical.SHA != digest(data) || len(canonical.Paths) != 1 {
		t.Fatal(canonical.SHA, err)
	}
	t.Logf("%d bytes, %d source reads, largest request %d", len(data), reader.calls, reader.largest)
	if reader.calls > 16 {
		t.Fatal("canonical verification issues excessive small reads")
	}
}
