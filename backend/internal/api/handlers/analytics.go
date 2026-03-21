package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/services/contracts"
)

// AnalyticsHandler handles platform analytics dashboard endpoints.
type AnalyticsHandler struct {
	analyticsService services.AnalyticsServiceInterface
}

// NewAnalyticsHandler creates a new analytics handler.
func NewAnalyticsHandler(
	analyticsService services.AnalyticsServiceInterface,
) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsService: analyticsService,
	}
}

// --- GetGrowthMetrics ---

// GetGrowthMetricsRequest is the Huma request for GET /admin/analytics/growth
type GetGrowthMetricsRequest struct {
	Months int `query:"months" required:"false" doc:"Number of months (default 6, max 24)"`
}

// MonthlyCountResponse is a single month+count pair.
type MonthlyCountResponse struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

// GetGrowthMetricsResponse is the Huma response for GET /admin/analytics/growth
type GetGrowthMetricsResponse struct {
	Body struct {
		Shows    []MonthlyCountResponse `json:"shows"`
		Artists  []MonthlyCountResponse `json:"artists"`
		Venues   []MonthlyCountResponse `json:"venues"`
		Releases []MonthlyCountResponse `json:"releases"`
		Labels   []MonthlyCountResponse `json:"labels"`
		Users    []MonthlyCountResponse `json:"users"`
	}
}

// GetGrowthMetricsHandler handles GET /admin/analytics/growth
func (h *AnalyticsHandler) GetGrowthMetricsHandler(ctx context.Context, req *GetGrowthMetricsRequest) (*GetGrowthMetricsResponse, error) {
	_, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	months := req.Months
	if months <= 0 {
		months = 6
	}

	data, err := h.analyticsService.GetGrowthMetrics(months)
	if err != nil {
		logger.FromContext(ctx).Error("analytics_growth_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get growth metrics")
	}

	resp := &GetGrowthMetricsResponse{}
	resp.Body.Shows = toMonthlyCountResponse(data.Shows)
	resp.Body.Artists = toMonthlyCountResponse(data.Artists)
	resp.Body.Venues = toMonthlyCountResponse(data.Venues)
	resp.Body.Releases = toMonthlyCountResponse(data.Releases)
	resp.Body.Labels = toMonthlyCountResponse(data.Labels)
	resp.Body.Users = toMonthlyCountResponse(data.Users)
	return resp, nil
}

// --- GetEngagementMetrics ---

// GetEngagementMetricsRequest is the Huma request for GET /admin/analytics/engagement
type GetEngagementMetricsRequest struct {
	Months int `query:"months" required:"false" doc:"Number of months (default 6, max 24)"`
}

// EngagementMetricResponse is a single month+count pair for engagement.
type EngagementMetricResponse struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

// GetEngagementMetricsResponse is the Huma response for GET /admin/analytics/engagement
type GetEngagementMetricsResponse struct {
	Body struct {
		Bookmarks       []EngagementMetricResponse `json:"bookmarks"`
		TagsAdded       []EngagementMetricResponse `json:"tags_added"`
		TagVotes        []EngagementMetricResponse `json:"tag_votes"`
		CollectionItems []EngagementMetricResponse `json:"crate_items"`
		Requests        []EngagementMetricResponse `json:"requests"`
		RequestVotes    []EngagementMetricResponse `json:"request_votes"`
		Revisions       []EngagementMetricResponse `json:"revisions"`
		Follows         []EngagementMetricResponse `json:"follows"`
		Attendance      []EngagementMetricResponse `json:"attendance"`
	}
}

// GetEngagementMetricsHandler handles GET /admin/analytics/engagement
func (h *AnalyticsHandler) GetEngagementMetricsHandler(ctx context.Context, req *GetEngagementMetricsRequest) (*GetEngagementMetricsResponse, error) {
	_, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	months := req.Months
	if months <= 0 {
		months = 6
	}

	data, err := h.analyticsService.GetEngagementMetrics(months)
	if err != nil {
		logger.FromContext(ctx).Error("analytics_engagement_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get engagement metrics")
	}

	resp := &GetEngagementMetricsResponse{}
	resp.Body.Bookmarks = toEngagementMetricResponse(data.Bookmarks)
	resp.Body.TagsAdded = toEngagementMetricResponse(data.TagsAdded)
	resp.Body.TagVotes = toEngagementMetricResponse(data.TagVotes)
	resp.Body.CollectionItems = toEngagementMetricResponse(data.CollectionItems)
	resp.Body.Requests = toEngagementMetricResponse(data.Requests)
	resp.Body.RequestVotes = toEngagementMetricResponse(data.RequestVotes)
	resp.Body.Revisions = toEngagementMetricResponse(data.Revisions)
	resp.Body.Follows = toEngagementMetricResponse(data.Follows)
	resp.Body.Attendance = toEngagementMetricResponse(data.Attendance)
	return resp, nil
}

