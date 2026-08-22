package shared

import (
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvServiceDisabled reports whether a default-ON background service has been
// switched off through its DISABLE_* flag.
//
// The semantics are exactly the repo's existing convention, deliberately not a new
// one: disabled IFF the value is the string "1"; any other value, including unset,
// leaves the service running. cmd/server/main.go's nine DISABLE_* flags all read
// this way, the frontend E2E harness sets them to "1", and the README documents
// them as one table. A default-ON service that invented its own spelling — an
// ENABLE_* name, or a truthy-string parse accepting "false"/"off" — would be found
// by nobody: an operator hunting for the off switch during an incident scans the
// DISABLE_* table, and an ENABLE_* name reads as "not on yet" even when it is.
//
// This exists as a shared function rather than an inline os.Getenv because a kill
// switch is typically consulted from BOTH halves of the thing it gates — the
// producer and the consumer, which usually sit in different packages. Two
// hand-rolled parses eventually disagree, and then one half is writing work the
// other never drains.
func EnvServiceDisabled(key string) bool {
	return strings.TrimSpace(os.Getenv(key)) == "1"
}

// EnvPositiveInt returns os.Getenv(key) parsed as a positive integer, or def when the
// variable is unset or not a positive integer. Used by background services for their
// batch-size / count tuning knobs.
func EnvPositiveInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// EnvPositiveDuration returns os.Getenv(key) parsed as a positive integer count of
// `unit` (e.g. unit=time.Hour means the variable holds a number of hours), or def when
// the variable is unset, not a positive integer, or too large to represent. Used by
// background services for their interval / re-attempt-window tuning knobs.
//
// The overflow guard is the non-obvious part, and it is a correctness issue rather
// than tidiness. time.Duration is int64 NANOSECONDS, so it wraps at ~292 years:
// `time.Duration(99999999) * time.Hour` is not a very long time, it is
// NEGATIVE (-2481912h). A bare `n > 0` check passes such a value straight
// through, and a negative duration inverts whatever it was meant to bound — a
// "wait at least this long" comparison becomes always-true, and bound into SQL as
// `NOW() - make_interval(secs => negative)` it moves the cutoff into the future.
//
// The realistic path here is an operator typing an intentionally huge number to
// mean "effectively never", which is exactly when silently getting the opposite is
// most damaging. Falling back to the default and saying so is the only safe
// reading of an unrepresentable value.
func EnvPositiveDuration(key string, unit, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if unit > 0 && int64(n) > math.MaxInt64/int64(unit) {
				slog.Default().Warn("env duration is too large to represent — using default",
					"key", key,
					"value", v,
					"unit", unit,
					"using", def,
				)
				return def
			}
			return time.Duration(n) * unit
		}
	}
	return def
}
