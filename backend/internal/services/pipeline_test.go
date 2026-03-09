package services

import (
	"fmt"
	"testing"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
)

// ============================================================================
// Test doubles for PipelineService dependencies
// ============================================================================

type stubFetcher struct {
	fetchFn              func(url, lastETag, lastContentHash string) (*FetchResult, error)
	fetchDynamicFn       func(url string) (*FetchResult, error)
	fetchScreenshotFn    func(url string) (*FetchResult, error)
	detectRenderMethodFn func(url string) (string, error)
}

func (s *stubFetcher) Fetch(url, lastETag, lastContentHash string) (*FetchResult, error) {
	if s.fetchFn != nil {
		return s.fetchFn(url, lastETag, lastContentHash)
	}
	return &FetchResult{Changed: true, Body: "<html>events</html>", ContentHash: "abc123", HTTPStatus: 200}, nil
}
func (s *stubFetcher) FetchDynamic(url string) (*FetchResult, error) {
	if s.fetchDynamicFn != nil {
		return s.fetchDynamicFn(url)
	}
	return &FetchResult{Changed: true, Body: "<html>dynamic events</html>", ContentHash: "def456", HTTPStatus: 200}, nil
}
func (s *stubFetcher) FetchScreenshot(url string) (*FetchResult, error) {
	if s.fetchScreenshotFn != nil {
		return s.fetchScreenshotFn(url)
	}
	return &FetchResult{Changed: true, Body: "base64screenshot", ContentHash: "ghi789", HTTPStatus: 200, ContentType: "image/png"}, nil
}
func (s *stubFetcher) DetectRenderMethod(url string) (string, error) {
	if s.detectRenderMethodFn != nil {
		return s.detectRenderMethodFn(url)
	}
	return "static", nil
}

type stubExtraction struct {
	extractCalendarPageFn func(venueName, content, contentType string) (*CalendarExtractionResponse, error)
}

func (s *stubExtraction) ExtractShow(req *ExtractShowRequest) (*ExtractShowResponse, error) {
	return nil, fmt.Errorf("not implemented in stub")
}
func (s *stubExtraction) ExtractCalendarPage(venueName, content, contentType string) (*CalendarExtractionResponse, error) {
	if s.extractCalendarPageFn != nil {
		return s.extractCalendarPageFn(venueName, content, contentType)
	}
	return &CalendarExtractionResponse{
		Success: true,
		Events: []CalendarEvent{
			{Date: "2026-04-01", Title: "Test Band", Artists: []CalendarArtist{{Name: "Test Band", IsHeadliner: true}}},
			{Date: "2026-04-02", Title: "Other Band", Artists: []CalendarArtist{{Name: "Other Band", IsHeadliner: true}}},
		},
	}, nil
}

type stubDiscovery struct {
	importEventsFn func(events []DiscoveredEvent, dryRun bool, allowUpdates bool) (*ImportResult, error)
}

func (s *stubDiscovery) ImportFromJSON(filepath string, dryRun bool) (*ImportResult, error) {
	return nil, fmt.Errorf("not implemented in stub")
}
func (s *stubDiscovery) ImportFromJSONWithDB(filepath string, dryRun bool, database *gorm.DB) (*ImportResult, error) {
	return nil, fmt.Errorf("not implemented in stub")
}
func (s *stubDiscovery) CheckEvents(events []CheckEventInput) (*CheckEventsResult, error) {
	return nil, fmt.Errorf("not implemented in stub")
}
func (s *stubDiscovery) ImportEvents(events []DiscoveredEvent, dryRun bool, allowUpdates bool) (*ImportResult, error) {
	if s.importEventsFn != nil {
		return s.importEventsFn(events, dryRun, allowUpdates)
	}
	return &ImportResult{Total: len(events), Imported: len(events)}, nil
}

type stubVenueConfig struct {
	getByVenueIDFn      func(venueID uint) (*models.VenueSourceConfig, error)
	createOrUpdateFn    func(config *models.VenueSourceConfig) (*models.VenueSourceConfig, error)
	updateAfterRunFn    func(venueID uint, contentHash, etag *string, eventsExtracted int) error
	incrementFailuresFn func(venueID uint) error
	recordRunFn         func(run *models.VenueExtractionRun) error
	getRecentRunsFn     func(venueID uint, limit int) ([]models.VenueExtractionRun, error)
	listConfiguredFn    func() ([]models.VenueSourceConfig, error)
}