// --- GetCommunityHealth ---

// GetCommunityHealthRequest is the Huma request for GET /admin/analytics/community
type GetCommunityHealthRequest struct{}

// TopContributorResponse represents a top contributor in the response.
type TopContributorResponse struct {
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	Count       int    `json:"count"`
}

// WeeklyContributionsResponse represents weekly contributions in the response.
type WeeklyContributionsResponse struct {
	Week  string `json:"week"`
	Count int    `json:"count"`
}

// GetCommunityHealthResponse is the Huma response for GET /admin/analytics/community
type GetCommunityHealthResponse struct {
	Body struct {
		ActiveContributors30d  int                           `json:"active_contributors_30d"`
		ContributionsPerWeek   []WeeklyContributionsResponse `json:"contributions_per_week"`
		RequestFulfillmentRate float64                       `json:"request_fulfillment_rate"`
		NewCollections30d      int                           `json:"new_crates_30d"`
		TopContributors        []TopContributorResponse      `json:"top_contributors"`
	}
}

// GetCommunityHealthHandler handles GET /admin/analytics/community
func (h *AnalyticsHandler) GetCommunityHealthHandler(ctx context.Context, _ *GetCommunityHealthRequest) (*GetCommunityHealthResponse, error) {
	_, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	data, err := h.analyticsService.GetCommunityHealth()
	if err != nil {
		logger.FromContext(ctx).Error("analytics_community_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get community health metrics")
	}

	resp := &GetCommunityHealthResponse{}
	resp.Body.ActiveContributors30d = data.ActiveContributors30d
	resp.Body.RequestFulfillmentRate = data.RequestFulfillmentRate
	resp.Body.NewCollections30d = data.NewCollections30d

	contribs := make([]WeeklyContributionsResponse, 0, len(data.ContributionsPerWeek))
	for _, c := range data.ContributionsPerWeek {
		contribs = append(contribs, WeeklyContributionsResponse{
			Week:  c.Week,
			Count: c.Count,
		})
	}
	resp.Body.ContributionsPerWeek = contribs

	tops := make([]TopContributorResponse, 0, len(data.TopContributors))
	for _, t := range data.TopContributors {
		tops = append(tops, TopContributorResponse{
			UserID:      t.UserID,
			Username:    t.Username,
			DisplayName: t.DisplayName,
			Count:       t.Count,
		})
	}
	resp.Body.TopContributors = tops
	return resp, nil
}

// --- GetDataQualityTrends ---

// GetDataQualityTrendsRequest is the Huma request for GET /admin/analytics/data-quality
type GetDataQualityTrendsRequest struct {
	Months int `query:"months" required:"false" doc:"Number of months (default 6, max 24)"`
}

// GetDataQualityTrendsResponse is the Huma response for GET /admin/analytics/data-quality
type GetDataQualityTrendsResponse struct {
	Body struct {
		ShowsApproved          []MonthlyCountResponse `json:"shows_approved"`
		ShowsRejected          []MonthlyCountResponse `json:"shows_rejected"`
		PendingReviewCount     int                    `json:"pending_review_count"`
		ArtistsWithoutReleases int                    `json:"artists_without_releases"`
		InactiveVenues90d      int                    `json:"inactive_venues_90d"`
	}
}

// GetDataQualityTrendsHandler handles GET /admin/analytics/data-quality
func (h *AnalyticsHandler) GetDataQualityTrendsHandler(ctx context.Context, req *GetDataQualityTrendsRequest) (*GetDataQualityTrendsResponse, error) {
	_, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	months := req.Months
	if months <= 0 {
		months = 6
	}

	data, err := h.analyticsService.GetDataQualityTrends(months)
	if err != nil {
		logger.FromContext(ctx).Error("analytics_data_quality_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get data quality trends")
	}

	resp := &GetDataQualityTrendsResponse{}
	resp.Body.ShowsApproved = toMonthlyCountResponse(data.ShowsApproved)
	resp.Body.ShowsRejected = toMonthlyCountResponse(data.ShowsRejected)
	resp.Body.PendingReviewCount = data.PendingReviewCount
	resp.Body.ArtistsWithoutReleases = data.ArtistsWithoutReleases
	resp.Body.InactiveVenues90d = data.InactiveVenues90d
	return resp, nil
}

// --- Response mapping helpers ---

func toMonthlyCountResponse(src []contracts.MonthlyCount) []MonthlyCountResponse {
	result := make([]MonthlyCountResponse, len(src))
	for i, s := range src {
		result[i] = MonthlyCountResponse{Month: s.Month, Count: s.Count}
	}
	return result
}

func toEngagementMetricResponse(src []contracts.EngagementMetric) []EngagementMetricResponse {
	result := make([]EngagementMetricResponse, len(src))
	for i, s := range src {
		result[i] = EngagementMetricResponse{Month: s.Month, Count: s.Count}
	}
	return result
}
