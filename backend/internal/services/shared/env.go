package shared

import (
	"os"
	"strconv"
	"time"
)

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
// the variable is unset or not a positive integer. Used by background services for
// their interval / re-attempt-window tuning knobs.
func EnvPositiveDuration(key string, unit, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * unit
		}
	}
	return def
}
