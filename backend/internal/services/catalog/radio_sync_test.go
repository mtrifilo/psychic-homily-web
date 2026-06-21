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

	t.Run("categorized errors -> partial", func(t *testing.T) {
		// importResultOutcome flips to partial on len(CategorizedErrors), and the
		// structured category flows straight through with no re-categorization
		// (PSY-1141). accumulateEpisodeResult records one categorized error per
		// per-episode failure, parallel to the human Errors line.
		out := importResultOutcome(&contracts.RadioImportResult{
			PlaysImported: 10, PlaysMatched: 10,
			EpisodeFetchErrors: 1,
			Errors:             []string{"fetch failed for episode ep-1: boom"},
			CategorizedErrors: []contracts.RadioRunError{
				{Category: catalogm.RadioSyncRunErrorProviderUnreachable, Detail: "fetch failed for episode ep-1: boom"},
			},
		}, 0)
		assert.Equal(t, catalogm.RadioSyncRunStatusPartial, out.status)
		assert.Equal(t, 0, out.playsUnmatched) // never negative
		assert.Len(t, out.errs, 1)
		assert.Equal(t, catalogm.RadioSyncRunErrorProviderUnreachable, out.errs[0].category)
	})

	t.Run("matched exceeding imported clamps unmatched to 0", func(t *testing.T) {
		out := importResultOutcome(&contracts.RadioImportResult{PlaysImported: 5, PlaysMatched: 9}, 0)
		assert.Equal(t, 0, out.playsUnmatched)
	})
}

// =============================================================================
// error categorization
// =============================================================================

// accumulateEpisodeResult categorizes per-episode signals (PSY-1141). Pure (no DB):
// covers the drop precedence (validation_drop outranks truncation), the FetchError
// category + its empty-fallback, and the Errors/CategorizedErrors 1:1 invariant.
func TestAccumulateEpisodeResult(t *testing.T) {
	t.Run("truncation-only records truncation + episode ref", func(t *testing.T) {
		res := &contracts.RadioImportResult{}
		accumulateEpisodeResult(res, "ep-1", &contracts.EpisodeImportResult{
			DropSummary: "dropped 1 plays: 1 over-length titles truncated", TruncatedPlays: 1,
		})
		assert.Len(t, res.CategorizedErrors, 1)
		assert.Equal(t, catalogm.RadioSyncRunErrorTruncation, res.CategorizedErrors[0].Category)
		assert.Equal(t, "ep-1", *res.CategorizedErrors[0].EpisodeRef)
		assert.Len(t, res.Errors, 1, "1:1 with CategorizedErrors")
	})
	t.Run("drop-only records validation_drop", func(t *testing.T) {
		res := &contracts.RadioImportResult{}
		accumulateEpisodeResult(res, "ep-1", &contracts.EpisodeImportResult{
			DropSummary: "dropped 2 plays: 2 missing artist_name", DroppedPlays: 2,
		})
		assert.Len(t, res.CategorizedErrors, 1)
		assert.Equal(t, catalogm.RadioSyncRunErrorValidationDrop, res.CategorizedErrors[0].Category)
	})
	t.Run("both classes collapse to one validation_drop (data loss outranks salvage)", func(t *testing.T) {
		res := &contracts.RadioImportResult{}
		accumulateEpisodeResult(res, "ep-1", &contracts.EpisodeImportResult{
			DropSummary:    "dropped 3 plays: 1 over-length titles truncated, 2 missing artist_name",
			TruncatedPlays: 1, DroppedPlays: 2,
		})
		assert.Len(t, res.CategorizedErrors, 1, "precedence picks one category, not two rows")
		assert.Equal(t, catalogm.RadioSyncRunErrorValidationDrop, res.CategorizedErrors[0].Category)
	})
	t.Run("FetchError uses its typed category", func(t *testing.T) {
		res := &contracts.RadioImportResult{}
		accumulateEpisodeResult(res, "ep-1", &contracts.EpisodeImportResult{
			FetchError: "parsing plays response: boom", FetchErrorCategory: catalogm.RadioSyncRunErrorParseError,
		})
		assert.Len(t, res.CategorizedErrors, 1)
		assert.Equal(t, catalogm.RadioSyncRunErrorParseError, res.CategorizedErrors[0].Category)
		assert.Equal(t, 1, res.EpisodeFetchErrors)
	})
	t.Run("FetchError with empty category falls back to provider_unreachable", func(t *testing.T) {
		res := &contracts.RadioImportResult{}
		accumulateEpisodeResult(res, "ep-1", &contracts.EpisodeImportResult{FetchError: "boom"})
		assert.Len(t, res.CategorizedErrors, 1)
		assert.Equal(t, catalogm.RadioSyncRunErrorProviderUnreachable, res.CategorizedErrors[0].Category)
	})
	t.Run("multiple signals keep Errors and CategorizedErrors 1:1", func(t *testing.T) {
		res := &contracts.RadioImportResult{}
		// All three branches active — artificial but exercises the invariant across appends.
		accumulateEpisodeResult(res, "ep-1", &contracts.EpisodeImportResult{
			FetchError: "boom", MatchPersistErrors: 1,
			DropSummary: "dropped 1 plays: 1 missing artist_name", DroppedPlays: 1,
		})
		assert.Len(t, res.CategorizedErrors, 3, "fetch + persist + drop")
		assert.Equal(t, len(res.Errors), len(res.CategorizedErrors), "parallel-slice invariant")
		// The persist entry carries the match_persist_error category (structural assignment).
		var sawPersist bool
		for _, e := range res.CategorizedErrors {
			if e.Category == catalogm.RadioSyncRunErrorMatchPersistError {
				sawPersist = true
			}
		}
		assert.True(t, sawPersist, "MatchPersistErrors records a match_persist_error category")
	})
}

