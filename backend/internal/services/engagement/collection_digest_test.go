package engagement

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// Unit tests — no DB required
// =============================================================================

// PSY-1940 repointed this at the canonical public chain. Two behaviour changes
// are pinned below: display_name now wins (it did not exist when this function
// was written), and an adder with no public name is "a contributor" rather than
// the local part of their EMAIL ADDRESS — which used to be mailed to a
// different person, since the candidate query excludes the recipient from the
// adders it reports.
func TestDigestDisplayName(t *testing.T) {
	adder := func(u *authm.User) *authm.User {
		u.ID = 1
		return u
	}

	t.Run("display name preferred", func(t *testing.T) {
		assert.Equal(t, "Alice A.", digestDisplayName(adder(&authm.User{
			DisplayName: strPtr("Alice A."),
			Username:    strPtr("alice"),
			FirstName:   strPtr("Alice"),
		})))
	})
	t.Run("username next", func(t *testing.T) {
		assert.Equal(t, "alice", digestDisplayName(adder(&authm.User{
			Username:  strPtr("alice"),
			FirstName: strPtr("Alice"),
		})))
	})
	t.Run("first and last name fallback", func(t *testing.T) {
		assert.Equal(t, "Alice Adams", digestDisplayName(adder(&authm.User{
			FirstName: strPtr("Alice"),
			LastName:  strPtr("Adams"),
		})))
	})
	t.Run("email is never rendered", func(t *testing.T) {
		got := digestDisplayName(adder(&authm.User{Email: strPtr("alice@example.com")}))
		assert.Equal(t, "a contributor", got)
		assert.NotContains(t, got, "alice", "email local-part leaked into a digest email")
	})
	// Adding a collection item counts toward TotalContributions on the profile,
	// so this byline is under the contributions setting — and it mails a third
	// party's name outward, which makes it the surface where getting that wrong
	// costs the most.
	t.Run("hidden contributions are not named", func(t *testing.T) {
		hidden := contracts.DefaultPrivacySettings()
		hidden.Contributions = contracts.PrivacyHidden
		encoded, err := json.Marshal(hidden)
		require.NoError(t, err)
		raw := json.RawMessage(encoded)

		got := digestDisplayName(adder(&authm.User{
			DisplayName:     strPtr("Alice A."),
			Username:        strPtr("alice"),
			PrivacySettings: &raw,
		}))
		assert.Equal(t, "a contributor", got)
		assert.NotContains(t, got, "Alice")
		assert.NotContains(t, got, "alice")
	})
	// A real name that happens to BE the chain's terminal. Detecting the
	// terminal by string-comparing the resolved name would erase this person.
	t.Run("a user actually named Anonymous is still credited", func(t *testing.T) {
		assert.Equal(t, "Anonymous", digestDisplayName(adder(&authm.User{
			DisplayName: strPtr("Anonymous"),
		})))
	})
	t.Run("nil user", func(t *testing.T) {
		assert.Equal(t, "a contributor", digestDisplayName(nil))
	})
	t.Run("adder row is gone", func(t *testing.T) {
		// A LEFT JOIN that matched nothing: every column NULL.
		assert.Equal(t, "a contributor", digestDisplayName(adder(&authm.User{})))
	})
	t.Run("empty and whitespace strings", func(t *testing.T) {
		empty := ""
		blank := "   "
		assert.Equal(t, "a contributor", digestDisplayName(adder(&authm.User{
			Username: &empty, DisplayName: &blank, FirstName: &blank,
		})))
	})
}

