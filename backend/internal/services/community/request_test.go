package community

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/testutil"
)

// createTestArtist seeds a minimal artist row and returns its ID. Used by
// PSY-748 fulfillment tests that need a real catalog entity for the
// type-match check to pass.
func createTestArtist(db *gorm.DB) uint {
	name := fmt.Sprintf("test-artist-%d", time.Now().UnixNano())
	artist := &catalogm.Artist{Name: name}
	if err := db.Create(artist).Error; err != nil {
		panic(fmt.Sprintf("seed artist failed: %v", err))
	}
	return artist.ID
}

// createTestVenue seeds a minimal venue row and returns its ID. Used by
// PSY-748 fulfillment tests to prove the type-mismatch path rejects
// when caller supplies the wrong table's row.
func createTestVenue(db *gorm.DB) uint {
	name := fmt.Sprintf("test-venue-%d", time.Now().UnixNano())
	venue := &catalogm.Venue{Name: name, City: "Phoenix", State: "AZ"}
	if err := db.Create(venue).Error; err != nil {
		panic(fmt.Sprintf("seed venue failed: %v", err))
	}
	return venue.ID
}

// createTestArtistWithSlug seeds an artist row carrying an explicit name +
// slug and returns both. PSY-917 ResolveEntityRef tests need a slug to
// assert the linkable-ref happy path (the plain createTestArtist helper
// leaves slug NULL, which exercises the suppress-link branch instead).
func createTestArtistWithSlug(db *gorm.DB) (id uint, name, slug string) {
	name = fmt.Sprintf("Slug Artist %d", time.Now().UnixNano())
	slug = fmt.Sprintf("slug-artist-%d", time.Now().UnixNano())
	artist := &catalogm.Artist{Name: name, Slug: &slug}
	if err := db.Create(artist).Error; err != nil {
		panic(fmt.Sprintf("seed artist-with-slug failed: %v", err))
	}
	return artist.ID, name, slug
}

