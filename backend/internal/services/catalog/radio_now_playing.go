package catalog

// Station now-playing (PSY-1022): GET /radio-stations/{slug}/now-playing.
//
// The payload comes from the station's provider live API when one exists
// (KEXP plays/shows, NTS live, WFMU current-live-shows aggregator) and falls
// back to the "latest archive" heuristic otherwise — the most-active show's
// latest episode's latest play, mirroring the frontend v1 derivation
// (frontend/features/radio/lib/stationOverview.ts) that this endpoint
// supersedes. Provider failures NEVER surface as errors: they degrade to the
// archive fallback and log.
//
// Caching: a per-station in-process TTL cache (map + per-entry mutex) sits in
// front of the fetch, so page views never fan out to provider APIs — at most
// one provider round-trip per station per TTL window, and concurrent cold
// requests for the same station serialize on the entry mutex instead of
// duplicating the fetch. Chosen over the ticker-refresh pattern
// (services/shared.RunTickerLoop) because the dial has a handful of stations
// and on-demand fill avoids polling providers for stations nobody is viewing.

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

const (
	// nowPlayingCacheTTL bounds provider-API request rates per station. 90s
	// sits in the ticket's 60-120s window: fresh enough for "what's on right
	// now", and at most ~40 provider hits/station/hour even under constant
	// page traffic.
	nowPlayingCacheTTL = 90 * time.Second

	// nowPlayingRecentArtistLimit caps the "earlier:" artist hops, matching
	// the frontend v1 derivation (recentArtistsFromEpisode limit 4).
	nowPlayingRecentArtistLimit = 4
)

// ntsLiveChannelByStationSlug maps our NTS station slugs onto NTS live
// channel_name values. Our single nts-radio station represents the network's
// primary channel 1; an NTS-2 station would need its own entry (an unmapped
// NTS station falls back to latest-archive rather than showing channel 1's
// broadcast under the wrong banner).
var ntsLiveChannelByStationSlug = map[string]string{
	"nts-radio": "1",
}

// wfmuLiveChannelByStationSlug maps our WFMU station slugs (seeded in
// internal/seeddata/radio.go) onto WFMU stream keys. Stations added later
// without an entry here fall back to latest-archive — honest, and visible in
// the payload's source discriminator.
var wfmuLiveChannelByStationSlug = map[string]string{
	"wfmu":                wfmuLiveChannelMain,
	"wfmu-drummer":        wfmuLiveChannelDrummer,
	"wfmu-rocknsoulradio": wfmuLiveChannelRockSoul,
	"wfmu-sheena":         wfmuLiveChannelSheena,
}

// liveChannelForStation resolves the provider live-channel key for a station.
// ok=false means the station has no live routing (unknown source, or a
// multi-stream provider station we can't place) → archive fallback.
func liveChannelForStation(source, stationSlug string) (channel string, ok bool) {
	switch source {
	case catalogm.PlaylistSourceKEXP:
		return "", true // single stream; channel unused
	case catalogm.PlaylistSourceNTS:
		channel, ok = ntsLiveChannelByStationSlug[stationSlug]
		return channel, ok
	case catalogm.PlaylistSourceWFMU:
		channel, ok = wfmuLiveChannelByStationSlug[stationSlug]
		return channel, ok
	default:
		return "", false
	}
}

// =============================================================================
// TTL cache
// =============================================================================

// nowPlayingCacheEntry is one station's cached payload. The per-entry mutex
// serializes refreshes: while one request fetches, concurrent requests for
// the same station wait and then read the fresh value.
type nowPlayingCacheEntry struct {
	mu      sync.Mutex
	value   *contracts.RadioNowPlayingResponse
	expires time.Time
}

