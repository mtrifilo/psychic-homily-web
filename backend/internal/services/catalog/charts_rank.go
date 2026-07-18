package catalog

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// GetChartEntityRank returns the entity's global-scope placement in its
// mapped chart module for window. Rank is computed as
// 1 + COUNT(entities that sort strictly ahead) under the same ordering the
// module pages use — so a badge never contradicts the drill-down list —
// not by scanning cached pages. Below-floor / out-of-window / (for shows)
// soonest_upcoming fallback → Rank=nil, never 0. Cached per
// (entity_type, entity_id, window) at module-tier TTL.
func (s *ChartsService) GetChartEntityRank(entityType contracts.ChartRankEntityType, entityID uint, window contracts.ChartWindow) (*contracts.ChartEntityRank, error) {
	normalized := window.OrDefault()
	module, ok := chartRankModuleFor(entityType)
	if !ok {
		return nil, fmt.Errorf("unsupported chart rank entity_type %q", entityType)
	}
	key := fmt.Sprintf("rank|%s|%d|%s", entityType, entityID, normalized)
	return chartsCached(s.cache, key, chartWindowTTL(s.cache, normalized, chartsModuleTTL), func() (*contracts.ChartEntityRank, error) {
		rank, err := s.chartEntityRankUncached(entityType, entityID, normalized)
		if err != nil {
			return nil, err
		}
		return &contracts.ChartEntityRank{
			EntityType: entityType,
			EntityID:   entityID,
			Window:     normalized,
			Module:     module,
			Rank:       rank,
		}, nil
	})
}

func chartRankModuleFor(entityType contracts.ChartRankEntityType) (contracts.ChartRankModule, bool) {
	switch entityType {
	case contracts.ChartRankEntityShow:
		return contracts.ChartRankModuleMostAnticipated, true
	case contracts.ChartRankEntityArtist:
		return contracts.ChartRankModuleMostActiveArtists, true
	case contracts.ChartRankEntityVenue:
		return contracts.ChartRankModuleBusiestVenues, true
	case contracts.ChartRankEntityRelease:
		return contracts.ChartRankModuleNewReleases, true
	default:
		return "", false
	}
}

func (s *ChartsService) chartEntityRankUncached(entityType contracts.ChartRankEntityType, entityID uint, window contracts.ChartWindow) (*int, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	switch entityType {
	case contracts.ChartRankEntityShow:
		return s.mostAnticipatedRank(entityID, window)
	case contracts.ChartRankEntityArtist:
		return s.mostActiveArtistRank(entityID, window)
	case contracts.ChartRankEntityVenue:
		return s.busiestVenueRank(entityID, window)
	case contracts.ChartRankEntityRelease:
		return s.newReleaseRank(entityID, window)
	default:
		return nil, fmt.Errorf("unsupported chart rank entity_type %q", entityType)
	}
}

// mostAnticipatedRank mirrors getMostAnticipatedShowsUncached eligibility,
// floor, min-qualifying mode gate, and ORDER BY save_count DESC, event_date
// ASC, id ASC — global scope only.
func (s *ChartsService) mostAnticipatedRank(showID uint, window contracts.ChartWindow) (*int, error) {
	startOfToday, windowEnd, pastCalendar := mostAnticipatedHorizon(window)
	if pastCalendar {
		return nil, nil
	}

	eligibilitySQL, eligibilityArgs := appendShowSceneScope(
		mostAnticipatedEligibilitySQL, []any{catalogm.ShowStatusApproved, startOfToday}, "")
	if windowEnd != nil {
		eligibilitySQL += `
			AND s.event_date < ?`
		eligibilityArgs = append(eligibilityArgs, *windowEnd)
	}

	// Qualifying set = same HAVING floor as the ranked module. Mode gate:
	// when fewer than minQualifying clear the floor, the module is in
	// soonest_upcoming fallback (no ranks on the page) → null here too.
	query := `
		WITH qualifying AS (
			SELECT
				s.id,
				s.event_date,
				COUNT(ub.id) AS save_count
			FROM shows s
			LEFT JOIN user_bookmarks ub ON ub.entity_id = s.id
				AND ub.entity_type = ?
				AND ub.action = ?
			` + eligibilitySQL + `
			GROUP BY s.id, s.event_date
			HAVING COUNT(ub.id) >= ?
		),
		target AS (
			SELECT id, event_date, save_count FROM qualifying WHERE id = ?
		)
		SELECT CASE
			WHEN (SELECT COUNT(*) FROM qualifying) < ? THEN NULL
			WHEN NOT EXISTS (SELECT 1 FROM target) THEN NULL
			ELSE 1 + (
				SELECT COUNT(*)::int FROM qualifying q
				CROSS JOIN target t
				WHERE q.save_count > t.save_count
					OR (q.save_count = t.save_count AND q.event_date < t.event_date)
					OR (q.save_count = t.save_count AND q.event_date = t.event_date AND q.id < t.id)
			)
		END AS rank`

	args := append([]any{
		engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave,
	}, eligibilityArgs...)
	args = append(args, mostAnticipatedSaveFloor, showID, mostAnticipatedMinQualifying)

	var rank *int
	if err := s.db.Raw(query, args...).Scan(&rank).Error; err != nil {
		return nil, fmt.Errorf("failed to get most-anticipated rank: %w", err)
	}
	return rank, nil
}

