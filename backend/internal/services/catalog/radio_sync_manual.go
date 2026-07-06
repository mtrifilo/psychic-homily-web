package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// radio_sync_manual.go is the admin-facing surface of the unified ingestion core
// (PSY-1135, P2 PR2). It collapses the old /fetch, /discover, /import, and
// /import-job endpoints onto RunStationSync: two async triggers (station-scoped
// discover/fetch, show-scoped backfill) that open a radio_sync_runs row and run
// in the background, plus a poll and a cancel keyed on the run id. The old
// radio_import_jobs machinery is retired; radio_sync_runs is the single trace.

// TriggerStationSync starts a manual station-scoped sync (discover or fetch) and
// returns the opened run for polling. The run executes in the background; this
// returns as soon as the radio_sync_runs row exists. A manual trigger bypasses
// the persistent breaker (operator override, enforced inside RunStationSync) and
// returns a 409-mapped ErrRadioSyncAlreadyRunning if another run holds the
// station's sync lock.
func (s *RadioService) TriggerStationSync(stationID uint, mode string) (*contracts.RadioSyncRunResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	// Backfill is show-scoped (TriggerShowBackfill); rematch is global. Only the
	// two station-scoped modes are valid here.
	if mode != catalogm.RadioSyncRunTypeDiscover && mode != catalogm.RadioSyncRunTypeFetch {
		return nil, fmt.Errorf("invalid station sync mode %q (want %q or %q)",
			mode, catalogm.RadioSyncRunTypeDiscover, catalogm.RadioSyncRunTypeFetch)
	}

	// Synchronous existence check so a missing station is a clean 404 rather than a
	// background failure surfaced as a generic error.
	if err := s.assertStationExists(stationID); err != nil {
		return nil, err
	}

	runID, err := s.startAsyncSync(stationID, RunStationSyncOpts{Mode: mode})
	if err != nil {
		return nil, err
	}
	return s.settleTriggeredRun(runID, &contracts.RadioSyncRunResponse{
		StationID: radioSyncStationID(stationID),
		RunType:   mode,
	})
}

// TriggerShowBackfill starts a manual historic re-ingestion of one show over
// [since, until] and returns the opened run for polling. Replaces the old
// import-job create+start. since/until are YYYY-MM-DD; the station id is derived
// from the show (the run + advisory lock are per-station).
func (s *RadioService) TriggerShowBackfill(showID uint, since, until string) (*contracts.RadioSyncRunResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	windowStart, err := time.Parse("2006-01-02", since)
	if err != nil {
		return nil, fmt.Errorf("invalid since date %q (expected YYYY-MM-DD): %w", since, err)
	}
	windowEnd, err := time.Parse("2006-01-02", until)
	if err != nil {
		return nil, fmt.Errorf("invalid until date %q (expected YYYY-MM-DD): %w", until, err)
	}

	// Load the show to derive the station id and 404 cleanly on a bad id.
	var show catalogm.RadioShow
	if err := s.db.Select("id", "station_id").First(&show, showID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRadioShowNotFound(showID)
		}
		return nil, fmt.Errorf("loading show: %w", err)
	}

	runID, err := s.startAsyncSync(show.StationID, RunStationSyncOpts{
		Mode:        catalogm.RadioSyncRunTypeBackfill,
		ShowID:      &showID,
		WindowStart: &windowStart,
		WindowEnd:   &windowEnd,
	})
	if err != nil {
		return nil, err
	}
	ws, we := since, until
	return s.settleTriggeredRun(runID, &contracts.RadioSyncRunResponse{
		StationID: radioSyncStationID(show.StationID),
		ShowID:    &showID,
		RunType:   catalogm.RadioSyncRunTypeBackfill,
		WindowStart: &ws,
		WindowEnd:   &we,
	})
}

