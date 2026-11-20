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

// feedRoute is one public address a feed token is served at.
type feedRoute struct {
	name       string
	path       func(token string) string
	atom       bool   // the follows feed rather than a calendar
	invalidLog string // the warning an unknown token is logged with
}

var feedRoutes = []feedRoute{
	{"saved-shows.ics", func(tok string) string { return "/feeds/" + tok + "/saved-shows.ics" }, false, "msg=calendar_feed_invalid_token"},
	{"legacy calendar", func(tok string) string { return "/calendar/" + tok }, false, "msg=calendar_feed_invalid_token"},
	{"follows.atom", func(tok string) string { return "/feeds/" + tok + "/follows.atom" }, true, "msg=follows_activity_feed_invalid_token"},
}

// retiredWording is the text a retired address answers with for one reason,
// as it appears on the wire.
type retiredWording struct {
	icsSummary, atomTitle, atomContent string
}

var retiredWordings = map[engagementm.CalendarLinkRetiredReason]retiredWording{
	engagementm.CalendarLinkRetiredReset: {
		"SUMMARY:Psychic Homily: this calendar link was reset. Add it again from your Library.",
		"<title>This feed link was reset</title>",
		`<content type="text">Add it again from Settings on Psychic Homily.</content>`,
	},
	engagementm.CalendarLinkRetiredReplaced: {
		"SUMMARY:Psychic Homily: this calendar link was replaced. Add the new one from your Library.",
		"<title>This feed link was replaced</title>",
		`<content type="text">Copy the new one from Settings on Psychic Homily.</content>`,
	},
	engagementm.CalendarLinkRetiredTurnedOff: {
		"SUMMARY:Psychic Homily: this calendar link was turned off. Turn it on again from your Library.",
		"<title>This feed link was turned off</title>",
		`<content type="text">Turn it on again from Settings on Psychic Homily.</content>`,
	},
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

	fetch := func(t *testing.T, path, ifNoneMatch string) fetchedFeed {
		t.Helper()
		w := httptest.NewRecorder()
		log := testlog.Capture(t, func() {
			r := testlog.Request(t, httptest.NewRequest(http.MethodGet, path, nil))
			r.Header.Set("User-Agent", "Google-Calendar-Importer")
			if ifNoneMatch != "" {
				r.Header.Set("If-None-Match", ifNoneMatch)
			}
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

	cases := []struct {
		name   string
		reason engagementm.CalendarLinkRetiredReason // "" for a deletion that remembers nothing
		// retire stops the user's link and returns the new link, if any.
		retire func(t *testing.T, userID uint) string
	}{
		{"New link", engagementm.CalendarLinkRetiredReplaced, func(t *testing.T, userID uint) string {
			return mint(t, userID)
		}},
		{"Turn off", engagementm.CalendarLinkRetiredTurnedOff, func(t *testing.T, userID uint) string {
			if err := sc.Calendar.DeleteToken(userID); err != nil {
				t.Fatalf("turn off: %v", err)
			}
			return ""
		}},
		{"sign out everywhere", engagementm.CalendarLinkRetiredReset, func(t *testing.T, userID uint) string {
			user, err := sc.User.GetUserByID(userID)
			if err != nil {
				t.Fatalf("load user: %v", err)
			}
			if _, err := sc.Auth.SignOutOtherSessions(user, time.Now()); err != nil {
				t.Fatalf("sign out everywhere: %v", err)
			}
			return ""
		}},
		{"account deletion", "", func(t *testing.T, userID uint) string {
			if err := sc.User.PermanentlyDeleteUser(userID); err != nil {
				t.Fatalf("delete account: %v", err)
			}
			return ""
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			user := testhelpers.CreateTestUser(td.DB)
			old := mint(t, user.ID)
			newToken := c.retire(t, user.ID)

			for _, route := range feedRoutes {
				path := route.path(old)
				redacted := strings.ReplaceAll(path, old, "phcal_<redacted>")

				cfg.Calendar.RetiredLinkNoticeEnabled = false
				off := fetch(t, path, "")
				t.Logf("switch off, GET %s\n%s", redacted, off.dump)
				if off.status != http.StatusUnauthorized || off.body != "invalid or expired token\n" {
					t.Errorf("%s switched off: %d %q", route.name, off.status, off.body)
				}
				if strings.Count(off.log, "level=WARN") != 1 || !strings.Contains(off.log, route.invalidLog) {
					t.Errorf("%s switched off logged %q", route.name, off.log)
				}

				cfg.Calendar.RetiredLinkNoticeEnabled = true
				on := fetch(t, path, "")
				t.Logf("switch on, GET %s\n%s", redacted, on.dump)

				if c.reason == "" {
					if on.status != http.StatusUnauthorized || on.body != off.body || on.log != off.log {
						t.Errorf("%s: a deleted account's address must answer as unknown, got %d %q %q", route.name, on.status, on.body, on.log)
					}
					continue
				}
				if on.status != http.StatusOK {
					t.Fatalf("%s switched on: status %d", route.name, on.status)
				}
				if strings.Contains(on.log, "level=WARN") || strings.Contains(on.log, "level=ERROR") {
					t.Errorf("%s switched on logged %q", route.name, on.log)
				}
				if on.header.Get("ETag") == "" || on.header.Get("Cache-Control") != "private, max-age=3600" {
					t.Errorf("%s cache headers: %v", route.name, on.header)
				}
				want := retiredWordings[c.reason]
				if route.atom {
					if !strings.Contains(on.body, want.atomTitle) || !strings.Contains(on.body, want.atomContent) {
						t.Errorf("%s body lacks the entry for %s: %s", route.name, c.reason, on.body)
					}
				} else if !strings.Contains(unfoldICS(on.body), "\r\n"+want.icsSummary+"\r\n") {
					t.Errorf("%s body lacks the note for %s: %s", route.name, c.reason, on.body)
				}
				if strings.Contains(on.body, old) {
					t.Errorf("%s notice carries the token", route.name)
				}

				if revalidated := fetch(t, path, on.header.Get("ETag")); revalidated.status != http.StatusNotModified || revalidated.body != "" {
					t.Errorf("%s revalidation: %d %q", route.name, revalidated.status, revalidated.body)
				}
			}

			if newToken != "" {
				live := fetch(t, feedRoutes[0].path(newToken), "")
				if live.status != http.StatusOK || !strings.Contains(live.body, "DESCRIPTION:Your saved shows from Psychic Homily") {
					t.Errorf("the new link must serve the live feed: %d %q", live.status, live.body)
				}
			}
		})
	}
}
