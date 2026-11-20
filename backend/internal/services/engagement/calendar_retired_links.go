package engagement

import (
	"context"
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

// retiredLinkNoteFor is how long after retirement the saved-shows notice
// calendar carries its note. The notice holds no show from the first poll
// on; the note is what tells a person why, and since it is placed on the date
// of every poll it would otherwise sit on today's date in their calendar for
// as long as they stay subscribed. The cutoff is this instant after the
// retirement, not a date boundary.
const retiredLinkNoteFor = 30 * 24 * time.Hour

// retiredLinkRemembered reports whether link is still remembered at the
// instant now: until one calendar year after it was retired, that instant
// included. After it the address answers as one the site never issued,
// whether or not ForgetExpiredRetiredLinks has deleted its row yet.
func retiredLinkRemembered(link *engagementm.RetiredCalendarLink, now time.Time) bool {
	return !now.After(link.RetiredAt.AddDate(1, 0, 0))
}

// retiredLinkSweepCutoff is the retired_at before which
// ForgetExpiredRetiredLinks deletes a row at the instant now: a year and a
// day earlier. The extra day covers the dates a calendar year normalises
// (a year after February 29 is March 1), so the sweep never deletes a row
// retiredLinkRemembered would still answer for.
func retiredLinkSweepCutoff(now time.Time) time.Time {
	return now.AddDate(-1, 0, -1)
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
// test checks that none is missing. A row whose reason has no entry here
// answers as unknown (retiredLinkAt).
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
// works. It deletes no other row: ForgetExpiredRetiredLinks does that, outside
// the revocations this runs inside.
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
	return deleted, nil
}

// retiredLinkSweepBatch is the most rows one statement of
// ForgetExpiredRetiredLinks deletes, so a large backlog is cleared in short
// statements rather than one long one.
const retiredLinkSweepBatch = 1000

// ForgetExpiredRetiredLinks deletes, through db, every retired link retired
// before retiredLinkSweepCutoff(now), in statements of at most
// retiredLinkSweepBatch rows, and reports how many it deleted. Reads already
// ignore a link once retiredLinkRemembered says so, so a sweep that fails or
// stops early changes no answer; it only bounds how long a row is kept. It
// stops at the first error or when ctx is done.
func ForgetExpiredRetiredLinks(ctx context.Context, db *gorm.DB, now time.Time) (int64, error) {
	var total int64
	for {
		result := db.WithContext(ctx).Exec(`
			DELETE FROM retired_calendar_links WHERE id IN (
				SELECT id FROM retired_calendar_links WHERE retired_at < ? ORDER BY retired_at LIMIT ?
			)`, retiredLinkSweepCutoff(now), retiredLinkSweepBatch)
		if result.Error != nil {
			return total, fmt.Errorf("failed to forget expired retired calendar links: %w", result.Error)
		}
		total += result.RowsAffected
		if result.RowsAffected < retiredLinkSweepBatch || ctx.Err() != nil {
			return total, nil
		}
	}
}

// retiredLinkAt is the retired link plainToken names, as remembered at the
// instant now, or nil when there is none or its reason has no wording in this
// build. It is one read through the token_hash unique index.
func (s *CalendarService) retiredLinkAt(plainToken string, now time.Time) (*engagementm.RetiredCalendarLink, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var links []engagementm.RetiredCalendarLink
	if err := s.db.
		Where("token_hash = ?", hashCalendarToken(plainToken)).
		Limit(1).
		Find(&links).Error; err != nil {
		return nil, fmt.Errorf("failed to look up retired calendar link: %w", err)
	}
	if len(links) == 0 || !retiredLinkRemembered(&links[0], now) {
		return nil, nil
	}
	// A reason added by a later migration, read by a build that predates it,
	// would otherwise render a note with no text.
	if _, ok := retiredLinkWordings[links[0].Reason]; !ok {
		return nil, nil
	}
	return &links[0], nil
}

// RetiredSavedShowsNotice renders the notice calendar for a retired address
// (contracts.CalendarServiceInterface).
func (s *CalendarService) RetiredSavedShowsNotice(plainToken string) (*contracts.RetiredFeedNotice, error) {
	return s.retiredNoticeAt(plainToken, time.Now(), renderRetiredSavedShows)
}

// RetiredFollowsNotice renders the notice Atom feed for a retired address
// (contracts.CalendarServiceInterface).
func (s *CalendarService) RetiredFollowsNotice(plainToken string) (*contracts.RetiredFeedNotice, error) {
	return s.retiredNoticeAt(plainToken, time.Now(), renderRetiredFollows)
}

// retiredNoticeRenderer renders a retired link's notice as of the instant now.
type retiredNoticeRenderer func(link *engagementm.RetiredCalendarLink, now time.Time) ([]byte, error)

// The notice each retired address answers with.
var (
	renderRetiredSavedShows retiredNoticeRenderer = func(link *engagementm.RetiredCalendarLink, now time.Time) ([]byte, error) {
		return buildRetiredLinkCalendar(link, now), nil
	}
	renderRetiredFollows retiredNoticeRenderer = func(link *engagementm.RetiredCalendarLink, _ time.Time) ([]byte, error) {
		return buildRetiredLinkAtom(link)
	}
)

// retiredNoticeAt renders, with render, the notice for the retired link
// plainToken names as remembered at the instant now, paired with a validator
// taken from its bytes in the form the public calendar feeds use. It returns
// nil when plainToken names no remembered retired link.
func (s *CalendarService) retiredNoticeAt(plainToken string, now time.Time, render retiredNoticeRenderer) (*contracts.RetiredFeedNotice, error) {
	link, err := s.retiredLinkAt(plainToken, now)
	if err != nil || link == nil {
		return nil, err
	}
	body, err := render(link, now)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	return &contracts.RetiredFeedNotice{
		Body: body,
		ETag: `W/"` + hex.EncodeToString(sum[:16]) + `"`,
	}, nil
}

// retiredLinkNoticeKey names a retired link in its notice's identifiers (the
// note's UID, the Atom ids). It is derived from the token's hash, which only
// a holder of the token can reproduce, so it reveals no row id or count, and
// it is the same on every poll for as long as the link is remembered.
func retiredLinkNoticeKey(link *engagementm.RetiredCalendarLink) string {
	sum := sha256.Sum256([]byte("retired-link-notice:" + link.TokenHash))
	return hex.EncodeToString(sum[:16])
}

// retiredLinkNoticeUID is the UID of a retired link's note. It stays the same
// on every day the note moves to, so a calendar app keeps one note rather
// than adding one a day.
func retiredLinkNoticeUID(link *engagementm.RetiredCalendarLink) string {
	return "retired-link-" + retiredLinkNoticeKey(link) + "@psychichomily.com"
}

// buildRetiredLinkCalendar renders the calendar a retired saved-shows address
// answers with at the instant now: the live feed's product id and name, and,
// until retiredLinkNoteFor has passed, one all-day note on now's date. It
// holds no show.
//
// The date is read in the site's fallback zone, the zone the feed reads a
// calendar day in for a show whose own zone it cannot name, since nothing
// about the subscriber's zone is known here. The bytes change when that date
// does, and once more when retiredLinkNoteFor passes: DTSTAMP is the later of
// the retirement and the start of the day, and SEQUENCE counts the days since
// the retirement, so each day's note outranks the one before it (RFC 5546
// 2.1.5).
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

		event := cal.AddEvent(retiredLinkNoticeUID(link))
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
		ID:      "tag:psychichomily.com,2026:retired-feed:" + retiredLinkNoticeKey(link),
		Updated: retiredAt,
		Author:  atomAuthor{Name: followsActivityFeedAuthor},
		Entry: retiredAtomEntry{
			Title:     wording.feedTitle,
			ID:        "tag:psychichomily.com,2026:retired-feed-notice:" + retiredLinkNoticeKey(link),
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
