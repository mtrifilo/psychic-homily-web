package notification

import (
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
)

// Scene-follow fan-out tests (PSY-1341) — run inside NotificationFilterSuite
// (real Postgres, all migrations). Scene rows + follows are seeded directly:
// the registry's get-or-create is catalog's concern (tested there); this suite
// owns the notify semantics.

// seedSceneFollow creates a fallback-scope scene row for Phoenix/AZ (matching
// createTestVenue's city/state, no metro) and a follow for the user, with the
// optional notify mode stored in settings.
func (s *NotificationFilterSuite) seedSceneFollow(userID uint, mode string) uint {
	var sceneID uint
	s.Require().NoError(s.db.Raw(`
		INSERT INTO scenes (metro, city, state, slug)
		VALUES (NULL, 'Phoenix', 'AZ', 'phoenix-az')
		ON CONFLICT DO NOTHING
		RETURNING id`).Scan(&sceneID).Error)
	if sceneID == 0 {
		s.Require().NoError(s.db.Raw(`SELECT id FROM scenes WHERE slug = 'phoenix-az'`).Scan(&sceneID).Error)
	}
	if mode == "" {
		s.Require().NoError(s.db.Exec(`
			INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at)
			VALUES (?, 'scene', ?, 'follow', now())`, userID, sceneID).Error)
	} else {
		s.Require().NoError(s.db.Exec(`
			INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at, settings)
			VALUES (?, 'scene', ?, 'follow', now(), jsonb_build_object('scene_notify_mode', ?::text))`,
			userID, sceneID, mode).Error)
	}
	return sceneID
}

// seedSceneFollowWithSettings is seedSceneFollow with a caller-supplied
// settings document, which is what a follow that has been configured carries.
// The document is shared: scene_notify_mode and the alerts object live side by
// side in it.
func (s *NotificationFilterSuite) seedSceneFollowWithSettings(userID uint, settingsJSON string) uint {
	sceneID := s.seedSceneFollow(userID, "")
	s.Require().NoError(s.db.Exec(`
		UPDATE user_bookmarks SET settings = ?::jsonb
		WHERE user_id = ? AND entity_type = 'scene' AND entity_id = ? AND action = 'follow'`,
		settingsJSON, userID, sceneID).Error)
	return sceneID
}

// setAccountShowEmail writes the ACCOUNT-level opt-in, which is the control the
// settings card's email box drives and the one an unsubscribe clears.
func (s *NotificationFilterSuite) setAccountShowEmail(userID uint, on bool) {
	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_preferences (user_id, alert_defaults)
		VALUES (?, jsonb_build_object('shows', jsonb_build_object('email', ?::boolean)))
		ON CONFLICT (user_id) DO UPDATE SET alert_defaults = EXCLUDED.alert_defaults`,
		userID, on).Error)
}

func (s *NotificationFilterSuite) followArtist(userID, artistID uint) {
	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at)
		VALUES (?, 'artist', ?, 'follow', now())`, userID, artistID).Error)
}

func (s *NotificationFilterSuite) sceneLogCount(userID, showID uint) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND entity_type = 'show' AND entity_id = ? AND channel = 'email'", userID, showID).
		Count(&n).Error)
	return n
}

func (s *NotificationFilterSuite) loadShow(showID uint) *catalogm.Show {
	var show catalogm.Show
	s.Require().NoError(s.db.First(&show, showID).Error)
	return &show
}

func (s *NotificationFilterSuite) TestSceneFollow_DefaultModeNotifiesAllShows() {
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")

	artistID := s.createTestArtist("Some Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Scene Show", []uint{artistID}, []uint{venueID})

	// No filters exist at all — the scene fan-out must still run (the filter
	// pass's zero-match case must not early-return past it).
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.sceneLogCount(userID, showID))

	// Idempotent on re-approval: the dedup check, not the UNIQUE constraint
	// (filter_id NULLs compare distinct), prevents a second row.
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.sceneLogCount(userID, showID))
}

