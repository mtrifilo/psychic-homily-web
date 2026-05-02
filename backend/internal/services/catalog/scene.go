package catalog

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// SceneService handles computed city-level aggregations for "scene" pages.
// No new tables — all data is derived from existing venue, show, and artist tables.
type SceneService struct {
	db *gorm.DB
}

// NewSceneService creates a new scene service.
func NewSceneService(database *gorm.DB) *SceneService {
	if database == nil {
		database = db.GetDB()
	}
	return &SceneService{db: database}
}

// Thresholds for a city to qualify as a "scene".
const (
	sceneMinVenues = 2
	sceneMinShows  = 3
)

// ListScenes returns cities that meet scene thresholds:
// 2+ verified venues AND 3+ approved shows (past or upcoming).
func (s *SceneService) ListScenes() ([]*contracts.SceneListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type cityRow struct {
		City       string `gorm:"column:city"`
		State      string `gorm:"column:state"`
		VenueCount int    `gorm:"column:venue_count"`
		ShowCount  int    `gorm:"column:show_count"`
	}

	// Step 1: Find cities with 1+ verified venues AND 1+ total approved shows.
	// Uses a single query that joins venues → shows to compute both counts.
	var cities []cityRow
	err := s.db.Raw(`
		SELECT v.city, v.state,
		       COUNT(DISTINCT v.id) AS venue_count,
		       COUNT(DISTINCT s.id) AS show_count
		FROM venues v
		LEFT JOIN show_venues sv ON sv.venue_id = v.id
		LEFT JOIN shows s ON s.id = sv.show_id AND s.status = ?
		WHERE v.verified = true
		  AND v.city IS NOT NULL AND v.city != ''
		  AND v.state IS NOT NULL AND v.state != ''
		GROUP BY v.city, v.state
		HAVING COUNT(DISTINCT v.id) >= ?
		   AND COUNT(DISTINCT s.id) >= ?
	`, catalogm.ShowStatusApproved, sceneMinVenues, sceneMinShows).Scan(&cities).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list scenes: %w", err)
	}

	// Step 2: For each qualifying city, count upcoming approved shows (for display).
	var results []*contracts.SceneListResponse
	for i := range cities {
		c := &cities[i]
		var upcomingCount int64
		err := s.db.Raw(`
			SELECT COUNT(DISTINCT s.id)
			FROM shows s
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE v.city = ? AND v.state = ?
			  AND s.status = ?
			  AND s.event_date >= ?
		`, c.City, c.State, catalogm.ShowStatusApproved, now).Scan(&upcomingCount).Error
		if err != nil {
			return nil, fmt.Errorf("failed to count shows for %s, %s: %w", c.City, c.State, err)
		}

		results = append(results, &contracts.SceneListResponse{
			City:              c.City,
			State:             c.State,
			Slug:              buildSceneSlug(c.City, c.State),
			VenueCount:        c.VenueCount,
			UpcomingShowCount: int(upcomingCount),
			TotalShowCount:    c.ShowCount,
		})
	}

	// Sort by total show count descending, then upcoming shows as tiebreaker.
	// Simple insertion sort is fine for a small number of cities.
	for i := 1; i < len(results); i++ {
		for j := i; j > 0; j-- {
			if results[j].TotalShowCount > results[j-1].TotalShowCount ||
				(results[j].TotalShowCount == results[j-1].TotalShowCount &&
					results[j].UpcomingShowCount > results[j-1].UpcomingShowCount) {
				results[j], results[j-1] = results[j-1], results[j]
			} else {
				break
			}
		}
	}

	return results, nil
}

