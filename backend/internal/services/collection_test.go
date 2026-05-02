package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/catalog"
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
	// tagService is wired into collectionService so the PSY-354 test paths
	// (tag rendering on detail/list, AddTagToCollection, RemoveTagFromCollection)
	// exercise the same code production uses.
	tagService *catalog.TagService
}

func (suite *CollectionServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.tagService = catalog.NewTagService(suite.testDB.DB)
	suite.collectionService = &CollectionService{db: suite.testDB.DB, tagService: suite.tagService}
}

func (suite *CollectionServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *CollectionServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	// PSY-354: clear polymorphic tag links + votes before users (added_by FKs
	// are NOT ON DELETE CASCADE, so leaked rows would block user deletion).
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM collection_likes")
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
	// Tag corpus last — LOWER(name) unique would collide between tests.
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestCollectionServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CollectionServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *CollectionServiceIntegrationTestSuite) createTestUser(name string) *authm.User {
	user := &authm.User{
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

func (suite *CollectionServiceIntegrationTestSuite) createTestUserWithUsername(name, username string) *authm.User {
	user := &authm.User{
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

func (suite *CollectionServiceIntegrationTestSuite) createTestArtist(name string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *CollectionServiceIntegrationTestSuite) createTestVenueForCollection(name string) *catalogm.Venue {
	venue := &catalogm.Venue{Name: name, City: "Phoenix", State: "AZ", Verified: true}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

// createBasicCollection creates a private collection (no items, no
// description) returned by CreateCollection. PSY-356 forbids creating
// public-at-create-time, so the caller is expected to rely on the slug
// alone for tests that don't care about visibility. Tests that need a
// public, gate-passing collection should call createPublicCollection
// instead.
func (suite *CollectionServiceIntegrationTestSuite) createBasicCollection(user *authm.User, title string) *contracts.CollectionDetailResponse {
	resp, err := suite.collectionService.CreateCollection(user.ID, &contracts.CreateCollectionRequest{
		Title:    title,
		IsPublic: false,
	})
	suite.Require().NoError(err)
	return resp
}

// createPublicCollection creates a collection that satisfies the PSY-356
// publish gate (>= 3 items, >= 50-char description) and flips it public.
// Use this when a test depends on the collection being publicly visible
// (anonymous viewer access, browse listing, etc.).
func (suite *CollectionServiceIntegrationTestSuite) createPublicCollection(user *authm.User, title string) *contracts.CollectionDetailResponse {
	priv := suite.createBasicCollection(user, title)

	for i := 0; i < MinPublicCollectionItems; i++ {
		artist := suite.createTestArtist(fmt.Sprintf("%s seed %d-%d", title, i, time.Now().UnixNano()))
		_, err := suite.collectionService.AddItem(priv.Slug, user.ID, &contracts.AddCollectionItemRequest{
			EntityType: communitym.CollectionEntityArtist,
			EntityID:   artist.ID,
		})
		suite.Require().NoError(err)
	}

	desc := fmt.Sprintf("Quality-gate description for %s — long enough to satisfy the 50-char minimum.", title)
	pub := true
	resp, err := suite.collectionService.UpdateCollection(priv.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Description: &desc,
		IsPublic:    &pub,
	})
	suite.Require().NoError(err)
	return resp
}

// createBareCollection is an alias for createBasicCollection kept for
// PSY-356 tests that read more clearly with the explicit "bare" name.
func (suite *CollectionServiceIntegrationTestSuite) createBareCollection(user *authm.User, title string) *contracts.CollectionDetailResponse {
	return suite.createBasicCollection(user, title)
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

// TestCreateCollection_Success creates as private (per PSY-356 — public at
// create time is rejected because items_count is always 0) and verifies the
// usual fields are persisted. Public-creation rejection is covered by
// TestCreateCollection_PublicAtCreateRejected.
func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_Success() {
	user := suite.createTestUser("Creator")

	desc := "My favorite artists"
	req := &contracts.CreateCollectionRequest{
		Title:         "Best Artists",
		Description:   &desc,
		Collaborative: true,
		IsPublic:      false,
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
	suite.False(resp.IsPublic) // PSY-356: cannot create public directly.
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

// PSY-353: detail responses must surface creator_username so the frontend
// can link the attribution to /users/:username. When the creator has no
// username, the field is null (not the empty string) so the frontend can
// distinguish "linkable" from "render unlinked".
func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_CreatorUsername() {
	user := suite.createTestUserWithUsername("Bea", "beam")
	resp := suite.createBasicCollection(user, "Username Test")
	suite.Require().NotNil(resp.CreatorUsername)
	suite.Equal("beam", *resp.CreatorUsername)
}

func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_CreatorUsername_NilWhenAbsent() {
	user := suite.createTestUser("NoUsernameCreator")
	resp := suite.createBasicCollection(user, "No Username Test")
	suite.Nil(resp.CreatorUsername)
}

// PSY-353: list responses must surface creator_username for the same
// reason as detail responses — collection cards link the attribution.
func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_CreatorUsername() {
	withUsername := suite.createTestUserWithUsername("Cara", "carac")
	withoutUsername := suite.createTestUser("NoNameLister")

	suite.createBasicCollection(withUsername, "List Username With")
	suite.createBasicCollection(withoutUsername, "List Username Without")

	resps, _, err := suite.collectionService.ListCollections(contracts.CollectionFilters{}, 50, 0)
	suite.Require().NoError(err)

	byCreator := map[uint]*contracts.CollectionListResponse{}
	for _, r := range resps {
		byCreator[r.CreatorID] = r
	}

	withResp := byCreator[withUsername.ID]
	suite.Require().NotNil(withResp)
	suite.Require().NotNil(withResp.CreatorUsername)
	suite.Equal("carac", *withResp.CreatorUsername)

	withoutResp := byCreator[withoutUsername.ID]
	suite.Require().NotNil(withoutResp)
	suite.Nil(withoutResp.CreatorUsername)
}

func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_DefaultDisplayModeUnranked() {
	user := suite.createTestUser("DefaultModeCreator")
	resp := suite.createBasicCollection(user, "Default Mode")
	suite.Equal(communitym.CollectionDisplayModeUnranked, resp.DisplayMode)
}

func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_OptInRankedMode() {
	user := suite.createTestUser("RankedCreator")
	mode := communitym.CollectionDisplayModeRanked
	req := &contracts.CreateCollectionRequest{
		Title:       "Top Albums of 2026",
		IsPublic:    false, // PSY-356: must be created private.
		DisplayMode: &mode,
	}

	resp, err := suite.collectionService.CreateCollection(user.ID, req)

	suite.Require().NoError(err)
	suite.Equal(communitym.CollectionDisplayModeRanked, resp.DisplayMode)
}

func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_InvalidDisplayMode() {
	user := suite.createTestUser("InvalidModeCreator")
	bogus := "best-of"
	req := &contracts.CreateCollectionRequest{
		Title:       "Bogus Mode",
		IsPublic:    false, // PSY-356: avoid colliding with the publish gate.
		DisplayMode: &bogus,
	}

	resp, err := suite.collectionService.CreateCollection(user.ID, req)

	suite.Require().Error(err)
	suite.Nil(resp)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionInvalidRequest, collErr.Code)
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
	created := suite.createPublicCollection(user, "Public Collection")

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
	suite.createPublicCollection(user, "Public One")

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
	// Public so the trailing GetBySlug call (with admin's user ID) is allowed.
	created := suite.createPublicCollection(creator, "Admin Editable")

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
		IsPublic:    false, // PSY-356: tested behavior is markdown rendering, not visibility.
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
		IsPublic:    false, // PSY-356: tested behavior is markdown rendering, not visibility.
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
		IsPublic:    false, // PSY-356: tested behavior is XSS sanitization, not visibility.
	})
	suite.Require().NoError(err)

	resp, err := suite.collectionService.GetBySlug(created.Slug, user.ID)
	suite.Require().NoError(err)
	// Raw markdown is preserved (the editor will show what was typed); the
	// rendered HTML must strip <script> per the bluemonday policy. Inner text
	// of stripped tags becomes plain visible text — harmless without the
	// surrounding executable tag — so we assert the tags themselves are gone,
	// not the inner text.
	suite.NotContains(resp.DescriptionHTML, "<script>")
	suite.NotContains(resp.DescriptionHTML, "</script>")
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

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_DisplayModeToggle() {
	user := suite.createTestUser("ToggleUpdater")
	created := suite.createBasicCollection(user, "Toggle Mode")

	// Add an item so we can verify positions survive the mode flip.
	artist := suite.createTestArtist("Toggle Artist")
	_, err := suite.collectionService.AddItem(created.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: artist.ID,
	})
	suite.Require().NoError(err)

	// Default → ranked
	mode := communitym.CollectionDisplayModeRanked
	resp, err := suite.collectionService.UpdateCollection(created.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		DisplayMode: &mode,
	})
	suite.Require().NoError(err)
	suite.Equal(communitym.CollectionDisplayModeRanked, resp.DisplayMode)
	suite.Equal(1, resp.ItemCount, "items should survive mode toggle")

	// Ranked → unranked (data preserved)
	mode = communitym.CollectionDisplayModeUnranked
	resp, err = suite.collectionService.UpdateCollection(resp.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		DisplayMode: &mode,
	})
	suite.Require().NoError(err)
	suite.Equal(communitym.CollectionDisplayModeUnranked, resp.DisplayMode)
	suite.Equal(1, resp.ItemCount, "items should survive mode toggle")
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_InvalidDisplayMode() {
	user := suite.createTestUser("BadModeUpdater")
	created := suite.createBasicCollection(user, "Bad Mode")

	bogus := "ranked-by-vibes"
	resp, err := suite.collectionService.UpdateCollection(created.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		DisplayMode: &bogus,
	})

	suite.Require().Error(err)
	suite.Nil(resp)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionInvalidRequest, collErr.Code)
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
		EntityType: communitym.CollectionEntityArtist,
		EntityID:   artist.ID,
	})

	err := suite.collectionService.DeleteCollection(created.Slug, user.ID, false)
	suite.Require().NoError(err)

	// Verify items and subscribers are cleaned up
	var itemCount int64
	suite.db.Model(&communitym.CollectionItem{}).Where("collection_id = ?", created.ID).Count(&itemCount)
	suite.Equal(int64(0), itemCount)

	var subCount int64
	suite.db.Model(&communitym.CollectionSubscriber{}).Where("collection_id = ?", created.ID).Count(&subCount)
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
		EntityType: communitym.CollectionEntityArtist,
		EntityID:   artist.ID,
	})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal(communitym.CollectionEntityArtist, resp.EntityType)
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
		EntityType: communitym.CollectionEntityVenue,
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
		EntityType: communitym.CollectionEntityArtist, EntityID: a1.ID,
	})
	resp2, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a2.ID,
	})
	resp3, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a3.ID,
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
		EntityType: communitym.CollectionEntityArtist,
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
		EntityType: communitym.CollectionEntityArtist,
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
		EntityType: communitym.CollectionEntityArtist,
		EntityID:   artist.ID,
		Notes:      &notes,
	})

	suite.Require().NoError(err)
	suite.NotContains(resp.NotesHTML, "<script>")
	suite.NotContains(resp.NotesHTML, "</script>")
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_NotesTooLong() {
	user := suite.createTestUser("LongNoteAdder")
	coll := suite.createBasicCollection(user, "Too Long Notes")
	artist := suite.createTestArtist("Long Artist")

	long := strings.Repeat("a", contracts.MaxCollectionItemNotesLength+1)
	_, err := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist,
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
		EntityType: communitym.CollectionEntityArtist,
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
		EntityType: communitym.CollectionEntityArtist, EntityID: artist.ID,
	})
	suite.Require().NoError(err)

	_, err = suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: artist.ID,
	})
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionItemExists, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_CollaborativeByOtherUser() {
	creator := suite.createTestUser("CollabOwner")
	collaborator := suite.createTestUser("Collaborator")

	req := &contracts.CreateCollectionRequest{Title: "Collab Collection", IsPublic: false, Collaborative: true}
	coll, err := suite.collectionService.CreateCollection(creator.ID, req)
	suite.Require().NoError(err)

	artist := suite.createTestArtist("Collab Artist")
	resp, err := suite.collectionService.AddItem(coll.Slug, collaborator.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: artist.ID,
	})

	suite.Require().NoError(err)
	suite.Equal(collaborator.ID, resp.AddedByUserID)
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_NonCollaborativeForbidden() {
	creator := suite.createTestUser("SoloOwner")
	other := suite.createTestUser("Outsider")

	req := &contracts.CreateCollectionRequest{Title: "Solo Collection", IsPublic: false, Collaborative: false}
	coll, err := suite.collectionService.CreateCollection(creator.ID, req)
	suite.Require().NoError(err)

	artist := suite.createTestArtist("Blocked Artist")
	resp, err := suite.collectionService.AddItem(coll.Slug, other.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: artist.ID,
	})

	suite.Require().Error(err)
	suite.Nil(resp)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_CollectionNotFound() {
	resp, err := suite.collectionService.AddItem("nonexistent-slug", 1, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: 1,
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
		EntityType: communitym.CollectionEntityArtist, EntityID: artist.ID,
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

	req := &contracts.CreateCollectionRequest{Title: "Collab Remove", IsPublic: false, Collaborative: true}
	coll, _ := suite.collectionService.CreateCollection(creator.ID, req)

	artist := suite.createTestArtist("Adder Artist")
	item, _ := suite.collectionService.AddItem(coll.Slug, adder.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: artist.ID,
	})

	// The adder should be able to remove their own item
	err := suite.collectionService.RemoveItem(coll.Slug, item.ID, adder.ID, false)
	suite.Require().NoError(err)
}

