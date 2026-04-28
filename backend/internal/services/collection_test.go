package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type CollectionServiceIntegrationTestSuite struct {
	suite.Suite
	testDB            *testutil.TestDatabase
	db                *gorm.DB
	collectionService *CollectionService
}

func (suite *CollectionServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.collectionService = &CollectionService{db: suite.testDB.DB}
}

func (suite *CollectionServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *CollectionServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM collection_subscribers")
	_, _ = sqlDB.Exec("DELETE FROM collection_items")
	_, _ = sqlDB.Exec("DELETE FROM collections")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM release_labels")
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM artist_labels")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM festival_venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestCollectionServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CollectionServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) createTestUser(name string) *models.User {
	user := &models.User{
		Email:         strPtrCollection(fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())),
		FirstName:     strPtrCollection(name),
		LastName:      strPtrCollection("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *CollectionServiceIntegrationTestSuite) createTestUserWithUsername(name, username string) *models.User {
	user := &models.User{
		Email:         strPtrCollection(fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())),
		Username:      strPtrCollection(username),
		FirstName:     strPtrCollection(name),
		LastName:      strPtrCollection("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *CollectionServiceIntegrationTestSuite) createTestArtist(name string) *models.Artist {
	artist := &models.Artist{Name: name}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *CollectionServiceIntegrationTestSuite) createTestVenueForCollection(name string) *models.Venue {
	venue := &models.Venue{Name: name, City: "Phoenix", State: "AZ", Verified: true}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *CollectionServiceIntegrationTestSuite) createBasicCollection(user *models.User, title string) *contracts.CollectionDetailResponse {
	req := &contracts.CreateCollectionRequest{
		Title:    title,
		IsPublic: true,
	}
	resp, err := suite.collectionService.CreateCollection(user.ID, req)
	suite.Require().NoError(err)
	return resp
}

func strPtrCollection(s string) *string {
	return &s
}

func boolPtrCollection(b bool) *bool {
	return &b
}

// =============================================================================
// Group 1: CreateCollection
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_Success() {
	user := suite.createTestUser("Creator")

	desc := "My favorite artists"
	req := &contracts.CreateCollectionRequest{
		Title:         "Best Artists",
		Description:   &desc,
		Collaborative: true,
		IsPublic:      true,
	}

	resp, err := suite.collectionService.CreateCollection(user.ID, req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal("Best Artists", resp.Title)
	suite.Equal("best-artists", resp.Slug)
	suite.Equal("My favorite artists", resp.Description)
	suite.Equal(user.ID, resp.CreatorID)
	suite.True(resp.Collaborative)
	suite.True(resp.IsPublic)
	suite.False(resp.IsFeatured)
	suite.Equal(0, resp.ItemCount)
	suite.Equal(1, resp.SubscriberCount) // Creator auto-subscribed
	suite.True(resp.IsSubscribed)        // Viewer is creator
}

func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_Private() {
	user := suite.createTestUser("PrivateCreator")

	req := &contracts.CreateCollectionRequest{
		Title:    "Private List",
		IsPublic: false,
	}

	resp, err := suite.collectionService.CreateCollection(user.ID, req)

	suite.Require().NoError(err)
	suite.False(resp.IsPublic)
}

func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_UniqueSlug() {
	user := suite.createTestUser("SlugCreator")

	resp1 := suite.createBasicCollection(user, "My Collection")
	resp2 := suite.createBasicCollection(user, "My Collection")

	suite.NotEqual(resp1.Slug, resp2.Slug)
	suite.Equal("my-collection", resp1.Slug)
	suite.Equal("my-collection-2", resp2.Slug)
}

func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_CreatorNameResolution() {
	user := suite.createTestUserWithUsername("Alex", "alexrocks")
	resp := suite.createBasicCollection(user, "Name Test")
	suite.Equal("alexrocks", resp.CreatorName)
}

// =============================================================================
// Group 2: GetBySlug
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_Success() {
	user := suite.createTestUser("Getter")
	created := suite.createBasicCollection(user, "Get Test Collection")

	resp, err := suite.collectionService.GetBySlug(created.Slug, user.ID)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal("Get Test Collection", resp.Title)
	suite.True(resp.IsSubscribed) // Creator is subscribed
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_NotFound() {
	resp, err := suite.collectionService.GetBySlug("nonexistent-slug-xyz", 0)

	suite.Require().Error(err)
	suite.Nil(resp)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionNotFound, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_PrivateCollectionByCreator() {
	user := suite.createTestUser("PrivateViewer")
	req := &contracts.CreateCollectionRequest{Title: "Private Collection", IsPublic: false}
	created, err := suite.collectionService.CreateCollection(user.ID, req)
	suite.Require().NoError(err)

	resp, err := suite.collectionService.GetBySlug(created.Slug, user.ID)
	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_PrivateCollectionByOtherUser() {
	creator := suite.createTestUser("PrivateOwner")
	other := suite.createTestUser("OtherViewer")
	req := &contracts.CreateCollectionRequest{Title: "Secret Collection", IsPublic: false}
	created, err := suite.collectionService.CreateCollection(creator.ID, req)
	suite.Require().NoError(err)

	resp, err := suite.collectionService.GetBySlug(created.Slug, other.ID)
	suite.Require().Error(err)
	suite.Nil(resp)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_PublicCollectionByAnonymous() {
	user := suite.createTestUser("PubCreator")
	created := suite.createBasicCollection(user, "Public Collection")

	resp, err := suite.collectionService.GetBySlug(created.Slug, 0) // viewerID=0 = anonymous
	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.False(resp.IsSubscribed) // Anonymous can't be subscribed
}

// =============================================================================
// Group 3: ListCollections
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_All() {
	user := suite.createTestUser("Lister")
	suite.createBasicCollection(user, "Collection A")
	suite.createBasicCollection(user, "Collection B")

	resp, total, err := suite.collectionService.ListCollections(contracts.CollectionFilters{}, 20, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
}

func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_PublicOnly() {
	user := suite.createTestUser("MixedLister")
	suite.createBasicCollection(user, "Public One")

	privateReq := &contracts.CreateCollectionRequest{Title: "Private One", IsPublic: false}
	suite.collectionService.CreateCollection(user.ID, privateReq)

	resp, total, err := suite.collectionService.ListCollections(contracts.CollectionFilters{PublicOnly: true}, 20, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(resp, 1)
	suite.Equal("Public One", resp[0].Title)
}

func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_FilterByCreator() {
	user1 := suite.createTestUser("Creator1")
	user2 := suite.createTestUser("Creator2")
	suite.createBasicCollection(user1, "User1 Collection")
	suite.createBasicCollection(user2, "User2 Collection")

	resp, total, err := suite.collectionService.ListCollections(contracts.CollectionFilters{CreatorID: user1.ID}, 20, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(resp, 1)
	suite.Equal("User1 Collection", resp[0].Title)
}

func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_FilterBySearch() {
	user := suite.createTestUser("Searcher")
	suite.createBasicCollection(user, "Phoenix Punk Bands")
	suite.createBasicCollection(user, "Chicago Jazz Venues")

	resp, total, err := suite.collectionService.ListCollections(contracts.CollectionFilters{Search: "punk"}, 20, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(resp, 1)
	suite.Equal("Phoenix Punk Bands", resp[0].Title)
}

func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_FilterByFeatured() {
	user := suite.createTestUser("FeaturedLister")
	coll := suite.createBasicCollection(user, "Featured Collection")
	suite.createBasicCollection(user, "Normal Collection")

	suite.collectionService.SetFeatured(coll.Slug, true)

	resp, total, err := suite.collectionService.ListCollections(contracts.CollectionFilters{Featured: true}, 20, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(resp, 1)
	suite.Equal("Featured Collection", resp[0].Title)
}

func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_Pagination() {
	user := suite.createTestUser("Paginator")
	for i := 0; i < 5; i++ {
		suite.createBasicCollection(user, fmt.Sprintf("Collection %d", i))
	}

	resp, total, err := suite.collectionService.ListCollections(contracts.CollectionFilters{}, 2, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(resp, 2)
}

func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_DefaultLimit() {
	user := suite.createTestUser("DefaultLimit")
	suite.createBasicCollection(user, "Single")

	resp, _, err := suite.collectionService.ListCollections(contracts.CollectionFilters{}, 0, 0)

	suite.Require().NoError(err)
	suite.Len(resp, 1) // Default limit 20, but only 1 exists
}

// =============================================================================
// Group 4: UpdateCollection
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_BasicFields() {
	user := suite.createTestUser("Updater")
	created := suite.createBasicCollection(user, "Original Title")

	newTitle := "Updated Title"
	newDesc := "Updated description"
	resp, err := suite.collectionService.UpdateCollection(created.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Title:       &newTitle,
		Description: &newDesc,
	})

	suite.Require().NoError(err)
	suite.Equal("Updated Title", resp.Title)
	suite.Equal("Updated description", resp.Description)
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_TitleChangesSlug() {
	user := suite.createTestUser("SlugUpdater")
	created := suite.createBasicCollection(user, "Old Title")
	oldSlug := created.Slug

	newTitle := "Brand New Title"
	resp, err := suite.collectionService.UpdateCollection(created.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Title: &newTitle,
	})

	suite.Require().NoError(err)
	suite.Equal("Brand New Title", resp.Title)
	suite.NotEqual(oldSlug, resp.Slug)
	suite.Equal("brand-new-title", resp.Slug)
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_BoolFields() {
	user := suite.createTestUser("BoolUpdater")
	created := suite.createBasicCollection(user, "Bool Test")

	collab := false
	pub := false
	resp, err := suite.collectionService.UpdateCollection(created.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Collaborative: &collab,
		IsPublic:      &pub,
	})

	suite.Require().NoError(err)
	suite.False(resp.Collaborative)
	suite.False(resp.IsPublic)
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_NotFound() {
	newTitle := "Anything"
	resp, err := suite.collectionService.UpdateCollection("nonexistent-slug", 1, false, &contracts.UpdateCollectionRequest{
		Title: &newTitle,
	})

	suite.Require().Error(err)
	suite.Nil(resp)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionNotFound, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_Forbidden() {
	creator := suite.createTestUser("RealOwner")
	other := suite.createTestUser("Intruder")
	created := suite.createBasicCollection(creator, "Protected Collection")

	newTitle := "Hacked!"
	resp, err := suite.collectionService.UpdateCollection(created.Slug, other.ID, false, &contracts.UpdateCollectionRequest{
		Title: &newTitle,
	})

	suite.Require().Error(err)
	suite.Nil(resp)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_AdminCanUpdate() {
	creator := suite.createTestUser("AdminUpdateOwner")
	admin := suite.createTestUser("AdminUpdater")
	created := suite.createBasicCollection(creator, "Admin Editable")

	newTitle := "Admin Updated"
	resp, err := suite.collectionService.UpdateCollection(created.Slug, admin.ID, true, &contracts.UpdateCollectionRequest{
		Title: &newTitle,
	})

	suite.Require().NoError(err)
	suite.Equal("Admin Updated", resp.Title)
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_NoChanges() {
	user := suite.createTestUser("NoopUpdater")
	created := suite.createBasicCollection(user, "Stable Collection")

	resp, err := suite.collectionService.UpdateCollection(created.Slug, user.ID, false, &contracts.UpdateCollectionRequest{})

	suite.Require().NoError(err)
	suite.Equal("Stable Collection", resp.Title)
}

// PSY-349: description and per-item notes are stored as raw markdown but
// returned with rendered+sanitized HTML. The renderer reuses utils.MarkdownRenderer
// (goldmark + bluemonday) which is the same policy used by comments and field
// notes, so allowed tags and XSS safety match exactly.
func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_RendersDescriptionMarkdown() {
	user := suite.createTestUser("MarkdownAuthor")

	desc := "**bold** and *italic* and [link](https://example.com)"
	req := &contracts.CreateCollectionRequest{
		Title:       "Markdown Description",
		Description: &desc,
		IsPublic:    true,
	}

	resp, err := suite.collectionService.CreateCollection(user.ID, req)
	suite.Require().NoError(err)

	// Raw markdown is preserved on the response so the editor can re-populate it.
	suite.Equal(desc, resp.Description)
	// Rendered HTML is sanitized + populated.
	suite.Contains(resp.DescriptionHTML, "<strong>bold</strong>")
	suite.Contains(resp.DescriptionHTML, "<em>italic</em>")
	suite.Contains(resp.DescriptionHTML, `href="https://example.com"`)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_RendersDescriptionMarkdownOnEachRead() {
	user := suite.createTestUser("MarkdownReader")
	desc := "> a quote\n\n- bullet one"
	created, err := suite.collectionService.CreateCollection(user.ID, &contracts.CreateCollectionRequest{
		Title:       "Quoted",
		Description: &desc,
		IsPublic:    true,
	})
	suite.Require().NoError(err)

	resp, err := suite.collectionService.GetBySlug(created.Slug, user.ID)
	suite.Require().NoError(err)
	suite.Contains(resp.DescriptionHTML, "<blockquote>")
	suite.Contains(resp.DescriptionHTML, "<ul>")
	suite.Contains(resp.DescriptionHTML, "<li>")
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_DescriptionXSSStripped() {
	user := suite.createTestUser("XSSAuthor")
	desc := "Trust me <script>alert('pwn')</script>"
	created, err := suite.collectionService.CreateCollection(user.ID, &contracts.CreateCollectionRequest{
		Title:       "XSS Attempt",
		Description: &desc,
		IsPublic:    true,
	})
	suite.Require().NoError(err)

	resp, err := suite.collectionService.GetBySlug(created.Slug, user.ID)
	suite.Require().NoError(err)
	// Raw markdown is preserved (the editor will show what was typed); the
	// rendered HTML must strip <script> per the bluemonday policy.
	suite.NotContains(resp.DescriptionHTML, "<script>")
	suite.NotContains(resp.DescriptionHTML, "alert(")
}

func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_DescriptionTooLong() {
	user := suite.createTestUser("TooLong")

	longDesc := strings.Repeat("a", contracts.MaxCollectionDescriptionLength+1)
	resp, err := suite.collectionService.CreateCollection(user.ID, &contracts.CreateCollectionRequest{
		Title:       "Too Long",
		Description: &longDesc,
		IsPublic:    true,
	})

	suite.Require().Error(err)
	suite.Nil(resp)
	suite.Contains(err.Error(), "exceeds maximum length")
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_DescriptionTooLong() {
	user := suite.createTestUser("UpdaterTooLong")
	created := suite.createBasicCollection(user, "Will Reject Long Update")

	longDesc := strings.Repeat("b", contracts.MaxCollectionDescriptionLength+1)
	resp, err := suite.collectionService.UpdateCollection(created.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Description: &longDesc,
	})

	suite.Require().Error(err)
	suite.Nil(resp)
	suite.Contains(err.Error(), "exceeds maximum length")
}

// =============================================================================
// Group 5: DeleteCollection
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestDeleteCollection_Success() {
	user := suite.createTestUser("Deleter")
	created := suite.createBasicCollection(user, "Delete Me")

	err := suite.collectionService.DeleteCollection(created.Slug, user.ID, false)
	suite.Require().NoError(err)

	// Verify it's gone
	_, err = suite.collectionService.GetBySlug(created.Slug, user.ID)
	suite.Error(err)
}

func (suite *CollectionServiceIntegrationTestSuite) TestDeleteCollection_NotFound() {
	err := suite.collectionService.DeleteCollection("nonexistent-slug", 1, false)

	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionNotFound, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestDeleteCollection_Forbidden() {
	creator := suite.createTestUser("DeleteOwner")
	other := suite.createTestUser("DeleteIntruder")
	created := suite.createBasicCollection(creator, "Cannot Delete")

	err := suite.collectionService.DeleteCollection(created.Slug, other.ID, false)

	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestDeleteCollection_AdminCanDelete() {
	creator := suite.createTestUser("AdminDeleteOwner")
	admin := suite.createTestUser("AdminDeleter")
	created := suite.createBasicCollection(creator, "Admin Deletable")

	err := suite.collectionService.DeleteCollection(created.Slug, admin.ID, true)
	suite.Require().NoError(err)
}

func (suite *CollectionServiceIntegrationTestSuite) TestDeleteCollection_CascadesItemsAndSubscribers() {
	user := suite.createTestUser("CascadeDeleter")
	created := suite.createBasicCollection(user, "Cascade Delete")

	artist := suite.createTestArtist("Cascade Artist")
	suite.collectionService.AddItem(created.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist,
		EntityID:   artist.ID,
	})

	err := suite.collectionService.DeleteCollection(created.Slug, user.ID, false)
	suite.Require().NoError(err)

	// Verify items and subscribers are cleaned up
	var itemCount int64
	suite.db.Model(&models.CollectionItem{}).Where("collection_id = ?", created.ID).Count(&itemCount)
	suite.Equal(int64(0), itemCount)

	var subCount int64
	suite.db.Model(&models.CollectionSubscriber{}).Where("collection_id = ?", created.ID).Count(&subCount)
	suite.Equal(int64(0), subCount)
}

// =============================================================================
// Group 6: AddItem
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_Artist() {
	user := suite.createTestUser("ItemAdder")
	coll := suite.createBasicCollection(user, "Artist Collection")
	artist := suite.createTestArtist("Test Artist")

	resp, err := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist,
		EntityID:   artist.ID,
	})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal(models.CollectionEntityArtist, resp.EntityType)
	suite.Equal(artist.ID, resp.EntityID)
	suite.Equal("Test Artist", resp.EntityName)
	suite.Equal(0, resp.Position)
	suite.Equal(user.ID, resp.AddedByUserID)
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_Venue() {
	user := suite.createTestUser("VenueAdder")
	coll := suite.createBasicCollection(user, "Venue Collection")
	venue := suite.createTestVenueForCollection("The Rebel Lounge")

	resp, err := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityVenue,
		EntityID:   venue.ID,
	})

	suite.Require().NoError(err)
	suite.Equal("The Rebel Lounge", resp.EntityName)
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_AutoIncrementPosition() {
	user := suite.createTestUser("PositionAdder")
	coll := suite.createBasicCollection(user, "Ordered Collection")

	a1 := suite.createTestArtist("First")
	a2 := suite.createTestArtist("Second")
	a3 := suite.createTestArtist("Third")

	resp1, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a1.ID,
	})
	resp2, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a2.ID,
	})
	resp3, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a3.ID,
	})

	suite.Equal(0, resp1.Position)
	suite.Equal(1, resp2.Position)
	suite.Equal(2, resp3.Position)
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_WithNotes() {
	user := suite.createTestUser("NoteAdder")
	coll := suite.createBasicCollection(user, "Notes Collection")
	artist := suite.createTestArtist("Noted Artist")

	notes := "Saw them live, amazing set"
	resp, err := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist,
		EntityID:   artist.ID,
		Notes:      &notes,
	})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp.Notes)
	suite.Equal("Saw them live, amazing set", *resp.Notes)
	// Plain-text notes also pass through the renderer (plain text is valid markdown).
	suite.NotEmpty(resp.NotesHTML)
	suite.Contains(resp.NotesHTML, "Saw them live")
}

// PSY-349: per-item notes accept markdown and respond with sanitized HTML.
func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_RendersMarkdownNotes() {
	user := suite.createTestUser("MdNoteAdder")
	coll := suite.createBasicCollection(user, "MD Notes")
	artist := suite.createTestArtist("MD Artist")

	notes := "**must-see** band — see [their site](https://example.com)"
	resp, err := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist,
		EntityID:   artist.ID,
		Notes:      &notes,
	})

	suite.Require().NoError(err)
	suite.Contains(resp.NotesHTML, "<strong>must-see</strong>")
	suite.Contains(resp.NotesHTML, `href="https://example.com"`)
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_NotesXSSStripped() {
	user := suite.createTestUser("XSSNoteAdder")
	coll := suite.createBasicCollection(user, "XSS Notes")
	artist := suite.createTestArtist("XSS Artist")

	notes := "<script>alert('hax')</script>nice band"
	resp, err := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist,
		EntityID:   artist.ID,
		Notes:      &notes,
	})

	suite.Require().NoError(err)
	suite.NotContains(resp.NotesHTML, "<script>")
	suite.NotContains(resp.NotesHTML, "alert(")
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_NotesTooLong() {
	user := suite.createTestUser("LongNoteAdder")
	coll := suite.createBasicCollection(user, "Too Long Notes")
	artist := suite.createTestArtist("Long Artist")

	long := strings.Repeat("a", contracts.MaxCollectionItemNotesLength+1)
	_, err := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist,
		EntityID:   artist.ID,
		Notes:      &long,
	})

	suite.Require().Error(err)
	suite.Contains(err.Error(), "exceed maximum length")
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateItem_RendersMarkdownNotesAndEnforcesLimit() {
	user := suite.createTestUser("UpdateNoteAdder")
	coll := suite.createBasicCollection(user, "Update Notes")
	artist := suite.createTestArtist("Update Notes Artist")

	added, err := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist,
		EntityID:   artist.ID,
	})
	suite.Require().NoError(err)

	updatedNotes := "*italic update*"
	resp, err := suite.collectionService.UpdateItem(coll.Slug, added.ID, user.ID, false, &contracts.UpdateCollectionItemRequest{
		Notes: &updatedNotes,
	})
	suite.Require().NoError(err)
	suite.Contains(resp.NotesHTML, "<em>italic update</em>")

	// Length-limit enforcement on update.
	long := strings.Repeat("z", contracts.MaxCollectionItemNotesLength+1)
	_, err = suite.collectionService.UpdateItem(coll.Slug, added.ID, user.ID, false, &contracts.UpdateCollectionItemRequest{
		Notes: &long,
	})
	suite.Require().Error(err)
	suite.Contains(err.Error(), "exceed maximum length")
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_Duplicate() {
	user := suite.createTestUser("DupAdder")
	coll := suite.createBasicCollection(user, "Dup Collection")
	artist := suite.createTestArtist("Unique Artist")

	_, err := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: artist.ID,
	})
	suite.Require().NoError(err)

	_, err = suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: artist.ID,
	})
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionItemExists, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_CollaborativeByOtherUser() {
	creator := suite.createTestUser("CollabOwner")
	collaborator := suite.createTestUser("Collaborator")

	req := &contracts.CreateCollectionRequest{Title: "Collab Collection", IsPublic: true, Collaborative: true}
	coll, err := suite.collectionService.CreateCollection(creator.ID, req)
	suite.Require().NoError(err)

	artist := suite.createTestArtist("Collab Artist")
	resp, err := suite.collectionService.AddItem(coll.Slug, collaborator.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: artist.ID,
	})

	suite.Require().NoError(err)
	suite.Equal(collaborator.ID, resp.AddedByUserID)
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_NonCollaborativeForbidden() {
	creator := suite.createTestUser("SoloOwner")
	other := suite.createTestUser("Outsider")

	req := &contracts.CreateCollectionRequest{Title: "Solo Collection", IsPublic: true, Collaborative: false}
	coll, err := suite.collectionService.CreateCollection(creator.ID, req)
	suite.Require().NoError(err)

	artist := suite.createTestArtist("Blocked Artist")
	resp, err := suite.collectionService.AddItem(coll.Slug, other.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: artist.ID,
	})

	suite.Require().Error(err)
	suite.Nil(resp)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_CollectionNotFound() {
	resp, err := suite.collectionService.AddItem("nonexistent-slug", 1, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: 1,
	})

	suite.Require().Error(err)
	suite.Nil(resp)
}

// =============================================================================
// Group 7: RemoveItem
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestRemoveItem_ByCreator() {
	user := suite.createTestUser("Remover")
	coll := suite.createBasicCollection(user, "Remove Collection")
	artist := suite.createTestArtist("Removable Artist")

	item, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: artist.ID,
	})

	err := suite.collectionService.RemoveItem(coll.Slug, item.ID, user.ID, false)
	suite.Require().NoError(err)

	// Verify removal
	detail, err := suite.collectionService.GetBySlug(coll.Slug, user.ID)
	suite.Require().NoError(err)
	suite.Equal(0, detail.ItemCount)
}

