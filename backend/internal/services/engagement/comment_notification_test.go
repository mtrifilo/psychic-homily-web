package engagement

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// Unit tests — no DB required
// =============================================================================

func TestParseMentions(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"empty body", "", nil},
		{"no mentions", "just a comment about great music", nil},
		{"single mention", "hey @alice check this out", []string{"alice"}},
		{"mention at start", "@bob the new album is wild", []string{"bob"}},
		{"mention at end", "nice review @carol", []string{"carol"}},
		{"multiple mentions", "thanks @alice and @bob for the rec", []string{"alice", "bob"}},
		{
			"mention dedup preserves first order",
			"@alice @bob @alice @carol @ALICE",
			[]string{"alice", "bob", "carol"},
		},
		{
			"email address is not a mention",
			"email me at user@example.com",
			nil,
		},
		{
			"at inside URL path is not a mention",
			"see https://mastodon.social/@bob/123456 for details",
			nil,
		},
		{
			"at inside URL query is not a mention",
			"link: https://example.com/search?tag=@something",
			nil,
		},
		{
			"mention alongside email and URL",
			"hi @alice, email me at alice@example.com or see https://example.com/@fake",
			[]string{"alice"},
		},
		{
			"usernames with underscore and hyphen",
			"@alpha_user and @beta-user and @gamma123",
			[]string{"alpha_user", "beta-user", "gamma123"},
		},
		{
			"too-short username (2 chars) is ignored",
			"@ab hi",
			nil,
		},
		{
			"three-char username accepted",
			"@abc hi",
			[]string{"abc"},
		},
		{
			"mention preceded by non-word punctuation is kept",
			"(@alice) and @bob.",
			[]string{"alice", "bob"},
		},
		{
			"mid-word @ is NOT a mention (e.g. handle inside tag)",
			"tag#user@handle weirdness",
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseMentions(tc.body)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCommentNotificationService_NilDB(t *testing.T) {
	svc := NewCommentNotificationService(nil, nil, "secret", "http://localhost:3000")

	t.Run("NotifySubscribers_NilDB", func(t *testing.T) {
		err := svc.NotifySubscribers(1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("NotifyMentioned_NilDB", func(t *testing.T) {
		err := svc.NotifyMentioned(1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})
}

func TestHMAC_CommentSubscriptionRoundTrip(t *testing.T) {
	const (
		baseURL    = "http://localhost:3000"
		secret     = "s3cr3t-for-tests"
		userID     = uint(42)
		entityType = "artist"
		entityID   = uint(101)
	)

	urlStr := GenerateCommentSubscriptionUnsubscribeURL(baseURL, userID, entityType, entityID, secret)
	assert.Contains(t, urlStr, "/unsubscribe/comment-subscription")
	assert.Contains(t, urlStr, "uid=42")
	assert.Contains(t, urlStr, "entity_type=artist")
	assert.Contains(t, urlStr, "entity_id=101")

	sig := ComputeCommentSubscriptionUnsubscribeSignature(userID, entityType, entityID, secret)
	assert.True(t, VerifyCommentSubscriptionUnsubscribeSignature(userID, entityType, entityID, sig, secret))

	// Tamper: wrong user ID
	assert.False(t, VerifyCommentSubscriptionUnsubscribeSignature(userID+1, entityType, entityID, sig, secret))
	// Tamper: wrong entity type
	assert.False(t, VerifyCommentSubscriptionUnsubscribeSignature(userID, "venue", entityID, sig, secret))
	// Tamper: wrong entity ID
	assert.False(t, VerifyCommentSubscriptionUnsubscribeSignature(userID, entityType, entityID+1, sig, secret))
	// Tamper: wrong secret
	assert.False(t, VerifyCommentSubscriptionUnsubscribeSignature(userID, entityType, entityID, sig, "different-secret"))
	// Empty sig
	assert.False(t, VerifyCommentSubscriptionUnsubscribeSignature(userID, entityType, entityID, "", secret))
}

func TestHMAC_MentionRoundTrip(t *testing.T) {
	const (
		baseURL = "http://localhost:3000"
		secret  = "s3cr3t-for-tests"
		userID  = uint(42)
	)

	urlStr := GenerateMentionUnsubscribeURL(baseURL, userID, secret)
	assert.Contains(t, urlStr, "/unsubscribe/mention")
	assert.Contains(t, urlStr, "uid=42")

	sig := ComputeMentionUnsubscribeSignature(userID, secret)
	assert.True(t, VerifyMentionUnsubscribeSignature(userID, sig, secret))
	assert.False(t, VerifyMentionUnsubscribeSignature(userID+1, sig, secret))
	assert.False(t, VerifyMentionUnsubscribeSignature(userID, sig, "different-secret"))

	// Mention signatures must NOT verify against comment-subscription
	// signatures (defense-in-depth against swapped domains).
	csSig := ComputeCommentSubscriptionUnsubscribeSignature(userID, "artist", 1, secret)
	assert.False(t, VerifyMentionUnsubscribeSignature(userID, csSig, secret))
}

func TestStripMarkdownToPlain(t *testing.T) {
	cases := map[string]string{
		"plain text":                      "plain text",
		"# heading":                       "heading",
		"**bold** and *italic*":           "**bold** and *italic*",
		"[link text](https://example.com)": "link text",
		"- item one\n- item two":          "item one item two",
		"> quoted text":                   "quoted text",
		"```\ncode\nblock\n```":            "",
		"mix `inline` and text":           "mix and text",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			got := stripMarkdownToPlain(in)
			assert.Equal(t, want, got)
		})
	}
}

func TestBuildExcerpt_TruncatesLongBodies(t *testing.T) {
	long := ""
	for i := 0; i < 300; i++ {
		long += "a"
	}
	got := buildExcerpt(long)
	// commentExcerptMaxChars + 1 (the ellipsis character).
	runes := []rune(got)
	assert.Equal(t, commentExcerptMaxChars+1, len(runes))
	assert.Equal(t, '…', runes[commentExcerptMaxChars])
}

// =============================================================================
// Integration tests — real DB
// =============================================================================

// captureEmailService records all Send* calls. Drop-in mock — zero behavior
// beyond recording.
type captureEmailService struct {
	mu                      sync.Mutex
	commentCalls            []commentEmailCall
	mentionCalls            []mentionEmailCall
	fail                    bool
	configured              bool
}

type commentEmailCall struct {
	ToEmail        string
	CommenterName  string
	EntityType     string
	EntityName     string
	CommentExcerpt string
	EntityURL      string
	UnsubscribeURL string
}

type mentionEmailCall struct {
	ToEmail        string
	MentionerName  string
	EntityType     string
	EntityName     string
	CommentExcerpt string
	CommentURL     string
	UnsubscribeURL string
}

func (m *captureEmailService) IsConfigured() bool {
	return m.configured
}

func (m *captureEmailService) SendVerificationEmail(_, _ string) error { return nil }
func (m *captureEmailService) SendMagicLinkEmail(_, _ string) error    { return nil }
func (m *captureEmailService) SendAccountRecoveryEmail(_, _ string, _ int) error {
	return nil
}
func (m *captureEmailService) SendShowReminderEmail(_, _, _, _ string, _ time.Time, _ []string) error {
	return nil
}
func (m *captureEmailService) SendFilterNotificationEmail(_, _, _, _ string) error { return nil }
func (m *captureEmailService) SendTierPromotionEmail(_, _, _, _, _ string, _ []string) error {
	return nil
}
func (m *captureEmailService) SendTierDemotionEmail(_, _, _, _, _ string) error { return nil }
func (m *captureEmailService) SendTierDemotionWarningEmail(_, _, _ string, _, _ float64) error {
	return nil
}
func (m *captureEmailService) SendEditApprovedEmail(_, _, _, _, _ string) error { return nil }
func (m *captureEmailService) SendEditRejectedEmail(_, _, _, _, _ string) error { return nil }

func (m *captureEmailService) SendCommentNotification(toEmail, commenterName, entityType, entityName, commentExcerpt, entityURL, unsubscribeURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return fmt.Errorf("forced failure")
	}
	m.commentCalls = append(m.commentCalls, commentEmailCall{
		ToEmail: toEmail, CommenterName: commenterName, EntityType: entityType,
		EntityName: entityName, CommentExcerpt: commentExcerpt,
		EntityURL: entityURL, UnsubscribeURL: unsubscribeURL,
	})
	return nil
}

func (m *captureEmailService) SendMentionNotification(toEmail, mentionerName, entityType, entityName, commentExcerpt, commentURL, unsubscribeURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return fmt.Errorf("forced failure")
	}
	m.mentionCalls = append(m.mentionCalls, mentionEmailCall{
		ToEmail: toEmail, MentionerName: mentionerName, EntityType: entityType,
		EntityName: entityName, CommentExcerpt: commentExcerpt,
		CommentURL: commentURL, UnsubscribeURL: unsubscribeURL,
	})
	return nil
}

func (m *captureEmailService) commentCallsSorted() []commentEmailCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]commentEmailCall, len(m.commentCalls))
	copy(out, m.commentCalls)
	sort.Slice(out, func(i, j int) bool { return out[i].ToEmail < out[j].ToEmail })
	return out
}