// settleTriggeredRun reads the freshly-opened run for the trigger response. The
// run row already exists and is executing in the background, so a transient
// failure of THIS enrichment read must not be reported as a trigger failure (a
// 500 would tell the operator the sync didn't start when it did, and a retry then
// hits the still-held advisory lock → confusing 409). On a read error, return a
// minimal pollable handle (the run id + the known fields) with the running status.
func (s *RadioService) settleTriggeredRun(runID uint, fallback *contracts.RadioSyncRunResponse) (*contracts.RadioSyncRunResponse, error) {
	run, err := s.GetSyncRun(runID)
	if err != nil {
		slog.Warn("radio: sync run started but initial read failed; returning minimal handle",
			"run_id", runID, "error", err)
		fallback.ID = runID
		fallback.Trigger = catalogm.RadioSyncRunTriggerManual
		fallback.Status = catalogm.RadioSyncRunStatusRunning
		return fallback, nil
	}
	return run, nil
}

// startAsyncSync runs RunStationSync(trigger=manual) in the background and returns
// the new run id as soon as the row is opened (via OnRunOpened), so the HTTP
// caller gets a poll handle without waiting for the run to finish. It maps the two
// no-row outcomes to errors: lock contention → ErrRadioSyncAlreadyRunning (409),
// and a pre-open failure (validation / station-not-found / lock-acquire / open) →
// the underlying error. Because OnRunOpened fires synchronously inside
// RunStationSync BEFORE the mode executes, the select below resolves the instant
// the row exists — it never blocks through the run body, and a panic in the
// executor (after the row opens) is recovered by GoSafe while RunStationSync's own
// defer fails the run. A deferred guard in the worker guarantees exactly one of
// the two channels is always written even if RunStationSync panics BEFORE opening
// the row — otherwise the select (and the HTTP caller) would block forever.
func (s *RadioService) startAsyncSync(stationID uint, opts RunStationSyncOpts) (uint, error) {
	opts.Trigger = catalogm.RadioSyncRunTriggerManual

	openedCh := make(chan uint, 1)
	failCh := make(chan error, 1)

	shared.GoSafe(context.Background(), "radio_manual_sync", func() {
		opened := false
		opts.OnRunOpened = func(runID uint) {
			opened = true
			openedCh <- runID
		}
		// If we leave without the row having opened — a pre-open error path that
		// somehow didn't signal, or a panic before OnRunOpened — release the
		// waiting caller. Non-blocking: if the switch below already wrote failCh,
		// this is a no-op (buffer full); the real error wins.
		defer func() {
			if !opened {
				select {
				case failCh <- fmt.Errorf("radio sync failed to start for station %d", stationID):
				default:
				}
			}
		}()

		res, err := s.RunStationSync(context.Background(), stationID, opts)
		switch {
		case res != nil && res.LockContended:
			failCh <- apperrors.ErrRadioSyncAlreadyRunning(stationID)
		case res == nil && err != nil:
			// Failed before the row opened (bad opts, station gone, lock-acquire
			// error, or the open INSERT itself failed). OnRunOpened never fired.
			failCh <- err
		default:
			// Row opened — its id is already on openedCh. Whatever the run's outcome
			// (success/partial/failed/cancelled, or an opened-then-close error), the
			// caller polls for it; nothing to report here.
		}
	})

	select {
	case runID := <-openedCh:
		return runID, nil
	case err := <-failCh:
		return 0, err
	}
}

// GetSyncRun returns a single sync run by id (poll endpoint). Preloads the
// station/show names and the categorized error list. Maps a missing id to a
// 404-mapped ErrRadioSyncRunNotFound.
func (s *RadioService) GetSyncRun(runID uint) (*contracts.RadioSyncRunResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var run catalogm.RadioSyncRun
	if err := s.db.
		Preload("Station").
		Preload("Show").
		Preload("Errors").
		First(&run, runID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRadioSyncRunNotFound(runID)
		}
		return nil, fmt.Errorf("loading sync run: %w", err)
	}

	return syncRunToResponse(&run), nil
}

