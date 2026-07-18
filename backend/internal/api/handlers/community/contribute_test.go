package community

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

func testContributeHandler() *ContributeHandler {
	return NewContributeHandler(&testhelpers.MockDataQualityService{})
}

// ============================================================================
// Tests: GetOpportunitiesHandler (public — no auth check)
// ============================================================================

func TestContributeHandler_Opportunities_Success(t *testing.T) {
	h := NewContributeHandler(&testhelpers.MockDataQualityService{
		GetContributeSummaryFn: func(viewerID *uint) (*contracts.DataQualitySummary, error) {
			return &contracts.DataQualitySummary{
				Categories: []contracts.DataQualityCategory{
					{
						Key:         "artists_missing_links",
						Label:       "Artists Missing Links",
						EntityType:  "artist",
						Count:       5,
						Description: "Artists with no social links",
					},
					{
						Key:         "shows_missing_price",
						Label:       "Shows Missing Price",
						EntityType:  "show",
						Count:       3,
						Description: "Upcoming shows with no price",
					},
				},
				TotalItems: 8,
			}, nil
		},
	})

	resp, err := h.GetOpportunitiesHandler(context.Background(), &GetOpportunitiesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.TotalItems != 8 {
		t.Errorf("expected total_items=8, got %d", resp.Body.TotalItems)
	}
	if len(resp.Body.Categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(resp.Body.Categories))
	}
	if resp.Body.Categories[0].Key != "artists_missing_links" {
		t.Errorf("expected key=artists_missing_links, got %s", resp.Body.Categories[0].Key)
	}
	if resp.Body.Categories[0].Count != 5 {
		t.Errorf("expected count=5, got %d", resp.Body.Categories[0].Count)
	}
}

