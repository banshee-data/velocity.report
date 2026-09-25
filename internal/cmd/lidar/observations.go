//go:build pcap
// +build pcap

package lidar

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

const observationsUsage = `velocity lidar observations — check VRLOG 1.x observation containers

Usage:
  velocity lidar observations verify [--require PROFILE] [--rebuild-index] DIR...
  velocity lidar observations inspect [--records N] DIR
  velocity lidar observations compare DIR DIR
  velocity lidar observations recover DIR

Commands:
  verify   Read every committed record: the commit chain, chunk digests,
           record checksums, indexes against their chunks, the stream
           contract, and the semantic digests the writer sealed. Exits 1 if
           any container fails, naming the damaged object, bytes and source
           sequences. Reports how the capture ended.
  inspect  Print the manifest and commit policy, the commit state, the
           committed chunks and the first records.
  compare  Compare two containers record by record, value for value. Capture
           identity and chunking may differ; evidence may not. Use it to check
           that a repeat extraction of the same source reproduces the first.
  recover  Make an interrupted capture consistent for offline readers: promote
           generations published but never acknowledged, move uncommitted
           objects under quarantine/, and record both in a recovery generation
           that marks the capture incomplete. Committed evidence is never
           rewritten; damage inside it is reported and left alone. Refuses a
           container a live writer holds. Running it twice changes nothing.

Write a container with:
  velocity lidar pcap-replay --pcap FILE --output DIR --observations OBS \
      --replay-case-id ID`

// ObservationsMain routes `velocity lidar observations`. args follow the
// command word. It returns the exit code.
func ObservationsMain(args []string) int {
	if len(args) == 0 {
		fmt.Println(observationsUsage)
		return 0
	}
	switch args[0] {
	case "verify":
		return observationsVerify(args[1:])
	case "inspect":
		return observationsInspect(args[1:])
	case "compare":
		return observationsCompare(args[1:])
	case "recover":
		return observationsRecover(args[1:])
	case "help", "-h", "--help":
		fmt.Println(observationsUsage)
		return 0
	}
	fmt.Fprintf(os.Stderr, "unknown observations command: %q\n\n%s\n", args[0], observationsUsage)
	return 2
}

func parseObservationFlags(fs *flag.FlagSet, args []string) (int, bool) {
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, observationsUsage)
		fmt.Fprintln(os.Stderr, "\nOptions:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0, false
		}
		return 2, false
	}
	return 0, true
}

func observationsVerify(args []string) int {
	fs := flag.NewFlagSet("velocity-lidar-observations-verify", flag.ContinueOnError)
	require := fs.String("require", "", "Fail unless the container's profile satisfies this evidence profile (e.g. foreground-complete)")
	rebuild := fs.Bool("rebuild-index", false, "Rebuild missing or damaged chunk indexes in memory from their chunks (never rewrites files)")
	if code, ok := parseObservationFlags(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one container directory is required")
		return 2
	}
	opts := vrlog.Options{RebuildIndexes: *rebuild}
	if *require != "" {
		profile, err := l4bobserve.LookupProfile(l4bobserve.ProfileName(*require))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
		opts.Require = profile.Capabilities.List()
	}
	failed := 0
	for _, dir := range fs.Args() {
		start := time.Now()
		report, err := verifyContainer(dir, opts)
		if err != nil {
			fmt.Printf("FAIL %s: %v\n", dir, err)
			failed++
			continue
		}
		fmt.Printf("ok   %s: %d frames, %d gaps, sequences [0, %d), %d chunks, %d bytes, generation %d; semantic %s (%s)\n",
			dir, report.Frames, report.Gaps, report.EndSequence, report.Chunks, report.ChunkBytes, report.Generation,
			report.Semantic, time.Since(start).Round(time.Millisecond))
		for _, line := range describeState(report.Status) {
			fmt.Printf("     %s\n", line)
		}
		if len(report.IndexesRebuilt) > 0 {
			fmt.Printf("     indexes rebuilt in memory for chunks %v\n", report.IndexesRebuilt)
		}
		if report.AncillarySkipped > 0 {
			fmt.Printf("     %d ancillary records of unknown kinds skipped\n", report.AncillarySkipped)
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func verifyContainer(dir string, opts vrlog.Options) (vrlog.VerifyReport, error) {
	r, err := vrlog.Open(dir, opts)
	if err != nil {
		return vrlog.VerifyReport{}, err
	}
	defer r.Close()
	return r.Verify()
}

func observationsInspect(args []string) int {
	fs := flag.NewFlagSet("velocity-lidar-observations-inspect", flag.ContinueOnError)
	records := fs.Int("records", 10, "Records to list (0 = none, -1 = all)")
	if code, ok := parseObservationFlags(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "error: exactly one container directory is required")
		return 2
	}
	r, err := vrlog.Open(fs.Arg(0), vrlog.Options{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "inspect: %v\n", err)
		return 1
	}
	defer r.Close()
	printManifest(r.Manifest())
	printChunks(r)
	if *records == 0 {
		return 0
	}
	fmt.Printf("\n%-8s %-6s %-11s %-10s %8s %8s %10s  %s\n", "SEQ", "KIND", "DISPOSITION", "COMPLETE", "POINTS", "CLUSTERS", "UNASSIGNED", "NOTE")
	for listed := 0; *records < 0 || listed < *records; listed++ {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			return 0
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "inspect: %v\n", err)
			return 1
		}
		printRecord(rec)
	}
	return 0
}

