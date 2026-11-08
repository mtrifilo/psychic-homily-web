package auth

import (
	"time"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	engagementsvc "psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/shared"
)

// RevokeAllSessions is shared.RevokeAllSessions, which documents the
// revocation contract: it refuses, from now on, every session user holds,
// writing users.sessions_valid_after through db and setting the same value on
// user. It only forwards, so every session revocation, whichever name its
// caller uses, behaves the same; behavior belongs in shared.RevokeAllSessions.
// It leaves the calendar-feed token in place: a revocation meant to shut out
// whoever else holds the account calls revokeSessionsAndFeedToken.
func RevokeAllSessions(db *gorm.DB, user *authm.User, now time.Time) error {
	return shared.RevokeAllSessions(db, user, now)
}

// revokeSessionsAndFeedToken refuses every session account holds
// (RevokeAllSessions) and deletes its calendar-feed token, both through tx,
// for a revocation meant to shut out whoever else holds the account: the
// feed URL is a bearer credential a session can carry away and no session
// revocation reaches. API tokens, passkeys and provider identities stay. The
// mailbox-proof reclaim deletes the token through evictOtherCredentials
// instead, with the provider identities it removes.
//
// account's users row is locked before the delete: the caller holds it FOR
// UPDATE, and RevokeAllSessions' write to it would take the lock otherwise. A
// feed-token mint judges its session under a lock on that row before it
// writes (engagement CalendarService.CreateToken), so a mint from a session
// this revokes either commits first, and its token is deleted here, or waits
// and is refused.
func revokeSessionsAndFeedToken(tx *gorm.DB, account *authm.User, now time.Time) error {
	if err := RevokeAllSessions(tx, account, now); err != nil {
		return err
	}
	_, err := engagementsvc.DeleteCalendarTokensTx(tx, account.ID)
	return err
}
