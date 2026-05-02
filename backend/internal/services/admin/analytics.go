package admin

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/contracts"
)

// AnalyticsService handles platform analytics dashboard queries.
type AnalyticsService struct {
	db *gorm.DB
}

// NewAnalyticsService creates a new analytics service.
func NewAnalyticsService(database *gorm.DB) *AnalyticsService {
	if database == nil {
		database = db.GetDB()
	}
	return &AnalyticsService{db: database}
}

// clampMonths restricts months to the 1–24 range.
func clampMonths(months int) int {
	if months < 1 {
		return 1
	}
	if months > 24 {
		return 24
	}
	return months
}

// generateMonthKeys returns a slice of "YYYY-MM" strings for the last N months
// (including the current month), in chronological order.
func generateMonthKeys(months int) []string {
	// Use 1st of current month to avoid AddDate skipping short months
	// (e.g., March 29 - 1 month = March 1 in non-leap years, not Feb 29)
	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	keys := make([]string, months)
	for i := 0; i < months; i++ {
		t := first.AddDate(0, -(months - 1 - i), 0)
		keys[i] = t.Format("2006-01")
	}
	return keys
}

// fillMonthlyGaps takes raw query results and ensures every month in the range
// has an entry (defaulting to 0).
func fillMonthlyGaps(raw []contracts.MonthlyCount, monthKeys []string) []contracts.MonthlyCount {
	lookup := make(map[string]int, len(raw))
	for _, r := range raw {
		lookup[r.Month] = r.Count
	}
	result := make([]contracts.MonthlyCount, len(monthKeys))
	for i, k := range monthKeys {
		result[i] = contracts.MonthlyCount{Month: k, Count: lookup[k]}
	}
	return result
}

// fillEngagementGaps is the same as fillMonthlyGaps but for EngagementMetric.
func fillEngagementGaps(raw []contracts.EngagementMetric, monthKeys []string) []contracts.EngagementMetric {
	lookup := make(map[string]int, len(raw))
	for _, r := range raw {
		lookup[r.Month] = r.Count
	}
	result := make([]contracts.EngagementMetric, len(monthKeys))
	for i, k := range monthKeys {
		result[i] = contracts.EngagementMetric{Month: k, Count: lookup[k]}
	}
	return result
}

// queryMonthlyCounts runs a monthly aggregation query on a given table.
func (s *AnalyticsService) queryMonthlyCounts(table string, since time.Time) ([]contracts.MonthlyCount, error) {
	var rows []contracts.MonthlyCount
	err := s.db.Raw(fmt.Sprintf(`
		SELECT TO_CHAR(DATE_TRUNC('month', created_at), 'YYYY-MM') AS month,
		       COUNT(*) AS count
		FROM %s
		WHERE created_at >= ?
		GROUP BY DATE_TRUNC('month', created_at)
		ORDER BY month
	`, table), since).Scan(&rows).Error
	return rows, err
}

// queryMonthlyCountsWithCondition runs a monthly aggregation with an extra WHERE clause.
func (s *AnalyticsService) queryMonthlyCountsWithCondition(table, condition string, since time.Time) ([]contracts.MonthlyCount, error) {
	var rows []contracts.MonthlyCount
	err := s.db.Raw(fmt.Sprintf(`
		SELECT TO_CHAR(DATE_TRUNC('month', created_at), 'YYYY-MM') AS month,
		       COUNT(*) AS count
		FROM %s
		WHERE created_at >= ? AND %s
		GROUP BY DATE_TRUNC('month', created_at)
		ORDER BY month
	`, table, condition), since).Scan(&rows).Error
	return rows, err
}

// queryEngagementMetric runs a monthly aggregation query for engagement metrics.
func (s *AnalyticsService) queryEngagementMetric(table string, since time.Time) ([]contracts.EngagementMetric, error) {
	var rows []contracts.EngagementMetric
	err := s.db.Raw(fmt.Sprintf(`
		SELECT TO_CHAR(DATE_TRUNC('month', created_at), 'YYYY-MM') AS month,
		       COUNT(*) AS count
		FROM %s
		WHERE created_at >= ?
		GROUP BY DATE_TRUNC('month', created_at)
		ORDER BY month
	`, table), since).Scan(&rows).Error
	return rows, err
}

