package community

import (
	apperrors "psychic-homily-backend/internal/errors"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// PSY-1992, Withdraw: the requester retracts their own PENDING request
//
// These run against a real Postgres through the shared suite, because every
// claim here is a claim about a conditional UPDATE and a partial index, and a
// mock would only restate the Go.
// =============================================================================

// The requester's own pending row moves to withdrawn, and the columns record
// that THEY ended it.
func (suite *EntityRequestServiceIntegrationTestSuite) TestWithdraw_OwnPendingRequest() {
	user := suite.createUser("withdrawer", tierContributor, false)

	created, _, err := suite.service.CreateRequest(
		user, "artist", suite.marshalArtist("Boris"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	withdrawn, err := suite.service.Withdraw(created.ID, user.ID)
	suite.Require().NoError(err)
	suite.Equal(communitym.EntityRequestStateWithdrawn, withdrawn.DecisionState)

	stored := suite.requireStored(created.ID)
	suite.Equal(communitym.EntityRequestStateWithdrawn, stored.DecisionState)
	suite.Require().NotNil(stored.DecidedBy)
	suite.Equal(user.ID, *stored.DecidedBy, "a withdrawal is decided by the requester")
	suite.NotNil(stored.DecidedAt)
	suite.Nil(stored.DecisionNote, "decision_note is the moderator's field")
	suite.NotNil(stored.Payload, "the row keeps what was requested")
}

// Someone else's row is not the caller's to retract, and the refusal says only
// that there is no such request FOR THEM. The row is untouched.
func (suite *EntityRequestServiceIntegrationTestSuite) TestWithdraw_AnotherUsersRequestIsNotFound() {
	owner := suite.createUser("owner", tierContributor, false)
	stranger := suite.createUser("stranger", tierContributor, false)

	created, _, err := suite.service.CreateRequest(
		owner, "artist", suite.marshalArtist("Earth"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	_, err = suite.service.Withdraw(created.ID, stranger.ID)
	suite.Require().Error(err)
	var reqErr *apperrors.EntityRequestError
	suite.Require().ErrorAs(err, &reqErr)
	suite.Equal(apperrors.CodeEntityRequestNotFound, reqErr.Code)

	stored := suite.requireStored(created.ID)
	suite.Equal(communitym.EntityRequestStatePending, stored.DecisionState,
		"a refused withdrawal writes nothing")
}

// A row that never existed answers exactly as another user's does, so the id
// space is not an oracle for who has requested what.
func (suite *EntityRequestServiceIntegrationTestSuite) TestWithdraw_MissingRequestIsNotFound() {
	user := suite.createUser("withdrawer", tierContributor, false)

	_, err := suite.service.Withdraw(999999, user.ID)
	suite.Require().Error(err)
	var reqErr *apperrors.EntityRequestError
	suite.Require().ErrorAs(err, &reqErr)
	suite.Equal(apperrors.CodeEntityRequestNotFound, reqErr.Code)
}

// The caller's OWN decided row refuses with the state it is in: it is theirs, so
// nothing is withheld, and the outcome is now the admin's.
func (suite *EntityRequestServiceIntegrationTestSuite) TestWithdraw_OwnDecidedRequestIsInvalidState() {
	user := suite.createUser("withdrawer", tierContributor, false)
	admin := suite.createUser("admin", tierLocalAmbassador, true)

	created, _, err := suite.service.CreateRequest(
		user, "artist", suite.marshalArtist("Sunn"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	_, err = suite.service.Decide(
		created.ID, admin.ID, communitym.EntityRequestStateRejected, nil, nil)
	suite.Require().NoError(err)

	_, err = suite.service.Withdraw(created.ID, user.ID)
	suite.Require().Error(err)
	var reqErr *apperrors.EntityRequestError
	suite.Require().ErrorAs(err, &reqErr)
	suite.Equal(apperrors.CodeEntityRequestInvalidState, reqErr.Code)

	stored := suite.requireStored(created.ID)
	suite.Equal(communitym.EntityRequestStateRejected, stored.DecisionState,
		"the admin's decision stands")
	suite.Equal(admin.ID, *stored.DecidedBy,
		"a refused withdrawal does not overwrite who decided")
}

// Withdrawing twice refuses the second time, naming the state the row is in.
func (suite *EntityRequestServiceIntegrationTestSuite) TestWithdraw_TwiceRefusesTheSecond() {
	user := suite.createUser("withdrawer", tierContributor, false)

	created, _, err := suite.service.CreateRequest(
		user, "artist", suite.marshalArtist("Melvins"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	_, err = suite.service.Withdraw(created.ID, user.ID)
	suite.Require().NoError(err)

	_, err = suite.service.Withdraw(created.ID, user.ID)
	suite.Require().Error(err)
	var reqErr *apperrors.EntityRequestError
	suite.Require().ErrorAs(err, &reqErr)
	suite.Equal(apperrors.CodeEntityRequestInvalidState, reqErr.Code)
}

// The dedup index is partial on 'pending', so a withdrawal FREES the key: the
// same name filed again opens a NEW row rather than replacing the withdrawn one.
// This is the whole difference between withdraw and a payload the requester
// overwrites with junk.
func (suite *EntityRequestServiceIntegrationTestSuite) TestWithdraw_FreesTheDedupKeyForARefiling() {
	user := suite.createUser("withdrawer", tierContributor, false)

	first, _, err := suite.service.CreateRequest(
		user, "artist", suite.marshalArtist("Neurosis"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	_, err = suite.service.Withdraw(first.ID, user.ID)
	suite.Require().NoError(err)

	second, superseded, err := suite.service.CreateRequest(
		user, "artist", suite.marshalArtist("Neurosis"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Nil(superseded, "a refiling after a withdrawal replaces nothing")
	suite.NotEqual(first.ID, second.ID, "the refiling is its own row")
	suite.Equal(communitym.EntityRequestStatePending, second.DecisionState)

	// The withdrawn row is still there, still withdrawn.
	suite.Equal(communitym.EntityRequestStateWithdrawn,
		suite.requireStored(first.ID).DecisionState)
}

// The moderation queue's default view excludes withdrawn rows, and asking for
// them by state returns them. Both halves of "filtered out by default, with a
// toggle" are one query's behaviour, so both are pinned here.
func (suite *EntityRequestServiceIntegrationTestSuite) TestWithdraw_QueueHidesThemByDefaultAndCanAskForThem() {
	user := suite.createUser("withdrawer", tierContributor, false)

	withdrawnReq, _, err := suite.service.CreateRequest(
		user, "artist", suite.marshalArtist("Godflesh"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	_, err = suite.service.Withdraw(withdrawnReq.ID, user.ID)
	suite.Require().NoError(err)

	stillPending, _, err := suite.service.CreateRequest(
		user, "artist", suite.marshalArtist("Swans"),
		communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	// Default: an empty state filter is the pending queue.
	rows, total, err := suite.service.ListRequests(&contracts.EntityRequestFilters{})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(rows, 1)
	suite.Equal(stillPending.ID, rows[0].ID)

	// The toggle: ask for withdrawn and the queue can still see them.
	rows, total, err = suite.service.ListRequests(&contracts.EntityRequestFilters{
		State: string(communitym.EntityRequestStateWithdrawn),
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(rows, 1)
	suite.Equal(withdrawnReq.ID, rows[0].ID)
}