func (suite *CollectionServiceIntegrationTestSuite) TestRemoveItem_Forbidden() {
	creator := suite.createTestUser("RemoveCreator")
	adder := suite.createTestUser("RemoveAdder")
	other := suite.createTestUser("RemoveOther")

	req := &contracts.CreateCollectionRequest{Title: "Remove Forbidden", IsPublic: false, Collaborative: true}
	coll, _ := suite.collectionService.CreateCollection(creator.ID, req)

	artist := suite.createTestArtist("Forbidden Remove Artist")
	item, _ := suite.collectionService.AddItem(coll.Slug, adder.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: artist.ID,
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
		EntityType: communitym.CollectionEntityArtist, EntityID: artist.ID,
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
		EntityType: communitym.CollectionEntityArtist, EntityID: a1.ID,
	})
	item2, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a2.ID,
	})
	item3, _ := suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a3.ID,
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
	// Public so a non-creator subscriber can read the result via GetBySlug.
	coll := suite.createPublicCollection(creator, "Sub Collection")

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
	coll := suite.createPublicCollection(creator, "Idemp Collection")

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
	coll := suite.createPublicCollection(creator, "Unsub Collection")

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
	var subscriber communitym.CollectionSubscriber
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
		EntityType: communitym.CollectionEntityArtist, EntityID: a1.ID,
	})
	suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a2.ID,
	})
	suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityVenue, EntityID: v1.ID,
	})

	stats, err := suite.collectionService.GetStats(coll.Slug)

	suite.Require().NoError(err)
	suite.Equal(3, stats.ItemCount)
	suite.Equal(1, stats.SubscriberCount) // Creator
	suite.Equal(1, stats.ContributorCount)
	suite.Equal(2, stats.EntityTypeCounts[communitym.CollectionEntityArtist])
	suite.Equal(1, stats.EntityTypeCounts[communitym.CollectionEntityVenue])
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

	// Public so the non-creator subscriber can subscribe.
	coll := suite.createPublicCollection(creator, "Subscribed Collection")
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
		EntityType: communitym.CollectionEntityArtist, EntityID: artist.ID,
	})
	suite.collectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityVenue, EntityID: venue.ID,
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

	req := &contracts.CreateCollectionRequest{Title: "Contrib Count", IsPublic: false, Collaborative: true}
	coll, _ := suite.collectionService.CreateCollection(creator.ID, req)

	a1 := suite.createTestArtist("Contrib Artist 1")
	a2 := suite.createTestArtist("Contrib Artist 2")
	a3 := suite.createTestArtist("Contrib Artist 3")

	suite.collectionService.AddItem(coll.Slug, creator.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a1.ID,
	})
	suite.collectionService.AddItem(coll.Slug, collab1.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a2.ID,
	})
	suite.collectionService.AddItem(coll.Slug, collab2.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a3.ID,
	})

	detail, err := suite.collectionService.GetBySlug(coll.Slug, creator.ID)
	suite.Require().NoError(err)
	suite.Equal(3, detail.ContributorCount)
}

