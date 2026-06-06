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
