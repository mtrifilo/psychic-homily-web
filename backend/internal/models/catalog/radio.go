package catalog

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Broadcast type constants for radio stations
const (
	BroadcastTypeTerrestrial = "terrestrial"
	BroadcastTypeInternet    = "internet"
	BroadcastTypeBoth        = "both"
)

// Playlist source constants for radio stations
const (
	PlaylistSourceKEXP   = "kexp_api"
	PlaylistSourceNTS    = "nts_api"
	PlaylistSourceWFMU   = "wfmu_scrape"
	PlaylistSourceManual = "manual"
)

// Rotation status constants for radio plays (KEXP). Enforced by the
// radio_plays_rotation_status_check CHECK constraint (PSY-1131). NULL is also
// accepted (most providers don't supply a rotation); an unrecognized provider
// value must be normalized to NULL by the pipeline before insert.
const (
	RotationStatusHeavy          = "heavy"
	RotationStatusMedium         = "medium"
	RotationStatusLight          = "light"
	RotationStatusRecommendedNew = "recommended_new"
	RotationStatusLibrary        = "library"
)

// Station provenance constants (radio_stations.source). PSY-1131.
//   - canonical: hand-curated seed (KEXP/WFMU/NTS)
//   - discovered: created on first observed episode by the ingestion pipeline
//   - manual: added by a human via admin UI
const (
	RadioStationSourceCanonical  = "canonical"
	RadioStationSourceDiscovered = "discovered"
	RadioStationSourceManual     = "manual"
)

// Show provenance constants (radio_shows.source). PSY-1131.
//   - provider: synced from a station's provider feed (includes pre-seeded shows)
//   - manual: added by a human
const (
	RadioShowSourceProvider = "provider"
	RadioShowSourceManual   = "manual"
)

// Lifecycle-state constants shared by radio_stations and radio_shows. PSY-1131.
// Replaces bare is_active as the operational signal: active = in service;
// dormant = temporarily not airing/syncing; retired = permanently gone.
const (
	RadioLifecycleActive  = "active"
	RadioLifecycleDormant = "dormant"
	RadioLifecycleRetired = "retired"
)

// Episode status constants (radio_episodes.status). PSY-1131. Makes "live" an
// explicit stored fact rather than the implicit "a row exists for today" that
// produced the false ON-AIR bug (PSY-1128). Windowless episodes default to
// 'aired' and are never 'live'.
const (
	RadioEpisodeStatusScheduled = "scheduled"
	RadioEpisodeStatusLive      = "live"
	RadioEpisodeStatusAired     = "aired"
	RadioEpisodeStatusArchived  = "archived"
)

// Playlist-fetch lifecycle constants (radio_episodes.playlist_state). PSY-1131.
// Decoupled from episode status: an aired episode can still have a pending
// playlist fetch.
const (
	RadioPlaylistStatePending     = "pending"
	RadioPlaylistStatePartial     = "partial"
	RadioPlaylistStateComplete    = "complete"
	RadioPlaylistStateUnavailable = "unavailable"
)

// Post-air playlist retry policy (PSY-1562). These three constants plus
// PlanPlaylistFetch are the WHOLE policy: how often an episode may be re-tried, and
// the instant past which it never is again. Modeled as consts (like
// radioCircuitBreakerThreshold), not env-tunable: they are data-quality policy, not
// operational cadence. The sweep cadence (interval, lookback) IS env-tunable — see
// radio_fetch_service.go.
const (
	// RadioPlaylistRetryCooldown is the minimum gap between two POST-AIR playlist
	// fetch attempts on the same episode. Before PSY-1562 there was none, so an
	// episode found empty was re-selected by the very next sweep (hourly) — which is
	// how 23 stranded episodes produced 269 zero-yield backfill runs a day on
	// production. Six hours is short enough that the common case (a tracklist
	// published minutes to hours after air) is still caught the same day, and long
	// enough that a tracklist that never appears costs ~4 attempts a day, not 24.
	//
	// The LIVE refresh (PSY-1370) is deliberately NOT cooled down: it exists to
	// accumulate tracks during the broadcast and terminates on its own at ends_at.
	RadioPlaylistRetryCooldown = 6 * time.Hour

	// RadioPlaylistGiveUpAfter is how long after an episode finished airing we keep
	// looking for a tracklist that has not appeared. Past it the give-up is TERMINAL:
	// the episode is never re-fetched again, and no code path can reset it, because
	// the deadline is derived from the episode's own FROZEN air time — a fact that
	// only ever moves forward.
	//
	// Five days is sized from the measured upstream publish lag: NTS routinely
	// publishes a tracklist 0–3 days after air, rarely 4 (measured 2026-07-29 on
	// PSY-1556). Five days covers the rare case with a day of margin. Do not shorten
	// it below 4 days without re-measuring.
	//
	// This REPLACES the attempt counter as the terminator, which is the PSY-1558
	// lesson: an attempt counter is mutable state, so every path that legitimately
	// reset it (a window heal, a schedule correction) also silently un-terminated the
	// episode, and the cap never fired at all for windowless episodes.
	RadioPlaylistGiveUpAfter = 5 * 24 * time.Hour

	// RadioAirDateZoneSlack absorbs air_date's missing timezone. air_date is a bare
	// local calendar date parsed with NO zone, i.e. at UTC midnight, so for a
	// WINDOWLESS episode — the only kind with no better air instant on the row — the
	// parsed value UNDERSTATES when the episode actually aired. 36h is the exact
	// worst case: a local calendar date ends at UTC midnight + 36h in UTC-12, the
	// westernmost zone. Without it a late-evening West-Coast broadcast would be given
	// up on ~34h early, eating most of the margin over the 4-day publish lag.
	RadioAirDateZoneSlack = 36 * time.Hour
)

// RadioBackfillMaxAttempts is a defense-in-depth CEILING on post-air playlist
// re-fetches, not the terminator — RadioPlaylistGiveUpAfter is (see there). It fires
// only if the cooldown memo (playlist_backfill_attempted_at) stops advancing, which
// would otherwise let an episode retry on every sweep until its deadline.
//
// DERIVED, never hand-set: it is the number of cooldown slots in the longest possible
// give-up deadline, plus slack, so it cannot contradict that deadline. A hand-set
// ceiling smaller than the deadline would silently swallow the late-publish window —
// the mirror image of the PSY-1558 ceiling that could never fire.
const RadioBackfillMaxAttempts = int((RadioPlaylistGiveUpAfter+RadioAirDateZoneSlack)/RadioPlaylistRetryCooldown) + 4

// ComputeEpisodeStatus derives an episode's lifecycle status from its FROZEN air
// window, playlist completeness, and the current time (PSY-1152).
//
// "live" is computed here, never trusted as a durable stored value, because it
// is a function of now — a stored "live" goes stale the instant the air window
// ends, which is exactly the PSY-1128 false-ON-AIR bug. Callers compute it at
// read time against the viewer's clock.
//
// A windowless episode (startsAt == nil — WFMU before PSY-1159, or any provider
// that supplies no time) is NEVER "live": it is 'archived' once its playlist is
// complete, else 'aired' (the conservative §9 decision-2 fallback). An episode
// with a start but no end (e.g. NTS, which gives a broadcast start but no
// duration) likewise can't be bounded as "live" and settles to aired/archived
// once started.
func ComputeEpisodeStatus(startsAt, endsAt *time.Time, playlistState string, now time.Time) string {
	settled := RadioEpisodeStatusAired
	if playlistState == RadioPlaylistStateComplete {
		settled = RadioEpisodeStatusArchived
	}

	if startsAt == nil {
		return settled // windowless: never scheduled or live
	}
	if now.Before(*startsAt) {
		return RadioEpisodeStatusScheduled
	}
	// now is at/after startsAt. "live" only with a bounded window we're still inside.
	if endsAt != nil && !now.After(*endsAt) {
		return RadioEpisodeStatusLive
	}
	return settled
}

