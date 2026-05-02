package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/services/contracts"
)

// Local alias so the helper isn't tied to the `services.` qualifier on every
// reference. PSY-356.
const MinPublicCollectionItems = services.MinPublicCollectionItems

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

// createCollectionViaService creates a collection. PSY-356 disallows creating
// public-at-create-time, so the helper always creates private. Tests that
// pass isPublic=true get the gate-passing dance applied (seed 3 items + 50+
// char description, then flip is_public). Tests that pass false get a bare
// private collection — most tests that only need a slug should call with
// false to keep item counts predictable.
func (s *CollectionHandlerIntegrationSuite) createCollectionViaService(user *models.User, title string, isPublic bool) *contracts.CollectionDetailResponse {
	priv, err := s.deps.collectionService.CreateCollection(user.ID, &contracts.CreateCollectionRequest{
		Title:    title,
		IsPublic: false,
	})
	s.Require().NoError(err)

	if !isPublic {
		return priv
	}

	// PSY-356 publish-gate dance: private → seed items + description → flip public.
	for i := 0; i < MinPublicCollectionItems; i++ {
		artist := createArtist(s.deps.db, fmt.Sprintf("%s seed %d-%d", title, i, time.Now().UnixNano()))
		_, err = s.deps.collectionService.AddItem(priv.Slug, user.ID, &contracts.AddCollectionItemRequest{
			EntityType: "artist",
			EntityID:   artist.ID,
		})
		s.Require().NoError(err)
	}

	desc := fmt.Sprintf("Quality-gate description for %s — long enough to satisfy the 50-char minimum.", title)
	pub := true
	resp, err := s.deps.collectionService.UpdateCollection(priv.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Description: &desc,
		IsPublic:    &pub,
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

	// PSY-356: created private — public-at-create is rejected by the gate.
	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = "My Favorite Artists"
	req.Body.IsPublic = false

	resp, err := s.handler.CreateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("My Favorite Artists", resp.Body.Title)
	s.False(resp.Body.IsPublic)
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
	req.Body.IsPublic = false // PSY-356: tests description persistence, not visibility.
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
	// Private — the test asserts a precise ItemCount=1 and the gate dance
	// would seed 3 extra items. Visibility is incidental here.
	coll := s.createCollectionViaService(user, "Stats Collection", false)

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

// PSY-352: sort=popular orders by HN gravity at the service layer; the
// handler's job is just to forward the value and reject unknowns.
func (s *CollectionHandlerIntegrationSuite) TestListCollections_PopularSort_Accepted() {
	user := createTestUser(s.deps.db)
	s.createCollectionViaService(user, "Popular Sort A", true)

	req := &ListCollectionsHandlerRequest{Sort: "popular"}
	_, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_UnknownSort_Rejected() {
	req := &ListCollectionsHandlerRequest{Sort: "bogus"}
	_, err := s.handler.ListCollectionsHandler(context.Background(), req)
	assertHumaError(s.T(), err, 400)
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
// CloneCollectionHandler (PSY-351)
// ============================================================================

// TestCloneCollection_CopiesItemsNotesAndPositions exercises the happy path:
// a public collection with items + notes + positions is copied faithfully
// into a new collection owned by the caller, with attribution back to the
// original. This is the primary acceptance criterion.
//
// PSY-356 note: createCollectionViaService(..., true) seeds 3 items as part
// of the publish-gate dance. Those land at positions 0..2, so the
// test-added items are at positions 3..5. The assertions index relative to
// the start of the test-added range.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_CopiesItemsNotesAndPositions() {
	owner := createTestUser(s.deps.db)
	cloner := createTestUser(s.deps.db)
	src := s.createCollectionViaService(owner, "Source Collection", true)

	// Add three items with notes; reorder to confirm position is preserved.
	a1 := createArtist(s.deps.db, "Artist One")
	a2 := createArtist(s.deps.db, "Artist Two")
	a3 := createArtist(s.deps.db, "Artist Three")
	notes1 := "first note"
	notes3 := "third note"
	_, err := s.deps.collectionService.AddItem(src.Slug, owner.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist", EntityID: a1.ID, Notes: &notes1,
	})
	s.Require().NoError(err)
	_, err = s.deps.collectionService.AddItem(src.Slug, owner.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist", EntityID: a2.ID,
	})
	s.Require().NoError(err)
	_, err = s.deps.collectionService.AddItem(src.Slug, owner.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist", EntityID: a3.ID, Notes: &notes3,
	})
	s.Require().NoError(err)

	ctx := ctxWithUser(cloner)
	req := &CloneCollectionHandlerRequest{Slug: src.Slug}
	resp, err := s.handler.CloneCollectionHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Body)

	// New collection is owned by the cloner, distinct from source.
	s.NotEqual(src.ID, resp.Body.ID)
	s.Equal(cloner.ID, resp.Body.CreatorID)
	s.Equal("Source Collection (fork)", resp.Body.Title)

	// Attribution back to original.
	s.Require().NotNil(resp.Body.ForkedFromCollectionID)
	s.Equal(src.ID, *resp.Body.ForkedFromCollectionID)
	s.Require().NotNil(resp.Body.ForkedFrom)
	s.Equal(src.ID, resp.Body.ForkedFrom.ID)
	s.Equal("Source Collection", resp.Body.ForkedFrom.Title)
	s.Equal(owner.ID, resp.Body.ForkedFrom.CreatorID)

	// Items copied: 3 gate-seeded + 3 explicit = 6 total. Index into the
	// test-added range (positions 3..5).
	s.Require().Len(resp.Body.Items, MinPublicCollectionItems+3)
	startIdx := MinPublicCollectionItems
	s.Equal(a1.ID, resp.Body.Items[startIdx].EntityID)
	s.Require().NotNil(resp.Body.Items[startIdx].Notes)
	s.Equal("first note", *resp.Body.Items[startIdx].Notes)
	s.Equal(a2.ID, resp.Body.Items[startIdx+1].EntityID)
	s.Nil(resp.Body.Items[startIdx+1].Notes)
	s.Equal(a3.ID, resp.Body.Items[startIdx+2].EntityID)
	s.Require().NotNil(resp.Body.Items[startIdx+2].Notes)
	s.Equal("third note", *resp.Body.Items[startIdx+2].Notes)

	// Positions are strictly increasing (matches source order).
	s.Less(resp.Body.Items[startIdx].Position, resp.Body.Items[startIdx+1].Position)
	s.Less(resp.Body.Items[startIdx+1].Position, resp.Body.Items[startIdx+2].Position)
}

