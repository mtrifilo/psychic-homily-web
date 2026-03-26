package handlers

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

type CollectionHandlerIntegrationSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *CollectionHandler
}

func (s *CollectionHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.handler = NewCollectionHandler(s.deps.collectionService, s.deps.auditLogService)
}

func (s *CollectionHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *CollectionHandlerIntegrationSuite) TearDownSuite() {
	s.deps.testDB.Cleanup()
}

func TestCollectionHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(CollectionHandlerIntegrationSuite))
}

// --- Helpers ---

func (s *CollectionHandlerIntegrationSuite) createCollectionViaService(user *models.User, title string, isPublic bool) *contracts.CollectionDetailResponse {
	resp, err := s.deps.collectionService.CreateCollection(user.ID, &contracts.CreateCollectionRequest{
		Title:    title,
		IsPublic: isPublic,
	})
	s.Require().NoError(err)
	return resp
}

// ============================================================================
// CreateCollectionHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestCreateCollection_Success() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = "My Favorite Artists"
	req.Body.IsPublic = true

	resp, err := s.handler.CreateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("My Favorite Artists", resp.Body.Title)
	s.True(resp.Body.IsPublic)
	s.Equal(user.ID, resp.Body.CreatorID)
	s.NotEmpty(resp.Body.Slug)
}

func (s *CollectionHandlerIntegrationSuite) TestCreateCollection_WithDescription() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	desc := "A curated list of favorites"
	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = "Curated List"
	req.Body.Description = &desc
	req.Body.IsPublic = true
	req.Body.Collaborative = true

	resp, err := s.handler.CreateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Curated List", resp.Body.Title)
	s.Equal("A curated list of favorites", resp.Body.Description)
	s.True(resp.Body.Collaborative)
}

func (s *CollectionHandlerIntegrationSuite) TestCreateCollection_NoAuth() {
	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = "Unauthorized Collection"

	_, err := s.handler.CreateCollectionHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestCreateCollection_EmptyTitle() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = ""

	_, err := s.handler.CreateCollectionHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

// ============================================================================
// GetCollectionHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestGetCollection_BySlug() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Get By Slug", true)

	req := &GetCollectionHandlerRequest{Slug: coll.Slug}
	resp, err := s.handler.GetCollectionHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(coll.ID, resp.Body.ID)
	s.Equal("Get By Slug", resp.Body.Title)
}

func (s *CollectionHandlerIntegrationSuite) TestGetCollection_NotFound() {
	req := &GetCollectionHandlerRequest{Slug: "nonexistent-slug"}
	_, err := s.handler.GetCollectionHandler(context.Background(), req)
	assertHumaError(s.T(), err, 404)
}

func (s *CollectionHandlerIntegrationSuite) TestGetCollection_AuthenticatedViewerSeesSubscription() {
	owner := createTestUser(s.deps.db)
	viewer := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(owner, "Sub Check", true)

	// Subscribe the viewer
	err := s.deps.collectionService.Subscribe(coll.Slug, viewer.ID)
	s.Require().NoError(err)

	ctx := ctxWithUser(viewer)
	req := &GetCollectionHandlerRequest{Slug: coll.Slug}
	resp, err := s.handler.GetCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.IsSubscribed)
}

// ============================================================================
// GetCollectionStatsHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestGetCollectionStats_Success() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Stats Collection", true)

	// Add an artist item
	artist := createArtist(s.deps.db, "Stats Artist")
	_, err := s.deps.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
	})
	s.Require().NoError(err)

	req := &GetCollectionStatsHandlerRequest{Slug: coll.Slug}
	resp, err := s.handler.GetCollectionStatsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.ItemCount)
}