// CancelSyncRun flips a running sync run to 'cancelled'. The flip is a single
// conditional UPDATE (WHERE status='running') so it races cleanly against the
// run's own terminal close in RunStationSync — exactly one wins, and the
// in-flight backfill's progressFn observes the cancel and stops early. Returns
// ErrRadioSyncRunNotFound (404) for a missing id and ErrRadioSyncNotCancellable
// (409) for a run that has already reached a terminal status.
func (s *RadioService) CancelSyncRun(runID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	res := s.db.Model(&catalogm.RadioSyncRun{}).
		Where("id = ? AND status = ?", runID, catalogm.RadioSyncRunStatusRunning).
		Updates(map[string]any{
			// finished_at is required by the lifecycle CHECK once status is terminal.
			"status":      catalogm.RadioSyncRunStatusCancelled,
			"finished_at": now,
			"updated_at":  now,
		})
	if res.Error != nil {
		return fmt.Errorf("cancelling sync run: %w", res.Error)
	}
	if res.RowsAffected == 1 {
		return nil
	}

	// 0 rows updated: the run is either gone or already terminal. Distinguish so
	// the handler can return a precise 404 vs 409.
	var run catalogm.RadioSyncRun
	if err := s.db.Select("status").First(&run, runID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrRadioSyncRunNotFound(runID)
		}
		return fmt.Errorf("loading sync run: %w", err)
	}
	return apperrors.ErrRadioSyncNotCancellable(runID, run.Status)
}

// assertStationExists returns a 404-mapped error if the station id is unknown.
func (s *RadioService) assertStationExists(stationID uint) error {
	var count int64
	if err := s.db.Model(&catalogm.RadioStation{}).Where("id = ?", stationID).Count(&count).Error; err != nil {
		return fmt.Errorf("checking station: %w", err)
	}
	if count == 0 {
		return apperrors.ErrRadioStationNotFound(stationID)
	}
	return nil
}

// syncRunToResponse maps a radio_sync_runs row (with Station/Show/Errors
// preloaded) onto its DTO. Window timestamps render as YYYY-MM-DD to match the
// old import-job since/until wire format; show fields are nil for station-scoped
// runs.
func syncRunToResponse(run *catalogm.RadioSyncRun) *contracts.RadioSyncRunResponse {
	resp := &contracts.RadioSyncRunResponse{
		ID:                 run.ID,
		RunType:            run.RunType,
		Trigger:            run.Trigger,
		Status:             run.Status,
		EpisodesFound:      run.EpisodesFound,
		EpisodesImported:   run.EpisodesImported,
		PlaysImported:      run.PlaysImported,
		PlaysMatched:       run.PlaysMatched,
		PlaysUnmatched:     run.PlaysUnmatched,
		CurrentEpisodeDate: run.CurrentEpisodeDate,
		BreakerSkipped:     run.BreakerSkipped,
		StartedAt:          run.StartedAt,
		FinishedAt:         run.FinishedAt,
		CreatedAt:          run.CreatedAt,
		UpdatedAt:          run.UpdatedAt,
	}

	if run.StationID != nil {
		resp.StationID = run.StationID
		if run.Station.Name != "" {
			resp.StationName = run.Station.Name
		}
	}

	if run.ShowID != nil {
		resp.ShowID = run.ShowID
		if run.Show != nil {
			name := run.Show.Name
			resp.ShowName = &name
		}
	}
	if run.WindowStart != nil {
		ws := run.WindowStart.Format("2006-01-02")
		resp.WindowStart = &ws
	}
	if run.WindowEnd != nil {
		we := run.WindowEnd.Format("2006-01-02")
		resp.WindowEnd = &we
	}
	for _, e := range run.Errors {
		resp.Errors = append(resp.Errors, contracts.RadioSyncRunErrorResponse{
			Category:   e.Category,
			Detail:     e.Detail,
			EpisodeRef: e.EpisodeRef,
		})
	}

	return resp
}

// TriggerGlobalRematch is the contracts-facing entry for bulk async rematch (PSY-1364).
func (s *RadioService) TriggerGlobalRematch(req contracts.GlobalRematchRequest) (*contracts.RadioSyncRunResponse, error) {
	return s.startGlobalRematchJob(GlobalRematchOpts{
		StationID: req.StationID,
		ShowID:    req.ShowID,
		Force:     req.Force,
	})
}
