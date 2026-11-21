package engagement

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
)

// calendarLinkMintsPerHour is how many calendar links one account may make or
// replace in one clock hour. A burst across an hour boundary can reach twice
// this; the cap bounds how fast an account can add retired-link rows, not how
// often a person may change their mind.
const calendarLinkMintsPerHour = 10

// countCalendarLinkMintTx counts one calendar link made for userID at the
// instant at through tx, which the caller commits, or refuses with
// *apperrors.CalendarLinkRateLimitedError when the account has already made
// calendarLinkMintsPerHour in that clock hour. A refusal writes nothing.
//
// The check and the count are one statement, so they cannot race: a
// concurrent mint for the same account waits on this row until tx ends, and
// then judges the count tx left, or the one before it if tx rolled back. A
// mint that rolls back after its count therefore uses no slot.
//
// A later hour starts the count again. An earlier one, read from a server
// clock that runs behind the one that last counted, counts against the later
// hour already stored, so clocks that disagree across an hour boundary cannot
// restart the count by taking turns.
func countCalendarLinkMintTx(tx *gorm.DB, userID uint, at time.Time) error {
	// The clock hour at falls in is the only time the row stores.
	hour := at.UTC().Truncate(time.Hour)
	result := tx.Exec(`
		INSERT INTO calendar_link_mint_counts AS c (user_id, mint_hour, mint_count)
		VALUES (?, ?, 1)
		ON CONFLICT (user_id) DO UPDATE SET
			mint_count = CASE WHEN EXCLUDED.mint_hour > c.mint_hour THEN 1 ELSE c.mint_count + 1 END,
			mint_hour = GREATEST(c.mint_hour, EXCLUDED.mint_hour)
		WHERE EXCLUDED.mint_hour > c.mint_hour OR c.mint_count < ?`,
		userID, hour, calendarLinkMintsPerHour)
	if result.Error != nil {
		return fmt.Errorf("failed to count calendar link mint: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}

	// The refused upsert left the row locked, so this reads the hour it judged.
	var counted struct{ MintHour time.Time }
	if err := tx.Raw(`SELECT mint_hour FROM calendar_link_mint_counts WHERE user_id = ?`, userID).
		Scan(&counted).Error; err != nil {
		return fmt.Errorf("failed to read calendar link mint count: %w", err)
	}
	return &apperrors.CalendarLinkRateLimitedError{
		Limit:      calendarLinkMintsPerHour,
		RetryAfter: counted.MintHour.Add(time.Hour).Sub(at),
	}
}