// PlaylistFetchFacts is the durable episode state that every playlist-fetch decision
// is made from (PSY-1562). Every field is a stored column, so a decision is
// reproducible from the row alone — there is no derived or read-time-repaired state
// in it.
type PlaylistFetchFacts struct {
	// StartsAt/EndsAt are the FROZEN air window; NULL when the provider supplies no
	// time (WFMU) or no duration (NTS gives a start only).
	StartsAt *time.Time
	EndsAt   *time.Time
	// AirDate is air_date parsed as a date. Consulted only for a WINDOWLESS episode,
	// which carries no better air instant; the zero value fails CLOSED (an episode
	// whose air instant is unknown cannot be bounded, so it is never fetched).
	AirDate time.Time
	// PlaylistState is read ONLY to recognise a settled playlist ('complete') and the
	// live-refresh-eligible set. It is deliberately NOT the terminal signal: an
	// 'unavailable' label never gates post-air eligibility, so a stale one cannot
	// strand an episode and no normalizer is needed to clear it. That inversion is
	// the whole point of PSY-1562 — under the old design three separate normalizers
	// existed purely to un-stick this label at read time.
	PlaylistState string
	// PlayCount is the denormalized play_count (monotonic on write, reconciled
	// nightly by ReconcilePlayCounts). It distinguishes "no playlist at all" from
	// "some tracks, but the final post-air playlist never appeared" — a distinction
	// the state writers need and the eligibility predicate does not.
	PlayCount int
	// Attempts is playlist_fetch_attempts, feeding the RadioBackfillMaxAttempts
	// ceiling only. It is NOT the terminator.
	Attempts int
	// LastBackfillAttemptAt is playlist_backfill_attempted_at: when the last POST-AIR
	// attempt ran. NULL means none has run yet (including rows written before
	// PSY-1562), which makes the episode immediately eligible once aired.
	LastBackfillAttemptAt *time.Time
}

// PlaylistFetchPlan is PlanPlaylistFetch's answer. Fetch and Exhausted are distinct:
// an episode that is merely cooling down wants a fetch later (neither flag set),
// whereas an Exhausted one never will again.
type PlaylistFetchPlan struct {
	// Fetch is true when the episode should have its playlist fetched right now.
	Fetch bool
	// Live marks the fetch as a live-window refresh (PSY-1370) rather than a post-air
	// backfill. Only meaningful when Fetch is true; the two are mutually exclusive by
	// time phase, so nothing double-drives an episode.
	Live bool
	// Exhausted is the TERMINAL give-up: this episode has aired, still has no
	// playlist, and is past the point where one could still appear. Nothing resets
	// it, because it is a function of frozen air time and now.
	Exhausted bool
}

// PlanPlaylistFetch is THE playlist-fetch eligibility predicate (PSY-1562): the one
// answer to "does this episode want a playlist fetch, and may it still try?". The
// backfill sweep's candidate query, the in-flight re-list decision, and the give-up
// label all refine to it, so selection and execution can never drift.
//
// By time phase:
//
//   - scheduled — never. The playlist legitimately does not exist yet.
//   - live — refresh while still incomplete (PSY-1370), with no cooldown and no
//     ceiling: a live fetch never burns an attempt, and ends_at terminates it.
//   - aired — the post-air backfill. Eligible while the playlist is unsettled, the
//     cooldown since the last attempt has elapsed, and the give-up deadline has not
//     passed.
//
// It replaces six predicates that had accumulated one per repair ticket, three of
// which repaired bad persisted state at READ time. The structural fix is that
// terminality is no longer a stored label that a writer could leave wrong and a
// reader had to fix — it is recomputed here from the episode's frozen air time,
// which no code path can move backwards.
func PlanPlaylistFetch(f PlaylistFetchFacts, now time.Time) PlaylistFetchPlan {
	// Pass pending so the result is the pure time phase (scheduled/live/aired) and
	// never 'archived' — completeness is handled below, not by the phase.
	switch ComputeEpisodeStatus(f.StartsAt, f.EndsAt, RadioPlaylistStatePending, now) {
	case RadioEpisodeStatusScheduled:
		return PlaylistFetchPlan{}
	case RadioEpisodeStatusLive:
		incomplete := f.PlaylistState == RadioPlaylistStatePending || f.PlaylistState == RadioPlaylistStatePartial
		return PlaylistFetchPlan{Fetch: incomplete, Live: incomplete}
	}

	// Aired. A settled playlist is final: the post-air fetch that returned plays is
	// the last one an episode ever needs.
	if f.PlaylistState == RadioPlaylistStateComplete {
		return PlaylistFetchPlan{}
	}
	deadline, ok := playlistGiveUpDeadline(f)
	if !ok || !now.Before(deadline) || f.Attempts >= RadioBackfillMaxAttempts {
		return PlaylistFetchPlan{Exhausted: true}
	}
	if f.LastBackfillAttemptAt != nil && now.Before(f.LastBackfillAttemptAt.Add(RadioPlaylistRetryCooldown)) {
		return PlaylistFetchPlan{} // cooling down — wants a fetch, just not yet
	}
	return PlaylistFetchPlan{Fetch: true}
}

// playlistGiveUpDeadline is the instant past which an episode's missing playlist is
// treated as permanent. It is measured from the best air instant the row carries, in
// descending order of precision: the end of the air window, the start of it, or —
// for a windowless episode — air_date widened by the timezone it doesn't record.
// Reports false when the row carries no usable air instant at all, which fails closed.
func playlistGiveUpDeadline(f PlaylistFetchFacts) (time.Time, bool) {
	switch {
	case f.EndsAt != nil:
		return f.EndsAt.Add(RadioPlaylistGiveUpAfter), true
	case f.StartsAt != nil:
		return f.StartsAt.Add(RadioPlaylistGiveUpAfter), true
	case !f.AirDate.IsZero():
		return f.AirDate.Add(RadioAirDateZoneSlack + RadioPlaylistGiveUpAfter), true
	}
	return time.Time{}, false
}