func TestCategorizeRunError(t *testing.T) {
	assert.Equal(t, catalogm.RadioSyncRunErrorTimeout, categorizeRunError(context.DeadlineExceeded))
	assert.Equal(t, catalogm.RadioSyncRunErrorRateLimited,
		categorizeRunError(&RadioHTTPError{Provider: "KEXP", StatusCode: http.StatusTooManyRequests}))
	assert.Equal(t, catalogm.RadioSyncRunErrorProviderUnreachable,
		categorizeRunError(&RadioHTTPError{Provider: "KEXP", StatusCode: http.StatusInternalServerError}))
	assert.Equal(t, catalogm.RadioSyncRunErrorProviderUnreachable, categorizeRunError(errors.New("show not found")))
	// wrapped deadline still detected
	assert.Equal(t, catalogm.RadioSyncRunErrorTimeout,
		categorizeRunError(errors.Join(errors.New("fetch"), context.DeadlineExceeded)))
	// PSY-1141: a provider parse/format failure (wrapped, untyped) is parse-detected so
	// it routes to parse_error and escalates instead of defaulting to provider_unreachable.
	assert.Equal(t, catalogm.RadioSyncRunErrorParseError,
		categorizeRunError(errors.New("parsing plays response: unexpected end of JSON input")))
	assert.Equal(t, catalogm.RadioSyncRunErrorParseError,
		categorizeRunError(errors.New("json: cannot unmarshal number into Go value of type string")))
}

// escalationError — the pure Sentry-escalation decision (PSY-1141). Every permanent
// scheduled/auto failure escalates; manual never; transient never; a per-episode
// parse_error in a PARTIAL run escalates (scraper drift surfaces as partial).
func TestEscalationError(t *testing.T) {
	permanentHard := errors.New("provider format changed") // classifyError → permanent (default)
	transientHard := context.DeadlineExceeded              // classifyError → transient
	parseErrs := []runError{{category: catalogm.RadioSyncRunErrorParseError, detail: "episode ep-1: parse failed"}}
	dropErrs := []runError{{category: catalogm.RadioSyncRunErrorValidationDrop, detail: "episode ep-1: dropped 2 plays"}}
	failed := catalogm.RadioSyncRunStatusFailed

	t.Run("manual never escalates", func(t *testing.T) {
		_, err := escalationError(syncOutcome{status: failed, hardErr: permanentHard}, catalogm.RadioSyncRunTriggerManual)
		assert.Nil(t, err)
	})
	t.Run("scheduled permanent hard failure escalates", func(t *testing.T) {
		cat, err := escalationError(syncOutcome{status: failed, hardErr: permanentHard}, catalogm.RadioSyncRunTriggerScheduled)
		assert.Equal(t, permanentHard, err)
		assert.Equal(t, catalogm.RadioSyncRunErrorProviderUnreachable, cat)
	})
	t.Run("scheduled transient hard failure does NOT escalate", func(t *testing.T) {
		_, err := escalationError(syncOutcome{status: failed, hardErr: transientHard}, catalogm.RadioSyncRunTriggerScheduled)
		assert.Nil(t, err)
	})
	t.Run("scheduled partial with a parse_error escalates (scraper drift)", func(t *testing.T) {
		cat, err := escalationError(syncOutcome{status: catalogm.RadioSyncRunStatusPartial, errs: parseErrs}, catalogm.RadioSyncRunTriggerScheduled)
		assert.Error(t, err)
		assert.Equal(t, catalogm.RadioSyncRunErrorParseError, cat)
	})
	t.Run("scheduled partial with only validation_drop does NOT escalate", func(t *testing.T) {
		_, err := escalationError(syncOutcome{status: catalogm.RadioSyncRunStatusPartial, errs: dropErrs}, catalogm.RadioSyncRunTriggerScheduled)
		assert.Nil(t, err)
	})
	t.Run("scheduled success does NOT escalate", func(t *testing.T) {
		_, err := escalationError(syncOutcome{status: catalogm.RadioSyncRunStatusSuccess}, catalogm.RadioSyncRunTriggerScheduled)
		assert.Nil(t, err)
	})
	t.Run("auto_backfill permanent escalates like scheduled", func(t *testing.T) {
		_, err := escalationError(syncOutcome{status: failed, hardErr: permanentHard}, catalogm.RadioSyncRunTriggerAutoBackfill)
		assert.Equal(t, permanentHard, err)
	})
}

func TestCategorizeErrorString(t *testing.T) {
	cases := map[string]string{
		"context deadline exceeded":       catalogm.RadioSyncRunErrorTimeout,
		"got 429 too many requests":       catalogm.RadioSyncRunErrorRateLimited,
		"failed to unmarshal json":        catalogm.RadioSyncRunErrorParseError,
		"dropped 2 plays: missing artist": catalogm.RadioSyncRunErrorValidationDrop,
		// Real summarizeDrops format always starts "dropped N plays:", so even a
		// truncation-only summary buckets validation_drop (truncation isn't reachable
		// via the string heuristic — see categorizeErrorString's note).
		"episode ep-1: dropped 3 plays: 3 over-length titles truncated": catalogm.RadioSyncRunErrorValidationDrop,
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
