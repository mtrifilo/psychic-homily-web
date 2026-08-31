package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/utils"
)

// PSY-1983: a comment SUBSCRIPTION carries the show detail route's visibility
// rule, at the moment it is made and on every read of it afterwards.
//
// PSY-1939 closed the show's own sub-resources and disclosed this family as the
// leak it had not reached. Subscribing validated the entity TYPE and nothing
// else, so a logged-in stranger could post one guessed id and turn
// GET /me/comment-subscriptions into a monitored feed of a private show — its
// title, its slug, its URL and a live comment count that moved whenever the show
// did. The same enrichment rendered the notification inbox, and the comment
// fan-out mailed the title and an excerpt to every subscriber on every new
// comment.
//
// Four tiers on every route, because a service test cannot assert the one that
// matters: below the auth boundary a service sees a user id, and an id that owns
// nothing is indistinguishable from no id at all. Only here does a real
// credential travel through the real middleware onto the real router.
//
// Read together with show_subresource_visibility_test.go, which is the same
// shape for the routes addressed by show id. This file covers the ones addressed
// by the CALLER — a watching list and an inbox are not reached by a show id at
// all, which is why the gate on the show's own routes never touched them.

const (
	// Independent of each other and of the sibling file's fixtures, so no
	// assertion here is satisfied by a value another one put in the body.
	watchGatedTitle   = "Basement Set On Mulberry Street"
	watchOpenTitle    = "Open Bill At The Rebel Lounge"
	watchGatedComment = "the amp lives behind the washing machine, mind the step"
	watchOpenComment  = "the second band went on late but it was worth the wait"
	watchLaterComment = "posting this while the show is private, nobody should see it"
)