// TestCloneCollection_NoAuth covers the authn boundary — the endpoint
// must reject anonymous callers.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_NoAuth() {
	owner := createTestUser(s.deps.db)
	src := s.createCollectionViaService(owner, "No Auth Clone", true)

	req := &CloneCollectionHandlerRequest{Slug: src.Slug}
	_, err := s.handler.CloneCollectionHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

// TestCloneCollection_PrivateSourceForbidden ensures the visibility check
// matches GetBySlug — non-owners cannot clone a private collection.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_PrivateSourceForbidden() {
	owner := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	private := s.createCollectionViaService(owner, "Private Source", false)

	ctx := ctxWithUser(other)
	req := &CloneCollectionHandlerRequest{Slug: private.Slug}
	_, err := s.handler.CloneCollectionHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

// TestCloneCollection_SourceNotFound ensures unknown slugs return 404.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_SourceNotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	req := &CloneCollectionHandlerRequest{Slug: "nope-not-real"}
	_, err := s.handler.CloneCollectionHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// TestCloneCollection_OwnerCanCloneOwnPrivate ensures the visibility check
// allows owners to clone their own private collections (matching GetBySlug
// — public OR owner). UI can still hide the button per the ticket.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_OwnerCanCloneOwnPrivate() {
	owner := createTestUser(s.deps.db)
	src := s.createCollectionViaService(owner, "Mine Private", false)

	ctx := ctxWithUser(owner)
	req := &CloneCollectionHandlerRequest{Slug: src.Slug}
	resp, err := s.handler.CloneCollectionHandler(ctx, req)
	s.Require().NoError(err)
	s.Equal(owner.ID, resp.Body.CreatorID)
	s.Require().NotNil(resp.Body.ForkedFromCollectionID)
	s.Equal(src.ID, *resp.Body.ForkedFromCollectionID)
}

