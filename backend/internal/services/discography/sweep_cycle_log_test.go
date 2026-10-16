package discography

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testlog"
	"psychic-homily-backend/internal/testutil"
)

// A cycle that fails on a database error writes the record the inline
// logger.Error pattern writes from the sweep's runCycle: level, message, the
// error as the only attribute, and a source naming runCycle and its file.
func TestArtistDiscographySweep_FailedCycleLogsAtError(t *testing.T) {
	logger, got := testlog.NewJSONCapture(slog.LevelDebug)
	sweep := NewArtistDiscographySweep(testutil.UnavailableDB(t), fakeBrowser{}, fakeCoverArt{})
	sweep.logger = logger
	sweep.runCycle(context.Background())

	inline, want := testlog.NewJSONCaptureAs(slog.LevelDebug, (*ArtistDiscographySweep).runCycle)
	inline.Error("artist discography sweep: cycle failed", "error", errors.New("load artists with MBID: sql: database is closed"))
	assert.Equal(t, want.String(), got.String())
}

// A canceled cycle stops between artists and reports its batch, so it writes
// no Error record.
func (s *ArtistDiscographySweepIntegrationTestSuite) TestCanceledCycleLogsNoError() {
	mbid := "11111111-1111-1111-1111-111111111111"
	s.Require().NoError(s.db.Create(&catalogm.Artist{Name: "Canceled Band", MusicBrainzArtistID: &mbid}).Error)
	logger, got := testlog.NewJSONCapture(slog.LevelDebug)
	sweep := NewArtistDiscographySweep(s.db, fakeBrowser{}, fakeCoverArt{})
	sweep.logger = logger
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sweep.RunSweepNow(ctx)

	s.Contains(got.String(), "artist discography sweep batch done")
	s.NotContains(got.String(), `"level":"ERROR"`)
}
