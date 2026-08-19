package admin

import (
	"errors"
	"fmt"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// SHOW VISIBILITY (PSY-1715)
// =============================================================================
//
// Shows are gated at the ENTITY level, so these cases are shaped differently
// from the venue redaction cases in revision_test.go: nothing is masked, the
// revision is simply not there. Read revision_visibility.go for the policy.
//
// What this file can and cannot pin. It sits below the auth boundary, so it is
// handed a viewer struct rather than a credential: which real caller resolves to
// which viewer is pinned in handlers/admin/revision_test.go, and end to end over
// real JWTs in routes/revision_show_visibility_test.go. What only this file can
// reach cheaply is the fail-closed edges — a deleted show, a merge-stamped row,
// each gated status in turn — because they need a database in a state no HTTP
// request can produce.

const gatedShowSummary = "the summary nobody outside the gate may read"

// seedShow inserts a show in the given status, optionally owned by a submitter.
// Written through the model so a column rename breaks the build rather than the
// test run.
func (s *RevisionServiceIntegrationTestSuite) seedShow(title string, status catalogm.ShowStatus, submittedBy *uint) *catalogm.Show {
	slug := fmt.Sprintf("test-show-%d", time.Now().UnixNano())
	show := &catalogm.Show{
		Title:       title,
		Slug:        &slug,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		Status:      status,
		SubmittedBy: submittedBy,
	}
	s.Require().NoError(s.db.Create(show).Error)

	// Read the status back: GORM writes a zero-value enum as the column default,
	// and a fixture that silently landed on 'approved' would make every gated
	// case below pass for the wrong reason.
	var stored struct{ Status catalogm.ShowStatus }
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Select("status").
		Where("id = ?", show.ID).Scan(&stored).Error)
	s.Require().Equal(status, stored.Status, "show fixture landed in the wrong status")

	return show
}

// recordShowRevision writes one revision against a show and returns it as
// stored, so a test can address it by id on the single-revision route.
func (s *RevisionServiceIntegrationTestSuite) recordShowRevision(showID, authorID uint) *adminm.Revision {
	changes := []adminm.FieldChange{
		{Field: "title", OldValue: "Before", NewValue: "After"},
	}
	s.Require().NoError(s.svc.RecordRevision("show", showID, authorID, changes, gatedShowSummary))

	var stored adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "show", showID).
		Order("id DESC").First(&stored).Error)
	return &stored
}

// viewerFor names the tier a specific user reads as. Spelled out rather than
// derived, so a test says which identity it is asserting.
func viewerFor(userID uint) contracts.RevisionViewer {
	return contracts.RevisionViewer{UserID: userID}
}

// Every non-approved status is gated by the detail route, so every one of them
// has to be gated here. Asserted as a table rather than on 'private' alone:
// picking one status is how a gate written as `status == "private"` passes
// review.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_HidesEveryGatedStatus() {
	author := s.createTestUser()

	for _, status := range []catalogm.ShowStatus{
		catalogm.ShowStatusPending,
		catalogm.ShowStatusRejected,
		catalogm.ShowStatusPrivate,
	} {
		s.Run(string(status), func() {
			show := s.seedShow("Gated "+string(status), status, nil)
			s.recordShowRevision(show.ID, author.ID)

			_, _, err := s.svc.GetEntityHistory("show", show.ID, 20, 0, viewerPublic)
			s.Require().Error(err)
			s.True(errors.Is(err, contracts.ErrRevisionEntityHidden),
				"a %s show's history must be refused, got %v", status, err)
		})
	}
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_ServesApprovedShowToEveryone() {
	author := s.createTestUser()
	show := s.seedShow("Open Show", catalogm.ShowStatusApproved, nil)
	s.recordShowRevision(show.ID, author.ID)

	revisions, total, err := s.svc.GetEntityHistory("show", show.ID, 20, 0, viewerPublic)
	s.Require().NoError(err)
	s.Len(revisions, 1)
	s.Equal(int64(1), total)
	s.Equal(gatedShowSummary, *revisions[0].Summary,
		"an approved show's history is untouched by the entity gate")
}