// GetSceneDetail returns computed aggregation stats and pulse for a city.
func (s *SceneService) GetSceneDetail(city, state string) (*contracts.SceneDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	// Venue count (verified only)
	var venueCount int64
	if err := s.db.Model(&catalogm.Venue{}).
		Where("city = ? AND state = ? AND verified = true", city, state).
		Count(&venueCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	}
	if venueCount < sceneMinVenues {
		return nil, fmt.Errorf("scene not found: %s, %s", city, state)
	}

	// Upcoming show count
	var upcomingShowCount int64
	if err := s.db.Raw(`
		SELECT COUNT(DISTINCT s.id)
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ?
	`, city, state, catalogm.ShowStatusApproved, now).Scan(&upcomingShowCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count upcoming shows: %w", err)
	}

	// Artist count: distinct artists with approved shows at venues in this city
	var artistCount int64
	if err := s.db.Raw(`
		SELECT COUNT(DISTINCT sa.artist_id)
		FROM show_artists sa
		JOIN shows s ON s.id = sa.show_id
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
	`, city, state, catalogm.ShowStatusApproved).Scan(&artistCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count artists: %w", err)
	}

	// Festival count: festivals with matching city
	var festivalCount int64
	if err := s.db.Model(&catalogm.Festival{}).
		Where("city = ? AND state = ?", city, state).
		Count(&festivalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count festivals: %w", err)
	}

	// ── Pulse computations ──

	// Current month boundaries
	thisMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonthStart := thisMonthStart.AddDate(0, 1, 0)
	prevMonthStart := thisMonthStart.AddDate(0, -1, 0)

	// Shows this month
	var showsThisMonth int64
	s.db.Raw(`
		SELECT COUNT(DISTINCT s.id)
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ? AND s.event_date < ?
	`, city, state, catalogm.ShowStatusApproved, thisMonthStart, nextMonthStart).Scan(&showsThisMonth)

	// Shows previous month
	var showsPrevMonth int64
	s.db.Raw(`
		SELECT COUNT(DISTINCT s.id)
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ? AND s.event_date < ?
	`, city, state, catalogm.ShowStatusApproved, prevMonthStart, thisMonthStart).Scan(&showsPrevMonth)

	// Trend string
	diff := int(showsThisMonth) - int(showsPrevMonth)
	showsTrend := "0"
	if diff > 0 {
		showsTrend = fmt.Sprintf("+%d", diff)
	} else if diff < 0 {
		showsTrend = fmt.Sprintf("%d", diff)
	}

	// New artists in last 30 days: artists whose first show in this city was in last 30 days
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	var newArtists30d int64
	s.db.Raw(`
		SELECT COUNT(*)
		FROM (
			SELECT sa.artist_id, MIN(s.event_date) AS first_show
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE v.city = ? AND v.state = ?
			  AND s.status = ?
			GROUP BY sa.artist_id
			HAVING MIN(s.event_date) >= ?
		) AS new_artists
	`, city, state, catalogm.ShowStatusApproved, thirtyDaysAgo).Scan(&newArtists30d)

	// Active venues this month: venues with at least 1 approved show this month
	var activeVenuesThisMonth int64
	s.db.Raw(`
		SELECT COUNT(DISTINCT v.id)
		FROM venues v
		JOIN show_venues sv ON sv.venue_id = v.id
		JOIN shows s ON s.id = sv.show_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ? AND s.event_date < ?
	`, city, state, catalogm.ShowStatusApproved, thisMonthStart, nextMonthStart).Scan(&activeVenuesThisMonth)

	// Shows by month: last 6 months (from 5 months ago through current month)
	showsByMonth := make([]int, 6)
	for i := 5; i >= 0; i-- {
		monthStart := thisMonthStart.AddDate(0, -i, 0)
		monthEnd := monthStart.AddDate(0, 1, 0)
		var count int64
		s.db.Raw(`
			SELECT COUNT(DISTINCT s.id)
			FROM shows s
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE v.city = ? AND v.state = ?
			  AND s.status = ?
			  AND s.event_date >= ? AND s.event_date < ?
		`, city, state, catalogm.ShowStatusApproved, monthStart, monthEnd).Scan(&count)
		showsByMonth[5-i] = int(count)
	}

	return &contracts.SceneDetailResponse{
		City:        city,
		State:       state,
		Slug:        buildSceneSlug(city, state),
		Description: nil, // nil until scenes table exists
		Stats: contracts.SceneStats{
			VenueCount:        int(venueCount),
			ArtistCount:       int(artistCount),
			UpcomingShowCount: int(upcomingShowCount),
			FestivalCount:     int(festivalCount),
		},
		Pulse: contracts.ScenePulse{
			ShowsThisMonth:        int(showsThisMonth),
			ShowsPrevMonth:        int(showsPrevMonth),
			ShowsTrend:            showsTrend,
			NewArtists30d:         int(newArtists30d),
			ActiveVenuesThisMonth: int(activeVenuesThisMonth),
			ShowsByMonth:          showsByMonth,
		},
	}, nil
}

