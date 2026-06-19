package catalog

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TableName Tests
// =============================================================================

func TestRadioStationTableName(t *testing.T) {
	assert.Equal(t, "radio_stations", RadioStation{}.TableName())
}

func TestRadioShowTableName(t *testing.T) {
	assert.Equal(t, "radio_shows", RadioShow{}.TableName())
}

func TestRadioEpisodeTableName(t *testing.T) {
	assert.Equal(t, "radio_episodes", RadioEpisode{}.TableName())
}

func TestRadioPlayTableName(t *testing.T) {
	assert.Equal(t, "radio_plays", RadioPlay{}.TableName())
}

func TestRadioArtistAffinityTableName(t *testing.T) {
	assert.Equal(t, "radio_artist_affinity", RadioArtistAffinity{}.TableName())
}

func TestRadioNetworkTableName(t *testing.T) {
	assert.Equal(t, "radio_networks", RadioNetwork{}.TableName())
}

// =============================================================================
// Broadcast Type Validation Tests
// =============================================================================

func TestIsValidBroadcastType_Valid(t *testing.T) {
	for _, bt := range BroadcastTypes {
		assert.True(t, IsValidBroadcastType(bt), "expected %q to be valid", bt)
	}
}

func TestIsValidBroadcastType_Invalid(t *testing.T) {
	assert.False(t, IsValidBroadcastType(""))
	assert.False(t, IsValidBroadcastType("invalid"))
	assert.False(t, IsValidBroadcastType("Both"))        // case-sensitive
	assert.False(t, IsValidBroadcastType("TERRESTRIAL")) // case-sensitive
	assert.False(t, IsValidBroadcastType("satellite"))
}

func TestIsValidPlaylistSource_Valid(t *testing.T) {
	for _, ps := range PlaylistSources {
		assert.True(t, IsValidPlaylistSource(ps), "expected %q to be valid", ps)
	}
	// Empty means "no automated provider" (link-only) and is accepted.
	assert.True(t, IsValidPlaylistSource(""))
}

func TestIsValidPlaylistSource_Invalid(t *testing.T) {
	// PSY-927: 'wfmu_html' was the runtime bad value that silently broke WFMU
	// playlist import — exactly what this validation now rejects.
	assert.False(t, IsValidPlaylistSource("wfmu_html"))
	assert.False(t, IsValidPlaylistSource("invalid"))
	assert.False(t, IsValidPlaylistSource("WFMU_SCRAPE")) // case-sensitive
	assert.False(t, IsValidPlaylistSource("kexp"))
}

// =============================================================================
// Broadcast Type Constants Tests
// =============================================================================

func TestBroadcastTypeConstants(t *testing.T) {
	assert.Equal(t, "terrestrial", BroadcastTypeTerrestrial)
	assert.Equal(t, "internet", BroadcastTypeInternet)
	assert.Equal(t, "both", BroadcastTypeBoth)
	assert.Len(t, BroadcastTypes, 3)
}

// =============================================================================
// Playlist Source Constants Tests
// =============================================================================

func TestPlaylistSourceConstants(t *testing.T) {
	assert.Equal(t, "kexp_api", PlaylistSourceKEXP)
	assert.Equal(t, "nts_api", PlaylistSourceNTS)
	assert.Equal(t, "wfmu_scrape", PlaylistSourceWFMU)
	assert.Equal(t, "manual", PlaylistSourceManual)
}

// =============================================================================
// Rotation Status Constants Tests
// =============================================================================

func TestRotationStatusConstants(t *testing.T) {
	assert.Equal(t, "heavy", RotationStatusHeavy)
	assert.Equal(t, "medium", RotationStatusMedium)
	assert.Equal(t, "light", RotationStatusLight)
	assert.Equal(t, "recommended_new", RotationStatusRecommendedNew)
	assert.Equal(t, "library", RotationStatusLibrary)
}

// =============================================================================
// PSY-1131 enum validators
// =============================================================================

func TestIsValidRotationStatus(t *testing.T) {
	// "" is valid (no rotation supplied -> NULL column).
	for _, v := range []string{"", "heavy", "medium", "light", "recommended_new", "library"} {
		assert.True(t, IsValidRotationStatus(v), "expected %q valid", v)
	}
	for _, v := range []string{"Heavy", "rotation", "HEAVY", "none"} {
		assert.False(t, IsValidRotationStatus(v), "expected %q invalid", v)
	}
}

func TestIsValidRadioStationSource(t *testing.T) {
	for _, v := range []string{"canonical", "discovered", "manual"} {
		assert.True(t, IsValidRadioStationSource(v))
	}
	for _, v := range []string{"", "provider", "Canonical", "seed"} {
		assert.False(t, IsValidRadioStationSource(v))
	}
}

