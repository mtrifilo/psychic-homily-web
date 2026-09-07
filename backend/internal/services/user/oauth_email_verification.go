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
//
// GitHub, registered in internal/auth/goth.go whenever GITHUB_CLIENT_ID is
// set, always lands on the false arm. goth's github provider fills RawData
// from GET /user, which carries no verification field under either key. Its
// one Primary-and-Verified filter sits in a private-email fallback that runs
// only when the requested scopes include user or user:email, and that provider
// is registered with no scopes at all. A GitHub sign-in therefore cannot link
// to an account that already holds the address.
func providerAssertsEmailVerified(gothUser goth.User) bool {
	for _, key := range emailVerifiedRawDataKeys {
		if verified, present := contracts.ParseEmailVerifiedClaim(gothUser.RawData[key]); present {
			return verified
		}
	}
	return false
}
