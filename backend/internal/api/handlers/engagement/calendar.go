package engagement

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/respond"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// CalendarHandler handles calendar feed and token management
type CalendarHandler struct {
	calendarService contracts.CalendarServiceInterface
	config          *config.Config
	// afterFeedResponse takes work once a feed response has been written, so
	// the work can neither delay nor change that response, and reports
	// whether it took it.
	afterFeedResponse func(work func()) bool
}

// NewCalendarHandler creates a new calendar handler
func NewCalendarHandler(calendarService contracts.CalendarServiceInterface, cfg *config.Config) *CalendarHandler {
	return &CalendarHandler{
		calendarService:   calendarService,
		config:            cfg,
		afterFeedResponse: boundedDetachedRunner(maxConcurrentFeedReadRecords),
	}
}

// feedReadRecordTimeout bounds how long one feed-read record may hold its
// slot and a database connection.
const feedReadRecordTimeout = 5 * time.Second

// maxConcurrentFeedReadRecords bounds the feed-read records in flight. The
// feed routes are exempt from the public-read limiter and the record runs
// after its request has ended, so without a bound polls could start records
// faster than the database pool drains them. A record past the bound is
// dropped; the client's next poll records the read instead.
const maxConcurrentFeedReadRecords = 8

// boundedDetachedRunner returns a runner that runs each work on its own
// goroutine while fewer than limit are running, and drops the work otherwise,
// reporting which it did. A panic in work is logged and stops there: nothing
// upstream of the goroutine can recover it, and it would otherwise end the
// process.
func boundedDetachedRunner(limit int) func(work func()) bool {
	slots := make(chan struct{}, limit)
	return func(work func()) bool {
		select {
		case slots <- struct{}{}:
		default:
			logger.Default().Debug("calendar_feed_after_response_dropped")
			return false
		}
		go func() {
			defer func() { <-slots }()
			defer func() {
				if p := recover(); p != nil {
					logger.Default().Error("calendar_feed_after_response_panic", "panic", fmt.Sprint(p))
				}
			}()
			work()
		}()
		return true
	}
}

// recordFeedRead hands the record of a served feed read to afterFeedResponse.
// The request's context ends with the response, so the write runs on a
// context detached from it and bounded by feedReadRecordTimeout. A failure is
// logged and goes no further.
func (h *CalendarHandler) recordFeedRead(r *http.Request, token string) {
	ctx := context.WithoutCancel(r.Context())
	userAgent := r.UserAgent()
	h.afterFeedResponse(func() {
		ctx, cancel := context.WithTimeout(ctx, feedReadRecordTimeout)
		defer cancel()
		if err := h.calendarService.RecordFeedRead(ctx, token, userAgent); err != nil {
			logger.FromContext(ctx).Warn("calendar_feed_read_record_failed",
				"error", err.Error(),
			)
		}
	})
}

// --- ICS Feed endpoint (Chi http.HandlerFunc, public, token-authenticated) ---

