package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// ============================================================================
// Mock: PipelineServiceInterface
// ============================================================================

type mockPipelineService struct {
	extractVenueFn func(venueID uint, dryRun bool) (*services.PipelineResult, error)
}

func (m *mockPipelineService) ExtractVenue(venueID uint, dryRun bool) (*services.PipelineResult, error) {
	if m.extractVenueFn != nil {
		return m.extractVenueFn(venueID, dryRun)
	}
	return &services.PipelineResult{
		VenueID:         venueID,
		VenueName:       "Test Venue",
		RenderMethod:    "static",
		EventsExtracted: 5,
		EventsImported:  3,
		DurationMs:      1234,
		DryRun:          dryRun,
	}, nil
}

// ============================================================================
// Mock: VenueSourceConfigServiceInterface
// ============================================================================

type mockVenueSourceConfigService struct {
	getByVenueIDFn      func(venueID uint) (*models.VenueSourceConfig, error)
	createOrUpdateFn    func(config *models.VenueSourceConfig) (*models.VenueSourceConfig, error)
	updateAfterRunFn    func(venueID uint, contentHash, etag *string, eventsExtracted int) error
	incrementFailuresFn func(venueID uint) error
	recordRunFn         func(run *models.VenueExtractionRun) error
	getRecentRunsFn     func(venueID uint, limit int) ([]models.VenueExtractionRun, error)
	listConfiguredFn    func() ([]models.VenueSourceConfig, error)
}

func (m *mockVenueSourceConfigService) GetByVenueID(venueID uint) (*models.VenueSourceConfig, error) {
	if m.getByVenueIDFn != nil {
		return m.getByVenueIDFn(venueID)
	}
	return nil, nil
}
func (m *mockVenueSourceConfigService) CreateOrUpdate(config *models.VenueSourceConfig) (*models.VenueSourceConfig, error) {
	if m.createOrUpdateFn != nil {
		return m.createOrUpdateFn(config)
	}
	return config, nil
}
func (m *mockVenueSourceConfigService) UpdateAfterRun(venueID uint, contentHash, etag *string, eventsExtracted int) error {
	if m.updateAfterRunFn != nil {
		return m.updateAfterRunFn(venueID, contentHash, etag, eventsExtracted)
	}
	return nil
}
func (m *mockVenueSourceConfigService) IncrementFailures(venueID uint) error {
	if m.incrementFailuresFn != nil {
		return m.incrementFailuresFn(venueID)
	}
	return nil
}
func (m *mockVenueSourceConfigService) RecordRun(run *models.VenueExtractionRun) error {
	if m.recordRunFn != nil {
		return m.recordRunFn(run)
	}
	return nil
}
func (m *mockVenueSourceConfigService) GetRecentRuns(venueID uint, limit int) ([]models.VenueExtractionRun, error) {
	if m.getRecentRunsFn != nil {
		return m.getRecentRunsFn(venueID, limit)
	}
	return nil, nil
}
func (m *mockVenueSourceConfigService) ListConfigured() ([]models.VenueSourceConfig, error) {
	if m.listConfiguredFn != nil {
		return m.listConfiguredFn()
	}
	return nil, nil
}

// ============================================================================
// Test helpers
// ============================================================================

func testPipelineHandler() *PipelineHandler {
	return NewPipelineHandler(nil, nil)
}

func pipelineAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 1, IsAdmin: true})
}

func pipelineNonAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 2, IsAdmin: false})
}

// ============================================================================
// Tests: NewPipelineHandler
// ============================================================================

func TestNewPipelineHandler(t *testing.T) {
	h := testPipelineHandler()
	if h == nil {
		t.Fatal("expected non-nil PipelineHandler")
	}
}

// ============================================================================
// Tests: Admin Guard
// ============================================================================

func TestPipelineHandler_RequiresAdmin(t *testing.T) {
	h := testPipelineHandler()

	tests := []struct {
		name string
		fn   func(ctx context.Context) error
	}{
		{"ExtractVenue", func(ctx context.Context) error {
			_, err := h.ExtractVenueHandler(ctx, &ExtractVenueRequest{VenueID: "1"})
			return err
		}},
		{"ListPipelineVenues", func(ctx context.Context) error {
			_, err := h.ListPipelineVenuesHandler(ctx, &ListPipelineVenuesRequest{})
			return err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_NoUser", func(t *testing.T) {
			err := tc.fn(context.Background())
			assertHumaError(t, err, 403)
		})
		t.Run(tc.name+"_NonAdmin", func(t *testing.T) {
			err := tc.fn(pipelineNonAdminCtx())
			assertHumaError(t, err, 403)
		})
	}
}

// ============================================================================
// Tests: ExtractVenueHandler
// ============================================================================

