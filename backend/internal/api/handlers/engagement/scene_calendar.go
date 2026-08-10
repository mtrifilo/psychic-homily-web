package engagement

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// SceneCalendarHandler serves the public per-scene ICS feed.
//
// Deliberately a plain chi handler rather than a Huma operation, matching its
// venue sibling: the response is a raw text/calendar document with hand-managed
// cache validators, which is not what Huma's JSON-shaped response modelling is
// for.
//
// Auth posture is identical to the venue feed's and is stated here so it cannot
// drift by accident: anonymous, unauthenticated, no token, no session. A scene
// is a computed aggregation of already-public listings, so there is nothing to
// scope to a user. It shares the venue feed's shared-cacheable Cache-Control for
// the same reason — every caller gets the same bytes.
type SceneCalendarHandler struct {
	sceneCalendarService contracts.SceneCalendarServiceInterface
	config               *config.Config
}

// NewSceneCalendarHandler creates the public scene calendar feed handler.
func NewSceneCalendarHandler(sceneCalendarService contracts.SceneCalendarServiceInterface, cfg *config.Config) *SceneCalendarHandler {
	return &SceneCalendarHandler{
		sceneCalendarService: sceneCalendarService,
		config:               cfg,
	}
}

// GetSceneCalendarFeedHandler serves GET/HEAD /scenes/{slug}/calendar.ics.
//
// HEAD is registered alongside GET because calendar clients and link checkers
// probe with it; net/http suppresses the body for HEAD, so one handler serves
// both correctly.
func (h *SceneCalendarHandler) GetSceneCalendarFeedHandler(w http.ResponseWriter, r *http.Request) {
	sceneSlug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if sceneSlug == "" || len(sceneSlug) > maxCalendarIdentifierLength {
		http.Error(w, "invalid scene identifier", http.StatusBadRequest)
		return
	}

	feed, err := h.sceneCalendarService.GenerateSceneFeed(sceneSlug, calendarFrontendURL(h.config))
	if err != nil {
		var sceneErr *apperrors.SceneError
		if errors.As(err, &sceneErr) && sceneErr.Code == apperrors.CodeSceneNotFound {
			http.Error(w, "scene not found", http.StatusNotFound)
			return
		}
		// The slug is caller-controlled, so it is logged but never echoed into
		// the response body.
		logger.FromContext(r.Context()).Error("scene_calendar_feed_generation_failed",
			"scene", sceneSlug,
			"error", err.Error(),
		)
		http.Error(w, "failed to generate calendar feed", http.StatusInternalServerError)
		return
	}

	// An unchanged feed costs the poller nothing beyond the request itself —
	// writeCalendarResponse answers a matching If-None-Match with a bodyless 304.
	writeCalendarResponse(w, r, "inline", sceneFeedFilename(feed.SceneSlug), feed.ETag, feed.ICS)
}

// sceneFeedFilename derives the download filename from the scene's canonical
// slug, through the same header-safety reduction the venue feed uses.
func sceneFeedFilename(slug string) string {
	return filenameSafeSlug(slug, "scene") + "-shows.ics"
}