func printManifest(m vrlog.Manifest) {
	caps := make([]string, 0)
	for _, c := range m.Profile.Capabilities.List() {
		caps = append(caps, string(c))
	}
	fmt.Printf("profile      %s\ncapabilities %s\n", m.Profile.Name, strings.Join(caps, ", "))
	fmt.Printf("capture      %s (%s, sensor %s, created %s)\n", m.Capture.UUID, m.Capture.SourceType, m.Capture.SensorID,
		time.Unix(0, m.Capture.CreatedUnixNanos).UTC().Format(time.RFC3339))
	e := m.Extraction
	fmt.Printf("source       %s\ncalibration  %s (%s -> %s)\nextractor    %s\nframe        %s\n",
		e.SourceID, e.CalibrationID, m.Calibration.FromFrame, m.Calibration.ToFrame, e.ExtractorID, e.CoordinateFrame)
	if e.ReplayCaseID != "" {
		fmt.Printf("replay case  %s, window start %s, warm-up %s, duration %s\n", e.ReplayCaseID,
			time.Duration(e.Window.StartOffsetNanos), time.Duration(e.Window.WarmupNanos), time.Duration(e.Window.DurationNanos))
	}
	for _, f := range e.CaptureFiles {
		fmt.Printf("capture file %s sha256:%s\n", f.Path, f.SHA256)
	}
	p := m.Provenance
	fmt.Printf("written by   %s %s (%s), params %s\n", p.Writer, p.BuildVersion, p.BuildGitSHA, p.ParamsHash)
	for _, o := range m.Metadata {
		fmt.Printf("metadata     %s (%s, %d bytes, sha256:%s)\n", o.Name, o.MediaType, len(o.Content), o.SHA256)
	}
	l := m.Limits
	fmt.Printf("limits       chunk target %d / max %d bytes, record %d bytes, %d points, %d clusters per frame\n",
		l.TargetChunkBytes, l.MaxChunkBytes, l.MaxRecordBytes, l.MaxPointsPerFrame, l.MaxClustersPerFrame)
	c := m.Commit
	mode := fmt.Sprintf("group commit at %s or %d bytes", c.MaxBatchAge, c.MaxBatchBytes)
	if c.Strict {
		mode = "strict (every record durable before its append returns)"
	}
	loss := c.CrashLoss(l)
	fmt.Printf("commit       %s; deadline %s, shed after %s; crash loss up to %s, %d bytes, %d frames (process crash, not power loss)\n",
		mode, c.CommitDeadline, c.ShedAfter, loss.Interval, loss.Bytes, loss.Frames)
}

