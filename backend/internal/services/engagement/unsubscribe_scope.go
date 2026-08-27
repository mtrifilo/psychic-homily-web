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

	// UnsubscribeScopeArtistShowAlerts opts a recipient out of the emails sent
	// when an artist they follow announces a show (PSY-1896).
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

	// UnsubscribeScopeArtistReleaseAlerts opts a recipient out of the weekly
	// roundup of new releases from artists they follow (PSY-1897).
	//
	// Its OWN scope, where the venue show digest deliberately REUSED
	// UnsubscribeScopeArtistShowAlerts. Both decisions follow the same rule —
	// one mutation, one name — and land on opposite answers because the mutations
	// differ. Venue and artist show alerts share the single `shows` key in
	// alert_defaults, so one setter genuinely stops both and a second URL would
	// have been two names for one action. Releases have their own `releases` key
	// and their own setter, so reusing the show scope would silence the reader's
	// SHOW alerts while their release emails kept arriving — an unsubscribe that
	// unsubscribes the wrong stream, which is strictly worse than no link.
	//
	// Signed on the USER for the same reason its sibling is: "unsubscribe" on a
	// mailbox provider's native button means stop this stream, not mute one band.
	// The IN-APP roundup is untouched — an email opt-out is not a request to stop
	// being notified in the product.
	UnsubscribeScopeArtistReleaseAlerts = "artist-release-alerts"
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