func TestHMAC_CollectionDigestRoundTrip(t *testing.T) {
	const (
		baseURL = "http://localhost:3000"
		secret  = "s3cr3t"
		userID  = uint(99)
	)

	urlStr := GenerateCollectionDigestUnsubscribeURL(baseURL, userID, secret)
	assert.Contains(t, urlStr, "/unsubscribe/collection-digest")
	assert.Contains(t, urlStr, "uid=99")

	sig := ComputeScopedUnsubscribeSignature(userID, UnsubscribeScopeCollectionDigest, secret)
	assert.True(t, VerifyScopedUnsubscribeSignature(userID, UnsubscribeScopeCollectionDigest, sig, secret))

	// Tamper: wrong user ID
	assert.False(t, VerifyScopedUnsubscribeSignature(userID+1, UnsubscribeScopeCollectionDigest, sig, secret))
	// Tamper: wrong secret
	assert.False(t, VerifyScopedUnsubscribeSignature(userID, UnsubscribeScopeCollectionDigest, sig, "different"))
	// Empty sig
	assert.False(t, VerifyScopedUnsubscribeSignature(userID, UnsubscribeScopeCollectionDigest, "", secret))

	// A signature minted for a *mention* unsubscribe must NOT verify against
	// the collection-digest scheme — defense-in-depth.
	mentionSig := ComputeScopedUnsubscribeSignature(userID, UnsubscribeScopeMention, secret)
	assert.False(t, VerifyScopedUnsubscribeSignature(userID, UnsubscribeScopeCollectionDigest, mentionSig, secret))
}

// TestCollectionDigestService_Construction_DefaultInterval verifies the
// constructor sets sane defaults and respects env overrides. We can't safely
// drive RunDigestCycleNow with a nil DB because GORM's Raw() panics — that
// would happen at startup anyway, and the constructor falls back to
// db.GetDB() when passed nil to avoid that path.
func TestCollectionDigestService_Construction_DefaultInterval(t *testing.T) {
	cfg := &config.Config{}
	cfg.Email.FrontendURL = "http://localhost:3000"
	cfg.JWT.SecretKey = "test"
	// Pass a non-nil zero-value gorm.DB to satisfy the constructor; the
	// service is not started, so no DB work happens.
	svc := NewCollectionDigestService(&gorm.DB{}, nil, cfg)
	require.NotNil(t, svc)
	assert.Equal(t, DefaultCollectionDigestInterval, svc.interval)
	assert.Equal(t, "http://localhost:3000", svc.frontendURL)
	assert.Equal(t, "test", svc.jwtSecret)
}

func TestCollectionDigestService_Construction_EnvOverride(t *testing.T) {
	t.Setenv("COLLECTION_DIGEST_INTERVAL_HOURS", "6")
	cfg := &config.Config{}
	cfg.Email.FrontendURL = "http://localhost:3000"
	cfg.JWT.SecretKey = "test"
	svc := NewCollectionDigestService(&gorm.DB{}, nil, cfg)
	assert.Equal(t, 6*time.Hour, svc.interval, "env override should be applied")
}

// =============================================================================
// Integration test scaffolding
// =============================================================================

// captureDigestEmailService records SendCollectionDigestEmail calls. Other
// methods are no-ops because the digest test exercises only that surface.
type captureDigestEmailService struct {
	mu         sync.Mutex
	calls      []digestEmailCall
	fail       bool
	configured bool
}

type digestEmailCall struct {
	ToEmail        string
	Groups         []contracts.CollectionDigestGroup
	UnsubscribeURL string
}

func (m *captureDigestEmailService) IsConfigured() bool {
	return m.configured
}

func (m *captureDigestEmailService) SendVerificationEmail(_, _ string) error { return nil }
func (m *captureDigestEmailService) SendMagicLinkEmail(_, _ string) error    { return nil }
func (m *captureDigestEmailService) SendAccountRecoveryEmail(_, _ string, _ int) error {
	return nil
}
func (m *captureDigestEmailService) SendShowReminderEmail(_, _, _, _ string, _ time.Time, _ []string) error {
	return nil
}
func (m *captureDigestEmailService) SendFilterNotificationEmail(_, _, _, _ string) error { return nil }
func (m *captureDigestEmailService) SendTierPromotionEmail(_, _, _, _, _, _ string, _ []string) error {
	return nil
}
func (m *captureDigestEmailService) SendTierDemotionEmail(_, _, _, _, _, _ string) error { return nil }
func (m *captureDigestEmailService) SendTierDemotionWarningEmail(_, _, _ string, _, _ float64, _ string) error {
	return nil
}
func (m *captureDigestEmailService) SendEditApprovedEmail(_, _, _, _, _, _ string) error { return nil }
func (m *captureDigestEmailService) SendEditRejectedEmail(_, _, _, _, _, _ string) error { return nil }
func (m *captureDigestEmailService) SendCommentNotification(_, _, _, _, _, _, _ string) error {
	return nil
}
func (m *captureDigestEmailService) SendMentionNotification(_, _, _, _, _, _, _ string) error {
	return nil
}

