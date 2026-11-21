package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// POST /calendar/token through a router built the way the server builds it,
// with the engagement-mutation limiter mounted and its opt-in flag unset: ten
// mints in a clock hour answer 200, and the eleventh answers 429 with the
// cap's code, the line the surfaces show for a link that could not be made,
// and a Retry-After of the seconds to the next clock hour. The link minted
// tenth still works and no retired row was added. Run with -v to print the
// eleventh response.
func TestCalendarLinkHourlyCapThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	flagUnset := func(string) string { return "" }
	router.Use(EngagementMutationRateLimiter(sc.JWT, noAPITokens, flagUnset))
	SetupRoutes(router, sc, cfg)

	post := func(bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/calendar/token", nil)
		req.Header.Set("Authorization", "Bearer "+bearer)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	retiredRows := func() int64 {
		var n int64
		if err := td.DB.Model(&engagementm.RetiredCalendarLink{}).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}

	// The cap is per clock hour on the real clock, so a run that crosses an
	// hour boundary starts its count again; such a run is repeated once on a
	// fresh account.
	for attempt := 1; ; attempt++ {
		user := testhelpers.CreateTestUser(td.DB)
		session := mintToken(t, sc, user)
		startHour := time.Now().UTC().Truncate(time.Hour)
		retiredAtStart := retiredRows()

		var last string
		for i := 1; i <= 10; i++ {
			w := post(session)
			if w.Code != http.StatusOK {
				t.Fatalf("mint %d: POST /calendar/token = %d; body: %s", i, w.Code, w.Body.String())
			}
			var minted struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &minted); err != nil || !strings.HasPrefix(minted.Token, "phcal_") {
				t.Fatalf("mint %d: token = %q (%v)", i, minted.Token, err)
			}
			last = minted.Token
		}

		w := post(session)
		requestHour := time.Now().UTC().Truncate(time.Hour)
		if !requestHour.Equal(startHour) {
			if attempt == 2 {
				t.Fatal("two runs in a row crossed an hour boundary")
			}
			continue
		}

		dump, err := httputil.DumpResponse(w.Result(), true)
		if err != nil {
			t.Fatalf("dump response: %v", err)
		}
		t.Logf("eleventh response:\n%s", dump)

		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("eleventh: POST /calendar/token = %d; body: %s", w.Code, w.Body.String())
		}
		var body struct {
			Success   bool   `json:"success"`
			Message   string `json:"message"`
			ErrorCode string `json:"error_code"`
			Token     string `json:"token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.ErrorCode != apperrors.CodeCalendarLinkRateLimitedHourly {
			t.Errorf("error_code = %q, want %q", body.ErrorCode, apperrors.CodeCalendarLinkRateLimitedHourly)
		}
		if body.Message != "Couldn't create your calendar link. Try again." || body.Success || body.Token != "" {
			t.Errorf("body = %+v", body)
		}
		retryAfter, err := strconv.Atoi(w.Header().Get("Retry-After"))
		toNextHour := time.Until(startHour.Add(time.Hour)).Seconds()
		if err != nil || retryAfter < 1 || float64(retryAfter) < toNextHour-5 || float64(retryAfter) > toNextHour+1 {
			t.Errorf("Retry-After = %q, want about %.0f, the seconds to the next clock hour", w.Header().Get("Retry-After"), toNextHour)
		}

		if got := retiredRows(); got != retiredAtStart+9 {
			t.Errorf("retired rows = %d, want %d: nine replacements and nothing from the refusal", got, retiredAtStart+9)
		}
		validated, err := sc.Calendar.ValidateCalendarToken(last)
		if err != nil || validated.ID != user.ID {
			t.Errorf("the link minted tenth no longer works: %v", err)
		}
		return
	}
}
