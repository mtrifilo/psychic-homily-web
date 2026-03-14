package pipeline

import (
	"fmt"
	"log"
	"time"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// PipelineService orchestrates the end-to-end AI extraction pipeline:
// fetch page -> detect changes -> extract events -> import shows.
type PipelineService struct {
	fetcher     contracts.FetcherServiceInterface
	extraction  contracts.ExtractionServiceInterface
	discovery   contracts.DiscoveryServiceInterface
	venueConfig contracts.VenueSourceConfigServiceInterface
	venue       contracts.VenueServiceInterface
}

// NewPipelineService creates a new pipeline orchestrator.
func NewPipelineService(
	fetcher contracts.FetcherServiceInterface,
	extraction contracts.ExtractionServiceInterface,
	discovery contracts.DiscoveryServiceInterface,
	venueConfig contracts.VenueSourceConfigServiceInterface,
	venue contracts.VenueServiceInterface,
) *PipelineService {
	return &PipelineService{
		fetcher:     fetcher,
		extraction:  extraction,
		discovery:   discovery,
		venueConfig: venueConfig,
		venue:       venue,
	}
}

// ExtractVenue runs the full extraction pipeline for a single venue.
func (s *PipelineService) ExtractVenue(venueID uint, dryRun bool) (*contracts.PipelineResult, error) {
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

	// 3. Check for feed-based extraction (iCal/RSS) — skip AI entirely if feed works
	if config.FeedURL != nil && *config.FeedURL != "" &&
		(config.PreferredSource == "ical" || config.PreferredSource == "rss") {
		result, feedErr := s.extractFromFeed(venue, config, dryRun, start)
		if feedErr == nil && result != nil {
			return result, nil
		}
		// Feed failed — fall through to AI extraction
		if feedErr != nil {
			log.Printf("venue %d: feed extraction failed, falling back to AI: %v", venueID, feedErr)
		}
	}

	// 4. Determine render method
	renderMethod := ""
	if config.RenderMethod != nil {
		renderMethod = *config.RenderMethod
	}

	// 5. Auto-detect render method if not set
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
	var fetchResult *contracts.FetchResult
	var fetchErr error

	switch renderMethod {
	case contracts.RenderMethodStatic:
		lastETag := ""
		lastHash := ""
		if config.LastETag != nil {
			lastETag = *config.LastETag
		}
		if config.LastContentHash != nil {
			lastHash = *config.LastContentHash
		}
		fetchResult, fetchErr = s.fetcher.Fetch(*config.CalendarURL, lastETag, lastHash)
	case contracts.RenderMethodDynamic:
		fetchResult, fetchErr = s.fetcher.FetchDynamic(*config.CalendarURL)
	case contracts.RenderMethodScreenshot:
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
	if !fetchResult.Changed && renderMethod == contracts.RenderMethodStatic {
		result := &contracts.PipelineResult{
			VenueID:      venueID,
			VenueName:    venue.Name,
			RenderMethod: renderMethod,
			Skipped:      true,
			SkipReason:   "page unchanged (hash match)",
			DurationMs:   time.Since(start).Milliseconds(),
			DryRun:       dryRun,
			InitialStatus: string(models.ShowStatusPending),
		}
		s.recordRun(venueID, renderMethod, config.PreferredSource, 0, 0, &fetchResult.ContentHash, fetchResult.HTTPStatus, start, nil)
		return result, nil
	}

	// 7. Determine content type for extraction
	contentType := "text"
	if renderMethod == contracts.RenderMethodScreenshot {
		contentType = "image"
	}

	// 8. Extract events via AI (include per-venue extraction notes if set)
	var extractionNotes string
	if config.ExtractionNotes != nil {
		extractionNotes = *config.ExtractionNotes
	}
	extractionResp, err := s.extraction.ExtractCalendarPage(venue.Name, fetchResult.Body, contentType, extractionNotes)
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

	// 9. Filter out non-music events
	musicEvents := filterMusicEvents(extractionResp.Events)
	eventsSkippedNonMusic := eventsExtracted - len(musicEvents)
	if eventsSkippedNonMusic > 0 {
		log.Printf("venue %d: filtered %d non-music events out of %d total", venueID, eventsSkippedNonMusic, eventsExtracted)
	}

	// 10. Convert to DiscoveredEvent format
	venueSlug := ""
	if venue.Slug != nil {
		venueSlug = *venue.Slug
	}
	discoveredEvents := CalendarEventsToDiscoveredEvents(venueSlug, musicEvents)

	// 11. Determine initial status based on auto_approve
	initialStatus := models.ShowStatusPending
	if config.AutoApprove {
		initialStatus = models.ShowStatusApproved
	}

	// 12. Import events (unless dry run)
	eventsImported := 0
	if !dryRun && len(discoveredEvents) > 0 {
		importResult, importErr := s.discovery.ImportEvents(discoveredEvents, false, false, initialStatus)
		if importErr != nil {
			log.Printf("warning: import failed for venue %d: %v (extraction succeeded with %d events)", venueID, importErr, eventsExtracted)
		} else {
			eventsImported = importResult.Imported
		}
	}

	duration := time.Since(start)

	// 13. Record run
	s.recordRun(venueID, renderMethod, config.PreferredSource, eventsExtracted, eventsImported, &fetchResult.ContentHash, fetchResult.HTTPStatus, start, nil)

	// 14. Update config after successful run
	if updateErr := s.venueConfig.UpdateAfterRun(venueID, &fetchResult.ContentHash, &fetchResult.ETag, eventsExtracted); updateErr != nil {
		log.Printf("warning: failed to update config after run for venue %d: %v", venueID, updateErr)
	}

	return &contracts.PipelineResult{
		VenueID:               venueID,
		VenueName:             venue.Name,
		RenderMethod:          renderMethod,
		EventsExtracted:       eventsExtracted,
		EventsImported:        eventsImported,
		EventsSkippedNonMusic: eventsSkippedNonMusic,
		DurationMs:            duration.Milliseconds(),
		Warnings:              extractionResp.Warnings,
		DryRun:                dryRun,
		InitialStatus:         string(initialStatus),
	}, nil
}

// filterMusicEvents returns only events where IsMusicEvent is not explicitly false.
// Events with IsMusicEvent=nil or IsMusicEvent=true are included.
func filterMusicEvents(events []contracts.CalendarEvent) []contracts.CalendarEvent {
	var filtered []contracts.CalendarEvent
	for _, e := range events {
		if e.IsMusicEvent != nil && !*e.IsMusicEvent {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
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

// extractFromFeed fetches and parses a venue's iCal or RSS feed.
// Returns nil result if the feed is empty or fails (caller should fall back to AI).
func (s *PipelineService) extractFromFeed(venue *models.Venue, config *models.VenueSourceConfig, dryRun bool, start time.Time) (*contracts.PipelineResult, error) {
	feedURL := *config.FeedURL
	feedType := config.PreferredSource // "ical" or "rss"

	// Fetch the feed with change detection
	lastETag := ""
	lastHash := ""
	if config.LastETag != nil {
		lastETag = *config.LastETag
	}
	if config.LastContentHash != nil {
		lastHash = *config.LastContentHash
	}

	fetchResult, err := s.fetcher.Fetch(feedURL, lastETag, lastHash)
	if err != nil {
		s.recordFailure(venue.ID, feedType, feedType, start, err)
		return nil, fmt.Errorf("feed fetch failed: %w", err)
	}

	// Check for changes
	if !fetchResult.Changed {
		result := &contracts.PipelineResult{
			VenueID:      venue.ID,
			VenueName:    venue.Name,
			RenderMethod: feedType,
			Skipped:      true,
			SkipReason:   "feed unchanged (hash match)",
			DurationMs:   time.Since(start).Milliseconds(),
			DryRun:       dryRun,
			InitialStatus: string(models.ShowStatusPending),
		}
		s.recordRun(venue.ID, feedType, feedType, 0, 0, &fetchResult.ContentHash, fetchResult.HTTPStatus, start, nil)
		return result, nil
	}

	// Parse the feed
	parser := NewFeedParser()
	venueSlug := ""
	if venue.Slug != nil {
		venueSlug = *venue.Slug
	}

	parsed, err := parser.ParseFeed([]byte(fetchResult.Body), feedType, venue.Name, venueSlug)
	if err != nil {
		s.recordFailure(venue.ID, feedType, feedType, start, err)
		return nil, fmt.Errorf("feed parse failed: %w", err)
	}

	eventsExtracted := len(parsed.Events)

	// Determine initial status
	initialStatus := models.ShowStatusPending
	if config.AutoApprove {
		initialStatus = models.ShowStatusApproved
	}

	// Import events (unless dry run)
	eventsImported := 0
	if !dryRun && eventsExtracted > 0 {
		importResult, importErr := s.discovery.ImportEvents(parsed.Events, false, false, initialStatus)
		if importErr != nil {
			log.Printf("warning: feed import failed for venue %d: %v", venue.ID, importErr)
		} else {
			eventsImported = importResult.Imported
		}
	}

	duration := time.Since(start)

	// Record run
	s.recordRun(venue.ID, feedType, feedType, eventsExtracted, eventsImported, &fetchResult.ContentHash, fetchResult.HTTPStatus, start, nil)

	// Update config after successful run
	if updateErr := s.venueConfig.UpdateAfterRun(venue.ID, &fetchResult.ContentHash, &fetchResult.ETag, eventsExtracted); updateErr != nil {
		log.Printf("warning: failed to update config after feed run for venue %d: %v", venue.ID, updateErr)
	}

	return &contracts.PipelineResult{
		VenueID:         venue.ID,
		VenueName:       venue.Name,
		RenderMethod:    feedType,
		EventsExtracted: eventsExtracted,
		EventsImported:  eventsImported,
		DurationMs:      duration.Milliseconds(),
		DryRun:          dryRun,
		InitialStatus:   string(initialStatus),
	}, nil
}
