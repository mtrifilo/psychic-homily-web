package catalog

import (
	"testing"
	"time"
)

// PSY-1270: the env-config helpers. Each must return the default for unset,
// unparseable, and out-of-range input; the positive vs non-negative variants differ
// only on whether 0 is accepted.

func TestEnvPositiveInt(t *testing.T) {
	const name = "PSY1270_TEST_POS_INT"
	cases := []struct {
		name, env string
		def, want int
	}{
		{"unset uses default", "", 5, 5},
		{"valid positive wins", "12", 5, 12},
		{"zero is invalid → default", "0", 5, 5},
		{"negative → default", "-3", 5, 5},
		{"garbage → default", "x", 5, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(name, tc.env)
			if got := envPositiveInt(name, tc.def); got != tc.want {
				t.Errorf("envPositiveInt(%q=%q, def=%d) = %d, want %d", name, tc.env, tc.def, got, tc.want)
			}
		})
	}
}

func TestEnvNonNegativeInt(t *testing.T) {
	const name = "PSY1270_TEST_NONNEG_INT"
	cases := []struct {
		name, env string
		def, want int
	}{
		{"unset uses default", "", 7, 7},
		{"valid positive wins", "30", 7, 30},
		{"zero is accepted (disable)", "0", 7, 0},
		{"negative → default", "-1", 7, 7},
		{"garbage → default", "soon", 7, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(name, tc.env)
			if got := envNonNegativeInt(name, tc.def); got != tc.want {
				t.Errorf("envNonNegativeInt(%q=%q, def=%d) = %d, want %d", name, tc.env, tc.def, got, tc.want)
			}
		})
	}
}

func TestEnvPositiveHours(t *testing.T) {
	const name = "PSY1270_TEST_POS_HOURS"
	def := 6 * time.Hour
	cases := []struct {
		name, env string
		want      time.Duration
	}{
		{"unset uses default", "", def},
		{"valid positive wins (hours→Duration)", "12", 12 * time.Hour},
		{"zero is invalid → default", "0", def},
		{"negative → default", "-2", def},
		{"garbage → default", "noon", def},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(name, tc.env)
			if got := envPositiveHours(name, def); got != tc.want {
				t.Errorf("envPositiveHours(%q=%q) = %v, want %v", name, tc.env, got, tc.want)
			}
		})
	}
}
