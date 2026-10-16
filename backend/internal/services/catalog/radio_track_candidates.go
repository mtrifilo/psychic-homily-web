package catalog

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/contracts"
)

// A track candidate is a keyed radio_tracks row ranked from its counted plays.
// A counted play aired in the window, is not excluded for the track ranking
// (RadioPlayExcludedSQL), is not in the ambiguous match state, and has a track
// key that radio_track_keys maps to a keyed track. A track qualifies when its
// counted plays clear the same station and play floors as an artist
// candidate. A qualifying track is in at most one view:
//   - unlinked: a counted play is linked to an artist, and no play of the
//     track, at any time, is linked to a release credited to that play's
//     linked artist (artist_releases); a link to a release the play's artist
//     is not credited on does not count;
//   - unmatched: no counted play is linked to an artist, and at least one
//     artist key among the counted plays is a qualifying artist candidate's
//     key, so each row joins to an artist candidate it supports.

// RadioTrackCandidateScore is the track candidate ranking score: distinct
// station rows x ln(plays).
func RadioTrackCandidateScore(stationRows, plays int) float64 {
	return float64(stationRows) * math.Log(float64(plays))
}

// radioTrackKeyColumnsSQL selects the track key halves of a radio_plays row
// aliased p, recomputed from the raw names, as track_artist_key and
// track_title_key.
const radioTrackKeyColumnsSQL = `radio_track_artist_key(p.artist_name) AS track_artist_key,
         radio_track_title_key(p.track_title) AS track_title_key`

// radioTrackJoinSQL joins a row of a CTE aliased pl that carries
// radioTrackKeyColumnsSQL to the keyed radio_tracks row its key maps to,
// exposing its id as t.id, NULL when the key maps to none or to the unkeyed
// marker.
const radioTrackJoinSQL = `
LEFT JOIN radio_track_keys tk ON tk.artist_key = pl.track_artist_key AND tk.title_key = pl.track_title_key
LEFT JOIN radio_tracks t ON t.id = tk.track_id AND NOT t.unkeyed`

// radioTrackCandidateRow is one row of radioTrackCandidatesSQL: a qualifying
// track with its counts, or, when TrackID is nil, the row that only carries
// UntrackedPlays.
type radioTrackCandidateRow struct {
	UntrackedPlays     int64          `gorm:"column:untracked_plays"`
	TrackID            *uint          `gorm:"column:track_id"`
	PlayCount          int            `gorm:"column:play_count"`
	StationRowCount    int            `gorm:"column:station_row_count"`
	StationFamilyCount int            `gorm:"column:station_family_count"`
	ShowCount          int            `gorm:"column:show_count"`
	FirstAiredOn       time.Time      `gorm:"column:first_aired_on"`
	LastAiredOn        time.Time      `gorm:"column:last_aired_on"`
	MBReleaseIDs       pq.StringArray `gorm:"column:mb_release_ids;type:text[]"`
	ArtistKeys         pq.StringArray `gorm:"column:artist_keys;type:text[]"`
	ArtistIDs          pq.Int64Array  `gorm:"column:artist_ids;type:bigint[]"`
	// CreditedReleaseLinked is true when some play of the track, at any
	// time, is linked to a release credited to that play's linked artist.
	CreditedReleaseLinked bool `gorm:"column:credited_release_linked"`
	NonLatinSpelling      bool `gorm:"column:non_latin_spelling"`
	VariantReleaseID      bool `gorm:"column:variant_release_id"`
}

