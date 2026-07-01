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

// RadioBackfillMaxAttempts is the number of failed post-air playlist re-fetches
// after which an aired episode is marked playlist_state='unavailable' and stops
// being retried (PSY-1154). A "failed attempt" is a post-air fetch that returned
// no playlist (an empty broadcast, a pulled show, or a provider error) — a fetch
// that returns plays settles the episode to 'complete' and never increments the
// counter. Modeled as a const (like radioCircuitBreakerThreshold), not env-tunable:
// the value is a data-quality policy, not an operational cadence. The backfill
// cadence (sweep interval, lookback window) IS env-tunable — see radio_fetch_service.go.
//
// Give-up budget: a windowless (WFMU) or start-only (NTS) episode is "aired" the
// moment it has started (no live window guards it), so attempts begin accruing at the
// first post-start fetch. The effective budget before 'unavailable' is therefore
// ~ maxAttempts × sweep-interval (default 5 × 1h = ~5h), which comfortably covers the
// usual minutes-to-hours playlist-publish delay; a provider that publishes a playlist
// slower than that budget can strand an episode at 'unavailable' until the janitor
// (PSY-1155) re-attempts it. Widen RADIO_BACKFILL_INTERVAL_HOURS for slow providers.
const RadioBackfillMaxAttempts = 5

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

// ComputePlaylistState decides an episode's playlist_state after one playlist fetch
// attempt, along with its (possibly incremented) attempt count (PSY-1154). It is the
// single completeness policy shared by the first-import path and the post-air backfill
// re-fetch path, kept pure so the transition table is unit-testable without a DB.
//
//   - fetch returned plays + episode has aired  → complete   (the final post-air playlist)
//   - fetch returned plays + episode still live → partial    (snapshot; more plays coming)
//   - no plays + episode not yet aired          → pending     (live/scheduled with nothing yet — normal)
//   - no plays + episode aired                  → a FAILED post-air attempt: increment the
//     counter; at maxAttempts give up → unavailable, else stay pending (eligible to retry).
//
// "no plays" covers both a fetch error and a legitimately empty playlist (provider
// returned zero tracks) — for the give-up policy they are the same: we still don't
// have a playlist. A failed re-fetch normalizes a prior 'partial' back to 'pending';
// both remain eligible, and the already-imported plays are untouched, so this is
// behavior-neutral.
func ComputePlaylistState(isAired, hasPlays, fetchFailed bool, attempts, maxAttempts int) (state string, newAttempts int) {
	if hasPlays && !fetchFailed {
		if isAired {
			return RadioPlaylistStateComplete, attempts
		}
		return RadioPlaylistStatePartial, attempts
	}
	if !isAired {
		// Live or scheduled with no playlist yet — expected, not a failure.
		return RadioPlaylistStatePending, attempts
	}
	// Aired but still no playlist: a genuine failed post-air attempt.
	newAttempts = attempts + 1
	if newAttempts >= maxAttempts {
		return RadioPlaylistStateUnavailable, newAttempts
	}
	return RadioPlaylistStatePending, newAttempts
}

// ShouldBackfillPlaylist reports whether an aired episode still needs a post-air
// playlist re-fetch (PSY-1154). It is the single source of truth for backfill
// eligibility — both importEpisode's existing-row branch and the backfill ticker's
// candidate query refine to it, so the in-flight re-fetch decision and the sweep
// selection can never drift. An episode is eligible when it is still incomplete
// (pending/partial — never complete/unavailable), has attempts left, and has aired
// (a windowless episode counts as aired; scheduled/live episodes are skipped — their
// playlist is legitimately not final yet).
func ShouldBackfillPlaylist(startsAt, endsAt *time.Time, playlistState string, attempts, maxAttempts int, now time.Time) bool {
	if playlistState != RadioPlaylistStatePending && playlistState != RadioPlaylistStatePartial {
		return false
	}
	if attempts >= maxAttempts {
		return false
	}
	// Pass pending so the result is the pure time-phase (scheduled/live/aired),
	// never 'archived' — completeness is already handled by the state check above.
	return ComputeEpisodeStatus(startsAt, endsAt, RadioPlaylistStatePending, now) == RadioEpisodeStatusAired
}

