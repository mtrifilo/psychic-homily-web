package community

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// PSY-869 — EntityRequestService trust-tier-gated creation flow
// =============================================================================

type EntityRequestServiceIntegrationTestSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	service *EntityRequestService
}

func (suite *EntityRequestServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.service = NewEntityRequestService(suite.testDB.DB)
}

func (suite *EntityRequestServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *EntityRequestServiceIntegrationTestSuite) SetupTest() {
	sqlDB, _ := suite.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM entity_requests")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

// createUser seeds a user pinned to the given tier and admin flag.
func (suite *EntityRequestServiceIntegrationTestSuite) createUser(name, tier string, isAdmin bool) *authm.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	user := &authm.User{
		Email:         &email,
		FirstName:     &name,
		IsActive:      true,
		EmailVerified: true,
		UserTier:      tier,
		IsAdmin:       isAdmin,
	}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

// marshalArtist builds a typed artist payload for the request body.
func (suite *EntityRequestServiceIntegrationTestSuite) marshalArtist(name string) []byte {
	raw, err := communitym.MarshalPayload(communitym.ArtistRequestPayload{Name: name})
	suite.Require().NoError(err)
	return raw
}

// requireStored reads a request back through the service, so a test comparing
// timestamps compares what POSTGRES holds.
//
// The trap this exists to close: a row returned straight from a create carries
// GORM's in-memory time.Now() stamps, which have nanosecond resolution, while a
// timestamptz column has microsecond resolution. The two are unequal by up to
// 999ns on any host whose wall clock is finer than a microsecond — Linux — and
// exactly equal on one whose clock is not — darwin. A timestamp assertion that
// mixes the two therefore passes every local run and fails every CI run.
func (suite *EntityRequestServiceIntegrationTestSuite) requireStored(requestID uint) *communitym.EntityRequest {
	stored, err := suite.service.GetRequest(requestID)
	suite.Require().NoError(err)
	suite.Require().NotNil(stored)
	return stored
}

// marshalShow builds a typed show payload on a fixed date, optionally carrying
// the bill the contributor knew (PSY-1858). Title AND event_date together are
// the dedup key for a show request (PSY-1977), so a test that varies only the
// bill must hold the date fixed — which this does.
func (suite *EntityRequestServiceIntegrationTestSuite) marshalShow(title string, artists ...communitym.ShowRequestArtist) []byte {
	return suite.marshalShowOn(title, "2026-09-12T21:00:00-07:00", artists...)
}

// marshalShowOn builds a typed show payload on a caller-chosen event_date, for
// the tests that exercise the date half of the dedup key.
func (suite *EntityRequestServiceIntegrationTestSuite) marshalShowOn(title, eventDate string, artists ...communitym.ShowRequestArtist) []byte {
	raw, err := communitym.MarshalPayload(communitym.ShowRequestPayload{
		Title:     title,
		EventDate: eventDate,
		Artists:   artists,
	})
	suite.Require().NoError(err)
	return raw
}

// marshalFestival builds a typed festival payload. start_date is the festival's
// half of the dedup key's occurrence term (PSY-1977).
func (suite *EntityRequestServiceIntegrationTestSuite) marshalFestival(name string, editionYear int, startDate, endDate string) []byte {
	raw, err := communitym.MarshalPayload(communitym.FestivalRequestPayload{
		Name:        name,
		EditionYear: editionYear,
		StartDate:   startDate,
		EndDate:     endDate,
	})
	suite.Require().NoError(err)
	return raw
}

// --- Trust-tier gating -------------------------------------------------------

// new_user → queued for admin (pending), no decision stamped.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_NewUser_Pending() {
	user := suite.createUser("newbie", tierNewUser, false)

	req, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Pending Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().NotNil(req)

	suite.Assert().Equal(communitym.EntityRequestStatePending, req.DecisionState)
	suite.Assert().Nil(req.DecidedBy)
	suite.Assert().Nil(req.DecidedAt)
	suite.Assert().Equal(user.ID, req.RequesterID)
	suite.Assert().Equal(communitym.EntityRequestArtist, req.EntityType)
}

// contributor → queued for admin (pending). This is the AC's canonical
// "contributor request → pending" assertion.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_Contributor_Pending() {
	user := suite.createUser("contrib", tierContributor, false)

	req, _, err := suite.service.CreateRequest(user, communitym.EntityRequestVenue,
		mustMarshalVenue(suite, "The Pending Room"), communitym.EntityRequestSourcePasteMode, nil, false)
	suite.Require().NoError(err)

	suite.Assert().Equal(communitym.EntityRequestStatePending, req.DecisionState)
	suite.Assert().Nil(req.DecidedBy)
}

// admin → auto-approved on create, decision stamped with the admin's own id.
// This is the AC's canonical "admin request → auto-approved" assertion.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_Admin_AutoApproved() {
	user := suite.createUser("boss", tierNewUser, true) // tier irrelevant; IsAdmin wins

	req, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Auto Approved Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	suite.Assert().Equal(communitym.EntityRequestStateApproved, req.DecisionState)
	suite.Require().NotNil(req.DecidedBy)
	suite.Assert().Equal(user.ID, *req.DecidedBy)
	suite.Require().NotNil(req.DecidedAt)
}