// The two tiers the detail route grants, over EVERY gated status rather than
// just private. The deny table above already sweeps the statuses; sweeping them
// on the grant side too is what stops a gate that special-cased one status in
// the granting direction from passing.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_ServesGatedShowToSubmitterAndAdmin() {
	submitter := s.createTestUser()
	author := s.createTestUser()

	for _, status := range []catalogm.ShowStatus{
		catalogm.ShowStatusPending,
		catalogm.ShowStatusRejected,
		catalogm.ShowStatusPrivate,
	} {
		show := s.seedShow("Unpublished "+string(status), status, &submitter.ID)
		s.recordShowRevision(show.ID, author.ID)

		for name, viewer := range map[string]contracts.RevisionViewer{
			"submitter": viewerFor(submitter.ID),
			"admin":     viewerAdmin,
		} {
			s.Run(string(status)+"/"+name, func() {
				revisions, total, err := s.svc.GetEntityHistory("show", show.ID, 20, 0, viewer)
				s.Require().NoError(err)
				s.Len(revisions, 1)
				s.Equal(int64(1), total)
			})
		}
	}
}

// Authorship is not visibility. The editor who wrote the revision reads it as a
// stranger unless they also submitted the show, because the contributions page
// carrying it is world-readable.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_AuthorshipDoesNotGrantAccess() {
	submitter := s.createTestUser()
	author := s.createTestUser()
	show := s.seedShow("Someone Else's Show", catalogm.ShowStatusPrivate, &submitter.ID)
	s.recordShowRevision(show.ID, author.ID)

	_, _, err := s.svc.GetEntityHistory("show", show.ID, 20, 0, viewerFor(author.ID))
	s.True(errors.Is(err, contracts.ErrRevisionEntityHidden),
		"the editor of a gated show they do not own must be refused, got %v", err)
}

// Fail closed. A revision can outlive its show — the polymorphic entity_id
// carries no foreign key — and a gate that answered "no row, nothing to hide"
// would publish exactly the history of shows that were removed.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_MissingShowIsHidden() {
	author := s.createTestUser()
	show := s.seedShow("Doomed", catalogm.ShowStatusApproved, nil)
	s.recordShowRevision(show.ID, author.ID)
	s.Require().NoError(s.db.Exec("DELETE FROM shows WHERE id = ?", show.ID).Error)

	_, _, err := s.svc.GetEntityHistory("show", show.ID, 20, 0, viewerPublic)
	s.True(errors.Is(err, contracts.ErrRevisionEntityHidden),
		"an orphaned show revision must not be published, got %v", err)

	// An admin still reads it. Losing the audit trail for a deleted show is the
	// cost of failing closed, and admins are who need it at that point.
	revisions, _, err := s.svc.GetEntityHistory("show", show.ID, 20, 0, viewerAdmin)
	s.Require().NoError(err)
	s.Len(revisions, 1)
}

// The merge carryover, read side. A row a show merge stamped is suppressed even
// though the show it now points at is approved and readable — which is the whole
// reason the stamp exists, since the show the gate would have consulted was
// deleted by that merge.
//
// The winner is seeded WITH a submitter on purpose. The stamped row's denial to
// that submitter is the diff's one permanent departure from "submitter and admin
// behavior matches the detail route", and a fixture with no submitter cannot
// pin it: viewerFor(someone) against an unowned show is just a stranger, and
// asserts nothing the anonymous case did not already assert.
func (s *RevisionServiceIntegrationTestSuite) TestStampedRowsAreSuppressedOnAnApprovedShow() {
	submitter := s.createTestUser()
	author := s.createTestUser()
	show := s.seedShow("Merge Winner", catalogm.ShowStatusApproved, &submitter.ID)

	kept := s.recordShowRevision(show.ID, author.ID)
	carried := s.recordShowRevision(show.ID, author.ID)
	s.Require().NoError(s.db.Model(&adminm.Revision{}).Where("id = ?", carried.ID).
		Update("from_gated_show", true).Error)

	// Every tier that is not an admin must be denied the stamped row, on every
	// route. The submitter row is the one that would pass vacuously if the
	// fixture were unowned, so it is asserted alongside the others rather than
	// on its own.
	for name, viewer := range map[string]contracts.RevisionViewer{
		"anonymous": viewerPublic,
		"submitter": viewerFor(submitter.ID),
		"author":    viewerFor(author.ID),
	} {
		s.Run(name+" is denied the stamped row", func() {
			revisions, total, err := s.svc.GetEntityHistory("show", show.ID, 20, 0, viewer)
			s.Require().NoError(err, "the show itself is approved, so its history must still open")
			s.Require().Len(revisions, 1, "the stamped row must not be served")
			s.Equal(kept.ID, revisions[0].ID)
			s.Equal(int64(1), total, "the total must count the rows the page contains")

			listed, listedTotal, err := s.svc.GetUserRevisions(author.ID, 20, 0, viewer)
			s.Require().NoError(err)
			s.Require().Len(listed, 1)
			s.Equal(kept.ID, listed[0].ID)
			s.Equal(int64(1), listedTotal)

			got, err := s.svc.GetRevision(carried.ID, viewer)
			s.Require().NoError(err)
			s.Nil(got, "a stamped row must answer like a row that does not exist")

			// The unstamped row on the same show stays readable, so the denial
			// above is the stamp and not a broken fixture.
			open, err := s.svc.GetRevision(kept.ID, viewer)
			s.Require().NoError(err)
			s.NotNil(open)
		})
	}

	s.Run("an admin reads the stamped row", func() {
		got, err := s.svc.GetRevision(carried.ID, viewerAdmin)
		s.Require().NoError(err)
		s.Require().NotNil(got)
		s.Equal(gatedShowSummary, *got.Summary)

		revisions, total, err := s.svc.GetEntityHistory("show", show.ID, 20, 0, viewerAdmin)
		s.Require().NoError(err)
		s.Len(revisions, 2)
		s.Equal(int64(2), total)
	})
}

