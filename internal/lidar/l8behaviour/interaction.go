package l8behaviour

// Persisted interaction records, per Sections 7.6, 10.2, 10.3 and 10.4 of
// docs/plans/lidar-behaviour-analytics-plan.md: the shape a following
// encounter takes in storage, and the rules a stored encounter must satisfy
// to be read back.
//
// One encounter becomes three kinds of record, one per table:
//
//   - InteractionEvent (lidar_interaction_events): identity, version, capture
//     interval, worst support, the measurements and any review-only
//     provisional block keyed by registry metric id, and the accounting with
//     its suppressions keyed by reason token. It is small, so listing and
//     aggregating events never parses a series.
//   - InteractionInstant (lidar_interaction_instants): one per follower
//     instant, the suppression history and the evidence under it: role, both
//     parties' support, validity and every reason, the two physical endpoints
//     with their source and extent provenance, and the series values keyed by
//     metric id with their one-sigma.
//   - ExposureWindow (lidar_exposure_windows): maximal contiguous runs of
//     valid following time and of predicted-only time, with the band time
//     inside each observed run, so a rate over any set of encounters can be
//     recomputed without re-running the pipeline (Section 10.3).
//
// Three rules shape them.
//
//   - Observed and predicted are kept apart by basis, not by convention. An
//     instant or window is observed only when both parties were observed. A
//     predicted-only instant may carry the review-only predicted gap and
//     nothing else and is never valid, and its windows never enter a
//     denominator; an observed instant never carries the predicted gap.
//   - A suppressed value is absent. Measurements keep their suppression and
//     reason; a series value exists only where the encounter's series has one;
//     nothing is written as zero to mean "none".
//   - Records are write-once per version. An event's id is a digest of its
//     source, type, pair and whole version provenance, so regeneration under a
//     new estimator, observation model, method, parameter hash, geometry or
//     stage writes new rows beside the old rather than over them, and a
//     reader selects exactly one InteractionVersion.
//
// Validate checks a record set against itself before a write and after a
// read: the instants must add up to the event's accounting, and the windows
// must be the ones the instants imply, so a stored summary cannot disagree
// with the evidence stored under it.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strconv"
)

// InteractionRecordSchema versions the stored layout of the three records,
// independently of the methods that fill them. A reader refuses any other.
const InteractionRecordSchema = "interaction_record_v1"

// InteractionVersion is what a reader selects so that it never mixes versions
// (Section 10.3): the estimate stage, and the estimator, observation model,
// behaviour method and estimator parameter versions. Stage is part of it
// because the same pair analysed over fixed-lag and then final estimates is
// two versions of one encounter. Geometry is not: each follower's local path
// is its own geometry, fixed by the method and the evidence, and recorded per
// event.
type InteractionVersion struct {
	EstimateStage EstimateStage `json:"estimate_stage"`
	EstimatorID   string        `json:"estimator_id"`
	ObsModelID    string        `json:"obs_model_id"`
	MethodID      string        `json:"method_id"`
	ParamHash     string        `json:"param_hash"`
}

// Validate requires every axis.
func (v InteractionVersion) Validate() error {
	if !v.EstimateStage.Valid() || v.EstimatorID == "" || v.ObsModelID == "" || v.MethodID == "" || v.ParamHash == "" {
		return fmt.Errorf("interaction version requires a stage, estimator, observation model, method and parameter hash")
	}
	return nil
}

// InteractionVersion is the version a reader selects this provenance by.
func (v VersionProvenance) InteractionVersion() InteractionVersion {
	return InteractionVersion{
		EstimateStage: v.EstimateStage, EstimatorID: v.EstimatorID, ObsModelID: v.ObsModelID,
		MethodID: v.MethodID, ParamHash: v.ParamHash,
	}
}

// SuppressionCount is the instants suppressed for one reason and the time
// they stand for; ReasonTally keyed by its reason.
type SuppressionCount struct {
	Instants int   `json:"instants"`
	Nanos    int64 `json:"nanos"`
}

// InteractionAccounting is EncounterAccounting keyed for storage. It is
// bookkeeping for review and recomputation, never a published value: time
// here may belong to a measurement that is suppressed, which is why an
// aggregate reads observed windows at a selected version and a report reads
// measurements. Unobserved time is PredictedOnlyNanos, named for its basis.
type InteractionAccounting struct {
	Instants   int   `json:"instants"`
	ValidNanos int64 `json:"valid_nanos"`
	// BandNanos is the valid time below each band, keyed by the band's
	// duration metric id.
	BandNanos          map[MetricID]int64                     `json:"band_nanos"`
	PredictedOnlyNanos int64                                  `json:"predicted_only_nanos"`
	RecordGapNanos     int64                                  `json:"record_gap_nanos"`
	Suppressions       map[SuppressionReason]SuppressionCount `json:"suppressions,omitempty"`
}