// =============================================================================
// Group 13 (PSY-350): GetBySlug bumps last_visited_at for authed subscribers
// =============================================================================

// TestGetBySlug_AuthenticatedSubscriber_BumpsLastVisitedAt verifies the
// fire-and-forget MarkVisited side-effect lands. Done via polling because
// the bump runs in a goroutine.
func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_AuthenticatedSubscriber_BumpsLastVisitedAt() {
	creator := suite.createTestUser("Visitor")
	coll := suite.createBasicCollection(creator, "Visit Test")

	// Reset last_visited_at to a known stale value.
	stale := time.Now().Add(-24 * time.Hour)
	suite.Require().NoError(
		suite.db.Model(&communitym.CollectionSubscriber{}).
			Where("collection_id = ? AND user_id = ?", coll.ID, creator.ID).
			Update("last_visited_at", stale).Error,
	)

	_, err := suite.collectionService.GetBySlug(coll.Slug, creator.ID)
	suite.Require().NoError(err)

	// Poll for up to ~250ms — the goroutine should have run by then.
	var subscriber communitym.CollectionSubscriber
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		err = suite.db.Where("collection_id = ? AND user_id = ?", coll.ID, creator.ID).First(&subscriber).Error
		suite.Require().NoError(err)
		if subscriber.LastVisitedAt != nil && subscriber.LastVisitedAt.After(stale) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	suite.Require().NotNil(subscriber.LastVisitedAt)
	suite.True(subscriber.LastVisitedAt.After(stale),
		"expected LastVisitedAt to advance past the stale value")
}

// TestGetBySlug_NonSubscriber_NoSideEffect — viewing a public collection
// without being subscribed must NOT create a subscription row.
func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_NonSubscriber_NoSideEffect() {
	creator := suite.createTestUser("Creator")
	viewer := suite.createTestUser("Viewer")
	coll := suite.createPublicCollection(creator, "Public Collection")

	_, err := suite.collectionService.GetBySlug(coll.Slug, viewer.ID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&communitym.CollectionSubscriber{}).
		Where("collection_id = ? AND user_id = ?", coll.ID, viewer.ID).
		Count(&count)
	suite.Equal(int64(0), count, "viewing without subscribing must not create a subscription row")
}

// =============================================================================
// Group 14 (PSY-350): GetUserCollections.NewSinceLastVisit
// =============================================================================

