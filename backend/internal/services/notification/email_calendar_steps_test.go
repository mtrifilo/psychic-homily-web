package notification

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The copy of board E1 (Product Designs, light 2298:1936), as the recipient
// reads it. The footer names sign-out-everywhere first because a signed-in
// password change alone does not end another session.
const (
	calendarStepsKicker   = "CALENDAR · YOUR SAVED SHOWS"
	calendarStepsHeadline = "Add your saved shows to Google Calendar."
	calendarStepsBody     = "This step needs a computer, because Google's phone app can't add a calendar. " +
		"Open this email on one and choose the button. The page walks you through it."
	calendarStepsButton = "Add to Google Calendar"
	calendarStepsFooter = "You asked for this email from Psychic Homily. If that wasn't you, sign out everywhere else in Settings, then change your password."
)

var hrefPattern = regexp.MustCompile(`href="([^"]*)"`)

// The email sent is board E1: its subject, its copy, and one link, to the
// frontend's /library/calendar page on the configured origin (the www form in
// production). It is transactional, so it carries no unsubscribe header.
func TestSendCalendarStepsEmail_MatchesBoardE1(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)
	svc.frontendURL = "https://www.psychichomily.com"

	require.NoError(t, svc.SendCalendarStepsEmail("listener@example.com"))
	email := <-emails

	assert.Equal(t, []string{"listener@example.com"}, email.To)
	assert.Equal(t, "Add your saved shows to Google Calendar", email.Subject)
	assert.Equal(t, email.Subject, headerSafeSubject(email.Subject), "the subject passes the header-safe filter unchanged")
	assert.Empty(t, email.Headers, "no List-Unsubscribe or any other extra header")

	body := email.Html
	for name, text := range map[string]string{
		"masthead": "PSYCHIC HOMILY",
		"kicker":   calendarStepsKicker,
		"headline": calendarStepsHeadline,
		"body":     calendarStepsBody,
		"button":   calendarStepsButton,
		"footer":   calendarStepsFooter,
	} {
		assert.Contains(t, body, html.EscapeString(text), name)
	}

	hrefs := hrefPattern.FindAllStringSubmatch(body, -1)
	require.Len(t, hrefs, 1, "the button is the email's only link")
	assert.Equal(t, "https://www.psychichomily.com/library/calendar", hrefs[0][1])

	assert.NotContains(t, body, "—", "no em dash in the copy")
}

// Nothing in the email is, or points at, the account's feeds: no feed token,
// no feed route, no backend origin, no subscribe scheme.
func TestSendCalendarStepsEmail_CarriesNoFeedAddress(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)
	svc.frontendURL = "https://www.psychichomily.com"

	require.NoError(t, svc.SendCalendarStepsEmail("listener@example.com"))
	email := <-emails

	whole := strings.ToLower(email.Subject + "\n" + email.Html)
	for k, v := range email.Headers {
		whole += "\n" + strings.ToLower(k+": "+v)
	}
	for _, absent := range []string{"phcal_", "/feeds/", "/calendar/", ".ics", "webcal", "token", "api."} {
		assert.NotContains(t, whole, absent)
	}
}

func TestSendCalendarStepsEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{}
	err := svc.SendCalendarStepsEmail("listener@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSendCalendarStepsEmail_APIError(t *testing.T) {
	svc := setupEmailTestError(t)
	err := svc.SendCalendarStepsEmail("listener@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send calendar steps email")
}
