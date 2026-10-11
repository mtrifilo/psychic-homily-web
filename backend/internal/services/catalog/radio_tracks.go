package catalog

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
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

// The rebuild's working tables. Temporary and dropped on commit, so they never
// generate WAL and never outlive the transaction.
const (
	radioTrackRebuildPlaysTable  = "radio_track_rebuild_plays"
	radioTrackRebuildJoinsTable  = "radio_track_rebuild_joins"
	radioTrackRebuildTracksTable = "radio_track_rebuild_tracks"
)

// radioTrackRebuildPlaysSQL keys every play. artist_key and title_key are both
// the empty string when either half is empty or NULL, which routes the play to the unkeyed
// marker row.
var radioTrackRebuildPlaysSQL = `
CREATE TEMP TABLE ` + radioTrackRebuildPlaysTable + ` ON COMMIT DROP AS
SELECT CASE WHEN raw.a = '' OR raw.t = '' THEN '' ELSE raw.a END AS artist_key,
       CASE WHEN raw.a = '' OR raw.t = '' THEN '' ELSE raw.t END AS title_key,
       sh.station_id,
       ` + broadcasterKeySQL("st") + ` AS family_key,
       e.show_id,
       e.air_date,
       p.artist_name,
       p.track_title,
       lower(nullif(btrim(p.musicbrainz_recording_id), '')) AS mb_recording_id,
       coalesce(p.album_title ~ '[^[:space:]]', false) AS has_album,
       coalesce(p.label_name ~ '[^[:space:]]', false) AS has_label
FROM radio_plays p
JOIN radio_episodes e ON e.id = p.episode_id
JOIN radio_shows sh ON sh.id = e.show_id
JOIN radio_stations st ON st.id = sh.station_id
CROSS JOIN LATERAL (
  SELECT coalesce(radio_track_artist_key(p.artist_name), '') AS a,
         coalesce(radio_track_title_key(p.track_title), '') AS t
) raw`

// radioTrackRebuildTracksSQL aggregates the keyed plays into one row per
// track. A key listed in the joins table counts toward the track it was joined
// into. Spelling ties break on the C collation so every environment picks the
// same one.
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
  WHERE track_title IS NOT NULL
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
       a.album_play_count, a.label_play_count
FROM agg a
JOIN artist_spelling s ON s.artist_key = a.artist_key AND s.title_key = a.title_key
LEFT JOIN title_spelling ts ON ts.artist_key = a.artist_key AND ts.title_key = a.title_key`

// radioTrackDerivedColumns are the radio_tracks columns a rebuild derives,
// in the order radioTrackRebuildTracksSQL selects them.
const radioTrackDerivedColumns = `display_artist, display_title, play_count, station_row_count,
  station_family_count, show_count, first_aired_on, last_aired_on, mb_recording_id,
  mb_recording_id_conflict, album_play_count, label_play_count`

var radioTrackDeleteStaleSQL = `
DELETE FROM radio_tracks t
WHERE NOT EXISTS (
  SELECT 1 FROM ` + radioTrackRebuildTracksTable + ` n
  WHERE n.artist_key = t.artist_key AND n.title_key = t.title_key
)`

// radioTrackUpsertSQL writes new tracks and rewrites only the rows whose
// derived columns changed, so a steady-state rebuild generates WAL for the
// tracks new plays touched rather than for the whole table.
var radioTrackUpsertSQL = `
INSERT INTO radio_tracks (artist_key, title_key, ` + radioTrackDerivedColumns + `, rebuilt_at)
SELECT artist_key, title_key, ` + radioTrackDerivedColumns + `, now()
FROM ` + radioTrackRebuildTracksTable + `
ON CONFLICT (artist_key, title_key) DO UPDATE SET
  (` + radioTrackDerivedColumns + `, rebuilt_at) =
  (EXCLUDED.display_artist, EXCLUDED.display_title, EXCLUDED.play_count, EXCLUDED.station_row_count,
   EXCLUDED.station_family_count, EXCLUDED.show_count, EXCLUDED.first_aired_on, EXCLUDED.last_aired_on,
   EXCLUDED.mb_recording_id, EXCLUDED.mb_recording_id_conflict, EXCLUDED.album_play_count,
   EXCLUDED.label_play_count, EXCLUDED.rebuilt_at)
WHERE (radio_tracks.display_artist, radio_tracks.display_title, radio_tracks.play_count,
       radio_tracks.station_row_count, radio_tracks.station_family_count, radio_tracks.show_count,
       radio_tracks.first_aired_on, radio_tracks.last_aired_on, radio_tracks.mb_recording_id,
       radio_tracks.mb_recording_id_conflict, radio_tracks.album_play_count, radio_tracks.label_play_count)
  IS DISTINCT FROM
      (EXCLUDED.display_artist, EXCLUDED.display_title, EXCLUDED.play_count, EXCLUDED.station_row_count,
       EXCLUDED.station_family_count, EXCLUDED.show_count, EXCLUDED.first_aired_on, EXCLUDED.last_aired_on,
       EXCLUDED.mb_recording_id, EXCLUDED.mb_recording_id_conflict, EXCLUDED.album_play_count,
       EXCLUDED.label_play_count)`

var radioTrackKeysUpsertSQL = `
INSERT INTO radio_track_keys (artist_key, title_key, track_id)
SELECT k.artist_key, k.title_key, t.id
FROM (SELECT DISTINCT artist_key, title_key FROM ` + radioTrackRebuildPlaysTable + `) k
LEFT JOIN ` + radioTrackRebuildJoinsTable + ` j
  ON j.artist_key = k.artist_key AND j.title_key = k.title_key
