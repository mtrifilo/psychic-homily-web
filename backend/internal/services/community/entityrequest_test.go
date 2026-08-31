package community

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
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

// A festival's occurrence term is start_date, which is required exactly as a
// show's event_date is (PSY-1977). Two editions of one festival share a name and
// must both stay queued.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_FestivalEditions_AreSeparateRequests() {
	user := suite.createUser("fest", tierContributor, false)

	first, _, err := suite.service.CreateRequest(user, communitym.EntityRequestFestival,
		suite.marshalFestival("Psycho Las Vegas", 2026, "2026-08-14", "2026-08-16"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	second, replaced, err := suite.service.CreateRequest(user, communitym.EntityRequestFestival,
		suite.marshalFestival("Psycho Las Vegas", 2027, "2027-08-13", "2027-08-15"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().False(replaced, "a different edition is a different festival request")
	suite.Assert().NotEqual(first.ID, second.ID)

	stored := suite.requireStored(first.ID)
	suite.Require().NotNil(stored.Payload)
	suite.Assert().JSONEq(string(suite.marshalFestival("Psycho Las Vegas", 2026, "2026-08-14", "2026-08-16")),
		string(*stored.Payload), "the 2026 edition survives untouched")
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

// PSY-1977: every occurrence key a payload type declares has to be READ by the
// dedup key's SQL. The payload interface already forces a new type to state
// whether it has an occurrence; this is the other half — a type that states one
// the expression does not read gets title-only dedup, which is the destructive
// collision this ticket removed, returning silently.
//
// A plain unit test: it needs no database, only the two declarations.
func TestDedupOccurrenceExprReadsEveryRegisteredKey(t *testing.T) {
	_, occurrence := dedupKeyExprs(dedupStoredPayload)

	declared := communitym.DedupOccurrenceJSONKeys()
	if len(declared) == 0 {
		t.Fatal("no payload type declares an occurrence key; the dedup key lost its date term")
	}
	for _, key := range declared {
		if !strings.Contains(occurrence, "'"+key+"'") {
			t.Errorf("payload registry declares occurrence key %q but the dedup expression does not read it: %s\n"+
				"add it to dedupKeyExprs AND to the uq_entity_requests_pending_dedup index, in the same order",
				key, occurrence)
		}
	}

	// The reverse direction: an expression reading a key no type declares is
	// either a typo or a leftover, and both silently split a dedup bucket.
	for _, match := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(occurrence, -1) {
		if !slices.Contains(declared, match[1]) {
			t.Errorf("the dedup expression reads %q, which no payload type declares as its occurrence key", match[1])
		}
	}
}

func TestEntityRequestServiceIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(EntityRequestServiceIntegrationTestSuite))
}
