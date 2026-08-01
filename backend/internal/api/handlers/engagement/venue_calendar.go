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

// publicCalendarCacheControl is what every anonymous calendar surface advertises.
//
// "public" (unlike the personal feed's "private") because the payload contains
// no per-subscriber data — every caller gets the same bytes, so shared caches
// SHOULD be allowed to absorb the traffic. 15 minutes is the trade being
// made explicit: long enough that a client polling every few minutes mostly
// never reaches the origin, short enough that a cancellation propagates the same
// afternoon it is entered.
const publicCalendarCacheControl = "public, max-age=900"

// maxCalendarIdentifierLength bounds the path segment before it becomes a
// database lookup. Real slugs are far shorter; this only exists so a public
// endpoint cannot be probed with megabyte-long identifiers.
const maxCalendarIdentifierLength = 200

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
	if venueIdentifier == "" || len(venueIdentifier) > maxCalendarIdentifierLength {
		http.Error(w, "invalid venue identifier", http.StatusBadRequest)
		return
	}

	feed, err := h.venueCalendarService.GenerateVenueFeed(venueIdentifier, calendarFrontendURL(h.config))
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

	// An unchanged feed costs the poller nothing beyond the request itself —
	// writeCalendarResponse answers a matching If-None-Match with a bodyless 304.
	writeCalendarResponse(w, r, "inline", venueFeedFilename(feed.VenueSlug), feed.ETag, feed.ICS)
}

// calendarFrontendURL resolves the origin used for links back into the app.
func calendarFrontendURL(cfg *config.Config) string {
	if cfg.Email.FrontendURL != "" {
		return cfg.Email.FrontendURL
	}
	return "http://localhost:3000"
}

// writeCalendarResponse sets the shared calendar-serving headers and answers
// conditional requests. disposition is "inline" (a feed rendered in place) or
// "attachment" (a download handed to the OS calendar client).
func writeCalendarResponse(w http.ResponseWriter, r *http.Request, disposition, filename, etag string, body []byte) {
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	w.Header().Set("Cache-Control", publicCalendarCacheControl)
	w.Header().Set("ETag", etag)

	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(http.StatusOK)
	respond.SafeWrite(r.Context(), w, body)
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
func venueFeedFilename(slug string) string {
	return filenameSafeSlug(slug, "venue") + "-shows.ics"
}

// filenameSafeSlug reduces a slug to filename-header-safe characters.
//
// The slug reaches this function from the database, but it is community-created
// data flowing into a response HEADER, and a header is exactly where a stray
// quote or newline stops being cosmetic. Only slug-shaped characters survive.
func filenameSafeSlug(slug, fallback string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(slug) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		cleaned = fallback
	}
	return cleaned
}
