package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	authsvc "psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/notification"
	"psychic-homily-backend/internal/testutil"
)

// The emailed password reset driven through the ROUTER: the request queues a
// row, the outbox sends the real email through a recording provider, and the
// link in that email opens, sets the password once, signs this browser in
// and signs the earlier session out, and the calendar-feed URL stops working.

// sentEmails records every email the provider is asked to send.
type sentEmails struct {
	mu    sync.Mutex
	html  []string
	inbox []string
}

func (s *sentEmails) RoundTrip(req *http.Request) (*http.Response, error) {
	var msg struct {
		To   []string `json:"to"`
		HTML string   `json:"html"`
	}
	body, _ := io.ReadAll(req.Body)
	_ = json.Unmarshal(body, &msg)
	s.mu.Lock()
	s.html = append(s.html, msg.HTML)
	s.inbox = append(s.inbox, strings.Join(msg.To, ","))
	s.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"sent"}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

var resetLinkPattern = regexp.MustCompile(`/auth/reset-password\?token=([A-Za-z0-9._-]+)`)

type passwordResetFixture struct {
	router *chi.Mux
	sc     *services.ServiceContainer
	td     *testutil.TestDatabase
	sent   *sentEmails
}

func newPasswordResetFixture(t *testing.T) *passwordResetFixture {
	t.Helper()
	td := testutil.SetupTestPostgres(t)
	t.Cleanup(td.Cleanup)

	cfg := testConfig()
	cfg.Email.FrontendURL = setPasswordFrontend
	cfg.Email.ResendAPIKey = "re_fake_key_for_testing"
	cfg.Email.FromEmail = "test@psychichomily.com"
	sc := services.NewServiceContainer(td.DB, cfg)
	sent := &sentEmails{}
	sc.Email = notification.NewEmailServiceWithTransport(cfg, sent)
	sc.AuthEmailOutbox = authsvc.NewAuthEmailOutbox(td.DB, sc.User, sc.JWT, sc.Email)
	sc.PasswordValidator = authsvc.NewPasswordValidatorWithTransport(breachListing{})

	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)
	return &passwordResetFixture{router: router, sc: sc, td: td, sent: sent}
}

func (f *passwordResetFixture) account(t *testing.T, email, password string, active bool) *authm.User {
	t.Helper()
	user := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		h := string(hash)
		user.PasswordHash = &h
	}
	if err := f.td.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if !active {
		testutil.DeactivateAccount(t, f.td.DB, user.ID)
	}
	return user
}

func (f *passwordResetFixture) post(t *testing.T, path, body, session string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	req.RemoteAddr = "203.0.113.201:1234"
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func (f *passwordResetFixture) profileStatus(t *testing.T, session string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/auth/profile", nil)
	req.Header.Set("Authorization", "Bearer "+session)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w.Code
}

func (f *passwordResetFixture) queuedResets(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := f.td.DB.Model(&authm.AuthEmailOutboxItem{}).Where("kind = ?", authm.AuthEmailKindPasswordReset).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// sendQueued runs the outbox once and returns the reset link of the newest
// email, failing when none was sent.
func (f *passwordResetFixture) sendQueued(t *testing.T) string {
	t.Helper()
	f.sc.AuthEmailOutbox.RunNow(context.Background())
	f.sent.mu.Lock()
	defer f.sent.mu.Unlock()
	if len(f.sent.html) == 0 {
		t.Fatal("the outbox sent no email")
	}
	match := resetLinkPattern.FindStringSubmatch(f.sent.html[len(f.sent.html)-1])
	if match == nil {
		t.Fatalf("the email carries no reset link: %s", f.sent.html[len(f.sent.html)-1])
	}
	return match[1]
}

type resetReply struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Email     string `json:"email"`
	ErrorCode string `json:"error_code"`
}

func decodeResetReply(t *testing.T, w *httptest.ResponseRecorder) resetReply {
	t.Helper()
	var reply resetReply
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatalf("decoding %q: %v", w.Body.String(), err)
	}
	return reply
}

func TestPasswordResetRoutesThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")
	f := newPasswordResetFixture(t)
	owner := f.account(t, "reset-owner@test.com", "planted-password-1!", true)
	f.account(t, "reset-closed@test.com", "closed-password-1!", false)
	squatterSession, err := f.sc.JWT.CreateToken(owner)
	if err != nil {
		t.Fatal(err)
	}
	squatterFeed, err := f.sc.Calendar.CreateToken(owner.ID, nil, "http://localhost:8080")
	if err != nil {
		t.Fatalf("create calendar token: %v", err)
	}
	if code := feedStatus(t, f.router, squatterFeed.Token); code != http.StatusOK {
		t.Fatalf("before the reset, the calendar feed answered %d, want 200", code)
	}

	unknown := f.post(t, "/auth/password-reset/request", `{"email":"nobody@test.com"}`, "")
	closed := f.post(t, "/auth/password-reset/request", `{"email":"reset-closed@test.com"}`, "")
	for name, w := range map[string]*httptest.ResponseRecorder{"an unknown address": unknown, "a closed account": closed} {
		if w.Code != http.StatusAccepted {
			t.Fatalf("%s: status = %d, want 202; body %s", name, w.Code, w.Body.String())
		}
	}
	if a, b := decodeResetReply(t, unknown), decodeResetReply(t, closed); a != b {
		t.Errorf("an unknown address and a closed account must get the same body: %+v vs %+v", a, b)
	}
	if n := f.queuedResets(t); n != 0 {
		t.Fatalf("queued %d resets for an unknown address and a closed account, want none", n)
	}

	known := f.post(t, "/auth/password-reset/request", `{"email":"reset-owner@test.com"}`, "")
	if known.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body %s", known.Code, known.Body.String())
	}
	if a, b := decodeResetReply(t, known), decodeResetReply(t, unknown); a != b {
		t.Errorf("a known address must get the same body as an unknown one: %+v vs %+v", a, b)
	}
	link := f.sendQueued(t)
	if got := f.sent.inbox[len(f.sent.inbox)-1]; got != "reset-owner@test.com" {
		t.Errorf("the reset went to %q, want the account's address", got)
	}

	check := decodeResetReply(t, f.post(t, "/auth/password-reset/check", fmt.Sprintf(`{"token":%q}`, link), ""))
	if !check.Success || check.Email != "reset-owner@test.com" {
		t.Fatalf("check = %+v, want the account's address", check)
	}

	breached := decodeResetReply(t, f.post(t, "/auth/password-reset/complete",
		fmt.Sprintf(`{"token":%q,"new_password":%q}`, link, setPasswordBreachedValue), ""))
	if breached.Success || breached.ErrorCode != "VALIDATION_FAILED" {
		t.Fatalf("a breached password = %+v, want VALIDATION_FAILED", breached)
	}

	done := f.post(t, "/auth/password-reset/complete", fmt.Sprintf(`{"token":%q,"new_password":"owners-new-password-1!"}`, link), "")
	if reply := decodeResetReply(t, done); !reply.Success {
		t.Fatalf("complete = %+v, want success", reply)
	}
	var session string
	for _, c := range done.Result().Cookies() {
		if c.Name == config.AuthCookieName {
			session = c.Value
		}
	}
	if session == "" {
		t.Fatal("the reset set no session cookie")
	}
	if code := f.profileStatus(t, session); code != http.StatusOK {
		t.Errorf("the new session reads the profile with %d, want 200", code)
	}
	if code := f.profileStatus(t, squatterSession); code != http.StatusUnauthorized {
		t.Errorf("the earlier session reads the profile with %d, want 401", code)
	}
	if code := feedStatus(t, f.router, squatterFeed.Token); code != http.StatusUnauthorized {
		t.Errorf("after the reset, the calendar feed answered %d, want 401", code)
	}

	login := f.post(t, "/auth/login", `{"email":"reset-owner@test.com","password":"owners-new-password-1!"}`, "")
	if !strings.Contains(login.Body.String(), `"success":true`) {
		t.Errorf("signing in with the new password: %s", login.Body.String())
	}
	planted := f.post(t, "/auth/login", `{"email":"reset-owner@test.com","password":"planted-password-1!"}`, "")
	if strings.Contains(planted.Body.String(), `"success":true`) {
		t.Error("the planted password still signs in")
	}

	for path, body := range map[string]string{
		"/auth/password-reset/check":    fmt.Sprintf(`{"token":%q}`, link),
		"/auth/password-reset/complete": fmt.Sprintf(`{"token":%q,"new_password":"someone-elses-password-1!"}`, link),
	} {
		reply := decodeResetReply(t, f.post(t, path, body, ""))
		if reply.Success || reply.ErrorCode != "INVALID_TOKEN" {
			t.Errorf("%s with a used link = %+v, want INVALID_TOKEN", path, reply)
		}
	}

	// Settings' send goes to the signed-in account's own address.
	own := f.post(t, "/auth/password-reset/send-own", "", session)
	if own.Code != http.StatusAccepted {
		t.Fatalf("send-own status = %d, want 202; body %s", own.Code, own.Body.String())
	}
	if ownLink := f.sendQueued(t); ownLink == link {
		t.Error("send-own must mint a new link")
	}
	if got := f.sent.inbox[len(f.sent.inbox)-1]; got != "reset-owner@test.com" {
		t.Errorf("send-own went to %q, want the account's own address", got)
	}
	if w := f.post(t, "/auth/password-reset/send-own", "", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("send-own with no session = %d, want 401", w.Code)
	}
}

// The signed-out request and Settings' send share one counter.
func TestPasswordResetSendRoutesShareOneBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	f := newPasswordResetFixture(t)
	owner := f.account(t, "reset-budget@test.com", "", true)
	session, err := f.sc.JWT.CreateToken(owner)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < VerificationResendPerMinute; i++ {
		path, body, sess := "/auth/password-reset/request", `{"email":"reset-budget@test.com"}`, ""
		if i%2 == 1 {
			path, body, sess = "/auth/password-reset/send-own", "", session
		}
		if w := f.post(t, path, body, sess); w.Code == http.StatusTooManyRequests {
			t.Fatalf("%s (request %d/%d) was limited early", path, i+1, VerificationResendPerMinute)
		}
	}
	if w := f.post(t, "/auth/password-reset/send-own", "", session); w.Code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429: the two send routes must share one budget", VerificationResendPerMinute+1, w.Code)
	}
	// The public auth budget is a different counter.
	if w := f.post(t, "/auth/password-reset/check", `{"token":"x"}`, ""); w.Code == http.StatusTooManyRequests {
		t.Error("the send budget must not meter the link's check")
	}
}