// nowPlayingCache is the per-station TTL cache. Entries are keyed by station
// ID, so its size is bounded by the station count (a handful). Cached
// responses are shared pointers — callers must treat them as read-only.
type nowPlayingCache struct {
	mu      sync.Mutex
	entries map[uint]*nowPlayingCacheEntry
	ttl     time.Duration
	now     func() time.Time // injectable for expiry tests
}

func newNowPlayingCache(ttl time.Duration) *nowPlayingCache {
	return &nowPlayingCache{
		entries: make(map[uint]*nowPlayingCacheEntry),
		ttl:     ttl,
		now:     time.Now,
	}
}

// getOrFetch returns the cached value when fresh, otherwise runs fetch and
// caches its result. Errors are NOT cached — the next request retries.
func (c *nowPlayingCache) getOrFetch(key uint, fetch func() (*contracts.RadioNowPlayingResponse, error)) (*contracts.RadioNowPlayingResponse, error) {
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		entry = &nowPlayingCacheEntry{}
		c.entries[key] = entry
	}
	c.mu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.value != nil && c.now().Before(entry.expires) {
		return entry.value, nil
	}

	value, err := fetch()
	if err != nil {
		if entry.value == nil {
			// A never-filled entry must not survive a failed fetch: numeric
			// station IDs reach here unvalidated (resolveStationID parses
			// them without a DB check), so probing nonexistent IDs would
			// otherwise grow the map unbounded. Lock order is safe — no
			// caller acquires entry.mu while holding c.mu.
			c.mu.Lock()
			if c.entries[key] == entry {
				delete(c.entries, key)
			}
			c.mu.Unlock()
		}
		return nil, err
	}
	entry.value = value
	entry.expires = c.now().Add(c.ttl)
	return value, nil
}

// =============================================================================
// Service entry point
// =============================================================================

// GetStationNowPlaying returns the station's current broadcast (live source
// when available, latest-archive fallback otherwise), via the TTL cache.
func (s *RadioService) GetStationNowPlaying(stationID uint) (*contracts.RadioNowPlayingResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return s.nowPlayingCacheInstance().getOrFetch(stationID, func() (*contracts.RadioNowPlayingResponse, error) {
		return s.fetchStationNowPlaying(stationID)
	})
}

// nowPlayingCacheInstance lazily initializes the cache so tests constructing
// &RadioService{db: ...} directly still work; a test that pre-assigns
// s.npCache (custom TTL / clock) keeps its instance.
func (s *RadioService) nowPlayingCacheInstance() *nowPlayingCache {
	s.npCacheOnce.Do(func() {
		if s.npCache == nil {
			s.npCache = newNowPlayingCache(nowPlayingCacheTTL)
		}
	})
	return s.npCache
}

func (s *RadioService) fetchStationNowPlaying(stationID uint) (*contracts.RadioNowPlayingResponse, error) {
	var station catalogm.RadioStation
	if err := s.db.First(&station, stationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		}
		return nil, fmt.Errorf("failed to get radio station: %w", err)
	}

	if live := s.tryLiveNowPlaying(&station); live != nil {
		return live, nil
	}
	return s.archiveNowPlaying(station.ID)
}

// =============================================================================
// Live path
// =============================================================================

// tryLiveNowPlaying attempts the provider live source. Any failure — no
// routing, provider error, nothing on air — returns nil so the caller serves
// the archive fallback; provider errors are logged, never propagated.
func (s *RadioService) tryLiveNowPlaying(station *catalogm.RadioStation) *contracts.RadioNowPlayingResponse {
	if station.PlaylistSource == nil || *station.PlaylistSource == "" {
		return nil
	}
	source := *station.PlaylistSource

	channel, ok := liveChannelForStation(source, station.Slug)
	if !ok {
		return nil
	}
	provider, cleanup, ok := s.resolveLiveProvider(source)
	if !ok {
		return nil
	}
	defer cleanup()

	live, err := provider.FetchLiveNowPlaying(channel)
	if err != nil {
		slog.Warn("radio now-playing: live fetch failed; serving archive fallback",
			"station_id", station.ID,
			"station_slug", station.Slug,
			"playlist_source", source,
			"error", err)
		return nil
	}
	if live == nil || live.ShowName == "" {
		return nil // provider answered: nothing live on this channel
	}

	return s.buildLiveNowPlayingResponse(station.ID, live)
}

