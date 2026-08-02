package utils

import "math"

// WholeNumber narrows a value decoded from arbitrary JSON to a Go int,
// reporting whether the value really was a whole number.
//
// It exists because the pending-edit pipeline stores field values as JSONB and
// hands them back as `any`: a numeric column edited through that pipeline
// always arrives as float64, encoding/json's representation for every number
// decoded into an interface. A string, a bool, an object and a fractional
// number are not acceptable.
//
// The int case is for callers that construct a value directly rather than
// decoding one, which today means tests only. It is kept because a helper that
// rejected a plain Go int would be surprising, not because production reaches
// it.
//
// Only int and float64 are accepted, because those are the only two shapes
// this codebase actually produces. A new decoder (json.Number via UseNumber,
// say) would arrive here as an unrecognized type and be rejected loudly rather
// than silently coerced, which is the right failure for a value headed into an
// untyped SQL update.
//
// MEASURED BEHAVIOR of the layers underneath, which is the whole reason this
// exists. Writing an unnarrowed value into an integer column through GORM's
// untyped Updates() raises NO error at any layer:
//
//	Updates(550.7)  -> stored 550     Updates(549.5) -> stored 549
//	Updates(1990.7) -> stored 1990    Updates("1985") -> stored 1985
//
// So a float is TRUNCATED, not rounded, and a numeric string is accepted
// outright. (A bare SQL cast disagrees: SELECT 550.7::int is 551. The driver
// path is what matters here, and it truncates.) Either way the result is a
// value nobody chose, written silently, in a column nobody re-reads. This is
// the single place that statement lives; other call sites point here.
//
// Two rejections follow from it:
//
//   - Fractional values, per the measurements above.
//   - Values outside the int range. Converting an out-of-range float64 to int
//     is implementation-defined in Go, so the check has to happen before the
//     conversion, not after.
//
// Callers apply their own domain bounds to the returned int; this helper only
// answers "is this a whole number I can safely hold in an int".
func WholeNumber(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case float64:
		// NaN fails this comparison (NaN != NaN) and the infinities fail the
		// range check below, so neither needs a special case.
		if v != math.Trunc(v) {
			return 0, false
		}
		// float64(math.MaxInt) rounds UP to 2^63, which is not a representable
		// int, so the upper bound is exclusive. The lower bound is exactly
		// -2^63 and stays inclusive.
		if v < float64(math.MinInt) || v >= float64(math.MaxInt) {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
}