// createTestReleaseWithSlug seeds a release row (title + slug) and returns
// both. Releases store their display name in `title`, not `name`, so this
// proves ResolveEntityRef reads the right column for that entity type.
func createTestReleaseWithSlug(db *gorm.DB) (id uint, title, slug string) {
	title = fmt.Sprintf("Slug Release %d", time.Now().UnixNano())
	slug = fmt.Sprintf("slug-release-%d", time.Now().UnixNano())
	release := &catalogm.Release{Title: title, Slug: &slug}
	if err := db.Create(release).Error; err != nil {
		panic(fmt.Sprintf("seed release-with-slug failed: %v", err))
	}
	return release.ID, title, slug
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type RequestServiceIntegrationTestSuite struct {
	suite.Suite
	testDB         *testutil.TestDatabase
	db             *gorm.DB
	requestService *RequestService
}

func (suite *RequestServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.db = suite.testDB.DB
	suite.requestService = NewRequestService(suite.testDB.DB)
}

func (suite *RequestServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *RequestServiceIntegrationTestSuite) SetupTest() {
	// Clean up between tests. Artists/venues live downstream of the
	// request fulfillment entity-type checks (PSY-748) so they must be
	// purged too — leaving them around would let the per-test ID
	// counters drift across runs and break the not-found assertions.
	sqlDB, _ := suite.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM request_votes")
	_, _ = sqlDB.Exec("DELETE FROM requests")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	// PSY-917: ResolveEntityRef tests seed releases; purge them too so IDs
	// don't drift across runs.
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM notification_log")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

// createTestUserForRequest creates a test user for request tests
func (suite *RequestServiceIntegrationTestSuite) createTestUser(name string) *authm.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	user := &authm.User{
		Email:         &email,
		FirstName:     &name,
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

// ============================================================================
// Test: CreateRequest
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestCreateRequest_Success() {
	user := suite.createTestUser("requester")

	request, err := suite.requestService.CreateRequest(user.ID, "Add Local Band XYZ", "They play shows frequently", "artist", nil)
	suite.Require().NoError(err)
	suite.Require().NotNil(request)

	suite.Assert().Equal("Add Local Band XYZ", request.Title)
	suite.Assert().NotNil(request.Description)
	suite.Assert().Equal("They play shows frequently", *request.Description)
	suite.Assert().Equal("artist", request.EntityType)
	suite.Assert().Equal(communitym.RequestStatusPending, request.Status)
	suite.Assert().Equal(user.ID, request.RequesterID)
	suite.Assert().Equal(0, request.VoteScore)
	suite.Assert().Equal(0, request.Upvotes)
	suite.Assert().Equal(0, request.Downvotes)
	suite.Assert().Nil(request.FulfillerID)
	suite.Assert().Nil(request.FulfilledAt)
	suite.Assert().NotZero(request.ID)
}

func (suite *RequestServiceIntegrationTestSuite) TestCreateRequest_WithEntityID() {
	user := suite.createTestUser("requester")

	entityID := uint(42)
	request, err := suite.requestService.CreateRequest(user.ID, "Update artist info", "Missing bio", "artist", &entityID)
	suite.Require().NoError(err)
	suite.Require().NotNil(request)

	suite.Assert().NotNil(request.RequestedEntityID)
	suite.Assert().Equal(uint(42), *request.RequestedEntityID)
}

func (suite *RequestServiceIntegrationTestSuite) TestCreateRequest_InvalidEntityType() {
	user := suite.createTestUser("requester")

	request, err := suite.requestService.CreateRequest(user.ID, "Test", "desc", "invalid_type", nil)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "invalid entity type")
	suite.Assert().Nil(request)
}

func (suite *RequestServiceIntegrationTestSuite) TestCreateRequest_EmptyDescription() {
	user := suite.createTestUser("requester")

	request, err := suite.requestService.CreateRequest(user.ID, "Test Request", "", "venue", nil)
	suite.Require().NoError(err)
	suite.Require().NotNil(request)

	// Empty description should be stored as nil
	suite.Assert().Nil(request.Description)
}

// ============================================================================
// Test: GetRequest
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestGetRequest_Success() {
	user := suite.createTestUser("requester")
	created, err := suite.requestService.CreateRequest(user.ID, "Test", "desc", "artist", nil)
	suite.Require().NoError(err)

	request, err := suite.requestService.GetRequest(created.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(request)

	suite.Assert().Equal(created.ID, request.ID)
	suite.Assert().Equal("Test", request.Title)
	// Requester should be preloaded
	suite.Assert().Equal(user.ID, request.Requester.ID)
}

func (suite *RequestServiceIntegrationTestSuite) TestGetRequest_NotFound() {
	request, err := suite.requestService.GetRequest(99999)
	suite.Assert().NoError(err)
	suite.Assert().Nil(request)
}

// ============================================================================
// Test: ListRequests
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestListRequests_All() {
	user := suite.createTestUser("requester")

	_, _ = suite.requestService.CreateRequest(user.ID, "Request 1", "", "artist", nil)
	_, _ = suite.requestService.CreateRequest(user.ID, "Request 2", "", "venue", nil)
	_, _ = suite.requestService.CreateRequest(user.ID, "Request 3", "", "release", nil)

	requests, total, err := suite.requestService.ListRequests("", "", "newest", 20, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(3), total)
	suite.Assert().Len(requests, 3)
}

func (suite *RequestServiceIntegrationTestSuite) TestListRequests_FilterByStatus() {
	user := suite.createTestUser("requester")

	r1, _ := suite.requestService.CreateRequest(user.ID, "Pending", "", "artist", nil)
	_, _ = suite.requestService.CreateRequest(user.ID, "Also Pending", "", "venue", nil)

	// Fulfill one request
	_ = suite.requestService.FulfillRequest(r1.ID, user.ID, nil)

	requests, total, err := suite.requestService.ListRequests("pending", "", "newest", 20, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total)
	suite.Assert().Len(requests, 1)
	suite.Assert().Equal("Also Pending", requests[0].Title)
}

func (suite *RequestServiceIntegrationTestSuite) TestListRequests_FilterByEntityType() {
	user := suite.createTestUser("requester")

	_, _ = suite.requestService.CreateRequest(user.ID, "Artist Request", "", "artist", nil)
	_, _ = suite.requestService.CreateRequest(user.ID, "Venue Request", "", "venue", nil)

	requests, total, err := suite.requestService.ListRequests("", "artist", "newest", 20, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total)
	suite.Assert().Len(requests, 1)
	suite.Assert().Equal("Artist Request", requests[0].Title)
}

func (suite *RequestServiceIntegrationTestSuite) TestListRequests_SortByVotes() {
	user := suite.createTestUser("requester")
	voter := suite.createTestUser("voter")

	r1, _ := suite.requestService.CreateRequest(user.ID, "Less Popular", "", "artist", nil)
	r2, _ := suite.requestService.CreateRequest(user.ID, "More Popular", "", "artist", nil)

	// Vote on r2
	_ = suite.requestService.Vote(r2.ID, voter.ID, true)
	// Downvote r1
	_ = suite.requestService.Vote(r1.ID, voter.ID, false)

	requests, _, err := suite.requestService.ListRequests("", "", "votes", 20, 0)
	suite.Require().NoError(err)
	suite.Assert().Len(requests, 2)
	// More popular should come first
	suite.Assert().Equal("More Popular", requests[0].Title)
	suite.Assert().Equal("Less Popular", requests[1].Title)
}

func (suite *RequestServiceIntegrationTestSuite) TestListRequests_Pagination() {
	user := suite.createTestUser("requester")

	for i := 0; i < 5; i++ {
		_, _ = suite.requestService.CreateRequest(user.ID, fmt.Sprintf("Request %d", i), "", "artist", nil)
	}

	requests, total, err := suite.requestService.ListRequests("", "", "newest", 2, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(5), total)
	suite.Assert().Len(requests, 2)

	// Second page
	requests2, total2, err := suite.requestService.ListRequests("", "", "newest", 2, 2)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(5), total2)
	suite.Assert().Len(requests2, 2)

	// Ensure different requests
	suite.Assert().NotEqual(requests[0].ID, requests2[0].ID)
}

// ============================================================================
// Test: UpdateRequest
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestUpdateRequest_Success() {
	user := suite.createTestUser("requester")
	created, _ := suite.requestService.CreateRequest(user.ID, "Original Title", "Original desc", "artist", nil)

	newTitle := "Updated Title"
	newDesc := "Updated description"
	updated, err := suite.requestService.UpdateRequest(created.ID, user.ID, &newTitle, &newDesc)
	suite.Require().NoError(err)
	suite.Require().NotNil(updated)

	suite.Assert().Equal("Updated Title", updated.Title)
	suite.Assert().NotNil(updated.Description)
	suite.Assert().Equal("Updated description", *updated.Description)
}

func (suite *RequestServiceIntegrationTestSuite) TestUpdateRequest_PartialUpdate() {
	user := suite.createTestUser("requester")
	created, _ := suite.requestService.CreateRequest(user.ID, "Original", "desc", "artist", nil)

	newTitle := "Updated Title Only"
	updated, err := suite.requestService.UpdateRequest(created.ID, user.ID, &newTitle, nil)
	suite.Require().NoError(err)
	suite.Assert().Equal("Updated Title Only", updated.Title)
}

func (suite *RequestServiceIntegrationTestSuite) TestUpdateRequest_NotOwner() {
	owner := suite.createTestUser("owner")
	other := suite.createTestUser("other")
	created, _ := suite.requestService.CreateRequest(owner.ID, "Test", "desc", "artist", nil)

	newTitle := "Hacked"
	_, err := suite.requestService.UpdateRequest(created.ID, other.ID, &newTitle, nil)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestForbidden, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestUpdateRequest_NotFound() {
	user := suite.createTestUser("requester")
	title := "Test"
	_, err := suite.requestService.UpdateRequest(99999, user.ID, &title, nil)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestNotFound, requestErr.Code)
}

// ============================================================================
// Test: DeleteRequest
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestDeleteRequest_ByOwner() {
	user := suite.createTestUser("requester")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Delete", "desc", "artist", nil)

	err := suite.requestService.DeleteRequest(created.ID, user.ID, false)
	suite.Assert().NoError(err)

	// Verify it's gone
	request, err := suite.requestService.GetRequest(created.ID)
	suite.Assert().NoError(err)
	suite.Assert().Nil(request)
}

func (suite *RequestServiceIntegrationTestSuite) TestDeleteRequest_ByAdmin() {
	user := suite.createTestUser("requester")
	admin := suite.createTestUser("admin")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Delete", "desc", "artist", nil)

	err := suite.requestService.DeleteRequest(created.ID, admin.ID, true)
	suite.Assert().NoError(err)
}

func (suite *RequestServiceIntegrationTestSuite) TestDeleteRequest_NotOwnerNotAdmin() {
	user := suite.createTestUser("requester")
	other := suite.createTestUser("other")
	created, _ := suite.requestService.CreateRequest(user.ID, "Test", "desc", "artist", nil)

	err := suite.requestService.DeleteRequest(created.ID, other.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestForbidden, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestDeleteRequest_NotFound() {
	user := suite.createTestUser("requester")
	err := suite.requestService.DeleteRequest(99999, user.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestNotFound, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestDeleteRequest_WithVotes() {
	user := suite.createTestUser("requester")
	voter := suite.createTestUser("voter")
	created, _ := suite.requestService.CreateRequest(user.ID, "With Votes", "desc", "artist", nil)

	// Add a vote
	_ = suite.requestService.Vote(created.ID, voter.ID, true)

	// Delete should succeed (votes are cleaned up)
	err := suite.requestService.DeleteRequest(created.ID, user.ID, false)
	suite.Assert().NoError(err)
}

// ============================================================================
// Test: Vote
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestVote_Upvote() {
	user := suite.createTestUser("requester")
	voter := suite.createTestUser("voter")
	created, _ := suite.requestService.CreateRequest(user.ID, "Test", "desc", "artist", nil)

	err := suite.requestService.Vote(created.ID, voter.ID, true)
	suite.Require().NoError(err)

	// Verify counts
	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(1, request.Upvotes)
	suite.Assert().Equal(0, request.Downvotes)
	suite.Assert().Equal(1, request.VoteScore)
}

func (suite *RequestServiceIntegrationTestSuite) TestVote_Downvote() {
	user := suite.createTestUser("requester")
	voter := suite.createTestUser("voter")
	created, _ := suite.requestService.CreateRequest(user.ID, "Test", "desc", "artist", nil)

	err := suite.requestService.Vote(created.ID, voter.ID, false)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(0, request.Upvotes)
	suite.Assert().Equal(1, request.Downvotes)
	suite.Assert().Equal(-1, request.VoteScore)
}

func (suite *RequestServiceIntegrationTestSuite) TestVote_ChangeVote() {
	user := suite.createTestUser("requester")
	voter := suite.createTestUser("voter")
	created, _ := suite.requestService.CreateRequest(user.ID, "Test", "desc", "artist", nil)

	// Upvote first
	_ = suite.requestService.Vote(created.ID, voter.ID, true)
	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(1, request.Upvotes)
	suite.Assert().Equal(0, request.Downvotes)

	// Change to downvote
	err := suite.requestService.Vote(created.ID, voter.ID, false)
	suite.Require().NoError(err)

	request, _ = suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(0, request.Upvotes)
	suite.Assert().Equal(1, request.Downvotes)
	suite.Assert().Equal(-1, request.VoteScore)
}

func (suite *RequestServiceIntegrationTestSuite) TestVote_MultipleVoters() {
	user := suite.createTestUser("requester")
	voter1 := suite.createTestUser("voter1")
	voter2 := suite.createTestUser("voter2")
	voter3 := suite.createTestUser("voter3")
	created, _ := suite.requestService.CreateRequest(user.ID, "Popular", "desc", "artist", nil)

	_ = suite.requestService.Vote(created.ID, voter1.ID, true)
	_ = suite.requestService.Vote(created.ID, voter2.ID, true)
	_ = suite.requestService.Vote(created.ID, voter3.ID, false)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(2, request.Upvotes)
	suite.Assert().Equal(1, request.Downvotes)
	suite.Assert().Equal(1, request.VoteScore)
}

func (suite *RequestServiceIntegrationTestSuite) TestVote_NotFound() {
	voter := suite.createTestUser("voter")
	err := suite.requestService.Vote(99999, voter.ID, true)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestNotFound, requestErr.Code)
}

// ============================================================================
// Test: RemoveVote
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestRemoveVote_Success() {
	user := suite.createTestUser("requester")
	voter := suite.createTestUser("voter")
	created, _ := suite.requestService.CreateRequest(user.ID, "Test", "desc", "artist", nil)

	// Vote first
	_ = suite.requestService.Vote(created.ID, voter.ID, true)

	// Remove vote
	err := suite.requestService.RemoveVote(created.ID, voter.ID)
	suite.Require().NoError(err)

	// Counts should be back to 0
	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(0, request.Upvotes)
	suite.Assert().Equal(0, request.Downvotes)
	suite.Assert().Equal(0, request.VoteScore)
}

func (suite *RequestServiceIntegrationTestSuite) TestRemoveVote_NoExistingVote() {
	user := suite.createTestUser("requester")
	voter := suite.createTestUser("voter")
	created, _ := suite.requestService.CreateRequest(user.ID, "Test", "desc", "artist", nil)

	// Should not error even if no vote exists
	err := suite.requestService.RemoveVote(created.ID, voter.ID)
	suite.Assert().NoError(err)
}

func (suite *RequestServiceIntegrationTestSuite) TestRemoveVote_NotFound() {
	voter := suite.createTestUser("voter")
	err := suite.requestService.RemoveVote(99999, voter.ID)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestNotFound, requestErr.Code)
}

// ============================================================================
// Test: FulfillRequest
// ============================================================================

// PSY-748: FulfillRequest is now a SUBMIT step that lands the request in
// pending_fulfillment. The two-step approve/reject flow below proves the
// transition to terminal "fulfilled" requires requester or admin approval.

func (suite *RequestServiceIntegrationTestSuite) TestFulfillRequest_SubmitsForApproval() {
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	err := suite.requestService.FulfillRequest(created.ID, fulfiller.ID, nil)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	// Pre-748 this would have been Fulfilled — the new gate moves it to
	// pending_fulfillment so the requester can confirm.
	suite.Assert().Equal(communitym.RequestStatusPendingFulfillment, request.Status)
	suite.Assert().NotNil(request.FulfillerID)
	suite.Assert().Equal(fulfiller.ID, *request.FulfillerID)
	// fulfilled_at is only stamped on approval, not submission.
	suite.Assert().Nil(request.FulfilledAt)
}

func (suite *RequestServiceIntegrationTestSuite) TestFulfillRequest_WithEntityID_TypeMatches() {
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	// Seed a real artist so the entity-existence + type-match check
	// passes — pre-748 the service blindly accepted any ID.
	artistID := createTestArtist(suite.db)
	err := suite.requestService.FulfillRequest(created.ID, fulfiller.ID, &artistID)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusPendingFulfillment, request.Status)
	suite.Assert().NotNil(request.RequestedEntityID)
	suite.Assert().Equal(artistID, *request.RequestedEntityID)
}

func (suite *RequestServiceIntegrationTestSuite) TestFulfillRequest_EntityTypeMismatch_Rejected() {
	// PSY-748: caller proposes a venue ID to fulfill an artist request.
	// Should reject with REQUEST_ENTITY_TYPE_MISMATCH and leave the
	// request unchanged (no row poisoning).
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(user.ID, "Artist Request", "desc", "artist", nil)

	// venueID exists in venues — but the request expects an artist.
	venueID := createTestVenue(suite.db)
	err := suite.requestService.FulfillRequest(created.ID, fulfiller.ID, &venueID)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Require().ErrorAs(err, &requestErr)
	// Mismatch surfaces as a missing artist with that ID — caller sees a
	// 400 from EntityNotFound. The exact code depends on whether there
	// happens to be an artist with the same numeric ID as the venue; the
	// happy path is the explicit ENTITY_NOT_FOUND code.
	suite.Assert().Contains([]string{
		apperrors.CodeRequestEntityNotFound,
		apperrors.CodeRequestEntityTypeMismatch,
	}, requestErr.Code)

	// Crucially: request state and fulfiller stayed pristine.
	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusPending, request.Status)
	suite.Assert().Nil(request.FulfillerID)
}

func (suite *RequestServiceIntegrationTestSuite) TestFulfillRequest_EntityNotFound_Rejected() {
	// PSY-748: caller proposes an artist ID that doesn't exist.
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(user.ID, "Artist Request", "desc", "artist", nil)

	nonExistentID := uint(999999)
	err := suite.requestService.FulfillRequest(created.ID, fulfiller.ID, &nonExistentID)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Require().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestEntityNotFound, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestFulfillRequest_AlreadyPendingFulfillment_Rejected() {
	// Once a request is in pending_fulfillment, a second submitter
	// can't overwrite the proposal — they'd have to wait for the
	// reject path.
	user := suite.createTestUser("requester")
	fulfiller1 := suite.createTestUser("fulfiller1")
	fulfiller2 := suite.createTestUser("fulfiller2")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	_ = suite.requestService.FulfillRequest(created.ID, fulfiller1.ID, nil)

	err := suite.requestService.FulfillRequest(created.ID, fulfiller2.ID, nil)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestAlreadyFulfilled, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestFulfillRequest_NotFound() {
	user := suite.createTestUser("fulfiller")
	err := suite.requestService.FulfillRequest(99999, user.ID, nil)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestNotFound, requestErr.Code)
}

// ============================================================================
// Test: ApproveFulfillment (PSY-748)
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestApproveFulfillment_ByRequester() {
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	// Submitter lands the request in pending_fulfillment.
	_ = suite.requestService.FulfillRequest(created.ID, fulfiller.ID, nil)

	// Requester approves → terminal fulfilled.
	err := suite.requestService.ApproveFulfillment(created.ID, user.ID, false)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusFulfilled, request.Status)
	suite.Assert().NotNil(request.FulfilledAt)
	// Fulfiller preserved across the approval.
	suite.Assert().NotNil(request.FulfillerID)
	suite.Assert().Equal(fulfiller.ID, *request.FulfillerID)
}

func (suite *RequestServiceIntegrationTestSuite) TestApproveFulfillment_ByAdmin() {
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	admin := suite.createTestUser("admin")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	_ = suite.requestService.FulfillRequest(created.ID, fulfiller.ID, nil)

	// Admin can approve on behalf of the requester (e.g. requester
	// went inactive).
	err := suite.requestService.ApproveFulfillment(created.ID, admin.ID, true)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusFulfilled, request.Status)
}

func (suite *RequestServiceIntegrationTestSuite) TestApproveFulfillment_NonRequesterNonAdmin_Forbidden() {
	// PSY-748 acceptance: a random user can't approve someone else's
	// pending fulfillment.
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	stranger := suite.createTestUser("stranger")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	_ = suite.requestService.FulfillRequest(created.ID, fulfiller.ID, nil)

	err := suite.requestService.ApproveFulfillment(created.ID, stranger.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Require().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestForbidden, requestErr.Code)

	// State must NOT have moved.
	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusPendingFulfillment, request.Status)
}

