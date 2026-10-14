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

	"psychic-homily-backend/internal/services/contracts"
)

// A label candidate is a label key, radio_normalize_name of the raw label
// name, ranked from its counted plays. A counted play aired in the window
// (RadioCandidateWindowDays), is not excluded for the label ranking
// (RadioPlayExcludedSQL), and has a label name that normalizes to a non-empty
// key. The match state does not matter: it describes the artist match.
//
// A key qualifies when its counted plays clear the station floor, it is rising
// (RadioLabelIsRising over the recent and prior velocity windows), and neither
// of the following holds:
//   - any play of the key, at any time, is linked to a label;
//   - a catalog label's name normalizes to the key, the matcher's only label
//     rung (matchLabel).

const (
	// RadioLabelVelocityDays is the length of each velocity window. The
	// recent window is the last RadioLabelVelocityDays days of the candidate
	// window, ending on the current UTC date with no upper bound; the prior
	// window is the RadioLabelVelocityDays days before it.
	RadioLabelVelocityDays = 30
	// RadioLabelMinVelocity is the rising rule for a key with prior plays:
	// its recent plays must be at least this multiple of its prior plays.
	RadioLabelMinVelocity = 1.5
	// RadioLabelMinNewPlays is the rising rule for a key with no prior plays:
	// it needs at least this many recent plays.
	RadioLabelMinNewPlays = 3
)

// Both velocity windows lie inside the candidate window, whose start bounds
// every counted play; this fails to compile otherwise.
const _ = uint(RadioCandidateWindowDays - 2*RadioLabelVelocityDays)

// RadioLabelIsRising reports whether a key with the given recent and prior
// velocity-window play counts is gaining play.
func RadioLabelIsRising(recent, prior int) bool {
	if prior == 0 {
		return recent >= RadioLabelMinNewPlays
	}
	return float64(recent) >= RadioLabelMinVelocity*float64(prior)
}

// RadioLabelVelocity is recent / prior, or recent when prior is zero.
func RadioLabelVelocity(recent, prior int) float64 {
	if prior == 0 {
		return float64(recent)
	}
	return float64(recent) / float64(prior)
}

// RadioLabelCandidateScore is the label candidate ranking score: distinct
// station rows x ln(plays in the window) x RadioLabelVelocity.
func RadioLabelCandidateScore(stationRows, plays, recent, prior int) float64 {
	return float64(stationRows) * math.Log(float64(plays)) * RadioLabelVelocity(recent, prior)
}

// radioLabelCandidateRow is one row of radioLabelCandidatesSQL: a key that
// clears the station floor and is not in the catalog, with its counts, or,
// when LabelKey is nil, the row that only carries UnkeyablePlays.
type radioLabelCandidateRow struct {
	UnkeyablePlays     int64     `gorm:"column:unkeyable_plays"`
	LabelKey           *string   `gorm:"column:label_key"`
	PlayCount          int       `gorm:"column:play_count"`
	RecentPlayCount    int       `gorm:"column:recent_play_count"`
	PriorPlayCount     int       `gorm:"column:prior_play_count"`
	StationRowCount    int       `gorm:"column:station_row_count"`
	StationFamilyCount int       `gorm:"column:station_family_count"`
	ShowCount          int       `gorm:"column:show_count"`
	FirstAiredOn       time.Time `gorm:"column:first_aired_on"`
	LastAiredOn        time.Time `gorm:"column:last_aired_on"`
	NonLatinSpelling   bool      `gorm:"column:non_latin_spelling"`
}

// radioLabelCandidatePlaysFrom joins a play to its episode, show and station,
// exposes its label key as k.label_key, and keeps the plays in the window that
// are not excluded for the label ranking. Keyability is left to the caller.
// Its one parameter is the window's first air date.
var radioLabelCandidatePlaysFrom = `
FROM radio_plays p
JOIN radio_episodes e ON e.id = p.episode_id
JOIN radio_shows sh ON sh.id = e.show_id
JOIN radio_stations st ON st.id = sh.station_id
CROSS JOIN LATERAL (SELECT radio_normalize_name(p.label_name) AS label_key) k
WHERE e.air_date >= ?::date
  AND NOT (` + RadioPlayExcludedSQL(RadioRankingLabel, radioCandidatePlayCols) + `)`