// TestGetUserCollections_NewSinceLastVisit_CountsItemsAfterLastVisit verifies
// the library tab "N new since last visit" badge math. PSY-350.
func (suite *CollectionServiceIntegrationTestSuite) TestGetUserCollections_NewSinceLastVisit_CountsItemsAfterLastVisit() {
	creator := suite.createTestUser("Creator")
	subscriber := suite.createTestUser("Subscriber")
	// Use a collaborative collection so the subscriber can also add items.
	// PSY-356: created private (gate test scope is unrelated to visibility).
	collResp, err := suite.collectionService.CreateCollection(creator.ID, &contracts.CreateCollectionRequest{
		Title:         "Tracked Collection",
		IsPublic:      false,
		Collaborative: true,
	})
	suite.Require().NoError(err)
	coll := collResp

	// Subscribe the second user with a fixed last_visited_at.
	visitedAt := time.Now().Add(-1 * time.Hour)
	sub := &communitym.CollectionSubscriber{
		CollectionID:  coll.ID,
		UserID:        subscriber.ID,
		LastVisitedAt: &visitedAt,
	}
	suite.Require().NoError(suite.db.Create(sub).Error)

	a1 := suite.createTestArtist("A1")
	a2 := suite.createTestArtist("A2")
	a3 := suite.createTestArtist("A3")

	// Item 1 added BEFORE visit — should not count.
	item1, err := suite.collectionService.AddItem(coll.Slug, creator.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a1.ID,
	})
	suite.Require().NoError(err)
	suite.Require().NoError(suite.db.Model(&communitym.CollectionItem{}).
		Where("id = ?", item1.ID).
		Update("created_at", visitedAt.Add(-30*time.Minute)).Error)

	// Item 2 added AFTER visit by creator — should count.
	item2, err := suite.collectionService.AddItem(coll.Slug, creator.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a2.ID,
	})
	suite.Require().NoError(err)
	suite.Require().NoError(suite.db.Model(&communitym.CollectionItem{}).
		Where("id = ?", item2.ID).
		Update("created_at", visitedAt.Add(15*time.Minute)).Error)

	// Item 3 added AFTER visit by subscriber themselves — should NOT count
	// (we exclude the viewer's own additions to keep the badge meaningful).
	item3, err := suite.collectionService.AddItem(coll.Slug, subscriber.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a3.ID,
	})
	suite.Require().NoError(err)
	suite.Require().NoError(suite.db.Model(&communitym.CollectionItem{}).
		Where("id = ?", item3.ID).
		Update("created_at", visitedAt.Add(45*time.Minute)).Error)

	// Fetch via the user-collections endpoint.
	resp, _, err := suite.collectionService.GetUserCollections(subscriber.ID, 20, 0)
	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal(1, resp[0].NewSinceLastVisit, "expected exactly one new item since visit (excluding self-add)")
}

// TestGetUserCollections_NewSinceLastVisit_NeverVisited_FallsBackToSubscriptionStart
// verifies that subscribers who have never visited get the count of items
// added after the subscription's created_at (excluding self).
func (suite *CollectionServiceIntegrationTestSuite) TestGetUserCollections_NewSinceLastVisit_NeverVisited_FallsBackToSubscriptionStart() {
	creator := suite.createTestUser("Creator")
	subscriber := suite.createTestUser("Sub")
	coll := suite.createBasicCollection(creator, "Coll")

	// Subscribe the second user with NULL last_visited_at.
	sub := &communitym.CollectionSubscriber{
		CollectionID:  coll.ID,
		UserID:        subscriber.ID,
		LastVisitedAt: nil,
	}
	suite.Require().NoError(suite.db.Create(sub).Error)

	// Add one item after subscribing — should count.
	a := suite.createTestArtist("A")
	_, err := suite.collectionService.AddItem(coll.Slug, creator.ID, &contracts.AddCollectionItemRequest{
		EntityType: communitym.CollectionEntityArtist, EntityID: a.ID,
	})
	suite.Require().NoError(err)

	resp, _, err := suite.collectionService.GetUserCollections(subscriber.ID, 20, 0)
	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal(1, resp[0].NewSinceLastVisit)
}

// =============================================================================
// PSY-352: Likes + popular sort
// =============================================================================

// TestLike_Success verifies a fresh like creates the row, returns the correct
// aggregate, and surfaces in subsequent GetBySlug responses.
func (suite *CollectionServiceIntegrationTestSuite) TestLike_Success() {
	creator := suite.createTestUser("Creator")
	liker := suite.createTestUser("Liker")
	coll := suite.createPublicCollection(creator, "Likeable")

	resp, err := suite.collectionService.Like(coll.Slug, liker.ID)
	suite.Require().NoError(err)
	suite.Equal(1, resp.LikeCount)
	suite.True(resp.UserLikesThis)

	detail, err := suite.collectionService.GetBySlug(coll.Slug, liker.ID)
	suite.Require().NoError(err)
	suite.Equal(1, detail.LikeCount)
	suite.True(detail.UserLikesThis)
}

// TestLike_Idempotent verifies that liking twice does not error and the count
// does not double — composite-PK + ON CONFLICT DO NOTHING is the contract.
func (suite *CollectionServiceIntegrationTestSuite) TestLike_Idempotent() {
	creator := suite.createTestUser("Creator")
	liker := suite.createTestUser("Liker")
	coll := suite.createPublicCollection(creator, "Idempotent")

	r1, err := suite.collectionService.Like(coll.Slug, liker.ID)
	suite.Require().NoError(err)
	suite.Equal(1, r1.LikeCount)

	r2, err := suite.collectionService.Like(coll.Slug, liker.ID)
	suite.Require().NoError(err)
	suite.Equal(1, r2.LikeCount)
	suite.True(r2.UserLikesThis)
}

// TestLike_NotFound returns a CollectionNotFound error for unknown slugs.
func (suite *CollectionServiceIntegrationTestSuite) TestLike_NotFound() {
	user := suite.createTestUser("User")

	_, err := suite.collectionService.Like("does-not-exist", user.ID)
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionNotFound, collErr.Code)
}

