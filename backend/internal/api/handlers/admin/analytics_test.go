package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

func testAnalyticsHandler() *AnalyticsHandler {
	return NewAnalyticsHandler(&mockAnalyticsService{})
}

func analyticsAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 1, IsAdmin: true})
}

func analyticsNonAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 2, IsAdmin: false})
}

// ============================================================================
// Tests: Admin Guard - Growth
// ============================================================================

func TestAnalyticsHandler_Growth_RequiresAdmin(t *testing.T) {
	h := testAnalyticsHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.GetGrowthMetricsHandler(context.Background(), &GetGrowthMetricsRequest{})
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.GetGrowthMetricsHandler(analyticsNonAdminCtx(), &GetGrowthMetricsRequest{})
		assertHumaError(t, err, 403)
	})
}

// ============================================================================
// Tests: Admin Guard - Engagement
// ============================================================================

func TestAnalyticsHandler_Engagement_RequiresAdmin(t *testing.T) {
	h := testAnalyticsHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.GetEngagementMetricsHandler(context.Background(), &GetEngagementMetricsRequest{})
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.GetEngagementMetricsHandler(analyticsNonAdminCtx(), &GetEngagementMetricsRequest{})
		assertHumaError(t, err, 403)
	})
}

// ============================================================================
// Tests: Admin Guard - Community
// ============================================================================

func TestAnalyticsHandler_Community_RequiresAdmin(t *testing.T) {
	h := testAnalyticsHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.GetCommunityHealthHandler(context.Background(), &GetCommunityHealthRequest{})
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.GetCommunityHealthHandler(analyticsNonAdminCtx(), &GetCommunityHealthRequest{})
		assertHumaError(t, err, 403)
	})
}

// ============================================================================
// Tests: Admin Guard - Data Quality Trends
// ============================================================================

func TestAnalyticsHandler_DataQuality_RequiresAdmin(t *testing.T) {
	h := testAnalyticsHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.GetDataQualityTrendsHandler(context.Background(), &GetDataQualityTrendsRequest{})
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.GetDataQualityTrendsHandler(analyticsNonAdminCtx(), &GetDataQualityTrendsRequest{})
		assertHumaError(t, err, 403)
	})
}

// ============================================================================
// Tests: GetGrowthMetricsHandler
// ============================================================================

func TestAnalyticsHandler_Growth_Success(t *testing.T) {
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getGrowthMetricsFn: func(months int) (*contracts.GrowthMetricsResponse, error) {
			if months != 6 {
				t.Errorf("expected months=6, got %d", months)
			}
			return &contracts.GrowthMetricsResponse{
				Shows:    []contracts.MonthlyCount{{Month: "2026-03", Count: 10}},
				Artists:  []contracts.MonthlyCount{{Month: "2026-03", Count: 5}},
				Venues:   []contracts.MonthlyCount{{Month: "2026-03", Count: 2}},
				Releases: []contracts.MonthlyCount{{Month: "2026-03", Count: 3}},
				Labels:   []contracts.MonthlyCount{{Month: "2026-03", Count: 1}},
				Users:    []contracts.MonthlyCount{{Month: "2026-03", Count: 8}},
			}, nil
		},
	})

	resp, err := h.GetGrowthMetricsHandler(analyticsAdminCtx(), &GetGrowthMetricsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Shows) != 1 {
		t.Fatalf("expected 1 show entry, got %d", len(resp.Body.Shows))
	}
	if resp.Body.Shows[0].Count != 10 {
		t.Errorf("expected shows count=10, got %d", resp.Body.Shows[0].Count)
	}
	if resp.Body.Users[0].Count != 8 {
		t.Errorf("expected users count=8, got %d", resp.Body.Users[0].Count)
	}
}

func TestAnalyticsHandler_Growth_DefaultMonths(t *testing.T) {
	var receivedMonths int
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getGrowthMetricsFn: func(months int) (*contracts.GrowthMetricsResponse, error) {
			receivedMonths = months
			return &contracts.GrowthMetricsResponse{
				Shows: []contracts.MonthlyCount{}, Artists: []contracts.MonthlyCount{},
				Venues: []contracts.MonthlyCount{}, Releases: []contracts.MonthlyCount{},
				Labels: []contracts.MonthlyCount{}, Users: []contracts.MonthlyCount{},
			}, nil
		},
	})

	_, err := h.GetGrowthMetricsHandler(analyticsAdminCtx(), &GetGrowthMetricsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMonths != 6 {
		t.Errorf("expected default months=6, got %d", receivedMonths)
	}
}

