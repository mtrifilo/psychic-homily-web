package config

import "testing"

// The retired-link notice is off unless the environment turns it on; a value
// that is not a boolean leaves it off.
func TestLoadCalendarRetiredLinkNoticeSwitch(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"false", false},
		{"not-a-bool", false},
		{"true", true},
		{"1", true},
	}
	for _, c := range cases {
		t.Run("value "+c.value, func(t *testing.T) {
			t.Setenv(EnvEnvironment, EnvDevelopment)
			t.Setenv(EnvDatabaseURL, "")
			t.Setenv(EnvCalendarRetiredLinkNoticeEnabled, c.value)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() returned error: %v", err)
			}
			if cfg.Calendar.RetiredLinkNoticeEnabled != c.want {
				t.Errorf("%s=%q: RetiredLinkNoticeEnabled = %v, want %v",
					EnvCalendarRetiredLinkNoticeEnabled, c.value, cfg.Calendar.RetiredLinkNoticeEnabled, c.want)
			}
		})
	}
}
