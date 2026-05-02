package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/engagement"
)

// UnsubscribeCollectionDigestPageHandler is a chi-level (NOT Huma) handler
// that serves the unsubscribe URL minted by the collection digest email.
//
// PSY-350 hardening — this is split out from the Huma user_preferences
// handlers because we need:
//
//   - GET to return text/html (Huma assumes JSON), so a recipient who clicks
//     the visible link in the email gets a confirmation page rather than a
//     raw JSON blob.
//   - POST to accept an RFC 8058 / RFC 2369 one-click body
//     (`Content-Type: application/x-www-form-urlencoded`,
//     body: `List-Unsubscribe=One-Click`). This is what Gmail and Yahoo
//     send when a recipient clicks the native "Unsubscribe" button next to
//     the sender name in the inbox.
//
// Both methods read uid+sig from the query string (not the body) — that's
// the only place they're available consistently across the two flows. The
// HMAC validation logic is shared with the email-side helper in the
// engagement package; the signature format itself is unchanged.
//
// Public endpoint: no auth required, the HMAC signature is the auth.
func (h *UserPreferencesHandler) UnsubscribeCollectionDigestPageHandler(w http.ResponseWriter, r *http.Request) {
	uidStr := r.URL.Query().Get("uid")
	sig := r.URL.Query().Get("sig")

	uid64, err := strconv.ParseUint(uidStr, 10, 64)
	if err != nil || uid64 == 0 || sig == "" {
		writeUnsubscribeError(w, r, http.StatusBadRequest, "Missing or invalid unsubscribe parameters.")
		return
	}
	uid := uint(uid64)

	if !engagement.VerifyCollectionDigestUnsubscribeSignature(uid, sig, h.jwtSecret) {
		writeUnsubscribeError(w, r, http.StatusForbidden, "This unsubscribe link is invalid or has been tampered with.")
		return
	}

	if err := h.userService.SetNotifyOnCollectionDigest(uid, false); err != nil {
		logger.FromContext(r.Context()).Error("unsubscribe_collection_digest_failed",
			"error", err.Error(),
			"user_id", uid,
			"method", r.Method,
		)
		writeUnsubscribeError(w, r, http.StatusInternalServerError, "We couldn't process your unsubscribe right now. Please try again, or open your notification settings to disable digest emails directly.")
		return
	}

	logger.FromContext(r.Context()).Info("unsubscribe_collection_digest_success",
		"user_id", uid,
		"method", r.Method,
	)

	if isOneClickPost(r) {
		// RFC 8058: respond with 200 and a minimal body. The body is not
		// strictly required, but Gmail's documented examples include one
		// and including it makes manual debugging easier.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"unsubscribed": true})
		return
	}

	// GET (manual click) — render an HTML confirmation page.
	writeUnsubscribeConfirmation(w, r)
}

// isOneClickPost returns true when the request is a POST with body
// `List-Unsubscribe=One-Click` (RFC 8058 §3.1). We accept any POST as
// "machine, return JSON" since mailbox providers send slightly different
// request shapes; the HMAC in the URL is the actual authentication.
func isOneClickPost(r *http.Request) bool {
	return r.Method == http.MethodPost
}

// writeUnsubscribeConfirmation renders the GET success page. Self-contained
// HTML — no shared template engine — to keep this surface tiny and avoid a
// new dependency just for one page.
func writeUnsubscribeConfirmation(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, unsubscribeConfirmationHTML)
}

// writeUnsubscribeError renders an error page on GET, or a JSON body on POST,
// so machine clients (mailbox providers) get a parseable response.
func writeUnsubscribeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if isOneClickPost(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"unsubscribed": false,
			"error":        message,
		})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	page := strings.ReplaceAll(unsubscribeErrorTemplate, "{{message}}", html.EscapeString(message))
	fmt.Fprint(w, page)
}

// unsubscribeConfirmationHTML is the success page rendered on GET. Keep
// inline-styled (no external CSS) so it works in any mail client preview
// and across our deploy targets without extra asset wiring.
const unsubscribeConfirmationHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Unsubscribed | Psychic Homily</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; margin: 0; padding: 32px 16px; background: #fafafa; color: #1a1a1a;">
    <div style="max-width: 480px; margin: 48px auto; background: #ffffff; border: 1px solid #e5e5e5; border-radius: 12px; padding: 32px; text-align: center;">
        <div style="width: 56px; height: 56px; margin: 0 auto 16px; border-radius: 50%; background: #d1fae5; display: flex; align-items: center; justify-content: center; font-size: 28px;">&#10003;</div>
        <h1 style="margin: 0 0 12px; font-size: 22px; font-weight: 600;">You&rsquo;re unsubscribed</h1>
        <p style="margin: 0 0 24px; color: #555; line-height: 1.5;">
            You&rsquo;ve been unsubscribed from collection digest emails.
            You can re-enable this anytime in your notification settings.
        </p>
        <p style="margin: 0;">
            <a href="/settings" style="display: inline-block; padding: 10px 20px; border-radius: 8px; background: #f97316; color: #ffffff; text-decoration: none; font-weight: 600;">Go to notification settings</a>
        </p>
    </div>
</body>
</html>`

// unsubscribeErrorTemplate is the error page rendered on GET. {{message}}
// is replaced with an HTML-escaped human-readable message.
const unsubscribeErrorTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Unsubscribe failed | Psychic Homily</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; margin: 0; padding: 32px 16px; background: #fafafa; color: #1a1a1a;">
    <div style="max-width: 480px; margin: 48px auto; background: #ffffff; border: 1px solid #e5e5e5; border-radius: 12px; padding: 32px; text-align: center;">
        <div style="width: 56px; height: 56px; margin: 0 auto 16px; border-radius: 50%; background: #fee2e2; display: flex; align-items: center; justify-content: center; font-size: 28px;">!</div>
        <h1 style="margin: 0 0 12px; font-size: 22px; font-weight: 600;">Unsubscribe failed</h1>
        <p style="margin: 0 0 24px; color: #555; line-height: 1.5;">{{message}}</p>
        <p style="margin: 0;">
            <a href="/settings" style="display: inline-block; padding: 10px 20px; border-radius: 8px; background: #f97316; color: #ffffff; text-decoration: none; font-weight: 600;">Open notification settings</a>
        </p>
    </div>
</body>
</html>`
