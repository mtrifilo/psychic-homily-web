package handlers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type SavedShowHandlerIntegrationSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *SavedShowHandler
}

func (s *SavedShowHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.handler = NewSavedShowHandler(s.deps.savedShowService)
}

func (s *SavedShowHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *SavedShowHandlerIntegrationSuite) TearDownSuite() {
	if s.deps.container != nil {
		s.deps.container.Terminate(s.deps.ctx)
	}
}

func TestSavedShowHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(SavedShowHandlerIntegrationSuite))
}

// --- SaveShowHandler ---

func (s *SavedShowHandlerIntegrationSuite) TestSaveShow_Success() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)
	req := &SaveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	resp, err := s.handler.SaveShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)
	s.Contains(resp.Body.Message, "saved")
}

func (s *SavedShowHandlerIntegrationSuite) TestSaveShow_AlreadySaved_Idempotent() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)
	req := &SaveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	// Save once
	_, err := s.handler.SaveShowHandler(ctx, req)
	s.NoError(err)

	// Save again — should succeed (service uses FirstOrCreate)
	resp, err := s.handler.SaveShowHandler(ctx, req)
	s.NoError(err)
	s.True(resp.Body.Success)
}

func (s *SavedShowHandlerIntegrationSuite) TestSaveShow_ShowNotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	req := &SaveShowRequest{ShowID: "99999"}

	_, err := s.handler.SaveShowHandler(ctx, req)
	s.Error(err)
}

// --- UnsaveShowHandler ---

func (s *SavedShowHandlerIntegrationSuite) TestUnsaveShow_Success() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)

	// Save first
	saveReq := &SaveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	_, err := s.handler.SaveShowHandler(ctx, saveReq)
	s.NoError(err)

	// Now unsave
	unsaveReq := &UnsaveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.UnsaveShowHandler(ctx, unsaveReq)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)
}

func (s *SavedShowHandlerIntegrationSuite) TestUnsaveShow_NotSaved() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)
	req := &UnsaveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	_, err := s.handler.UnsaveShowHandler(ctx, req)
	s.Error(err)
}

// --- GetSavedShowsHandler ---

func (s *SavedShowHandlerIntegrationSuite) TestGetSavedShows_Empty() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	req := &GetSavedShowsRequest{Limit: 50, Offset: 0}

	resp, err := s.handler.GetSavedShowsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
	s.Empty(resp.Body.Shows)
}

func (s *SavedShowHandlerIntegrationSuite) TestGetSavedShows_WithShows() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	// Create and save 3 shows
	for i := 0; i < 3; i++ {
		show := createApprovedShow(s.deps.db, user.ID, fmt.Sprintf("Show %d", i))
		saveReq := &SaveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
		_, err := s.handler.SaveShowHandler(ctx, saveReq)
		s.NoError(err)
	}

	req := &GetSavedShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetSavedShowsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(3), resp.Body.Total)
	s.Len(resp.Body.Shows, 3)
}

func (s *SavedShowHandlerIntegrationSuite) TestGetSavedShows_Pagination() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	// Create and save 3 shows
	for i := 0; i < 3; i++ {
		show := createApprovedShow(s.deps.db, user.ID, fmt.Sprintf("Show %d", i))
		saveReq := &SaveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
		_, err := s.handler.SaveShowHandler(ctx, saveReq)
		s.NoError(err)
	}

	// Get first page
	req := &GetSavedShowsRequest{Limit: 2, Offset: 0}
	resp, err := s.handler.GetSavedShowsHandler(ctx, req)
	s.NoError(err)
	s.Len(resp.Body.Shows, 2)
	s.Equal(int64(3), resp.Body.Total)

	// Get second page
	req2 := &GetSavedShowsRequest{Limit: 2, Offset: 2}
	resp2, err := s.handler.GetSavedShowsHandler(ctx, req2)
	s.NoError(err)
	s.Len(resp2.Body.Shows, 1)
	s.Equal(int64(3), resp2.Body.Total)
}

// --- CheckSavedHandler ---

func (s *SavedShowHandlerIntegrationSuite) TestCheckSaved_True() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")
	ctx := ctxWithUser(user)

	// Save it
	saveReq := &SaveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	_, err := s.handler.SaveShowHandler(ctx, saveReq)
	s.NoError(err)

	// Check
	checkReq := &CheckSavedRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.CheckSavedHandler(ctx, checkReq)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.IsSaved)
}

func (s *SavedShowHandlerIntegrationSuite) TestCheckSaved_False() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")
	ctx := ctxWithUser(user)

	checkReq := &CheckSavedRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.CheckSavedHandler(ctx, checkReq)
	s.NoError(err)
	s.NotNil(resp)
	s.False(resp.Body.IsSaved)
}