func TestContributeHandler_Opportunities_NoAuthRequired(t *testing.T) {
	h := testContributeHandler()

	// Should succeed without any user context (public endpoint)
	resp, err := h.GetOpportunitiesHandler(context.Background(), &GetOpportunitiesRequest{})
	if err != nil {
		t.Fatalf("expected no error for public endpoint, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestContributeHandler_Opportunities_ServiceError(t *testing.T) {
	h := NewContributeHandler(&testhelpers.MockDataQualityService{
		GetContributeSummaryFn: func(viewerID *uint) (*contracts.DataQualitySummary, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetOpportunitiesHandler(context.Background(), &GetOpportunitiesRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestContributeHandler_Opportunities_Empty(t *testing.T) {
	h := NewContributeHandler(&testhelpers.MockDataQualityService{
		GetContributeSummaryFn: func(viewerID *uint) (*contracts.DataQualitySummary, error) {
			return &contracts.DataQualitySummary{
				Categories: []contracts.DataQualityCategory{},
				TotalItems: 0,
			}, nil
		},
	})

	resp, err := h.GetOpportunitiesHandler(context.Background(), &GetOpportunitiesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.TotalItems != 0 {
		t.Errorf("expected total_items=0, got %d", resp.Body.TotalItems)
	}
	if len(resp.Body.Categories) != 0 {
		t.Fatalf("expected 0 categories, got %d", len(resp.Body.Categories))
	}
}

// ============================================================================
// Tests: GetOpportunityCategoryHandler (public — no auth check)
// ============================================================================

func TestContributeHandler_Category_Success(t *testing.T) {
	h := NewContributeHandler(&testhelpers.MockDataQualityService{
		GetContributeCategoryItemsFn: func(category string, viewerID *uint, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			if category != "artists_missing_links" {
				t.Errorf("expected category=artists_missing_links, got %s", category)
			}
			return []*contracts.DataQualityItem{
				{
					EntityType: "artist",
					EntityID:   1,
					Name:       "Band A",
					Slug:       "band-a",
					Reason:     "No social links or website",
					ShowCount:  10,
				},
				{
					EntityType: "artist",
					EntityID:   2,
					Name:       "Band B",
					Slug:       "band-b",
					Reason:     "No social links or website",
					ShowCount:  5,
				},
			}, 2, nil
		},
	})

	resp, err := h.GetOpportunityCategoryHandler(context.Background(), &GetOpportunityCategoryRequest{
		Category: "artists_missing_links",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Body.Total)
	}
	if len(resp.Body.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Body.Items))
	}
	if resp.Body.Items[0].Name != "Band A" {
		t.Errorf("expected name=Band A, got %s", resp.Body.Items[0].Name)
	}
	if resp.Body.Items[0].ShowCount != 10 {
		t.Errorf("expected show_count=10, got %d", resp.Body.Items[0].ShowCount)
	}
}

func TestContributeHandler_Category_NoAuthRequired(t *testing.T) {
	h := testContributeHandler()

	// Should succeed without any user context (public endpoint)
	resp, err := h.GetOpportunityCategoryHandler(context.Background(), &GetOpportunityCategoryRequest{
		Category: "artists_missing_links",
	})
	if err != nil {
		t.Fatalf("expected no error for public endpoint, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestContributeHandler_Category_InvalidCategory(t *testing.T) {
	h := NewContributeHandler(&testhelpers.MockDataQualityService{
		GetContributeCategoryItemsFn: func(category string, viewerID *uint, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			return nil, 0, apperrors.ErrDataQualityUnknownCategory(category)
		},
	})

	_, err := h.GetOpportunityCategoryHandler(context.Background(), &GetOpportunityCategoryRequest{
		Category: "nonexistent",
	})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestContributeHandler_Category_ServiceError(t *testing.T) {
	h := NewContributeHandler(&testhelpers.MockDataQualityService{
		GetContributeCategoryItemsFn: func(category string, viewerID *uint, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			return nil, 0, fmt.Errorf("database error")
		},
	})

	_, err := h.GetOpportunityCategoryHandler(context.Background(), &GetOpportunityCategoryRequest{
		Category: "artists_missing_links",
	})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestContributeHandler_Category_DefaultLimit(t *testing.T) {
	var receivedLimit int
	h := NewContributeHandler(&testhelpers.MockDataQualityService{
		GetContributeCategoryItemsFn: func(category string, viewerID *uint, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			receivedLimit = limit
			return nil, 0, nil
		},
	})

	_, err := h.GetOpportunityCategoryHandler(context.Background(), &GetOpportunityCategoryRequest{
		Category: "artists_missing_links",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 20 {
		t.Errorf("expected default limit=20, got %d", receivedLimit)
	}
}

func TestContributeHandler_Category_CustomLimit(t *testing.T) {
	var receivedLimit, receivedOffset int
	h := NewContributeHandler(&testhelpers.MockDataQualityService{
		GetContributeCategoryItemsFn: func(category string, viewerID *uint, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			receivedLimit = limit
			receivedOffset = offset
			return nil, 0, nil
		},
	})

	_, err := h.GetOpportunityCategoryHandler(context.Background(), &GetOpportunityCategoryRequest{
		Category: "venues_missing_social",
		Limit:    25,
		Offset:   10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 25 {
		t.Errorf("expected limit=25, got %d", receivedLimit)
	}
	if receivedOffset != 10 {
		t.Errorf("expected offset=10, got %d", receivedOffset)
	}
}

func TestContributeHandler_Category_Empty(t *testing.T) {
	h := NewContributeHandler(&testhelpers.MockDataQualityService{
		GetContributeCategoryItemsFn: func(category string, viewerID *uint, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			return []*contracts.DataQualityItem{}, 0, nil
		},
	})

	resp, err := h.GetOpportunityCategoryHandler(context.Background(), &GetOpportunityCategoryRequest{
		Category: "artists_missing_links",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 0 {
		t.Errorf("expected total=0, got %d", resp.Body.Total)
	}
	if len(resp.Body.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(resp.Body.Items))
	}
}

// ============================================================================
// Tests: viewer-id threading (Loose Ends categories, PSY-1483)
// ============================================================================

func TestContributeHandler_Opportunities_ViewerIDFromContext(t *testing.T) {
	var seen *uint
	mock := &testhelpers.MockDataQualityService{
		GetContributeSummaryFn: func(viewerID *uint) (*contracts.DataQualitySummary, error) {
			seen = viewerID
			return &contracts.DataQualitySummary{}, nil
		},
	}
	h := NewContributeHandler(mock)

	// Anonymous: nil viewer id passed through.
	if _, err := h.GetOpportunitiesHandler(context.Background(), &GetOpportunitiesRequest{}); err != nil {
		t.Fatalf("unexpected error (anon): %v", err)
	}
	if seen != nil {
		t.Errorf("expected nil viewer id for anonymous request, got %d", *seen)
	}

	// Authenticated: user id threaded through from context.
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 42})
	if _, err := h.GetOpportunitiesHandler(ctx, &GetOpportunitiesRequest{}); err != nil {
		t.Fatalf("unexpected error (authed): %v", err)
	}
	if seen == nil {
		t.Fatal("expected non-nil viewer id for authenticated request")
	}
	if *seen != 42 {
		t.Errorf("expected viewer id 42, got %d", *seen)
	}
}

func TestContributeHandler_Category_ViewerIDFromContext(t *testing.T) {
	var seen *uint
	mock := &testhelpers.MockDataQualityService{
		GetContributeCategoryItemsFn: func(category string, viewerID *uint, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			seen = viewerID
			return []*contracts.DataQualityItem{}, 0, nil
		},
	}
	h := NewContributeHandler(mock)

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7})
	if _, err := h.GetOpportunityCategoryHandler(ctx, &GetOpportunityCategoryRequest{
		Category: "followed_artists_missing_links",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen == nil || *seen != 7 {
		t.Fatalf("expected viewer id 7 threaded to service, got %v", seen)
	}
}