// The listing, on the gate that is not a stamp. Both revisions belong to the
// same author, and one of their shows was unpublished.
func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_DropsGatedShowRows() {
	submitter := s.createTestUser()
	author := s.createTestUser()

	open := s.seedShow("Still Public", catalogm.ShowStatusApproved, &submitter.ID)
	gated := s.seedShow("Pulled Down", catalogm.ShowStatusPrivate, &submitter.ID)
	openRevision := s.recordShowRevision(open.ID, author.ID)
	s.recordShowRevision(gated.ID, author.ID)

	s.Run("a stranger sees only the public one", func() {
		revisions, total, err := s.svc.GetUserRevisions(author.ID, 20, 0, viewerPublic)
		s.Require().NoError(err)
		s.Require().Len(revisions, 1)
		s.Equal(openRevision.ID, revisions[0].ID)
		s.Equal(int64(1), total)
	})

	s.Run("the show's submitter sees both", func() {
		revisions, total, err := s.svc.GetUserRevisions(author.ID, 20, 0, viewerFor(submitter.ID))
		s.Require().NoError(err)
		s.Len(revisions, 2)
		s.Equal(int64(2), total)
	})

	s.Run("an admin sees both", func() {
		revisions, total, err := s.svc.GetUserRevisions(author.ID, 20, 0, viewerAdmin)
		s.Require().NoError(err)
		s.Len(revisions, 2)
		s.Equal(int64(2), total)
	})
}

// The precedence guard for the listing filter. The visibility condition is
// ANDed with the caller's own filter, so an unwrapped top-level OR would bind as
// `user_id = x AND type <> 'show' OR <visible show>` and pull in every visible
// show revision by every OTHER author. Two authors is the minimum fixture that
// can tell the two bindings apart, and no other test in this package has one.
func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_StaysScopedToItsAuthor() {
	mine := s.createTestUser()
	theirs := s.createTestUser()

	show := s.seedShow("Shared Subject", catalogm.ShowStatusApproved, nil)
	myRevision := s.recordShowRevision(show.ID, mine.ID)
	s.recordShowRevision(show.ID, theirs.ID)

	revisions, total, err := s.svc.GetUserRevisions(mine.ID, 20, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(revisions, 1, "the visibility filter must not widen the author scope")
	s.Equal(myRevision.ID, revisions[0].ID)
	s.Equal(int64(1), total)
}

// The same precedence guard on the single-show lookup. Unwrapped, its OR would
// bind as `id = X AND status = 'approved' OR submitted_by = Y`, which answers
// yes for a gated show whenever the caller submitted ANY show at all. The
// stranger here submits an unrelated show precisely so that mistake would show.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_SubmittingSomeOtherShowGrantsNothing() {
	owner := s.createTestUser()
	stranger := s.createTestUser()
	author := s.createTestUser()

	s.seedShow("The Stranger's Own Show", catalogm.ShowStatusApproved, &stranger.ID)
	gated := s.seedShow("Not Theirs", catalogm.ShowStatusPrivate, &owner.ID)
	s.recordShowRevision(gated.ID, author.ID)

	_, _, err := s.svc.GetEntityHistory("show", gated.ID, 20, 0, viewerFor(stranger.ID))
	s.True(errors.Is(err, contracts.ErrRevisionEntityHidden),
		"submitting some other show must not unlock this one, got %v", err)
}

