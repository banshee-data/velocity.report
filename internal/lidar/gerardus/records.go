package gerardus

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Record states that place a lane field: an operator accepted the pose,
// from a passing registration or by hand. Gerardus refuses any other.
const (
	StateRegistered     = "registered"
	StateManuallyPlaced = "manually_placed"
)

// Registration states a clip can be in, for the clips view.
const (
	ClipRegistered     = "registered"     // an accepted record places it
	ClipNotRegistered  = "not_registered" // the site has no accepted record for this sensor
	ClipOtherSensor    = "other_sensor"   // accepted records exist, for other sensors only
	ClipAmbiguous      = "ambiguous"      // two accepted records fit equally well
	ClipNotLocated     = "not_located"    // the clip has no site
	ClipUnknown        = "unknown"        // Gerardus did not answer
	ClipNoLaneFieldAPI = "no_lane_fields" // the server takes no lane fields
)

// Record is a registration record as the job reads it.
type Record struct {
	RecordID   string    `json:"record_id"`
	State      string    `json:"state"`
	L16Token   string    `json:"l16_token"`
	ScanID     string    `json:"scan_id,omitempty"`
	CaptureRef string    `json:"capture_ref,omitempty"`
	SensorID   string    `json:"sensor_id,omitempty"`
	Supersedes string    `json:"supersedes_record_id,omitempty"`
	ReviewedAt time.Time `json:"reviewed_at,omitempty"`
}

// Accepted reports whether the record's pose was accepted.
func (r Record) Accepted() bool {
	return r.State == StateRegistered || r.State == StateManuallyPlaced
}

// Registration is a clip's answer to "is it registered": its state, the
// record that places it when it is, and why when it is not.
type Registration struct {
	State    string `json:"state"`
	RecordID string `json:"record_id,omitempty"`
	Record   string `json:"record_state,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Candidates are the accepted records an operator may pin when the clip
	// is ambiguous.
	Candidates []string  `json:"candidates,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// Placed reports whether a record places the clip.
func (r Registration) Placed() bool { return r.State == ClipRegistered && r.RecordID != "" }

// AcceptedRecord picks the record that places a clip of sensorID whose
// captures or dataset id are refs, from its site's records.
//
// Only accepted records count, and only those of the clip's sensor or of no
// stated sensor (Gerardus refuses a field whose sensor differs). A record
// superseded by another in the list is passed over. One record left places
// the clip. With several, the one whose capture_ref names one of refs (by
// basename, extension aside) places it; otherwise the clip is ambiguous and
// refused rather than guessed: a field placed by the wrong pose corrupts the
// site's sum for every later proposal.
func AcceptedRecord(records []Record, sensorID string, refs []string) Registration {
	superseded := map[string]bool{}
	for _, r := range records {
		if r.Supersedes != "" {
			superseded[r.Supersedes] = true
		}
	}
	var accepted, others []Record
	for _, r := range records {
		if !r.Accepted() || superseded[r.RecordID] {
			continue
		}
		if r.SensorID == "" || sensorID == "" || r.SensorID == sensorID {
			accepted = append(accepted, r)
		} else {
			others = append(others, r)
		}
	}
	if len(accepted) == 0 {
		if len(others) > 0 {
			sensors := map[string]bool{}
			for _, r := range others {
				sensors[r.SensorID] = true
			}
			names := make([]string, 0, len(sensors))
			for s := range sensors {
				names = append(names, s)
			}
			sort.Strings(names)
			return Registration{State: ClipOtherSensor, Reason: fmt.Sprintf("accepted only for sensor %s, not %s", strings.Join(names, ", "), sensorID)}
		}
		return Registration{State: ClipNotRegistered, Reason: "no accepted pose at this site"}
	}
	var named []Record
	for _, r := range accepted {
		if namesClip(r.CaptureRef, refs) {
			named = append(named, r)
		}
	}
	pool := accepted
	if len(named) > 0 {
		pool = named
	}
	// Two live records are two scans: two sensor positions, or two visits.
	// Unless the clip's own capture picks one, nothing here can tell which
	// pose its tracks were seen from, so it is refused, as the scene vectors
	// refuse a site with several accepted scans.
	if len(pool) > 1 {
		ids := make([]string, 0, len(pool))
		for _, r := range pool {
			ids = append(ids, r.RecordID)
		}
		sort.Strings(ids)
		why := "none names this clip's capture"
		if len(named) > 0 {
			why = "more than one names this clip's capture"
		}
		return Registration{State: ClipAmbiguous, Candidates: ids, Reason: fmt.Sprintf("%d accepted records at this site (%s) and %s", len(pool), strings.Join(ids, ", "), why)}
	}
	return Registration{State: ClipRegistered, RecordID: pool[0].RecordID, Record: pool[0].State}
}

// namesClip reports whether a record's capture_ref names any of refs. A
// capture_ref is velocity.report's name for the capture: a basename, a path,
// or "<basename>@<time>" as the lane field writes it.
func namesClip(captureRef string, refs []string) bool {
	if captureRef == "" {
		return false
	}
	have := strings.ToLower(filepath.Base(strings.SplitN(captureRef, "@", 2)[0]))
	for _, ref := range refs {
		want := strings.ToLower(filepath.Base(strings.SplitN(ref, "@", 2)[0]))
		if want != "" && (have == want || strings.TrimSuffix(have, filepath.Ext(have)) == strings.TrimSuffix(want, filepath.Ext(want))) {
			return true
		}
	}
	return false
}

// PinnedRecord checks a record an operator named for a clip: it must be one of
// the site's live accepted records, of the clip's sensor or of none.
func PinnedRecord(records []Record, sensorID, recordID string) Registration {
	superseded := map[string]bool{}
	for _, r := range records {
		if r.Supersedes != "" {
			superseded[r.Supersedes] = true
		}
	}
	for _, r := range records {
		if r.RecordID != recordID {
			continue
		}
		switch {
		case !r.Accepted():
			return Registration{State: ClipNotRegistered, Reason: fmt.Sprintf("record %s is %s, not an accepted pose", recordID, r.State)}
		case superseded[r.RecordID]:
			return Registration{State: ClipNotRegistered, Reason: fmt.Sprintf("record %s has been superseded", recordID)}
		case r.SensorID != "" && sensorID != "" && r.SensorID != sensorID:
			return Registration{State: ClipOtherSensor, Reason: fmt.Sprintf("record %s is sensor %s, not %s", recordID, r.SensorID, sensorID)}
		}
		return Registration{State: ClipRegistered, RecordID: r.RecordID, Record: r.State}
	}
	return Registration{State: ClipNotRegistered, Reason: fmt.Sprintf("no record %s at this site", recordID)}
}
