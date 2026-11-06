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
	State     string    `json:"state"`
	RecordID  string    `json:"record_id,omitempty"`
	Record    string    `json:"record_state,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// Placed reports whether a record places the clip.
func (r Registration) Placed() bool { return r.State == ClipRegistered && r.RecordID != "" }

// AcceptedRecord picks the record that places a clip of sensorID whose
// captures or dataset id are refs, from its site's records.
//
// Only accepted records count, and only those of the clip's sensor or of no
// stated sensor (Gerardus refuses a field whose sensor differs). A record
// superseded by another in the list is passed over. Among the rest, one
// whose capture_ref names one of refs (by basename, either way round) wins;
// with none naming the clip, the most recently reviewed wins. Two records
// equally good are refused rather than guessed between: a field placed by the
// wrong pose corrupts the site's sum for every later proposal.
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
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].ReviewedAt.After(pool[j].ReviewedAt) })
	if len(pool) > 1 && pool[0].ReviewedAt.Equal(pool[1].ReviewedAt) {
		return Registration{State: ClipAmbiguous, Reason: fmt.Sprintf("records %s and %s fit equally; name one", pool[0].RecordID, pool[1].RecordID)}
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
