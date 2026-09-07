// Package geoindex derives the S2 geographic identity of a located capture.
//
// It implements the levels, tokens and displays fixed by
// docs/lidar/architecture/geographic-indexing.md, which is normative. Two rules
// from that guide shape everything here:
//
//   - Only the canonical token is an identifier. It is what is stored, indexed,
//     put in a filename, and exchanged with other S2 implementations.
//   - The family display is presentation derived from the canonical token. It
//     may appear in logs and UI and must never be persisted or used as a key.
//
// The guide also requires that names describe purpose and S2 level rather than
// character counts, which is why nothing here is called "five-one".
package geoindex

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/golang/geo/s2"
)

// The S2 levels velocity.report indexes at.
//
// Coarse is the site: an L10 cell is roughly a kilometre across and is what
// groups captures taken at one junction over many visits. Fine locates a
// deployment within it. Precise distinguishes two sensors at one junction —
// opposite corners of an intersection fall in different L16 cells.
const (
	LevelCoarse  = 10
	LevelFine    = 13
	LevelPrecise = 16
)

// familyBoundary is where the family display's single hyphen goes. The guide
// calls this the configured family boundary; the shared prefix it produces is a
// visual aid, not an S2 cell.
const familyBoundary = 5

// Errors returned when a position or token cannot be indexed.
var (
	// ErrNoPosition is returned for a position that is not a real WGS84 fix.
	ErrNoPosition = errors.New("geoindex: no usable WGS84 position")
	// ErrBadToken is returned for a string that is not a canonical token or a
	// family display of one.
	ErrBadToken = errors.New("geoindex: not a valid S2 token")
	// ErrInconsistent is returned when a set of tokens are not ancestors of one
	// another, which means they did not come from one position.
	ErrInconsistent = errors.New("geoindex: tokens are not one cell family")
)

// Tokens are the canonical S2 tokens for one located capture: the identity that
// is stored, indexed and exchanged.
type Tokens struct {
	// Coarse, Fine and Precise are the canonical tokens at LevelCoarse,
	// LevelFine and LevelPrecise. Each is the standard S2 hexadecimal
	// serialisation with trailing zeroes removed.
	Coarse  string `json:"s2_l10_token"`
	Fine    string `json:"s2_l13_token"`
	Precise string `json:"s2_l16_token"`
}

// FromLatLng derives the canonical tokens for a WGS84 position.
//
// The fine and coarse cells are derived from the precise one with Parent, never
// recomputed from the position: the guide requires the coarse value to come
// only from Parent so a stored family can never disagree with itself.
func FromLatLng(latDegrees, lngDegrees float64) (Tokens, error) {
	if err := ValidatePosition(latDegrees, lngDegrees); err != nil {
		return Tokens{}, err
	}
	precise := s2.CellIDFromLatLng(s2.LatLngFromDegrees(latDegrees, lngDegrees)).Parent(LevelPrecise)
	return Tokens{
		Precise: precise.ToToken(),
		Fine:    precise.Parent(LevelFine).ToToken(),
		Coarse:  precise.Parent(LevelCoarse).ToToken(),
	}, nil
}

// ValidatePosition reports whether a position is a usable WGS84 fix.
//
// Null Island is rejected: a capture at exactly 0,0 is a missing fix that
// reached a float field, not a deployment in the Gulf of Guinea, and indexing
// it would put a phantom site on the scene map.
func ValidatePosition(latDegrees, lngDegrees float64) error {
	if math.IsNaN(latDegrees) || math.IsNaN(lngDegrees) ||
		math.IsInf(latDegrees, 0) || math.IsInf(lngDegrees, 0) {
		return fmt.Errorf("%w: latitude or longitude is not a number", ErrNoPosition)
	}
	if latDegrees < -90 || latDegrees > 90 {
		return fmt.Errorf("%w: latitude %g is outside ±90°", ErrNoPosition, latDegrees)
	}
	if lngDegrees < -180 || lngDegrees > 180 {
		return fmt.Errorf("%w: longitude %g is outside ±180°", ErrNoPosition, lngDegrees)
	}
	if latDegrees == 0 && lngDegrees == 0 {
		return fmt.Errorf("%w: 0,0 is an absent fix rather than a location", ErrNoPosition)
	}
	return nil
}

