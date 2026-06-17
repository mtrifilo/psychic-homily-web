package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// CreateImportJob creates a new pending import job for a radio show.
// Validates that no other job is currently running for the same show.
func (s *RadioService) CreateImportJob(showID uint, since, until string) (*contracts.RadioImportJobResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Validate show exists and get station ID
	var show catalogm.RadioShow
	if err := s.db.Preload("Station").First(&show, showID).Error; err != nil {
		return nil, fmt.Errorf("show not found: %w", err)
	}

	// Validate date format
	if _, err := time.Parse("2006-01-02", since); err != nil {
		return nil, fmt.Errorf("invalid since date format (expected YYYY-MM-DD): %w", err)
	}
	if _, err := time.Parse("2006-01-02", until); err != nil {
		return nil, fmt.Errorf("invalid until date format (expected YYYY-MM-DD): %w", err)
	}

	// Check for existing running/pending job
	var activeCount int64
	s.db.Model(&catalogm.RadioImportJob{}).
		Where("show_id = ? AND status IN ?", showID, []string{
			catalogm.RadioImportJobStatusPending,
			catalogm.RadioImportJobStatusRunning,
		}).
		Count(&activeCount)
	if activeCount > 0 {
		return nil, fmt.Errorf("an import job is already running or pending for this show")
	}

	job := &catalogm.RadioImportJob{
		ShowID:    showID,
		StationID: show.StationID,
		Since:     since,
		Until:     until,
		Status:    catalogm.RadioImportJobStatusPending,
	}

	if err := s.db.Create(job).Error; err != nil {
		return nil, fmt.Errorf("creating import job: %w", err)
	}

	return s.jobToResponse(job, show.Name, show.Station.Name), nil
}