func (suite *RequestServiceIntegrationTestSuite) TestApproveFulfillment_FulfillerCannotSelfApprove() {
	// The same user who submitted the fulfillment may not also approve
	// it — that would defeat the whole point of the two-step gate. They
	// can only approve via the admin path or if they happen to also be
	// the requester (small corner case, but explicit so the model is
	// clear).
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	_ = suite.requestService.FulfillRequest(created.ID, fulfiller.ID, nil)

	err := suite.requestService.ApproveFulfillment(created.ID, fulfiller.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Require().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestForbidden, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestApproveFulfillment_WrongState() {
	// Can't approve a request that isn't actually in pending_fulfillment.
	user := suite.createTestUser("requester")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	err := suite.requestService.ApproveFulfillment(created.ID, user.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Require().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestInvalidState, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestApproveFulfillment_NotFound() {
	user := suite.createTestUser("requester")
	err := suite.requestService.ApproveFulfillment(99999, user.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestNotFound, requestErr.Code)
}

// ============================================================================
// Test: RejectFulfillment (PSY-748)
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestRejectFulfillment_ByRequester() {
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	artistID := createTestArtist(suite.db)
	_ = suite.requestService.FulfillRequest(created.ID, fulfiller.ID, &artistID)

	// Requester sees the proposal, decides it's wrong, rejects.
	err := suite.requestService.RejectFulfillment(created.ID, user.ID, false)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	// Back to pending — request is open for someone else to try again.
	suite.Assert().Equal(communitym.RequestStatusPending, request.Status)
	// Fulfiller + entity link cleared so the next submitter starts clean.
	suite.Assert().Nil(request.FulfillerID)
	suite.Assert().Nil(request.RequestedEntityID)
}

func (suite *RequestServiceIntegrationTestSuite) TestRejectFulfillment_ByAdmin() {
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	admin := suite.createTestUser("admin")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	_ = suite.requestService.FulfillRequest(created.ID, fulfiller.ID, nil)

	err := suite.requestService.RejectFulfillment(created.ID, admin.ID, true)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusPending, request.Status)
}

func (suite *RequestServiceIntegrationTestSuite) TestRejectFulfillment_NonRequesterNonAdmin_Forbidden() {
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	stranger := suite.createTestUser("stranger")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	_ = suite.requestService.FulfillRequest(created.ID, fulfiller.ID, nil)

	err := suite.requestService.RejectFulfillment(created.ID, stranger.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Require().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestForbidden, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestRejectFulfillment_WrongState() {
	user := suite.createTestUser("requester")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	err := suite.requestService.RejectFulfillment(created.ID, user.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Require().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestInvalidState, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestRejectFulfillment_NotFound() {
	user := suite.createTestUser("requester")
	err := suite.requestService.RejectFulfillment(99999, user.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestNotFound, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestRejectFulfillment_AllowsResubmission() {
	// Lifecycle round-trip: reject puts the request back into the
	// community pool, where another fulfiller can try again.
	user := suite.createTestUser("requester")
	fulfiller1 := suite.createTestUser("fulfiller1")
	fulfiller2 := suite.createTestUser("fulfiller2")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	_ = suite.requestService.FulfillRequest(created.ID, fulfiller1.ID, nil)
	_ = suite.requestService.RejectFulfillment(created.ID, user.ID, false)

	// Second fulfiller succeeds now.
	err := suite.requestService.FulfillRequest(created.ID, fulfiller2.ID, nil)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusPendingFulfillment, request.Status)
	suite.Assert().NotNil(request.FulfillerID)
	suite.Assert().Equal(fulfiller2.ID, *request.FulfillerID)
}

// ============================================================================
// Test: CloseRequest
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestCloseRequest_ByOwner() {
	user := suite.createTestUser("requester")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Close", "desc", "artist", nil)

	err := suite.requestService.CloseRequest(created.ID, user.ID, false)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusCancelled, request.Status)
}

func (suite *RequestServiceIntegrationTestSuite) TestCloseRequest_ByAdmin() {
	user := suite.createTestUser("requester")
	admin := suite.createTestUser("admin")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Reject", "desc", "artist", nil)

	err := suite.requestService.CloseRequest(created.ID, admin.ID, true)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusRejected, request.Status)
}

func (suite *RequestServiceIntegrationTestSuite) TestCloseRequest_AdminSelfClose() {
	// When admin closes their own request, it should be "cancelled" not "rejected"
	admin := suite.createTestUser("admin")
	created, _ := suite.requestService.CreateRequest(admin.ID, "My Request", "desc", "artist", nil)

	err := suite.requestService.CloseRequest(created.ID, admin.ID, true)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	// Owner closing their own = cancelled, even if admin
	suite.Assert().Equal(communitym.RequestStatusCancelled, request.Status)
}

func (suite *RequestServiceIntegrationTestSuite) TestCloseRequest_NotOwnerNotAdmin() {
	user := suite.createTestUser("requester")
	other := suite.createTestUser("other")
	created, _ := suite.requestService.CreateRequest(user.ID, "Test", "desc", "artist", nil)

	err := suite.requestService.CloseRequest(created.ID, other.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestForbidden, requestErr.Code)
}

func (suite *RequestServiceIntegrationTestSuite) TestCloseRequest_NotFound() {
	user := suite.createTestUser("requester")
	err := suite.requestService.CloseRequest(99999, user.ID, false)
	suite.Assert().Error(err)

	var requestErr *apperrors.RequestError
	suite.Assert().ErrorAs(err, &requestErr)
	suite.Assert().Equal(apperrors.CodeRequestNotFound, requestErr.Code)
}

// ============================================================================
// Test: GetUserVote
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestGetUserVote_Exists() {
	user := suite.createTestUser("requester")
	voter := suite.createTestUser("voter")
	created, _ := suite.requestService.CreateRequest(user.ID, "Test", "desc", "artist", nil)

	_ = suite.requestService.Vote(created.ID, voter.ID, true)

	vote, err := suite.requestService.GetUserVote(created.ID, voter.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(vote)
	suite.Assert().Equal(1, vote.Vote)
}

func (suite *RequestServiceIntegrationTestSuite) TestGetUserVote_NotVoted() {
	user := suite.createTestUser("requester")
	voter := suite.createTestUser("voter")
	created, _ := suite.requestService.CreateRequest(user.ID, "Test", "desc", "artist", nil)

	vote, err := suite.requestService.GetUserVote(created.ID, voter.ID)
	suite.Assert().NoError(err)
	suite.Assert().Nil(vote)
}

// ============================================================================
// Test: WilsonScore integration
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestWilsonScore_Computed() {
	user := suite.createTestUser("requester")
	voter1 := suite.createTestUser("voter1")
	voter2 := suite.createTestUser("voter2")
	created, _ := suite.requestService.CreateRequest(user.ID, "Popular", "desc", "artist", nil)

	_ = suite.requestService.Vote(created.ID, voter1.ID, true)
	_ = suite.requestService.Vote(created.ID, voter2.ID, true)

	request, _ := suite.requestService.GetRequest(created.ID)
	wilsonScore := request.WilsonScore()
	suite.Assert().Greater(wilsonScore, 0.0)
	suite.Assert().LessOrEqual(wilsonScore, 1.0)
}

// ============================================================================
// Test: NotifyRequesterFulfillmentProposed (PSY-890)
// ============================================================================

// countRequestFulfillmentNotifications returns how many in-app
// request_fulfillment_proposed notification_log rows exist for (userID, requestID).
func (suite *RequestServiceIntegrationTestSuite) countRequestFulfillmentNotifications(userID, requestID uint) int64 {
	var n int64
	err := suite.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND channel = ?",
			userID, notificationm.NotificationEntityRequestFulfillmentProposed, requestID,
			notificationm.NotificationChannelInApp).
		Count(&n).Error
	suite.Require().NoError(err)
	return n
}

func (suite *RequestServiceIntegrationTestSuite) TestNotifyRequesterFulfillmentProposed_WritesInAppRow() {
	requester := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(requester.ID, "Notify Me", "desc", "artist", nil)

	err := suite.requestService.NotifyRequesterFulfillmentProposed(created.ID, requester.ID, fulfiller.ID)
	suite.Require().NoError(err)

	suite.Assert().Equal(int64(1), suite.countRequestFulfillmentNotifications(requester.ID, created.ID),
		"requester should have one in-app fulfillment-proposed notification")
}

func (suite *RequestServiceIntegrationTestSuite) TestNotifyRequesterFulfillmentProposed_SkipsSelfFulfill() {
	requester := suite.createTestUser("requester")
	created, _ := suite.requestService.CreateRequest(requester.ID, "Self Fulfill", "desc", "artist", nil)

	// Fulfiller IS the requester — they already know, so no self-notify.
	err := suite.requestService.NotifyRequesterFulfillmentProposed(created.ID, requester.ID, requester.ID)
	suite.Require().NoError(err)

	suite.Assert().Equal(int64(0), suite.countRequestFulfillmentNotifications(requester.ID, created.ID),
		"self-fulfillment should not notify the requester")
}

func (suite *RequestServiceIntegrationTestSuite) TestNotifyRequesterFulfillmentProposed_SkipsZeroRequester() {
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(fulfiller.ID, "Unresolved Requester", "desc", "artist", nil)

	// requesterID 0 simulates the handler failing to resolve the request owner;
	// the notification must be skipped rather than writing a row for user 0.
	err := suite.requestService.NotifyRequesterFulfillmentProposed(created.ID, 0, fulfiller.ID)
	suite.Require().NoError(err)

	var n int64
	suite.Require().NoError(suite.db.Model(&notificationm.NotificationLog{}).
		Where("entity_type = ? AND entity_id = ?",
			notificationm.NotificationEntityRequestFulfillmentProposed, created.ID).
		Count(&n).Error)
	suite.Assert().Equal(int64(0), n, "zero requesterID should not write a notification row")
}

// ============================================================================
// Test: ResolveEntityRef (PSY-917)
// ============================================================================

func (suite *RequestServiceIntegrationTestSuite) TestResolveEntityRef_ArtistWithSlug() {
	artistID, name, slug := createTestArtistWithSlug(suite.db)

	ref, err := suite.requestService.ResolveEntityRef("artist", artistID)
	suite.Require().NoError(err)
	suite.Require().NotNil(ref)
	suite.Require().NotNil(ref.Slug)
	suite.Assert().Equal(slug, *ref.Slug)
	suite.Assert().Equal(name, ref.Name)
}

func (suite *RequestServiceIntegrationTestSuite) TestResolveEntityRef_ReleaseUsesTitleColumn() {
	// Releases store their display name in `title`, not `name`. Proves the
	// per-type name-column map resolves the right column.
	releaseID, title, slug := createTestReleaseWithSlug(suite.db)

	ref, err := suite.requestService.ResolveEntityRef("release", releaseID)
	suite.Require().NoError(err)
	suite.Require().NotNil(ref)
	suite.Require().NotNil(ref.Slug)
	suite.Assert().Equal(slug, *ref.Slug)
	suite.Assert().Equal(title, ref.Name)
}

func (suite *RequestServiceIntegrationTestSuite) TestResolveEntityRef_NullSlugSuppressesLink() {
	// createTestArtist seeds an artist with a NULL slug (no service-level
	// slug generation in the helper). The ref should still resolve with the
	// name but a nil Slug so the frontend omits the broken link.
	artistID := createTestArtist(suite.db)

	ref, err := suite.requestService.ResolveEntityRef("artist", artistID)
	suite.Require().NoError(err)
	suite.Require().NotNil(ref)
	suite.Assert().Nil(ref.Slug)
	suite.Assert().NotEmpty(ref.Name)
}

func (suite *RequestServiceIntegrationTestSuite) TestResolveEntityRef_MissingRowReturnsNil() {
	// A stale RequestedEntityID (entity since deleted) degrades to "no link"
	// rather than an error.
	ref, err := suite.requestService.ResolveEntityRef("artist", 999999)
	suite.Require().NoError(err)
	suite.Assert().Nil(ref)
}

func (suite *RequestServiceIntegrationTestSuite) TestResolveEntityRef_UnknownTypeReturnsNil() {
	// Defensive: an unsupported entity_type can't be resolved, but it must
	// not error — CreateRequest already gates the type.
	ref, err := suite.requestService.ResolveEntityRef("mixtape", 1)
	suite.Require().NoError(err)
	suite.Assert().Nil(ref)
}

// ============================================================================
// Test Suite Runner
// ============================================================================

func TestRequestServiceIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(RequestServiceIntegrationTestSuite))
}