func (s *CollectionHandlerIntegrationSuite) TestGetCollectionStats_NotFound() {
	req := &GetCollectionStatsHandlerRequest{Slug: "nonexistent"}
	_, err := s.handler.GetCollectionStatsHandler(context.Background(), req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// ListCollectionsHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestListCollections_Success() {
	user := createTestUser(s.deps.db)
	s.createCollectionViaService(user, "List A", true)
	s.createCollectionViaService(user, "List B", true)
	s.createCollectionViaService(user, "List C", true)

	req := &ListCollectionsHandlerRequest{}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(3))
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_Empty() {
	req := &ListCollectionsHandlerRequest{}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_DefaultLimit() {
	user := createTestUser(s.deps.db)
	// Create more than 0 collections so we see results
	s.createCollectionViaService(user, "Default Limit A", true)

	req := &ListCollectionsHandlerRequest{} // Limit defaults to 20
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_WithLimit() {
	user := createTestUser(s.deps.db)
	s.createCollectionViaService(user, "Limited A", true)
	s.createCollectionViaService(user, "Limited B", true)
	s.createCollectionViaService(user, "Limited C", true)

	req := &ListCollectionsHandlerRequest{Limit: 2}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.LessOrEqual(len(resp.Body.Collections), 2)
	s.GreaterOrEqual(resp.Body.Total, int64(3))
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_OnlyPublic() {
	user := createTestUser(s.deps.db)
	s.createCollectionViaService(user, "Public One", true)
	s.createCollectionViaService(user, "Private One", false)

	req := &ListCollectionsHandlerRequest{}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	// Should only return public collections
	for _, c := range resp.Body.Collections {
		s.True(c.IsPublic, "expected only public collections in public listing")
	}
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_FeaturedFilter() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Featured Coll", true)
	s.createCollectionViaService(user, "Not Featured", true)

	// Set one as featured
	err := s.deps.collectionService.SetFeatured(coll.Slug, true)
	s.Require().NoError(err)

	req := &ListCollectionsHandlerRequest{Featured: 1}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(1), resp.Body.Total)
	s.Equal("Featured Coll", resp.Body.Collections[0].Title)
}

// ============================================================================
// UpdateCollectionHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_Success() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Original Title", true)

	ctx := ctxWithUser(user)
	newTitle := "Updated Title"
	req := &UpdateCollectionHandlerRequest{Slug: coll.Slug}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Title", resp.Body.Title)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_ChangeVisibility() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Visibility Test", true)

	ctx := ctxWithUser(user)
	isPublic := false
	req := &UpdateCollectionHandlerRequest{Slug: coll.Slug}
	req.Body.IsPublic = &isPublic

	resp, err := s.handler.UpdateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.False(resp.Body.IsPublic)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_NoAuth() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "No Auth Update", true)

	newTitle := "Hacked"
	req := &UpdateCollectionHandlerRequest{Slug: coll.Slug}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateCollectionHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_NotOwner() {
	owner := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(owner, "Not Mine", true)

	ctx := ctxWithUser(other)
	newTitle := "Hacked"
	req := &UpdateCollectionHandlerRequest{Slug: coll.Slug}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateCollectionHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_AdminCanUpdate() {
	owner := createTestUser(s.deps.db)
	admin := createAdminUser(s.deps.db)
	coll := s.createCollectionViaService(owner, "Admin Update", true)

	ctx := ctxWithUser(admin)
	newTitle := "Admin Updated"
	req := &UpdateCollectionHandlerRequest{Slug: coll.Slug}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Admin Updated", resp.Body.Title)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_NotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	newTitle := "Ghost"
	req := &UpdateCollectionHandlerRequest{Slug: "nonexistent-slug"}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateCollectionHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// DeleteCollectionHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestDeleteCollection_Success() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Deletable", true)

	ctx := ctxWithUser(user)
	req := &DeleteCollectionHandlerRequest{Slug: coll.Slug}
	_, err := s.handler.DeleteCollectionHandler(ctx, req)
	s.NoError(err)

	// Verify deleted
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	_, err = s.handler.GetCollectionHandler(context.Background(), getReq)
	assertHumaError(s.T(), err, 404)
}

func (s *CollectionHandlerIntegrationSuite) TestDeleteCollection_NoAuth() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "NoAuth Delete", true)

	req := &DeleteCollectionHandlerRequest{Slug: coll.Slug}
	_, err := s.handler.DeleteCollectionHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestDeleteCollection_NotOwner() {
	owner := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(owner, "Not My Delete", true)

	ctx := ctxWithUser(other)
	req := &DeleteCollectionHandlerRequest{Slug: coll.Slug}
	_, err := s.handler.DeleteCollectionHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestDeleteCollection_AdminCanDelete() {
	owner := createTestUser(s.deps.db)
	admin := createAdminUser(s.deps.db)
	coll := s.createCollectionViaService(owner, "Admin Deletable", true)

	ctx := ctxWithUser(admin)
	req := &DeleteCollectionHandlerRequest{Slug: coll.Slug}
	_, err := s.handler.DeleteCollectionHandler(ctx, req)
	s.NoError(err)

	// Verify deleted
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	_, err = s.handler.GetCollectionHandler(context.Background(), getReq)
	assertHumaError(s.T(), err, 404)
}

func (s *CollectionHandlerIntegrationSuite) TestDeleteCollection_NotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &DeleteCollectionHandlerRequest{Slug: "nonexistent-slug"}
	_, err := s.handler.DeleteCollectionHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// AddItemHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestAddItem_Success() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Add Item Coll", true)
	artist := createArtist(s.deps.db, "Item Artist")

	ctx := ctxWithUser(user)
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = artist.ID

	resp, err := s.handler.AddItemHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("artist", resp.Body.EntityType)
	s.Equal(artist.ID, resp.Body.EntityID)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_WithNotes() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Notes Coll", true)
	artist := createArtist(s.deps.db, "Notes Artist")

	ctx := ctxWithUser(user)
	notes := "Great live performances"
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = artist.ID
	req.Body.Notes = &notes

	resp, err := s.handler.AddItemHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body.Notes)
	s.Equal("Great live performances", *resp.Body.Notes)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_VenueEntity() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Venue Coll", true)
	venue := createVerifiedVenue(s.deps.db, "Item Venue", "Phoenix", "AZ")

	ctx := ctxWithUser(user)
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "venue"
	req.Body.EntityID = venue.ID

	resp, err := s.handler.AddItemHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("venue", resp.Body.EntityType)
	s.Equal(venue.ID, resp.Body.EntityID)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_NoAuth() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "NoAuth Item", true)

	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = 1

	_, err := s.handler.AddItemHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_MissingEntityType() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Missing Type", true)

	ctx := ctxWithUser(user)
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = ""
	req.Body.EntityID = 1

	_, err := s.handler.AddItemHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_MissingEntityID() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Missing ID", true)

	ctx := ctxWithUser(user)
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = 0

	_, err := s.handler.AddItemHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_NotOwner() {
	owner := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(owner, "Not My Add", true)
	artist := createArtist(s.deps.db, "Blocked Artist")

	ctx := ctxWithUser(other)
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = artist.ID

	_, err := s.handler.AddItemHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_CollectionNotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &AddItemHandlerRequest{Slug: "nonexistent"}
	req.Body.EntityType = "artist"
	req.Body.EntityID = 1

	_, err := s.handler.AddItemHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_DuplicateItem() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Dup Item", true)
	artist := createArtist(s.deps.db, "Dup Artist")

	ctx := ctxWithUser(user)

	// Add the item first
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = artist.ID
	_, err := s.handler.AddItemHandler(ctx, req)
	s.Require().NoError(err)

	// Try to add it again
	req2 := &AddItemHandlerRequest{Slug: coll.Slug}
	req2.Body.EntityType = "artist"
	req2.Body.EntityID = artist.ID
	_, err = s.handler.AddItemHandler(ctx, req2)
	assertHumaError(s.T(), err, 409)
}