// radioTrackCandidatesSQL returns every qualifying track with its counts or,
// when no track qualifies, one row whose track_id is NULL. Every row carries
// untracked_plays: the plays in the window, not excluded and not ambiguous,
// whose key maps to no keyed track. credited_release_linked reads every play
// of the track, whatever its window, exclusion or match state. Parameters:
// the window's first air date, then the play floor.
var radioTrackCandidatesSQL = `
WITH plays AS MATERIALIZED (
  SELECT ` + radioTrackKeyColumnsSQL + `,
         k.artist_key,
         p.artist_id,
         sh.station_id,
         ` + RadioBroadcasterKeySQL("st") + ` AS family_key,
         e.show_id,
         e.air_date,
         CASE WHEN btrim(p.musicbrainz_release_id) ~ '` + mbidUUIDPattern + `'
              THEN lower(btrim(p.musicbrainz_release_id)) END AS mb_release_id,
         radio_normalize_name(p.track_title) AS title_norm,
         (` + radioNonLatinSQL("p.artist_name") + ` OR ` + radioNonLatinSQL("p.track_title") + `) AS non_latin
  ` + radioCandidatePlaysFromSQL(RadioRankingTrack) + `
),
tracked AS MATERIALIZED (
  SELECT pl.*, t.id AS track_id
  FROM plays pl` + radioTrackJoinSQL + `
),
credited AS MATERIALIZED (
  SELECT DISTINCT t.id AS track_id
  FROM (
    SELECT ` + radioTrackKeyColumnsSQL + `
    FROM radio_plays p
    WHERE p.release_id IS NOT NULL
      AND p.artist_id IS NOT NULL
      AND EXISTS (SELECT 1 FROM artist_releases ar
                  WHERE ar.release_id = p.release_id AND ar.artist_id = p.artist_id)
  ) pl` + radioTrackJoinSQL + `
  WHERE t.id IS NOT NULL
),
qualifying AS (
  SELECT track_id,
         count(*) AS play_count,
         count(DISTINCT station_id) AS station_row_count,
         count(DISTINCT family_key) AS station_family_count,
         count(DISTINCT show_id) AS show_count,
         min(air_date) AS first_aired_on,
         max(air_date) AS last_aired_on,
         coalesce(array_agg(DISTINCT mb_release_id ORDER BY mb_release_id)
                  FILTER (WHERE mb_release_id IS NOT NULL), '{}') AS mb_release_ids,
         array_agg(DISTINCT artist_key ORDER BY artist_key) AS artist_keys,
         coalesce(array_agg(DISTINCT artist_id ORDER BY artist_id)
                  FILTER (WHERE artist_id IS NOT NULL), '{}') AS artist_ids,
         coalesce(bool_or(non_latin), false) AS non_latin_spelling,
         -- True when every play carrying a release id has a title the key
         -- strips, so the ids may belong to what was stripped
         -- (contracts.RadioTrackCandidate.VariantReleaseID).
         coalesce(bool_and(title_norm IS DISTINCT FROM track_title_key)
                  FILTER (WHERE mb_release_id IS NOT NULL), false) AS variant_release_id
  FROM tracked
  WHERE track_id IS NOT NULL
  GROUP BY track_id
  HAVING ` + RadioStationFloorSQL("station_id") + `
     AND count(*) >= ?
)
SELECT u.untracked_plays, q.track_id, q.play_count, q.station_row_count, q.station_family_count,
       q.show_count, q.first_aired_on, q.last_aired_on, q.mb_release_ids, q.artist_keys,
       q.artist_ids, q.track_id IN (SELECT track_id FROM credited) AS credited_release_linked,
       q.non_latin_spelling, q.variant_release_id
FROM (SELECT count(*) AS untracked_plays FROM tracked WHERE track_id IS NULL) u
LEFT JOIN qualifying q ON true`

// inRadioTrackCandidateView reports whether a qualifying track is in the view.
// artistCandidateKeys holds the qualifying artist candidates' keys; only the
// unmatched view reads it.
func inRadioTrackCandidateView(row radioTrackCandidateRow, view contracts.RadioTrackCandidateView, artistCandidateKeys map[string]bool) bool {
	artistLinked := len(row.ArtistIDs) > 0
	switch view {
	case contracts.RadioTrackCandidateViewUnlinked:
		return artistLinked && !row.CreditedReleaseLinked
	case contracts.RadioTrackCandidateViewUnmatched:
		return !artistLinked && slices.ContainsFunc(row.ArtistKeys, func(key string) bool { return artistCandidateKeys[key] })
	}
	return false
}

// radioTrackCandidateSortKeys maps each sort to the metric it ranks by first.
var radioTrackCandidateSortKeys = map[contracts.RadioCandidateSort]func(contracts.RadioTrackCandidate) float64{
	contracts.RadioCandidateSortScore:    func(c contracts.RadioTrackCandidate) float64 { return c.Score },
	contracts.RadioCandidateSortPlays:    func(c contracts.RadioTrackCandidate) float64 { return float64(c.PlayCount) },
	contracts.RadioCandidateSortStations: func(c contracts.RadioTrackCandidate) float64 { return float64(c.StationRowCount) },
}