func (suite *CollectionServiceIntegrationTestSuite) TestRemoveItem_ByItemAdder() {
	creator := suite.createTestUser("RemoveOwner")
	adder := suite.createTestUser("ItemAdderRemover")

	req := &contracts.CreateCollectionRequest{Title: "Collab Remove", IsPublic: true, Collaborative: true}
	coll, _ := suite.collectionService.CreateCollection(creator.ID, req)

	artist := suite.createTestArtist("Adder Artist")
	item, _ := suite.collectionService.AddItem(coll.Slug, adder.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: artist.ID,
	})

	// The adder should be able to remove their own item
	err := suite.collectionService.RemoveItem(coll.Slug, item.ID, adder.ID, false)
	suite.Require().NoError(err)
}

func (suite *CollectionServiceIntegrationTestSuite) TestRemoveItem_Forbidden() {
	creator := suite.createTestUser("RemoveCreator")
	adder := suite.createTestUser("RemoveAdder")
	other := suite.createTestUser("RemoveOther")

	req := &contracts.CreateCollectionRequest{Title: "Remove Forbidden", IsPublic: true, Collaborative: true}
	coll, _ := suite.collectionService.CreateCollection(creator.ID, req)

	artist := suite.createTestArtist("Forbidden Remove Artist")
	item, _ := suite.collectionService.AddItem(coll.Slug, adder.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: artist.ID,
	})

	// User who is neither creator nor adder should be forbidden
	err := suite.collectionService.RemoveItem(coll.Slug, item.ID, other.ID, false)
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestRemoveItem_AdminCanRemove() {
	creator := suite.createTestUser("AdminRemoveCreator")
	admin := suite.createTestUser("AdminRemover")

	coll := suite.createBasicCollection(creator, "Admin Remove")
	artist := suite.createTestArtist("Admin Removable")
	item, _ := suite.collectionService.AddItem(coll.Slug, creator.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: artist.ID,
	})

	err := suite.collectionService.RemoveItem(coll.Slug, item.ID, admin.ID, true)
	suite.Require().NoError(err)
}

