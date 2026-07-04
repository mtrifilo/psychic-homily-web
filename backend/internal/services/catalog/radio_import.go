package catalog

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// PSY-885: VARCHAR(500) limit on radio_plays text columns (artist_name,
// track_title, album_title, label_name). Counted in runes (not bytes) so
// truncation respects multi-byte boundaries — matches the Postgres semantics
// for varchar length, which is character-count, not byte-count.
const radioPlayVarcharMaxRunes = 500

// getProvider returns the appropriate RadioPlaylistProvider for a station's playlist_source.
func (s *RadioService) getProvider(source string) (RadioPlaylistProvider, error) {
	if s.playlistProviderFactory != nil {
		return s.playlistProviderFactory(source)
	}
	switch source {
	case catalogm.PlaylistSourceKEXP:
		return NewKEXPProvider(), nil
	case catalogm.PlaylistSourceWFMU:
		return NewWFMUProvider(), nil
	case catalogm.PlaylistSourceNTS:
		return NewNTSProvider(), nil
	case catalogm.PlaylistSourceManual:
		// "manual" is a valid, intentional source: playlists are curated by hand,
		// so there is no automated provider. The scheduled cycle never reaches
		// here for manual stations — GetActiveStationsWithPlaylistSource excludes
		// them — so this guards the manual import-job trigger path, returning a
		// clear error without the default branch's misconfiguration alert.
		return nil, fmt.Errorf("playlist source %q is manual; no automated provider", source)
	default:
		// A truly unrecognized playlist_source silently breaks ALL playlist
		// import for the station — every show imports 0 tracks with no obvious
		// cause. (PSY-927: the value "wfmu_html", which no provider handles, had
		// been set on the WFMU station and zeroed out every show's tracks.) Log
		// loudly with the offending value so a misconfigured station is visible
		// rather than disappearing into a per-cycle error count.
		slog.Default().Error("radio import: unsupported playlist source",
			"playlist_source", source,
			"valid", catalogm.PlaylistSources,
		)
		return nil, fmt.Errorf("unsupported playlist source: %s", source)
	}
}

// stationScopedShowDiscoverer is implemented by providers whose discovery
// source mixes multiple stations' shows in one index. WFMU's DJ index spans
// the 91.1 broadcast plus three stream-only channels; importing it unfiltered
// for every family station duplicated the entire catalog under each channel
// (PSY-1073). When a provider implements this, discovery flows call the
// station-scoped method so each station only receives shows that air on its
// stream.
type stationScopedShowDiscoverer interface {
	DiscoverShowsForStation(stationSlug string) ([]RadioShowImport, error)
}

// discoverShowsForStation routes discovery through the station-scoped path
// when the provider supports it, falling back to the provider-wide index for
// single-station providers (KEXP, NTS).
func discoverShowsForStation(provider RadioPlaylistProvider, station *catalogm.RadioStation) ([]RadioShowImport, error) {
	if scoped, ok := provider.(stationScopedShowDiscoverer); ok {
		return scoped.DiscoverShowsForStation(station.Slug)
	}
	return provider.DiscoverShows()
}

// parseImportDate parses an import-window bound (since/until). Backfill windows
// now arrive as RunStationSync formats them (date-only "2026-03-02"), but the
// defensive normalizeDateString trim is kept so a Postgres DATE-column round-trip
// form ("2026-03-02T00:00:00Z") still parses if a future caller passes one
// (PSY-927; the original import-job path that round-tripped from the DB is
// retired in PSY-1135).
func parseImportDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", normalizeDateString(s))
}

