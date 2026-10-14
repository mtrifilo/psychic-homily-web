package catalog

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// An artist candidate is an artist key, radio_normalize_name of the raw artist
// name, ranked from its counted plays. A counted play aired in the window, is
// not excluded for the artist ranking (RadioPlayExcludedSQL), is not in the
// ambiguous match state, and has a name that normalizes to a non-empty key.
// radio_normalize_name trims every character outside a-z and 0-9 from a
// name's edges, so a name carrying a character it may not fold, such as a
// non-Latin script, can key on a fragment; a candidate with such a spelling
// (radioUnfoldableCharSQL) is flagged and kept in the human lane.
//
// A key qualifies when its counted plays clear the station and play floors and
// none of the following holds:
//   - any play of the key, at any time, is linked to an artist;
//   - a catalog artist's name or alias normalizes to the key;
//   - a counted play carries a MusicBrainz artist id a catalog artist holds,
//     compared as the matcher compares it (trimmed, exact);
//   - the key splits as a collab credit (splitCollabArtistName) and a part's
//     normalized form is a catalog artist's name or alias.

const (
	// RadioCandidateWindowDays is the length of the trailing window, in
	// episode air dates, that a candidate's plays are counted over. The
	// window ends on the current UTC date and has no upper bound, so an
	// episode dated after it still counts.
	RadioCandidateWindowDays = 180
	// RadioCandidateMinPlays is the play floor: a key needs at least this many
	// counted plays.
	RadioCandidateMinPlays = 3

	// Evidence list lengths.
	radioCandidateTopTracks = 3
	radioCandidateTopNames  = 5

	// radioCandidateListingTimeout bounds one whole listing, every statement
	// together, below the ph CLI's 30 s request timeout, so a slow listing
	// fails on the server rather than as a client abort.
	radioCandidateListingTimeout = 25 * time.Second
)

// radioUnfoldableCharSQL is a regex over immutable_unaccent(lower(name)) that
// matches any character radio_normalize_name may not fold to a-z or 0-9. It
// is an allowlist: everything matches except ASCII other than A-Z and the
// listed punctuation and symbol blocks (Latin-1 punctuation and signs, general
// punctuation, currency, arrows through dingbats without the circled digits,
// CJK brackets, variation selectors, the byte-order mark, halfwidth
// punctuation, regional indicators and emoji). An ASCII
// capital matches because lower() has already run, so one can only come from
// a character unaccent folded to a capital, which the normalizer then trims.
// An unknown character therefore flags its key. The ranges are fixed code
// points, so the test does not depend on the database locale.
const radioUnfoldableCharSQL = `[^\u0001-\u0040\u005b-\u007f\u00a0-\u00a9\u00ab-\u00b1\u00b4\u00b6-\u00b8\u00bb\u00bf\u00d7\u00f7\u2000-\u206f\u20a0-\u20cf\u2190-\u245f\u2500-\u2775\u2794-\u2bff\u3000-\u3004\u3008-\u301f\ufe00-\ufe0f\ufeff\uff61-\uff65\U0001f000-\U0001f0ff\U0001f1e6-\U0001faff]`

// RadioArtistCandidateScore is the artist candidate ranking score: distinct
// station rows x ln(plays) x (1 + distinct shows / 10).
func RadioArtistCandidateScore(stationRows, plays, shows int) float64 {
	return float64(stationRows) * math.Log(float64(plays)) * (1 + float64(shows)/10)
}

// RadioCandidateLaneFacts are what a candidate's lane depends on.
type RadioCandidateLaneFacts struct {
	StationRows int
	// DistinctMBIDs counts the distinct MusicBrainz ids of the entity the
	// candidate would create: artist ids for an artist, release ids for a
	// track.
	DistinctMBIDs int
	// MBIDShared is true when another candidate's plays carry one of this
	// candidate's MusicBrainz ids.
	MBIDShared bool
	// NonLatinSpelling is true when a counted play's name carries a
	// character radio_normalize_name may not fold (radioUnfoldableCharSQL),
	// usually a non-Latin script, so the key may be a fragment of that name.
	NonLatinSpelling bool
}