// PSY-1466: "off" must suppress the immediate new-show notification entirely
// — no dedup log row, no email — regardless of the show's artists.
func (s *NotificationFilterSuite) TestSceneFollow_OffModeSuppressesNotification() {
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "off")

	artistID := s.createTestArtist("Muted Band")
	s.followArtist(userID, artistID) // even a followed-band match must not override "off"
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Muted Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(0), s.sceneLogCount(userID, showID))
}

// PSY-1466: a show mapping to two scenes the user follows — one "off", one
// qualifying ("all") — must still produce exactly one notification. The "off"
// row must not veto the qualifying row regardless of iteration order.
func (s *NotificationFilterSuite) TestSceneFollow_OffFollowDoesNotVetoAnotherQualifyingFollow() {
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "off") // phoenix-az

	var tucsonID uint
	s.Require().NoError(s.db.Raw(`
		INSERT INTO scenes (metro, city, state, slug)
		VALUES (NULL, 'Tucson', 'AZ', 'tucson-az') RETURNING id`).Scan(&tucsonID).Error)
	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at)
		VALUES (?, 'scene', ?, 'follow', now())`, userID, tucsonID).Error)

	artistID := s.createTestArtist("Two City Band")
	phxVenue := s.createTestVenue("The Rebel Lounge")
	tucsonVenue := catalogm.Venue{Name: "Club Congress", City: "Tucson", State: "AZ"}
	s.Require().NoError(s.db.Create(&tucsonVenue).Error)

	showID := s.createTestShow("Off Plus All Show", []uint{artistID}, []uint{phxVenue, tucsonVenue.ID})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.sceneLogCount(userID, showID))
}

// PSY-1466: when EVERY matching follow for a user is "off", the followed-
// bands gate must not be consulted at all — an artist match on an "off"
// scene follow must not accidentally qualify the user.
func (s *NotificationFilterSuite) TestSceneFollow_AllOffFollowsNeverNotify() {
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "off")

	artistID := s.createTestArtist("Off Only Band")
	s.followArtist(userID, artistID)
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Off Only Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(0), s.sceneLogCount(userID, showID))
}

// The gate itself: only a scene follower who also follows a band on the bill
// qualifies.
//
// Since PSY-1896 the fan is reached by the ARTIST-follow pass, which runs first
// and is the more specific of the two — an artist follow is what makes them
// qualify here, so telling them "a band you follow announced a show" instead of
// "a show is on in Phoenix" spends the one per-(user, show) slot on the better
// sentence. The gate is unchanged; what moved is which system delivers.
func (s *NotificationFilterSuite) TestSceneFollow_FollowedBandsOnlyGate() {
	fan := s.createTestUser()     // follows the artist → notified
	tourist := s.createTestUser() // follows only the scene → gated out
	s.seedSceneFollow(fan, "followed_bands_only")
	s.seedSceneFollow(tourist, "followed_bands_only")

	artistID := s.createTestArtist("Followed Band")
	s.followArtist(fan, artistID)

	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Gated Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.artistAlertRows(fan, showID, notificationm.NotificationChannelInApp),
		"the artist-follow pass claims the fan first")
	s.Equal(int64(0), s.sceneLogCount(fan, showID),
		"and the scene pass must not add a second notification on top of it")

	// The tourist follows no band on the bill, so neither system reaches them.
	s.Equal(int64(0), s.artistAlertRows(tourist, showID, notificationm.NotificationChannelInApp))
	s.Equal(int64(0), s.sceneLogCount(tourist, showID))
}

func (s *NotificationFilterSuite) TestSceneFollow_DedupsAgainstFilterMatch() {
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")

	artistID := s.createTestArtist("Filtered Band")
	venueID := s.createTestVenue("The Rebel Lounge")

	// A filter matching the artist — the user would match via BOTH systems.
	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:        "Filtered Band shows",
		ArtistIDs:   []int64{int64(artistID)},
		NotifyEmail: true,
		NotifyInApp: true,
	})
	s.Require().NoError(err)

	showID := s.createTestShow("Doubly Matched Show", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	// Exactly one notification: the filter's. The scene pass defers.
	s.Equal(int64(1), s.sceneLogCount(userID, showID))
}

func (s *NotificationFilterSuite) TestSceneFollow_AnyQualifyingFollowWins() {
	// User follows TWO scenes a multi-venue show maps to: one gated
	// (followed_bands_only, no matching artist follow) and one "all". The
	// explicit "all" subscription must win regardless of row order.
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "followed_bands_only") // phoenix-az

	var tucsonID uint
	s.Require().NoError(s.db.Raw(`
		INSERT INTO scenes (metro, city, state, slug)
		VALUES (NULL, 'Tucson', 'AZ', 'tucson-az') RETURNING id`).Scan(&tucsonID).Error)
	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at)
		VALUES (?, 'scene', ?, 'follow', now())`, userID, tucsonID).Error)

	artistID := s.createTestArtist("Unfollowed Band")
	phxVenue := s.createTestVenue("The Rebel Lounge")
	tucsonVenue := catalogm.Venue{Name: "Club Congress", City: "Tucson", State: "AZ"}
	s.Require().NoError(s.db.Create(&tucsonVenue).Error)

	showID := s.createTestShow("Two City Show", []uint{artistID}, []uint{phxVenue, tucsonVenue.ID})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.sceneLogCount(userID, showID))
}

