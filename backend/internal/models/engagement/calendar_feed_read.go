package engagement

import "time"

// CalendarFeedClient is the family of calendar client a feed read came from,
// as the server classifies the request's User-Agent. The values are the ones
// the calendar_feed_reads.client CHECK constraint admits.
type CalendarFeedClient string

const (
	CalendarFeedClientGoogle   CalendarFeedClient = "google"
	CalendarFeedClientAppleIOS CalendarFeedClient = "apple_ios"
	CalendarFeedClientAppleMac CalendarFeedClient = "apple_mac"
	CalendarFeedClientOther    CalendarFeedClient = "other"
)

// CalendarFeedRead is one calendar client family's reads of one feed token:
// when that family first fetched the feed and when it last did. It never
// holds the User-Agent the family was classified from.
type CalendarFeedRead struct {
	TokenID     uint               `gorm:"column:token_id;primaryKey"`
	Client      CalendarFeedClient `gorm:"column:client;primaryKey"`
	FirstReadAt time.Time          `gorm:"column:first_read_at"`
	LastReadAt  time.Time          `gorm:"column:last_read_at"`
}

func (CalendarFeedRead) TableName() string {
	return "calendar_feed_reads"
}

// CalendarLinkResetReason is why a security revocation deleted an account's
// feed link. The values are the ones the
// user_preferences.calendar_link_reset_reason CHECK constraint admits.
type CalendarLinkResetReason string

const (
	CalendarLinkResetSignOutEverywhere CalendarLinkResetReason = "sign_out_everywhere"
	CalendarLinkResetPasswordReset     CalendarLinkResetReason = "password_reset"
)