// describeState says how a capture ended and what lies beyond its
// committed prefix, one line each.
func describeState(st vrlog.Status) []string {
	var out []string
	switch st.State {
	case vrlog.CaptureClosed:
		out = append(out, "closed: a close generation committed the summary")
	case vrlog.CaptureFailed:
		f := st.Failure
		out = append(out, fmt.Sprintf("FAILED (%s): %s; accepted to sequence %d, committed to %d", f.Cause, f.Detail,
			f.AcceptedEndSequence, st.EndSequence))
	case vrlog.CaptureIncomplete:
		out = append(out, "INCOMPLETE: recovered after an interruption; the tail after the committed end is unknown")
	default:
		out = append(out, "OPEN: no terminal generation (still being written, or interrupted: run recover)")
	}
	if rec := st.Recovery; rec != nil {
		out = append(out, fmt.Sprintf("recovered at generation %d by %s: promoted %v, quarantined %d objects",
			rec.Generation, rec.RecoveredBy, rec.Promoted, rec.QuarantinedCount))
	}
	if st.PointerFallback != "" {
		out = append(out, "current pointer unusable, fell back to the validated chain: "+st.PointerFallback)
	}
	if len(st.Unpromoted) > 0 {
		out = append(out, fmt.Sprintf("generations %v published but never acknowledged: not read (recover promotes them)", st.Unpromoted))
	}
	if st.RejectedFrames+st.ShedFrames > 0 {
		out = append(out, fmt.Sprintf("%d frames over a limit and %d shed under writer backlog, each recorded as a gap",
			st.RejectedFrames, st.ShedFrames))
	}
	for _, tail := range st.UncommittedTail {
		out = append(out, "uncommitted, not evidence: "+tail)
	}
	return out
}

func printChunks(r *vrlog.Reader) {
	st := r.Status()
	fmt.Printf("\n%d chunks, %d records (%d frames, %d gaps), sequences [0, %d), %d bytes, generation %d\n",
		st.Chunks, st.Records, st.Frames, st.Gaps, st.EndSequence, st.ChunkBytes, st.Generation)
	for _, line := range describeState(st) {
		fmt.Println(line)
	}
	if s, ok := r.Summary(); ok && s.Commits > 0 {
		fmt.Printf("commit latency over %d generations: p50 %s, p95 %s, max %s\n", s.Commits,
			s.CommitLatency.P50, s.CommitLatency.P95, s.CommitLatency.Max)
	}
	fmt.Printf("%-8s %10s %8s %20s  %s\n", "CHUNK", "BYTES", "RECORDS", "SEQUENCES", "FIRST START (UTC)")
	for _, c := range r.Chunks() {
		first := "-"
		if c.HaveTime {
			first = time.Unix(0, c.FirstStartUnixNanos).UTC().Format(time.RFC3339Nano)
		}
		fmt.Printf("%-8d %10d %8d %20s  %s\n", c.Ordinal, c.Bytes, c.Records, fmt.Sprintf("[%d, %d)", c.FirstSequence, c.EndSequence), first)
	}
}

func printRecord(rec vrlog.Record) {
	switch rec.Kind {
	case vrlog.RecordFrame:
		f := rec.Frame
		note := ""
		if f.Disposition.Reason != "" {
			note = fmt.Sprintf("%s: %s", f.Disposition.Stage, f.Disposition.Reason)
		}
		fmt.Printf("%-8d %-6s %-11s %-10s %8d %8d %10d  %s\n", f.Sequence, "frame", f.Disposition.Kind, f.Completeness.State,
			f.Points.Len(), len(f.Clusters), len(f.Unassigned), note)
	case vrlog.RecordGap:
		g := rec.Gap
		seq := "-"
		if g.HasSequenceRange {
			seq = fmt.Sprintf("%d-%d", g.FirstSequence, g.LastSequence)
		}
		fmt.Printf("%-8s %-6s %-11s %-10s %8s %8s %10s  %s (missing frames %s)\n", seq, "gap", "-", "-", "-", "-", "-", g.Cause, g.MissingFrames)
	}
}