JOIN radio_tracks t
  ON t.artist_key = coalesce(j.track_artist_key, k.artist_key)
 AND t.title_key = coalesce(j.track_title_key, k.title_key)
ON CONFLICT (artist_key, title_key) DO UPDATE SET track_id = EXCLUDED.track_id
WHERE radio_track_keys.track_id <> EXCLUDED.track_id`

var radioTrackKeysDeleteStaleSQL = `
DELETE FROM radio_track_keys k
WHERE NOT EXISTS (
  SELECT 1 FROM ` + radioTrackRebuildPlaysTable + ` p
  WHERE p.artist_key = k.artist_key AND p.title_key = k.title_key
)`

// RebuildRadioTracks rebuilds radio_tracks and radio_track_keys from
// radio_plays in one transaction. It is idempotent: a rebuild over unchanged
// plays writes no row. Plays are read by a single statement, so the result
// reflects one snapshot of radio_plays; the rebuild takes no lock on
// radio_plays beyond that read, so it never blocks writers of the plays (the
// fetch and rematch loops). A rebuild that finds another rebuild in progress
// returns a Skipped report without waiting.
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

		// Stale rows go first so the cascade removes their keys before the
		// key sync rewrites the rest.
		res := tx.Exec(radioTrackDeleteStaleSQL)
		if res.Error != nil {
			return fmt.Errorf("deleting stale tracks: %w", res.Error)
		}
		report.Deleted = res.RowsAffected

		res = tx.Exec(radioTrackUpsertSQL)
		if res.Error != nil {
			return fmt.Errorf("writing tracks: %w", res.Error)
		}
		report.Written = res.RowsAffected

		if err := tx.Exec(radioTrackKeysUpsertSQL).Error; err != nil {
			return fmt.Errorf("writing track keys: %w", err)
		}
		if err := tx.Exec(radioTrackKeysDeleteStaleSQL).Error; err != nil {
			return fmt.Errorf("deleting stale track keys: %w", err)
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

// radioTrackSortColumns maps each listing sort to its radio_tracks column.
var radioTrackSortColumns = map[contracts.RadioTrackSort]string{
	contracts.RadioTrackSortPlays:           "play_count",
	contracts.RadioTrackSortStationRows:     "station_row_count",
	contracts.RadioTrackSortStationFamilies: "station_family_count",
}

// RadioTracksReader serves the internal top-tracks listing.
type RadioTracksReader struct {
	db *gorm.DB
}

// NewRadioTracksReader constructs a reader over radio_tracks.
func NewRadioTracksReader(database *gorm.DB) *RadioTracksReader {
	return &RadioTracksReader{db: database}
}

// radioTrackListRow is a radio_tracks row plus its key count.
type radioTrackListRow struct {
	catalogm.RadioTrack
	KeyCount int `gorm:"column:key_count"`
}

// ListTopRadioTracks returns up to limit keyed tracks ranked by the sort
// column, descending, then by play count and id. The unkeyed marker row is
// never listed; its play count is reported as UnkeyedPlays.
func (r *RadioTracksReader) ListTopRadioTracks(ctx context.Context, sort contracts.RadioTrackSort, limit int) (*contracts.RadioTrackListing, error) {
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}
	column, ok := radioTrackSortColumns[sort]
	if !ok {
		return nil, fmt.Errorf("unknown radio track sort %q", sort)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}

	db := r.db.WithContext(ctx)
	var rows []radioTrackListRow
	if err := db.Raw(`SELECT t.*, (SELECT count(*) FROM radio_track_keys k WHERE k.track_id = t.id) AS key_count
FROM radio_tracks t
WHERE NOT t.unkeyed
ORDER BY t.`+column+` DESC, t.play_count DESC, t.id
LIMIT ?`, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing radio tracks: %w", err)
	}

	listing := &contracts.RadioTrackListing{Tracks: make([]contracts.RadioTrackSummary, 0, len(rows))}
	if err := db.Raw(`SELECT count(*) FILTER (WHERE NOT unkeyed),
       coalesce(sum(play_count) FILTER (WHERE unkeyed), 0)
FROM radio_tracks`).Row().Scan(&listing.TotalTracks, &listing.UnkeyedPlays); err != nil {
		return nil, fmt.Errorf("counting radio tracks: %w", err)
	}
	for _, row := range rows {
		listing.Tracks = append(listing.Tracks, radioTrackSummary(row))
	}
	return listing, nil
}

func radioTrackSummary(row radioTrackListRow) contracts.RadioTrackSummary {
	return contracts.RadioTrackSummary{
		ID:                    row.ID,
		ArtistKey:             row.ArtistKey,
		TitleKey:              row.TitleKey,
		DisplayArtist:         row.DisplayArtist,
		DisplayTitle:          row.DisplayTitle,
		PlayCount:             row.PlayCount,
		StationRowCount:       row.StationRowCount,
		StationFamilyCount:    row.StationFamilyCount,
		ShowCount:             row.ShowCount,
		FirstAiredOn:          row.FirstAiredOn.Format(time.DateOnly),
		LastAiredOn:           row.LastAiredOn.Format(time.DateOnly),
		MBRecordingID:         row.MBRecordingID,
		MBRecordingIDConflict: row.MBRecordingIDConflict,
		KeyCount:              row.KeyCount,
		AlbumPlayCount:        row.AlbumPlayCount,
		LabelPlayCount:        row.LabelPlayCount,
		RebuiltAt:             row.RebuiltAt,
	}
}