func (suite *CollectionServiceIntegrationTestSuite) TestRemoveItem_ItemNotFound() {
	user := suite.createTestUser("ItemNotFoundRemover")
	coll := suite.createBasicCollection(user, "Empty Remove")

	err := suite.collectionService.RemoveItem(coll.Slug, 99999, user.ID, false)
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionItemNotFound, collErr.Code)
}

// =============================================================================
// Group 8: ReorderItems
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestReorderItems_Success() {
	user := suite.createTestUser("Reorderer")
	coll := suite.createBasicCollection(user, "Reorder Collection")

	a1 := suite.createTestArtist("Reorder First")
	a2 := suite.createTestArtist("Reorder Second")
	a3 := suite.createTestArtist("Reorder Third")

	item1, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a1.ID,
	})
	item2, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a2.ID,
	})
	item3, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a3.ID,
	})

	// Reverse the order
	err := suite.collectionService.ReorderItems(coll.Slug, user.ID, &contracts.ReorderCollectionItemsRequest{
		Items: []contracts.ReorderItem{
			{ItemID: item3.ID, Position: 0},
			{ItemID: item2.ID, Position: 1},
			{ItemID: item1.ID, Position: 2},
		},
	})

	suite.Require().NoError(err)

	// Verify new order
	detail, err := suite.collectionService.GetBySlug(coll.Slug, user.ID)
	suite.Require().NoError(err)
	suite.Require().Len(detail.Items, 3)
	suite.Equal("Reorder Third", detail.Items[0].EntityName)
	suite.Equal("Reorder Second", detail.Items[1].EntityName)
	suite.Equal("Reorder First", detail.Items[2].EntityName)
}