// GetActiveArtists returns artists ranked by show count in a city within the given period.
func (s *SceneService) GetActiveArtists(city, state string, periodDays, limit, offset int) ([]*contracts.SceneArtistResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Verify city qualifies as scene
	var venueCount int64
	if err := s.db.Model(&catalogm.Venue{}).
		Where("city = ? AND state = ? AND verified = true", city, state).
		Count(&venueCount).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count venues: %w", err)
	}
	if venueCount < sceneMinVenues {
		return nil, 0, fmt.Errorf("scene not found: %s, %s", city, state)
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -periodDays)

	// Count total distinct artists
	var total int64
	if err := s.db.Raw(`
		SELECT COUNT(DISTINCT sa.artist_id)
		FROM show_artists sa
		JOIN shows s ON s.id = sa.show_id
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ?
	`, city, state, catalogm.ShowStatusApproved, cutoff).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count active artists: %w", err)
	}

	type artistRow struct {
		ID        uint    `gorm:"column:id"`
		Slug      *string `gorm:"column:slug"`
		Name      string  `gorm:"column:name"`
		City      *string `gorm:"column:city"`
		State     *string `gorm:"column:state"`
		ShowCount int     `gorm:"column:show_count"`
	}

	var rows []artistRow
	if err := s.db.Raw(`
		SELECT a.id, a.slug, a.name, a.city, a.state, COUNT(DISTINCT s.id) AS show_count
		FROM artists a
		JOIN show_artists sa ON sa.artist_id = a.id
		JOIN shows s ON s.id = sa.show_id
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ?
		GROUP BY a.id
		ORDER BY show_count DESC, a.name ASC
		LIMIT ? OFFSET ?
	`, city, state, catalogm.ShowStatusApproved, cutoff, limit, offset).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get active artists: %w", err)
	}

	results := make([]*contracts.SceneArtistResponse, len(rows))
	for i, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		results[i] = &contracts.SceneArtistResponse{
			ID:        r.ID,
			Slug:      slug,
			Name:      r.Name,
			City:      r.City,
			State:     r.State,
			ShowCount: r.ShowCount,
		}
	}

	return results, total, nil
}

// ParseSceneSlug resolves a slug like "phoenix-az" to actual city and state
// by matching against verified venues in the database.
func (s *SceneService) ParseSceneSlug(slug string) (string, string, error) {
	if s.db == nil {
		return "", "", fmt.Errorf("database not initialized")
	}

	type cityState struct {
		City  string
		State string
	}

	var result cityState
	err := s.db.Raw(`
		SELECT DISTINCT city, state
		FROM venues
		WHERE verified = true
		  AND LOWER(REPLACE(city, ' ', '-')) || '-' || LOWER(state) = ?
		LIMIT 1
	`, strings.ToLower(slug)).Scan(&result).Error
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve scene slug: %w", err)
	}
	if result.City == "" {
		return "", "", fmt.Errorf("scene not found for slug: %s", slug)
	}

	return result.City, result.State, nil
}

