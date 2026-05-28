package catalog

import (
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// nameNormalizer strips combining marks after NFKD decomposition. Constructed
// once and reused; the runes filter is stateless and the transform.Chain
// itself is safe to reuse across calls (each String invocation creates its
// own state).
var nameNormalizer = transform.Chain(
	norm.NFKD,
	runes.Remove(runes.In(unicode.Mn)),
	norm.NFC,
)

// normalizeName produces the canonical form used for radio-matching name
// lookups. The pipeline:
//
//  1. NFKD-decompose and strip combining marks  ("José" → "Jose")
//  2. lowercase                                 ("Jose" → "jose")
//  3. trim leading/trailing non-letter/-digit   ("the who!" → "the who")
//  4. collapse interior whitespace runs         ("the   who" → "the who")
//
// Interior punctuation is intentionally preserved so distinct names like
// "AC/DC" and "ACDC", or "The The" and "The", still compare unequal.
//
// The DB side mirrors this with `immutable_unaccent(LOWER(col))` against an
// expression index; immutable_unaccent is a SQL wrapper marked IMMUTABLE so
// it can be used in indexes (the contrib `unaccent` is only STABLE). The
// Go-side trim/collapse covers radio-feed noise (trailing "!", double
// spaces) that PostgreSQL's `unaccent` does not.
func normalizeName(s string) string {
	if s == "" {
		return ""
	}
	folded, _, err := transform.String(nameNormalizer, s)
	if err != nil {
		// transform.String only errors when the chain itself errors; norm /
		// runes.Remove cannot fail on valid UTF-8, but if Go ever sees
		// invalid input we fall back to the original string so a partial
		// normalization never silently corrupts the lookup.
		folded = s
	}
	folded = strings.ToLower(folded)
	folded = strings.TrimFunc(folded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	// Collapse interior whitespace runs into a single ASCII space. After
	// TrimFunc the boundaries are clean, so any whitespace seen here is
	// interior. Tabs / NBSP / multiple spaces all flatten to one ' '.
	if !needsWhitespaceNormalize(folded) {
		return folded
	}
	var b strings.Builder
	b.Grow(len(folded))
	prevSpace := false
	for _, r := range folded {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return b.String()
}

// needsWhitespaceNormalize reports whether the interior-collapse rewrite
// would change s. Two cases force a rewrite: any non-' ' whitespace rune
// (tab / NBSP / etc.) or two consecutive whitespace runes.
func needsWhitespaceNormalize(s string) bool {
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if r != ' ' || prevSpace {
				return true
			}
			prevSpace = true
			continue
		}
		prevSpace = false
	}
	return false
}

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
		if err := m.db.Model(play).Updates(updates).Error; err != nil {
			// The match did not persist, so report the play as unmatched rather
			// than over-counting it; a swallowed error here left plays silently
			// unmatched in the DB while the caller recorded a match.
			slog.Error("radio match: failed to persist play match",
				"play_id", play.ID,
				"artist_name", play.ArtistName,
				"error", err)
			return false
		}
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

	// 2. Exact name match (diacritic- and case-insensitive).
	//    Uses the `idx_artists_name_unaccent_lower` expression index (PSY-886).
	normalized := normalizeName(name)
	if normalized == "" {
		return nil
	}

	var artist catalogm.Artist
	if err := m.db.Where("immutable_unaccent(LOWER(name)) = immutable_unaccent(LOWER(?))", normalized).First(&artist).Error; err == nil {
		return &artist.ID
	}

	// 3. Alias match (diacritic- and case-insensitive).
	var alias catalogm.ArtistAlias
	if err := m.db.Where("immutable_unaccent(LOWER(alias)) = immutable_unaccent(LOWER(?))", normalized).First(&alias).Error; err == nil {
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
	normalized := normalizeName(*title)
	if normalized == "" {
		return nil
	}

	var release catalogm.Release
	if err := m.db.Where("immutable_unaccent(LOWER(title)) = immutable_unaccent(LOWER(?))", normalized).First(&release).Error; err == nil {
		return &release.ID
	}

	return nil
}

// matchLabel tries to match a label by exact name (case-insensitive).
func (m *RadioMatchingEngine) matchLabel(name *string) *uint {
	if name == nil || *name == "" {
		return nil
	}
	normalized := normalizeName(*name)
	if normalized == "" {
		return nil
	}

	var label catalogm.Label
	if err := m.db.Where("immutable_unaccent(LOWER(name)) = immutable_unaccent(LOWER(?))", normalized).First(&label).Error; err == nil {
		return &label.ID
	}

	return nil
}
