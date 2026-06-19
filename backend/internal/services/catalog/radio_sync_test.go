package catalog

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// RunStationSync — validation (no DB; all checks return before the lock)
// =============================================================================

func TestRunStationSync_Validation(t *testing.T) {
	ctx := context.Background()

	t.Run("nil db", func(t *testing.T) {
		_, err := (&RadioService{db: nil}).RunStationSync(ctx, 1, RunStationSyncOpts{
			Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
		})
		assert.Error(t, err)
	})

	// A non-nil but unused *gorm.DB: the validations below all return before any
	// DB access (the advisory lock), so the empty handle is never touched.
	svc := &RadioService{db: &gorm.DB{}}

	t.Run("invalid mode", func(t *testing.T) {
		_, err := svc.RunStationSync(ctx, 1, RunStationSyncOpts{
			Mode: "bogus", Trigger: catalogm.RadioSyncRunTriggerScheduled,
		})
		assert.ErrorContains(t, err, "invalid sync run mode")
	})

	t.Run("rematch rejected (deferred from per-station orchestrator)", func(t *testing.T) {
		_, err := svc.RunStationSync(ctx, 1, RunStationSyncOpts{
			Mode: catalogm.RadioSyncRunTypeRematch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
		})
		assert.ErrorContains(t, err, "rematch")
	})

	t.Run("invalid trigger", func(t *testing.T) {
		_, err := svc.RunStationSync(ctx, 1, RunStationSyncOpts{
			Mode: catalogm.RadioSyncRunTypeFetch, Trigger: "bogus",
		})
		assert.ErrorContains(t, err, "invalid sync run trigger")
	})

	t.Run("backfill requires show + window", func(t *testing.T) {
		_, err := svc.RunStationSync(ctx, 1, RunStationSyncOpts{
			Mode: catalogm.RadioSyncRunTypeBackfill, Trigger: catalogm.RadioSyncRunTriggerManual,
		})
		assert.ErrorContains(t, err, "backfill requires")
	})
}

// =============================================================================
// terminalStatus
// =============================================================================

func TestTerminalStatus(t *testing.T) {
	assert.Equal(t, catalogm.RadioSyncRunStatusFailed, terminalStatus(true, 0))
	assert.Equal(t, catalogm.RadioSyncRunStatusFailed, terminalStatus(true, 5))
	assert.Equal(t, catalogm.RadioSyncRunStatusPartial, terminalStatus(false, 1))
	assert.Equal(t, catalogm.RadioSyncRunStatusSuccess, terminalStatus(false, 0))
}

// =============================================================================
// importResultOutcome — count mapping
// =============================================================================

func TestImportResultOutcome(t *testing.T) {
	t.Run("clean success derives unmatched", func(t *testing.T) {
		out := importResultOutcome(&contracts.RadioImportResult{
			EpisodesImported: 3, PlaysImported: 50, PlaysMatched: 40,
		}, 5)
		assert.Equal(t, catalogm.RadioSyncRunStatusSuccess, out.status)
		assert.Equal(t, 5, out.episodesFound)
		assert.Equal(t, 3, out.episodesImported)
		assert.Equal(t, 50, out.playsImported)
		assert.Equal(t, 40, out.playsMatched)
		assert.Equal(t, 10, out.playsUnmatched) // 50 - 40
		assert.NotNil(t, out.importResult)
	})

	t.Run("errors -> partial", func(t *testing.T) {
		out := importResultOutcome(&contracts.RadioImportResult{
			PlaysImported: 10, PlaysMatched: 10, EpisodeFetchErrors: 1,
		}, 0)
		assert.Equal(t, catalogm.RadioSyncRunStatusPartial, out.status)
		assert.Equal(t, 0, out.playsUnmatched) // never negative
	})

	t.Run("matched exceeding imported clamps unmatched to 0", func(t *testing.T) {
		out := importResultOutcome(&contracts.RadioImportResult{PlaysImported: 5, PlaysMatched: 9}, 0)
		assert.Equal(t, 0, out.playsUnmatched)
	})
}

// =============================================================================
// error categorization
// =============================================================================

func TestCategorizeRunError(t *testing.T) {
	assert.Equal(t, catalogm.RadioSyncRunErrorTimeout, categorizeRunError(context.DeadlineExceeded))
	assert.Equal(t, catalogm.RadioSyncRunErrorRateLimited,
		categorizeRunError(&RadioHTTPError{Provider: "KEXP", StatusCode: http.StatusTooManyRequests}))
	assert.Equal(t, catalogm.RadioSyncRunErrorProviderUnreachable,
		categorizeRunError(&RadioHTTPError{Provider: "KEXP", StatusCode: http.StatusInternalServerError}))
	assert.Equal(t, catalogm.RadioSyncRunErrorProviderUnreachable, categorizeRunError(errors.New("boom")))
	// wrapped deadline still detected
	assert.Equal(t, catalogm.RadioSyncRunErrorTimeout,
		categorizeRunError(errors.Join(errors.New("fetch"), context.DeadlineExceeded)))
}

func TestCategorizeErrorString(t *testing.T) {
	cases := map[string]string{
		"context deadline exceeded":        catalogm.RadioSyncRunErrorTimeout,
		"got 429 too many requests":        catalogm.RadioSyncRunErrorRateLimited,
		"failed to unmarshal json":         catalogm.RadioSyncRunErrorParseError,
		"dropped 2 plays: missing artist":  catalogm.RadioSyncRunErrorValidationDrop,
		"title truncated to fit":           catalogm.RadioSyncRunErrorTruncation,
		"match persist failed":             catalogm.RadioSyncRunErrorMatchPersistError,
		"some unrecognized provider error": catalogm.RadioSyncRunErrorProviderUnreachable,
	}
	for in, want := range cases {
		assert.Equal(t, want, categorizeErrorString(in), "input: %q", in)
	}
}

func TestTruncateForDetail(t *testing.T) {
	assert.Equal(t, "short", truncateForDetail("short"))

	markerRunes := utf8.RuneCountInString("…[truncated]")

	// ASCII over the limit: truncated to the rune budget + marker, valid UTF-8.
	got := truncateForDetail(strings.Repeat("x", runErrorDetailLimit+500))
	assert.True(t, utf8.ValidString(got))
	assert.Equal(t, runErrorDetailLimit+markerRunes, utf8.RuneCountInString(got))

	// Multi-byte runes straddling the limit must NOT be split mid-sequence — a
	// byte-slice would yield invalid UTF-8 that Postgres rejects on insert.
	for _, r := range []string{"é", "🎵"} { // 2-byte and 4-byte runes
		out := truncateForDetail(strings.Repeat(r, runErrorDetailLimit+50))
		assert.True(t, utf8.ValidString(out), "truncated %q detail must stay valid UTF-8", r)
		assert.Equal(t, runErrorDetailLimit+markerRunes, utf8.RuneCountInString(out))
	}
}
