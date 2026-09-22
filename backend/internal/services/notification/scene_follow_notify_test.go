package notification

import (
	"errors"
	"html"
	"net/url"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
	usersvc "psychic-homily-backend/internal/services/user"
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
// settings document.
func (s *NotificationFilterSuite) seedSceneFollowWithSettings(userID uint, settingsJSON string) uint {
	sceneID := s.seedSceneFollow(userID, "")
	s.Require().NoError(s.db.Exec(`
		UPDATE user_bookmarks SET settings = ?::jsonb
		WHERE user_id = ? AND entity_type = 'scene' AND entity_id = ? AND action = 'follow'`,
		settingsJSON, userID, sceneID).Error)
	return sceneID
}

// setAccountShowEmail writes the account alert matrix's show-alert email
// channel, the one scene emails read.
func (s *NotificationFilterSuite) setAccountShowEmail(userID uint, on bool) {
	s.Require().NoError(usersvc.NewUserService(s.db).SetAccountAlertDefaults(userID,
		authm.AccountAlertDefaultsUpdate{Shows: &authm.AlertChannelDefaultsUpdate{Email: &on}}))
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
// EMAIL OPT-IN
// =============================================================================

// A scene follow with no account opt-in gets the in-app row and NO mail. A user
// with no preferences row resolves to email off, so existing follows need no
// migration to align.
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

// The account matrix's show-alert email box is the opt-in, and the message it
// sends carries a working RFC 8058 unsubscribe.
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
	s.Equal("New show in Phoenix, AZ", sent.subject)

	// The header target is the BACKEND route that serves the one-click POST,
	// signed for this user under the shared show-alert scope.
	u, err := url.Parse(sent.unsubscribeURL)
	s.Require().NoError(err)
	s.Equal("/unsubscribe/"+engagement.UnsubscribeScopeArtistShowAlerts, u.Path)
	s.Equal(strconv.FormatUint(uint64(userID), 10), u.Query().Get("uid"))
	s.True(engagement.VerifyScopedUnsubscribeSignature(
		userID, engagement.UnsubscribeScopeArtistShowAlerts, u.Query().Get("sig"), s.svc.jwtSecret),
		"the link must verify, or the recipient's click 403s at the door")

	// One way out for the recipient and the mailbox provider alike.
	s.Contains(sent.html, `href="`+html.EscapeString(sent.unsubscribeURL)+`"`)
	s.Contains(sent.html, "You are getting this because you follow Phoenix, AZ and show alert emails are on in your settings.")
	s.NotContains(sent.html, "Pause this filter")
	s.NotContains(sent.html, "New show matching")
}

// Scene follows have no per-follow alert layer, so an alerts document on the
// follow row is not an opt-in. The account row is the whole gate, which is what
// lets the unsubscribe's account write silence this stream by itself.
func (s *NotificationFilterSuite) TestSceneAlert_StoredFollowOverrideIsNotAnOptIn() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	s.seedSceneFollowWithSettings(userID, `{"alerts":{"shows":{"email":true}}}`)

	artistID := s.createTestArtist("Override Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Override Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.sceneLogCount(userID, showID))
	s.Empty(capture.sent)
}

// Mode "off" outranks the email opt-in: a silenced scene sends nothing.
func (s *NotificationFilterSuite) TestSceneAlert_OffModeIsNotAnEmailOptIn() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	s.seedSceneFollow(userID, "off")
	s.setAccountShowEmail(userID, true)

	artistID := s.createTestArtist("Silent Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Silent Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(0), s.sceneLogCount(userID, showID))
	s.Empty(capture.sent)
}

// The email says "you follow X", so X must be a scene whose
// follow made the user qualify. Phoenix is inserted first (the lower id) and is
// set to "off"; naming it would put a silenced scene in the message.
func (s *NotificationFilterSuite) TestSceneAlert_EmailNamesAQualifyingScene() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	s.seedSceneFollow(userID, "off") // phoenix-az
	s.setAccountShowEmail(userID, true)

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
	showID := s.createTestShow("Two City Show", []uint{artistID}, []uint{phxVenue, tucsonVenue.ID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Require().Len(capture.sent, 1)
	s.Equal("New show in Tucson, AZ", capture.sent[0].subject)
}

// The real unsubscribe mutation, then the next show: the in-app row still
// arrives and the email does not.
func (s *NotificationFilterSuite) TestSceneAlert_UnsubscribeStopsTheNextShow() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")
	s.setAccountShowEmail(userID, true)

	artistID := s.createTestArtist("Leaving Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	firstShow := s.createTestShow("Before Unsubscribe", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(firstShow)))
	s.Require().Len(capture.sent, 1)

	s.Require().NoError(usersvc.NewUserService(s.db).UnsubscribeArtistShowAlertEmails(userID))

	secondShow := s.createTestShow("After Unsubscribe", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(secondShow)))

	s.Equal(int64(1), s.sceneLogCount(userID, secondShow),
		"an email opt-out is not a request to stop being notified in the product")
	s.Len(capture.sent, 1, "the unsubscribe has to stop the stream")
}

// An unreadable account matrix sends no scene email, and still writes every
// in-app row: the matrix gates only the email.
func (s *NotificationFilterSuite) TestSceneAlert_UnreadablePreferencesStillDeliverInApp() {
	capture := &capturingEmailService{}

	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	failing, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	s.Require().NoError(err)
	s.Require().NoError(failing.Callback().Row().Before("gorm:row").
		Register("psy_fail_user_preferences", func(tx *gorm.DB) {
			if tx.Statement.Table == "user_preferences" {
				_ = tx.AddError(errors.New("user_preferences unavailable"))
			}
		}))
	svc := NewNotificationFilterService(failing, capture, "test-secret", "http://localhost:3000")

	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")
	s.setAccountShowEmail(userID, true)

	artistID := s.createTestArtist("Unreadable Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Unreadable Show", []uint{artistID}, []uint{venueID})

	svc.notifySceneFollowers(s.loadShow(showID), nil)

	s.Equal(int64(1), s.sceneLogCount(userID, showID), "the bell row does not depend on the matrix")
	s.Empty(capture.sent, "an unread opt-in is not an opt-in")
}

// An "all" follow outranks a followed-bands follow when naming the scene, even
// when the followed-bands scene has the lower id and arrives first.
func (s *NotificationFilterSuite) TestSceneAlert_EmailPrefersTheAllModeScene() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	s.seedSceneFollow(userID, "followed_bands_only") // phoenix-az, lower id
	s.setAccountShowEmail(userID, true)

	var tucsonID uint
	s.Require().NoError(s.db.Raw(`
		INSERT INTO scenes (metro, city, state, slug)
		VALUES (NULL, 'Tucson', 'AZ', 'tucson-az') RETURNING id`).Scan(&tucsonID).Error)
	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at)
		VALUES (?, 'scene', ?, 'follow', now())`, userID, tucsonID).Error)

	artistID := s.createTestArtist("Priority Band")
	phxVenue := s.createTestVenue("The Rebel Lounge")
	tucsonVenue := catalogm.Venue{Name: "Club Congress", City: "Tucson", State: "AZ"}
	s.Require().NoError(s.db.Create(&tucsonVenue).Error)
	showID := s.createTestShow("Priority Show", []uint{artistID}, []uint{phxVenue, tucsonVenue.ID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Require().Len(capture.sent, 1)
	s.Equal("New show in Tucson, AZ", capture.sent[0].subject)
}

// The scene email gate reads the account matrix alone, which is sound only
// while no scene follow can store a per-follow override. If this starts
// passing a write through, route notifySceneFollowers through
// engagement.ResolveFollowAlerts and add scenes to the unsubscribe sweep in
// user.UnsubscribeArtistShowAlertEmails before changing this assertion.
func (s *NotificationFilterSuite) TestSceneAlert_SceneFollowsCannotStoreAnOverride() {
	userID := s.createTestUser()
	sceneID := s.seedSceneFollow(userID, "")
	on := true

	_, err := engagement.NewFollowService(s.db).SetFollowAlertSettings(userID, "scene", sceneID,
		contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{Email: &on}})

	s.ErrorContains(err, "invalid entity type for follow")
}

// The daily cap allows maxFilterEmailsPerDay emails. The row this pass writes
// for the show is not one of them, so a user with one fewer prior row still
// gets this email, and a user at the cap does not.
func (s *NotificationFilterSuite) TestSceneAlert_DailyBudgetBoundary() {
	capture := s.withCapturedEmail()

	seedPriorRows := func(userID uint, n int) {
		for i := 0; i < n; i++ {
			s.Require().NoError(s.db.Create(&notificationm.NotificationLog{
				UserID:     userID,
				EntityType: notificationm.NotificationEntityShow,
				EntityID:   uint(900000 + i),
				Channel:    notificationm.NotificationChannelEmail,
				SentAt:     time.Now().UTC().Add(-time.Hour),
			}).Error)
		}
	}

	underCap := s.createTestUser()
	s.seedSceneFollow(underCap, "")
	s.setAccountShowEmail(underCap, true)
	seedPriorRows(underCap, maxFilterEmailsPerDay-1)

	atCap := s.createTestUser()
	s.seedSceneFollow(atCap, "")
	s.setAccountShowEmail(atCap, true)
	seedPriorRows(atCap, maxFilterEmailsPerDay)

	artistID := s.createTestArtist("Budget Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Budget Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.sceneLogCount(underCap, showID))
	s.Equal(int64(1), s.sceneLogCount(atCap, showID))
	s.Require().Len(capture.sent, 1, "only the user under the cap is mailed")
	var underCapEmail string
	s.Require().NoError(s.db.Table("users").Where("id = ?", underCap).Pluck("email", &underCapEmail).Error)
	s.Equal(underCapEmail, capture.sent[0].to)
}
