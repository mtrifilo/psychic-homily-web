package auth

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
)

// RevokeAllSessions refuses, from now on, every session user was issued
// before now: it writes users.sessions_valid_after through db (a transaction,
// when the revocation is one step of several) and sets the same value on
// user. Session validation (JWTService.userFromSessionClaims) refuses a token
// issued before it. A session issued after it, through
// JWTService.createTokenAfterRevocation, is honored.
//
// It revokes session JWTs only, which is every credential minted from a
// sign-in: browser sessions, OAuth sign-ins and CLI tokens. API tokens and
// calendar-feed tokens are separate stored credentials and are not touched.
func RevokeAllSessions(db *gorm.DB, user *authm.User, now time.Time) error {
	validAfter := sessionsValidAfterFor(now)
	res := db.Model(&authm.User{}).Where("id = ?", user.ID).Update("sessions_valid_after", validAfter)
	if res.Error != nil {
		return fmt.Errorf("failed to revoke sessions: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("failed to revoke sessions: account %d not found", user.ID)
	}
	user.SessionsValidAfter = &validAfter
	return nil
}

// sessionsValidAfterFor is the revocation instant written for a revocation at
// now: the first whole second after it. Session tokens carry their issued-at
// in whole seconds, so a token issued in now's own second, before the
// revocation, reads as issued at that second's start; rounding the instant
// up refuses it as well. The session the revoking flow issues afterwards is
// stamped with this instant, at most a second ahead of the clock.
func sessionsValidAfterFor(now time.Time) time.Time {
	return now.UTC().Truncate(time.Second).Add(time.Second)
}
