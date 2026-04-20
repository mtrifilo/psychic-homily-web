package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func envFromMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestIsAuthRateLimitDisabled(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"unset", map[string]string{}, false},
		{"empty", map[string]string{"DISABLE_AUTH_RATE_LIMITS": ""}, false},
		{"zero", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "0"}, false},
		{"true-string", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "true"}, false},
		{"exactly-1", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "1"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAuthRateLimitDisabled(envFromMap(tc.env)); got != tc.want {
				t.Errorf("want %v got %v", tc.want, got)
			}
		})
	}
}

func TestValidateAuthRateLimitEnvironment(t *testing.T) {
	cases := []struct {
		name        string
		env         map[string]string
		wantError   bool
		errContains string
	}{
		// Flag off = always safe
		{"flag-off / env-unset", map[string]string{}, false, ""},
		{"flag-off / env-production", map[string]string{"ENVIRONMENT": "production"}, false, ""},
		{"flag-0 / env-production", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "0", "ENVIRONMENT": "production"}, false, ""},

		// Flag on + allowed env = safe
		{"flag-on / env-test", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "1", "ENVIRONMENT": "test"}, false, ""},
		{"flag-on / env-ci", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "1", "ENVIRONMENT": "ci"}, false, ""},
		{"flag-on / env-development", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "1", "ENVIRONMENT": "development"}, false, ""},

		// Flag on + not-allowed env = refuse
		{"flag-on / env-production", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "1", "ENVIRONMENT": "production"}, true, "production"},
		{"flag-on / env-stage", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "1", "ENVIRONMENT": "stage"}, true, "stage"},
		{"flag-on / env-preview", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "1", "ENVIRONMENT": "preview"}, true, "preview"},
		{"flag-on / env-unset", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "1"}, true, ""},
		{"flag-on / env-casing", map[string]string{"DISABLE_AUTH_RATE_LIMITS": "1", "ENVIRONMENT": "Test"}, true, "Test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAuthRateLimitEnvironment(envFromMap(tc.env))
			if tc.wantError {
				if err == nil {
					t.Fatal("want error got nil")
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

// TestNoopRateLimiter_PassesThrough asserts the no-op middleware doesn't
// block or modify requests — it just forwards to next.
func TestNoopRateLimiter_PassesThrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := noopRateLimiter()(next)

	// Fire 100 sequential requests; none should be blocked.
	for i := 0; i < 100; i++ {
		called = false
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		wrapped.ServeHTTP(w, req)
		if !called {
			t.Fatalf("request %d: next handler not invoked", i)
		}
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: want 200 got %d", i, w.Code)
		}
	}
}
