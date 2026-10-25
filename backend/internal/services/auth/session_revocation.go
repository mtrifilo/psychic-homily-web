package auth

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
)

// RevokeAllSessions refuses, from now on, every session user holds: it writes
// users.sessions_valid_after through db (a transaction, when the revocation
// is one step of several) and sets the same value on user.
//
// The value is a whole second, and always later than the one it replaces, so
// two revocations never share an instant. A session token carries the
// instant its mint read (JWTService.RenewSessionToken), and session
// validation (JWTService.userFromSessionClaims) honors only a token that
// names the current one. Sessions minted for the account as it stands after
// this commits are honored; every earlier one is refused, including one
// minted later from a row read before the commit.
//
// It revokes session JWTs only, which is every credential minted from a
// sign-in: browser sessions, OAuth sign-ins and CLI tokens. API tokens and
// calendar-feed tokens are separate stored credentials and are not touched.
func RevokeAllSessions(db *gorm.DB, user *authm.User, now time.Time) error {
	var validAfter []time.Time
	res := db.Raw(
		`UPDATE users
		    SET sessions_valid_after = GREATEST(?::timestamptz, sessions_valid_after + INTERVAL '1 second'),
		        updated_at = NOW()
		  WHERE id = ?
		RETURNING sessions_valid_after`,
		now.UTC().Truncate(time.Second), user.ID,
	).Scan(&validAfter)
	if res.Error != nil {
		return fmt.Errorf("failed to revoke sessions: %w", res.Error)
	}
	if len(validAfter) != 1 {
		return fmt.Errorf("failed to revoke sessions: account %d not found", user.ID)
	}
	revokedAt := validAfter[0].UTC()
	user.SessionsValidAfter = &revokedAt
	return nil
}