func (m *captureDigestEmailService) SendSceneDigestEmail(_ string, _ []contracts.SceneDigestGroup, _ string) error {
	return nil
}

func (m *captureDigestEmailService) SendCollectionDigestEmail(toEmail string, groups []contracts.CollectionDigestGroup, unsubscribeURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return fmt.Errorf("forced failure")
	}
	// Take a defensive copy of groups so the caller can mutate after sending.
	cp := make([]contracts.CollectionDigestGroup, len(groups))
	for i, g := range groups {
		items := make([]contracts.CollectionDigestEntry, len(g.Items))
		copy(items, g.Items)
		cp[i] = contracts.CollectionDigestGroup{
			CollectionTitle: g.CollectionTitle,
			CollectionURL:   g.CollectionURL,
			Items:           items,
		}
	}
	m.calls = append(m.calls, digestEmailCall{ToEmail: toEmail, Groups: cp, UnsubscribeURL: unsubscribeURL})
	return nil
}

// CollectionDigestServiceIntegrationSuite drives the digest service against a
// real Postgres via testcontainers.
type CollectionDigestServiceIntegrationSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	mock   *captureDigestEmailService
	svc    *CollectionDigestService
}

func (s *CollectionDigestServiceIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *CollectionDigestServiceIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *CollectionDigestServiceIntegrationSuite) SetupTest() {
	s.mock = &captureDigestEmailService{configured: true}
	cfg := &config.Config{}
	cfg.Email.FrontendURL = "http://localhost:3000"
	cfg.JWT.SecretKey = "test-secret"
	s.svc = NewCollectionDigestService(s.db, s.mock, cfg)
}

func (s *CollectionDigestServiceIntegrationSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM collection_subscribers")
	_, _ = sqlDB.Exec("DELETE FROM collection_items")
	_, _ = sqlDB.Exec("DELETE FROM collections")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestCollectionDigestServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(CollectionDigestServiceIntegrationSuite))
}

// -- helpers

// digestUserCounter ensures unique usernames within a test even when wall
// clock times collide (testcontainers can run sub-millisecond apart).
var digestUserCounter int64

func (s *CollectionDigestServiceIntegrationSuite) createUser(username string) *authm.User {
	digestUserCounter++
	email := fmt.Sprintf("%s-%d-%d@test.local", username, time.Now().UnixNano(), digestUserCounter)
	un := username
	fn := "First"
	u := &authm.User{
		Email:         &email,
		Username:      &un,
		FirstName:     &fn,
		IsActive:      true,
		EmailVerified: true,
		UserTier:      "contributor",
	}
	s.Require().NoError(s.db.Create(u).Error)
	return u
}

// createUserWithDigestPref creates a user with explicit notify_on_collection_digest.
// PSY-350: column default is FALSE (opt-IN). When `notify=true`, GORM writes
// the non-zero value directly; when `notify=false`, GORM skips the column and
// the DB default (false) wins.
func (s *CollectionDigestServiceIntegrationSuite) createUserWithDigestPref(username string, notify bool) *authm.User {
	u := s.createUser(username)
	prefs := &authm.UserPreferences{
		UserID:                      u.ID,
		NotifyOnCommentSubscription: true,
		NotifyOnMention:             true,
		NotifyOnCollectionDigest:    notify,
	}
	s.Require().NoError(s.db.Create(prefs).Error)
	return u
}

func (s *CollectionDigestServiceIntegrationSuite) createCollection(creator *authm.User, title, slug string) *communitym.Collection {
	coll := &communitym.Collection{
		Title:         title,
		Slug:          slug,
		Description:   "test",
		CreatorID:     creator.ID,
		Collaborative: true,
		IsPublic:      true,
	}
	s.Require().NoError(s.db.Create(coll).Error)
	return coll
}

