package engagement

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// feedReadRecordInterval is the least time between two recorded reads of one
// token by one client family. Calendar clients poll every few minutes to every
// few hours; recording each poll would write on every one of them for no gain
// in what the status can say. The API docs state the interval in words (the
// last_read_at doc tag on contracts.CalendarFeedReadResponse and
// CalendarServiceInterface.RecordFeedRead), so a change here must be copied
// to them.
const feedReadRecordInterval = 10 * time.Minute

// feedReadForgetAfter is how long after its last read a client family is
// forgotten. A forgotten read is kept as a row but treated everywhere as
// absent: the status does not list it, a revocation does not count it, and
// the family's next read starts it afresh, as a read never seen before
// would. The API docs state the period in words (the reads and link_reset_at
// doc tags on contracts.CalendarTokenStatusResponse and
// contracts.CalendarFeedStatusResponse, CalendarServiceInterface.GetTokenStatus,
// and the hand-written CalendarTokenStatusResponse in
// frontend/features/shows/types.ts), so a change here must be copied to them.
const feedReadForgetAfter = 30 * 24 * time.Hour

// feedReadRememberedSince is the earliest last_read_at a read can have and
// still be remembered at the instant now. A read exactly feedReadForgetAfter
// old is remembered; one any older is forgotten.
func feedReadRememberedSince(now time.Time) time.Time {
	return now.Add(-feedReadForgetAfter)
}

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
	case hasAnyPrefix(userAgent, appleIOSPrefixes) && containsAny(userAgent, appleIOSFetchers):
		return engagementm.CalendarFeedClientAppleIOS, true
	case hasAnyPrefix(userAgent, appleMacPrefixes) && containsAny(userAgent, appleMacFetchers),
		strings.Contains(userAgent, "CalendarAgent"):
		return engagementm.CalendarFeedClientAppleMac, true
	case strings.HasPrefix(userAgent, "Mozilla/"):
		return "", false
	default:
		return engagementm.CalendarFeedClientOther, true
	}
}