// buildSceneSlug generates a URL-safe slug from city and state.
// Example: "Phoenix", "AZ" → "phoenix-az"
func buildSceneSlug(city, state string) string {
	slug := strings.ToLower(city)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = slug + "-" + strings.ToLower(state)
	return slug
}

// Thresholds for genre intelligence.
const (
	sceneGenreMinTaggedArtists     = 30
	sceneDiversityMinTaggedArtists = 50
	sceneDiversityMinGenres        = 5
	venueGenreMinShows             = 10
)

// GetSceneGenreDistribution returns genre tags ranked by the number of distinct
// artists who play approved shows in this city and carry that genre tag.
// Returns empty if fewer than 30 tagged artists exist for the scene.
func (s *SceneService) GetSceneGenreDistribution(city, state string) ([]contracts.GenreCount, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	type genreRow struct {
		TagID uint   `gorm:"column:tag_id"`
		Name  string `gorm:"column:name"`
		Slug  string `gorm:"column:slug"`
		Count int    `gorm:"column:count"`
	}

	var rows []genreRow
	err := s.db.Raw(`
		SELECT t.id AS tag_id, t.name, t.slug, COUNT(DISTINCT sa.artist_id) AS count
		FROM show_artists sa
		JOIN show_venues sv ON sv.show_id = sa.show_id
		JOIN shows s ON s.id = sa.show_id
		JOIN venues v ON v.id = sv.venue_id
		JOIN entity_tags et ON et.entity_type = 'artist' AND et.entity_id = sa.artist_id
		JOIN tags t ON t.id = et.tag_id AND t.category = 'genre'
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		GROUP BY t.id, t.name, t.slug
		ORDER BY count DESC
	`, city, state, catalogm.ShowStatusApproved).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get scene genre distribution: %w", err)
	}

	// Check if total tagged artists meets threshold
	totalTagged := 0
	for _, r := range rows {
		totalTagged += r.Count
	}
	if totalTagged < sceneGenreMinTaggedArtists {
		return []contracts.GenreCount{}, nil
	}

	result := make([]contracts.GenreCount, len(rows))
	for i, r := range rows {
		result[i] = contracts.GenreCount{
			TagID: r.TagID,
			Name:  r.Name,
			Slug:  r.Slug,
			Count: r.Count,
		}
	}

	return result, nil
}

// GetGenreDiversityIndex computes the normalized Shannon entropy of the genre
// distribution for a city scene. Returns a value in [0, 1] where higher values
// indicate more genre diversity. Returns -1 when there is insufficient data
// (fewer than 50 tagged artists or fewer than 5 genres).
func (s *SceneService) GetGenreDiversityIndex(city, state string) (float64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	type genreRow struct {
		Count int `gorm:"column:count"`
	}

	var rows []genreRow
	err := s.db.Raw(`
		SELECT COUNT(DISTINCT sa.artist_id) AS count
		FROM show_artists sa
		JOIN show_venues sv ON sv.show_id = sa.show_id
		JOIN shows s ON s.id = sa.show_id
		JOIN venues v ON v.id = sv.venue_id
		JOIN entity_tags et ON et.entity_type = 'artist' AND et.entity_id = sa.artist_id
		JOIN tags t ON t.id = et.tag_id AND t.category = 'genre'
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		GROUP BY t.id
		ORDER BY count DESC
	`, city, state, catalogm.ShowStatusApproved).Scan(&rows).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get genre diversity index: %w", err)
	}

	// Check thresholds
	totalTagged := 0
	counts := make([]int, len(rows))
	for i, r := range rows {
		totalTagged += r.Count
		counts[i] = r.Count
	}

	if totalTagged < sceneDiversityMinTaggedArtists || len(counts) < sceneDiversityMinGenres {
		return -1, nil
	}

	return NormalizedShannonEntropy(counts), nil
}

