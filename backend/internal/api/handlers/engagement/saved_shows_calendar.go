package engagement

import (
	"net/http"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
)

// savedShowsDownloadCacheControl keeps the one-shot download out of every
// cache. The file is one user's saved list, so no shared cache may hold it, and
// it is rendered fresh on each request, so a private cache holding it would only
// replay a list the user has since changed.
const savedShowsDownloadCacheControl = "private, no-store"

// savedShowsDownloadDisposition is "attachment" so a desktop or Android browser
// hands the file to the calendar app, the same as the per-show download.
// "inline" is the alternative for a browser that offers an import sheet for an
// inline calendar.
const savedShowsDownloadDisposition = "attachment"

// savedShowsDownloadFilename is the name the downloaded file is saved under.
const savedShowsDownloadFilename = "psychic-homily-saved-shows.ics"

// GetSavedShowsCalendarDownloadHandler serves GET /saved-shows/calendar.ics: a
// one-shot calendar file of the signed-in user's upcoming saved shows, for
// calendar apps that cannot subscribe to a feed URL.
//
// The route sits behind middleware.JWTMiddleware, which refuses a request with
// no valid session before this runs; the nil check is the handler's own floor.
//
// It renders through RenderSavedShowsCalendar rather than GenerateICSFeed,
// because the subscription feed's per-user cache would leave out a show saved a
// moment before the download. The response carries no ETag: under no-store no
// cache keeps the bytes a validator would revalidate.
func (h *CalendarHandler) GetSavedShowsCalendarDownloadHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	icsData, err := h.calendarService.RenderSavedShowsCalendar(user.ID, calendarFrontendURL(h.config))
	if err != nil {
		logger.FromContext(r.Context()).Error("saved_shows_calendar_download_failed",
			"user_id", user.ID,
			"error", err.Error(),
		)
		http.Error(w, "failed to generate calendar file", http.StatusInternalServerError)
		return
	}

	writeCalendarResponse(w, r, savedShowsDownloadCacheControl, savedShowsDownloadDisposition,
		savedShowsDownloadFilename, "", icsData)
}