// TestCloneCollection_DeletingOriginalSetsForkedFromNull verifies the
// ON DELETE SET NULL behavior on the new FK. Deleting the source must
// not cascade-delete forks; the cloned page should still load with the
// FK reset and ForkedFrom = nil so the frontend renders fallback copy.
// This is the explicit user requirement for the FK semantics.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_DeletingOriginalSetsForkedFromNull() {
	owner := createTestUser(s.deps.db)
	cloner := createTestUser(s.deps.db)
	src := s.createCollectionViaService(owner, "Doomed Source", true)

	// Clone first.
	ctx := ctxWithUser(cloner)
	cloneReq := &CloneCollectionHandlerRequest{Slug: src.Slug}
	cloneResp, err := s.handler.CloneCollectionHandler(ctx, cloneReq)
	s.Require().NoError(err)
	cloneSlug := cloneResp.Body.Slug

	// Delete the source.
	delErr := s.deps.collectionService.DeleteCollection(src.Slug, owner.ID, false)
	s.Require().NoError(delErr)

	// Clone still exists; ForkedFromCollectionID must be NULL post-cascade.
	getReq := &GetCollectionHandlerRequest{Slug: cloneSlug}
	getResp, err := s.handler.GetCollectionHandler(context.Background(), getReq)
	s.Require().NoError(err)
	s.Require().NotNil(getResp)
	s.Nil(getResp.Body.ForkedFromCollectionID,
		"ON DELETE SET NULL should clear the FK when the source is deleted")
	s.Nil(getResp.Body.ForkedFrom,
		"ForkedFrom should be nil so the frontend renders fallback copy")
}

// TestCloneCollection_OriginalShowsForksCount verifies the public fork
// count on the original collection. After two clones, the source's
// `forks_count` should be 2.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_OriginalShowsForksCount() {
	owner := createTestUser(s.deps.db)
	cloner1 := createTestUser(s.deps.db)
	cloner2 := createTestUser(s.deps.db)
	src := s.createCollectionViaService(owner, "Forky Source", true)

	// Two clones from different users.
	for _, c := range []*models.User{cloner1, cloner2} {
		ctx := ctxWithUser(c)
		req := &CloneCollectionHandlerRequest{Slug: src.Slug}
		_, err := s.handler.CloneCollectionHandler(ctx, req)
		s.Require().NoError(err)
	}

	// Reload the source via the public detail endpoint.
	getReq := &GetCollectionHandlerRequest{Slug: src.Slug}
	getResp, err := s.handler.GetCollectionHandler(context.Background(), getReq)
	s.Require().NoError(err)
	s.Equal(2, getResp.Body.ForksCount,
		"public forks_count should reflect clone count")
}