// StartImportJob transitions a pending job to running and launches the background goroutine.
//
// The pending→running transition is performed as a single conditional UPDATE
// (WHERE status = pending) and the launch only fires when RowsAffected == 1.
// Two concurrent callers therefore cannot both succeed: the loser sees
// RowsAffected == 0, reads the row to surface the actual current status, and
// returns an error without spawning a duplicate runImportJob goroutine.
func (s *RadioService) StartImportJob(jobID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// First confirm the job exists so the not-found path returns a clear error
	// rather than the generic "not in pending status" RowsAffected==0 fallback.
	var job catalogm.RadioImportJob
	if err := s.db.First(&job, jobID).Error; err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	now := time.Now()
	result := s.db.Model(&catalogm.RadioImportJob{}).
		Where("id = ? AND status = ?", jobID, catalogm.RadioImportJobStatusPending).
		Updates(map[string]interface{}{
			"status":     catalogm.RadioImportJobStatusRunning,
			"started_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("starting import job: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Either the status changed between First() and Updates() (race with
		// another StartImportJob/CancelImportJob call), or it was never
		// pending. Re-read to report the actual current status.
		if err := s.db.Select("status").First(&job, jobID).Error; err != nil {
			return fmt.Errorf("job not found: %w", err)
		}
		return fmt.Errorf("job is not in pending status (current: %s)", job.Status)
	}

	// Launch the import goroutine. Safe to do unconditionally now: only the
	// caller that won the conditional UPDATE reaches this line.
	shared.GoSafe(context.Background(), "radio_import_job", func() {
		s.runImportJob(jobID)
	})

	return nil
}

// CancelImportJob sets a running or pending job to cancelled.
// If the job is running, the goroutine will check status periodically and stop.
func (s *RadioService) CancelImportJob(jobID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var job catalogm.RadioImportJob
	if err := s.db.First(&job, jobID).Error; err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	if job.Status != catalogm.RadioImportJobStatusRunning && job.Status != catalogm.RadioImportJobStatusPending {
		return fmt.Errorf("job cannot be cancelled (current status: %s)", job.Status)
	}

	now := time.Now()
	s.db.Model(&job).Updates(map[string]interface{}{
		"status":       catalogm.RadioImportJobStatusCancelled,
		"completed_at": now,
	})

	return nil
}

// GetImportJob returns a single import job by ID with show/station names.
func (s *RadioService) GetImportJob(jobID uint) (*contracts.RadioImportJobResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var job catalogm.RadioImportJob
	if err := s.db.Preload("Show").Preload("Station").First(&job, jobID).Error; err != nil {
		return nil, fmt.Errorf("job not found: %w", err)
	}

	return s.jobToResponse(&job, job.Show.Name, job.Station.Name), nil
}

// ListImportJobs returns all import jobs for a given show, ordered by newest first.
func (s *RadioService) ListImportJobs(showID uint) ([]*contracts.RadioImportJobResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var jobs []catalogm.RadioImportJob
	if err := s.db.Preload("Show").Preload("Station").
		Where("show_id = ?", showID).
		Order("created_at DESC").
		Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("listing import jobs: %w", err)
	}

	results := make([]*contracts.RadioImportJobResponse, len(jobs))
	for i, job := range jobs {
		results[i] = s.jobToResponse(&job, job.Show.Name, job.Station.Name)
	}

	return results, nil
}

// runImportJob is the background goroutine that performs the actual import work.
// It delegates to importShowEpisodesWithProgress with a callback that updates
// the job's DB row with progress and checks for cancellation.
func (s *RadioService) runImportJob(jobID uint) {
	logger := slog.Default().With("job_id", jobID)
	logger.Info("radio_import_job_started")

	// Reload job from DB to get show/since/until
	var job catalogm.RadioImportJob
	if err := s.db.First(&job, jobID).Error; err != nil {
		logger.Error("radio_import_job_load_failed", "error", err.Error())
		return
	}

	// Track total episodes processed (including errors) for interval-based checks
	var totalProcessed int
	var lastEpisodeDate string

	episodesFoundFn := func(count int) {
		s.db.Model(&catalogm.RadioImportJob{}).Where("id = ?", jobID).
			Update("episodes_found", count)
		logger.Info("radio_import_job_episodes_found", "in_date_range", count)
	}

	progressFn := func(episodesImported, playsImported, playsMatched int, currentDate string, errors []string) (cancel bool) {
		totalProcessed++
		lastEpisodeDate = currentDate

		// Check for cancellation every 5 episodes
		if totalProcessed%5 == 0 {
			if s.isJobCancelled(jobID) {
				logger.Info("radio_import_job_cancelled", "episodes_processed", totalProcessed)
				return true
			}
		}

		// Batch update progress every 10 episodes
		if totalProcessed%10 == 0 {
			s.db.Model(&catalogm.RadioImportJob{}).Where("id = ?", jobID).
				Updates(map[string]interface{}{
					"episodes_imported":    episodesImported,
					"plays_imported":       playsImported,
					"plays_matched":        playsMatched,
					"current_episode_date": currentDate,
				})
		}

		return false
	}

	// job.Since/job.Until are read straight from Postgres DATE columns and
	// round-trip as "...T00:00:00Z"; importShowEpisodesWithProgress normalizes
	// them via parseImportDate before parsing, so passing the raw values is safe
	// (do NOT add a bare time.Parse here). (PSY-927)
	result, err := s.importShowEpisodesWithProgress(job.ShowID, job.Since, job.Until, episodesFoundFn, progressFn)
	if err != nil {
		s.failJob(jobID, err.Error())
		return
	}

	// If the job was cancelled mid-import, don't overwrite its status
	if s.isJobCancelled(jobID) {
		return
	}

	// Final update: mark completed
	now := time.Now()
	updates := map[string]interface{}{
		"status":            catalogm.RadioImportJobStatusCompleted,
		"episodes_imported": result.EpisodesImported,
		"plays_imported":    result.PlaysImported,
		"plays_matched":     result.PlaysMatched,
		"completed_at":      now,
	}

	if lastEpisodeDate != "" {
		updates["current_episode_date"] = lastEpisodeDate
	}

	// PSY-1119: when episodes failed to fetch (or matches failed to persist),
	// the job is "completed with errors" rather than cleanly completed. We avoid
	// a status-enum migration (the status column is varchar(20); a new value
	// would need to widen it) and instead make the error_log self-describing
	// with a stable, machine-greppable header. The episode_fetch_errors count is
	// the distinct queryable signal the frontend branches on: status=="completed"
	// with a non-empty error_log whose first line carries this header means the
	// import lost plays, not that it ran clean.
	if len(result.Errors) > 0 {
		errorLog := buildJobErrorLog(result)
		updates["error_log"] = errorLog
	}

	s.db.Model(&catalogm.RadioImportJob{}).Where("id = ?", jobID).Updates(updates)

	logger.Info("radio_import_job_completed",
		"episodes_imported", result.EpisodesImported,
		"plays_imported", result.PlaysImported,
		"plays_matched", result.PlaysMatched,
		"episode_fetch_errors", result.EpisodeFetchErrors,
		"match_persist_errors", result.MatchPersistErrors,
		"errors", len(result.Errors),
	)
}

// buildJobErrorLog renders the job's error_log text. When the import finished
// with episode-level errors (failed playlist fetches or unpersisted matches —
// PSY-1119), it prepends a stable summary header so an admin or the frontend
// can tell at a glance, without parsing every per-episode line, that the job
// "completed with errors" and how many episodes lost their plays. The header is
// kept on its own line above the existing per-episode detail lines so prior
// log-scraping of those lines is unaffected (additive).
func buildJobErrorLog(result *contracts.RadioImportResult) string {
	var b strings.Builder
	if result.EpisodeFetchErrors > 0 || result.MatchPersistErrors > 0 {
		fmt.Fprintf(&b,
			"completed with errors: %d episodes failed to fetch, %d play matches failed to persist\n",
			result.EpisodeFetchErrors, result.MatchPersistErrors)
	}
	for _, msg := range result.Errors {
		b.WriteString(msg)
		b.WriteString("\n")
	}
	return b.String()
}

// failJob marks a job as failed with an error message.
func (s *RadioService) failJob(jobID uint, errMsg string) {
	now := time.Now()
	s.db.Model(&catalogm.RadioImportJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":       catalogm.RadioImportJobStatusFailed,
		"error_log":    errMsg,
		"completed_at": now,
	})
	slog.Default().Error("radio_import_job_failed", "job_id", jobID, "error", errMsg)
}

