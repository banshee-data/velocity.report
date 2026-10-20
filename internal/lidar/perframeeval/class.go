package perframeeval

import (
	"fmt"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// The class measure: for every reviewed road user, the class the following
// gate would read from the tracks the scored matching put on it, at each
// reviewed frame and at the track's end, against the object's label.
//
// What a track's class is. The classifier's decision is persisted only on
// lidar_track_solid_bodies rows (motion_class, motion_posterior), one per
// associated frame, written when a replay runs the solid body. A point
// estimate row carries no class, and the persisted-estimate adapter reads
// every such track as unknown. So the measure reads the online solid-body
// version filed beside the arm's rows (LoadClassRows), joined to the arm's
// tracks by creation sequence and frame. A replay without the
// solid body has no class to read, and the measure refuses it rather than
// reporting every vehicle unclassified.
//
// What rigid means. Exactly what the following gate admits: the row's belief
// through l8behaviour.PassageFromL5, the class a trajectory's passage
// carries, then l8behaviour.ClassApplicability for the following metrics with
// a rigid counterpart (see followingAdmits). The class at a track's end is its
// latest row's, as l8behaviour.TrajectoriesFromSolidBodies takes it.
//
// Which track counts. Per frame, the track the per-frame matching (the one
// the MOT numbers tally, l8analytics.MatchPerFrame) put on the object at that
// frame, and its class at that frame. Per object, the end track is the track
// matched at the object's last matched reviewed frame, and the object ends
// rigid when that track's latest row is rigid. A fragment that ended before
// the object did, or a duplicate the matching did not choose, counts only
// through any_track_ends_rigid and the per-track list.

// ClassUnmatched is the end class of an object no track was matched to.
const ClassUnmatched = "unmatched"

// ClassUnclassed is the class of a matched track with no class row at the
// frame (per frame) or at all (at its end).
const ClassUnclassed = "unclassed"

// ClassRow is one track's classifier decision at one frame.
type ClassRow struct {
	FrameUnixNanos int64
	Belief         l5tracks.MotionClassBelief
}

// ClassSource is the solid-body version a class measure read.
type ClassSource struct {
	Table              string `json:"table"`
	SourceID           string `json:"source_id"`
	EstimatorID        string `json:"estimator_id"`
	ObservationModelID string `json:"observation_model_id"`
	ParamHash          string `json:"param_hash"`
	Stage              string `json:"stage"`
	Rows               int    `json:"rows"`
	Tracks             int    `json:"tracks"`
}

// ClassRows is one arm's class rows, keyed as the arm's hypotheses are
// (seq-NNNNNN from creation_sequence), each track's rows in frame order.
type ClassRows struct {
	Source ClassSource
	Tracks map[string][]ClassRow
}

// ClassDefinition states the rules the numbers were computed under, so a
// reader of the JSON need not read this file.
type ClassDefinition struct {
	Rigid       string `json:"rigid"`
	Attribution string `json:"attribution"`
	Age         string `json:"age"`
	// LabelMotionClasses is the class each reviewed label is expected to
	// end with, for every label scored.
	LabelMotionClasses map[string]string `json:"label_motion_classes"`
}

// AgeStats summarises how long a class took to arrive.
type AgeStats struct {
	// N reached it; Never did not.
	N     int `json:"n"`
	Never int `json:"never"`
	// Median and P90 are linear-interpolation quantiles over N, in seconds;
	// null when N is zero.
	Median *float64 `json:"median_s"`
	P90    *float64 `json:"p90_s"`
}

// VehicleClassSummary is the reading's headline over reviewed vehicles.
type VehicleClassSummary struct {
	Objects        int `json:"objects"`
	MatchedObjects int `json:"matched_objects"`
	// EndRigid objects' end tracks end rigid; the share is over Objects, so
	// an unmatched vehicle counts against it.
	EndRigid      int     `json:"end_rigid"`
	EndRigidShare float64 `json:"end_rigid_share"`
	// AnyTrackEndsRigid counts objects any of whose matched tracks ends rigid.
	AnyTrackEndsRigid      int     `json:"any_track_ends_rigid"`
	AnyTrackEndsRigidShare float64 `json:"any_track_ends_rigid_share"`
	// Frames are reviewed (scored) vehicle frames; RigidFrames those whose
	// matched track was rigid at that frame.
	Frames                 int     `json:"frames"`
	MatchedFrames          int     `json:"matched_frames"`
	RigidFrames            int     `json:"rigid_frames"`
	RigidFrameShare        float64 `json:"rigid_frame_share"`
	RigidMatchedFrameShare float64 `json:"rigid_matched_frame_share"`
	// TrackAgeAtFirstRigid is over matched vehicles' end tracks: the time
	// from the track's first class row to its first rigid row.
	TrackAgeAtFirstRigid AgeStats `json:"track_age_at_first_rigid"`
	// ObjectTimeToFirstRigid is over matched vehicles: the time from the
	// object's first reviewed frame to the first reviewed frame at which its
	// matched track was rigid.
	ObjectTimeToFirstRigid AgeStats `json:"object_time_to_first_rigid"`
}

// FalseApplicability is reviewed pedestrians and two-wheelers whose tracks
// the following gate would admit.
type FalseApplicability struct {
	Objects         int     `json:"objects"`
	MatchedObjects  int     `json:"matched_objects"`
	EndRigid        int     `json:"end_rigid"`
	EndRigidShare   float64 `json:"end_rigid_share"`
	Frames          int     `json:"frames"`
	RigidFrames     int     `json:"rigid_frames"`
	RigidFrameShare float64 `json:"rigid_frame_share"`
}

// LabelClassSummary is recall for one reviewed label: the share of its
// objects whose end track ends in the expected class, and of its reviewed
// frames whose matched track has it at that frame.
type LabelClassSummary struct {
	Label               string  `json:"label"`
	ExpectedMotionClass string  `json:"expected_motion_class"`
	Objects             int     `json:"objects"`
	MatchedObjects      int     `json:"matched_objects"`
	EndExpected         int     `json:"end_expected"`
	EndRecall           float64 `json:"end_recall"`
	Frames              int     `json:"frames"`
	MatchedFrames       int     `json:"matched_frames"`
	ExpectedFrames      int     `json:"expected_frames"`
	FrameRecall         float64 `json:"frame_recall"`
	// EndClasses counts objects by their end class: a motion class,
	// unclassed, or unmatched.
	EndClasses map[string]int `json:"end_classes"`
	// FrameClasses counts reviewed frames by the matched track's class at
	// the frame: a motion class, unclassed, or unmatched.
	FrameClasses map[string]int `json:"frame_classes"`
}

// ClassTrack is one track matched to an object.
type ClassTrack struct {
	Track         string `json:"track"`
	MatchedFrames int    `json:"matched_frames"`
	FirstMatchNs  int64  `json:"first_match_ns"`
	LastMatchNs   int64  `json:"last_match_ns"`
	ClassRows     int    `json:"class_rows"`
	// EndClass is the class at the track's latest row, over its whole life.
	EndClass     string   `json:"end_class"`
	EndPosterior *float64 `json:"end_posterior,omitempty"`
	EndsRigid    bool     `json:"ends_rigid"`
	// FirstRigidAgeS and FirstRigidAgeRows are how long after its first
	// class row, and how many rows in, the track was first rigid; absent
	// when it never was.
	FirstRigidAgeS    *float64 `json:"first_rigid_age_s,omitempty"`
	FirstRigidAgeRows *int     `json:"first_rigid_age_rows,omitempty"`
}

// ClassObject is one reviewed object's class evidence in one episode.
type ClassObject struct {
	EpisodeID           string `json:"episode_id"`
	ObjectID            string `json:"object_id"`
	Label               string `json:"label"`
	ExpectedMotionClass string `json:"expected_motion_class"`
	Frames              int    `json:"frames"`
	MatchedFrames       int    `json:"matched_frames"`
	RigidFrames         int    `json:"rigid_frames"`
	ExpectedFrames      int    `json:"expected_frames"`
	UnclassedFrames     int    `json:"unclassed_frames"`
	// Tracks are the distinct matched tracks in order of first match.
	Tracks            []ClassTrack `json:"tracks"`
	EndTrack          string       `json:"end_track,omitempty"`
	EndClass          string       `json:"end_class"`
	EndsRigid         bool         `json:"ends_rigid"`
	AnyTrackEndsRigid bool         `json:"any_track_ends_rigid"`
	// TimeToFirstRigidS is from the object's first reviewed frame to the
	// first reviewed frame its matched track was rigid; absent when never.
	TimeToFirstRigidS *float64 `json:"time_to_first_rigid_s,omitempty"`
	// frameClasses counts this object's frames by class, for the label sums.
	frameClasses map[string]int
}

// ClassSummary is the measure's summaries over a set of objects.
type ClassSummary struct {
	Vehicles           VehicleClassSummary `json:"vehicles"`
	FalseApplicability FalseApplicability  `json:"false_applicability"`
	ByLabel            []LabelClassSummary `json:"by_label"`
}

// ClassEpisode is the summaries over one episode's objects.
type ClassEpisode struct {
	EpisodeID string `json:"episode_id"`
	ClassSummary
}

// ClassResult is one arm's class measure. The top-level summaries pool the
// episodes as the MOT totals do: an object scored in two overlapping
// episodes counts once in each, so a per-object reading takes one episode's
// summary, or a run restricted to one episode with -episodes.
type ClassResult struct {
	Arm         ArmIdentity `json:"arm"`
	ClassSource ClassSource `json:"class_source"`
	ClassSummary
	Episodes []ClassEpisode `json:"episodes"`
	Objects  []ClassObject  `json:"objects"`
}

// ClassArms is both arms' class measures, beside the comparison.
type ClassArms struct {
	Definition ClassDefinition `json:"definition"`
	A          ClassResult     `json:"a"`
	B          ClassResult     `json:"b"`
}

// followingAdmits is the following gate's class rule applied to one row's
// belief: the passage class l8behaviour.PassageFromL5 builds from it (the
// belief's effective class), admitted as the subject of the following
// metrics against a rigid counterpart by l8behaviour.ClassApplicability,
// exactly as AnalyseFollowing asks it (following.go). Nothing here restates
// which classes are rigid.
func followingAdmits(belief l5tracks.MotionClassBelief) bool {
	class := passageClass(belief)
	reason, err := l8behaviour.ClassApplicability(l8behaviour.MetricFollowingSpatialGap, class, l8behaviour.MotionRigidVehicle)
	return err == nil && reason == l8behaviour.ReasonUnspecified
}

// passageClass is the class a passage built from this belief carries.
func passageClass(belief l5tracks.MotionClassBelief) l8behaviour.MotionClass {
	return l8behaviour.PassageFromL5("", "", "", belief, "").MotionClass
}

// labelMotionClass is the motion class a reviewed label should end with:
// the classifier's own label mapping (l5tracks.MotionClassForLabel), plus the
// two annotation labels the classifier does not emit.
func labelMotionClass(label string) l8behaviour.MotionClass {
	switch l := strings.ToLower(strings.TrimSpace(label)); l {
	case "van":
		return l8behaviour.MotionRigidVehicle
	case "motorcycle":
		return l8behaviour.MotionTwoWheeler
	default:
		return l8behaviour.MotionClassFromL5(l5tracks.MotionClassForLabel(l))
	}
}

// LoadClassRows reads the class rows filed beside an arm: the online
// solid-body version of the arm's source and estimator (the one with the
// arm's parameter hash when there are several). The classifier decides
// online, and the solid body exists at the online stage only in a replay
// that does not refine it, so a refined arm reads the decisions its online
// rows carried, at the same frames.
func LoadClassRows(spec ArmSpec, arm ArmIdentity) (ClassRows, error) {
	if arm.Kind != ArmEstimates {
		return ClassRows{}, fmt.Errorf("arm %s: the class measure needs an estimate version; an analysis run's tracks are not keyed by creation sequence", arm.Label)
	}
	database, err := sqlite.OpenReadOnly(spec.DBPath)
	if err != nil {
		return ClassRows{}, fmt.Errorf("arm %s: open %s: %w", arm.Label, spec.DBPath, err)
	}
	defer database.Close()
	store := sqlite.NewStateEstimateStore(database)
	versions, err := store.ListSolidBodyVersions()
	if err != nil {
		return ClassRows{}, fmt.Errorf("arm %s: %w", arm.Label, err)
	}
	version, err := selectClassVersion(versions, arm)
	if err != nil {
		return ClassRows{}, err
	}
	rows, err := store.ListSolidBodyClasses(version)
	if err != nil {
		return ClassRows{}, fmt.Errorf("arm %s: %w", arm.Label, err)
	}
	out := ClassRows{
		Source: ClassSource{
			Table: solidBodyTable, SourceID: version.SourceID, EstimatorID: version.EstimatorID,
			ObservationModelID: version.ObservationModelID, ParamHash: version.ParamHash, Stage: version.Stage,
		},
		Tracks: map[string][]ClassRow{},
	}
	trackOf := map[int64]string{}
	for _, r := range rows {
		if first, seen := trackOf[r.CreationSequence]; seen && first != r.TrackID {
			return ClassRows{}, fmt.Errorf("arm %s: creation_sequence %d names two tracks (%s and %s) in %s; this source holds more than one tracker run",
				arm.Label, r.CreationSequence, first, r.TrackID, solidBodyTable)
		}
		trackOf[r.CreationSequence] = r.TrackID
		key := fmt.Sprintf("seq-%06d", r.CreationSequence)
		out.Tracks[key] = append(out.Tracks[key], ClassRow{FrameUnixNanos: r.FrameUnixNanos, Belief: r.Motion})
	}
	out.Source.Rows, out.Source.Tracks = len(rows), len(out.Tracks)
	return out, nil
}

func selectClassVersion(versions []sqlite.EstimateVersion, arm ArmIdentity) (sqlite.EstimateVersion, error) {
	var matched []sqlite.EstimateVersion
	for _, v := range versions {
		if v.SourceID == arm.SourceID && v.EstimatorID == arm.EstimatorID && v.Stage == StageOnline {
			matched = append(matched, v)
		}
	}
	if len(matched) > 1 {
		var same []sqlite.EstimateVersion
		for _, v := range matched {
			if v.ParamHash == arm.ParamHash {
				same = append(same, v)
			}
		}
		matched = same
	}
	switch len(matched) {
	case 1:
		return matched[0], nil
	case 0:
		return sqlite.EstimateVersion{}, fmt.Errorf("arm %s: the class measure reads the classifier's decision from %s, which holds no single online version for source %s and estimator %s; "+
			"replay with the solid_body experiment, which writes it without changing the tracks. The database's solid-body versions:\n%s",
			arm.Label, solidBodyTable, arm.SourceID, arm.EstimatorID, listVersions(versions))
	default:
		return sqlite.EstimateVersion{}, fmt.Errorf("arm %s: %d solid-body versions could hold its classes; the database holds:\n%s",
			arm.Label, len(matched), listVersions(matched))
	}
}

// classTrackState is what the measure knows of one track's class rows.
type classTrackState struct {
	rows    []ClassRow
	byFrame map[int64]l5tracks.MotionClassBelief
}

func newClassTrackState(rows []ClassRow) *classTrackState {
	s := &classTrackState{rows: rows, byFrame: make(map[int64]l5tracks.MotionClassBelief, len(rows))}
	for _, r := range rows {
		s.byFrame[r.FrameUnixNanos] = r.Belief
	}
	return s
}

// describe fills a ClassTrack's whole-life fields from the track's rows.
func (s *classTrackState) describe(t *ClassTrack) {
	t.ClassRows = len(s.rows)
	if len(s.rows) == 0 {
		t.EndClass = ClassUnclassed
		return
	}
	last := s.rows[len(s.rows)-1].Belief
	t.EndClass = passageClass(last).String()
	posterior := float64(last.Posterior)
	t.EndPosterior = &posterior
	t.EndsRigid = followingAdmits(last)
	for i, r := range s.rows {
		if followingAdmits(r.Belief) {
			age := float64(r.FrameUnixNanos-s.rows[0].FrameUnixNanos) / 1e9
			rows := i
			t.FirstRigidAgeS, t.FirstRigidAgeRows = &age, &rows
			return
		}
	}
}

// ScoreClass computes one arm's class measure. The matching is the one
// ScoreArm tallies: the same alignment, gate and per-frame assignment.
func ScoreClass(ref *Reference, hyp Hypothesis, rows ClassRows, opts ScoreOptions) (ClassResult, error) {
	if err := opts.Validate(); err != nil {
		return ClassResult{}, err
	}
	res := ClassResult{Arm: hyp.Identity, ClassSource: rows.Source}
	tracks := map[string]*classTrackState{}
	trackState := func(id string) *classTrackState {
		s := tracks[id]
		if s == nil {
			s = newClassTrackState(rows.Tracks[id])
			tracks[id] = s
		}
		return s
	}

	for _, ep := range ref.Episodes {
		aligned, _, origins := alignWithOrigins(hyp.Series, ep, opts.FrameToleranceNanos)
		matchAt := map[int64]map[string]string{}
		for _, c := range l8analytics.MatchPerFrame(ep.Series, aligned, opts.Gate) {
			matchAt[c.TimestampNanos] = c.Matches
		}
		for _, s := range ep.Series {
			obj := ClassObject{
				EpisodeID: ep.EpisodeID, ObjectID: s.ID, Label: ref.ObjectClasses[s.ID],
				Tracks: []ClassTrack{}, frameClasses: map[string]int{},
			}
			obj.ExpectedMotionClass = labelMotionClass(obj.Label).String()
			trackIndex := map[string]int{}
			var firstFrame int64
			for _, p := range s.Points {
				if p.Ignore {
					continue
				}
				if obj.Frames == 0 {
					firstFrame = p.TimestampNanos
				}
				obj.Frames++
				hypID, matched := matchAt[p.TimestampNanos][s.ID]
				if !matched {
					obj.frameClasses[ClassUnmatched]++
					continue
				}
				obj.MatchedFrames++
				i, seen := trackIndex[hypID]
				if !seen {
					i = len(obj.Tracks)
					trackIndex[hypID] = i
					obj.Tracks = append(obj.Tracks, ClassTrack{Track: hypID, FirstMatchNs: p.TimestampNanos})
				}
				obj.Tracks[i].MatchedFrames++
				obj.Tracks[i].LastMatchNs = p.TimestampNanos
				obj.EndTrack = hypID

				belief, classed := trackState(hypID).byFrame[origins[hypID][p.TimestampNanos]]
				if !classed {
					obj.UnclassedFrames++
					obj.frameClasses[ClassUnclassed]++
					continue
				}
				class := passageClass(belief).String()
				obj.frameClasses[class]++
				if class == obj.ExpectedMotionClass {
					obj.ExpectedFrames++
				}
				if followingAdmits(belief) {
					obj.RigidFrames++
					if obj.TimeToFirstRigidS == nil {
						d := float64(p.TimestampNanos-firstFrame) / 1e9
						obj.TimeToFirstRigidS = &d
					}
				}
			}
			if obj.Frames == 0 {
				// Not a reviewed road user in this episode: every point
				// is ignored, so nothing about it is scored.
				continue
			}
			obj.EndClass = ClassUnmatched
			for i := range obj.Tracks {
				t := &obj.Tracks[i]
				trackState(t.Track).describe(t)
				obj.AnyTrackEndsRigid = obj.AnyTrackEndsRigid || t.EndsRigid
				if t.Track == obj.EndTrack {
					obj.EndClass, obj.EndsRigid = t.EndClass, t.EndsRigid
				}
			}
			res.Objects = append(res.Objects, obj)
		}
	}
	sort.SliceStable(res.Objects, func(i, j int) bool {
		if res.Objects[i].EpisodeID != res.Objects[j].EpisodeID {
			return res.Objects[i].EpisodeID < res.Objects[j].EpisodeID
		}
		return res.Objects[i].ObjectID < res.Objects[j].ObjectID
	})
	res.ClassSummary = summariseClass(res.Objects)
	for _, ep := range ref.Episodes {
		var objects []ClassObject
		for _, o := range res.Objects {
			if o.EpisodeID == ep.EpisodeID {
				objects = append(objects, o)
			}
		}
		res.Episodes = append(res.Episodes, ClassEpisode{EpisodeID: ep.EpisodeID, ClassSummary: summariseClass(objects)})
	}
	return res, nil
}

func summariseClass(objects []ClassObject) ClassSummary {
	var v VehicleClassSummary
	var f FalseApplicability
	var trackAges, objectTimes []float64
	byLabel := map[string]*LabelClassSummary{}
	for _, o := range objects {
		l := byLabel[o.Label]
		if l == nil {
			l = &LabelClassSummary{Label: o.Label, ExpectedMotionClass: o.ExpectedMotionClass,
				EndClasses: map[string]int{}, FrameClasses: map[string]int{}}
			byLabel[o.Label] = l
		}
		matched := len(o.Tracks) > 0
		l.Objects++
		l.Frames += o.Frames
		l.MatchedFrames += o.MatchedFrames
		l.ExpectedFrames += o.ExpectedFrames
		l.EndClasses[o.EndClass]++
		for class, n := range o.frameClasses {
			l.FrameClasses[class] += n
		}
		if matched {
			l.MatchedObjects++
		}
		if o.EndClass == o.ExpectedMotionClass {
			l.EndExpected++
		}

		switch labelMotionClass(o.Label) {
		case l8behaviour.MotionRigidVehicle:
			v.Objects++
			v.Frames += o.Frames
			v.MatchedFrames += o.MatchedFrames
			v.RigidFrames += o.RigidFrames
			if o.EndsRigid {
				v.EndRigid++
			}
			if o.AnyTrackEndsRigid {
				v.AnyTrackEndsRigid++
			}
			if !matched {
				continue
			}
			v.MatchedObjects++
			if o.TimeToFirstRigidS != nil {
				objectTimes = append(objectTimes, *o.TimeToFirstRigidS)
			} else {
				v.ObjectTimeToFirstRigid.Never++
			}
			for _, t := range o.Tracks {
				if t.Track != o.EndTrack {
					continue
				}
				if t.FirstRigidAgeS != nil {
					trackAges = append(trackAges, *t.FirstRigidAgeS)
				} else {
					v.TrackAgeAtFirstRigid.Never++
				}
				break
			}
		case l8behaviour.MotionPedestrian, l8behaviour.MotionTwoWheeler:
			f.Objects++
			f.Frames += o.Frames
			f.RigidFrames += o.RigidFrames
			if matched {
				f.MatchedObjects++
			}
			if o.EndsRigid {
				f.EndRigid++
			}
		}
	}
	v.EndRigidShare = share(v.EndRigid, v.Objects)
	v.AnyTrackEndsRigidShare = share(v.AnyTrackEndsRigid, v.Objects)
	v.RigidFrameShare = share(v.RigidFrames, v.Frames)
	v.RigidMatchedFrameShare = share(v.RigidFrames, v.MatchedFrames)
	v.TrackAgeAtFirstRigid = ageStats(trackAges, v.TrackAgeAtFirstRigid.Never)
	v.ObjectTimeToFirstRigid = ageStats(objectTimes, v.ObjectTimeToFirstRigid.Never)
	f.EndRigidShare = share(f.EndRigid, f.Objects)
	f.RigidFrameShare = share(f.RigidFrames, f.Frames)

	labels := make([]string, 0, len(byLabel))
	for l := range byLabel {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	out := make([]LabelClassSummary, 0, len(labels))
	for _, name := range labels {
		l := byLabel[name]
		l.EndRecall = share(l.EndExpected, l.Objects)
		l.FrameRecall = share(l.ExpectedFrames, l.Frames)
		out = append(out, *l)
	}
	return ClassSummary{Vehicles: v, FalseApplicability: f, ByLabel: out}
}

func share(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

func ageStats(values []float64, never int) AgeStats {
	s := AgeStats{N: len(values), Never: never}
	if len(values) == 0 {
		return s
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	median, p90 := quantileSorted(sorted, 0.5), quantileSorted(sorted, 0.9)
	s.Median, s.P90 = &median, &p90
	return s
}

// classDefinition is the measure's rules in words, recorded with its numbers.
func classDefinition(objects ...[]ClassObject) ClassDefinition {
	labels := map[string]string{}
	for _, objs := range objects {
		for _, o := range objs {
			labels[o.Label] = o.ExpectedMotionClass
		}
	}
	return ClassDefinition{
		Rigid: "a lidar_track_solid_bodies row's motion belief, through l8behaviour.PassageFromL5 (its effective class), " +
			"admitted by l8behaviour.ClassApplicability for the following metrics against a rigid counterpart: what the following gate admits",
		Attribution: "per frame, the track the per-frame matching assigned to the object at that frame (the MOT matching), and its class at that frame; " +
			"per object, the end track is the track matched at the object's last matched reviewed frame, read at its own latest row " +
			"(its end, as the following adapter reads a trajectory); fragments and duplicates appear in tracks and any_track_ends_rigid only",
		Age: "track age: seconds from a track's first class row to its first rigid row, over the track's whole life; " +
			"object time: seconds from the object's first reviewed frame to the first reviewed frame its matched track was rigid",
		LabelMotionClasses: labels,
	}
}

// scoreClassArms runs the class measure on both arms of a comparison.
func scoreClassArms(ref *Reference, opts ScoreOptions, specs [2]ArmSpec, hyps [2]Hypothesis) (*ClassArms, error) {
	var results [2]ClassResult
	for i := range specs {
		rows, err := LoadClassRows(specs[i], hyps[i].Identity)
		if err != nil {
			return nil, err
		}
		if results[i], err = ScoreClass(ref, hyps[i], rows, opts); err != nil {
			return nil, err
		}
	}
	return &ClassArms{Definition: classDefinition(results[0].Objects, results[1].Objects), A: results[0], B: results[1]}, nil
}