// TestLike_PrivateCollection_OtherUser blocks likes on private collections
// the caller does not own.
func (suite *CollectionServiceIntegrationTestSuite) TestLike_PrivateCollection_OtherUser() {
	creator := suite.createTestUser("Creator")
	other := suite.createTestUser("Other")

	private := false
	req := &contracts.CreateCollectionRequest{
		Title:    "Private",
		IsPublic: private,
	}
	coll, err := suite.collectionService.CreateCollection(creator.ID, req)
	suite.Require().NoError(err)
	// CreateCollection's bool gotcha workaround leaves IsPublic true on
	// initial create then updates to false — assert post-state.
	suite.Require().False(coll.IsPublic)

	_, err = suite.collectionService.Like(coll.Slug, other.ID)
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

// TestUnlike_Success verifies that unliking decrements the count.
func (suite *CollectionServiceIntegrationTestSuite) TestUnlike_Success() {
	creator := suite.createTestUser("Creator")
	liker := suite.createTestUser("Liker")
	coll := suite.createPublicCollection(creator, "Unlikeable")

	_, err := suite.collectionService.Like(coll.Slug, liker.ID)
	suite.Require().NoError(err)

	resp, err := suite.collectionService.Unlike(coll.Slug, liker.ID)
	suite.Require().NoError(err)
	suite.Equal(0, resp.LikeCount)
	suite.False(resp.UserLikesThis)
}

// TestUnlike_Idempotent verifies that unliking when no like exists is a no-op.
func (suite *CollectionServiceIntegrationTestSuite) TestUnlike_Idempotent() {
	creator := suite.createTestUser("Creator")
	user := suite.createTestUser("User")
	coll := suite.createPublicCollection(creator, "NoOp")

	resp, err := suite.collectionService.Unlike(coll.Slug, user.ID)
	suite.Require().NoError(err)
	suite.Equal(0, resp.LikeCount)
	suite.False(resp.UserLikesThis)
}

// TestListCollections_PopulatesLikeAggregates verifies aggregate counts and
// per-viewer like state on the public list response.
func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_PopulatesLikeAggregates() {
	creator := suite.createTestUser("Creator")
	liker1 := suite.createTestUser("L1")
	liker2 := suite.createTestUser("L2")
	coll := suite.createPublicCollection(creator, "Aggregated")

	_, err := suite.collectionService.Like(coll.Slug, liker1.ID)
	suite.Require().NoError(err)
	_, err = suite.collectionService.Like(coll.Slug, liker2.ID)
	suite.Require().NoError(err)

	// Anonymous viewer: count populated, user_likes_this false.
	resp, _, err := suite.collectionService.ListCollections(contracts.CollectionFilters{}, 20, 0)
	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal(2, resp[0].LikeCount)
	suite.False(resp[0].UserLikesThis)

	// Liker viewer: count populated, user_likes_this true.
	resp, _, err = suite.collectionService.ListCollections(
		contracts.CollectionFilters{ViewerID: liker1.ID}, 20, 0,
	)
	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal(2, resp[0].LikeCount)
	suite.True(resp[0].UserLikesThis)
}

// TestListCollections_PopularSort orders by HN gravity. A young collection
// with a few likes should outrank an old collection with many likes when
// the age delta is large enough.
func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_PopularSort() {
	creator := suite.createTestUser("Creator")

	// Old collection: created 30 days ago, 5 likes.
	old := suite.createPublicCollection(creator, "Old Hits")
	thirtyDaysAgo := time.Now().Add(-30 * 24 * time.Hour)
	suite.Require().NoError(
		suite.db.Model(&communitym.Collection{}).Where("id = ?", old.ID).
			Update("created_at", thirtyDaysAgo).Error,
	)
	for i := 0; i < 5; i++ {
		liker := suite.createTestUser(fmt.Sprintf("Old%d", i))
		_, err := suite.collectionService.Like(old.Slug, liker.ID)
		suite.Require().NoError(err)
	}

	// Young collection: created now, 3 likes.
	young := suite.createPublicCollection(creator, "Young Buzz")
	for i := 0; i < 3; i++ {
		liker := suite.createTestUser(fmt.Sprintf("Young%d", i))
		_, err := suite.collectionService.Like(young.Slug, liker.ID)
		suite.Require().NoError(err)
	}

	resp, _, err := suite.collectionService.ListCollections(
		contracts.CollectionFilters{Sort: contracts.CollectionSortPopular}, 20, 0,
	)
	suite.Require().NoError(err)
	suite.Require().GreaterOrEqual(len(resp), 2)
	// Young Buzz should beat Old Hits under HN gravity:
	//   young: 3 / (0 + 2)^1.8     ≈ 3 / 3.48   ≈ 0.86
	//   old:   5 / (720 + 2)^1.8   ≈ 5 / 188400 ≈ 0.000027
	suite.Equal(young.ID, resp[0].ID, "expected young collection ranked first under HN gravity")
}

// TestListCollections_DefaultSort_PreservedAfterPopularAdded verifies that
// the default ordering remains updated_at DESC when sort is empty.
func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_DefaultSort_PreservedAfterPopularAdded() {
	creator := suite.createTestUser("Creator")

	first := suite.createPublicCollection(creator, "First")
	time.Sleep(20 * time.Millisecond)
	second := suite.createPublicCollection(creator, "Second")

	resp, _, err := suite.collectionService.ListCollections(contracts.CollectionFilters{}, 20, 0)
	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
	suite.Equal(second.ID, resp[0].ID)
	suite.Equal(first.ID, resp[1].ID)
}

// TestBatchCountLikes returns correct counts for multiple collections.
func (suite *CollectionServiceIntegrationTestSuite) TestBatchCountLikes() {
	creator := suite.createTestUser("Creator")
	a := suite.createPublicCollection(creator, "A")
	b := suite.createPublicCollection(creator, "B")
	c := suite.createPublicCollection(creator, "C")

	l1 := suite.createTestUser("L1")
	l2 := suite.createTestUser("L2")
	_, _ = suite.collectionService.Like(a.Slug, l1.ID)
	_, _ = suite.collectionService.Like(a.Slug, l2.ID)
	_, _ = suite.collectionService.Like(b.Slug, l1.ID)
	// c gets no likes.

	counts := suite.collectionService.batchCountLikes([]uint{a.ID, b.ID, c.ID})
	suite.Equal(2, counts[a.ID])
	suite.Equal(1, counts[b.ID])
	suite.Equal(0, counts[c.ID]) // missing key returns zero, which is correct
}

// TestBatchCheckUserLikes returns the correct set of liked collection IDs.
func (suite *CollectionServiceIntegrationTestSuite) TestBatchCheckUserLikes() {
	creator := suite.createTestUser("Creator")
	a := suite.createPublicCollection(creator, "A")
	b := suite.createPublicCollection(creator, "B")
	c := suite.createPublicCollection(creator, "C")

	user := suite.createTestUser("Viewer")
	_, _ = suite.collectionService.Like(a.Slug, user.ID)
	_, _ = suite.collectionService.Like(c.Slug, user.ID)

	result := suite.collectionService.batchCheckUserLikes(user.ID, []uint{a.ID, b.ID, c.ID})
	suite.True(result[a.ID])
	suite.False(result[b.ID])
	suite.True(result[c.ID])

	// Anonymous viewer (userID == 0) returns empty.
	result = suite.collectionService.batchCheckUserLikes(0, []uint{a.ID, b.ID, c.ID})
	suite.Empty(result)
}

// =============================================================================
// Group 15 (PSY-356): Public-visibility quality gates
// =============================================================================
//
// The gate has two halves: items_count >= 3 AND CHAR_LENGTH(description) >= 50.
// It applies in two places:
//   1. ListCollections(PublicOnly=true) — browse filter.
//   2. CreateCollection / UpdateCollection — forward gate at private→public
//      transitions (and at create-time when IsPublic=true is requested).
//
// The user's own library (GetUserCollections) intentionally does NOT filter
// by the gate — curators must be able to see their own under-gate
// collections to know what's missing.

// gateSeedItems forces a private collection past the items half of the gate
// by adding `count` artist items. Returns the slug, which may have changed
// if the caller passes title-update later.
func (suite *CollectionServiceIntegrationTestSuite) gateSeedItems(slug string, userID uint, count int) {
	for i := 0; i < count; i++ {
		artist := suite.createTestArtist(fmt.Sprintf("seed-%s-%d-%d", slug, i, time.Now().UnixNano()))
		_, err := suite.collectionService.AddItem(slug, userID, &contracts.AddCollectionItemRequest{
			EntityType: communitym.CollectionEntityArtist,
			EntityID:   artist.ID,
		})
		suite.Require().NoError(err)
	}
}

func (suite *CollectionServiceIntegrationTestSuite) TestPublicOnly_ExcludesBelowItemThreshold() {
	user := suite.createTestUser("BrowseGateItems")
	// Two items + good description → fails on items.
	priv := suite.createBareCollection(user, "Two Items Only")
	suite.gateSeedItems(priv.Slug, user.ID, MinPublicCollectionItems-1)

	// Force is_public=true behind the back of the service to simulate a
	// grandfathered (pre-PSY-356) row that the gate must drop from browse.
	suite.Require().NoError(suite.db.Model(&communitym.Collection{}).
		Where("id = ?", priv.ID).
		Updates(map[string]interface{}{
			"is_public":   true,
			"description": strings.Repeat("x", MinPublicCollectionDescriptionChars),
		}).Error)

	resp, total, err := suite.collectionService.ListCollections(contracts.CollectionFilters{PublicOnly: true}, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(0), total, "below-items collection must drop from browse")
	suite.Len(resp, 0)
}

func (suite *CollectionServiceIntegrationTestSuite) TestPublicOnly_ExcludesBelowDescriptionThreshold() {
	user := suite.createTestUser("BrowseGateDesc")
	priv := suite.createBareCollection(user, "Three Items No Desc")
	suite.gateSeedItems(priv.Slug, user.ID, MinPublicCollectionItems)

	// Grandfather + zero-length description.
	suite.Require().NoError(suite.db.Model(&communitym.Collection{}).
		Where("id = ?", priv.ID).
		Updates(map[string]interface{}{
			"is_public":   true,
			"description": "",
		}).Error)

	resp, total, err := suite.collectionService.ListCollections(contracts.CollectionFilters{PublicOnly: true}, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(0), total, "empty-description collection must drop from browse")
	suite.Len(resp, 0)
}

func (suite *CollectionServiceIntegrationTestSuite) TestPublicOnly_IncludesGatePassing() {
	user := suite.createTestUser("BrowseGatePass")
	// createPublicCollection satisfies the gate (3 items + 50+ char desc + flips public).
	suite.createPublicCollection(user, "Passes The Gate")

	resp, total, err := suite.collectionService.ListCollections(contracts.CollectionFilters{PublicOnly: true}, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("Passes The Gate", resp[0].Title)
}

// TestUserLibrary_NotFilteredByGate ensures the curator's own library
// surfaces under-gate collections — the curator MUST see them to fix them.
func (suite *CollectionServiceIntegrationTestSuite) TestUserLibrary_NotFilteredByGate() {
	user := suite.createTestUser("LibraryOwner")
	suite.createBareCollection(user, "Below Gate Library")

	resp, total, err := suite.collectionService.GetUserCollections(user.ID, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total, "user's own library must include under-gate collections")
	suite.Require().Len(resp, 1)
	suite.Equal("Below Gate Library", resp[0].Title)
}

func (suite *CollectionServiceIntegrationTestSuite) TestCreateCollection_PublicAtCreateRejected() {
	user := suite.createTestUser("CreatePublicReject")
	desc := strings.Repeat("d", MinPublicCollectionDescriptionChars)
	resp, err := suite.collectionService.CreateCollection(user.ID, &contracts.CreateCollectionRequest{
		Title:       "Public At Create",
		Description: &desc,
		IsPublic:    true,
	})
	suite.Require().Error(err)
	suite.Nil(resp)

	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionInvalidRequest, collErr.Code)
	// Item count is 0 at create time, so the items half of the gate fires.
	suite.Contains(collErr.Message, "at least 3 items")
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_PrivateToPublic_RejectsBelowItems() {
	user := suite.createTestUser("FlipItemReject")
	priv := suite.createBareCollection(user, "Flip Below Items")
	desc := strings.Repeat("d", MinPublicCollectionDescriptionChars)
	pub := true
	resp, err := suite.collectionService.UpdateCollection(priv.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Description: &desc,
		IsPublic:    &pub,
	})
	suite.Require().Error(err)
	suite.Nil(resp)

	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionInvalidRequest, collErr.Code)
	suite.Contains(collErr.Message, "at least 3 items")
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_PrivateToPublic_RejectsBelowDescription() {
	user := suite.createTestUser("FlipDescReject")
	priv := suite.createBareCollection(user, "Flip Below Desc")
	suite.gateSeedItems(priv.Slug, user.ID, MinPublicCollectionItems)

	pub := true
	resp, err := suite.collectionService.UpdateCollection(priv.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		IsPublic: &pub, // no description in patch; current description is empty.
	})
	suite.Require().Error(err)
	suite.Nil(resp)

	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionInvalidRequest, collErr.Code)
	suite.Contains(collErr.Message, "50 characters")
}

