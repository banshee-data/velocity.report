package server

import (
	"errors"
	"net/http"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/labelwork"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/labelworkdb"
)

// The labelling plans (data/experiments/label/*.md), each bound by the
// operator to the window it is run on, with every step's status read from
// that window's stores.
//
//	PUT /api/lidar/workspace/plan-binding   {plan_id, item_key, note, bound_by, base_revision}

type workspacePlans struct {
	Available bool                     `json:"available"`
	Dir       string                   `json:"dir"`
	Error     string                   `json:"error,omitempty"`
	Plans     []labelwork.PlanProgress `json:"plans"`
}

type planCandidate struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Stage string `json:"stage"`
	Why   string `json:"why"`
	Rank  int    `json:"-"`
}

func (ws *Server) planBindings() *labelworkdb.PlanBindings {
	if ws.db == nil {
		return nil
	}
	return &labelworkdb.PlanBindings{DB: ws.db.DB}
}

// planFacts is what a plan's step rules read about a row.
func planFacts(r workspaceRow) *labelwork.PlanFacts {
	f := &labelwork.PlanFacts{ItemKey: r.Key, Title: r.Title, Stage: r.Stage, Progress: r.Progress, Frozen: r.Splits,
		WorkflowDone: map[string]bool{}, Protected: r.Protected}
	for _, pk := range r.Packs {
		if pk.Error == "" {
			f.HasPack = true
		}
		f.Packs = append(f.Packs, labelwork.PackFacts{Handle: pk.Handle, Digest: pk.PackDigest, Physical: pk.Physical, Error: pk.Error})
	}
	for _, sg := range r.Segments {
		f.Segments = append(f.Segments, labelwork.SegmentFacts{SegmentID: sg.SegmentID, Role: sg.Role, Finder: sg.Finder, Capture: sg.Capture})
	}
	for _, it := range r.Workflow {
		if it.State == "done" {
			f.WorkflowDone[it.Layer] = true
		}
	}
	return f
}

// planProgress evaluates every plan against the rows (all of them, not a
// filtered page) and lists the rows a plan could be bound to.
func (ws *Server) planProgress(rows []workspaceRow) (workspacePlans, []planCandidate) {
	root := ws.plansRoot
	if root == "" {
		root = "."
	}
	read := labelwork.ReadPlans(root)
	out := workspacePlans{Available: read.Available, Dir: read.Dir, Error: read.Error, Plans: []labelwork.PlanProgress{}}
	byKey := map[string]workspaceRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	bindings := map[string]labelwork.PlanBinding{}
	if b := ws.planBindings(); b != nil {
		all, err := b.All()
		if err != nil {
			out.Error = "plan bindings could not be read: " + err.Error()
		} else {
			bindings = all
		}
	} else if out.Error == "" {
		out.Error = "no database: plans cannot be bound to a window"
	}
	// Twice, so a plan that depends on one listed after it sees its result.
	done := map[string]bool{}
	for pass := 0; pass < 2; pass++ {
		out.Plans = out.Plans[:0]
		for _, p := range read.Plans {
			var binding *labelwork.PlanBinding
			var facts *labelwork.PlanFacts
			boundErr := ""
			if b, ok := bindings[p.ID]; ok && b.ItemKey != "" {
				bc := b
				binding = &bc
				if r, ok := byKey[b.ItemKey]; ok {
					facts = planFacts(r)
				} else {
					boundErr = "the bound item " + b.ItemKey + " is not in the inventory: its files may be missing"
				}
			}
			pp := labelwork.Evaluate(p, binding, facts, boundErr, done)
			done[p.ID] = len(pp.Progress) > 0 && pp.StepsDone == len(pp.Progress)
			out.Plans = append(out.Plans, pp)
		}
	}
	var candidates []planCandidate
	for _, r := range rows {
		if r.Protected || r.Kind == "pack" && len(r.Segments) == 0 && r.LabelSet == nil && r.Error != "" {
			continue
		}
		c := planCandidate{Key: r.Key, Title: r.Title, Stage: r.Stage, Rank: 2, Why: "a " + r.Kind}
		for _, sg := range r.Segments {
			if sg.Role == "held_out" && sg.Finder == "random" {
				c.Rank, c.Why = 0, "drawn by lot (random) for held-out use"
			} else if c.Rank > 1 {
				c.Rank, c.Why = 1, "selected by "+sg.Finder+" for "+sg.Role
			}
		}
		candidates = append(candidates, c)
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Rank < candidates[j].Rank })
	if len(candidates) > 200 {
		candidates = candidates[:200]
	}
	if candidates == nil {
		candidates = []planCandidate{}
	}
	return out, candidates
}

type planBindingRequest struct {
	PlanID       string `json:"plan_id"`
	ItemKey      string `json:"item_key"`
	Note         string `json:"note"`
	BoundBy      string `json:"bound_by"`
	BaseRevision int    `json:"base_revision"`
}

func (ws *Server) handlePlanBinding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	b := ws.planBindings()
	if b == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no database: plans cannot be bound")
		return
	}
	var req planBindingRequest
	if !ws.decodePhysicalRequest(w, r, &req) {
		return
	}
	saved, err := b.Put(labelwork.PlanBinding{PlanID: req.PlanID, ItemKey: req.ItemKey, Note: req.Note, BoundBy: req.BoundBy}, req.BaseRevision)
	switch {
	case errors.Is(err, labelwork.ErrStale):
		ws.writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error() + ": reload before binding", "code": "stale"})
	case errors.Is(err, labelwork.ErrInvalid):
		ws.writeJSONError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
	default:
		ws.writeJSON(w, http.StatusOK, saved)
	}
}
