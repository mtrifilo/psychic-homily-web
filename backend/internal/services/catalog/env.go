package catalog

import (
	"os"
	"strconv"
	"time"
)

// Env-config helpers for the background services' tunable knobs (PSY-1270). Each
// returns the default when the variable is unset, unparseable, or out of the
// helper's accepted range, so a typo silently falls back instead of taking a bad
// value. The helper CHOICE names the disable-vs-invalid distinction that used to be
// an easy-to-fat-finger inline `> 0` vs `>= 0`:
//   - envPositiveInt / envPositiveHours: the value must be > 0 (0 is invalid).
//   - envNonNegativeInt: the value must be >= 0 (0 is a valid "disable").
//
// These deliberately do NOT log a rejected value (matching the pre-existing
// silent-fallback behavior of every knob they migrate). A knob that needs to surface
// a rejected override (e.g. resolveFetchLookbackFloorDays, which also enforces an
// upper bound and Warns) keeps its own bespoke parsing.

// envPositiveInt returns the env var parsed as a positive int (> 0), or def when the
// var is unset, unparseable, or non-positive.
func envPositiveInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// envNonNegativeInt returns the env var parsed as a non-negative int (>= 0), or def
// when the var is unset, unparseable, or negative. 0 is accepted (typically "disable").
func envNonNegativeInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// envPositiveHours returns the env var parsed as a positive WHOLE number of hours
// (as a Duration), or def when the var is unset, unparseable, or non-positive. The
// value is whole hours only — a fractional or unit-suffixed value (e.g. "1.5", "30m")
// is unparseable by strconv.Atoi and silently falls back to def.
func envPositiveHours(name string, def time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if hours, err := strconv.Atoi(v); err == nil && hours > 0 {
			return time.Duration(hours) * time.Hour
		}
	}
	return def
}
