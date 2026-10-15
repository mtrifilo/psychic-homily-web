package catalog

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/lib/pq"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// radio_tracks is a derived read model over radio_plays; the migration that
// creates it states the key and the MusicBrainz join rule. RebuildRadioTracks
// is the only writer of radio_tracks and radio_track_keys.

// radioTracksRebuildLock serializes rebuilds across processes and callers.
// Arbitrary app-unique advisory-lock key.
const radioTracksRebuildLock = 7412620003

// radioTrackKey is the (artist_key, title_key) pair that identifies a key.
type radioTrackKey struct {
	Artist string
	Title  string
}

func compareRadioTrackKeys(a, b radioTrackKey) int {
	if c := cmp.Compare(a.Artist, b.Artist); c != 0 {
		return c
	}
	return cmp.Compare(a.Title, b.Title)
}

// RadioTracksRebuildReport is one rebuild's outcome.
type RadioTracksRebuildReport struct {
	// Skipped is true when another rebuild held the lock; nothing else is set.
	Skipped bool
	// Plays is every radio_plays row the rebuild read.
	Plays int64
	// UnkeyedPlays is the plays counted on the unkeyed marker row.
	UnkeyedPlays int64
	// Tracks and Keys are the radio_tracks and radio_track_keys row counts
	// after the rebuild, the unkeyed marker row included in both.
	Tracks int64
	Keys   int64
	// JoinedKeys is the keys a MusicBrainz recording id joined into another
	// key's track.
	JoinedKeys int
	// Written is the radio_tracks rows inserted or changed; Deleted the rows
	// whose key no longer names a track.
	Written int64
	Deleted int64
}

// The rebuild's working tables. Temporary and dropped on commit: their rows are
// not WAL-logged (creating, analyzing and dropping them writes a little catalog
// WAL per rebuild), and they never outlive the transaction.
const (
	radioTrackRebuildPlaysTable  = "radio_track_rebuild_plays"
	radioTrackRebuildJoinsTable  = "radio_track_rebuild_joins"
	radioTrackRebuildTracksTable = "radio_track_rebuild_tracks"
	radioTrackRebuildKeysTable   = "radio_track_rebuild_keys"
)

// radioTrackRebuildPlaysSQL keys every play. artist_key and title_key are both
// the empty string when either half is empty or NULL, which routes the play to
// the unkeyed marker row. A recording id that is not shaped like a UUID is
// treated as absent, so it can neither join keys nor set a track's id.
// has_album is the one radio_play_exclusions read of the rebuild: it holds
// when the play's album counts as release evidence (RadioRankingAlbum), so
// album_play_count follows the exclusion rows as of the latest rebuild while
// every other column counts every play.
var radioTrackRebuildPlaysSQL = `
CREATE TEMP TABLE ` + radioTrackRebuildPlaysTable + ` ON COMMIT DROP AS
SELECT CASE WHEN k.unkeyed THEN '' ELSE raw.a END AS artist_key,
       CASE WHEN k.unkeyed THEN '' ELSE raw.t END AS title_key,
       sh.station_id,
       ` + broadcasterKeySQL("st") + ` AS family_key,
       e.show_id,
       e.air_date,
       p.artist_name,
       p.track_title,
       mb.id AS mb_recording_id,
       NOT ` + RadioPlayExcludedSQL(RadioRankingAlbum, radioCandidatePlayCols) + ` AS has_album,
       coalesce(p.label_name ~ '[^[:space:]]', false) AS has_label
FROM radio_plays p
JOIN radio_episodes e ON e.id = p.episode_id
JOIN radio_shows sh ON sh.id = e.show_id
JOIN radio_stations st ON st.id = sh.station_id
CROSS JOIN LATERAL (
  SELECT coalesce(radio_track_artist_key(p.artist_name), '') AS a,
         coalesce(radio_track_title_key(p.track_title), '') AS t
) raw
CROSS JOIN LATERAL (SELECT raw.a = '' OR raw.t = '' AS unkeyed) k
CROSS JOIN LATERAL (SELECT lower(btrim(p.musicbrainz_recording_id)) AS raw_id) mbr
CROSS JOIN LATERAL (
  SELECT CASE WHEN mbr.raw_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
              THEN mbr.raw_id END AS id
) mb`