// subscribe creates a CollectionSubscriber row. The subscription's created_at
// is forced to "2 hours ago" so test items added with "1 hour ago" timestamps
// fall *after* the effective digest cursor (which is GREATEST(last_digest, sub.created_at)).
func (s *CollectionDigestServiceIntegrationSuite) subscribe(coll *communitym.Collection, user *authm.User, lastVisited *time.Time, lastDigest *time.Time) {
	sub := &communitym.CollectionSubscriber{
		CollectionID:     coll.ID,
		UserID:           user.ID,
		LastVisitedAt:    lastVisited,
		LastDigestSentAt: lastDigest,
	}
	s.Require().NoError(s.db.Create(sub).Error)
	// Backdate so items added "1 hour ago" in tests fall after the cursor.
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	s.Require().NoError(
		s.db.Model(&communitym.CollectionSubscriber{}).
			Where("collection_id = ? AND user_id = ?", coll.ID, user.ID).
			Update("created_at", twoHoursAgo).Error,
	)
}

func (s *CollectionDigestServiceIntegrationSuite) addItem(coll *communitym.Collection, addedBy *authm.User, entityType string, entityID uint, when time.Time) *communitym.CollectionItem {
	item := &communitym.CollectionItem{
		CollectionID:  coll.ID,
		EntityType:    entityType,
		EntityID:      entityID,
		Position:      0,
		AddedByUserID: addedBy.ID,
	}
	s.Require().NoError(s.db.Create(item).Error)
	// Force the created_at to a known value for deterministic windowing.
	s.Require().NoError(s.db.Model(item).Update("created_at", when).Error)
	return item
}

func (s *CollectionDigestServiceIntegrationSuite) createArtist(name string) *catalogm.Artist {
	slug := name
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a
}

// -- tests

// TestDigest_NoCandidates_NoEmails — happy path: nothing to send sends
// nothing and updates no cursors.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_NoCandidates_NoEmails() {
	creator := s.createUser("creator")
	subscriber := s.createUserWithDigestPref("sub", true)
	coll := s.createCollection(creator, "Empty", "empty")
	s.subscribe(coll, subscriber, nil, nil)

	s.svc.RunDigestCycleNow()

	assert.Empty(s.T(), s.mock.calls, "no items should mean no emails")
}

// TestDigest_OneItem_OneSubscriber_OneEmail — basic delivery.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_OneItem_OneSubscriber_OneEmail() {
	creator := s.createUser("creator")
	subscriber := s.createUserWithDigestPref("sub", true)
	coll := s.createCollection(creator, "C1", "c1")
	s.subscribe(coll, subscriber, nil, nil)

	a := s.createArtist("Artist1")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	s.svc.RunDigestCycleNow()

	require.Len(s.T(), s.mock.calls, 1)
	call := s.mock.calls[0]
	assert.NotEmpty(s.T(), call.ToEmail)
	require.Len(s.T(), call.Groups, 1)
	g := call.Groups[0]
	assert.Equal(s.T(), "C1", g.CollectionTitle)
	require.Len(s.T(), g.Items, 1)
	assert.Equal(s.T(), "Artist1", g.Items[0].EntityName)
	assert.Equal(s.T(), communitym.CollectionEntityArtist, g.Items[0].EntityType)
	// AddedBy is asserted THROUGH THE REAL QUERY, which is the only thing that
	// pins the candidate query's column list. Every unit test of
	// digestDisplayName builds the user in memory, so a mistyped SELECT alias
	// would scan every identity column as nil and quietly credit "a contributor"
	// in every outgoing digest, forever, with no error and no failing test.
	// createUser sets a username, so the resolved credit is that.
	require.NotNil(s.T(), creator.Username)
	assert.Equal(s.T(), *creator.Username, g.Items[0].AddedBy,
		"AddedBy did not resolve — check the added_by_* aliases in queryCandidates")
}

