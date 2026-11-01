package routes

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	authsvc "psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/testutil"
)

// POST /auth/password driven through the ROUTER by a caller holding a real
// session for an account with no password: the re-authentication gate, the
// password validator the container wires, the write, and the budget the route
// is mounted on.

// setPasswordBreachedValue is the one password the stub breach listing reports
// as exposed; every other password is absent from it.
const setPasswordBreachedValue = "breached-password-value-1!"

// breachListing answers every pwned-passwords range request with a listing
// that holds setPasswordBreachedValue's suffix, so the real validator runs
// with no network.
type breachListing struct{}

func (breachListing) RoundTrip(*http.Request) (*http.Response, error) {
	sum := sha1.Sum([]byte(setPasswordBreachedValue))
	suffix := strings.ToUpper(hex.EncodeToString(sum[:]))[5:]
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(suffix + ":42\r\n")),
		Header:     http.Header{},
	}, nil
}

type setPasswordFixture struct {
	router *chi.Mux
	sc     *services.ServiceContainer
	td     *testutil.TestDatabase
	user   *authm.User
}

func newSetPasswordFixture(t *testing.T, email string) *setPasswordFixture {
	t.Helper()
	td := testutil.SetupTestPostgres(t)
	t.Cleanup(td.Cleanup)

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	sc.PasswordValidator = authsvc.NewPasswordValidatorWithTransport(breachListing{})

	user := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
	if err := td.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)
	return &setPasswordFixture{router: router, sc: sc, td: td, user: user}
}

// session mints a session for the fixture's account whose factor completed at
// authAt.
func (f *setPasswordFixture) session(t *testing.T, authAt time.Time) string {
	t.Helper()
	token, err := f.sc.JWT.RenewSessionToken(f.user, authAt)
	if err != nil {
		t.Fatalf("mint session: %v", err)
	}
	return token
}

func setPasswordBody(password string) string {
	return fmt.Sprintf(`{"new_password":%q}`, password)
}

type setPasswordReply struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	ErrorCode string `json:"error_code"`
}

func decodeSetPasswordReply(t *testing.T, body string) setPasswordReply {
	t.Helper()
	var reply setPasswordReply
	if err := json.Unmarshal([]byte(body), &reply); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return reply
}

func TestSetPasswordRouteThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")
	f := newSetPasswordFixture(t, "set-password-route@test.com")

	t.Run("a stale session is asked to sign in again", func(t *testing.T) {
		w := sendAuthed(t, f.router, "POST", "/auth/password", setPasswordBody("a-new-strong-password-1!"),
			"203.0.113.171:1234", f.session(t, time.Now().Add(-2*time.Hour)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body %s", w.Code, w.Body.String())
		}
		if reply := decodeSetPasswordReply(t, w.Body.String()); reply.ErrorCode != autherrors.CodeReauthRequired {
			t.Errorf("error_code = %q, want %q", reply.ErrorCode, autherrors.CodeReauthRequired)
		}
	})

	fresh := f.session(t, time.Now())

	t.Run("a breached password is refused in the validator's words", func(t *testing.T) {
		w := sendAuthed(t, f.router, "POST", "/auth/password", setPasswordBody(setPasswordBreachedValue),
			"203.0.113.172:1234", fresh)
		reply := decodeSetPasswordReply(t, w.Body.String())
		if w.Code != http.StatusOK || reply.Success || reply.ErrorCode != autherrors.CodeValidationFailed {
			t.Fatalf("got %d %+v, want a 200 VALIDATION_FAILED refusal", w.Code, reply)
		}
		if !strings.Contains(reply.Message, "data breach") {
			t.Errorf("message = %q, want the breach refusal", reply.Message)
		}
	})

	t.Run("a fresh session sets the password and keeps its session", func(t *testing.T) {
		w := sendAuthed(t, f.router, "POST", "/auth/password", setPasswordBody("a-new-strong-password-1!"),
			"203.0.113.173:1234", fresh)
		reply := decodeSetPasswordReply(t, w.Body.String())
		if w.Code != http.StatusOK || !reply.Success {
			t.Fatalf("got %d %+v, want success", w.Code, reply)
		}
		if !strings.Contains(w.Header().Get("Set-Cookie"), config.AuthCookieName+"=") {
			t.Error("success must hand back the renewed session")
		}

		var stored authm.User
		if err := f.td.DB.First(&stored, f.user.ID).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		if !stored.HasPassword() || f.sc.User.VerifyPassword(*stored.PasswordHash, "a-new-strong-password-1!") != nil {
			t.Fatal("the password was not stored")
		}
	})

	t.Run("a second set answers PASSWORD_ALREADY_SET", func(t *testing.T) {
		// A session validated after the first set loads the principal with
		// its password, so this is the early refusal; the service's own
		// refusal under the row lock is covered in services/user.
		w := sendAuthed(t, f.router, "POST", "/auth/password", setPasswordBody("another-strong-password-2!"),
			"203.0.113.174:1234", fresh)
		reply := decodeSetPasswordReply(t, w.Body.String())
		if w.Code != http.StatusOK || reply.Success || reply.ErrorCode != autherrors.CodePasswordAlreadySet {
			t.Fatalf("got %d %+v, want a PASSWORD_ALREADY_SET refusal", w.Code, reply)
		}
	})
}

// The route meters on its own counter at the password-confirm size: spending
// it does not throttle change-password, and spending change-password's does
// not throttle it.
func TestSetPasswordRouteHasItsOwnBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	f := newSetPasswordFixture(t, "set-password-budget@test.com")
	session := f.session(t, time.Now())
	// An empty password is refused by the handler before the validator runs,
	// so these requests reach the handler and change nothing.
	const emptyBody = `{"new_password":""}`
	changePassword := passwordConfirmBudgetRoutes[0]

	t.Run("its own budget throttles it", func(t *testing.T) {
		const ip = "203.0.113.181:1234"
		for i := 0; i < PasswordConfirmAttemptsPerMinute; i++ {
			w := sendAuthed(t, f.router, "POST", "/auth/password", emptyBody, ip, session)
			if !strings.Contains(w.Body.String(), "New password is required") {
				t.Fatalf("attempt %d did not reach the handler: %d %s", i+1, w.Code, w.Body.String())
			}
		}
		w := sendAuthed(t, f.router, "POST", "/auth/password", emptyBody, ip, session)
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("request past the budget returned %d, want 429", w.Code)
		}
		if got := w.Header().Get("Retry-After"); got != "60" {
			t.Errorf("429 carried Retry-After %q, want 60", got)
		}

		w = sendAuthed(t, f.router, "POST", changePassword.path, changePassword.body, ip, session)
		if w.Code == http.StatusTooManyRequests {
			t.Error("change-password was throttled by the set-password budget; the two share a counter")
		}
	})

	t.Run("the password-confirm budget does not throttle it", func(t *testing.T) {
		const ip = "203.0.113.182:1234"
		for i := 0; i < PasswordConfirmAttemptsPerMinute; i++ {
			sendAuthed(t, f.router, "POST", changePassword.path, changePassword.body, ip, session)
		}
		if w := sendAuthed(t, f.router, "POST", changePassword.path, changePassword.body, ip, session); w.Code != http.StatusTooManyRequests {
			t.Fatalf("change-password past its budget returned %d, want 429", w.Code)
		}
		if w := sendAuthed(t, f.router, "POST", "/auth/password", emptyBody, ip, session); w.Code == http.StatusTooManyRequests {
			t.Error("set-password was throttled by the password-confirm budget; the two share a counter")
		}
	})
}