// radioTrackRebuildTracksSQL aggregates the keyed plays into one row per
// track. A key listed in the joins table counts toward the track it was joined
// into. track_id is the id of the track's existing radio_tracks row, NULL for
// a new track. Spelling ties break on the C collation so every environment
// picks the same one.
var radioTrackRebuildTracksSQL = `
CREATE TEMP TABLE ` + radioTrackRebuildTracksTable + ` ON COMMIT DROP AS
WITH t AS (
  SELECT coalesce(j.track_artist_key, p.artist_key) AS artist_key,
         coalesce(j.track_title_key, p.title_key) AS title_key,
         p.station_id, p.family_key, p.show_id, p.air_date,
         p.artist_name, p.track_title, p.mb_recording_id, p.has_album, p.has_label
  FROM ` + radioTrackRebuildPlaysTable + ` p
  LEFT JOIN ` + radioTrackRebuildJoinsTable + ` j
    ON j.artist_key = p.artist_key AND j.title_key = p.title_key
),
agg AS (
  SELECT artist_key, title_key,
         count(*) AS play_count,
         count(DISTINCT station_id) AS station_row_count,
         count(DISTINCT family_key) AS station_family_count,
         count(DISTINCT show_id) AS show_count,
         min(air_date) AS first_aired_on,
         max(air_date) AS last_aired_on,
         count(DISTINCT mb_recording_id) AS mb_ids,
         min(mb_recording_id) AS some_mb_id,
         count(*) FILTER (WHERE has_album) AS album_play_count,
         count(*) FILTER (WHERE has_label) AS label_play_count
  FROM t
  GROUP BY artist_key, title_key
),
artist_spelling AS (
  SELECT DISTINCT ON (artist_key, title_key) artist_key, title_key, artist_name
  FROM t
  GROUP BY artist_key, title_key, artist_name
  ORDER BY artist_key, title_key, count(*) DESC, artist_name COLLATE "C"
),
title_spelling AS (
  SELECT DISTINCT ON (artist_key, title_key) artist_key, title_key, track_title
  FROM t
  GROUP BY artist_key, title_key, track_title
  ORDER BY artist_key, title_key, count(*) DESC, track_title COLLATE "C"
)
SELECT a.artist_key, a.title_key,
       CASE WHEN a.artist_key = '' THEN '' ELSE s.artist_name END AS display_artist,
       CASE WHEN a.artist_key = '' THEN '' ELSE ts.track_title END AS display_title,
       a.play_count, a.station_row_count, a.station_family_count, a.show_count,
       a.first_aired_on, a.last_aired_on,
       CASE WHEN a.artist_key <> '' AND a.mb_ids = 1 THEN a.some_mb_id END AS mb_recording_id,
       (a.artist_key <> '' AND a.mb_ids > 1) AS mb_recording_id_conflict,
       a.album_play_count, a.label_play_count,
       rt.id AS track_id
FROM agg a
JOIN artist_spelling s ON s.artist_key = a.artist_key AND s.title_key = a.title_key
JOIN title_spelling ts ON ts.artist_key = a.artist_key AND ts.title_key = a.title_key
LEFT JOIN radio_tracks rt ON rt.artist_key = a.artist_key AND rt.title_key = a.title_key`

// radioTrackDerivedColumns are the radio_tracks columns a rebuild derives.
var radioTrackDerivedColumns = []string{
	"display_artist", "display_title", "play_count", "station_row_count",
	"station_family_count", "show_count", "first_aired_on", "last_aired_on",
	"mb_recording_id", "mb_recording_id_conflict", "album_play_count", "label_play_count",
}

// radioTrackColumns lists the derived columns, each qualified by alias when
// alias is non-empty.
func radioTrackColumns(alias string) string {
	if alias == "" {
		return strings.Join(radioTrackDerivedColumns, ", ")
	}
	return alias + "." + strings.Join(radioTrackDerivedColumns, ", "+alias+".")
}

// The write phase. Every write touches only rows that are new, changed or
// stale, so a rebuild over unchanged plays writes (and logs to WAL) nothing.
// Existing ids are resolved into the working tables before anything is
// written, so no statement reads the table it is writing: an anti-join
// against a table that is growing inside the same statement can be planned as
// a sequential rescan per row. INSERT ... ON CONFLICT is not used because it
// locks, and so rewrites, every conflicting row. The rebuild lock is what makes
// these plain statements safe: no other writer touches these tables.

