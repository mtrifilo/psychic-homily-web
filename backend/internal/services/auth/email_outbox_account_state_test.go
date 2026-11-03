package auth

import (
	"context"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// closeAccount closes user the way named: "deactivated" turns is_active off
// and leaves deleted_at unset, "soft-deleted" is the account-deletion path.
func (s *AuthEmailOutboxSuite) closeAccount(user *authm.User, how string) {
	s.T().Helper()
	switch how {
	case "deactivated":
		testutil.DeactivateAccount(s.T(), s.db, user.ID)
	case "soft-deleted":
		s.Require().NoError(s.users.SoftDeleteAccount(user.ID, nil))
	default:
		s.T().Fatalf("unknown closure %q", how)
	}
}

func (s *AuthEmailOutboxSuite) assertOnlyRowSkipped(reason string) {
	s.T().Helper()
	row := s.onlyRow()
	s.Equal(authm.AuthEmailStatusSkipped, row.Status, "a skipped row is terminal, not retried")
	s.Require().NotNil(row.LastError)
	s.Equal(reason, *row.LastError)
}

// An account closed between the request and the send gets no sign-in,
// verification or password reset link, and the row is retired with the
// closed-account reason.
func (s *AuthEmailOutboxSuite) TestClosedAccount_GetsNoMagicLinkOrVerification() {
	for _, kind := range []authm.AuthEmailKind{authm.AuthEmailKindMagicLink, authm.AuthEmailKindVerification, authm.AuthEmailKindPasswordReset} {
		for _, how := range []string{"deactivated", "soft-deleted"} {
			s.Run(string(kind)+"/"+how, func() {
				defer s.TearDownTest()
				s.SetupTest()
				user := s.verifiedUser("closed-" + how + "-" + string(kind) + "@example.com")
				if kind == authm.AuthEmailKindVerification {
					s.Require().NoError(s.db.Model(user).Update("email_verified", false).Error)
				}
				s.enqueue(kind, user.ID)
				s.closeAccount(user, how)

				s.outbox.RunNow(context.Background())
				s.makeDue()
				s.outbox.RunNow(context.Background())

				s.Empty(s.sender.recorded(), "a closed account is sent nothing")
				s.assertOnlyRowSkipped(authEmailSkipClosedAccount)
				s.Equal(1, s.onlyRow().Attempts, "the skip spends the one claim and is never claimed again")
			})
		}
	}
}

// Recovery reopens a closed account, so a soft-deleted account inside its
// window still gets its recovery link.
func (s *AuthEmailOutboxSuite) TestClosedAccount_RecoverableSoftDeletedStillGetsRecovery() {
	user := s.softDeletedUser("recoverable@example.com")
	s.enqueue(authm.AuthEmailKindAccountRecovery, user.ID)

	s.outbox.RunNow(context.Background())

	sends := s.sender.recorded()
	s.Require().Len(sends, 1)
	s.Equal(authm.AuthEmailKindAccountRecovery, sends[0].kind)
	s.Equal("recoverable@example.com", sends[0].to)
	s.Equal(authm.AuthEmailStatusSent, s.onlyRow().Status)
}

// A deactivated account that was never deleted has no recovery window, so it
// gets no recovery link: the recovery handlers refuse it too
// (IsAccountRecoverable needs deleted_at).
func (s *AuthEmailOutboxSuite) TestClosedAccount_DeactivatedGetsNoRecovery() {
	user := s.softDeletedUser("deactivated-only@example.com")
	s.enqueue(authm.AuthEmailKindAccountRecovery, user.ID)
	s.Require().NoError(s.db.Exec("UPDATE users SET deleted_at = NULL WHERE id = ?", user.ID).Error)

	s.outbox.RunNow(context.Background())

	s.Empty(s.sender.recorded())
	s.assertOnlyRowSkipped(authEmailSkipIneligible)
}