// normalizeDateString strips any time component from a date string so a value
// always parses as YYYY-MM-DD. Postgres DATE columns round-trip through GORM into
// Go strings as "2026-04-01T00:00:00Z" even though the column only holds a date;
// this trims it back to the 10-char form.
func normalizeDateString(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// ImportStation runs a full import: discover shows + fetch episodes for the last N days.
//
// NOTE (PSY-1135): this is a legacy full-import helper that does NOT route through
// RunStationSync, so it leaves NO radio_sync_runs trace. It is intentionally not
// wired to any admin route or ticker. Do NOT expose it as an ingestion entry point
// — a new "full import" action must go through RunStationSync (discover then
// backfill) so every run is observable. Kept only because it predates the
// orchestrator and is still part of the service contract.
func (s *RadioService) ImportStation(stationID uint, backfillDays int) (*contracts.RadioImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station catalogm.RadioStation
	if err := s.db.First(&station, stationID).Error; err != nil {
		return nil, fmt.Errorf("station not found: %w", err)
	}

	if station.PlaylistSource == nil || *station.PlaylistSource == "" {
		return nil, fmt.Errorf("station %d has no playlist source configured", stationID)
	}

	provider, err := s.getProvider(*station.PlaylistSource)
	if err != nil {
		return nil, err
	}
	defer closeProvider(provider)

	result := &contracts.RadioImportResult{}

	// 1. Discover shows (station-scoped for multi-stream providers, PSY-1073). Errors
	// go through recordImportError so the Errors/CategorizedErrors invariant holds here
	// too — even though this legacy helper no longer routes through RunStationSync
	// (PSY-1135), keeping it consistent prevents the dead path from becoming a landmine
	// if it is ever re-wired (PSY-1141 review).
	importedShows, err := discoverShowsForStation(provider, &station)
	if err != nil {
		recordImportError(result, categorizeRunError(err), fmt.Sprintf("discover shows: %v", err), nil)
		return result, nil
	}

	showMap := make(map[string]uint) // external_id → our show ID
	for _, importShow := range importedShows {
		showID, _, err := s.upsertRadioShow(stationID, importShow)
		if err != nil {
			recordImportError(result, categorizeRunError(err), fmt.Sprintf("upsert show %s: %v", importShow.Name, err), nil)
			continue
		}
		showMap[importShow.ExternalID] = showID
		result.ShowsDiscovered++
	}

	// 2. Fetch episodes for each show
	since := time.Now().AddDate(0, 0, -backfillDays)
	var fetchAttempts, fetchSuccesses, episodesReturned int
	for extID, showID := range showMap {
		fetchAttempts++
		episodes, err := provider.FetchNewEpisodes(extID, since, time.Time{})
		if err != nil {
			recordImportError(result, categorizeRunError(err), fmt.Sprintf("fetch episodes for show %s: %v", extID, err), nil)
			continue
		}
		fetchSuccesses++
		episodesReturned += len(episodes)

		for _, ep := range episodes {
			epResult, err := s.importEpisode(showID, ep, provider)
			if err != nil {
				ref := ep.ExternalID
				recordImportError(result, categorizeRunError(err), fmt.Sprintf("import episode %s: %v", ep.ExternalID, err), &ref)
				continue
			}
			accumulateEpisodeResult(result, ep.ExternalID, epResult)
		}
	}

	// Advance the watermark through the shared gate (PSY-1269): a total failure here
	// holds it stale too. This legacy full-import path is unwired in production today,
	// but routing it through advanceLastFetch keeps the watermark invariant intact if
	// it is ever re-exposed (it remains on the contracts.RadioService interface). Only
	// the station roll-up is advanced — the per-show watermarks (PSY-1272) are left
	// untouched, since this is a bounded historical import, not the incremental frontier.
	s.advanceLastFetch(stationID, fetchAttempts, fetchSuccesses, episodesReturned, result.EpisodesImported)

	return result, nil
}

// accumulateEpisodeResult folds one episode's import outcome into the running
// station/show import result. It is the single place that decides how each
// per-episode signal surfaces, so the three import orchestrators (ImportStation,
// FetchNewEpisodes, importShowEpisodesWithProgress) stay consistent.
//
// PSY-1119: a FetchError means the episode's playlist fetch failed and every
// play for it was lost. It increments EpisodeFetchErrors and is recorded in
// Errors so the failure can never again pass as a clean 0-play success. The
// episode row WAS created, so EpisodesImported still counts it — the dedicated
// error counter (not the imported count) is what tells callers the import
// finished with episode errors. A legitimately empty playlist leaves FetchError
// empty and stays a clean success. MatchPersistErrors aggregates plays that
// matched but could not be saved.
func accumulateEpisodeResult(result *contracts.RadioImportResult, episodeExternalID string, epResult *contracts.EpisodeImportResult) {
	result.EpisodesImported++
	result.PlaysImported += epResult.PlaysImported
	result.PlaysMatched += epResult.PlaysMatched

	ref := episodeExternalID
	if epResult.FetchError != "" {
		result.EpisodeFetchErrors++
		cat := epResult.FetchErrorCategory
		if cat == "" {
			cat = catalogm.RadioSyncRunErrorProviderUnreachable
		}
		recordImportError(result, cat,
			fmt.Sprintf("fetch failed for episode %s: %s", episodeExternalID, epResult.FetchError), &ref)
	}
	if epResult.MatchPersistErrors > 0 {
		result.MatchPersistErrors += epResult.MatchPersistErrors
		recordImportError(result, catalogm.RadioSyncRunErrorMatchPersistError,
			fmt.Sprintf("episode %s: %d play matches failed to persist", episodeExternalID, epResult.MatchPersistErrors), &ref)
	}
	if epResult.DropSummary != "" {
		// Pick the category by precedence: a real drop (data loss) outranks a
		// salvaged truncation. A truncation-only episode records as 'truncation' —
		// the case the old string heuristic could never reach, since summarizeDrops
		// always prefixes "dropped N plays:" which categorizeErrorString bucketed as
		// validation_drop. The detail still names both classes. PSY-1141.
		cat := catalogm.RadioSyncRunErrorTruncation
		if epResult.DroppedPlays > 0 {
			cat = catalogm.RadioSyncRunErrorValidationDrop
		}
		recordImportError(result, cat,
			fmt.Sprintf("episode %s: %s", episodeExternalID, epResult.DropSummary), &ref)
	}
}

// recordImportError appends a per-import error to BOTH the human Errors slice (the
// admin log line) and the structured CategorizedErrors slice (the pre-typed category
// the sync layer records into radio_sync_run_errors, with no substring
// re-categorization). The two slices stay parallel — same order, same length. PSY-1141.
func recordImportError(result *contracts.RadioImportResult, category, detail string, episodeRef *string) {
	result.Errors = append(result.Errors, detail)
	result.CategorizedErrors = append(result.CategorizedErrors, contracts.RadioRunError{
		Category:   category,
		Detail:     detail,
		EpisodeRef: episodeRef,
	})
}

// fetchLookbackFloorDays is the DEFAULT incremental-fetch lookback floor (days);
// resolveFetchLookbackFloorDays applies the RADIO_FETCH_LOOKBACK_FLOOR_DAYS override.
// The floor stops the forward-advancing last_playlist_fetch_at watermark (the fetch loop
// runs every 6h) from overtaking a show that airs less often than that cadence — without
// it such a show slips behind `since` and is skipped on every later run, permanently
// (PSY-1230). It applies to the per-show watermark each show is fetched against (PSY-1272).
// The cold-start (first-fetch) window uses the same floor; see fetchSince.
//
// 45 days covers the longest REGULAR cadence on the roster with margin. The roster is
// NTS-monthly-dominant (~92% monthly, dominant 28-day interval); the stage roster's
// max median inter-episode gap over shows with a real cadence signal is ~31d, none
// > 45d (PSY-1241 measured, PSY-1268 re-confirmed). A wider floor mostly just
// re-lists already-complete episodes, which importEpisode dedups on
// (show_id, external_id) — a cheap no-op; the paging cost is negligible (a monthly
// show is one NTS page regardless of the floor, and only the handful of daily shows
// page one or two deeper). Env-tunable so the value can be widened without a deploy
// if the roster's cadence tail grows (PSY-1268).
const fetchLookbackFloorDays = 45

// fetchSince computes the lower bound (`since`) for an incremental playlist fetch. It is
// called PER SHOW with that show's own last_playlist_fetch_at watermark (PSY-1272).
// floorDays (from resolveFetchLookbackFloorDays) is the load-bearing fix: it stops the
// forward-advancing watermark from overtaking a show that airs less often than the 6h
// fetch cadence (PSY-1230). The cold-start branch (no prior last_playlist_fetch_at) uses
// the SAME floor — a first fetch must never look back less than a subsequent one, or a
// monthly show's most recent episode could be missed before the floor takes over; deep
// initial population (history older than the floor) is the backfill path's job
// (ImportStation / discover create-on-first), not this incremental one.
//
// `since` is interpreted PER PROVIDER: WFMU compares it against an air_date
// parsed at UTC midnight (radio_provider_wfmu.go), while NTS/KEXP compare it
// against a broadcast instant. Normalizing to UTC midnight therefore gives WFMU
// an exact N-day floor and, for all three, keeps the bound independent of the
// server's wall-clock zone; for NTS/KEXP it merely rounds the lower bound down
// to midnight (harmless — it only widens the window by <1d).
//
// Re-scanning the wider window is cheap: an already-COMPLETE episode hits a
// dedup no-op (importEpisode keys on (show_id, external_id)); only a
// recently-aired, still-pending episode re-fetches its playlist, bounded by
// RadioBackfillMaxAttempts. A genuinely older lastFetch — a re-enabled station,
// a multi-day provider outage during which shouldAdvanceLastFetch held the
// timestamp back (PSY-1241), or a single show held back by its OWN watermark
// while its siblings advanced (PSY-1272) — widens the window further so recovery
// re-scans back to the true gap.
func fetchSince(lastFetch *time.Time, now time.Time, floorDays int) time.Time {
	today := now.UTC().Truncate(24 * time.Hour)
	floor := today.AddDate(0, 0, -floorDays)
	if lastFetch == nil {
		return floor
	}
	if lastFetch.Before(floor) {
		return *lastFetch
	}
	return floor
}

// maxFetchLookbackFloorDays caps the RADIO_FETCH_LOOKBACK_FLOOR_DAYS override. The
// floor only needs to cover the longest regular cadence + publish-lag margin (the
// roster tops out ~31d), so 365 is generous headroom. It also bounds the worst case
// for KEXP, whose FetchNewEpisodes pages the whole [since, now] window with no
// 422-style cap (unlike NTS) — so a fat-fingered huge floor would otherwise make it
// page its entire archive every cycle. (PSY-1268)
const maxFetchLookbackFloorDays = 365

// resolveFetchLookbackFloorDays returns the incremental-fetch lookback floor in
// days. RADIO_FETCH_LOOKBACK_FLOOR_DAYS overrides the fetchLookbackFloorDays default
// so ops can widen the window without a deploy if the roster's regular-cadence tail
// exceeds the measured default (PSY-1268). It is read here (per fetch) rather than
// resolved once in NewRadioFetchService like the loop-interval knobs, so the floor
// logic stays self-contained in this file and an override takes effect on the next
// cycle; the resolved value is echoed once in the service startup log. An
// out-of-range (<=0 or >maxFetchLookbackFloorDays) or unparseable override is
// ignored — a 0/negative floor would reintroduce the PSY-1230 permanent-skip bug and
// a huge one would make KEXP page its whole archive — and logged so a bad override
// doesn't silently fall back unnoticed.
func resolveFetchLookbackFloorDays() int {
	envVal := os.Getenv("RADIO_FETCH_LOOKBACK_FLOOR_DAYS")
	if envVal == "" {
		return fetchLookbackFloorDays
	}
	if days, err := strconv.Atoi(envVal); err == nil && days > 0 && days <= maxFetchLookbackFloorDays {
		return days
	}
	slog.Default().Warn("radio fetch: ignoring out-of-range RADIO_FETCH_LOOKBACK_FLOOR_DAYS; using default",
		"value", envVal, "default", fetchLookbackFloorDays, "max", maxFetchLookbackFloorDays)
	return fetchLookbackFloorDays
}

// shouldAdvanceLastFetch reports whether an incremental fetch run earned a
// last_playlist_fetch_at bump. The timestamp is a "durably persisted up to here"
// watermark, so a run advances it only when it had nothing to do or made real
// progress; a run that tried and wholly failed must hold it stale so the next good
// run re-scans the true gap via fetchSince's catch-up branch (lastFetch < floor).
// Without this gate the timestamp advanced to ~now on every failed run, pinning
// fetchSince at the floor and skipping forever any episode that aired during the
// failure but is older than the floor by the time it recovers.
//
//   - fetchAttempts == 0  → no fetchable shows; nothing to catch up on → advance.
//   - fetchSuccesses == 0 → every show's provider fetch errored (a total-station
//     provider/network outage) → hold.
//   - episodesReturned > 0 && episodesImported == 0 → providers responded but every
//     episode failed to persist (e.g. a DB write outage during the import loop) →
//     hold. (A run that fetched and found nothing new — episodesReturned == 0 — is
//     not a failure and advances.)
//   - otherwise → advance.
//
// Scope: this gate is granularity-agnostic — it takes counts, not a station, so the same
// rule applies at two granularities. PSY-1241 applies it to the per-station roll-up
// (advance unless EVERY fetchable show failed — the sustained-outage signal the PSY-1269
// janitor reads). PSY-1272 ALSO applies it per show (attempts=successes=1, with that
// show's own returned/imported counts) so a single persistently-failing show — e.g. a
// renamed/removed external_id that 404s until an admin corrects it — holds only its OWN
// watermark and recovers its gap via fetchSince's catch-up branch once it succeeds again,
// without the rest of the station stalling. (PSY-1241/PSY-1272)
func shouldAdvanceLastFetch(fetchAttempts, fetchSuccesses, episodesReturned, episodesImported int) bool {
	switch {
	case fetchAttempts == 0:
		return true
	case fetchSuccesses == 0:
		return false
	case episodesReturned > 0 && episodesImported == 0:
		return false
	default:
		return true
	}
}

// advanceLastFetch advances a STATION's last_playlist_fetch_at — the total-station
// roll-up watermark. It advances whenever the station as a whole made progress (≥1 show
// fetched OK, or there was nothing fetchable) and holds stale only on a total-station
// outage (every fetchable show failed), so the PSY-1269 janitor (EscalateStaleFetchOutages)
// can escalate a sustained one. The per-SHOW incremental frontier is advanced separately
// by advanceShowLastFetch (PSY-1272). The two steady-state station-fetch paths
// (FetchNewEpisodes, ImportStation) route through here. Scoped backfill imports
// (importShowEpisodesWithProgress, roster create-on-first) deliberately do NOT advance
// either watermark — they import a bounded historical window and must not move the
// incremental frontier. (PSY-1241/PSY-1269)
func (s *RadioService) advanceLastFetch(stationID uint, fetchAttempts, fetchSuccesses, episodesReturned, episodesImported int) {
	s.advanceFetchWatermark(&catalogm.RadioStation{}, stationID,
		fetchAttempts, fetchSuccesses, episodesReturned, episodesImported)
}

// advanceShowLastFetch advances a SHOW's last_playlist_fetch_at — the per-show incremental
// frontier (PSY-1272). Same gate as the station roll-up, scoped to one show. The two ways a
// show holds its watermark stale (so fetchSince's catch-up branch re-scans its true gap next
// run): (1) its provider fetch errored — held simply by the caller NOT calling this (it skips
// to the next show); (2) it fetched OK but every returned episode's ROW write failed
// (importEpisode errored — a DB/infra issue). NOTE: a per-episode playlist FetchError does
// NOT hold — the episode row IS created (so it counts as imported), and the post-air backfill
// sweep re-fetches the playlist (PSY-1119/PSY-1154); the watermark tracks "episodes listed up
// to here", not "playlists complete". This is what recovers a single persistently-failing
// show, e.g. a renamed/removed external_id, once it is corrected — without the rest of the
// station ever stalling for it. The sole caller passes fetchAttempts=fetchSuccesses=1 (the
// fetch-error case already continued), so the gate's attempts==0 / successes==0 branches are
// unreachable for a show — only the episodesReturned>0 && episodesImported==0 hold can fire.
func (s *RadioService) advanceShowLastFetch(showID uint, fetchAttempts, fetchSuccesses, episodesReturned, episodesImported int) {
	s.advanceFetchWatermark(&catalogm.RadioShow{}, showID,
		fetchAttempts, fetchSuccesses, episodesReturned, episodesImported)
}

// advanceFetchWatermark is the shared gated update behind advanceLastFetch (station) and
// advanceShowLastFetch (show): it stamps the entity's last_playlist_fetch_at to now ONLY
// when the run made progress (shouldAdvanceLastFetch). It uses UpdateColumn so advancing
// this operational watermark does NOT bump updated_at — that column is the
// content-modification timestamp surfaced in the API, and a 6h fetch cycle must not make
// every station/show look freshly edited. A no-progress run and a failed write are both
// logged, not swallowed — the no-progress warning is the immediate per-cycle outage
// signal. The log label is derived from the model (radioWatermarkEntity) so it can never
// desync from the table actually written. model is a zero-valued GORM model whose table
// the update targets.
func (s *RadioService) advanceFetchWatermark(model interface{}, id uint, fetchAttempts, fetchSuccesses, episodesReturned, episodesImported int) {
	entity := radioWatermarkEntity(model)
	if !shouldAdvanceLastFetch(fetchAttempts, fetchSuccesses, episodesReturned, episodesImported) {
		slog.Default().Warn("radio fetch: run made no progress; holding last_playlist_fetch_at stale",
			entity+"_id", id,
			"fetch_attempts", fetchAttempts, "fetch_successes", fetchSuccesses,
			"episodes_returned", episodesReturned, "episodes_imported", episodesImported)
		return
	}
	if err := s.db.Model(model).Where("id = ?", id).
		UpdateColumn("last_playlist_fetch_at", time.Now()).Error; err != nil {
		slog.Default().Error("radio fetch: failed to advance last_playlist_fetch_at",
			entity+"_id", id, "error", err)
	}
}

// bumpShowFetchFailureStreak increments a show's consecutive-fetch-failure counter —
// the PSY-1274 per-show sustained-outage signal. Bumped ONLY by scheduled incremental
// runs (fetchNewEpisodes gates on the trigger); manual admin fetches and scoped
// backfill/manual imports never inflate it, mirroring how they leave the watermarks
// alone. UpdateColumn keeps this operational counter from bumping updated_at (same
// rationale as advanceFetchWatermark). A failed write is logged, not swallowed —
// losing an increment only delays escalation by one cycle, never corrupts the streak
// semantics.
func (s *RadioService) bumpShowFetchFailureStreak(showID uint) {
	if err := s.db.Model(&catalogm.RadioShow{}).Where("id = ?", showID).
		UpdateColumn("consecutive_fetch_failures", gorm.Expr("consecutive_fetch_failures + 1")).Error; err != nil {
		slog.Default().Error("radio fetch: failed to bump consecutive_fetch_failures",
			"show_id", showID, "error", err)
	}
}

// resetShowFetchFailureStreak zeroes a show's consecutive-fetch-failure counter on a
// successful fetch (PSY-1274). A fetch that succeeds but returns zero episodes IS a
// success — this reset is what keeps the streak cadence-independent for infrequent
// shows. The `<> 0` guard skips the write in the (overwhelmingly common) already-zero
// case so the steady-state fetch cycle costs no extra row writes.
func (s *RadioService) resetShowFetchFailureStreak(showID uint) {
	if err := s.db.Model(&catalogm.RadioShow{}).
		Where("id = ? AND consecutive_fetch_failures <> 0", showID).
		UpdateColumn("consecutive_fetch_failures", 0).Error; err != nil {
		slog.Default().Error("radio fetch: failed to reset consecutive_fetch_failures",
			"show_id", showID, "error", err)
	}
}

// radioWatermarkEntity returns the log label ("station"/"show") for a watermark model so
// advanceFetchWatermark's log key is derived from the table it writes — never desynced from
// a hand-passed string. An unrecognized model falls back to a generic label rather than
// panicking; the only callers are the two advance* wrappers.
func radioWatermarkEntity(model interface{}) string {
	switch model.(type) {
	case *catalogm.RadioStation:
		return "station"
	case *catalogm.RadioShow:
		return "show"
	default:
		return "radio_entity"
	}
}

// FetchNewEpisodes does an incremental fetch of every active show on the station, each
// since its OWN last_playlist_fetch_at watermark (PSY-1272), floored so infrequent shows
// aren't skipped (PSY-1230; see fetchSince). Each show advances its own watermark on
// success; the station watermark is the total-station roll-up (the PSY-1269 outage signal).
// This exported wrapper runs with SCHEDULED-run semantics (failure streaks are
// maintained); executeSyncMode passes the run's real trigger to fetchNewEpisodes so a
// manual admin retry can never inflate a streak (PSY-1274).
func (s *RadioService) FetchNewEpisodes(stationID uint) (*contracts.RadioImportResult, error) {
	return s.fetchNewEpisodes(stationID, catalogm.RadioSyncRunTriggerScheduled, nil)
}

// fetchNewEpisodes is FetchNewEpisodes with the sync run's trigger made explicit
// and an optional single-show scope.
//
// The trigger gates ONLY the failure-streak bump: the streak means "consecutive
// SCHEDULED cycles failed" (that is what makes threshold × fetch interval a
// wall-clock outage duration), so an admin manually re-running a flapping station
// three times in ten minutes must not count as three cycles. A successful fetch
// resets the streak regardless of trigger — a manual verification run after fixing
// a show's external_id should clear the alert condition immediately.
//
// onlyShowID, when non-nil, narrows the loop to that one show — the PSY-1333
// slot-fetch path (a targeted fetch right after a show's scheduled start/end).
// A SCOPED run never bumps the failure streak either, even when scheduled:
// slot fetches would add extra attempts per day and silently tighten the
// PSY-1274 "N consecutive cycles ≈ N × fetch interval" escalation clock. The
// reset stays unconditional — any successful fetch clears the alert condition.
// A scoped start-boundary fetch often finds ZERO episodes (playlist not yet
// published) and still advances the show watermark ("progress" per
// shouldAdvanceLastFetch) — safe because fetchSince floors the window in days,
// so nothing published later inside the slot can slip past the frontier.
func (s *RadioService) fetchNewEpisodes(stationID uint, trigger string, onlyShowID *uint) (*contracts.RadioImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station catalogm.RadioStation
	if err := s.db.First(&station, stationID).Error; err != nil {
		return nil, fmt.Errorf("station not found: %w", err)
	}

	if station.PlaylistSource == nil || *station.PlaylistSource == "" {
		return nil, fmt.Errorf("station %d has no playlist source configured", stationID)
	}

	provider, err := s.getProvider(*station.PlaylistSource)
	if err != nil {
		return nil, err
	}
	defer closeProvider(provider)

	now := time.Now()
	floorDays := resolveFetchLookbackFloorDays()

	result := &contracts.RadioImportResult{}

	// Get active shows for this station
	var shows []catalogm.RadioShow
	if err := s.db.Where("station_id = ? AND is_active = ?", stationID, true).Find(&shows).Error; err != nil {
		return nil, fmt.Errorf("loading shows: %w", err)
	}

	// Per-run STATION roll-up counts: they drive the station watermark, which is the
	// PSY-1269 sustained-outage signal (it holds stale only when EVERY fetchable show
	// failed). Each show ALSO carries its own watermark, advanced independently below, so
	// one persistently-failing show holds only its own frontier (PSY-1272). A show with
	// no external id can't be fetched and is not counted as an attempt.
	var stationFetchAttempts, stationFetchSuccesses, stationEpisodesReturned int
	for _, show := range shows {
		if onlyShowID != nil && show.ID != *onlyShowID {
			continue
		}
		if show.ExternalID == nil || *show.ExternalID == "" {
			continue
		}

		// `since` is PER SHOW (PSY-1272): a show held stale by its own prior failures
		// widens its own window via fetchSince's catch-up branch, while its healthy
		// siblings stay at the floor.
		since := fetchSince(show.LastPlaylistFetchAt, now, floorDays)

		stationFetchAttempts++
		episodes, err := provider.FetchNewEpisodes(*show.ExternalID, since, time.Time{})
		if err != nil {
			// Fetch error → leave THIS show's watermark unadvanced (held stale) so the
			// next good run re-scans its true gap; the failure is captured in the run
			// errors. The station roll-up still advances if a sibling succeeds. The
			// failure streak feeds the janitor's per-show escalation (PSY-1274).
			recordImportError(result, categorizeRunError(err),
				fmt.Sprintf("fetch episodes for show %s: %v", show.Name, err), nil)
			if trigger == catalogm.RadioSyncRunTriggerScheduled && onlyShowID == nil {
				s.bumpShowFetchFailureStreak(show.ID)
			}
			continue
		}
		stationFetchSuccesses++
		stationEpisodesReturned += len(episodes)
		s.resetShowFetchFailureStreak(show.ID)

		showEpisodesImported := 0
		for _, ep := range episodes {
			epResult, err := s.importEpisode(show.ID, ep, provider)
			if err != nil {
				ref := ep.ExternalID
				recordImportError(result, categorizeRunError(err),
					fmt.Sprintf("import episode %s: %v", ep.ExternalID, err), &ref)
				continue
			}
			accumulateEpisodeResult(result, ep.ExternalID, epResult)
			showEpisodesImported++
		}

		// The successful fetch's listing doubles as the retraction signal
		// (PSY-1286): for an exhaustive-listing provider (WFMU), a stored
		// placeholder row inside this window that the listing no longer
		// carries was deleted upstream — remove it. No-op for other providers.
		// Safe even on an all-imports-failed cycle: rows in `episodes` are
		// excluded by external_id regardless of whether their import wrote.
		// A reconcile error is a non-fatal run error, never an import failure.
		retracted, rErr := s.reconcileRetractedEpisodes(show.ID, provider, episodes, since, now)
		if rErr != nil {
			recordImportError(result, categorizeRunError(rErr),
				fmt.Sprintf("reconcile retracted episodes for show %s: %v", show.Name, rErr), nil)
		}
		result.EpisodesRetracted += retracted

		// Advance THIS show's watermark only when its own fetch made progress (it
		// fetched OK and didn't return episodes that all failed to import). attempts and
		// successes are both 1 here because the fetch-error path above already continued.
		s.advanceShowLastFetch(show.ID, 1, 1, len(episodes), showEpisodesImported)
	}

	// Advance the STATION roll-up watermark only when the run made progress. It holds
	// stale only on a total-station outage (every fetchable show failed) so the next good
	// run re-scans and the janitor escalates a sustained one (PSY-1241/PSY-1269);
	// per-show recovery is the per-show watermarks above (PSY-1272). last_playlist_fetch_at
	// is a "last successful progress" watermark, NOT "last attempt" — attempt history
	// lives in radio_sync_runs. A SCOPED run advances it only off a real attempt: its
	// "nothing fetchable" case (the target show deactivated between scheduling and
	// running) says nothing about station health, unlike the sweep's genuinely-empty
	// station, so it must not refresh the outage signal.
	if onlyShowID == nil || stationFetchAttempts > 0 {
		s.advanceLastFetch(stationID, stationFetchAttempts, stationFetchSuccesses, stationEpisodesReturned, result.EpisodesImported)
	}

	return result, nil
}

// ImportEpisodePlaylist fetches and imports a single episode's playlist by external ID.
func (s *RadioService) ImportEpisodePlaylist(showID uint, episodeExternalID string) (*contracts.EpisodeImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Look up show and station to get the provider
	var show catalogm.RadioShow
	if err := s.db.Preload("Station").First(&show, showID).Error; err != nil {
		return nil, fmt.Errorf("show not found: %w", err)
	}

	if show.Station.PlaylistSource == nil || *show.Station.PlaylistSource == "" {
		return nil, fmt.Errorf("station has no playlist source configured")
	}

	provider, err := s.getProvider(*show.Station.PlaylistSource)
	if err != nil {
		return nil, err
	}
	defer closeProvider(provider)

	// Find the episode by external_id
	var episode catalogm.RadioEpisode
	err = s.db.Where("show_id = ? AND external_id = ?", showID, episodeExternalID).First(&episode).Error
	if err != nil {
		return nil, fmt.Errorf("episode not found: %w", err)
	}

	// Fetch and import the playlist
	plays, err := provider.FetchPlaylist(episodeExternalID)
	if err != nil {
		return nil, fmt.Errorf("fetching playlist: %w", err)
	}

	drops, err := s.importPlays(episode.ID, plays)
	if err != nil {
		return nil, fmt.Errorf("importing plays: %w", err)
	}

	// Run matching
	matcher := NewRadioMatchingEngine(s.db)
	matchResult, err := matcher.MatchPlaysForEpisode(episode.ID)
	if err != nil {
		return episodeResultFromDrops(drops), nil
	}

	res := episodeResultFromDrops(drops)
	res.PlaysMatched = matchResult.Matched
	res.MatchPersistErrors = matchResult.PersistErrors
	return res, nil
}

// MatchPlays runs the matching engine on unmatched plays for an episode.
func (s *RadioService) MatchPlays(episodeID uint) (*contracts.MatchResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	matcher := NewRadioMatchingEngine(s.db)
	return matcher.MatchPlaysForEpisode(episodeID)
}

// DiscoverStationShows discovers all shows for a station without importing episodes.
func (s *RadioService) DiscoverStationShows(stationID uint) (*contracts.RadioDiscoverResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station catalogm.RadioStation
	if err := s.db.First(&station, stationID).Error; err != nil {
		return nil, fmt.Errorf("station not found: %w", err)
	}

	if station.PlaylistSource == nil || *station.PlaylistSource == "" {
		return nil, fmt.Errorf("station %d has no playlist source configured", stationID)
	}

	provider, err := s.getProvider(*station.PlaylistSource)
	if err != nil {
		return nil, err
	}
	defer closeProvider(provider)

	result := &contracts.RadioDiscoverResult{}

	// Station-scoped for multi-stream providers (PSY-1073): each WFMU-family
	// station only receives the shows that air on its own stream.
	importedShows, err := discoverShowsForStation(provider, &station)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("discover shows: %v", err))
		return result, nil
	}

	for _, importShow := range importedShows {
		result.ShowsDiscovered++
		result.ShowNames = append(result.ShowNames, importShow.Name)

		// PSY-1153 create-on-first-episode: only update a show that ALREADY exists;
		// never create a row here. A roster show with no row yet is returned as a
		// candidate (NewRosterShows); the caller's create-on-first step
		// (createOnFirstForRoster, same discover run) creates it only once its first
		// episode is ingested — an episode-less roster DJ never becomes a row.
		_, found, err := s.findAndUpdateExistingShow(stationID, importShow)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("discover show %s: %v", importShow.Name, err))
			continue
		}
		if !found {
			result.ShowsNew++
			result.NewShowNames = append(result.NewShowNames, importShow.Name)
			result.NewRosterShows = append(result.NewRosterShows, contracts.RadioRosterShow{
				ExternalID:  importShow.ExternalID,
				Name:        importShow.Name,
				HostName:    importShow.HostName,
				Description: importShow.Description,
				ImageURL:    importShow.ImageURL,
				ArchiveURL:  importShow.ArchiveURL,
			})
		}
	}

	return result, nil
}