// radioLabelCandidatesSQL returns every key that clears the station floor and
// is neither linked nor a catalog label name, with its counts, or, when no key
// does, one row whose label_key is NULL; every row carries the number of
// in-window plays whose label name normalizes to empty. The rising rule is
// applied by the caller. Parameters: the window's first air date, the recent
// window's first air date twice, then the prior window's first air date.
var radioLabelCandidatesSQL = `
WITH plays AS MATERIALIZED (
  SELECT k.label_key,
         public.immutable_unaccent(lower(p.label_name)) ~ '` + radioUnfoldableCharSQL + `' AS non_latin,
         sh.station_id,
         ` + RadioBroadcasterKeySQL("st") + ` AS family_key,
         e.show_id,
         e.air_date
  ` + radioLabelCandidatePlaysFrom + `
),
linked_keys AS (
  SELECT DISTINCT radio_normalize_name(label_name) AS label_key
  FROM radio_plays
  WHERE label_id IS NOT NULL
),
agg AS (
  SELECT label_key,
         count(*) AS play_count,
         count(*) FILTER (WHERE air_date >= ?::date) AS recent_play_count,
         count(*) FILTER (WHERE air_date < ?::date AND air_date >= ?::date) AS prior_play_count,
         count(DISTINCT station_id) AS station_row_count,
         count(DISTINCT family_key) AS station_family_count,
         count(DISTINCT show_id) AS show_count,
         min(air_date) AS first_aired_on,
         max(air_date) AS last_aired_on,
         bool_or(non_latin) AS non_latin_spelling
  FROM plays
  WHERE label_key <> ''
  GROUP BY label_key
  HAVING ` + RadioStationFloorSQL("station_id") + `
),
qualifying AS (
  SELECT agg.*
  FROM agg
  WHERE NOT EXISTS (SELECT 1 FROM linked_keys lk WHERE lk.label_key = agg.label_key)
    AND NOT EXISTS (SELECT 1 FROM labels l WHERE radio_normalize_name(l.name) = agg.label_key)
)
SELECT u.unkeyable_plays, q.label_key, q.play_count, q.recent_play_count, q.prior_play_count,
       q.station_row_count, q.station_family_count, q.show_count, q.first_aired_on, q.last_aired_on,
       q.non_latin_spelling
FROM (SELECT count(*) AS unkeyable_plays FROM plays WHERE label_key = '') u
LEFT JOIN qualifying q ON true`

// radioLabelEvidenceRow is one row of radioLabelCandidateEvidenceSQL. Every
// branch of that UNION ALL selects exactly these columns, in this order and
// under these names: kind, label_key, ref_id, name, slug, play_count,
// distinct_count, ref_ids, rn; rn only orders the rows and is not scanned.
// Kind says which candidate field the row fills.
type radioLabelEvidenceRow struct {
	Kind          string        `gorm:"column:kind"`
	LabelKey      string        `gorm:"column:label_key"`
	RefID         uint          `gorm:"column:ref_id"`
	Name          string        `gorm:"column:name"`
	Slug          string        `gorm:"column:slug"`
	PlayCount     int           `gorm:"column:play_count"`
	DistinctCount int           `gorm:"column:distinct_count"`
	RefIDs        pq.Int64Array `gorm:"column:ref_ids;type:bigint[]"`
}

const (
	radioLabelEvidenceArtist    = "artist"
	radioLabelEvidenceRelease   = "release"
	radioLabelEvidenceMBRelease = "mb_release"
)

