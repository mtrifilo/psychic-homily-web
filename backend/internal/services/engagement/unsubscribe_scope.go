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
)

// ComputeScopedUnsubscribeSignature computes HMAC-SHA256 over
// "unsubscribe:<scope>:<userID>" — the same payload shape as the older
// per-domain helpers (ComputeUnsubscribeSignature for show-reminders,
// ComputeCollectionDigestUnsubscribeSignature for collection-digest), but with
// the scope passed in rather than baked into the function. Used for the
// tier-change and edit-review categories.
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
