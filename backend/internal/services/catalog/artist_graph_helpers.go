package catalog

// Shared helpers for the artist graph endpoints (scene graph PSY-367, venue
// bill network PSY-365, station graph PSY-1081). Extracted in PSY-1081 when a
// third copy of the upcoming-show-count batch query was about to land.

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// batchArtistUpcomingShowCounts returns a map of artist_id → upcoming
// approved show count (globally, not scoped to the graph's anchor entity), so
// the graph node green-dot indicator stays consistent with the rest of the
// app. Returns an empty map (never nil) so callers can index without a nil
// check. Errors degrade to zero counts — the indicator is decorative, not
// load-bearing (same posture as the original scene/venue helpers).
func batchArtistUpcomingShowCounts(db *gorm.DB, artistIDs []uint) map[uint]int {
	out := make(map[uint]int, len(artistIDs))
	if len(artistIDs) == 0 {
		return out
	}
	type row struct {
		ArtistID  uint
		ShowCount int64
	}
	var rows []row
	db.Table("show_artists").
		Select("show_artists.artist_id, COUNT(DISTINCT shows.id) AS show_count").
		Joins("JOIN shows ON shows.id = show_artists.show_id").
		Where("show_artists.artist_id IN ? AND shows.status = ? AND shows.event_date > NOW()",
			artistIDs, catalogm.ShowStatusApproved).
		Group("show_artists.artist_id").
		Scan(&rows)
	for _, r := range rows {
		out[r.ArtistID] = int(r.ShowCount)
	}
	return out
}

// batchArtistNextShows returns a map of artist_id → summary of that artist's
// SOONEST upcoming approved show, in ONE query for the whole node set
// (PSY-1449) — the batched analog of ArtistService.GetNextShowForArtist
// (PSY-1352), whose per-artist 2-query join would cost 2N round-trips here.
//
// DISTINCT ON (artist_id) with the same event_date ASC + shows.id ASC ordering
// keeps the pick deterministic and consistent with the graph-card's next-show
// line; the trailing venue_id tiebreak pins which venue row wins for the rare
// multi-venue show. The upcoming cutoff (event_date > NOW()) deliberately
// matches batchArtistUpcomingShowCounts, NOT GetNextShowForArtist's
// timezone-aware start-of-today — on a graph node the invariant that matters
// is next_show ≠ nil ⟺ upcoming_show_count > 0.
//
// Artists with no upcoming show are simply absent (map lookup yields nil).
// Errors degrade to an empty map — same decorative posture as the sibling
// count helper above.
func batchArtistNextShows(db *gorm.DB, artistIDs []uint) map[uint]*contracts.ArtistGraphCardShow {
	out := make(map[uint]*contracts.ArtistGraphCardShow, len(artistIDs))
	if len(artistIDs) == 0 {
		return out
	}
	type row struct {
		ArtistID      uint
		ShowID        uint
		EventDate     time.Time
		VenueName     *string
		VenueCity     *string
		VenueState    *string
		VenueTimezone *string
	}
	var rows []row
	db.Table("show_artists").
		Select(`DISTINCT ON (show_artists.artist_id)
			show_artists.artist_id,
			shows.id AS show_id,
			shows.event_date,
			venues.name AS venue_name,
			venues.city AS venue_city,
			venues.state AS venue_state,
			venues.timezone AS venue_timezone`).
		Joins("JOIN shows ON shows.id = show_artists.show_id").
		Joins("LEFT JOIN show_venues ON show_venues.show_id = shows.id").
		Joins("LEFT JOIN venues ON venues.id = show_venues.venue_id").
		Where("show_artists.artist_id IN ? AND shows.status = ? AND shows.event_date > NOW()",
			artistIDs, catalogm.ShowStatusApproved).
		Order("show_artists.artist_id, shows.event_date ASC, shows.id ASC, show_venues.venue_id ASC").
		Scan(&rows)
	for _, r := range rows {
		next := &contracts.ArtistGraphCardShow{
			ID:            r.ShowID,
			EventDate:     r.EventDate,
			VenueTimezone: r.VenueTimezone,
		}
		// LEFT-JOINed venue columns are nil for a venueless show; the contract
		// carries plain strings (empty = unknown), matching the graph-card
		// handler's mapping of a nil ArtistShowResponse.Venue.
		if r.VenueName != nil {
			next.VenueName = *r.VenueName
		}
		if r.VenueCity != nil {
			next.VenueCity = *r.VenueCity
		}
		if r.VenueState != nil {
			next.VenueState = *r.VenueState
		}
		out[r.ArtistID] = next
	}
	return out
}

// spotifyEmbeddablePath mirrors the frontend parseSpotifyEmbed (lib/spotify.ts):
// an open.spotify.com artist/album/track URL with the canonical 22-char base62
// id. The id length IS pinned to 22 here — unlike isValidSpotifyURL in the
// artist handler, which deliberately does not — so this flag never marks a node
// the frontend embed can't actually play (a dead marker, PSY-1379 AC).
var spotifyEmbeddablePath = regexp.MustCompile(`/(artist|album|track)/[A-Za-z0-9]{22}(?:/|$)`)

// hasEmbeddableSpotify reports whether a stored Spotify URL is one the frontend
// MusicEmbed can render. Kept in sync with parseSpotifyEmbed (lib/spotify.ts):
// canonical open.spotify.com host + a 22-char base62 id on an artist/album/track
// path. Stored values are http(s) URLs (the artist create/update validator
// requires one), so the `spotify:` URI form the FE also accepts is not handled.
func hasEmbeddableSpotify(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	// Tolerate scheme-less stored values (e.g. "open.spotify.com/artist/<id>").
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || strings.ToLower(u.Hostname()) != "open.spotify.com" {
		return false
	}
	return spotifyEmbeddablePath.MatchString(u.Path)
}

// batchArtistPlayableAudio returns artist_id → whether selecting that node opens
// a playable embed (PSY-1379): a stored Bandcamp embed URL, or a Spotify URL the
// frontend can embed. Mirrors the ArtistContextPanel `hasPlayableAudio` gate so
// the scene-graph canvas marker never promises a player the panel won't render.
// Returns an empty map (never nil); a query error degrades to "no markers" —
// the affordance is decorative, not load-bearing (same posture as the
// upcoming-show-count batch above).
func batchArtistPlayableAudio(db *gorm.DB, artistIDs []uint) map[uint]bool {
	out := make(map[uint]bool, len(artistIDs))
	if len(artistIDs) == 0 {
		return out
	}
	type row struct {
		ID               uint    `gorm:"column:id"`
		BandcampEmbedURL *string `gorm:"column:bandcamp_embed_url"`
		Spotify          *string `gorm:"column:spotify"`
	}
	var rows []row
	if err := db.Model(&catalogm.Artist{}).
		Select("id, bandcamp_embed_url, spotify").
		Where("id IN ?", artistIDs).
		Scan(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		bandcamp := r.BandcampEmbedURL != nil && strings.TrimSpace(*r.BandcampEmbedURL) != ""
		spotify := r.Spotify != nil && hasEmbeddableSpotify(*r.Spotify)
		if bandcamp || spotify {
			out[r.ID] = true
		}
	}
	return out
}
