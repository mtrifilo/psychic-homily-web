package notification

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	resend "github.com/resend/resend-go/v2"
)

// PSY-756: the tier-change and edit-review senders must set RFC 8058
// List-Unsubscribe headers AND carry a prominent in-body opt-out card, the
// same as the already-compliant collection-digest sender. These tests use a
// harness that captures the Headers map (the legacy capturedEmail in
// email_test.go does not).

// setupHeaderCapturingEmailTest mirrors setupDigestEmailTest but lives here so
// the tier/edit header tests don't depend on the digest test file's harness.
func setupHeaderCapturingEmailTest(t *testing.T) (*EmailService, chan capturedDigestEmail) {
	t.Helper()
	requests := make(chan capturedDigestEmail, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req capturedDigestEmail
		_ = json.NewDecoder(r.Body).Decode(&req)
		requests <- req
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "test-email-id"})
	}))
	t.Cleanup(server.Close)

	client := resend.NewCustomClient(server.Client(), "test-api-key")
	serverURL, _ := url.Parse(server.URL)
	client.BaseURL = serverURL

	return &EmailService{
		client:      client,
		fromEmail:   "noreply@test.com",
		frontendURL: "http://localhost:3000",
	}, requests
}

// assertUnsubscribeWired checks the RFC 8058 headers and that the unsubscribe
// URL appears in the body as a one-click opt-out card.
func assertUnsubscribeWired(t *testing.T, email capturedDigestEmail, unsubURL string) {
	t.Helper()
	require.NotNil(t, email.Headers, "Headers must be set so Gmail/Yahoo render the native Unsubscribe button")
	assert.Equal(t, "<"+unsubURL+">", email.Headers["List-Unsubscribe"],
		"List-Unsubscribe must wrap the URL in <> per RFC 2369")
	assert.Equal(t, "List-Unsubscribe=One-Click", email.Headers["List-Unsubscribe-Post"],
		"List-Unsubscribe-Post must be exactly 'List-Unsubscribe=One-Click' per RFC 8058")
	assert.Contains(t, email.Html, unsubURL,
		"the unsubscribe URL must appear as a clickable link in the email body")
	assert.Contains(t, email.Html, "one click",
		"the in-body opt-out card must communicate the one-click affordance")
}

func TestSendTierPromotionEmail_UnsubscribeWired(t *testing.T) {
	svc, emails := setupHeaderCapturingEmailTest(t)
	unsubURL := "http://api.test.local/unsubscribe/tier-notifications?uid=42&sig=abc"

	require.NoError(t, svc.SendTierPromotionEmail("u@test.com", "alice", "new_user", "contributor", "5 edits", unsubURL, []string{"Submit edits"}))
	assertUnsubscribeWired(t, <-emails, unsubURL)
}

func TestSendTierDemotionEmail_UnsubscribeWired(t *testing.T) {
	svc, emails := setupHeaderCapturingEmailTest(t)
	unsubURL := "http://api.test.local/unsubscribe/tier-notifications?uid=42&sig=abc"

	require.NoError(t, svc.SendTierDemotionEmail("u@test.com", "alice", "contributor", "new_user", "low rate", unsubURL))
	assertUnsubscribeWired(t, <-emails, unsubURL)
}

func TestSendTierDemotionWarningEmail_UnsubscribeWired(t *testing.T) {
	svc, emails := setupHeaderCapturingEmailTest(t)
	unsubURL := "http://api.test.local/unsubscribe/tier-notifications?uid=42&sig=abc"

	require.NoError(t, svc.SendTierDemotionWarningEmail("u@test.com", "alice", "contributor", 0.82, 0.80, unsubURL))
	assertUnsubscribeWired(t, <-emails, unsubURL)
}

func TestSendEditApprovedEmail_UnsubscribeWired(t *testing.T) {
	svc, emails := setupHeaderCapturingEmailTest(t)
	unsubURL := "http://api.test.local/unsubscribe/edit-notifications?uid=42&sig=abc"

	require.NoError(t, svc.SendEditApprovedEmail("u@test.com", "alice", "artist", "Band", "http://localhost:3000/artists/band", unsubURL))
	assertUnsubscribeWired(t, <-emails, unsubURL)
}

func TestSendEditRejectedEmail_UnsubscribeWired(t *testing.T) {
	svc, emails := setupHeaderCapturingEmailTest(t)
	unsubURL := "http://api.test.local/unsubscribe/edit-notifications?uid=42&sig=abc"

	require.NoError(t, svc.SendEditRejectedEmail("u@test.com", "alice", "artist", "Band", "wrong info", unsubURL))
	assertUnsubscribeWired(t, <-emails, unsubURL)
}