// ============================================================================
// PSY-356: publish-gate handler integration
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestCreateCollection_PublicAtCreateRejectedAs400() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = "Public At Create"
	req.Body.IsPublic = true

	_, err := s.handler.CreateCollectionHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_FlipPublicBelowGateRejectedAs400() {
	user := createTestUser(s.deps.db)
	priv := s.createCollectionViaService(user, "Flip Below Gate", false)

	ctx := ctxWithUser(user)
	pub := true
	req := &UpdateCollectionHandlerRequest{Slug: priv.Slug}
	req.Body.IsPublic = &pub

	_, err := s.handler.UpdateCollectionHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

// ============================================================================
// mapCollectionError
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestMapCollectionError_NotFound() {
	err := mapCollectionError(fmt.Errorf("generic error"))
	s.Nil(err, "non-CollectionError should return nil")
}

// ============================================================================
// PSY-354: collection tag endpoints
// ============================================================================

// promoteContributorForTags lifts a user above the new_user trust tier so the
// inline-tag-creation path passes (otherwise free-form tag names get a 403
// from the tag service's createTagInline gate). The trust-tier gate itself
// is covered in catalog/tag_service_test.go — these tests focus on the
// collection-side behavior.
func (s *CollectionHandlerIntegrationSuite) promoteContributorForTags(user *models.User) {
	s.Require().NoError(s.deps.db.Model(&models.User{}).
		Where("id = ?", user.ID).
		Update("user_tier", "contributor").Error)
}

func (s *CollectionHandlerIntegrationSuite) TestAddCollectionTag_Success() {
	user := createTestUser(s.deps.db)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Tagged Coll", false)

	ctx := ctxWithUser(user)
	req := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	req.Body.TagName = "first-tag"

	resp, err := s.handler.AddCollectionTagHandler(ctx, req)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Body.Tags, 1)
	s.Equal("first-tag", resp.Body.Tags[0].Name)
}