var radioTrackDeleteStaleSQL = `
DELETE FROM radio_tracks t
WHERE NOT EXISTS (
  SELECT 1 FROM ` + radioTrackRebuildTracksTable + ` n
  WHERE n.artist_key = t.artist_key AND n.title_key = t.title_key
)`

var radioTrackUpdateSQL = `
UPDATE radio_tracks t
SET (` + radioTrackColumns("") + `, rebuilt_at) = (` + radioTrackColumns("n") + `, now())
FROM ` + radioTrackRebuildTracksTable + ` n
WHERE n.track_id = t.id
  AND (` + radioTrackColumns("t") + `) IS DISTINCT FROM (` + radioTrackColumns("n") + `)`

// radioTrackInsertSQL inserts the tracks that have no row yet and records the
// new ids in the working table; its row count is the number inserted.
var radioTrackInsertSQL = `
WITH inserted AS (
  INSERT INTO radio_tracks (artist_key, title_key, ` + radioTrackColumns("") + `, rebuilt_at)
  SELECT artist_key, title_key, ` + radioTrackColumns("") + `, now()
  FROM ` + radioTrackRebuildTracksTable + `
  WHERE track_id IS NULL
  RETURNING id, artist_key, title_key
)
UPDATE ` + radioTrackRebuildTracksTable + ` n
SET track_id = inserted.id
FROM inserted
WHERE inserted.artist_key = n.artist_key AND inserted.title_key = n.title_key`

// radioTrackRebuildKeysSQL lists every key seen in the plays with the id of
// its track and the track_id its radio_track_keys row holds now (NULL when it
// has none).
var radioTrackRebuildKeysSQL = `
CREATE TEMP TABLE ` + radioTrackRebuildKeysTable + ` ON COMMIT DROP AS
SELECT k.artist_key, k.title_key, n.track_id, rk.track_id AS current_track_id
FROM (SELECT DISTINCT artist_key, title_key FROM ` + radioTrackRebuildPlaysTable + `) k
LEFT JOIN ` + radioTrackRebuildJoinsTable + ` j
  ON j.artist_key = k.artist_key AND j.title_key = k.title_key
JOIN ` + radioTrackRebuildTracksTable + ` n
  ON n.artist_key = coalesce(j.track_artist_key, k.artist_key)
 AND n.title_key = coalesce(j.track_title_key, k.title_key)
LEFT JOIN radio_track_keys rk
  ON rk.artist_key = k.artist_key AND rk.title_key = k.title_key`

var radioTrackKeysDeleteStaleSQL = `
DELETE FROM radio_track_keys rk
WHERE NOT EXISTS (
  SELECT 1 FROM ` + radioTrackRebuildKeysTable + ` k
  WHERE k.artist_key = rk.artist_key AND k.title_key = rk.title_key
)`

var radioTrackKeysUpdateSQL = `
UPDATE radio_track_keys rk
SET track_id = k.track_id
FROM ` + radioTrackRebuildKeysTable + ` k
WHERE k.artist_key = rk.artist_key AND k.title_key = rk.title_key
  AND k.current_track_id <> k.track_id`

var radioTrackKeysInsertSQL = `
INSERT INTO radio_track_keys (artist_key, title_key, track_id)
SELECT artist_key, title_key, track_id
FROM ` + radioTrackRebuildKeysTable + `
WHERE current_track_id IS NULL`

