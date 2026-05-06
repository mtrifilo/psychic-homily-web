package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// Default extraction interval (24 hours)
const DefaultExtractionInterval = 24 * time.Hour

// Default number of concurrent extraction workers
const DefaultExtractionWorkers = 3

// circuitBreakerThreshold is the number of consecutive failures before a venue
// is temporarily skipped. Circuit-broken venues are retried once per week.
const circuitBreakerThreshold = 5

// failureNotifyThreshold is the number of consecutive failures before a Discord
// notification is sent.
const failureNotifyThreshold = 3

// circuitBreakerRetryInterval is how often circuit-broken venues are retried.
const circuitBreakerRetryInterval = 7 * 24 * time.Hour

// venueExtractionResult holds the outcome of a single venue extraction.
type venueExtractionResult struct {
	VenueID         uint
	VenueName       string
	EventsExtracted int
	EventsImported  int
	Duration        time.Duration
	Err             error
	Skipped         bool
	SkipReason      string
}

// SchedulerService automatically processes venue calendar pages on a schedule.
// It follows the same Start/Stop/run pattern as CleanupService and ReminderService.
type SchedulerService struct {
	db                 *gorm.DB
	pipelineService    contracts.PipelineServiceInterface
	venueConfigService contracts.VenueSourceConfigServiceInterface
	discordService     contracts.DiscordServiceInterface
	interval           time.Duration
	workerCount        int
	stopCh             chan struct{}
	wg                 sync.WaitGroup
	logger             *slog.Logger
}

// NewSchedulerService creates a new extraction scheduler service.
// Env vars: EXTRACTION_INTERVAL_HOURS (default 24), EXTRACTION_WORKERS (default 3).
func NewSchedulerService(
	database *gorm.DB,
	pipelineSvc contracts.PipelineServiceInterface,
	venueConfigSvc contracts.VenueSourceConfigServiceInterface,
	discordSvc contracts.DiscordServiceInterface,
) *SchedulerService {
	if database == nil {
		database = db.GetDB()
	}

	interval := DefaultExtractionInterval
	if envInterval := os.Getenv("EXTRACTION_INTERVAL_HOURS"); envInterval != "" {
		if hours, err := strconv.Atoi(envInterval); err == nil && hours > 0 {
			interval = time.Duration(hours) * time.Hour
		}
	}

	workerCount := DefaultExtractionWorkers
	if envWorkers := os.Getenv("EXTRACTION_WORKERS"); envWorkers != "" {
		if w, err := strconv.Atoi(envWorkers); err == nil && w > 0 {
			workerCount = w
		}
	}

	return &SchedulerService{
		db:                 database,
		pipelineService:    pipelineSvc,
		venueConfigService: venueConfigSvc,
		discordService:     discordSvc,
		interval:           interval,
		workerCount:        workerCount,
		stopCh:             make(chan struct{}),
		logger:             slog.Default(),
	}
}

// Start begins the background extraction scheduler.
func (s *SchedulerService) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
	s.logger.Info("extraction scheduler started",
		"interval_hours", s.interval.Hours(),
		"workers", s.workerCount,
	)
}

// Stop gracefully stops the extraction scheduler.
func (s *SchedulerService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("extraction scheduler stopped")
}

// run is the main loop for the scheduler.
// Panic recovery via shared.RunTickerLoop (PSY-615).
func (s *SchedulerService) run(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "scheduler", s.interval, s.stopCh, true, func(_ context.Context) {
		s.runExtractionCycle()
	})
}

