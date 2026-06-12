package catalog

// PSY-1081 — Station-scoped radio co-occurrence subgraph.
//
// The radio analog of the scene graph (PSY-367) and the venue bill network
// (PSY-365): nodes are the station's top-N artists by play count, edges are
// within-station co-occurrence (two artists matched in the same episode on
// THIS station), and clusters group artists by the radio show they are most
// played on.
//
// Derivation decision: edges are computed AT QUERY TIME from radio_plays.
// The aggregate pipeline (ComputeAffinity → radio_artist_affinity →
// SyncAffinityToRelationships) collapses station attribution to a
// station_count integer — neither table records WHICH stations contributed a
// pair — so the aggregate radio_cooccurrence edges cannot be filtered to one
// station. Parameterizing the aggregate computation by station would require
// a schema change (station dimension on the affinity PK). Restricting the
// query-time self-join to the top-N matched artists bounds the pair space at
// N(N-1)/2, which keeps the derivation cheap enough to run per request.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

const (
	stationGraphWindow12M = "12m"
	stationGraphWindowAll = "all"

	// stationGraphDefaultNodeLimit is the default top-N artist cap. The
	// handler exposes `limit` (1..stationGraphMaxNodeLimit); the service
	// clamps defensively for non-HTTP callers.
	stationGraphDefaultNodeLimit = 75
	stationGraphMaxNodeLimit     = 150

	// stationGraphMinCoOccurrence is the minimum number of shared episodes
	// for an edge to surface. Mirrors ComputeAffinity's HAVING >= 2 and the
	// venue bill network's min-shared-shows threshold.
	stationGraphMinCoOccurrence = 2
)

// stationGraphArtistRow is one top-N artist with display fields, station play
// count, and the show (on this station) the artist is most played on — the
// cluster signal. Primary-show fields are nullable defensively (LEFT JOIN).
type stationGraphArtistRow struct {
	ArtistID        uint    `gorm:"column:artist_id"`
	Name            string  `gorm:"column:name"`
	Slug            *string `gorm:"column:slug"`
	City            *string `gorm:"column:city"`
	State           *string `gorm:"column:state"`
	PlayCount       int     `gorm:"column:play_count"`
	PrimaryShowID   *uint   `gorm:"column:primary_show_id"`
	PrimaryShowName *string `gorm:"column:primary_show_name"`
}

// stationGraphEdgeRow is one co-occurring artist pair within the station.
// artist_a_id < artist_b_id (canonical ordering from the self-join).
type stationGraphEdgeRow struct {
	ArtistAID         uint   `gorm:"column:artist_a_id"`
	ArtistBID         uint   `gorm:"column:artist_b_id"`
	CoOccurrenceCount int    `gorm:"column:co_occurrence_count"`
	LastCoOccurrence  string `gorm:"column:last_co_occurrence"`
}

