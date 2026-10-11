package catalog

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/shared"
)

const (
	// RadioTracksRebuildLoopName is the loop's name in logs and its
	// background_service_runs primary key.
	RadioTracksRebuildLoopName = "radio_tracks_rebuild"

	// defaultRadioTracksRebuildInterval follows the station fetch loop's
	// default cadence, the rate at which new plays arrive in bulk.
	defaultRadioTracksRebuildInterval = DefaultRadioFetchInterval
)

// RadioTracksRebuildLoop rebuilds radio_tracks on a schedule. It runs no cycle
// on the boot path; RunScheduledLoop schedules the first cycle from persisted
// run state, so a deploy never resets its clock.
type RadioTracksRebuildLoop struct {
	db         *gorm.DB
	interval   time.Duration
	startDelay time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewRadioTracksRebuildLoop constructs the loop. RADIO_TRACKS_REBUILD_INTERVAL_HOURS
// and RADIO_TRACKS_REBUILD_START_DELAY_MINUTES override the defaults; the start
// delay applies only when no run state exists yet.
func NewRadioTracksRebuildLoop(database *gorm.DB) *RadioTracksRebuildLoop {
	if database == nil {
		database = db.GetDB()
	}
	return &RadioTracksRebuildLoop{
		db:         database,
		interval:   shared.EnvPositiveDuration("RADIO_TRACKS_REBUILD_INTERVAL_HOURS", time.Hour, defaultRadioTracksRebuildInterval),
		startDelay: shared.EnvPositiveDuration("RADIO_TRACKS_REBUILD_START_DELAY_MINUTES", time.Minute, 0),
		stopCh:     make(chan struct{}),
		logger:     slog.Default(),
	}
}

// Start begins the loop.
func (l *RadioTracksRebuildLoop) Start(ctx context.Context) {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		shared.RunScheduledLoop(ctx, shared.LoopConfig{
			Name:       RadioTracksRebuildLoopName,
			Interval:   l.interval,
			StopCh:     l.stopCh,
			StartDelay: l.startDelay,
		}, l.runCycle)
	}()
	l.logger.Info("radio tracks rebuild loop started", "interval", l.interval)
}

// Stop stops the loop and waits for an in-flight cycle to return.
func (l *RadioTracksRebuildLoop) Stop() {
	close(l.stopCh)
	l.wg.Wait()
	l.logger.Info("radio tracks rebuild loop stopped")
}

// runCycle runs one rebuild and reports its track count to the run state.
func (l *RadioTracksRebuildLoop) runCycle(ctx context.Context) {
	start := time.Now()
	report, err := RebuildRadioTracks(ctx, l.db)
	if err != nil {
		logFn := l.logger.Error
		if errors.Is(err, context.Canceled) {
			logFn = l.logger.Info
		}
		logFn("radio tracks rebuild: cycle failed", "error", err)
		shared.ReportCycle(ctx, 0, err)
		return
	}
	if report.Skipped {
		l.logger.Info("radio tracks rebuild: another rebuild is running; skipped")
		shared.ReportCycle(ctx, 0, nil)
		return
	}
	shared.ReportCycle(ctx, int(report.Tracks), nil)
	l.logger.Info("radio tracks rebuild: cycle complete",
		"plays", report.Plays,
		"unkeyed_plays", report.UnkeyedPlays,
		"tracks", report.Tracks,
		"keys", report.Keys,
		"joined_keys", report.JoinedKeys,
		"written", report.Written,
		"deleted", report.Deleted,
		"duration", time.Since(start),
	)
}
