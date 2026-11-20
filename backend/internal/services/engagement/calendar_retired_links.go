package engagement

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"time"

	ics "github.com/arran4/golang-ical"
	"gorm.io/gorm"

	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// retiredLinkForgetAfter is how long a retired feed address is remembered.
// After it the address answers as one the site never issued, and the row is
// deleted the next time any link is retired (RetireCalendarTokensTx).
const retiredLinkForgetAfter = 365 * 24 * time.Hour

// retiredLinkNoteFor is how long after retirement the saved-shows notice
// calendar carries its note. A calendar note recurs on every day it is
// served, so it stops; the calendar stays valid and empty after it, which is
// what lets a calendar app that is still subscribed clear the old shows.
const retiredLinkNoteFor = 30 * 24 * time.Hour

// retiredLinkRememberedSince is the earliest retired_at a link can have and
// still be remembered at the instant now. A link retired exactly
// retiredLinkForgetAfter ago is remembered; one any older is forgotten.
func retiredLinkRememberedSince(now time.Time) time.Time {
	return now.Add(-retiredLinkForgetAfter)
}

// retiredLinkWording is the owner-approved text a retired address answers
// with for one reason: the calendar note's title, and the feed reader
// entry's title and body.
type retiredLinkWording struct {
	calendarNote string
	feedTitle    string
	feedBody     string
}

// retiredLinkWordings holds the text for every CalendarLinkRetiredReason; a
// test checks that none is missing.
var retiredLinkWordings = map[engagementm.CalendarLinkRetiredReason]retiredLinkWording{
	engagementm.CalendarLinkRetiredReset: {
		calendarNote: "Psychic Homily: this calendar link was reset. Add it again from your Library.",
		feedTitle:    "This feed link was reset",
		feedBody:     "Add it again from Settings on Psychic Homily.",
	},
	engagementm.CalendarLinkRetiredReplaced: {
		calendarNote: "Psychic Homily: this calendar link was replaced. Add the new one from your Library.",
		feedTitle:    "This feed link was replaced",
		feedBody:     "Copy the new one from Settings on Psychic Homily.",
	},
	engagementm.CalendarLinkRetiredTurnedOff: {
		calendarNote: "Psychic Homily: this calendar link was turned off. Turn it on again from your Library.",
		feedTitle:    "This feed link was turned off",
		feedBody:     "Turn it on again from Settings on Psychic Homily.",
	},
}

// RetireCalendarTokensTx deletes userID's feed token through tx, which the
// caller commits, remembers its address as retired for reason at the instant
// at, and reports how many tokens it deleted. The token's recorded feed reads
// are deleted with it (calendar_feed_reads cascades from calendar_tokens).
// Nothing but the token's hash, the reason and the instant is kept. It writes
// no link-reset marker; a security revocation retires through
// RevokeCalendarLinkTx, which does.
//
// A retired token is refused by ValidateCalendarToken before any feed is
// served, so a feed cached in some process's CalendarService cannot reach its
// holder; the next token CreateToken mints clears that cache.
//
// The delete and the remembering are one statement, so a token is never
// deleted without its address being remembered, nor remembered while it still
// works. It also deletes every retired link older than retiredLinkForgetAfter,
// which no read would answer for any more.
func RetireCalendarTokensTx(tx *gorm.DB, userID uint, reason engagementm.CalendarLinkRetiredReason, at time.Time) (int64, error) {
	var deleted int64
	if err := tx.Raw(`
		WITH retired AS (
			DELETE FROM calendar_tokens WHERE user_id = ? RETURNING token_hash
		), remembered AS (
			INSERT INTO retired_calendar_links (token_hash, reason, retired_at)
			SELECT token_hash, ?, ? FROM retired
			ON CONFLICT (token_hash) DO NOTHING
		)
		SELECT count(*) FROM retired`,
		userID, string(reason), at.Truncate(time.Microsecond)).Scan(&deleted).Error; err != nil {
		return 0, fmt.Errorf("failed to retire calendar token: %w", err)
	}
	if err := tx.Exec("DELETE FROM retired_calendar_links WHERE retired_at < ?",
		retiredLinkRememberedSince(at)).Error; err != nil {
		return 0, fmt.Errorf("failed to forget old retired calendar links: %w", err)
	}
	return deleted, nil
}

