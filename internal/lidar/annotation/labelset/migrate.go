package labelset

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// MigrateConfig names a version 1 pack, the capture it was cut from, and
// where the label set and the upgraded version 2 pack go.
type MigrateConfig struct {
	PackDir     string
	PCAPPath    string
	UDPPort     int
	LabelSetDir string
	// ViewPackDir, when set, receives the pack upgraded to version 2: the same
	// points, samples and backgrounds plus their returns, linked to the label
	// set, with the pack-bound files carried across.
	ViewPackDir string
	Name        string
	Build       string
	// History converts every retained revision; without it the label set
	// starts from the head as a base.
	History  bool
	Progress func(format string, args ...any)
}

// OverlapCounts records returns that two objects claimed in one window, and
// how they were resolved: a reviewed claim keeps the return, and each proposed
// claim on it moves the return to that mask's uncertain set.
type OverlapCounts struct {
	ProposedVsReviewed int `json:"proposed_vs_reviewed"`
	ProposedVsProposed int `json:"proposed_vs_proposed"`
	MasksTouched       int `json:"masks_touched"`
}

// MigrateReport is what a migration did and measured.
type MigrateReport struct {
	PackDir            string        `json:"pack_dir"`
	PackDigest         string        `json:"pack_digest"`
	DatasetID          string        `json:"dataset_id"`
	PCAPName           string        `json:"pcap_name"`
	PCAPSHA256         string        `json:"pcap_sha256"`
	PCAPBytes          int64         `json:"pcap_bytes"`
	CapturePackets     int           `json:"capture_packets"`
	Samples            int           `json:"samples"`
	FramesDecoded      int           `json:"frames_decoded"`
	PackPoints         int           `json:"pack_points"`
	MatchedUnique      int           `json:"matched_unique"`
	Coincident         int           `json:"coincident"`
	Unmatched          int           `json:"unmatched"`
	PacketTable        int           `json:"packet_table"`
	MappingDigest      string        `json:"mapping_digest"`
	HeadRevision       int           `json:"head_revision"`
	RevisionsConverted int           `json:"revisions_converted"`
	HeadObjects        int           `json:"head_objects"`
	HeadMasks          int           `json:"head_masks"`
	HeadMembers        int           `json:"head_members"`
	HeadReviewedMasks  int           `json:"head_reviewed_masks"`
	HeadOverlaps       OverlapCounts `json:"head_overlaps"`
	RevisionsResolved  int           `json:"revisions_with_overlaps"`
	LabelSetID         string        `json:"label_set_id"`
	LabelSetDir        string        `json:"label_set_dir"`
	StateDigest        string        `json:"state_digest"`
	JournalBytes       int64         `json:"journal_bytes"`
	HeadBytes          int64         `json:"head_bytes"`
	V1HistoryBytes     int64         `json:"v1_history_bytes"`
	ViewPackDir        string        `json:"view_pack_dir,omitempty"`
	ViewPackDigest     string        `json:"view_pack_digest,omitempty"`
	ProjectionMatches  bool          `json:"projection_matches_v1_head"`
	Carried            []string      `json:"carried,omitempty"`
	PinsRewritten      int           `json:"pins_rewritten"`
	Notes              []string      `json:"notes,omitempty"`
	ElapsedSeconds     float64       `json:"elapsed_seconds"`
}

// sampleMap is one sample's mapping onto decoded returns.
type sampleMap struct {
	found   bool
	refs    []ReturnRef // per pack point
	packets []PacketKey // every packet of the frame, in order
	first   ReturnRef   // first and last return of the whole frame
	last    ReturnRef
	endNs   int64
}