// RadioCandidateLaneFor returns a candidate's lane: auto when its plays carry
// exactly one distinct MusicBrainz id, no other candidate carries that id, no
// spelling carries a character normalization may not fold, and it clears the
// station floor; human otherwise. Several ids mean the name covers more than
// one act, a shared id means one act under several names, and an unfoldable
// character means the key may be a fragment of the name, so a person decides
// each.
func RadioCandidateLaneFor(f RadioCandidateLaneFacts) contracts.RadioCandidateLane {
	if f.DistinctMBIDs == 1 && !f.MBIDShared && !f.NonLatinSpelling && RadioMeetsStationFloor(f.StationRows) {
		return contracts.RadioCandidateLaneAuto
	}
	return contracts.RadioCandidateLaneHuman
}

// RadioCandidatesReader serves the enrichment candidate rankings.
type RadioCandidatesReader struct {
	db  *gorm.DB
	now func() time.Time
}

// NewRadioCandidatesReader constructs a reader over radio_plays.
func NewRadioCandidatesReader(database *gorm.DB) *RadioCandidatesReader {
	return &RadioCandidatesReader{db: database, now: time.Now}
}

// radioArtistCandidateRow is one row of radioArtistCandidatesSQL: a
// qualifying key with its counts, or, when ArtistKey is nil, the row that only
// carries UnkeyablePlays.
type radioArtistCandidateRow struct {
	UnkeyablePlays     int64          `gorm:"column:unkeyable_plays"`
	ArtistKey          *string        `gorm:"column:artist_key"`
	PlayCount          int            `gorm:"column:play_count"`
	StationRowCount    int            `gorm:"column:station_row_count"`
	StationFamilyCount int            `gorm:"column:station_family_count"`
	ShowCount          int            `gorm:"column:show_count"`
	FirstAiredOn       time.Time      `gorm:"column:first_aired_on"`
	LastAiredOn        time.Time      `gorm:"column:last_aired_on"`
	MBArtistIDs        pq.StringArray `gorm:"column:mb_artist_ids;type:text[]"`
	NonLatinSpelling   bool           `gorm:"column:non_latin_spelling"`
}

// radioCandidateEvidenceRow is one row of radioArtistCandidateEvidenceSQL.
// Every branch of that UNION ALL selects exactly these columns, in this order
// and under these names: kind, artist_key, ref_id, name, slug, play_count,
// distinct_count, rn. Kind says which candidate field the row fills.
type radioCandidateEvidenceRow struct {
	Kind          string `gorm:"column:kind"`
	ArtistKey     string `gorm:"column:artist_key"`
	RefID         uint   `gorm:"column:ref_id"`
	Name          string `gorm:"column:name"`
	Slug          string `gorm:"column:slug"`
	PlayCount     int    `gorm:"column:play_count"`
	DistinctCount int    `gorm:"column:distinct_count"`
}

const (
	radioEvidenceSpelling = "spelling"
	radioEvidenceStation  = "station"
	radioEvidenceTrack    = "track"
	radioEvidenceAlbum    = "album"
	radioEvidenceLabel    = "label"
)

var radioCandidatePlayCols = RadioPlayColumns{
	StationID:  "sh.station_id",
	ArtistName: "p.artist_name",
	TrackTitle: "p.track_title",
	LabelName:  "p.label_name",
}

// radioCandidatePlaysFromSQL returns a FROM clause that joins a play to its
// episode, show and station, exposes its artist key as k.artist_key, and keeps
// the plays in the window that are not excluded for the dimension's ranking
// and not ambiguous. Keyability is left to the caller. Its one parameter is
// the window's first air date.
func radioCandidatePlaysFromSQL(dim RadioRankingDimension) string {
	return `
FROM radio_plays p
JOIN radio_episodes e ON e.id = p.episode_id
JOIN radio_shows sh ON sh.id = e.show_id
JOIN radio_stations st ON st.id = sh.station_id
CROSS JOIN LATERAL (SELECT radio_normalize_name(p.artist_name) AS artist_key) k
WHERE e.air_date >= ?::date
  AND p.match_state <> '` + catalogm.RadioPlayMatchStateAmbiguous + `'
  AND NOT (` + RadioPlayExcludedSQL(dim, radioCandidatePlayCols) + `)`
}

// radioCandidatePlaysFrom is radioCandidatePlaysFromSQL for the artist
// ranking.
var radioCandidatePlaysFrom = radioCandidatePlaysFromSQL(RadioRankingArtist)

