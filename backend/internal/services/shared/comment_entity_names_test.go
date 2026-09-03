package shared_test

import (
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// The display column CommentEntityPathAndTable names has to be the column the
// table actually spells, and nothing in the enrichment path says so when it is
// not: LoadCommentEntityNames logs the undefined-column error and skips the
// batch, so the surface renders "<type> #<id>" and reads as an entity that was
// deleted.
//
// This probe runs the SELECT the loader builds, per type, and fails on the error
// the loader swallows. LIMIT 0 still parses the projection, so it decides the
// column and not the fixture.
func TestEveryCommentParentTableSpellsItsDisplayColumn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	for entityType := range engagementm.ValidCommentEntityTypes {
		entityType := string(entityType)
		t.Run(entityType, func(t *testing.T) {
			_, table, nameCol, ok := engagementm.CommentEntityPathAndTable(entityType)
			if !ok {
				t.Fatalf("%q is a valid comment entity type with no path-and-table entry", entityType)
			}
			// The same projection LoadCommentEntityNames builds, over no rows: an
			// undefined column is a PARSE failure, so it is reported whether or
			// not the table has data.
			var rows []shared.EntityNameRow
			err := td.DB.Table(table).
				Select(fmt.Sprintf("id, %s AS name, slug", nameCol)).
				Limit(0).
				Scan(&rows).Error
			if err != nil {
				t.Errorf("table %s has no column %s: %v", table, nameCol, err)
			}
		})
	}
}

// The identity fence, asserted directly.
//
// The two halves are one test because either alone passes for the wrong reason:
// a fence that withheld everything satisfies the stranger's half, and a loader
// with no fence at all satisfies the creator's.
func TestLoadCommentEntityNamesFencesPrivateCollections(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	creator := testhelpers.CreateTestUser(td.DB)
	stranger := testhelpers.CreateTestUser(td.DB)
	admin := testhelpers.CreateAdminUser(td.DB)

	const (
		gatedTitle = "Tapes I Have Not Shared"
		gatedSlug  = "tapes-i-have-not-shared"
		openTitle  = "Records I Keep Lending Out"
		openSlug   = "records-i-keep-lending-out"
	)
	gated := testhelpers.CreateCollection(t, td.DB, creator.ID, gatedTitle, gatedSlug, false)
	open := testhelpers.CreateCollection(t, td.DB, creator.ID, openTitle, openSlug, true)

	ids := map[string][]uint{"collection": {gated.ID, open.ID}}

	for _, c := range []struct {
		name    string
		viewer  contracts.ShowViewer
		mayRead bool
	}{
		{"anonymous", contracts.ShowViewer{}, false},
		{"an authenticated stranger", contracts.ShowViewer{UserID: stranger.ID}, false},
		{"the creator", contracts.ShowViewer{UserID: creator.ID}, true},
		// On the stranger's side: no collection read path in this codebase grants
		// an admin a private collection.
		{"an admin", contracts.ShowViewer{UserID: admin.ID, IsAdmin: true}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			byID := shared.LoadCommentEntityNames(td.DB, ids, c.viewer)["collection"]

			// The PUBLIC collection resolves for everybody, which is what
			// separates "the fence works" from "the column is still wrong".
			if got := byID[open.ID]; got.Name != openTitle || got.Slug != openSlug {
				t.Errorf("the public collection resolved to %+v, want title %q and slug %q",
					got, openTitle, openSlug)
			}

			got, present := byID[gated.ID]
			if c.mayRead {
				if !present || got.Name != gatedTitle || got.Slug != gatedSlug {
					t.Errorf("the private collection was withheld from its creator: %+v (present=%v)", got, present)
				}
				return
			}
			if present {
				t.Errorf("the fence resolved a private collection's identity for %s: %+v", c.name, got)
			}
		})
	}
}
