package community

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
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

// --- Trust-tier gating -------------------------------------------------------

// new_user → queued for admin (pending), no decision stamped.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_NewUser_Pending() {
	user := suite.createUser("newbie", tierNewUser, false)

	req, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Pending Band"), communitym.EntityRequestSourceManual, false)
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

	req, err := suite.service.CreateRequest(user, communitym.EntityRequestVenue,
		mustMarshalVenue(suite, "The Pending Room"), communitym.EntityRequestSourcePasteMode, false)
	suite.Require().NoError(err)

	suite.Assert().Equal(communitym.EntityRequestStatePending, req.DecisionState)
	suite.Assert().Nil(req.DecidedBy)
}

// admin → auto-approved on create, decision stamped with the admin's own id.
// This is the AC's canonical "admin request → auto-approved" assertion.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_Admin_AutoApproved() {
	user := suite.createUser("boss", tierNewUser, true) // tier irrelevant; IsAdmin wins

	req, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Auto Approved Band"), communitym.EntityRequestSourceManual, false)
	suite.Require().NoError(err)

	suite.Assert().Equal(communitym.EntityRequestStateApproved, req.DecisionState)
	suite.Require().NotNil(req.DecidedBy)
	suite.Assert().Equal(user.ID, *req.DecidedBy)
	suite.Require().NotNil(req.DecidedAt)
}

// local_ambassador → auto-approved on create (highest non-admin trust).
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_LocalAmbassador_AutoApproved() {
	user := suite.createUser("amb", tierLocalAmbassador, false)

	req, err := suite.service.CreateRequest(user, communitym.EntityRequestLabel,
		mustMarshalLabel(suite, "Ambassador Records"), communitym.EntityRequestSourceManual, false)
	suite.Require().NoError(err)

	suite.Assert().Equal(communitym.EntityRequestStateApproved, req.DecisionState)
	suite.Require().NotNil(req.DecidedBy)
	suite.Assert().Equal(user.ID, *req.DecidedBy)
}

// trusted_contributor + confirmed → auto-approved (FE confirm step is the gate).
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_TrustedConfirmed_AutoApproved() {
	user := suite.createUser("trusted", tierTrustedContributor, false)

	req, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Trusted Confirmed Band"), communitym.EntityRequestSourceManual, true)
	suite.Require().NoError(err)

	suite.Assert().Equal(communitym.EntityRequestStateApproved, req.DecisionState)
	suite.Require().NotNil(req.DecidedBy)
}

// trusted_contributor + NOT confirmed → still queued (pending). The confirm
// step is what unlocks auto-approve for this tier.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_TrustedUnconfirmed_Pending() {
	user := suite.createUser("trusted2", tierTrustedContributor, false)

	req, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Trusted Unconfirmed Band"), communitym.EntityRequestSourceManual, false)
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

	created, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		raw, communitym.EntityRequestSourceManual, false)
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
	_, err := suite.service.CreateRequest(user, "podcast",
		suite.marshalArtist("X"), communitym.EntityRequestSourceManual, false)
	suite.Require().Error(err)
}

func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_InvalidSourceContext() {
	user := suite.createUser("bad2", tierNewUser, false)
	_, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("X"), "telepathy", false)
	suite.Require().Error(err)
}

func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_EmptyPayload() {
	user := suite.createUser("bad3", tierNewUser, false)
	_, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		nil, communitym.EntityRequestSourceManual, false)
	suite.Require().Error(err)
}

// --- Moderation decisions ----------------------------------------------------

// Admin approves a pending request → state flips, decider stamped.
func (suite *EntityRequestServiceIntegrationTestSuite) TestDecide_ApprovePending() {
	requester := suite.createUser("req", tierNewUser, false)
	admin := suite.createUser("mod", tierNewUser, true)

	pending, err := suite.service.CreateRequest(requester, communitym.EntityRequestArtist,
		suite.marshalArtist("Needs Review"), communitym.EntityRequestSourceManual, false)
	suite.Require().NoError(err)
	suite.Require().Equal(communitym.EntityRequestStatePending, pending.DecisionState)

	note := "looks good"
	decided, err := suite.service.Decide(pending.ID, admin.ID, communitym.EntityRequestStateApproved, &note)
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
	approved, err := suite.service.CreateRequest(admin, communitym.EntityRequestArtist,
		suite.marshalArtist("Already Approved"), communitym.EntityRequestSourceManual, false)
	suite.Require().NoError(err)
	suite.Require().Equal(communitym.EntityRequestStateApproved, approved.DecisionState)

	_, err = suite.service.Decide(approved.ID, admin.ID, communitym.EntityRequestStateRejected, nil)
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

	pending, err := suite.service.CreateRequest(requester, communitym.EntityRequestArtist,
		suite.marshalArtist("Contested"), communitym.EntityRequestSourceManual, false)
	suite.Require().NoError(err)

	// First decision wins: approve.
	decided, err := suite.service.Decide(pending.ID, admin.ID, communitym.EntityRequestStateApproved, nil)
	suite.Require().NoError(err)
	suite.Require().Equal(communitym.EntityRequestStateApproved, decided.DecisionState)

	// Second decision (reject) must fail and leave the row APPROVED.
	_, err = suite.service.Decide(pending.ID, admin.ID, communitym.EntityRequestStateRejected, nil)
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
	pending, err := suite.service.CreateRequest(requester, communitym.EntityRequestArtist,
		suite.marshalArtist("X"), communitym.EntityRequestSourceManual, false)
	suite.Require().NoError(err)

	_, err = suite.service.Decide(pending.ID, admin.ID, communitym.EntityRequestStatePending, nil)
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
	_, err := suite.service.CreateRequest(newbie, communitym.EntityRequestArtist,
		suite.marshalArtist("Pending A"), communitym.EntityRequestSourceManual, false)
	suite.Require().NoError(err)
	_, err = suite.service.CreateRequest(newbie, communitym.EntityRequestVenue,
		mustMarshalVenue(suite, "Pending V"), communitym.EntityRequestSourceManual, false)
	suite.Require().NoError(err)
	_, err = suite.service.CreateRequest(admin, communitym.EntityRequestArtist,
		suite.marshalArtist("Approved A"), communitym.EntityRequestSourceManual, false)
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

func TestEntityRequestServiceIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(EntityRequestServiceIntegrationTestSuite))
}