func (suite *CollectionServiceIntegrationTestSuite) TestReorderItems_Forbidden() {
	creator := suite.createTestUser("ReorderOwner")
	other := suite.createTestUser("ReorderOther")
	coll := suite.createBasicCollection(creator, "Reorder Forbidden")

	err := suite.collectionService.ReorderItems(coll.Slug, other.ID, &contracts.ReorderCollectionItemsRequest{
		Items: []contracts.ReorderItem{},
	})

	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestReorderItems_CollectionNotFound() {
	err := suite.collectionService.ReorderItems("nonexistent-slug", 1, &contracts.ReorderCollectionItemsRequest{})
	suite.Require().Error(err)
}

// =============================================================================
// Group 9: Subscribe / Unsubscribe
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestSubscribe_Success() {
	creator := suite.createTestUser("SubCreator")
	subscriber := suite.createTestUser("Subscriber")
	coll := suite.createBasicCollection(creator, "Sub Collection")

	err := suite.collectionService.Subscribe(coll.Slug, subscriber.ID)
	suite.Require().NoError(err)

	// Verify subscriber sees it
	detail, err := suite.collectionService.GetBySlug(coll.Slug, subscriber.ID)
	suite.Require().NoError(err)
	suite.True(detail.IsSubscribed)
	suite.Equal(2, detail.SubscriberCount) // Creator + subscriber
}

func (suite *CollectionServiceIntegrationTestSuite) TestSubscribe_Idempotent() {
	creator := suite.createTestUser("IdempCreator")
	subscriber := suite.createTestUser("IdempSubscriber")
	coll := suite.createBasicCollection(creator, "Idemp Collection")

	err := suite.collectionService.Subscribe(coll.Slug, subscriber.ID)
	suite.Require().NoError(err)

	// Subscribe again — should not error
	err = suite.collectionService.Subscribe(coll.Slug, subscriber.ID)
	suite.Require().NoError(err)

	// Still only 2 subscribers
	detail, err := suite.collectionService.GetBySlug(coll.Slug, subscriber.ID)
	suite.Require().NoError(err)
	suite.Equal(2, detail.SubscriberCount)
}

func (suite *CollectionServiceIntegrationTestSuite) TestSubscribe_PrivateCollectionForbidden() {
	creator := suite.createTestUser("PrivSubCreator")
	other := suite.createTestUser("PrivSubOther")

	req := &contracts.CreateCollectionRequest{Title: "Private Sub", IsPublic: false}
	coll, _ := suite.collectionService.CreateCollection(creator.ID, req)

	err := suite.collectionService.Subscribe(coll.Slug, other.ID)
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestUnsubscribe_Success() {
	creator := suite.createTestUser("UnsubCreator")
	subscriber := suite.createTestUser("Unsubscriber")
	coll := suite.createBasicCollection(creator, "Unsub Collection")

	suite.collectionService.Subscribe(coll.Slug, subscriber.ID)

	err := suite.collectionService.Unsubscribe(coll.Slug, subscriber.ID)
	suite.Require().NoError(err)

	// Verify
	detail, err := suite.collectionService.GetBySlug(coll.Slug, subscriber.ID)
	suite.Require().NoError(err)
	suite.False(detail.IsSubscribed)
	suite.Equal(1, detail.SubscriberCount) // Only creator remains
}

func (suite *CollectionServiceIntegrationTestSuite) TestUnsubscribe_NotSubscribed() {
	creator := suite.createTestUser("NoSubCreator")
	other := suite.createTestUser("NeverSubbed")
	coll := suite.createBasicCollection(creator, "No Sub Collection")

	// Unsubscribe without being subscribed — should not error
	err := suite.collectionService.Unsubscribe(coll.Slug, other.ID)
	suite.Require().NoError(err)
}

func (suite *CollectionServiceIntegrationTestSuite) TestUnsubscribe_CollectionNotFound() {
	err := suite.collectionService.Unsubscribe("nonexistent-slug", 1)
	suite.Require().Error(err)
}

// =============================================================================
// Group 10: MarkVisited
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestMarkVisited_Success() {
	user := suite.createTestUser("Visitor")
	coll := suite.createBasicCollection(user, "Visit Collection")

	// Creator is auto-subscribed, so marking visited should work
	err := suite.collectionService.MarkVisited(coll.Slug, user.ID)
	suite.Require().NoError(err)

	// Verify timestamp was updated
	var subscriber models.CollectionSubscriber
	err = suite.db.Where("collection_id = ? AND user_id = ?", coll.ID, user.ID).First(&subscriber).Error
	suite.Require().NoError(err)
	suite.Require().NotNil(subscriber.LastVisitedAt)
}

func (suite *CollectionServiceIntegrationTestSuite) TestMarkVisited_CollectionNotFound() {
	err := suite.collectionService.MarkVisited("nonexistent-slug", 1)
	suite.Require().Error(err)
}

// =============================================================================
// Group 11: GetStats
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestGetStats_Success() {
	user := suite.createTestUser("StatsUser")
	coll := suite.createBasicCollection(user, "Stats Collection")

	a1 := suite.createTestArtist("Stats Artist 1")
	a2 := suite.createTestArtist("Stats Artist 2")
	v1 := suite.createTestVenueForCollection("Stats Venue")

	suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a1.ID,
	})
	suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a2.ID,
	})
	suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityVenue, EntityID: v1.ID,
	})

	stats, err := suite.collectionService.GetStats(coll.Slug)

	suite.Require().NoError(err)
	suite.Equal(3, stats.ItemCount)
	suite.Equal(1, stats.SubscriberCount) // Creator
	suite.Equal(1, stats.ContributorCount)
	suite.Equal(2, stats.EntityTypeCounts[models.CollectionEntityArtist])
	suite.Equal(1, stats.EntityTypeCounts[models.CollectionEntityVenue])
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetStats_EmptyCollection() {
	user := suite.createTestUser("EmptyStatsUser")
	coll := suite.createBasicCollection(user, "Empty Stats")

	stats, err := suite.collectionService.GetStats(coll.Slug)

	suite.Require().NoError(err)
	suite.Equal(0, stats.ItemCount)
	suite.Equal(1, stats.SubscriberCount) // Creator
	suite.Equal(0, stats.ContributorCount)
	suite.Empty(stats.EntityTypeCounts)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetStats_NotFound() {
	resp, err := suite.collectionService.GetStats("nonexistent-slug")
	suite.Require().Error(err)
	suite.Nil(resp)
}

// =============================================================================
// Group 12: GetUserCollections
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestGetUserCollections_Created() {
	user := suite.createTestUser("UserCollCreator")
	suite.createBasicCollection(user, "My Collection 1")
	suite.createBasicCollection(user, "My Collection 2")

	resp, total, err := suite.collectionService.GetUserCollections(user.ID, 20, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetUserCollections_IncludesSubscribed() {
	creator := suite.createTestUser("SubCollCreator")
	subscriber := suite.createTestUser("SubCollSubscriber")

	coll := suite.createBasicCollection(creator, "Subscribed Collection")
	suite.collectionService.Subscribe(coll.Slug, subscriber.ID)

	// Subscriber's own collection
	suite.createBasicCollection(subscriber, "Own Collection")

	resp, total, err := suite.collectionService.GetUserCollections(subscriber.ID, 20, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total) // 1 created + 1 subscribed
	suite.Len(resp, 2)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetUserCollections_Empty() {
	user := suite.createTestUser("EmptyUserColl")

	resp, total, err := suite.collectionService.GetUserCollections(user.ID, 20, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(resp)
}

// =============================================================================
// Group 13: SetFeatured
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestSetFeatured_Success() {
	user := suite.createTestUser("FeaturedCreator")
	coll := suite.createBasicCollection(user, "Feature Me")

	err := suite.collectionService.SetFeatured(coll.Slug, true)
	suite.Require().NoError(err)

	detail, err := suite.collectionService.GetBySlug(coll.Slug, user.ID)
	suite.Require().NoError(err)
	suite.True(detail.IsFeatured)
}

func (suite *CollectionServiceIntegrationTestSuite) TestSetFeatured_Unfeature() {
	user := suite.createTestUser("UnfeatureCreator")
	coll := suite.createBasicCollection(user, "Unfeature Me")

	suite.collectionService.SetFeatured(coll.Slug, true)

	err := suite.collectionService.SetFeatured(coll.Slug, false)
	suite.Require().NoError(err)

	detail, err := suite.collectionService.GetBySlug(coll.Slug, user.ID)
	suite.Require().NoError(err)
	suite.False(detail.IsFeatured)
}

func (suite *CollectionServiceIntegrationTestSuite) TestSetFeatured_NotFound() {
	err := suite.collectionService.SetFeatured("nonexistent-slug", true)
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionNotFound, collErr.Code)
}

// =============================================================================
// Group 14: Entity name resolution
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_ItemEntityNamesResolved() {
	user := suite.createTestUser("NameResolver")
	coll := suite.createBasicCollection(user, "Name Resolution")

	artist := suite.createTestArtist("Resolved Artist")
	venue := suite.createTestVenueForCollection("Resolved Venue")

	suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: artist.ID,
	})
	suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityVenue, EntityID: venue.ID,
	})

	detail, err := suite.collectionService.GetBySlug(coll.Slug, user.ID)
	suite.Require().NoError(err)
	suite.Require().Len(detail.Items, 2)

	// Items are ordered by position
	suite.Equal("Resolved Artist", detail.Items[0].EntityName)
	suite.Equal("Resolved Venue", detail.Items[1].EntityName)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_ContributorCount() {
	creator := suite.createTestUser("ContribOwner")
	collab1 := suite.createTestUser("Contrib1")
	collab2 := suite.createTestUser("Contrib2")

	req := &contracts.CreateCollectionRequest{Title: "Contrib Count", IsPublic: true, Collaborative: true}
	coll, _ := suite.collectionService.CreateCollection(creator.ID, req)

	a1 := suite.createTestArtist("Contrib Artist 1")
	a2 := suite.createTestArtist("Contrib Artist 2")
	a3 := suite.createTestArtist("Contrib Artist 3")

	suite.collectionService.AddItem(coll.Slug, creator.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a1.ID,
	})
	suite.collectionService.AddItem(coll.Slug, collab1.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a2.ID,
	})
	suite.collectionService.AddItem(coll.Slug, collab2.ID, &contracts.AddCollectionItemRequest{
		EntityType: models.CollectionEntityArtist, EntityID: a3.ID,
	})

	detail, err := suite.collectionService.GetBySlug(coll.Slug, creator.ID)
	suite.Require().NoError(err)
	suite.Equal(3, detail.ContributorCount)
}
