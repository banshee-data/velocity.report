package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
)

// joinRecordWriter writes the motion join's records as JSON lines, one per
// joined cluster. The pipeline hook cannot return an error, so the first one
// is kept, later records are dropped, and close reports it: a partial file is
// never mistaken for a complete one.
type joinRecordWriter struct {
	buf   *bufio.Writer
	enc   *json.Encoder
	count int
	err   error
}

func newJoinRecordWriter(w io.Writer) *joinRecordWriter {
	buf := bufio.NewWriter(w)
	return &joinRecordWriter{buf: buf, enc: json.NewEncoder(buf)}
}

func (j *joinRecordWriter) write(records []pipeline.FragmentJoinRecord) {
	for _, r := range records {
		if j.err != nil {
			return
		}
		if err := j.enc.Encode(r); err != nil {
			j.err = fmt.Errorf("record %d: %w", j.count+1, err)
			return
		}
		j.count++
	}
}

func (j *joinRecordWriter) close() error {
	if j.err != nil {
		return j.err
	}
	return j.buf.Flush()
}