func (s *stubVenueConfig) GetByVenueID(venueID uint) (*models.VenueSourceConfig, error) {
	if s.getByVenueIDFn != nil {
		return s.getByVenueIDFn(venueID)
	}
	return nil, nil
}
func (s *stubVenueConfig) CreateOrUpdate(config *models.VenueSourceConfig) (*models.VenueSourceConfig, error) {
	if s.createOrUpdateFn != nil {
		return s.createOrUpdateFn(config)
	}
	return config, nil
}
func (s *stubVenueConfig) UpdateAfterRun(venueID uint, contentHash, etag *string, eventsExtracted int) error {
	if s.updateAfterRunFn != nil {
		return s.updateAfterRunFn(venueID, contentHash, etag, eventsExtracted)
	}
	return nil
}
func (s *stubVenueConfig) IncrementFailures(venueID uint) error {
	if s.incrementFailuresFn != nil {
		return s.incrementFailuresFn(venueID)
	}
	return nil
}
func (s *stubVenueConfig) RecordRun(run *models.VenueExtractionRun) error {
	if s.recordRunFn != nil {
		return s.recordRunFn(run)
	}
	return nil
}
func (s *stubVenueConfig) GetRecentRuns(venueID uint, limit int) ([]models.VenueExtractionRun, error) {
	if s.getRecentRunsFn != nil {
		return s.getRecentRunsFn(venueID, limit)
	}
	return nil, nil
}
func (s *stubVenueConfig) ListConfigured() ([]models.VenueSourceConfig, error) {
	if s.listConfiguredFn != nil {
		return s.listConfiguredFn()
	}
	return nil, nil
}

type stubVenueService struct {
	getVenueModelFn func(venueID uint) (*models.Venue, error)
}

// Satisfy the full VenueServiceInterface with panics for methods we don't use.
func (s *stubVenueService) CreateVenue(req *CreateVenueRequest, isAdmin bool) (*VenueDetailResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) GetVenue(venueID uint) (*VenueDetailResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) GetVenueBySlug(slug string) (*VenueDetailResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) GetVenues(filters map[string]interface{}) ([]*VenueDetailResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) UpdateVenue(venueID uint, updates map[string]interface{}) (*VenueDetailResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) DeleteVenue(venueID uint) error { panic("not implemented") }
func (s *stubVenueService) SearchVenues(query string) ([]*VenueDetailResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) FindOrCreateVenue(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*models.Venue, bool, error) {
	panic("not implemented")
}
func (s *stubVenueService) VerifyVenue(venueID uint) (*VenueDetailResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) GetVenuesWithShowCounts(filters VenueListFilters, limit, offset int) ([]*VenueWithShowCountResponse, int64, error) {
	panic("not implemented")
}
func (s *stubVenueService) GetUpcomingShowsForVenue(venueID uint, timezone string, limit int) ([]*VenueShowResponse, int64, error) {
	panic("not implemented")
}
func (s *stubVenueService) GetShowsForVenue(venueID uint, timezone string, limit int, timeFilter string) ([]*VenueShowResponse, int64, error) {
	panic("not implemented")
}
func (s *stubVenueService) GetVenueCities() ([]*VenueCityResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) CreatePendingVenueEdit(venueID uint, userID uint, req *VenueEditRequest) (*PendingVenueEditResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) GetPendingEditForVenue(venueID uint, userID uint) (*PendingVenueEditResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) GetPendingVenueEdits(limit, offset int) ([]*PendingVenueEditResponse, int64, error) {
	panic("not implemented")
}
func (s *stubVenueService) GetPendingVenueEdit(editID uint) (*PendingVenueEditResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) ApproveVenueEdit(editID uint, reviewerID uint) (*VenueDetailResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) RejectVenueEdit(editID uint, reviewerID uint, reason string) (*PendingVenueEditResponse, error) {
	panic("not implemented")
}
func (s *stubVenueService) CancelPendingVenueEdit(editID uint, userID uint) error {
	panic("not implemented")
}
func (s *stubVenueService) GetVenueModel(venueID uint) (*models.Venue, error) {
	if s.getVenueModelFn != nil {
		return s.getVenueModelFn(venueID)
	}
	slug := "test-venue"
	return &models.Venue{ID: venueID, Name: "Test Venue", Slug: &slug, City: "Phoenix", State: "AZ"}, nil
}
func (s *stubVenueService) GetUnverifiedVenues(limit, offset int) ([]*UnverifiedVenueResponse, int64, error) {
	panic("not implemented")
}

