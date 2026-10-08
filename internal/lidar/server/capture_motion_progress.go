package server

import (
	"fmt"
	"os"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// motionProgressInterval is the longest a running motion pass goes without
// updating its job, and the shortest gap between two updates within one
// capture. A pass reads a 700 MB capture in under a minute; the reader reports
// every hundred packets, far more often than the job row needs writing.
const motionProgressInterval = 2 * time.Second

// indexedExtents returns the session's captures as the index probed them, for
// a motion pass to join without reading every capture once to count it first.
//
// It returns nil, so the pass counts them itself, unless every capture has a
// probe on record and still has the size and modification time it was indexed
// with. A capture rewritten since its probe would otherwise be joined from
// extents that no longer describe it.
func indexedExtents(files []sqlite.CaptureFile, paths []string,
	stat func(string) (os.FileInfo, error)) []capseq.Segment {

	if len(files) == 0 || len(files) != len(paths) {
		return nil
	}
	extents := make([]capseq.Segment, 0, len(files))
	for i, f := range files {
		if f.FirstPacketNs == nil || f.LastPacketNs == nil || f.PacketCount == nil || *f.PacketCount <= 0 {
			return nil
		}
		info, err := stat(paths[i])
		if err != nil || info.Size() != f.SizeBytes || info.ModTime().UnixNano() != f.ModifiedAtNs {
			return nil
		}
		extents = append(extents, capseq.Segment{
			Path:        paths[i],
			FirstPacket: time.Unix(0, *f.FirstPacketNs),
			LastPacket:  time.Unix(0, *f.LastPacketNs),
			PacketCount: uint64(*f.PacketCount),
		})
	}
	return extents
}

// motionProgress turns the reader's packet count into a motion pass's job
// progress: packets read of the session's total, and which capture of the
// session that is. It passes an update on when the capture changes or the
// interval has gone by, and drops the rest.
type motionProgress struct {
	captures int
	// ends is the cumulative packet count at the end of each capture, when
	// the extents are known; without them the capture is estimated from the
	// share of packets read.
	ends     []uint64
	interval time.Duration
	now      func() time.Time

	last        time.Time
	lastCapture int
}

func newMotionProgress(extents []capseq.Segment, captures int, interval time.Duration,
	now func() time.Time) *motionProgress {

	m := &motionProgress{captures: captures, interval: interval, now: now}
	if len(extents) == captures {
		var sum uint64
		for _, e := range extents {
			sum += e.PacketCount
			m.ends = append(m.ends, sum)
		}
	}
	return m
}

// observe reports whether this reading should update the job, and with what.
func (m *motionProgress) observe(current, total uint64) (capjobs.Progress, bool) {
	capture := m.captureAt(current, total)
	t := m.now()
	if capture == m.lastCapture && t.Sub(m.last) < m.interval {
		return capjobs.Progress{}, false
	}
	m.last, m.lastCapture = t, capture

	p := capjobs.Progress{Current: int64(current), Total: int64(total)}
	if capture == 0 {
		p.Detail = fmt.Sprintf("classifying %d captures", m.captures)
	} else {
		p.Detail = fmt.Sprintf("classifying capture %d of %d", capture, m.captures)
	}
	return p, true
}

// captureAt is the 1-based capture holding packet current, or 0 when it
// cannot be told: the reader gave no total and no extents were known.
func (m *motionProgress) captureAt(current, total uint64) int {
	if len(m.ends) > 0 {
		for i, end := range m.ends {
			if current < end {
				return i + 1
			}
		}
		return m.captures
	}
	if total == 0 || m.captures == 0 {
		return 0
	}
	capture := int(current*uint64(m.captures)/total) + 1
	if capture > m.captures {
		capture = m.captures
	}
	return capture
}

// newestProgress delivers progress to write on a goroutine of its own, so the
// goroutine reading captures never waits on the database. While a write is in
// flight, later updates replace each other and only the newest is written;
// a superseded count is worth nothing. stop delivers what is pending and
// returns once the writer has finished.
func newestProgress(write func(capjobs.Progress)) (send func(capjobs.Progress), stop func()) {
	pending := make(chan capjobs.Progress, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for p := range pending {
			write(p)
		}
	}()
	send = func(p capjobs.Progress) {
		for {
			select {
			case pending <- p:
				return
			default:
			}
			// Full: drop the older update, then try again.
			select {
			case <-pending:
			default:
			}
		}
	}
	stop = func() {
		close(pending)
		<-done
	}
	return send, stop
}

// totalPackets is the session's packet count when its extents are known, and
// zero otherwise.
func totalPackets(extents []capseq.Segment) uint64 {
	var sum uint64
	for _, e := range extents {
		sum += e.PacketCount
	}
	return sum
}
