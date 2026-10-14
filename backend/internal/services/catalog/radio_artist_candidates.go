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

// An artist candidate is an artist key (radio_normalize_name of the raw
// artist name, which is also radio_track_artist_key) that no catalog artist
// answers to, ranked from its counted plays. A counted play aired in the
// window, is not excluded for the artist ranking (RadioPlayExcludedSQL), and is
// not in the ambiguous match state. A key qualifies when its counted plays
// clear both floors, none of them is linked to an artist, and the catalog has
// no artist whose name, alias or MusicBrainz id the matcher would link it to.

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
)

// radioMBIDPatternSQL is the shape a MusicBrainz id must have to count; any
// other value is treated as absent.
const radioMBIDPatternSQL = `'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'`

// RadioArtistCandidateScore is the artist candidate ranking score: distinct
// station rows x ln(plays) x (1 + distinct shows / 10).
func RadioArtistCandidateScore(stationRows, plays, shows int) float64 {
	return float64(stationRows) * math.Log(float64(plays)) * (1 + float64(shows)/10)
}

// RadioCandidateLaneFor returns a candidate's lane: auto when its plays carry
// exactly one distinct MusicBrainz id and it clears the station floor, human
// otherwise. Several ids mean the name covers more than one act, so a person
// decides.
func RadioCandidateLaneFor(stationRows, distinctMBIDs int) contracts.RadioCandidateLane {
	if distinctMBIDs == 1 && RadioMeetsStationFloor(stationRows) {
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

// radioArtistCandidateRow is one qualifying key as the aggregate query returns
// it.
type radioArtistCandidateRow struct {
	ArtistKey          string         `gorm:"column:artist_key"`
	PlayCount          int            `gorm:"column:play_count"`
	StationRowCount    int            `gorm:"column:station_row_count"`
	StationFamilyCount int            `gorm:"column:station_family_count"`
	ShowCount          int            `gorm:"column:show_count"`
	FirstAiredOn       time.Time      `gorm:"column:first_aired_on"`
	LastAiredOn        time.Time      `gorm:"column:last_aired_on"`
	MBArtistIDs        pq.StringArray `gorm:"column:mb_artist_ids;type:text[]"`
}

// radioCandidateEvidenceRow is one row of the evidence query; Kind says which
// field of the candidate it fills.
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

// radioCandidatePlaysFrom joins a play to its episode, show and station and
// keeps only the plays an artist candidate counts. Its one parameter is the
// window's first air date.
var radioCandidatePlaysFrom = `
FROM radio_plays p
JOIN radio_episodes e ON e.id = p.episode_id
JOIN radio_shows sh ON sh.id = e.show_id
JOIN radio_stations st ON st.id = sh.station_id
WHERE e.air_date >= ?::date
  AND p.match_state <> '` + catalogm.RadioPlayMatchStateAmbiguous + `'
  AND NOT (` + RadioPlayExcludedSQL(RadioRankingArtist, radioCandidatePlayCols) + `)`

// radioArtistCandidatesSQL returns every qualifying key with its counts.
// Parameters: the window's first air date, then the play floor.
var radioArtistCandidatesSQL = `
WITH counted AS (
  SELECT radio_normalize_name(p.artist_name) AS artist_key,
         p.artist_id,
         sh.station_id,
         ` + RadioBroadcasterKeySQL("st") + ` AS family_key,
         e.show_id,
         e.air_date,
         CASE WHEN lower(btrim(p.musicbrainz_artist_id)) ~ ` + radioMBIDPatternSQL + `
              THEN lower(btrim(p.musicbrainz_artist_id)) END AS mb_artist_id
  ` + radioCandidatePlaysFrom + `
),
catalog_mb AS (
  SELECT DISTINCT lower(btrim(musicbrainz_artist_id)) AS id
  FROM artists
  WHERE musicbrainz_artist_id IS NOT NULL
),
agg AS (
  SELECT c.artist_key,
         count(*) AS play_count,
         count(DISTINCT c.station_id) AS station_row_count,
         count(DISTINCT c.family_key) AS station_family_count,
         count(DISTINCT c.show_id) AS show_count,
         min(c.air_date) AS first_aired_on,
         max(c.air_date) AS last_aired_on,
         coalesce(array_agg(DISTINCT c.mb_artist_id ORDER BY c.mb_artist_id)
                  FILTER (WHERE c.mb_artist_id IS NOT NULL), '{}') AS mb_artist_ids,
         bool_or(cm.id IS NOT NULL) AS catalog_mb
  FROM counted c
  LEFT JOIN catalog_mb cm ON cm.id = c.mb_artist_id
  WHERE c.artist_key <> ''
  GROUP BY c.artist_key
  HAVING ` + RadioStationFloorSQL("c.station_id") + `
     AND count(*) >= ?
     AND bool_and(c.artist_id IS NULL)
)
SELECT artist_key, play_count, station_row_count, station_family_count, show_count,
       first_aired_on, last_aired_on, mb_artist_ids
FROM agg
WHERE NOT catalog_mb
  AND NOT EXISTS (SELECT 1 FROM artists a WHERE radio_normalize_name(a.name) = agg.artist_key)
  AND NOT EXISTS (SELECT 1 FROM artist_aliases al WHERE radio_normalize_name(al.alias) = agg.artist_key)`

// radioCandidateNamesSQL ranks the album or label names on the selected keys'
// counted plays: names group by radio_normalize_name, show their most common
// trimmed spelling, and only plays for which filter holds contribute.
func radioCandidateNamesSQL(kind, column, filter string) string {
	return fmt.Sprintf(`
SELECT '%[1]s' AS kind, artist_key, 0 AS ref_id, name, '' AS slug, play_count::int, distinct_count::int, rn
FROM (
  SELECT artist_key, name, play_count,
         count(*) OVER (PARTITION BY artist_key) AS distinct_count,
         row_number() OVER (PARTITION BY artist_key ORDER BY play_count DESC, name COLLATE "C") AS rn
  FROM (
    SELECT artist_key, norm, sum(n) AS play_count,
           (array_agg(spelling ORDER BY n DESC, spelling COLLATE "C"))[1] AS name
    FROM (
      SELECT artist_key, radio_normalize_name(%[2]s) AS norm, btrim(%[2]s) AS spelling, count(*) AS n
      FROM counted
      WHERE %[2]s ~ '[^[:space:]]' AND %[3]s
      GROUP BY 1, 2, 3
    ) spellings
    WHERE norm <> ''
    GROUP BY artist_key, norm
  ) names
) ranked
WHERE rn <= %[4]d`, kind, column, filter, radioCandidateTopNames)
}

// radioArtistCandidateEvidenceSQL returns the evidence rows of the selected
// keys. Parameters: the window's first air date, then the keys. The
// artist_id IS NULL condition holds for every counted play of a qualifying key
// and lets the lookup use the partial normalized-name index.
var radioArtistCandidateEvidenceSQL = `
WITH counted AS MATERIALIZED (
  SELECT radio_normalize_name(p.artist_name) AS artist_key,
         p.artist_name, p.track_title, p.album_title,
         sh.station_id,
         ` + RadioPlayExcludedSQL(RadioRankingTrack, radioCandidatePlayCols) + ` AS track_excluded,
         ` + RadioPlayExcludedSQL(RadioRankingLabel, radioCandidatePlayCols) + ` AS label_excluded,
         p.label_name
  ` + radioCandidatePlaysFrom + `
    AND p.artist_id IS NULL
    AND radio_normalize_name(p.artist_name) = ANY (?::text[])
)
SELECT '` + radioEvidenceSpelling + `' AS kind, artist_key, 0 AS ref_id, artist_name AS name, '' AS slug,
       play_count::int, 0 AS distinct_count, 1::bigint AS rn
FROM (
  SELECT DISTINCT ON (artist_key) artist_key, artist_name, count(*) AS play_count
  FROM counted
  GROUP BY artist_key, artist_name
  ORDER BY artist_key, count(*) DESC, artist_name COLLATE "C"
) spelling
UNION ALL
SELECT '` + radioEvidenceStation + `', c.artist_key, st.id, st.name, st.slug, count(*)::int, 0,
       row_number() OVER (PARTITION BY c.artist_key ORDER BY count(*) DESC, st.id)
FROM counted c
JOIN radio_stations st ON st.id = c.station_id
GROUP BY c.artist_key, st.id, st.name, st.slug
UNION ALL
SELECT '` + radioEvidenceTrack + `', artist_key, track_id, title, '', play_count::int, 0, rn
FROM (
  SELECT c.artist_key, t.id AS track_id, t.display_title AS title, count(*) AS play_count,
         row_number() OVER (PARTITION BY c.artist_key ORDER BY count(*) DESC, t.id) AS rn
  FROM counted c
  JOIN radio_track_keys k ON k.artist_key = c.artist_key AND k.title_key = radio_track_title_key(c.track_title)
  JOIN radio_tracks t ON t.id = k.track_id
  WHERE NOT c.track_excluded AND NOT t.unkeyed
  GROUP BY c.artist_key, t.id, t.display_title
) tracks
WHERE rn <= ` + fmt.Sprint(radioCandidateTopTracks) + `
UNION ALL` + radioCandidateNamesSQL(radioEvidenceAlbum, "album_title", "true") + `
UNION ALL` + radioCandidateNamesSQL(radioEvidenceLabel, "label_name", "NOT label_excluded") + `
ORDER BY kind, artist_key, rn`

// ListArtistCandidates returns the artist candidate ranking: the qualifying
// keys in the requested lanes, ordered by the requested sort, up to limit, each
// with its evidence. Everything is read from one snapshot.
func (r *RadioCandidatesReader) ListArtistCandidates(ctx context.Context, q contracts.RadioArtistCandidatesQuery) (*contracts.RadioArtistCandidateListing, error) {
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}
	switch q.Lane {
	case contracts.RadioCandidateLaneFilterAll, contracts.RadioCandidateLaneFilterAuto, contracts.RadioCandidateLaneFilterHuman:
	default:
		return nil, fmt.Errorf("unknown candidate lane filter %q", q.Lane)
	}
	if _, ok := radioArtistCandidateSortKeys[q.Sort]; !ok {
		return nil, fmt.Errorf("unknown artist candidate sort %q", q.Sort)
	}
	if q.Limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", q.Limit)
	}

	asOf := r.now().UTC()
	windowStart := asOf.AddDate(0, 0, -(RadioCandidateWindowDays - 1))
	listing := &contracts.RadioArtistCandidateListing{
		AsOf:           asOf.Format(time.DateOnly),
		WindowStart:    windowStart.Format(time.DateOnly),
		WindowDays:     RadioCandidateWindowDays,
		MinStationRows: RadioMinDistinctStations,
		MinPlays:       RadioCandidateMinPlays,
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []radioArtistCandidateRow
		if err := tx.Raw(radioArtistCandidatesSQL, listing.WindowStart, RadioCandidateMinPlays).Scan(&rows).Error; err != nil {
			return fmt.Errorf("ranking artist candidates: %w", err)
		}
		candidates := rankRadioArtistCandidates(rows, q.Sort)
		listing.Candidates = make([]contracts.RadioArtistCandidate, 0, min(q.Limit, len(candidates)))
		for _, c := range candidates {
			if c.Lane == contracts.RadioCandidateLaneAuto {
				listing.AutoTotal++
			} else {
				listing.HumanTotal++
			}
			if q.Lane != contracts.RadioCandidateLaneFilterAll && string(q.Lane) != string(c.Lane) {
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
		if err := tx.Raw("SELECT (SELECT last_success_at FROM background_service_runs WHERE name = ?)", RadioTracksRebuildLoopName).
			Row().Scan(&listing.TracksRebuiltAt); err != nil {
			return fmt.Errorf("reading the tracks rebuild time: %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return listing, nil
}

// radioArtistCandidateSortKeys maps each sort to the metric it ranks by first.
var radioArtistCandidateSortKeys = map[contracts.RadioArtistCandidateSort]func(contracts.RadioArtistCandidate) float64{
	contracts.RadioArtistCandidateSortScore:    func(c contracts.RadioArtistCandidate) float64 { return c.Score },
	contracts.RadioArtistCandidateSortPlays:    func(c contracts.RadioArtistCandidate) float64 { return float64(c.PlayCount) },
	contracts.RadioArtistCandidateSortStations: func(c contracts.RadioArtistCandidate) float64 { return float64(c.StationRowCount) },
}

// rankRadioArtistCandidates scores the qualifying keys, assigns their lanes
// and orders them by the sort's metric, descending, then by score, the most
// recent play and the artist key. The evidence fields are left empty.
func rankRadioArtistCandidates(rows []radioArtistCandidateRow, sort contracts.RadioArtistCandidateSort) []contracts.RadioArtistCandidate {
	candidates := make([]contracts.RadioArtistCandidate, 0, len(rows))
	for _, row := range rows {
		mbIDs := []string(row.MBArtistIDs)
		if mbIDs == nil {
			mbIDs = []string{}
		}
		c := contracts.RadioArtistCandidate{
			ArtistKey:          row.ArtistKey,
			Lane:               RadioCandidateLaneFor(row.StationRowCount, len(mbIDs)),
			Score:              RadioArtistCandidateScore(row.StationRowCount, row.PlayCount, row.ShowCount),
			PlayCount:          row.PlayCount,
			StationRowCount:    row.StationRowCount,
			StationFamilyCount: row.StationFamilyCount,
			ShowCount:          row.ShowCount,
			FirstAiredOn:       row.FirstAiredOn.Format(time.DateOnly),
			LastAiredOn:        row.LastAiredOn.Format(time.DateOnly),
			Stations:           []contracts.RadioCandidateStation{},
			TopTracks:          []contracts.RadioCandidateTrack{},
			AlbumNames:         []contracts.RadioCandidateName{},
			LabelNames:         []contracts.RadioCandidateName{},
			MBArtistIDs:        mbIDs,
			MBArtistIDConflict: len(mbIDs) > 1,
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
			c.DisplayName = row.Name
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