// GetStationGraph returns the station-scoped co-occurrence graph for
// GET /radio-stations/{slug}/graph. See the file-level comment for the
// derivation decision.
func (s *RadioService) GetStationGraph(stationID uint, window string, limit int) (*contracts.RadioStationGraphResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Load the station (slug + name feed the info block; missing → 404,
	// consistent with the other station-scoped reads).
	var station catalogm.RadioStation
	if err := s.db.Select("id, slug, name").First(&station, stationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		}
		return nil, fmt.Errorf("failed to get radio station: %w", err)
	}

	resolvedWindow := normalizeStationGraphWindow(window)
	cutoff := stationGraphWindowCutoff(resolvedWindow)

	if limit <= 0 {
		limit = stationGraphDefaultNodeLimit
	}
	if limit > stationGraphMaxNodeLimit {
		limit = stationGraphMaxNodeLimit
	}

	resp := &contracts.RadioStationGraphResponse{
		Station: contracts.RadioStationGraphInfo{
			ID:   station.ID,
			Slug: station.Slug,
			Name: station.Name,
		},
		Clusters: []contracts.RadioStationGraphCluster{},
		Nodes:    []contracts.RadioStationGraphNode{},
		Links:    []contracts.RadioStationGraphLink{},
	}
	switch resolvedWindow {
	case stationGraphWindowAll:
		resp.Station.Window = "all_time"
	default:
		resp.Station.Window = "last_12m"
	}

	// 1. Top-N matched artists by play count + each artist's primary show.
	rows, err := s.queryStationGraphArtists(stationID, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query station graph artists: %w", err)
	}
	resp.Station.ArtistCount = len(rows)
	if len(rows) == 0 {
		return resp, nil
	}

	artistIDs := make([]uint, 0, len(rows))
	for _, r := range rows {
		artistIDs = append(artistIDs, r.ArtistID)
	}

	// 2. Cluster pass — group by primary show, mirroring the scene graph's
	//    primary-venue clustering (same size floor, first-class cap, and
	//    "other" rollup).
	clusters, clusterIDByShow := buildStationGraphClusters(rows)
	resp.Clusters = clusters
	clusterByArtist := make(map[uint]string, len(rows))
	for _, r := range rows {
		clusterID := sceneClusterOtherID
		if r.PrimaryShowID != nil {
			if cid, ok := clusterIDByShow[*r.PrimaryShowID]; ok {
				clusterID = cid
			}
		}
		clusterByArtist[r.ArtistID] = clusterID
	}

	// 3. Within-station co-occurrence edges between the top-N artists.
	edges, err := s.queryStationGraphEdges(stationID, artistIDs, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to query station graph edges: %w", err)
	}

	connected := make(map[uint]bool, len(rows))
	for _, e := range edges {
		// Score normalization mirrors SyncAffinityToRelationships
		// (min(1, count/50)) so the radio_cooccurrence edge grammar stays
		// consistent; the cross-station multiplier doesn't apply because the
		// scope is a single station by construction.
		score := float64(e.CoOccurrenceCount) / 50.0
		if score > 1.0 {
			score = 1.0
		}
		detail := map[string]any{
			"co_occurrence_count": e.CoOccurrenceCount,
		}
		if e.LastCoOccurrence != "" {
			detail["last_co_occurrence"] = e.LastCoOccurrence
		}
		resp.Links = append(resp.Links, contracts.RadioStationGraphLink{
			SourceID:       e.ArtistAID,
			TargetID:       e.ArtistBID,
			Type:           catalogm.RelationshipTypeRadioCooccurrence,
			Score:          score,
			Detail:         detail,
			IsCrossCluster: clusterByArtist[e.ArtistAID] != clusterByArtist[e.ArtistBID],
		})
		connected[e.ArtistAID] = true
		connected[e.ArtistBID] = true
	}
	resp.Station.EdgeCount = len(resp.Links)

	// 4. Node list. Upcoming-show counts use the shared scene/venue helper so
	//    the green-dot indicator stays consistent across all three graphs.
	upcomingByArtist := batchArtistUpcomingShowCounts(s.db, artistIDs)
	for _, r := range rows {
		resp.Nodes = append(resp.Nodes, contracts.RadioStationGraphNode{
			ID:                r.ArtistID,
			Name:              r.Name,
			Slug:              derefString(r.Slug),
			City:              derefString(r.City),
			State:             derefString(r.State),
			UpcomingShowCount: upcomingByArtist[r.ArtistID],
			ClusterID:         clusterByArtist[r.ArtistID],
			IsIsolate:         !connected[r.ArtistID],
			PlayCount:         r.PlayCount,
		})
	}

	return resp, nil
}

// normalizeStationGraphWindow coerces the caller's `window` string to a known
// value. Empty/unknown input falls back to "12m" — the endpoint's default
// (unlike the venue bill network, which defaults to all-time) — so a
// malformed query param degrades gracefully rather than 500ing.
func normalizeStationGraphWindow(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case stationGraphWindowAll:
		return stationGraphWindowAll
	default:
		return stationGraphWindow12M
	}
}