// resolveLiveProvider returns the live adapter for a playlist source, plus a
// cleanup func (providers own rate-limiter tickers). Tests override via the
// liveProviderFactory field.
func (s *RadioService) resolveLiveProvider(source string) (RadioLiveProvider, func(), bool) {
	if s.liveProviderFactory != nil {
		return s.liveProviderFactory(source)
	}
	switch source {
	case catalogm.PlaylistSourceKEXP:
		p := NewKEXPProvider()
		return p, p.Close, true
	case catalogm.PlaylistSourceNTS:
		p := NewNTSProvider()
		return p, p.Close, true
	case catalogm.PlaylistSourceWFMU:
		p := NewWFMUProvider()
		return p, p.Close, true
	default:
		return nil, nil, false
	}
}

func (s *RadioService) buildLiveNowPlayingResponse(stationID uint, live *RadioLiveNowPlaying) *contracts.RadioNowPlayingResponse {
	showName := live.ShowName
	resp := &contracts.RadioNowPlayingResponse{
		Source:   contracts.NowPlayingSourceLive,
		OnAir:    true,
		ShowName: &showName,
		HostName: live.HostName,
		Show:     s.matchStationShow(stationID, live.ShowExternalID, live.ShowName),
	}

	currentArtist := ""
	if live.CurrentTrack != nil {
		resp.CurrentTrack = s.buildLiveTrack(live.CurrentTrack)
		currentArtist = live.CurrentTrack.ArtistName
	}

	resp.RecentArtists = s.liveRecentArtists(live)
	if len(resp.RecentArtists) == 0 {
		// Show-level-only live sources (NTS, WFMU) carry no play history;
		// borrow the hops from the archived playlists.
		resp.RecentArtists = s.archiveRecentArtistsForLive(stationID, resp.Show, currentArtist)
	}
	return resp
}

// matchStationShow resolves a live show to our radio_shows row, scoped to the
// REQUESTED station (PSY-1073: WFMU's catalog is duplicated across channel
// stations, so cross-station matching would be ambiguous by construction).
// Match order: provider external_id (strongest), then exact name
// (case-insensitive). Zero or multiple matches → nil; the response still
// carries the raw show_name so the UI renders plain text, not a wrong link.
// Matching never blocks the response: lookup errors log and return nil.
func (s *RadioService) matchStationShow(stationID uint, externalID *string, name string) *contracts.RadioNowPlayingShowRef {
	if externalID != nil && *externalID != "" {
		if ref := s.findSingleShow("station_id = ? AND external_id = ?", stationID, *externalID); ref != nil {
			return ref
		}
	}
	if name != "" {
		if ref := s.findSingleShow("station_id = ? AND LOWER(name) = LOWER(?)", stationID, name); ref != nil {
			return ref
		}
	}
	return nil
}

// findSingleShow returns a show ref iff the condition matches exactly one row.
func (s *RadioService) findSingleShow(query string, args ...interface{}) *contracts.RadioNowPlayingShowRef {
	var shows []catalogm.RadioShow
	if err := s.db.Where(query, args...).Limit(2).Find(&shows).Error; err != nil {
		slog.Warn("radio now-playing: show match lookup failed", "error", err)
		return nil
	}
	if len(shows) != 1 {
		return nil
	}
	return nowPlayingShowRef(&shows[0])
}

func nowPlayingShowRef(show *catalogm.RadioShow) *contracts.RadioNowPlayingShowRef {
	return &contracts.RadioNowPlayingShowRef{
		ID:       show.ID,
		Name:     show.Name,
		Slug:     show.Slug,
		HostName: show.HostName,
	}
}