// rankRadioTrackCandidates scores the rows, assigns the lanes and orders the
// candidates by the sort's metric, descending, then by score, the most recent
// play and the track id. ArtistCandidateKeys holds the row keys found in
// artistCandidateKeys, in key order until the evidence orders them; the other
// evidence fields are left empty. Every row must have a non-nil TrackID.
func rankRadioTrackCandidates(rows []radioTrackCandidateRow, artistCandidateKeys map[string]bool, sort contracts.RadioCandidateSort) []contracts.RadioTrackCandidate {
	candidates := make([]contracts.RadioTrackCandidate, 0, len(rows))
	for _, row := range rows {
		mbIDs := []string(row.MBReleaseIDs)
		if mbIDs == nil {
			mbIDs = []string{}
		}
		candidateKeys := []string{}
		for _, key := range row.ArtistKeys {
			if artistCandidateKeys[key] {
				candidateKeys = append(candidateKeys, key)
			}
		}
		c := contracts.RadioTrackCandidate{
			TrackID: *row.TrackID,
			// MBIDShared stays false: the tracks of one release share its id,
			// so sharing it with another track does not contest it.
			Lane: RadioCandidateLaneFor(RadioCandidateLaneFacts{
				StationRows:      row.StationRowCount,
				DistinctMBIDs:    len(mbIDs),
				MBIDOnVariant:    row.VariantReleaseID,
				NonLatinSpelling: row.NonLatinSpelling,
			}),
			ArtistCandidateKeys: candidateKeys,
			Score:               RadioTrackCandidateScore(row.StationRowCount, row.PlayCount),
			PlayCount:           row.PlayCount,
			StationRowCount:     row.StationRowCount,
			StationFamilyCount:  row.StationFamilyCount,
			ShowCount:           row.ShowCount,
			FirstAiredOn:        row.FirstAiredOn.Format(time.DateOnly),
			LastAiredOn:         row.LastAiredOn.Format(time.DateOnly),
			TitleSpellings:      []contracts.RadioCandidateName{},
			ArtistKeys:          []contracts.RadioCandidateName{},
			Artists:             []contracts.RadioCandidateArtist{},
			Stations:            []contracts.RadioCandidateStation{},
			AlbumNames:          []contracts.RadioCandidateName{},
			MBReleaseIDs:        mbIDs,
			NonLatinSpelling:    row.NonLatinSpelling,
			VariantReleaseID:    row.VariantReleaseID,
		}
		if len(mbIDs) == 1 {
			c.MBReleaseID = &mbIDs[0]
		}
		candidates = append(candidates, c)
	}
	sortRadioCandidates(candidates, radioTrackCandidateSortKeys[sort], func(c contracts.RadioTrackCandidate) radioCandidateOrder {
		return radioCandidateOrder{score: c.Score, lastAiredOn: c.LastAiredOn}
	}, func(a, b contracts.RadioTrackCandidate) int { return cmp.Compare(a.TrackID, b.TrackID) })
	return candidates
}

