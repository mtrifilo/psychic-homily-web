package shared

import "testing"

func TestEnvFlagEnabledIsOnOnlyForOne(t *testing.T) {
	const key = "PSY_TEST_ENV_FLAG_ENABLED"
	cases := []struct {
		value string
		want  bool
	}{
		{value: "", want: false},
		{value: "1", want: true},
		{value: " 1\n", want: true},
		{value: "0", want: false},
		{value: "true", want: false},
		{value: "yes", want: false},
		{value: "on", want: false},
		{value: "11", want: false},
	}
	for _, tc := range cases {
		t.Setenv(key, tc.value)
		if got := EnvFlagEnabled(key); got != tc.want {
			t.Errorf("EnvFlagEnabled(%q) = %v, want %v", tc.value, got, tc.want)
		}
		if got := EnvServiceDisabled(key); got != tc.want {
			t.Errorf("EnvServiceDisabled(%q) = %v, want %v: the two must share one parse", tc.value, got, tc.want)
		}
	}
}

func TestEnvPositiveIntFallsBackForZeroNegativeAndMalformed(t *testing.T) {
	const key = "PSY_TEST_ENV_POSITIVE_INT"
	cases := []struct {
		value string
		want  int
	}{
		{value: "", want: 7},
		{value: "0", want: 7},
		{value: "-30", want: 7},
		{value: "abc", want: 7},
		{value: "30.5", want: 7},
		{value: "30", want: 30},
		{value: " 30\n", want: 30},
		{value: "30 days", want: 7},
	}
	for _, tc := range cases {
		t.Setenv(key, tc.value)
		if got := EnvPositiveInt(key, 7); got != tc.want {
			t.Errorf("EnvPositiveInt(%q, 7) = %d, want %d", tc.value, got, tc.want)
		}
	}
}