// RebuildRadioTracks rebuilds radio_tracks and radio_track_keys from
// radio_plays in one transaction. It is idempotent: a rebuild over unchanged
// plays writes no row. Plays are read by a single statement, so the result
// reflects one snapshot of radio_plays. That read takes ACCESS SHARE locks on
// radio_plays, radio_episodes, radio_shows and radio_stations, held until the
// transaction ends: row writers of those tables (the fetch and rematch loops)
// are never blocked, but DDL on them waits for the rebuild to finish. A rebuild
// that finds another rebuild in progress returns a Skipped report without
// waiting.
//
// A track row keeps its id across rebuilds while its key keeps naming a track.
func RebuildRadioTracks(ctx context.Context, database *gorm.DB) (*RadioTracksRebuildReport, error) {
	if database == nil {
		return nil, errors.New("database not initialized")
	}
	report := &RadioTracksRebuildReport{}
	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked bool
		if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", radioTracksRebuildLock).Scan(&locked).Error; err != nil {
			return fmt.Errorf("taking the rebuild lock: %w", err)
		}
		if !locked {
			report.Skipped = true
			return nil
		}

		if err := tx.Exec(radioTrackRebuildPlaysSQL).Error; err != nil {
			return fmt.Errorf("keying plays: %w", err)
		}
		if err := tx.Exec("ANALYZE " + radioTrackRebuildPlaysTable).Error; err != nil {
			return fmt.Errorf("analyzing keyed plays: %w", err)
		}
		if err := tx.Raw("SELECT count(*), count(*) FILTER (WHERE artist_key = '') FROM "+radioTrackRebuildPlaysTable).
			Row().Scan(&report.Plays, &report.UnkeyedPlays); err != nil {
			return fmt.Errorf("counting keyed plays: %w", err)
		}

		joined, err := writeRadioTrackJoins(tx)
		if err != nil {
			return err
		}
		report.JoinedKeys = joined

		if err := tx.Exec(radioTrackRebuildTracksSQL).Error; err != nil {
			return fmt.Errorf("aggregating tracks: %w", err)
		}
		if err := tx.Exec("ANALYZE " + radioTrackRebuildTracksTable).Error; err != nil {
			return fmt.Errorf("analyzing tracks: %w", err)
		}

		// Stale rows go first so the cascade removes their keys before the
		// keys are compared.
		res := tx.Exec(radioTrackDeleteStaleSQL)
		if res.Error != nil {
			return fmt.Errorf("deleting stale tracks: %w", res.Error)
		}
		report.Deleted = res.RowsAffected
		for _, stmt := range []string{radioTrackUpdateSQL, radioTrackInsertSQL} {
			res = tx.Exec(stmt)
			if res.Error != nil {
				return fmt.Errorf("writing tracks: %w", res.Error)
			}
			report.Written += res.RowsAffected
		}

		if err := tx.Exec(radioTrackRebuildKeysSQL).Error; err != nil {
			return fmt.Errorf("listing track keys: %w", err)
		}
		for _, stmt := range []string{radioTrackKeysDeleteStaleSQL, radioTrackKeysUpdateSQL, radioTrackKeysInsertSQL} {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("writing track keys: %w", err)
			}
		}

		if err := tx.Raw("SELECT (SELECT count(*) FROM radio_tracks), (SELECT count(*) FROM radio_track_keys)").
			Row().Scan(&report.Tracks, &report.Keys); err != nil {
			return fmt.Errorf("counting tracks: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("rebuilding radio_tracks: %w", err)
	}
	return report, nil
}

// radioTrackMBPair is one distinct (key, recording id) pair among the keyed
// plays.
type radioTrackMBPair struct {
	ArtistKey     string
	TitleKey      string
	MBRecordingID string
}

// writeRadioTrackJoins creates the joins table: one row for every key that a
// shared MusicBrainz recording id joins into another key's track, naming that
// track's key. It returns the number of rows written.
func writeRadioTrackJoins(tx *gorm.DB) (int, error) {
	if err := tx.Exec(`CREATE TEMP TABLE ` + radioTrackRebuildJoinsTable + ` (
  artist_key TEXT NOT NULL,
  title_key TEXT NOT NULL,
  track_artist_key TEXT NOT NULL,
  track_title_key TEXT NOT NULL,
  PRIMARY KEY (artist_key, title_key)
) ON COMMIT DROP`).Error; err != nil {
		return 0, fmt.Errorf("creating the joins table: %w", err)
	}

	var pairs []radioTrackMBPair
	if err := tx.Raw(`SELECT DISTINCT artist_key, title_key, mb_recording_id
FROM ` + radioTrackRebuildPlaysTable + `
WHERE mb_recording_id IS NOT NULL AND artist_key <> ''`).Scan(&pairs).Error; err != nil {
		return 0, fmt.Errorf("reading recording ids: %w", err)
	}
	groups := joinRadioTrackKeys(pairs)
	if len(groups) == 0 {
		return 0, nil
	}

	existing, err := existingRadioTrackIDs(tx, groups)
	if err != nil {
		return 0, err
	}

	var from, to []radioTrackKey
	for _, group := range groups {
		canonical := canonicalRadioTrackKey(group, existing)
		for _, k := range group {
			if k != canonical {
				from = append(from, k)
				to = append(to, canonical)
			}
		}
	}

	fromArtist, fromTitle := splitRadioTrackKeys(from)
	toArtist, toTitle := splitRadioTrackKeys(to)
	if err := tx.Exec(`INSERT INTO `+radioTrackRebuildJoinsTable+`
SELECT * FROM unnest(?::text[], ?::text[], ?::text[], ?::text[])`,
		pq.Array(fromArtist), pq.Array(fromTitle), pq.Array(toArtist), pq.Array(toTitle)).Error; err != nil {
		return 0, fmt.Errorf("writing joins: %w", err)
	}
	return len(from), nil
}