// NormalizedShannonEntropy computes normalized Shannon entropy in [0, 1].
// Exported for testing.
func NormalizedShannonEntropy(counts []int) float64 {
	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return 0
	}
	entropy := 0.0
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / float64(total)
		entropy -= p * math.Log2(p)
	}
	maxEntropy := math.Log2(float64(len(counts)))
	if maxEntropy == 0 {
		return 0
	}
	return entropy / maxEntropy
}

// DiversityLabel returns a human-readable label for a diversity index value.
func DiversityLabel(index float64) string {
	if index < 0 {
		return ""
	}
	if index >= 0.8 {
		return "Highly diverse"
	}
	if index >= 0.5 {
		return "Mixed"
	}
	if index >= 0.2 {
		return "Genre-focused"
	}
	return ""
}

// ──────────────────────────────────────────────
// Scene graph (PSY-367)
// ──────────────────────────────────────────────

// Cluster sizing constants for the scene graph. v1 cluster signal is each artist's
// most-frequently-played venue within the scene; clusters smaller than the
// threshold roll up to a single "other" bucket, and the visible palette caps at
// the Okabe-Ito 8-color set (see docs/features/scene-graph-layout.md §5).
const (
	sceneClusterMinSize       = 6 // first-class cluster floor (else rolled to "other")
	sceneClusterMaxFirstClass = 8 // cap = Okabe-Ito palette size
	sceneClusterOtherID       = "other"
	sceneClusterOtherLabel    = "Other"
)

// allowedSceneEdgeTypes whitelists relationship types that the scene graph
// surfaces. shared_bills + shared_label + member_of carry signal at scene scale;
// `similar` is an editorial vote that doesn't compose well across many artists,
// and `radio_cooccurrence` is station-level rather than scene-level. Constrain
// here so the API surface stays predictable as new types are added.
var allowedSceneEdgeTypes = map[string]bool{
	"shared_bills": true,
	"shared_label": true,
	"member_of":    true,
	"side_project": true,
}

// sceneArtistRow is the unified result of the scene-artists + primary-venue CTE.
type sceneArtistRow struct {
	ArtistID         uint    `gorm:"column:artist_id"`
	Name             string  `gorm:"column:name"`
	Slug             *string `gorm:"column:slug"`
	City             *string `gorm:"column:city"`
	State            *string `gorm:"column:state"`
	PrimaryVenueID   *uint   `gorm:"column:primary_venue_id"`
	PrimaryVenueName *string `gorm:"column:primary_venue_name"`
}

// sceneRelationshipRow is the in-scope relationship payload from artist_relationships.
type sceneRelationshipRow struct {
	SourceArtistID   uint            `gorm:"column:source_artist_id"`
	TargetArtistID   uint            `gorm:"column:target_artist_id"`
	RelationshipType string          `gorm:"column:relationship_type"`
	Score            float32         `gorm:"column:score"`
	Detail           json.RawMessage `gorm:"column:detail"`
}

