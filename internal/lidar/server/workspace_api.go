package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/labelwork"
)

// The labelling workspace: what captures, segments, clips, runs, packs, label
// sets and splits exist, how far each layer of labelling and review has got,
// what is outstanding and why, and what each item may be used for.
//
//	GET  /api/lidar/workspace                  overview and a filtered page of rows
//	GET  /api/lidar/workspace/item?key=        one row's detail
//	PUT  /api/lidar/workspace/workflow         save one workflow item (revision-checked)
//	POST /api/lidar/annotations/objects/review review, reject or reopen one object
//
// A row is a label set, counted once however many runs' packs view it; a
// pack with no label set yet and a selected segment with no pack yet are rows
// of their own. Progress is read from the label sets and physical-reference
// documents on every request (cached against their files), so it is what the
// stores hold. Workflow state is kept in the database beside it and never
// changes a review status.

const (
	maxWorkspaceSegments = 2000
	maxWorkspaceSplits   = 2000
	defaultWorkspacePage = 50
	maxWorkspacePage     = 200
	maxJournalEntries    = 50
)

// sourceState says whether one inventory source could be read. A source
// that could not be read makes its rows unavailable, never empty.
type sourceState struct {
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}

// workspaceCapture is one capture a row's data was cut from.
type workspaceCapture struct {
	Basename string `json:"basename"`
	SHA256   string `json:"sha256,omitempty"`
	// Verified is true when a label set bound the file by hash.
	Verified bool `json:"verified"`
}

type workspaceSegment struct {
	SegmentID       string `json:"segment_id"`
	RunID           string `json:"run_id,omitempty"`
	ClipID          string `json:"clip_id"`
	Role            string `json:"role"`
	Finder          string `json:"finder"`
	Selector        string `json:"selector,omitempty"`
	HeldOutEligible bool   `json:"held_out_eligible"`
	Capture         string `json:"capture"`
	WindowStartNs   int64  `json:"window_start_ns,string"`
	WindowEndNs     int64  `json:"window_end_ns,string"`
	PackDir         string `json:"pack_dir,omitempty"`
	PackDigest      string `json:"pack_digest,omitempty"`
	CreatedAtNs     int64  `json:"created_at_ns,string"`
	// PackMissing marks a pack the database names that is not on disk.
	PackMissing bool `json:"pack_missing,omitempty"`
}

type workspacePack struct {
	Handle     string                      `json:"handle"`
	PackDigest string                      `json:"pack_digest"`
	DatasetID  string                      `json:"dataset_id"`
	Run        string                      `json:"run"`
	SegmentID  string                      `json:"segment_id,omitempty"`
	Role       string                      `json:"selection_role,omitempty"`
	PCAP       string                      `json:"pcap_basename,omitempty"`
	Samples    int                         `json:"samples"`
	BuildSHA   string                      `json:"build_git_sha,omitempty"`
	Physical   *labelwork.PhysicalProgress `json:"physical,omitempty"`
	Error      string                      `json:"error,omitempty"`
}

type workspaceLabelSet struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Revision    int    `json:"revision"`
	StateDigest string `json:"state_digest"`
	Dir         string `json:"dir"`
}

type workspaceEligibility struct {
	Purpose      eligibility.Purpose `json:"purpose"`
	HeldOutScore bool                `json:"held_out_score"`
	SplitDigest  string              `json:"split_digest,omitempty"`
	Allowed      bool                `json:"allowed"`
	Reasons      []string            `json:"reasons"`
}

// workspaceRow is one inventory row.
type workspaceRow struct {
	Key       string                      `json:"key"`
	Kind      string                      `json:"kind"` // label_set, pack, segment
	Title     string                      `json:"title"`
	Stage     string                      `json:"stage"`
	Flags     []string                    `json:"flags"`
	Protected bool                        `json:"protected"`
	Cohort    string                      `json:"cohort"`
	SiteID    string                      `json:"site_id,omitempty"`
	Roles     []string                    `json:"roles"`
	Captures  []workspaceCapture          `json:"captures"`
	Runs      []string                    `json:"runs"`
	Segments  []workspaceSegment          `json:"segments"`
	Packs     []workspacePack             `json:"packs"`
	LabelSet  *workspaceLabelSet          `json:"label_set,omitempty"`
	Splits    []labelwork.FrozenRef       `json:"splits"`
	Progress  *labelwork.LabelSetProgress `json:"progress,omitempty"`
	Eval      *workspaceEligibility       `json:"evaluation,omitempty"`
	Workflow  []labelwork.Item            `json:"workflow"`
	Tasks     []labelwork.Task            `json:"tasks"`
	Error     string                      `json:"error,omitempty"`

	labelSetDir string
	spans       []labelwork.ObjectSpan
}

type workspaceTotals struct {
	Rows      int `json:"rows"`
	LabelSets int `json:"label_sets"`
	Packs     int `json:"packs"`
	Captures  int `json:"captures"`
	Segments  int `json:"segments"`
	Runs      int `json:"runs"`
	Splits    int `json:"splits"`
	// Layers sum the label sets' own counts, each label set once. A total
	// is null when any label set's is unknown or any label set could not be
	// read: a sum that leaves a set out is not a total.
	Layers      []labelwork.LayerProgress `json:"layers"`
	Unreadable  int                       `json:"unreadable"`
	Outstanding int                       `json:"outstanding_tasks"`
	Blocked     int                       `json:"blocked_tasks"`
	TBD         int                       `json:"tbd_tasks"`
	Stages      map[string]int            `json:"stages"`
}

