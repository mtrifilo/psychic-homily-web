package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/testutil"
)

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
	// Clean up between tests
	sqlDB, _ := suite.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM request_votes")
	_, _ = sqlDB.Exec("DELETE FROM requests")
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

func (suite *RequestServiceIntegrationTestSuite) TestFulfillRequest_Success() {
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	err := suite.requestService.FulfillRequest(created.ID, fulfiller.ID, nil)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusFulfilled, request.Status)
	suite.Assert().NotNil(request.FulfillerID)
	suite.Assert().Equal(fulfiller.ID, *request.FulfillerID)
	suite.Assert().NotNil(request.FulfilledAt)
}

func (suite *RequestServiceIntegrationTestSuite) TestFulfillRequest_WithEntityID() {
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	entityID := uint(42)
	err := suite.requestService.FulfillRequest(created.ID, fulfiller.ID, &entityID)
	suite.Require().NoError(err)

	request, _ := suite.requestService.GetRequest(created.ID)
	suite.Assert().Equal(communitym.RequestStatusFulfilled, request.Status)
	suite.Assert().NotNil(request.RequestedEntityID)
	suite.Assert().Equal(uint(42), *request.RequestedEntityID)
}

func (suite *RequestServiceIntegrationTestSuite) TestFulfillRequest_AlreadyFulfilled() {
	user := suite.createTestUser("requester")
	fulfiller := suite.createTestUser("fulfiller")
	created, _ := suite.requestService.CreateRequest(user.ID, "To Fulfill", "desc", "artist", nil)

	_ = suite.requestService.FulfillRequest(created.ID, fulfiller.ID, nil)

	// Try to fulfill again
	err := suite.requestService.FulfillRequest(created.ID, fulfiller.ID, nil)
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
// Test Suite Runner
// ============================================================================

func TestRequestServiceIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(RequestServiceIntegrationTestSuite))
}
