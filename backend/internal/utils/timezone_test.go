package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetTimezoneForState_KnownStates(t *testing.T) {
	assert.Equal(t, "America/Phoenix", GetTimezoneForState("AZ"))
	assert.Equal(t, "America/Los_Angeles", GetTimezoneForState("CA"))
	assert.Equal(t, "America/Los_Angeles", GetTimezoneForState("NV"))
	assert.Equal(t, "America/Denver", GetTimezoneForState("CO"))
	assert.Equal(t, "America/Denver", GetTimezoneForState("NM"))
	assert.Equal(t, "America/Chicago", GetTimezoneForState("IL"))
	assert.Equal(t, "America/Chicago", GetTimezoneForState("TX"))
	assert.Equal(t, "America/New_York", GetTimezoneForState("NY"))
}

// The map must cover all 50 states + DC (PSY-987/PSY-1009). A short map that
// defaulted unmapped states to Phoenix would reintroduce the venue-timezone
// backfill corruption (a correctly-stored explicit-time show reading as a false
// 20:00 Phoenix default). Spot-check representative states across every zone.
func TestGetTimezoneForState_FullMapCoverage(t *testing.T) {
	cases := map[string]string{
		"WA": "America/Los_Angeles",          // Pacific
		"OR": "America/Los_Angeles",          // Pacific
		"ID": "America/Boise",                // Mountain (distinct zone)
		"MT": "America/Denver",               // Mountain
		"UT": "America/Denver",               // Mountain
		"WI": "America/Chicago",              // Central
		"TN": "America/Chicago",              // Central
		"MO": "America/Chicago",              // Central
		"IN": "America/Indiana/Indianapolis", // Eastern-ish (distinct zone)
		"FL": "America/New_York",             // Eastern
		"GA": "America/New_York",             // Eastern
		"MA": "America/New_York",             // Eastern
		"MI": "America/New_York",             // Eastern
		"DC": "America/New_York",             // Eastern (district)
		"KY": "America/New_York",             // split-state, predominant Eastern
		"AK": "America/Anchorage",            // non-contiguous
		"HI": "Pacific/Honolulu",             // non-contiguous
	}
	for state, want := range cases {
		assert.Equalf(t, want, GetTimezoneForState(state), "state %s", state)
	}
	// All 50 states + DC must resolve to a real (non-default) entry.
	assert.Len(t, StateTimezones, 51, "expected 50 states + DC")
}

func TestGetTimezoneForState_CaseInsensitive(t *testing.T) {
	assert.Equal(t, "America/Phoenix", GetTimezoneForState("az"))
	assert.Equal(t, "America/Los_Angeles", GetTimezoneForState("ca"))
	assert.Equal(t, "America/Phoenix", GetTimezoneForState("Az"))
}

func TestGetTimezoneForState_UnknownDefaultsToPhoenix(t *testing.T) {
	assert.Equal(t, "America/Phoenix", GetTimezoneForState("XX"))
	assert.Equal(t, "America/Phoenix", GetTimezoneForState(""))
}

func TestEventLocation(t *testing.T) {
	ptr := func(s string) *string { return &s }
	// 8 PM Central on Jul 9 is stored as 01:00Z on Jul 10.
	instant := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)

	t.Run("explicit venue timezone wins over state", func(t *testing.T) {
		// Venue tz = Chicago, state = NY (Eastern): must use Chicago.
		got := instant.In(EventLocation(ptr("America/Chicago"), "NY")).Format("Jan 2, 2006 3:04 PM")
		assert.Equal(t, "Jul 9, 2026 8:00 PM", got)
	})

	t.Run("state fallback when no venue timezone", func(t *testing.T) {
		assert.Equal(t, "America/Chicago", EventLocation(nil, "TX").String())
		assert.Equal(t, "America/New_York", EventLocation(ptr(""), "NY").String())
	})

	t.Run("malformed venue timezone falls through to state, not UTC", func(t *testing.T) {
		// A bad IANA string must NOT collapse to UTC when a valid state exists.
		assert.Equal(t, "America/Chicago", EventLocation(ptr("Mars/Olympus"), "TX").String())
	})

	t.Run("unknown state defaults to Phoenix", func(t *testing.T) {
		assert.Equal(t, "America/Phoenix", EventLocation(nil, "").String())
		assert.Equal(t, "America/Phoenix", EventLocation(nil, "ZZ").String())
	})
}