// ============================================================================
// Helper to build PipelineService with stubs
// ============================================================================

func newTestPipeline(opts ...func(*testPipelineOpts)) *PipelineService {
	o := &testPipelineOpts{
		fetcher:     &stubFetcher{},
		extraction:  &stubExtraction{},
		discovery:   &stubDiscovery{},
		venueConfig: &stubVenueConfig{},
		venue:       &stubVenueService{},
	}
	for _, opt := range opts {
		opt(o)
	}
	return NewPipelineService(o.fetcher, o.extraction, o.discovery, o.venueConfig, o.venue)
}

type testPipelineOpts struct {
	fetcher     FetcherServiceInterface
	extraction  ExtractionServiceInterface
	discovery   DiscoveryServiceInterface
	venueConfig VenueSourceConfigServiceInterface
	venue       VenueServiceInterface
}

func withFetcher(f FetcherServiceInterface) func(*testPipelineOpts) {
	return func(o *testPipelineOpts) { o.fetcher = f }
}
func withExtraction(e ExtractionServiceInterface) func(*testPipelineOpts) {
	return func(o *testPipelineOpts) { o.extraction = e }
}
func withDiscovery(d DiscoveryServiceInterface) func(*testPipelineOpts) {
	return func(o *testPipelineOpts) { o.discovery = d }
}
func withVenueConfig(vc VenueSourceConfigServiceInterface) func(*testPipelineOpts) {
	return func(o *testPipelineOpts) { o.venueConfig = vc }
}
func withVenue(v VenueServiceInterface) func(*testPipelineOpts) {
	return func(o *testPipelineOpts) { o.venue = v }
}

// defaultConfig returns a VenueSourceConfig with calendar URL and static render method.
func defaultConfig() *models.VenueSourceConfig {
	calURL := "https://example.com/events"
	rm := "static"
	return &models.VenueSourceConfig{
		ID:              1,
		VenueID:         1,
		CalendarURL:     &calURL,
		PreferredSource: "ai",
		RenderMethod:    &rm,
	}
}

// ============================================================================
// Tests
// ============================================================================

func TestPipeline_NewPipelineService(t *testing.T) {
	ps := NewPipelineService(nil, nil, nil, nil, nil)
	if ps == nil {
		t.Fatal("expected non-nil PipelineService")
	}
}

func TestPipeline_ExtractVenue_Success(t *testing.T) {
	var recordedRun *models.VenueExtractionRun
	var updatedHash *string

	ps := newTestPipeline(
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				return defaultConfig(), nil
			},
			recordRunFn: func(run *models.VenueExtractionRun) error {
				recordedRun = run
				return nil
			},
			updateAfterRunFn: func(venueID uint, contentHash, etag *string, eventsExtracted int) error {
				updatedHash = contentHash
				return nil
			},
		}),
	)

	result, err := ps.ExtractVenue(1, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.VenueID != 1 {
		t.Errorf("expected venue_id=1, got %d", result.VenueID)
	}
	if result.VenueName != "Test Venue" {
		t.Errorf("expected venue_name=Test Venue, got %s", result.VenueName)
	}
	if result.RenderMethod != "static" {
		t.Errorf("expected render_method=static, got %s", result.RenderMethod)
	}
	if result.EventsExtracted != 2 {
		t.Errorf("expected events_extracted=2, got %d", result.EventsExtracted)
	}
	if result.EventsImported != 2 {
		t.Errorf("expected events_imported=2, got %d", result.EventsImported)
	}
	if result.Skipped {
		t.Error("expected skipped=false")
	}
	if result.DryRun {
		t.Error("expected dry_run=false")
	}
	if result.DurationMs < 0 {
		t.Errorf("expected non-negative duration, got %d", result.DurationMs)
	}

	// Verify run was recorded
	if recordedRun == nil {
		t.Fatal("expected run to be recorded")
	}
	if recordedRun.VenueID != 1 {
		t.Errorf("expected run venue_id=1, got %d", recordedRun.VenueID)
	}
	if recordedRun.EventsExtracted != 2 {
		t.Errorf("expected run events_extracted=2, got %d", recordedRun.EventsExtracted)
	}

	// Verify config was updated
	if updatedHash == nil {
		t.Fatal("expected config to be updated with hash")
	}
}

