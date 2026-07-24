package catalog

// Airing-feed ingestion (PSY-1509): the KEXP/NTS analog of what the WFMU
// schedule scrape + slot fetch derive from stored weekly grids. KEXP and NTS
// publish no stable weekly schedule we could store (NTS programming is largely
// freeform one-offs), but both publish an AIRING feed — concrete broadcast
// instances with real start/end instants (KEXP /v2/shows + /v2/timeslots, NTS
// /v2/live). Each slot-fetch tick ingests those feeds and creates/heals the
// airing show's windowed episode row, so:
//
//   - the row exists within ~one slot-fetch interval of broadcast start
//     (instead of waiting out the 6h station sweep — the "Astral Plane live
//     with a week-old newest row" gap);
//   - the live-refresh work list (ShowsWithLiveIncompleteEpisodes, now keyed
//     on the live windowed episode itself) picks the show up the same tick and
//     grows its playlist through the existing scoped-fetch machinery;
//   - the live now-playing payload can deep-link the airing episode
//     (coveringLiveEpisode in radio_now_playing.go).
//
// Like the WFMU schedule scrape, this path writes NO radio_sync_runs rows and
// never touches station health, the breaker state, the volume-anomaly
// baseline, or failure streaks — all scoped-run neutrality invariants are
// preserved by construction (row creation is not a sync run; the playlist
// fetches it enables flow through RunStationSync's scoped-fetch path, which is
// already breaker/health/anomaly/streak-neutral and test-pinned). It does
// HONOR an open breaker (skip while blocked) so a station in outage isn't
// polled by yet another loop.
//
// Airing feeds never mint radio_shows: an airing that matches no existing
// active show (external id, then unambiguous exact name) is skipped — show
// creation stays with the discover cycle's create-on-first policy.

