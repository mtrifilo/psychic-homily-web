package auth

import (
	"net/http"
	"strconv"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/engagement"
)

// Chi-level (NOT Huma) unsubscribe handlers for the tier-change and
// edit-review notification categories. Same dual-method shape as the
// collection-digest handler (UnsubscribeCollectionDigestPageHandler):
//
//   - GET renders an HTML confirmation page for a recipient who clicks the
//     visible in-body link.
//   - POST accepts the RFC 8058 / RFC 2369 one-click body Gmail/Yahoo send
//     when a recipient clicks the native "Unsubscribe" button.
//
// Both read uid+sig from the query string and verify the per-scope HMAC; no
// login required. The scope binds the signature so a link minted for one
// category can't be replayed against another.

// scopedUnsubscribeConfig pairs an unsubscribe scope with the preference
// mutation it performs and the human-readable noun for the confirmation page.
type scopedUnsubscribeConfig struct {
	scope     string
	setPref   func(userID uint) error
	noun      string // e.g. "tier-change emails"
	logSuffix string // structured-log event discriminator
}

// UnsubscribeTierNotificationsPageHandler serves /unsubscribe/tier-notifications.
func (h *UserPreferencesHandler) UnsubscribeTierNotificationsPageHandler(w http.ResponseWriter, r *http.Request) {
	h.handleScopedUnsubscribe(w, r, scopedUnsubscribeConfig{
		scope:     engagement.UnsubscribeScopeTierNotifications,
		setPref:   func(uid uint) error { return h.userService.SetNotifyOnTierNotifications(uid, false) },
		noun:      "tier-change emails",
		logSuffix: "tier_notifications",
	})
}

// UnsubscribeEditNotificationsPageHandler serves /unsubscribe/edit-notifications.
func (h *UserPreferencesHandler) UnsubscribeEditNotificationsPageHandler(w http.ResponseWriter, r *http.Request) {
	h.handleScopedUnsubscribe(w, r, scopedUnsubscribeConfig{
		scope:     engagement.UnsubscribeScopeEditNotifications,
		setPref:   func(uid uint) error { return h.userService.SetNotifyOnEditNotifications(uid, false) },
		noun:      "edit-review emails",
		logSuffix: "edit_notifications",
	})
}

// UnsubscribeSceneDigestPageHandler serves /unsubscribe/scene-digest (PSY-1342).
func (h *UserPreferencesHandler) UnsubscribeSceneDigestPageHandler(w http.ResponseWriter, r *http.Request) {
	h.handleScopedUnsubscribe(w, r, scopedUnsubscribeConfig{
		scope:     engagement.UnsubscribeScopeSceneDigest,
		setPref:   func(uid uint) error { return h.userService.SetNotifyOnSceneDigest(uid, false) },
		noun:      "weekly scene digests",
		logSuffix: "scene_digest",
	})
}

// handleScopedUnsubscribe is the shared GET/POST body for the scoped
// unsubscribe handlers. Mirrors UnsubscribeCollectionDigestPageHandler but
// parameterized by scope + preference setter.
func (h *UserPreferencesHandler) handleScopedUnsubscribe(w http.ResponseWriter, r *http.Request, cfg scopedUnsubscribeConfig) {
	uidStr := r.URL.Query().Get("uid")
	sig := r.URL.Query().Get("sig")

	uid64, err := strconv.ParseUint(uidStr, 10, 64)
	if err != nil || uid64 == 0 || sig == "" {
		writeUnsubscribeError(w, r, http.StatusBadRequest, "Missing or invalid unsubscribe parameters.")
		return
	}
	uid := uint(uid64)

	if !engagement.VerifyScopedUnsubscribeSignature(uid, cfg.scope, sig, h.jwtSecret) {
		writeUnsubscribeError(w, r, http.StatusForbidden, "This unsubscribe link is invalid or has been tampered with.")
		return
	}

	if err := cfg.setPref(uid); err != nil {
		logger.FromContext(r.Context()).Error("unsubscribe_"+cfg.logSuffix+"_failed",
			"error", err.Error(),
			"user_id", uid,
			"method", r.Method,
		)
		writeUnsubscribeError(w, r, http.StatusInternalServerError, "We couldn't process your unsubscribe right now. Please try again, or open your notification settings to disable these emails directly.")
		return
	}

	logger.FromContext(r.Context()).Info("unsubscribe_"+cfg.logSuffix+"_success",
		"user_id", uid,
		"method", r.Method,
	)

	if isOneClickPost(r) {
		writeOneClickUnsubscribed(r.Context(), w)
		return
	}

	writeScopedUnsubscribeConfirmation(r.Context(), w, cfg.noun)
}