// queryEngagementMetricWithCondition runs a monthly engagement aggregation with an extra WHERE clause.
func (s *AnalyticsService) queryEngagementMetricWithCondition(table, condition string, since time.Time) ([]contracts.EngagementMetric, error) {
	var rows []contracts.EngagementMetric
	err := s.db.Raw(fmt.Sprintf(`
		SELECT TO_CHAR(DATE_TRUNC('month', created_at), 'YYYY-MM') AS month,
		       COUNT(*) AS count
		FROM %s
		WHERE created_at >= ? AND %s
		GROUP BY DATE_TRUNC('month', created_at)
		ORDER BY month
	`, table, condition), since).Scan(&rows).Error
	return rows, err
}

// GetGrowthMetrics returns time-series entity creation counts over N months.
func (s *AnalyticsService) GetGrowthMetrics(months int) (*contracts.GrowthMetricsResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	months = clampMonths(months)
	since := time.Now().UTC().AddDate(0, -(months - 1), 0)
	since = time.Date(since.Year(), since.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthKeys := generateMonthKeys(months)

	resp := &contracts.GrowthMetricsResponse{}

	shows, err := s.queryMonthlyCounts("shows", since)
	if err != nil {
		return nil, fmt.Errorf("querying shows growth: %w", err)
	}
	resp.Shows = fillMonthlyGaps(shows, monthKeys)

	artists, err := s.queryMonthlyCounts("artists", since)
	if err != nil {
		return nil, fmt.Errorf("querying artists growth: %w", err)
	}
	resp.Artists = fillMonthlyGaps(artists, monthKeys)

	venues, err := s.queryMonthlyCounts("venues", since)
	if err != nil {
		return nil, fmt.Errorf("querying venues growth: %w", err)
	}
	resp.Venues = fillMonthlyGaps(venues, monthKeys)

	releases, err := s.queryMonthlyCounts("releases", since)
	if err != nil {
		return nil, fmt.Errorf("querying releases growth: %w", err)
	}
	resp.Releases = fillMonthlyGaps(releases, monthKeys)

	labels, err := s.queryMonthlyCounts("labels", since)
	if err != nil {
		return nil, fmt.Errorf("querying labels growth: %w", err)
	}
	resp.Labels = fillMonthlyGaps(labels, monthKeys)

	users, err := s.queryMonthlyCounts("users", since)
	if err != nil {
		return nil, fmt.Errorf("querying users growth: %w", err)
	}
	resp.Users = fillMonthlyGaps(users, monthKeys)

	return resp, nil
}

// GetEngagementMetrics returns monthly engagement action counts.
func (s *AnalyticsService) GetEngagementMetrics(months int) (*contracts.EngagementMetricsResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	months = clampMonths(months)
	since := time.Now().UTC().AddDate(0, -(months - 1), 0)
	since = time.Date(since.Year(), since.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthKeys := generateMonthKeys(months)

	resp := &contracts.EngagementMetricsResponse{}

	// Bookmarks — save/bookmark actions (exclude follow, going, interested as they're counted separately)
	bookmarks, err := s.queryEngagementMetricWithCondition(
		"user_bookmarks",
		"action IN ('save', 'bookmark')",
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("querying bookmarks: %w", err)
	}
	resp.Bookmarks = fillEngagementGaps(bookmarks, monthKeys)

	// Tags added
	tagsAdded, err := s.queryEngagementMetric("entity_tags", since)
	if err != nil {
		return nil, fmt.Errorf("querying tags added: %w", err)
	}
	resp.TagsAdded = fillEngagementGaps(tagsAdded, monthKeys)

	// Tag votes
	tagVotes, err := s.queryEngagementMetric("tag_votes", since)
	if err != nil {
		return nil, fmt.Errorf("querying tag votes: %w", err)
	}
	resp.TagVotes = fillEngagementGaps(tagVotes, monthKeys)

	// Collection items
	collectionItems, err := s.queryEngagementMetric("collection_items", since)
	if err != nil {
		return nil, fmt.Errorf("querying collection items: %w", err)
	}
	resp.CollectionItems = fillEngagementGaps(collectionItems, monthKeys)

	// Requests
	requests, err := s.queryEngagementMetric("requests", since)
	if err != nil {
		return nil, fmt.Errorf("querying requests: %w", err)
	}
	resp.Requests = fillEngagementGaps(requests, monthKeys)

	// Request votes
	requestVotes, err := s.queryEngagementMetric("request_votes", since)
	if err != nil {
		return nil, fmt.Errorf("querying request votes: %w", err)
	}
	resp.RequestVotes = fillEngagementGaps(requestVotes, monthKeys)

	// Revisions
	revisions, err := s.queryEngagementMetric("revisions", since)
	if err != nil {
		return nil, fmt.Errorf("querying revisions: %w", err)
	}
	resp.Revisions = fillEngagementGaps(revisions, monthKeys)

	// Follows — user_bookmarks with action='follow'
	follows, err := s.queryEngagementMetricWithCondition(
		"user_bookmarks",
		"action = 'follow'",
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("querying follows: %w", err)
	}
	resp.Follows = fillEngagementGaps(follows, monthKeys)

	// Attendance — user_bookmarks with action IN ('going', 'interested')
	attendance, err := s.queryEngagementMetricWithCondition(
		"user_bookmarks",
		"action IN ('going', 'interested')",
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("querying attendance: %w", err)
	}
	resp.Attendance = fillEngagementGaps(attendance, monthKeys)

	return resp, nil
}

// GetCommunityHealth returns a snapshot of community health metrics.
func (s *AnalyticsService) GetCommunityHealth() (*contracts.CommunityHealthResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	resp := &contracts.CommunityHealthResponse{}
	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)

	// Active contributors — users who made any contribution in last 30 days.
	// Contributions: entity_tags, tag_votes, revisions, requests, request_votes,
	// collection_items, user_bookmarks.
	var activeCount int
	err := s.db.Raw(`
		SELECT COUNT(DISTINCT contributor_id) FROM (
			SELECT added_by_user_id AS contributor_id FROM entity_tags WHERE created_at >= ?
			UNION
			SELECT user_id FROM tag_votes WHERE created_at >= ?
			UNION
			SELECT user_id FROM revisions WHERE created_at >= ?
			UNION
			SELECT requester_id FROM requests WHERE created_at >= ?
			UNION
			SELECT user_id FROM request_votes WHERE created_at >= ?
			UNION
			SELECT added_by_user_id FROM collection_items WHERE created_at >= ?
			UNION
			SELECT user_id FROM user_bookmarks WHERE created_at >= ?
		) sub
	`, thirtyDaysAgo, thirtyDaysAgo, thirtyDaysAgo, thirtyDaysAgo,
		thirtyDaysAgo, thirtyDaysAgo, thirtyDaysAgo).Scan(&activeCount).Error
	if err != nil {
		return nil, fmt.Errorf("querying active contributors: %w", err)
	}
	resp.ActiveContributors30d = activeCount

	// Contributions per week (last 12 weeks)
	twelveWeeksAgo := time.Now().UTC().AddDate(0, 0, -84)
	type weekRow struct {
		Week  string
		Count int
	}
	var weekRows []weekRow
	err = s.db.Raw(`
		SELECT week, SUM(cnt) AS count FROM (
			SELECT TO_CHAR(DATE_TRUNC('week', created_at), 'IYYY-"W"IW') AS week, COUNT(*)::int AS cnt
			FROM entity_tags WHERE created_at >= ? GROUP BY DATE_TRUNC('week', created_at)
			UNION ALL
			SELECT TO_CHAR(DATE_TRUNC('week', created_at), 'IYYY-"W"IW'), COUNT(*)::int
			FROM tag_votes WHERE created_at >= ? GROUP BY DATE_TRUNC('week', created_at)
			UNION ALL
			SELECT TO_CHAR(DATE_TRUNC('week', created_at), 'IYYY-"W"IW'), COUNT(*)::int
			FROM revisions WHERE created_at >= ? GROUP BY DATE_TRUNC('week', created_at)
			UNION ALL
			SELECT TO_CHAR(DATE_TRUNC('week', created_at), 'IYYY-"W"IW'), COUNT(*)::int
			FROM requests WHERE created_at >= ? GROUP BY DATE_TRUNC('week', created_at)
			UNION ALL
			SELECT TO_CHAR(DATE_TRUNC('week', created_at), 'IYYY-"W"IW'), COUNT(*)::int
			FROM request_votes WHERE created_at >= ? GROUP BY DATE_TRUNC('week', created_at)
			UNION ALL
			SELECT TO_CHAR(DATE_TRUNC('week', created_at), 'IYYY-"W"IW'), COUNT(*)::int
			FROM collection_items WHERE created_at >= ? GROUP BY DATE_TRUNC('week', created_at)
			UNION ALL
			SELECT TO_CHAR(DATE_TRUNC('week', created_at), 'IYYY-"W"IW'), COUNT(*)::int
			FROM user_bookmarks WHERE created_at >= ? GROUP BY DATE_TRUNC('week', created_at)
		) sub
		GROUP BY week
		ORDER BY week
	`, twelveWeeksAgo, twelveWeeksAgo, twelveWeeksAgo, twelveWeeksAgo,
		twelveWeeksAgo, twelveWeeksAgo, twelveWeeksAgo).Scan(&weekRows).Error
	if err != nil {
		return nil, fmt.Errorf("querying contributions per week: %w", err)
	}
	contribs := make([]contracts.WeeklyContributions, 0, len(weekRows))
	for _, r := range weekRows {
		contribs = append(contribs, contracts.WeeklyContributions{
			Week:  r.Week,
			Count: r.Count,
		})
	}
	resp.ContributionsPerWeek = contribs

	// Request fulfillment rate — fulfilled / (total non-canceled)
	type rateRow struct {
		Total     int
		Fulfilled int
	}
	var rate rateRow
	err = s.db.Raw(`
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'fulfilled') AS fulfilled
		FROM requests
		WHERE status != 'canceled'
	`).Scan(&rate).Error
	if err != nil {
		return nil, fmt.Errorf("querying request fulfillment rate: %w", err)
	}
	if rate.Total > 0 {
		resp.RequestFulfillmentRate = float64(rate.Fulfilled) / float64(rate.Total)
	}

	// New collections in last 30 days
	var newCollections int
	err = s.db.Raw(`
		SELECT COUNT(*) FROM collections WHERE created_at >= ?
	`, thirtyDaysAgo).Scan(&newCollections).Error
	if err != nil {
		return nil, fmt.Errorf("querying new collections: %w", err)
	}
	resp.NewCollections30d = newCollections

	// Top contributors (top 10 by contribution count in last 30 days)
	type contribRow struct {
		UserID      uint
		Username    string
		DisplayName string
		Count       int
	}
	var topRows []contribRow
	err = s.db.Raw(`
		SELECT sub.contributor_id AS user_id,
		       COALESCE(u.username, '') AS username,
		       TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')) AS display_name,
		       SUM(sub.cnt)::int AS count
		FROM (
			SELECT added_by_user_id AS contributor_id, COUNT(*)::int AS cnt
			FROM entity_tags WHERE created_at >= ? GROUP BY added_by_user_id
			UNION ALL
			SELECT user_id, COUNT(*)::int FROM tag_votes WHERE created_at >= ? GROUP BY user_id
			UNION ALL
			SELECT user_id, COUNT(*)::int FROM revisions WHERE created_at >= ? GROUP BY user_id
			UNION ALL
			SELECT requester_id, COUNT(*)::int FROM requests WHERE created_at >= ? GROUP BY requester_id
			UNION ALL
			SELECT user_id, COUNT(*)::int FROM request_votes WHERE created_at >= ? GROUP BY user_id
			UNION ALL
			SELECT added_by_user_id, COUNT(*)::int FROM collection_items WHERE created_at >= ? GROUP BY added_by_user_id
			UNION ALL
			SELECT user_id, COUNT(*)::int FROM user_bookmarks WHERE created_at >= ? GROUP BY user_id
		) sub
		JOIN users u ON u.id = sub.contributor_id
		GROUP BY sub.contributor_id, u.username, u.first_name, u.last_name
		ORDER BY count DESC
		LIMIT 10
	`, thirtyDaysAgo, thirtyDaysAgo, thirtyDaysAgo, thirtyDaysAgo,
		thirtyDaysAgo, thirtyDaysAgo, thirtyDaysAgo).Scan(&topRows).Error
	if err != nil {
		return nil, fmt.Errorf("querying top contributors: %w", err)
	}
	topContributors := make([]contracts.TopContributor, 0, len(topRows))
	for _, r := range topRows {
		topContributors = append(topContributors, contracts.TopContributor{
			UserID:      r.UserID,
			Username:    r.Username,
			DisplayName: r.DisplayName,
			Count:       r.Count,
		})
	}
	resp.TopContributors = topContributors

	return resp, nil
}

// GetDataQualityTrends returns data quality metrics and monthly approval/rejection trends.
func (s *AnalyticsService) GetDataQualityTrends(months int) (*contracts.DataQualityTrendsResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	months = clampMonths(months)
	since := time.Now().UTC().AddDate(0, -(months - 1), 0)
	since = time.Date(since.Year(), since.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthKeys := generateMonthKeys(months)

	resp := &contracts.DataQualityTrendsResponse{}

	// Monthly approved shows (status changed to approved — we use shows with status='approved')
	approved, err := s.queryMonthlyCountsWithCondition("shows", "status = 'approved'", since)
	if err != nil {
		return nil, fmt.Errorf("querying approved shows: %w", err)
	}
	resp.ShowsApproved = fillMonthlyGaps(approved, monthKeys)

	// Monthly rejected shows
	rejected, err := s.queryMonthlyCountsWithCondition("shows", "status = 'rejected'", since)
	if err != nil {
		return nil, fmt.Errorf("querying rejected shows: %w", err)
	}
	resp.ShowsRejected = fillMonthlyGaps(rejected, monthKeys)

	// Current pending review count
	var pendingCount int
	err = s.db.Raw(`SELECT COUNT(*) FROM shows WHERE status = 'pending'`).Scan(&pendingCount).Error
	if err != nil {
		return nil, fmt.Errorf("querying pending shows: %w", err)
	}
	resp.PendingReviewCount = pendingCount

	// Artists without releases
	var noReleases int
	err = s.db.Raw(`
		SELECT COUNT(*) FROM artists a
		WHERE NOT EXISTS (
			SELECT 1 FROM artist_releases ar WHERE ar.artist_id = a.id
		)
	`).Scan(&noReleases).Error
	if err != nil {
		return nil, fmt.Errorf("querying artists without releases: %w", err)
	}
	resp.ArtistsWithoutReleases = noReleases

	// Inactive venues (no shows in last 90 days)
	ninetyDaysAgo := time.Now().UTC().AddDate(0, 0, -90)
	var inactiveVenues int
	err = s.db.Raw(`
		SELECT COUNT(*) FROM venues v
		WHERE v.verified = true
		  AND NOT EXISTS (
			SELECT 1 FROM show_venues sv
			JOIN shows s ON s.id = sv.show_id
			WHERE sv.venue_id = v.id
			  AND s.event_date >= ?
		  )
	`, ninetyDaysAgo).Scan(&inactiveVenues).Error
	if err != nil {
		return nil, fmt.Errorf("querying inactive venues: %w", err)
	}
	resp.InactiveVenues90d = inactiveVenues

	return resp, nil
}