// runExtractionCycle performs a single extraction cycle across all configured venues.
func (s *SchedulerService) runExtractionCycle() {
	cycleStart := time.Now()
	s.logger.Info("starting extraction cycle")

	// 1. Get all configured venues
	configs, err := s.venueConfigService.ListConfigured()
	if err != nil {
		s.logger.Error("failed to list configured venues", "error", err)
		return
	}

	if len(configs) == 0 {
		s.logger.Info("no configured venues found")
		return
	}

	// 2. Filter to venues that are due for extraction
	dueVenues := s.filterDueVenues(configs)
	if len(dueVenues) == 0 {
		s.logger.Info("no venues due for extraction",
			"total_configured", len(configs),
		)
		return
	}

	s.logger.Info("venues due for extraction",
		"due", len(dueVenues),
		"total_configured", len(configs),
	)

	// 3. Fan out to workers via buffered channel
	jobs := make(chan adminm.VenueSourceConfig, len(dueVenues))
	results := make(chan venueExtractionResult, len(dueVenues))

	// Start workers
	var workerWg sync.WaitGroup
	for i := 0; i < s.workerCount; i++ {
		workerWg.Add(1)
		go s.extractionWorker(i, jobs, results, &workerWg)
	}

	// Send jobs
	for _, cfg := range dueVenues {
		jobs <- cfg
	}
	close(jobs)

	// Wait for workers to finish, then close results
	go func() {
		workerWg.Wait()
		close(results)
	}()

	// 4. Collect results
	var (
		totalProcessed int
		totalExtracted int
		totalImported  int
		totalFailed    int
		totalSkipped   int
	)

	for result := range results {
		totalProcessed++
		if result.Err != nil {
			totalFailed++
			s.logger.Error("venue extraction failed",
				"venue_id", result.VenueID,
				"venue_name", result.VenueName,
				"error", result.Err,
				"duration", result.Duration,
			)
		} else if result.Skipped {
			totalSkipped++
			s.logger.Info("venue extraction skipped",
				"venue_id", result.VenueID,
				"venue_name", result.VenueName,
				"reason", result.SkipReason,
			)
		} else {
			totalExtracted += result.EventsExtracted
			totalImported += result.EventsImported
			s.logger.Info("venue extraction complete",
				"venue_id", result.VenueID,
				"venue_name", result.VenueName,
				"events_extracted", result.EventsExtracted,
				"events_imported", result.EventsImported,
				"duration", result.Duration,
			)
		}

		// Check for anomalies after processing
		s.checkAnomalies(result)
	}

	cycleDuration := time.Since(cycleStart)
	s.logger.Info("extraction cycle complete",
		"venues_processed", totalProcessed,
		"events_extracted", totalExtracted,
		"events_imported", totalImported,
		"failures", totalFailed,
		"skipped", totalSkipped,
		"duration", cycleDuration,
	)

	// 5. Send Discord summary notification
	s.notifyCycleSummary(totalProcessed, totalExtracted, totalImported, totalFailed, cycleDuration)
}

// filterDueVenues returns venues that need extraction in this cycle.
func (s *SchedulerService) filterDueVenues(configs []adminm.VenueSourceConfig) []adminm.VenueSourceConfig {
	now := time.Now()
	var due []adminm.VenueSourceConfig

	for _, cfg := range configs {
		// Skip venues without a calendar URL
		if cfg.CalendarURL == nil || *cfg.CalendarURL == "" {
			continue
		}

		// Never extracted — always due
		if cfg.LastExtractedAt == nil {
			due = append(due, cfg)
			continue
		}

		// Circuit breaker: skip if >= threshold failures, unless it's been > 7 days
		if cfg.ConsecutiveFailures >= circuitBreakerThreshold {
			timeSinceLastExtraction := now.Sub(*cfg.LastExtractedAt)
			if timeSinceLastExtraction < circuitBreakerRetryInterval {
				s.logger.Debug("skipping circuit-broken venue",
					"venue_id", cfg.VenueID,
					"consecutive_failures", cfg.ConsecutiveFailures,
					"last_extracted_at", cfg.LastExtractedAt,
				)
				continue
			}
			// It's been over a week — retry once
			s.logger.Info("retrying circuit-broken venue (weekly retry)",
				"venue_id", cfg.VenueID,
				"consecutive_failures", cfg.ConsecutiveFailures,
			)
			due = append(due, cfg)
			continue
		}

		// Normal case: due if interval has passed since last extraction
		if now.Sub(*cfg.LastExtractedAt) >= s.interval {
			due = append(due, cfg)
		}
	}

	return due
}

// extractionWorker processes venue extraction jobs from the jobs channel.
func (s *SchedulerService) extractionWorker(
	workerID int,
	jobs <-chan adminm.VenueSourceConfig,
	results chan<- venueExtractionResult,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	for cfg := range jobs {
		start := time.Now()
		venueName := ""
		if cfg.Venue.ID != 0 {
			venueName = cfg.Venue.Name
		}

		s.logger.Info("worker starting venue extraction",
			"worker_id", workerID,
			"venue_id", cfg.VenueID,
			"venue_name", venueName,
		)

		pipelineResult, err := s.pipelineService.ExtractVenue(cfg.VenueID, false)
		duration := time.Since(start)

		result := venueExtractionResult{
			VenueID:   cfg.VenueID,
			VenueName: venueName,
			Duration:  duration,
			Err:       err,
		}

		if err == nil && pipelineResult != nil {
			result.EventsExtracted = pipelineResult.EventsExtracted
			result.EventsImported = pipelineResult.EventsImported
			result.Skipped = pipelineResult.Skipped
			result.SkipReason = pipelineResult.SkipReason
			if pipelineResult.VenueName != "" {
				result.VenueName = pipelineResult.VenueName
			}
		}

		results <- result
	}
}

