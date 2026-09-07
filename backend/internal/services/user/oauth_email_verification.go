package user

import "github.com/markbates/goth"

// emailVerifiedRawDataKeys are the keys the configured goth providers use to
// assert that they verified the address they returned, most likely spelling
// first. Google's oauth2/v2/userinfo response (the endpoint goth's google
// provider reads) names it verified_email; the OIDC userinfo shape names it
// email_verified. goth copies the whole provider response into
// goth.User.RawData, so whichever the provider sent is readable there.
var emailVerifiedRawDataKeys = []string{"verified_email", "email_verified"}

// ProviderAssertsEmailVerified reports whether the OAuth provider asserted
// that it verified gothUser.Email.
//
// Absence of a signal is not verification. An address the provider will not
// vouch for is a caller-supplied string, and that string is the only thing
// tying a provider identity to an account that already exists.
func ProviderAssertsEmailVerified(gothUser goth.User) bool {
	for _, key := range emailVerifiedRawDataKeys {
		if verified, present := verificationFlag(gothUser.RawData[key]); present {
			return verified
		}
	}
	return false
}

// verificationFlag reads one raw claim value. Providers send the flag as a
// JSON bool or, like Apple's identity token, as the string "true"/"false".
// present is false both for a missing key and for any other shape, so an
// unreadable value is never mistaken for an assertion in either direction.
func verificationFlag(raw any) (verified bool, present bool) {
	switch v := raw.(type) {
	case bool:
		return v, true
	case string:
		switch v {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}
