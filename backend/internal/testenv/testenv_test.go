package testenv

import (
	"strings"
	"testing"
)

func envFromMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestIsFlagEnabled(t *testing.T) {
	const flag = "SOME_FLAG"
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"unset", map[string]string{}, false},
		{"empty", map[string]string{flag: ""}, false},
		{"zero", map[string]string{flag: "0"}, false},
		{"truthy-but-not-1", map[string]string{flag: "true"}, false},
		{"exactly-1", map[string]string{flag: "1"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsFlagEnabled(flag, envFromMap(tc.env)); got != tc.want {
				t.Errorf("want %v got %v", tc.want, got)
			}
		})
	}
}

func TestIsAllowedEnvironment(t *testing.T) {
	cases := []struct {
		env  string
		want bool
	}{
		{"test", true},
		{"ci", true},
		{"development", true},
		{"production", false},
		{"stage", false},
		{"preview", false},
		{"", false},
		{"Test", false}, // case-sensitive
	}
	for _, tc := range cases {
		t.Run(tc.env, func(t *testing.T) {
			if got := IsAllowedEnvironment(tc.env); got != tc.want {
				t.Errorf("env %q: want %v got %v", tc.env, tc.want, got)
			}
		})
	}
}

func TestValidateFlagEnvironment(t *testing.T) {
	const flag = "SOME_FLAG"
	cases := []struct {
		name        string
		env         map[string]string
		wantError   bool
		errContains string
	}{
		// Flag off = always safe, regardless of ENVIRONMENT.
		{"flag-off / env-unset", map[string]string{}, false, ""},
		{"flag-off / env-production", map[string]string{"ENVIRONMENT": "production"}, false, ""},
		{"flag-0 / env-production", map[string]string{flag: "0", "ENVIRONMENT": "production"}, false, ""},

		// Flag on + allowed env = safe.
		{"flag-on / env-test", map[string]string{flag: "1", "ENVIRONMENT": "test"}, false, ""},
		{"flag-on / env-ci", map[string]string{flag: "1", "ENVIRONMENT": "ci"}, false, ""},
		{"flag-on / env-development", map[string]string{flag: "1", "ENVIRONMENT": "development"}, false, ""},

		// Flag on + non-allowed env = refuse to boot.
		{"flag-on / env-production", map[string]string{flag: "1", "ENVIRONMENT": "production"}, true, "production"},
		{"flag-on / env-stage", map[string]string{flag: "1", "ENVIRONMENT": "stage"}, true, "stage"},
		{"flag-on / env-preview", map[string]string{flag: "1", "ENVIRONMENT": "preview"}, true, "preview"},
		{"flag-on / env-unset", map[string]string{flag: "1"}, true, ""},
		{"flag-on / env-casing", map[string]string{flag: "1", "ENVIRONMENT": "Test"}, true, "Test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFlagEnvironment(flag, envFromMap(tc.env))
			if tc.wantError {
				if err == nil {
					t.Fatal("want error got nil")
				}
				// The error must name the offending flag so misconfig is debuggable.
				if !strings.Contains(err.Error(), flag) {
					t.Errorf("error %q missing flag name %q", err.Error(), flag)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q missing %q", err.Error(), tc.errContains)
				}
			} else if err != nil {
				t.Errorf("want no error got %v", err)
			}
		})
	}
}