// The digest names a THIRD PARTY (the join excludes the recipient), and adding
// a collection item counts toward TotalContributions, so a contributor who hid
// their contributions must not be named in outgoing email. Asserted through the
// real query because the gate reads privacy_settings, a column the candidate
// query has to remember to select — the same fail-open a narrowed Select causes
// on the revision routes.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_HiddenContributorIsNotNamed() {
	creator := s.createUser("hidden-creator")
	hidden := contracts.DefaultPrivacySettings()
	hidden.Contributions = contracts.PrivacyHidden
	encoded, err := json.Marshal(hidden)
	s.Require().NoError(err)
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", creator.ID).
		Update("privacy_settings", string(encoded)).Error)
	// Read back: a fixture that failed to store the setting would make the gate
	// look correct for the wrong reason.
	var stored struct{ PrivacySettings *string }
	s.Require().NoError(s.db.Table("users").
		Select("privacy_settings::text AS privacy_settings").
		Where("id = ?", creator.ID).Scan(&stored).Error)
	s.Require().NotNil(stored.PrivacySettings, "fixture privacy_settings did not store")
	s.Require().Contains(*stored.PrivacySettings, "hidden", "fixture did not store hidden")

	subscriber := s.createUserWithDigestPref("sub", true)
	coll := s.createCollection(creator, "C1", "c1")
	s.subscribe(coll, subscriber, nil, nil)

	a := s.createArtist("Artist1")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	s.svc.RunDigestCycleNow()

	require.Len(s.T(), s.mock.calls, 1)
	g := s.mock.calls[0].Groups[0]
	require.Len(s.T(), g.Items, 1)
	assert.Equal(s.T(), "a contributor", g.Items[0].AddedBy)
	require.NotNil(s.T(), creator.Username)
	assert.NotContains(s.T(), g.Items[0].AddedBy, *creator.Username)
	// The item itself still goes out. The gate hides the person, not the news.
	assert.Equal(s.T(), "Artist1", g.Items[0].EntityName)
}

// TestDigest_AdderExcluded — the user who added the item should not receive
// an email for their own addition.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_AdderExcluded() {
	adder := s.createUserWithDigestPref("adder", true)
	other := s.createUserWithDigestPref("other", true)
	coll := s.createCollection(adder, "C1", "c1")
	// Both users are subscribed (creator auto-subscribed by usual flow; here we
	// subscribe both manually since we're not going through the service).
	s.subscribe(coll, adder, nil, nil)
	s.subscribe(coll, other, nil, nil)

	a := s.createArtist("Artist1")
	s.addItem(coll, adder, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	s.svc.RunDigestCycleNow()

	// Only the OTHER subscriber should receive an email; adder should not.
	require.Len(s.T(), s.mock.calls, 1)
	assert.Contains(s.T(), s.mock.calls[0].ToEmail, "other-")
}

// TestDigest_RespectsPreference — users with notify_on_collection_digest=false
// must not receive emails.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_RespectsPreference() {
	creator := s.createUser("creator")
	noNotify := s.createUserWithDigestPref("no-notify", false)
	coll := s.createCollection(creator, "C1", "c1")
	s.subscribe(coll, noNotify, nil, nil)

	a := s.createArtist("Artist1")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	s.svc.RunDigestCycleNow()
	assert.Empty(s.T(), s.mock.calls, "user with notify off should receive nothing")
}

// TestDigest_IdempotentAcrossCycles — the second cycle right after the first
// must send nothing because cursors moved.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_IdempotentAcrossCycles() {
	creator := s.createUser("creator")
	sub := s.createUserWithDigestPref("sub", true)
	coll := s.createCollection(creator, "C1", "c1")
	s.subscribe(coll, sub, nil, nil)

	a := s.createArtist("Artist1")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	s.svc.RunDigestCycleNow()
	require.Len(s.T(), s.mock.calls, 1)

	// Second run, no new items: no new email.
	s.svc.RunDigestCycleNow()
	require.Len(s.T(), s.mock.calls, 1, "second cycle must not duplicate the email")

	// Sleep briefly so wall-clock advances past the just-set last_digest_sent_at
	// (which was set to the first cycle's `now`). Then add a new item with
	// the current time — it will be after the cursor and before the next
	// cycle's `now`.
	time.Sleep(50 * time.Millisecond)
	a2 := s.createArtist("Artist2")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a2.ID, time.Now())
	s.svc.RunDigestCycleNow()
	require.Len(s.T(), s.mock.calls, 2, "new item after cursor must trigger a new email")
	require.Len(s.T(), s.mock.calls[1].Groups, 1)
	require.Len(s.T(), s.mock.calls[1].Groups[0].Items, 1)
	assert.Equal(s.T(), "Artist2", s.mock.calls[1].Groups[0].Items[0].EntityName)
}