func (m *captureEmailService) mentionCallsSorted() []mentionEmailCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mentionEmailCall, len(m.mentionCalls))
	copy(out, m.mentionCalls)
	sort.Slice(out, func(i, j int) bool { return out[i].ToEmail < out[j].ToEmail })
	return out
}

// CommentNotificationServiceIntegrationSuite drives the service against a real
// Postgres via testcontainers.
type CommentNotificationServiceIntegrationSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	mock    *captureEmailService
	svc     *CommentNotificationService
	comment *CommentService
}

func (s *CommentNotificationServiceIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *CommentNotificationServiceIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *CommentNotificationServiceIntegrationSuite) SetupTest() {
	s.mock = &captureEmailService{configured: true}
	s.svc = NewCommentNotificationService(s.db, s.mock, "test-secret", "http://localhost:3000")
	s.comment = NewCommentService(s.db)
}

func (s *CommentNotificationServiceIntegrationSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM comment_subscriptions")
	_, _ = sqlDB.Exec("DELETE FROM comments")
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestCommentNotificationServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(CommentNotificationServiceIntegrationSuite))
}

// -- helpers

func (s *CommentNotificationServiceIntegrationSuite) createUser(username string) *models.User {
	email := fmt.Sprintf("%s-%d@test.local", username, time.Now().UnixNano())
	un := username
	fn := "First"
	u := &models.User{
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

// createUserWithPrefs inserts user + preferences with an explicit state for
// the two PSY-289 bools. Uses Create-then-Update for the false cases because
// GORM skips zero-value bools on Create (the "GORM bool gotcha" — DB default
// of true would win otherwise).
func (s *CommentNotificationServiceIntegrationSuite) createUserWithPrefs(username string, notifyComment, notifyMention bool) *models.User {
	u := s.createUser(username)
	prefs := &models.UserPreferences{
		UserID:                      u.ID,
		NotifyOnCommentSubscription: true,
		NotifyOnMention:             true,
	}
	s.Require().NoError(s.db.Create(prefs).Error)
	updates := map[string]interface{}{}
	if !notifyComment {
		updates["notify_on_comment_subscription"] = false
	}
	if !notifyMention {
		updates["notify_on_mention"] = false
	}
	if len(updates) > 0 {
		s.Require().NoError(s.db.Model(&models.UserPreferences{}).Where("user_id = ?", u.ID).Updates(updates).Error)
	}
	return u
}

func (s *CommentNotificationServiceIntegrationSuite) createArtist(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	a := &models.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a.ID
}

func (s *CommentNotificationServiceIntegrationSuite) insertComment(userID uint, entityID uint, body string) *models.Comment {
	c := &models.Comment{
		EntityType: models.CommentEntityArtist,
		EntityID:   entityID,
		Kind:       models.CommentKindComment,
		UserID:     userID,
		Body:       body,
		BodyHTML:   "<p>" + body + "</p>",
		Visibility: models.CommentVisibilityVisible,
	}
	s.Require().NoError(s.db.Create(c).Error)
	return c
}

func (s *CommentNotificationServiceIntegrationSuite) subscribe(userID uint, entityID uint) {
	sub := &models.CommentSubscription{
		UserID:       userID,
		EntityType:   string(models.CommentEntityArtist),
		EntityID:     entityID,
		SubscribedAt: time.Now().UTC(),
	}
	s.Require().NoError(s.db.Create(sub).Error)
}

// -- tests

// TestNotifySubscribers_HappyPath verifies the basic fan-out path: two
// subscribers get notified, commenter does not, last_notified_at is updated.
func (s *CommentNotificationServiceIntegrationSuite) TestNotifySubscribers_HappyPath() {
	author := s.createUser("author")
	subA := s.createUser("suba")
	subB := s.createUser("subb")
	artistID := s.createArtist("The Band")

	s.subscribe(author.ID, artistID) // author should be skipped
	s.subscribe(subA.ID, artistID)
	s.subscribe(subB.ID, artistID)

	c := s.insertComment(author.ID, artistID, "Check out this show")

	s.Require().NoError(s.svc.NotifySubscribers(c.ID))

	calls := s.mock.commentCallsSorted()
	s.Require().Len(calls, 2)
	s.Assert().Equal(*subA.Email, calls[0].ToEmail)
	s.Assert().Equal(*subB.Email, calls[1].ToEmail)
	s.Assert().Equal("author", calls[0].CommenterName)
	s.Assert().Equal("The Band", calls[0].EntityName)
	s.Assert().Contains(calls[0].EntityURL, "/artists/")
	s.Assert().Contains(calls[0].UnsubscribeURL, "/unsubscribe/comment-subscription")

	// last_notified_at should be set on the two non-author subscribers but
	// NOT on the author's own subscription.
	var subs []models.CommentSubscription
	s.Require().NoError(s.db.Order("user_id ASC").Find(&subs).Error)
	for _, sub := range subs {
		if sub.UserID == author.ID {
			s.Assert().Nil(sub.LastNotifiedAt, "author's subscription should not be notified")
		} else {
			s.Assert().NotNil(sub.LastNotifiedAt, "subscriber's last_notified_at should be set")
		}
	}
}

// TestNotifySubscribers_DedupWithinHour — second call within an hour skips
// already-notified subscribers; waiting past the cutoff re-notifies.
func (s *CommentNotificationServiceIntegrationSuite) TestNotifySubscribers_DedupWithinHour() {
	author := s.createUser("author")
	sub := s.createUser("sub")
	artistID := s.createArtist("Dedup Artist")

	s.subscribe(sub.ID, artistID)

	c1 := s.insertComment(author.ID, artistID, "First comment")
	s.Require().NoError(s.svc.NotifySubscribers(c1.ID))
	s.Assert().Len(s.mock.commentCalls, 1)

	// Second comment within the dedup window — should skip.
	c2 := s.insertComment(author.ID, artistID, "Second comment")
	s.Require().NoError(s.svc.NotifySubscribers(c2.ID))
	s.Assert().Len(s.mock.commentCalls, 1, "second email within dedup window should be skipped")

	// Simulate passage of time: set last_notified_at to >1h ago.
	past := time.Now().UTC().Add(-2 * time.Hour)
	s.Require().NoError(s.db.Model(&models.CommentSubscription{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ?", sub.ID, "artist", artistID).
		Update("last_notified_at", past).Error)

	c3 := s.insertComment(author.ID, artistID, "Third comment")
	s.Require().NoError(s.svc.NotifySubscribers(c3.ID))
	s.Assert().Len(s.mock.commentCalls, 2, "after dedup window, subscriber should be notified again")
}

// TestNotifySubscribers_RespectsPreference — subscriber with
// notify_on_comment_subscription=false does NOT receive email.
func (s *CommentNotificationServiceIntegrationSuite) TestNotifySubscribers_RespectsPreference() {
	author := s.createUser("author")
	optedIn := s.createUserWithPrefs("optedin", true, true)
	optedOut := s.createUserWithPrefs("optedout", false, true)
	artistID := s.createArtist("Pref Artist")

	s.subscribe(optedIn.ID, artistID)
	s.subscribe(optedOut.ID, artistID)

	c := s.insertComment(author.ID, artistID, "A comment")
	s.Require().NoError(s.svc.NotifySubscribers(c.ID))

	calls := s.mock.commentCallsSorted()
	s.Require().Len(calls, 1)
	s.Assert().Equal(*optedIn.Email, calls[0].ToEmail)
}

// TestNotifySubscribers_SkipsInactiveAndDeletedUsers
func (s *CommentNotificationServiceIntegrationSuite) TestNotifySubscribers_SkipsInactiveAndDeletedUsers() {
	author := s.createUser("author")
	active := s.createUser("active")
	inactive := s.createUser("inactive")
	deleted := s.createUser("deleted")
	artistID := s.createArtist("Filter Artist")

	s.subscribe(active.ID, artistID)
	s.subscribe(inactive.ID, artistID)
	s.subscribe(deleted.ID, artistID)

	// Flip flags via UPDATE (Create skips zero-value false for IsActive).
	s.Require().NoError(s.db.Model(&models.User{}).Where("id = ?", inactive.ID).Update("is_active", false).Error)
	now := time.Now().UTC()
	s.Require().NoError(s.db.Model(&models.User{}).Where("id = ?", deleted.ID).Update("deleted_at", now).Error)

	c := s.insertComment(author.ID, artistID, "filter test")
	s.Require().NoError(s.svc.NotifySubscribers(c.ID))

	calls := s.mock.commentCallsSorted()
	s.Require().Len(calls, 1)
	s.Assert().Equal(*active.Email, calls[0].ToEmail)
}

// TestNotifyMentioned_HappyPath parses @handles and emails them.
func (s *CommentNotificationServiceIntegrationSuite) TestNotifyMentioned_HappyPath() {
	author := s.createUser("author")
	alice := s.createUser("alice")
	bob := s.createUser("bob")
	artistID := s.createArtist("Mention Artist")

	c := s.insertComment(author.ID, artistID, "great show @alice @bob")
	s.Require().NoError(s.svc.NotifyMentioned(c.ID))

	calls := s.mock.mentionCallsSorted()
	s.Require().Len(calls, 2)
	s.Assert().Equal(*alice.Email, calls[0].ToEmail)
	s.Assert().Equal(*bob.Email, calls[1].ToEmail)
	s.Assert().Equal("author", calls[0].MentionerName)
	s.Assert().Contains(calls[0].CommentURL, "#comment-")
	s.Assert().Contains(calls[0].UnsubscribeURL, "/unsubscribe/mention")
}

// TestNotifyMentioned_IgnoresEmailAndURLContainingAt
func (s *CommentNotificationServiceIntegrationSuite) TestNotifyMentioned_IgnoresEmailAndURLContainingAt() {
	author := s.createUser("author")
	alice := s.createUser("alice")
	// Create a user whose username matches the URL fragment so we can verify
	// it is NOT notified.
	_ = s.createUser("mastodon")
	artistID := s.createArtist("Ignore Artist")

	body := "hey @alice check https://mastodon.social/@mastodon and email alice@example.com"
	c := s.insertComment(author.ID, artistID, body)
	s.Require().NoError(s.svc.NotifyMentioned(c.ID))

	calls := s.mock.mentionCallsSorted()
	s.Require().Len(calls, 1)
	s.Assert().Equal(*alice.Email, calls[0].ToEmail)
}

// TestNotifyMentioned_RespectsPreference
func (s *CommentNotificationServiceIntegrationSuite) TestNotifyMentioned_RespectsPreference() {
	author := s.createUser("author")
	_ = s.createUserWithPrefs("alice", true, false) // mention pref off
	bob := s.createUserWithPrefs("bob", true, true) // mention pref on
	artistID := s.createArtist("Mention Pref Artist")

	c := s.insertComment(author.ID, artistID, "hey @alice and @bob")
	s.Require().NoError(s.svc.NotifyMentioned(c.ID))

	calls := s.mock.mentionCallsSorted()
	s.Require().Len(calls, 1)
	s.Assert().Equal(*bob.Email, calls[0].ToEmail)
}

// TestNotifyMentioned_SelfMentionSuppressed — author mentions self, no email.
func (s *CommentNotificationServiceIntegrationSuite) TestNotifyMentioned_SelfMentionSuppressed() {
	author := s.createUser("solo")
	artistID := s.createArtist("Solo Artist")

	c := s.insertComment(author.ID, artistID, "shouting out @solo myself")
	s.Require().NoError(s.svc.NotifyMentioned(c.ID))

	s.Assert().Len(s.mock.mentionCalls, 0)
}

// TestNotifyMentioned_UnknownUsernamesNoop
func (s *CommentNotificationServiceIntegrationSuite) TestNotifyMentioned_UnknownUsernamesNoop() {
	author := s.createUser("author")
	artistID := s.createArtist("Unknown Artist")
	c := s.insertComment(author.ID, artistID, "@ghost @missing @notreal")
	s.Require().NoError(s.svc.NotifyMentioned(c.ID))
	s.Assert().Len(s.mock.mentionCalls, 0)
}

// TestNotifySubscribers_NoEmail — subscriber whose user.email is NULL is skipped
// without error.
func (s *CommentNotificationServiceIntegrationSuite) TestNotifySubscribers_NoEmail() {
	author := s.createUser("author")
	// Create a user with a NULL email.
	u := &models.User{
		Username:  stringPtr("noemail"),
		IsActive:  true,
		UserTier:  "contributor",
	}
	s.Require().NoError(s.db.Create(u).Error)

	artistID := s.createArtist("No Email Artist")
	s.subscribe(u.ID, artistID)

	c := s.insertComment(author.ID, artistID, "nothing here")
	s.Require().NoError(s.svc.NotifySubscribers(c.ID))

	// No email call because no email address; also, last_notified_at should
	// remain NULL because we short-circuit before sending.
	s.Assert().Len(s.mock.commentCalls, 0)
	var sub models.CommentSubscription
	s.Require().NoError(s.db.Where("user_id = ?", u.ID).First(&sub).Error)
	s.Assert().Nil(sub.LastNotifiedAt)
}

// TestCreateComment_WiresNotifier — via the real CommentService with a notifier
// attached, creating a comment fires both goroutines. Uses a visible-tier user
// so no pending_review gate.
func (s *CommentNotificationServiceIntegrationSuite) TestCreateComment_WiresNotifier() {
	author := s.createUser("author")
	sub := s.createUser("subscriber")
	mentioned := s.createUser("pinged")
	artistID := s.createArtist("Wiring Artist")

	s.subscribe(sub.ID, artistID)
	s.comment.SetNotifier(s.svc)

	_, err := s.comment.CreateComment(author.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "nice to see you @pinged",
	})
	s.Require().NoError(err)

	// Goroutines are fire-and-forget — give them a moment. Polling keeps
	// this fast when they complete quickly.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.mock.mu.Lock()
		cDone := len(s.mock.commentCalls) >= 1
		mDone := len(s.mock.mentionCalls) >= 1
		s.mock.mu.Unlock()
		if cDone && mDone {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	commentCalls := s.mock.commentCallsSorted()
	mentionCalls := s.mock.mentionCallsSorted()
	s.Require().Len(commentCalls, 1, "subscriber should receive comment notification")
	s.Assert().Equal(*sub.Email, commentCalls[0].ToEmail)
	s.Require().Len(mentionCalls, 1, "mentioned user should receive mention notification")
	s.Assert().Equal(*mentioned.Email, mentionCalls[0].ToEmail)
}
