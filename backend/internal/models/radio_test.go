package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.False(t, IsValidBroadcastType("Both"))       // case-sensitive
	assert.False(t, IsValidBroadcastType("TERRESTRIAL")) // case-sensitive
	assert.False(t, IsValidBroadcastType("satellite"))
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
