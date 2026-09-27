package headway

import (
	"errors"
	"fmt"
)

// Status is what a headway report's numbers are, per the staged delivery of
// Section 10.4 of docs/plans/lidar-behaviour-analytics-plan.md. It is a
// closed vocabulary with the l8behaviour shape: the zero value is
// unspecified and refuses to serialise, so a report whose builder forgot its
// status cannot be written at all, let alone written without its label.
type Status uint8

const (
	// StatusUnspecified is the zero value and never valid.
	StatusUnspecified Status = iota
	// StatusSyntheticOracle: analytic two-body trajectories with known
	// bumpers, gap and time gap, rendered through the real output path so
	// the report contract can be reviewed before an estimator reaches it.
	StatusSyntheticOracle
	// StatusProvisional: persisted estimator output that has not passed the
	// field promotion gates. It proves integration and exposes missing
	// evidence; it claims no physical accuracy.
	StatusProvisional
	// StatusPromoted: field results that passed G-GEO-1, G-UNC-1, G-SMO-1 and
	// the metric gate. Reserved: no build can assert those gates yet, so
	// Build refuses it.
	StatusPromoted
	statusEnd
)

var (
	statusTokens = [statusEnd]string{"", "synthetic_oracle", "provisional", "promoted"}
	statusLabels = [statusEnd]string{"", "SYNTHETIC ORACLE", "PROVISIONAL", "PROMOTED"}
	statusNotes  = [statusEnd]string{
		"",
		"Synthetic oracle: analytic two-body trajectories whose bumpers, gap and time gap are known. " +
			"No sensor data is used. The report exists so its contract can be reviewed before any estimator reaches it.",
		"Provisional: estimator output that has not passed the field promotion gates. " +
			"It proves integration and exposes missing evidence; it claims no physical accuracy.",
		"Promoted: field results that passed the promotion gates.",
	}
)

// ErrPromotionGated is returned for a promoted report: the promotion gates
// are not yet something a build can assert, so nothing may carry the label.
var ErrPromotionGated = errors.New("headway report status promoted requires the field promotion gates " +
	"(G-GEO-1, G-UNC-1, G-SMO-1 and the metric gate), which no build can assert yet")

// Valid reports whether s is a registered status.
func (s Status) Valid() bool { return s > StatusUnspecified && s < statusEnd }

// String is the wire token, or a diagnostic for an invalid value.
func (s Status) String() string {
	if s.Valid() {
		return statusTokens[s]
	}
	if s == StatusUnspecified {
		return "unspecified"
	}
	return fmt.Sprintf("status(%d)", uint8(s))
}

// Label is the status as it is printed on every page and every chart.
func (s Status) Label() string {
	if s.Valid() {
		return statusLabels[s]
	}
	return ""
}

// Note is the status's one-paragraph explanation, printed with the label.
func (s Status) Note() string {
	if s.Valid() {
		return statusNotes[s]
	}
	return ""
}

// MarshalText writes the registered token and refuses anything else.
func (s Status) MarshalText() ([]byte, error) {
	if !s.Valid() {
		return nil, fmt.Errorf("cannot serialise headway report status %s", s)
	}
	return []byte(statusTokens[s]), nil
}

// UnmarshalText accepts registered tokens only.
func (s *Status) UnmarshalText(b []byte) error {
	v, err := ParseStatus(string(b))
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// ParseStatus parses a registered token.
func ParseStatus(token string) (Status, error) {
	for s := StatusUnspecified + 1; s < statusEnd; s++ {
		if statusTokens[s] == token {
			return s, nil
		}
	}
	return StatusUnspecified, fmt.Errorf("unknown headway report status %q", token)
}

// Statuses lists every status.
func Statuses() []Status {
	out := make([]Status, 0, statusEnd-1)
	for s := StatusUnspecified + 1; s < statusEnd; s++ {
		out = append(out, s)
	}
	return out
}