// createOnFirstForRoster implements PSY-1153 create-on-first-episode for the
// newly-discovered roster shows of a discover run. For each roster show it fetches the
// show's episodes and — only if ≥1 aired in [since, until] — creates the radio_shows
// row AND imports those episodes, so a row never exists empty and an episode-less
// roster DJ never becomes a row (§9 decision 1). It runs INSIDE the calling discover
// run (executeSyncMode), i.e. under that run's per-station advisory lock + breaker gate,
// so both the scheduled discover cycle and the manual admin "discover" trigger
// materialize aired shows through the same path. The loop is cancel-aware
// (isSyncRunCancelled) so a shutdown/cancel unwinds within ~one show.
//
// Returns the merged import result and the names of shows actually CREATED (for the
// discover cycle's notification). One provider instance is opened for the whole loop.
func (s *RadioService) createOnFirstForRoster(stationID, runID uint, roster []contracts.RadioRosterShow, since, until time.Time) (*contracts.RadioImportResult, []string) {
	result := &contracts.RadioImportResult{}
	var createdNames []string
	if len(roster) == 0 {
		return result, createdNames
	}

	var station catalogm.RadioStation
	if err := s.db.First(&station, stationID).Error; err != nil {
		recordImportError(result, catalogm.RadioSyncRunErrorValidationDrop, fmt.Sprintf("load station %d: %v", stationID, err), nil)
		return result, createdNames
	}
	if station.PlaylistSource == nil || *station.PlaylistSource == "" {
		return result, createdNames // nothing to fetch; not an error
	}
	provider, err := s.getProvider(*station.PlaylistSource)
	if err != nil {
		recordImportError(result, categorizeRunError(err), fmt.Sprintf("get provider: %v", err), nil)
		return result, createdNames
	}
	defer closeProvider(provider)

	for _, rs := range roster {
		// Stop within ~one show on a mid-run cancel / shutdown (the discover cycle's
		// watcher flips the run to cancelled on stopCh — same mechanism as backfill).
		if s.isSyncRunCancelled(runID) {
			break
		}
		if s.importRosterShowEpisodes(stationID, rs, since, until, provider, result) {
			createdNames = append(createdNames, rs.Name)
		}
	}
	return result, createdNames
}

