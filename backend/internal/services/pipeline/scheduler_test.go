package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// ── Stub implementations ──────────────────────────────────────────────

// stubPipelineService is a test double for PipelineServiceInterface.
type stubPipelineService struct {
	mu      sync.Mutex
	calls   []uint // venueIDs that were extracted
	results map[uint]*contracts.PipelineResult
	errors  map[uint]error
}

func newStubPipelineService() *stubPipelineService {
	return &stubPipelineService{
		results: make(map[uint]*contracts.PipelineResult),
		errors:  make(map[uint]error),
	}
}

func (s *stubPipelineService) ExtractVenue(venueID uint, dryRun bool) (*contracts.PipelineResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, venueID)

	if err, ok := s.errors[venueID]; ok {
		return nil, err
	}
	if result, ok := s.results[venueID]; ok {
		return result, nil
	}
	return &contracts.PipelineResult{
		VenueID:         venueID,
		VenueName:       fmt.Sprintf("Venue %d", venueID),
		EventsExtracted: 5,
		EventsImported:  3,
	}, nil
}

func (s *stubPipelineService) getCalls() []uint {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]uint, len(s.calls))
	copy(result, s.calls)
	return result
}

// stubVenueConfigService is a test double for VenueSourceConfigServiceInterface.
type stubVenueConfigService struct {
	configs []models.VenueSourceConfig
	byID    map[uint]*models.VenueSourceConfig
}

func newStubVenueConfigService() *stubVenueConfigService {
	return &stubVenueConfigService{
		byID: make(map[uint]*models.VenueSourceConfig),
	}
}

func (s *stubVenueConfigService) ListConfigured() ([]models.VenueSourceConfig, error) {
	return s.configs, nil
}

func (s *stubVenueConfigService) GetByVenueID(venueID uint) (*models.VenueSourceConfig, error) {
	if cfg, ok := s.byID[venueID]; ok {
		return cfg, nil
	}
	return nil, nil
}

func (s *stubVenueConfigService) CreateOrUpdate(config *models.VenueSourceConfig) (*models.VenueSourceConfig, error) {
	return config, nil
}

func (s *stubVenueConfigService) UpdateAfterRun(venueID uint, contentHash, etag *string, eventsExtracted int) error {
	return nil
}

func (s *stubVenueConfigService) IncrementFailures(venueID uint) error {
	return nil
}

func (s *stubVenueConfigService) RecordRun(run *models.VenueExtractionRun) error {
	return nil
}

func (s *stubVenueConfigService) GetRecentRuns(venueID uint, limit int) ([]models.VenueExtractionRun, error) {
	return nil, nil
}

func (s *stubVenueConfigService) GetAllRecentRuns(limit, offset int) ([]contracts.ImportHistoryEntry, int64, error) {
	return nil, 0, nil
}

func (s *stubVenueConfigService) GetRejectionStats(venueID uint) (*contracts.VenueRejectionStats, error) {
	return nil, nil
}

func (s *stubVenueConfigService) UpdateExtractionNotes(venueID uint, notes *string) error {
	return nil
}

func (s *stubVenueConfigService) ResetRenderMethod(venueID uint) error {
	return nil
}

// stubDiscordService is a test double for DiscordServiceInterface.
type stubDiscordService struct {
	configured bool
}

func (s *stubDiscordService) IsConfigured() bool { return s.configured }

func (s *stubDiscordService) NotifyNewUser(user *models.User)                               {}
func (s *stubDiscordService) NotifyNewShow(show *contracts.ShowResponse, submitterEmail string) {}
func (s *stubDiscordService) NotifyShowStatusChange(showTitle string, showID uint, oldStatus, newStatus, actorEmail string) {
}
func (s *stubDiscordService) NotifyShowApproved(show *contracts.ShowResponse)              {}
func (s *stubDiscordService) NotifyShowRejected(show *contracts.ShowResponse, reason string) {}
func (s *stubDiscordService) NotifyShowReport(report *models.ShowReport, reporterEmail string) {}
func (s *stubDiscordService) NotifyArtistReport(report *models.ArtistReport, reporterEmail string) {
}
func (s *stubDiscordService) NotifyNewVenue(venueID uint, venueName, city, state string, address *string, submitterEmail string) {
}

// ── Tests ──────────────────────────────────────────────────────────────

func TestSchedulerService_NilDB(t *testing.T) {
	// Constructing with nil DB should not panic (falls back to db.GetDB)
	svc := NewSchedulerService(nil, newStubPipelineService(), newStubVenueConfigService(), &stubDiscordService{})
	require.NotNil(t, svc)
	assert.Equal(t, DefaultExtractionInterval, svc.interval)
	assert.Equal(t, DefaultExtractionWorkers, svc.workerCount)
}

