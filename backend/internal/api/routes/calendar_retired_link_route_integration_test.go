package routes

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testlog"
	"psychic-homily-backend/internal/testutil"
)

// The three public feed addresses a token is served at.
func feedPaths(token string) map[string]string {
	return map[string]string{
		"saved-shows.ics": "/feeds/" + token + "/saved-shows.ics",
		"legacy calendar": "/calendar/" + token,
		"follows.atom":    "/feeds/" + token + "/follows.atom",
	}
}

// fetchedFeed is one response through the router and what was logged for it.
type fetchedFeed struct {
	status int
	header http.Header
	body   string
	log    string
	dump   string
}

// logTime is a log record's timestamp, stripped so two requests' logs compare.
var logTime = regexp.MustCompile(`time=\S+ `)

// unfoldICS joins RFC 5545 folded lines, so a long SUMMARY reads whole.
func unfoldICS(body string) string {
	return strings.ReplaceAll(body, "\r\n ", "")
}

// Every way a feed link stops, through the built router, the real services
// and a real database. Switched off, each old address on each of the three
// routes answers the 401 and warning an unknown address gets. Switched on,
// the old address answers 200 with the note for its reason, the new link
// still serves the live feed, and a deleted account's address still answers
// as unknown. Run with -v to print the raw responses.
func TestRetiredFeedLinksThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	cfg.Email.FrontendURL = "https://www.psychichomily.com"
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	fetch := func(t *testing.T, path string) fetchedFeed {
		t.Helper()
		w := httptest.NewRecorder()
		var r *http.Request
		log := testlog.Capture(t, func() {
			r = testlog.Request(t, httptest.NewRequest(http.MethodGet, path, nil))
			r.Header.Set("User-Agent", "Google-Calendar-Importer")
			router.ServeHTTP(w, r)
		})
		dump, err := httputil.DumpResponse(w.Result(), true)
		if err != nil {
			t.Fatalf("dump response: %v", err)
		}
		return fetchedFeed{status: w.Code, header: w.Header(), body: w.Body.String(), log: logTime.ReplaceAllString(log, ""), dump: string(dump)}
	}

	mint := func(t *testing.T, userID uint) string {
		t.Helper()
		created, err := sc.Calendar.CreateToken(userID, nil, "http://localhost:8080")
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		return created.Token
	}

	type retired struct {
		name     string
		reason   engagementm.CalendarLinkRetiredReason // "" for a deletion that remembers nothing
		oldToken string
		newToken string
	}
	var cases []retired

	{ // New link
		user := testhelpers.CreateTestUser(td.DB)
		old := mint(t, user.ID)
		cases = append(cases, retired{"New link", engagementm.CalendarLinkRetiredReplaced, old, mint(t, user.ID)})
	}
	{ // Turn off
		user := testhelpers.CreateTestUser(td.DB)
		old := mint(t, user.ID)
		if err := sc.Calendar.DeleteToken(user.ID); err != nil {
			t.Fatalf("turn off: %v", err)
		}
		cases = append(cases, retired{"Turn off", engagementm.CalendarLinkRetiredTurnedOff, old, ""})
	}
	{ // Sign out everywhere
		user := testhelpers.CreateTestUser(td.DB)
		old := mint(t, user.ID)
		if _, err := sc.Auth.SignOutOtherSessions(user, time.Now()); err != nil {
			t.Fatalf("sign out everywhere: %v", err)
		}
		cases = append(cases, retired{"sign out everywhere", engagementm.CalendarLinkRetiredReset, old, ""})
	}
	{ // Account deletion
		user := testhelpers.CreateTestUser(td.DB)
		old := mint(t, user.ID)
		if err := sc.User.PermanentlyDeleteUser(user.ID); err != nil {
			t.Fatalf("delete account: %v", err)
		}
		cases = append(cases, retired{"account deletion", "", old, ""})
	}

	notes := map[engagementm.CalendarLinkRetiredReason][2]string{
		engagementm.CalendarLinkRetiredReset:     {"SUMMARY:Psychic Homily: this calendar link was reset. Add it again from your Library.", "<title>This feed link was reset</title>"},
		engagementm.CalendarLinkRetiredReplaced:  {"SUMMARY:Psychic Homily: this calendar link was replaced. Add the new one from your Library.", "<title>This feed link was replaced</title>"},
		engagementm.CalendarLinkRetiredTurnedOff: {"SUMMARY:Psychic Homily: this calendar link was turned off. Turn it on again from your Library.", "<title>This feed link was turned off</title>"},
	}
	feedBodies := map[engagementm.CalendarLinkRetiredReason]string{
		engagementm.CalendarLinkRetiredReset:     `<content type="text">Add it again from Settings on Psychic Homily.</content>`,
		engagementm.CalendarLinkRetiredReplaced:  `<content type="text">Copy the new one from Settings on Psychic Homily.</content>`,
		engagementm.CalendarLinkRetiredTurnedOff: `<content type="text">Turn it on again from Settings on Psychic Homily.</content>`,
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for route, path := range feedPaths(c.oldToken) {
				redacted := strings.ReplaceAll(path, c.oldToken, "phcal_<redacted>")
				invalidLog := "msg=calendar_feed_invalid_token"
				if route == "follows.atom" {
					invalidLog = "msg=follows_activity_feed_invalid_token"
				}

				cfg.Calendar.RetiredLinkNoticeEnabled = false
				off := fetch(t, path)
				t.Logf("switch off, GET %s\n%s", redacted, off.dump)
				if off.status != http.StatusUnauthorized || off.body != "invalid or expired token\n" {
					t.Errorf("%s switched off: %d %q", route, off.status, off.body)
				}
				if strings.Count(off.log, "level=WARN") != 1 || !strings.Contains(off.log, invalidLog) {
					t.Errorf("%s switched off logged %q", route, off.log)
				}

				cfg.Calendar.RetiredLinkNoticeEnabled = true
				on := fetch(t, path)
				t.Logf("switch on, GET %s\n%s", redacted, on.dump)

				if c.reason == "" {
					if on.status != http.StatusUnauthorized || on.body != off.body || on.log != off.log {
						t.Errorf("%s: a deleted account's address must answer as unknown, got %d %q %q", route, on.status, on.body, on.log)
					}
					continue
				}
				if on.status != http.StatusOK {
					t.Fatalf("%s switched on: status %d", route, on.status)
				}
				if strings.Contains(on.log, "level=WARN") || strings.Contains(on.log, "level=ERROR") {
					t.Errorf("%s switched on logged %q", route, on.log)
				}
				if on.header.Get("ETag") == "" || on.header.Get("Cache-Control") != "private, max-age=3600" {
					t.Errorf("%s cache headers: %v", route, on.header)
				}
				if route == "follows.atom" {
					if !strings.Contains(on.body, notes[c.reason][1]) || !strings.Contains(on.body, feedBodies[c.reason]) {
						t.Errorf("%s body lacks the entry for %s: %s", route, c.reason, on.body)
					}
				} else if !strings.Contains(unfoldICS(on.body), "\r\n"+notes[c.reason][0]+"\r\n") {
					t.Errorf("%s body lacks the note for %s: %s", route, c.reason, on.body)
				}
				if strings.Contains(on.body, c.oldToken) {
					t.Errorf("%s notice carries the token", route)
				}

				revalidated := func() fetchedFeed {
					w := httptest.NewRecorder()
					r := httptest.NewRequest(http.MethodGet, path, nil)
					r.Header.Set("If-None-Match", on.header.Get("ETag"))
					router.ServeHTTP(w, r)
					return fetchedFeed{status: w.Code, body: w.Body.String()}
				}()
				if revalidated.status != http.StatusNotModified || revalidated.body != "" {
					t.Errorf("%s revalidation: %d %q", route, revalidated.status, revalidated.body)
				}
			}

			if c.newToken != "" {
				live := fetch(t, feedPaths(c.newToken)["saved-shows.ics"])
				if live.status != http.StatusOK || !strings.Contains(live.body, "DESCRIPTION:Your saved shows from Psychic Homily") {
					t.Errorf("the new link must serve the live feed: %d %q", live.status, live.body)
				}
			}
		})
	}
}
