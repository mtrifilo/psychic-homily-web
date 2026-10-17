package auth

import (
	"context"

	"github.com/markbates/goth"

	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// A deleted account inside its recovery window is refused with
// ACCOUNT_RECOVERABLE, which the auth page answers with a link to account
// recovery, on password sign-in and on a Google sign-in. Outside that state the
// refusals are the ones each path already had.

// Password sign-in names the account's state only after the password is
// proved, and mints no session for a closed account.
func (s *AuthHandlerIntegrationSuite) TestLogin_AccountState() {
	const password = "strong-password-123!"
	states := []struct {
		name  string
		apply func(userID uint)
		want  *autherrors.AuthError
	}{
		{name: "deactivated", apply: s.deactivateUser, want: autherrors.ErrAccountInactive()},
		{name: "soft-deleted", apply: s.softDeleteUser, want: autherrors.ErrAccountRecoverable()},
		{name: "soft-deleted-past-window", apply: s.softDeleteUserExpired, want: autherrors.ErrAccountInactive()},
	}

	h := s.newAuthHandler(false)
	for _, state := range states {
		s.Run(state.name, func() {
			address := "login-state-" + state.name + "@test.com"
			user := s.createUserWithPassword(address, password)
			state.apply(user.ID)

			input := &LoginRequest{}
			input.Body.Email = address
			input.Body.Password = password
			resp, err := h.LoginHandler(context.Background(), input)
			s.Require().NoError(err, "a refusal answers in the HTTP 200 envelope")
			s.False(resp.Body.Success)
			s.Equal(state.want.Code, resp.Body.ErrorCode)
			s.Equal(state.want.UserMessage(), resp.Body.Message)
			s.Empty(resp.Body.Token)
			s.Nil(resp.Body.User)
			s.Empty(resp.SetCookie.Value)

			input.Body.Password = "wrong-password-999!"
			resp, err = h.LoginHandler(context.Background(), input)
			s.Require().NoError(err)
			s.False(resp.Body.Success)
			s.Equal(autherrors.CodeInvalidCredentials, resp.Body.ErrorCode, "a wrong password learns nothing of the account's state")
		})
	}
}

// A Google identity attached to no account, whose address belongs to a
// recoverable deleted account, is sent to recovery only when Google vouches for
// the address and the account's address is verified; otherwise it gets the
// ordinary link refusal. None of them attaches the identity or creates an
// account, and the address stays out of the redirect.
func (s *OAuthHandlerIntegrationSuite) TestGoogleCallback_AddressOfRecoverableAccount() {
	cases := []struct {
		name            string
		vouched         bool
		accountVerified bool
	}{
		{name: "vouched", vouched: true, accountVerified: true},
		{name: "unvouched", vouched: false, accountVerified: true},
		{name: "vouched-unverified-account", vouched: true, accountVerified: false},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			address := "callback-recoverable-" + tc.name + "@test.com"
			subject := "google-recoverable-" + tc.name
			existing := &authm.User{Email: strPtr(address), IsActive: true, EmailVerified: tc.accountVerified}
			s.Require().NoError(s.deps.DB.Create(existing).Error)
			s.Require().NoError(s.deps.UserService.SoftDeleteAccount(existing.ID, nil))

			handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
				Provider: "google",
				UserID:   subject,
				Email:    address,
				RawData:  map[string]any{"verified_email": tc.vouched},
			}})
			w, req := oauthCallbackRequest("google")
			s.addSignupConsentCookie(req)
			handler.OAuthCallbackHTTPHandler(w, req)

			if tc.vouched && tc.accountVerified {
				s.assertAuthPageRefusal(w, autherrors.ErrAccountRecoverable())
				s.NotContains(w.Header().Get("Location"), address)
			} else {
				s.assertSignInRefusal(w, address)
			}

			var identityRows int64
			s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
				Where("user_id = ? OR provider_user_id = ?", existing.ID, subject).Count(&identityRows).Error)
			s.Equal(int64(0), identityRows, "a refusal attaches the identity to no account")

			var accounts int64
			s.Require().NoError(s.deps.DB.Model(&authm.User{}).
				Where(authm.EmailIdentityWhere, address).Count(&accounts).Error)
			s.Equal(int64(1), accounts, "a refusal creates no account")
		})
	}
}