func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_PrivateToPublic_AcceptsWhenGatePasses() {
	user := suite.createTestUser("FlipAccept")
	priv := suite.createBareCollection(user, "Flip Pass")
	suite.gateSeedItems(priv.Slug, user.ID, MinPublicCollectionItems)

	desc := strings.Repeat("d", MinPublicCollectionDescriptionChars)
	pub := true
	resp, err := suite.collectionService.UpdateCollection(priv.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Description: &desc,
		IsPublic:    &pub,
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.True(resp.IsPublic)
}

// TestUpdateCollection_PublicToPrivate_AlwaysAllowed: even when the
// collection is below the gate (e.g., grandfathered), reverting to private
// must succeed without re-running the gate.
func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_PublicToPrivate_AlwaysAllowed() {
	user := suite.createTestUser("UnpublishOwner")
	// Grandfather a public-but-below-gate row directly.
	priv := suite.createBareCollection(user, "Grandfathered")
	suite.Require().NoError(suite.db.Model(&communitym.Collection{}).
		Where("id = ?", priv.ID).
		Update("is_public", true).Error)

	pub := false
	resp, err := suite.collectionService.UpdateCollection(priv.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		IsPublic: &pub,
	})
	suite.Require().NoError(err)
	suite.False(resp.IsPublic)
}

// TestUpdateCollection_GrandfatheredEditableWithoutGate: a public-but-below-
// gate collection can still be edited (e.g., title change) without the patch
// triggering gate validation.
func (suite *CollectionServiceIntegrationTestSuite) TestUpdateCollection_GrandfatheredEditableWithoutGate() {
	user := suite.createTestUser("GrandfatheredEditor")
	priv := suite.createBareCollection(user, "Edit Me")
	suite.Require().NoError(suite.db.Model(&communitym.Collection{}).
		Where("id = ?", priv.ID).
		Update("is_public", true).Error)

	newTitle := "Edited Title"
	resp, err := suite.collectionService.UpdateCollection(priv.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Title: &newTitle,
	})
	suite.Require().NoError(err)
	suite.Equal("Edited Title", resp.Title)
	suite.True(resp.IsPublic, "grandfathered row stays public")
}