func TestCommentSubscriptionsMirrorTheDetailRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	admin := testhelpers.CreateAdminUser(td.DB)
	// Tiered, because a new user's comments land in pending_review and never
	// reach a listing: an untiered author would make every read below empty and
	// every gate look correct.
	submitter := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	commenter := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	stranger := testhelpers.CreateTestUser(td.DB)
	// A second stranger, for the unsubscribe subtest alone. Removing the first
	// one's subscription would silently disarm every assertion after it.
	quitter := testhelpers.CreateTestUser(td.DB)

	// Past-dated for the same reason the sibling file dates its fixtures in the
	// past, and both approved to begin with: the reported repro is a show that
	// was public when it was subscribed to.
	gated := testhelpers.CreatePastApprovedShow(td.DB, submitter.ID, watchGatedTitle, 3)
	open := testhelpers.CreatePastApprovedShow(td.DB, submitter.ID, watchOpenTitle, 3)

	seeder := newCommentSeeder(td.DB)
	gatedCommentID := seedComment(t, seeder, commenter.ID, gated.ID, watchGatedComment)
	openCommentID := seedComment(t, seeder, commenter.ID, open.ID, watchOpenComment)

	token := func(u *authm.User) string {
		t.Helper()
		tok, err := sc.JWT.CreateToken(u)
		if err != nil {
			t.Fatalf("mint token for user %d: %v", u.ID, err)
		}
		return tok
	}

	// Credentials ride the cookie, which is the carrier the product uses.
	do := func(t *testing.T, method, path, credential string, body []byte) (int, []byte) {
		t.Helper()
		var req *http.Request
		if body == nil {
			req = httptest.NewRequest(method, path, nil)
		} else {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		}
		if credential != "" {
			req.AddCookie(&http.Cookie{Name: "auth_token", Value: credential})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code, w.Body.Bytes()
	}
	get := func(t *testing.T, path, credential string) (int, []byte) {
		t.Helper()
		return do(t, http.MethodGet, path, credential, nil)
	}
	subscribePath := func(showID uint) string {
		return fmt.Sprintf("/entities/show/%d/subscribe", showID)
	}

	// Everybody subscribes to BOTH shows WHILE THEY ARE APPROVED, over the real
	// route. That is the repro, and it is also what makes the assertions below
	// about a show taken private afterwards mean anything: the subscribe gate
	// alone would leave every row made before this ticket shipped still leaking.
	for _, u := range []*authm.User{stranger, quitter, submitter, admin} {
		for _, show := range []*catalogm.Show{gated, open} {
			if code, body := do(t, http.MethodPost, subscribePath(show.ID), token(u), nil); code != http.StatusOK {
				t.Fatalf("subscribe user %d to approved show %d = %d; body: %s", u.ID, show.ID, code, body)
			}
		}
	}

	// Fan both comments out synchronously. In production this runs in a
	// goroutine off comment creation; called directly, the inbox rows exist
	// before the first assertion rather than racing it.
	fanOut(t, sc, gatedCommentID)
	fanOut(t, sc, openCommentID)

	// The stranger's inbox now holds exactly two rows, one per show, and that
	// pair is what every suppression assertion below is measured against.
	assertInbox(t, get, token(stranger), "before the show is gated", 2, 2)

	// Now take the subject private, and read the status back rather than trust
	// the update: a fixture left approved would make every assertion below pass
	// for the wrong reason.
	if err := td.DB.Model(&catalogm.Show{}).Where("id = ?", gated.ID).
		Update("status", catalogm.ShowStatusPrivate).Error; err != nil {
		t.Fatalf("make show private: %v", err)
	}
	assertShowStatus(t, td.DB, gated.ID, catalogm.ShowStatusPrivate)
	assertShowStatus(t, td.DB, open.ID, catalogm.ShowStatusApproved)

	callers := []subResourceCaller{
		{"anonymous", "", false},
		{"an authenticated stranger", token(stranger), false},
		{"the submitter", token(submitter), true},
		{"an admin", token(admin), true},
	}

	// The route every gate below mirrors, pinned here as well as in the sibling
	// file: these two files gate different surfaces on one rule, and a change to
	// the rule must fail in both rather than leave them describing different
	// ones.
	t.Run("the detail route it mirrors", func(t *testing.T) {
		for _, c := range callers {
			want := http.StatusNotFound
			if c.mayRead {
				want = http.StatusOK
			}
			if code, body := get(t, fmt.Sprintf("/shows/%d", gated.ID), c.token); code != want {
				t.Errorf("GET the gated show as %s = %d, want %d; body: %s", c.name, code, want, body)
			}
		}
	})

	// The WRITE gate. A subscription is a standing request for a show's
	// activity, so it is refused exactly as an id nobody has ever used is
	// refused — the pair being distinguishable is the whole oracle, over an id
	// space dense enough to walk.
	const absentShowID = uint(99999999)
	writeRoutes := []struct {
		name string
		path func(showID uint) string
	}{
		{"subscribe", subscribePath},
		{"mark-read", func(id uint) string { return fmt.Sprintf("/entities/show/%d/mark-read", id) }},
	}
	for _, w := range writeRoutes {
		w := w
		t.Run("the "+w.name+" route refuses like a missing show", func(t *testing.T) {
			for _, c := range callers {
				gatedCode, gatedBody := do(t, http.MethodPost, w.path(gated.ID), c.token, nil)
				absentCode, absentBody := do(t, http.MethodPost, w.path(absentShowID), c.token, nil)

				if c.mayRead {
					if gatedCode != http.StatusOK {
						t.Errorf("POST %s on the gated show as %s = %d, want 200; body: %s",
							w.name, c.name, gatedCode, gatedBody)
					}
					continue
				}
				if gatedCode < 400 {
					t.Errorf("POST %s on the gated show as %s = %d, want a refusal; body: %s",
						w.name, c.name, gatedCode, gatedBody)
				}
				// The caller's own id is echoed into one of these messages, and
				// echoing a caller's own input discloses nothing. Normalised out
				// so the comparison is about everything else: any OTHER
				// difference sorts real gated shows from ids never used.
				gatedNorm := strings.ReplaceAll(string(gatedBody), fmt.Sprint(gated.ID), "<id>")
				absentNorm := strings.ReplaceAll(string(absentBody), fmt.Sprint(absentShowID), "<id>")
				if gatedCode != absentCode || !sameErrorMessage([]byte(gatedNorm), []byte(absentNorm)) {
					t.Errorf("POST %s answers %d/%s for a gated show but %d/%s for a nonexistent one as %s — the difference is the leak",
						w.name, gatedCode, gatedNorm, absentCode, absentNorm, c.name)
				}
			}

			// The control. A gate that simply broke the route could not pass this.
			if code, body := do(t, http.MethodPost, w.path(open.ID), token(stranger), nil); code != http.StatusOK {
				t.Errorf("POST %s on the APPROVED show as a stranger = %d, want 200; body: %s", w.name, code, body)
			}
		})
	}

	// The status route is the one that must NOT refuse. It reports a live unread
	// count, so the truthful answer is a running measure of how busy a show the
	// caller cannot see is — and a 404 would confirm the id. An id nobody has
	// used already answers `{subscribed:false, unread_count:0}`, so a gated show
	// answers that, byte for byte.
	t.Run("the status route answers not-subscribed", func(t *testing.T) {
		statusPath := func(id uint) string { return fmt.Sprintf("/entities/show/%d/subscribe/status", id) }
		for _, c := range callers {
			gatedCode, gatedBody := get(t, statusPath(gated.ID), c.token)
			if c.mayRead {
				if gatedCode != http.StatusOK || !strings.Contains(string(gatedBody), `"subscribed":true`) {
					t.Errorf("the status route hid the gated show's subscription from %s: %d; body: %s",
						c.name, gatedCode, gatedBody)
				}
				continue
			}
			// Two assertions, and the second is not redundant. Matching the
			// absent id is the no-oracle property, but a prior subtest that
			// subscribed or marked-read the absent id could make BOTH answers
			// wrong in the same way and the comparison would still hold. So the
			// content is pinned outright as well: the answer is the one an
			// unsubscribed caller gets, whatever else has happened.
			if gatedCode == http.StatusOK {
				var parsed contracts.SubscriptionStatusResponse
				if err := json.Unmarshal(gatedBody, &parsed); err != nil {
					t.Fatalf("parse the status response for %s: %v; body: %s", c.name, err, gatedBody)
				}
				if parsed.Subscribed || parsed.UnreadCount != 0 {
					t.Errorf("the status route told %s they are subscribed to the gated show (unread=%d); body: %s",
						c.name, parsed.UnreadCount, gatedBody)
				}
			}

			absentCode, absentBody := get(t, statusPath(absentShowID), c.token)
			// A refusal body carries a per-request id, so the two are compared
			// by their human-readable fields; a success body carries none and is
			// compared whole, where a difference in any field including the
			// count would restore the oracle.
			same := sameJSON(gatedBody, absentBody)
			if gatedCode >= 400 {
				same = sameErrorMessage(gatedBody, absentBody)
			}
			if gatedCode != absentCode || !same {
				t.Errorf("the status route answers %s differently for a gated show than for a nonexistent one: %s vs %s",
					c.name, gatedBody, absentBody)
			}
		}
	})

	// The watching list. Not addressed by a show id at all, which is why the
	// gate on the show's own routes never reached it.
	t.Run("the watching list suppresses the entry and its count", func(t *testing.T) {
		for _, c := range callers {
			if c.token == "" {
				// Protected route: anonymous has no list to suppress anything
				// from, and answers 401 whatever the show's status.
				if code, _ := get(t, "/me/comment-subscriptions?limit=100", ""); code != http.StatusUnauthorized {
					t.Errorf("the watching list answered anonymous %d, want 401", code)
				}
				continue
			}
			items, total, body := watchingList(t, get, c.token)

			if c.mayRead {
				if !strings.Contains(string(body), watchGatedTitle) {
					t.Errorf("the watching list withheld the gated show from %s, who may read it; body: %s", c.name, body)
				}
			} else {
				if strings.Contains(string(body), watchGatedTitle) {
					t.Errorf("the watching list published %q to %s; body: %s", watchGatedTitle, c.name, body)
				}
				// The id alone is the whole of what an enumeration oracle needs,
				// so the row must be ABSENT rather than merely unnamed: a row
				// rendered "show #41" beside a real id marks the show more
				// loudly than its title did.
				if hasWatchingEntry(items, gated.ID) {
					t.Errorf("the watching list published the gated show's id to %s; body: %s", c.name, body)
				}
			}
			if !hasWatchingEntry(items, open.ID) {
				t.Errorf("the watching list dropped the APPROVED show for %s; body: %s", c.name, body)
			}
			// The count moves with the page. A total held whole beside a
			// filtered page publishes the withheld rows as arithmetic, to an
			// account that already knows which shows it subscribed to.
			if total != int64(len(items)) {
				t.Errorf("the watching list reported total=%d beside %d rows for %s — the difference is a count of what was withheld",
					total, len(items), c.name)
			}
		}
	})

	// The notification inbox, fed by the same enrichment. The rows here were
	// minted while the show was public and legitimate at the time, which is what
	// the subscribe gate alone cannot reach.
	t.Run("the notification inbox suppresses the entry and its unread count", func(t *testing.T) {
		strangerBody := assertInbox(t, get, token(stranger), "after the show is gated", 1, 1)
		if strings.Contains(string(strangerBody), watchGatedTitle) ||
			strings.Contains(string(strangerBody), watchGatedComment) {
			t.Errorf("the inbox published the gated show to a stranger; body: %s", strangerBody)
		}
		if !strings.Contains(string(strangerBody), watchOpenTitle) {
			t.Errorf("the inbox dropped the APPROVED show's row too; body: %s", strangerBody)
		}
		// The submitter's own inbox is the control on the other side: their two
		// rows both survive, because the detail route grants them the show.
		assertInbox(t, get, token(submitter), "the submitter's inbox", 2, 2)
	})

	// The counted write. Mark-all reports how many rows it flipped, so a run
	// that touched the hidden rows would publish the withheld count as one
	// integer, whatever the list showed.
	t.Run("mark-all counts only what the inbox showed", func(t *testing.T) {
		code, body := do(t, http.MethodPost, "/me/notifications/mark-read", token(stranger), []byte(`{}`))
		if code != http.StatusOK {
			t.Fatalf("mark-all as a stranger = %d; body: %s", code, body)
		}
		var parsed struct {
			UpdatedCount int64 `json:"updated_count"`
			UnreadCount  int64 `json:"unread_count"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("parse mark-all response: %v; body: %s", err, body)
		}
		if parsed.UpdatedCount != 1 {
			t.Errorf("mark-all flipped %d rows for a stranger who could see 1 — the count names the hidden row", parsed.UpdatedCount)
		}
		if parsed.UnreadCount != 0 {
			t.Errorf("unread after mark-all = %d, want 0", parsed.UnreadCount)
		}
	})

	// Unsubscribe is DELIBERATELY UNGATED, and this pins that. Gating it would
	// leave a subscriber holding a row they cannot see, cannot silence and
	// cannot delete, on a show that keeps mailing them.
	t.Run("unsubscribe still works on a gated show", func(t *testing.T) {
		code, body := do(t, http.MethodDelete, subscribePath(gated.ID), token(quitter), nil)
		if code >= 400 {
			t.Fatalf("unsubscribe from the gated show = %d; body: %s", code, body)
		}
		var count int64
		if err := td.DB.Table("comment_subscriptions").
			Where("user_id = ? AND entity_type = 'show' AND entity_id = ?", quitter.ID, gated.ID).
			Count(&count).Error; err != nil {
			t.Fatalf("count the quitter's subscription: %v", err)
		}
		if count != 0 {
			t.Errorf("the quitter's subscription survived unsubscribe on a gated show")
		}
	})

	// The fan-out is the channel a read-time gate cannot reach: it sends EMAIL
	// carrying the title, an excerpt and a link, and a message already delivered
	// is not withdrawn by a later 404.
	t.Run("the fan-out stops reaching viewers who lost the show", func(t *testing.T) {
		// Only the submitter may comment on their own gated show now, which is
		// PSY-1939's gate; so the submitter is who writes it.
		laterID := seedComment(t, seeder, submitter.ID, gated.ID, watchLaterComment)
		fanOut(t, sc, laterID)

		for _, u := range []*authm.User{stranger, admin} {
			if got := notificationRowsFor(t, td.DB, u.ID, laterID); got != 0 {
				t.Errorf("the fan-out delivered a gated show's comment to user %d (%d rows)", u.ID, got)
			}
		}

		// The control, and it is what separates "the filter works" from "the
		// fan-out is broken": the same author, the same call, on the show that
		// is still public.
		openLaterID := seedComment(t, seeder, submitter.ID, open.ID, watchLaterComment)
		fanOut(t, sc, openLaterID)
		if got := notificationRowsFor(t, td.DB, stranger.ID, openLaterID); got != 1 {
			t.Errorf("the fan-out delivered %d rows for an APPROVED show, want 1", got)
		}
	})

	// Suppression, not deletion. The rows were never removed, the gate is
	// re-evaluated on every read, and publishing the show again restores the
	// entry with the unread count it was withheld with — which is why mark-all
	// above was not allowed to mark it seen.
	t.Run("republishing restores the entry and its backlog", func(t *testing.T) {
		if err := td.DB.Model(&catalogm.Show{}).Where("id = ?", gated.ID).
			Update("status", catalogm.ShowStatusApproved).Error; err != nil {
			t.Fatalf("re-approve show: %v", err)
		}

		items, total, body := watchingList(t, get, token(stranger))
		if !hasWatchingEntry(items, gated.ID) || !strings.Contains(string(body), watchGatedTitle) {
			t.Errorf("the watching list did not restore the republished show; body: %s", body)
		}
		if total != int64(len(items)) {
			t.Errorf("after republication total=%d beside %d rows", total, len(items))
		}

		// Two rows again — the one minted before it was gated and the one minted
		// while it was — and both still unread, because mark-all was not allowed
		// to touch what the inbox had not shown.
		restored := assertInbox(t, get, token(stranger), "after republication", 3, 2)
		if !strings.Contains(string(restored), watchGatedTitle) {
			t.Errorf("the inbox did not restore the republished show's title; body: %s", restored)
		}
	})
}

// newCommentSeeder builds a CommentService with NO notifier attached, for
// creating the fixtures' comments.
//
// The container's CommentService fans out in a GOROUTINE off every create
// (comment_service.go, PSY-289), and writeInAppNotification's ON CONFLICT does
// not dedupe: filter_id is NULL on a comment row and Postgres treats NULL as
// distinct, so a second pass over the same (user, comment) writes a SECOND row.
// A fixture that both created a comment and called the fan-out would therefore
// land a duplicate at an unpredictable moment — an assertion that passes and a
// later count that does not.
//
// So seeding and fanning out are separated: this service writes the comment and
// nothing else, and each test drives NotifySubscribers itself, which is the same
// function the goroutine calls. Everything else about the row — moderation
// status, kind, visibility, rendering — comes from the real service, because a
// hand-built row could sit in pending_review and empty a listing for a reason
// that has nothing to do with the gate under test.
func newCommentSeeder(db *gorm.DB) *engagement.CommentService {
	return engagement.NewCommentService(db, utils.NewMarkdownRenderer())
}

// seedComment writes one comment and returns its id, without fanning it out.
func seedComment(t *testing.T, seeder *engagement.CommentService, authorID, showID uint, body string) uint {
	t.Helper()
	comment, err := seeder.CreateComment(authorID, &contracts.CreateCommentRequest{
		EntityType: "show",
		EntityID:   showID,
		Body:       body,
	})
	if err != nil {
		t.Fatalf("seed comment on show %d: %v", showID, err)
	}
	return comment.ID
}

// fanOut runs the subscriber notification pass synchronously, so the rows it
// writes exist before the assertion that reads them instead of racing it, and a
// pass that silently failed is a test failure rather than a flake.
func fanOut(t *testing.T, sc *services.ServiceContainer, commentID uint) {
	t.Helper()
	if err := sc.CommentNotification.NotifySubscribers(commentID); err != nil {
		t.Fatalf("fan out comment %d: %v", commentID, err)
	}
}

// watchingList reads GET /me/comment-subscriptions and returns the decoded rows,
// the reported total, and the raw body.
//
// The raw body comes back too because the assertions search IT rather than the
// decoded fields: a field-by-field check passes if a future writer echoes the
// withheld title under another key or into an error message.
func watchingList(t *testing.T, get func(*testing.T, string, string) (int, []byte), credential string) ([]contracts.WatchingItem, int64, []byte) {
	t.Helper()
	code, body := get(t, "/me/comment-subscriptions?limit=100", credential)
	if code != http.StatusOK {
		t.Fatalf("GET the watching list = %d; body: %s", code, body)
	}
	var parsed contracts.WatchingListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse the watching list: %v; body: %s", err, body)
	}
	return parsed.Items, parsed.Total, body
}

// hasWatchingEntry reports whether the list names the given show id on a
// show-typed row.
func hasWatchingEntry(items []contracts.WatchingItem, showID uint) bool {
	for _, item := range items {
		if item.EntityType == "show" && item.EntityID == showID {
			return true
		}
	}
	return false
}

// assertInbox reads GET /me/notifications and pins both the row count and the
// unread count, returning the raw body.
//
// BOTH numbers, always. Either one alone is half a gate: a list filtered while
// the badge counts whole publishes the withheld rows as the difference, to a
// reader who already knows which shows they subscribed to.
func assertInbox(t *testing.T, get func(*testing.T, string, string) (int, []byte), credential, when string, wantRows int, wantUnread int64) []byte {
	t.Helper()
	code, body := get(t, "/me/notifications?limit=100", credential)
	if code != http.StatusOK {
		t.Fatalf("GET the inbox %s = %d; body: %s", when, code, body)
	}
	var parsed struct {
		Notifications []contracts.NotificationLogEntry `json:"notifications"`
		UnreadCount   int64                            `json:"unread_count"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse the inbox %s: %v; body: %s", when, err, body)
	}
	if len(parsed.Notifications) != wantRows {
		t.Errorf("the inbox %s carried %d rows, want %d; body: %s", when, len(parsed.Notifications), wantRows, body)
	}
	if parsed.UnreadCount != wantUnread {
		t.Errorf("the inbox %s reported unread=%d, want %d; body: %s", when, parsed.UnreadCount, wantUnread, body)
	}
	return body
}

// notificationRowsFor counts the notification_log rows a fan-out wrote for one
// user about one comment.
//
// Read from the TABLE rather than from the inbox route, because the two answer
// different questions: the route is gated at read time and would report zero for
// a row that exists. This is what proves the fan-out declined to write it at
// all, which is the only thing that stops the email.
func notificationRowsFor(t *testing.T, db *gorm.DB, userID, commentID uint) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND entity_id = ?", userID, commentID).
		Count(&count).Error; err != nil {
		t.Fatalf("count notification rows for user %d: %v", userID, err)
	}
	return count
}