import (
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// AiringIngestResult summarizes one airing-ingestion pass for the cycle log.
type AiringIngestResult struct {
	StationsPolled int // stations whose airing feed was fetched successfully
	RowsCreated    int // new windowed episode rows created
	WindowsHealed  int // existing rows whose missing window (or end bound) was stamped
}

// IngestCurrentAirings polls every active automated station whose provider
// exposes an airing feed and creates/heals the currently-airing episode rows.
// Per-station failures are logged and skipped — one broken feed never starves
// the rest of the tick, and errors never propagate to the caller (the ticker).
func (s *RadioService) IngestCurrentAirings(now time.Time) AiringIngestResult {
	var res AiringIngestResult
	if s.db == nil {
		return res
	}
	stations, err := s.GetActiveStationsWithPlaylistSource()
	if err != nil {
		slog.Warn("radio airing ingest: listing stations failed", "error", err)
		return res
	}
	for i := range stations {
		station := &stations[i]
		if station.PlaylistSource == nil || *station.PlaylistSource == "" {
			continue
		}
		source := *station.PlaylistSource
		channel, ok := liveChannelForStation(source, station.Slug)
		if !ok {
			continue // no live routing for this station (mirrors the now-playing path)
		}
		// Breaker: skip ONLY while the breaker is open and inside its cooldown
		// (gateBlocked). Once past cooldown (gateTrial) the cheap feed GET is
		// allowed through on every tick — it is deliberately NOT the sweep's
		// half-open trial and never reads as one: this path writes no breaker
		// state, so an open breaker stays open (and this loop keeps polling)
		// until the sweep's real trial resolves it.
		if breakerGateFor(s.readBreakerSnapshot(station.ID), now) == gateBlocked {
			continue
		}
		provider, err := s.getProvider(source)
		if err != nil {
			continue // unsupported/manual source — getProvider already logged
		}
		lister, ok := provider.(RadioAiringLister)
		if !ok {
			closeProvider(provider)
			continue // provider has no airing feed (WFMU: the schedule scrape owns it)
		}
		airings, err := lister.FetchCurrentAirings(channel)
		if err != nil {
			closeProvider(provider)
			slog.Warn("radio airing ingest: fetching airing feed failed",
				"station_id", station.ID, "station_slug", station.Slug, "error", err)
			continue
		}
		closeProvider(provider)
		res.StationsPolled++
		for _, airing := range airings {
			created, healed := s.ingestAiring(station.ID, airing, now)
			if created {
				res.RowsCreated++
			}
			if healed {
				res.WindowsHealed++
			}
		}
	}
	return res
}

// ingestAiring upserts one airing onto its matched show: a missing row is
// created with the airing's frozen window (same validation + creation path as
// the listing import — createEpisodeFromImport); an existing row gets a
// missing window/end bound healed (healAiringWindow). An already-windowed row
// is a no-op — playlist growth belongs to the live-refresh scoped fetch, not
// this path (which would otherwise double-fetch every tick).
func (s *RadioService) ingestAiring(stationID uint, airing RadioAiring, now time.Time) (created, healed bool) {
	ep := airing.Episode
	if ep.ExternalID == "" || ep.StartsAt == nil {
		return false, false // not an ingestable airing (no identity / no window start)
	}
	show := s.matchAiringShow(stationID, airing.ShowExternalID, airing.ShowName)
	if show == nil {
		return false, false // unmatched airing — never create shows from airing feeds
	}

	var existing catalogm.RadioEpisode
	err := s.db.Where("show_id = ? AND external_id = ?", show.ID, ep.ExternalID).First(&existing).Error
	if err == nil {
		return false, s.healAiringWindow(&existing, ep, now)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Warn("radio airing ingest: episode lookup failed",
			"show_id", show.ID, "external_id", ep.ExternalID, "error", err)
		return false, false
	}

	episode, err := s.createEpisodeFromImport(show.ID, ep, now)
	if err != nil {
		slog.Warn("radio airing ingest: creating airing episode failed",
			"show_id", show.ID, "external_id", ep.ExternalID, "error", err)
		return false, false
	}
	return episode != nil, false
}

// matchAiringShow resolves an airing to the station's radio_shows row using the
// same tiers as the live now-playing match (matchStationShow): provider
// external id first (strongest), then unambiguous case-insensitive exact name.
// Only active shows qualify — the population the sweep itself fetches. Zero or
// multiple matches → nil (never guess, never create).
func (s *RadioService) matchAiringShow(stationID uint, externalID, name string) *catalogm.RadioShow {
	if externalID != "" {
		if show := s.findSingleShowRow("station_id = ? AND is_active = TRUE AND external_id = ?", stationID, externalID); show != nil {
			return show
		}
	}
	if name != "" {
		if show := s.findSingleShowRow("station_id = ? AND is_active = TRUE AND LOWER(name) = LOWER(?)", stationID, name); show != nil {
			// The name tier feeds a WRITE path (episode creation), unlike the
			// read-only now-playing match it mirrors — log it so a persistent
			// name collision misattributing airings is discoverable.
			slog.Info("radio airing ingest: airing matched by exact name (no external-id match)",
				"station_id", stationID, "provider_external_id", externalID,
				"show_id", show.ID, "show_name", name)
			return show
		}
	}
	return nil
}

// findSingleShowRow returns the show iff the condition matches exactly one row
// (the model-row sibling of findSingleShow, which returns a contract ref).
func (s *RadioService) findSingleShowRow(query string, args ...interface{}) *catalogm.RadioShow {
	var shows []catalogm.RadioShow
	if err := s.db.Where(query, args...).Limit(2).Find(&shows).Error; err != nil {
		slog.Warn("radio airing ingest: show match lookup failed", "error", err)
		return nil
	}
	if len(shows) != 1 {
		return nil
	}
	return &shows[0]
}

// healAiringWindow stamps a missing frozen window — or a missing END bound —
// onto an existing episode row from the airing feed's instants. The frozen-
// window rule holds: an already-complete window is never rewritten; the end
// bound is filled ONLY when the feed's start matches the stored start exactly
// (same broadcast, same instant — KEXP rows imported from the listing carry
// start_time but the listing publishes no end_time, so sweep-created rows for
// the current broadcast arrive end-less and would otherwise never read "live").
// When the healed row is now LIVE, a prematurely-settled playlist state is
// reopened (reopenLivePlaylistState) so the live refresh + final post-air fetch
// can run at the correct phase.
func (s *RadioService) healAiringWindow(existing *catalogm.RadioEpisode, ep RadioEpisodeImport, now time.Time) bool {
	updates := map[string]any{}
	switch {
	case existing.StartsAt == nil:
		existing.StartsAt = ep.StartsAt
		updates["starts_at"] = ep.StartsAt
		// Defense in depth: no current import path produces (nil starts_at,
		// non-nil ends_at), but never let a feed with no end bound null out a
		// previously-known one.
		if ep.EndsAt != nil {
			existing.EndsAt = ep.EndsAt
			updates["ends_at"] = ep.EndsAt
		}
	case existing.EndsAt == nil && ep.EndsAt != nil && ep.StartsAt.Equal(*existing.StartsAt):
		existing.EndsAt = ep.EndsAt
		updates["ends_at"] = ep.EndsAt
	default:
		return false // window already frozen (or feed disagrees on the start) — never rewrite
	}

	newStatus := catalogm.ComputeEpisodeStatus(existing.StartsAt, existing.EndsAt, existing.PlaylistState, now)
	if newState, newAttempts := reopenLivePlaylistState(newStatus, existing.PlaylistState, existing.PlaylistFetchAttempts, existing.PlayCount); newState != existing.PlaylistState || newAttempts != existing.PlaylistFetchAttempts {
		existing.PlaylistState = newState
		existing.PlaylistFetchAttempts = newAttempts
		updates["playlist_state"] = newState
		updates["playlist_fetch_attempts"] = newAttempts
		// Status depends on playlist state (complete → archived); recompute.
		newStatus = catalogm.ComputeEpisodeStatus(existing.StartsAt, existing.EndsAt, newState, now)
	}
	updates["status"] = newStatus
	existing.Status = newStatus

	if err := s.db.Model(existing).Updates(updates).Error; err != nil {
		slog.Warn("radio airing ingest: healing episode window failed",
			"episode_id", existing.ID, "error", err)
		return false
	}
	return true
}

// reopenLivePlaylistState clears a prematurely-settled playlist state on an
// episode that just became LIVE via a window heal. A sweep-created KEXP row is
// end-less → settles to 'aired' mid-broadcast → a backfill fetch can mark it
// 'complete' (or burn attempts to 'unavailable') while the show is still
// airing; once the heal reveals it is actually live, those settlements were
// made at the wrong phase:
//
//   - complete + plays → partial (still growing; the final post-air fetch
//     re-settles it to complete after ends_at);
//   - unavailable → pending, attempts reset (the "gave up" verdict was reached
//     pre-air/mid-air; the post-air backfill deserves its real attempts).
//
// pending/partial (the live-refresh-eligible states) pass through untouched,
// as does everything on a non-live episode. Pure — unit-tested without a DB.
func reopenLivePlaylistState(status, playlistState string, attempts, playCount int) (state string, newAttempts int) {
	if status != catalogm.RadioEpisodeStatusLive {
		return playlistState, attempts
	}
	switch playlistState {
	case catalogm.RadioPlaylistStateComplete:
		if playCount > 0 {
			return catalogm.RadioPlaylistStatePartial, attempts
		}
		return catalogm.RadioPlaylistStatePending, 0
	case catalogm.RadioPlaylistStateUnavailable:
		return catalogm.RadioPlaylistStatePending, 0
	default:
		return playlistState, attempts
	}
}