// buildLiveTrack converts a provider live track into the response shape,
// linking the artist by exact-name lookup. Release/label links stay nil on
// the live path — the archive pipeline's matching engine owns those, and a
// live page-view path shouldn't grow matching work (album/label still render
// as text).
func (s *RadioService) buildLiveTrack(t *RadioPlayImport) *contracts.RadioNowPlayingTrack {
	track := &contracts.RadioNowPlayingTrack{
		ArtistName:     t.ArtistName,
		TrackTitle:     t.TrackTitle,
		AlbumTitle:     t.AlbumTitle,
		LabelName:      t.LabelName,
		ReleaseYear:    t.ReleaseYear,
		RotationStatus: t.RotationStatus,
		DJComment:      t.DJComment,
	}
	track.ArtistID, track.ArtistSlug = s.lookupArtistByName(t.ArtistName)
	return track
}

// lookupArtistByName resolves an artist by exact (diacritic/case-insensitive)
// name, same semantics as the matching engine's name path (PSY-886 expression
// index). Errors and misses both return nils — never blocks the response.
func (s *RadioService) lookupArtistByName(name string) (*uint, *string) {
	normalized := normalizeName(name)
	if normalized == "" {
		return nil, nil
	}
	var artist catalogm.Artist
	if err := s.db.Select("id", "slug").
		Where("immutable_unaccent(LOWER(name)) = immutable_unaccent(LOWER(?))", normalized).
		First(&artist).Error; err != nil {
		return nil, nil
	}
	return &artist.ID, artist.Slug
}

// liveRecentArtists derives the "earlier:" hops from a live play history
// (KEXP): distinct by artist name, most recent first, excluding the current
// track's artist, capped at the hop limit.
func (s *RadioService) liveRecentArtists(live *RadioLiveNowPlaying) []contracts.RadioEpisodePreviewArtist {
	out := []contracts.RadioEpisodePreviewArtist{}
	seen := make(map[string]bool)
	if live.CurrentTrack != nil {
		seen[recentArtistKey(live.CurrentTrack.ArtistName)] = true
	}
	for _, t := range live.RecentTracks {
		key := recentArtistKey(t.ArtistName)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		preview := contracts.RadioEpisodePreviewArtist{ArtistName: t.ArtistName}
		preview.ArtistID, preview.ArtistSlug = s.lookupArtistByName(t.ArtistName)
		out = append(out, preview)
		if len(out) >= nowPlayingRecentArtistLimit {
			break
		}
	}
	return out
}

func recentArtistKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// archiveRecentArtistsForLive fills a live payload's hops from archived
// playlists: the matched show's latest episode when we have one, else the
// station's v1-heuristic episode. Errors degrade to no hops.
func (s *RadioService) archiveRecentArtistsForLive(stationID uint, matchedShow *contracts.RadioNowPlayingShowRef, skipArtistName string) []contracts.RadioEpisodePreviewArtist {
	empty := []contracts.RadioEpisodePreviewArtist{}

	var showID uint
	if matchedShow != nil {
		showID = matchedShow.ID
	} else {
		show, err := s.mostActiveShow(stationID)
		if err != nil || show == nil {
			return empty
		}
		showID = show.ID
	}

	episode, err := s.latestEpisodeForShow(showID)
	if err != nil || episode == nil {
		return empty
	}
	rows, err := s.episodePlayRows(episode.ID)
	if err != nil {
		return empty
	}
	return recentArtistsFromPlayRows(rows, false, skipArtistName)
}

// =============================================================================
// Archive fallback (the v1 heuristic, server-side)
// =============================================================================

