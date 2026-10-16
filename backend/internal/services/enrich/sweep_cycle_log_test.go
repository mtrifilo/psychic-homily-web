package enrich

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// A cycle that fails on a database error writes the record the inline
// logger.Error pattern writes from the sweep's runCycle: level, message, the
// error as the only attribute, and a source naming runCycle and its file.

func TestArtistLocationSweep_FailedCycleLogsAtError(t *testing.T) {
	logger, got := testutil.NewLogCapture(slog.LevelDebug)
	sweep := NewArtistLocationSweep(testutil.UnavailableDB(t), fakeBandcamp{}, &fakeMB{})
	sweep.logger = logger
	sweep.runCycle(context.Background())

	inline, want := testutil.NewLogCaptureAs(slog.LevelDebug, (*ArtistLocationSweep).runCycle)
	inline.Error("artist location sweep: cycle failed", "error", errors.New("load artists: sql: database is closed"))
	assert.Equal(t, want.String(), got.String())
}

func TestArtistLinksSweep_FailedCycleLogsAtError(t *testing.T) {
	logger, got := testutil.NewLogCapture(slog.LevelDebug)
	sweep := NewArtistLinksSweep(testutil.UnavailableDB(t), &fakeLinksMB{}, &recordingLinksWriter{})
	sweep.logger = logger
	sweep.runCycle(context.Background())

	inline, want := testutil.NewLogCaptureAs(slog.LevelDebug, (*ArtistLinksSweep).runCycle)
	inline.Error("artist links sweep: cycle failed", "error", errors.New("load artists needing links: sql: database is closed"))
	assert.Equal(t, want.String(), got.String())
}

func TestReleaseLinksSweep_FailedCycleLogsAtError(t *testing.T) {
	logger, got := testutil.NewLogCapture(slog.LevelDebug)
	sweep := NewReleaseLinksSweep(testutil.UnavailableDB(t), &fakeReleaseLinksMB{}, &fakeLinkWriter{})
	sweep.logger = logger
	sweep.runCycle(context.Background())

	inline, want := testutil.NewLogCaptureAs(slog.LevelDebug, (*ReleaseLinksSweep).runCycle)
	inline.Error("release links sweep: cycle failed", "error", errors.New("load candidate releases: sql: database is closed"))
	assert.Equal(t, want.String(), got.String())
}

// canceledContext stands in for a sweep's context after a shutdown canceled it.
func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// A canceled cycle stops between artists and reports its batch, so it writes
// no Error record.
func (s *ArtistLocationSweepIntegrationTestSuite) TestCanceledCycleLogsNoError() {
	s.Require().NoError(s.db.Create(&catalogm.Artist{Name: "Canceled Band"}).Error)
	logger, got := testutil.NewLogCapture(slog.LevelDebug)
	sweep := NewArtistLocationSweep(s.db, fakeBandcamp{}, &fakeMB{})
	sweep.logger = logger

	sweep.RunSweepNow(canceledContext())

	s.Contains(got.String(), "artist location sweep batch done")
	s.NotContains(got.String(), `"level":"ERROR"`)
}

func (s *ArtistLinksSweepIntegrationTestSuite) TestCanceledCycleLogsNoError() {
	mbid := "11111111-1111-1111-1111-111111111111"
	s.Require().NoError(s.db.Create(&catalogm.Artist{Name: "Canceled Band", MusicBrainzArtistID: &mbid}).Error)
	logger, got := testutil.NewLogCapture(slog.LevelDebug)
	sweep := NewArtistLinksSweep(s.db, &fakeLinksMB{}, &recordingLinksWriter{db: s.db})
	sweep.logger = logger

	sweep.RunSweepNow(canceledContext())

	s.Contains(got.String(), "artist links sweep batch done")
	s.NotContains(got.String(), `"level":"ERROR"`)
}

// A canceled cycle returns the context's error, which logs at Info.
func (s *ReleaseLinksIntegrationTestSuite) TestCanceledCycleLogsAtInfo() {
	s.seedRelease("Canceled", "canceled", rgptr(1))
	logger, got := testutil.NewLogCapture(slog.LevelDebug)
	sweep := NewReleaseLinksSweep(s.db, &fakeReleaseLinksMB{}, &fakeLinkWriter{})
	sweep.logger = logger

	sweep.RunSweepNow(canceledContext())

	inline, want := testutil.NewLogCaptureAs(slog.LevelDebug, (*ReleaseLinksSweep).runCycle)
	inline.Info("release links sweep: cycle failed", "error", context.Canceled)
	s.Equal(want.String(), got.String())
}