// The policy is written TWICE — once as a Go predicate (revisionVisibleTo, which
// the single-revision route uses) and once as a SQL WHERE clause
// (visibleRevisionsOnly, which the two listings use). They have to agree, and a
// doc comment saying so is not a mechanism.
//
// This is the mechanism. It builds every combination of the three facts the
// policy reads — show status, whether the viewer submitted the show, whether the
// row carries a merge stamp — and asserts that for every viewer tier the set of
// revisions the SQL path returns is EXACTLY the set the Go path admits. An edit
// that changes one spelling and not the other fails here rather than in
// production, where the two routes would disagree about the same row.
func (s *RevisionServiceIntegrationTestSuite) TestTheGoPredicateAndTheSQLFilterAgree() {
	submitter := s.createTestUser()
	author := s.createTestUser()
	stranger := s.createTestUser()

	type cell struct {
		name    string
		status  catalogm.ShowStatus
		owned   bool
		stamped bool
	}
	var cells []cell
	for _, status := range []catalogm.ShowStatus{
		catalogm.ShowStatusApproved,
		catalogm.ShowStatusPending,
		catalogm.ShowStatusRejected,
		catalogm.ShowStatusPrivate,
	} {
		for _, owned := range []bool{true, false} {
			for _, stamped := range []bool{true, false} {
				cells = append(cells, cell{
					name:    fmt.Sprintf("%s/owned=%t/stamped=%t", status, owned, stamped),
					status:  status,
					owned:   owned,
					stamped: stamped,
				})
			}
		}
	}

	revisionIDs := make([]uint, 0, len(cells))
	for _, c := range cells {
		var owner *uint
		if c.owned {
			owner = &submitter.ID
		}
		show := s.seedShow(c.name, c.status, owner)
		rev := s.recordShowRevision(show.ID, author.ID)
		if c.stamped {
			s.Require().NoError(s.db.Model(&adminm.Revision{}).Where("id = ?", rev.ID).
				Update("from_gated_show", true).Error)
		}
		revisionIDs = append(revisionIDs, rev.ID)
	}

	// admitted records how many rows each tier could read, so the agreement
	// assertion below cannot pass by both paths returning nothing.
	admitted := map[string]int{}

	for name, viewer := range map[string]contracts.RevisionViewer{
		"anonymous": viewerPublic,
		"submitter": viewerFor(submitter.ID),
		"stranger":  viewerFor(stranger.ID),
		"admin":     viewerAdmin,
	} {
		s.Run(name, func() {
			// The SQL path. limit is above the fixture count so one page holds
			// every admitted row.
			listed, total, err := s.svc.GetUserRevisions(author.ID, 100, 0, viewer)
			s.Require().NoError(err)
			viaSQL := make(map[uint]bool, len(listed))
			for _, r := range listed {
				viaSQL[r.ID] = true
			}
			s.Equal(int64(len(listed)), total, "the total must count the rows the page contains")

			// The Go path, asked about the same rows one at a time.
			viaGo := make(map[uint]bool, len(revisionIDs))
			for _, id := range revisionIDs {
				got, err := s.svc.GetRevision(id, viewer)
				s.Require().NoError(err)
				if got != nil {
					viaGo[id] = true
				}
			}

			s.Equal(viaGo, viaSQL,
				"revisionVisibleTo and visibleRevisionsOnly disagree for a %s caller — "+
					"the policy is spelled in Go and in SQL and both spellings must admit "+
					"the same rows", name)
			admitted[name] = len(viaSQL)
		})
	}

	// The agreement above is satisfied by two paths that both return nothing, so
	// the matrix has to be shown to discriminate. An admin reads the whole
	// fixture; an anonymous caller reads strictly less than that and more than
	// none; a stranger reads exactly what an anonymous caller does, because an
	// id that owns nothing buys nothing.
	s.Equal(len(revisionIDs), admitted["admin"], "an admin reads the whole matrix")
	s.Less(admitted["anonymous"], len(revisionIDs), "the gate must withhold something")
	s.Greater(admitted["anonymous"], 0, "the gate must not withhold everything")
	s.Equal(admitted["anonymous"], admitted["stranger"],
		"an authenticated caller who submitted none of these shows reads what anonymous reads")
	s.Greater(admitted["submitter"], admitted["anonymous"],
		"submitting the show must actually unlock rows, or the submitter tier is untested")
}

// Other entity types must be untouched by the show gate. Without this, a gate
// written slightly too broadly would silently empty artist, venue, release,
// label and festival history and no other test in this package would notice.
func (s *RevisionServiceIntegrationTestSuite) TestNonShowHistoryIsUnaffected() {
	author := s.createTestUser()
	venue := s.createTestVenue("Verified Room")
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("verified", true).Error)

	changes := []adminm.FieldChange{{Field: "name", OldValue: "Old", NewValue: "New"}}
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, author.ID, changes, "renamed it"))

	revisions, total, err := s.svc.GetEntityHistory("venue", venue.ID, 20, 0, viewerPublic)
	s.Require().NoError(err)
	s.Len(revisions, 1)
	s.Equal(int64(1), total)
}