// TestCloneCollection_AutoPassesGateInBrowse: a clone inherits items +
// description from the source, so it satisfies the gate without special-
// casing in CloneCollection.
func (suite *CollectionServiceIntegrationTestSuite) TestCloneCollection_AutoPassesGateInBrowse() {
	owner := suite.createTestUser("CloneSource")
	cloner := suite.createTestUser("Cloner")

	src := suite.createPublicCollection(owner, "Source Coll")
	cloned, err := suite.collectionService.CloneCollection(src.Slug, cloner.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(cloned)
	suite.True(cloned.IsPublic)

	// Both should appear in PublicOnly browse.
	resp, total, err := suite.collectionService.ListCollections(contracts.CollectionFilters{PublicOnly: true}, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total, "source + clone both pass the gate")
	suite.Len(resp, 2)
}

// =============================================================================
// PSY-354: Collection tags
// =============================================================================
//
// Tags reuse the polymorphic entity_tags table. Coverage:
//   - Add by free-form name creates the tag inline + applies it.
//   - Add reuses an existing tag when one is found by name.
//   - Add enforces MaxCollectionTags (rejects 11th).
//   - Permission rule: creator OR collaborative-and-authenticated; otherwise 403.
//   - Remove unapplies the tag and decrements usage_count.
//   - Detail / list responses surface tags.
//   - ListCollections accepts ?tag=<slug> and filters correctly.

// promoteContributor flips a test user's tier to "contributor" so the tag
// service's createTagInline gate (new_user → 403) doesn't reject the test
// path. createTestUser doesn't set UserTier, so the DB default ("new_user")
// applies — identical to the dogfooded gate, which we want to side-step
// for the bulk of these tests since the trust-tier gate is covered in
// catalog/tag_service_test.go.
func (suite *CollectionServiceIntegrationTestSuite) promoteContributor(user *authm.User) {
	suite.Require().NoError(suite.db.Model(&authm.User{}).
		Where("id = ?", user.ID).
		Update("user_tier", "contributor").Error)
}

// TestAddTagToCollection_ByName_HappyPath creates the tag inline and surfaces
// it on the post-mutation response.
func (suite *CollectionServiceIntegrationTestSuite) TestAddTagToCollection_ByName_HappyPath() {
	creator := suite.createTestUser("TagCreator")
	suite.promoteContributor(creator)
	coll := suite.createBasicCollection(creator, "Tagged Collection")

	resp, err := suite.collectionService.AddTagToCollection(coll.Slug, creator.ID,
		&contracts.AddCollectionTagRequest{TagName: "best-of-2026"})
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Tags, 1)
	suite.Equal("best-of-2026", resp.Tags[0].Name)
	suite.Equal("other", resp.Tags[0].Category, "default category for new collection tags is 'other'")
}

// TestAddTagToCollection_ByID applies an existing tag without creating a new one.
func (suite *CollectionServiceIntegrationTestSuite) TestAddTagToCollection_ByID() {
	creator := suite.createTestUser("TagCreator")
	suite.promoteContributor(creator)
	coll := suite.createBasicCollection(creator, "By ID Collection")

	// Pre-create a tag.
	tag, err := suite.tagService.CreateTag("phoenix", nil, nil, catalogm.TagCategoryLocale, false, &creator.ID)
	suite.Require().NoError(err)

	resp, err := suite.collectionService.AddTagToCollection(coll.Slug, creator.ID,
		&contracts.AddCollectionTagRequest{TagID: tag.ID})
	suite.Require().NoError(err)
	suite.Require().Len(resp.Tags, 1)
	suite.Equal(tag.ID, resp.Tags[0].TagID)
	suite.Equal("phoenix", resp.Tags[0].Name)
	suite.Equal("locale", resp.Tags[0].Category, "category preserved when applying an existing tag")
}

// TestAddTagToCollection_DefaultCategory_OtherForFreeForm verifies that a
// free-form tag name without a category gets "other" rather than picking
// up "genre" by default — the rest of the tag system defaults to genre,
// but collection meta-tags rarely fit that taxonomy.
func (suite *CollectionServiceIntegrationTestSuite) TestAddTagToCollection_DefaultCategory_OtherForFreeForm() {
	creator := suite.createTestUser("TagCreator")
	suite.promoteContributor(creator)
	coll := suite.createBasicCollection(creator, "Default Category Test")

	resp, err := suite.collectionService.AddTagToCollection(coll.Slug, creator.ID,
		&contracts.AddCollectionTagRequest{TagName: "post-show-essentials"})
	suite.Require().NoError(err)
	suite.Require().Len(resp.Tags, 1)
	suite.Equal("other", resp.Tags[0].Category)
}

// TestAddTagToCollection_MaxLimit_Rejects11th hits the cap and verifies the
// 400 carries the cap + current count.
func (suite *CollectionServiceIntegrationTestSuite) TestAddTagToCollection_MaxLimit_Rejects11th() {
	creator := suite.createTestUser("CapCreator")
	suite.promoteContributor(creator)
	coll := suite.createBasicCollection(creator, "Capped Collection")

	for i := 0; i < contracts.MaxCollectionTags; i++ {
		_, err := suite.collectionService.AddTagToCollection(coll.Slug, creator.ID,
			&contracts.AddCollectionTagRequest{TagName: fmt.Sprintf("cap-tag-%d", i)})
		suite.Require().NoError(err, "failed adding tag %d", i)
	}

	_, err := suite.collectionService.AddTagToCollection(coll.Slug, creator.ID,
		&contracts.AddCollectionTagRequest{TagName: "one-too-many"})
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionTagLimitExceeded, collErr.Code)
	suite.Contains(collErr.Message, "10 tags")
}