// local_ambassador → auto-approved on create (highest non-admin trust).
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_LocalAmbassador_AutoApproved() {
	user := suite.createUser("amb", tierLocalAmbassador, false)

	req, _, err := suite.service.CreateRequest(user, communitym.EntityRequestLabel,
		mustMarshalLabel(suite, "Ambassador Records"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	suite.Assert().Equal(communitym.EntityRequestStateApproved, req.DecisionState)
	suite.Require().NotNil(req.DecidedBy)
	suite.Assert().Equal(user.ID, *req.DecidedBy)
}

// trusted_contributor + confirmed → auto-approved (FE confirm step is the gate).
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_TrustedConfirmed_AutoApproved() {
	user := suite.createUser("trusted", tierTrustedContributor, false)

	req, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Trusted Confirmed Band"), communitym.EntityRequestSourceManual, nil, true)
	suite.Require().NoError(err)

	suite.Assert().Equal(communitym.EntityRequestStateApproved, req.DecisionState)
	suite.Require().NotNil(req.DecidedBy)
}

// trusted_contributor + NOT confirmed → still queued (pending). The confirm
// step is what unlocks auto-approve for this tier.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_TrustedUnconfirmed_Pending() {
	user := suite.createUser("trusted2", tierTrustedContributor, false)

	req, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Trusted Unconfirmed Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	suite.Assert().Equal(communitym.EntityRequestStatePending, req.DecisionState)
	suite.Assert().Nil(req.DecidedBy)
}

// --- Payload integrity through the DB ---------------------------------------

// An artist-type row written through the service reads back as a clean
// ArtistRequestPayload with no field loss after a full DB round-trip. This is
// the substantive guarantee behind the ticket's "a backfilled artist row reads
// back cleanly as ArtistRequestPayload" AC — adapted to a freshly-created row
// because there is no artist_requests source table to backfill (see PR body).
func (suite *EntityRequestServiceIntegrationTestSuite) TestArtistPayload_RoundTripsThroughDB() {
	user := suite.createUser("rt", tierNewUser, false)

	full := communitym.ArtistRequestPayload{
		Name:             "Round Trip Band",
		City:             strptr("Phoenix"),
		State:            strptr("AZ"),
		Country:          strptr("USA"),
		Description:      strptr("Full payload."),
		ImageURL:         strptr("https://img.example/rt.jpg"),
		BandcampEmbedURL: strptr("https://bandcamp.example/rt"),
	}
	raw, err := communitym.MarshalPayload(full)
	suite.Require().NoError(err)

	created, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		raw, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	// Re-fetch from the DB (not the in-memory struct) to prove JSONB persistence.
	fetched, err := suite.service.GetRequest(created.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(fetched)
	suite.Require().NotNil(fetched.Payload)

	out, err := communitym.UnmarshalPayload[communitym.ArtistRequestPayload](*fetched.Payload)
	suite.Require().NoError(err)
	suite.Assert().Equal(full, out)
}

// --- Validation --------------------------------------------------------------

func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_InvalidEntityType() {
	user := suite.createUser("bad", tierNewUser, false)
	_, _, err := suite.service.CreateRequest(user, "podcast",
		suite.marshalArtist("X"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().Error(err)
}

func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_InvalidSourceContext() {
	user := suite.createUser("bad2", tierNewUser, false)
	_, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("X"), "telepathy", nil, false)
	suite.Require().Error(err)
}

func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_EmptyPayload() {
	user := suite.createUser("bad3", tierNewUser, false)
	_, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		nil, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().Error(err)
}

// --- Moderation decisions ----------------------------------------------------

// Admin approves a pending request → state flips, decider stamped.
func (suite *EntityRequestServiceIntegrationTestSuite) TestDecide_ApprovePending() {
	requester := suite.createUser("req", tierNewUser, false)
	admin := suite.createUser("mod", tierNewUser, true)

	pending, _, err := suite.service.CreateRequest(requester, communitym.EntityRequestArtist,
		suite.marshalArtist("Needs Review"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().Equal(communitym.EntityRequestStatePending, pending.DecisionState)

	note := "looks good"
	decided, err := suite.service.Decide(pending.ID, admin.ID, communitym.EntityRequestStateApproved, &note, nil)
	suite.Require().NoError(err)
	suite.Assert().Equal(communitym.EntityRequestStateApproved, decided.DecisionState)
	suite.Require().NotNil(decided.DecidedBy)
	suite.Assert().Equal(admin.ID, *decided.DecidedBy)
	suite.Require().NotNil(decided.DecisionNote)
	suite.Assert().Equal(note, *decided.DecisionNote)
}

// Deciding a non-pending request is rejected (idempotency / no double-decide).
func (suite *EntityRequestServiceIntegrationTestSuite) TestDecide_AlreadyResolved() {
	admin := suite.createUser("mod2", tierNewUser, true)

	// Admin-created request is already approved on create.
	approved, _, err := suite.service.CreateRequest(admin, communitym.EntityRequestArtist,
		suite.marshalArtist("Already Approved"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().Equal(communitym.EntityRequestStateApproved, approved.DecisionState)

	_, err = suite.service.Decide(approved.ID, admin.ID, communitym.EntityRequestStateRejected, nil, nil)
	suite.Require().Error(err)
}

// A second decision on a request that was already decided loses: the atomic
// WHERE decision_state='pending' guard matches 0 rows and the call returns an
// invalid-state conflict reporting the CURRENT (first-winner) state — it does
// NOT silently clobber the first decision. This is the sequential proxy for
// the concurrent double-decide race the conditional UPDATE guards against.
func (suite *EntityRequestServiceIntegrationTestSuite) TestDecide_SecondDecisionDoesNotClobber() {
	requester := suite.createUser("req3", tierNewUser, false)
	admin := suite.createUser("mod4", tierNewUser, true)

	pending, _, err := suite.service.CreateRequest(requester, communitym.EntityRequestArtist,
		suite.marshalArtist("Contested"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	// First decision wins: approve.
	decided, err := suite.service.Decide(pending.ID, admin.ID, communitym.EntityRequestStateApproved, nil, nil)
	suite.Require().NoError(err)
	suite.Require().Equal(communitym.EntityRequestStateApproved, decided.DecisionState)

	// Second decision (reject) must fail and leave the row APPROVED.
	_, err = suite.service.Decide(pending.ID, admin.ID, communitym.EntityRequestStateRejected, nil, nil)
	suite.Require().Error(err)

	fetched, err := suite.service.GetRequest(pending.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal(communitym.EntityRequestStateApproved, fetched.DecisionState,
		"first decision must survive; second must not clobber it")
}

// Decide rejects a non-approve/reject target state.
func (suite *EntityRequestServiceIntegrationTestSuite) TestDecide_InvalidTargetState() {
	requester := suite.createUser("req2", tierNewUser, false)
	admin := suite.createUser("mod3", tierNewUser, true)
	pending, _, err := suite.service.CreateRequest(requester, communitym.EntityRequestArtist,
		suite.marshalArtist("X"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	_, err = suite.service.Decide(pending.ID, admin.ID, communitym.EntityRequestStatePending, nil, nil)
	suite.Require().Error(err)
}

// --- Listing -----------------------------------------------------------------

// ListPending returns only pending rows, newest-first, and respects the
// entity_type filter.
func (suite *EntityRequestServiceIntegrationTestSuite) TestListPending_FiltersAndExcludesApproved() {
	newbie := suite.createUser("lister", tierNewUser, false)
	admin := suite.createUser("listmod", tierNewUser, true)

	// One pending artist (new_user), one pending venue (new_user), one approved
	// artist (admin auto-approve) that must NOT appear.
	_, _, err := suite.service.CreateRequest(newbie, communitym.EntityRequestArtist,
		suite.marshalArtist("Pending A"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	_, _, err = suite.service.CreateRequest(newbie, communitym.EntityRequestVenue,
		mustMarshalVenue(suite, "Pending V"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	_, _, err = suite.service.CreateRequest(admin, communitym.EntityRequestArtist,
		suite.marshalArtist("Approved A"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	all, total, err := suite.service.ListPending("", 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(2), total, "approved row must be excluded")
	suite.Assert().Len(all, 2)

	artistsOnly, total, err := suite.service.ListPending(communitym.EntityRequestArtist, 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total)
	suite.Require().Len(artistsOnly, 1)
	suite.Assert().Equal(communitym.EntityRequestArtist, artistsOnly[0].EntityType)
}

// --- PSY-997: ListRequests (admin queue with state + source filters) --------

// Default state (empty filter) returns pending only, mirroring ListPending,
// and excludes approved rows.
func (suite *EntityRequestServiceIntegrationTestSuite) TestListRequests_DefaultPendingExcludesApproved() {
	newbie := suite.createUser("lr-newbie", tierNewUser, false)
	admin := suite.createUser("lr-admin", tierNewUser, true)

	_, _, err := suite.service.CreateRequest(newbie, communitym.EntityRequestArtist,
		suite.marshalArtist("LR Pending"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	_, _, err = suite.service.CreateRequest(admin, communitym.EntityRequestArtist,
		suite.marshalArtist("LR Approved"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	rows, total, err := suite.service.ListRequests(&contracts.EntityRequestFilters{})
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total, "default filter is pending-only")
	suite.Require().Len(rows, 1)
	suite.Assert().Equal(communitym.EntityRequestStatePending, rows[0].DecisionState)
}

// Explicit state=approved returns the approved rows.
func (suite *EntityRequestServiceIntegrationTestSuite) TestListRequests_StateApproved() {
	admin := suite.createUser("lr-admin2", tierNewUser, true)
	_, _, err := suite.service.CreateRequest(admin, communitym.EntityRequestArtist,
		suite.marshalArtist("LR Approved 2"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	rows, total, err := suite.service.ListRequests(&contracts.EntityRequestFilters{
		State: string(communitym.EntityRequestStateApproved),
	})
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total)
	suite.Require().Len(rows, 1)
	suite.Assert().Equal(communitym.EntityRequestStateApproved, rows[0].DecisionState)
}

// source_context filter narrows the pending queue to one origin.
func (suite *EntityRequestServiceIntegrationTestSuite) TestListRequests_SourceContextFilter() {
	newbie := suite.createUser("lr-src", tierNewUser, false)

	_, _, err := suite.service.CreateRequest(newbie, communitym.EntityRequestArtist,
		suite.marshalArtist("Manual one"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	_, _, err = suite.service.CreateRequest(newbie, communitym.EntityRequestArtist,
		suite.marshalArtist("Paste one"), communitym.EntityRequestSourcePasteMode, nil, false)
	suite.Require().NoError(err)

	rows, total, err := suite.service.ListRequests(&contracts.EntityRequestFilters{
		SourceContext: communitym.EntityRequestSourcePasteMode,
	})
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total)
	suite.Require().Len(rows, 1)
	suite.Assert().Equal(communitym.EntityRequestSourcePasteMode, rows[0].SourceContext)
}

// entity_type filter narrows by type; pagination bounds the result page while
// total reflects the full filtered count.
func (suite *EntityRequestServiceIntegrationTestSuite) TestListRequests_EntityTypeAndPagination() {
	newbie := suite.createUser("lr-page", tierNewUser, false)

	for i := 0; i < 3; i++ {
		_, _, err := suite.service.CreateRequest(newbie, communitym.EntityRequestArtist,
			suite.marshalArtist(fmt.Sprintf("Page artist %d", i)), communitym.EntityRequestSourceManual, nil, false)
		suite.Require().NoError(err)
	}
	_, _, err := suite.service.CreateRequest(newbie, communitym.EntityRequestVenue,
		mustMarshalVenue(suite, "Page venue"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	rows, total, err := suite.service.ListRequests(&contracts.EntityRequestFilters{
		EntityType: communitym.EntityRequestArtist,
		Limit:      2,
		Offset:     0,
	})
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(3), total, "total counts all filtered rows, not the page")
	suite.Assert().Len(rows, 2, "page is bounded by limit")
	for _, r := range rows {
		suite.Assert().Equal(communitym.EntityRequestArtist, r.EntityType)
	}
}

// --- PSY-1008 / PSY-1948: a resubmission REPLACES the pending duplicate ------

// A second PENDING request for the same (entity_type, requester, normalized
// name) lands on the EXISTING row — no error, no duplicate row — and overwrites
// its payload with the resubmission's. Casing + surrounding whitespace are
// normalized, matching the unique index.
//
// This is also what pins the occurrence term to the empty string rather than
// NULL for a type whose payload carries no date (PSY-1977). An artist has no
// occurrence, so both rows key on the empty string and collide exactly as they
// always did. Were the term NULL instead, Postgres's unique-index semantics
// would make every row DISTINCT, this second submission would file its own row,
// and all three assertions below would fail.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_DuplicatePending_ReplacesPayload() {
	user := suite.createUser("dup", tierContributor, false)

	first, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Duplicate Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().Equal(communitym.EntityRequestStatePending, first.DecisionState)
	suite.Assert().False(replaced, "the first submission files a new row")

	// The timestamp baseline, read back BEFORE the resubmission so it is what
	// Postgres holds and not what GORM stamped in memory. See the note below.
	beforeReplace := suite.requireStored(first.ID)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("  duplicate band  "), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced, "a resubmission must report that it corrected the queued row")
	suite.Assert().Equal(first.ID, second.ID, "a resubmission corrects the queued row, it does not file a second")
	suite.Assert().Equal(communitym.EntityRequestStatePending, second.DecisionState,
		"a replacement leaves the row queued, it does not decide it")
	suite.Require().NotNil(second.Payload)
	suite.Assert().JSONEq(`{"name":"  duplicate band  "}`, string(*second.Payload),
		"the stored payload must be the resubmitted one")

	// updated_at is the only signal that separates a corrected request from an
	// untouched one, and the admin queue reads it (AdminEntityRequestView). A
	// refactor that stops bumping it would leave a replacement invisible.
	//
	// BOTH SIDES of every timestamp comparison must be values Postgres has
	// stored. A fresh create's timestamps are GORM's in-memory time.Now() stamps,
	// which never went through the database and so still carry nanoseconds, while
	// timestamptz holds microseconds. Comparing one against the other is a
	// platform coin-flip that passes on darwin (its wall clock is already
	// microsecond-aligned, so the two coincide) and fails on Linux (it is not, so
	// they never do) — which is exactly how this test passed locally five times
	// and failed its first CI run by 24ns.
	suite.Assert().Equal(beforeReplace.CreatedAt, second.CreatedAt,
		"a replacement keeps the request's place in the queue")
	suite.Assert().True(second.UpdatedAt.After(beforeReplace.UpdatedAt),
		"a replacement must move updated_at, the queue's only sign it happened")

	var count int64
	suite.Require().NoError(suite.db.Model(&communitym.EntityRequest{}).Count(&count).Error)
	suite.Assert().Equal(int64(1), count, "no duplicate row should be created")
}

// An auto-approving tier never replaces: its row is stamped 'approved' before the
// insert, so it never meets the pending-only dedup index.
//
// The consequence worth pinning is what is LEFT BEHIND: the earlier PENDING
// request stays queued carrying the payload the requester has since moved on
// from, beside a new approved row for the same name. An admin who later approves
// the stale one fulfills it into a duplicate catalog entity, or a 409 and an
// approved-but-unfulfilled orphan. Reached through the API by any tier whose
// confirmation state changes between two submissions, not through a particular
// UI flow — the shipping trusted-tier client confirms before its first POST.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_AutoApprovingTier_DoesNotReplaceItsPendingRow() {
	user := suite.createUser("trusted-redo", tierTrustedContributor, false)

	queued, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Boris"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().Equal(communitym.EntityRequestStatePending, queued.DecisionState)
	suite.Require().False(replaced)

	confirmed, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Boris"), communitym.EntityRequestSourceManual, nil, true)
	suite.Require().NoError(err)
	suite.Assert().False(replaced, "an auto-approved submission replaces nothing")
	suite.Assert().NotEqual(queued.ID, confirmed.ID, "it files its own approved row")
	suite.Assert().Equal(communitym.EntityRequestStateApproved, confirmed.DecisionState)

	stale, err := suite.service.GetRequest(queued.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal(communitym.EntityRequestStatePending, stale.DecisionState,
		"the earlier request is still queued, carrying its original payload")
}

// The canonical correction (PSY-1858's bill): a contributor files a show with no
// bill, learns the bill, and resubmits the same title. The queued request must
// carry the corrected bill — the whole reason a payload bill is worth storing.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_ResubmittedShow_CorrectsTheBill() {
	user := suite.createUser("bill", tierContributor, false)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		suite.marshalShow("Doom Night"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	headliner := "headliner"
	correctedPayload := suite.marshalShow("Doom Night",
		communitym.ShowRequestArtist{Name: "Boris", SetType: &headliner},
		communitym.ShowRequestArtist{Name: "Earth"},
	)
	corrected, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		correctedPayload, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().True(replaced)
	suite.Require().Equal(first.ID, corrected.ID)

	// Read the row back rather than trusting the returned struct: the moderation
	// queue reads the stored payload, and that is what has to carry the bill.
	fetched, err := suite.service.GetRequest(first.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(fetched.Payload)
	suite.Assert().JSONEq(string(correctedPayload), string(*fetched.Payload),
		"the corrected bill, and nothing else, is what the queue now holds")
}

// source_context and source_detail describe the SUBMISSION, so they move with
// the payload. A resubmission carrying no detail clears the stored one rather
// than leaving a detail that described the superseded payload.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_Resubmission_RefreshesSourceFields() {
	user := suite.createUser("srcref", tierContributor, false)
	detail, err := json.Marshal(communitym.EntityRequestSourceDetail{
		URL: strptr("https://example.com/first"),
	})
	suite.Require().NoError(err)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Sourced Correction"), communitym.EntityRequestSourceAIExtraction, detail, false)
	suite.Require().NoError(err)
	suite.Require().NotNil(first.SourceDetail)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Sourced Correction"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().True(replaced)

	suite.Assert().Equal(communitym.EntityRequestSourceManual, second.SourceContext,
		"the resubmission's origin replaces the superseded one")
	suite.Assert().Nil(second.SourceDetail,
		"a resubmission with no source detail clears the stored one")
}

// Dedup is PENDING-only: once the prior request is decided, an identical new
// request creates a fresh row (a user may legitimately re-request) and the
// decided row is left exactly as it was decided.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_DuplicateAfterDecision_CreatesNew() {
	user := suite.createUser("redup", tierContributor, false)
	admin := suite.createUser("redup-admin", tierNewUser, true)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Reborn Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	_, err = suite.service.Decide(first.ID, admin.ID, communitym.EntityRequestStateRejected, nil, nil)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("REBORN BAND"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().False(replaced, "a decided row is never replaced")
	suite.Assert().NotEqual(first.ID, second.ID, "after the prior request is decided, a re-request is a new row")

	decided, err := suite.service.GetRequest(first.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(decided.Payload)
	suite.Assert().JSONEq(`{"name":"Reborn Band"}`, string(*decided.Payload),
		"the decided row's payload must survive a later resubmission")
	suite.Assert().Equal(communitym.EntityRequestStateRejected, decided.DecisionState)
}

// The dedup key is the normalized NAME, so a resubmission that changes it is a
// different request, not a correction of the queued one. Correcting a misspelled
// name therefore leaves the original queued — stated here because it is the
// boundary a contributor is most likely to walk into.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_ResubmissionUnderANewName_FilesASecondRequest() {
	user := suite.createUser("rename", tierContributor, false)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Borsi"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Boris"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().False(replaced, "a different name is a different request")
	suite.Assert().NotEqual(first.ID, second.ID)

	var count int64
	suite.Require().NoError(suite.db.Model(&communitym.EntityRequest{}).Count(&count).Error)
	suite.Assert().Equal(int64(2), count, "the misspelled request stays queued until an admin decides it")
}

// PSY-1977: a recurring night is the domain's normal case, not an edge one. Two
// same-titled shows on DIFFERENT dates from one requester are two distinct
// requests, so both must survive as separate pending rows. Before the date
// joined the dedup key the second submission REPLACED the first, and the
// September request was gone.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameTitleDifferentDate_FilesASecondRequest() {
	user := suite.createUser("recurring", tierContributor, false)

	september, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		suite.marshalShowOn("Open Mic", "2026-09-03T20:00:00-07:00"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().False(replaced)

	october, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		suite.marshalShowOn("Open Mic", "2026-10-01T20:00:00-07:00"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().False(replaced, "a different date is a different show, not a correction")
	suite.Assert().NotEqual(september.ID, october.ID, "it files its own row")

	// The September request must still hold the September date. Read it back
	// rather than trusting the returned struct: the destruction this guards was a
	// write to the STORED payload.
	stored := suite.requireStored(september.ID)
	suite.Require().NotNil(stored.Payload)
	suite.Assert().JSONEq(string(suite.marshalShowOn("Open Mic", "2026-09-03T20:00:00-07:00")), string(*stored.Payload),
		"the earlier request survives untouched")
	suite.Assert().Equal(communitym.EntityRequestStatePending, stored.DecisionState)

	var count int64
	suite.Require().NoError(suite.db.Model(&communitym.EntityRequest{}).Count(&count).Error)
	suite.Assert().Equal(int64(2), count, "both nights are queued")
}

// The replace-on-resubmit contract (PSY-1948) is unchanged for a TRUE match: the
// same title on the same date is the same request, and resubmitting it corrects
// the queued row rather than filing a second one.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameTitleSameDate_StillReplaces() {
	user := suite.createUser("samenight", tierContributor, false)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		suite.marshalShowOn("Open Mic", "2026-09-03T20:00:00-07:00"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	corrected := suite.marshalShowOn("  open mic  ", "2026-09-03T20:00:00-07:00",
		communitym.ShowRequestArtist{Name: "Boris"})
	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		corrected, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced, "the same night resubmitted is a correction")
	suite.Assert().Equal(first.ID, second.ID)

	stored := suite.requireStored(first.ID)
	suite.Require().NotNil(stored.Payload)
	suite.Assert().JSONEq(string(corrected), string(*stored.Payload))

	var count int64
	suite.Require().NoError(suite.db.Model(&communitym.EntityRequest{}).Count(&count).Error)
	suite.Assert().Equal(int64(1), count)
}

// The occurrence term of the key normalizes the way the name term does: a
// resubmission whose date carries surrounding SPACES still matches, because both
// sides are trimmed. A date the payload validator accepts must not become a
// second request on spacing alone.
//
// Spaces specifically, because SQL trim() strips only those while the validator's
// strings.TrimSpace strips every Unicode space character. A tab-padded date would
// validate and then key differently, filing a second request. That asymmetry is
// the name term's too and predates this key — the point here is that both terms
// normalize the SAME way, not that either normalizes exhaustively. It costs a
// duplicate row an admin rejects, never a destroyed one.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameDateWithSurroundingSpace_StillReplaces() {
	user := suite.createUser("spacedate", tierContributor, false)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		suite.marshalShowOn("Emo Night", "2026-09-03T20:00:00-07:00"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		suite.marshalShowOn("Emo Night", " 2026-09-03T20:00:00-07:00 "),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced, "whitespace around the date is not a different night")
	suite.Assert().Equal(first.ID, second.ID)
}

// PSY-1989: two editions of one festival are two requests. Before this key the
// second destroyed the first.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_FestivalEditions_FileTwoRequests() {
	user := suite.createUser("fest", tierContributor, false)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestFestival,
		suite.marshalFestival("Psycho Las Vegas", 2026, "2026-08-14", "2026-08-16"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestFestival,
		suite.marshalFestival("Psycho Las Vegas", 2027, "2027-08-13", "2027-08-15"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().False(replaced, "a second edition is a second request, not a correction")
	suite.Assert().NotEqual(first.ID, second.ID)

	var count int64
	suite.Require().NoError(suite.db.Model(&communitym.EntityRequest{}).Count(&count).Error)
	suite.Assert().Equal(int64(2), count, "both editions are queued")
}

// PSY-1989: the SAME edition resubmitted is still a correction (PSY-1948's
// contract), which is the half a widened key can silently break.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameFestivalEdition_StillReplaces() {
	user := suite.createUser("festsame", tierContributor, false)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestFestival,
		suite.marshalFestival("Psycho Las Vegas", 2026, "2026-08-14", "2026-08-16"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	// A corrected end date, same edition: the term reads the edition year, not the
	// dates, so this lands on the queued row.
	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestFestival,
		suite.marshalFestival("Psycho Las Vegas", 2026, "2026-08-14", "2026-08-17"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced, "one edition resubmitted is a correction")
	suite.Assert().Equal(first.ID, second.ID)
}

// PSY-1989: a festival that STATES no edition year is keyed on the year derived
// from its start_date, which is the derivation fulfillment performs. Two things
// are pinned here, and the second is why the term carries an absent-sentinel: a
// stated 2026 and a marshalled zero value with a 2026 start date are ONE request,
// not two, because they fulfil to one catalog edition.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_FestivalEditionYearDerivedFromStartDate() {
	user := suite.createUser("festderive", tierContributor, false)

	stated, _, err := suite.service.CreateRequest(user, communitym.EntityRequestFestival,
		suite.marshalFestival("Gilead Fest", 2026, "2026-07-10", "2026-07-12"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	// EditionYear 0 is what a marshalled payload carries when the client states no
	// edition; fulfillment reads it as "derive from start_date".
	derived, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestFestival,
		suite.marshalFestival("Gilead Fest", 0, "2026-07-10", "2026-07-12"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced,
		"a stated 2026 and a derived 2026 are one edition; keying them apart would file "+
			"two requests that fulfil to the same festival")
	suite.Assert().Equal(stated.ID, derived.ID)

	// And a different derived year is still a different edition.
	next, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestFestival,
		suite.marshalFestival("Gilead Fest", 0, "2027-07-09", "2027-07-11"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().False(replaced)
	suite.Assert().NotEqual(stated.ID, next.ID)
}

// The residual SHOW collision, pinned because it is the ticket's own harm class
// left standing: same title, same date, different venue. The payload has no venue
// field and city/state are optional, so the key cannot separate these two. A
// franchise night running two cities the same evening is the real case.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameTitleSameDateDifferentCity_StillCollides() {
	user := suite.createUser("franchise", tierContributor, false)

	phoenix, err := communitym.MarshalPayload(communitym.ShowRequestPayload{
		Title: "Emo Night", EventDate: "2026-09-03T20:00:00-07:00", City: strptr("Phoenix"),
	})
	suite.Require().NoError(err)
	tucson, err := communitym.MarshalPayload(communitym.ShowRequestPayload{
		Title: "Emo Night", EventDate: "2026-09-03T20:00:00-07:00", City: strptr("Tucson"),
	})
	suite.Require().NoError(err)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		phoenix, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		tucson, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced, "UNFIXED: the Tucson night replaces the Phoenix one")
	suite.Assert().Equal(first.ID, second.ID)

	stored := suite.requireStored(first.ID)
	suite.Require().NotNil(stored.Payload)
	suite.Assert().JSONEq(string(tucson), string(*stored.Payload), "the Phoenix request is gone")
}

// PSY-1989: two same-named venues in different cities are two requests. Before
// this key the second destroyed the first.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameVenueNameDifferentCity_FilesTwoRequests() {
	user := suite.createUser("chain", tierContributor, false)

	sf, err := communitym.MarshalPayload(communitym.VenueRequestPayload{
		Name: "The Fillmore", City: "San Francisco", State: "CA",
	})
	suite.Require().NoError(err)
	philly, err := communitym.MarshalPayload(communitym.VenueRequestPayload{
		Name: "The Fillmore", City: "Philadelphia", State: "PA",
	})
	suite.Require().NoError(err)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestVenue,
		sf, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestVenue,
		philly, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().False(replaced, "two different Fillmores are two requests")
	suite.Assert().NotEqual(first.ID, second.ID)

	stored := suite.requireStored(first.ID)
	suite.Require().NotNil(stored.Payload)
	suite.Assert().JSONEq(string(sf), string(*stored.Payload), "the San Francisco request survives")
}

// PSY-1989: the SAME venue resubmitted is still a correction, and the city term
// is CASE-FOLDED so that "Philadelphia" and "philadelphia" — one venue under the
// catalog's UNIQUE (LOWER(name), LOWER(city)) — are one request here too.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameVenueSameCity_StillReplaces() {
	user := suite.createUser("chainsame", tierContributor, false)

	first, err := communitym.MarshalPayload(communitym.VenueRequestPayload{
		Name: "The Fillmore", City: "Philadelphia", State: "PA",
	})
	suite.Require().NoError(err)
	// A different case and surrounding space on the city, plus a corrected
	// address: one venue, so one request.
	corrected, err := communitym.MarshalPayload(communitym.VenueRequestPayload{
		Name: "The Fillmore", City: "  philadelphia ", State: "PA",
		Address: strptr("29 E Allen St"),
	})
	suite.Require().NoError(err)

	created, _, err := suite.service.CreateRequest(user, communitym.EntityRequestVenue,
		first, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestVenue,
		corrected, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced,
		"the catalog compares city case-insensitively, so these are one venue")
	suite.Assert().Equal(created.ID, second.ID)
}

// PSY-1989 deliberately leaves STATE out of the venue key, because the catalog's
// constraint is (LOWER(name), LOWER(city)) and CreateVenue refuses a second venue
// with that pair. Keying on state would file two requests that fulfil to one
// venue, and the second approval would fail after its row was claimed, leaving an
// approved-but-unfulfilled row. Pinned so that adding state is a decision rather
// than an accident.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameVenueNameAndCityDifferentState_StillReplaces() {
	user := suite.createUser("chainstate", tierContributor, false)

	az, err := communitym.MarshalPayload(communitym.VenueRequestPayload{
		Name: "Crescent Ballroom", City: "Phoenix", State: "AZ",
	})
	suite.Require().NoError(err)
	ny, err := communitym.MarshalPayload(communitym.VenueRequestPayload{
		Name: "Crescent Ballroom", City: "Phoenix", State: "NY",
	})
	suite.Require().NoError(err)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestVenue,
		az, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestVenue,
		ny, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced, "the catalog treats these as one venue, so the queue does too")
	suite.Assert().Equal(first.ID, second.ID)
}

// PSY-1989: an artist keys on the name ALONE, and that is correct rather than a
// gap — artists_lower_name_uniq makes an artist name globally unique, so two
// same-named artist requests are one catalog artist. Pinned so that adding a city
// term forces a look at that constraint first.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameArtistNameDifferentCity_StillReplaces() {
	user := suite.createUser("artistcity", tierContributor, false)

	phx, err := communitym.MarshalPayload(communitym.ArtistRequestPayload{
		Name: "Sunburst", City: strptr("Phoenix"), State: strptr("AZ"),
	})
	suite.Require().NoError(err)
	chi, err := communitym.MarshalPayload(communitym.ArtistRequestPayload{
		Name: "Sunburst", City: strptr("Chicago"), State: strptr("IL"),
	})
	suite.Require().NoError(err)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		phx, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		chi, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced, "the catalog holds one artist per name, so the queue does too")
	suite.Assert().Equal(first.ID, second.ID)
}

// PSY-1989: a label still collides, and unlike an artist that IS a gap — the
// catalog permits two same-named labels. Every field that would separate them is
// optional. Pinned so that promoting one to required forces a look at this key.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameLabelNameDifferentCity_StillCollides() {
	user := suite.createUser("labelcity", tierContributor, false)

	phx, err := communitym.MarshalPayload(communitym.LabelRequestPayload{
		Name: "Vanity Press", City: strptr("Phoenix"),
	})
	suite.Require().NoError(err)
	ldn, err := communitym.MarshalPayload(communitym.LabelRequestPayload{
		Name: "Vanity Press", City: strptr("London"),
	})
	suite.Require().NoError(err)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestLabel,
		phx, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestLabel,
		ldn, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced, "UNFIXED: two different labels sharing a name are one request")
	suite.Assert().Equal(first.ID, second.ID)
}

// The unfixed collision for releases, which PSY-1989 named explicitly
// (self-titled, "Untitled", "Live at X"). What separates two same-titled releases
// is the ARTIST, and the payload has no artist field; release_date is OPTIONAL,
// so it cannot be the occurrence term without turning "queued without a date,
// resubmitted with one" into a second request instead of the correction it is.
// Pinned so that adding an artist to the payload forces a look at this key.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_ReleasesWithDifferentDates_StillCollide() {
	user := suite.createUser("rel", tierContributor, false)

	early, err := communitym.MarshalPayload(communitym.ReleaseRequestPayload{
		Title: "Untitled", ReleaseDate: strptr("1994-03-01"),
	})
	suite.Require().NoError(err)
	late, err := communitym.MarshalPayload(communitym.ReleaseRequestPayload{
		Title: "Untitled", ReleaseDate: strptr("2011-09-20"),
	})
	suite.Require().NoError(err)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestRelease,
		early, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestRelease,
		late, communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().True(replaced, "UNFIXED: two distinct releases sharing a title still collide")
	suite.Assert().Equal(first.ID, second.ID)
}

// Two spellings of ONE moment are two requests, because the occurrence term is
// the event_date STRING compared byte for byte, not the instant it denotes. This
// is the cost of not canonicalizing at the boundary, and it is pinned rather than
// left to be discovered: a producer re-serializing from a JS Date emits the Z
// form every time and will land here. See ShowRequestPayload's doc block.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SameInstantDifferentSpelling_FilesASecondRequest() {
	user := suite.createUser("spelling", tierContributor, false)

	local := "2026-09-03T20:00:00-07:00"
	utc := "2026-09-04T03:00:00Z"
	parsedLocal, err := time.Parse(time.RFC3339, local)
	suite.Require().NoError(err)
	parsedUTC, err := time.Parse(time.RFC3339, utc)
	suite.Require().NoError(err)
	suite.Require().True(parsedLocal.Equal(parsedUTC), "the fixture must be the same instant")

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		suite.marshalShowOn("Emo Night", local), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		suite.marshalShowOn("Emo Night", utc), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().False(replaced, "byte-different spellings do not match, so this is not a correction")
	suite.Assert().NotEqual(first.ID, second.ID)

	// The date-only form is the same trap on the more common path: it is anchored
	// at 20:00 venue-local at fulfillment, so for an Arizona venue it denotes this
	// same evening, and it still files its own row.
	third, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		suite.marshalShowOn("Emo Night", "2026-09-03"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().False(replaced)
	suite.Assert().NotEqual(first.ID, third.ID)
}

// The index must be able to store a row whose event_date is far too long for a
// btree key. This is the property the up migration's "the build cannot fail on
// existing data" claim rests on: rows queued BEFORE maxRequestDateLen existed can
// hold a multi-kilobyte event_date, and the migration has to index them.
//
// The service is used deliberately rather than a raw INSERT, because it is the
// index — not the boundary — that this pins: CreateRequest does not validate
// payloads (that is the HTTP layer's job), so this is exactly the shape a legacy
// row has. Without left(..., 64) on the term, Postgres answers SQLSTATE 54000,
// which is not a duplicate-key error, so the dedup branch would not fire and the
// contributor would get a 500.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_OversizedEventDate_IsStillIndexable() {
	user := suite.createUser("longdate", tierContributor, false)

	// The digits must be INCOMPRESSIBLE. Postgres TOAST-compresses an oversized
	// index value before measuring it, so a run of one repeated digit fits
	// comfortably and proves nothing; a pseudo-random run does not. Seeded, so the
	// fixture is the same on every run.
	digits := make([]byte, 3000)
	rng := rand.New(rand.NewSource(1977))
	for i := range digits {
		digits[i] = byte('0' + rng.Intn(10))
	}
	oversized := "2026-09-03T20:00:00." + string(digits) + "-07:00"

	_, parseErr := time.Parse(time.RFC3339, oversized)
	suite.Require().NoError(parseErr,
		"the fixture must still PARSE, or it is not the shape a legacy row has")

	stored, _, err := suite.service.CreateRequest(user, communitym.EntityRequestShow,
		suite.marshalShowOn("Long Night", oversized), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err, "the truncated term keeps an oversized event_date indexable")
	suite.Require().NotNil(stored)

	// Truncation is only about what the INDEX stores. The payload is untouched.
	fetched := suite.requireStored(stored.ID)
	suite.Require().NotNil(fetched.Payload)
	suite.Assert().Contains(string(*fetched.Payload), oversized,
		"the stored payload keeps the full value; only the key is truncated")
}

// The race the conditional UPDATE exists for: an admin decides the row between
// the duplicate lookup and the replacement. Driven directly against the writer,
// because the window between those two statements cannot be widened from the
// public API. A payload must never be resurrected onto a decided row.
func (suite *EntityRequestServiceIntegrationTestSuite) TestReplacePendingSubmission_DecidedRowIsNeverRewritten() {
	user := suite.createUser("race", tierContributor, false)
	admin := suite.createUser("race-admin", tierNewUser, true)

	queued, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Raced Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	_, err = suite.service.Decide(queued.ID, admin.ID, communitym.EntityRequestStateApproved, nil, nil)
	suite.Require().NoError(err)

	refreshed, err := suite.service.replacePendingSubmission(queued, communitym.EntityRequestArtist,
		suite.marshalArtist("Raced Band Corrected"), communitym.EntityRequestSourceManual, nil)
	suite.Require().NoError(err)
	suite.Assert().Nil(refreshed, "a decided row matches no pending update")

	stored, err := suite.service.GetRequest(queued.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(stored.Payload)
	suite.Assert().JSONEq(`{"name":"Raced Band"}`, string(*stored.Payload),
		"the approved row keeps the payload it was approved with")
	suite.Assert().Equal(communitym.EntityRequestStateApproved, stored.DecisionState)
}

// The optimistic claim: a decision validated against one payload must not land
// on a row the requester has since replaced. Without this the approve path's SSRF
// host guard is bypassable — it runs pre-claim and is deliberately NOT re-run at
// fulfillment, so a payload swapped in between is fulfilled unchecked.
func (suite *EntityRequestServiceIntegrationTestSuite) TestDecide_RefusesARowRevisedSinceTheCallersRead() {
	user := suite.createUser("revise", tierContributor, false)
	admin := suite.createUser("revise-admin", tierNewUser, true)

	reviewed, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Boris"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	// The admin's pre-claim read happens here — through the service, exactly as
	// AdminDecideEntityRequestHandler does it, so the version under test is the
	// stored one and not GORM's in-memory stamp (see requireStored). Taking it
	// off the create's return value would leave this passing for the wrong
	// reason: the claim would be refused by the nanosecond remainder rather than
	// by the replacement.
	reviewedVersion := suite.requireStored(reviewed.ID).UpdatedAt
	_, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("boris"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().True(replaced)

	_, err = suite.service.Decide(reviewed.ID, admin.ID,
		communitym.EntityRequestStateApproved, nil, &reviewedVersion)
	suite.Require().Error(err)
	var reqErr *apperrors.EntityRequestError
	suite.Require().ErrorAs(err, &reqErr)
	suite.Assert().Equal(apperrors.CodeEntityRequestStale, reqErr.Code,
		"a revised row is a distinct conflict from an already-decided one")

	stale, err := suite.service.GetRequest(reviewed.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal(communitym.EntityRequestStatePending, stale.DecisionState,
		"a refused claim leaves the row decidable once the admin re-reads")

	// Re-reading and deciding against the current version succeeds.
	decided, err := suite.service.Decide(reviewed.ID, admin.ID,
		communitym.EntityRequestStateApproved, nil, &stale.UpdatedAt)
	suite.Require().NoError(err)
	suite.Assert().Equal(communitym.EntityRequestStateApproved, decided.DecisionState)
}

// An unrevised row still claims cleanly when the caller passes the version it
// read: the guard must not have closed the ordinary approve path.
func (suite *EntityRequestServiceIntegrationTestSuite) TestDecide_UnrevisedRowClaimsWithItsVersion() {
	user := suite.createUser("unrevised", tierContributor, false)
	admin := suite.createUser("unrevised-admin", tierNewUser, true)

	queued, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Earth"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	read, err := suite.service.GetRequest(queued.ID)
	suite.Require().NoError(err)

	decided, err := suite.service.Decide(queued.ID, admin.ID,
		communitym.EntityRequestStateApproved, nil, &read.UpdatedAt)
	suite.Require().NoError(err)
	suite.Assert().Equal(communitym.EntityRequestStateApproved, decided.DecisionState)
}

// The overwrite is destructive and the superseded payload is unrecoverable, so
// the writer fails closed on an invalid payload rather than trusting that every
// caller validated first. Driven against the writer because the API boundary
// answers a bad payload with a 422 long before it gets here.
func (suite *EntityRequestServiceIntegrationTestSuite) TestReplacePendingSubmission_InvalidPayloadIsRefused() {
	user := suite.createUser("junk", tierContributor, false)

	queued, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Good Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	refreshed, err := suite.service.replacePendingSubmission(queued, communitym.EntityRequestArtist,
		[]byte(`{"name":""}`), communitym.EntityRequestSourceManual, nil)
	suite.Require().Error(err)
	suite.Assert().Nil(refreshed)

	stored, err := suite.service.GetRequest(queued.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(stored.Payload)
	suite.Assert().JSONEq(`{"name":"Good Band"}`, string(*stored.Payload),
		"a refused replacement must leave the queued payload untouched")
}

// Dedup is per-requester: two different users requesting the same name each get
// their own pending row.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_DuplicateAcrossRequesters_Separate() {
	u1 := suite.createUser("dup-a", tierContributor, false)
	u2 := suite.createUser("dup-b", tierContributor, false)

	r1, _, err := suite.service.CreateRequest(u1, communitym.EntityRequestArtist,
		suite.marshalArtist("Shared Name"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	r2, _, err := suite.service.CreateRequest(u2, communitym.EntityRequestArtist,
		suite.marshalArtist("Shared Name"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().NotEqual(r1.ID, r2.ID, "different requesters are not duplicates")
}

// The dedup key coalesces to the payload's TITLE for release/show (not name),
// so two release requests with the same title dedup.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_DuplicateReleaseByTitle() {
	user := suite.createUser("rel-dup", tierContributor, false)
	raw, err := communitym.MarshalPayload(communitym.ReleaseRequestPayload{Title: "Same Title"})
	suite.Require().NoError(err)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestRelease, raw,
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	// The corrected resubmission must replace on the TITLE branch of the dedup
	// key's coalesce, not just the NAME branch the artist tests cover.
	year := 2026
	corrected, err := communitym.MarshalPayload(communitym.ReleaseRequestPayload{
		Title:       "Same Title",
		ReleaseYear: &year,
	})
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestRelease, corrected,
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().Equal(first.ID, second.ID, "release dedup keys on title")
	suite.Assert().True(replaced, "a release resubmission corrects the queued row")
	suite.Require().NotNil(second.Payload)
	suite.Assert().JSONEq(string(corrected), string(*second.Payload))
}

// --- PSY-1008: source_detail persistence ------------------------------------

// source_detail round-trips through the JSONB column intact.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_SourceDetail_RoundTripsThroughDB() {
	user := suite.createUser("sd", tierNewUser, false)
	detail, err := json.Marshal(communitym.EntityRequestSourceDetail{
		URL:     strptr("https://example.com/article"),
		Excerpt: strptr("Boris announced a tour."),
	})
	suite.Require().NoError(err)

	created, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Sourced Band"), communitym.EntityRequestSourceAIExtraction, detail, false)
	suite.Require().NoError(err)

	fetched, err := suite.service.GetRequest(created.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(fetched.SourceDetail)

	var sd communitym.EntityRequestSourceDetail
	suite.Require().NoError(json.Unmarshal(*fetched.SourceDetail, &sd))
	suite.Require().NotNil(sd.URL)
	suite.Assert().Equal("https://example.com/article", *sd.URL)
	suite.Require().NotNil(sd.Excerpt)
	suite.Assert().Equal("Boris announced a tour.", *sd.Excerpt)
}

// No source_detail → the column is NULL (nil), not an empty object.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_NoSourceDetail_StoresNull() {
	user := suite.createUser("nosd", tierNewUser, false)
	created, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Plain Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	fetched, err := suite.service.GetRequest(created.ID)
	suite.Require().NoError(err)
	suite.Assert().Nil(fetched.SourceDetail)
}

// --- PSY-1008: RecordFulfillment --------------------------------------------

// RecordFulfillment persists created_entity_id onto the request row.
func (suite *EntityRequestServiceIntegrationTestSuite) TestRecordFulfillment_PersistsCreatedEntityID() {
	user := suite.createUser("rf", tierNewUser, false)
	created, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Fulfilled Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().Nil(created.CreatedEntityID)

	suite.Require().NoError(suite.service.RecordFulfillment(created.ID, 4242))

	fetched, err := suite.service.GetRequest(created.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(fetched.CreatedEntityID)
	suite.Assert().Equal(uint(4242), *fetched.CreatedEntityID)
}

// RecordFulfillment on a missing request is an error (no silent success).
func (suite *EntityRequestServiceIntegrationTestSuite) TestRecordFulfillment_NotFound() {
	err := suite.service.RecordFulfillment(999999, 1)
	suite.Require().Error(err)
}

// --- PSY-1088: rescue path for approved-but-unfulfilled rows -----------------

// newApprovedUnfulfilled seeds a row in the orphan state: decision_state =
// 'approved', created_entity_id IS NULL. An admin's CreateRequest auto-approves
// without fulfilling (the SERVICE never fulfills — only the handler does), so
// the row lands exactly in the rescue-target state.
func (suite *EntityRequestServiceIntegrationTestSuite) newApprovedUnfulfilled(name string) *communitym.EntityRequest {
	admin := suite.createUser("rescue-admin-"+name, tierNewUser, true)
	req, _, err := suite.service.CreateRequest(admin, communitym.EntityRequestArtist,
		suite.marshalArtist(name), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().Equal(communitym.EntityRequestStateApproved, req.DecisionState)
	suite.Require().Nil(req.CreatedEntityID, "fixture must be unfulfilled")
	return req
}

// The Unfulfilled filter narrows state=approved to created_entity_id IS NULL —
// the "needs attention" rescue queue. A fulfilled approved row is excluded.
func (suite *EntityRequestServiceIntegrationTestSuite) TestListRequests_UnfulfilledFilter() {
	orphan := suite.newApprovedUnfulfilled("Orphan Band")
	fulfilled := suite.newApprovedUnfulfilled("Fulfilled Band")
	// Mark the second one fulfilled so it drops out of the rescue queue.
	suite.Require().NoError(suite.service.RecordFulfillment(fulfilled.ID, 999))

	rows, total, err := suite.service.ListRequests(&contracts.EntityRequestFilters{
		State:       string(communitym.EntityRequestStateApproved),
		Unfulfilled: true,
	})
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total, "only the unfulfilled approved row is in the rescue queue")
	suite.Require().Len(rows, 1)
	suite.Assert().Equal(orphan.ID, rows[0].ID)
	suite.Assert().Nil(rows[0].CreatedEntityID)
}

// ClaimRescueFulfillment stamps created_entity_id on an approved-but-unfulfilled
// row and reports claimed=true.
func (suite *EntityRequestServiceIntegrationTestSuite) TestClaimRescueFulfillment_Claims() {
	orphan := suite.newApprovedUnfulfilled("Claim Me")

	claimed, err := suite.service.ClaimRescueFulfillment(orphan.ID, 321)
	suite.Require().NoError(err)
	suite.Assert().True(claimed)

	fetched, err := suite.service.GetRequest(orphan.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(fetched.CreatedEntityID)
	suite.Assert().Equal(uint(321), *fetched.CreatedEntityID)
}

// A second claim on an already-fulfilled row loses: claimed=false and the
// original created_entity_id is NOT overwritten. This is the sequential proxy
// for the concurrent double-fulfill race the conditional update guards against.
func (suite *EntityRequestServiceIntegrationTestSuite) TestClaimRescueFulfillment_SecondClaimLoses() {
	orphan := suite.newApprovedUnfulfilled("Contested Claim")

	first, err := suite.service.ClaimRescueFulfillment(orphan.ID, 100)
	suite.Require().NoError(err)
	suite.Require().True(first)

	second, err := suite.service.ClaimRescueFulfillment(orphan.ID, 200)
	suite.Require().NoError(err)
	suite.Assert().False(second, "a row that already has created_entity_id is not re-claimable")

	fetched, err := suite.service.GetRequest(orphan.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(fetched.CreatedEntityID)
	suite.Assert().Equal(uint(100), *fetched.CreatedEntityID, "first claim's id must survive")
}

// Claiming a PENDING row fails (claimed=false): only approved rows are
// rescuable, so a pending row is left untouched.
func (suite *EntityRequestServiceIntegrationTestSuite) TestClaimRescueFulfillment_PendingNotClaimable() {
	newbie := suite.createUser("claim-pending", tierNewUser, false)
	pending, _, err := suite.service.CreateRequest(newbie, communitym.EntityRequestArtist,
		suite.marshalArtist("Still Pending"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	claimed, err := suite.service.ClaimRescueFulfillment(pending.ID, 5)
	suite.Require().NoError(err)
	suite.Assert().False(claimed)

	fetched, err := suite.service.GetRequest(pending.ID)
	suite.Require().NoError(err)
	suite.Assert().Nil(fetched.CreatedEntityID)
}

// VoidApprovedUnfulfilled rejects an orphan and re-stamps the decider + note.
func (suite *EntityRequestServiceIntegrationTestSuite) TestVoidApprovedUnfulfilled_Rejects() {
	orphan := suite.newApprovedUnfulfilled("Void Me")
	admin := suite.createUser("voider", tierNewUser, true)

	note := "should not have been approved"
	voided, err := suite.service.VoidApprovedUnfulfilled(orphan.ID, admin.ID, &note)
	suite.Require().NoError(err)
	suite.Assert().True(voided)

	fetched, err := suite.service.GetRequest(orphan.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal(communitym.EntityRequestStateRejected, fetched.DecisionState)
	suite.Require().NotNil(fetched.DecidedBy)
	suite.Assert().Equal(admin.ID, *fetched.DecidedBy)
	suite.Require().NotNil(fetched.DecisionNote)
	suite.Assert().Equal(note, *fetched.DecisionNote)
}

// Voiding WITHOUT a note CLEARS any stale approval-time note: an orphan from
// the decide-approve path can carry an approval note, and leaving it on the
// now-rejected row would be misleading. Adversarial-review (Future-Maintainer).
func (suite *EntityRequestServiceIntegrationTestSuite) TestVoidApprovedUnfulfilled_ClearsStaleNote() {
	orphan := suite.newApprovedUnfulfilled("Stale Note Orphan")
	// Simulate the decide-approve path having stamped an approval note.
	staleNote := "approved — looks legit"
	suite.Require().NoError(suite.db.Model(&communitym.EntityRequest{}).
		Where("id = ?", orphan.ID).Update("decision_note", staleNote).Error)
	admin := suite.createUser("voider-clear", tierNewUser, true)

	// Void with NO note.
	voided, err := suite.service.VoidApprovedUnfulfilled(orphan.ID, admin.ID, nil)
	suite.Require().NoError(err)
	suite.Assert().True(voided)

	fetched, err := suite.service.GetRequest(orphan.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal(communitym.EntityRequestStateRejected, fetched.DecisionState)
	suite.Assert().Nil(fetched.DecisionNote, "void with no note must clear the stale approval note")
}

// A FULFILLED approved row can NOT be voided — voiding it would strand the
// already-created entity behind a rejected request.
func (suite *EntityRequestServiceIntegrationTestSuite) TestVoidApprovedUnfulfilled_FulfilledNotVoidable() {
	orphan := suite.newApprovedUnfulfilled("Already Fulfilled")
	suite.Require().NoError(suite.service.RecordFulfillment(orphan.ID, 555))
	admin := suite.createUser("voider2", tierNewUser, true)

	voided, err := suite.service.VoidApprovedUnfulfilled(orphan.ID, admin.ID, nil)
	suite.Require().NoError(err)
	suite.Assert().False(voided, "a fulfilled row is not voidable")

	fetched, err := suite.service.GetRequest(orphan.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal(communitym.EntityRequestStateApproved, fetched.DecisionState,
		"fulfilled row must stay approved")
	suite.Require().NotNil(fetched.CreatedEntityID)
}

// Voiding a PENDING row fails (only approved-but-unfulfilled rows are voidable
// via the rescue path; pending rows go through Decide).
func (suite *EntityRequestServiceIntegrationTestSuite) TestVoidApprovedUnfulfilled_PendingNotVoidable() {
	newbie := suite.createUser("void-pending", tierNewUser, false)
	pending, _, err := suite.service.CreateRequest(newbie, communitym.EntityRequestArtist,
		suite.marshalArtist("Pending Void"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	voided, err := suite.service.VoidApprovedUnfulfilled(pending.ID, 7, nil)
	suite.Require().NoError(err)
	suite.Assert().False(voided)

	fetched, err := suite.service.GetRequest(pending.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal(communitym.EntityRequestStatePending, fetched.DecisionState)
}

// strptr is a local pointer helper for the entity-request payload fixtures.
// (collection_test.go has strPtrCollection but it's collection-scoped; keep
// this one named for its own use site.)
func strptr(s string) *string { return &s }

func mustMarshalVenue(suite *EntityRequestServiceIntegrationTestSuite, name string) []byte {
	raw, err := communitym.MarshalPayload(communitym.VenueRequestPayload{Name: name, City: "Phoenix", State: "AZ"})
	suite.Require().NoError(err)
	return raw
}

func mustMarshalLabel(suite *EntityRequestServiceIntegrationTestSuite, name string) []byte {
	raw, err := communitym.MarshalPayload(communitym.LabelRequestPayload{Name: name})
	suite.Require().NoError(err)
	return raw
}

// PSY-1977: the two sides of the comparison are asymmetric, and only one of them
// may carry placeholders. The stored side reads the column; the candidate side
// interpolates the payload once per JSON key it reads.
//
// Asserting the candidate side's arity here would be tautological — dedupKeyArgs
// derives the count from the same expression — and that is the point: the arity
// bug this had (narrowing the coalesce to one key while both call sites still
// passed two args, which Postgres refuses with "could not determine data type of
// parameter $N") is now unrepresentable because nothing writes the count down.
// The statements themselves are exercised by the integration tests, which is the
// only place a binding error can actually surface.
//
// What IS worth asserting is the asymmetry, because it is an assumption both call
// sites make silently: they bind args for the CANDIDATE expression only.
// See TestDedupKeyStoredSideCarriesNoPlaceholder below.

// dedupIndexMigration is the migration that currently defines
// uq_entity_requests_pending_dedup. A migration that redefines that index must
// move this pointer, which is the same "edit them as a pair" discipline the
// index-vs-Go tests exist to enforce.
const dedupIndexMigration = "../../../db/migrations/20260902223753_entity_requests_dedup_per_type_occurrence.up.sql"

// PSY-1989: the migration's CASE and dedupOccurrenceIndexExpr must be the SAME
// expression, character for character modulo whitespace.
//
// TestDedupOccurrenceMatchesTheIndex compares against the index Postgres actually
// built, which is the stronger claim about deployment but a weaker one about
// structure: Postgres rewrites the expression (trim becomes btrim, casts appear),
// so that test compares only the literals and widths that survive verbatim.
// pg_indexes DOES render everything else, including lower() — it is this
// COMPARISON that cannot see it, not Postgres. Dropping the venue term's lower()
// would leave the query case-folding a term the index does not, which selects the
// wrong row rather than erroring, and that test would still pass.
//
// This closes that gap structurally and needs no database, so the pair's
// structural half fails in `go test` rather than only under an integration run.
// The two are complementary: this one cannot see whether the deployed database
// actually ran this migration, and that one can.
func TestDedupOccurrenceIndexExprMatchesTheMigration(t *testing.T) {
	sqlBytes, err := os.ReadFile(dedupIndexMigration)
	if err != nil {
		t.Fatalf("reading %s: %v (a migration that redefines the index must move dedupIndexMigration)", dedupIndexMigration, err)
	}
	sql := string(sqlBytes)

	start := strings.Index(sql, "CASE entity_type")
	end := strings.LastIndex(sql, "END")
	if start < 0 || end < start {
		t.Fatalf("%s no longer contains a CASE entity_type ... END block; it must, or this index is not built from the payload registry", dedupIndexMigration)
	}

	collapse := regexp.MustCompile(`\s+`)
	fromFile := collapse.ReplaceAllString(strings.TrimSpace(sql[start:end+len("END")]), " ")
	fromGo := collapse.ReplaceAllString(dedupOccurrenceIndexExpr(dedupStoredPayload), " ")

	if fromFile != fromGo {
		t.Errorf("the migration's occurrence expression and dedupOccurrenceIndexExpr differ.\n"+
			"migration: %s\nGo:        %s\n"+
			"They must be edited as a pair, in a NEW migration.", fromFile, fromGo)
	}
}

// The occurrence renderer interpolates a term's JSON keys and absent-sentinel
// into SQL as quoted literals, and it does not escape them. That is safe only
// because every one of those values is a package constant in the payload
// registry, never contributor input — the payload itself is always a bound
// parameter. This asserts the premise rather than leaving it to be assumed by
// whoever adds the seventh payload type.
func TestDedupOccurrenceLiteralsCannotBreakOutOfSQL(t *testing.T) {
	for _, entityType := range communitym.ValidEntityRequestTypes() {
		term := communitym.DedupOccurrenceTermFor(entityType)
		for _, literal := range append(append([]string{}, term.JSONKeys...), term.AbsentAs) {
			if strings.ContainsAny(literal, "'\\") {
				t.Errorf("%s declares occurrence literal %q, which would break out of the "+
					"quoted SQL the renderer builds", entityType, literal)
			}
		}
	}
}

func TestDedupKeyStoredSideCarriesNoPlaceholder(t *testing.T) {
	for _, entityType := range communitym.ValidEntityRequestTypes() {
		t.Run(entityType, func(t *testing.T) {
			storedName, storedOccurrence := dedupKeyExprs(dedupStoredPayload, entityType)
			for _, expr := range []string{storedName, storedOccurrence} {
				if strings.Contains(expr, "?") {
					t.Errorf("the stored-side expression must not carry a placeholder, "+
						"or every call site is binding one arg too few: %s", expr)
				}
			}

			candidateName, candidateOccurrence := dedupKeyExprs(dedupCandidatePayload, entityType)
			if !strings.Contains(candidateName, "?") {
				t.Errorf("the candidate-side name expression must carry a placeholder, "+
					"or the payload is being compared against nothing: %s", candidateName)
			}
			// The occurrence is the exception: a type declaring no term renders the
			// constant '', which reads no payload and so binds nothing. Both sides
			// render it, so the comparison is '' = '' and every such type keys on the
			// name alone, which is what those types declare.
			if len(communitym.DedupOccurrenceTermFor(entityType).JSONKeys) == 0 {
				if strings.Contains(candidateOccurrence, "?") {
					t.Errorf("a type with no occurrence term must read no payload: %s", candidateOccurrence)
				}
				return
			}
			if !strings.Contains(candidateOccurrence, "?") {
				t.Errorf("the candidate-side occurrence must carry a placeholder, "+
					"or the payload is being compared against nothing: %s", candidateOccurrence)
			}
		})
	}
}

// PSY-1977: the dedup key lives in THREE places that must agree — the payload
// registry's per-type declarations, dedupKeyExprs, and the
// uq_entity_requests_pending_dedup index. This reads the index back out of
// Postgres, so the pairing the Go code cannot enforce on its own is enforced
// here, against the schema the real migrations actually built.
//
// It matters most in the direction that is silent. An expression NARROWER than
// the index 500s loudly. An expression WIDER selects the lowest-id row sharing
// the wider key, which can be a different request — and before
// replacePendingSubmission re-asserted the key, that destroyed it.
func (suite *EntityRequestServiceIntegrationTestSuite) TestDedupOccurrenceMatchesTheIndex() {
	var indexDef string
	suite.Require().NoError(suite.db.Raw(
		"SELECT indexdef FROM pg_indexes WHERE indexname = 'uq_entity_requests_pending_dedup'",
	).Scan(&indexDef).Error)
	suite.Require().NotEmpty(indexDef, "the dedup index must exist; the migrations build it")

	declaredTypes := communitym.DedupOccurrenceTypes()
	suite.Require().NotEmpty(declaredTypes,
		"no payload type declares an occurrence term; the dedup key lost its occurrence half")

	// BOTH halves, not just the occurrence. The name term is the older and larger
	// half of the key and already carries two coexisting keys ('name', 'title'),
	// so it is the half where a widening edit or a reordered coalesce is most
	// likely — and a query wider than the index is the drift that picks the wrong
	// row rather than erroring.
	//
	// The name half takes no entity type; the occurrence half is the CASE the index
	// has to carry, which is what dedupOccurrenceIndexExpr renders.
	goExpr := dedupNameExpr(dedupStoredPayload) + " " + dedupOccurrenceIndexExpr(dedupStoredPayload)

	// Postgres rewrites the expression (trim becomes btrim, ::text casts appear,
	// CASE is re-spaced), so compare the SINGLE-QUOTED LITERALS it preserves
	// verbatim, in order. That sequence carries everything the key is made of:
	// which payload keys are read, which entity_type each branch answers to, each
	// branch's absent-sentinel, and the '' defaults. Anything that reorders,
	// renames, adds or drops one of those fails here.
	// The partial predicate is part of the index but not part of the key, so it is
	// split off and asserted on its own. That the index is PENDING-ONLY is what
	// keeps a decided row from ever being rewritten, so it is worth pinning rather
	// than dropping.
	keyDef := indexDef
	if i := strings.LastIndex(indexDef, " WHERE "); i >= 0 {
		keyDef = indexDef[:i]
		suite.Assert().Contains(indexDef[i:], "'"+string(communitym.EntityRequestStatePending)+"'",
			"the dedup index must stay partial on the pending state, or a decided request "+
				"can be replaced")
	}

	goLiterals := sqlQuotedLiterals(goExpr)
	indexLiterals := sqlQuotedLiterals(keyDef)
	suite.Assert().Equal(goLiterals, indexLiterals,
		"the Go dedup expression and uq_entity_requests_pending_dedup do not read the "+
			"same literals, or read them in a different order.\nGo:    %v\nindex: %v\n"+
			"They must be edited as a pair, in a NEW migration.", goLiterals, indexLiterals)

	// Every declared type must appear as a branch, stated separately from the
	// literal comparison so a registry entry the index never learned about names
	// itself in the failure.
	for _, entityType := range declaredTypes {
		suite.Assert().Contains(indexLiterals, entityType,
			"payload type %q declares an occurrence term but uq_entity_requests_pending_dedup "+
				"has no branch for it; add it in a NEW migration", entityType)
		for _, key := range communitym.DedupOccurrenceTermFor(entityType).JSONKeys {
			suite.Assert().Contains(indexLiterals, key,
				"payload type %q declares occurrence key %q but the index does not read it",
				entityType, key)
		}
	}

	// The truncation WIDTHS are part of the key too, and bare integers in three
	// files are exactly the kind of thing that drifts silently: an index narrower
	// than the query folds two distinct values into one bucket.
	goWidths := sqlIntegerArgs(goExpr)
	indexWidths := sqlIntegerArgs(keyDef)
	suite.Assert().Equal(goWidths, indexWidths,
		"the Go dedup expression and the index truncate at different widths, or in a "+
			"different order.\nGo:    %v\nindex: %v", goWidths, indexWidths)

	// Each INDEX-SAFETY width must equal the boundary cap on the value it keys, or
	// the index stores a prefix of a value the boundary accepted whole. A
	// festival's width is semantic (the four digits of a year), so it has no cap to
	// pair with and is not asserted here.
	suite.Assert().Equal(
		communitym.MaxRequestDateLen(),
		communitym.DedupOccurrenceTermFor(communitym.EntityRequestShow).Width,
		"a show's occurrence truncation and the API boundary's event_date cap must be equal")
	suite.Assert().Equal(
		communitym.MaxRequestVenueCityLen(),
		communitym.DedupOccurrenceTermFor(communitym.EntityRequestVenue).Width,
		"a venue's occurrence truncation and the API boundary's city cap must be equal")
}

// sqlQuotedLiterals pulls every single-quoted literal out of a SQL expression, in
// order. Postgres preserves these verbatim in pg_indexes.indexdef even as it
// rewrites everything around them, which is what makes them comparable against
// the Go-rendered expression.
func sqlQuotedLiterals(expr string) []string {
	var out []string
	for _, match := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(expr, -1) {
		out = append(out, match[1])
	}
	return out
}

// sqlIntegerArgs pulls every integer argument that closes a function call, in
// order — the left(..., N) truncation widths. Written to match on both sides:
// Postgres renders left as "left"(..., 64), which this reads the same way.
func sqlIntegerArgs(expr string) []string {
	var out []string
	for _, match := range regexp.MustCompile(`,\s*(\d+)\)`).FindAllStringSubmatch(expr, -1) {
		out = append(out, match[1])
	}
	return out
}

func TestEntityRequestServiceIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(EntityRequestServiceIntegrationTestSuite))
}
