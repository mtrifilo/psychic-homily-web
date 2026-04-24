package notification

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendCommentNotification covers the happy-path for the PSY-289 subscriber
// email. We verify the captured request shape via the shared resend test
// harness (setupEmailTest).
func TestSendCommentNotification_Success(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendCommentNotification(
		"subscriber@test.com",
		"alice",
		"artist",
		"The Cool Band",
		"Great show last night!",
		"http://localhost:3000/artists/the-cool-band",
		"http://localhost:3000/unsubscribe/comment-subscription?uid=1&entity_type=artist&entity_id=42&sig=deadbeef",
	)

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.From, "noreply@test.com")
	assert.Equal(t, []string{"subscriber@test.com"}, email.To)
	assert.Contains(t, email.Subject, "The Cool Band")
	assert.Contains(t, email.Html, "alice")
	assert.Contains(t, email.Html, "The Cool Band")
	assert.Contains(t, email.Html, "Great show last night!")
	assert.Contains(t, email.Html, "http://localhost:3000/artists/the-cool-band")
	assert.Contains(t, email.Html, "/unsubscribe/comment-subscription")
}

func TestSendCommentNotification_NotConfigured(t *testing.T) {
	svc := &EmailService{}
	err := svc.SendCommentNotification(
		"a@b.com", "user", "artist", "Band", "excerpt", "https://u", "https://unsub",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSendCommentNotification_APIError(t *testing.T) {
	svc := setupEmailTestError(t)
	err := svc.SendCommentNotification(
		"a@b.com", "user", "artist", "Band", "excerpt", "https://u", "https://unsub",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send comment notification email")
}

func TestSendCommentNotification_EmptyCommenterName(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendCommentNotification(
		"subscriber@test.com",
		"",
		"show",
		"Awesome Show",
		"my excerpt",
		"http://localhost:3000/shows/1",
		"http://localhost:3000/unsubscribe/comment-subscription?uid=1&entity_type=show&entity_id=1&sig=abc",
	)
	require.NoError(t, err)
	email := <-emails
	// Falls back to "A contributor".
	assert.Contains(t, email.Html, "A contributor")
}

func TestSendMentionNotification_Success(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendMentionNotification(
		"mentioned@test.com",
		"bob",
		"release",
		"Best Album",
		"check out @mentioned",
		"http://localhost:3000/releases/best-album#comment-99",
		"http://localhost:3000/unsubscribe/mention?uid=2&sig=feed",
	)
	require.NoError(t, err)
	email := <-emails
	assert.Equal(t, []string{"mentioned@test.com"}, email.To)
	assert.Contains(t, email.Subject, "bob")
	assert.Contains(t, email.Subject, "Best Album")
	assert.Contains(t, email.Html, "check out")
	assert.Contains(t, email.Html, "#comment-99")
	assert.Contains(t, email.Html, "/unsubscribe/mention")
}

func TestSendMentionNotification_NotConfigured(t *testing.T) {
	svc := &EmailService{}
	err := svc.SendMentionNotification(
		"a@b.com", "user", "artist", "Band", "excerpt", "https://u", "https://unsub",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSendMentionNotification_APIError(t *testing.T) {
	svc := setupEmailTestError(t)
	err := svc.SendMentionNotification(
		"a@b.com", "user", "artist", "Band", "excerpt", "https://u", "https://unsub",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send mention notification email")
}

func TestSendMentionNotification_EmptyMentionerName(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)
	err := svc.SendMentionNotification(
		"mentioned@test.com",
		"",
		"artist",
		"The Band",
		"excerpt",
		"http://example.com",
		"http://example.com/unsub",
	)
	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.Html, "Someone")
	assert.Contains(t, email.Subject, "Someone")
}