// retiredLinkAt is the retired link plainToken names, as remembered at the
// instant now, or nil when there is none. It is one read through the
// token_hash unique index.
func (s *CalendarService) retiredLinkAt(plainToken string, now time.Time) (*engagementm.RetiredCalendarLink, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var links []engagementm.RetiredCalendarLink
	if err := s.db.
		Where("token_hash = ? AND retired_at >= ?", hashCalendarToken(plainToken), retiredLinkRememberedSince(now)).
		Limit(1).
		Find(&links).Error; err != nil {
		return nil, fmt.Errorf("failed to look up retired calendar link: %w", err)
	}
	if len(links) == 0 {
		return nil, nil
	}
	if _, ok := retiredLinkWordings[links[0].Reason]; !ok {
		return nil, fmt.Errorf("retired calendar link has unknown reason %q", links[0].Reason)
	}
	return &links[0], nil
}

// RetiredSavedShowsNotice renders the notice calendar for a retired address
// (contracts.CalendarServiceInterface).
func (s *CalendarService) RetiredSavedShowsNotice(plainToken string) (*contracts.RetiredFeedNotice, error) {
	return s.retiredSavedShowsNoticeAt(plainToken, time.Now())
}

func (s *CalendarService) retiredSavedShowsNoticeAt(plainToken string, now time.Time) (*contracts.RetiredFeedNotice, error) {
	link, err := s.retiredLinkAt(plainToken, now)
	if err != nil || link == nil {
		return nil, err
	}
	return retiredFeedNotice(buildRetiredLinkCalendar(link, now)), nil
}

// RetiredFollowsNotice renders the notice Atom feed for a retired address
// (contracts.CalendarServiceInterface).
func (s *CalendarService) RetiredFollowsNotice(plainToken string) (*contracts.RetiredFeedNotice, error) {
	return s.retiredFollowsNoticeAt(plainToken, time.Now())
}

func (s *CalendarService) retiredFollowsNoticeAt(plainToken string, now time.Time) (*contracts.RetiredFeedNotice, error) {
	link, err := s.retiredLinkAt(plainToken, now)
	if err != nil || link == nil {
		return nil, err
	}
	body, err := buildRetiredLinkAtom(link)
	if err != nil {
		return nil, err
	}
	return retiredFeedNotice(body), nil
}

// retiredFeedNotice pairs body with a validator taken from its bytes, in the
// form the public calendar feeds use.
func retiredFeedNotice(body []byte) *contracts.RetiredFeedNotice {
	sum := sha256.Sum256(body)
	return &contracts.RetiredFeedNotice{
		Body: body,
		ETag: `W/"` + hex.EncodeToString(sum[:16]) + `"`,
	}
}

// retiredLinkNoticeUID is the UID of a retired link's note. It names the
// remembered row, not the account, and stays the same on every day the note
// moves to, so a calendar app keeps one note rather than adding one a day.
func retiredLinkNoticeUID(linkID uint) string {
	return fmt.Sprintf("retired-link-%d@psychichomily.com", linkID)
}

