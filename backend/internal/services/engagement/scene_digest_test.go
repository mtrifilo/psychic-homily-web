package engagement

import (
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
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// Unit tests — no DB
// =============================================================================

func TestSceneDigestService_Construction(t *testing.T) {
	cfg := &config.Config{}
	cfg.Email.FrontendURL = "http://localhost:3000"
	cfg.JWT.SecretKey = "test"
	svc := NewSceneDigestService(&gorm.DB{}, nil, nil, cfg)
	require.NotNil(t, svc)
	assert.Equal(t, DefaultSceneDigestInterval, svc.interval)
	assert.Equal(t, "http://localhost:3000", svc.frontendURL)
}

func TestSceneDigestService_EnvOverride(t *testing.T) {
	t.Setenv("SCENE_DIGEST_INTERVAL_HOURS", "3")
	cfg := &config.Config{}
	cfg.Email.FrontendURL = "http://localhost:3000"
	cfg.JWT.SecretKey = "test"
	svc := NewSceneDigestService(&gorm.DB{}, nil, nil, cfg)
	assert.Equal(t, 3*time.Hour, svc.interval)
}

func TestSceneShowDisplayTitle(t *testing.T) {
	assert.Equal(t, "Real Title", sceneShowDisplayTitle("  Real Title ", []string{"A"}))
	assert.Equal(t, "A, B", sceneShowDisplayTitle("", []string{"A", "B"}))
	assert.Equal(t, "A, B, C +1 more", sceneShowDisplayTitle("   ", []string{"A", "B", "C", "D"}))
	assert.Equal(t, "Untitled Show", sceneShowDisplayTitle("", []string{"", "  "}))
	assert.Equal(t, "Untitled Show", sceneShowDisplayTitle("", nil))
}

func TestHMAC_SceneDigestScope(t *testing.T) {
	const secret, userID = "s3cr3t", uint(42)
	sig := ComputeScopedUnsubscribeSignature(userID, UnsubscribeScopeSceneDigest, secret)
	assert.True(t, VerifyScopedUnsubscribeSignature(userID, UnsubscribeScopeSceneDigest, sig, secret))
	// A collection-digest signature must NOT verify against the scene scope.
	other := ComputeScopedUnsubscribeSignature(userID, UnsubscribeScopeCollectionDigest, secret)
	assert.False(t, VerifyScopedUnsubscribeSignature(userID, UnsubscribeScopeSceneDigest, other, secret))
	url := GenerateScopedUnsubscribeURL("http://localhost:8080", userID, UnsubscribeScopeSceneDigest, secret)
	assert.Contains(t, url, "/unsubscribe/scene-digest")
	assert.Contains(t, url, "uid=42")
}

// =============================================================================
// Integration suite — real Postgres, real SceneService (venue/artist scope)
// =============================================================================

type captureSceneDigestEmailService struct {
	mu    sync.Mutex
	calls []sceneDigestEmailCall
}

type sceneDigestEmailCall struct {
	ToEmail string
	Groups  []contracts.SceneDigestGroup
	Unsub   string
}

func (m *captureSceneDigestEmailService) IsConfigured() bool { return true }
func (m *captureSceneDigestEmailService) SendSceneDigestEmail(to string, g []contracts.SceneDigestGroup, unsub string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]contracts.SceneDigestGroup, len(g))
	copy(cp, g)
	m.calls = append(m.calls, sceneDigestEmailCall{ToEmail: to, Groups: cp, Unsub: unsub})
	return nil
}

// Unused EmailServiceInterface surface.
func (m *captureSceneDigestEmailService) SendVerificationEmail(_, _ string) error { return nil }
func (m *captureSceneDigestEmailService) SendMagicLinkEmail(_, _ string) error    { return nil }
func (m *captureSceneDigestEmailService) SendAccountRecoveryEmail(_, _ string, _ int) error {
	return nil
}
func (m *captureSceneDigestEmailService) SendShowReminderEmail(_, _, _, _ string, _ time.Time, _ []string) error {
	return nil
}
func (m *captureSceneDigestEmailService) SendFilterNotificationEmail(_, _, _, _ string) error {
	return nil
}
func (m *captureSceneDigestEmailService) SendTierPromotionEmail(_, _, _, _, _, _ string, _ []string) error {
	return nil
}
func (m *captureSceneDigestEmailService) SendTierDemotionEmail(_, _, _, _, _, _ string) error {
	return nil
}
func (m *captureSceneDigestEmailService) SendTierDemotionWarningEmail(_, _, _ string, _, _ float64, _ string) error {
	return nil
}
func (m *captureSceneDigestEmailService) SendEditApprovedEmail(_, _, _, _, _, _ string) error {
	return nil
}
func (m *captureSceneDigestEmailService) SendEditRejectedEmail(_, _, _, _, _, _ string) error {
	return nil
}
func (m *captureSceneDigestEmailService) SendCommentNotification(_, _, _, _, _, _, _ string) error {
	return nil
}
func (m *captureSceneDigestEmailService) SendMentionNotification(_, _, _, _, _, _, _ string) error {
	return nil
}
func (m *captureSceneDigestEmailService) SendCollectionDigestEmail(_ string, _ []contracts.CollectionDigestGroup, _ string) error {
	return nil
}

type SceneDigestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	mock   *captureSceneDigestEmailService
	svc    *SceneDigestService
}

func TestSceneDigestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(SceneDigestSuite))
}

func (s *SceneDigestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *SceneDigestSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *SceneDigestSuite) SetupTest() {
	s.mock = &captureSceneDigestEmailService{}
	cfg := &config.Config{}
	cfg.Email.FrontendURL = "http://localhost:3000"
	cfg.JWT.SecretKey = "test-secret"
	s.svc = NewSceneDigestService(s.db, s.mock, catalog.NewSceneService(s.db), cfg)
}

func (s *SceneDigestSuite) TearDownTest() {
	for _, t := range []string{"show_venues", "show_artists", "shows", "artists", "venues", "scenes", "user_bookmarks", "user_preferences", "users"} {
		s.db.Exec("DELETE FROM " + t)
	}
}

// --- seed helpers (a fallback scene "Testville, ZZ" — ZZ has no CBSA, so the
// scope stays city/state and doesn't depend on the geocoder). ---

func (s *SceneDigestSuite) createUser() (uint, string) {
	email := fmt.Sprintf("u-%d@example.com", time.Now().UnixNano())
	u := authm.User{Email: &email}
	s.Require().NoError(s.db.Create(&u).Error)
	return u.ID, email
}

func (s *SceneDigestSuite) setSceneDigestPref(userID uint, enabled bool) {
	s.Require().NoError(s.db.Exec(
		`INSERT INTO user_preferences (user_id, notify_on_scene_digest) VALUES (?, ?)`,
		userID, enabled).Error)
}

// createScene makes the registry row + the 2 verified venues the scene needs
// to clear GetSceneUpcomingShows' existence gate. Returns (sceneID, venueID).
func (s *SceneDigestSuite) createScene() (uint, uint) {
	var sceneID uint
	s.Require().NoError(s.db.Raw(
		`INSERT INTO scenes (metro, city, state, slug) VALUES (NULL, 'Testville', 'ZZ', 'testville-zz') RETURNING id`).
		Scan(&sceneID).Error)
	var firstVenue uint
	for i := 0; i < 2; i++ {
		v := catalogm.Venue{Name: fmt.Sprintf("Venue %d", i), City: "Testville", State: "ZZ", Verified: true}
		s.Require().NoError(s.db.Create(&v).Error)
		s.db.Model(&v).Update("verified", true) // GORM bool zero-value gotcha
		if i == 0 {
			firstVenue = v.ID
		}
	}
	return sceneID, firstVenue
}

func (s *SceneDigestSuite) followScene(userID, sceneID uint, cursor *time.Time, followedAt time.Time) {
	if cursor == nil {
		s.Require().NoError(s.db.Exec(
			`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at) VALUES (?, 'scene', ?, 'follow', ?)`,
			userID, sceneID, followedAt).Error)
	} else {
		s.Require().NoError(s.db.Exec(
			`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at, scene_digest_sent_at) VALUES (?, 'scene', ?, 'follow', ?, ?)`,
			userID, sceneID, followedAt, *cursor).Error)
	}
}