// SettlePlaylistStateAfterFetch decides an episode's playlist_state and attempt count
// after ONE playlist fetch (PSY-1154, reworked by PSY-1562). It is the write-time
// half of the state machine, shared by the first-import path and the backfill
// re-fetch path:
//
//   - plays + aired  → complete (the final post-air playlist)
//   - plays + live   → partial  (a snapshot; more are coming)
//   - none + !aired  → pending, or 'partial' held (PSY-1370, see below)
//   - none + aired   → a genuine failed post-air attempt: burn one, then ask
//     PlanPlaylistFetch whether a next attempt is even possible. If it isn't, the
//     give-up is recorded NOW as 'unavailable' rather than left as a row that reads
//     'pending' forever while never being fetched again.
//
// "no plays" covers a fetch error and a legitimately empty playlist alike — for the
// give-up policy they are the same: we still don't have a playlist.
//
// An episode that already HAS plays never reads 'pending' or 'unavailable', whatever
// the fetch returned: those labels would contradict the tracks the episode is showing.
// It settles to 'partial' — some tracks, no final playlist — which is the same answer
// RederivePlaylistState gives, so the two writers cannot disagree about a row.
// postAir reports whether this was a POST-AIR attempt, which is what the caller
// stamps the retry memo on. Returned rather than recomputed by the caller so the memo
// and the state it rate-limits cannot disagree about which phase the fetch happened
// in.
func SettlePlaylistStateAfterFetch(f PlaylistFetchFacts, hasPlays bool, now time.Time) (state string, attempts int, postAir bool) {
	aired := ComputeEpisodeStatus(f.StartsAt, f.EndsAt, RadioPlaylistStatePending, now) == RadioEpisodeStatusAired

	if hasPlays {
		if aired {
			return RadioPlaylistStateComplete, f.Attempts, true
		}
		return RadioPlaylistStatePartial, f.Attempts, false
	}
	if !aired {
		// Live or scheduled with nothing yet — expected, not a failure. 'partial' is
		// monotonic within the live window (PSY-1370): a transient empty round must
		// not erase "this show already has tracks".
		if f.PlaylistState == RadioPlaylistStatePartial || f.PlayCount > 0 {
			return RadioPlaylistStatePartial, f.Attempts, false
		}
		return RadioPlaylistStatePending, f.Attempts, false
	}

	// A genuine failed post-air attempt: burn one regardless of the label below, since
	// the attempt happened and the ceiling counts attempts.
	attempts = f.Attempts + 1
	if f.PlayCount > 0 {
		return RadioPlaylistStatePartial, attempts, true
	}

	// The label is decided by the same predicate the sweep uses, evaluated one
	// cooldown ahead — the earliest moment this episode could be tried again. If it
	// is already exhausted there, this attempt was the last one.
	next := f
	next.Attempts = attempts
	next.PlaylistState = RadioPlaylistStatePending
	next.LastBackfillAttemptAt = &now
	if PlanPlaylistFetch(next, now.Add(RadioPlaylistRetryCooldown)).Exhausted {
		return RadioPlaylistStateUnavailable, attempts, true
	}
	return RadioPlaylistStatePending, attempts, true
}

// RederivePlaylistState re-settles playlist_state and playlist_fetch_attempts from an
// episode's CURRENT air window. Every write path that touches an episode calls it, so
// the stored label is always the one the window and play count imply — which is what
// lets readers stop repairing state. It replaces three read-time normalizers
// (PSY-1285's scheduled reset, PSY-1287's window heal, PSY-1287's stranded reopen) and
// reopenLivePlaylistState, each of which existed because a writer could leave a verdict
// that was wrong for the episode's real phase.
//
// It is total and idempotent, so calling it on every re-list is safe — deliberately,
// because the bad states it corrects arise on rows whose window was ALREADY present
// (PSY-1285's stranded scheduled rows were stranded exactly that way), not only on the
// heal-from-NULL transition.
//
// Attempts are cleared only when the episode has NOT aired, where a burned attempt
// provably cannot have been earned: a windowless episode reads as 'aired' the moment
// it starts, so the backfill burns attempts on it, and only the corrected window
// reveals it was really scheduled or still live. An AIRED episode keeps its attempts —
// they really happened, and they feed the RadioBackfillMaxAttempts ceiling. Not
// clearing them here is what keeps this from being another PSY-1558-shaped reset on a
// path that runs every cycle.
func RederivePlaylistState(f PlaylistFetchFacts, now time.Time) (state string, attempts int) {
	switch ComputeEpisodeStatus(f.StartsAt, f.EndsAt, RadioPlaylistStatePending, now) {
	case RadioEpisodeStatusScheduled:
		// Nothing has aired: no verdict about a missing playlist can stand. A state
		// carrying real plays is left alone — it is a genuine snapshot, not a verdict.
		if f.PlayCount > 0 {
			return f.PlaylistState, f.Attempts
		}
		return RadioPlaylistStatePending, 0
	case RadioEpisodeStatusLive:
		// Settled mid-broadcast at the wrong phase — a sweep-created end-less row that
		// read as 'aired' until the airing feed supplied its end bound.
		if f.PlayCount > 0 {
			return RadioPlaylistStatePartial, f.Attempts
		}
		return RadioPlaylistStatePending, 0
	}

	// Aired under the new window.
	if f.PlaylistState == RadioPlaylistStateComplete {
		return RadioPlaylistStateComplete, f.Attempts
	}
	if f.PlayCount > 0 {
		return RadioPlaylistStatePartial, f.Attempts
	}
	// No playlist and no verdict worth keeping: the label is whatever the predicate
	// says about this episode right now — 'unavailable' once it is past its deadline,
	// 'pending' while it can still be tried. Asking the predicate rather than trusting
	// the stored label is what makes a stale 'unavailable' self-correcting instead of
	// something a reader has to normalize away. Passing f unmodified is safe because
	// 'complete' is the only label PlanPlaylistFetch short-circuits on, and it returned
	// above.
	if PlanPlaylistFetch(f, now).Exhausted {
		return RadioPlaylistStateUnavailable, f.Attempts
	}
	return RadioPlaylistStatePending, f.Attempts
}

// Match-state constants (radio_plays.match_state). PSY-1131. Replaces the
// implicit "artist_id IS NULL == unmatched" with an explicit state. no_match
// (matcher ran, found nothing) is distinct from unmatched (matcher not yet run).
const (
	RadioPlayMatchStateUnmatched = "unmatched"
	RadioPlayMatchStateMatched   = "matched"
	RadioPlayMatchStateAmbiguous = "ambiguous"
	RadioPlayMatchStateNoMatch   = "no_match"
)

// BroadcastTypes is the list of valid broadcast types
var BroadcastTypes = []string{
	BroadcastTypeTerrestrial,
	BroadcastTypeInternet,
	BroadcastTypeBoth,
}

// IsValidBroadcastType checks whether a string is a valid broadcast type
func IsValidBroadcastType(s string) bool {
	for _, bt := range BroadcastTypes {
		if bt == s {
			return true
		}
	}
	return false
}

// PlaylistSources is the list of valid playlist sources. getProvider dispatches
// the three scraper/API sources (kexp_api, nts_api, wfmu_scrape) to a provider;
// "manual" is a valid value meaning hand-curated playlists with no automated
// provider. The empty string is also accepted by IsValidPlaylistSource and
// likewise means "no automated provider" (a link-only station not auto-imported).
var PlaylistSources = []string{
	PlaylistSourceKEXP,
	PlaylistSourceNTS,
	PlaylistSourceWFMU,
	PlaylistSourceManual,
}

// IsValidPlaylistSource reports whether s is an accepted playlist_source. The
// empty string is valid (no automated provider / link-only). Rejecting anything
// else stops invalid values like "wfmu_html" from being persisted and silently
// breaking all playlist import for the station. (PSY-927)
func IsValidPlaylistSource(s string) bool {
	if s == "" {
		return true
	}
	for _, ps := range PlaylistSources {
		if ps == s {
			return true
		}
	}
	return false
}

// IsValidRotationStatus reports whether s is an accepted rotation_status. The
// empty string is valid (no rotation supplied — the common case for non-KEXP
// providers); it maps to a NULL column. Any other unrecognized value is invalid
// and must be normalized to "" by the pipeline before insert, or the
// radio_plays_rotation_status_check CHECK will reject the row (PSY-1131).
func IsValidRotationStatus(s string) bool {
	switch s {
	case "", RotationStatusHeavy, RotationStatusMedium, RotationStatusLight,
		RotationStatusRecommendedNew, RotationStatusLibrary:
		return true
	default:
		return false
	}
}