// radioLabelEvidenceNamesSQL ranks the names in a column of the label
// evidence query's counted CTE: names group by radio_normalize_name, show
// their most common trimmed spelling and count all their plays. refIDs is the
// aggregate the ref_ids column carries for each name, SQL built from
// constants. Panics on a column that is not an identifier.
func radioLabelEvidenceNamesSQL(kind, column, refIDs string) string {
	col := mustSQLColumnRef(column)
	return fmt.Sprintf(`
SELECT '%[1]s' AS kind, label_key, 0 AS ref_id, name, '' AS slug,
       play_count::int AS play_count, distinct_count::int AS distinct_count, ref_ids, rn
FROM (
  SELECT label_key, name, play_count, ref_ids,
         count(*) OVER (PARTITION BY label_key) AS distinct_count,
         row_number() OVER (PARTITION BY label_key ORDER BY play_count DESC, name COLLATE "C") AS rn
  FROM (
    SELECT label_key, count(*) AS play_count,
           (array_agg(spelling ORDER BY spelling_count DESC, spelling COLLATE "C"))[1] AS name,
           %[3]s AS ref_ids
    FROM (
      SELECT c.*, radio_normalize_name(c.%[2]s) AS norm, btrim(c.%[2]s) AS spelling,
             count(*) OVER (PARTITION BY c.label_key, btrim(c.%[2]s)) AS spelling_count
      FROM counted c
      WHERE c.%[2]s ~ '[^[:space:]]'
    ) spellings
    WHERE norm <> ''
    GROUP BY label_key, norm
  ) names
) ranked
WHERE rn <= %[4]d`, kind, col, refIDs, radioCandidateTopNames)
}

// radioLabelCandidateEvidenceSQL returns the evidence rows of the selected
// keys, one radioLabelEvidenceRow per row. Parameters: the window's first air
// date, then the keys.
var radioLabelCandidateEvidenceSQL = `
WITH counted AS MATERIALIZED (
  SELECT k.label_key,
         p.label_name, p.artist_name, p.artist_id, p.album_title,
         CASE WHEN btrim(p.musicbrainz_release_id) ~ '` + mbidUUIDPattern + `'
              THEN lower(btrim(p.musicbrainz_release_id)) END AS mb_release_id,
         sh.station_id
  ` + radioLabelCandidatePlaysFrom + `
    AND k.label_key = ANY (?::text[])
)
SELECT '` + radioEvidenceSpelling + `' AS kind, label_key, 0 AS ref_id, name, '' AS slug,
       play_count::int AS play_count, distinct_count::int AS distinct_count, '{}'::bigint[] AS ref_ids, rn
FROM (
  SELECT label_key, btrim(label_name) AS name, count(*) AS play_count,
         count(*) OVER (PARTITION BY label_key) AS distinct_count,
         row_number() OVER (PARTITION BY label_key ORDER BY count(*) DESC, btrim(label_name) COLLATE "C") AS rn
  FROM counted
  GROUP BY label_key, btrim(label_name)
) spellings
WHERE rn <= ` + fmt.Sprint(radioCandidateTopNames) + `
UNION ALL
SELECT '` + radioEvidenceStation + `' AS kind, c.label_key, st.id AS ref_id, st.name AS name, st.slug AS slug,
       count(*)::int AS play_count, 0 AS distinct_count, '{}'::bigint[] AS ref_ids,
       row_number() OVER (PARTITION BY c.label_key ORDER BY count(*) DESC, st.id) AS rn
FROM counted c
JOIN radio_stations st ON st.id = c.station_id
GROUP BY c.label_key, st.id, st.name, st.slug
UNION ALL` + radioLabelEvidenceNamesSQL(radioLabelEvidenceArtist, "artist_name",
	`coalesce(array_agg(DISTINCT artist_id::bigint ORDER BY artist_id::bigint) FILTER (WHERE artist_id IS NOT NULL), '{}')`) + `
UNION ALL` + radioLabelEvidenceNamesSQL(radioLabelEvidenceRelease, "album_title", `'{}'::bigint[]`) + `
UNION ALL
SELECT '` + radioLabelEvidenceMBRelease + `' AS kind, label_key, 0 AS ref_id, mb_release_id AS name, '' AS slug,
       play_count::int AS play_count, distinct_count::int AS distinct_count, '{}'::bigint[] AS ref_ids, rn
FROM (
  SELECT label_key, mb_release_id, count(*) AS play_count,
         count(*) OVER (PARTITION BY label_key) AS distinct_count,
         row_number() OVER (PARTITION BY label_key ORDER BY count(*) DESC, mb_release_id COLLATE "C") AS rn
  FROM counted
  WHERE mb_release_id IS NOT NULL
  GROUP BY label_key, mb_release_id
) releases
WHERE rn <= ` + fmt.Sprint(radioCandidateTopNames) + `
ORDER BY kind, label_key, rn`