func TestSchedulerService_EnvVars(t *testing.T) {
	t.Setenv("EXTRACTION_INTERVAL_HOURS", "6")
	t.Setenv("EXTRACTION_WORKERS", "5")

	svc := NewSchedulerService(nil, newStubPipelineService(), newStubVenueConfigService(), &stubDiscordService{})
	require.NotNil(t, svc)
	assert.Equal(t, 6*time.Hour, svc.interval)
	assert.Equal(t, 5, svc.workerCount)
}

func TestSchedulerService_EnvVarsInvalid(t *testing.T) {
	t.Setenv("EXTRACTION_INTERVAL_HOURS", "not-a-number")
	t.Setenv("EXTRACTION_WORKERS", "-1")

	svc := NewSchedulerService(nil, newStubPipelineService(), newStubVenueConfigService(), &stubDiscordService{})
	require.NotNil(t, svc)
	// Should fall back to defaults
	assert.Equal(t, DefaultExtractionInterval, svc.interval)
	assert.Equal(t, DefaultExtractionWorkers, svc.workerCount)
}

func TestSchedulerService_StartStop(t *testing.T) {
	// Use stubs that return no configs so the cycle is a no-op
	svc := NewSchedulerService(nil, newStubPipelineService(), newStubVenueConfigService(), &stubDiscordService{})

	ctx, cancel := context.WithCancel(context.Background())

	svc.Start(ctx)
	// Give the goroutine a moment to start
	time.Sleep(50 * time.Millisecond)

	cancel()
	svc.Stop()
	// Should not hang — goroutine exits cleanly
}

// ── IsDueForExtraction tests ───────────────────────────────────────────

func TestIsDueForExtraction_NoCalendarURL(t *testing.T) {
	cfg := models.VenueSourceConfig{VenueID: 1}
	due, reason := IsDueForExtraction(cfg, 24*time.Hour, time.Now())
	assert.False(t, due)
	assert.Equal(t, "no calendar URL", reason)
}

func TestIsDueForExtraction_EmptyCalendarURL(t *testing.T) {
	empty := ""
	cfg := models.VenueSourceConfig{VenueID: 1, CalendarURL: &empty}
	due, reason := IsDueForExtraction(cfg, 24*time.Hour, time.Now())
	assert.False(t, due)
	assert.Equal(t, "no calendar URL", reason)
}

func TestIsDueForExtraction_NeverExtracted(t *testing.T) {
	url := "https://example.com/calendar"
	cfg := models.VenueSourceConfig{VenueID: 1, CalendarURL: &url}
	due, reason := IsDueForExtraction(cfg, 24*time.Hour, time.Now())
	assert.True(t, due)
	assert.Equal(t, "never extracted", reason)
}

func TestIsDueForExtraction_IntervalElapsed(t *testing.T) {
	url := "https://example.com/calendar"
	lastExtracted := time.Now().Add(-25 * time.Hour) // 25 hours ago
	cfg := models.VenueSourceConfig{
		VenueID:         1,
		CalendarURL:     &url,
		LastExtractedAt: &lastExtracted,
	}
	due, reason := IsDueForExtraction(cfg, 24*time.Hour, time.Now())
	assert.True(t, due)
	assert.Equal(t, "interval elapsed", reason)
}

func TestIsDueForExtraction_NotYetDue(t *testing.T) {
	url := "https://example.com/calendar"
	lastExtracted := time.Now().Add(-12 * time.Hour) // 12 hours ago, interval is 24
	cfg := models.VenueSourceConfig{
		VenueID:         1,
		CalendarURL:     &url,
		LastExtractedAt: &lastExtracted,
	}
	due, reason := IsDueForExtraction(cfg, 24*time.Hour, time.Now())
	assert.False(t, due)
	assert.Equal(t, "not yet due", reason)
}

func TestIsDueForExtraction_CircuitBreakerActive(t *testing.T) {
	url := "https://example.com/calendar"
	lastExtracted := time.Now().Add(-2 * 24 * time.Hour) // 2 days ago
	cfg := models.VenueSourceConfig{
		VenueID:             1,
		CalendarURL:         &url,
		LastExtractedAt:     &lastExtracted,
		ConsecutiveFailures: 5,
	}
	due, reason := IsDueForExtraction(cfg, 24*time.Hour, time.Now())
	assert.False(t, due)
	assert.Contains(t, reason, "circuit breaker active")
}

func TestIsDueForExtraction_CircuitBreakerWeeklyRetry(t *testing.T) {
	url := "https://example.com/calendar"
	lastExtracted := time.Now().Add(-8 * 24 * time.Hour) // 8 days ago
	cfg := models.VenueSourceConfig{
		VenueID:             1,
		CalendarURL:         &url,
		LastExtractedAt:     &lastExtracted,
		ConsecutiveFailures: 7,
	}
	due, reason := IsDueForExtraction(cfg, 24*time.Hour, time.Now())
	assert.True(t, due)
	assert.Equal(t, "weekly retry of circuit-broken venue", reason)
}

