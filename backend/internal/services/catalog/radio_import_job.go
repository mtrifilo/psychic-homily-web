package catalog

import (
	"fmt"
	"log/slog"
	"time"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// CreateImportJob creates a new pending import job for a radio show.
// Validates that no other job is currently running for the same show.
func (s *RadioService) CreateImportJob(showID uint, since, until string) (*contracts.RadioImportJobResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Validate show exists and get station ID
	var show models.RadioShow
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
	s.db.Model(&models.RadioImportJob{}).
		Where("show_id = ? AND status IN ?", showID, []string{
			models.RadioImportJobStatusPending,
			models.RadioImportJobStatusRunning,
		}).
		Count(&activeCount)
	if activeCount > 0 {
		return nil, fmt.Errorf("an import job is already running or pending for this show")
	}

	job := &models.RadioImportJob{
		ShowID:    showID,
		StationID: show.StationID,
		Since:     since,
		Until:     until,
		Status:    models.RadioImportJobStatusPending,
	}

	if err := s.db.Create(job).Error; err != nil {
		return nil, fmt.Errorf("creating import job: %w", err)
	}

	return s.jobToResponse(job, show.Name, show.Station.Name), nil
}

// StartImportJob transitions a pending job to running and launches the background goroutine.
func (s *RadioService) StartImportJob(jobID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var job models.RadioImportJob
	if err := s.db.First(&job, jobID).Error; err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	if job.Status != models.RadioImportJobStatusPending {
		return fmt.Errorf("job is not in pending status (current: %s)", job.Status)
	}

	now := time.Now()
	s.db.Model(&job).Updates(map[string]interface{}{
		"status":     models.RadioImportJobStatusRunning,
		"started_at": now,
	})

	// Launch the import goroutine
	go s.runImportJob(job.ID)

	return nil
}

// CancelImportJob sets a running or pending job to cancelled.
// If the job is running, the goroutine will check status periodically and stop.
func (s *RadioService) CancelImportJob(jobID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var job models.RadioImportJob
	if err := s.db.First(&job, jobID).Error; err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	if job.Status != models.RadioImportJobStatusRunning && job.Status != models.RadioImportJobStatusPending {
		return fmt.Errorf("job cannot be cancelled (current status: %s)", job.Status)
	}

	now := time.Now()
	s.db.Model(&job).Updates(map[string]interface{}{
		"status":       models.RadioImportJobStatusCancelled,
		"completed_at": now,
	})

	return nil
}

// GetImportJob returns a single import job by ID with show/station names.
func (s *RadioService) GetImportJob(jobID uint) (*contracts.RadioImportJobResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var job models.RadioImportJob
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

	var jobs []models.RadioImportJob
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
func (s *RadioService) runImportJob(jobID uint) {
	logger := slog.Default().With("job_id", jobID)
	logger.Info("radio_import_job_started")

	// Reload job from DB
	var job models.RadioImportJob
	if err := s.db.Preload("Show").Preload("Show.Station").First(&job, jobID).Error; err != nil {
		logger.Error("radio_import_job_load_failed", "error", err.Error())
		return
	}

	station := job.Show.Station
	if station.PlaylistSource == nil || *station.PlaylistSource == "" {
		s.failJob(jobID, "station has no playlist source configured")
		return
	}

	provider, err := s.getProvider(*station.PlaylistSource)
	if err != nil {
		s.failJob(jobID, fmt.Sprintf("getting provider: %v", err))
		return
	}
	defer closeProvider(provider)

	// Parse date range
	sinceTime, err := time.Parse("2006-01-02", job.Since)
	if err != nil {
		s.failJob(jobID, fmt.Sprintf("parsing since date: %v", err))
		return
	}
	untilTime, err := time.Parse("2006-01-02", job.Until)
	if err != nil {
		s.failJob(jobID, fmt.Sprintf("parsing until date: %v", err))
		return
	}

	// Get external ID for the show
	if job.Show.ExternalID == nil || *job.Show.ExternalID == "" {
		s.failJob(jobID, "show has no external ID")
		return
	}

	// Fetch episodes from provider
	episodes, err := provider.FetchNewEpisodes(*job.Show.ExternalID, sinceTime)
	if err != nil {
		s.failJob(jobID, fmt.Sprintf("fetching episodes: %v", err))
		return
	}

	// Filter episodes to the date range
	var filtered []RadioEpisodeImport
	for _, ep := range episodes {
		epDate, parseErr := time.Parse("2006-01-02", ep.AirDate)
		if parseErr != nil {
			continue
		}
		if !epDate.Before(sinceTime) && !epDate.After(untilTime) {
			filtered = append(filtered, ep)
		}
	}

	// Update episodes found count
	s.db.Model(&models.RadioImportJob{}).Where("id = ?", jobID).
		Update("episodes_found", len(filtered))

	logger.Info("radio_import_job_episodes_found",
		"total_from_provider", len(episodes),
		"in_date_range", len(filtered),
	)

	var (
		totalPlaysImported int
		totalPlaysMatched  int
		episodesImported   int
		errorMessages      []string
	)

	for i, ep := range filtered {
		// Check for cancellation every 5 episodes
		if i > 0 && i%5 == 0 {
			var currentJob models.RadioImportJob
			if err := s.db.Select("status").First(&currentJob, jobID).Error; err == nil {
				if currentJob.Status == models.RadioImportJobStatusCancelled {
					logger.Info("radio_import_job_cancelled", "episodes_processed", i)
					return
				}
			}
		}

		// Import the episode
		epResult, importErr := s.importEpisode(job.ShowID, ep, provider)
		if importErr != nil {
			errorMessages = append(errorMessages, fmt.Sprintf("episode %s: %v", ep.AirDate, importErr))
			continue
		}

		episodesImported++
		totalPlaysImported += epResult.PlaysImported
		totalPlaysMatched += epResult.PlaysMatched

		// Batch update progress every 10 episodes
		if i > 0 && i%10 == 0 {
			currentDate := ep.AirDate
			s.db.Model(&models.RadioImportJob{}).Where("id = ?", jobID).
				Updates(map[string]interface{}{
					"episodes_imported":    episodesImported,
					"plays_imported":       totalPlaysImported,
					"plays_matched":        totalPlaysMatched,
					"current_episode_date": currentDate,
				})
		}
	}

	// Final update: mark completed
	now := time.Now()
	updates := map[string]interface{}{
		"status":            models.RadioImportJobStatusCompleted,
		"episodes_imported": episodesImported,
		"plays_imported":    totalPlaysImported,
		"plays_matched":     totalPlaysMatched,
		"completed_at":      now,
	}

	if len(errorMessages) > 0 {
		errorLog := ""
		for _, msg := range errorMessages {
			errorLog += msg + "\n"
		}
		updates["error_log"] = errorLog
	}

	// Set current_episode_date to the last processed episode
	if len(filtered) > 0 {
		updates["current_episode_date"] = filtered[len(filtered)-1].AirDate
	}

	s.db.Model(&models.RadioImportJob{}).Where("id = ?", jobID).Updates(updates)

	logger.Info("radio_import_job_completed",
		"episodes_imported", episodesImported,
		"plays_imported", totalPlaysImported,
		"plays_matched", totalPlaysMatched,
		"errors", len(errorMessages),
	)
}

// failJob marks a job as failed with an error message.
func (s *RadioService) failJob(jobID uint, errMsg string) {
	now := time.Now()
	s.db.Model(&models.RadioImportJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":       models.RadioImportJobStatusFailed,
		"error_log":    errMsg,
		"completed_at": now,
	})
	slog.Default().Error("radio_import_job_failed", "job_id", jobID, "error", errMsg)
}

// jobToResponse maps a model to a DTO response.
func (s *RadioService) jobToResponse(job *models.RadioImportJob, showName, stationName string) *contracts.RadioImportJobResponse {
	return &contracts.RadioImportJobResponse{
		ID:                 job.ID,
		ShowID:             job.ShowID,
		ShowName:           showName,
		StationID:          job.StationID,
		StationName:        stationName,
		Since:              job.Since,
		Until:              job.Until,
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

	var jobs []models.RadioImportJob
	if err := s.db.Preload("Show").Preload("Station").
		Where("status IN ?", []string{
			models.RadioImportJobStatusPending,
			models.RadioImportJobStatusRunning,
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
	var job models.RadioImportJob
	if err := s.db.Select("status").First(&job, jobID).Error; err != nil {
		return false
	}
	return job.Status == models.RadioImportJobStatusCancelled
}