type queueEntry struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	labelwork.Task
}

type workspaceResponse struct {
	GeneratedAtNs int64                  `json:"generated_at_ns,string"`
	Sources       map[string]sourceState `json:"sources"`
	Eligibility   eligibility.Status     `json:"eligibility"`
	Totals        workspaceTotals        `json:"totals"`
	Queue         []queueEntry           `json:"queue"`
	Items         []workspaceRow         `json:"items"`
	Total         int                    `json:"total"`
	Limit         int                    `json:"limit"`
	Offset        int                    `json:"offset"`
	Plans         labelwork.Plans        `json:"plans"`
	Facets        map[string][]string    `json:"facets"`
}

type workspaceSplit struct {
	name  string
	split *annotation.FrozenSplit
}

func (ws *Server) workflowStore() *labelwork.Store {
	if ws.db == nil {
		return nil
	}
	return &labelwork.Store{DB: ws.db.DB}
}

// buildWorkspace assembles every row. Each source is read once, bounded.
func (ws *Server) buildWorkspace(now time.Time) ([]workspaceRow, map[string]sourceState, int) {
	sources := map[string]sourceState{}
	var listings []packListing
	if ws.annotationPacksDir == "" {
		sources["annotation_root"] = sourceState{Error: "annotation packs directory is not configured: start the server with --lidar-annotation-dir"}
	} else if dirs, err := annotation.ListPackDirs(ws.annotationPacksDir); err != nil {
		sources["annotation_root"] = sourceState{Error: err.Error()}
	} else {
		sources["annotation_root"] = sourceState{Available: true}
		listings = ws.packListings.list(dirs, now)
	}
	segs, segErr := ws.workspaceSegments()
	if segErr != nil {
		sources["database"] = sourceState{Error: segErr.Error()}
	} else {
		sources["database"] = sourceState{Available: true}
	}
	splits, splitErr := ws.workspaceSplits()
	if splitErr != nil {
		sources["splits"] = sourceState{Error: splitErr.Error()}
	} else {
		sources["splits"] = sourceState{Available: true}
	}

	rows := map[string]*workspaceRow{}
	var order []string
	packByDigest := map[string]*workspaceRow{}
	rowFor := func(key, kind, title string) *workspaceRow {
		if r, ok := rows[key]; ok {
			return r
		}
		r := &workspaceRow{Key: key, Kind: kind, Title: title, Flags: []string{}, Roles: []string{}, Captures: []workspaceCapture{},
			Runs: []string{}, Segments: []workspaceSegment{}, Packs: []workspacePack{}, Splits: []labelwork.FrozenRef{},
			Workflow: []labelwork.Item{}, Tasks: []labelwork.Task{}}
		rows[key] = r
		order = append(order, key)
		return r
	}
	for _, l := range listings {
		handle, _ := filepath.Rel(ws.annotationPacksDir, l.PackDir)
		handle = filepath.ToSlash(handle)
		var r *workspaceRow
		link, _ := annotation.ReadLabelSetLink(l.PackDir)
		if l.LabelSetID != "" && l.LabelSetDir != "" {
			r = rowFor("label-set:"+l.LabelSetID, "label_set", firstNonEmpty(l.LabelSetName, l.LabelSetID))
			r.labelSetDir = l.LabelSetDir
		} else if link != nil && link.LabelSetID != "" {
			// Linked to a label set that could not be read: the label set's
			// row, unavailable, so no total leaves it out silently.
			r = rowFor("label-set:"+link.LabelSetID, "label_set", link.LabelSetID)
		} else if l.PackDigest != "" {
			r = rowFor("pack:"+l.PackDigest, "pack", handle)
		} else {
			r = rowFor("pack:"+handle, "pack", handle)
		}
		pk := workspacePack{Handle: handle, PackDigest: l.PackDigest, DatasetID: l.DatasetID, Run: packRun(handle),
			SegmentID: l.SegmentID, Role: l.Role, PCAP: l.Source.PCAPBasename, Samples: l.SampleCount, BuildSHA: l.Source.BuildGitSHA, Error: l.Error}
		if l.Error != "" && r.Error == "" {
			r.Error = l.Error
		}
		r.Packs = append(r.Packs, pk)
		r.Runs = appendUnique(r.Runs, pk.Run)
		if l.Source.PCAPBasename != "" {
			r.Captures = addCapture(r.Captures, workspaceCapture{Basename: l.Source.PCAPBasename})
		}
		for _, b := range l.LabelSetCaptures {
			r.Captures = addCapture(r.Captures, workspaceCapture{Basename: b.Name, SHA256: b.SHA256, Verified: true})
		}
		if l.Role != "" {
			r.Roles = appendUnique(r.Roles, "selection:"+l.Role)
		}
		if l.PackDigest != "" {
			packByDigest[l.PackDigest] = r
		}
	}
	for _, sg := range segs {
		var r *workspaceRow
		if sg.PackDigest != "" {
			r = packByDigest[sg.PackDigest]
			if r == nil {
				sg.PackMissing = true
			}
		}
		if r == nil {
			r = rowFor("segment:"+sg.SegmentID, "segment", sg.Capture+" "+sg.SegmentID)
		}
		r.Segments = append(r.Segments, sg)
		if sg.RunID != "" {
			r.Runs = appendUnique(r.Runs, sg.RunID)
		}
		r.Roles = appendUnique(r.Roles, "selection:"+sg.Role)
		if sg.Capture != "" {
			r.Captures = addCapture(r.Captures, workspaceCapture{Basename: filepath.Base(sg.Capture)})
		}
		if sg.PackMissing && !contains(r.Flags, "missing_files") {
			r.Flags = append(r.Flags, "missing_files")
		}
	}

	// Label sets, once each; then physical references per pack.
	for _, key := range order {
		r := rows[key]
		if r.labelSetDir == "" {
			continue
		}
		sum := ws.workspace.labelSet(r.labelSetDir, now)
		rel, _ := filepath.Rel(ws.annotationPacksDir, r.labelSetDir)
		r.LabelSet = &workspaceLabelSet{ID: sum.ID, Name: sum.Name, Revision: sum.Revision, StateDigest: sum.StateDigest, Dir: filepath.ToSlash(rel)}
		if sum.Error != "" {
			r.Error = "label set: " + sum.Error
			continue
		}
		progress := sum.Progress
		r.Progress, r.spans = &progress, progress.Spans
		for i := range r.Packs {
			pk := &r.Packs[i]
			if pk.Error != "" {
				continue
			}
			ph := ws.workspace.physicalOf(filepath.Join(ws.annotationPacksDir, filepath.FromSlash(pk.Handle)), r.labelSetDir, progress.ReviewedObjects, now)
			if ph.Error != "" {
				pk.Error = "physical references: " + ph.Error
				continue
			}
			p := ph.Progress
			pk.Physical = &p
		}
	}

	// Frozen splits, by the packs they pin.
	for _, sp := range splits {
		roles := map[string]annotation.SplitRole{}
		for _, part := range sp.split.Partitions {
			roles[part.Name] = part.Role
		}
		for _, fp := range sp.split.Packs {
			r := packByDigest[fp.PackDigest]
			if r == nil {
				continue
			}
			ref := labelwork.FrozenRef{Name: sp.name, SplitDigest: sp.split.SplitDigest, Revision: sp.split.Revision,
				PackDigest: fp.PackDigest, PinnedRevision: fp.SidecarRevision, Roles: []string{}}
			for _, o := range fp.Objects {
				ref.Roles = appendUnique(ref.Roles, string(roles[o.Partition]))
			}
			if r.LabelSet != nil {
				ref.HeadRevision = r.LabelSet.Revision
				ref.HeadMoved = r.LabelSet.Revision != fp.SidecarRevision
			}
			r.Splits = append(r.Splits, ref)
			for _, role := range ref.Roles {
				r.Roles = appendUnique(r.Roles, "split:"+role)
			}
		}
	}

	// Workflow items, eligibility, tasks, stage and flags.
	var workflow map[string][]labelwork.Item
	if st := ws.workflowStore(); st != nil {
		items, err := st.Items()
		if err != nil {
			sources["workflow"] = sourceState{Error: err.Error()}
		} else {
			sources["workflow"] = sourceState{Available: true}
			workflow = map[string][]labelwork.Item{}
			for _, it := range items {
				workflow[it.ItemKey] = append(workflow[it.ItemKey], it)
			}
		}
	} else {
		sources["workflow"] = sourceState{Error: "no database: workflow notes, assignments and blockers cannot be kept"}
	}
	svc := ws.eligibilityService()
	out := make([]workspaceRow, 0, len(order))
	for _, key := range order {
		r := rows[key]
		if w := workflow[key]; w != nil {
			r.Workflow = w
		}
		ws.decideRow(svc, r)
		facts := labelwork.RowFacts{SourceRevision: rowRevision(r), Progress: r.Progress, HasLabelSet: r.LabelSet != nil,
			Cohort: r.Cohort, Protected: r.Protected, Frozen: r.Splits}
		if r.LabelSet != nil {
			facts.LabelSetDir = r.LabelSet.Dir
		}
		if r.Kind == "label_set" && r.Progress == nil {
			facts.HasLabelSet = true
			facts.ProgressError = firstNonEmpty(strings.TrimPrefix(r.Error, "label set: "), "the label set could not be read")
		}
		for _, c := range r.Captures {
			facts.Bound = facts.Bound || c.Verified
		}
		for _, pk := range r.Packs {
			facts.Packs = append(facts.Packs, labelwork.PackFacts{Handle: pk.Handle, Digest: pk.PackDigest, Physical: pk.Physical, Error: pk.Error})
			if pk.Role != "" {
				facts.SelectionRole = pk.Role
			}
		}
		if r.Eval != nil && !r.Eval.Allowed {
			facts.EvaluationBlocked = strings.Join(r.Eval.Reasons, "; ")
		}
		if r.Kind == "segment" {
			r.Tasks = segmentTasks(r)
		} else {
			r.Tasks = labelwork.Tasks(facts)
		}
		stageAndFlags(r)
		if r.Protected {
			redactRow(r)
		}
		out = append(out, *r)
	}
	sort.SliceStable(out, func(i, j int) bool { return rowRank(out[i]) < rowRank(out[j]) })
	return out, sources, len(splits)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// packRun is the run a pack handle names: <capture>/packs/<run>,
// <capture>/<set>/views/<view> or clip-<job>/pack.
func packRun(handle string) string {
	parts := strings.Split(handle, "/")
	if len(parts) >= 3 && (parts[len(parts)-2] == annotation.PacksDir || parts[len(parts)-2] == "views") {
		return parts[len(parts)-1]
	}
	return parts[0]
}

func appendUnique(list []string, v string) []string {
	if v == "" || contains(list, v) {
		return list
	}
	return append(list, v)
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func addCapture(list []workspaceCapture, c workspaceCapture) []workspaceCapture {
	for i, have := range list {
		if have.Basename == c.Basename && (have.SHA256 == "" || c.SHA256 == "" || have.SHA256 == c.SHA256) {
			if c.SHA256 != "" {
				list[i].SHA256, list[i].Verified = c.SHA256, list[i].Verified || c.Verified
			}
			return list
		}
	}
	return append(list, c)
}

func rowRevision(r *workspaceRow) string {
	if r.LabelSet != nil && r.LabelSet.StateDigest != "" {
		return fmt.Sprintf("label-set:%d:%s", r.LabelSet.Revision, r.LabelSet.StateDigest)
	}
	if len(r.Packs) > 0 && r.Packs[0].PackDigest != "" {
		return "pack:" + r.Packs[0].PackDigest
	}
	return ""
}

// decideRow asks the eligibility service, without recording, what an
// evaluation of the row's captures would be decided: a held-out score under
// the split that holds it out, else a development evaluation. The captures
// are named by the hashes label sets verified; a capture known only by name
// has no identity and is refused.
func (ws *Server) decideRow(svc *eligibility.Service, r *workspaceRow) {
	var inputs []eligibility.Input
	for _, c := range r.Captures {
		id := eligibility.Identity{Kind: eligibility.KindCapture, Basename: c.Basename, SHA256: c.SHA256}
		in := eligibility.Input{Identity: id}
		if c.SHA256 == "" {
			in.Err = errors.New("not bound by hash: bind its label set to the capture file")
		}
		inputs = append(inputs, in)
	}
	r.Cohort = string(eligibility.CohortUnassigned)
	if len(inputs) == 0 {
		r.Eval = &workspaceEligibility{Purpose: eligibility.PurposeDevelopmentEvaluation, Reasons: []string{"no capture is known for this item"}}
		return
	}
	req := eligibility.Request{Purpose: eligibility.PurposeDevelopmentEvaluation, Inputs: inputs, EntryPoint: "workspace"}
	for _, sp := range r.Splits {
		if contains(sp.Roles, string(annotation.SplitRoleHeldOut)) {
			req.HeldOutScore, req.SplitDigest = true, sp.SplitDigest
		}
	}
	d := svc.Check(req)
	r.Eval = &workspaceEligibility{Purpose: req.Purpose, HeldOutScore: req.HeldOutScore, SplitDigest: req.SplitDigest, Allowed: d.Allowed, Reasons: []string{}}
	for _, reason := range d.Reasons {
		if reason.Blocking {
			r.Eval.Reasons = append(r.Eval.Reasons, reason.Message)
		}
	}
	cohorts := map[string]bool{}
	for _, in := range d.Inputs {
		cohorts[string(in.Assignment.Cohort)] = true
		if in.Assignment.SiteID != "" {
			r.SiteID = in.Assignment.SiteID
		}
		for _, reason := range in.Reasons {
			if reason.Blocking {
				r.Eval.Reasons = append(r.Eval.Reasons, in.Identity.Basename+": "+reason.Message)
			}
		}
	}
	switch {
	case cohorts["qualification"]:
		r.Cohort, r.Protected = "qualification", true
	case len(cohorts) == 1:
		for c := range cohorts {
			r.Cohort = c
		}
	case len(cohorts) > 1:
		r.Cohort = "conflicting"
	}
}

func segmentTasks(r *workspaceRow) []labelwork.Task {
	sg := r.Segments[0]
	t := labelwork.Task{Layer: "spans", Needs: []string{"human"}, Where: labelwork.WhereBrowser,
		SourceRevision: "segment:" + sg.SegmentID}
	switch {
	case sg.PackMissing:
		t.Action, t.Reason, t.TBD, t.Blocked = "Find or re-cut the segment's pack", "the database names pack "+sg.PackDir+", which is not under the annotation root", "unavailable_source", true
	case sg.PackDir == "":
		t.Action, t.Reason = "Queue the clip's pack from the Segments page", "the segment has a clip but no pack to label yet"
	default:
		t.Action, t.Reason = "Wait for the pack job", "the pack job is queued or running"
	}
	return []labelwork.Task{t}
}

func stageAndFlags(r *workspaceRow) {
	flag := func(f string) {
		if !contains(r.Flags, f) {
			r.Flags = append(r.Flags, f)
		}
	}
	for _, t := range r.Tasks {
		if t.Blocked {
			flag("blocked")
		}
		if t.TBD != "" {
			flag("tbd")
		}
	}
	for _, it := range r.Workflow {
		switch it.State {
		case "blocked":
			flag("blocked")
		case "tbd":
			flag("tbd")
		}
	}
	if r.Error != "" {
		flag("unavailable")
		if strings.Contains(r.Error, "no such file") || strings.Contains(r.Error, "not exist") {
			flag("missing_files")
		}
	}
	for _, pk := range r.Packs {
		if pk.Error != "" {
			flag("unavailable")
		}
		if pk.Physical != nil && (pk.Physical.Stale > 0 || len(pk.Physical.ProposedRecords) > 0) {
			if pk.Physical.Stale > 0 {
				flag("stale_revision")
			}
			if len(pk.Physical.ProposedRecords) > 0 {
				flag("ready_for_review")
			}
		}
	}
	frozenHead := false
	for _, sp := range r.Splits {
		if sp.HeadMoved {
			flag("stale_revision")
		} else {
			frozenHead = true
		}
	}
	p := r.Progress
	switch {
	case r.Error != "" && p == nil:
		r.Stage = "unavailable"
	case r.Kind == "segment":
		r.Stage = "unlabelled"
	case p == nil || p.Objects == 0:
		r.Stage = "unlabelled"
	case frozenHead:
		r.Stage = "frozen"
	case p.ProposedObjects > 0 || p.ProposedMasks > 0:
		r.Stage = "in_review"
		flag("ready_for_review")
	case len(p.FreezeBlockers) == 0:
		r.Stage = "ready_to_freeze"
		flag("ready_for_freeze")
	default:
		r.Stage = "labelling"
	}
	if r.Protected {
		r.Stage = "protected"
	}
	sort.Strings(r.Flags)
}

// redactRow keeps only aggregate metadata of blind qualification data: no
// pack handles, label-set paths, object lists or previews reach a developer
// session.
func redactRow(r *workspaceRow) {
	r.Title = "Protected qualification item"
	for i := range r.Packs {
		r.Packs[i] = workspacePack{Handle: "(protected)", PackDigest: r.Packs[i].PackDigest}
	}
	for i := range r.Segments {
		sg := r.Segments[i]
		r.Segments[i] = workspaceSegment{SegmentID: sg.SegmentID, Role: sg.Role}
	}
	if r.LabelSet != nil {
		r.LabelSet.Dir, r.LabelSet.Name = "", ""
	}
	if r.Progress != nil {
		p := *r.Progress
		p.FreezeBlockers = []string{}
		r.Progress = &p
	}
	r.Runs = []string{}
	r.spans = nil
}

func rowRank(r workspaceRow) int {
	switch r.Stage {
	case "in_review":
		return 0
	case "ready_to_freeze":
		return 1
	case "labelling":
		return 2
	case "unlabelled":
		return 3
	case "frozen":
		return 4
	case "unavailable":
		return 5
	}
	return 6
}

func (ws *Server) workspaceSegments() ([]workspaceSegment, error) {
	if ws.db == nil {
		return nil, errors.New("no database: segments, clips and runs cannot be listed")
	}
	rows, err := ws.db.Query(`SELECT s.segment_id, COALESCE(s.run_id, ''), s.clip_id, s.role, s.finder, s.capture,
		s.window_start_ns, s.window_end_ns, COALESCE(JSON_EXTRACT(s.selector_json, '$.id'), ''),
		COALESCE(JSON_EXTRACT(s.selector_json, '$.held_out_eligible'), 0), COALESCE(j.pack_dir, ''),
		COALESCE(j.pack_digest, ''), s.created_at_ns
		FROM lidar_segment_selections s
		LEFT JOIN lidar_segment_clip_jobs j ON j.segment_id = s.segment_id AND j.pack_digest IS NOT NULL
		ORDER BY s.created_at_ns DESC LIMIT ?`, maxWorkspaceSegments)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var out []workspaceSegment
	for rows.Next() {
		var sg workspaceSegment
		var held int
		if err := rows.Scan(&sg.SegmentID, &sg.RunID, &sg.ClipID, &sg.Role, &sg.Finder, &sg.Capture, &sg.WindowStartNs,
			&sg.WindowEndNs, &sg.Selector, &held, &sg.PackDir, &sg.PackDigest, &sg.CreatedAtNs); err != nil {
			return nil, err
		}
		if seen[sg.SegmentID] {
			continue
		}
		seen[sg.SegmentID] = true
		sg.HeldOutEligible = held == 1
		out = append(out, sg)
	}
	return out, rows.Err()
}

func (ws *Server) workspaceSplits() ([]workspaceSplit, error) {
	if ws.annotationPacksDir == "" {
		return nil, errors.New("annotation packs directory is not configured")
	}
	entries, err := os.ReadDir(filepath.Join(ws.annotationPacksDir, splitsDirName))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []workspaceSplit
	var bad []string
	for _, e := range entries {
		if e.IsDir() || !splitNamePattern.MatchString(e.Name()) {
			continue
		}
		if len(out)+len(bad) >= maxWorkspaceSplits {
			bad = append(bad, fmt.Sprintf("more than %d splits", maxWorkspaceSplits))
			break
		}
		f, err := annotation.LoadFrozenSplit(filepath.Join(ws.annotationPacksDir, splitsDirName, e.Name()))
		if err != nil {
			bad = append(bad, e.Name()+": "+err.Error())
			continue
		}
		out = append(out, workspaceSplit{name: e.Name(), split: f})
	}
	if len(bad) > 0 {
		return out, fmt.Errorf("unreadable splits: %s", strings.Join(bad, "; "))
	}
	return out, nil
}

// workspaceFilter is a page request.
type workspaceFilter struct {
	kind, q, capture, run, cohort, role, eligible, stage, assignee, flag string
	limit, offset                                                        int
}

func parseWorkspaceFilter(r *http.Request) workspaceFilter {
	q := r.URL.Query()
	f := workspaceFilter{kind: q.Get("kind"), q: strings.ToLower(strings.TrimSpace(q.Get("q"))), capture: strings.ToLower(q.Get("capture")),
		run: q.Get("run"), cohort: q.Get("cohort"), role: q.Get("role"), eligible: q.Get("eligible"), stage: q.Get("stage"),
		assignee: strings.ToLower(strings.TrimSpace(q.Get("assignee"))), flag: q.Get("flag")}
	f.limit, _ = strconv.Atoi(q.Get("limit"))
	f.offset, _ = strconv.Atoi(q.Get("offset"))
	if f.limit <= 0 {
		f.limit = defaultWorkspacePage
	}
	f.limit = min(f.limit, maxWorkspacePage)
	f.offset = max(f.offset, 0)
	return f
}

func (f workspaceFilter) match(r workspaceRow) bool {
	if f.kind != "" && r.Kind != f.kind {
		return false
	}
	if f.cohort != "" && r.Cohort != f.cohort {
		return false
	}
	if f.stage != "" && r.Stage != f.stage {
		return false
	}
	if f.flag != "" && !contains(r.Flags, f.flag) {
		return false
	}
	if f.run != "" && !contains(r.Runs, f.run) {
		return false
	}
	if f.role != "" {
		if f.role == "none" {
			if len(r.Roles) > 0 {
				return false
			}
		} else if !contains(r.Roles, "split:"+f.role) && !contains(r.Roles, "selection:"+f.role) {
			return false
		}
	}
	if f.eligible != "" {
		allowed := r.Eval != nil && r.Eval.Allowed
		if (f.eligible == "yes") != allowed {
			return false
		}
	}
	if f.capture != "" {
		found := false
		for _, c := range r.Captures {
			found = found || strings.Contains(strings.ToLower(c.Basename), f.capture) || strings.Contains(c.SHA256, f.capture)
		}
		if !found {
			return false
		}
	}
	if f.assignee != "" {
		found := false
		for _, it := range r.Workflow {
			found = found || strings.ToLower(it.Assignee) == f.assignee || strings.ToLower(it.Reviewer) == f.assignee
		}
		if !found {
			return false
		}
	}
	if f.q != "" {
		hay := []string{strings.ToLower(r.Title), strings.ToLower(r.Key)}
		for _, pk := range r.Packs {
			hay = append(hay, strings.ToLower(pk.Handle))
		}
		for _, c := range r.Captures {
			hay = append(hay, strings.ToLower(c.Basename))
		}
		hay = append(hay, r.Runs...)
		found := false
		for _, h := range hay {
			found = found || strings.Contains(h, f.q)
		}
		if !found {
			return false
		}
	}
	return true
}

func (ws *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	now := time.Now()
	rows, sources, splitCount := ws.buildWorkspace(now)
	f := parseWorkspaceFilter(r)
	resp := workspaceResponse{GeneratedAtNs: now.UnixNano(), Sources: sources, Eligibility: ws.eligibilityService().Status(),
		Limit: f.limit, Offset: f.offset, Items: []workspaceRow{}, Queue: []queueEntry{}, Facets: map[string][]string{}}
	resp.Totals = workspaceTotalsOf(rows, splitCount)
	var matched []workspaceRow
	runs := map[string]bool{}
	for _, row := range rows {
		for _, run := range row.Runs {
			runs[run] = true
		}
		if f.match(row) {
			matched = append(matched, row)
		}
	}
	resp.Total = len(matched)
	if f.offset < len(matched) {
		resp.Items = matched[f.offset:min(len(matched), f.offset+f.limit)]
	}
	for _, row := range matched {
		for _, t := range row.Tasks {
			resp.Queue = append(resp.Queue, queueEntry{Key: row.Key, Title: row.Title, Task: t})
		}
	}
	sort.SliceStable(resp.Queue, func(i, j int) bool {
		if resp.Queue[i].Blocked != resp.Queue[j].Blocked {
			return !resp.Queue[i].Blocked
		}
		return layerIndex(resp.Queue[i].Layer) < layerIndex(resp.Queue[j].Layer)
	})
	if len(resp.Queue) > 100 {
		resp.Queue = resp.Queue[:100]
	}
	for run := range runs {
		resp.Facets["runs"] = append(resp.Facets["runs"], run)
	}
	sort.Strings(resp.Facets["runs"])
	resp.Plans = labelwork.ReadPlans(".")
	sources["plans"] = sourceState{Available: resp.Plans.Available, Error: resp.Plans.Error}
	ws.writeJSON(w, http.StatusOK, resp)
}

func layerIndex(layer string) int {
	for i, l := range labelwork.Layers {
		if l == layer {
			return i
		}
	}
	return len(labelwork.Layers)
}

// workspaceTotalsOf sums the label sets' layers, each label set once.
func workspaceTotalsOf(rows []workspaceRow, splits int) workspaceTotals {
	t := workspaceTotals{Rows: len(rows), Splits: splits, Stages: map[string]int{}, Layers: []labelwork.LayerProgress{}}
	captures, runs := map[string]bool{}, map[string]bool{}
	type acc struct {
		layer labelwork.LayerProgress
		have  bool
	}
	layers := map[string]*acc{}
	var layerOrder []string
	for _, r := range rows {
		t.Stages[r.Stage]++
		t.Packs += len(r.Packs)
		t.Segments += len(r.Segments)
		for _, c := range r.Captures {
			key := c.SHA256
			if key == "" {
				key = "name:" + c.Basename
			}
			captures[key] = true
		}
		for _, run := range r.Runs {
			runs[run] = true
		}
		for _, task := range r.Tasks {
			t.Outstanding++
			if task.Blocked {
				t.Blocked++
			}
			if task.TBD != "" {
				t.TBD++
			}
		}
		if r.Kind != "label_set" {
			continue
		}
		t.LabelSets++
		if r.Progress == nil {
			t.Unreadable++
			continue
		}
		for _, l := range r.Progress.Layers {
			a := layers[l.Layer]
			if a == nil {
				a = &acc{layer: labelwork.LayerProgress{Layer: l.Layer, Title: l.Title, Where: l.Where, Note: l.Note}}
				layers[l.Layer] = a
				layerOrder = append(layerOrder, l.Layer)
			}
			if !a.have {
				a.layer.Counts = make([]labelwork.Count, len(l.Counts))
				for i, c := range l.Counts {
					a.layer.Counts[i] = labelwork.Count{Name: c.Name, Unit: c.Unit, Total: new(int)}
				}
				a.have = true
			}
			for i, c := range l.Counts {
				a.layer.Counts[i].Done += c.Done
				if c.Total == nil || a.layer.Counts[i].Total == nil {
					a.layer.Counts[i].Total = nil
				} else {
					*a.layer.Counts[i].Total += *c.Total
				}
			}
		}
	}
	t.Captures, t.Runs = len(captures), len(runs)
	for _, name := range layerOrder {
		l := layers[name].layer
		if t.Unreadable > 0 {
			for i := range l.Counts {
				l.Counts[i].Total = nil
			}
			l.Note = fmt.Sprintf("%d label sets could not be read, so no total is known. %s", t.Unreadable, l.Note)
		}
		t.Layers = append(t.Layers, l)
	}
	return t
}

// journalEntry is one label-set revision as its journal records it.
type journalEntry struct {
	Revision     int    `json:"revision"`
	State        string `json:"state"`
	Author       string `json:"author"`
	Operation    string `json:"operation"`
	CreatedUTC   string `json:"created_utc"`
	RestoredFrom int    `json:"restored_from,omitempty"`
}

// readJournal returns the journal's newest entries, oldest first. The
// journal is local history; a published copy has none.
func readJournal(dir string) ([]journalEntry, error) {
	f, err := os.Open(filepath.Join(dir, labelset.JournalFile))
	if os.IsNotExist(err) {
		return []journalEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 256<<20)
	var out []journalEntry
	for sc.Scan() {
		var d struct {
			Revision     int             `json:"revision"`
			State        string          `json:"state"`
			Change       labelset.Change `json:"change"`
			RestoredFrom int             `json:"restored_from"`
		}
		if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
			continue
		}
		out = append(out, journalEntry{Revision: d.Revision, State: d.State, Author: d.Change.Author, Operation: d.Change.Operation,
			CreatedUTC: d.Change.CreatedUTC, RestoredFrom: d.RestoredFrom})
		if len(out) > maxJournalEntries {
			out = out[1:]
		}
	}
	if out == nil {
		out = []journalEntry{}
	}
	return out, sc.Err()
}

type workspaceDetail struct {
	Row        workspaceRow           `json:"row"`
	Restricted bool                   `json:"restricted"`
	Objects    []labelwork.ObjectSpan `json:"objects"`
	Journal    []journalEntry         `json:"journal"`
	JournalErr string                 `json:"journal_error,omitempty"`
	History    []labelwork.Item       `json:"workflow_history"`
	Layers     []string               `json:"layers"`
	States     []string               `json:"states"`
	TBDReasons []string               `json:"tbd_reasons"`
}

func (ws *Server) handleWorkspaceItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	key := r.URL.Query().Get("key")
	rows, _, _ := ws.buildWorkspace(time.Now())
	var row *workspaceRow
	for i := range rows {
		if rows[i].Key == key {
			row = &rows[i]
		}
	}
	if row == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no workspace item %q", key))
		return
	}
	d := workspaceDetail{Row: *row, Restricted: row.Protected, Objects: []labelwork.ObjectSpan{}, Journal: []journalEntry{},
		History: []labelwork.Item{}, Layers: labelwork.Layers, States: labelwork.States, TBDReasons: labelwork.TBDReasons}
	if !row.Protected {
		if row.spans != nil {
			d.Objects = row.spans
		}
		if row.labelSetDir != "" {
			if j, err := readJournal(row.labelSetDir); err != nil {
				d.JournalErr = err.Error()
			} else {
				d.Journal = j
			}
		}
	}
	if st := ws.workflowStore(); st != nil {
		if h, err := st.History(key, 0); err == nil {
			d.History = h
		}
	}
	ws.writeJSON(w, http.StatusOK, d)
}