// ListTrackCandidates returns one view of the track candidate ranking: the
// view's qualifying tracks in the requested lanes, ordered by the requested
// sort, up to limit, each with its evidence. Everything is read from one
// snapshot. It creates nothing.
func (r *RadioCandidatesReader) ListTrackCandidates(ctx context.Context, q contracts.RadioTrackCandidatesQuery) (*contracts.RadioTrackCandidateListing, error) {
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}
	if !q.View.Valid() {
		return nil, fmt.Errorf("unknown track candidate view %q", q.View)
	}
	if !q.Lane.Valid() {
		return nil, fmt.Errorf("unknown candidate lane filter %q", q.Lane)
	}
	if _, ok := radioTrackCandidateSortKeys[q.Sort]; !ok {
		return nil, fmt.Errorf("unknown track candidate sort %q", q.Sort)
	}
	if q.Limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", q.Limit)
	}

	asOf, windowStart := r.window()
	listing := &contracts.RadioTrackCandidateListing{
		View:           q.View,
		AsOf:           asOf,
		WindowStart:    windowStart,
		WindowDays:     RadioCandidateWindowDays,
		MinStationRows: RadioMinDistinctStations,
		MinPlays:       RadioCandidateMinPlays,
	}

	err := r.inSnapshot(ctx, func(tx *gorm.DB) error {
		var rows []radioTrackCandidateRow
		if err := tx.Raw(radioTrackCandidatesSQL, windowStart, RadioCandidateMinPlays).Scan(&rows).Error; err != nil {
			return fmt.Errorf("ranking track candidates: %w", err)
		}
		if len(rows) > 0 {
			listing.UntrackedPlays = rows[0].UntrackedPlays
		}
		rows = slices.DeleteFunc(rows, func(row radioTrackCandidateRow) bool { return row.TrackID == nil })
		listing.QualifyingTracks = len(rows)

		var artistCandidateKeys map[string]bool
		if q.View == contracts.RadioTrackCandidateViewUnmatched {
			artistRows, _, err := qualifyingRadioArtistRows(tx, windowStart)
			if err != nil {
				return err
			}
			artistCandidateKeys = make(map[string]bool, len(artistRows))
			for _, row := range artistRows {
				artistCandidateKeys[*row.ArtistKey] = true
			}
		}
		rows = slices.DeleteFunc(rows, func(row radioTrackCandidateRow) bool {
			return !inRadioTrackCandidateView(row, q.View, artistCandidateKeys)
		})

		page := pageRadioCandidates(rankRadioTrackCandidates(rows, artistCandidateKeys, q.Sort),
			func(c contracts.RadioTrackCandidate) contracts.RadioCandidateLane { return c.Lane }, q.Lane, q.Limit)
		listing.Candidates, listing.Total, listing.AutoTotal, listing.HumanTotal = page.candidates, page.total, page.auto, page.human

		if err := attachRadioTrackCandidateEvidence(tx, windowStart, listing.Candidates, rows); err != nil {
			return err
		}
		var err error
		listing.TracksRebuiltAt, err = radioTracksRebuiltAt(tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return listing, nil
}

// radioTrackEvidenceRow is one row of radioTrackCandidateEvidenceSQL. Every
// branch of that UNION ALL selects exactly these columns, in this order and
// under these names: kind, track_id, ref_id, name, slug, play_count,
// distinct_count, rn. Kind says which candidate field the row fills.
type radioTrackEvidenceRow struct {
	Kind          string `gorm:"column:kind"`
	TrackID       uint   `gorm:"column:track_id"`
	RefID         uint   `gorm:"column:ref_id"`
	Name          string `gorm:"column:name"`
	Slug          string `gorm:"column:slug"`
	PlayCount     int    `gorm:"column:play_count"`
	DistinctCount int    `gorm:"column:distinct_count"`
}

const (
	radioEvidenceTitle      = "title"
	radioEvidenceArtistName = "artist_name"
	radioEvidenceArtistKey  = "artist_key"
	radioEvidenceArtist     = "artist"
)

// radioTrackCandidateEvidenceSQL returns the evidence rows of the selected
// tracks, one radioTrackEvidenceRow per row. Parameters: the window's first
// air date, the selected tracks' artist keys, their catalog artist ids, then
// the track ids. A counted play of a selected track is either linked to one
// of those artists or unlinked under one of those keys, so the key and id
// condition only narrows the scan to the plays the indexes on artist_id and
// on the normalized name of an unlinked play can find. The album list counts
// only plays whose album counts as release evidence (RadioRankingAlbum).
var radioTrackCandidateEvidenceSQL = `
WITH plays AS MATERIALIZED (
  SELECT ` + radioTrackKeyColumnsSQL + `,
         k.artist_key,
         p.artist_name, p.track_title, p.album_title, p.artist_id,
         sh.station_id,
         NOT ` + RadioPlayExcludedSQL(RadioRankingAlbum, radioCandidatePlayCols) + ` AS album_counts
  ` + radioCandidatePlaysFromSQL(RadioRankingTrack) + `
    AND ((p.artist_id IS NULL AND radio_normalize_name(p.artist_name) = ANY (?::text[]))
         OR p.artist_id = ANY (?::bigint[]))
),
counted AS MATERIALIZED (
  SELECT pl.*, t.id AS track_id
  FROM plays pl` + radioTrackJoinSQL + `
  WHERE t.id = ANY (?::bigint[])
)
` + radioCandidateEvidenceSpellingsSQL(radioEvidenceTitle, "track_id", "track_title") + `
UNION ALL
SELECT '` + radioEvidenceArtistName + `' AS kind, track_id, 0 AS ref_id, artist_name AS name, '' AS slug,
       play_count::int AS play_count, 0 AS distinct_count, rn
FROM (
  SELECT track_id, artist_name, count(*) AS play_count,
         row_number() OVER (PARTITION BY track_id ORDER BY count(*) DESC, artist_name COLLATE "C") AS rn
  FROM counted
  GROUP BY track_id, artist_name
) artist_names
WHERE rn = 1
UNION ALL
SELECT '` + radioEvidenceArtistKey + `' AS kind, track_id, 0 AS ref_id, artist_key AS name, '' AS slug,
       count(*)::int AS play_count, 0 AS distinct_count,
       row_number() OVER (PARTITION BY track_id ORDER BY count(*) DESC, artist_key COLLATE "C") AS rn
FROM counted
GROUP BY track_id, artist_key
UNION ALL
SELECT '` + radioEvidenceArtist + `' AS kind, c.track_id, a.id AS ref_id, a.name AS name, coalesce(a.slug, '') AS slug,
       count(*)::int AS play_count, 0 AS distinct_count,
       row_number() OVER (PARTITION BY c.track_id ORDER BY count(*) DESC, a.id) AS rn
FROM counted c
JOIN artists a ON a.id = c.artist_id
GROUP BY c.track_id, a.id, a.name, a.slug
UNION ALL` + radioCandidateEvidenceStationsSQL("track_id") + `
UNION ALL` + radioCandidateEvidenceNamesSQL(radioEvidenceAlbum, "track_id", "album_title", "album_counts") + `
ORDER BY kind, track_id, rn`

// attachRadioTrackCandidateEvidence fills the evidence lists of the candidates
// in place. rows are the ranked rows the candidates came from.
func attachRadioTrackCandidateEvidence(tx *gorm.DB, windowStart string, candidates []contracts.RadioTrackCandidate, rows []radioTrackCandidateRow) error {
	if len(candidates) == 0 {
		return nil
	}
	rowByTrack := make(map[uint]radioTrackCandidateRow, len(rows))
	for _, row := range rows {
		rowByTrack[*row.TrackID] = row
	}
	var keys []string
	var artistIDs, trackIDs []int64
	byTrack := make(map[uint]*contracts.RadioTrackCandidate, len(candidates))
	for i := range candidates {
		c := &candidates[i]
		byTrack[c.TrackID] = c
		trackIDs = append(trackIDs, int64(c.TrackID))
		row := rowByTrack[c.TrackID]
		keys = append(keys, row.ArtistKeys...)
		artistIDs = append(artistIDs, row.ArtistIDs...)
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	slices.Sort(artistIDs)
	artistIDs = slices.Compact(artistIDs)

	var evidence []radioTrackEvidenceRow
	if err := tx.Raw(radioTrackCandidateEvidenceSQL, windowStart, pq.Array(keys), pq.Array(artistIDs), pq.Array(trackIDs)).
		Scan(&evidence).Error; err != nil {
		return fmt.Errorf("reading track candidate evidence: %w", err)
	}
	for _, e := range evidence {
		c, ok := byTrack[e.TrackID]
		if !ok {
			continue
		}
		switch e.Kind {
		case radioEvidenceTitle:
			// Rows arrive by rank, so the first is the most common spelling.
			if len(c.TitleSpellings) == 0 {
				c.DisplayTitle = e.Name
			}
			c.TitleSpellings = append(c.TitleSpellings, contracts.RadioCandidateName{Name: e.Name, PlayCount: e.PlayCount})
			c.TitleSpellingCount = e.DistinctCount
		case radioEvidenceArtistName:
			c.DisplayArtist = e.Name
		case radioEvidenceArtistKey:
			c.ArtistKeys = append(c.ArtistKeys, contracts.RadioCandidateName{Name: e.Name, PlayCount: e.PlayCount})
		case radioEvidenceArtist:
			c.Artists = append(c.Artists, contracts.RadioCandidateArtist{
				ArtistID: e.RefID, Name: e.Name, Slug: e.Slug, PlayCount: e.PlayCount,
			})
		case radioEvidenceStation:
			c.Stations = append(c.Stations, contracts.RadioCandidateStation{
				StationID: e.RefID, Slug: e.Slug, Name: e.Name, PlayCount: e.PlayCount,
			})
		case radioEvidenceAlbum:
			c.AlbumNames = append(c.AlbumNames, contracts.RadioCandidateName{Name: e.Name, PlayCount: e.PlayCount})
			c.AlbumNameCount = e.DistinctCount
		}
	}
	for i := range candidates {
		orderByArtistKeys(&candidates[i])
	}
	return nil
}

// orderByArtistKeys puts ArtistCandidateKeys in ArtistKeys order, by play
// count.
func orderByArtistKeys(c *contracts.RadioTrackCandidate) {
	ordered := make([]string, 0, len(c.ArtistCandidateKeys))
	for _, k := range c.ArtistKeys {
		if slices.Contains(c.ArtistCandidateKeys, k.Name) {
			ordered = append(ordered, k.Name)
		}
	}
	c.ArtistCandidateKeys = ordered
}
