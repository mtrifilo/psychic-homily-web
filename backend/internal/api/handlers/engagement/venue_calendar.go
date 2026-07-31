package engagement

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/respond"
	"psychic-homily-backend/internal/services/contracts"
)

// venueFeedCacheControl is what an anonymous, aggressively-polled feed advertises.
//
// "public" (unlike the personal feed's "private") because the payload contains
// no per-subscriber data — every caller gets the same bytes, so shared caches
// SHOULD be allowed to absorb the poll traffic. 15 minutes is the trade being
// made explicit: long enough that a client polling every few minutes mostly
// never reaches the origin, short enough that a cancellation propagates the same
// afternoon it is entered.
const venueFeedCacheControl = "public, max-age=900"

// maxVenueIdentifierLength bounds the path segment before it becomes a database
// lookup. Real slugs are far shorter; this only exists so a public endpoint
// cannot be probed with megabyte-long identifiers.
const maxVenueIdentifierLength = 200

// VenueCalendarHandler serves the public per-venue ICS feed (PSY-1584).
//
// Deliberately a plain chi handler rather than a Huma operation: the response is
// a raw text/calendar document with hand-managed cache validators, which is not
// what Huma's JSON-shaped response modelling is for. This matches how the
// existing personal ICS/Atom feeds are served.
type VenueCalendarHandler struct {
	venueCalendarService contracts.VenueCalendarServiceInterface
	config               *config.Config
}

// NewVenueCalendarHandler creates the public venue calendar feed handler.
func NewVenueCalendarHandler(venueCalendarService contracts.VenueCalendarServiceInterface, cfg *config.Config) *VenueCalendarHandler {
	return &VenueCalendarHandler{
		venueCalendarService: venueCalendarService,
		config:               cfg,
	}
}

// GetVenueCalendarFeedHandler serves GET/HEAD /venues/{venue_id}/calendar.ics.
//
// HEAD is registered alongside GET because calendar clients and link checkers
// probe with it; net/http suppresses the body for HEAD, so one handler serves
// both correctly.
func (h *VenueCalendarHandler) GetVenueCalendarFeedHandler(w http.ResponseWriter, r *http.Request) {
	venueIdentifier := strings.TrimSpace(chi.URLParam(r, "venue_id"))
	if venueIdentifier == "" || len(venueIdentifier) > maxVenueIdentifierLength {
		http.Error(w, "invalid venue identifier", http.StatusBadRequest)
		return
	}

	frontendURL := h.config.Email.FrontendURL
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	feed, err := h.venueCalendarService.GenerateVenueFeed(venueIdentifier, frontendURL)
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			http.Error(w, "venue not found", http.StatusNotFound)
			return
		}
		// The venue identifier is caller-controlled, so it is logged but never
		// echoed into the response body.
		logger.FromContext(r.Context()).Error("venue_calendar_feed_generation_failed",
			"venue", venueIdentifier,
			"error", err.Error(),
		)
		http.Error(w, "failed to generate calendar feed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=\""+venueFeedFilename(feed.VenueSlug)+"\"")
	w.Header().Set("Cache-Control", venueFeedCacheControl)
	w.Header().Set("ETag", feed.ETag)

	// An unchanged feed costs the poller nothing beyond the request itself.
	if matchesETag(r.Header.Get("If-None-Match"), feed.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(http.StatusOK)
	respond.SafeWrite(r.Context(), w, feed.ICS)
}

// matchesETag reports whether an If-None-Match header covers the current ETag.
// Handles the "*" wildcard and comma-separated lists per RFC 9110 8.8.3.
func matchesETag(ifNoneMatch, etag string) bool {
	ifNoneMatch = strings.TrimSpace(ifNoneMatch)
	if ifNoneMatch == "" {
		return false
	}
	if ifNoneMatch == "*" {
		return true
	}
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		if strings.TrimSpace(candidate) == etag {
			return true
		}
	}
	return false
}

// venueFeedFilename derives the download filename from the venue slug.
//
// The slug reaches this function from the database, but it is community-created
// data flowing into a response HEADER, and a header is exactly where a stray
// quote or newline stops being cosmetic. Only slug-shaped characters survive.
func venueFeedFilename(slug string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(slug) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		cleaned = "venue"
	}
	return cleaned + "-shows.ics"
}
