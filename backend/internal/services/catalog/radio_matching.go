package catalog

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// RadioMatchingEngine handles matching radio plays to entities in our knowledge graph.
// It is provider-agnostic and runs after plays are imported.
type RadioMatchingEngine struct {
	db *gorm.DB
}

// NewRadioMatchingEngine creates a new matching engine.
func NewRadioMatchingEngine(db *gorm.DB) *RadioMatchingEngine {
	return &RadioMatchingEngine{db: db}
}

// MatchPlaysForEpisode runs the matching engine on all unmatched plays for an episode.
// Returns a MatchResult summarizing how many were matched.
func (m *RadioMatchingEngine) MatchPlaysForEpisode(episodeID uint) (*contracts.MatchResult, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var plays []catalogm.RadioPlay
	err := m.db.Where("episode_id = ? AND artist_id IS NULL", episodeID).Find(&plays).Error
	if err != nil {
		return nil, fmt.Errorf("loading unmatched plays: %w", err)
	}

	result := &contracts.MatchResult{
		Total: len(plays),
	}

	for i := range plays {
		matched := m.matchPlay(&plays[i])
		if matched {
			result.Matched++
		} else {
			result.Unmatched++
		}
	}

	return result, nil
}

// MatchAllUnmatched runs the matching engine on all unmatched plays in the database.
func (m *RadioMatchingEngine) MatchAllUnmatched() (*contracts.MatchResult, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var plays []catalogm.RadioPlay
	err := m.db.Where("artist_id IS NULL").Find(&plays).Error
	if err != nil {
		return nil, fmt.Errorf("loading unmatched plays: %w", err)
	}

	result := &contracts.MatchResult{
		Total: len(plays),
	}

	for i := range plays {
		matched := m.matchPlay(&plays[i])
		if matched {
			result.Matched++
		} else {
			result.Unmatched++
		}
	}

	return result, nil
}

// matchPlay attempts to match a single play to entities in our knowledge graph.
// Returns true if at least the artist was matched. Updates the play record in the DB.
func (m *RadioMatchingEngine) matchPlay(play *catalogm.RadioPlay) bool {
	updates := make(map[string]interface{})
	artistMatched := false

	// 1. Match artist
	if artistID := m.matchArtist(play.ArtistName, play.MusicBrainzArtistID); artistID != nil {
		updates["artist_id"] = *artistID
		artistMatched = true
	}

	// 2. Match release (by MB ID or exact title match)
	if releaseID := m.matchRelease(play.AlbumTitle, play.MusicBrainzReleaseID); releaseID != nil {
		updates["release_id"] = *releaseID
	}

	// 3. Match label (exact name match)
	if labelID := m.matchLabel(play.LabelName); labelID != nil {
		updates["label_id"] = *labelID
	}

	if len(updates) > 0 {
		m.db.Model(play).Updates(updates)
	}

	return artistMatched
}

// matchArtist tries to match an artist name to our knowledge graph.
// Priority: MusicBrainz ID → exact name → alias match.
func (m *RadioMatchingEngine) matchArtist(name string, mbID *string) *uint {
	// 1. MusicBrainz ID match (highest confidence)
	// Note: Artists table doesn't have a musicbrainz_id column yet,
	// so we skip this path. When the column exists, uncomment:
	// if mbID != nil && *mbID != "" {
	// 	var artist catalogm.Artist
	// 	if err := m.db.Where("musicbrainz_id = ?", *mbID).First(&artist).Error; err == nil {
	// 		return &artist.ID
	// 	}
	// }

	// 2. Exact name match (case-insensitive)
	var artist catalogm.Artist
	if err := m.db.Where("LOWER(name) = LOWER(?)", strings.TrimSpace(name)).First(&artist).Error; err == nil {
		return &artist.ID
	}

	// 3. Alias match (case-insensitive)
	var alias catalogm.ArtistAlias
	if err := m.db.Where("LOWER(alias) = LOWER(?)", strings.TrimSpace(name)).First(&alias).Error; err == nil {
		return &alias.ArtistID
	}

	return nil
}

// matchRelease tries to match a release by MusicBrainz ID or exact title.
func (m *RadioMatchingEngine) matchRelease(title *string, mbID *string) *uint {
	// Note: Releases table doesn't have a musicbrainz_id column yet.
	// When it exists, add MB ID matching here.

	if title == nil || *title == "" {
		return nil
	}

	var release catalogm.Release
	if err := m.db.Where("LOWER(title) = LOWER(?)", strings.TrimSpace(*title)).First(&release).Error; err == nil {
		return &release.ID
	}

	return nil
}

// matchLabel tries to match a label by exact name (case-insensitive).
func (m *RadioMatchingEngine) matchLabel(name *string) *uint {
	if name == nil || *name == "" {
		return nil
	}

	var label catalogm.Label
	if err := m.db.Where("LOWER(name) = LOWER(?)", strings.TrimSpace(*name)).First(&label).Error; err == nil {
		return &label.ID
	}

	return nil
}