// importRosterShowEpisodes fetches one roster show's episodes once, and if ≥1 aired in
// [since, until], creates the row (create-on-first) and imports those episodes,
// accumulating per-episode outcomes into result. Returns true iff a row was created.
// A single fetch serves both the create decision and the import (no double fetch).
func (s *RadioService) importRosterShowEpisodes(stationID uint, roster contracts.RadioRosterShow, since, until time.Time, provider RadioPlaylistProvider, result *contracts.RadioImportResult) (created bool) {
	episodes, err := provider.FetchNewEpisodes(roster.ExternalID, since, until)
	if err != nil {
		ref := roster.ExternalID
		recordImportError(result, categorizeRunError(err), fmt.Sprintf("fetch episodes for roster show %s: %v", roster.ExternalID, err), &ref)
		return false
	}

	var filtered []RadioEpisodeImport
	for _, ep := range episodes {
		if episodeInWindow(ep, since, until) {
			filtered = append(filtered, ep)
		}
	}
	if len(filtered) == 0 {
		return false // no episode in window → do not create a row (§9 dec 1)
	}

	showID, wasCreated, err := s.upsertRadioShow(stationID, rosterToImport(roster))
	if err != nil {
		ref := roster.ExternalID
		recordImportError(result, categorizeRunError(err), fmt.Sprintf("create roster show %s: %v", roster.ExternalID, err), &ref)
		return false
	}

	for _, ep := range filtered {
		epResult, impErr := s.importEpisode(showID, ep, provider)
		if impErr != nil {
			ref := ep.ExternalID
			recordImportError(result, categorizeRunError(impErr), fmt.Sprintf("import episode %s: %v", ep.ExternalID, impErr), &ref)
			continue
		}
		accumulateEpisodeResult(result, ep.ExternalID, epResult)
	}
	// Return wasCreated (not a bare true): on a TOCTOU race where the row appeared
	// between discovery's findAndUpdateExistingShow miss and this upsert, episodes
	// still import but the show is NOT re-reported as created (createdNames stays correct).
	return wasCreated
}

// episodeInWindow reports whether a provider episode's air_date falls within the
// inclusive [since, until] bound. Shared by the create-on-first path and
// importShowEpisodesWithProgress so the two never diverge on the boundary semantics
// (providers filter coarsely; this is the precise bound).
func episodeInWindow(ep RadioEpisodeImport, since, until time.Time) bool {
	epDate, err := time.Parse("2006-01-02", ep.AirDate)
	if err != nil {
		return false
	}
	return !epDate.Before(since) && !epDate.After(until)
}

// rosterToImport maps the contracts-layer roster carrier back to the catalog import shape.
func rosterToImport(r contracts.RadioRosterShow) RadioShowImport {
	return RadioShowImport{
		ExternalID:  r.ExternalID,
		Name:        r.Name,
		HostName:    r.HostName,
		Description: r.Description,
		ImageURL:    r.ImageURL,
		ArchiveURL:  r.ArchiveURL,
	}
}

// importProgressCallback is called periodically during episode import to report
// cumulative progress. Returning cancel=true stops the import early.
type importProgressCallback func(episodesImported, playsImported, playsMatched int, currentDate string, errors []string) (cancel bool)

// importShowEpisodesWithProgress is the shared implementation for importing
// episodes of a single show within a date range. It handles date parsing,
// provider setup, episode fetching/filtering, and per-episode import.
//
// If progressFn is non-nil it is called after every episode with cumulative
// stats; a true return value stops the import early. The episodesFound callback
// (if non-nil) is called once after filtering with the total episode count.
func (s *RadioService) importShowEpisodesWithProgress(
	showID uint,
	since, until string,
	episodesFoundFn func(int),
	progressFn importProgressCallback,
) (*contracts.RadioImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	sinceTime, err := parseImportDate(since)
	if err != nil {
		return nil, fmt.Errorf("invalid since date %q: %w", since, err)
	}
	untilTime, err := parseImportDate(until)
	if err != nil {
		return nil, fmt.Errorf("invalid until date %q: %w", until, err)
	}

	var show catalogm.RadioShow
	if err := s.db.Preload("Station").First(&show, showID).Error; err != nil {
		return nil, fmt.Errorf("show not found: %w", err)
	}

	if show.Station.PlaylistSource == nil || *show.Station.PlaylistSource == "" {
		return nil, fmt.Errorf("station has no playlist source configured")
	}

	provider, err := s.getProvider(*show.Station.PlaylistSource)
	if err != nil {
		return nil, err
	}
	defer closeProvider(provider)

	if show.ExternalID == nil || *show.ExternalID == "" {
		return nil, fmt.Errorf("show %d has no external ID", showID)
	}

	episodes, err := provider.FetchNewEpisodes(*show.ExternalID, sinceTime, untilTime)
	if err != nil {
		return nil, fmt.Errorf("fetching episodes: %w", err)
	}

	// Filter episodes by air_date within [since, until] (inclusive both ends).
	// Providers filter coarsely; episodeInWindow is the shared precise bound.
	var filtered []RadioEpisodeImport
	for _, ep := range episodes {
		if episodeInWindow(ep, sinceTime, untilTime) {
			filtered = append(filtered, ep)
		}
	}

	if episodesFoundFn != nil {
		episodesFoundFn(len(filtered))
	}

	result := &contracts.RadioImportResult{}

	for _, ep := range filtered {
		epResult, importErr := s.importEpisode(show.ID, ep, provider)
		if importErr != nil {
			ref := ep.ExternalID
			recordImportError(result, categorizeRunError(importErr),
				fmt.Sprintf("import episode %s: %v", ep.ExternalID, importErr), &ref)
		} else {
			accumulateEpisodeResult(result, ep.ExternalID, epResult)
		}

		if progressFn != nil {
			if cancel := progressFn(
				result.EpisodesImported,
				result.PlaysImported,
				result.PlaysMatched,
				ep.AirDate,
				result.Errors,
			); cancel {
				return result, nil
			}
		}
	}

	return result, nil
}

// BackfillCandidate is one show with aired episodes still missing a complete
// playlist, plus the air-date window spanning those episodes (PSY-1154). The
// post-air backfill ticker runs RunStationSync(backfill) per candidate over
// [Since, Until], which re-lists the show's episodes in that window and re-fetches
// the playlists of the incomplete-aired ones (the complete ones are dedup-skipped).
type BackfillCandidate struct {
	StationID uint
	ShowID    uint
	Since     time.Time
	Until     time.Time
}

// ListBackfillCandidates finds shows whose aired episodes still need a post-air
// playlist re-fetch (PSY-1154): playlist_state pending/partial, attempts left, aired,
// and aired within `lookback` of `now`. The lookback bounds the candidate set (and,
// at rollout, the one-time re-fetch of recently-aired episodes) and matches the
// reality that providers only keep recent episodes listable for re-fetch — older
// stragglers are the janitor's job (PSY-1155).
//
// A coarse SQL filter (state / attempts / air_date) narrows the scan; the precise
// aired check is the shared ShouldBackfillPlaylist predicate applied per row, so the
// sweep selection and the in-flight re-fetch decision (importEpisode) can never
// diverge. Results are grouped by show — one [min, max] air-date window each — and
// ordered by show id for deterministic processing.
func (s *RadioService) ListBackfillCandidates(lookback time.Duration, maxAttempts int, now time.Time) ([]BackfillCandidate, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	cutoff := now.Add(-lookback).Format("2006-01-02")

	type candidateRow struct {
		ShowID                uint
		StationID             uint
		AirDate               string
		StartsAt              *time.Time
		EndsAt                *time.Time
		PlaylistState         string
		PlaylistFetchAttempts int
		PlayCount             int
	}
	var rows []candidateRow
	err := s.db.Model(&catalogm.RadioEpisode{}).
		Select("radio_episodes.show_id, radio_shows.station_id, radio_episodes.air_date, "+
			"radio_episodes.starts_at, radio_episodes.ends_at, radio_episodes.playlist_state, "+
			"radio_episodes.playlist_fetch_attempts, radio_episodes.play_count").
		Joins("JOIN radio_shows ON radio_shows.id = radio_episodes.show_id").
		Where(`(
			(radio_episodes.playlist_state IN ? AND radio_episodes.playlist_fetch_attempts < ?)
			OR (radio_episodes.playlist_state = ? AND radio_episodes.starts_at IS NULL AND radio_episodes.play_count = 0)
		)`, []string{catalogm.RadioPlaylistStatePending, catalogm.RadioPlaylistStatePartial}, maxAttempts, catalogm.RadioPlaylistStateUnavailable).
		Where("radio_episodes.air_date >= ?", cutoff).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing backfill candidates: %w", err)
	}

	byShow := make(map[uint]*BackfillCandidate)
	for _, r := range rows {
		state, attempts := r.PlaylistState, r.PlaylistFetchAttempts
		if normState, normAttempts := catalogm.NormalizeStrandedWindowlessPlaylistState(
			r.StartsAt, r.PlaylistState, r.PlaylistFetchAttempts, r.PlayCount, now,
		); normState != state || normAttempts != attempts {
			state, attempts = normState, normAttempts
		}
		if !catalogm.ShouldBackfillPlaylist(r.StartsAt, r.EndsAt, state, attempts, maxAttempts, now) {
			continue
		}
		d, perr := parseImportDate(r.AirDate)
		if perr != nil {
			continue // unparseable air_date can't bound a window; it won't be re-listed anyway
		}
		c, ok := byShow[r.ShowID]
		if !ok {
			byShow[r.ShowID] = &BackfillCandidate{StationID: r.StationID, ShowID: r.ShowID, Since: d, Until: d}
			continue
		}
		if d.Before(c.Since) {
			c.Since = d
		}
		if d.After(c.Until) {
			c.Until = d
		}
	}

	candidates := make([]BackfillCandidate, 0, len(byShow))
	for _, c := range byShow {
		candidates = append(candidates, *c)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ShowID < candidates[j].ShowID })
	return candidates, nil
}

