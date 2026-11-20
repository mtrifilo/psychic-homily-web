package engagement

import "time"

// CalendarLinkRetiredReason is why a feed link stopped working while its
// owner kept the account, as its old address is told. The values are the ones
// the retired_calendar_links.reason CHECK constraint admits. It is not
// CalendarLinkResetReason, the owner's marker of which security revocation
// reset the link: every such revocation is CalendarLinkRetiredReset here.
type CalendarLinkRetiredReason string

const (
	// CalendarLinkRetiredReset: a security revocation (signing out
	// everywhere, a password reset, or the mailbox-proof reclaim) deleted the
	// link.
	CalendarLinkRetiredReset CalendarLinkRetiredReason = "reset"
	// CalendarLinkRetiredReplaced: the owner made a new link.
	CalendarLinkRetiredReplaced CalendarLinkRetiredReason = "replaced"
	// CalendarLinkRetiredTurnedOff: the owner turned the feed off.
	CalendarLinkRetiredTurnedOff CalendarLinkRetiredReason = "turned_off"
)

// CalendarLinkRetiredReasons is every retirement reason. The
// retired_calendar_links.reason CHECK constraint spells the same set; a test
// compares the two.
var CalendarLinkRetiredReasons = []CalendarLinkRetiredReason{
	CalendarLinkRetiredReset,
	CalendarLinkRetiredReplaced,
	CalendarLinkRetiredTurnedOff,
}

// RetiredCalendarLink is what the site remembers of a deleted feed token: the
// hash calendar_tokens held for it, why it was retired and when. It holds
// nothing that names the account or its shows, and never the plaintext.
type RetiredCalendarLink struct {
	ID        uint                      `gorm:"column:id;primaryKey"`
	TokenHash string                    `gorm:"column:token_hash"`
	Reason    CalendarLinkRetiredReason `gorm:"column:reason"`
	RetiredAt time.Time                 `gorm:"column:retired_at"`
}

func (RetiredCalendarLink) TableName() string {
	return "retired_calendar_links"
}