// buildRetiredLinkCalendar renders the calendar a retired saved-shows address
// answers with at the instant now: the live feed's product id and name, and,
// until retiredLinkNoteFor has passed, one all-day note on now's date. It
// holds no show.
//
// The date is read in the site's fallback zone, the zone the feed reads a
// calendar day in for a show whose own zone it cannot name, since nothing
// about the subscriber's zone is known here. The bytes change only when that
// date does: DTSTAMP is the later of the retirement and the start of the day,
// and SEQUENCE counts the days since the retirement, so each day's note
// outranks the one before it (RFC 5546 2.1.5).
func buildRetiredLinkCalendar(link *engagementm.RetiredCalendarLink, now time.Time) []byte {
	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)
	cal.SetProductId(savedShowsFeedProductID)
	cal.SetName(savedShowsFeedName)
	cal.SetXWRCalName(savedShowsFeedName)

	if !now.After(link.RetiredAt.Add(retiredLinkNoteFor)) {
		loc, _ := shared.EventLocationResolved(nil, "")
		today := startOfDay(now.In(loc))
		retiredDay := startOfDay(link.RetiredAt.In(loc))

		stamp := today
		if stamp.Before(link.RetiredAt) {
			stamp = link.RetiredAt
		}

		event := cal.AddEvent(retiredLinkNoticeUID(link.ID))
		event.SetDtStampTime(stamp)
		event.SetSequence(max(calendarDaysBetween(retiredDay, today), 0))
		event.SetAllDayStartAt(today)
		event.SetAllDayEndAt(today.AddDate(0, 0, 1))
		event.SetSummary(retiredLinkWordings[link.Reason].calendarNote)
		// A note is not a commitment, so it does not mark the subscriber busy.
		event.SetTimeTransparency(ics.TransparencyTransparent)
	}

	return []byte(cal.Serialize(ics.WithNewLine("\r\n")))
}

// startOfDay is midnight at the start of t's date, in t's location.
func startOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

// calendarDaysBetween counts the dates from from's to to's, read as dates
// with no zone, so a day that a zone transition shortens or lengthens still
// counts as one.
func calendarDaysBetween(from, to time.Time) int {
	fromDate := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toDate := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(toDate.Sub(fromDate) / (24 * time.Hour))
}

// retiredAtomFeed is the Atom document (RFC 4287) a retired follows address
// answers with: the live feed's title and author, and exactly one entry.
type retiredAtomFeed struct {
	XMLName xml.Name         `xml:"feed"`
	Xmlns   string           `xml:"xmlns,attr"`
	Title   string           `xml:"title"`
	ID      string           `xml:"id"`
	Updated string           `xml:"updated"`
	Author  atomAuthor       `xml:"author"`
	Entry   retiredAtomEntry `xml:"entry"`
}

// retiredAtomEntry carries its text as atom:content, so it needs no
// alternate link (RFC 4287 4.1.2).
type retiredAtomEntry struct {
	Title     string   `xml:"title"`
	ID        string   `xml:"id"`
	Updated   string   `xml:"updated"`
	Published string   `xml:"published"`
	Content   atomText `xml:"content"`
}

type atomText struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

// buildRetiredLinkAtom renders the Atom feed a retired follows address
// answers with. Every value comes from the remembered row, so the bytes are
// the same on every poll for as long as the link is remembered, and the entry
// keeps one id and one date throughout.
func buildRetiredLinkAtom(link *engagementm.RetiredCalendarLink) ([]byte, error) {
	wording := retiredLinkWordings[link.Reason]
	retiredAt := link.RetiredAt.UTC().Format(time.RFC3339)
	feed := retiredAtomFeed{
		Xmlns:   "http://www.w3.org/2005/Atom",
		Title:   followsActivityFeedTitle,
		ID:      fmt.Sprintf("tag:psychichomily.com,2026:retired-feed:%d", link.ID),
		Updated: retiredAt,
		Author:  atomAuthor{Name: followsActivityFeedAuthor},
		Entry: retiredAtomEntry{
			Title:     wording.feedTitle,
			ID:        fmt.Sprintf("tag:psychichomily.com,2026:retired-feed-notice:%d", link.ID),
			Updated:   retiredAt,
			Published: retiredAt,
			Content:   atomText{Type: "text", Text: wording.feedBody},
		},
	}
	payload, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal retired link atom feed: %w", err)
	}
	return append([]byte(xml.Header), payload...), nil
}
