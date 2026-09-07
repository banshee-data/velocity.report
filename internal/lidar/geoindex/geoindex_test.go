package geoindex

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// Broadway & Columbus, San Francisco — the junction the field captures name.
const (
	broadwayLat = 37.7987
	broadwayLng = -122.4073
)

func TestFromLatLngDerivesOneFamily(t *testing.T) {
	got, err := FromLatLng(broadwayLat, broadwayLng)
	if err != nil {
		t.Fatalf("FromLatLng: %v", err)
	}

	// The lengths the guide fixes for these levels. They are asserted because
	// the family display's hyphen position depends on them.
	for _, tc := range []struct {
		level int
		token string
		want  int
	}{
		{LevelCoarse, got.Coarse, 6},
		{LevelFine, got.Fine, 8},
		{LevelPrecise, got.Precise, 9},
	} {
		if len(tc.token) != tc.want {
			t.Errorf("L%d token %q is %d chars, want %d", tc.level, tc.token, len(tc.token), tc.want)
		}
	}

	// The whole point of deriving with Parent: the family cannot disagree.
	if err := Validate(got); err != nil {
		t.Errorf("derived tokens are not one family: %v", err)
	}
}

func TestFromLatLngIsStableAndLocal(t *testing.T) {
	a, err := FromLatLng(broadwayLat, broadwayLng)
	if err != nil {
		t.Fatalf("FromLatLng: %v", err)
	}
	again, err := FromLatLng(broadwayLat, broadwayLng)
	if err != nil {
		t.Fatalf("FromLatLng (repeat): %v", err)
	}
	if a != again {
		t.Errorf("same position gave %+v then %+v", a, again)
	}

	// A few metres away stays in the same site but may leave the precise cell,
	// which is what makes L16 useful for telling two sensors at one junction
	// apart.
	near, err := FromLatLng(broadwayLat+0.0002, broadwayLng)
	if err != nil {
		t.Fatalf("FromLatLng: %v", err)
	}
	if near.Coarse != a.Coarse {
		t.Errorf("20 m moved the site cell: %q → %q", a.Coarse, near.Coarse)
	}

	// A different city is a different site.
	far, err := FromLatLng(51.5074, -0.1278)
	if err != nil {
		t.Fatalf("FromLatLng: %v", err)
	}
	if far.Coarse == a.Coarse {
		t.Error("London and San Francisco share a site cell")
	}
}

func TestValidatePositionRejectsNonPositions(t *testing.T) {
	tests := []struct {
		name     string
		lat, lng float64
	}{
		{"latitude past the pole", 91, 0},
		{"latitude past the south pole", -91, 0},
		{"longitude past the meridian", 0, 181},
		{"longitude past the antimeridian", 0, -181},
		{"not a number", math.NaN(), 0},
		{"longitude not a number", 0, math.NaN()},
		{"infinite", math.Inf(1), 0},
		// A capture at exactly 0,0 is a missing fix that reached a float field,
		// not a deployment in the Gulf of Guinea.
		{"null island", 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidatePosition(tc.lat, tc.lng); !errors.Is(err, ErrNoPosition) {
				t.Fatalf("ValidatePosition(%v, %v) = %v, want ErrNoPosition", tc.lat, tc.lng, err)
			}
			if _, err := FromLatLng(tc.lat, tc.lng); err == nil {
				t.Error("FromLatLng accepted a position ValidatePosition rejected")
			}
		})
	}
}

func TestValidatePositionAcceptsRealFixes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lat, lng float64
	}{
		{"san francisco", broadwayLat, broadwayLng},
		{"the equator on a real meridian", 0, -122.4},
		{"the meridian at a real latitude", 51.48, 0},
		{"the poles", 90, 0},
		{"the antimeridian", 0, 180},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidatePosition(tc.lat, tc.lng); err != nil {
				t.Errorf("ValidatePosition(%v, %v) = %v, want nil", tc.lat, tc.lng, err)
			}
		})
	}
}