func TestPipelineHandler_ExtractVenue_Success(t *testing.T) {
	h := NewPipelineHandler(
		&mockPipelineService{},
		&mockVenueSourceConfigService{},
	)

	resp, err := h.ExtractVenueHandler(pipelineAdminCtx(), &ExtractVenueRequest{VenueID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.VenueID != 1 {
		t.Errorf("expected venue_id=1, got %d", resp.Body.VenueID)
	}
	if resp.Body.EventsExtracted != 5 {
		t.Errorf("expected events_extracted=5, got %d", resp.Body.EventsExtracted)
	}
}

func TestPipelineHandler_ExtractVenue_DryRun(t *testing.T) {
	var receivedDryRun bool
	h := NewPipelineHandler(
		&mockPipelineService{
			extractVenueFn: func(venueID uint, dryRun bool) (*services.PipelineResult, error) {
				receivedDryRun = dryRun
				return &services.PipelineResult{VenueID: venueID, DryRun: dryRun, EventsExtracted: 3}, nil
			},
		},
		&mockVenueSourceConfigService{},
	)

	resp, err := h.ExtractVenueHandler(pipelineAdminCtx(), &ExtractVenueRequest{VenueID: "1", DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !receivedDryRun {
		t.Error("expected dry_run=true to be passed to service")
	}
	if !resp.Body.DryRun {
		t.Error("expected dry_run=true in response")
	}
}

func TestPipelineHandler_ExtractVenue_InvalidVenueID(t *testing.T) {
	h := NewPipelineHandler(
		&mockPipelineService{},
		&mockVenueSourceConfigService{},
	)

	_, err := h.ExtractVenueHandler(pipelineAdminCtx(), &ExtractVenueRequest{VenueID: "not-a-number"})
	assertHumaError(t, err, 400)
}

func TestPipelineHandler_ExtractVenue_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&mockPipelineService{
			extractVenueFn: func(venueID uint, dryRun bool) (*services.PipelineResult, error) {
				return nil, fmt.Errorf("venue has no calendar URL configured")
			},
		},
		&mockVenueSourceConfigService{},
	)

	_, err := h.ExtractVenueHandler(pipelineAdminCtx(), &ExtractVenueRequest{VenueID: "1"})
	assertHumaError(t, err, 422)
}

// ============================================================================
// Tests: ListPipelineVenuesHandler
// ============================================================================

func TestPipelineHandler_ListVenues_Success(t *testing.T) {
	calURL := "https://example.com/events"
	rm := "static"
	slug := "test-venue"

	h := NewPipelineHandler(
		&mockPipelineService{},
		&mockVenueSourceConfigService{
			listConfiguredFn: func() ([]models.VenueSourceConfig, error) {
				return []models.VenueSourceConfig{
					{
						ID:              1,
						VenueID:         10,
						CalendarURL:     &calURL,
						PreferredSource: "ai",
						RenderMethod:    &rm,
						Venue: models.Venue{
							ID:   10,
							Name: "Test Venue",
							Slug: &slug,
						},
					},
				}, nil
			},
			getRecentRunsFn: func(venueID uint, limit int) ([]models.VenueExtractionRun, error) {
				return []models.VenueExtractionRun{
					{ID: 1, VenueID: venueID, EventsExtracted: 5, EventsImported: 3},
				}, nil
			},
		},
	)

	resp, err := h.ListPipelineVenuesHandler(pipelineAdminCtx(), &ListPipelineVenuesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Venues) != 1 {
		t.Fatalf("expected 1 venue, got %d", len(resp.Body.Venues))
	}
	v := resp.Body.Venues[0]
	if v.VenueID != 10 {
		t.Errorf("expected venue_id=10, got %d", v.VenueID)
	}
	if v.VenueName != "Test Venue" {
		t.Errorf("expected venue_name=Test Venue, got %s", v.VenueName)
	}
	if v.VenueSlug != "test-venue" {
		t.Errorf("expected venue_slug=test-venue, got %s", v.VenueSlug)
	}
	if v.LastRun == nil {
		t.Fatal("expected last_run to be populated")
	}
	if v.LastRun.EventsExtracted != 5 {
		t.Errorf("expected last_run.events_extracted=5, got %d", v.LastRun.EventsExtracted)
	}
}

func TestPipelineHandler_ListVenues_Empty(t *testing.T) {
	h := NewPipelineHandler(
		&mockPipelineService{},
		&mockVenueSourceConfigService{
			listConfiguredFn: func() ([]models.VenueSourceConfig, error) {
				return []models.VenueSourceConfig{}, nil
			},
		},
	)

	resp, err := h.ListPipelineVenuesHandler(pipelineAdminCtx(), &ListPipelineVenuesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 0 {
		t.Errorf("expected total=0, got %d", resp.Body.Total)
	}
	if len(resp.Body.Venues) != 0 {
		t.Errorf("expected 0 venues, got %d", len(resp.Body.Venues))
	}
}

func TestPipelineHandler_ListVenues_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&mockPipelineService{},
		&mockVenueSourceConfigService{
			listConfiguredFn: func() ([]models.VenueSourceConfig, error) {
				return nil, fmt.Errorf("database error")
			},
		},
	)

	_, err := h.ListPipelineVenuesHandler(pipelineAdminCtx(), &ListPipelineVenuesRequest{})
	assertHumaError(t, err, 500)
}