// FamilyDisplay renders a canonical token for human-facing text, with one
// hyphen at the family boundary.
//
// This is presentation. It must not be stored, used as a key, or written into a
// filename. Removing the hyphen recovers the canonical token, which is what
// ParseToken relies on when accepting operator input.
//
// A token shorter than the boundary is returned unchanged: there is nothing to
// group, and inventing a hyphen would produce a string that is neither a
// canonical token nor a family display.
func FamilyDisplay(canonicalToken string) string {
	if len(canonicalToken) <= familyBoundary {
		return canonicalToken
	}
	return canonicalToken[:familyBoundary] + "-" + canonicalToken[familyBoundary:]
}

// FamilyPrefix is the part of a family display before the hyphen.
//
// It is a visual aid for spotting captures from nearby cells in a list. It is
// not an S2 cell: one prefix can span several coarse cells, and it must never
// be treated as an identifier.
func FamilyPrefix(canonicalToken string) string {
	if len(canonicalToken) <= familyBoundary {
		return canonicalToken
	}
	return canonicalToken[:familyBoundary]
}

// ParseToken accepts a canonical token or a family display of one and returns
// the cell.
//
// Operator input may carry the presentation hyphen, so it is removed before
// validation; nothing else is stripped, because a string with two hyphens or
// stray spacing is a mistake worth reporting rather than a token worth
// guessing at.
func ParseToken(value string) (s2.CellID, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("%w: empty", ErrBadToken)
	}
	if strings.Count(trimmed, "-") > 1 {
		return 0, fmt.Errorf("%w: %q has more than one hyphen", ErrBadToken, value)
	}
	canonical := strings.Replace(trimmed, "-", "", 1)

	cell := s2.CellIDFromToken(canonical)
	if !cell.IsValid() {
		return 0, fmt.Errorf("%w: %q", ErrBadToken, value)
	}
	// CellIDFromToken accepts strings that round-trip to a different token —
	// upper case, or trailing zeroes the canonical form omits. Requiring the
	// round trip is what makes "the canonical token is the identifier" true
	// rather than aspirational.
	if cell.ToToken() != canonical {
		return 0, fmt.Errorf("%w: %q is not canonical (canonical form is %q)",
			ErrBadToken, value, cell.ToToken())
	}
	return cell, nil
}

// Validate checks that a set of tokens describes one cell family: each coarser
// token must be the Parent of the finer one.
//
// A row carrying tokens that are not one family did not come from one position,
// and the guide treats that as a hard provenance error rather than something to
// reconcile.
func Validate(t Tokens) error {
	precise, err := parseAtLevel(t.Precise, LevelPrecise)
	if err != nil {
		return err
	}
	fine, err := parseAtLevel(t.Fine, LevelFine)
	if err != nil {
		return err
	}
	coarse, err := parseAtLevel(t.Coarse, LevelCoarse)
	if err != nil {
		return err
	}
	if precise.Parent(LevelFine) != fine {
		return fmt.Errorf("%w: L%d %s is not inside L%d %s",
			ErrInconsistent, LevelPrecise, t.Precise, LevelFine, t.Fine)
	}
	if fine.Parent(LevelCoarse) != coarse {
		return fmt.Errorf("%w: L%d %s is not inside L%d %s",
			ErrInconsistent, LevelFine, t.Fine, LevelCoarse, t.Coarse)
	}
	return nil
}

// parseAtLevel parses a token and requires it to be at the expected level.
func parseAtLevel(token string, level int) (s2.CellID, error) {
	cell, err := ParseToken(token)
	if err != nil {
		return 0, err
	}
	if cell.Level() != level {
		return 0, fmt.Errorf("%w: %q is level %d, want level %d",
			ErrBadToken, token, cell.Level(), level)
	}
	return cell, nil
}

// CentreOf returns the WGS84 centre of a cell, for placing it on a map.
func CentreOf(canonicalToken string) (latDegrees, lngDegrees float64, err error) {
	cell, err := ParseToken(canonicalToken)
	if err != nil {
		return 0, 0, err
	}
	ll := cell.LatLng()
	return ll.Lat.Degrees(), ll.Lng.Degrees(), nil
}

// BoundOf returns the WGS84 bounding box of a cell as south-west and north-east
// corners, so a map can draw the cell rather than only a point.
func BoundOf(canonicalToken string) (swLat, swLng, neLat, neLng float64, err error) {
	cell, err := ParseToken(canonicalToken)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	rect := s2.CellFromCellID(cell).RectBound()
	return rect.Lo().Lat.Degrees(), rect.Lo().Lng.Degrees(),
		rect.Hi().Lat.Degrees(), rect.Hi().Lng.Degrees(), nil
}
