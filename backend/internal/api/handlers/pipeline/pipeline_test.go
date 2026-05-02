package pipeline

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

func testPipelineHandler() *PipelineHandler {
	return NewPipelineHandler(nil, nil, nil)
}

func pipelineAdminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
}

func pipelineNonAdminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 2, IsAdmin: false})
}

// ============================================================================
// Tests: Admin Guard
// ============================================================================

// ============================================================================
// Tests: ExtractVenueHandler
// ============================================================================

func TestPipelineHandler_ExtractVenue_Success(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{
			ExtractVenueFn: func(venueID uint, dryRun bool) (*contracts.PipelineResult, error) {
				if venueID != 25 {
					t.Errorf("expected venueID=25, got %d", venueID)
				}
				if dryRun {
					t.Errorf("expected dryRun=false, got true")
				}
				return &contracts.PipelineResult{
					VenueID:         venueID,
					VenueName:       "Test Venue",
					RenderMethod:    "static",
					EventsExtracted: 5,
					EventsImported:  3,
					DurationMs:      1234,
					DryRun:          dryRun,
				}, nil
			},
		},
		&testhelpers.MockVenueSourceConfigService{},
		&testhelpers.MockEnrichmentService{},
	)

	resp, err := h.ExtractVenueHandler(pipelineAdminCtx(), &ExtractVenueRequest{VenueID: "25"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.VenueID != 25 {
		t.Errorf("expected venue_id=25, got %d", resp.Body.VenueID)
	}
	if resp.Body.EventsExtracted != 5 {
		t.Errorf("expected events_extracted=5, got %d", resp.Body.EventsExtracted)
	}
}

func TestPipelineHandler_ExtractVenue_DryRun(t *testing.T) {
	var receivedDryRun bool
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{
			ExtractVenueFn: func(venueID uint, dryRun bool) (*contracts.PipelineResult, error) {
				receivedDryRun = dryRun
				return &contracts.PipelineResult{VenueID: venueID, DryRun: dryRun, EventsExtracted: 3}, nil
			},
		},
		&testhelpers.MockVenueSourceConfigService{},
		&testhelpers.MockEnrichmentService{},
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
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{},
		&testhelpers.MockEnrichmentService{},
	)

	_, err := h.ExtractVenueHandler(pipelineAdminCtx(), &ExtractVenueRequest{VenueID: "not-a-number"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestPipelineHandler_ExtractVenue_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{
			ExtractVenueFn: func(venueID uint, dryRun bool) (*contracts.PipelineResult, error) {
				return nil, fmt.Errorf("venue has no calendar URL configured")
			},
		},
		&testhelpers.MockVenueSourceConfigService{},
		&testhelpers.MockEnrichmentService{},
	)

	_, err := h.ExtractVenueHandler(pipelineAdminCtx(), &ExtractVenueRequest{VenueID: "1"})
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Tests: ListPipelineVenuesHandler
// ============================================================================

func TestPipelineHandler_ListVenues_Success(t *testing.T) {
	calURL := "https://example.com/events"
	rm := "static"
	slug := "test-venue"

	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			ListConfiguredFn: func() ([]adminm.VenueSourceConfig, error) {
				return []adminm.VenueSourceConfig{
					{
						ID:              1,
						VenueID:         10,
						CalendarURL:     &calURL,
						PreferredSource: "ai",
						RenderMethod:    &rm,
						Venue: catalogm.Venue{
							ID:   10,
							Name: "Test Venue",
							Slug: &slug,
						},
					},
				}, nil
			},
			GetRecentRunsFn: func(venueID uint, limit int) ([]adminm.VenueExtractionRun, error) {
				return []adminm.VenueExtractionRun{
					{ID: 1, VenueID: venueID, EventsExtracted: 5, EventsImported: 3},
				}, nil
			},
		},
		&testhelpers.MockEnrichmentService{},
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
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			ListConfiguredFn: func() ([]adminm.VenueSourceConfig, error) {
				return []adminm.VenueSourceConfig{}, nil
			},
		},
		&testhelpers.MockEnrichmentService{},
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
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			ListConfiguredFn: func() ([]adminm.VenueSourceConfig, error) {
				return nil, fmt.Errorf("database error")
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	_, err := h.ListPipelineVenuesHandler(pipelineAdminCtx(), &ListPipelineVenuesRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: VenueRejectionStatsHandler
// ============================================================================

func TestPipelineHandler_RejectionStats_Success(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			GetRejectionStatsFn: func(venueID uint) (*contracts.VenueRejectionStats, error) {
				return &contracts.VenueRejectionStats{
					TotalExtracted:       100,
					Approved:             85,
					Rejected:             10,
					Pending:              5,
					RejectionBreakdown:   map[string]int64{"non_music": 7, "duplicate": 3},
					ApprovalRate:         0.8947,
					SuggestedAutoApprove: false,
				}, nil
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	resp, err := h.VenueRejectionStatsHandler(pipelineAdminCtx(), &VenueRejectionStatsRequest{VenueID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.TotalExtracted != 100 {
		t.Errorf("expected total_extracted=100, got %d", resp.Body.TotalExtracted)
	}
	if resp.Body.Approved != 85 {
		t.Errorf("expected approved=85, got %d", resp.Body.Approved)
	}
	if resp.Body.RejectionBreakdown["non_music"] != 7 {
		t.Errorf("expected non_music=7, got %d", resp.Body.RejectionBreakdown["non_music"])
	}
}

func TestPipelineHandler_RejectionStats_InvalidVenueID(t *testing.T) {
	h := NewPipelineHandler(&testhelpers.MockPipelineService{}, &testhelpers.MockVenueSourceConfigService{}, &testhelpers.MockEnrichmentService{})

	_, err := h.VenueRejectionStatsHandler(pipelineAdminCtx(), &VenueRejectionStatsRequest{VenueID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestPipelineHandler_RejectionStats_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			GetRejectionStatsFn: func(venueID uint) (*contracts.VenueRejectionStats, error) {
				return nil, fmt.Errorf("venue not found")
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	_, err := h.VenueRejectionStatsHandler(pipelineAdminCtx(), &VenueRejectionStatsRequest{VenueID: "999"})
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Tests: UpdateExtractionNotesHandler
// ============================================================================

func TestPipelineHandler_UpdateNotes_Success(t *testing.T) {
	var receivedVenueID uint
	var receivedNotes *string
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			UpdateExtractionNotesFn: func(venueID uint, notes *string) error {
				receivedVenueID = venueID
				receivedNotes = notes
				return nil
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	notes := "Skip karaoke Tuesdays and trivia Wednesdays"
	req := &UpdateExtractionNotesRequest{VenueID: "10"}
	req.Body.ExtractionNotes = &notes

	resp, err := h.UpdateExtractionNotesHandler(pipelineAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if receivedVenueID != 10 {
		t.Errorf("expected venueID=10, got %d", receivedVenueID)
	}
	if receivedNotes == nil || *receivedNotes != notes {
		t.Errorf("expected notes=%q, got %v", notes, receivedNotes)
	}
}

func TestPipelineHandler_UpdateNotes_ClearNotes(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{},
		&testhelpers.MockEnrichmentService{},
	)

	req := &UpdateExtractionNotesRequest{VenueID: "10"}
	req.Body.ExtractionNotes = nil

	resp, err := h.UpdateExtractionNotesHandler(pipelineAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestPipelineHandler_UpdateNotes_InvalidVenueID(t *testing.T) {
	h := NewPipelineHandler(&testhelpers.MockPipelineService{}, &testhelpers.MockVenueSourceConfigService{}, &testhelpers.MockEnrichmentService{})

	_, err := h.UpdateExtractionNotesHandler(pipelineAdminCtx(), &UpdateExtractionNotesRequest{VenueID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestPipelineHandler_UpdateNotes_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			UpdateExtractionNotesFn: func(venueID uint, notes *string) error {
				return fmt.Errorf("venue source config not found for venue 999")
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	req := &UpdateExtractionNotesRequest{VenueID: "999"}
	notes := "some notes"
	req.Body.ExtractionNotes = &notes

	_, err := h.UpdateExtractionNotesHandler(pipelineAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Tests: UpdateVenueConfigHandler
// ============================================================================

func TestPipelineHandler_UpdateConfig_Success(t *testing.T) {
	calURL := "https://example.com/calendar"
	slug := "test-venue"
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			CreateOrUpdateFn: func(config *adminm.VenueSourceConfig) (*adminm.VenueSourceConfig, error) {
				config.Venue = catalogm.Venue{ID: config.VenueID, Name: "Test Venue", Slug: &slug}
				return config, nil
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	req := &UpdateVenueConfigRequest{VenueID: "10"}
	req.Body.CalendarURL = &calURL
	req.Body.PreferredSource = "ai"
	req.Body.AutoApprove = true
	req.Body.StrategyLocked = false

	resp, err := h.UpdateVenueConfigHandler(pipelineAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.VenueID != 10 {
		t.Errorf("expected venue_id=10, got %d", resp.Body.VenueID)
	}
	if resp.Body.VenueName != "Test Venue" {
		t.Errorf("expected venue_name=Test Venue, got %s", resp.Body.VenueName)
	}
	if !resp.Body.AutoApprove {
		t.Error("expected auto_approve=true")
	}
}

func TestPipelineHandler_UpdateConfig_InvalidVenueID(t *testing.T) {
	h := NewPipelineHandler(&testhelpers.MockPipelineService{}, &testhelpers.MockVenueSourceConfigService{}, &testhelpers.MockEnrichmentService{})

	_, err := h.UpdateVenueConfigHandler(pipelineAdminCtx(), &UpdateVenueConfigRequest{VenueID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestPipelineHandler_UpdateConfig_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			CreateOrUpdateFn: func(config *adminm.VenueSourceConfig) (*adminm.VenueSourceConfig, error) {
				return nil, fmt.Errorf("database error")
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	req := &UpdateVenueConfigRequest{VenueID: "10"}
	req.Body.PreferredSource = "ai"

	_, err := h.UpdateVenueConfigHandler(pipelineAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Tests: GetVenueRunsHandler
// ============================================================================

func TestPipelineHandler_GetVenueRuns_Success(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			GetRecentRunsFn: func(venueID uint, limit int) ([]adminm.VenueExtractionRun, error) {
				return []adminm.VenueExtractionRun{
					{ID: 1, VenueID: venueID, EventsExtracted: 10, EventsImported: 8},
					{ID: 2, VenueID: venueID, EventsExtracted: 5, EventsImported: 3},
				}, nil
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	resp, err := h.GetVenueRunsHandler(pipelineAdminCtx(), &GetVenueRunsRequest{VenueID: "10", Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Body.Total)
	}
	if len(resp.Body.Runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(resp.Body.Runs))
	}
	if resp.Body.Runs[0].EventsExtracted != 10 {
		t.Errorf("expected first run events_extracted=10, got %d", resp.Body.Runs[0].EventsExtracted)
	}
}

func TestPipelineHandler_GetVenueRuns_DefaultLimit(t *testing.T) {
	var receivedLimit int
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			GetRecentRunsFn: func(venueID uint, limit int) ([]adminm.VenueExtractionRun, error) {
				receivedLimit = limit
				return nil, nil
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	_, err := h.GetVenueRunsHandler(pipelineAdminCtx(), &GetVenueRunsRequest{VenueID: "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 10 {
		t.Errorf("expected default limit=10, got %d", receivedLimit)
	}
}

func TestPipelineHandler_GetVenueRuns_InvalidVenueID(t *testing.T) {
	h := NewPipelineHandler(&testhelpers.MockPipelineService{}, &testhelpers.MockVenueSourceConfigService{}, &testhelpers.MockEnrichmentService{})

	_, err := h.GetVenueRunsHandler(pipelineAdminCtx(), &GetVenueRunsRequest{VenueID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestPipelineHandler_GetVenueRuns_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			GetRecentRunsFn: func(venueID uint, limit int) ([]adminm.VenueExtractionRun, error) {
				return nil, fmt.Errorf("database error")
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	_, err := h.GetVenueRunsHandler(pipelineAdminCtx(), &GetVenueRunsRequest{VenueID: "10"})
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Tests: ResetRenderMethodHandler
// ============================================================================

func TestPipelineHandler_ResetRenderMethod_Success(t *testing.T) {
	var receivedVenueID uint
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			ResetRenderMethodFn: func(venueID uint) error {
				receivedVenueID = venueID
				return nil
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	resp, err := h.ResetRenderMethodHandler(pipelineAdminCtx(), &ResetRenderMethodRequest{VenueID: "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if receivedVenueID != 10 {
		t.Errorf("expected venueID=10, got %d", receivedVenueID)
	}
}

func TestPipelineHandler_ResetRenderMethod_InvalidVenueID(t *testing.T) {
	h := NewPipelineHandler(&testhelpers.MockPipelineService{}, &testhelpers.MockVenueSourceConfigService{}, &testhelpers.MockEnrichmentService{})

	_, err := h.ResetRenderMethodHandler(pipelineAdminCtx(), &ResetRenderMethodRequest{VenueID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestPipelineHandler_ResetRenderMethod_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			ResetRenderMethodFn: func(venueID uint) error {
				return fmt.Errorf("venue source config not found for venue 999")
			},
		},
		&testhelpers.MockEnrichmentService{},
	)

	_, err := h.ResetRenderMethodHandler(pipelineAdminCtx(), &ResetRenderMethodRequest{VenueID: "999"})
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Tests: GetImportHistoryHandler
// ============================================================================

func TestPipelineHandler_GetImportHistory_Success(t *testing.T) {
	rm := "dynamic"
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			GetAllRecentRunsFn: func(limit, offset int) ([]contracts.ImportHistoryEntry, int64, error) {
				return []contracts.ImportHistoryEntry{
					{
						ID:              1,
						VenueID:         10,
						VenueName:       "Test Venue",
						VenueSlug:       "test-venue",
						SourceType:      "ai",
						RenderMethod:    &rm,
						EventsExtracted: 8,
						EventsImported:  6,
						DurationMs:      1500,
					},
					{
						ID:              2,
						VenueID:         20,
						VenueName:       "Other Venue",
						VenueSlug:       "other-venue",
						SourceType:      "ical",
						EventsExtracted: 12,
						EventsImported:  12,
						DurationMs:      300,
					},
				}, 2, nil
			},
		},
		nil,
	)

	resp, err := h.GetImportHistoryHandler(pipelineAdminCtx(), &GetImportHistoryRequest{Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Body.Total)
	}
	if len(resp.Body.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(resp.Body.Imports))
	}
	if resp.Body.Imports[0].VenueName != "Test Venue" {
		t.Errorf("expected venue_name=Test Venue, got %s", resp.Body.Imports[0].VenueName)
	}
	if resp.Body.Imports[0].SourceType != "ai" {
		t.Errorf("expected source_type=ai, got %s", resp.Body.Imports[0].SourceType)
	}
	if resp.Body.Imports[1].SourceType != "ical" {
		t.Errorf("expected source_type=ical, got %s", resp.Body.Imports[1].SourceType)
	}
}

func TestPipelineHandler_GetImportHistory_Empty(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			GetAllRecentRunsFn: func(limit, offset int) ([]contracts.ImportHistoryEntry, int64, error) {
				return []contracts.ImportHistoryEntry{}, 0, nil
			},
		},
		nil,
	)

	resp, err := h.GetImportHistoryHandler(pipelineAdminCtx(), &GetImportHistoryRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 0 {
		t.Errorf("expected total=0, got %d", resp.Body.Total)
	}
	if len(resp.Body.Imports) != 0 {
		t.Errorf("expected 0 imports, got %d", len(resp.Body.Imports))
	}
}

func TestPipelineHandler_GetImportHistory_PaginationPassedThrough(t *testing.T) {
	var receivedLimit, receivedOffset int
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			GetAllRecentRunsFn: func(limit, offset int) ([]contracts.ImportHistoryEntry, int64, error) {
				receivedLimit = limit
				receivedOffset = offset
				return nil, 0, nil
			},
		},
		nil,
	)

	_, err := h.GetImportHistoryHandler(pipelineAdminCtx(), &GetImportHistoryRequest{Limit: 50, Offset: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 50 {
		t.Errorf("expected limit=50, got %d", receivedLimit)
	}
	if receivedOffset != 10 {
		t.Errorf("expected offset=10, got %d", receivedOffset)
	}
}

func TestPipelineHandler_GetImportHistory_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{
			GetAllRecentRunsFn: func(limit, offset int) ([]contracts.ImportHistoryEntry, int64, error) {
				return nil, 0, fmt.Errorf("database error")
			},
		},
		nil,
	)

	_, err := h.GetImportHistoryHandler(pipelineAdminCtx(), &GetImportHistoryRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: EnrichmentStatusHandler
// ============================================================================

func TestPipelineHandler_EnrichmentStatus_Success(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{},
		&testhelpers.MockEnrichmentService{
			GetQueueStatsFn: func() (*contracts.EnrichmentQueueStats, error) {
				return &contracts.EnrichmentQueueStats{
					Pending:        5,
					Processing:     2,
					CompletedToday: 10,
					FailedToday:    1,
				}, nil
			},
		},
	)

	resp, err := h.EnrichmentStatusHandler(pipelineAdminCtx(), &EnrichmentStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Pending != 5 {
		t.Errorf("expected pending=5, got %d", resp.Body.Pending)
	}
	if resp.Body.Processing != 2 {
		t.Errorf("expected processing=2, got %d", resp.Body.Processing)
	}
	if resp.Body.CompletedToday != 10 {
		t.Errorf("expected completed_today=10, got %d", resp.Body.CompletedToday)
	}
	if resp.Body.FailedToday != 1 {
		t.Errorf("expected failed_today=1, got %d", resp.Body.FailedToday)
	}
}

func TestPipelineHandler_EnrichmentStatus_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{},
		&testhelpers.MockEnrichmentService{
			GetQueueStatsFn: func() (*contracts.EnrichmentQueueStats, error) {
				return nil, fmt.Errorf("database error")
			},
		},
	)

	_, err := h.EnrichmentStatusHandler(pipelineAdminCtx(), &EnrichmentStatusRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: TriggerEnrichmentHandler
// ============================================================================

func TestPipelineHandler_TriggerEnrichment_Success(t *testing.T) {
	var receivedShowID uint
	var receivedType string
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{},
		&testhelpers.MockEnrichmentService{
			QueueShowForEnrichmentFn: func(showID uint, enrichmentType string) error {
				receivedShowID = showID
				receivedType = enrichmentType
				return nil
			},
		},
	)

	resp, err := h.TriggerEnrichmentHandler(pipelineAdminCtx(), &TriggerEnrichmentRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if receivedShowID != 42 {
		t.Errorf("expected showID=42, got %d", receivedShowID)
	}
	if receivedType != "all" {
		t.Errorf("expected type=all, got %s", receivedType)
	}
}

func TestPipelineHandler_TriggerEnrichment_InvalidShowID(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{},
		&testhelpers.MockEnrichmentService{},
	)

	_, err := h.TriggerEnrichmentHandler(pipelineAdminCtx(), &TriggerEnrichmentRequest{ShowID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestPipelineHandler_TriggerEnrichment_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockPipelineService{},
		&testhelpers.MockVenueSourceConfigService{},
		&testhelpers.MockEnrichmentService{
			QueueShowForEnrichmentFn: func(showID uint, enrichmentType string) error {
				return fmt.Errorf("show not found")
			},
		},
	)

	_, err := h.TriggerEnrichmentHandler(pipelineAdminCtx(), &TriggerEnrichmentRequest{ShowID: "999"})
	testhelpers.AssertHumaError(t, err, 422)
}
