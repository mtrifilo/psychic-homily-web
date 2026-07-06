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

// GlobalRematchOpts scopes an async bulk rematch job. All fields optional —
// empty opts rematch every distinct unmatched artist name in the archive.
type GlobalRematchOpts struct {
	StationID *uint
	ShowID    *uint
}

// startGlobalRematchJob opens a radio_sync_runs row (run_type=rematch) and
// executes ReMatchUnmatchedChunked in the background. Returns the poll handle
// immediately (PSY-1364). Only one rematch run may be in flight at a time.
func (s *RadioService) startGlobalRematchJob(opts GlobalRematchOpts) (*contracts.RadioSyncRunResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if err := s.validateGlobalRematchOpts(opts); err != nil {
		return nil, err
	}

	if err := s.assertNoRunningRematch(); err != nil {
		return nil, err
	}

	run, err := s.openRematchRun(opts)
	if err != nil {
		return nil, err
	}

	filter := UnmatchedArtistNameFilter{
		StationID: opts.StationID,
		ShowID:    opts.ShowID,
	}
	runID := run.ID

	shared.GoSafe(context.Background(), "radio_global_rematch", func() {
		ctx := context.Background()
		agg, runErr := s.ReMatchUnmatchedChunked(ctx, defaultReMatchNamePageSize, filter, runID)
		s.finishRematchRun(runID, agg, runErr)
	})

	return s.settleTriggeredRun(runID, syncRunToResponse(run))
}

func (s *RadioService) validateGlobalRematchOpts(opts GlobalRematchOpts) error {
	if opts.ShowID != nil {
		var show catalogm.RadioShow
		if err := s.db.Select("id", "station_id").First(&show, *opts.ShowID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.ErrRadioShowNotFound(*opts.ShowID)
			}
			return fmt.Errorf("loading show: %w", err)
		}
		if opts.StationID != nil && show.StationID != *opts.StationID {
			return fmt.Errorf("show %d is not on station %d", *opts.ShowID, *opts.StationID)
		}
	}
	if opts.StationID != nil {
		if err := s.assertStationExists(*opts.StationID); err != nil {
			return err
		}
	}
	return nil
}

func (s *RadioService) assertNoRunningRematch() error {
	var count int64
	if err := s.db.Model(&catalogm.RadioSyncRun{}).
		Where("run_type = ? AND status = ?", catalogm.RadioSyncRunTypeRematch, catalogm.RadioSyncRunStatusRunning).
		Count(&count).Error; err != nil {
		return fmt.Errorf("checking running rematch runs: %w", err)
	}
	if count > 0 {
		return apperrors.ErrRadioRematchAlreadyRunning()
	}
	return nil
}

func (s *RadioService) openRematchRun(opts GlobalRematchOpts) (*catalogm.RadioSyncRun, error) {
	now := time.Now()
	run := catalogm.RadioSyncRun{
		StationID: opts.StationID,
		ShowID:    opts.ShowID,
		RunType:   catalogm.RadioSyncRunTypeRematch,
		Trigger:   catalogm.RadioSyncRunTriggerManual,
		Status:    catalogm.RadioSyncRunStatusRunning,
		StartedAt: now,
	}
	if err := s.db.Create(&run).Error; err != nil {
		return nil, fmt.Errorf("open rematch run: %w", err)
	}
	return &run, nil
}

func (s *RadioService) finishRematchRun(runID uint, agg *ReMatchChunkedResult, runErr error) {
	if agg == nil {
		agg = &ReMatchChunkedResult{}
	}

	if s.isSyncRunCancelled(runID) {
		return
	}

	status := catalogm.RadioSyncRunStatusSuccess
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) {
			// Admin cancel already flipped the row; don't overwrite.
			if s.isSyncRunCancelled(runID) {
				return
			}
			status = catalogm.RadioSyncRunStatusCancelled
		} else {
			status = catalogm.RadioSyncRunStatusFailed
			slog.Error("global rematch run failed", "run_id", runID, "error", runErr)
		}
	}

	now := time.Now()
	res := s.db.Model(&catalogm.RadioSyncRun{}).
		Where("id = ? AND status = ?", runID, catalogm.RadioSyncRunStatusRunning).
		Updates(map[string]any{
			"status":           status,
			"finished_at":      now,
			"plays_matched":    agg.Matched,
			"plays_unmatched":  agg.Unmatched,
			"updated_at":       now,
		})
	if res.Error != nil {
		slog.Error("close rematch run failed", "run_id", runID, "error", res.Error)
	}
}

func (s *RadioService) writeRematchRunProgress(runID uint, agg *ReMatchChunkedResult) {
	if runID == 0 || agg == nil {
		return
	}
	s.db.Model(&catalogm.RadioSyncRun{}).
		Where("id = ? AND status = ?", runID, catalogm.RadioSyncRunStatusRunning).
		Updates(map[string]any{
			"plays_matched":   agg.Matched,
			"plays_unmatched": agg.Unmatched,
			"updated_at":      time.Now(),
		})
}
