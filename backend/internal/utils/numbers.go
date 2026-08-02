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
// Two rejections are load-bearing rather than fussy:
//
//   - Fractional values. Writing a float to an integer column raises NO error
//     anywhere in the stack; measured, 1990.7 lands as 1990. A value the
//     contributor never typed, stored silently, is worse than a rejection.
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