// stationGraphWindowCutoff returns the inclusive air-date lower bound as a
// YYYY-MM-DD string (radio_episodes.air_date is a DATE column), or "" for the
// all-time case so the query helpers can skip the predicate.
func stationGraphWindowCutoff(window string) string {
	if window == stationGraphWindowAll {
		return ""
	}
	return time.Now().UTC().AddDate(-1, 0, 0).Format("2006-01-02")
}

// queryStationGraphArtists runs the top-N + primary-show CTE. station_plays
// narrows once to this station's matched plays (and the window); top_artists
// ranks by play count; artist_show_counts resolves each top artist's
// most-played show on the station with ROW_NUMBER (tie-break: more recent
// air date, then show id — same shape as the scene graph's primary-venue
// CTE). Final ORDER BY name for determinism.
func (s *RadioService) queryStationGraphArtists(stationID uint, cutoff string, limit int) ([]stationGraphArtistRow, error) {
	windowFilter := ""
	args := []any{stationID}
	if cutoff != "" {
		windowFilter = "AND re.air_date >= ?"
		args = append(args, cutoff)
	}
	args = append(args, limit)

	q := fmt.Sprintf(`
		WITH station_plays AS (
			SELECT rp.artist_id, re.show_id, re.air_date
			FROM radio_plays rp
			JOIN radio_episodes re ON re.id = rp.episode_id
			JOIN radio_shows rsh ON rsh.id = re.show_id
			WHERE rsh.station_id = ? AND rp.artist_id IS NOT NULL %s
		),
		top_artists AS (
			SELECT artist_id, COUNT(*) AS play_count
			FROM station_plays
			GROUP BY artist_id
			ORDER BY play_count DESC, artist_id ASC
			LIMIT ?
		),
		artist_show_counts AS (
			SELECT
				sp.artist_id,
				sp.show_id,
				ROW_NUMBER() OVER (
					PARTITION BY sp.artist_id
					ORDER BY COUNT(*) DESC, MAX(sp.air_date) DESC, sp.show_id ASC
				) AS rn
			FROM station_plays sp
			JOIN top_artists ta ON ta.artist_id = sp.artist_id
			GROUP BY sp.artist_id, sp.show_id
		)
		SELECT
			a.id    AS artist_id,
			a.name  AS name,
			a.slug  AS slug,
			a.city  AS city,
			a.state AS state,
			ta.play_count  AS play_count,
			ascnt.show_id  AS primary_show_id,
			rsh.name       AS primary_show_name
		FROM top_artists ta
		JOIN artists a ON a.id = ta.artist_id
		LEFT JOIN artist_show_counts ascnt ON ascnt.artist_id = ta.artist_id AND ascnt.rn = 1
		LEFT JOIN radio_shows rsh ON rsh.id = ascnt.show_id
		ORDER BY a.name ASC
	`, windowFilter)

	var rows []stationGraphArtistRow
	if err := s.db.Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// queryStationGraphEdges self-joins radio_plays within each of this station's
// episodes, restricted to the top-N artist set. The weight is
// COUNT(DISTINCT episode) — the number of episodes on this station where both
// artists appeared — per the PSY-1081 spec ("two artists in the same episode
// on THIS station"). Note this intentionally differs from ComputeAffinity's
// COUNT(*) of play-pairs, which double-counts an episode when an artist is
// played twice in it. last_co_occurrence is cast to text in SQL so the scan
// target is a plain string.
func (s *RadioService) queryStationGraphEdges(stationID uint, artistIDs []uint, cutoff string) ([]stationGraphEdgeRow, error) {
	if len(artistIDs) < 2 {
		return nil, nil
	}
	windowFilter := ""
	args := []any{stationID, artistIDs, artistIDs}
	if cutoff != "" {
		windowFilter = "AND re.air_date >= ?"
		args = append(args, cutoff)
	}
	args = append(args, stationGraphMinCoOccurrence)

	q := fmt.Sprintf(`
		SELECT
			rp1.artist_id AS artist_a_id,
			rp2.artist_id AS artist_b_id,
			COUNT(DISTINCT rp1.episode_id) AS co_occurrence_count,
			TO_CHAR(MAX(re.air_date), 'YYYY-MM-DD') AS last_co_occurrence
		FROM radio_plays rp1
		JOIN radio_plays rp2 ON rp2.episode_id = rp1.episode_id
			AND rp1.artist_id < rp2.artist_id
		JOIN radio_episodes re ON re.id = rp1.episode_id
		JOIN radio_shows rsh ON rsh.id = re.show_id
		WHERE rsh.station_id = ?
			AND rp1.artist_id IN ?
			AND rp2.artist_id IN ?
			%s
		GROUP BY rp1.artist_id, rp2.artist_id
		HAVING COUNT(DISTINCT rp1.episode_id) >= ?
	`, windowFilter)

	var rows []stationGraphEdgeRow
	if err := s.db.Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// buildStationGraphClusters converts the per-artist primary-show rows into a
// sorted cluster list, mirroring buildSceneClusters: shows with at least
// sceneClusterMinSize artists are first-class (capped at the Okabe-Ito
// palette size); the remainder roll up to "other". Returns the cluster list
// and a show_id → cluster_id lookup.
func buildStationGraphClusters(rows []stationGraphArtistRow) ([]contracts.RadioStationGraphCluster, map[uint]string) {
	type showCount struct {
		showID uint
		name   string
		count  int
	}
	byShow := make(map[uint]*showCount)
	for _, r := range rows {
		if r.PrimaryShowID == nil {
			continue
		}
		entry, ok := byShow[*r.PrimaryShowID]
		if !ok {
			entry = &showCount{showID: *r.PrimaryShowID, name: derefString(r.PrimaryShowName)}
			byShow[*r.PrimaryShowID] = entry
		}
		entry.count++
	}

	shows := make([]*showCount, 0, len(byShow))
	for _, v := range byShow {
		shows = append(shows, v)
	}
	// Sort by count desc, then name asc for deterministic ordering on ties —
	// same insertion-sort shape as buildSceneClusters.
	for i := 1; i < len(shows); i++ {
		for j := i; j > 0; j-- {
			a, b := shows[j], shows[j-1]
			better := a.count > b.count || (a.count == b.count && a.name < b.name)
			if !better {
				break
			}
			shows[j], shows[j-1] = b, a
		}
	}

	clusterIDByShow := make(map[uint]string, len(shows))
	clusters := make([]contracts.RadioStationGraphCluster, 0, len(shows)+1)
	otherSize := 0

	for i, v := range shows {
		if v.count >= sceneClusterMinSize && len(clusters) < sceneClusterMaxFirstClass {
			cid := fmt.Sprintf("rs_%d", v.showID)
			clusterIDByShow[v.showID] = cid
			clusters = append(clusters, contracts.RadioStationGraphCluster{
				ID:         cid,
				Label:      v.name,
				Size:       v.count,
				ColorIndex: i,
			})
			continue
		}
		clusterIDByShow[v.showID] = sceneClusterOtherID
		otherSize += v.count
	}

	// Artists with no primary show (defensive — LEFT JOIN miss) land in "other".
	for _, r := range rows {
		if r.PrimaryShowID == nil {
			otherSize++
		}
	}

	if otherSize > 0 {
		clusters = append(clusters, contracts.RadioStationGraphCluster{
			ID:         sceneClusterOtherID,
			Label:      sceneClusterOtherLabel,
			Size:       otherSize,
			ColorIndex: -1,
		})
	}

	return clusters, clusterIDByShow
}