// Migrate carries a version 1 pack's labels into a label set. It decodes the
// capture, proves that every pack point is exactly one decoded return of its
// frame (bit-identical coordinates, in order), and refuses otherwise: labels
// move only by an exact mapping, never by approximate coordinates.
func Migrate(cfg MigrateConfig) (*MigrateReport, error) {
	start := time.Now()
	logf := cfg.Progress
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if cfg.UDPPort == 0 {
		cfg.UDPPort = 2369
	}
	p, err := annotation.OpenPack(cfg.PackDir)
	if err != nil {
		return nil, err
	}
	if p.Manifest.SchemaVersion != annotation.PackSchemaVersionLegacy {
		return nil, fmt.Errorf("%s is a version %d pack; only version 1 packs need migrating", cfg.PackDir, p.Manifest.SchemaVersion)
	}
	rep := &MigrateReport{PackDir: cfg.PackDir, PackDigest: p.Manifest.PackDigest, DatasetID: p.Manifest.DatasetID,
		PCAPName: filepath.Base(cfg.PCAPPath), Samples: len(p.Samples), LabelSetDir: cfg.LabelSetDir, ViewPackDir: cfg.ViewPackDir}
	if p.Manifest.Source.PCAPBasename != "" && p.Manifest.Source.PCAPBasename != rep.PCAPName {
		rep.Notes = append(rep.Notes, fmt.Sprintf("pack names capture %s; migrating against %s", p.Manifest.Source.PCAPBasename, rep.PCAPName))
	}

	logf("decoding %s", cfg.PCAPPath)
	maps, decoded, scan, err := decodeAndMatch(p, cfg.PCAPPath, cfg.UDPPort, rep)
	if err != nil {
		return nil, err
	}
	rep.FramesDecoded, rep.PCAPSHA256, rep.PCAPBytes, rep.CapturePackets = decoded, scan.SHA256, scan.Bytes, scan.Packets
	if rep.Unmatched > 0 || rep.Coincident > 0 {
		return rep, fmt.Errorf("refusing to migrate: %d pack points have no exact return and %d coincide with another return", rep.Unmatched, rep.Coincident)
	}
	logf("all %d pack points map to exactly one return", rep.MatchedUnique)

	layout := Pandar40P
	table, index, err := packetTable(maps)
	if err != nil {
		return rep, err
	}
	rep.PacketTable = len(table)
	ret := func(r ReturnRef) uint64 { n, _ := layout.Return(index[r.Key], r.Block, r.Channel); return n }
	domain := DomainOf(p)
	domain.Note = fmt.Sprintf("foreground of version 1 pack %s (%s), migrated", p.Manifest.PackDigest, p.Manifest.Coverage)
	base := State{Packets: table, Domains: []Domain{domain}}
	offsets := make([][]uint64, len(p.Samples)) // sample -> point -> offset in window
	viewReturns := make([][]uint64, len(p.Samples))
	for i, smp := range p.Samples {
		m := maps[i]
		w := Window{WindowID: smp.SampleID, Start: ret(m.first), End: ret(m.last) + 1, CaptureStartNs: smp.TimestampNs, CaptureEndNs: m.endNs}
		id := smp.SampleID
		w.LegacySampleID = &id
		base.Windows = append(base.Windows, w)
		offsets[i] = make([]uint64, len(m.refs))
		viewReturns[i] = make([]uint64, len(m.refs))
		for j, r := range m.refs {
			n := ret(r)
			if n < w.Start || n >= w.End || (j > 0 && n <= viewReturns[i][j-1]) {
				return rep, fmt.Errorf("sample %d point %d: return %d does not fit its frame [%d,%d) in order", i, j, n, w.Start, w.End)
			}
			viewReturns[i][j] = n
			offsets[i][j] = n - w.Start
		}
		base.DomainReturns = append(base.DomainReturns, DomainReturns{DomainID: domain.DomainID, WindowID: w.WindowID, Returns: RangesOf(offsets[i])})
	}
	rep.MappingDigest = mappingDigest(p.Manifest.PackDigest, scan.SHA256, table, viewReturns)
	convert := func(sc *annotation.Sidecar) (State, OverlapCounts, error) {
		return convertSidecar(base, offsets, sc)
	}

	// The head, and the history before it.
	headBytes, head, err := readV1Sidecar(p, "annotations.json")
	if err != nil {
		return rep, fmt.Errorf("read head: %w", err)
	}
	rep.HeadRevision = head.Revision
	m := Manifest{Name: cfg.Name, Layout: layout, MigratedFrom: &Migration{
		PackDigest: p.Manifest.PackDigest, DatasetID: p.Manifest.DatasetID, PackDir: filepath.Base(cfg.PackDir),
		SidecarRevision: head.Revision, PCAPSHA256: scan.SHA256, PCAPName: rep.PCAPName, DecoderBuild: cfg.Build,
		MappingDigest: rep.MappingDigest,
		Note:          "pack points mapped to decoded returns by exact float32 coordinates in frame order; overlapping proposed claims moved to uncertain",
	}}
	hw, err := NewHistory(cfg.LabelSetDir, m)
	if err != nil {
		return rep, err
	}
	v1Digests := map[string]int{}    // v1 sidecar bytes digest -> revision
	stateDigests := map[int]string{} // revision -> label set state digest
	finished := false
	defer func() {
		if !finished {
			hw.Abort()
		}
	}()
	add := func(rev int, b []byte, sc *annotation.Sidecar, asBase bool) error {
		if sc.Revision != rev {
			return fmt.Errorf("v1 revision file %d holds revision %d", rev, sc.Revision)
		}
		v1Digests[sha256Hex(b)] = rev
		s, ov, err := convert(sc)
		if err != nil {
			return fmt.Errorf("revision %d: %w", rev, err)
		}
		if ov.MasksTouched > 0 {
			rep.RevisionsResolved++
		}
		if rev == head.Revision {
			rep.HeadOverlaps = ov
		}
		change := Change{Author: sc.Change.Author, Session: sc.Change.Session, Operation: sc.Change.Operation, CreatedUTC: sc.Change.CreatedUTC}
		if change.CreatedUTC == "" {
			change.CreatedUTC = sc.UpdatedUTC
		}
		if change.Author == "" {
			change.Author = "unknown (version 1)"
		}
		var digest string
		if asBase {
			digest, err = hw.Base(rev, s, change)
		} else {
			digest, err = hw.Add(s, change, sc.RestoredFrom)
		}
		stateDigests[rev] = digest
		rep.RevisionsConverted++
		return err
	}
	historyBytes := int64(0)
	if cfg.History && head.Revision > 1 {
		for rev := 1; rev < head.Revision; rev++ {
			name := fmt.Sprintf("annotation-revisions/%010d.json", rev)
			b, sc, err := readV1Sidecar(p, name)
			if err != nil {
				return rep, fmt.Errorf("revision %d: %w (rerun without history to start from the head)", rev, err)
			}
			historyBytes += int64(len(b))
			if err := add(rev, b, sc, false); err != nil {
				return rep, err
			}
			if rev%200 == 0 {
				logf("converted revision %d of %d", rev, head.Revision)
			}
		}
		if err := add(head.Revision, headBytes, head, false); err != nil {
			return rep, err
		}
	} else if err := add(head.Revision, headBytes, head, head.Revision > 1); err != nil {
		return rep, err
	}
	rep.V1HistoryBytes = historyBytes
	ls, err := hw.Finish()
	finished = true
	if err != nil {
		return rep, err
	}
	rep.LabelSetID, rep.StateDigest = ls.Manifest.LabelSetID, ls.StateDigest()
	rep.HeadObjects, rep.HeadMasks = len(ls.State.Records.Objects), len(ls.State.Masks)
	rep.HeadReviewedMasks = len(ls.State.ReviewedMasks())
	for _, mk := range ls.State.Masks {
		rep.HeadMembers += int(mk.Members.Count())
	}
	if err := recordBinding(cfg.LabelSetDir, Binding{Name: rep.PCAPName, SHA256: scan.SHA256, Bytes: scan.Bytes, UDPPort: cfg.UDPPort,
		PacketsFound: len(table), WindowsComplete: len(base.Windows), VerifiedUTC: time.Now().UTC().Format(time.RFC3339), VerifiedBuild: cfg.Build}); err != nil {
		return rep, err
	}
	logf("verifying the journal")
	if _, err := Verify(cfg.LabelSetDir); err != nil {
		return rep, fmt.Errorf("verify: %w", err)
	}
	rep.JournalBytes = fileSize(filepath.Join(cfg.LabelSetDir, JournalFile))
	for _, name := range stateFiles {
		rep.HeadBytes += fileSize(filepath.Join(cfg.LabelSetDir, name))
	}

	if cfg.ViewPackDir != "" {
		logf("writing version 2 pack %s", cfg.ViewPackDir)
		if err := writeViewPack(cfg, p, table, viewReturns, ls, v1Digests, stateDigests, rep); err != nil {
			return rep, err
		}
	}
	rep.ElapsedSeconds = time.Since(start).Seconds()
	return rep, nil
}