func TestPipeline_ExtractVenue_DryRun(t *testing.T) {
	importCalled := false

	ps := newTestPipeline(
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				return defaultConfig(), nil
			},
		}),
		withDiscovery(&stubDiscovery{
			importEventsFn: func(events []DiscoveredEvent, dryRun bool, allowUpdates bool) (*ImportResult, error) {
				importCalled = true
				return &ImportResult{Total: len(events), Imported: len(events)}, nil
			},
		}),
	)

	result, err := ps.ExtractVenue(1, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if importCalled {
		t.Error("import should not be called in dry run mode")
	}
	if !result.DryRun {
		t.Error("expected dry_run=true in result")
	}
	if result.EventsExtracted != 2 {
		t.Errorf("expected events_extracted=2, got %d", result.EventsExtracted)
	}
	if result.EventsImported != 0 {
		t.Errorf("expected events_imported=0 in dry run, got %d", result.EventsImported)
	}
}

func TestPipeline_ExtractVenue_NoConfig(t *testing.T) {
	ps := newTestPipeline(
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				return nil, nil // no config
			},
		}),
	)

	_, err := ps.ExtractVenue(1, false)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	if want := "venue 1 (Test Venue) has no source config"; err.Error() != want {
		t.Errorf("expected error %q, got %q", want, err.Error())
	}
}

func TestPipeline_ExtractVenue_NoCalendarURL(t *testing.T) {
	ps := newTestPipeline(
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				return &models.VenueSourceConfig{
					ID:      1,
					VenueID: 1,
					// CalendarURL is nil
				}, nil
			},
		}),
	)

	_, err := ps.ExtractVenue(1, false)
	if err == nil {
		t.Fatal("expected error for missing calendar URL")
	}
	if want := "venue 1 (Test Venue) has no calendar URL configured"; err.Error() != want {
		t.Errorf("expected error %q, got %q", want, err.Error())
	}
}

func TestPipeline_ExtractVenue_EmptyCalendarURL(t *testing.T) {
	ps := newTestPipeline(
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				empty := ""
				return &models.VenueSourceConfig{
					ID:          1,
					VenueID:     1,
					CalendarURL: &empty,
				}, nil
			},
		}),
	)

	_, err := ps.ExtractVenue(1, false)
	if err == nil {
		t.Fatal("expected error for empty calendar URL")
	}
}