// ListLabelCandidates returns the label candidate ranking: the qualifying keys
// in the requested lanes, ordered by the requested sort, up to limit, each
// with its evidence. Everything is read from one snapshot.
func (r *RadioCandidatesReader) ListLabelCandidates(ctx context.Context, q contracts.RadioLabelCandidatesQuery) (*contracts.RadioLabelCandidateListing, error) {
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}
	if !q.Lane.Valid() {
		return nil, fmt.Errorf("unknown candidate lane filter %q", q.Lane)
	}
	if _, ok := radioLabelCandidateSortKeys[q.Sort]; !ok {
		return nil, fmt.Errorf("unknown label candidate sort %q", q.Sort)
	}
	if q.Limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", q.Limit)
	}

	asOf := r.now().UTC()
	windowStart := asOf.AddDate(0, 0, -(RadioCandidateWindowDays - 1))
	recentStart := asOf.AddDate(0, 0, -(RadioLabelVelocityDays - 1))
	priorStart := recentStart.AddDate(0, 0, -RadioLabelVelocityDays)
	listing := &contracts.RadioLabelCandidateListing{
		AsOf:           asOf.Format(time.DateOnly),
		WindowStart:    windowStart.Format(time.DateOnly),
		WindowDays:     RadioCandidateWindowDays,
		RecentStart:    recentStart.Format(time.DateOnly),
		PriorStart:     priorStart.Format(time.DateOnly),
		VelocityDays:   RadioLabelVelocityDays,
		MinStationRows: RadioMinDistinctStations,
		MinVelocity:    RadioLabelMinVelocity,
		MinNewPlays:    RadioLabelMinNewPlays,
	}

	ctx, cancel := context.WithTimeout(ctx, radioCandidateListingTimeout)
	defer cancel()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The server-side bound holds even if the client's cancellation is lost.
		if err := tx.Exec(fmt.Sprintf("SET LOCAL statement_timeout = %d", radioCandidateListingTimeout.Milliseconds())).Error; err != nil {
			return fmt.Errorf("bounding the listing: %w", err)
		}
		var rows []radioLabelCandidateRow
		if err := tx.Raw(radioLabelCandidatesSQL,
			listing.WindowStart, listing.RecentStart, listing.RecentStart, listing.PriorStart,
		).Scan(&rows).Error; err != nil {
			return fmt.Errorf("ranking label candidates: %w", err)
		}
		if len(rows) > 0 {
			listing.UnkeyablePlays = rows[0].UnkeyablePlays
		}
		rows = slices.DeleteFunc(rows, func(row radioLabelCandidateRow) bool {
			return row.LabelKey == nil || !RadioLabelIsRising(row.RecentPlayCount, row.PriorPlayCount)
		})

		candidates := rankRadioLabelCandidates(rows, q.Sort)
		listing.Candidates = make([]contracts.RadioLabelCandidate, 0, min(q.Limit, len(candidates)))
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
		return attachRadioLabelCandidateEvidence(tx, listing.WindowStart, listing.Candidates)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return listing, nil
}

// radioLabelCandidateSortKeys maps each sort to the metric it ranks by first.
var radioLabelCandidateSortKeys = map[contracts.RadioLabelCandidateSort]func(contracts.RadioLabelCandidate) float64{
	contracts.RadioLabelCandidateSortScore:    func(c contracts.RadioLabelCandidate) float64 { return c.Score },
	contracts.RadioLabelCandidateSortPlays:    func(c contracts.RadioLabelCandidate) float64 { return float64(c.PlayCount) },
	contracts.RadioLabelCandidateSortStations: func(c contracts.RadioLabelCandidate) float64 { return float64(c.StationRowCount) },
	contracts.RadioLabelCandidateSortVelocity: func(c contracts.RadioLabelCandidate) float64 { return c.Velocity },
}