// archiveNowPlaying builds the latest-archive payload: the most-active show's
// latest episode's latest play, mirroring the frontend v1 derivation
// (pickNowPlayingShow + deriveNowPlaying). DB errors here are real server
// errors and DO propagate.
func (s *RadioService) archiveNowPlaying(stationID uint) (*contracts.RadioNowPlayingResponse, error) {
	resp := &contracts.RadioNowPlayingResponse{
		Source:        contracts.NowPlayingSourceLatestArchive,
		OnAir:         false,
		RecentArtists: []contracts.RadioEpisodePreviewArtist{},
	}

	show, err := s.mostActiveShow(stationID)
	if err != nil {
		return nil, err
	}
	if show == nil {
		return resp, nil // station with no shows: empty, honestly-labeled payload
	}
	resp.Show = nowPlayingShowRef(show)
	resp.ShowName = &show.Name

	episode, err := s.latestEpisodeForShow(show.ID)
	if err != nil {
		return nil, err
	}
	if episode == nil {
		return resp, nil
	}
	airDate := normalizeDate(episode.AirDate)
	resp.EpisodeAirDate = &airDate

	rows, err := s.episodePlayRows(episode.ID)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		resp.CurrentTrack = rows[len(rows)-1].toNowPlayingTrack()
		resp.RecentArtists = recentArtistsFromPlayRows(rows, true, "")
	}
	return resp, nil
}

// mostActiveShow picks the station's "current" show: the show with the most
// VISIBLE-aired episodes (per airedEpisodeVisibleSQL, PSY-1285 — NOT raw logged
// episodes, so placeholder/not-yet-aired rows don't inflate the count), ties broken
// by lower id (stable). Returns nil when the station has no shows.
func (s *RadioService) mostActiveShow(stationID uint) (*catalogm.RadioShow, error) {
	var shows []catalogm.RadioShow
	// Rank by VISIBLE-aired episodes only (PSY-1285): count the same episodes
	// latestEpisodeForShow can actually select (airedEpisodeVisibleSQL), so the
	// most-active pick and the latest-episode pick agree — otherwise a show with the
	// most 0-track/not-yet-aired rows could be chosen and then yield an empty payload
	// while a less-"active" show has real archived content.
	err := s.db.Raw(`
		SELECT rs.* FROM radio_shows rs
		LEFT JOIN radio_episodes re ON re.show_id = rs.id
		WHERE rs.station_id = ?
		GROUP BY rs.id
		ORDER BY COUNT(re.id) FILTER (WHERE `+airedEpisodeVisibleSQL("re.")+`) DESC, rs.id ASC
		LIMIT 1`, stationID, time.Now()).Scan(&shows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to pick now-playing show: %w", err)
	}
	if len(shows) == 0 {
		return nil, nil
	}
	return &shows[0], nil
}

// latestEpisodeForShow returns the show's most recent AIRED episode (shared
// ordering, episodeLatestFirstOrderSQL — air window within a day, so it agrees
// with GetEpisodes and the "Latest playlists" feeds), or nil when the show has
// none. Accepted PSY-1297 tradeoff: on a day where the show has BOTH a windowed
// slot episode and a windowless off-schedule extra, NULLS LAST now picks the
// windowed one (the old id-DESC pick was whichever imported last) — the
// scheduled playlist is an acceptable "latest" for this rare shape.
// Aired-only (PSY-1205): the now-playing archive
// fallback derives its "Latest playlist" date + deep-link from this, and a
// future-dated placeholder would render "aired {future date}" and link to an
// empty page. The day-granular date bound can't catch a same-day not-yet-aired
// page, so it shares the feed/directory air-window gate (airedEpisodeVisibleSQL,
// PSY-1285): the selection skips a not-yet-aired (future-windowed) row and a
// windowless 0-track placeholder, which also keeps the live-show artist-hop
// fallback off empty rows. Live detection is unaffected — it is computed from the
// provider/air window, never from this selection.
func (s *RadioService) latestEpisodeForShow(showID uint) (*catalogm.RadioEpisode, error) {
	today, err := s.stationLocalTodayForShow(showID)
	if err != nil {
		return nil, err
	}
	var episodes []catalogm.RadioEpisode
	err = s.db.Where("show_id = ? AND air_date <= ? AND "+airedEpisodeVisibleSQL(""), showID, today, time.Now()).
		Order(episodeLatestFirstOrderSQL("")).
		Limit(1).
		Find(&episodes).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get latest episode: %w", err)
	}
	if len(episodes) == 0 {
		return nil, nil
	}
	return &episodes[0], nil
}

