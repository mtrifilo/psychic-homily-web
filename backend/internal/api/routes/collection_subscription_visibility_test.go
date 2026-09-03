package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// A comment SUBSCRIPTION to a COLLECTION carries the collection detail route's
// visibility rule, at the moment it is made and on every read of it afterwards.
//
// What is at stake if it does not: one POST with a guessed id turns
// GET /me/comment-subscriptions into a monitored feed of a private collection,
// carrying its id, its comment count, its last-activity timestamp and the
// display name of whoever commented last, and the fan-out mails every new
// comment on it to whoever subscribed.
//
// This is the sibling of comment_subscription_visibility_test.go, same shape,
// same four tiers, on the other gated entity type. Read them together. The one
// difference that matters is the ADMIN COLUMN: an admin sees every show and NO
// private collection, because that is what the two detail routes do. Every
// assertion below puts the admin on the stranger's side of the line, and that is
// the assertion, not an oversight.

const (
	// Independent of the sibling file's fixtures, so no assertion here is
	// satisfied by a value another test put in the body.
	watchGatedCollectionTitle = "Tapes From The Ice House Basement"
	watchOpenCollectionTitle  = "Records I Keep Lending Out"
	watchGatedCollectionSlug  = "tapes-from-the-ice-house-basement"
	watchOpenCollectionSlug   = "records-i-keep-lending-out"
	gatedCollectionComment    = "side b is the one with the false ending"
	gatedCollectionTag        = "ice-house-basement-tape"
	openCollectionComment     = "the reissue has a different mastering"
	laterCollectionComment    = "posting this while the collection is private, nobody should see it"
)

