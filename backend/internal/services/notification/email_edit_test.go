package notification

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// SendEditApprovedEmail
// =============================================================================

func TestSendEditApprovedEmail_Success(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendEditApprovedEmail(
		"user@test.com",
		"cooluser",
		"artist",
		"The Cool Band",
		"http://localhost:3000/artists/the-cool-band",
	)

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.From, "noreply@test.com")
	assert.Equal(t, []string{"user@test.com"}, email.To)
	assert.Contains(t, email.Subject, "The Cool Band")
	assert.Contains(t, email.Subject, "approved")
	assert.Contains(t, email.Html, "cooluser")
	assert.Contains(t, email.Html, "The Cool Band")
	assert.Contains(t, email.Html, "http://localhost:3000/artists/the-cool-band")
	assert.Contains(t, email.Html, "artist")
	assert.Contains(t, email.Html, "approved")
}

func TestSendEditApprovedEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{client: nil, fromEmail: ""}

	err := svc.SendEditApprovedEmail("user@test.com", "user", "artist", "Band", "http://example.com")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSendEditApprovedEmail_APIError(t *testing.T) {
	svc := setupEmailTestError(t)

	err := svc.SendEditApprovedEmail("user@test.com", "user", "artist", "Band", "http://example.com")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send edit approved email")
}

func TestSendEditApprovedEmail_EmptyUsername(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendEditApprovedEmail("user@test.com", "", "venue", "Valley Bar", "http://localhost:3000/venues/valley-bar")

	require.NoError(t, err)
	email := <-emails
	// Should use "there" as greeting when username is empty
	assert.Contains(t, email.Html, "Hi there")
}

func TestSendEditApprovedEmail_VenueEntity(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendEditApprovedEmail("user@test.com", "admin", "venue", "Crescent Ballroom", "http://localhost:3000/venues/crescent-ballroom")

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.Subject, "Crescent Ballroom")
	assert.Contains(t, email.Html, "venue")
	assert.Contains(t, email.Html, "Crescent Ballroom")
	assert.Contains(t, email.Html, "View Venue")
}

func TestSendEditApprovedEmail_FestivalEntity(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendEditApprovedEmail("user@test.com", "testuser", "festival", "M3F Fest 2026", "http://localhost:3000/festivals/m3f-fest-2026")

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.Html, "festival")
	assert.Contains(t, email.Html, "M3F Fest 2026")
	assert.Contains(t, email.Html, "View Festival")
}

// =============================================================================
// SendEditRejectedEmail
// =============================================================================

func TestSendEditRejectedEmail_Success(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendEditRejectedEmail(
		"user@test.com",
		"testuser",
		"artist",
		"Some Artist",
		"The name does not match the artist's official spelling",
	)

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.From, "noreply@test.com")
	assert.Equal(t, []string{"user@test.com"}, email.To)
	assert.Contains(t, email.Subject, "Some Artist")
	assert.Contains(t, email.Subject, "Update on your edit")
	assert.Contains(t, email.Html, "testuser")
	assert.Contains(t, email.Html, "Some Artist")
	assert.Contains(t, email.Html, "not accepted")
	assert.Contains(t, email.Html, "The name does not match the artist's official spelling")
	assert.Contains(t, email.Html, "Tips for future edits")
}

func TestSendEditRejectedEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{client: nil, fromEmail: ""}

	err := svc.SendEditRejectedEmail("user@test.com", "user", "artist", "Band", "reason")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSendEditRejectedEmail_APIError(t *testing.T) {
	svc := setupEmailTestError(t)

	err := svc.SendEditRejectedEmail("user@test.com", "user", "artist", "Band", "reason")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send edit rejected email")
}

func TestSendEditRejectedEmail_EmptyUsername(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendEditRejectedEmail("user@test.com", "", "venue", "Valley Bar", "incorrect info")

	require.NoError(t, err)
	email := <-emails
	// Should use "there" as greeting when username is empty
	assert.Contains(t, email.Html, "Hi there")
}

func TestSendEditRejectedEmail_ContainsEncouragement(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendEditRejectedEmail("user@test.com", "testuser", "artist", "Band", "wrong info")

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.Html, "discouraged")
	assert.Contains(t, email.Html, "contributions are valued")
}