// GetSceneGraph returns the typed-edge artist relationship graph scoped to a
// single scene (city + state). Cluster IDs are computed at query time from each
// artist's most-frequent venue within the scene; the result is read-only (no
// vote data) and includes derived fields (`is_isolate`, `is_cross_cluster`)
// that the frontend would otherwise have to recompute on every render.
//
// types filters to the subset of allowed scene edge types (see
// allowedSceneEdgeTypes); empty/nil means "all allowed types".
func (s *SceneService) GetSceneGraph(city, state string, types []string) (*contracts.SceneGraphResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Validate scene exists (mirrors GetActiveArtists / GetSceneDetail).
	var venueCount int64
	if err := s.db.Model(&catalogm.Venue{}).
		Where("city = ? AND state = ? AND verified = true", city, state).
		Count(&venueCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	}
	if venueCount < sceneMinVenues {
		return nil, fmt.Errorf("scene not found: %s, %s", city, state)
	}

	// Whitelist + dedupe types. Empty input means "all allowed types"; an input
	// that was non-empty but resolved to nothing (every value unknown to the
	// allowlist) must short-circuit to zero edges, not silently fall back to
	// "all types".
	resolvedTypes := resolveSceneEdgeTypes(types)
	noEdgesByFilter := len(types) > 0 && len(resolvedTypes) == 0

	// 1. Single CTE query — scene artists + their most-frequent in-scene venue.
	//    Tie-break: more recent show wins (matches the spike doc §4 spec).
	rows, err := s.querySceneArtistsWithPrimaryVenue(city, state)
	if err != nil {
		return nil, fmt.Errorf("failed to query scene artists: %w", err)
	}

	resp := &contracts.SceneGraphResponse{
		Scene: contracts.SceneGraphInfo{
			Slug:        buildSceneSlug(city, state),
			City:        city,
			State:       state,
			ArtistCount: len(rows),
		},
		Clusters: []contracts.SceneGraphCluster{},
		Nodes:    []contracts.SceneGraphNode{},
		Links:    []contracts.SceneGraphLink{},
	}

	if len(rows) == 0 {
		return resp, nil
	}

	// 2. Cluster pass — count artists per primary venue, identify first-class clusters
	//    (size >= sceneClusterMinSize, capped at Okabe-Ito palette size), roll the
	//    long tail into a single "other" cluster.
	clusters, clusterIDByVenue := buildSceneClusters(rows)
	resp.Clusters = clusters

	// 3. Build node list with cluster assignment. Order by artist name for
	//    determinism (frontend doesn't depend on order, but tests do).
	artistIDs := make([]uint, 0, len(rows))
	clusterByArtist := make(map[uint]string, len(rows))
	for _, r := range rows {
		artistIDs = append(artistIDs, r.ArtistID)
		clusterID := sceneClusterOtherID
		if r.PrimaryVenueID != nil {
			if cid, ok := clusterIDByVenue[*r.PrimaryVenueID]; ok {
				clusterID = cid
			}
		}
		clusterByArtist[r.ArtistID] = clusterID
	}

	// 4. Batch query upcoming-show-count for every node (mirror GetArtistGraph §4).
	upcomingByArtist := s.batchUpcomingShowCount(artistIDs)

	// 5. Query in-scope relationships — both endpoints in the scene's artist set.
	var links []sceneRelationshipRow
	if !noEdgesByFilter {
		fetched, err := s.querySceneRelationships(artistIDs, resolvedTypes)
		if err != nil {
			return nil, fmt.Errorf("failed to query scene relationships: %w", err)
		}
		links = fetched
	}

	// 6. Build the link payload + flag cross-cluster ties.
	connected := make(map[uint]bool, len(rows))
	for _, l := range links {
		srcCluster := clusterByArtist[l.SourceArtistID]
		tgtCluster := clusterByArtist[l.TargetArtistID]

		var detail any
		if len(l.Detail) > 0 {
			_ = json.Unmarshal(l.Detail, &detail)
		}

		resp.Links = append(resp.Links, contracts.SceneGraphLink{
			SourceID:       l.SourceArtistID,
			TargetID:       l.TargetArtistID,
			Type:           l.RelationshipType,
			Score:          float64(l.Score),
			Detail:         detail,
			IsCrossCluster: srcCluster != tgtCluster,
		})

		connected[l.SourceArtistID] = true
		connected[l.TargetArtistID] = true
	}
	resp.Scene.EdgeCount = len(resp.Links)

	// 7. Build node list with is_isolate set from the post-filter link set.
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		ncity := ""
		if r.City != nil {
			ncity = *r.City
		}
		nstate := ""
		if r.State != nil {
			nstate = *r.State
		}
		resp.Nodes = append(resp.Nodes, contracts.SceneGraphNode{
			ID:                r.ArtistID,
			Name:              r.Name,
			Slug:              slug,
			City:              ncity,
			State:             nstate,
			UpcomingShowCount: upcomingByArtist[r.ArtistID],
			ClusterID:         clusterByArtist[r.ArtistID],
			IsIsolate:         !connected[r.ArtistID],
		})
	}

	return resp, nil
}

