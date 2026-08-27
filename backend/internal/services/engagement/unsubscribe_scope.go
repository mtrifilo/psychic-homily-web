package engagement

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Unsubscribe scopes used to bind an HMAC signature to one notification
// category, so a link minted for one email can't be replayed against another.
// These string values are part of the signed payload — changing one
// invalidates every URL already in recipients' inboxes for that scope.
const (
	UnsubscribeScopeTierNotifications = "tier-notifications"
	UnsubscribeScopeEditNotifications = "edit-notifications"
	UnsubscribeScopeShowReminders     = "show-reminders"
	UnsubscribeScopeMention           = "mention"
	UnsubscribeScopeCollectionDigest  = "collection-digest"
	UnsubscribeScopeSceneDigest       = "scene-digest"

	// UnsubscribeScopeArtistShowAlerts opts a recipient out of NEW-SHOW ALERT
	// EMAILS across every follow type that sends them: artists (PSY-1896),
	// venues (PSY-1895) and scenes (PSY-1926).
	//
	// The name is narrower than the scope, and the value is frozen: it is part
	// of the signed payload, so re-spelling it would 404 every link already
	// sitting in a recipient's inbox. Read it as "show alerts".
	//
	// One scope for all three because they are one MUTATION, not merely one
	// stream: alert_defaults carries a single `shows` key covering all of them,
	// and UserService.UnsubscribeArtistShowAlertEmails clears that key and
	// sweeps the per-follow overrides of all three follow types. A second scope
	// would mint a second URL performing an identical write, which is not extra
	// precision: it is two names for one action, and the day one of them drifts
	// is the day an unsubscribe stops unsubscribing.
	//
	// Signed on the USER, not on one follow, and that is deliberate. A per-follow
	// link would be the tighter mirror of the per-filter scheme, but "unsubscribe"
	// on a mailbox provider's native button means "stop this stream"; a link that
	// silenced one band out of thirty would leave the recipient still receiving
	// the mail they just refused, and the next lever they reach for is Report
	// Spam. It stops the whole artist-show-alert email stream, and leaves the
	// IN-APP alerts alone: an email opt-out is not a request to stop being
	// notified in the product.
	UnsubscribeScopeArtistShowAlerts = "artist-show-alerts"
)

// ComputeScopedUnsubscribeSignature computes HMAC-SHA256 over
// "unsubscribe:<scope>:<userID>". The scope discriminates inbox URLs across
// notification types so a link minted for one category can't be replayed
// against another.
func ComputeScopedUnsubscribeSignature(userID uint, scope, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	// hash.Hash.Write never returns an error; the drop is intentional.
	_, _ = fmt.Fprintf(mac, "unsubscribe:%s:%d", scope, userID)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyScopedUnsubscribeSignature constant-time-compares a signature against
// the expected value for (userID, scope). Constant-time via hmac.Equal.
func VerifyScopedUnsubscribeSignature(userID uint, scope, signature, secret string) bool {
	expected := ComputeScopedUnsubscribeSignature(userID, scope, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// GenerateScopedUnsubscribeURL mints the HMAC-signed one-click unsubscribe URL
// for a notification category. `baseURL` must be the public backend URL (NOT
// the frontend) — the chi route at /unsubscribe/<scope> serves an HTML
// confirmation page on GET and accepts an RFC 8058 one-click POST.
func GenerateScopedUnsubscribeURL(baseURL string, userID uint, scope, secret string) string {
	sig := ComputeScopedUnsubscribeSignature(userID, scope, secret)
	return fmt.Sprintf("%s/unsubscribe/%s?uid=%d&sig=%s", baseURL, scope, userID, sig)
}