// radioNonLatinSQL returns a boolean SQL expression that is true when the
// text column carries a character radio_normalize_name cannot fold. Panics on
// a column that is not an identifier.
func radioNonLatinSQL(column string) string {
	return `public.immutable_unaccent(lower(` + mustSQLColumnRef(column) + `)) ~ '` + radioUnfoldableCharSQL + `'`
}

// radioCandidateNonLatinSQL is true for a play of radioCandidatePlaysFrom
// whose name carries a character radio_normalize_name cannot fold.
var radioCandidateNonLatinSQL = radioNonLatinSQL("p.artist_name")

// radioArtistCandidatesSQL returns every qualifying key with its counts or,
// when no key qualifies, one row whose artist_key is NULL; every row carries
// the number of in-window plays whose name normalizes to empty. Parameters:
// the window's first air date, then the play floor.
var radioArtistCandidatesSQL = `
WITH plays AS MATERIALIZED (
  SELECT k.artist_key,
         ` + radioCandidateNonLatinSQL + ` AS non_latin,
         p.artist_id,
         sh.station_id,
         ` + RadioBroadcasterKeySQL("st") + ` AS family_key,
         e.show_id,
         e.air_date,
         CASE WHEN btrim(p.musicbrainz_artist_id) ~ '` + mbidUUIDPattern + `'
              THEN btrim(p.musicbrainz_artist_id) END AS mb_raw
  ` + radioCandidatePlaysFrom + `
),
counted AS (
  SELECT * FROM plays WHERE artist_key <> ''
),
catalog_mb AS (
  SELECT DISTINCT musicbrainz_artist_id AS id
  FROM artists
  WHERE musicbrainz_artist_id IS NOT NULL
),
linked_keys AS (
  SELECT DISTINCT radio_normalize_name(artist_name) AS artist_key
  FROM radio_plays
  WHERE artist_id IS NOT NULL
),
agg AS (
  SELECT c.artist_key,
         count(*) AS play_count,
         count(DISTINCT c.station_id) AS station_row_count,
         count(DISTINCT c.family_key) AS station_family_count,
         count(DISTINCT c.show_id) AS show_count,
         min(c.air_date) AS first_aired_on,
         max(c.air_date) AS last_aired_on,
         coalesce(array_agg(DISTINCT lower(c.mb_raw) ORDER BY lower(c.mb_raw))
                  FILTER (WHERE c.mb_raw IS NOT NULL), '{}') AS mb_artist_ids,
         bool_or(cm.id IS NOT NULL) AS catalog_mb,
         bool_or(c.non_latin) AS non_latin_spelling
  FROM counted c
  LEFT JOIN catalog_mb cm ON cm.id = c.mb_raw
  GROUP BY c.artist_key
  HAVING ` + RadioStationFloorSQL("c.station_id") + `
     AND count(*) >= ?
     AND bool_and(c.artist_id IS NULL)
),
qualifying AS (
  SELECT agg.*
  FROM agg
  WHERE NOT agg.catalog_mb
    AND NOT EXISTS (SELECT 1 FROM linked_keys lk WHERE lk.artist_key = agg.artist_key)
    AND NOT ` + radioCatalogAnswersToSQL("agg.artist_key") + `
)
SELECT u.unkeyable_plays, q.artist_key, q.play_count, q.station_row_count, q.station_family_count,
       q.show_count, q.first_aired_on, q.last_aired_on, q.mb_artist_ids, q.non_latin_spelling
FROM (SELECT count(*) AS unkeyable_plays FROM plays WHERE artist_key = '') u
LEFT JOIN qualifying q ON true`

// radioCatalogAnswersToSQL returns a boolean SQL expression that is true when
// a catalog artist's name or alias normalizes to the normalized name expr: the
// matcher's name and alias rungs (matchArtistByNameOrAlias). expr is SQL built
// from constants, never input.
func radioCatalogAnswersToSQL(expr string) string {
	return fmt.Sprintf(`(EXISTS (SELECT 1 FROM artists a WHERE radio_normalize_name(a.name) = %[1]s)
   OR EXISTS (SELECT 1 FROM artist_aliases al WHERE radio_normalize_name(al.alias) = %[1]s))`, expr)
}