func TestIsValidRadioShowSource(t *testing.T) {
	for _, v := range []string{"provider", "manual"} {
		assert.True(t, IsValidRadioShowSource(v))
	}
	// "canonical" is intentionally NOT a valid show source.
	for _, v := range []string{"", "canonical", "discovered", "Provider"} {
		assert.False(t, IsValidRadioShowSource(v))
	}
}

func TestIsValidRadioLifecycleState(t *testing.T) {
	for _, v := range []string{"active", "dormant", "retired"} {
		assert.True(t, IsValidRadioLifecycleState(v))
	}
	for _, v := range []string{"", "inactive", "Active", "deleted"} {
		assert.False(t, IsValidRadioLifecycleState(v))
	}
}

func TestIsValidRadioEpisodeStatus(t *testing.T) {
	for _, v := range []string{"scheduled", "live", "aired", "archived"} {
		assert.True(t, IsValidRadioEpisodeStatus(v))
	}
	for _, v := range []string{"", "on_air", "Live", "done"} {
		assert.False(t, IsValidRadioEpisodeStatus(v))
	}
}

func TestIsValidRadioPlaylistState(t *testing.T) {
	for _, v := range []string{"pending", "partial", "complete", "unavailable"} {
		assert.True(t, IsValidRadioPlaylistState(v))
	}
	for _, v := range []string{"", "done", "Complete", "missing"} {
		assert.False(t, IsValidRadioPlaylistState(v))
	}
}

func TestIsValidRadioPlayMatchState(t *testing.T) {
	for _, v := range []string{"unmatched", "matched", "ambiguous", "no_match"} {
		assert.True(t, IsValidRadioPlayMatchState(v))
	}
	for _, v := range []string{"", "nomatch", "Matched", "unknown"} {
		assert.False(t, IsValidRadioPlayMatchState(v))
	}
}

// =============================================================================
// PSY-1131 RadioSchedule validation
// =============================================================================

func TestRadioSchedule_Validate(t *testing.T) {
	valid := RadioSchedule{
		Timezone: "America/Los_Angeles",
		Slots: []RadioScheduleSlot{
			{DayOfWeek: 1, Start: "06:00", End: "10:00"},
			{DayOfWeek: 6, Start: "23:00", End: "01:00"}, // legal midnight-wrap
		},
	}
	assert.NoError(t, valid.Validate())

	cases := []struct {
		name string
		s    RadioSchedule
	}{
		{"empty timezone", RadioSchedule{Timezone: "  ", Slots: nil}},
		{"unknown timezone", RadioSchedule{Timezone: "Mars/Olympus", Slots: nil}},
		{"day too high", RadioSchedule{Timezone: "UTC", Slots: []RadioScheduleSlot{{DayOfWeek: 7, Start: "06:00", End: "10:00"}}}},
		{"day negative", RadioSchedule{Timezone: "UTC", Slots: []RadioScheduleSlot{{DayOfWeek: -1, Start: "06:00", End: "10:00"}}}},
		{"bad start", RadioSchedule{Timezone: "UTC", Slots: []RadioScheduleSlot{{DayOfWeek: 0, Start: "6:00", End: "10:00"}}}},
		{"bad end hour", RadioSchedule{Timezone: "UTC", Slots: []RadioScheduleSlot{{DayOfWeek: 0, Start: "06:00", End: "24:00"}}}},
		{"bad end minute", RadioSchedule{Timezone: "UTC", Slots: []RadioScheduleSlot{{DayOfWeek: 0, Start: "06:00", End: "10:60"}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Error(t, c.s.Validate())
		})
	}
}

func TestParseRadioSchedule(t *testing.T) {
	// nil / empty / "null" -> (nil, nil): a show need not have a schedule.
	for _, raw := range []*json.RawMessage{nil, rawMsg(""), rawMsg("null")} {
		sched, err := ParseRadioSchedule(raw)
		assert.NoError(t, err)
		assert.Nil(t, sched)
	}

	good := rawMsg(`{"timezone":"Europe/London","slots":[{"day_of_week":3,"start":"10:00","end":"13:00"}]}`)
	sched, err := ParseRadioSchedule(good)
	require.NoError(t, err)
	require.NotNil(t, sched)
	assert.Equal(t, "Europe/London", sched.Timezone)
	require.Len(t, sched.Slots, 1)
	assert.Equal(t, 3, sched.Slots[0].DayOfWeek)

	// Malformed JSON and shape violations both error.
	_, err = ParseRadioSchedule(rawMsg(`{not json`))
	assert.Error(t, err)
	_, err = ParseRadioSchedule(rawMsg(`{"timezone":"","slots":[]}`))
	assert.Error(t, err)
}

func rawMsg(s string) *json.RawMessage {
	r := json.RawMessage(s)
	return &r
}