// TestDigest_MultipleCollectionsGroupedByCollection — one user, two
// subscribed collections, gets one email with two groups.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_MultipleCollectionsGroupedByCollection() {
	creator := s.createUser("creator")
	sub := s.createUserWithDigestPref("sub", true)
	c1 := s.createCollection(creator, "C1", "c1")
	c2 := s.createCollection(creator, "C2", "c2")
	s.subscribe(c1, sub, nil, nil)
	s.subscribe(c2, sub, nil, nil)

	a1 := s.createArtist("A1")
	a2 := s.createArtist("A2")
	s.addItem(c1, creator, communitym.CollectionEntityArtist, a1.ID, time.Now().Add(-2*time.Hour))
	s.addItem(c2, creator, communitym.CollectionEntityArtist, a2.ID, time.Now().Add(-1*time.Hour))

	s.svc.RunDigestCycleNow()
	require.Len(s.T(), s.mock.calls, 1, "one user -> one email")
	require.Len(s.T(), s.mock.calls[0].Groups, 2, "two collections -> two groups")
}

// TestDigest_SkipsInactiveOrDeletedUsers — inactive/deleted users must not
// receive emails even if they're still subscribed.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_SkipsInactiveOrDeletedUsers() {
	creator := s.createUser("creator")
	inactive := s.createUserWithDigestPref("inactive", true)
	deleted := s.createUserWithDigestPref("deleted", true)
	good := s.createUserWithDigestPref("good", true)

	// Mark inactive
	s.Require().NoError(s.db.Model(inactive).Update("is_active", false).Error)
	// Mark deleted (soft delete)
	now := time.Now()
	s.Require().NoError(s.db.Model(deleted).Update("deleted_at", &now).Error)

	coll := s.createCollection(creator, "C1", "c1")
	s.subscribe(coll, inactive, nil, nil)
	s.subscribe(coll, deleted, nil, nil)
	s.subscribe(coll, good, nil, nil)

	a := s.createArtist("Artist1")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	s.svc.RunDigestCycleNow()
	require.Len(s.T(), s.mock.calls, 1, "only the active+non-deleted user should be emailed")
	assert.Contains(s.T(), s.mock.calls[0].ToEmail, "good-")
}

// TestDigest_UnsubscribeStopsImmediately — after unsubscribing (deleting
// the row), the next cycle excludes that user even if items exist.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_UnsubscribeStopsImmediately() {
	creator := s.createUser("creator")
	sub := s.createUserWithDigestPref("sub", true)
	coll := s.createCollection(creator, "C1", "c1")
	s.subscribe(coll, sub, nil, nil)

	a := s.createArtist("Artist1")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	// Unsubscribe BEFORE cycling.
	s.Require().NoError(
		s.db.Where("collection_id = ? AND user_id = ?", coll.ID, sub.ID).
			Delete(&communitym.CollectionSubscriber{}).Error,
	)

	s.svc.RunDigestCycleNow()
	assert.Empty(s.T(), s.mock.calls, "unsubscribed user must not be emailed")
}

// TestDigest_RespectsCursorBoundaries — items added BEFORE the existing
// last_digest_sent_at must not be re-included.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_RespectsCursorBoundaries() {
	creator := s.createUser("creator")
	sub := s.createUserWithDigestPref("sub", true)
	coll := s.createCollection(creator, "C1", "c1")

	// Subscriber's cursor is at "now - 30 minutes".
	cursor := time.Now().Add(-30 * time.Minute)
	s.subscribe(coll, sub, nil, &cursor)

	// Item added 1 HOUR ago — *before* the cursor.
	a := s.createArtist("Artist1")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	s.svc.RunDigestCycleNow()
	assert.Empty(s.T(), s.mock.calls, "item before cursor must not be included")
}

// TestDigest_BumpsCursorOnSuccess — the per-subscription cursor advances after
// a successful send so subsequent cycles don't re-include the same items.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_BumpsCursorOnSuccess() {
	creator := s.createUser("creator")
	sub := s.createUserWithDigestPref("sub", true)
	coll := s.createCollection(creator, "C1", "c1")
	s.subscribe(coll, sub, nil, nil)

	a := s.createArtist("Artist1")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	s.svc.RunDigestCycleNow()
	require.Len(s.T(), s.mock.calls, 1)

	// Verify cursor moved.
	var subRow communitym.CollectionSubscriber
	s.Require().NoError(
		s.db.Where("collection_id = ? AND user_id = ?", coll.ID, sub.ID).
			First(&subRow).Error,
	)
	require.NotNil(s.T(), subRow.LastDigestSentAt, "cursor must be set after send")
}