// GetCalendarFeedHandler serves the ICS calendar feed and, once the feed is
// written, records which calendar client family read it (recordFeedRead).
// Only a served feed is recorded.
func (h *CalendarHandler) GetCalendarFeedHandler(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	user, err := h.calendarService.ValidateCalendarToken(token)
	if err != nil {
		logger.FromContext(r.Context()).Warn("calendar_feed_invalid_token",
			"error", err.Error(),
		)
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	frontendURL := h.config.Email.FrontendURL
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	icsData, err := h.calendarService.GenerateICSFeed(user.ID, frontendURL)
	if err != nil {
		logger.FromContext(r.Context()).Error("calendar_feed_generation_failed",
			"user_id", user.ID,
			"error", err.Error(),
		)
		http.Error(w, "failed to generate calendar feed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=\"psychic-homily-saved-shows.ics\"")
	// Short private cache: calendar clients poll coarsely; regenerate clears
	// the server-side cache immediately, and private avoids shared-proxy reuse.
	w.Header().Set("Cache-Control", "private, max-age=120")
	w.WriteHeader(http.StatusOK)
	respond.SafeWrite(r.Context(), w, icsData)
	h.recordFeedRead(r, token)
}

// GetFollowsActivityFeedHandler serves the Atom feed of followed-artist
// activity (new shows + new releases). Same personal feed token as iCal
// (PSY-1505); never session JWT.
func (h *CalendarHandler) GetFollowsActivityFeedHandler(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	user, err := h.calendarService.ValidateCalendarToken(token)
	if err != nil {
		logger.FromContext(r.Context()).Warn("follows_activity_feed_invalid_token",
			"error", err.Error(),
		)
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	frontendURL := h.config.Email.FrontendURL
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	atomData, err := h.calendarService.GenerateFollowsActivityFeed(user.ID, frontendURL)
	if err != nil {
		logger.FromContext(r.Context()).Error("follows_activity_feed_generation_failed",
			"user_id", user.ID,
			"error", err.Error(),
		)
		http.Error(w, "failed to generate activity feed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=\"psychic-homily-follows.atom\"")
	w.Header().Set("Cache-Control", "private, max-age=120")
	w.WriteHeader(http.StatusOK)
	respond.SafeWrite(r.Context(), w, atomData)
}

// --- Token CRUD endpoints (Huma, protected) ---

// CreateCalendarTokenRequest is empty — user is derived from JWT context
type CreateCalendarTokenRequest struct{}

// CreateCalendarTokenResponse wraps the service response
type CreateCalendarTokenResponse struct {
	Body contracts.CalendarTokenCreateResponse
}

// CreateCalendarTokenHandler creates or regenerates a calendar token.
//
// The feed URL is a bearer credential, but unlike the other credential mints
// this one is not gated on a recent authentication
// (middleware.RequireRecentSessionAuth): any session can already read the
// current URL from the status endpoint, so gating its rotation protects
// nothing more. It does require a session credential, because the protected
// group also admits API-token (phk_) principals, and those must not mint,
// rotate or read the account's feed URLs. A CLI token is a session JWT and
// counts as a session here.
func (h *CalendarHandler) CreateCalendarTokenHandler(ctx context.Context, req *CreateCalendarTokenRequest) (*CreateCalendarTokenResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if err := middleware.RequireSessionCredential(ctx, "create_calendar_token"); err != nil {
		return nil, err
	}

	// Feed URLs use the API domain, not the frontend domain.
	apiBaseURL := h.config.BackendOrigin()

	result, err := h.calendarService.CreateToken(user.ID, user.SessionsValidAfter, apiBaseURL)
	if err != nil {
		// The session was revoked, or the account closed, after the JWT
		// middleware validated it.
		if shared.IsSessionRefusal(err) {
			return nil, middleware.RefuseSession(ctx, "create_calendar_token", err, h.config.Session)
		}
		logger.FromContext(ctx).Error("create_calendar_token_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create calendar token (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("create_calendar_token_success",
		"user_id", user.ID,
		"request_id", requestID,
	)

	return &CreateCalendarTokenResponse{Body: *result}, nil
}

// GetCalendarTokenStatusRequest is empty — user is derived from JWT context
type GetCalendarTokenStatusRequest struct{}

// GetCalendarTokenStatusResponse wraps the service response. It can carry
// the feed URLs, which are bearer credentials, so no cache may keep it.
type GetCalendarTokenStatusResponse struct {
	CacheControl string `header:"Cache-Control"`
	Body         contracts.CalendarTokenStatusResponse
}

// GetCalendarTokenStatusHandler reports whether the caller holds a feed token
// and, to a session caller, the feed URLs when they are recoverable. An
// API-token (phk_) principal is told only whether a token exists and when it
// was minted: the protected group admits API tokens, and one must not be
// exchangeable for a credential to the account's feeds. A CLI token is a
// session JWT and counts as a session here.
func (h *CalendarHandler) GetCalendarTokenStatusHandler(ctx context.Context, req *GetCalendarTokenStatusRequest) (*GetCalendarTokenStatusResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	status, err := h.calendarService.GetTokenStatus(user.ID, h.config.BackendOrigin())
	if err != nil {
		logger.FromContext(ctx).Error("get_calendar_token_status_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get calendar token status (request_id: %s)", requestID),
		)
	}

	body := *status
	if !middleware.IsSessionCredentialFromContext(ctx) {
		body = body.WithoutFeedURLs()
	}

	return &GetCalendarTokenStatusResponse{CacheControl: "no-store", Body: body}, nil
}

// DeleteCalendarTokenRequest is empty — user is derived from JWT context
type DeleteCalendarTokenRequest struct{}

// DeleteCalendarTokenResponse returns success status
type DeleteCalendarTokenResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// DeleteCalendarTokenHandler removes a user's calendar token
func (h *CalendarHandler) DeleteCalendarTokenHandler(ctx context.Context, req *DeleteCalendarTokenRequest) (*DeleteCalendarTokenResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	err := h.calendarService.DeleteToken(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("delete_calendar_token_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to delete calendar token (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("delete_calendar_token_success",
		"user_id", user.ID,
		"request_id", requestID,
	)

	return &DeleteCalendarTokenResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Calendar token deleted successfully",
		},
	}, nil
}
