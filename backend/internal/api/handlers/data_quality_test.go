package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Mock: DataQualityServiceInterface
// ============================================================================

type mockDataQualityService struct {
	getSummaryFn      func() (*contracts.DataQualitySummary, error)
	getCategoryItemsFn func(category string, limit, offset int) ([]*contracts.DataQualityItem, int64, error)
}

func (m *mockDataQualityService) GetSummary() (*contracts.DataQualitySummary, error) {
	if m.getSummaryFn != nil {
		return m.getSummaryFn()
	}
	return &contracts.DataQualitySummary{Categories: []contracts.DataQualityCategory{}}, nil
}

func (m *mockDataQualityService) GetCategoryItems(category string, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	if m.getCategoryItemsFn != nil {
		return m.getCategoryItemsFn(category, limit, offset)
	}
	return nil, 0, nil
}

// ============================================================================
// Test helpers
// ============================================================================

func testDataQualityHandler() *DataQualityHandler {
	return NewDataQualityHandler(&mockDataQualityService{})
}

func dataQualityAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 1, IsAdmin: true})
}

func dataQualityNonAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 2, IsAdmin: false})
}

// ============================================================================
// Tests: NewDataQualityHandler
// ============================================================================

func TestNewDataQualityHandler(t *testing.T) {
	h := testDataQualityHandler()
	if h == nil {
		t.Fatal("expected non-nil DataQualityHandler")
	}
}

// ============================================================================
// Tests: Admin Guard
// ============================================================================

func TestDataQualityHandler_Summary_RequiresAdmin(t *testing.T) {
	h := testDataQualityHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.GetDataQualitySummaryHandler(context.Background(), &GetDataQualitySummaryRequest{})
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.GetDataQualitySummaryHandler(dataQualityNonAdminCtx(), &GetDataQualitySummaryRequest{})
		assertHumaError(t, err, 403)
	})
}

func TestDataQualityHandler_Category_RequiresAdmin(t *testing.T) {
	h := testDataQualityHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.GetDataQualityCategoryHandler(context.Background(), &GetDataQualityCategoryRequest{Category: "artists_missing_links"})
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.GetDataQualityCategoryHandler(dataQualityNonAdminCtx(), &GetDataQualityCategoryRequest{Category: "artists_missing_links"})
		assertHumaError(t, err, 403)
	})
}

// ============================================================================
// Tests: GetDataQualitySummaryHandler
// ============================================================================

func TestDataQualityHandler_Summary_Success(t *testing.T) {
	h := NewDataQualityHandler(&mockDataQualityService{
		getSummaryFn: func() (*contracts.DataQualitySummary, error) {
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

	resp, err := h.GetDataQualitySummaryHandler(dataQualityAdminCtx(), &GetDataQualitySummaryRequest{})
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

func TestDataQualityHandler_Summary_ServiceError(t *testing.T) {
	h := NewDataQualityHandler(&mockDataQualityService{
		getSummaryFn: func() (*contracts.DataQualitySummary, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetDataQualitySummaryHandler(dataQualityAdminCtx(), &GetDataQualitySummaryRequest{})
	assertHumaError(t, err, 500)
}

func TestDataQualityHandler_Summary_Empty(t *testing.T) {
	h := NewDataQualityHandler(&mockDataQualityService{
		getSummaryFn: func() (*contracts.DataQualitySummary, error) {
			return &contracts.DataQualitySummary{
				Categories: []contracts.DataQualityCategory{},
				TotalItems: 0,
			}, nil
		},
	})

	resp, err := h.GetDataQualitySummaryHandler(dataQualityAdminCtx(), &GetDataQualitySummaryRequest{})
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
// Tests: GetDataQualityCategoryHandler
// ============================================================================

func TestDataQualityHandler_Category_Success(t *testing.T) {
	h := NewDataQualityHandler(&mockDataQualityService{
		getCategoryItemsFn: func(category string, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
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

	resp, err := h.GetDataQualityCategoryHandler(dataQualityAdminCtx(), &GetDataQualityCategoryRequest{
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

func TestDataQualityHandler_Category_InvalidCategory(t *testing.T) {
	h := NewDataQualityHandler(&mockDataQualityService{
		getCategoryItemsFn: func(category string, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			return nil, 0, fmt.Errorf("unknown category: %s", category)
		},
	})

	_, err := h.GetDataQualityCategoryHandler(dataQualityAdminCtx(), &GetDataQualityCategoryRequest{
		Category: "nonexistent",
	})
	assertHumaError(t, err, 400)
}

func TestDataQualityHandler_Category_ServiceError(t *testing.T) {
	h := NewDataQualityHandler(&mockDataQualityService{
		getCategoryItemsFn: func(category string, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			return nil, 0, fmt.Errorf("database error")
		},
	})

	_, err := h.GetDataQualityCategoryHandler(dataQualityAdminCtx(), &GetDataQualityCategoryRequest{
		Category: "artists_missing_links",
	})
	assertHumaError(t, err, 500)
}

func TestDataQualityHandler_Category_DefaultLimit(t *testing.T) {
	var receivedLimit int
	h := NewDataQualityHandler(&mockDataQualityService{
		getCategoryItemsFn: func(category string, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			receivedLimit = limit
			return nil, 0, nil
		},
	})

	_, err := h.GetDataQualityCategoryHandler(dataQualityAdminCtx(), &GetDataQualityCategoryRequest{
		Category: "artists_missing_links",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 50 {
		t.Errorf("expected default limit=50, got %d", receivedLimit)
	}
}

func TestDataQualityHandler_Category_CustomLimit(t *testing.T) {
	var receivedLimit, receivedOffset int
	h := NewDataQualityHandler(&mockDataQualityService{
		getCategoryItemsFn: func(category string, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			receivedLimit = limit
			receivedOffset = offset
			return nil, 0, nil
		},
	})

	_, err := h.GetDataQualityCategoryHandler(dataQualityAdminCtx(), &GetDataQualityCategoryRequest{
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

func TestDataQualityHandler_Category_Empty(t *testing.T) {
	h := NewDataQualityHandler(&mockDataQualityService{
		getCategoryItemsFn: func(category string, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
			return []*contracts.DataQualityItem{}, 0, nil
		},
	})

	resp, err := h.GetDataQualityCategoryHandler(dataQualityAdminCtx(), &GetDataQualityCategoryRequest{
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