func TestFamilyDisplayMatchesTheGuide(t *testing.T) {
	// The guide's worked examples, which the implementation must reproduce
	// exactly: these strings appear in operator-facing text.
	for _, tc := range []struct{ canonical, want string }{
		{"808581", "80858-1"},
		{"80858004", "80858-004"},
		{"808580f3f", "80858-0f3f"},
	} {
		if got := FamilyDisplay(tc.canonical); got != tc.want {
			t.Errorf("FamilyDisplay(%q) = %q, want %q", tc.canonical, got, tc.want)
		}
	}
}

func TestFamilyDisplayRoundTripsToTheCanonicalToken(t *testing.T) {
	// Removing the hyphen recovers the identifier. This is what lets operator
	// input accept either form.
	tokens, err := FromLatLng(broadwayLat, broadwayLng)
	if err != nil {
		t.Fatalf("FromLatLng: %v", err)
	}
	for _, canonical := range []string{tokens.Coarse, tokens.Fine, tokens.Precise} {
		display := FamilyDisplay(canonical)
		if !strings.Contains(display, "-") {
			t.Errorf("FamilyDisplay(%q) = %q, want a hyphen", canonical, display)
		}
		if got := strings.Replace(display, "-", "", 1); got != canonical {
			t.Errorf("removing the hyphen from %q gave %q, want %q", display, got, canonical)
		}
		cell, err := ParseToken(display)
		if err != nil {
			t.Fatalf("ParseToken(%q): %v", display, err)
		}
		if cell.ToToken() != canonical {
			t.Errorf("ParseToken(%q) resolved to %q, want %q", display, cell.ToToken(), canonical)
		}
	}
}

func TestFamilyDisplayLeavesShortTokensAlone(t *testing.T) {
	// Inventing a hyphen would produce a string that is neither a canonical
	// token nor a family display.
	for _, short := range []string{"", "8", "80858"} {
		if got := FamilyDisplay(short); got != short {
			t.Errorf("FamilyDisplay(%q) = %q, want it unchanged", short, got)
		}
		if got := FamilyPrefix(short); got != short {
			t.Errorf("FamilyPrefix(%q) = %q, want it unchanged", short, got)
		}
	}
}

func TestFamilyPrefixIsNotAnIdentifier(t *testing.T) {
	// One prefix can span several coarse cells, which is why it is a visual aid
	// and never a key. Two different L10 cells sharing a prefix demonstrate it.
	if FamilyPrefix("808581") != "80858" {
		t.Errorf("FamilyPrefix(808581) = %q, want 80858", FamilyPrefix("808581"))
	}
	if FamilyPrefix("808583") != FamilyPrefix("808581") {
		t.Error("two distinct L10 cells in one family have different prefixes")
	}
	if "808583" == "808581" {
		t.Error("the two tokens above are not actually distinct")
	}
}

func TestParseTokenAcceptsBothForms(t *testing.T) {
	cell, err := ParseToken("808581")
	if err != nil {
		t.Fatalf("ParseToken canonical: %v", err)
	}
	fromDisplay, err := ParseToken("80858-1")
	if err != nil {
		t.Fatalf("ParseToken family display: %v", err)
	}
	if cell != fromDisplay {
		t.Error("the canonical token and its family display resolved to different cells")
	}
	if got := ParseTokenLevel(t, "808581"); got != LevelCoarse {
		t.Errorf("level = %d, want %d", got, LevelCoarse)
	}
}

// ParseTokenLevel is a test helper reporting a token's level.
func ParseTokenLevel(t *testing.T, token string) int {
	t.Helper()
	cell, err := ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken(%q): %v", token, err)
	}
	return cell.Level()
}

func TestParseTokenRejectsMalformedInput(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"empty", ""},
		{"whitespace", "   "},
		{"not hexadecimal", "zzzzzz"},
		{"two hyphens", "808-58-1"},
		// Upper case round-trips to a different string, so accepting it would
		// make "the canonical token is the identifier" aspirational rather
		// than true: one cell would have two spellings and so two keys.
		{"upper case", "808580F3F"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseToken(tc.value); !errors.Is(err, ErrBadToken) {
				t.Fatalf("ParseToken(%q) = %v, want ErrBadToken", tc.value, err)
			}
		})
	}
}

