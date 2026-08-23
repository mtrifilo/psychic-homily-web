package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
)

// Account-level alert preferences (PSY-1907).
//
// One read serves the whole account alerts surface (the home area and the
// resolved alert matrix), because both are edited on the same settings card and
// the matrix is only meaningful next to the area that its near-me scope depends
// on. Writes stay one endpoint per preference, matching every sibling in
// user_preferences.go, and both return the full resolved state so the client
// re-renders from the response instead of guessing what the server stored.

// AlertPreferencesResponse is the shared read shape: home area plus the
// RESOLVED alert matrix. Resolved, not raw, because "unset" only has meaning
// against the shipped defaults and duplicating those in the client is exactly
// the drift the three-layer design exists to avoid.
type AlertPreferencesResponse struct {
	Body struct {
		Success       bool                       `json:"success"`
		HomeMetro     *string                    `json:"home_metro" doc:"Home metro CBSA code, or null when no home area is set"`
		AlertDefaults authm.AccountAlertDefaults `json:"alert_defaults" doc:"Resolved account-level alert defaults, per alert type and channel"`
	}
}

// GetAlertPreferencesRequest takes no input beyond the session.
type GetAlertPreferencesRequest struct{}

// GetAlertPreferencesHandler handles GET /auth/preferences/alerts.
func (h *UserPreferencesHandler) GetAlertPreferencesHandler(ctx context.Context, _ *GetAlertPreferencesRequest) (*AlertPreferencesResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	return h.alertPreferencesResponse(ctx, user.ID)
}

// SetHomeMetroRequest replaces the user's home area. Omitting metro, or sending
// null or an empty string, clears it. This is a PUT, so the body is the whole
// new value.
type SetHomeMetroRequest struct {
	Body struct {
		Metro *string `json:"metro" required:"false" doc:"Metro CBSA code (e.g. 38060); null or empty clears the home area" example:"38060"`
	}
}

// SetHomeMetroHandler handles PUT /auth/preferences/home-metro.
func (h *UserPreferencesHandler) SetHomeMetroHandler(ctx context.Context, req *SetHomeMetroRequest) (*AlertPreferencesResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if err := h.userService.SetHomeMetro(user.ID, req.Body.Metro); err != nil {
		logger.FromContext(ctx).Error("set_home_metro_failed",
			"error", err.Error(),
			"user_id", user.ID,
			"request_id", logger.GetRequestID(ctx),
		)
		// Only a rejected value is a client error. A failed write is not, and
		// reporting one as a 422 would tell the user their input was bad, stop
		// the client retrying, and log a 4xx for a server fault. The service
		// error text is never returned either: it can carry driver detail.
		var authErr *autherrors.AuthError
		if errors.As(err, &authErr) && authErr.Code == autherrors.CodeUnknownHomeMetro {
			return nil, huma.Error422UnprocessableEntity(authErr.Message)
		}
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to save home metro (request_id: %s)", logger.GetRequestID(ctx)),
		)
	}

	logger.FromContext(ctx).Info("set_home_metro_success",
		"user_id", user.ID,
		"cleared", req.Body.Metro == nil,
	)

	return h.alertPreferencesResponse(ctx, user.ID)
}

// AlertChannelDefaultsInput is one alert type's channel update. Both fields are
// optional pointers so a caller can flip one channel without pinning the other:
// an absent channel keeps inheriting the shipped default rather than being
// written at today's resolved value.
type AlertChannelDefaultsInput struct {
	InApp *bool `json:"in_app,omitempty" required:"false" doc:"Deliver this alert type in-app by default"`
	Email *bool `json:"email,omitempty" required:"false" doc:"Deliver this alert type by email by default"`
}

// SetAlertDefaultsRequest is a partial update to the account alert matrix.
type SetAlertDefaultsRequest struct {
	Body struct {
		Shows    *AlertChannelDefaultsInput `json:"shows,omitempty" required:"false" doc:"Default channels for new-show alerts"`
		Releases *AlertChannelDefaultsInput `json:"releases,omitempty" required:"false" doc:"Default channels for new-release alerts"`
	}
}

// SetAlertDefaultsHandler handles PATCH /auth/preferences/alert-defaults.
func (h *UserPreferencesHandler) SetAlertDefaultsHandler(ctx context.Context, req *SetAlertDefaultsRequest) (*AlertPreferencesResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	update := authm.AccountAlertDefaultsUpdate{
		Shows:    alertChannelUpdate(req.Body.Shows),
		Releases: alertChannelUpdate(req.Body.Releases),
	}
	// A request that pins nothing is rejected rather than silently accepted, so
	// a client that mis-serialised its body finds out. Matches the 422 the
	// comment-notification and tier-notification toggles return.
	if update.SetsNothing() {
		return nil, huma.Error422UnprocessableEntity("No alert defaults provided")
	}

	if err := h.userService.SetAccountAlertDefaults(user.ID, update); err != nil {
		logger.FromContext(ctx).Error("set_alert_defaults_failed",
			"error", err.Error(),
			"user_id", user.ID,
			"request_id", logger.GetRequestID(ctx),
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update alert defaults (request_id: %s)", logger.GetRequestID(ctx)),
		)
	}

	logger.FromContext(ctx).Info("set_alert_defaults_success",
		"user_id", user.ID,
	)

	return h.alertPreferencesResponse(ctx, user.ID)
}

// alertChannelUpdate adapts one alert type's request input to the service
// update shape. A nil input stays nil, which is what "leave this alert type
// alone" means all the way down.
func alertChannelUpdate(input *AlertChannelDefaultsInput) *authm.AlertChannelDefaultsUpdate {
	if input == nil {
		return nil
	}
	return &authm.AlertChannelDefaultsUpdate{InApp: input.InApp, Email: input.Email}
}

// alertPreferencesResponse reads the authoritative state back from the database
// and renders it. Reading back rather than echoing the request is what makes a
// write's response show the MERGED result: a request only ever carries the axes
// it changed.
func (h *UserPreferencesHandler) alertPreferencesResponse(ctx context.Context, userID uint) (*AlertPreferencesResponse, error) {
	prefs, err := h.userService.GetAlertPreferences(userID)
	// A nil result with a nil error is a broken implementation, not a state
	// this endpoint can render: every real user resolves to at least the
	// shipped defaults. Treat it as the server fault it is rather than
	// dereferencing it.
	if err == nil && prefs == nil {
		err = fmt.Errorf("alert preferences resolved to nothing")
	}
	if err != nil {
		logger.FromContext(ctx).Error("get_alert_preferences_failed",
			"error", err.Error(),
			"user_id", userID,
			"request_id", logger.GetRequestID(ctx),
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to load alert preferences (request_id: %s)", logger.GetRequestID(ctx)),
		)
	}

	resp := &AlertPreferencesResponse{}
	resp.Body.Success = true
	resp.Body.HomeMetro = prefs.HomeMetro
	resp.Body.AlertDefaults = prefs.AlertDefaults
	return resp, nil
}