func (s *NotificationFilterSuite) TestSceneFollow_SubmitterIsNotSelfNotified() {
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")

	artistID := s.createTestArtist("My Own Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("My Own Show", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.db.Exec(`UPDATE shows SET submitted_by = ? WHERE id = ?`, userID, showID).Error)

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(0), s.sceneLogCount(userID, showID))
}

func (s *NotificationFilterSuite) TestSceneFollow_FallbackRowMatchesMetroStampedVenue() {
	// Scope drift: a fallback scene row predates a venue-metro backfill. The
	// join must still connect them (normalized city/state, regardless of the
	// venue's new metro) so existing followers aren't stranded.
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")

	metro := "38060"
	venue := catalogm.Venue{Name: "Stamped Venue", City: "phoenix", State: "az", Metro: &metro}
	s.Require().NoError(s.db.Create(&venue).Error)
	artistID := s.createTestArtist("Drift Band")
	showID := s.createTestShow("Drift Show", []uint{artistID}, []uint{venue.ID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.sceneLogCount(userID, showID))
}

// =============================================================================
// EMAIL OPT-IN (PSY-1926)
// =============================================================================

// The violation this ticket exists to fix, stated as a test: a fresh scene
// follow gets the in-app row and NO mail. Before PSY-1926 the only gate on the
// email was whether a provider was configured, which this suite always is.
//
// It is also the "existing rows align to off" claim in executable form. The
// follow here stores nothing but a mode, which is exactly the shape every
// scene follow in production carries, and nothing had to be migrated for it to
// resolve to off.
func (s *NotificationFilterSuite) TestSceneAlert_EmailIsOffUntilOptedIn() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")

	artistID := s.createTestArtist("Unasked Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Unasked Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.sceneLogCount(userID, showID),
		"in-app delivery is unchanged: the bell row is still written")
	s.Empty(capture.sent, "email is an intentional opt-in on every alert type")
}