// ============================================================================
// RemoveItemHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestRemoveItem_Success() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Remove Item", true)
	artist := createArtist(s.deps.db, "Removable Artist")

	// Add item via service
	item, err := s.deps.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
	})
	s.Require().NoError(err)

	ctx := ctxWithUser(user)
	req := &RemoveItemHandlerRequest{
		Slug:   coll.Slug,
		ItemID: fmt.Sprintf("%d", item.ID),
	}
	_, err = s.handler.RemoveItemHandler(ctx, req)
	s.NoError(err)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveItem_NoAuth() {
	req := &RemoveItemHandlerRequest{Slug: "some-slug", ItemID: "1"}
	_, err := s.handler.RemoveItemHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveItem_InvalidItemID() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &RemoveItemHandlerRequest{Slug: "some-slug", ItemID: "not-a-number"}
	_, err := s.handler.RemoveItemHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveItem_NotOwner() {
	owner := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(owner, "Not My Remove", true)
	artist := createArtist(s.deps.db, "Not My Artist")

	item, err := s.deps.collectionService.AddItem(coll.Slug, owner.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
	})
	s.Require().NoError(err)

	ctx := ctxWithUser(other)
	req := &RemoveItemHandlerRequest{
		Slug:   coll.Slug,
		ItemID: fmt.Sprintf("%d", item.ID),
	}
	_, err = s.handler.RemoveItemHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveItem_CollectionNotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &RemoveItemHandlerRequest{Slug: "nonexistent", ItemID: "1"}
	_, err := s.handler.RemoveItemHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// ReorderItemsHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestReorderItems_Success() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Reorder Coll", true)
	artist1 := createArtist(s.deps.db, "Reorder Artist 1")
	artist2 := createArtist(s.deps.db, "Reorder Artist 2")

	item1, err := s.deps.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist1.ID,
	})
	s.Require().NoError(err)

	item2, err := s.deps.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist2.ID,
	})
	s.Require().NoError(err)

	ctx := ctxWithUser(user)
	req := &ReorderItemsHandlerRequest{Slug: coll.Slug}
	req.Body.Items = []contracts.ReorderItem{
		{ItemID: item1.ID, Position: 2},
		{ItemID: item2.ID, Position: 1},
	}

	_, err = s.handler.ReorderItemsHandler(ctx, req)
	s.NoError(err)
}