// The platform prefixes and fetcher names an Apple calendar User-Agent is
// recognised by. A User-Agent needs one of its platform's prefixes and one of
// its fetchers, except that CalendarAgent alone is enough for apple_mac.
var (
	appleIOSPrefixes = []string{"iOS/", "iPadOS/"}
	appleIOSFetchers = []string{"dataaccessd", "accountsd"}
	appleMacPrefixes = []string{"macOS/", "Mac OS X/", "Mac_OS_X/", "Mac+OS+X/"}
	appleMacFetchers = []string{"dataaccessd", "accountsd", "CalendarAgent"}
)

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
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
// replaced since the feed was served matches nothing and nothing is written.
// A token deleted after the INSERT found it but before the row's foreign-key
// check fails that check; that is the same outcome and reports the same way.
// A read recorded less than feedReadRecordInterval before at also makes the
// SELECT empty, so a poll inside the interval only reads (an ON CONFLICT
// update refused by its WHERE would still lock, and so write, the row). The
// conflict branch repeats that bound for two polls that race past the
// SELECT, and keeps last_read_at from moving backwards. A conflicting row that
// was forgotten as of at (feedReadForgetAfter) takes at as its first_read_at
// too, so a family that comes back reads as newly seen.
func (s *CalendarService) recordFeedReadAt(ctx context.Context, plainToken, userAgent string, at time.Time) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	client, ok := classifyCalendarFeedClient(userAgent)
	if !ok {
		return false, nil
	}

	cutoff := at.Add(-feedReadRecordInterval)
	result := s.db.WithContext(ctx).Exec(`
		INSERT INTO calendar_feed_reads (token_id, client, first_read_at, last_read_at)
		SELECT t.id, ?::text, ?::timestamptz, ?::timestamptz
		FROM calendar_tokens t
		WHERE t.token_hash = ?
		  AND NOT EXISTS (
			SELECT 1 FROM calendar_feed_reads r
			WHERE r.token_id = t.id AND r.client = ?::text AND r.last_read_at > ?::timestamptz
		  )
		ON CONFLICT (token_id, client) DO UPDATE SET
			first_read_at = CASE
				WHEN calendar_feed_reads.last_read_at < ?::timestamptz THEN EXCLUDED.first_read_at
				ELSE calendar_feed_reads.first_read_at
			END,
			last_read_at = EXCLUDED.last_read_at
		WHERE calendar_feed_reads.last_read_at <= ?::timestamptz`,
		string(client), at, at, hashCalendarToken(plainToken), string(client), cutoff,
		feedReadRememberedSince(at), cutoff)
	if errors.Is(result.Error, gorm.ErrForeignKeyViolated) {
		return false, nil
	}
	if result.Error != nil {
		return false, fmt.Errorf("failed to record calendar feed read: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// feedReads lists the reads of tokenID still remembered at the instant now
// (feedReadForgetAfter), most recently read first.
func (s *CalendarService) feedReads(tokenID uint, now time.Time) ([]contracts.CalendarFeedReadResponse, error) {
	var rows []engagementm.CalendarFeedRead
	if err := s.db.Where("token_id = ? AND last_read_at >= ?", tokenID, feedReadRememberedSince(now)).
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

// ForgetFeedRead deletes the recorded reads of userID's feed link by client
// (contracts.CalendarServiceInterface). The client's next read inserts a new
// row, so it comes back as newly seen.
func (s *CalendarService) ForgetFeedRead(userID uint, client engagementm.CalendarFeedClient) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if err := s.db.Exec(`
		DELETE FROM calendar_feed_reads
		WHERE client = ?
		  AND token_id IN (SELECT id FROM calendar_tokens WHERE user_id = ?)`,
		string(client), userID).Error; err != nil {
		return fmt.Errorf("failed to forget calendar feed read: %w", err)
	}
	return nil
}

// linkResetMarker reads userID's outstanding link-reset marker. Both results
// are nil when there is none, including when the user has no preferences row;
// the pair CHECK constraint keeps them from differing.
func (s *CalendarService) linkResetMarker(userID uint) (*time.Time, *engagementm.CalendarLinkResetReason, error) {
	var marker struct {
		At     *time.Time                           `gorm:"column:calendar_link_reset_at"`
		Reason *engagementm.CalendarLinkResetReason `gorm:"column:calendar_link_reset_reason"`
	}
	if err := s.db.Raw(`
		SELECT calendar_link_reset_at, calendar_link_reset_reason
		FROM user_preferences
		WHERE user_id = ?`, userID).Scan(&marker).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to load calendar link reset: %w", err)
	}
	return marker.At, marker.Reason, nil
}

// RevokeCalendarLinkTx deletes userID's feed token through tx, which the
// caller commits, for a security revocation, and records the link reset at
// the instant at for reason when the deleted token had a read still
// remembered at at (feedReadForgetAfter). A link with none leaves no marker:
// no calendar app its owner is still told about could stop updating. An
// account with no token is left as it is, its marker included.
//
// The token row is locked before its reads are counted, so a read being
// recorded concurrently either commits first and is counted, or waits for
// this transaction and then fails its foreign-key check and writes nothing.
// The count runs before the delete, whose cascade removes the reads.
func RevokeCalendarLinkTx(tx *gorm.DB, userID uint, at time.Time, reason engagementm.CalendarLinkResetReason) error {
	var tokenIDs []uint
	if err := tx.Model(&engagementm.CalendarToken{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).
		Pluck("id", &tokenIDs).Error; err != nil {
		return fmt.Errorf("failed to lock calendar token: %w", err)
	}
	if len(tokenIDs) == 0 {
		return nil
	}

	var read bool
	if err := tx.Raw(
		"SELECT EXISTS (SELECT 1 FROM calendar_feed_reads WHERE token_id IN ? AND last_read_at >= ?)",
		tokenIDs, feedReadRememberedSince(at),
	).Scan(&read).Error; err != nil {
		return fmt.Errorf("failed to check calendar feed reads: %w", err)
	}

	if _, err := DeleteCalendarTokensTx(tx, userID); err != nil {
		return err
	}
	if !read {
		return nil
	}
	return MarkCalendarLinkResetTx(tx, userID, at, reason)
}

// MarkCalendarLinkResetTx records through tx, which the caller commits, that
// a security revocation deleted userID's feed link at the instant at, for
// reason. RevokeCalendarLinkTx is its caller: it marks only a link it deleted
// that a calendar app had read. Creates the preferences row when the user has
// none; every column the INSERT does not name takes its DDL default.
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
// which the caller commits, and reports whether there was one to remove. A
// user with no marker, or no preferences row, is left untouched.
func ClearCalendarLinkResetTx(tx *gorm.DB, userID uint) (bool, error) {
	result := tx.Exec(`
		UPDATE user_preferences SET
			calendar_link_reset_at = NULL,
			calendar_link_reset_reason = NULL,
			updated_at = NOW()
		WHERE user_id = ? AND calendar_link_reset_at IS NOT NULL`,
		userID)
	if result.Error != nil {
		return false, fmt.Errorf("failed to clear calendar link reset: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}