func TestIsDueForExtraction_CircuitBreakerExactThreshold(t *testing.T) {
	url := "https://example.com/calendar"
	lastExtracted := time.Now().Add(-3 * 24 * time.Hour) // 3 days ago
	cfg := models.VenueSourceConfig{
		VenueID:             1,
		CalendarURL:         &url,
		LastExtractedAt:     &lastExtracted,
		ConsecutiveFailures: 5, // exactly at threshold
	}
	due, reason := IsDueForExtraction(cfg, 24*time.Hour, time.Now())
	assert.False(t, due)
	assert.Contains(t, reason, "circuit breaker active")
}

func TestIsDueForExtraction_BelowCircuitBreaker(t *testing.T) {
	url := "https://example.com/calendar"
	lastExtracted := time.Now().Add(-25 * time.Hour)
	cfg := models.VenueSourceConfig{
		VenueID:             1,
		CalendarURL:         &url,
		LastExtractedAt:     &lastExtracted,
		ConsecutiveFailures: 4, // below threshold
	}
	due, reason := IsDueForExtraction(cfg, 24*time.Hour, time.Now())
	assert.True(t, due)
	assert.Equal(t, "interval elapsed", reason)
}

// ── filterDueVenues tests ──────────────────────────────────────────────

func TestFilterDueVenues(t *testing.T) {
	url1 := "https://venue1.com/calendar"
	url2 := "https://venue2.com/calendar"
	url3 := "https://venue3.com/calendar"
	url4 := "https://venue4.com/calendar"
	url5 := "https://venue5.com/calendar"

	now := time.Now()
	recentExtraction := now.Add(-12 * time.Hour) // 12h ago
	oldExtraction := now.Add(-25 * time.Hour)    // 25h ago
	veryOld := now.Add(-8 * 24 * time.Hour)      // 8 days ago

	configs := []models.VenueSourceConfig{
		{VenueID: 1, CalendarURL: &url1, LastExtractedAt: nil},                                                                          // never extracted
		{VenueID: 2, CalendarURL: &url2, LastExtractedAt: &recentExtraction},                                                            // not yet due
		{VenueID: 3, CalendarURL: &url3, LastExtractedAt: &oldExtraction},                                                               // due (25h > 24h interval)
		{VenueID: 4, CalendarURL: &url4, LastExtractedAt: &oldExtraction, ConsecutiveFailures: 6},                                       // circuit broken, recent
		{VenueID: 5, CalendarURL: &url5, LastExtractedAt: &veryOld, ConsecutiveFailures: 6},                                             // circuit broken, weekly retry
		{VenueID: 6},                                                                                                                     // no URL
	}

	svc := &SchedulerService{interval: 24 * time.Hour, logger: newTestLogger()}
	due := svc.filterDueVenues(configs)

	dueIDs := make([]uint, len(due))
	for i, c := range due {
		dueIDs[i] = c.VenueID
	}

	assert.Contains(t, dueIDs, uint(1), "never-extracted venue should be due")
	assert.Contains(t, dueIDs, uint(3), "venue past interval should be due")
	assert.Contains(t, dueIDs, uint(5), "circuit-broken venue past 7 days should be due for weekly retry")
	assert.NotContains(t, dueIDs, uint(2), "recently-extracted venue should not be due")
	assert.NotContains(t, dueIDs, uint(4), "circuit-broken venue within 7 days should not be due")
	assert.NotContains(t, dueIDs, uint(6), "venue without calendar URL should not be due")
	assert.Len(t, due, 3)
}

// ── Integration-style test: runExtractionCycle ─────────────────────────

