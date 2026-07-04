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

// markStripper is the runes filter used to drop combining marks after NFKD
// decomposition. It's stateless and safe to share across calls.
//
// The transform.Chain wrapper that combines NFKD → markStripper → NFC is
// NOT shared, however — `chain` keeps mutable per-call state (link buffers
// + position counters), so two goroutines using one Chain instance would
// race. Two radio background loops (runFetchLoop + runReMatchLoop) plus
// admin-HTTP-triggered imports satisfy that scenario, so we construct a
// fresh chain on each normalizeName call. Construction is cheap (three
// transformer values + a small slice).
var markStripper = runes.Remove(runes.In(unicode.Mn))

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
	// Per-call chain — see markStripper comment for why this isn't shared.
	normalizer := transform.Chain(norm.NFKD, markStripper, norm.NFC)
	folded, _, err := transform.String(normalizer, s)
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

	return m.matchPlays(plays)
}

// matchPlays runs the matcher over a pre-loaded slice of plays.
func (m *RadioMatchingEngine) matchPlays(plays []catalogm.RadioPlay) (*contracts.MatchResult, error) {
	result := &contracts.MatchResult{
		Total: len(plays),
	}

	for i := range plays {
		matched, persistErr := m.matchPlayWithErr(&plays[i])
		if matched {
			result.Matched++
		} else {
			result.Unmatched++
		}
		if persistErr != nil {
			result.PersistErrors++
		}
	}

	return result, nil
}

// MatchUnmatchedPlaysForArtistName rematches only unmatched plays whose
// artist_name normalizes to the given name (same predicate as matchArtist).
func (m *RadioMatchingEngine) MatchUnmatchedPlaysForArtistName(name string) (*contracts.MatchResult, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	normalized := normalizeName(name)
	if normalized == "" {
		return &contracts.MatchResult{}, nil
	}

	var plays []catalogm.RadioPlay
	err := m.db.Where(
		"artist_id IS NULL AND immutable_unaccent(LOWER(artist_name)) = immutable_unaccent(LOWER(?))",
		normalized,
	).Find(&plays).Error
	if err != nil {
		return nil, fmt.Errorf("loading unmatched plays for artist %q: %w", name, err)
	}

	return m.matchPlays(plays)
}

// MatchUnmatchedPlaysForLabelName rematches only unmatched plays whose
// label_name normalizes to the given name (same predicate as matchLabel).
func (m *RadioMatchingEngine) MatchUnmatchedPlaysForLabelName(name string) (*contracts.MatchResult, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	normalized := normalizeName(name)
	if normalized == "" {
		return &contracts.MatchResult{}, nil
	}

	var plays []catalogm.RadioPlay
	err := m.db.Where(
		"label_id IS NULL AND label_name IS NOT NULL AND immutable_unaccent(LOWER(label_name)) = immutable_unaccent(LOWER(?))",
		normalized,
	).Find(&plays).Error
	if err != nil {
		return nil, fmt.Errorf("loading unmatched plays for label %q: %w", name, err)
	}

	return m.matchPlays(plays)
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

	return m.matchPlays(plays)
}

// matchPlay attempts to match a single play to entities in our knowledge graph.
// Returns true if at least the artist was matched. Updates the play record in
// the DB. A persist failure is reported as not-matched (the play is genuinely
// unmatched on disk) — use matchPlayWithErr when the caller needs to count
// persist failures separately (PSY-1119).
func (m *RadioMatchingEngine) matchPlay(play *catalogm.RadioPlay) bool {
	matched, _ := m.matchPlayWithErr(play)
	return matched
}

// matchPlayWithErr is the persist-error-aware form of matchPlay. It returns
// (artistMatched, persistErr). When the update fails, artistMatched is false
// (the match did not reach disk) AND persistErr is non-nil so the caller can
// distinguish "computed a match but couldn't save it" from "no match found" —
// a distinction that previously lived only in logs (PSY-1119, building on the
// PSY-814 fix that already stopped over-counting these as matched).
func (m *RadioMatchingEngine) matchPlayWithErr(play *catalogm.RadioPlay) (bool, error) {
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
			return false, err
		}
	}

	return artistMatched, nil
}

// matchArtist tries to match an artist name to our knowledge graph.
// Priority: MusicBrainz ID → exact name → alias match → collab split (PSY-1353).
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

	if id := m.matchArtistByNameOrAlias(name); id != nil {
		return id
	}
	return m.matchArtistByCollabParts(name)
}

// matchArtistByNameOrAlias matches a single artist credit by exact normalized name
// or alias. Does not split collab strings.
func (m *RadioMatchingEngine) matchArtistByNameOrAlias(name string) *uint {
	normalized := normalizeName(name)
	if normalized == "" {
		return nil
	}

	var artist catalogm.Artist
	if err := m.db.Where("immutable_unaccent(LOWER(name)) = immutable_unaccent(LOWER(?))", normalized).First(&artist).Error; err == nil {
		return &artist.ID
	}

	var alias catalogm.ArtistAlias
	if err := m.db.Where("immutable_unaccent(LOWER(alias)) = immutable_unaccent(LOWER(?))", normalized).First(&alias).Error; err == nil {
		return &alias.ArtistID
	}

	return nil
}

// matchArtistByCollabParts splits WFMU-style collab credits and links when
// exactly one segment resolves to an artist. When multiple distinct artists
// match, the play stays unmatched (true duet billing — no guess).
func (m *RadioMatchingEngine) matchArtistByCollabParts(name string) *uint {
	parts := splitCollabArtistName(name)
	if len(parts) < 2 {
		return nil
	}

	var matched []uint
	for _, part := range parts {
		if id := m.matchArtistByNameOrAlias(part); id != nil {
			matched = appendUniqueUint(matched, *id)
		}
	}
	if len(matched) != 1 {
		return nil
	}
	return &matched[0]
}

func appendUniqueUint(ids []uint, id uint) []uint {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
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
