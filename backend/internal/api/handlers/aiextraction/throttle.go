// Package aiextraction exposes the per-user rate-limit gate the Next.js BFF
// extract routes call before doing any (paid) Anthropic work (PSY-855).
//
// The route is registered on the Protected group, so the caller is identified
// by the JWT in the forwarded auth_token cookie — the user_id is NOT taken from
// the request body and cannot be spoofed. Admins bypass the limit per the
// PSY-345 convention (admin == IsAdmin on the authenticated user).
package aiextraction

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/ratelimit"
)

// ThrottleHandler gates the AI extraction routes behind a per-user counter.
type ThrottleHandler struct {
	svc *ratelimit.AIExtractionThrottleService
}

// NewThrottleHandler creates the throttle handler.
func NewThrottleHandler(svc *ratelimit.AIExtractionThrottleService) *ThrottleHandler {
	return &ThrottleHandler{svc: svc}
}

// CheckThrottleRequest is empty — the user is derived from the JWT context, not
// the request body, so it can't be spoofed.
type CheckThrottleRequest struct{}

// CheckThrottleResponse reports the throttle decision to the calling BFF route.
//
// The endpoint itself always returns 200 with this body (it is an internal
// decision oracle, not the user-facing surface). The Next.js route translates
// Allowed=false into the user-facing 429 (status + Retry-After header + JSON
// body), and only calls Anthropic when Allowed=true.
type CheckThrottleResponse struct {
	Body struct {
		Allowed bool `json:"allowed"`
		// RetryAfterSeconds is the whole-second wait until the user's window
		// resets. Only meaningful (and >= 1) when Allowed is false.
		RetryAfterSeconds int `json:"retry_after_seconds"`
		// Limit / WindowSeconds echo the active policy so the caller can build
		// human-readable copy without hardcoding the numbers in two places.
		Limit         int `json:"limit"`
		WindowSeconds int `json:"window_seconds"`
	}
}

// CheckThrottleHandler records one extraction attempt for the authenticated
// user and returns whether it is allowed. Admins always pass without consuming
// a slot (PSY-345).
func (h *ThrottleHandler) CheckThrottleHandler(ctx context.Context, _ *CheckThrottleRequest) (*CheckThrottleResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		// Defensive: the Protected middleware already guarantees a user, but a
		// nil here must fail closed (401), never silently "allow".
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	resp := &CheckThrottleResponse{}
	resp.Body.Limit = ratelimit.AIExtractionLimit
	resp.Body.WindowSeconds = int(ratelimit.AIExtractionWindow.Seconds())

	// Admins bypass the limit and do NOT increment the counter — bulk admin
	// imports shouldn't burn the per-user budget (PSY-345).
	if user.IsAdmin {
		resp.Body.Allowed = true
		return resp, nil
	}

	decision, err := h.svc.CheckAndIncrement(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("ai_extraction_throttle_failed",
			"user_id", user.ID,
			"error", err.Error(),
		)
		// Fail CLOSED: if we can't account the attempt we must not hand out a
		// free pass that would let a user drain the Anthropic budget while the
		// counter is down. Surface a 503 so the BFF route does NOT call Anthropic.
		return nil, huma.Error503ServiceUnavailable("Rate limit check temporarily unavailable")
	}

	resp.Body.Allowed = decision.Allowed
	resp.Body.RetryAfterSeconds = decision.RetryAfterSeconds
	return resp, nil
}