// =============================================================================
// Internal import helpers
// =============================================================================

// upsertRadioShow creates or updates a radio show from import data.
// Returns the internal show ID.
//
// Matching priority:
//  1. (station_id, external_id) — canonical match
//  2. (station_id, slug) — fallback for seeded shows whose external_id
//     may have been incorrect; also updates external_id to the correct value
//  3. Create new show
//
// When a show already exists, only fields that are currently empty/NULL in
// the database are populated from the import data. This preserves
// admin-curated or migration-seeded values.
// upsertRadioShow returns (showID, created, error). created is true ONLY when
// a brand-new row was inserted; both the match-by-external-id and the
// match-by-slug fallback paths return false because they update an existing
// row. Callers use the bool to distinguish "new arrival" from "idempotent
// re-run" — e.g. to fire a notification only on actually-new shows.
func (s *RadioService) upsertRadioShow(stationID uint, importShow RadioShowImport) (uint, bool, error) {
	if id, found, err := s.findAndUpdateExistingShow(stationID, importShow); err != nil {
		return 0, false, err
	} else if found {
		return id, false, nil
	}

	// PSY-1349: family-wide stickiness. WFMU show↔station ownership is scraped live
	// from volatile roster pages each discover run, and unknown/ambiguous codes
	// default to the flagship — so ownership FLAPS across runs, and every flap used
	// to mint a twin row (same external_id under two family stations) that then
	// accrued its own episode history forever. If the code already has a row under
	// ANY WFMU-family station, reuse that row instead of creating a second one; the
	// established home is the strong claim, this run's ownership resolution the weak
	// one. This pairing is STEADY-STATE, not transitional: a map-absent code homed on
	// a substream keeps being fed by the flagship's discover run indefinitely (under
	// the flagship run's lock/breaker/stats) — the per-reuse Info log below is the
	// operational breadcrumb. Re-homing a show that GENUINELY moved streams is
	// deliberately left to cmd/dedup-radio-shows (full-history view).
	//
	// The lookup+create runs under a family-wide pg advisory xact lock: the sync
	// layer's advisory locks are per-station, so two family stations' runs (e.g. the
	// scheduled discover cycle racing a manual admin trigger) could otherwise both
	// miss the lookup and both create — the (station_id, external_id) unique index
	// does not span stations, so nothing at the DB level stops the second insert.
	// Family creates are rare (new shows only), so one global lock is uncontended.
	isFamily, err := s.isWFMUFamilyStation(stationID)
	if err != nil {
		return 0, false, err
	}
	if !isFamily {
		return s.createRadioShow(s.db, stationID, importShow)
	}

	var id uint
	var created bool
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext('psy_wfmu_family_show_create'))").Error; err != nil {
			return fmt.Errorf("acquiring WFMU family create lock: %w", err)
		}
		if eid, found, err := s.findAndUpdateWFMUFamilyShow(tx, stationID, importShow); err != nil {
			return err
		} else if found {
			id, created = eid, false
			return nil
		}
		nid, created2, err := s.createRadioShow(tx, stationID, importShow)
		if err != nil {
			return err
		}
		id, created = nid, created2
		return nil
	})
	if txErr != nil {
		return 0, false, txErr
	}
	return id, created, nil
}

// createRadioShow inserts a new show row (the create half of upsertRadioShow),
// generating a unique slug against db (which may be a transaction).
func (s *RadioService) createRadioShow(db *gorm.DB, stationID uint, importShow RadioShowImport) (uint, bool, error) {
	baseSlug := utils.GenerateArtistSlug(importShow.Name)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		db.Model(&catalogm.RadioShow{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	// Lifecycle at creation follows the station's semantics (PSY-1348). On a
	// recency station a row exists only because its first episode is being
	// ingested (PSY-1153), so it IS active (the model/DB default). On a
	// schedule-authoritative station "active" means "on the current grid", and a
	// just-created show has no schedule yet — most such creations are fill-ins,
	// which must not enter the active lineup even for a day. The cost is a known
	// transient for a show genuinely ADDED to the grid mid-season: it stays
	// dormant until the next weekly scrape stamps its schedule and the following
	// janitor run promotes it (disclosed on PSY-1348).
	lifecycle := catalogm.RadioLifecycleActive
	authoritative, err := s.stationIsScheduleAuthoritative(stationID)
	if err != nil {
		return 0, false, fmt.Errorf("checking station schedule authority: %w", err)
	}
	if authoritative {
		lifecycle = catalogm.RadioLifecycleDormant
	}

	show := &catalogm.RadioShow{
		StationID:      stationID,
		Name:           importShow.Name,
		Slug:           slug,
		HostName:       importShow.HostName,
		Description:    importShow.Description,
		ImageURL:       importShow.ImageURL,
		ArchiveURL:     importShow.ArchiveURL,
		ExternalID:     &importShow.ExternalID,
		LifecycleState: lifecycle,
	}

	if err := db.Create(show).Error; err != nil {
		return 0, false, fmt.Errorf("creating show: %w", err)
	}

	return show.ID, true, nil
}

// isWFMUFamilyStation reports whether the station is one of the WFMU-family stations
// (WFMUFamilySlugs — pinned in sync with the provider's wfmuStationChannels by
// radio_provider_wfmu_scoped_test.go, so a fifth stream can't silently escape).
func (s *RadioService) isWFMUFamilyStation(stationID uint) (bool, error) {
	var station catalogm.RadioStation
	if err := s.db.Select("slug").First(&station, stationID).Error; err != nil {
		return false, fmt.Errorf("loading station for family twin check: %w", err)
	}
	return slices.Contains(WFMUFamilySlugs, station.Slug), nil
}

// findAndUpdateWFMUFamilyShow looks up a show by external_id across ALL WFMU-family
// stations and, on a hit, fills null-safe metadata onto it (same write behavior as
// findAndUpdateExistingShow — the name discloses the mutation). Caller guarantees
// stationID is a family station and holds the family create lock. A hit means
// episodes import onto the existing sibling-station row and NO new row is created.
// Non-family stations (KEXP, NTS) never share external_id namespaces, so
// upsertRadioShow skips this entirely for them.
func (s *RadioService) findAndUpdateWFMUFamilyShow(db *gorm.DB, stationID uint, importShow RadioShowImport) (uint, bool, error) {
	if importShow.ExternalID == "" {
		return 0, false, nil // code-less rows can't be family-matched
	}

	// Oldest row wins when (pathologically) more than one already exists — the
	// pre-existing twins are cmd/dedup-radio-shows' job, not this guard's.
	var existing catalogm.RadioShow
	err := db.
		Joins("JOIN radio_stations ON radio_stations.id = radio_shows.station_id").
		Where("radio_stations.slug IN ?", WFMUFamilySlugs).
		Where("radio_shows.external_id = ?", importShow.ExternalID).
		Order("radio_shows.id").
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("checking WFMU family for existing show: %w", err)
	}

	updates := s.buildNullSafeShowUpdates(&existing, importShow)
	if len(updates) > 0 {
		if uErr := db.Model(&existing).Updates(updates).Error; uErr != nil {
			slog.Warn("radio import: family stickiness metadata fill failed", "show_id", existing.ID, "error", uErr)
		}
	}
	slog.Info("radio import: WFMU family stickiness — reusing sibling-station row instead of creating a twin",
		"code", importShow.ExternalID, "show_id", existing.ID,
		"home_station_id", existing.StationID, "requested_station_id", stationID)
	return existing.ID, true, nil
}

// findAndUpdateExistingShow looks up a roster show by (station_id, external_id) then
// (station_id, slug), filling null-safe fields if found. Returns (showID, found, err).
// It NEVER creates a row — discovery uses it so an episode-less roster show is not
// persisted (PSY-1153 create-on-first-episode); upsertRadioShow uses it as the
// find-half before creating.
func (s *RadioService) findAndUpdateExistingShow(stationID uint, importShow RadioShowImport) (uint, bool, error) {
	// Try matching by external_id first (canonical path).
	var existing catalogm.RadioShow
	err := s.db.Where("station_id = ? AND external_id = ?", stationID, importShow.ExternalID).First(&existing).Error
	if err == nil {
		// Only fill in fields that are currently empty — never overwrite curated data.
		updates := s.buildNullSafeShowUpdates(&existing, importShow)
		if len(updates) > 0 {
			s.db.Model(&existing).Updates(updates)
		}
		return existing.ID, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, fmt.Errorf("checking existing show by external_id: %w", err)
	}

	// Fallback: match by slug within the same station. This handles seeded shows that
	// had incorrect external_ids — the slug derived from the name still matches, so we
	// adopt the API's external_id instead of creating a duplicate.
	baseSlug := utils.GenerateArtistSlug(importShow.Name)
	err = s.db.Where("station_id = ? AND slug = ?", stationID, baseSlug).First(&existing).Error
	if err == nil {
		updates := s.buildNullSafeShowUpdates(&existing, importShow)
		updates["external_id"] = importShow.ExternalID
		s.db.Model(&existing).Updates(updates)
		return existing.ID, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, fmt.Errorf("checking existing show by slug: %w", err)
	}

	return 0, false, nil
}

// buildNullSafeShowUpdates returns a map of fields to update, only including
// fields that are currently empty/NULL in the existing record. This preserves
// admin-curated or migration-seeded values.
func (s *RadioService) buildNullSafeShowUpdates(existing *catalogm.RadioShow, importShow RadioShowImport) map[string]interface{} {
	updates := map[string]interface{}{}

	if existing.Name == "" && importShow.Name != "" {
		updates["name"] = importShow.Name
	}
	if existing.HostName == nil && importShow.HostName != nil {
		updates["host_name"] = *importShow.HostName
	}
	if existing.Description == nil && importShow.Description != nil {
		updates["description"] = *importShow.Description
	}
	if existing.ImageURL == nil && importShow.ImageURL != nil {
		updates["image_url"] = *importShow.ImageURL
	}
	if existing.ArchiveURL == nil && importShow.ArchiveURL != nil {
		updates["archive_url"] = *importShow.ArchiveURL
	}
	return updates
}

// scheduleDerivedWindowMaxAgeDays bounds how far back a WFMU episode's air window
// may be DERIVED from the show's CURRENT schedule. The stored schedule reflects
// the current season; an episode older than this likely aired under a different
// (since-churned) schedule, so deriving its window from today's slots would
// mis-window it — the frozen-window rule (a historical event's time is never
// recomputed against current config; see pattern_radio_show_lifecycle_churn /
// PSY-1152). It comfortably exceeds the incremental fetch re-list window (so
// recently-aired episodes always get a window) while staying well under WFMU's
// seasonal schedule churn. Beyond it an episode is long-aired anyway, so a nil
// window is correct (ComputeEpisodeStatus settles it to aired/archived). (PSY-1238)
const scheduleDerivedWindowMaxAgeDays = 30

// futureAirDateToleranceDays bounds how far ahead of the UTC date an episode's
// air_date may be at import (PSY-1350). air_date is date-only and station zones
// can run ahead of UTC, so "today" in the station zone can be UTC-tomorrow;
// 2 days accepts any legitimate same-day/next-day listing while catching the
// real failure (an upstream typo months out, which would pin its show 'active'
// under the recency janitor and float atop latest-first feeds).
const futureAirDateToleranceDays = 2

// errFutureAirDate / errInvalidAirDate mark episodes rejected by the air-date
// guard in importEpisode. Typed so categorizeRunError buckets them as
// validation_drop (structural, per the PSY-1141 convention) instead of
// provider_unreachable.
var (
	errFutureAirDate  = errors.New("future-dated episode rejected")
	errInvalidAirDate = errors.New("unparseable air_date rejected")
)

// episodeAirWindow resolves the frozen [starts_at, ends_at] for an episode. A
// provider-supplied window wins (KEXP gives start+end, NTS a start) — those
// providers already carry the broadcast's own instants, so the result is never
// re-derived. A WINDOWLESS episode (today, only WFMU — it carries a date but no
// air time) has its window derived from the show's stored weekly schedule
// (PSY-1159) + the episode's air_date, in the schedule's timezone (PSY-1238),
// but ONLY when the air_date is recent enough that the current schedule still
// applies (scheduleDerivedWindowMaxAgeDays — the churn guard). Returns (nil, nil)
// when there is no provider window AND (the air_date is too old OR no schedule
// slot matches) — ComputeEpisodeStatus then settles the episode to aired/archived
// (never falsely live), the conservative windowless fallback.
//
// The schedule read happens only on this path (creating a windowless episode, or
// healing a still-windowless one within the guard), never for an already-windowed
// re-list — a self-draining, per-windowless-episode read dwarfed by that
// episode's own playlist fetch. Any load/parse error degrades to a nil window
// (logged) rather than failing the import. NOTE: this sets only starts_at/ends_at
// (the authoritative air window); air_time (a separate provider-supplied display
// string) stays as the provider left it — nil for WFMU, unchanged by this path.
func (s *RadioService) episodeAirWindow(showID uint, ep RadioEpisodeImport, now time.Time) (startsAt, endsAt *time.Time) {
	if ep.StartsAt != nil {
		return ep.StartsAt, ep.EndsAt
	}
	if ep.AirDate == "" {
		return nil, nil
	}
	// Churn FLOOR: only derive a window for an episode aired within the guard (see
	// the const). Both sides are compared as UTC-midnight dates (matching
	// fetchSince's convention) so the cutoff is reproducible regardless of server
	// zone. A malformed air_date is out-of-window (nil). This is a floor, not a
	// band: a future air_date (a WFMU "upcoming broadcast" placeholder) passes and
	// yields a correctly-'scheduled' window — the current schedule applies to
	// upcoming airings, and it self-corrects to aired once it airs. Whether to
	// import such placeholders at all is PSY-1240's concern, not this derivation's.
	airDay, err := time.Parse("2006-01-02", ep.AirDate)
	cutoff := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -scheduleDerivedWindowMaxAgeDays)
	if err != nil || airDay.Before(cutoff) {
		return nil, nil
	}
	var show catalogm.RadioShow
	if err := s.db.Select("schedule").First(&show, showID).Error; err != nil {
		slog.Warn("radio import: loading show schedule for air window failed", "show_id", showID, "error", err)
		return nil, nil
	}
	sched, err := catalogm.ParseRadioSchedule(show.Schedule)
	if err != nil {
		slog.Warn("radio import: parsing show schedule for air window failed", "show_id", showID, "error", err)
		return nil, nil
	}
	if sched == nil {
		return nil, nil // no structured schedule (e.g. a WFMU show not yet scraped)
	}
	startsAt, endsAt, err = sched.WindowForDate(ep.AirDate)
	if err != nil {
		slog.Warn("radio import: computing air window failed", "show_id", showID, "air_date", ep.AirDate, "error", err)
		return nil, nil
	}
	return startsAt, endsAt
}