func (s *CollectionHandlerIntegrationSuite) TestReorderItems_NoAuth() {
	req := &ReorderItemsHandlerRequest{Slug: "some-slug"}
	_, err := s.handler.ReorderItemsHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestReorderItems_NotOwner() {
	owner := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(owner, "Not My Reorder", true)

	ctx := ctxWithUser(other)
	req := &ReorderItemsHandlerRequest{Slug: coll.Slug}
	req.Body.Items = []contracts.ReorderItem{
		{ItemID: 1, Position: 1},
	}

	_, err := s.handler.ReorderItemsHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

// ============================================================================
// SubscribeHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestSubscribe_Success() {
	owner := createTestUser(s.deps.db)
	subscriber := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(owner, "Subscribable", true)

	ctx := ctxWithUser(subscriber)
	req := &SubscribeHandlerRequest{Slug: coll.Slug}
	_, err := s.handler.SubscribeHandler(ctx, req)
	s.NoError(err)

	// Verify subscription via get endpoint
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	getResp, err := s.handler.GetCollectionHandler(ctx, getReq)
	s.NoError(err)
	s.True(getResp.Body.IsSubscribed)
}

func (s *CollectionHandlerIntegrationSuite) TestSubscribe_NoAuth() {
	req := &SubscribeHandlerRequest{Slug: "some-slug"}
	_, err := s.handler.SubscribeHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestSubscribe_CollectionNotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &SubscribeHandlerRequest{Slug: "nonexistent"}
	_, err := s.handler.SubscribeHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// UnsubscribeHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestUnsubscribe_Success() {
	owner := createTestUser(s.deps.db)
	subscriber := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(owner, "Unsubscribable", true)

	// Subscribe first
	err := s.deps.collectionService.Subscribe(coll.Slug, subscriber.ID)
	s.Require().NoError(err)

	ctx := ctxWithUser(subscriber)
	req := &UnsubscribeHandlerRequest{Slug: coll.Slug}
	_, err = s.handler.UnsubscribeHandler(ctx, req)
	s.NoError(err)

	// Verify unsubscription
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	getResp, err := s.handler.GetCollectionHandler(ctx, getReq)
	s.NoError(err)
	s.False(getResp.Body.IsSubscribed)
}

func (s *CollectionHandlerIntegrationSuite) TestUnsubscribe_NoAuth() {
	req := &UnsubscribeHandlerRequest{Slug: "some-slug"}
	_, err := s.handler.UnsubscribeHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestUnsubscribe_CollectionNotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &UnsubscribeHandlerRequest{Slug: "nonexistent"}
	_, err := s.handler.UnsubscribeHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// SetFeaturedHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestSetFeatured_AdminSuccess() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Featureable", true)

	ctx := ctxWithUser(admin)
	req := &SetFeaturedHandlerRequest{Slug: coll.Slug}
	req.Body.Featured = true

	_, err := s.handler.SetFeaturedHandler(ctx, req)
	s.NoError(err)

	// Verify it's featured
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	getResp, err := s.handler.GetCollectionHandler(context.Background(), getReq)
	s.NoError(err)
	s.True(getResp.Body.IsFeatured)
}

func (s *CollectionHandlerIntegrationSuite) TestSetFeatured_Unfeature() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Unfeature Me", true)

	// Feature first
	err := s.deps.collectionService.SetFeatured(coll.Slug, true)
	s.Require().NoError(err)

	ctx := ctxWithUser(admin)
	req := &SetFeaturedHandlerRequest{Slug: coll.Slug}
	req.Body.Featured = false

	_, err = s.handler.SetFeaturedHandler(ctx, req)
	s.NoError(err)

	// Verify it's no longer featured
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	getResp, err := s.handler.GetCollectionHandler(context.Background(), getReq)
	s.NoError(err)
	s.False(getResp.Body.IsFeatured)
}

