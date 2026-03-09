package services

import (
	"fmt"
	"log"
	"time"

	"psychic-homily-backend/internal/models"
)

// PipelineService orchestrates the end-to-end AI extraction pipeline:
// fetch page -> detect changes -> extract events -> import shows.
type PipelineService struct {
	fetcher     FetcherServiceInterface
	extraction  ExtractionServiceInterface
	discovery   DiscoveryServiceInterface
	venueConfig VenueSourceConfigServiceInterface
	venue       VenueServiceInterface
}

// NewPipelineService creates a new pipeline orchestrator.
func NewPipelineService(
	fetcher FetcherServiceInterface,
	extraction ExtractionServiceInterface,
	discovery DiscoveryServiceInterface,
	venueConfig VenueSourceConfigServiceInterface,
	venue VenueServiceInterface,
) *PipelineService {
	return &PipelineService{
		fetcher:     fetcher,
		extraction:  extraction,
		discovery:   discovery,
		venueConfig: venueConfig,
		venue:       venue,
	}
}

// PipelineResult contains the outcome of a single venue extraction run.
type PipelineResult struct {
	VenueID         uint     `json:"venue_id"`
	VenueName       string   `json:"venue_name"`
	RenderMethod    string   `json:"render_method"`
	EventsExtracted int      `json:"events_extracted"`
	EventsImported  int      `json:"events_imported"`
	DurationMs      int64    `json:"duration_ms"`
	Skipped         bool     `json:"skipped"`
	SkipReason      string   `json:"skip_reason,omitempty"`
	Error           string   `json:"error,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	DryRun          bool     `json:"dry_run"`
}

// ExtractVenue runs the full extraction pipeline for a single venue.
func (s *PipelineService) ExtractVenue(venueID uint, dryRun bool) (*PipelineResult, error) {
	start := time.Now()

	// 1. Get venue
	venue, err := s.venue.GetVenueModel(venueID)
	if err != nil {
		return nil, fmt.Errorf("venue not found: %w", err)
	}

	// 2. Get source config
	config, err := s.venueConfig.GetByVenueID(venueID)
	if err != nil {
		return nil, fmt.Errorf("failed to get venue source config: %w", err)
	}
	if config == nil {
		return nil, fmt.Errorf("venue %d (%s) has no source config", venueID, venue.Name)
	}
	if config.CalendarURL == nil || *config.CalendarURL == "" {
		return nil, fmt.Errorf("venue %d (%s) has no calendar URL configured", venueID, venue.Name)
	}

	// 3. Determine render method
	renderMethod := ""
	if config.RenderMethod != nil {
		renderMethod = *config.RenderMethod
	}

	// 4. Auto-detect render method if not set
	if renderMethod == "" {
		detected, detectErr := s.fetcher.DetectRenderMethod(*config.CalendarURL)
		if detectErr != nil {
			s.recordFailure(venueID, renderMethod, config.PreferredSource, start, detectErr)
			return nil, fmt.Errorf("render method auto-detection failed: %w", detectErr)
		}
		renderMethod = detected
		// Persist detected method
		config.RenderMethod = &renderMethod
		if _, updateErr := s.venueConfig.CreateOrUpdate(config); updateErr != nil {
			log.Printf("warning: failed to save detected render method for venue %d: %v", venueID, updateErr)
		}
	}

	// 5. Fetch based on render method
	var fetchResult *FetchResult
	var fetchErr error

	switch renderMethod {
	case RenderMethodStatic:
		lastETag := ""
		lastHash := ""
		if config.LastETag != nil {
			lastETag = *config.LastETag
		}
		if config.LastContentHash != nil {
			lastHash = *config.LastContentHash
		}
		fetchResult, fetchErr = s.fetcher.Fetch(*config.CalendarURL, lastETag, lastHash)
	case RenderMethodDynamic:
		fetchResult, fetchErr = s.fetcher.FetchDynamic(*config.CalendarURL)
	case RenderMethodScreenshot:
		fetchResult, fetchErr = s.fetcher.FetchScreenshot(*config.CalendarURL)
	default:
		fetchErr = fmt.Errorf("unknown render method: %s", renderMethod)
	}

	if fetchErr != nil {
		s.recordFailure(venueID, renderMethod, config.PreferredSource, start, fetchErr)
		if incrementErr := s.venueConfig.IncrementFailures(venueID); incrementErr != nil {
			log.Printf("warning: failed to increment failures for venue %d: %v", venueID, incrementErr)
		}
		return nil, fmt.Errorf("fetch failed: %w", fetchErr)
	}

	// 6. Check for changes (static only -- dynamic/screenshot always proceed)
	if !fetchResult.Changed && renderMethod == RenderMethodStatic {
		result := &PipelineResult{
			VenueID:      venueID,
			VenueName:    venue.Name,
			RenderMethod: renderMethod,
			Skipped:      true,
			SkipReason:   "page unchanged (hash match)",
			DurationMs:   time.Since(start).Milliseconds(),
			DryRun:       dryRun,
		}
		s.recordRun(venueID, renderMethod, config.PreferredSource, 0, 0, &fetchResult.ContentHash, fetchResult.HTTPStatus, start, nil)
		return result, nil
	}

	// 7. Determine content type for extraction
	contentType := "text"
	if renderMethod == RenderMethodScreenshot {
		contentType = "image"
	}

	// 8. Extract events via AI
	extractionResp, err := s.extraction.ExtractCalendarPage(venue.Name, fetchResult.Body, contentType)
	if err != nil {
		s.recordFailure(venueID, renderMethod, config.PreferredSource, start, err)
		if incrementErr := s.venueConfig.IncrementFailures(venueID); incrementErr != nil {
			log.Printf("warning: failed to increment failures for venue %d: %v", venueID, incrementErr)
		}
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	if !extractionResp.Success {
		extractionErr := fmt.Errorf("extraction returned error: %s", extractionResp.Error)
		s.recordFailure(venueID, renderMethod, config.PreferredSource, start, extractionErr)
		if incrementErr := s.venueConfig.IncrementFailures(venueID); incrementErr != nil {
			log.Printf("warning: failed to increment failures for venue %d: %v", venueID, incrementErr)
		}
		return nil, extractionErr
	}

	eventsExtracted := len(extractionResp.Events)

	// 9. Convert to DiscoveredEvent format
	venueSlug := ""
	if venue.Slug != nil {
		venueSlug = *venue.Slug
	}
	discoveredEvents := CalendarEventsToDiscoveredEvents(venueSlug, extractionResp.Events)

	// 10. Import events (unless dry run)
	eventsImported := 0
	if !dryRun && len(discoveredEvents) > 0 {
		importResult, importErr := s.discovery.ImportEvents(discoveredEvents, false, false)
		if importErr != nil {
			log.Printf("warning: import failed for venue %d: %v (extraction succeeded with %d events)", venueID, importErr, eventsExtracted)
		} else {
			eventsImported = importResult.Imported
		}
	}

	duration := time.Since(start)

	// 11. Record run
	s.recordRun(venueID, renderMethod, config.PreferredSource, eventsExtracted, eventsImported, &fetchResult.ContentHash, fetchResult.HTTPStatus, start, nil)

	// 12. Update config after successful run
	if updateErr := s.venueConfig.UpdateAfterRun(venueID, &fetchResult.ContentHash, &fetchResult.ETag, eventsExtracted); updateErr != nil {
		log.Printf("warning: failed to update config after run for venue %d: %v", venueID, updateErr)
	}

	return &PipelineResult{
		VenueID:         venueID,
		VenueName:       venue.Name,
		RenderMethod:    renderMethod,
		EventsExtracted: eventsExtracted,
		EventsImported:  eventsImported,
		DurationMs:      duration.Milliseconds(),
		Warnings:        extractionResp.Warnings,
		DryRun:          dryRun,
	}, nil
}

// recordRun persists an extraction run record (fire-and-forget).
func (s *PipelineService) recordRun(venueID uint, renderMethod, preferredSource string, extracted, imported int, contentHash *string, httpStatus int, start time.Time, runErr error) {
	run := &models.VenueExtractionRun{
		VenueID:         venueID,
		RenderMethod:    strPtrIfNonEmpty(renderMethod),
		PreferredSource: strPtrIfNonEmpty(preferredSource),
		EventsExtracted: extracted,
		EventsImported:  imported,
		ContentHash:     contentHash,
		HTTPStatus:      intPtrIfNonZero(httpStatus),
		DurationMs:      int(time.Since(start).Milliseconds()),
	}
	if runErr != nil {
		errStr := runErr.Error()
		run.Error = &errStr
	}
	if err := s.venueConfig.RecordRun(run); err != nil {
		log.Printf("warning: failed to record extraction run for venue %d: %v", venueID, err)
	}
}

// recordFailure is a convenience wrapper for recording a failed run.
func (s *PipelineService) recordFailure(venueID uint, renderMethod, preferredSource string, start time.Time, err error) {
	s.recordRun(venueID, renderMethod, preferredSource, 0, 0, nil, 0, start, err)
}

// strPtrIfNonEmpty returns a pointer to s if non-empty, else nil.
func strPtrIfNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// intPtrIfNonZero returns a pointer to i if non-zero, else nil.
func intPtrIfNonZero(i int) *int {
	if i == 0 {
		return nil
	}
	return &i
}
