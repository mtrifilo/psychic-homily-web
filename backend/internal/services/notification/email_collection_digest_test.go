package notification

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	resend "github.com/resend/resend-go/v2"

	"psychic-homily-backend/internal/services/contracts"
)

// PSY-350: tests for SendCollectionDigestEmail covering the spam-prevention
// hardening — RFC 8058 / RFC 2369 List-Unsubscribe headers, weekly copy,
// prominent in-body opt-out block.

// capturedDigestEmail extends the standard captured-email shape to also
// capture the headers map, which the digest email's anti-spam hardening
// depends on. Pulled out to keep the legacy test harness in email_test.go
// untouched.
type capturedDigestEmail struct {
	From    string            `json:"from"`
	To      []string          `json:"to"`
	Subject string            `json:"subject"`
	Html    string            `json:"html"`
	Headers map[string]string `json:"headers,omitempty"`
}

func setupDigestEmailTest(t *testing.T) (*EmailService, chan capturedDigestEmail) {
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

	service := &EmailService{
		client:      client,
		fromEmail:   "noreply@test.com",
		frontendURL: "http://localhost:3000",
	}
	return service, requests
}

// TestSendCollectionDigest_WeeklyCopy_Headers_OptOutBlock verifies the
// hardening surface in one shot: subject reads "weekly", List-Unsubscribe
// headers are set per RFC 8058, and the body has a prominent in-body
// unsubscribe block (not just a fine-print footer).
func TestSendCollectionDigest_WeeklyCopy_Headers_OptOutBlock(t *testing.T) {
	svc, emails := setupDigestEmailTest(t)

	groups := []contracts.CollectionDigestGroup{
		{
			CollectionTitle: "Best of 2026",
			CollectionURL:   "http://localhost:3000/collections/best-of-2026",
			Items: []contracts.CollectionDigestEntry{
				{
					EntityType: "artist",
					EntityName: "Deafheaven",
					EntityURL:  "http://localhost:3000/artists/deafheaven",
					AddedBy:    "alice",
				},
			},
		},
	}
	unsubURL := "http://api.test.local/unsubscribe/collection-digest?uid=42&sig=abc"

	require.NoError(t, svc.SendCollectionDigestEmail("user@test.com", groups, unsubURL))

	email := <-emails

	// --- Subject says "weekly", not "daily" or generic.
	assert.Contains(t, email.Subject, "week",
		"subject must communicate weekly cadence, got %q", email.Subject)

	// --- RFC 8058 / RFC 2369: List-Unsubscribe + List-Unsubscribe-Post.
	require.NotNil(t, email.Headers, "Headers must be set so Gmail/Yahoo render the native Unsubscribe button")
	assert.Equal(t, "<"+unsubURL+">", email.Headers["List-Unsubscribe"],
		"List-Unsubscribe must wrap the URL in <> per RFC 2369")
	assert.Equal(t, "List-Unsubscribe=One-Click", email.Headers["List-Unsubscribe-Post"],
		"List-Unsubscribe-Post must be exactly 'List-Unsubscribe=One-Click' per RFC 8058")

	// --- The in-body opt-out block must be prominent (its own visible
	// section above the footer) — we look for the user-facing "in one click"
	// copy and the unsubscribe URL itself.
	assert.Contains(t, email.Html, unsubURL,
		"the unsubscribe URL must appear as a clickable link in the email body")
	assert.Contains(t, email.Html, "one click",
		"the in-body unsubscribe block must communicate the one-click affordance")

	// --- The body should mention "weekly" so a recipient understands the
	// cadence even after Gmail strips the subject prefix in conversations.
	assert.Contains(t, strings.ToLower(email.Html), "week",
		"body should reference the weekly cadence")
}

// TestSendCollectionDigest_RejectsEmpty makes sure an empty groups slice
// fails fast rather than sending a "you have nothing new" no-op email.
func TestSendCollectionDigest_RejectsEmpty(t *testing.T) {
	svc, _ := setupDigestEmailTest(t)
	err := svc.SendCollectionDigestEmail("user@test.com", nil, "http://x/y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no digest groups")
}

// TestSendCollectionDigest_RejectsNoItems exercises the "groups but no
// items" guard.
func TestSendCollectionDigest_RejectsNoItems(t *testing.T) {
	svc, _ := setupDigestEmailTest(t)
	groups := []contracts.CollectionDigestGroup{
		{CollectionTitle: "Empty", CollectionURL: "http://x/y", Items: nil},
	}
	err := svc.SendCollectionDigestEmail("user@test.com", groups, "http://x/y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no items")
}
