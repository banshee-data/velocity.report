package runner

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/jobs"
)

// Campaign is one set of configs over one set of captures: what
// data/experiments/try/option-scorecard/pass7-configs.json is. Submitted as
// one, it queues one child attempt per config, all of the same kind, and its
// summary is the table of the children's.
type Campaign struct {
	// Kind is what each config runs; today state_estimation_baseline.
	Kind string `json:"kind"`
	// Base is the request every config starts from: the captures, the
	// tuning document, the commit and the replay contract.
	Base jobs.JobRequest `json:"base"`
	// Configs are the variations, each applied to the base.
	Configs []CampaignConfig `json:"configs"`
	Note    string           `json:"note,omitempty"`
}

// CampaignConfig is one variation: a name, dotted-key overrides on the
// tuning document, and the experiments and parameters for that config.
type CampaignConfig struct {
	Name string `json:"name"`
	// Overrides are dotted keys into the tuning document, such as
	// "l4.dbscan_xy_v1.foreground_dbscan_eps", to the value to set.
	Overrides   map[string]any  `json:"overrides,omitempty"`
	Experiments []string        `json:"experiments,omitempty"`
	Params      json.RawMessage `json:"params,omitempty"`
}

// Expand turns a campaign into the child requests it queues, in order.
func (c Campaign) Expand() ([]jobs.JobRequest, []string, error) {
	if _, ok := jobs.Kinds[c.Kind]; !ok {
		return nil, nil, fmt.Errorf("unknown kind %q", c.Kind)
	}
	if len(c.Configs) == 0 {
		return nil, nil, fmt.Errorf("a campaign needs at least one config")
	}
	var base any
	if err := json.Unmarshal(c.Base.Tuning, &base); err != nil {
		return nil, nil, fmt.Errorf("base tuning: %w", err)
	}
	seen := map[string]bool{}
	var requests []jobs.JobRequest
	var names []string
	for i, cfg := range c.Configs {
		name := strings.TrimSpace(cfg.Name)
		if name == "" {
			return nil, nil, fmt.Errorf("config %d has no name", i)
		}
		if seen[name] {
			return nil, nil, fmt.Errorf("config %q appears twice", name)
		}
		seen[name] = true
		tuning, err := applyOverrides(base, cfg.Overrides)
		if err != nil {
			return nil, nil, fmt.Errorf("config %s: %w", name, err)
		}
		req := c.Base
		req.Kind = c.Kind
		req.Tuning = tuning
		experiments := append([]string(nil), cfg.Experiments...)
		sort.Strings(experiments)
		req.Experiments = experiments
		if len(cfg.Params) > 0 {
			req.Params = cfg.Params
		}
		req.Note = strings.TrimSpace(c.Note + " " + name)
		if err := req.Validate(); err != nil {
			return nil, nil, fmt.Errorf("config %s: %w", name, err)
		}
		requests = append(requests, req)
		names = append(names, name)
	}
	return requests, names, nil
}

// applyOverrides sets dotted keys in a copy of the document. A key must name
// an existing path: a typo that created a new key would silently run the
// defaults and report a config that never was.
func applyOverrides(base any, overrides map[string]any) (json.RawMessage, error) {
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.Split(key, ".")
		node := doc
		for i, part := range parts {
			obj, ok := node.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("override %q: %q is not an object", key, strings.Join(parts[:i], "."))
			}
			if i == len(parts)-1 {
				if _, exists := obj[part]; !exists {
					return nil, fmt.Errorf("override %q names a key the tuning document does not have", key)
				}
				obj[part] = overrides[key]
				break
			}
			next, exists := obj[part]
			if !exists {
				return nil, fmt.Errorf("override %q names a key the tuning document does not have", key)
			}
			node = next
		}
	}
	return json.Marshal(doc)
}

