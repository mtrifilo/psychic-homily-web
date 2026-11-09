package engagement

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// feedReadRecordInterval is the least time between two recorded reads of one
// token by one client family. Calendar clients poll every few minutes to
// every few hours; recording each poll would write on every one of them for
// no gain in what the status can say.
const feedReadRecordInterval = 10 * time.Minute

// classifyCalendarFeedClient names the calendar client family a feed request
// came from, judged by its User-Agent alone. ok is false for a browser (a
// Mozilla/ User-Agent carrying none of the calendar markers): that is a person
// opening the link in a tab, not a subscribed calendar, so it is not a read.
// Anything else that matches no known family, an empty User-Agent included,
// is "other".
//
// The markers are checked before the browser test so a calendar client that
// also says Mozilla/ is still classified by its marker.
func classifyCalendarFeedClient(userAgent string) (client engagementm.CalendarFeedClient, ok bool) {
	switch {
	case strings.Contains(userAgent, "Google-Calendar-Importer"):
		return engagementm.CalendarFeedClientGoogle, true
	case (strings.HasPrefix(userAgent, "iOS/") || strings.HasPrefix(userAgent, "iPadOS/")) &&
		strings.Contains(userAgent, "dataaccessd"):
		return engagementm.CalendarFeedClientAppleIOS, true
	case strings.HasPrefix(userAgent, "macOS/") && strings.Contains(userAgent, "dataaccessd"),
		strings.Contains(userAgent, "CalendarAgent"):
		return engagementm.CalendarFeedClientAppleMac, true
	case strings.HasPrefix(userAgent, "Mozilla/"):
		return "", false
	default:
		return engagementm.CalendarFeedClientOther, true
	}
}

// RecordFeedRead notes that the holder of plainToken fetched the saved-shows
// feed with userAgent (contracts.CalendarServiceInterface).
func (s *CalendarService) RecordFeedRead(ctx context.Context, plainToken, userAgent string) error {
	_, err := s.recordFeedReadAt(ctx, plainToken, userAgent, time.Now())
	return err
}

// recordFeedReadAt is RecordFeedRead at the instant at, reporting whether it
// wrote a row.
//
// One statement, so it holds no process state and needs no read first: the
// token is resolved by its hash inside the INSERT, so a token deleted or
// replaced since the feed was served matches nothing and nothing is written;
// and the conflict branch updates only when the stored read is at least
// feedReadRecordInterval older than at, so a poll inside the interval is a
// no-op and last_read_at never moves backwards.
func (s *CalendarService) recordFeedReadAt(ctx context.Context, plainToken, userAgent string, at time.Time) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	client, ok := classifyCalendarFeedClient(userAgent)
	if !ok {
		return false, nil
	}

	result := s.db.WithContext(ctx).Exec(`
		INSERT INTO calendar_feed_reads (token_id, client, first_read_at, last_read_at)
		SELECT id, ?::text, ?::timestamptz, ?::timestamptz
		FROM calendar_tokens
		WHERE token_hash = ?
		ON CONFLICT (token_id, client) DO UPDATE SET
			last_read_at = EXCLUDED.last_read_at
		WHERE calendar_feed_reads.last_read_at <= ?::timestamptz`,
		string(client), at, at, hashCalendarToken(plainToken), at.Add(-feedReadRecordInterval))
	if result.Error != nil {
		return false, fmt.Errorf("failed to record calendar feed read: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// feedReads lists the recorded reads of tokenID, most recently read first.
func (s *CalendarService) feedReads(tokenID uint) ([]contracts.CalendarFeedReadResponse, error) {
	var rows []engagementm.CalendarFeedRead
	if err := s.db.Where("token_id = ?", tokenID).
		Order("last_read_at DESC, client").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load calendar feed reads: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	reads := make([]contracts.CalendarFeedReadResponse, len(rows))
	for i, row := range rows {
		reads[i] = contracts.CalendarFeedReadResponse{
			Client:      row.Client,
			FirstReadAt: row.FirstReadAt,
			LastReadAt:  row.LastReadAt,
		}
	}
	return reads, nil
}

// linkResetMarker reads userID's outstanding link-reset marker. Both results
// are nil when there is none, including when the user has no preferences row.
func (s *CalendarService) linkResetMarker(userID uint) (*time.Time, *engagementm.CalendarLinkResetReason, error) {
	var marker struct {
		At     *time.Time `gorm:"column:calendar_link_reset_at"`
		Reason *string    `gorm:"column:calendar_link_reset_reason"`
	}
	if err := s.db.Raw(`
		SELECT calendar_link_reset_at, calendar_link_reset_reason
		FROM user_preferences
		WHERE user_id = ?`, userID).Scan(&marker).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to load calendar link reset: %w", err)
	}
	if marker.At == nil || marker.Reason == nil {
		return nil, nil, nil
	}
	reason := engagementm.CalendarLinkResetReason(*marker.Reason)
	return marker.At, &reason, nil
}

// MarkCalendarLinkResetTx records through tx, which the caller commits, that
// a security revocation deleted userID's feed link at the instant at, for
// reason. The caller marks only after a delete that removed a token, so the
// marker always names a link that existed. Creates the preferences row when
// the user has none; every column the INSERT does not name takes its DDL
// default.
func MarkCalendarLinkResetTx(tx *gorm.DB, userID uint, at time.Time, reason engagementm.CalendarLinkResetReason) error {
	if err := tx.Exec(`
		INSERT INTO user_preferences (user_id, calendar_link_reset_at, calendar_link_reset_reason)
		VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			calendar_link_reset_at = EXCLUDED.calendar_link_reset_at,
			calendar_link_reset_reason = EXCLUDED.calendar_link_reset_reason,
			updated_at = NOW()`,
		userID, at, string(reason)).Error; err != nil {
		return fmt.Errorf("failed to mark calendar link reset: %w", err)
	}
	return nil
}

// ClearCalendarLinkResetTx removes userID's link-reset marker through tx,
// which the caller commits. A user with no marker, or no preferences row, is
// left untouched.
func ClearCalendarLinkResetTx(tx *gorm.DB, userID uint) error {
	if err := tx.Exec(`
		UPDATE user_preferences SET
			calendar_link_reset_at = NULL,
			calendar_link_reset_reason = NULL,
			updated_at = NOW()
		WHERE user_id = ? AND calendar_link_reset_at IS NOT NULL`,
		userID).Error; err != nil {
		return fmt.Errorf("failed to clear calendar link reset: %w", err)
	}
	return nil
}
