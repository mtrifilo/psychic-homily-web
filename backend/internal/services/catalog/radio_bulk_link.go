package catalog

import (
	"context"
	"fmt"
	"log/slog"
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
WHERE rp.id = sub.play_id
`

// bulkLinkExactNameSQL links when immutable_unaccent(LOWER(play.artist_name))
// equals exactly one artist's canonical name. Two artists sharing the same
// normalized name are skipped (Go .First() would pick arbitrarily; we decline).
const bulkLinkExactNameSQL = `
UPDATE radio_plays rp
SET artist_id = sub.artist_id, match_state = 'matched'
FROM (
	SELECT rp2.id AS play_id, MIN(a.id) AS artist_id
	FROM radio_plays rp2
	INNER JOIN artists a
		ON immutable_unaccent(LOWER(rp2.artist_name)) = immutable_unaccent(LOWER(a.name))
	WHERE rp2.artist_id IS NULL
		AND rp2.match_state = 'unmatched'
	GROUP BY rp2.id
	HAVING COUNT(DISTINCT a.id) = 1
) sub
WHERE rp.id = sub.play_id
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
		ON immutable_unaccent(LOWER(rp2.artist_name)) = immutable_unaccent(LOWER(aa.alias))
	WHERE rp2.artist_id IS NULL
		AND rp2.match_state = 'unmatched'
	GROUP BY rp2.id
	HAVING COUNT(DISTINCT aa.artist_id) = 1
) sub
WHERE rp.id = sub.play_id
`

// BulkLinkUnmatchedArtistPlays runs MBID → exact name → alias UPDATE passes for
// plays where artist_id IS NULL. Each phase runs in its own transaction.
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
	sqlDB, err := m.db.DB()
	if err != nil {
		return 0, fmt.Errorf("sql db: %w", err)
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return rows, nil
}