// SubmitCampaign queues the campaign: a parent record that lists its
// children, and one child per config. The parent is never run itself; it
// completes when its children have.
func (s *Store) SubmitCampaign(c Campaign, now time.Time) (Record, error) {
	requests, names, err := c.Expand()
	if err != nil {
		return Record{}, err
	}
	parentID := newID("cmp", now)
	parent := Record{
		Job: jobs.JobRequest{Kind: c.Kind, CaptureManifest: c.Base.CaptureManifest, Tuning: c.Base.Tuning,
			Code: c.Base.Code, Replay: c.Base.Replay, Note: c.Note},
		Attempt: jobs.Attempt{AttemptID: parentID, JobID: parentID, Ordinal: 1, State: jobs.StateQueued, CreatedAt: now.UTC()},
		Label:   "campaign",
	}
	for i, req := range requests {
		child, err := s.SubmitChild(req, parentID, names[i], now.Add(time.Duration(i)*time.Microsecond))
		if err != nil {
			return Record{}, err
		}
		parent.Children = append(parent.Children, child.Attempt.AttemptID)
	}
	if err := s.create(parent); err != nil {
		return Record{}, err
	}
	return parent, nil
}

// finishParent completes a campaign once every child is terminal: accepted if
// all were, failed otherwise, with a summary table either way.
func (r *Runner) finishParent(child Record) {
	if child.Parent == "" {
		return
	}
	parent, err := r.Store.Get(child.Parent)
	if err != nil || parent.Attempt.State.Terminal() {
		return
	}
	type row struct {
		Attempt string          `json:"attempt"`
		Config  string          `json:"config"`
		State   jobs.State      `json:"state"`
		Summary json.RawMessage `json:"summary,omitempty"`
		Reason  string          `json:"reason,omitempty"`
	}
	var rows []row
	allDone, allGood := true, true
	for _, id := range parent.Children {
		c, err := r.Store.Get(id)
		if err != nil {
			continue
		}
		if !c.Attempt.State.Terminal() {
			allDone = false
			continue
		}
		rw := row{Attempt: id, Config: c.Label, State: c.Attempt.State}
		if c.Attempt.State == jobs.StateAccepted {
			if b, err := r.Store.ReadBundle(id); err == nil {
				rw.Summary = b.Summary
			}
		} else {
			allGood = false
			if c.Attempt.Failure != nil {
				rw.Reason = c.Attempt.Failure.Reason
			}
		}
		rows = append(rows, rw)
	}
	if !allDone {
		return
	}
	now := r.now()
	summary, _ := json.Marshal(map[string]any{"configs": rows})
	// The parent's own bundle is its table. It walks the same states so the
	// listing reads the same for a campaign as for one job.
	for _, to := range []struct {
		state jobs.State
		actor jobs.Actor
	}{{jobs.StateLeased, jobs.ActorHub}, {jobs.StateRunning, jobs.ActorWorker}, {jobs.StateUploading, jobs.ActorWorker}, {jobs.StateVerifying, jobs.ActorHub}} {
		if _, err := r.Store.Transition(parent.Attempt.AttemptID, to.state, to.actor, now); err != nil {
			r.event("%s: %v", parent.Attempt.AttemptID, err)
			return
		}
	}
	final := jobs.StateAccepted
	if !allGood {
		final = jobs.StateFailed
	}
	done, err := r.Store.Transition(parent.Attempt.AttemptID, final, jobs.ActorHub, now)
	if err != nil {
		r.event("%s: %v", parent.Attempt.AttemptID, err)
		return
	}
	_ = writeJSONUnder(r.Store.BundlePath(done.Attempt.AttemptID), "campaign.json", json.RawMessage(summary))
	if !allGood {
		done.Attempt.Failure = &jobs.Failure{Reason: "one or more configs did not complete", LocalPath: r.Store.BundlePath(done.Attempt.AttemptID)}
	}
	_ = r.Store.Save(done)
	r.event("%s %s (%d configs)", done.Attempt.AttemptID, final, len(rows))
}

func writeJSONUnder(dir, name string, v any) error {
	if err := ensureDir(dir); err != nil {
		return err
	}
	return writeJSON(dir+"/"+name, v)
}