// importEpisode imports a single episode and its playlist. A brand-new episode is
// created and its playlist fetched. An episode that already exists heals a missing
// air window (PSY-1152/PSY-1238) and, if it has aired with an incomplete playlist,
// runs a post-air backfill re-fetch (PSY-1154) — otherwise it is a dedup skip.
func (s *RadioService) importEpisode(showID uint, ep RadioEpisodeImport, provider RadioPlaylistProvider) (*contracts.EpisodeImportResult, error) {
	now := time.Now()

	// Check for existing episode (dedup by show_id + external_id)
	var existing catalogm.RadioEpisode
	err := s.db.Where("show_id = ? AND external_id = ?", showID, ep.ExternalID).First(&existing).Error
	if err == nil {
		return s.reimportExistingEpisode(&existing, ep, provider, now)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("checking existing episode: %w", err)
	}

	// air_date is NOT NULL; an episode with no recoverable date (no broadcast
	// and no parseable alias date) can't be stored. Skip it rather than fail the
	// whole import batch on a date NOT NULL violation — same posture as the
	// dedup skip above.
	if ep.AirDate == "" {
		return &contracts.EpisodeImportResult{}, nil
	}

	// PSY-1350: reject an air_date implausibly far in the future — an upstream
	// typo (observed: a WFMU playlist dated a year out) would otherwise pin its
	// show 'active' under the recency janitor for months and sort atop
	// latest-first feeds. Provider-agnostic backstop: WFMU is already bounded to
	// station-local today at fetch (PSY-1240), and KEXP/NTS list aired episodes
	// only — so nothing legitimate is near the bound. The tolerance exists
	// because air_date is date-only and station zones run ahead of UTC; the
	// error is a typed sentinel so the sync-run feed records it as a
	// validation_drop (visible, not silently clamped — upstream fixes then
	// import normally on a later fetch). Scope: NEW episodes only — a
	// future-dated row persisted before this guard shipped re-enters via the
	// dedup path above and is out of reach (the one known instance, stage
	// episode 4917, was deleted by hand; evidence on PSY-1350). A non-empty
	// air_date that fails the canonical YYYY-MM-DD parse is ALSO rejected —
	// the DATE column would happily accept formats Go's strict parse does not
	// ("2027-6-5", "06/05/2027"), which would smuggle a future date past the
	// bound; every provider normalizes to YYYY-MM-DD, so nothing legitimate
	// trips this.
	airDay, parseErr := time.Parse("2006-01-02", ep.AirDate)
	if parseErr != nil {
		return nil, fmt.Errorf("%w: %q", errInvalidAirDate, ep.AirDate)
	}
	if airDay.After(now.UTC().Truncate(24 * time.Hour).AddDate(0, 0, futureAirDateToleranceDays)) {
		return nil, fmt.Errorf("%w: air_date %s is beyond now+%dd", errFutureAirDate, ep.AirDate, futureAirDateToleranceDays)
	}

	// Create episode. StartsAt/EndsAt are the frozen air window (PSY-1152) — the
	// provider's instants (KEXP/NTS) or, for WFMU, derived once here from the
	// show's schedule + air_date (PSY-1238) — and never re-derived from a later
	// schedule (the frozen-window rule). Status is a best-effort snapshot computed
	// at ingest; the authoritative live/aired state is recomputed on read (a
	// stored "live" would go stale), and the column is kept fresh by the janitor
	// (PSY-1155). The playlist starts pending; the fetch below settles it
	// (PSY-1154), which is what can promote status to 'archived'.
	startsAt, endsAt := s.episodeAirWindow(showID, ep, now)
	episode := &catalogm.RadioEpisode{
		ShowID:          showID,
		Title:           ep.Title,
		AirDate:         ep.AirDate,
		AirTime:         ep.AirTime,
		DurationMinutes: ep.DurationMinutes,
		ArchiveURL:      ep.ArchiveURL,
		ExternalID:      &ep.ExternalID,
		StartsAt:        startsAt,
		EndsAt:          endsAt,
		Status: catalogm.ComputeEpisodeStatus(
			startsAt, endsAt, catalogm.RadioPlaylistStatePending, now,
		),
	}

	if err := s.db.Create(episode).Error; err != nil {
		return nil, fmt.Errorf("creating episode: %w", err)
	}

	// PSY-1153 real-time reactivation: a new episode for a known-but-dormant show
	// (a DJ returning from a leave of absence) flips it back to 'active' immediately,
	// rather than waiting for the nightly janitor (PSY-1155). The scheduled fetch keeps
	// polling dormant shows (is_active is left true), so this is the path that catches
	// a return between janitor runs. No-op if the show is already active (or retired —
	// retired is manual-only and intentionally NOT auto-reactivated).
	s.reactivateShowIfDormant(showID, now)

	return s.fetchImportAndRecordPlaylist(episode, ep.ExternalID, provider, now)
}

// reactivateShowIfDormant flips a show 'dormant' → 'active' when a new episode lands
// (PSY-1153). The WHERE guard makes it a no-op for active shows (and never touches
// 'retired', the manual-only state). Best-effort: a failure here doesn't fail the
// import (the janitor reconcile, PSY-1155, will correct lifecycle_state on its next run).
//
// PSY-1348: on a schedule-authoritative station (see scheduleAuthoritativeStations)
// active means "on the current grid", not "aired recently" — so a fill-in's new
// episode must NOT promote it (otherwise this path and the nightly janitor would
// fight, flapping the show active↔dormant every day). The guard mirrors the
// janitor's grid rule leg for leg: promote if on the scrape-maintained grid, if
// code-less (NULL/empty external_id — the grid can't speak for rows the scrape can't
// match, same exemption as the grid demote), or if on a recency-semantics station.
// schedule_locked shows on authoritative stations keep the admin-set lifecycle.
func (s *RadioService) reactivateShowIfDormant(showID uint, now time.Time) {
	if err := s.db.Model(&catalogm.RadioShow{}).
		Where("id = ? AND lifecycle_state = ?", showID, catalogm.RadioLifecycleDormant).
		Where("("+radioShowOnGridSQL+") OR "+radioShowCodelessSQL+" OR station_id NOT IN (?)",
			s.scheduleAuthoritativeStations()).
		Updates(map[string]any{
			"lifecycle_state": catalogm.RadioLifecycleActive,
			"updated_at":      now,
		}).Error; err != nil {
		slog.Warn("radio import: reactivating dormant show failed", "show_id", showID, "error", err)
	}
}

