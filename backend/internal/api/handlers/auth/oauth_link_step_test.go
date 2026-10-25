package auth

import (
	"errors"
	"reflect"
	"testing"

	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// Every request that reads the pending link declares it under both names a
// deployment may set it under, because huma binds no parameter declared on an
// embedded struct.
func TestLinkStepRequestsDeclareTheirCookies(t *testing.T) {
	cookieTags := func(v any) []string {
		var tags []string
		typ := reflect.TypeOf(v)
		for i := 0; i < typ.NumField(); i++ {
			if tag := typ.Field(i).Tag.Get("cookie"); tag != "" {
				tags = append(tags, tag)
			}
		}
		return tags
	}
	want := []string{oauthPendingLinkCookie.setName(false), oauthPendingLinkCookie.setName(true)}
	for name, request := range map[string]any{
		"PendingOAuthLinkRequest": PendingOAuthLinkRequest{},
		"OAuthLinkByEmailRequest": OAuthLinkByEmailRequest{},
		"LoginRequest":            LoginRequest{},
	} {
		if got := cookieTags(request); !reflect.DeepEqual(got, want) {
			t.Errorf("%s cookies = %v, want %v", name, got, want)
		}
	}
}

// With Secure cookies only the "__Host-" name counts: a bare one could have
// been planted by a sibling subdomain.
func TestPendingLinkToken_SecureReadsOnlyThePrefixedName(t *testing.T) {
	if got := pendingLinkToken("bare", "host", true); got != "host" {
		t.Errorf("secure = %q, want the prefixed one", got)
	}
	if got := pendingLinkToken("bare", "", true); got != "" {
		t.Errorf("secure with only a bare cookie = %q, want none", got)
	}
	if got := pendingLinkToken("bare", "host", false); got != "bare" {
		t.Errorf("insecure = %q, want the bare one", got)
	}
}

// The collision state serves Google only, and only for the refusal that
// carries a pending link.
func TestLinkStepRequired(t *testing.T) {
	required := &contracts.OAuthLinkRequiredError{UserID: 3, Refusal: autherrors.ErrOAuthLinkRefused("a@example.com")}
	if got, ok := linkStepRequired("google", required); !ok || got != required {
		t.Error("google's link-required refusal does not land on the collision state")
	}
	if _, ok := linkStepRequired("github", required); ok {
		t.Error("github lands on the Google collision state")
	}
	if _, ok := linkStepRequired("google", autherrors.ErrOAuthLinkRefused("a@example.com")); ok {
		t.Error("the plain refusal lands on the collision state")
	}
	var authErr *autherrors.AuthError
	if !errors.As(required, &authErr) || authErr.Code != autherrors.CodeOAuthLinkRefused {
		t.Error("the link-required refusal does not unwrap to OAUTH_LINK_REFUSED for callers that offer no proof")
	}
}
