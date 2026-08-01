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

// ShowCalendarHandler serves the public per-show ICS download.
//
// A plain chi handler rather than a Huma operation, for the same reason as the
// venue feed: the response is a raw text/calendar document with hand-managed
// cache validators.
type ShowCalendarHandler struct {
	showCalendarService contracts.ShowCalendarServiceInterface
	config              *config.Config
}

// NewShowCalendarHandler creates the public show calendar handler.
func NewShowCalendarHandler(showCalendarService contracts.ShowCalendarServiceInterface, cfg *config.Config) *ShowCalendarHandler {
	return &ShowCalendarHandler{
		showCalendarService: showCalendarService,
		config:              cfg,
	}
}

// GetShowCalendarHandler serves GET/HEAD /shows/{show_id}/calendar.ics.
//
// HEAD is registered alongside GET because calendar clients and link checkers
// probe with it; net/http suppresses the body for HEAD, so one handler serves
// both correctly.
func (h *ShowCalendarHandler) GetShowCalendarHandler(w http.ResponseWriter, r *http.Request) {
	showIdentifier := strings.TrimSpace(chi.URLParam(r, "show_id"))
	if showIdentifier == "" || len(showIdentifier) > maxCalendarIdentifierLength {
		http.Error(w, "invalid show identifier", http.StatusBadRequest)
		return
	}

	event, err := h.showCalendarService.GenerateShowEvent(showIdentifier, calendarFrontendURL(h.config))
	if err != nil {
		var showErr *apperrors.ShowError
		if errors.As(err, &showErr) && showErr.Code == apperrors.CodeShowNotFound {
			http.Error(w, "show not found", http.StatusNotFound)
			return
		}
		// The show identifier is caller-controlled, so it is logged but never
		// echoed into the response body.
		logger.FromContext(r.Context()).Error("show_calendar_generation_failed",
			"show", showIdentifier,
			"error", err.Error(),
		)
		http.Error(w, "failed to generate calendar file", http.StatusInternalServerError)
		return
	}

	// attachment, not inline: this IS the download path, and macOS/Windows hand
	// a downloaded .ics straight to the default calendar client.
	writeCalendarResponse(w, r, "attachment", showEventFilename(event.ShowSlug), event.ETag, event.ICS)
}

// showEventFilename derives the download filename from the show slug.
func showEventFilename(slug string) string {
	return filenameSafeSlug(slug, "show") + ".ics"
}