// normalizeDateString strips any time component from a date string so the
// response always returns YYYY-MM-DD. Postgres DATE columns round-trip through
// GORM into Go strings as "2025-04-01T00:00:00Z" even though the column only
// holds a date, so we trim it back to the 10-char form the API expects.
func normalizeDateString(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// jobToResponse maps a model to a DTO response.
func (s *RadioService) jobToResponse(job *catalogm.RadioImportJob, showName, stationName string) *contracts.RadioImportJobResponse {
	return &contracts.RadioImportJobResponse{
		ID:                 job.ID,
		ShowID:             job.ShowID,
		ShowName:           showName,
		StationID:          job.StationID,
		StationName:        stationName,
		Since:              normalizeDateString(job.Since),
		Until:              normalizeDateString(job.Until),
		Status:             job.Status,
		EpisodesFound:      job.EpisodesFound,
		EpisodesImported:   job.EpisodesImported,
		PlaysImported:      job.PlaysImported,
		PlaysMatched:       job.PlaysMatched,
		CurrentEpisodeDate: job.CurrentEpisodeDate,
		ErrorLog:           job.ErrorLog,
		StartedAt:          job.StartedAt,
		CompletedAt:        job.CompletedAt,
		CreatedAt:          job.CreatedAt,
		UpdatedAt:          job.UpdatedAt,
	}
}

// ListAllActiveJobs returns all running and pending import jobs.
func (s *RadioService) ListAllActiveJobs() ([]*contracts.RadioImportJobResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var jobs []catalogm.RadioImportJob
	if err := s.db.Preload("Show").Preload("Station").
		Where("status IN ?", []string{
			catalogm.RadioImportJobStatusPending,
			catalogm.RadioImportJobStatusRunning,
		}).
		Order("created_at DESC").
		Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("listing active import jobs: %w", err)
	}

	results := make([]*contracts.RadioImportJobResponse, len(jobs))
	for i, job := range jobs {
		results[i] = s.jobToResponse(&job, job.Show.Name, job.Station.Name)
	}

	return results, nil
}

// isJobCancelled checks if a job has been cancelled.
func (s *RadioService) isJobCancelled(jobID uint) bool {
	var job catalogm.RadioImportJob
	if err := s.db.Select("status").First(&job, jobID).Error; err != nil {
		return false
	}
	return job.Status == catalogm.RadioImportJobStatusCancelled
}