// NormalizeScheduledPlaylistState enforces the invariant that a not-yet-aired
// (scheduled) episode never carries a terminal/exhausted playlist state (PSY-1285).
// A scheduled episode's playlist legitimately doesn't exist yet, so it must be
// 'pending' with zero burned backfill attempts. The bad state arises because a
// WINDOWLESS episode is settled to 'aired' (no window to wait through), so the
// post-air backfill burns attempts on it → 'unavailable'; if it is later given a
// FUTURE window — by PSY-1283's schedule correction or a heal-on-relist — it becomes
// 'scheduled', but its playlist_state would otherwise stay stuck 'unavailable'.
//
// Returns the (possibly reset) state + attempts. It is a no-op for anything that is
// NOT scheduled (an aired/live/windowless episode keeps its state — a windowless
// 'aired' episode legitimately reaching 'unavailable' is PSY-1287's concern, not this
// invariant). For a scheduled episode it clears ONLY the stranded/exhausted shape — a
// terminal 'unavailable', or burned backfill attempts on a STILL-pending episode (which
// a not-yet-aired episode can never have legitimately earned) — and leaves a scheduled
// 'partial'/'complete' (and its play count + attempts) intact, since those carry real
// plays and are not the AC#2 'unavailable' violation.
func NormalizeScheduledPlaylistState(startsAt, endsAt *time.Time, playlistState string, attempts int, now time.Time) (state string, newAttempts int) {
	if ComputeEpisodeStatus(startsAt, endsAt, RadioPlaylistStatePending, now) != RadioEpisodeStatusScheduled {
		return playlistState, attempts // not a not-yet-aired episode → leave untouched
	}
	if playlistState == RadioPlaylistStateUnavailable {
		return RadioPlaylistStatePending, 0 // terminal give-up on a not-yet-aired episode (AC#2)
	}
	if playlistState == RadioPlaylistStatePending && attempts > 0 {
		return RadioPlaylistStatePending, 0 // a pending scheduled episode can't have earned attempts
	}
	return playlistState, attempts // pending+0, or a 'partial'/'complete' carrying real plays
}

// NormalizeWindowHealPlaylistState clears a false playlist give-up from the windowless
// era once a real air window is frozen (PSY-1287). A WFMU episode with no matching
// schedule slot (PSY-1283 off-by-one) stayed windowless → immediately 'aired' → the
// post-air backfill burned attempts on empty pre-air playlists → 'unavailable'. After
// the schedule is corrected and a window is assigned on re-list, reset so backfill
// runs at the correct lifecycle phase. Only when play_count==0 (no imported plays).
func NormalizeWindowHealPlaylistState(hadWindow bool, startsAt *time.Time, playlistState string, attempts, playCount int) (state string, newAttempts int) {
	if hadWindow || startsAt == nil || playCount != 0 {
		return playlistState, attempts
	}
	if playlistState == RadioPlaylistStateUnavailable || attempts > 0 {
		return RadioPlaylistStatePending, 0
	}
	return playlistState, attempts
}

// NormalizeStrandedWindowlessPlaylistState re-opens backfill for a windowless aired
// episode that gave up with zero plays (PSY-1287). Lets the backfill candidate query
// find the show again so a re-list can heal the window from a corrected schedule.
// A windowless episode with real plays is left untouched.
func NormalizeStrandedWindowlessPlaylistState(startsAt *time.Time, playlistState string, attempts, playCount int, now time.Time) (state string, newAttempts int) {
	if startsAt != nil || playCount != 0 {
		return playlistState, attempts
	}
	if playlistState != RadioPlaylistStateUnavailable {
		return playlistState, attempts
	}
	if ComputeEpisodeStatus(nil, nil, RadioPlaylistStatePending, now) != RadioEpisodeStatusAired {
		return playlistState, attempts
	}
	return RadioPlaylistStatePending, 0
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
	sh, sm, _ := parseHHMM(match.Start) // ok: filtered above
	eh, em, ok := parseHHMM(match.End)
	if !ok {
		return nil, nil, nil // malformed end on the chosen slot (defensive; slots are validated)
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), sh, sm, 0, 0, loc)
	end := time.Date(day.Year(), day.Month(), day.Day(), eh, em, 0, 0, loc)
	if !end.After(start) {
		// End <= Start wraps past midnight; End == Start is a degenerate full
		// 24-hour slot (fails safe — only ever over-reports "live").
		end = end.AddDate(0, 0, 1)
	}
	return &start, &end, nil
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
	// PlaylistFetchAttempts counts FAILED post-air playlist re-fetches (PSY-1154).
	// At RadioBackfillMaxAttempts the backfill loop gives up → playlist_state
	// 'unavailable'. A fetch that returns plays settles to 'complete' and never
	// increments this.
	PlaylistFetchAttempts int       `gorm:"column:playlist_fetch_attempts;not null;default:0"`
	CreatedAt             time.Time `gorm:"not null"`
	UpdatedAt             time.Time `gorm:"column:updated_at;not null"`

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
	ID        uint   `gorm:"primaryKey"`
	StationID uint   `gorm:"column:station_id;not null"`
	ShowID    *uint  `gorm:"column:show_id"`
	RunType   string `gorm:"column:run_type;not null"`
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