func TestAnalyticsHandler_Growth_CustomMonths(t *testing.T) {
	var receivedMonths int
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getGrowthMetricsFn: func(months int) (*contracts.GrowthMetricsResponse, error) {
			receivedMonths = months
			return &contracts.GrowthMetricsResponse{
				Shows: []contracts.MonthlyCount{}, Artists: []contracts.MonthlyCount{},
				Venues: []contracts.MonthlyCount{}, Releases: []contracts.MonthlyCount{},
				Labels: []contracts.MonthlyCount{}, Users: []contracts.MonthlyCount{},
			}, nil
		},
	})

	_, err := h.GetGrowthMetricsHandler(analyticsAdminCtx(), &GetGrowthMetricsRequest{Months: 12})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMonths != 12 {
		t.Errorf("expected months=12, got %d", receivedMonths)
	}
}

func TestAnalyticsHandler_Growth_ServiceError(t *testing.T) {
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getGrowthMetricsFn: func(months int) (*contracts.GrowthMetricsResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetGrowthMetricsHandler(analyticsAdminCtx(), &GetGrowthMetricsRequest{})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Tests: GetEngagementMetricsHandler
// ============================================================================

func TestAnalyticsHandler_Engagement_Success(t *testing.T) {
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getEngagementMetricsFn: func(months int) (*contracts.EngagementMetricsResponse, error) {
			return &contracts.EngagementMetricsResponse{
				Bookmarks:       []contracts.EngagementMetric{{Month: "2026-03", Count: 15}},
				TagsAdded:       []contracts.EngagementMetric{{Month: "2026-03", Count: 8}},
				TagVotes:        []contracts.EngagementMetric{},
				CollectionItems: []contracts.EngagementMetric{},
				Requests:        []contracts.EngagementMetric{},
				RequestVotes:    []contracts.EngagementMetric{},
				Revisions:       []contracts.EngagementMetric{},
				Follows:         []contracts.EngagementMetric{},
				Attendance:      []contracts.EngagementMetric{},
			}, nil
		},
	})

	resp, err := h.GetEngagementMetricsHandler(analyticsAdminCtx(), &GetEngagementMetricsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Bookmarks[0].Count != 15 {
		t.Errorf("expected bookmarks count=15, got %d", resp.Body.Bookmarks[0].Count)
	}
	if resp.Body.TagsAdded[0].Count != 8 {
		t.Errorf("expected tags_added count=8, got %d", resp.Body.TagsAdded[0].Count)
	}
}

func TestAnalyticsHandler_Engagement_ServiceError(t *testing.T) {
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getEngagementMetricsFn: func(months int) (*contracts.EngagementMetricsResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetEngagementMetricsHandler(analyticsAdminCtx(), &GetEngagementMetricsRequest{})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Tests: GetCommunityHealthHandler
// ============================================================================

func TestAnalyticsHandler_Community_Success(t *testing.T) {
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getCommunityHealthFn: func() (*contracts.CommunityHealthResponse, error) {
			return &contracts.CommunityHealthResponse{
				ActiveContributors30d:  42,
				RequestFulfillmentRate: 0.75,
				NewCollections30d:      3,
				ContributionsPerWeek: []contracts.WeeklyContributions{
					{Week: "2026-W10", Count: 100},
					{Week: "2026-W11", Count: 120},
				},
				TopContributors: []contracts.TopContributor{
					{UserID: 1, Username: "alice", DisplayName: "Alice", Count: 50},
					{UserID: 2, Username: "bob", Count: 30},
				},
			}, nil
		},
	})

	resp, err := h.GetCommunityHealthHandler(analyticsAdminCtx(), &GetCommunityHealthRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ActiveContributors30d != 42 {
		t.Errorf("expected active_contributors_30d=42, got %d", resp.Body.ActiveContributors30d)
	}
	if resp.Body.RequestFulfillmentRate != 0.75 {
		t.Errorf("expected request_fulfillment_rate=0.75, got %f", resp.Body.RequestFulfillmentRate)
	}
	if resp.Body.NewCollections30d != 3 {
		t.Errorf("expected new_collections_30d=3, got %d", resp.Body.NewCollections30d)
	}
	if len(resp.Body.ContributionsPerWeek) != 2 {
		t.Fatalf("expected 2 weekly entries, got %d", len(resp.Body.ContributionsPerWeek))
	}
	if resp.Body.ContributionsPerWeek[0].Week != "2026-W10" {
		t.Errorf("expected week=2026-W10, got %s", resp.Body.ContributionsPerWeek[0].Week)
	}
	if len(resp.Body.TopContributors) != 2 {
		t.Fatalf("expected 2 top contributors, got %d", len(resp.Body.TopContributors))
	}
	if resp.Body.TopContributors[0].Username != "alice" {
		t.Errorf("expected username=alice, got %s", resp.Body.TopContributors[0].Username)
	}
	if resp.Body.TopContributors[1].DisplayName != "" {
		t.Errorf("expected empty display_name, got %s", resp.Body.TopContributors[1].DisplayName)
	}
}

func TestAnalyticsHandler_Community_ServiceError(t *testing.T) {
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getCommunityHealthFn: func() (*contracts.CommunityHealthResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetCommunityHealthHandler(analyticsAdminCtx(), &GetCommunityHealthRequest{})
	assertHumaError(t, err, 500)
}

func TestAnalyticsHandler_Community_Empty(t *testing.T) {
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getCommunityHealthFn: func() (*contracts.CommunityHealthResponse, error) {
			return &contracts.CommunityHealthResponse{
				ContributionsPerWeek: []contracts.WeeklyContributions{},
				TopContributors:      []contracts.TopContributor{},
			}, nil
		},
	})

	resp, err := h.GetCommunityHealthHandler(analyticsAdminCtx(), &GetCommunityHealthRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ActiveContributors30d != 0 {
		t.Errorf("expected 0 active contributors, got %d", resp.Body.ActiveContributors30d)
	}
	if len(resp.Body.ContributionsPerWeek) != 0 {
		t.Errorf("expected 0 weekly entries, got %d", len(resp.Body.ContributionsPerWeek))
	}
	if len(resp.Body.TopContributors) != 0 {
		t.Errorf("expected 0 top contributors, got %d", len(resp.Body.TopContributors))
	}
}

// ============================================================================
// Tests: GetDataQualityTrendsHandler
// ============================================================================

func TestAnalyticsHandler_DataQuality_Success(t *testing.T) {
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getDataQualityTrendsFn: func(months int) (*contracts.DataQualityTrendsResponse, error) {
			return &contracts.DataQualityTrendsResponse{
				ShowsApproved:          []contracts.MonthlyCount{{Month: "2026-03", Count: 20}},
				ShowsRejected:          []contracts.MonthlyCount{{Month: "2026-03", Count: 5}},
				PendingReviewCount:     7,
				ArtistsWithoutReleases: 15,
				InactiveVenues90d:      3,
			}, nil
		},
	})

	resp, err := h.GetDataQualityTrendsHandler(analyticsAdminCtx(), &GetDataQualityTrendsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ShowsApproved[0].Count != 20 {
		t.Errorf("expected approved=20, got %d", resp.Body.ShowsApproved[0].Count)
	}
	if resp.Body.ShowsRejected[0].Count != 5 {
		t.Errorf("expected rejected=5, got %d", resp.Body.ShowsRejected[0].Count)
	}
	if resp.Body.PendingReviewCount != 7 {
		t.Errorf("expected pending=7, got %d", resp.Body.PendingReviewCount)
	}
	if resp.Body.ArtistsWithoutReleases != 15 {
		t.Errorf("expected artists_without_releases=15, got %d", resp.Body.ArtistsWithoutReleases)
	}
	if resp.Body.InactiveVenues90d != 3 {
		t.Errorf("expected inactive_venues=3, got %d", resp.Body.InactiveVenues90d)
	}
}

func TestAnalyticsHandler_DataQuality_ServiceError(t *testing.T) {
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getDataQualityTrendsFn: func(months int) (*contracts.DataQualityTrendsResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetDataQualityTrendsHandler(analyticsAdminCtx(), &GetDataQualityTrendsRequest{})
	assertHumaError(t, err, 500)
}

func TestAnalyticsHandler_DataQuality_DefaultMonths(t *testing.T) {
	var receivedMonths int
	h := NewAnalyticsHandler(&mockAnalyticsService{
		getDataQualityTrendsFn: func(months int) (*contracts.DataQualityTrendsResponse, error) {
			receivedMonths = months
			return &contracts.DataQualityTrendsResponse{
				ShowsApproved: []contracts.MonthlyCount{},
				ShowsRejected: []contracts.MonthlyCount{},
			}, nil
		},
	})

	_, err := h.GetDataQualityTrendsHandler(analyticsAdminCtx(), &GetDataQualityTrendsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMonths != 6 {
		t.Errorf("expected default months=6, got %d", receivedMonths)
	}
}