// NormalizeRotationStatus coerces a provider-supplied rotation_status into a
// value the radio_plays_rotation_status_check CHECK accepts (PSY-1131):
// trimmed + lowercased, returning nil (SQL NULL) for empty or unrecognized
// values. KEXP sends capitalized values (e.g. "Library"); other providers send
// none. Call this at the persist boundary before insert. The live now-playing
// response path intentionally surfaces the raw provider value (display-only, not
// persisted), so a live track may show "Library" where the archived row stores
// "library".
func NormalizeRotationStatus(s *string) *string {
	if s == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*s))
	if normalized == "" || !IsValidRotationStatus(normalized) {
		return nil
	}
	return &normalized
}

// IsValidRadioStationSource reports whether s is an accepted station source.
func IsValidRadioStationSource(s string) bool {
	switch s {
	case RadioStationSourceCanonical, RadioStationSourceDiscovered, RadioStationSourceManual:
		return true
	default:
		return false
	}
}

// IsValidRadioShowSource reports whether s is an accepted show source.
func IsValidRadioShowSource(s string) bool {
	switch s {
	case RadioShowSourceProvider, RadioShowSourceManual:
		return true
	default:
		return false
	}
}

// IsValidRadioLifecycleState reports whether s is an accepted lifecycle_state
// for a station or show.
func IsValidRadioLifecycleState(s string) bool {
	switch s {
	case RadioLifecycleActive, RadioLifecycleDormant, RadioLifecycleRetired:
		return true
	default:
		return false
	}
}

// IsValidRadioEpisodeStatus reports whether s is an accepted episode status.
func IsValidRadioEpisodeStatus(s string) bool {
	switch s {
	case RadioEpisodeStatusScheduled, RadioEpisodeStatusLive,
		RadioEpisodeStatusAired, RadioEpisodeStatusArchived:
		return true
	default:
		return false
	}
}

// IsValidRadioPlaylistState reports whether s is an accepted playlist_state.
func IsValidRadioPlaylistState(s string) bool {
	switch s {
	case RadioPlaylistStatePending, RadioPlaylistStatePartial,
		RadioPlaylistStateComplete, RadioPlaylistStateUnavailable:
		return true
	default:
		return false
	}
}

// IsValidRadioPlayMatchState reports whether s is an accepted play match_state.
func IsValidRadioPlayMatchState(s string) bool {
	switch s {
	case RadioPlayMatchStateUnmatched, RadioPlayMatchStateMatched,
		RadioPlayMatchStateAmbiguous, RadioPlayMatchStateNoMatch:
		return true
	default:
		return false
	}
}

// =============================================================================
// PSY-1132: radio observability enum vocabularies (radio_sync_runs,
// radio_sync_run_errors, radio_station_health). Same constant + IsValid* +
// unit-test pattern as the PSY-1131 enums above; consumed at the P2 write
// boundary (RunStationSync), tested now.
// =============================================================================

// Sync-run type constants (radio_sync_runs.run_type). PSY-1132.
//   - discover: enumerate a station's provider roster
//   - fetch:    pull new episodes
//   - backfill: re-ingest a historic window (window_start/window_end)
//   - rematch:  re-run unmatched plays against the knowledge graph
const (
	RadioSyncRunTypeDiscover = "discover"
	RadioSyncRunTypeFetch    = "fetch"
	RadioSyncRunTypeBackfill = "backfill"
	RadioSyncRunTypeRematch  = "rematch"
)

// Sync-run trigger constants (radio_sync_runs.trigger_source). PSY-1132. The
// column is trigger_source because `trigger` is a reserved SQL keyword.
//   - scheduled:     a background ticker
//   - manual:        an admin action ("Sync now" / historic backfill)
//   - auto_backfill: kicked off on first discovery of a show
const (
	RadioSyncRunTriggerScheduled    = "scheduled"
	RadioSyncRunTriggerManual       = "manual"
	RadioSyncRunTriggerAutoBackfill = "auto_backfill"
)

// Sync-run status constants (radio_sync_runs.status). PSY-1132. A run opens
// 'running' and resolves to one terminal state. partial = completed but flagged
// by the anomaly guard / per-episode errors; skipped = breaker open; cancelled =
// in-flight backfill aborted by an admin (carried forward from radio_import_jobs).
const (
	RadioSyncRunStatusRunning   = "running"
	RadioSyncRunStatusSuccess   = "success"
	RadioSyncRunStatusPartial   = "partial"
	RadioSyncRunStatusFailed    = "failed"
	RadioSyncRunStatusSkipped   = "skipped"
	RadioSyncRunStatusCancelled = "cancelled"
)

// Sync-run error category constants (radio_sync_run_errors.category). PSY-1132.
// Generalizes PSY-1119's per-episode capture; filterable instead of grep-only.
const (
	RadioSyncRunErrorProviderUnreachable = "provider_unreachable"
	RadioSyncRunErrorRateLimited         = "rate_limited"
	RadioSyncRunErrorParseError          = "parse_error"
	RadioSyncRunErrorEmptyUnexpected     = "empty_unexpected"
	RadioSyncRunErrorValidationDrop      = "validation_drop"
	RadioSyncRunErrorTruncation          = "truncation"
	RadioSyncRunErrorMatchPersistError   = "match_persist_error"
	RadioSyncRunErrorTimeout             = "timeout"
)

// Circuit-breaker state constants (radio_station_health.breaker_state). PSY-1132.
// Persisted so the breaker survives restarts (today it is in-memory; PSY-887).
const (
	RadioBreakerStateClosed   = "closed"
	RadioBreakerStateOpen     = "open"
	RadioBreakerStateHalfOpen = "half_open"
)

// IsValidRadioSyncRunType reports whether s is an accepted sync-run run_type.
func IsValidRadioSyncRunType(s string) bool {
	switch s {
	case RadioSyncRunTypeDiscover, RadioSyncRunTypeFetch,
		RadioSyncRunTypeBackfill, RadioSyncRunTypeRematch:
		return true
	default:
		return false
	}
}

// IsValidRadioSyncRunTrigger reports whether s is an accepted sync-run trigger.
func IsValidRadioSyncRunTrigger(s string) bool {
	switch s {
	case RadioSyncRunTriggerScheduled, RadioSyncRunTriggerManual,
		RadioSyncRunTriggerAutoBackfill:
		return true
	default:
		return false
	}
}

// IsValidRadioSyncRunStatus reports whether s is an accepted sync-run status.
func IsValidRadioSyncRunStatus(s string) bool {
	switch s {
	case RadioSyncRunStatusRunning, RadioSyncRunStatusSuccess,
		RadioSyncRunStatusPartial, RadioSyncRunStatusFailed,
		RadioSyncRunStatusSkipped, RadioSyncRunStatusCancelled:
		return true
	default:
		return false
	}
}

// IsValidRadioSyncRunErrorCategory reports whether s is an accepted error category.
func IsValidRadioSyncRunErrorCategory(s string) bool {
	switch s {
	case RadioSyncRunErrorProviderUnreachable, RadioSyncRunErrorRateLimited,
		RadioSyncRunErrorParseError, RadioSyncRunErrorEmptyUnexpected,
		RadioSyncRunErrorValidationDrop, RadioSyncRunErrorTruncation,
		RadioSyncRunErrorMatchPersistError, RadioSyncRunErrorTimeout:
		return true
	default:
		return false
	}
}

// IsValidRadioBreakerState reports whether s is an accepted breaker state.
func IsValidRadioBreakerState(s string) bool {
	switch s {
	case RadioBreakerStateClosed, RadioBreakerStateOpen, RadioBreakerStateHalfOpen:
		return true
	default:
		return false
	}
}