// The ACCOUNT matrix is the opt-in a user actually has today (the settings
// card's email box), and it must reach a follow that overrode nothing.
func (s *NotificationFilterSuite) TestSceneAlert_AccountOptInSendsWithWorkingUnsubscribe() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")
	s.setAccountShowEmail(userID, true)

	artistID := s.createTestArtist("Asked Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Asked Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.sceneLogCount(userID, showID))
	s.Require().Len(capture.sent, 1)
	sent := capture.sent[0]
	s.Contains(sent.subject, "New show in Phoenix, AZ")

	// The RFC 8058 requirement: the header value points at the BACKEND route
	// that serves the one-click POST, not at a frontend page that redirects.
	s.Contains(sent.unsubscribeURL, "/unsubscribe/"+engagement.UnsubscribeScopeArtistShowAlerts)
	s.NotContains(sent.unsubscribeURL, "/following",
		"the old target redirected to the library and could not honour the POST")
	// Same endpoint in the header (raw, per RFC 2369) and in the body (HTML
	// attribute-escaped). A recipient and a mailbox provider get one way out.
	s.Contains(sent.html, `href="`+htmlEscape(sent.unsubscribeURL)+`"`,
		"the header URL and the in-body link must be the same endpoint")
	// The filter template's copy does not belong on this message: the reader
	// authored no query and has no filter to pause.
	s.NotContains(sent.html, "Pause this filter")
	s.NotContains(sent.html, "New show matching")
}

// A per-follow override sits BELOW the account default, which is why the
// unsubscribe has to sweep the follows as well as write the account row.
func (s *NotificationFilterSuite) TestSceneAlert_PerFollowOverrideBeatsTheAccount() {
	capture := s.withCapturedEmail()

	optedIn := s.createTestUser()
	s.seedSceneFollowWithSettings(optedIn, `{"alerts":{"shows":{"email":true}}}`)

	silenced := s.createTestUser()
	s.seedSceneFollowWithSettings(silenced, `{"alerts":{"shows":{"email":false}}}`)
	s.setAccountShowEmail(silenced, true)

	artistID := s.createTestArtist("Override Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Override Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.sceneLogCount(optedIn, showID))
	s.Equal(int64(1), s.sceneLogCount(silenced, showID))
	s.Require().Len(capture.sent, 1,
		"only the follow that opted in is mailed")
}

// The mode is the WHICH-SHOWS axis and still outranks the channels: an "off"
// follow is no place to read an email opt-in from, so a user who silenced the
// scene cannot be mailed by switching a channel on somewhere else.
func (s *NotificationFilterSuite) TestSceneAlert_OffModeIsNotAnEmailOptIn() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	s.seedSceneFollowWithSettings(userID,
		`{"scene_notify_mode":"off","alerts":{"shows":{"email":true}}}`)

	artistID := s.createTestArtist("Silent Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Silent Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(0), s.sceneLogCount(userID, showID))
	s.Empty(capture.sent)
}

// Clearing the account default with both halves of the shipped unsubscribe is
// what the emailed link does, and the next show must then be silent. This is
// the acceptance criterion the broken link could never meet.
func (s *NotificationFilterSuite) TestSceneAlert_UnsubscribeStopsTheNextShow() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	s.seedSceneFollowWithSettings(userID, `{"alerts":{"shows":{"email":true}}}`)
	s.setAccountShowEmail(userID, true)

	artistID := s.createTestArtist("Leaving Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	firstShow := s.createTestShow("Before Unsubscribe", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(firstShow)))
	s.Require().Len(capture.sent, 1)

	// Both halves, in the order the unsubscribe endpoint performs them: the
	// account default, then the per-follow override that sits below it.
	s.setAccountShowEmail(userID, false)
	s.Require().NoError(engagement.NewFollowService(s.db).DisableFollowAlertEmailChannel(
		userID, "scene", contracts.FollowAlertTypeShows))

	secondShow := s.createTestShow("After Unsubscribe", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(secondShow)))

	s.Equal(int64(1), s.sceneLogCount(userID, secondShow),
		"an email opt-out is not a request to stop being notified in the product")
	s.Len(capture.sent, 1, "the unsubscribe has to stop the stream, not slow it")
}