// rankRadioLabelCandidates scores the qualifying keys, assigns the lanes and
// orders the candidates by the sort's metric, descending, then by score, the
// most recent play and the label key. The evidence fields are left empty.
// Every row must have a non-nil LabelKey.
func rankRadioLabelCandidates(rows []radioLabelCandidateRow, sort contracts.RadioLabelCandidateSort) []contracts.RadioLabelCandidate {
	candidates := make([]contracts.RadioLabelCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, contracts.RadioLabelCandidate{
			LabelKey: *row.LabelKey,
			// Plays carry no MusicBrainz label id, so no label has the one id
			// the auto lane needs.
			Lane:               RadioCandidateLaneFor(RadioCandidateLaneFacts{StationRows: row.StationRowCount, NonLatinSpelling: row.NonLatinSpelling}),
			Score:              RadioLabelCandidateScore(row.StationRowCount, row.PlayCount, row.RecentPlayCount, row.PriorPlayCount),
			Velocity:           RadioLabelVelocity(row.RecentPlayCount, row.PriorPlayCount),
			PlayCount:          row.PlayCount,
			RecentPlayCount:    row.RecentPlayCount,
			PriorPlayCount:     row.PriorPlayCount,
			StationRowCount:    row.StationRowCount,
			StationFamilyCount: row.StationFamilyCount,
			ShowCount:          row.ShowCount,
			FirstAiredOn:       row.FirstAiredOn.Format(time.DateOnly),
			LastAiredOn:        row.LastAiredOn.Format(time.DateOnly),
			LabelSpellings:     []contracts.RadioCandidateName{},
			Stations:           []contracts.RadioCandidateStation{},
			Artists:            []contracts.RadioLabelCandidateArtist{},
			ReleaseNames:       []contracts.RadioCandidateName{},
			MBReleaseIDs:       []string{},
			NonLatinSpelling:   row.NonLatinSpelling,
		})
	}
	metric := radioLabelCandidateSortKeys[sort]
	slices.SortFunc(candidates, func(a, b contracts.RadioLabelCandidate) int {
		if c := cmp.Compare(metric(b), metric(a)); c != 0 {
			return c
		}
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		if c := cmp.Compare(b.LastAiredOn, a.LastAiredOn); c != 0 {
			return c
		}
		return cmp.Compare(a.LabelKey, b.LabelKey)
	})
	return candidates
}

// attachRadioLabelCandidateEvidence fills the display name and evidence lists
// of the candidates in place.
func attachRadioLabelCandidateEvidence(tx *gorm.DB, windowStart string, candidates []contracts.RadioLabelCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	keys := make([]string, len(candidates))
	byKey := make(map[string]*contracts.RadioLabelCandidate, len(candidates))
	for i := range candidates {
		keys[i] = candidates[i].LabelKey
		byKey[keys[i]] = &candidates[i]
	}
	var rows []radioLabelEvidenceRow
	if err := tx.Raw(radioLabelCandidateEvidenceSQL, windowStart, pq.Array(keys)).Scan(&rows).Error; err != nil {
		return fmt.Errorf("reading label candidate evidence: %w", err)
	}
	for _, row := range rows {
		c, ok := byKey[row.LabelKey]
		if !ok {
			continue
		}
		switch row.Kind {
		case radioEvidenceSpelling:
			// Rows arrive by rank, so the first is the most common spelling.
			if len(c.LabelSpellings) == 0 {
				c.DisplayName = row.Name
			}
			c.LabelSpellings = append(c.LabelSpellings, contracts.RadioCandidateName{Name: row.Name, PlayCount: row.PlayCount})
			c.LabelSpellingCount = row.DistinctCount
		case radioEvidenceStation:
			c.Stations = append(c.Stations, contracts.RadioCandidateStation{
				StationID: row.RefID, Slug: row.Slug, Name: row.Name, PlayCount: row.PlayCount,
			})
		case radioLabelEvidenceArtist:
			ids := make([]uint, len(row.RefIDs))
			for i, id := range row.RefIDs {
				ids[i] = uint(id)
			}
			c.Artists = append(c.Artists, contracts.RadioLabelCandidateArtist{Name: row.Name, PlayCount: row.PlayCount, ArtistIDs: ids})
			c.ArtistCount = row.DistinctCount
		case radioLabelEvidenceRelease:
			c.ReleaseNames = append(c.ReleaseNames, contracts.RadioCandidateName{Name: row.Name, PlayCount: row.PlayCount})
			c.ReleaseNameCount = row.DistinctCount
		case radioLabelEvidenceMBRelease:
			c.MBReleaseIDs = append(c.MBReleaseIDs, row.Name)
			c.MBReleaseIDCount = row.DistinctCount
		}
	}
	return nil
}
