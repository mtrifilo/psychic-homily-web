package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// softDeletePastRecoveryWindow soft-deletes the account with a deleted_at a day
// older than the recovery window, so it is deleted and no longer recoverable.
func (s *AppleAuthIntegrationTestSuite) softDeletePastRecoveryWindow(userID uint) {
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", userID).Updates(map[string]any{
		"is_active":  false,
		"deleted_at": time.Now().Add(-shared.AccountRecoveryGracePeriod - 24*time.Hour),
	}).Error)
}

// A returning Apple identity resolves to its account before anything else is
// consulted, so the account's state is the only thing between that identity
// and a session. A deleted account that account recovery can restore is
// refused with ACCOUNT_RECOVERABLE; every other closed account with
// ACCOUNT_INACTIVE.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_AccountState() {
	softDelete := func(userID uint) {
		s.Require().NoError(usersvc.NewUserService(s.db).SoftDeleteAccount(userID, nil))
	}
	states := []struct {
		name    string
		noEmail bool
		apply   func(userID uint)
		refusal *apperrors.AuthError
	}{
		{name: "active", apply: func(uint) {}},
		{name: "deactivated", apply: func(userID uint) {
			s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", userID).Update("is_active", false).Error)
		}, refusal: apperrors.ErrAccountInactive()},
		{name: "soft-deleted", apply: softDelete, refusal: apperrors.ErrAccountRecoverable()},
		{name: "soft-deleted-past-window", apply: s.softDeletePastRecoveryWindow, refusal: apperrors.ErrAccountInactive()},
		{name: "soft-deleted-no-email", noEmail: true, apply: softDelete, refusal: apperrors.ErrAccountInactive()},
	}

	for _, state := range states {
		s.Run(state.name, func() {
			svc := s.newService()
			claims := &contracts.AppleIdentityTokenClaims{
				Email:            "apple-" + state.name + "@example.com",
				EmailVerified:    true,
				RegisteredClaims: jwt.RegisteredClaims{Subject: "apple-sub-" + state.name},
			}
			if state.noEmail {
				claims.Email = ""
				claims.EmailVerified = nil
			}
			created, err := svc.FindOrCreateAppleUser(claims, "", "", signupconsent.CurrentOAuthSignupConsent())
			s.Require().NoError(err)
			if state.noEmail {
				s.Require().Nil(created.Email)
			}
			state.apply(created.ID)

			// No consent: a returning identity never needs one.
			user, err := svc.FindOrCreateAppleUser(claims, "", "", nil)

			if state.refusal == nil {
				s.Require().NoError(err)
				s.Require().NotNil(user)
				s.Equal(created.ID, user.ID)
				return
			}

			s.Require().Error(err)
			s.Nil(user)
			var authErr *apperrors.AuthError
			s.Require().ErrorAs(err, &authErr)
			s.Equal(state.refusal.Code, authErr.Code)
			s.Equal(state.refusal.UserMessage(), authErr.UserMessage())

			// The refusal creates nothing: the identity still names the one account.
			var identities int64
			s.Require().NoError(s.db.Model(&authm.OAuthAccount{}).
				Where("provider = ? AND provider_user_id = ?", "apple", claims.Subject).
				Count(&identities).Error)
			s.Equal(int64(1), identities)
		})
	}
}

// An Apple identity attached to no account, whose address belongs to a deleted
// account, is sent to recovery only when Apple vouches for the address, the
// account's own address is verified and the account is still recoverable;
// otherwise it gets Apple's ordinary address refusal. None of them attaches the
// identity or creates an account.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_AddressOfDeletedAccount() {
	cases := []struct {
		name            string
		appleVouches    bool
		accountVerified bool
		pastWindow      bool
		recoverable     bool
	}{
		{name: "vouched-verified", appleVouches: true, accountVerified: true, recoverable: true},
		{name: "vouched-unverified-account", appleVouches: true, accountVerified: false},
		{name: "unvouched-verified", appleVouches: false, accountVerified: true},
		{name: "vouched-verified-past-window", appleVouches: true, accountVerified: true, pastWindow: true},
	}

	for _, tc := range cases {
		s.Run(tc.name, func() {
			address := "apple-address-" + tc.name + "@example.com"
			existing := &authm.User{Email: &address, IsActive: true, EmailVerified: tc.accountVerified}
			s.Require().NoError(s.db.Create(existing).Error)
			if tc.pastWindow {
				s.softDeletePastRecoveryWindow(existing.ID)
			} else {
				s.Require().NoError(usersvc.NewUserService(s.db).SoftDeleteAccount(existing.ID, nil))
			}

			claims := &contracts.AppleIdentityTokenClaims{
				Email:            address,
				EmailVerified:    tc.appleVouches,
				RegisteredClaims: jwt.RegisteredClaims{Subject: "apple-sub-address-" + tc.name},
			}
			user, err := s.newService().FindOrCreateAppleUser(claims, "", "", signupconsent.CurrentOAuthSignupConsent())

			s.Require().Error(err)
			s.Nil(user)
			var authErr *apperrors.AuthError
			s.Require().ErrorAs(err, &authErr)
			want := apperrors.ErrAppleSignInRefused(address)
			if tc.recoverable {
				want = apperrors.ErrAccountRecoverable()
			}
			s.Equal(want.Code, authErr.Code)
			s.Equal(want.UserMessage(), authErr.UserMessage())

			var identities int64
			s.Require().NoError(s.db.Model(&authm.OAuthAccount{}).
				Where("user_id = ? OR provider_user_id = ?", existing.ID, claims.Subject).
				Count(&identities).Error)
			s.Zero(identities, "a refusal attaches the identity to no account")

			var accounts int64
			s.Require().NoError(s.db.Model(&authm.User{}).
				Where(authm.EmailIdentityWhere, address).Count(&accounts).Error)
			s.Equal(int64(1), accounts, "a refusal creates no account")
		})
	}
}