// RadioScheduleSlot is one recurring weekly air slot in a RadioSchedule.
// DayOfWeek is 0=Sunday..6=Saturday. Start/End are "HH:MM" 24-hour local times
// in the parent RadioSchedule's Timezone. An End <= Start denotes a slot that
// wraps past midnight (e.g. 23:00–01:00).
type RadioScheduleSlot struct {
	DayOfWeek int    `json:"day_of_week"`
	Start     string `json:"start"`
	End       string `json:"end"`
}

// RadioSchedule is the validated JSONB shape stored in radio_shows.schedule
// (PSY-1131). It is the basis for the air-window / "live" computation consumed
// in P4. The column itself is a plain JSONB; ParseRadioSchedule + Validate are
// invoked by the admin create/update show handlers (the app boundary) to reject
// a malformed schedule with 422, so the rule lives in one place rather than a
// brittle JSONB CHECK. NOTE: the discovery/import write path does not yet route
// through this validator (deferred to P4 with the air-window consumer).
//
//	{ "timezone": "America/Los_Angeles",
//	  "slots": [ { "day_of_week": 1, "start": "06:00", "end": "10:00" } ] }
type RadioSchedule struct {
	Timezone string              `json:"timezone"`
	Slots    []RadioScheduleSlot `json:"slots"`
}

// hhmmPattern matches an "HH:MM" 24-hour time string (00:00–23:59).
var hhmmPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// Validate checks that a RadioSchedule is well-formed: a non-empty IANA
// timezone that the standard library can load, and each slot a valid weekday
// (0–6) with "HH:MM" start/end times. It does NOT reject End <= Start (that is
// the legitimate midnight-wrap encoding). Returns the first violation found.
func (s RadioSchedule) Validate() error {
	if strings.TrimSpace(s.Timezone) == "" {
		return fmt.Errorf("radio schedule: timezone is required")
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("radio schedule: invalid timezone %q: %w", s.Timezone, err)
	}
	for i, slot := range s.Slots {
		if slot.DayOfWeek < 0 || slot.DayOfWeek > 6 {
			return fmt.Errorf("radio schedule: slot %d: day_of_week %d out of range 0–6", i, slot.DayOfWeek)
		}
		if !hhmmPattern.MatchString(slot.Start) {
			return fmt.Errorf("radio schedule: slot %d: start %q is not HH:MM", i, slot.Start)
		}
		if !hhmmPattern.MatchString(slot.End) {
			return fmt.Errorf("radio schedule: slot %d: end %q is not HH:MM", i, slot.End)
		}
	}
	return nil
}

// ParseRadioSchedule decodes and validates a radio_shows.schedule JSONB value.
// A nil/empty raw message is treated as "no schedule" (nil, nil) — a show is
// not required to have a structured schedule. Use this anywhere the stored
// schedule is read so the validated shape is the only one callers ever see.
func ParseRadioSchedule(raw *json.RawMessage) (*RadioSchedule, error) {
	if raw == nil || len(*raw) == 0 || string(*raw) == "null" {
		return nil, nil
	}
	var sched RadioSchedule
	if err := json.Unmarshal(*raw, &sched); err != nil {
		return nil, fmt.Errorf("radio schedule: invalid JSON: %w", err)
	}
	if err := sched.Validate(); err != nil {
		return nil, err
	}
	return &sched, nil
}

// WindowForDate computes the frozen [startsAt, endsAt] air window for a broadcast
// that aired on airDate (a "2006-01-02" calendar date), from the weekly slot
// whose DayOfWeek matches that date's weekday, in the schedule's Timezone. This
// is the producer half of the PSY-1152 air-window subsystem for providers that
// carry a date but no air time (WFMU): the consumer is ComputeEpisodeStatus.
//
// An End <= Start slot wraps past midnight, so endsAt lands on the following day.
// Times are built in the schedule's IANA zone (DST-correct — never a fixed
// offset). Returns (nil, nil, nil) when no slot matches the weekday (an
// off-schedule / pop-up airing), so the caller leaves the episode windowless and
// ComputeEpisodeStatus settles it to aired/archived — never falsely live. When a
// weekday has more than one slot, the EARLIEST-starting slot wins deterministically
// (air_date is date-only, so a same-day double airing can't be disambiguated — we
// freeze a stable choice rather than depend on stored slot order). An End == Start
// slot is a degenerate full-24-hour window (treated like the midnight wrap).
//
// Known edge: a wall-clock time that falls in the once-a-year spring-forward gap
// (02:00–02:59 in US zones) doesn't exist, so time.Date normalizes it (e.g. an
// overnight slot ending 02:30 on the transition day → 01:30). This shifts that
// one airing's window by up to an hour, but FAILS SAFE: the window only ever
// closes earlier, so ComputeEpisodeStatus can drop "live" early but never reports
// a stale episode as falsely live. Not worth special-casing for a twice-a-year,
// 2 a.m.-bounded slot. (PSY-1238)
func (s *RadioSchedule) WindowForDate(airDate string) (startsAt, endsAt *time.Time, err error) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return nil, nil, fmt.Errorf("radio schedule: invalid timezone %q: %w", s.Timezone, err)
	}
	day, err := time.ParseInLocation("2006-01-02", airDate, loc)
	if err != nil {
		return nil, nil, fmt.Errorf("radio schedule: invalid air_date %q: %w", airDate, err)
	}
	weekday := int(day.Weekday()) // 0=Sunday..6=Saturday — matches RadioScheduleSlot.DayOfWeek
	// Pick the EARLIEST-starting slot for this weekday, deterministically. A show
	// with two same-weekday slots can't be disambiguated from a date-only
	// air_date, so we freeze a stable choice (HH:MM sorts lexicographically =
	// chronologically) rather than an arbitrary stored-array-order pick — the
	// latter could flip the "frozen" window if the scraper re-orders slots.
	var match *RadioScheduleSlot
	for i := range s.Slots {
		if s.Slots[i].DayOfWeek != weekday {
			continue
		}
		if _, _, ok := parseHHMM(s.Slots[i].Start); !ok {
			continue
		}
		if match == nil || s.Slots[i].Start < match.Start {
			match = &s.Slots[i]
		}
	}
	if match == nil {
		return nil, nil, nil // no (parseable) slot for this weekday
	}
	start, end, ok := match.TimesOnDay(day, loc)
	if !ok {
		return nil, nil, nil // malformed end on the chosen slot (defensive; slots are validated)
	}
	return &start, &end, nil
}

// TimesOnDay materializes this weekly slot as concrete instants on a given
// day in a given zone — the ONE definition of slot-time semantics, shared by
// WindowForDate (episode air windows) and the /radio guide's slot expansion
// (PSY-1053), so the wrap and parsing conventions can never silently fork
// between the two consumers. `day` supplies only the calendar date; its own
// clock time is ignored.
//
// An End <= Start slot wraps past midnight, so end lands on the following
// day; End == Start is a degenerate full-24-hour slot (fails safe — only
// ever over-reports coverage). ok=false when either time fails the HH:MM
// parse (defensive; slots are validated at write time).
func (sl RadioScheduleSlot) TimesOnDay(day time.Time, loc *time.Location) (start, end time.Time, ok bool) {
	sh, sm, sok := parseHHMM(sl.Start)
	eh, em, eok := parseHHMM(sl.End)
	if !sok || !eok {
		return time.Time{}, time.Time{}, false
	}
	start = time.Date(day.Year(), day.Month(), day.Day(), sh, sm, 0, 0, loc)
	end = time.Date(day.Year(), day.Month(), day.Day(), eh, em, 0, 0, loc)
	if !end.After(start) {
		end = end.AddDate(0, 0, 1)
	}
	return start, end, true
}

