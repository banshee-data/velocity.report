package jobs

import (
	"fmt"
	"time"
)

// State is where a job's current attempt stands. The states, their owners and
// their meanings are the pool plan's §5.
type State string

const (
	StateQueued    State = "queued"
	StateLeased    State = "leased"
	StateRunning   State = "running"
	StateUploading State = "uploading"
	StateVerifying State = "verifying"
	StateAccepted  State = "accepted"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
	StateLost      State = "lost"
)

// Actor is who may make a transition. The hub owns state; a worker may only
// move the attempt it holds the lease for, and only forwards.
type Actor string

const (
	ActorHub    Actor = "hub"
	ActorWorker Actor = "worker"
	// ActorOperator is a person, through the hub: the only one who cancels.
	ActorOperator Actor = "operator"
)

// Terminal reports whether no further transition is possible. A terminal
// attempt is evidence; a retry is a new attempt.
func (s State) Terminal() bool {
	switch s {
	case StateAccepted, StateFailed, StateCancelled, StateLost:
		return true
	}
	return false
}

// transitions is the whole state machine: from, to, and who may do it.
var transitions = map[State]map[State][]Actor{
	StateQueued: {
		StateLeased:    {ActorHub},
		StateCancelled: {ActorOperator},
	},
	StateLeased: {
		StateRunning:   {ActorWorker},
		StateLost:      {ActorHub},
		StateCancelled: {ActorOperator},
		// A worker that finds it cannot run the job it leased, such as a
		// capture whose bytes do not match, fails it rather than dropping it.
		StateFailed: {ActorWorker},
	},
	StateRunning: {
		StateUploading: {ActorWorker},
		StateFailed:    {ActorWorker},
		StateLost:      {ActorHub},
		StateCancelled: {ActorOperator},
	},
	StateUploading: {
		StateVerifying: {ActorHub},
		StateFailed:    {ActorWorker},
		StateLost:      {ActorHub},
		StateCancelled: {ActorOperator},
	},
	StateVerifying: {
		StateAccepted: {ActorHub},
		StateFailed:   {ActorHub},
	},
}

// Transition reports whether actor may move an attempt from one state to
// another, and why not when it may not.
func Transition(from, to State, actor Actor) error {
	if from.Terminal() {
		return fmt.Errorf("attempt is %s, which is final; a retry is a new attempt", from)
	}
	allowed, ok := transitions[from][to]
	if !ok {
		return fmt.Errorf("no transition from %s to %s", from, to)
	}
	for _, a := range allowed {
		if a == actor {
			return nil
		}
	}
	return fmt.Errorf("%s may not move an attempt from %s to %s", actor, from, to)
}

// Attempt is one execution of a job. A job has many attempts over its life
// and at most one that is not terminal.
type Attempt struct {
	AttemptID string `json:"attempt_id"`
	JobID     string `json:"job_id"`
	// Ordinal is which attempt of the job this is, from one.
	Ordinal int `json:"ordinal"`
	// WorkerID is the worker that leased it, once one has.
	WorkerID string `json:"worker_id,omitempty"`
	State    State  `json:"state"`
	// LeaseExpires is when the worker's claim lapses unless renewed. The
	// lease is authoritative; heartbeats and progress are advisory.
	LeaseExpires *time.Time `json:"lease_expires,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	// Progress is advisory: how far the worker says it has got.
	Progress Progress `json:"progress"`
	// Failure is why a failed or lost attempt ended, with what evidence the
	// worker kept.
	Failure *Failure `json:"failure,omitempty"`
	// BundleDigest is set once a bundle has been accepted for this attempt.
	BundleDigest Digest `json:"bundle_digest,omitempty"`
}

// Progress is what a worker reports while it runs.
type Progress struct {
	Current int64  `json:"current"`
	Total   int64  `json:"total"`
	Detail  string `json:"detail,omitempty"`
}

// Failure is the evidence of an attempt that did not produce an accepted
// result. The partial directory stays where it is; this says where.
type Failure struct {
	Reason string `json:"reason"`
	// LocalPath is where the worker left what it had, on the worker.
	LocalPath string `json:"local_path,omitempty"`
	// Partial says a bundle exists but is incomplete.
	Partial bool `json:"partial,omitempty"`
}

// LeaseDuration is how long a lease lasts without renewal. Long enough that
// a worker mid-hash of a large capture is not lost; short enough that a blade
// that was switched off is not waited for through lunch.
const LeaseDuration = 3 * time.Minute

// LeaseExpired reports whether the attempt's lease has lapsed at now.
func (a Attempt) LeaseExpired(now time.Time) bool {
	if a.State.Terminal() || a.LeaseExpires == nil {
		return false
	}
	switch a.State {
	case StateLeased, StateRunning, StateUploading:
		return now.After(*a.LeaseExpires)
	}
	return false
}