func TestCollectionSubscriptionsMirrorTheDetailRoute(t *testing.T) {
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
	creator := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	commenter := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	stranger := testhelpers.CreateTestUser(td.DB)
	// A second stranger, for the unsubscribe subtest alone. Removing the first
	// one's subscription would silently disarm every assertion after it.
	quitter := testhelpers.CreateTestUser(td.DB)

	// BOTH PUBLIC to begin with, because the case that matters is a collection
	// that was public when it was subscribed to. A gate on the subscribe route
	// alone leaves every row made before the flip still leaking.
	gated := testhelpers.CreateCollection(t, td.DB, creator.ID, watchGatedCollectionTitle, watchGatedCollectionSlug, true).ID
	open := testhelpers.CreateCollection(t, td.DB, creator.ID, watchOpenCollectionTitle, watchOpenCollectionSlug, true).ID

	// A tag ON the gated collection, so the tag arm below has something to
	// withhold. Without it the assertion is satisfied by an empty list whatever
	// the gate does.
	seedEntityTag(t, td.DB, "collection", gated, gatedCollectionTag, creator.ID)

	seeder := newCommentSeeder(td.DB)
	gatedCommentID := seedEntityComment(t, seeder, commenter.ID, "collection", gated, gatedCollectionComment)
	openCommentID := seedEntityComment(t, seeder, commenter.ID, "collection", open, openCollectionComment)

	token := func(u *authm.User) string { return mintToken(t, sc, u) }

	// Bound to this test's router; the carrier and the raw-body contract live
	// in routes_test.go, shared with the sibling matrices.
	do := func(t *testing.T, method, path, credential string, body []byte) (int, []byte) {
		return doRequest(t, router, method, path, credential, body)
	}
	get := func(t *testing.T, path, credential string) (int, []byte) {
		return getRequest(t, router, path, credential)
	}
	subscribePath := func(collectionID uint) string {
		return fmt.Sprintf("/entities/collection/%d/subscribe", collectionID)
	}

	// Everybody subscribes to BOTH collections WHILE THEY ARE PUBLIC, over the
	// real route.
	for _, u := range []*authm.User{stranger, quitter, creator, admin} {
		for _, id := range []uint{gated, open} {
			if code, body := do(t, http.MethodPost, subscribePath(id), token(u), nil); code != http.StatusOK {
				t.Fatalf("subscribe user %d to public collection %d = %d; body: %s", u.ID, id, code, body)
			}
		}
	}

	// Fan both comments out synchronously. In production this runs in a
	// goroutine off comment creation; called directly, the inbox rows exist
	// before the first assertion rather than racing it.
	fanOut(t, sc, gatedCommentID)
	fanOut(t, sc, openCommentID)

	// The stranger's inbox now holds exactly two rows, one per collection, and
	// that pair is what every suppression assertion below is measured against.
	assertInbox(t, get, token(stranger), "before the collection is private", 2, 2)

	// Now take the subject private, and read the flag back rather than trust the
	// update: GORM omits a false boolean on Create, and a fixture left public
	// would make every assertion below pass for the wrong reason.
	testhelpers.SetCollectionPublic(t, td.DB, gated, false)

	// mayRead is the CREATOR only. Not the admin: no collection read path in this
	// codebase grants one, so a gate that did would be more permissive than the
	// route it mirrors.
	callers := []subResourceCaller{
		{"anonymous", "", false},
		{"an authenticated stranger", token(stranger), false},
		{"the creator", token(creator), true},
		{"an admin", token(admin), false},
	}

	// THE SUBTESTS BELOW SHARE MUTABLE STATE AND MUST RUN IN ORDER, for the
	// reason the sibling file gives at length: mark-all writes read_at and the
	// fan-out subtest seeds more comments, both of which are preconditions of the
	// republication counts. Run the whole test, not one subtest with `-run`.

	// The route every gate below mirrors.
	//
	// NOT FOUND, not forbidden, and the same answer for a slug nobody has used.
	// The route is reachable by a dense integer id as well as by the slug
	// (GetCollectionHandler parses the segment as a uint first), so a 403 that
	// echoed the resolved slug would publish the title of every private
	// collection to a caller counting upward.
	t.Run("the detail route it mirrors", func(t *testing.T) {
		for _, c := range callers {
			want := http.StatusNotFound
			if c.mayRead {
				want = http.StatusOK
			}
			code, body := get(t, "/collections/"+watchGatedCollectionSlug, c.token)
			if code != want {
				t.Errorf("GET the private collection as %s = %d, want %d; body: %s", c.name, code, want, body)
			}
			if c.mayRead {
				continue
			}
			// The refusal must not carry the title, and it must be the answer a
			// slug nobody has used gets.
			if strings.Contains(string(body), watchGatedCollectionTitle) {
				t.Errorf("the detail route's refusal published the private collection's title to %s: %s",
					c.name, body)
			}
			missingCode, _ := get(t, "/collections/a-slug-nobody-has-ever-used", c.token)
			if missingCode != code {
				t.Errorf("the detail route answers %d for a private collection but %d for a slug "+
					"nobody has used, as %s", code, missingCode, c.name)
			}
		}
	})

	// THE NUMERIC-ID SPELLING OF THE SAME ROUTE, which is the one an enumerator
	// walks. It must answer the id back, never the slug it resolved.
	t.Run("the detail route by numeric id", func(t *testing.T) {
		for _, c := range callers {
			want := http.StatusNotFound
			if c.mayRead {
				want = http.StatusOK
			}
			code, body := get(t, fmt.Sprintf("/collections/%d", gated), c.token)
			if code != want {
				t.Errorf("GET the private collection by id as %s = %d, want %d; body: %s",
					c.name, code, want, body)
			}
			if c.mayRead {
				continue
			}
			if strings.Contains(string(body), watchGatedCollectionSlug) ||
				strings.Contains(string(body), watchGatedCollectionTitle) {
				t.Errorf("the id route resolved and published the private collection's identity to %s: %s",
					c.name, body)
			}
		}
	})

	// The COMMENT THREAD and the TAG list, which are the same polymorphic routes
	// the subscription family sits on and were default-open for the same reason.
	//
	// Worth being explicit about what this arm is: these two are a WIDER leak
	// than the one the ticket was filed for. A subscription published a private
	// collection's id and its activity to somebody who had subscribed; the thread
	// route published its comments, in full, to anybody who guessed the id, with
	// no subscription and no prior step. They are closed by the same registry
	// entry, so they are proved by the same fixture rather than left to be
	// noticed later.
	//
	// EMPTY, not a refusal: a collection with no comments and no tags already
	// answers that way and the two must be indistinguishable.
	t.Run("the comment thread and tag list read empty", func(t *testing.T) {
		for _, c := range callers {
			code, body := get(t, fmt.Sprintf("/entities/collection/%d/comments", gated), c.token)
			if code != http.StatusOK {
				t.Fatalf("GET the private collection's comments as %s = %d; body: %s", c.name, code, body)
			}
			if c.mayRead {
				if !strings.Contains(string(body), gatedCollectionComment) {
					t.Errorf("the thread withheld the private collection's comments from %s, who may read it; body: %s", c.name, body)
				}
			} else if strings.Contains(string(body), gatedCollectionComment) {
				t.Errorf("the thread published a private collection's comment body to %s; body: %s", c.name, body)
			}

			tagCode, tagBody := get(t, fmt.Sprintf("/entities/collection/%d/tags", gated), c.token)
			if tagCode != http.StatusOK {
				t.Errorf("GET the private collection's tags as %s = %d; body: %s", c.name, tagCode, tagBody)
			}
			if c.mayRead {
				if !strings.Contains(string(tagBody), gatedCollectionTag) {
					t.Errorf("the tag list withheld the private collection's tags from %s, who may read it; body: %s",
						c.name, tagBody)
				}
			} else if strings.Contains(string(tagBody), gatedCollectionTag) {
				t.Errorf("the tag list published a private collection's tag to %s; body: %s", c.name, tagBody)
			}
		}

		// The control: the public collection still serves its thread to a
		// stranger, so the arm above is measuring the gate and not a broken route.
		code, body := get(t, fmt.Sprintf("/entities/collection/%d/comments", open), token(stranger))
		if code != http.StatusOK || !strings.Contains(string(body), openCollectionComment) {
			t.Errorf("the thread on the PUBLIC collection answered a stranger %d; body: %s", code, body)
		}

		// POSTING to the thread is gated by the same call, and it is a different
		// harm from reading: a stranger could otherwise write into a private
		// collection's discussion, and the creator would find it there.
		postPath := fmt.Sprintf("/entities/collection/%d/comments", gated)
		postBody := []byte(`{"body":"a stranger should not be able to write this"}`)
		for _, c := range callers {
			if c.token == "" || c.mayRead {
				continue // anonymous has no write route; the creator is the control below
			}
			if code, body := do(t, http.MethodPost, postPath, c.token, postBody); code < 400 {
				t.Errorf("POST a comment on the private collection as %s = %d, want a refusal; body: %s",
					c.name, code, body)
			}
		}
		// The creator's own post succeeds — the control that separates "the gate
		// works" from "the write route is broken for everybody".
		//
		// On a collection of its OWN, not on `gated`. CreateComment rate-limits
		// one author to one comment per entity per minute, so posting here would
		// silently take out the fan-out subtest's seed further down, and the
		// failure would surface as a wrong inbox count three subtests away from
		// its cause.
		writable := testhelpers.CreateCollection(t, td.DB, creator.ID,
			"Notes I Have Not Shared", "notes-i-have-not-shared", false).ID
		writablePath := fmt.Sprintf("/entities/collection/%d/comments", writable)
		if code, body := do(t, http.MethodPost, writablePath, token(creator), postBody); code != http.StatusOK {
			t.Errorf("the creator could not comment on their own private collection: %d; body: %s", code, body)
		}
		// And a stranger cannot, on that same fresh collection — which pins that
		// the refusal above was about the gate rather than about the fixture.
		if code, body := do(t, http.MethodPost, writablePath, token(stranger), postBody); code < 400 {
			t.Errorf("a stranger wrote into a private collection's thread: %d; body: %s", code, body)
		}
	})

	// The WRITE gate. A subscription is a standing request for a collection's
	// activity, so it is refused exactly as an id nobody has ever used is
	// refused — the pair being distinguishable is the whole oracle, and unlike
	// the detail route this family is addressed by a DENSE INTEGER id rather than
	// by a slug, which is what makes walking it cheap.
	const absentCollectionID = uint(99999999)
	writeRoutes := []struct {
		name string
		path func(id uint) string
	}{
		{"subscribe", subscribePath},
		{"mark-read", func(id uint) string { return fmt.Sprintf("/entities/collection/%d/mark-read", id) }},
	}
	for _, w := range writeRoutes {
		w := w
		t.Run("the "+w.name+" route refuses like a missing collection", func(t *testing.T) {
			for _, c := range callers {
				gatedCode, gatedBody := do(t, http.MethodPost, w.path(gated), c.token, nil)

				if c.mayRead {
					if gatedCode != http.StatusOK {
						t.Errorf("POST %s on the private collection as %s = %d, want 200; body: %s",
							w.name, c.name, gatedCode, gatedBody)
					}
					continue
				}
				if gatedCode < 400 {
					t.Errorf("POST %s on the private collection as %s = %d, want a refusal; body: %s",
						w.name, c.name, gatedCode, gatedBody)
				}
				absentCode, absentBody := do(t, http.MethodPost, w.path(absentCollectionID), c.token, nil)

				// The caller's own id is echoed into one of these messages, and
				// echoing a caller's own input discloses nothing. Normalised out
				// so the comparison is about everything else: any OTHER
				// difference sorts real private collections from ids never used.
				gatedNorm := elideEntityID(string(gatedBody), gated)
				absentNorm := elideEntityID(string(absentBody), absentCollectionID)
				if gatedCode != absentCode || !sameErrorMessage([]byte(gatedNorm), []byte(absentNorm)) {
					t.Errorf("POST %s answers %d/%s for a private collection but %d/%s for a nonexistent one as %s — the difference is the leak",
						w.name, gatedCode, gatedNorm, absentCode, absentNorm, c.name)
				}
			}

			// The control. A gate that simply broke the route could not pass this.
			if code, body := do(t, http.MethodPost, w.path(open), token(stranger), nil); code != http.StatusOK {
				t.Errorf("POST %s on the PUBLIC collection as a stranger = %d, want 200; body: %s", w.name, code, body)
			}
		})
	}

	// The status route is the one that must NOT refuse: it reports a live unread
	// count, so the truthful answer is a running measure of how busy a collection
	// the caller cannot see is, and a 404 would confirm the id.
	t.Run("the status route answers not-subscribed", func(t *testing.T) {
		statusPath := func(id uint) string { return fmt.Sprintf("/entities/collection/%d/subscribe/status", id) }
		for _, c := range callers {
			if c.token == "" {
				continue // protected route; the watching-list subtest covers 401
			}
			gatedCode, gatedBody := get(t, statusPath(gated), c.token)
			if c.mayRead {
				if gatedCode != http.StatusOK || !strings.Contains(string(gatedBody), `"subscribed":true`) {
					t.Errorf("the status route hid the private collection's subscription from %s: %d; body: %s",
						c.name, gatedCode, gatedBody)
				}
				continue
			}
			// The CONTENT PIN is the load-bearing assertion. The absent-id
			// comparison below holds largely by construction — CollectionVisibleTo
			// fails closed on a missing collection, so both leave the handler
			// from the same line — which makes it a regression guard against
			// somebody giving the gated path its own answer, not proof on its
			// own.
			if gatedCode == http.StatusOK {
				var parsed struct {
					Subscribed  bool `json:"subscribed"`
					UnreadCount int  `json:"unread_count"`
				}
				if err := json.Unmarshal(gatedBody, &parsed); err != nil {
					t.Fatalf("parse the status response for %s: %v; body: %s", c.name, err, gatedBody)
				}
				if parsed.Subscribed || parsed.UnreadCount != 0 {
					t.Errorf("the status route told %s they are subscribed to the private collection (unread=%d); body: %s",
						c.name, parsed.UnreadCount, gatedBody)
				}
			}

			absentCode, absentBody := get(t, statusPath(absentCollectionID), c.token)
			same := sameJSON(gatedBody, absentBody)
			if gatedCode >= 400 {
				same = sameErrorMessage(gatedBody, absentBody)
			}
			if gatedCode != absentCode || !same {
				t.Errorf("the status route answers %s differently for a private collection than for a nonexistent one: %s vs %s",
					c.name, gatedBody, absentBody)
			}
		}
	})

	// The watching list. Not addressed by a collection id at all, which is why
	// the gate on the collection's own routes never reached it.
	t.Run("the watching list suppresses the entry and its count", func(t *testing.T) {
		for _, c := range callers {
			if c.token == "" {
				if code, _ := get(t, "/me/comment-subscriptions?limit=100", ""); code != http.StatusUnauthorized {
					t.Errorf("the watching list answered anonymous %d, want 401", code)
				}
				continue
			}
			items, total, body := watchingList(t, get, c.token)

			if c.mayRead {
				if !hasWatchingEntryOfType(items, "collection", gated) {
					t.Errorf("the watching list withheld the private collection from %s, who may read it; body: %s", c.name, body)
				}
			} else {
				// The ID ALONE is the whole of what an enumeration oracle needs,
				// so the row must be ABSENT rather than merely unnamed.
				//
				// The id is also, today, ALL this row would have disclosed: the
				// watching list renders a collection as "collection #<id>"
				// because CommentEntityPathAndTable names a `name` column the
				// collections table does not have, so the title lookup errors and
				// is skipped. That bug is not fixed here — a privacy change must
				// not widen a disclosure path on its way past — which is exactly
				// why this asserts on the ID and the ROW rather than on the title.
				// See services/shared/comment_entity_names.go.
				if hasWatchingEntryOfType(items, "collection", gated) {
					t.Errorf("the watching list published the private collection's id to %s; body: %s", c.name, body)
				}
				if strings.Contains(string(body), watchGatedCollectionTitle) ||
					strings.Contains(string(body), watchGatedCollectionSlug) {
					t.Errorf("the watching list published the private collection's title or slug to %s; body: %s", c.name, body)
				}
			}
			if !hasWatchingEntryOfType(items, "collection", open) {
				t.Errorf("the watching list dropped the PUBLIC collection for %s; body: %s", c.name, body)
			}
			// The count moves with the page, or the withheld row is published as
			// arithmetic to an account that already knows what it subscribed to.
			if total != int64(len(items)) {
				t.Errorf("the watching list reported total=%d beside %d rows for %s — the difference is a count of what was withheld",
					total, len(items), c.name)
			}
		}
	})

	// The notification inbox, fed by the same enrichment. These rows were minted
	// while the collection was public and legitimate at the time, which is what
	// the subscribe gate alone cannot reach.
	t.Run("the notification inbox suppresses the entry and its unread count", func(t *testing.T) {
		for _, c := range callers {
			if c.token == "" {
				if code, _ := get(t, "/me/notifications?limit=100", ""); code != http.StatusUnauthorized {
					t.Errorf("the inbox answered anonymous %d, want 401", code)
				}
				continue
			}
			wantRows, wantUnread := 1, int64(1)
			if c.mayRead {
				wantRows, wantUnread = 2, 2
			}
			body := assertInbox(t, get, c.token, "the inbox for "+c.name, wantRows, wantUnread)

			if c.mayRead {
				if !strings.Contains(string(body), gatedCollectionComment) {
					t.Errorf("the inbox withheld the private collection from %s, who may read it; body: %s", c.name, body)
				}
			} else if strings.Contains(string(body), gatedCollectionComment) ||
				strings.Contains(string(body), watchGatedCollectionTitle) ||
				strings.Contains(string(body), watchGatedCollectionSlug) {
				t.Errorf("the inbox published the private collection to %s; body: %s", c.name, body)
			}
			if !strings.Contains(string(body), openCollectionComment) {
				t.Errorf("the inbox dropped the PUBLIC collection's row for %s; body: %s", c.name, body)
			}
		}
	})

	// The counted write. Mark-all reports how many rows it flipped, so a run that
	// touched the hidden rows would publish the withheld count as one integer.
	//
	// The creator and the stranger are run as a PAIR, because one number alone
	// proves nothing: 1 is only meaningful beside the 2 the same call returns to
	// somebody who can see both rows. The ADMIN is here too, on the stranger's
	// side, because they are the tier a "make it consistent with shows" edit
	// would move.
	t.Run("mark-all counts only what the inbox showed", func(t *testing.T) {
		markAll := func(t *testing.T, credential, who string) (int64, int64) {
			t.Helper()
			code, body := do(t, http.MethodPost, "/me/notifications/mark-read", credential, []byte(`{}`))
			if code != http.StatusOK {
				t.Fatalf("mark-all as %s = %d; body: %s", who, code, body)
			}
			var parsed struct {
				UpdatedCount int64 `json:"updated_count"`
				UnreadCount  int64 `json:"unread_count"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatalf("parse mark-all response for %s: %v; body: %s", who, err, body)
			}
			return parsed.UpdatedCount, parsed.UnreadCount
		}

		// The creator first: they see both rows, so they flip both. Doing this
		// before the others also proves the accounts' rows are independent.
		if updated, unread := markAll(t, token(creator), "the creator"); updated != 2 || unread != 0 {
			t.Errorf("mark-all for the creator = %d flipped / %d unread, want 2/0", updated, unread)
		}
		for _, c := range []struct {
			credential string
			who        string
		}{
			{token(stranger), "a stranger"},
			{token(admin), "an admin"},
		} {
			updated, unread := markAll(t, c.credential, c.who)
			if updated != 1 {
				t.Errorf("mark-all flipped %d rows for %s who could see 1 — the count names the hidden row", updated, c.who)
			}
			if unread != 0 {
				t.Errorf("unread after mark-all for %s = %d, want 0", c.who, unread)
			}
		}
	})

	// Unsubscribe is DELIBERATELY UNGATED, and this pins that. Gating it would
	// leave a subscriber holding a row they cannot see, cannot silence and cannot
	// delete, on a collection that keeps mailing them.
	t.Run("unsubscribe still works on a private collection", func(t *testing.T) {
		code, body := do(t, http.MethodDelete, subscribePath(gated), token(quitter), nil)
		if code >= 400 {
			t.Fatalf("unsubscribe from the private collection = %d; body: %s", code, body)
		}
		var count int64
		if err := td.DB.Table("comment_subscriptions").
			Where("user_id = ? AND entity_type = 'collection' AND entity_id = ?", quitter.ID, gated).
			Count(&count).Error; err != nil {
			t.Fatalf("count the quitter's subscription: %v", err)
		}
		if count != 0 {
			t.Errorf("the quitter's subscription survived unsubscribe on a private collection")
		}
	})

	// The fan-out is the channel a read-time gate cannot reach: it sends EMAIL
	// carrying the parent's name, an excerpt and a link, and a message already
	// delivered is not withdrawn by a later 403.
	//
	// It is also the one gate here that is FINAL — a row not written is not
	// restored by republishing — which is why the ADMIN is asserted to receive
	// NOTHING rather than merely left un-checked. On shows the admin case is
	// asserted positively for exactly the symmetrical reason; the two files
	// disagree here because the two detail routes do.
	t.Run("the fan-out stops reaching viewers who lost the collection", func(t *testing.T) {
		laterID := seedEntityComment(t, seeder, creator.ID, "collection", gated, laterCollectionComment)
		fanOut(t, sc, laterID)

		for _, r := range []struct {
			userID uint
			who    string
		}{
			{stranger.ID, "a stranger"},
			{quitter.ID, "the unsubscribed quitter"},
			{admin.ID, "an ADMIN, who the detail route also refuses"},
		} {
			if got := notificationRowsFor(t, td.DB, r.userID, laterID); got != 0 {
				t.Errorf("the fan-out delivered a private collection's comment to %s (%d rows)", r.who, got)
			}
		}

		// The control, and it is what separates "the filter works" from "the
		// fan-out is broken": the same author, the same call, on the collection
		// that is still public.
		openLaterID := seedEntityComment(t, seeder, creator.ID, "collection", open, laterCollectionComment)
		fanOut(t, sc, openLaterID)
		if got := notificationRowsFor(t, td.DB, stranger.ID, openLaterID); got != 1 {
			t.Errorf("the fan-out delivered %d rows for a PUBLIC collection, want 1", got)
		}
	})

	// Suppression, not deletion — for the rows that EXIST. The gate is
	// re-evaluated on every read, so making the collection public again restores
	// them still unread, which is why mark-all above was not allowed to mark them
	// seen.
	//
	// It does NOT restore what the fan-out declined to write, and the counts
	// below are asserted with that in mind.
	t.Run("republishing restores the entry and its backlog", func(t *testing.T) {
		testhelpers.SetCollectionPublic(t, td.DB, gated, true)

		items, total, body := watchingList(t, get, token(stranger))
		if !hasWatchingEntryOfType(items, "collection", gated) {
			t.Errorf("the watching list did not restore the republished collection; body: %s", body)
		}
		if total != int64(len(items)) {
			t.Errorf("after republication total=%d beside %d rows", total, len(items))
		}

		// THREE rows, and naming them is the point, because the obvious reading
		// of this number is wrong:
		//   1. the private collection's comment from BEFORE it went private —
		//      restored, still unread, because mark-all could not touch what it
		//      could not see;
		//   2. the public collection's first comment — read, by that mark-all;
		//   3. the public collection's later comment — unread.
		// So two unread. The DURING-window comment is absent and always will be:
		// the fan-out never wrote it. If a future change makes this 4, that is
		// not "the backlog arrived", it is the write gate having been removed.
		restored := assertInbox(t, get, token(stranger), "after republication", 3, 2)
		if !strings.Contains(string(restored), gatedCollectionComment) {
			t.Errorf("the inbox did not restore the republished collection's comment; body: %s", restored)
		}
	})
}

// elideEntityID replaces WHOLE-NUMBER occurrences of id in body with a
// placeholder, so two error messages can be compared for everything except the
// caller's own echoed input.
//
// Word-anchored rather than a plain substring replacement: collection ids are
// small integers, and replacing "3" everywhere also rewrites the 3 inside a
// timestamp, a status code or a longer id, which would make two genuinely
// different messages compare equal and turn the assertion into a no-op.
func elideEntityID(body string, id uint) string {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(strconv.FormatUint(uint64(id), 10)) + `\b`)
	return pattern.ReplaceAllString(body, "<id>")
}

// THE SLUG-ADDRESSED OWNER WRITES, over the real router.
//
// The route inventory records a disposition for each of these; a map that says
// a route is safe is a claim, not a test. This walks all five with a stranger
// and requires the answer to be indistinguishable from the answer a slug nobody
// has ever used gets — status and message both, with the caller's own slug
// normalised out because echoing a caller's own input discloses nothing.
//
// The item writes carry an item id that IS in the private collection, which is
// what makes them about the gate's POSITION: a gate placed after the item lookup
// answers item-not-found for an id in another collection and forbidden for one
// in this collection, and that pair reports membership over a global sequence.
func TestCollectionOwnerWritesRefuseLikeAMissingCollection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	creator := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	stranger := testhelpers.CreateTestUser(td.DB)
	admin := testhelpers.CreateAdminUser(td.DB)

	// COLLABORATIVE and public to begin with, so the stranger can legitimately
	// add an item and then be locked out by the flip alone. A contributor who
	// added an item while it was public is the caller the AddedByUserID branch
	// used to admit.
	gated := testhelpers.CreateCollection(t, td.DB, creator.ID,
		"Things I Stopped Sharing", "things-i-stopped-sharing", true)
	if err := td.DB.Model(gated).Update("collaborative", true).Error; err != nil {
		t.Fatalf("make collection collaborative: %v", err)
	}
	artist := testhelpers.CreateArtist(td.DB, "Owner Write Route Artist")

	do := func(t *testing.T, method, path, credential string, body []byte) (int, []byte) {
		return doRequest(t, router, method, path, credential, body)
	}
	strangerToken := mintToken(t, sc, stranger)
	adminToken := mintToken(t, sc, admin)

	addBody := []byte(fmt.Sprintf(`{"entity_type":"artist","entity_id":%d}`, artist.ID))
	code, body := do(t, http.MethodPost, "/collections/"+gated.Slug+"/items", strangerToken, addBody)
	if code != http.StatusOK {
		t.Fatalf("the control: a stranger adding to the PUBLIC collaborative collection = %d; body: %s", code, body)
	}
	var added struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(body, &added); err != nil || added.ID == 0 {
		t.Fatalf("read back the item id from %s: %v", body, err)
	}

	testhelpers.SetCollectionPublic(t, td.DB, gated.ID, false)

	const missingSlug = "a-slug-nobody-has-ever-used"
	reorderBody := []byte(fmt.Sprintf(`{"items":[{"item_id":%d,"position":1}]}`, added.ID))

	writes := []struct {
		name   string
		method string
		path   func(slug string) string
		body   []byte
		// adminIsModerator marks the two routes an admin reaches on a private
		// collection: flipping it public and removing it are the remedies the
		// entity-report queue exists to reach. Their admin arms run last, on
		// fixtures of their own, because both of them CHANGE the collection —
		// a rename regenerates the slug and a delete removes the row, either of
		// which would make every later subtest pass for the wrong reason.
		adminIsModerator bool
	}{
		{
			name:             "update the collection",
			method:           http.MethodPut,
			path:             func(slug string) string { return "/collections/" + slug },
			body:             []byte(`{"title":"renamed by a stranger"}`),
			adminIsModerator: true,
		},
		{
			name:             "delete the collection",
			method:           http.MethodDelete,
			path:             func(slug string) string { return "/collections/" + slug },
			adminIsModerator: true,
		},
		{
			name:   "update an item",
			method: http.MethodPatch,
			path: func(slug string) string {
				return fmt.Sprintf("/collections/%s/items/%d", slug, added.ID)
			},
			body: []byte(`{"notes":"a stranger should not be able to write this"}`),
		},
		{
			name:   "remove an item",
			method: http.MethodDelete,
			path: func(slug string) string {
				return fmt.Sprintf("/collections/%s/items/%d", slug, added.ID)
			},
		},
		{
			name:   "reorder the items",
			method: http.MethodPut,
			path:   func(slug string) string { return "/collections/" + slug + "/items/reorder" },
			body:   reorderBody,
		},
	}

	// EVERY REFUSAL IS A NON-MUTATION, so all five can share the one fixture:
	// the stranger arm never succeeds, and neither does the admin arm on the
	// three routes that are not moderation remedies.
	for _, w := range writes {
		w := w
		t.Run(w.name, func(t *testing.T) {
			gatedCode, gatedBody := do(t, w.method, w.path(gated.Slug), strangerToken, w.body)
			if gatedCode < 400 {
				t.Fatalf("a stranger could %s on the private collection: %d; body: %s",
					w.name, gatedCode, gatedBody)
			}
			missingCode, missingBody := do(t, w.method, w.path(missingSlug), strangerToken, w.body)

			gatedNorm := strings.ReplaceAll(string(gatedBody), gated.Slug, "<slug>")
			missingNorm := strings.ReplaceAll(string(missingBody), missingSlug, "<slug>")
			if gatedCode != missingCode || !sameErrorMessage([]byte(gatedNorm), []byte(missingNorm)) {
				t.Errorf("%s answers %d/%s for a private collection but %d/%s for a slug nobody "+
					"has used — the difference is the leak",
					w.name, gatedCode, gatedNorm, missingCode, missingNorm)
			}
			if strings.Contains(string(gatedBody), gated.Title) {
				t.Errorf("%s published the private collection's title: %s", w.name, gatedBody)
			}

			if w.adminIsModerator {
				return // asserted below, on its own fixture
			}
			// No collection read grants an admin a private collection, so the
			// three non-moderation writes must answer them exactly as they answer
			// the stranger. Both requests name the same slug, so the bodies
			// compare directly.
			adminCode, adminBody := do(t, w.method, w.path(gated.Slug), adminToken, w.body)
			if adminCode != gatedCode || !sameErrorMessage(adminBody, gatedBody) {
				t.Errorf("an admin gets %d for %s on a private collection while a stranger gets %d; "+
					"no collection read grants an admin one, so the two must answer alike. body: %s",
					adminCode, w.name, gatedCode, adminBody)
			}
		})
	}

	// THE TWO MODERATION REMEDIES, each on a fixture of its own because each
	// consumes it. An admin holds these on a private collection deliberately:
	// the entity-report queue shows them reports naming private collections, and
	// flipping one public or removing it are the only remedies there are.
	t.Run("an admin may flip a private collection public", func(t *testing.T) {
		target := testhelpers.CreateCollection(t, td.DB, creator.ID,
			"Reported And Private", "reported-and-private", false)
		code, body := do(t, http.MethodPut, "/collections/"+target.Slug, adminToken,
			[]byte(`{"is_public":true}`))
		if code != http.StatusOK {
			t.Errorf("an admin was refused the flip on a private collection: %d; body: %s", code, body)
		}
	})

	t.Run("an admin may delete a private collection", func(t *testing.T) {
		target := testhelpers.CreateCollection(t, td.DB, creator.ID,
			"Reported And Removable", "reported-and-removable", false)
		code, body := do(t, http.MethodDelete, "/collections/"+target.Slug, adminToken, nil)
		if code >= 400 {
			t.Errorf("an admin was refused the delete on a private collection: %d; body: %s", code, body)
		}
	})
}