type workflowPutRequest struct {
	labelwork.Item
	BaseRevision int `json:"base_revision"`
}

func (ws *Server) handleWorkspaceWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	st := ws.workflowStore()
	if st == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no database: workflow items cannot be saved")
		return
	}
	var req workflowPutRequest
	if !ws.decodePhysicalRequest(w, r, &req) {
		return
	}
	item := req.Item
	item.Revision, item.UpdatedAtNs, item.CreatedAtNs = 0, 0, 0
	saved, err := st.Put(item, req.BaseRevision)
	switch {
	case errors.Is(err, labelwork.ErrStale):
		current, _ := st.Items(item.ItemKey)
		ws.writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error() + ": reload and reconcile before saving", "code": "stale", "current": current})
	case errors.Is(err, labelwork.ErrInvalid):
		ws.writeJSONError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
	default:
		ws.writeJSON(w, http.StatusOK, saved)
	}
}

type objectReviewRequest struct {
	LabelSetID   string `json:"label_set_id"`
	BaseRevision int    `json:"base_revision"`
	BaseState    string `json:"base_state"`
	ObjectID     string `json:"object_id"`
	Status       string `json:"status"`
	Reviewer     string `json:"reviewer"`
	Session      string `json:"session,omitempty"`
}

// handleObjectReview reviews, rejects or reopens one object of a label set:
// its span, class and identity. Its masks keep their own review, and no
// physical reference is touched. The write names the revision and state the
// reviewer looked at, and is refused when the head has moved since.
func (ws *Server) handleObjectReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writePhysicalError(w, http.StatusMethodNotAllowed, physicalCodeBadRequest, "this endpoint only accepts POST requests")
		return
	}
	var req objectReviewRequest
	if !ws.decodePhysicalRequest(w, r, &req) {
		return
	}
	status := annotation.ReviewStatus(req.Status)
	if status != annotation.StatusReviewed && status != annotation.StatusRejected && status != annotation.StatusProposed {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "status is reviewed, rejected or proposed")
		return
	}
	if strings.TrimSpace(req.Reviewer) == "" || req.ObjectID == "" || req.LabelSetID == "" {
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "label_set_id, object_id and reviewer are required")
		return
	}
	var dir string
	var captures []eligibility.Input
	for _, p := range ws.listPacks() {
		if p.LabelSetID == req.LabelSetID && p.LabelSetDir != "" {
			dir = p.LabelSetDir
			for _, c := range p.LabelSetCaptures {
				captures = append(captures, eligibility.Input{Identity: eligibility.Identity{Kind: eligibility.KindCapture, SHA256: c.SHA256, Basename: c.Name}})
			}
			if len(p.LabelSetCaptures) == 0 && p.Source.PCAPBasename != "" {
				captures = append(captures, eligibility.Input{Identity: eligibility.Identity{Kind: eligibility.KindCapture, Basename: p.Source.PCAPBasename},
					Err: errors.New("not bound by hash")})
			}
		}
	}
	if dir == "" {
		ws.writePhysicalError(w, http.StatusNotFound, physicalCodeNotFound, fmt.Sprintf("no label set %q under the annotation packs directory", req.LabelSetID))
		return
	}
	// Review is a use of the data too: recorded with its reviewer, and
	// refused for what may be blind qualification data while no access
	// control protects it.
	if len(captures) == 0 {
		captures = []eligibility.Input{{Identity: eligibility.Identity{Kind: eligibility.KindCapture, Basename: req.LabelSetID},
			Err: errors.New("the label set names no capture")}}
	}
	if !ws.authorizeDevelopment(w, r, eligibility.Request{Purpose: eligibility.PurposeAnnotationReview, Inputs: captures}) {
		return
	}
	ls, err := labelset.Open(dir)
	if err != nil {
		ws.writePhysicalError(w, http.StatusInternalServerError, physicalCodeInternal, "open label set: "+err.Error())
		return
	}
	if ls.Revision() != req.BaseRevision || ls.StateDigest() != req.BaseState {
		ws.writeJSON(w, http.StatusConflict, map[string]any{
			"error": fmt.Sprintf("the label set is at revision %d (%s); the review was made against %d (%s): reload and review again",
				ls.Revision(), ls.StateDigest(), req.BaseRevision, req.BaseState),
			"code": physicalCodeConflict, "revision": ls.Revision(), "state": ls.StateDigest(),
		})
		return
	}
	found := false
	for i := range ls.State.Records.Objects {
		o := &ls.State.Records.Objects[i]
		if o.ObjectID == req.ObjectID {
			found = true
			o.Status = status
		}
	}
	if !found {
		ws.writePhysicalError(w, http.StatusNotFound, physicalCodeNotFound, fmt.Sprintf("label set %s has no object %q", req.LabelSetID, req.ObjectID))
		return
	}
	err = ls.Save(labelset.Change{Author: req.Reviewer, Session: req.Session, Operation: "object_review:" + req.Status,
		CreatedUTC: time.Now().UTC().Format(time.RFC3339)})
	switch {
	case errors.Is(err, labelset.ErrConflict):
		ws.writePhysicalError(w, http.StatusConflict, physicalCodeConflict, "the label set changed while saving: reload and review again")
		return
	case err != nil:
		ws.writePhysicalError(w, http.StatusUnprocessableEntity, physicalCodeInvalid, err.Error())
		return
	}
	progress := labelwork.MembershipProgress(&ls.State)
	ws.writeJSON(w, http.StatusOK, map[string]any{"revision": ls.Revision(), "state": ls.StateDigest(), "progress": progress})
}