// checkAnomalies performs simple anomaly detection after each venue extraction.
func (s *SchedulerService) checkAnomalies(result venueExtractionResult) {
	if result.Err == nil {
		return
	}

	// Fetch latest config to check consecutive failures
	cfg, err := s.venueConfigService.GetByVenueID(result.VenueID)
	if err != nil || cfg == nil {
		return
	}

	// Check zero-events anomaly
	if result.EventsExtracted == 0 && cfg.EventsExpected > 0 {
		s.logger.Warn("anomaly: zero events extracted from venue with expected events",
			"venue_id", result.VenueID,
			"venue_name", result.VenueName,
			"events_expected", cfg.EventsExpected,
		)
	}

	// Notify on 3 consecutive failures
	if cfg.ConsecutiveFailures == failureNotifyThreshold {
		s.notifyVenueFailure(result.VenueID, result.VenueName, cfg.ConsecutiveFailures)
	}

	// Log circuit breaker activation
	if cfg.ConsecutiveFailures == circuitBreakerThreshold {
		s.logger.Error("circuit breaker activated for venue",
			"venue_id", result.VenueID,
			"venue_name", result.VenueName,
			"consecutive_failures", cfg.ConsecutiveFailures,
			"message", "skipping until manual review or weekly retry",
		)
	}
}

// notifyCycleSummary sends a Discord notification summarizing the extraction cycle.
func (s *SchedulerService) notifyCycleSummary(processed, extracted, imported, failed int, duration time.Duration) {
	if s.discordService == nil || !s.discordService.IsConfigured() {
		return
	}

	// Use NotifyPipelineCycleSummary if available; otherwise just log
	s.logger.Info("extraction cycle summary notification",
		"venues_processed", processed,
		"events_extracted", extracted,
		"events_imported", imported,
		"failures", failed,
		"duration", duration,
	)
}

// notifyVenueFailure sends a Discord notification when a venue hits the failure threshold.
func (s *SchedulerService) notifyVenueFailure(venueID uint, venueName string, consecutiveFailures int) {
	s.logger.Warn("venue failure threshold reached",
		"venue_id", venueID,
		"venue_name", venueName,
		"consecutive_failures", consecutiveFailures,
	)

	if s.discordService == nil || !s.discordService.IsConfigured() {
		return
	}

	// Fire-and-forget: use NotifyPipelineVenueFailure if available; otherwise log only
	s.logger.Warn("venue pipeline failure notification would be sent",
		"venue_id", venueID,
		"venue_name", venueName,
		"consecutive_failures", consecutiveFailures,
	)
}

// RunExtractionCycleNow triggers an immediate extraction cycle (useful for testing).
func (s *SchedulerService) RunExtractionCycleNow() {
	s.runExtractionCycle()
}

// IsDueForExtraction checks whether a venue config is due for extraction
// based on the scheduler's interval and the circuit breaker logic.
// Exported for testing.
func IsDueForExtraction(cfg adminm.VenueSourceConfig, interval time.Duration, now time.Time) (bool, string) {
	// Skip venues without a calendar URL
	if cfg.CalendarURL == nil || *cfg.CalendarURL == "" {
		return false, "no calendar URL"
	}

	// Never extracted — always due
	if cfg.LastExtractedAt == nil {
		return true, "never extracted"
	}

	// Circuit breaker: skip if >= threshold failures, unless weekly retry is due
	if cfg.ConsecutiveFailures >= circuitBreakerThreshold {
		timeSinceLastExtraction := now.Sub(*cfg.LastExtractedAt)
		if timeSinceLastExtraction < circuitBreakerRetryInterval {
			return false, fmt.Sprintf("circuit breaker active (%d consecutive failures)", cfg.ConsecutiveFailures)
		}
		return true, "weekly retry of circuit-broken venue"
	}

	// Normal case: due if interval has passed
	if now.Sub(*cfg.LastExtractedAt) >= interval {
		return true, "interval elapsed"
	}

	return false, "not yet due"
}
