package catalog

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// mbidUUIDPattern is the SQL-side mirror of utils.IsValidMBID — canonical
// 8-4-4-4-12 hex UUID before we trust a play's musicbrainz_artist_id column.
const mbidUUIDPattern = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`

// BulkLinkArtistsResult counts plays linked per deterministic SQL phase (PSY-1365).
// Collab strings and ambiguous exact names are intentionally left for the Go matcher.
type BulkLinkArtistsResult struct {
	MBIDLinked  int64
	NameLinked  int64
	AliasLinked int64
}

// TotalLinked returns the sum across all bulk-link phases.
func (r BulkLinkArtistsResult) TotalLinked() int64 {
	return r.MBIDLinked + r.NameLinked + r.AliasLinked
}

// bulkLinkMBIDSQL links unmatched plays when exactly one artist shares the play's
// MusicBrainz artist MBID. Duplicate MBIDs in artists are skipped (ambiguous).
const bulkLinkMBIDSQL = `
UPDATE radio_plays rp
SET artist_id = sub.artist_id, match_state = 'matched'
FROM (
	SELECT rp2.id AS play_id, MIN(a.id) AS artist_id
	FROM radio_plays rp2
	INNER JOIN artists a ON a.musicbrainz_artist_id = TRIM(rp2.musicbrainz_artist_id)
	WHERE rp2.artist_id IS NULL
		AND rp2.match_state = 'unmatched'
		AND rp2.musicbrainz_artist_id IS NOT NULL
		AND TRIM(rp2.musicbrainz_artist_id) <> ''
		AND TRIM(rp2.musicbrainz_artist_id) ~ '` + mbidUUIDPattern + `'
	GROUP BY rp2.id
	HAVING COUNT(DISTINCT a.id) = 1
) sub
WHERE rp.id = sub.play_id AND rp.artist_id IS NULL
RETURNING rp.id, rp.artist_id, rp.album_title, rp.release_id, rp.release_link_source
`

// bulkLinkExactNameSQL links when radio_normalize_name(play.artist_name)
// equals exactly one artist's canonical name. Two artists sharing the same
// normalized name are skipped (Go .First() would pick arbitrarily; we decline).
const bulkLinkExactNameSQL = `
UPDATE radio_plays rp
SET artist_id = sub.artist_id, match_state = 'matched'
FROM (
	SELECT rp2.id AS play_id, MIN(a.id) AS artist_id
	FROM radio_plays rp2
	INNER JOIN artists a
		ON radio_normalize_name(rp2.artist_name) = radio_normalize_name(a.name)
	WHERE rp2.artist_id IS NULL
		AND rp2.match_state = 'unmatched'
	GROUP BY rp2.id
	HAVING COUNT(DISTINCT a.id) = 1
) sub
WHERE rp.id = sub.play_id AND rp.artist_id IS NULL
RETURNING rp.id, rp.artist_id, rp.album_title, rp.release_id, rp.release_link_source
`

// bulkLinkAliasSQL links when the play artist_name matches exactly one alias row.
// Ambiguous alias collisions (two artists, same normalized alias) are skipped.
const bulkLinkAliasSQL = `
UPDATE radio_plays rp
SET artist_id = sub.artist_id, match_state = 'matched'
FROM (
	SELECT rp2.id AS play_id, MIN(aa.artist_id) AS artist_id
	FROM radio_plays rp2
	INNER JOIN artist_aliases aa
		ON radio_normalize_name(rp2.artist_name) = radio_normalize_name(aa.alias)
	WHERE rp2.artist_id IS NULL
		AND rp2.match_state = 'unmatched'
	GROUP BY rp2.id
	HAVING COUNT(DISTINCT aa.artist_id) = 1
) sub
WHERE rp.id = sub.play_id AND rp.artist_id IS NULL
RETURNING rp.id, rp.artist_id, rp.album_title, rp.release_id, rp.release_link_source
`

// BulkLinkUnmatchedArtistPlays runs MBID → exact name → alias UPDATE passes for
// plays where artist_id IS NULL. Each phase runs in its own transaction, which
// also applies the release credit rule (linkArtistsWithCreditRule) to the
// plays the phase linked.
// Remaining plays (collab billing, punctuation/whitespace noise, ambiguous names)
// are left for matchPlays / ReMatchUnmatchedChunked.
func (m *RadioMatchingEngine) BulkLinkUnmatchedArtistPlays(ctx context.Context) (*BulkLinkArtistsResult, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &BulkLinkArtistsResult{}
	phases := []struct {
		name string
		sql  string
		dest *int64
	}{
		{"mbid", bulkLinkMBIDSQL, &result.MBIDLinked},
		{"exact_name", bulkLinkExactNameSQL, &result.NameLinked},
		{"alias", bulkLinkAliasSQL, &result.AliasLinked},
	}

	for _, phase := range phases {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		n, err := m.bulkLinkPhase(ctx, phase.sql)
		if err != nil {
			return result, fmt.Errorf("bulk link %s: %w", phase.name, err)
		}
		*phase.dest = n
		if n > 0 {
			slog.Info("radio bulk link phase complete",
				"phase", phase.name, "linked", n)
		}
	}

	return result, nil
}

func (m *RadioMatchingEngine) bulkLinkPhase(ctx context.Context, query string) (int64, error) {
	var linkedPlays, linkedReleases, clearedReleases int64
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		linkedPlays, linkedReleases, clearedReleases, err = linkArtistsWithCreditRule(tx, query)
		return err
	})
	if err != nil {
		return 0, err
	}
	if linkedReleases > 0 || clearedReleases > 0 {
		slog.Info("radio bulk link: applied the release credit rule",
			"releases_linked", linkedReleases, "releases_cleared", clearedReleases)
	}
	return linkedPlays, nil
}
