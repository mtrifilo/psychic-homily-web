package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	authsvc "psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/notification"
	"psychic-homily-backend/internal/testutil"
)

// stepsEmail is the part of a sent email's request body these tests read.
type stepsEmail struct {
	To      []string          `json:"to"`
	Subject string            `json:"subject"`
	HTML    string            `json:"html"`
	Headers map[string]string `json:"headers"`
	raw     string
}

func decodedEmails(t *testing.T, mailbox *sentEmails) []stepsEmail {
	t.Helper()
	mailbox.mu.Lock()
	defer mailbox.mu.Unlock()
	emails := make([]stepsEmail, len(mailbox.raw))
	for i, raw := range mailbox.raw {
		if err := json.Unmarshal([]byte(raw), &emails[i]); err != nil {
			t.Fatalf("decode sent email: %v", err)
		}
		emails[i].raw = raw
	}
	return emails
}

var stepsHref = regexp.MustCompile(`href="([^"]*)"`)

// POST /calendar/token/email-steps through the built router, the real outbox
// and the real email service: an API token and an unverified address are
// refused with their codes and queue nothing; a verified session gets 204, one
// row of the kind, and an email whose only link is the frontend's
// /library/calendar and which carries nothing of the feed token the account
// holds; the fourth request in an hour is refused with 429, its code and a
// Retry-After.
func TestCalendarStepsEmailThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	cfg.Email.FrontendURL = "https://www.psychichomily.com"
	cfg.Email.ResendAPIKey = "re_fake_key_for_testing"
	cfg.Email.FromEmail = "test@psychichomily.com"
	sc := services.NewServiceContainer(td.DB, cfg)
	mailbox := &sentEmails{}
	sc.Email = notification.NewEmailServiceWithTransport(cfg, mailbox)
	sc.AuthEmailOutbox = authsvc.NewAuthEmailOutbox(td.DB, sc.User, sc.JWT, sc.Email)
	// HumaJWTMiddleware validates API tokens on the process-wide handle it
	// reads when the router is built, so it is pointed at this database first.
	prev := db.DB
	db.DB = td.DB
	t.Cleanup(func() { db.DB = prev })
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	// API tokens validate only for an admin owner, so the account under test
	// is an admin and holds both credentials.
	user := testhelpers.CreateAdminUser(td.DB)
	session := mintToken(t, sc, user)
	apiToken, err := sc.APIToken.CreateToken(user.ID, nil, 1)
	if err != nil {
		t.Fatalf("mint API token: %v", err)
	}
	if _, _, err := sc.APIToken.ValidateToken(apiToken.Token); err != nil {
		t.Fatalf("the API token fixture is not valid, so its refusal proves nothing: %v", err)
	}
	unverified := testhelpers.CreateTestUser(td.DB)
	if err := td.DB.Model(unverified).Update("email_verified", false).Error; err != nil {
		t.Fatal(err)
	}
	unverifiedSession := mintToken(t, sc, unverified)

	post := func(path, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer "+bearer)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	errorCode := func(w *httptest.ResponseRecorder) string {
		var body struct {
			ErrorCode string `json:"error_code"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return body.ErrorCode
	}
	queued := func() int64 {
		var n int64
		if err := td.DB.Model(&authm.AuthEmailOutboxItem{}).Where("kind = ?", authm.AuthEmailKindCalendarSteps).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}

	// The account holds a feed token, so the email has one it could leak.
	w := post("/calendar/token", session)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /calendar/token = %d; body: %s", w.Code, w.Body.String())
	}
	var minted struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &minted); err != nil || !strings.HasPrefix(minted.Token, "phcal_") {
		t.Fatalf("minted token = %q (%v)", minted.Token, err)
	}

	w = post(calendarStepsEmailRoute, apiToken.Token)
	if w.Code != http.StatusForbidden || errorCode(w) != apperrors.CodeSessionCredentialRequired {
		t.Errorf("API token: POST %s = %d %s, want 403 %s", calendarStepsEmailRoute, w.Code, w.Body.String(), apperrors.CodeSessionCredentialRequired)
	}
	w = post(calendarStepsEmailRoute, unverifiedSession)
	if w.Code != http.StatusForbidden || errorCode(w) != apperrors.CodeEmailVerificationRequired {
		t.Errorf("unverified: POST %s = %d %s, want 403 %s", calendarStepsEmailRoute, w.Code, w.Body.String(), apperrors.CodeEmailVerificationRequired)
	}
	if n := queued(); n != 0 {
		t.Fatalf("refused requests queued %d rows", n)
	}

	// Three requests are queued and sent, one at a time.
	for i := 1; i <= 3; i++ {
		w = post(calendarStepsEmailRoute, session)
		if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
			t.Fatalf("request %d: POST %s = %d %q, want 204 with no body", i, calendarStepsEmailRoute, w.Code, w.Body.String())
		}
		if n := queued(); n != int64(i) {
			t.Fatalf("request %d: %d rows queued, want %d", i, n, i)
		}
		sc.AuthEmailOutbox.RunNow(context.Background())
		if sent := decodedEmails(t, mailbox); len(sent) != i {
			t.Fatalf("request %d: %d emails sent, want %d", i, len(sent), i)
		}
	}

	email := decodedEmails(t, mailbox)[0]
	if strings.Join(email.To, ",") != *user.Email {
		t.Errorf("sent to %v, want the account's own address", email.To)
	}
	if email.Subject != "Add your saved shows to Google Calendar" {
		t.Errorf("subject = %q", email.Subject)
	}
	if len(email.Headers) != 0 {
		t.Errorf("headers = %v, want none", email.Headers)
	}
	hrefs := stepsHref.FindAllStringSubmatch(email.HTML, -1)
	if len(hrefs) != 1 || hrefs[0][1] != "https://www.psychichomily.com/library/calendar" {
		t.Errorf("links = %v, want only the frontend /library/calendar", hrefs)
	}
	for _, absent := range []string{minted.Token, "phcal_", "/feeds/", "/calendar/", ".ics", "webcal"} {
		if strings.Contains(email.raw, absent) {
			t.Errorf("the email carries %q", absent)
		}
	}

	// The fourth inside the hour is refused and queues nothing.
	w = post(calendarStepsEmailRoute, session)
	if w.Code != http.StatusTooManyRequests || errorCode(w) != apperrors.CodeCalendarStepsRateLimitedHourly {
		t.Fatalf("fourth: POST %s = %d %s, want 429 %s", calendarStepsEmailRoute, w.Code, w.Body.String(), apperrors.CodeCalendarStepsRateLimitedHourly)
	}
	retryAfter, err := strconv.Atoi(w.Header().Get("Retry-After"))
	if err != nil || retryAfter < 3500 || retryAfter > 3600 {
		t.Errorf("Retry-After = %q, want the seconds until the first request leaves the hour", w.Header().Get("Retry-After"))
	}
	if n := queued(); n != 3 {
		t.Errorf("after the refusal: %d rows queued, want 3", n)
	}
}