// InteractionEvent is one pairwise encounter at one version: the
// lidar_interaction_events row.
type InteractionEvent struct {
	Schema  string `json:"schema"`
	EventID string `json:"event_id"`
	// SourceID is the capture the trajectories were read from: the
	// lidar_track_estimates source_id, a content-addressed capture identity.
	SourceID string          `json:"source_id"`
	Type     InteractionType `json:"interaction_type"`
	// PrimaryTrackID and SecondaryTrackID are geometric roles fixed by Type,
	// never fault: for following, the follower and the leader.
	PrimaryTrackID   string            `json:"primary_track_id"`
	SecondaryTrackID string            `json:"secondary_track_id"`
	StartUnixNanos   int64             `json:"start_unix_nanos"`
	EndUnixNanos     int64             `json:"end_unix_nanos"`
	Version          VersionProvenance `json:"version"`
	Input            InputProvenance   `json:"input"`
	WorstSupport     SupportState      `json:"worst_support"`
	// Measurements is the production contract, keyed by metric id.
	Measurements map[MetricID]Measurement `json:"measurements"`
	// Provisional carries the values of a non-final encounter. It is
	// review-only, and present exactly when the stage is not final.
	Provisional map[MetricID]Measurement `json:"provisional,omitempty"`
	Accounting  InteractionAccounting    `json:"accounting"`
}

// SeriesValue is one supported series value with its one-sigma, in the unit
// its metric id names.
type SeriesValue struct {
	Value float64 `json:"value"`
	Sigma float64 `json:"sigma"`
}

// InteractionInstant is one follower instant of an encounter: the
// lidar_interaction_instants row.
type InteractionInstant struct {
	EventID          string `json:"event_id"`
	CaptureUnixNanos int64  `json:"capture_unix_nanos"`
	// IntervalNanos is the time the instant stands for, as EncounterInstant.
	IntervalNanos int64 `json:"interval_nanos"`
	RecordGap     bool  `json:"record_gap,omitempty"`
	// Basis is observed exactly when both parties were observed.
	Basis           ObservationBasis     `json:"basis"`
	Role            CandidateDisposition `json:"role"`
	FollowerSupport SupportState         `json:"follower_support"`
	LeaderSupport   SupportState         `json:"leader_support"`
	Valid           bool                 `json:"valid"`
	Reason          SuppressionReason    `json:"reason,omitempty"`
	Condition       PathCondition        `json:"condition,omitempty"`
	// Reasons is every reason at the evaluated point, in precedence order;
	// absent when the pair was not evaluated at this instant.
	Reasons []SuppressionReason `json:"reasons,omitempty"`
	// LeaderTrailing and FollowerLeading are the physical endpoints the gap
	// is measured between, present when both bodies projected; on a
	// predicted-only instant they are predicted, and say so in their support.
	LeaderTrailing  *Endpoint `json:"leader_trailing,omitempty"`
	FollowerLeading *Endpoint `json:"follower_leading,omitempty"`
	// Values holds the instant's series values keyed by metric id: on an
	// observed instant the spatial gap where supported and the net time gap
	// where valid, and on a predicted-only instant the review-only predicted
	// gap alone. The values carry the event's stage.
	Values map[MetricID]SeriesValue `json:"values,omitempty"`
	// CoastAgeNanos is the longest time since either party was last observed;
	// zero on an observed instant.
	CoastAgeNanos int64 `json:"coast_age_nanos,omitempty"`
}

// ExposureWindow is one maximal contiguous run of an encounter's time on one
// basis: the lidar_exposure_windows row. Only an observed window is
// opportunity (Section 6, rule 1); a predicted-only one records the time a
// prediction stood in for observation, so it can be shown and never counted.
type ExposureWindow struct {
	WindowID string       `json:"window_id"`
	SourceID string       `json:"source_id"`
	EventID  string       `json:"event_id"`
	Kind     ExposureKind `json:"kind"`
	// Basis decides whether the window may enter a denominator.
	Basis ObservationBasis `json:"basis"`
	// TrackID is the subject, the follower for valid_following; the
	// counterpart is the leader.
	TrackID            string `json:"track_id"`
	CounterpartTrackID string `json:"counterpart_track_id"`
	StartUnixNanos     int64  `json:"start_unix_nanos"`
	EndUnixNanos       int64  `json:"end_unix_nanos"`
	// DurationNanos is End - Start. Nanoseconds rather than the plan's
	// seconds, so sums over windows are exact.
	DurationNanos int64 `json:"duration_nanos"`
	// BandNanos is the window's time below each band, keyed by the band's
	// duration metric id, on an observed window only.
	BandNanos map[MetricID]int64 `json:"band_nanos,omitempty"`
	Version   VersionProvenance  `json:"version"`
}