// reimportExistingEpisode handles importEpisode's already-exists path. It first
// heals a missing frozen air window (PSY-1152): rows imported before window
// stamping have a NULL window and would never show "live", so a show airing
// across the deploy would lose its ON AIR strip until re-ingest. The window comes
// from the provider's instants (KEXP/NTS) or, for WFMU, the show's schedule +
// air_date (PSY-1238) — so a windowless WFMU episode that gets re-listed inside
// the fetch window self-heals. It then runs a post-air playlist backfill
// (PSY-1154) iff the episode has aired with an incomplete playlist and has
// attempts left — a complete, exhausted (unavailable), still-live, or scheduled
// episode is left untouched (dedup skip), so a routine re-list never re-fetches a
// playlist that is already final or legitimately still in progress.
func (s *RadioService) reimportExistingEpisode(existing *catalogm.RadioEpisode, ep RadioEpisodeImport, provider RadioPlaylistProvider, now time.Time) (*contracts.EpisodeImportResult, error) {
	// Heal a missing frozen window AND enforce the PSY-1285 scheduled-never-unavailable
	// invariant in a SINGLE update: both can change `status`, so the fields are collected
	// here and status is computed once from the final window + state — no redundant second
	// write on the heal+reset path.
	updates := map[string]any{}
	hadWindow := existing.StartsAt != nil

	if existing.StartsAt == nil {
		// Resolve a window (provider instants, or schedule-derived for a recent
		// WFMU episode). When none can be resolved (no schedule yet, off-schedule,
		// or past the churn guard) the episode stays windowless and this is a
		// no-op — its status is left to recordPlaylistOutcome / the janitor, the
		// same as before window stamping (the prior `ep.StartsAt != nil` guard
		// likewise never fired for a windowless WFMU row).
		if startsAt, endsAt := s.episodeAirWindow(existing.ShowID, ep, now); startsAt != nil {
			existing.StartsAt = startsAt
			existing.EndsAt = endsAt
			updates["starts_at"] = startsAt
			updates["ends_at"] = endsAt
		}
	}

	// PSY-1285: a windowless episode is settled to 'aired', so the backfill burns attempts
	// on it → 'unavailable'; once it is given a FUTURE window (the heal above, or PSY-1283's
	// schedule correction) it is 'scheduled' but its playlist_state would otherwise stay
	// stuck 'unavailable'. Reset it so the post-air backfill can run once it actually airs —
	// whether the window was just healed or was already present (so it also clears rows
	// stranded before this fix). A no-op for aired/live episodes.
	if newState, newAttempts := catalogm.NormalizeScheduledPlaylistState(
		existing.StartsAt, existing.EndsAt, existing.PlaylistState, existing.PlaylistFetchAttempts, now,
	); newState != existing.PlaylistState || newAttempts != existing.PlaylistFetchAttempts {
		existing.PlaylistState = newState
		existing.PlaylistFetchAttempts = newAttempts
		updates["playlist_state"] = newState
		updates["playlist_fetch_attempts"] = newAttempts
	}

	// PSY-1287: once a windowless false give-up gets a real air window (schedule heal),
	// clear the exhausted playlist state so post-air backfill can run at the right phase.
	if newState, newAttempts := catalogm.NormalizeWindowHealPlaylistState(
		hadWindow, existing.StartsAt, existing.PlaylistState, existing.PlaylistFetchAttempts, existing.PlayCount,
	); newState != existing.PlaylistState || newAttempts != existing.PlaylistFetchAttempts {
		existing.PlaylistState = newState
		existing.PlaylistFetchAttempts = newAttempts
		updates["playlist_state"] = newState
		updates["playlist_fetch_attempts"] = newAttempts
	}

	if len(updates) > 0 {
		newStatus := catalogm.ComputeEpisodeStatus(existing.StartsAt, existing.EndsAt, existing.PlaylistState, now)
		updates["status"] = newStatus
		existing.Status = newStatus
		if err := s.db.Model(existing).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("healing/normalizing episode on re-list: %w", err)
		}
	}

	if !catalogm.ShouldBackfillPlaylist(existing.StartsAt, existing.EndsAt, existing.PlaylistState,
		existing.PlaylistFetchAttempts, catalogm.RadioBackfillMaxAttempts, now) {
		return &contracts.EpisodeImportResult{}, nil
	}

	return s.fetchImportAndRecordPlaylist(existing, ep.ExternalID, provider, now)
}

// fetchImportAndRecordPlaylist fetches an episode's provider playlist, imports its
// plays (idempotent — P3's per-play dedup makes re-fetch safe), runs matching, and
// records the playlist-completeness outcome (playlist_state, playlist_fetched_at,
// playlist_fetch_attempts, recomputed status + play_count). Shared by the
// first-import path and the post-air backfill re-fetch path (PSY-1154).
//
// A FetchPlaylist failure is non-fatal to the batch (the episode row is kept) but is
// NOT silently swallowed (PSY-1119): the episode has no new plays purely because the
// fetch failed. Providers signal a legitimately-empty playlist with (nil, nil) — e.g.
// KEXP returns that for a 404 / no-start-time episode — which leaves FetchError empty;
// only a non-nil error sets FetchError. Either way, a post-air attempt that yields no
// playlist is recorded so the backfill loop can eventually give up (PSY-1154) instead
// of retrying a permanently-missing playlist forever.
func (s *RadioService) fetchImportAndRecordPlaylist(episode *catalogm.RadioEpisode, externalID string, provider RadioPlaylistProvider, now time.Time) (*contracts.EpisodeImportResult, error) {
	plays, err := provider.FetchPlaylist(externalID)
	if err != nil {
		if recErr := s.recordPlaylistOutcome(episode, 0, true, now); recErr != nil {
			slog.Error("radio import: recording failed playlist outcome", "episode_id", episode.ID, "error", recErr)
		}
		slog.Error("radio import: playlist fetch failed; episode kept with no new plays",
			"episode_id", episode.ID,
			"external_id", externalID,
			"error", err)
		// Categorize the FetchError here, where the provider error's type is still
		// live (the same classifier the top-level path uses) — PSY-1141.
		return &contracts.EpisodeImportResult{FetchError: err.Error(), FetchErrorCategory: categorizeRunError(err)}, nil
	}

	drops, err := s.importPlays(episode.ID, plays)
	if err != nil {
		// Hard infra error persisting plays — NOT a "playlist unavailable" signal.
		// Leave playlist_state unchanged (still eligible) and don't burn an attempt;
		// a transient persist failure should be retried, not counted toward giving up.
		return episodeResultFromDrops(drops), nil
	}

	// Settle the playlist (state/attempts/status/play_count) from the fetched playlist.
	if err := s.recordPlaylistOutcome(episode, drops.Imported, false, now); err != nil {
		slog.Error("radio import: recording playlist outcome", "episode_id", episode.ID, "error", err)
	}

	// Run matching
	matcher := NewRadioMatchingEngine(s.db)
	matchResult, err := matcher.MatchPlaysForEpisode(episode.ID)
	if err != nil {
		return episodeResultFromDrops(drops), nil
	}

	res := episodeResultFromDrops(drops)
	res.PlaysMatched = matchResult.Matched
	res.MatchPersistErrors = matchResult.PersistErrors
	return res, nil
}

// recordPlaylistOutcome applies the PSY-1154 completeness policy to an episode after
// one playlist fetch attempt: it derives the new playlist_state + attempt count
// (ComputePlaylistState), recomputes the episode status from the frozen window
// (ComputeEpisodeStatus — so a now-complete playlist promotes the episode to
// 'archived'), stamps playlist_fetched_at, and persists them in a single update. The
// in-memory episode is kept in sync for the caller.
//
// play_count is maintained MONOTONICALLY — max(current, fetched) — and only on a
// fetch that returned plays. radio_plays is append-only (importPlays does ON CONFLICT
// DO NOTHING, never deletes), so the row count never legitimately shrinks. A naive
// `play_count = playsImported` would corrupt the denormalized count when a re-fetch of
// an existing `partial` episode returns fewer plays than are already stored — e.g. a
// live KEXP snapshot of 5 tracks followed by a transient empty/short post-air re-fetch
// would zero/shrink play_count while the original rows persist, surfacing "0 plays" on
// an episode that has tracks. max() + the empty-fetch skip make the count never decrease.
func (s *RadioService) recordPlaylistOutcome(episode *catalogm.RadioEpisode, playsImported int, fetchFailed bool, now time.Time) error {
	phase := catalogm.ComputeEpisodeStatus(episode.StartsAt, episode.EndsAt, catalogm.RadioPlaylistStatePending, now)
	isAired := phase == catalogm.RadioEpisodeStatusAired
	newState, newAttempts := catalogm.ComputePlaylistState(
		isAired, playsImported > 0, fetchFailed, episode.PlaylistFetchAttempts, catalogm.RadioBackfillMaxAttempts)
	newStatus := catalogm.ComputeEpisodeStatus(episode.StartsAt, episode.EndsAt, newState, now)

	updates := map[string]any{
		"playlist_state":          newState,
		"playlist_fetched_at":     now,
		"playlist_fetch_attempts": newAttempts,
		"status":                  newStatus,
	}
	// Only advance play_count on a fetch that actually returned plays, and never
	// below the current value (a failed/empty/short re-fetch must not clobber it).
	newPlayCount := episode.PlayCount
	if !fetchFailed && playsImported > 0 {
		newPlayCount = max(episode.PlayCount, playsImported)
		updates["play_count"] = newPlayCount
	}
	if err := s.db.Model(episode).Updates(updates).Error; err != nil {
		return fmt.Errorf("recording playlist outcome for episode %d: %w", episode.ID, err)
	}

	episode.PlaylistState = newState
	episode.PlaylistFetchAttempts = newAttempts
	episode.Status = newStatus
	episode.PlaylistFetchedAt = &now
	episode.PlayCount = newPlayCount
	return nil
}

// episodeResultFromDrops builds the per-episode result carrying the play tally plus
// the structured drop counts (truncation salvage + validation drop), shared by
// importEpisode's and ImportEpisodePlaylist's returns (PSY-1141).
func episodeResultFromDrops(d importPlaysOutcome) *contracts.EpisodeImportResult {
	return &contracts.EpisodeImportResult{
		PlaysImported:  d.Imported,
		DropSummary:    d.Summary,
		TruncatedPlays: d.Truncated,
		DroppedPlays:   d.Dropped,
	}
}

const (
	// playUpsertMaxAttempts caps the retry-on-conflict loop on the play upsert. A
	// transient conflict (deadlock / serialization failure) is rare here — the
	// per-station advisory lock (P2) serializes same-station runs and different
	// stations touch disjoint rows — so a low cap absorbs the transient case while
	// still surfacing a genuinely stuck transaction instead of looping on it.
	playUpsertMaxAttempts = 3
	// playUpsertRetryBackoff is the base inter-attempt delay; it scales linearly
	// with the attempt number (5ms, then 10ms). Plain linear backoff (not the
	// Full-Jitter that PSY-1142 uses for cross-station HTTP fetches) is sufficient
	// here because this upsert is single-process and advisory-lock-serialized per
	// station — there is no cross-station retry herd to de-synchronize.
	playUpsertRetryBackoff = 5 * time.Millisecond
)