func TestParseTokenRequiresTheCanonicalForm(t *testing.T) {
	// The S2 library accepts a token with the trailing zeroes the canonical
	// form omits. The guide does not: two spellings of one cell would mean two
	// database keys for one place.
	if _, err := ParseToken("808581000"); !errors.Is(err, ErrBadToken) {
		t.Errorf("ParseToken accepted a spelling with trailing zeroes: %v", err)
	}
	// Surrounding whitespace is trimmed, because it is a paste artefact rather
	// than a different token.
	if _, err := ParseToken("  808581  "); err != nil {
		t.Errorf("ParseToken rejected a padded canonical token: %v", err)
	}
}

func TestValidateRejectsMixedFamilies(t *testing.T) {
	sf, err := FromLatLng(broadwayLat, broadwayLng)
	if err != nil {
		t.Fatalf("FromLatLng: %v", err)
	}
	london, err := FromLatLng(51.5074, -0.1278)
	if err != nil {
		t.Fatalf("FromLatLng: %v", err)
	}

	mixed := sf
	mixed.Coarse = london.Coarse
	if err := Validate(mixed); !errors.Is(err, ErrInconsistent) {
		t.Fatalf("Validate accepted tokens from two cities: %v", err)
	}

	mixedFine := sf
	mixedFine.Fine = london.Fine
	if err := Validate(mixedFine); !errors.Is(err, ErrInconsistent) {
		t.Fatalf("Validate accepted a fine cell from another city: %v", err)
	}
}

func TestValidateRejectsWrongLevels(t *testing.T) {
	sf, err := FromLatLng(broadwayLat, broadwayLng)
	if err != nil {
		t.Fatalf("FromLatLng: %v", err)
	}
	swapped := sf
	swapped.Coarse, swapped.Fine = sf.Fine, sf.Coarse
	if err := Validate(swapped); !errors.Is(err, ErrBadToken) {
		t.Fatalf("Validate accepted tokens at the wrong levels: %v", err)
	}
}

func TestValidateRejectsMissingTokens(t *testing.T) {
	if err := Validate(Tokens{}); err == nil {
		t.Fatal("Validate accepted an empty set")
	}
}

func TestCentreOfRoundTripsThePosition(t *testing.T) {
	tokens, err := FromLatLng(broadwayLat, broadwayLng)
	if err != nil {
		t.Fatalf("FromLatLng: %v", err)
	}
	lat, lng, err := CentreOf(tokens.Precise)
	if err != nil {
		t.Fatalf("CentreOf: %v", err)
	}
	// An L16 cell is a few tens of metres across, so its centre is close to any
	// position inside it.
	if math.Abs(lat-broadwayLat) > 0.001 || math.Abs(lng-broadwayLng) > 0.001 {
		t.Errorf("centre of the precise cell is %v,%v — too far from %v,%v",
			lat, lng, broadwayLat, broadwayLng)
	}
	if _, _, err := CentreOf("nonsense"); err == nil {
		t.Error("CentreOf accepted a bad token")
	}
}

func TestBoundOfContainsThePosition(t *testing.T) {
	tokens, err := FromLatLng(broadwayLat, broadwayLng)
	if err != nil {
		t.Fatalf("FromLatLng: %v", err)
	}
	swLat, swLng, neLat, neLng, err := BoundOf(tokens.Coarse)
	if err != nil {
		t.Fatalf("BoundOf: %v", err)
	}
	if broadwayLat < swLat || broadwayLat > neLat {
		t.Errorf("latitude %v outside the cell bound [%v, %v]", broadwayLat, swLat, neLat)
	}
	if broadwayLng < swLng || broadwayLng > neLng {
		t.Errorf("longitude %v outside the cell bound [%v, %v]", broadwayLng, swLng, neLng)
	}
	if _, _, _, _, err := BoundOf("nonsense"); err == nil {
		t.Error("BoundOf accepted a bad token")
	}
}