func (s *CollectionHandlerIntegrationSuite) TestSetFeatured_NonAdminForbidden() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Not Your Feature", true)

	ctx := ctxWithUser(user)
	req := &SetFeaturedHandlerRequest{Slug: coll.Slug}
	req.Body.Featured = true

	_, err := s.handler.SetFeaturedHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestSetFeatured_NoAuth() {
	req := &SetFeaturedHandlerRequest{Slug: "some-slug"}
	req.Body.Featured = true

	_, err := s.handler.SetFeaturedHandler(context.Background(), req)
	assertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestSetFeatured_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &SetFeaturedHandlerRequest{Slug: "nonexistent"}
	req.Body.Featured = true

	_, err := s.handler.SetFeaturedHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// GetUserCollectionsHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestGetUserCollections_Success() {
	user := createTestUser(s.deps.db)
	s.createCollectionViaService(user, "My Coll A", true)
	s.createCollectionViaService(user, "My Coll B", false)

	ctx := ctxWithUser(user)
	req := &GetUserCollectionsHandlerRequest{}
	resp, err := s.handler.GetUserCollectionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(2), resp.Body.Total)
}

func (s *CollectionHandlerIntegrationSuite) TestGetUserCollections_Empty() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &GetUserCollectionsHandlerRequest{}
	resp, err := s.handler.GetUserCollectionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *CollectionHandlerIntegrationSuite) TestGetUserCollections_NoAuth() {
	req := &GetUserCollectionsHandlerRequest{}
	_, err := s.handler.GetUserCollectionsHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestGetUserCollections_DoesNotIncludeOtherUsers() {
	user1 := createTestUser(s.deps.db)
	user2 := createTestUser(s.deps.db)
	s.createCollectionViaService(user1, "User1 Coll", true)
	s.createCollectionViaService(user2, "User2 Coll", true)

	ctx := ctxWithUser(user1)
	req := &GetUserCollectionsHandlerRequest{}
	resp, err := s.handler.GetUserCollectionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(1), resp.Body.Total)
	s.Equal("User1 Coll", resp.Body.Collections[0].Title)
}

func (s *CollectionHandlerIntegrationSuite) TestGetUserCollections_WithLimit() {
	user := createTestUser(s.deps.db)
	s.createCollectionViaService(user, "Limit A", true)
	s.createCollectionViaService(user, "Limit B", true)
	s.createCollectionViaService(user, "Limit C", true)

	ctx := ctxWithUser(user)
	req := &GetUserCollectionsHandlerRequest{Limit: 2}
	resp, err := s.handler.GetUserCollectionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.LessOrEqual(len(resp.Body.Collections), 2)
	s.Equal(int64(3), resp.Body.Total)
}

// ============================================================================
// mapCollectionError
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestMapCollectionError_NotFound() {
	err := mapCollectionError(fmt.Errorf("generic error"))
	s.Nil(err, "non-CollectionError should return nil")
}