// retryTransientConflict runs op, retrying up to playUpsertMaxAttempts times on a
// transient Postgres conflict — a deadlock (SQLSTATE 40P01) or a serialization
// failure (40001) — with a short linear backoff. Success or any other error
// returns immediately and unchanged, so the caller's "hard infrastructural error
// — bubble it up" contract is preserved; a persistent conflict surfaces after the
// final attempt rather than looping forever.
//
// This is mostly defense-in-depth. At the production READ COMMITTED isolation
// (db/connection.go), a plain INSERT … ON CONFLICT DO NOTHING does NOT raise
// 40001 (that needs REPEATABLE READ / SERIALIZABLE); a 40P01 deadlock can arise
// even at READ COMMITTED but is very unlikely here, because the per-station
// advisory lock serializes same-station writes and different stations insert
// disjoint rows. The guard exists so the upsert stays correct if it is ever moved
// to a higher isolation level (research §3 recommends the lock + retry pair).
// Retrying is safe because the upsert is idempotent: a conflict rolls the
// transaction back fully, so re-running the whole CreateInBatches re-inserts
// cleanly — and any row that did commit re-conflicts on the (episode_id,
// dedup_key) unique index and is skipped. Kept local to this package per
// PSY-1143 (promote to services/shared only if a second caller appears).
func retryTransientConflict(op func() error) error {
	var err error
	for attempt := 1; attempt <= playUpsertMaxAttempts; attempt++ {
		err = op()
		// Retry only the two canonical transient-conflict codes; everything else
		// (and success) returns immediately so a real error still bubbles up.
		if err == nil || (!shared.IsSerializationFailure(err) && !shared.IsDeadlock(err)) {
			return err
		}
		if attempt < playUpsertMaxAttempts {
			time.Sleep(playUpsertRetryBackoff * time.Duration(attempt))
		}
	}
	return err
}

// importPlays batch-creates play records for an episode.
//
// PSY-885: validate-at-the-boundary semantics. Each provider-returned play is
// passed through sanitizePlay BEFORE the batch insert so a single malformed
// row (NULL artist_name, over-length VARCHAR field) can no longer poison its
// 100-row CreateInBatches batch and silently drop all 99 sibling rows. CLAUDE.md:
// "defensive programming at boundaries, trust internally".
//
// Returns (rowsCommitted, summary, err). rowsCommitted is the count of rows
// actually written to the database — rejected rows are excluded; truncated
// rows ARE included (they were salvaged with shortened text). summary is a
// human-readable per-episode aggregate of "dropped N plays: ..." or "" when
// no intervention was needed; callers append it to RadioImportResult.Errors
// so the outcome is visible in admin job logs without per-row noise.
func (s *RadioService) importPlays(episodeID uint, plays []RadioPlayImport) (importPlaysOutcome, error) {
	if len(plays) == 0 {
		return importPlaysOutcome{}, nil
	}

	records := make([]catalogm.RadioPlay, 0, len(plays))
	truncatedRows := 0
	missingArtistRows := 0

	for _, p := range plays {
		record, err := sanitizePlay(episodeID, p)
		if err != nil {
			// Per-row diagnostic at WARN — surfaces the actual culprit in logs
			// while the per-episode summary stays compact.
			slog.Warn("radio import: dropping play row",
				"episode_id", episodeID,
				"position", p.Position,
				"reason", err.Error(),
			)
			if errors.Is(err, errMissingArtistName) {
				missingArtistRows++
			}
			continue
		}
		if playNeededTruncation(p) {
			truncatedRows++
		}
		records = append(records, record)
	}

	// droppedRows is computed from the actual delta so new sanitize-drop
	// reasons (added later without a matching per-class counter) still get
	// reflected in the N total — only the per-class breakdown will lag.
	droppedRows := len(plays) - len(records)
	summary := summarizeDrops(droppedRows, truncatedRows, missingArtistRows)
	drops := importPlaysOutcome{Summary: summary, Truncated: truncatedRows, Dropped: droppedRows}

	if len(records) == 0 {
		return drops, nil
	}

	// Batch insert with ON CONFLICT DO NOTHING so duplicate rows (re-imports
	// of the same playlist; chronic during dev / scheduled re-fetches) are
	// silently skipped rather than rolling back the entire 100-row batch.
	// Dedup is enforced by the idx_radio_plays_dedup UNIQUE index on
	// (episode_id, dedup_key) — a generated content hash over (position,
	// artist_name, track_title), PSY-1131. Records are pre-validated (PSY-885), so
	// a non-UNIQUE constraint violation here (FK gone, NOT NULL) is a hard
	// infrastructural error — bubble it up.
	// Wrap the upsert in retry-on-transient-conflict (deadlock 40P01 /
	// serialization 40001) as defense-in-depth. ON CONFLICT is not a blanket
	// atomic-under-concurrency guarantee (research §3). The per-station advisory
	// lock already serializes same-station runs, and at READ COMMITTED these
	// conflicts are unlikely on this upsert — so this mainly guards a future
	// higher-isolation move (see retryTransientConflict). It re-runs the
	// idempotent ON CONFLICT upsert a bounded number of times before surfacing a
	// stuck txn.
	var result *gorm.DB
	if err := retryTransientConflict(func() error {
		result = s.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(records, 100)
		return result.Error
	}); err != nil {
		return drops, fmt.Errorf("batch inserting plays: %w", err)
	}

	if skipped := len(records) - int(result.RowsAffected); skipped > 0 {
		slog.Info("radio import: skipped duplicate plays",
			"episode_id", episodeID,
			"skipped", skipped,
			"total", len(records),
		)
	}

	// Return len(records) (attempted) rather than RowsAffected (newly
	// inserted) so callers like importEpisode keep using it to set
	// play_count on the episode without regressing on re-imports where
	// most rows are duplicates. summary carries the PSY-885 drop aggregate.
	drops.Imported = len(records)
	return drops, nil
}

// importPlaysOutcome is importPlays' structured result: rows committed plus the
// per-class boundary-intervention counts, so callers categorize drops (truncation
// salvage vs validation drop) without re-parsing the human Summary string (PSY-1141).
type importPlaysOutcome struct {
	Imported  int
	Summary   string
	Truncated int // over-length rows salvaged → truncation category
	Dropped   int // rows rejected by sanitizePlay → validation_drop category
}

// errMissingArtistName flags a play with no artist_name. radio_plays.artist_name
// is NOT NULL in the schema — we can't make up an artist, so the row is dropped.
var errMissingArtistName = errors.New("missing artist_name")

// sanitizePlay validates and normalizes a provider-returned play DTO for safe
// insertion into radio_plays. It is the single boundary checkpoint where bad
// provider data is rejected (NULL artist_name) or salvaged (truncate
// over-length VARCHAR fields to the column's 500-rune width). All downstream
// code can then trust that any RadioPlay it sees is schema-valid.
//
// Returns the sanitized record on success, or an error explaining why the row
// must be dropped. Truncation does NOT produce an error — it's a non-fatal
// repair, surfaced via playNeededTruncation for the caller's per-episode
// summary counter.
func sanitizePlay(episodeID uint, p RadioPlayImport) (catalogm.RadioPlay, error) {
	// artist_name is NOT NULL in the schema. Trimmed empty / whitespace-only
	// names can't be salvaged — drop the row.
	if strings.TrimSpace(p.ArtistName) == "" {
		return catalogm.RadioPlay{}, errMissingArtistName
	}

	// Defensive boundary guard: an empty/whitespace provider_play_id would make
	// the generated dedup_key COALESCE to '' and collide every such play in the
	// episode. Normalize blank to nil so the content-hash branch applies instead.
	// KEXP already guards id <= 0; this protects against a future provider wiring
	// an empty id (trust internally — every downstream RadioPlay is then known to
	// carry either a non-empty provider id or nil).
	providerPlayID := p.ProviderPlayID
	if providerPlayID != nil && strings.TrimSpace(*providerPlayID) == "" {
		providerPlayID = nil
	}

	return catalogm.RadioPlay{
		EpisodeID:              episodeID,
		Position:               p.Position,
		ProviderPlayID:         providerPlayID,
		ArtistName:             truncateRunes(p.ArtistName, radioPlayVarcharMaxRunes),
		TrackTitle:             truncateOptionalRunes(p.TrackTitle, radioPlayVarcharMaxRunes),
		AlbumTitle:             truncateOptionalRunes(p.AlbumTitle, radioPlayVarcharMaxRunes),
		LabelName:              truncateOptionalRunes(p.LabelName, radioPlayVarcharMaxRunes),
		ReleaseYear:            p.ReleaseYear,
		IsNew:                  p.IsNew,
		RotationStatus:         catalogm.NormalizeRotationStatus(p.RotationStatus),
		DJComment:              p.DJComment, // TEXT column, no length cap
		IsLivePerformance:      p.IsLivePerformance,
		IsRequest:              p.IsRequest,
		MusicBrainzArtistID:    p.MusicBrainzArtistID,
		MusicBrainzRecordingID: p.MusicBrainzRecordingID,
		MusicBrainzReleaseID:   p.MusicBrainzReleaseID,
		AirTimestamp:           p.AirTimestamp,
	}, nil
}

// playNeededTruncation reports whether sanitizePlay had to shorten any of the
// four VARCHAR(500) text fields on p. Used by importPlays to count
// truncated-row count for the per-episode summary without re-running the
// sanitize logic.
func playNeededTruncation(p RadioPlayImport) bool {
	if utf8.RuneCountInString(p.ArtistName) > radioPlayVarcharMaxRunes {
		return true
	}
	if p.TrackTitle != nil && utf8.RuneCountInString(*p.TrackTitle) > radioPlayVarcharMaxRunes {
		return true
	}
	if p.AlbumTitle != nil && utf8.RuneCountInString(*p.AlbumTitle) > radioPlayVarcharMaxRunes {
		return true
	}
	if p.LabelName != nil && utf8.RuneCountInString(*p.LabelName) > radioPlayVarcharMaxRunes {
		return true
	}
	return false
}

// truncateRunes shortens s to at most max runes, respecting rune boundaries
// (no split multi-byte sequences). Returns s unchanged when within budget.
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

// truncateOptionalRunes is the *string variant of truncateRunes, preserving
// nil semantics for Huma-style optional pointers.
func truncateOptionalRunes(s *string, max int) *string {
	if s == nil {
		return nil
	}
	if utf8.RuneCountInString(*s) <= max {
		return s
	}
	truncated := truncateRunes(*s, max)
	return &truncated
}

// summarizeDrops formats a single-line per-episode aggregate of plays that
// required boundary intervention (PSY-885). Returns "" when nothing needed
// intervention. Format is stable for log scraping:
//
//	dropped N plays: X over-length titles truncated, Y missing artist_name
//
// N is droppedCount + truncatedCount — the total count of rows the sanitize
// step touched. "Dropped" reads loosely here: truncated rows are kept (just
// with shortened text), but grouping under one number gives admins a single
// "needed attention" magnitude. The per-class breakdown clauses distinguish
// salvage (truncated) from data loss (missing artist_name).
//
// droppedCount is taken as the authoritative drop count from the caller (not
// re-derived from missingArtistCount) so future sanitize-drop classes added
// without a matching counter still appear in N — the breakdown will lag the
// total in that case, surfacing the omission rather than hiding it.
func summarizeDrops(droppedCount, truncatedCount, missingArtistCount int) string {
	total := droppedCount + truncatedCount
	if total == 0 {
		return ""
	}
	var parts []string
	if truncatedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d over-length titles truncated", truncatedCount))
	}
	if missingArtistCount > 0 {
		parts = append(parts, fmt.Sprintf("%d missing artist_name", missingArtistCount))
	}
	return fmt.Sprintf("dropped %d plays: %s", total, strings.Join(parts, ", "))
}

// closeProvider closes a provider if it implements a Close method.
func closeProvider(provider RadioPlaylistProvider) {
	if closer, ok := provider.(interface{ Close() }); ok {
		closer.Close()
	}
}
