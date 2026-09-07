package user

import (
	"github.com/markbates/goth"

	"psychic-homily-backend/internal/services/contracts"
)

// emailVerifiedRawDataKeys are the keys a provider's verification flag can
// arrive under in goth.User.RawData, most likely spelling first. Google's
// oauth2/v2/userinfo response, the endpoint goth's google provider reads,
// names it verified_email; the OIDC userinfo shape names it email_verified.
// Both are read so a provider that moves between the two shapes does not
// silently stop being linkable.
var emailVerifiedRawDataKeys = []string{"verified_email", "email_verified"}

// providerAssertsEmailVerified reports whether the OAuth provider asserted
// that it verified gothUser.Email. Absence of a signal is not verification:
// an address the provider will not vouch for is a caller-supplied string.
func providerAssertsEmailVerified(gothUser goth.User) bool {
	for _, key := range emailVerifiedRawDataKeys {
		if verified, present := contracts.ParseEmailVerifiedClaim(gothUser.RawData[key]); present {
			return verified
		}
	}
	return false
}