// FollowingInteraction is one following encounter as stored: its event, its
// instants in capture order and its windows in start order.
type FollowingInteraction struct {
	Event    InteractionEvent     `json:"event"`
	Instants []InteractionInstant `json:"instants"`
	Windows  []ExposureWindow     `json:"windows"`
}

// InteractionEventID is an event's identity: a digest of its source, type,
// pair and every version axis, so any change to one is a new event.
func InteractionEventID(sourceID string, t InteractionType, primaryTrackID, secondaryTrackID string, v VersionProvenance) string {
	return "interaction/v1/" + identityDigest("interaction-event-v1", sourceID, t.String(), primaryTrackID,
		secondaryTrackID, v.EstimateStage.String(), v.EstimatorID, v.ObsModelID, v.MethodID, v.GeometryID, v.ParamHash)
}

// ExposureWindowID is a window's identity within its event.
func ExposureWindowID(eventID string, k ExposureKind, b ObservationBasis, startUnixNanos int64) string {
	return "exposure/v1/" + identityDigest("exposure-window-v1", eventID, k.String(), b.String(),
		strconv.FormatInt(startUnixNanos, 10))
}

// identityDigest is the first 32 hex digits of the SHA-256 of the
// length-prefixed parts, so no two part lists share an encoding.
func identityDigest(parts ...string) string {
	h := sha256.New()
	var n [8]byte
	for _, p := range parts {
		binary.BigEndian.PutUint64(n[:], uint64(len(p)))
		h.Write(n[:])
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// FollowingInteractions maps every encounter of an analysis to its stored
// form, in the analysis's order.
func FollowingInteractions(sourceID string, a FollowingAnalysis) ([]FollowingInteraction, error) {
	out := make([]FollowingInteraction, 0, len(a.Encounters))
	for _, e := range a.Encounters {
		fi, err := NewFollowingInteraction(sourceID, e)
		if err != nil {
			return nil, fmt.Errorf("encounter %s -> %s: %w", e.LeaderTrackID, e.FollowerTrackID, err)
		}
		out = append(out, fi)
	}
	return out, nil
}

// NewFollowingInteraction maps one encounter to its stored form and validates
// the result. Nothing is recomputed: every value is the encounter's own.
func NewFollowingInteraction(sourceID string, e Encounter) (FollowingInteraction, error) {
	if sourceID == "" {
		return FollowingInteraction{}, fmt.Errorf("interaction requires a source id")
	}
	if len(e.Measurements) == 0 || len(e.Instants) == 0 {
		return FollowingInteraction{}, fmt.Errorf("encounter has no measurements or instants")
	}
	prov := e.Measurements[0].Provenance
	if prov.Version.MethodID != e.MethodID || prov.Version.GeometryID != e.GeometryID || prov.Version.EstimateStage != e.Stage {
		return FollowingInteraction{}, fmt.Errorf("encounter method, geometry or stage disagrees with its measurements' provenance")
	}
	measurements, err := keyMeasurements(e.Measurements)
	if err != nil {
		return FollowingInteraction{}, err
	}
	provisional, err := keyMeasurements(e.Provisional)
	if err != nil {
		return FollowingInteraction{}, err
	}
	ev := InteractionEvent{
		Schema: InteractionRecordSchema, SourceID: sourceID, Type: InteractionFollowing,
		PrimaryTrackID: e.FollowerTrackID, SecondaryTrackID: e.LeaderTrackID,
		StartUnixNanos: e.FirstUnixNanos, EndUnixNanos: e.LastUnixNanos,
		Version: prov.Version, Input: prov.Input, WorstSupport: e.WorstSupport,
		Measurements: measurements, Provisional: provisional,
		Accounting: keyAccounting(e.Accounting, len(e.Instants)),
	}
	ev.EventID = InteractionEventID(ev.SourceID, ev.Type, ev.PrimaryTrackID, ev.SecondaryTrackID, ev.Version)

	gaps, thws := seriesAt(e.SpatialGapSeries), seriesAt(e.NetTimeGapSeries)
	predicted := make(map[int64]PredictedPoint, len(e.PredictedGapSeries))
	for _, p := range e.PredictedGapSeries {
		predicted[p.CaptureUnixNanos] = p
	}
	instants := make([]InteractionInstant, 0, len(e.Instants))
	for _, inst := range e.Instants {
		r := InteractionInstant{
			EventID: ev.EventID, CaptureUnixNanos: inst.CaptureUnixNanos, IntervalNanos: inst.IntervalNanos,
			RecordGap: inst.RecordGap, Basis: basisOf(inst.FollowerSupport, inst.LeaderSupport), Role: inst.Role,
			FollowerSupport: inst.FollowerSupport, LeaderSupport: inst.LeaderSupport,
			Valid: inst.Valid, Reason: inst.Reason, Condition: inst.Condition,
		}
		if inst.Unobserved != (r.Basis == BasisPredictedOnly) {
			return FollowingInteraction{}, fmt.Errorf("instant %d: unobserved %v disagrees with supports %s and %s",
				inst.CaptureUnixNanos, inst.Unobserved, inst.FollowerSupport, inst.LeaderSupport)
		}
		if pt := inst.Point; pt != nil {
			r.Reasons = append([]SuppressionReason(nil), pt.Reasons...)
			if pt.Leader != nil {
				r.LeaderTrailing = ptr(pt.Leader.Trailing)
			}
			if pt.Follower != nil {
				r.FollowerLeading = ptr(pt.Follower.Leading)
			}
			if r.Basis == BasisPredictedOnly {
				r.CoastAgeNanos = pt.CoastAgeNanos
			}
		}
		values := map[MetricID]SeriesValue{}
		if v, ok := gaps[inst.CaptureUnixNanos]; ok {
			values[MetricFollowingSpatialGap] = v
		}
		if v, ok := thws[inst.CaptureUnixNanos]; ok {
			values[MetricFollowingNetTimeGap] = v
		}
		if p, ok := predicted[inst.CaptureUnixNanos]; ok {
			values[MetricFollowingPredictedGap] = SeriesValue{Value: p.ValueM, Sigma: p.SigmaM}
			if p.CoastAgeNanos != r.CoastAgeNanos {
				return FollowingInteraction{}, fmt.Errorf("instant %d: predicted gap coast age disagrees with its point", inst.CaptureUnixNanos)
			}
		}
		if len(values) > 0 {
			r.Values = values
		}
		instants = append(instants, r)
	}
	fi := FollowingInteraction{Event: ev, Instants: instants, Windows: deriveWindows(ev, instants)}
	if err := fi.Validate(); err != nil {
		return FollowingInteraction{}, err
	}
	return fi, nil
}

func keyMeasurements(ms []Measurement) (map[MetricID]Measurement, error) {
	if len(ms) == 0 {
		return nil, nil
	}
	out := make(map[MetricID]Measurement, len(ms))
	for _, m := range ms {
		if _, dup := out[m.Name]; dup {
			return nil, fmt.Errorf("measurement %s appears twice", m.Name)
		}
		out[m.Name] = m
	}
	return out, nil
}

func keyAccounting(a EncounterAccounting, instants int) InteractionAccounting {
	out := InteractionAccounting{
		Instants: instants, ValidNanos: a.ValidNanos, BandNanos: map[MetricID]int64{},
		PredictedOnlyNanos: a.UnobservedNanos, RecordGapNanos: a.RecordGapNanos,
	}
	for b, band := range FollowingBands() {
		if b < len(a.BandNanos) {
			out.BandNanos[band.Duration] = a.BandNanos[b]
		}
	}
	for _, t := range a.Suppressions {
		if out.Suppressions == nil {
			out.Suppressions = map[SuppressionReason]SuppressionCount{}
		}
		out.Suppressions[t.Reason] = SuppressionCount{Instants: t.Instants, Nanos: t.Nanos}
	}
	return out
}

func seriesAt(points []SeriesPoint) map[int64]SeriesValue {
	out := make(map[int64]SeriesValue, len(points))
	for _, p := range points {
		out[p.CaptureUnixNanos] = SeriesValue{Value: p.Value, Sigma: p.Sigma}
	}
	return out
}

// deriveWindows cuts an encounter's instants into maximal runs of valid time
// and of predicted-only time. A run continues while each instant's interval
// ends exactly at the next instant's capture; a record gap stands for nothing
// and ends any run, and observed time that is not valid belongs to no window.
func deriveWindows(ev InteractionEvent, instants []InteractionInstant) []ExposureWindow {
	var out []ExposureWindow
	var open *ExposureWindow
	flush := func() {
		if open != nil && open.DurationNanos > 0 {
			open.WindowID = ExposureWindowID(open.EventID, open.Kind, open.Basis, open.StartUnixNanos)
			out = append(out, *open)
		}
		open = nil
	}
	for _, in := range instants {
		basis := BasisUnspecified
		switch {
		case in.RecordGap:
		case in.Valid:
			basis = BasisObserved
		case in.Basis == BasisPredictedOnly:
			basis = BasisPredictedOnly
		}
		if open != nil && (basis != open.Basis || open.EndUnixNanos != in.CaptureUnixNanos) {
			flush()
		}
		if basis == BasisUnspecified {
			continue
		}
		if open == nil {
			open = &ExposureWindow{
				SourceID: ev.SourceID, EventID: ev.EventID, Kind: ExposureValidFollowing, Basis: basis,
				TrackID: ev.PrimaryTrackID, CounterpartTrackID: ev.SecondaryTrackID,
				StartUnixNanos: in.CaptureUnixNanos, EndUnixNanos: in.CaptureUnixNanos, Version: ev.Version,
			}
			if basis == BasisObserved {
				open.BandNanos = map[MetricID]int64{}
				for _, band := range FollowingBands() {
					open.BandNanos[band.Duration] = 0
				}
			}
		}
		open.EndUnixNanos += in.IntervalNanos
		open.DurationNanos += in.IntervalNanos
		if basis == BasisObserved {
			thw := in.Values[MetricFollowingNetTimeGap]
			for _, band := range FollowingBands() {
				if band.Contains(thw.Value) {
					open.BandNanos[band.Duration] += in.IntervalNanos
				}
			}
		}
	}
	flush()
	return out
}

// Validate checks the record set against itself: each record, the instants'
// order and span, the accounting they add up to, and the windows they imply.
func (fi FollowingInteraction) Validate() error {
	ev := fi.Event
	if err := ev.Validate(); err != nil {
		return err
	}
	if len(fi.Instants) == 0 || len(fi.Instants) != ev.Accounting.Instants {
		return fmt.Errorf("event %s: %d instants stored, %d accounted", ev.EventID, len(fi.Instants), ev.Accounting.Instants)
	}
	sum := InteractionAccounting{Instants: len(fi.Instants), BandNanos: map[MetricID]int64{}}
	for _, band := range FollowingBands() {
		sum.BandNanos[band.Duration] = 0
	}
	for i, in := range fi.Instants {
		if err := in.validate(ev); err != nil {
			return fmt.Errorf("event %s instant %d: %w", ev.EventID, in.CaptureUnixNanos, err)
		}
		if i > 0 && in.CaptureUnixNanos <= fi.Instants[i-1].CaptureUnixNanos {
			return fmt.Errorf("event %s: instants are not in strictly increasing capture order", ev.EventID)
		}
		// The same accounting rules as buildEncounter, from the stored rows.
		counted := in.IntervalNanos
		if in.RecordGap {
			sum.RecordGapNanos += in.IntervalNanos
			counted = 0
		}
		if in.Basis == BasisPredictedOnly {
			sum.PredictedOnlyNanos += counted
		}
		if in.Valid {
			sum.ValidNanos += counted
			for _, band := range FollowingBands() {
				if band.Contains(in.Values[MetricFollowingNetTimeGap].Value) {
					sum.BandNanos[band.Duration] += counted
				}
			}
			continue
		}
		if sum.Suppressions == nil {
			sum.Suppressions = map[SuppressionReason]SuppressionCount{}
		}
		c := sum.Suppressions[in.Reason]
		c.Instants++
		c.Nanos += counted
		sum.Suppressions[in.Reason] = c
	}
	if first, last := fi.Instants[0].CaptureUnixNanos, fi.Instants[len(fi.Instants)-1].CaptureUnixNanos; first != ev.StartUnixNanos || last != ev.EndUnixNanos {
		return fmt.Errorf("event %s: instants span %d to %d, event %d to %d", ev.EventID, first, last, ev.StartUnixNanos, ev.EndUnixNanos)
	}
	if !reflect.DeepEqual(sum, ev.Accounting) {
		return fmt.Errorf("event %s: accounting %+v disagrees with its instants' %+v", ev.EventID, ev.Accounting, sum)
	}
	want := deriveWindows(ev, fi.Instants)
	if len(want) != len(fi.Windows) {
		return fmt.Errorf("event %s: %d windows stored, %d implied by its instants", ev.EventID, len(fi.Windows), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(want[i], fi.Windows[i]) {
			return fmt.Errorf("event %s: window %s is not the one its instants imply", ev.EventID, fi.Windows[i].WindowID)
		}
	}
	for _, w := range fi.Windows {
		if err := w.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate checks one event on its own: identity, version, provenance, the
// complete measurement set and the stage labels.
func (ev InteractionEvent) Validate() error {
	if ev.Schema != InteractionRecordSchema {
		return fmt.Errorf("interaction schema %q, want %q", ev.Schema, InteractionRecordSchema)
	}
	if ev.SourceID == "" || !ev.Type.Valid() || ev.PrimaryTrackID == "" || ev.SecondaryTrackID == "" ||
		ev.PrimaryTrackID == ev.SecondaryTrackID {
		return fmt.Errorf("interaction requires a source, a type and two distinct tracks")
	}
	if ev.StartUnixNanos <= 0 || ev.StartUnixNanos > ev.EndUnixNanos {
		return fmt.Errorf("interaction interval %d to %d is not ordered", ev.StartUnixNanos, ev.EndUnixNanos)
	}
	if err := ev.Version.Validate(); err != nil {
		return err
	}
	if ev.Version.GeometryID == "" {
		return fmt.Errorf("a following interaction records the path it was measured along")
	}
	if want := InteractionEventID(ev.SourceID, ev.Type, ev.PrimaryTrackID, ev.SecondaryTrackID, ev.Version); ev.EventID != want {
		return fmt.Errorf("interaction id %q is not its identity %q", ev.EventID, want)
	}
	if err := ev.Input.Validate(); err != nil {
		return err
	}
	ids := append([]string(nil), ev.Input.ContributingTrackIDs...)
	sort.Strings(ids)
	pair := []string{ev.PrimaryTrackID, ev.SecondaryTrackID}
	sort.Strings(pair)
	if !reflect.DeepEqual(ids, pair) {
		return fmt.Errorf("interaction contributing tracks %v are not its pair", ev.Input.ContributingTrackIDs)
	}
	if ev.Input.FirstUnixNanos != ev.StartUnixNanos || ev.Input.LastUnixNanos != ev.EndUnixNanos {
		return fmt.Errorf("interaction input interval disagrees with the event's")
	}
	if !ev.WorstSupport.Valid() {
		return fmt.Errorf("interaction worst support is %s", ev.WorstSupport)
	}
	final := ev.Version.EstimateStage == StageFinal
	if err := ev.validateMeasurements(ev.Measurements, "measurement", func(m Measurement) error {
		// A non-final encounter publishes nothing; a final one never carries
		// the stage guard.
		if !final && !m.Suppressed {
			return fmt.Errorf("carries a value on a %s encounter", ev.Version.EstimateStage)
		}
		if final && m.Reason == ReasonEstimateNotFinal {
			return fmt.Errorf("is suppressed as not final on a final encounter")
		}
		return nil
	}); err != nil {
		return err
	}
	if final != (len(ev.Provisional) == 0) {
		return fmt.Errorf("interaction at stage %s has %d provisional measurements", ev.Version.EstimateStage, len(ev.Provisional))
	}
	if !final {
		if err := ev.validateMeasurements(ev.Provisional, "provisional measurement", func(m Measurement) error {
			if m.Reason == ReasonEstimateNotFinal {
				return fmt.Errorf("is suppressed by the stage guard it exists to bypass")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return ev.Accounting.validate()
}

func (ev InteractionEvent) validateMeasurements(ms map[MetricID]Measurement, what string, extra func(Measurement) error) error {
	want := EncounterMetrics()
	if len(ms) != len(want) {
		return fmt.Errorf("interaction has %d %ss, want %d", len(ms), what, len(want))
	}
	prov := Provenance{Version: ev.Version, Input: ev.Input}
	for _, id := range want {
		m, ok := ms[id]
		if !ok {
			return fmt.Errorf("interaction has no %s %s", what, id)
		}
		if m.Name != id {
			return fmt.Errorf("%s keyed %s is named %s", what, id, m.Name)
		}
		if err := m.Validate(); err != nil {
			return err
		}
		if !reflect.DeepEqual(m.Provenance, prov) {
			return fmt.Errorf("%s %s provenance disagrees with its interaction's", what, id)
		}
		if err := extra(m); err != nil {
			return fmt.Errorf("%s %s %w", what, id, err)
		}
	}
	return nil
}

func (a InteractionAccounting) validate() error {
	if a.Instants <= 0 || a.ValidNanos < 0 || a.PredictedOnlyNanos < 0 || a.RecordGapNanos < 0 {
		return fmt.Errorf("interaction accounting must count instants and no negative time")
	}
	bands := FollowingBands()
	if len(a.BandNanos) != len(bands) {
		return fmt.Errorf("interaction accounting has %d bands, want %d", len(a.BandNanos), len(bands))
	}
	for _, band := range bands {
		n, ok := a.BandNanos[band.Duration]
		if !ok || n < 0 || n > a.ValidNanos {
			return fmt.Errorf("interaction accounting band %s is missing or outside valid time", band.Duration)
		}
	}
	for r, c := range a.Suppressions {
		if !r.Valid() || c.Instants <= 0 || c.Nanos < 0 {
			return fmt.Errorf("interaction accounting suppression %s must count instants and no negative time", r)
		}
	}
	return nil
}

// instantValues are the metrics an instant may carry on each basis.
var instantValues = map[ObservationBasis]map[MetricID]bool{
	BasisObserved:      {MetricFollowingSpatialGap: true, MetricFollowingNetTimeGap: true},
	BasisPredictedOnly: {MetricFollowingPredictedGap: true},
}

func (in InteractionInstant) validate(ev InteractionEvent) error {
	if in.EventID != ev.EventID {
		return fmt.Errorf("belongs to event %q", in.EventID)
	}
	if in.CaptureUnixNanos <= 0 || in.IntervalNanos < 0 || (in.RecordGap && in.IntervalNanos == 0) {
		return fmt.Errorf("capture time or interval out of range")
	}
	if !in.FollowerSupport.Valid() || !in.LeaderSupport.Valid() || !in.Role.Valid() {
		return fmt.Errorf("requires both parties' support and the leader's role")
	}
	if in.Basis != basisOf(in.FollowerSupport, in.LeaderSupport) {
		return fmt.Errorf("basis %s disagrees with supports %s and %s", in.Basis, in.FollowerSupport, in.LeaderSupport)
	}
	if in.Condition != PathConditionUnspecified && !in.Condition.Valid() {
		return fmt.Errorf("condition is %s", in.Condition)
	}
	switch {
	case in.Valid && in.Basis != BasisObserved:
		return fmt.Errorf("is valid on basis %s", in.Basis)
	case in.Valid && in.Reason != ReasonUnspecified:
		return fmt.Errorf("is valid and suppressed as %s", in.Reason)
	case !in.Valid && !in.Reason.Valid():
		return fmt.Errorf("is not valid and states no reason")
	}
	for i, r := range in.Reasons {
		if !r.Valid() || (i > 0 && r <= in.Reasons[i-1]) {
			return fmt.Errorf("reasons %v are not registered reasons in precedence order", in.Reasons)
		}
	}
	for _, e := range []struct {
		ep      *Endpoint
		want    PathExtremity
		trackID string
		support SupportState
	}{
		{in.LeaderTrailing, ExtremityTrailing, ev.SecondaryTrackID, in.LeaderSupport},
		{in.FollowerLeading, ExtremityLeading, ev.PrimaryTrackID, in.FollowerSupport},
	} {
		if e.ep == nil {
			continue
		}
		if err := e.ep.validate(e.want); err != nil {
			return err
		}
		if e.ep.TrackID != e.trackID || e.ep.CaptureUnixNanos != in.CaptureUnixNanos || e.ep.Support != e.support {
			return fmt.Errorf("%s endpoint belongs to %s at %d with support %s", e.want, e.ep.TrackID, e.ep.CaptureUnixNanos, e.ep.Support)
		}
	}
	allowed := instantValues[in.Basis]
	for id, v := range in.Values {
		if !allowed[id] {
			return fmt.Errorf("carries %s on basis %s", id, in.Basis)
		}
		if !finite(v.Value) || !finiteNonNegative(v.Sigma) {
			return fmt.Errorf("%s must be finite with a non-negative sigma", id)
		}
		if in.LeaderTrailing == nil || in.FollowerLeading == nil {
			return fmt.Errorf("carries %s without the endpoints it was measured between", id)
		}
	}
	if _, thw := in.Values[MetricFollowingNetTimeGap]; thw != in.Valid {
		return fmt.Errorf("net time gap present %v on an instant valid %v", thw, in.Valid)
	}
	if _, gap := in.Values[MetricFollowingSpatialGap]; in.Valid && !gap {
		return fmt.Errorf("is valid without a spatial gap")
	}
	if in.CoastAgeNanos < 0 || (in.Basis == BasisObserved && in.CoastAgeNanos != 0) {
		return fmt.Errorf("coast age %d on basis %s", in.CoastAgeNanos, in.Basis)
	}
	return nil
}

// Validate checks one window on its own, as a reader of denominators sees it.
func (w ExposureWindow) Validate() error {
	if w.SourceID == "" || w.EventID == "" || !w.Kind.Valid() || !w.Basis.Valid() {
		return fmt.Errorf("exposure window requires a source, an event, a kind and a basis")
	}
	if w.TrackID == "" || w.CounterpartTrackID == "" || w.TrackID == w.CounterpartTrackID {
		return fmt.Errorf("exposure window %s requires two distinct tracks", w.WindowID)
	}
	if want := ExposureWindowID(w.EventID, w.Kind, w.Basis, w.StartUnixNanos); w.WindowID != want {
		return fmt.Errorf("exposure window id %q is not its identity %q", w.WindowID, want)
	}
	if w.StartUnixNanos <= 0 || w.DurationNanos <= 0 || w.EndUnixNanos-w.StartUnixNanos != w.DurationNanos {
		return fmt.Errorf("exposure window %s must span a positive duration from start to end", w.WindowID)
	}
	if err := w.Version.Validate(); err != nil {
		return err
	}
	if !w.Basis.CountsTowardExposure() {
		if len(w.BandNanos) != 0 {
			return fmt.Errorf("exposure window %s is %s and carries band time", w.WindowID, w.Basis)
		}
		return nil
	}
	bands := FollowingBands()
	if len(w.BandNanos) != len(bands) {
		return fmt.Errorf("exposure window %s has %d bands, want %d", w.WindowID, len(w.BandNanos), len(bands))
	}
	for _, band := range bands {
		if n, ok := w.BandNanos[band.Duration]; !ok || n < 0 || n > w.DurationNanos {
			return fmt.Errorf("exposure window %s band %s is missing or outside its duration", w.WindowID, band.Duration)
		}
	}
	return nil
}

// --- Reading back ----------------------------------------------------------

// OrderedMeasurements returns the production measurements in
// EncounterMetrics() order, as Encounter.Measurements holds them.
func (ev InteractionEvent) OrderedMeasurements() []Measurement {
	return orderedMeasurements(ev.Measurements)
}

// OrderedProvisional returns the review-only measurements in the same order;
// nil for a final encounter.
func (ev InteractionEvent) OrderedProvisional() []Measurement {
	return orderedMeasurements(ev.Provisional)
}

func orderedMeasurements(ms map[MetricID]Measurement) []Measurement {
	if len(ms) == 0 {
		return nil
	}
	var out []Measurement
	for _, id := range EncounterMetrics() {
		if m, ok := ms[id]; ok {
			out = append(out, m)
		}
	}
	return out
}

// EncounterAccounting returns the accounting in Encounter's shape: bands in
// FollowingBands() order and suppressions in precedence order.
func (ev InteractionEvent) EncounterAccounting() EncounterAccounting {
	a := ev.Accounting
	out := EncounterAccounting{
		ValidNanos: a.ValidNanos, UnobservedNanos: a.PredictedOnlyNanos, RecordGapNanos: a.RecordGapNanos,
	}
	for _, band := range FollowingBands() {
		out.BandNanos = append(out.BandNanos, a.BandNanos[band.Duration])
	}
	for _, r := range SuppressionReasons() {
		if c, ok := a.Suppressions[r]; ok {
			out.Suppressions = append(out.Suppressions, ReasonTally{Reason: r, Instants: c.Instants, Nanos: c.Nanos})
		}
	}
	return out
}

// SpatialGapSeries returns the supported spatial gap series in capture order.
func (fi FollowingInteraction) SpatialGapSeries() []SeriesPoint {
	return fi.series(MetricFollowingSpatialGap)
}

// NetTimeGapSeries returns the valid net time gap series in capture order.
func (fi FollowingInteraction) NetTimeGapSeries() []SeriesPoint {
	return fi.series(MetricFollowingNetTimeGap)
}

func (fi FollowingInteraction) series(id MetricID) []SeriesPoint {
	var out []SeriesPoint
	for _, in := range fi.Instants {
		if v, ok := in.Values[id]; ok {
			out = append(out, SeriesPoint{CaptureUnixNanos: in.CaptureUnixNanos, Value: v.Value, Sigma: v.Sigma})
		}
	}
	return out
}

// PredictedGapSeries returns the review-only predicted gap series in capture
// order, with coast age.
func (fi FollowingInteraction) PredictedGapSeries() []PredictedPoint {
	var out []PredictedPoint
	for _, in := range fi.Instants {
		if v, ok := in.Values[MetricFollowingPredictedGap]; ok {
			out = append(out, PredictedPoint{
				CaptureUnixNanos: in.CaptureUnixNanos, ValueM: v.Value, SigmaM: v.Sigma, CoastAgeNanos: in.CoastAgeNanos,
			})
		}
	}
	return out
}

// OpportunityNanos is the time a set of windows contributes to a denominator:
// the observed windows' duration and nothing else, whatever the caller passed.
func OpportunityNanos(windows []ExposureWindow) int64 {
	var total int64
	for _, w := range windows {
		if w.Basis.CountsTowardExposure() {
			total += w.DurationNanos
		}
	}
	return total
}