func TestPipeline_ExtractVenue_UnchangedPage(t *testing.T) {
	var recordedRun *models.VenueExtractionRun

	ps := newTestPipeline(
		withFetcher(&stubFetcher{
			fetchFn: func(url, lastETag, lastContentHash string) (*FetchResult, error) {
				return &FetchResult{Changed: false, ContentHash: "same-hash", HTTPStatus: 200}, nil
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				return defaultConfig(), nil
			},
			recordRunFn: func(run *models.VenueExtractionRun) error {
				recordedRun = run
				return nil
			},
		}),
	)

	result, err := ps.ExtractVenue(1, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Skipped {
		t.Error("expected skipped=true for unchanged page")
	}
	if result.SkipReason != "page unchanged (hash match)" {
		t.Errorf("unexpected skip reason: %s", result.SkipReason)
	}
	if result.EventsExtracted != 0 {
		t.Errorf("expected events_extracted=0, got %d", result.EventsExtracted)
	}

	// Should still record the run
	if recordedRun == nil {
		t.Fatal("expected skipped run to be recorded")
	}
}

func TestPipeline_ExtractVenue_AutoDetection(t *testing.T) {
	detectedMethod := ""
	var savedConfig *models.VenueSourceConfig

	ps := newTestPipeline(
		withFetcher(&stubFetcher{
			detectRenderMethodFn: func(url string) (string, error) {
				return "dynamic", nil
			},
			fetchDynamicFn: func(url string) (*FetchResult, error) {
				return &FetchResult{Changed: true, Body: "<html>dynamic</html>", ContentHash: "dyn-hash", HTTPStatus: 200}, nil
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				calURL := "https://example.com/events"
				return &models.VenueSourceConfig{
					ID:              1,
					VenueID:         1,
					CalendarURL:     &calURL,
					PreferredSource: "ai",
					RenderMethod:    nil, // not set — trigger auto-detect
				}, nil
			},
			createOrUpdateFn: func(config *models.VenueSourceConfig) (*models.VenueSourceConfig, error) {
				savedConfig = config
				if config.RenderMethod != nil {
					detectedMethod = *config.RenderMethod
				}
				return config, nil
			},
		}),
	)

	result, err := ps.ExtractVenue(1, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detectedMethod != "dynamic" {
		t.Errorf("expected detected method=dynamic, got %s", detectedMethod)
	}
	if result.RenderMethod != "dynamic" {
		t.Errorf("expected result render_method=dynamic, got %s", result.RenderMethod)
	}
	if savedConfig == nil {
		t.Fatal("expected config to be saved with detected render method")
	}
}

func TestPipeline_ExtractVenue_AutoDetectionFails(t *testing.T) {
	var failureRecorded bool

	ps := newTestPipeline(
		withFetcher(&stubFetcher{
			detectRenderMethodFn: func(url string) (string, error) {
				return "", fmt.Errorf("detection error")
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				calURL := "https://example.com/events"
				return &models.VenueSourceConfig{
					ID:              1,
					VenueID:         1,
					CalendarURL:     &calURL,
					PreferredSource: "ai",
					RenderMethod:    nil, // trigger auto-detect
				}, nil
			},
			recordRunFn: func(run *models.VenueExtractionRun) error {
				failureRecorded = run.Error != nil
				return nil
			},
		}),
	)

	_, err := ps.ExtractVenue(1, false)
	if err == nil {
		t.Fatal("expected error from auto-detection failure")
	}
	if !failureRecorded {
		t.Error("expected failure to be recorded")
	}
}

func TestPipeline_ExtractVenue_ExtractionFails(t *testing.T) {
	var failureRecorded bool
	var failuresIncremented bool

	ps := newTestPipeline(
		withExtraction(&stubExtraction{
			extractCalendarPageFn: func(venueName, content, contentType string) (*CalendarExtractionResponse, error) {
				return nil, fmt.Errorf("AI service unavailable")
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				return defaultConfig(), nil
			},
			recordRunFn: func(run *models.VenueExtractionRun) error {
				failureRecorded = run.Error != nil
				return nil
			},
			incrementFailuresFn: func(venueID uint) error {
				failuresIncremented = true
				return nil
			},
		}),
	)

	_, err := ps.ExtractVenue(1, false)
	if err == nil {
		t.Fatal("expected error from extraction failure")
	}
	if !failureRecorded {
		t.Error("expected failure run to be recorded")
	}
	if !failuresIncremented {
		t.Error("expected consecutive failures to be incremented")
	}
}

func TestPipeline_ExtractVenue_ExtractionReturnsError(t *testing.T) {
	var failuresIncremented bool

	ps := newTestPipeline(
		withExtraction(&stubExtraction{
			extractCalendarPageFn: func(venueName, content, contentType string) (*CalendarExtractionResponse, error) {
				return &CalendarExtractionResponse{
					Success: false,
					Error:   "AI service not configured",
				}, nil
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				return defaultConfig(), nil
			},
			incrementFailuresFn: func(venueID uint) error {
				failuresIncremented = true
				return nil
			},
		}),
	)

	_, err := ps.ExtractVenue(1, false)
	if err == nil {
		t.Fatal("expected error when extraction returns success=false")
	}
	if !failuresIncremented {
		t.Error("expected consecutive failures to be incremented")
	}
}

func TestPipeline_ExtractVenue_FetchFails(t *testing.T) {
	var failureRecorded bool
	var failuresIncremented bool

	ps := newTestPipeline(
		withFetcher(&stubFetcher{
			fetchFn: func(url, lastETag, lastContentHash string) (*FetchResult, error) {
				return nil, fmt.Errorf("HTTP 503 server error")
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				return defaultConfig(), nil
			},
			recordRunFn: func(run *models.VenueExtractionRun) error {
				failureRecorded = run.Error != nil
				return nil
			},
			incrementFailuresFn: func(venueID uint) error {
				failuresIncremented = true
				return nil
			},
		}),
	)

	_, err := ps.ExtractVenue(1, false)
	if err == nil {
		t.Fatal("expected error from fetch failure")
	}
	if !failureRecorded {
		t.Error("expected failure run to be recorded")
	}
	if !failuresIncremented {
		t.Error("expected consecutive failures to be incremented")
	}
}

func TestPipeline_ExtractVenue_DynamicRenderMethod(t *testing.T) {
	var fetchedDynamic bool

	ps := newTestPipeline(
		withFetcher(&stubFetcher{
			fetchDynamicFn: func(url string) (*FetchResult, error) {
				fetchedDynamic = true
				return &FetchResult{Changed: true, Body: "<html>dynamic</html>", ContentHash: "dyn-hash", HTTPStatus: 200}, nil
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				calURL := "https://example.com/events"
				rm := "dynamic"
				return &models.VenueSourceConfig{
					ID:              1,
					VenueID:         1,
					CalendarURL:     &calURL,
					PreferredSource: "ai",
					RenderMethod:    &rm,
				}, nil
			},
		}),
	)

	result, err := ps.ExtractVenue(1, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fetchedDynamic {
		t.Error("expected FetchDynamic to be called")
	}
	if result.RenderMethod != "dynamic" {
		t.Errorf("expected render_method=dynamic, got %s", result.RenderMethod)
	}
}

func TestPipeline_ExtractVenue_ScreenshotRenderMethod(t *testing.T) {
	var fetchedScreenshot bool
	var extractionContentType string

	ps := newTestPipeline(
		withFetcher(&stubFetcher{
			fetchScreenshotFn: func(url string) (*FetchResult, error) {
				fetchedScreenshot = true
				return &FetchResult{Changed: true, Body: "base64data", ContentHash: "ss-hash", HTTPStatus: 200, ContentType: "image/png"}, nil
			},
		}),
		withExtraction(&stubExtraction{
			extractCalendarPageFn: func(venueName, content, contentType string) (*CalendarExtractionResponse, error) {
				extractionContentType = contentType
				return &CalendarExtractionResponse{
					Success: true,
					Events:  []CalendarEvent{{Date: "2026-04-01", Title: "Test"}},
				}, nil
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				calURL := "https://example.com/events"
				rm := "screenshot"
				return &models.VenueSourceConfig{
					ID:              1,
					VenueID:         1,
					CalendarURL:     &calURL,
					PreferredSource: "ai",
					RenderMethod:    &rm,
				}, nil
			},
		}),
	)

	result, err := ps.ExtractVenue(1, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fetchedScreenshot {
		t.Error("expected FetchScreenshot to be called")
	}
	if extractionContentType != "image" {
		t.Errorf("expected extraction content_type=image, got %s", extractionContentType)
	}
	if result.RenderMethod != "screenshot" {
		t.Errorf("expected render_method=screenshot, got %s", result.RenderMethod)
	}
}

func TestPipeline_ExtractVenue_VenueNotFound(t *testing.T) {
	ps := newTestPipeline(
		withVenue(&stubVenueService{
			getVenueModelFn: func(venueID uint) (*models.Venue, error) {
				return nil, fmt.Errorf("venue not found")
			},
		}),
	)

	_, err := ps.ExtractVenue(999, false)
	if err == nil {
		t.Fatal("expected error for venue not found")
	}
}

func TestPipeline_ExtractVenue_UnknownRenderMethod(t *testing.T) {
	var failuresIncremented bool

	ps := newTestPipeline(
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				calURL := "https://example.com/events"
				rm := "unknown_method"
				return &models.VenueSourceConfig{
					ID:              1,
					VenueID:         1,
					CalendarURL:     &calURL,
					PreferredSource: "ai",
					RenderMethod:    &rm,
				}, nil
			},
			incrementFailuresFn: func(venueID uint) error {
				failuresIncremented = true
				return nil
			},
		}),
	)

	_, err := ps.ExtractVenue(1, false)
	if err == nil {
		t.Fatal("expected error for unknown render method")
	}
	if !failuresIncremented {
		t.Error("expected failures to be incremented for unknown render method")
	}
}

func TestPipeline_ExtractVenue_ImportFails_NonFatal(t *testing.T) {
	// Import failure should NOT cause the pipeline to error — extraction still succeeded
	ps := newTestPipeline(
		withDiscovery(&stubDiscovery{
			importEventsFn: func(events []DiscoveredEvent, dryRun bool, allowUpdates bool) (*ImportResult, error) {
				return nil, fmt.Errorf("database error")
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				return defaultConfig(), nil
			},
		}),
	)

	result, err := ps.ExtractVenue(1, false)
	if err != nil {
		t.Fatalf("import failure should be non-fatal, got: %v", err)
	}
	if result.EventsExtracted != 2 {
		t.Errorf("expected events_extracted=2, got %d", result.EventsExtracted)
	}
	if result.EventsImported != 0 {
		t.Errorf("expected events_imported=0 after import failure, got %d", result.EventsImported)
	}
}

func TestPipeline_ExtractVenue_NoEventsExtracted(t *testing.T) {
	ps := newTestPipeline(
		withExtraction(&stubExtraction{
			extractCalendarPageFn: func(venueName, content, contentType string) (*CalendarExtractionResponse, error) {
				return &CalendarExtractionResponse{
					Success:  true,
					Events:   []CalendarEvent{},
					Warnings: []string{"No events were found on the calendar page"},
				}, nil
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				return defaultConfig(), nil
			},
		}),
	)

	result, err := ps.ExtractVenue(1, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EventsExtracted != 0 {
		t.Errorf("expected events_extracted=0, got %d", result.EventsExtracted)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warnings to be propagated")
	}
}

func TestPipeline_ExtractVenue_DynamicAlwaysProceeds(t *testing.T) {
	// Even when Changed=false (shouldn't happen for dynamic, but test the logic),
	// dynamic should still proceed because we only check Changed for static.
	ps := newTestPipeline(
		withFetcher(&stubFetcher{
			fetchDynamicFn: func(url string) (*FetchResult, error) {
				return &FetchResult{Changed: false, Body: "<html>dynamic</html>", ContentHash: "dyn-hash", HTTPStatus: 200}, nil
			},
		}),
		withVenueConfig(&stubVenueConfig{
			getByVenueIDFn: func(venueID uint) (*models.VenueSourceConfig, error) {
				calURL := "https://example.com/events"
				rm := "dynamic"
				return &models.VenueSourceConfig{
					ID:              1,
					VenueID:         1,
					CalendarURL:     &calURL,
					PreferredSource: "ai",
					RenderMethod:    &rm,
				}, nil
			},
		}),
	)

	result, err := ps.ExtractVenue(1, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Skipped {
		t.Error("dynamic fetch should not be skipped even if Changed=false")
	}
	if result.EventsExtracted != 2 {
		t.Errorf("expected events_extracted=2, got %d", result.EventsExtracted)
	}
}

// ============================================================================
// Helper function unit tests
// ============================================================================

func TestPipeline_StrPtrIfNonEmpty(t *testing.T) {
	if strPtrIfNonEmpty("") != nil {
		t.Error("expected nil for empty string")
	}
	p := strPtrIfNonEmpty("test")
	if p == nil || *p != "test" {
		t.Error("expected pointer to 'test'")
	}
}

func TestPipeline_IntPtrIfNonZero(t *testing.T) {
	if intPtrIfNonZero(0) != nil {
		t.Error("expected nil for zero")
	}
	p := intPtrIfNonZero(42)
	if p == nil || *p != 42 {
		t.Error("expected pointer to 42")
	}
}
