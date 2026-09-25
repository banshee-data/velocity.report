package headway

// Display formats. Go formats every number the report prints, so the golden
// data.json holds the printed text and the template stays a layout.
//
// Values are rounded to four decimals and written without trailing zeros:
// enough for the oracle's exact quarter- and eighth-metre geometry (13.875 m,
// 1.3875 s) to print exactly, and no more. Field values print with the same
// rule beside their uncertainty, which is what says how many of those digits
// mean anything.

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

const displayDecimals = 4

// formatNumber rounds to displayDecimals and drops trailing zeros.
func formatNumber(v float64) string {
	scale := math.Pow(10, displayDecimals)
	r := math.Round(v*scale) / scale
	if r == 0 {
		r = 0 // drop a negative zero
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// formatWithUnit writes a value in a registry unit. A ratio is dimensionless
// and prints bare.
func formatWithUnit(v float64, unit string) string {
	if unit == "ratio" || unit == "" {
		return formatNumber(v)
	}
	return formatNumber(v) + " " + unit
}

// formatNanos writes a duration in seconds.
func formatNanos(n int64) string {
	return formatWithUnit(float64(n)/1e9, "s")
}

// formatRange writes an interval of offsets.
func formatRange(start, end int64) string {
	return formatNumber(float64(start)/1e9) + " to " + formatNanos(end)
}

// formatSpread writes a minimum and maximum, collapsing them when equal.
func formatSpread(lo, hi float64, unit string) string {
	if lo == hi {
		return formatWithUnit(lo, unit)
	}
	return formatNumber(lo) + " to " + formatWithUnit(hi, unit)
}

// formatShare writes a share of one as a percentage with one decimal.
func formatShare(share float64) string {
	return strconv.FormatFloat(share*100, 'f', 1, 64) + "%"
}

// formatUTC writes an absolute capture time as UTC ISO 8601 with a Z.
func formatUTC(unixNanos int64) string {
	return time.Unix(0, unixNanos).UTC().Format("2006-01-02T15:04:05.000Z")
}

// suppressedDisplay is how every suppressed value prints: its reason and no
// number, so it cannot be read as zero.
func suppressedDisplay(r l8behaviour.SuppressionReason) string {
	return "suppressed: " + r.String()
}

// formatUncertainty writes an uncertainty in the representation it carries.
func formatUncertainty(u *l8behaviour.Uncertainty, unit string) string {
	if u == nil {
		return ""
	}
	switch u.Kind {
	case l8behaviour.UncertaintyNone:
		return "none"
	case l8behaviour.UncertaintySigma:
		return "sigma " + formatWithUnit(*u.Sigma, unit) + " (" + u.Method.String() + ")"
	case l8behaviour.UncertaintyInterval:
		coverage := strconv.FormatFloat(*u.Coverage*100, 'f', -1, 64)
		s := coverage + "% interval " + formatNumber(*u.Lower) + " to " + formatWithUnit(*u.Upper, unit) +
			" (" + u.Method.String()
		if u.Samples > 0 {
			s += ", " + strconv.Itoa(u.Samples) + " samples"
		}
		return s + ")"
	case l8behaviour.UncertaintyBounds:
		var parts []string
		if u.Lower != nil {
			parts = append(parts, "lower "+formatWithUnit(*u.Lower, unit))
		}
		if u.Upper != nil {
			parts = append(parts, "upper "+formatWithUnit(*u.Upper, unit))
		}
		return "bounds " + strings.Join(parts, ", ") + " (" + u.Method.String() + ")"
	}
	return u.Kind.String()
}

// formatMillis writes a bin edge given in milliseconds, in seconds.
func formatMillis(ms int) string {
	return formatNumber(float64(ms) / 1000)
}