func (s *SceneDigestSuite) createThisWeekShow(venueID uint) uint {
	slug := fmt.Sprintf("show-%d", time.Now().UnixNano())
	show := catalogm.Show{
		Title:     "Big Show",
		Slug:      &slug,
		EventDate: time.Now().UTC().Add(48 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
	}
	s.Require().NoError(s.db.Create(&show).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`, show.ID, venueID).Error)
	return show.ID
}

func (s *SceneDigestSuite) createArtist(name string, createdAt time.Time) {
	slug := name
	a := catalogm.Artist{Name: name, Slug: &slug, City: strPtr("Testville"), State: strPtr("ZZ"), CreatedAt: createdAt, UpdatedAt: createdAt}
	s.Require().NoError(s.db.Create(&a).Error)
}

func (s *SceneDigestSuite) cursorFor(userID, sceneID uint) *time.Time {
	var b struct{ SceneDigestSentAt *time.Time }
	s.Require().NoError(s.db.Raw(
		`SELECT scene_digest_sent_at FROM user_bookmarks WHERE user_id = ? AND entity_type = 'scene' AND entity_id = ?`,
		userID, sceneID).Scan(&b).Error)
	return b.SceneDigestSentAt
}

// --- tests ---

func (s *SceneDigestSuite) TestOptInGate() {
	userID, _ := s.createUser()
	sceneID, venueID := s.createScene()
	s.followScene(userID, sceneID, nil, time.Now().Add(-24*time.Hour))
	s.createThisWeekShow(venueID)

	// Pref OFF → no email even with content.
	s.setSceneDigestPref(userID, false)
	s.svc.RunDigestCycleNow()
	s.Empty(s.mock.calls, "opt-out user must not be emailed")

	// Pref ON → email.
	s.db.Exec(`UPDATE user_preferences SET notify_on_scene_digest = TRUE WHERE user_id = ?`, userID)
	s.svc.RunDigestCycleNow()
	s.Require().Len(s.mock.calls, 1)
	s.Require().Len(s.mock.calls[0].Groups, 1)
	s.Len(s.mock.calls[0].Groups[0].Shows, 1)
	s.Contains(s.mock.calls[0].Unsub, "/unsubscribe/scene-digest")
}

func (s *SceneDigestSuite) TestNewBandsSinceCursorAndIdempotency() {
	userID, _ := s.createUser()
	sceneID, _ := s.createScene()
	s.setSceneDigestPref(userID, true)
	followedAt := time.Now().Add(-72 * time.Hour)
	s.followScene(userID, sceneID, nil, followedAt)

	// One band created AFTER the follow (included), one BEFORE (excluded).
	s.createArtist("New Band", time.Now().Add(-24*time.Hour))
	s.createArtist("Old Band", time.Now().Add(-96*time.Hour))

	s.svc.RunDigestCycleNow()
	s.Require().Len(s.mock.calls, 1)
	s.Require().Len(s.mock.calls[0].Groups[0].NewArtists, 1)
	s.Equal("New Band", s.mock.calls[0].Groups[0].NewArtists[0].Name)
	s.NotNil(s.cursorFor(userID, sceneID), "cursor must advance after a send")

	// Idempotent: a second run has no new content past the cursor → no email.
	s.mock.calls = nil
	s.svc.RunDigestCycleNow()
	s.Empty(s.mock.calls, "second run must send nothing (cursor advanced)")
}

func (s *SceneDigestSuite) TestEmptySceneSendsNothing() {
	userID, _ := s.createUser()
	sceneID, _ := s.createScene() // venues but no shows, no new bands
	s.setSceneDigestPref(userID, true)
	s.followScene(userID, sceneID, nil, time.Now().Add(-24*time.Hour))

	s.svc.RunDigestCycleNow()
	s.Empty(s.mock.calls, "a scene with no this-week shows and no new bands is skipped")
	s.Nil(s.cursorFor(userID, sceneID), "an empty scene's cursor must NOT advance")
}

func (s *SceneDigestSuite) TestCursorOnlyBumpsContributingScenes() {
	userID, _ := s.createUser()
	s.setSceneDigestPref(userID, true)

	full, fullVenue := s.createScene()
	// A second, empty scene (different city so it's a distinct registry row).
	var emptyScene uint
	s.Require().NoError(s.db.Raw(
		`INSERT INTO scenes (metro, city, state, slug) VALUES (NULL, 'Emptyburg', 'ZZ', 'emptyburg-zz') RETURNING id`).
		Scan(&emptyScene).Error)

	s.followScene(userID, full, nil, time.Now().Add(-24*time.Hour))
	s.followScene(userID, emptyScene, nil, time.Now().Add(-24*time.Hour))
	s.createThisWeekShow(fullVenue)

	s.svc.RunDigestCycleNow()
	s.Require().Len(s.mock.calls, 1)
	s.Len(s.mock.calls[0].Groups, 1, "only the scene with content is in the email")
	s.NotNil(s.cursorFor(userID, full), "contributing scene's cursor advances")
	s.Nil(s.cursorFor(userID, emptyScene), "empty scene's cursor stays so a later band isn't lost")
}