// parseHHMM parses a "HH:MM" 24-hour string into hour + minute, reusing the same
// hhmmPattern that Validate enforces at write time so the producer and the
// validator share ONE definition of a well-formed slot time. Schedule slots are
// HH:MM-validated by ParseRadioSchedule, so ok=false is defensive.
func parseHHMM(s string) (hour, minute int, ok bool) {
	if !hhmmPattern.MatchString(s) {
		return 0, 0, false
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

// RadioStation represents a radio station entity in the knowledge graph
type RadioStation struct {
	ID                  uint             `gorm:"primaryKey"`
	Name                string           `gorm:"not null"`
	Slug                string           `gorm:"not null;uniqueIndex"`
	Description         *string          `gorm:"column:description"`
	City                *string          `gorm:"column:city"`
	State               *string          `gorm:"column:state"`
	Country             *string          `gorm:"column:country;default:'US'"`
	Timezone            *string          `gorm:"column:timezone"`
	StreamURL           *string          `gorm:"column:stream_url"`
	StreamURLs          *json.RawMessage `gorm:"column:stream_urls;type:jsonb;default:'{}'"`
	Website             *string          `gorm:"column:website"`
	DonationURL         *string          `gorm:"column:donation_url"`
	DonationEmbedURL    *string          `gorm:"column:donation_embed_url"`
	LogoURL             *string          `gorm:"column:logo_url"`
	Social              *json.RawMessage `gorm:"column:social;type:jsonb;default:'{}'"`
	BroadcastType       string           `gorm:"column:broadcast_type;not null;default:'both'"`
	FrequencyMHz        *float64         `gorm:"column:frequency_mhz;type:decimal(5,1)"`
	PlaylistSource      *string          `gorm:"column:playlist_source"`
	PlaylistConfig      *json.RawMessage `gorm:"column:playlist_config;type:jsonb"`
	LastPlaylistFetchAt *time.Time       `gorm:"column:last_playlist_fetch_at"`
	// IsActive is retained for backward compatibility with existing read paths
	// (idx_radio_shows_active, GORM model default). LifecycleState is the new
	// operational signal (PSY-1131); reads should migrate to it over the P2/P4
	// pipeline rebuild.
	IsActive       bool      `gorm:"column:is_active;not null;default:true"`
	Source         string    `gorm:"column:source;not null;default:canonical"`
	LifecycleState string    `gorm:"column:lifecycle_state;not null;default:active"`
	NetworkID      *uint     `gorm:"column:network_id"`
	IsFlagship     bool      `gorm:"column:is_flagship;not null;default:false"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`

	// Relationships
	Shows   []RadioShow   `gorm:"foreignKey:StationID"`
	Network *RadioNetwork `gorm:"foreignKey:NetworkID"`
}

// TableName specifies the table name for RadioStation
func (RadioStation) TableName() string {
	return "radio_stations"
}

// RadioNetwork represents a parent brand grouping sibling radio_stations.
// Example: WFMU's 91.1 broadcast plus three stream-only sub-channels are
// all siblings under the WFMU network. Networks are flat (no hierarchy);
// stations link to networks via radio_stations.network_id.
type RadioNetwork struct {
	ID        uint      `gorm:"primaryKey"`
	Slug      string    `gorm:"not null;uniqueIndex"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`

	// Relationships
	Stations []RadioStation `gorm:"foreignKey:NetworkID"`
}

// TableName specifies the table name for RadioNetwork
func (RadioNetwork) TableName() string {
	return "radio_networks"
}

// RadioShow represents a recurring radio program on a station
type RadioShow struct {
	ID              uint             `gorm:"primaryKey"`
	StationID       uint             `gorm:"column:station_id;not null"`
	Name            string           `gorm:"not null"`
	Slug            string           `gorm:"not null;uniqueIndex"`
	HostName        *string          `gorm:"column:host_name"`
	Description     *string          `gorm:"column:description"`
	ScheduleDisplay *string          `gorm:"column:schedule_display"`
	Schedule        *json.RawMessage `gorm:"column:schedule;type:jsonb"`
	// ScheduleLocked: when true, the weekly WFMU scrape (PSY-1159) leaves this show's
	// schedule alone — an admin curated it by hand (PSY-1186). UpdateShow auto-locks on a
	// structured-schedule edit; clearing it (schedule_locked=false) resumes auto-scrape.
	// Settable from the admin show editor's "Lock schedule" toggle (PSY-1193), or implicitly
	// via a structured-schedule edit on the API.
	ScheduleLocked bool             `gorm:"column:schedule_locked;not null;default:false"`
	GenreTags      *json.RawMessage `gorm:"column:genre_tags;type:jsonb;default:'[]'"`
	ArchiveURL     *string          `gorm:"column:archive_url"`
	ImageURL       *string          `gorm:"column:image_url"`
	ExternalID     *string          `gorm:"column:external_id"`
	// LastPlaylistFetchAt is the per-show incremental-fetch watermark (PSY-1272):
	// the high-water mark of "playlists durably imported up to here" for THIS show.
	// FetchNewEpisodes computes each show's `since` from it and advances it per show
	// (only when that show's own fetch + import made progress), so a single
	// persistently-failing show (e.g. a renamed external_id) holds its OWN watermark
	// and recovers its gap once it succeeds again — independent of its siblings.
	// radio_stations.last_playlist_fetch_at remains the total-station roll-up the
	// PSY-1269 sustained-outage janitor reads. NULL = never fetched (cold-start to
	// the floor; see fetchSince).
	LastPlaylistFetchAt *time.Time `gorm:"column:last_playlist_fetch_at"`
	// ConsecutiveFetchFailures counts back-to-back provider fetch failures for this
	// show's episode-listing call (PSY-1274). Incremented when the fetch errors, reset
	// on the next successful fetch — a fetch returning zero episodes is a SUCCESS, so
	// infrequent shows (monthly/biweekly) never accumulate a streak and the signal is
	// cadence-independent (the reason a staleness threshold on LastPlaylistFetchAt
	// can't work here; see PSY-1241/PSY-1269). The janitor escalates a streak past
	// radioShowFetchFailureEscalationThreshold on an otherwise-healthy station.
	ConsecutiveFetchFailures int `gorm:"column:consecutive_fetch_failures;not null;default:0"`
	// IsActive retained for backward compatibility; LifecycleState is the new
	// operational signal (PSY-1131).
	IsActive       bool      `gorm:"column:is_active;not null;default:true"`
	Source         string    `gorm:"column:source;not null;default:provider"`
	LifecycleState string    `gorm:"column:lifecycle_state;not null;default:active"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`

	// Relationships
	Station  RadioStation   `gorm:"foreignKey:StationID"`
	Episodes []RadioEpisode `gorm:"foreignKey:ShowID"`
}

// TableName specifies the table name for RadioShow
func (RadioShow) TableName() string {
	return "radio_shows"
}

// RadioEpisode represents a single broadcast of a radio show
type RadioEpisode struct {
	ID              uint             `gorm:"primaryKey"`
	ShowID          uint             `gorm:"column:show_id;not null"`
	Title           *string          `gorm:"column:title"`
	AirDate         string           `gorm:"column:air_date;type:date;not null"`
	AirTime         *string          `gorm:"column:air_time;type:time"`
	DurationMinutes *int             `gorm:"column:duration_minutes"`
	Description     *string          `gorm:"column:description"`
	ArchiveURL      *string          `gorm:"column:archive_url"`
	MixcloudURL     *string          `gorm:"column:mixcloud_url"`
	ExternalID      *string          `gorm:"column:external_id"`
	GenreTags       *json.RawMessage `gorm:"column:genre_tags;type:jsonb"`
	MoodTags        *json.RawMessage `gorm:"column:mood_tags;type:jsonb"`
	PlayCount       int              `gorm:"column:play_count;not null;default:0"`
	// Status is the episode lifecycle state. NOTE (PSY-1152): the API does NOT
	// read this stored column for live/aired display — GetEpisodes recomputes it
	// on read via ComputeEpisodeStatus, because "live" is a function of now and a
	// stored value goes stale the instant the window ends. The persisted value is
	// only an import-time snapshot, kept fresh by the janitor (PSY-1155, not yet
	// shipped) — do NOT query it for live state until then. Windowless episodes
	// are 'aired', never 'live'.
	Status string `gorm:"column:status;not null;default:aired"`
	// StartsAt/EndsAt are the real air window (timezone-aware), NULL when the
	// provider supplies no time. The basis for the air-window "live" computation.
	StartsAt *time.Time `gorm:"column:starts_at"`
	EndsAt   *time.Time `gorm:"column:ends_at"`
	// PlaylistState/PlaylistFetchedAt track the playlist-fetch lifecycle,
	// decoupled from episode Status.
	PlaylistState     string     `gorm:"column:playlist_state;not null;default:pending"`
	PlaylistFetchedAt *time.Time `gorm:"column:playlist_fetched_at"`
	// PlaylistFetchAttempts counts FAILED post-air playlist re-fetches (PSY-1154). A
	// fetch that returns plays settles to 'complete' and never increments it. It is
	// an observability counter and the RadioBackfillMaxAttempts ceiling — NOT the
	// give-up terminator, which is the air-time deadline in PlanPlaylistFetch
	// (PSY-1562).
	PlaylistFetchAttempts int `gorm:"column:playlist_fetch_attempts;not null;default:0"`
	// PlaylistBackfillAttemptedAt is when the last POST-AIR playlist attempt ran
	// (PSY-1562), the durable memo that rate-limits retries. Distinct from
	// PlaylistFetchedAt, which any fetch stamps including a live-window refresh: only
	// aired attempts are cooled down. NULL means no post-air attempt yet.
	PlaylistBackfillAttemptedAt *time.Time `gorm:"column:playlist_backfill_attempted_at"`
	CreatedAt                   time.Time  `gorm:"not null"`
	UpdatedAt                   time.Time  `gorm:"column:updated_at;not null"`

	// Relationships
	Show  RadioShow   `gorm:"foreignKey:ShowID"`
	Plays []RadioPlay `gorm:"foreignKey:EpisodeID"`
}

// TableName specifies the table name for RadioEpisode
func (RadioEpisode) TableName() string {
	return "radio_episodes"
}

// RadioPlay represents a single track played in a radio episode
type RadioPlay struct {
	ID        uint `gorm:"primaryKey"`
	EpisodeID uint `gorm:"column:episode_id;not null"`
	Position  int  `gorm:"column:position;not null;default:0"`

	// Raw metadata from source (always stored, never overwritten)
	ArtistName  string  `gorm:"column:artist_name;not null"`
	TrackTitle  *string `gorm:"column:track_title"`
	AlbumTitle  *string `gorm:"column:album_title"`
	LabelName   *string `gorm:"column:label_name"`
	ReleaseYear *int    `gorm:"column:release_year"`

	// Curation signals
	IsNew             bool    `gorm:"column:is_new;not null;default:false"`
	RotationStatus    *string `gorm:"column:rotation_status"`
	DJComment         *string `gorm:"column:dj_comment"`
	IsLivePerformance bool    `gorm:"column:is_live_performance;not null;default:false"`
	IsRequest         bool    `gorm:"column:is_request;not null;default:false"`

	// MatchState is the explicit matching lifecycle (PSY-1131), replacing the
	// implicit "ArtistID IS NULL == unmatched". Defaults to 'unmatched'.
	MatchState string `gorm:"column:match_state;not null;default:unmatched"`
	// ProviderPlayID is a stable provider-supplied play id (e.g. KEXP) used as
	// the dedup key when present; NULL falls back to the content hash.
	ProviderPlayID *string `gorm:"column:provider_play_id"`
	// DedupKey is a GENERATED STORED column (provider_play_id, else a content
	// hash). Read-only from Go ("->" tag): Postgres computes it, GORM never
	// writes it. Backs the (episode_id, dedup_key) unique index.
	DedupKey string `gorm:"->;column:dedup_key"`

	// Linked to our knowledge graph (populated by matching engine, nullable)
	ArtistID  *uint `gorm:"column:artist_id"`
	ReleaseID *uint `gorm:"column:release_id"`
	LabelID   *uint `gorm:"column:label_id"`

	// External IDs for cross-referencing and deduplication
	MusicBrainzRecordingID *string `gorm:"column:musicbrainz_recording_id"`
	MusicBrainzArtistID    *string `gorm:"column:musicbrainz_artist_id"`
	MusicBrainzReleaseID   *string `gorm:"column:musicbrainz_release_id"`

	// Timing
	AirTimestamp *time.Time `gorm:"column:air_timestamp"`
	CreatedAt    time.Time  `gorm:"not null"`

	// Relationships
	Episode RadioEpisode `gorm:"foreignKey:EpisodeID"`
	Artist  *Artist      `gorm:"foreignKey:ArtistID"`
	Release *Release     `gorm:"foreignKey:ReleaseID"`
	Label   *Label       `gorm:"foreignKey:LabelID"`
}

// TableName specifies the table name for RadioPlay
func (RadioPlay) TableName() string {
	return "radio_plays"
}

// Import job status constants
const (
	RadioImportJobStatusPending   = "pending"
	RadioImportJobStatusRunning   = "running"
	RadioImportJobStatusCompleted = "completed"
	RadioImportJobStatusFailed    = "failed"
	RadioImportJobStatusCancelled = "cancelled"
)

// RadioImportJob represents an async import job for a radio show's episodes.
type RadioImportJob struct {
	ID                 uint         `gorm:"primaryKey" json:"id"`
	ShowID             uint         `gorm:"not null" json:"show_id"`
	Show               RadioShow    `gorm:"foreignKey:ShowID" json:"-"`
	StationID          uint         `gorm:"not null" json:"station_id"`
	Station            RadioStation `gorm:"foreignKey:StationID" json:"-"`
	Since              string       `gorm:"type:date;not null" json:"since"`
	Until              string       `gorm:"type:date;not null" json:"until"`
	Status             string       `gorm:"type:varchar(20);not null;default:pending" json:"status"`
	EpisodesFound      int          `gorm:"not null;default:0" json:"episodes_found"`
	EpisodesImported   int          `gorm:"not null;default:0" json:"episodes_imported"`
	PlaysImported      int          `gorm:"not null;default:0" json:"plays_imported"`
	PlaysMatched       int          `gorm:"not null;default:0" json:"plays_matched"`
	CurrentEpisodeDate *string      `json:"current_episode_date,omitempty"`
	ErrorLog           *string      `gorm:"type:text" json:"error_log,omitempty"`
	StartedAt          *time.Time   `json:"started_at,omitempty"`
	CompletedAt        *time.Time   `json:"completed_at,omitempty"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

// TableName specifies the table name for RadioImportJob
func (RadioImportJob) TableName() string { return "radio_import_jobs" }

// RadioSyncRun is one execution of any ingestion path (scheduled fetch/discover,
// manual sync, historic backfill, or rematch) against a station — the
// observability backbone (PSY-1132). Opened with Status 'running' at the start of
// a run and resolved to a terminal status. Unifies and replaces RadioImportJob in
// P2; WindowStart/WindowEnd carry the old Since/Until historic-backfill range so
// admin-triggered historic re-ingestion stays parameterizable and observable. (The
// P2 unification must widen radio_import_jobs' int4 ids when carrying rows over —
// this table is BIGINT throughout, matching the BIGSERIAL parent PKs.)
type RadioSyncRun struct {
	ID        uint  `gorm:"primaryKey"`
	// StationID is NULL only on global rematch runs (run_type='rematch' with no
	// station filter). All discover/fetch/backfill/station-scoped runs set it.
	StationID *uint `gorm:"column:station_id"`
	// ShowID is set on backfill runs AND on show-SCOPED fetch runs (PSY-1333 slot
	// fetch). For run_type='fetch', non-NULL show_id means single-show scope:
	// station-scale aggregates over fetch runs (volume-anomaly baseline, station
	// health rates) MUST exclude those rows or the per-boundary scoped volume
	// swamps the sweep signal — see detectVolumeAnomaly / computeStationRates.
	ShowID  *uint  `gorm:"column:show_id"`
	RunType string `gorm:"column:run_type;not null"`
	// Trigger maps to the trigger_source column (`trigger` is a reserved SQL word).
	Trigger string `gorm:"column:trigger_source;not null"`
	Status  string `gorm:"column:status;not null;default:running"`
	// WindowStart/WindowEnd are the requested historic backfill range; NULL on a
	// normal scheduled/fetch run. Replaces RadioImportJob.Since/Until.
	WindowStart *time.Time `gorm:"column:window_start"`
	WindowEnd   *time.Time `gorm:"column:window_end"`
	// StartedAt: the P2 write path MUST set this explicitly at run-open (time.Now())
	// rather than rely on the SQL DEFAULT NOW() — the default fires only for a raw
	// INSERT that omits the column, and GORM's skip-zero-value-with-default behavior
	// (cf. the bool gotcha) makes deferring to it subtle. Same for Status above (set
	// it to a status constant; don't lean on default:running). FinishedAt is nil
	// while Status == running and set on the terminal transition (DB
	// radio_sync_runs_lifecycle_check enforces the running<=>NULL pairing).
	StartedAt  time.Time  `gorm:"column:started_at;not null;default:now()"`
	FinishedAt *time.Time `gorm:"column:finished_at"`

	EpisodesFound    int `gorm:"column:episodes_found;not null;default:0"`
	EpisodesImported int `gorm:"column:episodes_imported;not null;default:0"`
	PlaysImported    int `gorm:"column:plays_imported;not null;default:0"`
	PlaysMatched     int `gorm:"column:plays_matched;not null;default:0"`
	PlaysUnmatched   int `gorm:"column:plays_unmatched;not null;default:0"`
	PlaysDropped     int `gorm:"column:plays_dropped;not null;default:0"`
	PlaysTruncated   int `gorm:"column:plays_truncated;not null;default:0"`

	BreakerSkipped     bool    `gorm:"column:breaker_skipped;not null;default:false"`
	CurrentEpisodeDate *string `gorm:"column:current_episode_date"`

	CreatedAt time.Time `gorm:"column:created_at;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`

	// Relationships
	Station RadioStation        `gorm:"foreignKey:StationID"`
	Show    *RadioShow          `gorm:"foreignKey:ShowID"`
	Errors  []RadioSyncRunError `gorm:"foreignKey:SyncRunID"`
}

// TableName specifies the table name for RadioSyncRun
func (RadioSyncRun) TableName() string {
	return "radio_sync_runs"
}

// RadioSyncRunError is one structured, categorized error recorded against a
// RadioSyncRun (PSY-1132). EpisodeRef is a soft reference (provider date/external
// id), deliberately NOT an FK, so errors about episodes that failed to be created
// are still recordable.
type RadioSyncRunError struct {
	ID         uint      `gorm:"primaryKey"`
	SyncRunID  uint      `gorm:"column:sync_run_id;not null"`
	Category   string    `gorm:"column:category;not null"`
	Detail     *string   `gorm:"column:detail"`
	EpisodeRef *string   `gorm:"column:episode_ref"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`

	// Relationships
	SyncRun RadioSyncRun `gorm:"foreignKey:SyncRunID"`
}

// TableName specifies the table name for RadioSyncRunError
func (RadioSyncRunError) TableName() string {
	return "radio_sync_run_errors"
}

// RadioStationHealth is the derived operational state of a station (PSY-1132),
// isolated from the durable RadioStation entity (Code Complete: separate volatile
// operational state) and persisted so the circuit breaker survives restarts. One
// row per station. Rate fields are nullable: NULL = never computed (distinct from
// 0.0 = computed and genuinely zero).
type RadioStationHealth struct {
	StationID           uint       `gorm:"column:station_id;primaryKey"`
	LastSuccessAt       *time.Time `gorm:"column:last_success_at"`
	LastRunAt           *time.Time `gorm:"column:last_run_at"`
	ConsecutiveFailures int        `gorm:"column:consecutive_failures;not null;default:0"`
	BreakerState        string     `gorm:"column:breaker_state;not null;default:closed"`
	BreakerTrippedAt    *time.Time `gorm:"column:breaker_tripped_at"`
	RecentSuccessRate   *float64   `gorm:"column:recent_success_rate"`
	PlayMatchRate       *float64   `gorm:"column:play_match_rate"`
	ZeroPlayEpisodeRate *float64   `gorm:"column:zero_play_episode_rate"`
	CreatedAt           time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;not null"`

	// Relationships
	Station RadioStation `gorm:"foreignKey:StationID"`
}

// TableName specifies the table name for RadioStationHealth
func (RadioStationHealth) TableName() string {
	return "radio_station_health"
}

// RadioArtistAffinity represents co-occurrence of two artists across radio playlists.
// The composite primary key is (artist_a_id, artist_b_id).
// A CHECK constraint ensures artist_a_id < artist_b_id (canonical ordering).
type RadioArtistAffinity struct {
	ArtistAID         uint      `gorm:"column:artist_a_id;primaryKey"`
	ArtistBID         uint      `gorm:"column:artist_b_id;primaryKey"`
	CoOccurrenceCount int       `gorm:"column:co_occurrence_count;not null;default:0"`
	ShowCount         int       `gorm:"column:show_count;not null;default:0"`
	StationCount      int       `gorm:"column:station_count;not null;default:0"`
	LastCoOccurrence  *string   `gorm:"column:last_co_occurrence;type:date"`
	UpdatedAt         time.Time `gorm:"not null"`

	// BackboneSignificance is the disparity-filter significance of this edge (PSY-1261) — the
	// smaller of its two endpoints' p-values, computed over the full radio graph. LOWER = stronger;
	// an edge is in the backbone at level alpha iff this is < alpha. NULL until the nightly backbone
	// pass runs; 0 for an edge to a degree-1 node (always kept). See catalog.DisparitySignificance.
	BackboneSignificance *float64 `gorm:"column:backbone_significance"`

	// Relationships
	ArtistA Artist `gorm:"foreignKey:ArtistAID"`
	ArtistB Artist `gorm:"foreignKey:ArtistBID"`
}

// TableName specifies the table name for RadioArtistAffinity
func (RadioArtistAffinity) TableName() string {
	return "radio_artist_affinity"
}