func (s *CollectionHandlerIntegrationSuite) TestAddCollectionTag_NoAuth() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Need Auth", false)

	req := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	req.Body.TagName = "no-auth"

	_, err := s.handler.AddCollectionTagHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestAddCollectionTag_MissingArgs() {
	user := createTestUser(s.deps.db)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Missing Args", false)

	ctx := ctxWithUser(user)
	req := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	// no tag_id, no tag_name

	_, err := s.handler.AddCollectionTagHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *CollectionHandlerIntegrationSuite) TestAddCollectionTag_NonOwner_Forbidden() {
	owner := createTestUser(s.deps.db)
	stranger := createTestUser(s.deps.db)
	s.promoteContributorForTags(stranger)
	coll := s.createCollectionViaService(owner, "Solo Owner", false)
	// createCollectionViaService leaves Collaborative=false (CreateCollection's
	// GORM-bool dance), so the stranger cannot tag the collection.

	ctx := ctxWithUser(stranger)
	req := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	req.Body.TagName = "stranger-tag"

	_, err := s.handler.AddCollectionTagHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestAddCollectionTag_LimitExceeded() {
	user := createTestUser(s.deps.db)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Capped Collection", false)
	ctx := ctxWithUser(user)

	for i := 0; i < contracts.MaxCollectionTags; i++ {
		r := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
		r.Body.TagName = fmt.Sprintf("cap-%d", i)
		_, err := s.handler.AddCollectionTagHandler(ctx, r)
		s.Require().NoError(err)
	}

	r := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	r.Body.TagName = "one-too-many"
	_, err := s.handler.AddCollectionTagHandler(ctx, r)
	assertHumaError(s.T(), err, 400)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveCollectionTag_Success() {
	user := createTestUser(s.deps.db)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Remove Tag", false)
	ctx := ctxWithUser(user)

	addReq := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	addReq.Body.TagName = "to-remove"
	addResp, err := s.handler.AddCollectionTagHandler(ctx, addReq)
	s.Require().NoError(err)
	s.Require().Len(addResp.Body.Tags, 1)
	tagID := addResp.Body.Tags[0].TagID

	delReq := &RemoveCollectionTagHandlerRequest{
		Slug:  coll.Slug,
		TagID: fmt.Sprintf("%d", tagID),
	}
	_, err = s.handler.RemoveCollectionTagHandler(ctx, delReq)
	s.NoError(err)

	// Verify it's gone via the detail endpoint.
	getResp, err := s.handler.GetCollectionHandler(ctx, &GetCollectionHandlerRequest{Slug: coll.Slug})
	s.Require().NoError(err)
	s.Empty(getResp.Body.Tags)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveCollectionTag_NoAuth() {
	user := createTestUser(s.deps.db)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Auth Needed Remove", false)
	ctx := ctxWithUser(user)

	addReq := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	addReq.Body.TagName = "tagged"
	addResp, err := s.handler.AddCollectionTagHandler(ctx, addReq)
	s.Require().NoError(err)
	tagID := addResp.Body.Tags[0].TagID

	delReq := &RemoveCollectionTagHandlerRequest{
		Slug:  coll.Slug,
		TagID: fmt.Sprintf("%d", tagID),
	}
	_, err = s.handler.RemoveCollectionTagHandler(context.Background(), delReq)
	assertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveCollectionTag_InvalidID() {
	user := createTestUser(s.deps.db)
	coll := s.createCollectionViaService(user, "Invalid ID", false)

	delReq := &RemoveCollectionTagHandlerRequest{
		Slug:  coll.Slug,
		TagID: "not-a-number",
	}
	_, err := s.handler.RemoveCollectionTagHandler(ctxWithUser(user), delReq)
	assertHumaError(s.T(), err, 400)
}

func (s *CollectionHandlerIntegrationSuite) TestGetCollection_SurfacesTags() {
	user := createTestUser(s.deps.db)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Surface Tags", false)
	ctx := ctxWithUser(user)

	addReq := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	addReq.Body.TagName = "surfaced"
	_, err := s.handler.AddCollectionTagHandler(ctx, addReq)
	s.Require().NoError(err)

	getResp, err := s.handler.GetCollectionHandler(ctx, &GetCollectionHandlerRequest{Slug: coll.Slug})
	s.Require().NoError(err)
	s.Require().Len(getResp.Body.Tags, 1)
	s.Equal("surfaced", getResp.Body.Tags[0].Name)
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_TagFilter() {
	user := createTestUser(s.deps.db)
	s.promoteContributorForTags(user)

	tagged := s.createCollectionViaService(user, "Tagged Browse", true)
	s.createCollectionViaService(user, "Untagged Browse", true)

	ctx := ctxWithUser(user)
	addReq := &AddCollectionTagHandlerRequest{Slug: tagged.Slug}
	addReq.Body.TagName = "browse-filter-tag"
	_, err := s.handler.AddCollectionTagHandler(ctx, addReq)
	s.Require().NoError(err)

	// Filter by the tag.
	listReq := &ListCollectionsHandlerRequest{Tag: "browse-filter-tag"}
	listResp, err := s.handler.ListCollectionsHandler(context.Background(), listReq)
	s.Require().NoError(err)
	s.Equal(int64(1), listResp.Body.Total)
	s.Require().Len(listResp.Body.Collections, 1)
	s.Equal(tagged.ID, listResp.Body.Collections[0].ID)

	// No filter — both surface.
	listReq = &ListCollectionsHandlerRequest{}
	listResp, err = s.handler.ListCollectionsHandler(context.Background(), listReq)
	s.Require().NoError(err)
	s.Equal(int64(2), listResp.Body.Total)
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_PopulatesTags() {
	user := createTestUser(s.deps.db)
	s.promoteContributorForTags(user)
	tagged := s.createCollectionViaService(user, "Browse Tags", true)

	ctx := ctxWithUser(user)
	addReq := &AddCollectionTagHandlerRequest{Slug: tagged.Slug}
	addReq.Body.TagName = "browse-chip"
	_, err := s.handler.AddCollectionTagHandler(ctx, addReq)
	s.Require().NoError(err)

	listResp, err := s.handler.ListCollectionsHandler(context.Background(), &ListCollectionsHandlerRequest{})
	s.Require().NoError(err)
	s.Require().Len(listResp.Body.Collections, 1)
	s.Require().Len(listResp.Body.Collections[0].Tags, 1)
	s.Equal("browse-chip", listResp.Body.Collections[0].Tags[0].Name)
}