// resolveSceneEdgeTypes filters the caller's requested types against the
// scene-graph allowlist and returns a deterministic slice. Empty input → all
// allowed types.
func resolveSceneEdgeTypes(requested []string) []string {
	if len(requested) == 0 {
		out := make([]string, 0, len(allowedSceneEdgeTypes))
		for t := range allowedSceneEdgeTypes {
			out = append(out, t)
		}
		// Deterministic order so query plans + tests don't churn.
		sortStringsAsc(out)
		return out
	}
	seen := make(map[string]bool, len(requested))
	out := make([]string, 0, len(requested))
	for _, t := range requested {
		if !allowedSceneEdgeTypes[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sortStringsAsc(out)
	return out
}

// sortStringsAsc is a tiny insertion sort on strings — same shape as the
// insertion sort patterns in artist_relationship_service.go. Kept inline to
// avoid pulling in `sort` for one call site.
func sortStringsAsc(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// querySceneArtistsWithPrimaryVenue runs the CTE that lists every artist with
// at least one approved show in (city, state) and resolves each artist's
// most-frequently-played in-scene venue. LEFT JOINs the venue back so the
// PrimaryVenueID/Name columns are nullable for any artist whose venue plays
// don't resolve (defensive — should not happen given the source set).
func (s *SceneService) querySceneArtistsWithPrimaryVenue(city, state string) ([]sceneArtistRow, error) {
	const q = `
		WITH scene_artists AS (
			SELECT DISTINCT sa.artist_id
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE v.city = ? AND v.state = ? AND s.status = ?
		),
		artist_venue_counts AS (
			SELECT
				sa.artist_id,
				v.id AS venue_id,
				v.name AS venue_name,
				COUNT(DISTINCT s.id) AS plays,
				MAX(s.event_date) AS last_played,
				ROW_NUMBER() OVER (
					PARTITION BY sa.artist_id
					ORDER BY COUNT(DISTINCT s.id) DESC, MAX(s.event_date) DESC
				) AS rn
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE v.city = ? AND v.state = ? AND s.status = ?
				AND sa.artist_id IN (SELECT artist_id FROM scene_artists)
			GROUP BY sa.artist_id, v.id, v.name
		)
		SELECT
			a.id   AS artist_id,
			a.name AS name,
			a.slug AS slug,
			a.city AS city,
			a.state AS state,
			avc.venue_id   AS primary_venue_id,
			avc.venue_name AS primary_venue_name
		FROM artists a
		JOIN scene_artists sa ON sa.artist_id = a.id
		LEFT JOIN artist_venue_counts avc ON avc.artist_id = a.id AND avc.rn = 1
		ORDER BY a.name ASC
	`
	var rows []sceneArtistRow
	if err := s.db.Raw(q,
		city, state, catalogm.ShowStatusApproved,
		city, state, catalogm.ShowStatusApproved,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// querySceneRelationships fetches all stored relationships where BOTH source
// and target artist IDs are in the scene's artist set, optionally filtered to
// the resolved type list. The relationships table already pre-filters
// shared_bills below the production threshold (see DeriveSharedBills minShows
// default), so no `min_weight` query parameter is needed at v1.
func (s *SceneService) querySceneRelationships(artistIDs []uint, types []string) ([]sceneRelationshipRow, error) {
	if len(artistIDs) < 2 {
		return nil, nil
	}
	var rows []sceneRelationshipRow
	q := s.db.Table("artist_relationships").
		Select("source_artist_id, target_artist_id, relationship_type, score, detail").
		Where("source_artist_id IN ? AND target_artist_id IN ?", artistIDs, artistIDs)
	if len(types) > 0 {
		q = q.Where("relationship_type IN ?", types)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// batchUpcomingShowCount returns a map of artist_id → upcoming approved show
// count, mirroring the batch pattern in GetArtistGraph step 4. Returns an empty
// map (never nil) so callers can index without a nil check.
func (s *SceneService) batchUpcomingShowCount(artistIDs []uint) map[uint]int {
	out := make(map[uint]int, len(artistIDs))
	if len(artistIDs) == 0 {
		return out
	}
	type row struct {
		ArtistID  uint
		ShowCount int64
	}
	var rows []row
	s.db.Table("show_artists").
		Select("show_artists.artist_id, COUNT(DISTINCT shows.id) AS show_count").
		Joins("JOIN shows ON shows.id = show_artists.show_id").
		Where("show_artists.artist_id IN ? AND shows.status = ? AND shows.event_date > NOW()",
			artistIDs, catalogm.ShowStatusApproved).
		Group("show_artists.artist_id").
		Scan(&rows)
	for _, r := range rows {
		out[r.ArtistID] = int(r.ShowCount)
	}
	return out
}

// buildSceneClusters converts the per-artist primary-venue rows into a sorted
// list of cluster definitions. Clusters >= sceneClusterMinSize are first-class
// (capped at sceneClusterMaxFirstClass = 8, the Okabe-Ito palette size); the
// remainder roll up to a single "other" cluster. Returns the cluster list
// (caller-facing) and a venue_id → cluster_id lookup (used to assign nodes).
func buildSceneClusters(rows []sceneArtistRow) ([]contracts.SceneGraphCluster, map[uint]string) {
	type venueCount struct {
		venueID uint
		name    string
		count   int
	}
	byVenue := make(map[uint]*venueCount)
	for _, r := range rows {
		if r.PrimaryVenueID == nil {
			continue
		}
		entry, ok := byVenue[*r.PrimaryVenueID]
		if !ok {
			name := ""
			if r.PrimaryVenueName != nil {
				name = *r.PrimaryVenueName
			}
			entry = &venueCount{venueID: *r.PrimaryVenueID, name: name}
			byVenue[*r.PrimaryVenueID] = entry
		}
		entry.count++
	}

	venues := make([]*venueCount, 0, len(byVenue))
	for _, v := range byVenue {
		venues = append(venues, v)
	}
	// Sort by count desc, then name asc for deterministic ordering on ties.
	for i := 1; i < len(venues); i++ {
		for j := i; j > 0; j-- {
			a, b := venues[j], venues[j-1]
			better := a.count > b.count || (a.count == b.count && a.name < b.name)
			if !better {
				break
			}
			venues[j], venues[j-1] = b, a
		}
	}

	clusterIDByVenue := make(map[uint]string, len(venues))
	clusters := make([]contracts.SceneGraphCluster, 0, len(venues)+1)
	otherSize := 0

	for i, v := range venues {
		if v.count >= sceneClusterMinSize && len(clusters) < sceneClusterMaxFirstClass {
			cid := fmt.Sprintf("v_%d", v.venueID)
			clusterIDByVenue[v.venueID] = cid
			clusters = append(clusters, contracts.SceneGraphCluster{
				ID:         cid,
				Label:      v.name,
				Size:       v.count,
				ColorIndex: i,
			})
			continue
		}
		// Falls into "other" — reuses the bucket id; no per-venue mapping needed.
		clusterIDByVenue[v.venueID] = sceneClusterOtherID
		otherSize += v.count
	}

	// Artists with no primary venue (defensive — e.g. data anomalies) land in "other".
	for _, r := range rows {
		if r.PrimaryVenueID == nil {
			otherSize++
		}
	}

	if otherSize > 0 {
		clusters = append(clusters, contracts.SceneGraphCluster{
			ID:         sceneClusterOtherID,
			Label:      sceneClusterOtherLabel,
			Size:       otherSize,
			ColorIndex: -1,
		})
	}

	return clusters, clusterIDByVenue
}