func fileSize(path string) int64 {
	if fi, err := os.Stat(path); err == nil {
		return fi.Size()
	}
	return 0
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// readV1Sidecar reads a version 1 sidecar without validating it: older
// revisions can break rules added later (two objects claiming a point), and
// the migration resolves those explicitly.
func readV1Sidecar(p *annotation.Pack, name string) ([]byte, *annotation.Sidecar, error) {
	b, err := os.ReadFile(filepath.Join(p.Dir, name))
	if err != nil {
		return nil, nil, err
	}
	var sc annotation.Sidecar
	if err := json.Unmarshal(b, &sc); err != nil {
		return nil, nil, err
	}
	if sc.PackDigest != p.Manifest.PackDigest {
		return nil, nil, fmt.Errorf("%s was written against pack %s, not %s", name, sc.PackDigest, p.Manifest.PackDigest)
	}
	return b, &sc, nil
}

type triple [3]uint32

func tripleOf(x, y, z float32) triple {
	return triple{math.Float32bits(x), math.Float32bits(y), math.Float32bits(z)}
}

// decodeAndMatch replays the capture through the L1 parser and the offline L2
// frame builder, as an offline replay frames it, and matches each sample to
// the frame with its start time as an order-preserving subsequence of
// bit-identical coordinates.
func decodeAndMatch(p *annotation.Pack, path string, port int, rep *MigrateReport) ([]sampleMap, int, ScanResult, error) {
	type packPoints struct {
		idx int
		t   []triple
	}
	bySample := make(map[int64]packPoints, len(p.Samples))
	for i, s := range p.Samples {
		if _, dup := bySample[s.TimestampNs]; dup {
			return nil, 0, ScanResult{}, fmt.Errorf("samples %d and another share start time %d", i, s.TimestampNs)
		}
		pts, err := p.PointsAt(i)
		if err != nil {
			return nil, 0, ScanResult{}, err
		}
		t := make([]triple, len(pts.X))
		for j := range t {
			t[j] = tripleOf(pts.X[j], pts.Y[j], pts.Z[j])
		}
		bySample[s.TimestampNs] = packPoints{i, t}
		rep.PackPoints += len(t)
	}
	maps := make([]sampleMap, len(p.Samples))
	var mu sync.Mutex
	decoded := 0
	var matchErr error
	cfg, err := parse.LoadEmbeddedPandar40PConfig()
	if err != nil {
		return nil, 0, ScanResult{}, err
	}
	parser := parse.NewPandar40PParser(*cfg)
	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
		SensorID: p.Manifest.Source.SensorID, FrameChCapacity: 32,
		BufferTimeout: time.Duration(math.MaxInt64), CleanupInterval: time.Duration(math.MaxInt64),
		FrameCallback: func(f *l2frames.LiDARFrame) {
			mu.Lock()
			defer mu.Unlock()
			decoded++
			pp, ok := bySample[f.StartTimestamp.UnixNano()]
			if !ok || len(f.Points) == 0 {
				return
			}
			m := sampleMap{found: true, refs: make([]ReturnRef, len(pp.t)), endNs: f.EndTimestamp.UnixNano()}
			count := make(map[triple]int, len(f.Points))
			for _, pt := range f.Points {
				count[tripleOf(float32(pt.X), float32(pt.Y), float32(pt.Z))]++
				if pt.PacketKey == 0 {
					matchErr = fmt.Errorf("decoded frame at %d carries no packet keys", f.StartTimestamp.UnixNano())
					return
				}
				if n := len(m.packets); n == 0 || m.packets[n-1] != PacketKey(pt.PacketKey) {
					m.packets = append(m.packets, PacketKey(pt.PacketKey))
				}
			}
			first, last := f.Points[0], f.Points[len(f.Points)-1]
			m.first = ReturnRef{PacketKey(first.PacketKey), first.BlockID, first.Channel}
			m.last = ReturnRef{PacketKey(last.PacketKey), last.BlockID, last.Channel}
			j := 0
			for i, t := range pp.t {
				for j < len(f.Points) && tripleOf(float32(f.Points[j].X), float32(f.Points[j].Y), float32(f.Points[j].Z)) != t {
					j++
				}
				if j == len(f.Points) {
					rep.Unmatched += len(pp.t) - i
					break
				}
				if count[t] > 1 {
					rep.Coincident++
				} else {
					rep.MatchedUnique++
				}
				pt := f.Points[j]
				m.refs[i] = ReturnRef{PacketKey(pt.PacketKey), pt.BlockID, pt.Channel}
				j++
			}
			maps[pp.idx] = m
		},
	})
	scan, err := ScanCapture(path, port, func(pk Packet) error {
		parser.SetPacketTime(time.Unix(0, pk.CaptureNs))
		points, err := parser.ParsePacket(pk.Payload)
		if err != nil {
			return nil // the replay skips packets it cannot parse
		}
		fb.AddPointsPolar(points)
		if ms := parser.GetLastMotorSpeed(); ms > 0 {
			fb.SetMotorSpeed(ms)
		}
		return nil
	})
	fb.Close()
	if err != nil {
		return nil, 0, ScanResult{}, err
	}
	if matchErr != nil {
		return nil, 0, ScanResult{}, matchErr
	}
	var missing []int
	for i, m := range maps {
		if !m.found {
			missing = append(missing, i)
		}
	}
	if len(missing) > 0 {
		return nil, 0, ScanResult{}, fmt.Errorf("%d samples match no decoded frame by start time (first: %d)", len(missing), missing[0])
	}
	return maps, decoded, scan, nil
}