// radioCandidateEvidenceNamesSQL ranks the album or label names on the
// evidence query's counted CTE, per value of its owner column (the candidate's
// identity, which the rows carry under the same name): names group by
// radio_normalize_name, show their most common trimmed spelling, and only rows
// where onlyWhere (a boolean column of counted, or "" for every row) holds
// contribute. Panics on a column that is not an identifier.
func radioCandidateEvidenceNamesSQL(kind, owner, column, onlyWhere string) string {
	own := mustSQLColumnRef(owner)
	col := mustSQLColumnRef(column)
	filter := "true"
	if onlyWhere != "" {
		filter = mustSQLColumnRef(onlyWhere)
	}
	return fmt.Sprintf(`
SELECT '%[1]s' AS kind, %[5]s, 0 AS ref_id, name, '' AS slug,
       play_count::int AS play_count, distinct_count::int AS distinct_count, rn
FROM (
  SELECT %[5]s, name, play_count,
         count(*) OVER (PARTITION BY %[5]s) AS distinct_count,
         row_number() OVER (PARTITION BY %[5]s ORDER BY play_count DESC, name COLLATE "C") AS rn
  FROM (
    SELECT %[5]s, norm, sum(n) AS play_count,
           (array_agg(spelling ORDER BY n DESC, spelling COLLATE "C"))[1] AS name
    FROM (
      SELECT %[5]s, radio_normalize_name(%[2]s) AS norm, btrim(%[2]s) AS spelling, count(*) AS n
      FROM counted
      WHERE %[2]s ~ '[^[:space:]]' AND %[3]s
      GROUP BY 1, 2, 3
    ) spellings
    WHERE norm <> ''
    GROUP BY %[5]s, norm
  ) names
) ranked
WHERE rn <= %[4]d`, kind, col, filter, radioCandidateTopNames, own)
}

// radioArtistCandidateEvidenceSQL returns the evidence rows of the selected
// keys, one radioCandidateEvidenceRow per row. Parameters: the window's first
// air date, then the keys. The artist_id IS NULL condition holds for every
// counted play of a qualifying key and lets the lookup use the partial
// normalized-name index.
var radioArtistCandidateEvidenceSQL = `
WITH counted AS MATERIALIZED (
  SELECT k.artist_key,
         p.artist_name, p.track_title, p.album_title, p.label_name,
         sh.station_id,
         ` + RadioPlayExcludedSQL(RadioRankingTrack, radioCandidatePlayCols) + ` AS track_excluded,
         NOT ` + RadioPlayExcludedSQL(RadioRankingLabel, radioCandidatePlayCols) + ` AS label_counts
  ` + radioCandidatePlaysFrom + `
    AND p.artist_id IS NULL
    AND radio_normalize_name(p.artist_name) = ANY (?::text[])
)
SELECT '` + radioEvidenceSpelling + `' AS kind, artist_key, 0 AS ref_id, artist_name AS name, '' AS slug,
       play_count::int AS play_count, distinct_count::int AS distinct_count, rn
FROM (
  SELECT artist_key, artist_name, count(*) AS play_count,
         count(*) OVER (PARTITION BY artist_key) AS distinct_count,
         row_number() OVER (PARTITION BY artist_key ORDER BY count(*) DESC, artist_name COLLATE "C") AS rn
  FROM counted
  GROUP BY artist_key, artist_name
) spellings
WHERE rn <= ` + fmt.Sprint(radioCandidateTopNames) + `
UNION ALL
SELECT '` + radioEvidenceStation + `' AS kind, c.artist_key, st.id AS ref_id, st.name AS name, st.slug AS slug,
       count(*)::int AS play_count, 0 AS distinct_count,
       row_number() OVER (PARTITION BY c.artist_key ORDER BY count(*) DESC, st.id) AS rn
FROM counted c
JOIN radio_stations st ON st.id = c.station_id
GROUP BY c.artist_key, st.id, st.name, st.slug
UNION ALL
SELECT '` + radioEvidenceTrack + `' AS kind, artist_key, track_id AS ref_id, title AS name, '' AS slug,
       play_count::int AS play_count, 0 AS distinct_count, rn
FROM (
  SELECT c.artist_key, t.id AS track_id, t.display_title AS title, count(*) AS play_count,
         row_number() OVER (PARTITION BY c.artist_key ORDER BY count(*) DESC, t.id) AS rn
  FROM counted c
  -- radio_track_keys is keyed by the track key functions of the raw names,
  -- not by the candidate key, so the join recomputes them from the raw names.
  JOIN radio_track_keys tk
    ON tk.artist_key = radio_track_artist_key(c.artist_name)
   AND tk.title_key = radio_track_title_key(c.track_title)
  JOIN radio_tracks t ON t.id = tk.track_id
  WHERE NOT c.track_excluded AND NOT t.unkeyed
  GROUP BY c.artist_key, t.id, t.display_title
) tracks
WHERE rn <= ` + fmt.Sprint(radioCandidateTopTracks) + `
UNION ALL` + radioCandidateEvidenceNamesSQL(radioEvidenceAlbum, "artist_key", "album_title", "") + `
UNION ALL` + radioCandidateEvidenceNamesSQL(radioEvidenceLabel, "artist_key", "label_name", "label_counts") + `
ORDER BY kind, artist_key, rn`