// mostActiveArtistRank mirrors getMostActiveArtistsUncached: in-window
// played shows, ORDER BY show_count DESC, name ASC, id ASC. Zero in-window
// shows → null (the module never lists them).
func (s *ChartsService) mostActiveArtistRank(artistID uint, window contracts.ChartWindow) (*int, error) {
	now := time.Now().UTC()
	bounds := chartWindowBounds(window, now)

	coreSQL := `
		SELECT
			a.id,
			a.name,
			COUNT(*) AS show_count
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		JOIN shows s ON s.id = sa.show_id
		WHERE s.status = ?`
	coreArgs := []any{catalogm.ShowStatusApproved}
	coreSQL, coreArgs = appendChartShowWindow(coreSQL, coreArgs, bounds)
	// Same GROUP BY columns as getMostActiveArtistsUncached so the ranked
	// set can't drift if a future column is added to the module select.
	coreSQL += `
		GROUP BY a.id, a.name, a.slug, a.city, a.state`

	query := `
		WITH ranked AS (` + coreSQL + `),
		target AS (
			SELECT id, name, show_count FROM ranked WHERE id = ?
		)
		SELECT CASE
			WHEN NOT EXISTS (SELECT 1 FROM target) THEN NULL
			ELSE 1 + (
				SELECT COUNT(*)::int FROM ranked r
				CROSS JOIN target t
				WHERE r.show_count > t.show_count
					OR (r.show_count = t.show_count AND r.name < t.name)
					OR (r.show_count = t.show_count AND r.name = t.name AND r.id < t.id)
			)
		END AS rank`
	args := append(append([]any{}, coreArgs...), artistID)

	var rank *int
	if err := s.db.Raw(query, args...).Scan(&rank).Error; err != nil {
		return nil, fmt.Errorf("failed to get most-active-artists rank: %w", err)
	}
	return rank, nil
}

// busiestVenueRank mirrors getBusiestVenuesUncached: in-window hosted shows,
// ORDER BY show_count DESC, name ASC, id ASC.
func (s *ChartsService) busiestVenueRank(venueID uint, window contracts.ChartWindow) (*int, error) {
	now := time.Now().UTC()
	bounds := chartWindowBounds(window, now)

	coreSQL := `
		SELECT
			v.id,
			v.name,
			COUNT(*) AS show_count
		FROM show_venues sv
		JOIN venues v ON v.id = sv.venue_id
		JOIN shows s ON s.id = sv.show_id
		WHERE s.status = ?`
	coreArgs := []any{catalogm.ShowStatusApproved}
	coreSQL, coreArgs = appendChartShowWindow(coreSQL, coreArgs, bounds)
	coreSQL += `
		GROUP BY v.id, v.name, v.slug, v.city, v.state`

	query := `
		WITH ranked AS (` + coreSQL + `),
		target AS (
			SELECT id, name, show_count FROM ranked WHERE id = ?
		)
		SELECT CASE
			WHEN NOT EXISTS (SELECT 1 FROM target) THEN NULL
			ELSE 1 + (
				SELECT COUNT(*)::int FROM ranked r
				CROSS JOIN target t
				WHERE r.show_count > t.show_count
					OR (r.show_count = t.show_count AND r.name < t.name)
					OR (r.show_count = t.show_count AND r.name = t.name AND r.id < t.id)
			)
		END AS rank`
	args := append(append([]any{}, coreArgs...), venueID)

	var rank *int
	if err := s.db.Raw(query, args...).Scan(&rank).Error; err != nil {
		return nil, fmt.Errorf("failed to get busiest-venues rank: %w", err)
	}
	return rank, nil
}

// newReleaseRank mirrors getNewReleasesUncached: windowed by
// newReleaseDateSQL, ORDER BY date DESC, created_at DESC, id DESC.
func (s *ChartsService) newReleaseRank(releaseID uint, window contracts.ChartWindow) (*int, error) {
	now := time.Now().UTC()
	bounds := chartWindowBounds(window, now)

	upperOperator := "<="
	upperDay := now.Format("2006-01-02")
	if bounds.calendarEnd != nil && !bounds.calendarEnd.After(now) {
		upperOperator = "<"
		upperDay = bounds.calendarEnd.Format("2006-01-02")
	}
	coreSQL := `
		SELECT
			r.id,
			r.created_at,
			` + newReleaseDateSQL + ` AS sort_date
		FROM releases r
		WHERE ` + newReleaseDateSQL + ` ` + upperOperator + ` ?`
	coreArgs := []any{upperDay}
	if bounds.start != nil {
		coreSQL += `
			AND ` + newReleaseDateSQL + ` >= ?`
		coreArgs = append(coreArgs, bounds.start.Format("2006-01-02"))
	}

	query := `
		WITH ranked AS (` + coreSQL + `),
		target AS (
			SELECT id, created_at, sort_date FROM ranked WHERE id = ?
		)
		SELECT CASE
			WHEN NOT EXISTS (SELECT 1 FROM target) THEN NULL
			ELSE 1 + (
				SELECT COUNT(*)::int FROM ranked r
				CROSS JOIN target t
				WHERE r.sort_date > t.sort_date
					OR (r.sort_date = t.sort_date AND r.created_at > t.created_at)
					OR (r.sort_date = t.sort_date AND r.created_at = t.created_at AND r.id > t.id)
			)
		END AS rank`
	args := append(append([]any{}, coreArgs...), releaseID)

	var rank *int
	if err := s.db.Raw(query, args...).Scan(&rank).Error; err != nil {
		return nil, fmt.Errorf("failed to get new-releases rank: %w", err)
	}
	return rank, nil
}