func observationsRecover(args []string) int {
	fs := flag.NewFlagSet("velocity-lidar-observations-recover", flag.ContinueOnError)
	if code, ok := parseObservationFlags(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "error: exactly one container directory is required")
		return 2
	}
	report, err := vrlog.Recover(fs.Arg(0), vrlog.Options{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "recover: %v\n", err)
		return 1
	}
	action := "nothing to do: the container is consistent"
	switch {
	case report.Wrote:
		action = fmt.Sprintf("wrote recovery generation %d", report.Generation)
	case report.PointerRewritten:
		action = fmt.Sprintf("completed an interrupted recovery: pointer now names generation %d", report.Generation)
	}
	fmt.Printf("%s: %s; %s, %d records to sequence %d\n", fs.Arg(0), action, report.State, report.Records, report.EndSequence)
	if report.PointerDetail != "" {
		fmt.Printf("  pointer was unusable: %s\n", report.PointerDetail)
	}
	if len(report.Promoted) > 0 {
		fmt.Printf("  promoted generations %v (published, never acknowledged)\n", report.Promoted)
	}
	for _, q := range report.Quarantined {
		fmt.Printf("  quarantined %s\n", q)
	}
	return 0
}

func observationsCompare(args []string) int {
	fs := flag.NewFlagSet("velocity-lidar-observations-compare", flag.ContinueOnError)
	if code, ok := parseObservationFlags(fs, args); !ok {
		return code
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "error: exactly two container directories are required")
		return 2
	}
	same, err := compareContainers(os.Stdout, fs.Arg(0), fs.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "compare: %v\n", err)
		return 1
	}
	if !same {
		return 1
	}
	return 0
}

// compareContainers reports whether two containers hold the same evidence,
// printing identities and the first difference.
func compareContainers(out io.Writer, a, b string) (bool, error) {
	ra, err := vrlog.Open(a, vrlog.Options{})
	if err != nil {
		return false, err
	}
	defer ra.Close()
	rb, err := vrlog.Open(b, vrlog.Options{})
	if err != nil {
		return false, err
	}
	defer rb.Close()
	ma, mb := ra.Manifest(), rb.Manifest()
	for _, field := range []struct{ name, a, b string }{
		{"profile", string(ma.Profile.Name), string(mb.Profile.Name)},
		{"source", ma.Extraction.SourceID, mb.Extraction.SourceID},
		{"calibration", ma.Extraction.CalibrationID, mb.Extraction.CalibrationID},
		{"extractor", ma.Extraction.ExtractorID, mb.Extraction.ExtractorID},
	} {
		if field.a != field.b {
			fmt.Fprintf(out, "note: %s differs: %s vs %s\n", field.name, field.a, field.b)
		}
	}
	if ma.Capture.UUID == mb.Capture.UUID {
		fmt.Fprintf(out, "note: both containers claim capture %s; independent runs should not\n", ma.Capture.UUID)
	}
	// Once every record has compared equal, one stream digest describes both.
	stream := l4bobserve.NewStreamDigest()
	for n := 0; ; n++ {
		recA, errA := ra.Next()
		recB, errB := rb.Next()
		endA, endB := errors.Is(errA, io.EOF), errors.Is(errB, io.EOF)
		switch {
		case errA != nil && !endA:
			return false, errA
		case errB != nil && !endB:
			return false, errB
		case endA && endB:
			fmt.Fprintf(out, "identical evidence: %d records, semantic %s\n", n, stream.Sum())
			return true, nil
		case endA || endB:
			fmt.Fprintf(out, "DIFFERENT: one container ends after %d records\n", n)
			return false, nil
		}
		var diff error
		switch {
		case recA.Kind != recB.Kind:
			diff = fmt.Errorf("a %s against a %s", recA.Kind, recB.Kind)
		case recA.Kind == vrlog.RecordFrame:
			diff = l4bobserve.DiffFrames(recA.Frame, recB.Frame)
			stream.AddFrame(l4bobserve.FrameDigest(recA.Frame))
		case recA.Kind == vrlog.RecordGap:
			diff = l4bobserve.DiffGaps(recA.Gap, recB.Gap)
			stream.AddGap(l4bobserve.GapDigest(recA.Gap))
		}
		if diff != nil {
			fmt.Fprintf(out, "DIFFERENT at record %d: %v\n", n, diff)
			return false, nil
		}
	}
}