// radioCatalogNamesSQL returns which of the given normalized names a catalog
// artist's name or alias normalizes to.
var radioCatalogNamesSQL = `
SELECT n.name
FROM unnest(?::text[]) AS n(name)
WHERE ` + radioCatalogAnswersToSQL("n.name")

// ListArtistCandidates returns the artist candidate ranking: the qualifying
// keys in the requested lanes, ordered by the requested sort, up to limit, each
// with its evidence. Everything is read from one snapshot.
func (r *RadioCandidatesReader) ListArtistCandidates(ctx context.Context, q contracts.RadioArtistCandidatesQuery) (*contracts.RadioArtistCandidateListing, error) {
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}
	if !q.Lane.Valid() {
		return nil, fmt.Errorf("unknown candidate lane filter %q", q.Lane)
	}
	if _, ok := radioArtistCandidateSortKeys[q.Sort]; !ok {
		return nil, fmt.Errorf("unknown artist candidate sort %q", q.Sort)
	}
	if q.Limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", q.Limit)
	}

	asOf, windowStart := r.window()
	listing := &contracts.RadioArtistCandidateListing{
		AsOf:           asOf,
		WindowStart:    windowStart,
		WindowDays:     RadioCandidateWindowDays,
		MinStationRows: RadioMinDistinctStations,
		MinPlays:       RadioCandidateMinPlays,
	}

	err := r.inSnapshot(ctx, func(tx *gorm.DB) error {
		rows, unkeyable, err := qualifyingRadioArtistRows(tx, listing.WindowStart)
		if err != nil {
			return err
		}
		listing.UnkeyablePlays = unkeyable

		candidates := rankRadioArtistCandidates(rows, q.Sort)
		listing.Candidates = make([]contracts.RadioArtistCandidate, 0, min(q.Limit, len(candidates)))
		for _, c := range candidates {
			if c.Lane == contracts.RadioCandidateLaneAuto {
				listing.AutoTotal++
			} else {
				listing.HumanTotal++
			}
			if !q.Lane.Includes(c.Lane) {
				continue
			}
			listing.Total++
			if len(listing.Candidates) < q.Limit {
				listing.Candidates = append(listing.Candidates, c)
			}
		}

		if err := attachRadioArtistCandidateEvidence(tx, listing.WindowStart, listing.Candidates); err != nil {
			return err
		}
		listing.TracksRebuiltAt, err = radioTracksRebuiltAt(tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return listing, nil
}

// window returns the UTC date the candidate window ends on and its first air
// date, both as YYYY-MM-DD.
func (r *RadioCandidatesReader) window() (asOf, windowStart string) {
	now := r.now().UTC()
	return now.Format(time.DateOnly), now.AddDate(0, 0, -(RadioCandidateWindowDays - 1)).Format(time.DateOnly)
}

// inSnapshot runs fn in one REPEATABLE READ, read-only transaction bounded by
// radioCandidateListingTimeout, so every statement of a listing reads the same
// snapshot.
func (r *RadioCandidatesReader) inSnapshot(ctx context.Context, fn func(tx *gorm.DB) error) error {
	ctx, cancel := context.WithTimeout(ctx, radioCandidateListingTimeout)
	defer cancel()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The server-side bound holds even if the client's cancellation is lost.
		if err := tx.Exec(fmt.Sprintf("SET LOCAL statement_timeout = %d", radioCandidateListingTimeout.Milliseconds())).Error; err != nil {
			return fmt.Errorf("bounding the listing: %w", err)
		}
		return fn(tx)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

// radioTracksRebuiltAt returns when the radio_tracks rebuild loop last
// succeeded, or nil when it never has.
func radioTracksRebuiltAt(tx *gorm.DB) (*time.Time, error) {
	var at *time.Time
	if err := tx.Raw("SELECT (SELECT last_success_at FROM background_service_runs WHERE name = ?)", RadioTracksRebuildLoopName).
		Row().Scan(&at); err != nil {
		return nil, fmt.Errorf("reading the tracks rebuild time: %w", err)
	}
	return at, nil
}

// qualifyingRadioArtistRows returns the qualifying artist keys with their
// counts, every row with a non-nil ArtistKey, and the number of in-window plays
// whose name normalizes to empty.
func qualifyingRadioArtistRows(tx *gorm.DB, windowStart string) ([]radioArtistCandidateRow, int64, error) {
	var rows []radioArtistCandidateRow
	if err := tx.Raw(radioArtistCandidatesSQL, windowStart, RadioCandidateMinPlays).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("ranking artist candidates: %w", err)
	}
	var unkeyable int64
	if len(rows) > 0 {
		unkeyable = rows[0].UnkeyablePlays
	}
	rows = slices.DeleteFunc(rows, func(row radioArtistCandidateRow) bool { return row.ArtistKey == nil })
	rows, err := dropCatalogCollabKeys(tx, rows)
	if err != nil {
		return nil, 0, err
	}
	return rows, unkeyable, nil
}

// dropCatalogCollabKeys removes the keys that split as a collab credit with a
// part whose normalized form a catalog artist's name or alias carries: the
// matcher links such a credit to that artist.
func dropCatalogCollabKeys(tx *gorm.DB, rows []radioArtistCandidateRow) ([]radioArtistCandidateRow, error) {
	partsByKey := map[string][]string{}
	var names []string
	for _, row := range rows {
		for _, part := range splitCollabArtistName(*row.ArtistKey) {
			name := normalizeName(part)
			partsByKey[*row.ArtistKey] = append(partsByKey[*row.ArtistKey], name)
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return rows, nil
	}
	var known []string
	if err := tx.Raw(radioCatalogNamesSQL, pq.Array(names)).Scan(&known).Error; err != nil {
		return nil, fmt.Errorf("matching collab parts to the catalog: %w", err)
	}
	return slices.DeleteFunc(rows, func(row radioArtistCandidateRow) bool {
		return slices.ContainsFunc(partsByKey[*row.ArtistKey], func(part string) bool {
			return slices.Contains(known, part)
		})
	}), nil
}

// radioArtistCandidateSortKeys maps each sort to the metric it ranks by first.
var radioArtistCandidateSortKeys = map[contracts.RadioCandidateSort]func(contracts.RadioArtistCandidate) float64{
	contracts.RadioCandidateSortScore:    func(c contracts.RadioArtistCandidate) float64 { return c.Score },
	contracts.RadioCandidateSortPlays:    func(c contracts.RadioArtistCandidate) float64 { return float64(c.PlayCount) },
	contracts.RadioCandidateSortStations: func(c contracts.RadioArtistCandidate) float64 { return float64(c.StationRowCount) },
}

// rankRadioArtistCandidates scores the qualifying keys, finds the MusicBrainz
// ids shared between keys, assigns the lanes and orders the candidates by the
// sort's metric, descending, then by score, the most recent play and the
// artist key. The evidence fields other than SharedMBArtistKeys are left
// empty. Every row must have a non-nil ArtistKey.
func rankRadioArtistCandidates(rows []radioArtistCandidateRow, sort contracts.RadioCandidateSort) []contracts.RadioArtistCandidate {
	keysByMBID := map[string][]string{}
	for _, row := range rows {
		for _, id := range row.MBArtistIDs {
			keysByMBID[id] = append(keysByMBID[id], *row.ArtistKey)
		}
	}

	candidates := make([]contracts.RadioArtistCandidate, 0, len(rows))
	for _, row := range rows {
		key := *row.ArtistKey
		mbIDs := []string(row.MBArtistIDs)
		if mbIDs == nil {
			mbIDs = []string{}
		}
		shared := []string{}
		for _, id := range mbIDs {
			for _, other := range keysByMBID[id] {
				if other != key && !slices.Contains(shared, other) {
					shared = append(shared, other)
				}
			}
		}
		slices.Sort(shared)
		c := contracts.RadioArtistCandidate{
			ArtistKey: key,
			Lane: RadioCandidateLaneFor(RadioCandidateLaneFacts{
				StationRows:      row.StationRowCount,
				DistinctMBIDs:    len(mbIDs),
				MBIDShared:       len(shared) > 0,
				NonLatinSpelling: row.NonLatinSpelling,
			}),
			Score:              RadioArtistCandidateScore(row.StationRowCount, row.PlayCount, row.ShowCount),
			PlayCount:          row.PlayCount,
			StationRowCount:    row.StationRowCount,
			StationFamilyCount: row.StationFamilyCount,
			ShowCount:          row.ShowCount,
			FirstAiredOn:       row.FirstAiredOn.Format(time.DateOnly),
			LastAiredOn:        row.LastAiredOn.Format(time.DateOnly),
			ArtistSpellings:    []contracts.RadioCandidateName{},
			Stations:           []contracts.RadioCandidateStation{},
			TopTracks:          []contracts.RadioCandidateTrack{},
			AlbumNames:         []contracts.RadioCandidateName{},
			LabelNames:         []contracts.RadioCandidateName{},
			MBArtistIDs:        mbIDs,
			MBArtistIDConflict: len(mbIDs) > 1,
			SharedMBArtistKeys: shared,
			NonLatinSpelling:   row.NonLatinSpelling,
		}
		if len(mbIDs) == 1 {
			c.MBArtistID = &mbIDs[0]
		}
		candidates = append(candidates, c)
	}
	metric := radioArtistCandidateSortKeys[sort]
	slices.SortFunc(candidates, func(a, b contracts.RadioArtistCandidate) int {
		if c := cmp.Compare(metric(b), metric(a)); c != 0 {
			return c
		}
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		if c := cmp.Compare(b.LastAiredOn, a.LastAiredOn); c != 0 {
			return c
		}
		return cmp.Compare(a.ArtistKey, b.ArtistKey)
	})
	return candidates
}

// attachRadioArtistCandidateEvidence fills the display name and evidence lists
// of the candidates in place.
func attachRadioArtistCandidateEvidence(tx *gorm.DB, windowStart string, candidates []contracts.RadioArtistCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	keys := make([]string, len(candidates))
	byKey := make(map[string]*contracts.RadioArtistCandidate, len(candidates))
	for i := range candidates {
		keys[i] = candidates[i].ArtistKey
		byKey[keys[i]] = &candidates[i]
	}
	var rows []radioCandidateEvidenceRow
	if err := tx.Raw(radioArtistCandidateEvidenceSQL, windowStart, pq.Array(keys)).Scan(&rows).Error; err != nil {
		return fmt.Errorf("reading artist candidate evidence: %w", err)
	}
	for _, row := range rows {
		c, ok := byKey[row.ArtistKey]
		if !ok {
			continue
		}
		switch row.Kind {
		case radioEvidenceSpelling:
			// Rows arrive by rank, so the first is the most common spelling.
			if len(c.ArtistSpellings) == 0 {
				c.DisplayName = row.Name
			}
			c.ArtistSpellings = append(c.ArtistSpellings, contracts.RadioCandidateName{Name: row.Name, PlayCount: row.PlayCount})
			c.ArtistSpellingCount = row.DistinctCount
		case radioEvidenceStation:
			c.Stations = append(c.Stations, contracts.RadioCandidateStation{
				StationID: row.RefID, Slug: row.Slug, Name: row.Name, PlayCount: row.PlayCount,
			})
		case radioEvidenceTrack:
			c.TopTracks = append(c.TopTracks, contracts.RadioCandidateTrack{
				TrackID: row.RefID, Title: row.Name, PlayCount: row.PlayCount,
			})
		case radioEvidenceAlbum:
			c.AlbumNames = append(c.AlbumNames, contracts.RadioCandidateName{Name: row.Name, PlayCount: row.PlayCount})
			c.AlbumNameCount = row.DistinctCount
		case radioEvidenceLabel:
			c.LabelNames = append(c.LabelNames, contracts.RadioCandidateName{Name: row.Name, PlayCount: row.PlayCount})
			c.LabelNameCount = row.DistinctCount
		}
	}
	return nil
}