// TestDigest_FailedSendDoesNotBumpCursor — if SendCollectionDigestEmail fails,
// the cursor stays put so we retry next cycle.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_FailedSendDoesNotBumpCursor() {
	creator := s.createUser("creator")
	sub := s.createUserWithDigestPref("sub", true)
	coll := s.createCollection(creator, "C1", "c1")
	s.subscribe(coll, sub, nil, nil)

	a := s.createArtist("Artist1")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	s.mock.fail = true
	s.svc.RunDigestCycleNow()
	assert.Empty(s.T(), s.mock.calls, "forced failure should record no successful calls")

	var subRow communitym.CollectionSubscriber
	s.Require().NoError(
		s.db.Where("collection_id = ? AND user_id = ?", coll.ID, sub.ID).
			First(&subRow).Error,
	)
	assert.Nil(s.T(), subRow.LastDigestSentAt, "cursor must NOT advance on send failure")

	// Now allow the send and re-run — we should get the email this time.
	s.mock.fail = false
	s.svc.RunDigestCycleNow()
	require.Len(s.T(), s.mock.calls, 1)
}

// TestDigest_PrefDefaultsToFalse — a subscriber with no user_preferences row
// at all should NOT receive the digest. PSY-350 hardening: the column default
// is FALSE (opt-IN), so an absent prefs row COALESCEs to FALSE and is excluded.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_PrefDefaultsToFalse() {
	creator := s.createUser("creator")
	sub := s.createUser("sub-no-prefs") // no preferences row created
	coll := s.createCollection(creator, "C1", "c1")
	s.subscribe(coll, sub, nil, nil)

	a := s.createArtist("Artist1")
	s.addItem(coll, creator, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	s.svc.RunDigestCycleNow()
	assert.Empty(s.T(), s.mock.calls, "missing prefs row should default to NOT receiving digest")
}

// strPtr returns a pointer to s. Test helper.
func strPtr(s string) *string { return &s }

// A COLLECTION THAT GOES PRIVATE STOPS MAILING, and it stops mailing to the
// people already subscribed to it.
//
// The subscribe-time gate cannot cover this on its own: it decides the moment a
// row is made, and the collection's flag moves afterwards. Mail is final, so a
// cycle that read only the subscription would keep delivering a private
// collection's contents to a stranger who subscribed while it was public, one
// item summary at a time, forever.
//
// The creator is asserted in the same run: withholding from THEM would be a
// broken feature rather than a closed leak, and the two halves are what make
// this about visibility rather than about the query having stopped working.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_PrivateCollectionMailsOnlyItsCreator() {
	creator := s.createUserWithDigestPref("creator", true)
	stranger := s.createUserWithDigestPref("stranger", true)
	coll := s.createCollection(creator, "Goes Private", "goes-private")
	s.subscribe(coll, creator, nil, nil)
	s.subscribe(coll, stranger, nil, nil)

	// A third party adds the item, so neither subscriber is excluded by the
	// added_by_user_id <> cs.user_id join condition.
	adder := s.createUser("adder")
	a := s.createArtist("PrivateFlipArtist")
	s.addItem(coll, adder, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))

	// Public first: both subscribers are candidates, which is the control.
	s.svc.RunDigestCycleNow()
	require.Len(s.T(), s.mock.calls, 2,
		"while the collection is public both subscribers should be mailed")

	// Now flip it private, and reset the per-subscriber cursors the first cycle
	// stamped so the same item is a candidate again.
	s.Require().NoError(s.db.Model(&communitym.Collection{}).
		Where("id = ?", coll.ID).Update("is_public", false).Error)
	s.Require().NoError(s.db.Model(&communitym.CollectionSubscriber{}).
		Where("collection_id = ?", coll.ID).Update("last_digest_sent_at", nil).Error)
	s.mock.calls = nil

	s.svc.RunDigestCycleNow()

	require.Len(s.T(), s.mock.calls, 1,
		"a private collection must mail its creator and nobody else")
	s.Require().NotNil(creator.Email)
	assert.Equal(s.T(), *creator.Email, s.mock.calls[0].ToEmail,
		"the one remaining recipient must be the creator")
	s.Require().NotNil(stranger.Email)
	assert.NotEqual(s.T(), *stranger.Email, s.mock.calls[0].ToEmail)
}