// packetTable lists every packet of the samples' frames in capture order.
func packetTable(maps []sampleMap) ([]PacketKey, map[PacketKey]int, error) {
	var table []PacketKey
	index := map[PacketKey]int{}
	for i, m := range maps {
		for _, k := range m.packets {
			if at, ok := index[k]; ok {
				if at == len(table)-1 {
					continue // a packet split across consecutive frames
				}
				return nil, nil, fmt.Errorf("sample %d repeats packet %s from earlier in the capture", i, k)
			}
			index[k] = len(table)
			table = append(table, k)
		}
	}
	return table, index, nil
}

func mappingDigest(packDigest, pcapSHA string, table []PacketKey, returns [][]uint64) string {
	h := sha256.New()
	fmt.Fprintf(h, "velocity.report label set migration v1\n%s\n%s\n", packDigest, pcapSHA)
	var b [8]byte
	for _, k := range table {
		binary.LittleEndian.PutUint64(b[:], uint64(k))
		h.Write(b[:])
	}
	for _, rs := range returns {
		for _, r := range rs {
			binary.LittleEndian.PutUint64(b[:], r)
			h.Write(b[:])
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// convertSidecar turns one version 1 sidecar into a state over the migrated
// windows, resolving returns two objects claim in one window: a reviewed
// claim keeps the return, every proposed claim on it moves the return to that
// mask's uncertain set. Two reviewed claims are refused.
func convertSidecar(base State, offsets [][]uint64, sc *annotation.Sidecar) (State, OverlapCounts, error) {
	s := base.Clone()
	s.Records = Records{
		Objects:            append([]annotation.Object(nil), sc.Objects...),
		Correspondences:    append([]annotation.TrackCorrespondence(nil), sc.Correspondences...),
		DismissedProposals: append([]string(nil), sc.DismissedProposals...),
	}
	status := map[string]annotation.ReviewStatus{}
	for _, o := range sc.Objects {
		status[o.ObjectID] = o.Status
	}
	domain := base.Domains[0].DomainID
	toRanges := func(sample int, idx []int) (Ranges, error) {
		out := make([]uint64, 0, len(idx))
		for _, j := range idx {
			if j < 0 || j >= len(offsets[sample]) {
				return nil, fmt.Errorf("sample %d point %d is outside the pack", sample, j)
			}
			out = append(out, offsets[sample][j])
		}
		return RangesOf(out), nil
	}
	for _, fm := range sc.Masks {
		if fm.SampleID < 0 || fm.SampleID >= len(offsets) {
			return s, OverlapCounts{}, fmt.Errorf("mask %s cites sample %d", fm.ObjectID, fm.SampleID)
		}
		members, err := toRanges(fm.SampleID, fm.PointIndices)
		if err != nil {
			return s, OverlapCounts{}, err
		}
		uncertain, err := toRanges(fm.SampleID, fm.UncertainIndices)
		if err != nil {
			return s, OverlapCounts{}, err
		}
		s.Masks = append(s.Masks, Mask{ObjectID: fm.ObjectID, WindowID: base.Windows[fm.SampleID].WindowID, DomainID: domain,
			Members: members, Uncertain: Difference(uncertain, members), Completeness: fm.Completeness, Visibility: fm.Visibility,
			Status: fm.Status, Pose: fm.Pose, Provenance: fm.Provenance})
	}
	var counts OverlapCounts
	byWindow := map[int][]int{}
	for i, m := range s.Masks {
		byWindow[m.WindowID] = append(byWindow[m.WindowID], i)
	}
	touched := map[int]bool{}
	for _, idx := range byWindow {
		claims := map[uint64][]int{}
		for _, i := range idx {
			for _, r := range s.Masks[i].Members.Members() {
				claims[r] = append(claims[r], i)
			}
		}
		move := map[int][]uint64{}
		for r, cs := range claims {
			if len(cs) < 2 {
				continue
			}
			var reviewed []int
			for _, i := range cs {
				m := s.Masks[i]
				if m.Status == annotation.StatusReviewed && status[m.ObjectID] == annotation.StatusReviewed {
					reviewed = append(reviewed, i)
				}
			}
			if len(reviewed) > 1 {
				return s, counts, fmt.Errorf("window %d return %d is claimed by two reviewed objects", s.Masks[cs[0]].WindowID, r)
			}
			if len(reviewed) == 1 {
				counts.ProposedVsReviewed++
			} else {
				counts.ProposedVsProposed++
			}
			for _, i := range cs {
				if len(reviewed) == 1 && i == reviewed[0] {
					continue
				}
				move[i] = append(move[i], r)
				touched[i] = true
			}
		}
		for i, rs := range move {
			moved := RangesOf(rs)
			s.Masks[i].Members = Difference(s.Masks[i].Members, moved)
			s.Masks[i].Uncertain = Union(s.Masks[i].Uncertain, moved)
		}
	}
	counts.MasksTouched = len(touched)
	s.Canonicalise()
	return s, counts, nil
}

// writeViewPack writes the pack again as version 2 with its returns, links it
// to the label set, and carries the pack-bound records across with their pins
// moved to the new pack: pack digest, dataset id, and each pinned membership
// revision's digest, which becomes the label set's state digest at that
// revision, the token a linked pack's membership is pinned by.
func writeViewPack(cfg MigrateConfig, p *annotation.Pack, table []PacketKey, returns [][]uint64,
	ls *LabelSet, v1Digests map[string]int, stateDigests map[int]string, rep *MigrateReport) error {
	samples := append([]annotation.Sample(nil), p.Samples...)
	blocks := make([][]byte, len(samples))
	for i := range samples {
		pts, err := p.PointsAt(i)
		if err != nil {
			return err
		}
		if blocks[i], err = annotation.EncodePoints(pts); err != nil {
			return err
		}
		w := ls.State.Windows[i]
		samples[i].ReturnStart, samples[i].ReturnEnd = w.Start, w.End
	}
	m := p.Manifest
	m.DatasetID = ""
	m.CoverageNote = strings.TrimSpace(m.CoverageNote + "; version 2 copy of pack " + p.Manifest.PackDigest + " with returns from the label set migration")
	keys := make([]uint64, len(table))
	for i, k := range table {
		keys[i] = uint64(k)
	}
	var bgBlocks [][]byte
	if len(p.Backgrounds) > 0 {
		raw, err := os.ReadFile(filepath.Join(p.Dir, "background.bin"))
		if err != nil {
			return err
		}
		if sha256Hex(raw) != p.Manifest.BackgroundPointsSHA256 {
			return fmt.Errorf("background.bin changed since the pack was opened")
		}
		for _, b := range p.Backgrounds {
			end := b.ByteOffset + int64(b.PointCount)*12
			bgBlocks = append(bgBlocks, raw[b.ByteOffset:end])
		}
	}
	pr := annotation.PackReturns{
		Layout:  annotation.ReturnLayout{Model: Pandar40P.Model, Blocks: Pandar40P.Blocks, Channels: Pandar40P.Channels},
		Packets: keys, Returns: returns,
	}
	if err := annotation.WritePackWithBackground(cfg.ViewPackDir, m, samples, blocks, pr, append([]annotation.Background(nil), p.Backgrounds...), bgBlocks); err != nil {
		return err
	}
	view, err := annotation.OpenPack(cfg.ViewPackDir)
	if err != nil {
		return err
	}
	rep.ViewPackDigest = view.Manifest.PackDigest
	if err := annotation.WriteLabelSetLink(view, ls.Manifest.LabelSetID, cfg.LabelSetDir); err != nil {
		return err
	}

	// The projection of the head onto the new pack must hold exactly the
	// version 1 head's masks, apart from overlaps moved to uncertain.
	_, v1Head, err := readV1Sidecar(p, "annotations.json")
	if err != nil {
		return err
	}
	projected, err := annotation.LoadSidecar(view)
	if err != nil {
		return fmt.Errorf("project the head onto the version 2 pack: %w", err)
	}
	rep.ProjectionMatches = sameMembership(v1Head, projected)
	if !rep.ProjectionMatches && rep.HeadOverlaps.MasksTouched == 0 {
		return fmt.Errorf("the head projected onto the version 2 pack differs from the version 1 head")
	}

	// Carry pack-bound records, moving their pins.
	replace := map[string]string{p.Manifest.PackDigest: view.Manifest.PackDigest, p.Manifest.DatasetID: view.Manifest.DatasetID}
	var carried []string
	for _, name := range []string{"physical-references.json", "feature-proposals.pb", "segment.json"} {
		if _, err := os.Stat(filepath.Join(p.Dir, name)); err == nil {
			carried = append(carried, name)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(p.Dir, "proposals")); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") && !strings.HasPrefix(e.Name(), ".") {
				carried = append(carried, filepath.Join("proposals", e.Name()))
			}
		}
	}
	digestRe := regexp.MustCompile(`sha256:[0-9a-f]{64}`)
	contents := map[string][]byte{}
	for _, name := range carried {
		b, err := os.ReadFile(filepath.Join(p.Dir, name))
		if err != nil {
			return err
		}
		contents[name] = b
		for _, d := range digestRe.FindAll(b, -1) {
			if rev, ok := v1Digests[string(d)]; ok {
				state, ok := stateDigests[rev]
				if !ok {
					return fmt.Errorf("%s pins membership revision %d, which was not migrated", name, rev)
				}
				replace[string(d)] = state
			}
		}
	}
	for _, name := range carried {
		b := contents[name]
		for old, new := range replace {
			if len(old) != len(new) {
				return fmt.Errorf("cannot rewrite %s: replacement for %s changes length", name, old)
			}
			n := bytes.Count(b, []byte(old))
			if n > 0 && old != new {
				b = bytes.ReplaceAll(b, []byte(old), []byte(new))
				rep.PinsRewritten += n
			}
		}
		dst := filepath.Join(cfg.ViewPackDir, name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	rep.Carried = carried
	if len(carried) > 0 {
		if _, err := annotation.LoadPhysicalReferences(view); err != nil && !errors.Is(err, os.ErrNotExist) {
			rep.Notes = append(rep.Notes, "physical references carried but do not load against the projection: "+err.Error())
		}
		if _, err := annotation.LoadFeatures(view); err != nil {
			rep.Notes = append(rep.Notes, "feature proposals carried but do not load against the projection: "+err.Error())
		}
	}
	return nil
}

func sameMembership(a, b *annotation.Sidecar) bool {
	type key struct {
		o string
		s int
	}
	index := func(sc *annotation.Sidecar) map[key]annotation.FrameMask {
		out := map[key]annotation.FrameMask{}
		for _, m := range sc.Masks {
			out[key{m.ObjectID, m.SampleID}] = m
		}
		return out
	}
	ia, ib := index(a), index(b)
	if len(ia) != len(ib) || len(a.Objects) != len(b.Objects) {
		return false
	}
	for k, m := range ia {
		n, ok := ib[k]
		if !ok || m.Status != n.Status || m.Completeness != n.Completeness || m.Visibility != n.Visibility ||
			!equalInts(annotation.CanonicalIndices(m.PointIndices), annotation.CanonicalIndices(n.PointIndices)) ||
			!equalInts(annotation.CanonicalIndices(m.UncertainIndices), annotation.CanonicalIndices(n.UncertainIndices)) {
			return false
		}
	}
	return true
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