// nowPlayingPlayRow is a radio_plays row joined with its matched entities'
// slugs, for building track payloads without per-row lookups.
type nowPlayingPlayRow struct {
	catalogm.RadioPlay
	ArtistSlug  *string `gorm:"column:artist_slug"`
	ReleaseSlug *string `gorm:"column:release_slug"`
	LabelSlug   *string `gorm:"column:label_slug"`
}

func (r *nowPlayingPlayRow) toNowPlayingTrack() *contracts.RadioNowPlayingTrack {
	return &contracts.RadioNowPlayingTrack{
		ArtistName:     r.ArtistName,
		TrackTitle:     r.TrackTitle,
		AlbumTitle:     r.AlbumTitle,
		LabelName:      r.LabelName,
		ReleaseYear:    r.ReleaseYear,
		RotationStatus: r.RotationStatus,
		DJComment:      r.DJComment,
		ArtistID:       r.ArtistID,
		ArtistSlug:     r.ArtistSlug,
		ReleaseID:      r.ReleaseID,
		ReleaseSlug:    r.ReleaseSlug,
		LabelID:        r.LabelID,
		LabelSlug:      r.LabelSlug,
	}
}

// episodePlayRows loads an episode's plays in position order (position 1 =
// first spin; last row = most recent), with matched-entity slugs joined in.
func (s *RadioService) episodePlayRows(episodeID uint) ([]nowPlayingPlayRow, error) {
	var rows []nowPlayingPlayRow
	err := s.db.Raw(`
		SELECT rp.*, a.slug AS artist_slug, r.slug AS release_slug, l.slug AS label_slug
		FROM radio_plays rp
		LEFT JOIN artists a ON a.id = rp.artist_id
		LEFT JOIN releases r ON r.id = rp.release_id
		LEFT JOIN labels l ON l.id = rp.label_id
		WHERE rp.episode_id = ?
		ORDER BY rp.position ASC`, episodeID).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load episode plays: %w", err)
	}
	return rows, nil
}

// recentArtistsFromPlayRows mirrors the frontend recentArtistsFromEpisode:
// walk plays most-recent-first, de-duplicate by name (case-insensitive),
// cap at the hop limit. skipLast drops the final row (the archive payload's
// "current" track); skipArtistName seeds the dedup set (the live payload's
// current artist). WFMU "Music behind DJ:" segment rows are skipped — the
// hops exist to surface artists, not background-music log entries (PSY-1078).
func recentArtistsFromPlayRows(rows []nowPlayingPlayRow, skipLast bool, skipArtistName string) []contracts.RadioEpisodePreviewArtist {
	out := []contracts.RadioEpisodePreviewArtist{}
	seen := make(map[string]bool)
	if skipArtistName != "" {
		seen[recentArtistKey(skipArtistName)] = true
	}
	for i := len(rows) - 1; i >= 0; i-- {
		if skipLast && i == len(rows)-1 {
			continue
		}
		row := rows[i]
		if isPseudoArtistName(row.ArtistName) {
			continue
		}
		key := recentArtistKey(row.ArtistName)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, contracts.RadioEpisodePreviewArtist{
			ArtistName: row.ArtistName,
			ArtistID:   row.ArtistID,
			ArtistSlug: row.ArtistSlug,
		})
		if len(out) >= nowPlayingRecentArtistLimit {
			break
		}
	}
	return out
}