// joinRadioTrackKeys groups keys that share a MusicBrainz recording id,
// transitively, and returns every group of two or more keys. Each group is
// sorted and the groups are ordered by their first key, so the result depends
// only on the set of pairs.
func joinRadioTrackKeys(pairs []radioTrackMBPair) [][]radioTrackKey {
	parent := map[radioTrackKey]radioTrackKey{}
	var find func(radioTrackKey) radioTrackKey
	find = func(k radioTrackKey) radioTrackKey {
		p, ok := parent[k]
		if !ok || p == k {
			return k
		}
		root := find(p)
		parent[k] = root
		return root
	}

	firstKeyForID := map[string]radioTrackKey{}
	for _, pair := range pairs {
		k := radioTrackKey{Artist: pair.ArtistKey, Title: pair.TitleKey}
		first, seen := firstKeyForID[pair.MBRecordingID]
		if !seen {
			firstKeyForID[pair.MBRecordingID] = k
			continue
		}
		a, b := find(first), find(k)
		if a == b {
			continue
		}
		parent[a] = a
		parent[b] = a
	}

	members := map[radioTrackKey][]radioTrackKey{}
	for k := range parent {
		root := find(k)
		members[root] = append(members[root], k)
	}
	groups := make([][]radioTrackKey, 0, len(members))
	for _, group := range members {
		if len(group) < 2 {
			continue
		}
		slices.SortFunc(group, compareRadioTrackKeys)
		groups = append(groups, group)
	}
	slices.SortFunc(groups, func(a, b []radioTrackKey) int { return compareRadioTrackKeys(a[0], b[0]) })
	return groups
}

// existingRadioTrackIDs returns the radio_tracks id of every group member that
// currently names a track row.
func existingRadioTrackIDs(tx *gorm.DB, groups [][]radioTrackKey) (map[radioTrackKey]uint, error) {
	var all []radioTrackKey
	for _, group := range groups {
		all = append(all, group...)
	}
	artists, titles := splitRadioTrackKeys(all)
	var rows []catalogm.RadioTrackKey
	if err := tx.Raw(`SELECT t.artist_key, t.title_key, t.id AS track_id
FROM radio_tracks t
JOIN unnest(?::text[], ?::text[]) AS k(artist_key, title_key)
  ON k.artist_key = t.artist_key AND k.title_key = t.title_key`,
		pq.Array(artists), pq.Array(titles)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("reading existing tracks: %w", err)
	}
	existing := make(map[radioTrackKey]uint, len(rows))
	for _, r := range rows {
		existing[radioTrackKey{Artist: r.ArtistKey, Title: r.TitleKey}] = r.TrackID
	}
	return existing, nil
}

// canonicalRadioTrackKey picks the key whose radio_tracks row a joined group
// keeps: the member with the oldest existing track row, so a track keeps its
// id when keys join it, else the smallest key.
func canonicalRadioTrackKey(group []radioTrackKey, existing map[radioTrackKey]uint) radioTrackKey {
	canonical := group[0]
	var canonicalID uint
	for _, k := range group {
		id, ok := existing[k]
		if ok && (canonicalID == 0 || id < canonicalID) {
			canonical, canonicalID = k, id
		}
	}
	return canonical
}

func splitRadioTrackKeys(keys []radioTrackKey) (artists, titles []string) {
	artists = make([]string, len(keys))
	titles = make([]string, len(keys))
	for i, k := range keys {
		artists[i], titles[i] = k.Artist, k.Title
	}
	return artists, titles
}
