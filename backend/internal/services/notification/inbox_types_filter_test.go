package notification

import (
	"fmt"
	"time"

	notificationm "psychic-homily-backend/internal/models/notification"
)

// TestGetUserNotifications_TypesNarrowTheListOnly: the types filter returns
// only the named discriminators, newest first, and the unread count it sits
// beside keeps counting the whole inbox.
func (s *NotificationFilterSuite) TestGetUserNotifications_TypesNarrowTheListOnly() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)
	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	// Two rows of kinds the filter leaves out: a show-filter row and a request
	// row, both unread.
	s.Require().NoError(s.db.Exec(`
		INSERT INTO notification_log (user_id, entity_type, entity_id, channel, sent_at)
		VALUES (?, 'show', ?, 'email', now() + interval '1 minute'),
		       (?, 'request_fulfillment_proposed', 1, 'in_app', now() + interval '2 minutes')`,
		userID, showID, userID).Error)

	all, err := s.svc.GetUserNotifications(inboxViewer(userID), 20, 0)
	s.Require().NoError(err)
	s.Require().Len(all, 3, "no types means every inbox row")

	narrowed, err := s.svc.GetUserNotifications(inboxViewer(userID), 20, 0,
		notificationm.NotificationEntityArtistShowAlert, notificationm.NotificationEntityVenueShowAlert)
	s.Require().NoError(err)
	s.Require().Len(narrowed, 1)
	s.Equal(notificationm.NotificationEntityArtistShowAlert, narrowed[0].EntityType)

	unread, err := s.svc.GetUnreadCount(inboxViewer(userID))
	s.Require().NoError(err)
	s.Equal(int64(3), unread, "the unread count is the whole inbox, not the filtered list")
}

// TestGetUserNotifications_TypesStillHonourTheEmailLaneRule: narrowing to a
// two-lane alert type must not resurrect its email lane, which is never a bell
// entry.
func (s *NotificationFilterSuite) TestGetUserNotifications_TypesStillHonourTheEmailLaneRule() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{"alerts":{"shows":{"in_app":true,"email":true}}}`)
	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Require().Equal(int64(1), s.artistAlertRows(userID, showID, notificationm.NotificationChannelEmail))

	entries, err := s.svc.GetUserNotifications(inboxViewer(userID), 20, 0,
		notificationm.NotificationEntityArtistShowAlert)
	s.Require().NoError(err)
	s.Require().Len(entries, 1, "one bell entry: the in-app lane, not the email lane beside it")
	s.Equal(notificationm.NotificationChannelInApp, entries[0].Channel)
}

// TestArtistAlert_InboxRowCarriesVenueAndDate: the row names where and when
// the announced show happens, with the zone its date reads in.
func (s *NotificationFilterSuite) TestArtistAlert_InboxRowCarriesVenueAndDate() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)
	// Two venues: the lower id is the one the row names.
	first := s.createTestVenue("Valley Bar")
	second := s.createTestVenue("Crescent Ballroom")
	showID := s.createTestShow("Oneida", []uint{artistID}, []uint{second, first})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	entries, err := s.svc.GetUserNotifications(inboxViewer(userID), 20, 0)
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	e := entries[0]

	s.Equal("Valley Bar", e.AlertShowVenueName)
	s.Require().NotNil(e.AlertShowEventDate)
	show := s.loadShow(showID)
	s.True(show.EventDate.Equal(*e.AlertShowEventDate), "the date is the show's own event_date")
	s.Equal("America/Phoenix", e.AlertShowTimezone, "an AZ venue with no stored zone resolves through the state")
}

// TestVenueAlert_InboxRowCarriesDateSpan: the row bounds its batch's event
// dates over the same members its count counts.
func (s *NotificationFilterSuite) TestVenueAlert_InboxRowCarriesDateSpan() {
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)

	var dates []time.Time
	for i := 0; i < 3; i++ {
		showID := s.createTestShow(fmt.Sprintf("Night %d", i), nil, []uint{venueID})
		date := time.Now().Add(time.Duration(48*(3-i)) * time.Hour).UTC().Truncate(time.Second)
		s.Require().NoError(s.db.Exec("UPDATE shows SET event_date = ? WHERE id = ?", date, showID).Error)
		dates = append(dates, date)
		s.announce(showID)
	}
	s.Equal(1, s.flushNow())

	entries, err := s.svc.GetUserNotifications(inboxViewer(userID), 20, 0,
		notificationm.NotificationEntityVenueShowAlert)
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	e := entries[0]

	s.Equal(3, e.AlertShowCount)
	s.Require().NotNil(e.AlertShowFirstDate)
	s.Require().NotNil(e.AlertShowLastDate)
	// Created in DESCENDING date order, so the span cannot be an accident of
	// insertion order.
	s.True(dates[2].Equal(*e.AlertShowFirstDate), "first = earliest event date, got %v", e.AlertShowFirstDate)
	s.True(dates[0].Equal(*e.AlertShowLastDate), "last = latest event date, got %v", e.AlertShowLastDate)
	s.Equal("America/Phoenix", e.AlertShowTimezone)
}
