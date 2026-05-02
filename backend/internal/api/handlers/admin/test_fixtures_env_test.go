package handlers

import (
	"strings"
	"testing"
)

// envFromMap is a test helper that returns an os.Getenv-compatible function
// backed by an in-memory map. Avoids touching the real process env.
func envFromMap(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func TestIsTestFixturesEnabled(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		want    bool
	}{
		{"flag unset", map[string]string{}, false},
		{"flag empty", map[string]string{"ENABLE_TEST_FIXTURES": ""}, false},
		{"flag zero", map[string]string{"ENABLE_TEST_FIXTURES": "0"}, false},
		{"flag truthy-but-not-1", map[string]string{"ENABLE_TEST_FIXTURES": "true"}, false},
		{"flag exactly 1", map[string]string{"ENABLE_TEST_FIXTURES": "1"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsTestFixturesEnabled(envFromMap(tc.env))
			if got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestValidateTestFixturesEnvironment(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		wantError bool
		errIncludes string
	}{
		// Safe: flag off means no check
		{"flag off / env production", map[string]string{"ENVIRONMENT": "production"}, false, ""},
		{"flag off / env unset", map[string]string{}, false, ""},
		{"flag off-explicit / env production", map[string]string{"ENABLE_TEST_FIXTURES": "0", "ENVIRONMENT": "production"}, false, ""},

		// Safe: flag on in allowed envs
		{"flag on / env test", map[string]string{"ENABLE_TEST_FIXTURES": "1", "ENVIRONMENT": "test"}, false, ""},
		{"flag on / env ci", map[string]string{"ENABLE_TEST_FIXTURES": "1", "ENVIRONMENT": "ci"}, false, ""},
		{"flag on / env development", map[string]string{"ENABLE_TEST_FIXTURES": "1", "ENVIRONMENT": "development"}, false, ""},

		// Unsafe: flag on in non-allowed envs (production, staging, preview, unset)
		{"flag on / env production", map[string]string{"ENABLE_TEST_FIXTURES": "1", "ENVIRONMENT": "production"}, true, "production"},
		{"flag on / env staging", map[string]string{"ENABLE_TEST_FIXTURES": "1", "ENVIRONMENT": "staging"}, true, "staging"},
		{"flag on / env preview", map[string]string{"ENABLE_TEST_FIXTURES": "1", "ENVIRONMENT": "preview"}, true, "preview"},
		{"flag on / env unset", map[string]string{"ENABLE_TEST_FIXTURES": "1"}, true, ""},
		{"flag on / env typo (Test uppercase)", map[string]string{"ENABLE_TEST_FIXTURES": "1", "ENVIRONMENT": "Test"}, true, "Test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTestFixturesEnvironment(envFromMap(tc.env))
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errIncludes != "" && !strings.Contains(err.Error(), tc.errIncludes) {
					t.Errorf("error %q missing %q", err.Error(), tc.errIncludes)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}