// TestAddTagToCollection_NonOwner_NonCollaborative_Rejected covers the
// permission gate: a non-creator on a non-collaborative collection cannot
// add tags. PSY-354. createBasicCollection's GORM-bool dance lands the
// collection as Collaborative=false by default, which is the state we want.
func (suite *CollectionServiceIntegrationTestSuite) TestAddTagToCollection_NonOwner_NonCollaborative_Rejected() {
	creator := suite.createTestUser("Owner")
	stranger := suite.createTestUser("Stranger")
	suite.promoteContributor(stranger)

	coll := suite.createBasicCollection(creator, "Solo Curator")
	suite.Require().False(coll.Collaborative, "expected default Collaborative=false from createBasicCollection")

	_, err := suite.collectionService.AddTagToCollection(coll.Slug, stranger.ID,
		&contracts.AddCollectionTagRequest{TagName: "intruder-tag"})
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

// TestAddTagToCollection_Collaborator_Allowed verifies the open path: any
// authenticated user can tag a collaborative collection. createBasicCollection
// defaults to Collaborative=false (per CreateCollection's GORM-bool dance);
// flip it explicitly with UpdateCollection so the test exercises the
// collaborative branch of canEditCollectionTags.
func (suite *CollectionServiceIntegrationTestSuite) TestAddTagToCollection_Collaborator_Allowed() {
	creator := suite.createTestUser("Owner")
	collaborator := suite.createTestUser("Helper")
	suite.promoteContributor(collaborator)

	coll := suite.createBasicCollection(creator, "Collab Curator")
	collab := true
	_, err := suite.collectionService.UpdateCollection(coll.Slug, creator.ID, false,
		&contracts.UpdateCollectionRequest{Collaborative: &collab})
	suite.Require().NoError(err)

	resp, err := suite.collectionService.AddTagToCollection(coll.Slug, collaborator.ID,
		&contracts.AddCollectionTagRequest{TagName: "community-pick"})
	suite.Require().NoError(err)
	suite.Require().Len(resp.Tags, 1)
	suite.Equal("community-pick", resp.Tags[0].Name)
}

// TestAddTagToCollection_NotFound returns 404-shaped error.
func (suite *CollectionServiceIntegrationTestSuite) TestAddTagToCollection_NotFound() {
	user := suite.createTestUser("AnyUser")
	suite.promoteContributor(user)
	_, err := suite.collectionService.AddTagToCollection("does-not-exist-slug", user.ID,
		&contracts.AddCollectionTagRequest{TagName: "tag"})
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionNotFound, collErr.Code)
}

// TestAddTagToCollection_MissingArgs rejects bodies without tag_id or tag_name.
func (suite *CollectionServiceIntegrationTestSuite) TestAddTagToCollection_MissingArgs() {
	creator := suite.createTestUser("Owner")
	suite.promoteContributor(creator)
	coll := suite.createBasicCollection(creator, "Missing Args")

	_, err := suite.collectionService.AddTagToCollection(coll.Slug, creator.ID,
		&contracts.AddCollectionTagRequest{})
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionInvalidRequest, collErr.Code)
}

// TestRemoveTagFromCollection_Success removes the application and the tag
// disappears from the detail response.
func (suite *CollectionServiceIntegrationTestSuite) TestRemoveTagFromCollection_Success() {
	creator := suite.createTestUser("Owner")
	suite.promoteContributor(creator)
	coll := suite.createBasicCollection(creator, "Remove Tag Test")

	resp, err := suite.collectionService.AddTagToCollection(coll.Slug, creator.ID,
		&contracts.AddCollectionTagRequest{TagName: "to-be-removed"})
	suite.Require().NoError(err)
	suite.Require().Len(resp.Tags, 1)
	tagID := resp.Tags[0].TagID

	suite.Require().NoError(suite.collectionService.RemoveTagFromCollection(coll.Slug, tagID, creator.ID))

	detail, err := suite.collectionService.GetBySlug(coll.Slug, creator.ID)
	suite.Require().NoError(err)
	suite.Empty(detail.Tags, "removed tag must drop from the detail response")
}

// TestRemoveTagFromCollection_NonOwner_Rejected mirrors the add gate.
func (suite *CollectionServiceIntegrationTestSuite) TestRemoveTagFromCollection_NonOwner_Rejected() {
	creator := suite.createTestUser("Owner")
	stranger := suite.createTestUser("Stranger")
	suite.promoteContributor(creator)

	coll := suite.createBasicCollection(creator, "Solo Curator Removal")
	suite.Require().False(coll.Collaborative, "expected default Collaborative=false")

	resp, err := suite.collectionService.AddTagToCollection(coll.Slug, creator.ID,
		&contracts.AddCollectionTagRequest{TagName: "owner-only"})
	suite.Require().NoError(err)
	tagID := resp.Tags[0].TagID

	err = suite.collectionService.RemoveTagFromCollection(coll.Slug, tagID, stranger.ID)
	suite.Require().Error(err)
	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

// TestGetBySlug_PopulatesTags verifies tags surface on the detail response.
func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_PopulatesTags() {
	creator := suite.createTestUser("Curator")
	suite.promoteContributor(creator)
	coll := suite.createBasicCollection(creator, "Detail With Tags")

	for _, name := range []string{"genre-foo", "vibe-bar"} {
		_, err := suite.collectionService.AddTagToCollection(coll.Slug, creator.ID,
			&contracts.AddCollectionTagRequest{TagName: name})
		suite.Require().NoError(err)
	}

	detail, err := suite.collectionService.GetBySlug(coll.Slug, creator.ID)
	suite.Require().NoError(err)
	suite.Require().Len(detail.Tags, 2)

	names := []string{detail.Tags[0].Name, detail.Tags[1].Name}
	suite.Contains(names, "genre-foo")
	suite.Contains(names, "vibe-bar")
}

// TestListCollections_PopulatesTagSummaries verifies tag chips appear on
// list cards.
func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_PopulatesTagSummaries() {
	creator := suite.createTestUser("Curator")
	suite.promoteContributor(creator)
	coll := suite.createPublicCollection(creator, "List With Tags")

	_, err := suite.collectionService.AddTagToCollection(coll.Slug, creator.ID,
		&contracts.AddCollectionTagRequest{TagName: "card-tag"})
	suite.Require().NoError(err)

	resp, _, err := suite.collectionService.ListCollections(contracts.CollectionFilters{PublicOnly: true}, 20, 0)
	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Require().Len(resp[0].Tags, 1)
	suite.Equal("card-tag", resp[0].Tags[0].Name)
}

// TestListCollections_FilterByTag returns only collections matching the
// given tag slug.
func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_FilterByTag() {
	creator := suite.createTestUser("Curator")
	suite.promoteContributor(creator)

	tagged := suite.createPublicCollection(creator, "Tagged List")
	suite.createPublicCollection(creator, "Untagged List")

	addResp, err := suite.collectionService.AddTagToCollection(tagged.Slug, creator.ID,
		&contracts.AddCollectionTagRequest{TagName: "indie-2026"})
	suite.Require().NoError(err)
	suite.Require().Len(addResp.Tags, 1)

	resp, total, err := suite.collectionService.ListCollections(
		contracts.CollectionFilters{PublicOnly: true, Tag: "indie-2026"}, 20, 0,
	)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal(tagged.ID, resp[0].ID)
}

// TestListCollections_FilterByTag_Unknown returns empty when no collection
// has the requested tag.
func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_FilterByTag_Unknown() {
	creator := suite.createTestUser("Curator")
	suite.createPublicCollection(creator, "Some Collection")

	resp, total, err := suite.collectionService.ListCollections(
		contracts.CollectionFilters{PublicOnly: true, Tag: "no-such-tag-slug-xyz"}, 20, 0,
	)
	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(resp)
}