func TestSchedulerService_RunExtractionCycle(t *testing.T) {
	url1 := "https://venue1.com/calendar"
	url2 := "https://venue2.com/calendar"
	url3 := "https://venue3.com/calendar"

	oldExtraction := time.Now().Add(-25 * time.Hour)

	pipelineSvc := newStubPipelineService()
	pipelineSvc.results[1] = &contracts.PipelineResult{
		VenueID:         1,
		VenueName:       "Venue One",
		EventsExtracted: 10,
		EventsImported:  8,
	}
	pipelineSvc.results[2] = &contracts.PipelineResult{
		VenueID:         2,
		VenueName:       "Venue Two",
		EventsExtracted: 5,
		EventsImported:  3,
	}
	pipelineSvc.errors[3] = fmt.Errorf("extraction failed: API error")

	venueConfigSvc := newStubVenueConfigService()
	venueConfigSvc.configs = []models.VenueSourceConfig{
		{
			VenueID:         1,
			CalendarURL:     &url1,
			LastExtractedAt: &oldExtraction,
			Venue:           models.Venue{Name: "Venue One"},
		},
		{
			VenueID:         2,
			CalendarURL:     &url2,
			LastExtractedAt: &oldExtraction,
			Venue:           models.Venue{Name: "Venue Two"},
		},
		{
			VenueID:         3,
			CalendarURL:     &url3,
			LastExtractedAt: &oldExtraction,
			Venue:           models.Venue{Name: "Venue Three"},
		},
	}
	// For anomaly checking
	venueConfigSvc.byID[3] = &models.VenueSourceConfig{
		VenueID:             3,
		ConsecutiveFailures: 2, // below threshold, no notification
	}

	svc := &SchedulerService{
		pipelineService:    pipelineSvc,
		venueConfigService: venueConfigSvc,
		discordService:     &stubDiscordService{configured: false},
		interval:           24 * time.Hour,
		workerCount:        2,
		logger:             newTestLogger(),
	}

	svc.runExtractionCycle()

	// Verify all 3 venues were processed
	calls := pipelineSvc.getCalls()
	assert.Len(t, calls, 3)
	assert.Contains(t, calls, uint(1))
	assert.Contains(t, calls, uint(2))
	assert.Contains(t, calls, uint(3))
}

func TestSchedulerService_RunExtractionCycle_NoConfigs(t *testing.T) {
	pipelineSvc := newStubPipelineService()
	venueConfigSvc := newStubVenueConfigService()
	// No configs

	svc := &SchedulerService{
		pipelineService:    pipelineSvc,
		venueConfigService: venueConfigSvc,
		discordService:     &stubDiscordService{},
		interval:           24 * time.Hour,
		workerCount:        2,
		logger:             newTestLogger(),
	}

	svc.runExtractionCycle()

	// No venues should have been processed
	calls := pipelineSvc.getCalls()
	assert.Empty(t, calls)
}

func TestSchedulerService_RunExtractionCycle_AllNotDue(t *testing.T) {
	url1 := "https://venue1.com/calendar"
	recentExtraction := time.Now().Add(-1 * time.Hour) // 1 hour ago

	pipelineSvc := newStubPipelineService()
	venueConfigSvc := newStubVenueConfigService()
	venueConfigSvc.configs = []models.VenueSourceConfig{
		{
			VenueID:         1,
			CalendarURL:     &url1,
			LastExtractedAt: &recentExtraction,
			Venue:           models.Venue{Name: "Venue One"},
		},
	}

	svc := &SchedulerService{
		pipelineService:    pipelineSvc,
		venueConfigService: venueConfigSvc,
		discordService:     &stubDiscordService{},
		interval:           24 * time.Hour,
		workerCount:        2,
		logger:             newTestLogger(),
	}

	svc.runExtractionCycle()

	calls := pipelineSvc.getCalls()
	assert.Empty(t, calls)
}

func TestSchedulerService_RunExtractionCycleNow(t *testing.T) {
	pipelineSvc := newStubPipelineService()
	venueConfigSvc := newStubVenueConfigService()

	svc := &SchedulerService{
		pipelineService:    pipelineSvc,
		venueConfigService: venueConfigSvc,
		discordService:     &stubDiscordService{},
		interval:           24 * time.Hour,
		workerCount:        2,
		logger:             newTestLogger(),
	}

	// Should not panic
	svc.RunExtractionCycleNow()
}

func TestSchedulerService_SkippedResult(t *testing.T) {
	url1 := "https://venue1.com/calendar"
	oldExtraction := time.Now().Add(-25 * time.Hour)

	pipelineSvc := newStubPipelineService()
	pipelineSvc.results[1] = &contracts.PipelineResult{
		VenueID:   1,
		VenueName: "Venue One",
		Skipped:   true,
		SkipReason: "page unchanged (hash match)",
	}

	venueConfigSvc := newStubVenueConfigService()
	venueConfigSvc.configs = []models.VenueSourceConfig{
		{
			VenueID:         1,
			CalendarURL:     &url1,
			LastExtractedAt: &oldExtraction,
			Venue:           models.Venue{Name: "Venue One"},
		},
	}

	svc := &SchedulerService{
		pipelineService:    pipelineSvc,
		venueConfigService: venueConfigSvc,
		discordService:     &stubDiscordService{},
		interval:           24 * time.Hour,
		workerCount:        1,
		logger:             newTestLogger(),
	}

	svc.runExtractionCycle()

	calls := pipelineSvc.getCalls()
	assert.Len(t, calls, 1)
	assert.Equal(t, uint(1), calls[0])
}

// ── Helper ─────────────────────────────────────────────────────────────

func newTestLogger() *slog.Logger {
	return slog.Default()
}
