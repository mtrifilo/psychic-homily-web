package user

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	engagementm "psychic-homily-backend/internal/models/engagement"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// assertProfileSectionNotFound asserts err is the typed section-not-found
// error. Wrong-owner is surfaced as not-found so we don't leak existence of
// another user's section.
func (suite *ContributorProfileServiceIntegrationTestSuite) assertProfileSectionNotFound(err error) {
	suite.Require().Error(err)
	var profileErr *apperrors.ProfileError
	suite.Require().True(stderrors.As(err, &profileErr), "expected *apperrors.ProfileError, got %T", err)
	suite.Equal(apperrors.CodeProfileSectionNotFound, profileErr.Code)
}

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestValidatePrivacySettings(t *testing.T) {
	t.Run("Valid_Defaults", func(t *testing.T) {
		err := ValidatePrivacySettings(contracts.DefaultPrivacySettings())
		assert.NoError(t, err)
	})

	t.Run("Valid_AllVisible", func(t *testing.T) {
		ps := contracts.PrivacySettings{
			Contributions:   contracts.PrivacyVisible,
			SavedShows:      contracts.PrivacyVisible,
			Following:       contracts.PrivacyVisible,
			Collections:     contracts.PrivacyVisible,
			LastActive:      contracts.PrivacyVisible,
			ProfileSections: contracts.PrivacyVisible,
		}
		assert.NoError(t, ValidatePrivacySettings(ps))
	})

	t.Run("Valid_AllHidden", func(t *testing.T) {
		ps := contracts.PrivacySettings{
			Contributions:   contracts.PrivacyHidden,
			SavedShows:      contracts.PrivacyHidden,
			Following:       contracts.PrivacyHidden,
			Collections:     contracts.PrivacyHidden,
			LastActive:      contracts.PrivacyHidden,
			ProfileSections: contracts.PrivacyHidden,
		}
		assert.NoError(t, ValidatePrivacySettings(ps))
	})

	t.Run("Invalid_BadLevel", func(t *testing.T) {
		ps := contracts.DefaultPrivacySettings()
		ps.Contributions = "invalid"
		err := ValidatePrivacySettings(ps)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid privacy level")
	})

	t.Run("Invalid_CountOnly_LastActive", func(t *testing.T) {
		ps := contracts.DefaultPrivacySettings()
		ps.LastActive = contracts.PrivacyCountOnly
		err := ValidatePrivacySettings(ps)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only supports 'visible' or 'hidden'")
	})

	t.Run("Invalid_CountOnly_ProfileSections", func(t *testing.T) {
		ps := contracts.DefaultPrivacySettings()
		ps.ProfileSections = contracts.PrivacyCountOnly
		err := ValidatePrivacySettings(ps)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only supports 'visible' or 'hidden'")
	})

	t.Run("Valid_CountOnly_Contributions", func(t *testing.T) {
		ps := contracts.DefaultPrivacySettings()
		ps.Contributions = contracts.PrivacyCountOnly
		assert.NoError(t, ValidatePrivacySettings(ps))
	})
}

// TestBuildSectionResponse_RendersContentHTML verifies PSY-747: the profile
// section response exposes Content rendered to sanitized HTML in ContentHTML
// while preserving the raw markdown in Content for edit round-tripping.
func TestBuildSectionResponse_RendersContentHTML(t *testing.T) {
	t.Run("RendersMarkdownToHTML", func(t *testing.T) {
		section := &authm.UserProfileSection{
			ID:      1,
			Title:   "About",
			Content: "I like **post-punk** and [shows](https://example.com).",
		}
		resp := buildSectionResponse(section)

		// Raw markdown preserved for edit forms.
		assert.Equal(t, "I like **post-punk** and [shows](https://example.com).", resp.Content)
		// Rendered HTML carries the bold + link markup.
		assert.Contains(t, resp.ContentHTML, "<strong>post-punk</strong>")
		assert.Contains(t, resp.ContentHTML, `href="https://example.com"`)
	})

	t.Run("SanitizesScriptTags", func(t *testing.T) {
		section := &authm.UserProfileSection{
			Content: "Hello <script>alert('xss')</script> world",
		}
		resp := buildSectionResponse(section)

		assert.NotContains(t, resp.ContentHTML, "<script>")
		assert.NotContains(t, resp.ContentHTML, "alert('xss')")
	})

	t.Run("EmptyContentYieldsEmptyHTML", func(t *testing.T) {
		resp := buildSectionResponse(&authm.UserProfileSection{Content: ""})
		assert.Empty(t, resp.ContentHTML)
	})
}

// TestRenderBioHTML verifies the profile bio is rendered to sanitized HTML using
// the same goldmark + bluemonday policy as profile sections, so the bio renders
// as markdown on the public profile instead of showing raw source.
func TestRenderBioHTML(t *testing.T) {
	t.Run("RendersMarkdownToHTML", func(t *testing.T) {
		bio := "I like **post-punk**, [shows](https://example.com), and:\n\n* one\n* two"
		html := renderBioHTML(&bio)

		assert.Contains(t, html, "<strong>post-punk</strong>")
		assert.Contains(t, html, `href="https://example.com"`)
		assert.Contains(t, html, "<li>one</li>")
	})

	t.Run("SanitizesScriptTags", func(t *testing.T) {
		bio := "Hello <script>alert('xss')</script> world"
		html := renderBioHTML(&bio)

		assert.NotContains(t, html, "<script>")
		assert.NotContains(t, html, "alert('xss')")
	})

	t.Run("RendersAllHeadingLevels", func(t *testing.T) {
		// All heading levels render (h1-h6) so `#`/`##` behave as users expect,
		// matching sections, comments, and field notes.
		bio := "# Wow\n\n## whoah!\n\n### small"
		html := renderBioHTML(&bio)

		assert.Contains(t, html, "<h1>Wow</h1>")
		assert.Contains(t, html, "<h2>whoah!</h2>")
		assert.Contains(t, html, "<h3>small</h3>")
	})

	t.Run("NilBioYieldsEmptyHTML", func(t *testing.T) {
		assert.Empty(t, renderBioHTML(nil))
	})

	t.Run("EmptyBioYieldsEmptyHTML", func(t *testing.T) {
		empty := ""
		assert.Empty(t, renderBioHTML(&empty))
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ContributorProfileServiceIntegrationTestSuite struct {
	suite.Suite
	testDB         *testutil.TestDatabase
	db             *gorm.DB
	profileService *ContributorProfileService
	auditLog       *adminsvc.AuditLogService
}

func (suite *ContributorProfileServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.profileService = &ContributorProfileService{db: suite.testDB.DB}
	suite.auditLog = adminsvc.NewAuditLogService(suite.testDB.DB)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM user_profile_sections")
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM pending_entity_edits")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationship_votes")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationships")
	_, _ = sqlDB.Exec("DELETE FROM request_votes")
	_, _ = sqlDB.Exec("DELETE FROM requests")
	_, _ = sqlDB.Exec("DELETE FROM collection_subscribers")
	_, _ = sqlDB.Exec("DELETE FROM collection_items")
	_, _ = sqlDB.Exec("DELETE FROM collections")
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	_, _ = sqlDB.Exec("DELETE FROM entity_reports")
	_, _ = sqlDB.Exec("DELETE FROM show_reports")
	_, _ = sqlDB.Exec("DELETE FROM artist_reports")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM entity_requests")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestContributorProfileServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ContributorProfileServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) createTestUser(username string) *authm.User {
	user := &authm.User{
		Email:             stringPtr(fmt.Sprintf("%s-%d@test.com", username, time.Now().UnixNano())),
		Username:          stringPtr(username),
		FirstName:         stringPtr("Test"),
		LastName:          stringPtr("User"),
		Bio:               stringPtr("Music enthusiast"),
		ProfileVisibility: "public",
		IsActive:          true,
		EmailVerified:     true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *ContributorProfileServiceIntegrationTestSuite) createPrivateUser(username string) *authm.User {
	user := suite.createTestUser(username)
	err := suite.db.Model(user).Update("profile_visibility", "private").Error
	suite.Require().NoError(err)
	user.ProfileVisibility = "private"
	return user
}

func (suite *ContributorProfileServiceIntegrationTestSuite) createShow(submittedBy uint, title string) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		SubmittedBy: &submittedBy,
		Status:      "approved",
		EventDate:   time.Now(),
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *ContributorProfileServiceIntegrationTestSuite) createVenue(submittedBy uint, name string) *catalogm.Venue {
	venue := &catalogm.Venue{
		Name:        name,
		City:        "Phoenix",
		State:       "AZ",
		SubmittedBy: &submittedBy,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *ContributorProfileServiceIntegrationTestSuite) setPrivacySettings(userID uint, ps contracts.PrivacySettings) {
	raw, err := json.Marshal(ps)
	suite.Require().NoError(err)
	rawMsg := json.RawMessage(raw)
	err = suite.db.Model(&authm.User{}).Where("id = ?", userID).Update("privacy_settings", &rawMsg).Error
	suite.Require().NoError(err)
}

// =============================================================================
// Group 1: GetPublicProfile
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_Success() {
	user := suite.createTestUser("contributor1")

	profile, err := suite.profileService.GetPublicProfile("contributor1", contracts.ShowViewer{})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal("contributor1", profile.Username)
	suite.Equal("Music enthusiast", *profile.Bio)
	// Bio is also exposed as server-sanitized HTML for markdown rendering.
	suite.Contains(profile.BioHTML, "Music enthusiast")
	suite.Equal("Test", *profile.FirstName)
	suite.Equal("public", profile.ProfileVisibility)
	suite.Equal(user.CreatedAt.Unix(), profile.JoinedAt.Unix())
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_NotFound() {
	profile, err := suite.profileService.GetPublicProfile("nonexistent", contracts.ShowViewer{})

	suite.Require().NoError(err)
	suite.Nil(profile)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_PrivateProfile_Anonymous() {
	suite.createPrivateUser("privateperson")

	profile, err := suite.profileService.GetPublicProfile("privateperson", contracts.ShowViewer{})

	suite.Require().NoError(err)
	suite.Nil(profile, "Private profiles should not be visible to anonymous users")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_PrivateProfile_OtherUser() {
	suite.createPrivateUser("privateperson2")
	otherUser := suite.createTestUser("otheruser")

	profile, err := suite.profileService.GetPublicProfile("privateperson2", contracts.ShowViewer{UserID: otherUser.ID})

	suite.Require().NoError(err)
	suite.Nil(profile, "Private profiles should not be visible to other users")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_PrivateProfile_Owner() {
	owner := suite.createPrivateUser("privateperson3")

	profile, err := suite.profileService.GetPublicProfile("privateperson3", contracts.ShowViewer{UserID: owner.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile, "Private profiles should be visible to the owner")
	suite.Equal("privateperson3", profile.Username)
	suite.Equal("private", profile.ProfileVisibility)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_IncludesStats() {
	user := suite.createTestUser("statsuser")
	suite.createShow(user.ID, "Show 1")
	suite.createShow(user.ID, "Show 2")
	suite.createVenue(user.ID, "Test Venue")

	profile, err := suite.profileService.GetPublicProfile("statsuser", contracts.ShowViewer{})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal(int64(2), profile.Stats.ShowsSubmitted)
	suite.Equal(int64(1), profile.Stats.VenuesSubmitted)
	suite.Equal(int64(3), profile.Stats.TotalContributions)
}

// =============================================================================
// Group 2: GetOwnProfile
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnProfile_Success() {
	user := suite.createTestUser("ownprofile")

	profile, err := suite.profileService.GetOwnProfile(contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal("ownprofile", profile.Username)
	// Owner profile also exposes the bio as server-sanitized HTML.
	suite.Contains(profile.BioHTML, "Music enthusiast")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnProfile_PrivateBypassesVisibility() {
	user := suite.createPrivateUser("ownprivate")

	profile, err := suite.profileService.GetOwnProfile(contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile, "GetOwnProfile should always work regardless of visibility")
	suite.Equal("ownprivate", profile.Username)
	suite.Equal("private", profile.ProfileVisibility)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnProfile_NotFound() {
	profile, err := suite.profileService.GetOwnProfile(contracts.ShowViewer{UserID: 99999})

	suite.Require().NoError(err)
	suite.Nil(profile)
}

// =============================================================================
// Group 3: GetContributionStats
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_Empty() {
	user := suite.createTestUser("emptystats")

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(stats)
	suite.Equal(int64(0), stats.TotalContributions)
	suite.Equal(int64(0), stats.ShowsSubmitted)
	suite.Equal(int64(0), stats.VenuesSubmitted)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ShowsAndVenues() {
	user := suite.createTestUser("showvenueuser")
	suite.createShow(user.ID, "Show A")
	suite.createShow(user.ID, "Show B")
	suite.createShow(user.ID, "Show C")
	suite.createVenue(user.ID, "Venue A")

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(3), stats.ShowsSubmitted)
	suite.Equal(int64(1), stats.VenuesSubmitted)
	suite.Equal(int64(4), stats.TotalContributions)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_AuditLogActions() {
	user := suite.createTestUser("audituser")

	// Content creation actions stay in audit_logs.
	suite.auditLog.LogAction(user.ID, "create_release", "release", 1, nil)
	suite.auditLog.LogAction(user.ID, "create_release", "release", 2, nil)
	suite.auditLog.LogAction(user.ID, "create_label", "label", 1, nil)
	// PSY-618: edit events live in entity_edit_audit_logs now.
	suite.auditLog.LogEntityEdit(user.ID, "artist", 1, nil)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.ReleasesCreated)
	suite.Equal(int64(1), stats.LabelsCreated)
	suite.Equal(int64(1), stats.ArtistsEdited)
	suite.Equal(int64(4), stats.TotalContributions)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ModerationActions() {
	user := suite.createTestUser("moderator")

	// REAL shows, because the audit arm is now gated: a row naming a show that
	// does not exist fails closed, exactly as the same row does in the
	// contributions timeline this count sits beside.
	submitter := suite.createTestUser("moderatedsubmitter")
	approved := suite.createShow(submitter.ID, "Approved Show")
	alsoApproved := suite.createShow(submitter.ID, "Another Approved Show")
	venue := suite.createVenue(submitter.ID, "Moderated Venue")

	suite.auditLog.LogAction(user.ID, "approve_show", "show", approved.ID, nil)
	suite.auditLog.LogAction(user.ID, "reject_show", "show", alsoApproved.ID, nil)
	suite.auditLog.LogAction(user.ID, "verify_venue", "venue", venue.ID, nil)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(3), stats.ModerationActions)
	suite.Equal(int64(3), stats.TotalContributions)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_MixedSources() {
	user := suite.createTestUser("mixeduser")

	// Submissions
	suite.createShow(user.ID, "My Show")
	suite.createVenue(user.ID, "My Venue")

	// Audit actions. The show-typed row names a real approved show, because the
	// audit arm withholds a row naming a show nobody can resolve. Submitted by
	// somebody else, so it moderates rather than inflating shows_submitted.
	moderated := suite.createShow(suite.createTestUser("mixedsubmitter").ID, "Moderated Show")
	suite.auditLog.LogAction(user.ID, "create_release", "release", 1, nil)
	suite.auditLog.LogAction(user.ID, "approve_show", "show", moderated.ID, nil)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.ShowsSubmitted)
	suite.Equal(int64(1), stats.VenuesSubmitted)
	suite.Equal(int64(1), stats.ReleasesCreated)
	suite.Equal(int64(1), stats.ModerationActions)
	suite.Equal(int64(4), stats.TotalContributions)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_DoesNotCountOtherUsers() {
	user1 := suite.createTestUser("user1stats")
	user2 := suite.createTestUser("user2stats")

	suite.createShow(user1.ID, "User1 Show")
	suite.createShow(user2.ID, "User2 Show")
	suite.auditLog.LogAction(user2.ID, "create_release", "release", 1, nil)

	stats, err := suite.profileService.GetContributionStats(user1.ID, contracts.ShowViewer{UserID: user1.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.ShowsSubmitted)
	suite.Equal(int64(0), stats.ReleasesCreated)
	suite.Equal(int64(1), stats.TotalContributions)
}

// =============================================================================
// Group 3b: GetContributionStats — Expanded Stat Types
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_TagVotes() {
	user := suite.createTestUser("tagvoter")

	// Create a tag
	tag := &catalogm.Tag{Name: "punk", Slug: "punk", Category: "genre"}
	suite.Require().NoError(suite.db.Create(tag).Error)

	// Create an artist to tag-vote on
	artist := &catalogm.Artist{Name: "Bad Brains"}
	suite.Require().NoError(suite.db.Create(artist).Error)

	// Cast tag votes
	suite.Require().NoError(suite.db.Create(&catalogm.TagVote{
		TagID: tag.ID, EntityType: "artist", EntityID: artist.ID, UserID: user.ID, Vote: 1,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.TagVotesCast)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_RelationshipVotes() {
	user := suite.createTestUser("relvoter")

	// Create two artists for relationship
	artist1 := &catalogm.Artist{Name: "Artist A"}
	artist2 := &catalogm.Artist{Name: "Artist B"}
	suite.Require().NoError(suite.db.Create(artist1).Error)
	suite.Require().NoError(suite.db.Create(artist2).Error)

	source, target := catalogm.CanonicalOrder(artist1.ID, artist2.ID)

	// Create relationship
	suite.Require().NoError(suite.db.Create(&catalogm.ArtistRelationship{
		SourceArtistID: source, TargetArtistID: target,
		RelationshipType: catalogm.RelationshipTypeSimilar,
	}).Error)

	// Cast vote
	suite.Require().NoError(suite.db.Create(&catalogm.ArtistRelationshipVote{
		SourceArtistID: source, TargetArtistID: target,
		RelationshipType: catalogm.RelationshipTypeSimilar,
		UserID:           user.ID, Direction: 1,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.RelationshipVotesCast)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_RequestVotes() {
	user := suite.createTestUser("reqvoter")
	requester := suite.createTestUser("requester")

	// Create a request
	request := &communitym.Request{
		Title: "Add new band", EntityType: "artist",
		RequesterID: requester.ID, Status: communitym.RequestStatusPending,
	}
	suite.Require().NoError(suite.db.Create(request).Error)

	// Cast votes
	suite.Require().NoError(suite.db.Create(&communitym.RequestVote{
		RequestID: request.ID, UserID: user.ID, Vote: 1,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.RequestVotesCast)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_CollectionItems() {
	user := suite.createTestUser("collector")

	// Create a collection
	collection := &communitym.Collection{
		Title: "My Favorites", Slug: fmt.Sprintf("my-favorites-%d", time.Now().UnixNano()),
		CreatorID: user.ID,
	}
	suite.Require().NoError(suite.db.Create(collection).Error)

	// Add items
	suite.Require().NoError(suite.db.Create(&communitym.CollectionItem{
		CollectionID: collection.ID, EntityType: "artist", EntityID: 1,
		AddedByUserID: user.ID,
	}).Error)
	suite.Require().NoError(suite.db.Create(&communitym.CollectionItem{
		CollectionID: collection.ID, EntityType: "release", EntityID: 2,
		AddedByUserID: user.ID,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.CollectionItemsAdded)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_CollectionSubscriptions() {
	user := suite.createTestUser("subscriber")
	creator := suite.createTestUser("creator")

	// Create collections
	col1 := &communitym.Collection{
		Title: "Coll 1", Slug: fmt.Sprintf("coll-1-%d", time.Now().UnixNano()),
		CreatorID: creator.ID,
	}
	col2 := &communitym.Collection{
		Title: "Coll 2", Slug: fmt.Sprintf("coll-2-%d", time.Now().UnixNano()),
		CreatorID: creator.ID,
	}
	suite.Require().NoError(suite.db.Create(col1).Error)
	suite.Require().NoError(suite.db.Create(col2).Error)

	// Subscribe
	suite.Require().NoError(suite.db.Create(&communitym.CollectionSubscriber{
		CollectionID: col1.ID, UserID: user.ID,
	}).Error)
	suite.Require().NoError(suite.db.Create(&communitym.CollectionSubscriber{
		CollectionID: col2.ID, UserID: user.ID,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.CollectionSubscriptions)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_Revisions() {
	user := suite.createTestUser("reviser")

	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&adminm.Revision{
		EntityType: "artist", EntityID: 1, UserID: user.ID,
		FieldChanges: &fieldChanges,
	}).Error)
	suite.Require().NoError(suite.db.Create(&adminm.Revision{
		EntityType: "venue", EntityID: 2, UserID: user.ID,
		FieldChanges: &fieldChanges,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.RevisionsMade)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_PendingEdits() {
	user := suite.createTestUser("pendinguser")

	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&adminm.PendingEntityEdit{
		EntityType: "artist", EntityID: 1, SubmittedBy: user.ID,
		FieldChanges: &fieldChanges, Summary: "Fix name",
		Status: adminm.PendingEditStatusPending,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.PendingEditsSubmitted)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ApprovalRate() {
	user := suite.createTestUser("approvaluser")

	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	// 3 approved, 1 rejected = 75% approval rate
	for i := 0; i < 3; i++ {
		suite.Require().NoError(suite.db.Create(&adminm.PendingEntityEdit{
			EntityType: "artist", EntityID: uint(i + 1), SubmittedBy: user.ID,
			FieldChanges: &fieldChanges, Summary: "Edit",
			Status: adminm.PendingEditStatusApproved,
		}).Error)
	}
	suite.Require().NoError(suite.db.Create(&adminm.PendingEntityEdit{
		EntityType: "venue", EntityID: 1, SubmittedBy: user.ID,
		FieldChanges: &fieldChanges, Summary: "Edit",
		Status: adminm.PendingEditStatusRejected,
	}).Error)
	// Pending edits should not affect rate
	suite.Require().NoError(suite.db.Create(&adminm.PendingEntityEdit{
		EntityType: "venue", EntityID: 2, SubmittedBy: user.ID,
		FieldChanges: &fieldChanges, Summary: "Edit",
		Status: adminm.PendingEditStatusPending,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Require().NotNil(stats.ApprovalRate)
	suite.InDelta(0.75, *stats.ApprovalRate, 0.001)
	suite.Equal(int64(5), stats.PendingEditsSubmitted)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ApprovalRate_NilWhenNone() {
	user := suite.createTestUser("noapproval")

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Nil(stats.ApprovalRate, "ApprovalRate should be nil when no approved/rejected edits exist")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ReportsFiled() {
	user := suite.createTestUser("reporter")

	// Entity report
	suite.Require().NoError(suite.db.Create(&communitym.EntityReport{
		EntityType: "artist", EntityID: 1, ReportedBy: user.ID,
		ReportType: "inaccurate", Status: communitym.EntityReportStatusPending,
	}).Error)

	// Show report
	show := suite.createShow(user.ID, "Test Show")
	suite.Require().NoError(suite.db.Create(&communitym.ShowReport{
		ShowID: show.ID, ReportedBy: user.ID,
		ReportType: communitym.ShowReportTypeCancelled, Status: communitym.ShowReportStatusPending,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.ReportsFiled)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ReportsResolved() {
	user := suite.createTestUser("resolver")
	reporter := suite.createTestUser("filereporter")

	// Resolved entity report
	now := time.Now()
	suite.Require().NoError(suite.db.Create(&communitym.EntityReport{
		EntityType: "venue", EntityID: 1, ReportedBy: reporter.ID,
		ReportType: "inaccurate", Status: communitym.EntityReportStatusResolved,
		ReviewedBy: &user.ID, ReviewedAt: &now,
	}).Error)

	// Dismissed show report
	show := suite.createShow(reporter.ID, "Reported Show")
	suite.Require().NoError(suite.db.Create(&communitym.ShowReport{
		ShowID: show.ID, ReportedBy: reporter.ID,
		ReportType: communitym.ShowReportTypeInaccurate, Status: communitym.ShowReportStatusDismissed,
		ReviewedBy: &user.ID, ReviewedAt: &now,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.ReportsResolved)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_FollowingCount() {
	user := suite.createTestUser("follower")
	other := suite.createTestUser("followed")

	// Follow some entities
	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID: user.ID, EntityType: engagementm.BookmarkEntityArtist,
		EntityID: 1, Action: engagementm.BookmarkActionFollow,
	}).Error)
	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID: user.ID, EntityType: engagementm.BookmarkEntityVenue,
		EntityID: 1, Action: engagementm.BookmarkActionFollow,
	}).Error)
	// User→user follow must NOT inflate following_count (PSY-1496)
	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID: user.ID, EntityType: engagementm.BookmarkEntityType("user"),
		EntityID: other.ID, Action: engagementm.BookmarkActionFollow,
	}).Error)
	// "save" action should not count
	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID: user.ID, EntityType: engagementm.BookmarkEntityShow,
		EntityID: 1, Action: engagementm.BookmarkActionSave,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.FollowingCount)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_FollowersCount() {
	target := suite.createTestUser("celeb")
	a := suite.createTestUser("fan_a")
	b := suite.createTestUser("fan_b")

	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID: a.ID, EntityType: engagementm.BookmarkEntityType("user"),
		EntityID: target.ID, Action: engagementm.BookmarkActionFollow,
	}).Error)
	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID: b.ID, EntityType: engagementm.BookmarkEntityType("user"),
		EntityID: target.ID, Action: engagementm.BookmarkActionFollow,
	}).Error)
	// Entity follow of someone else's artist must not count as a user follower
	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID: a.ID, EntityType: engagementm.BookmarkEntityArtist,
		EntityID: target.ID, Action: engagementm.BookmarkActionFollow,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(target.ID, contracts.ShowViewer{UserID: target.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.FollowersCount)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_TotalIncludesNewStats() {
	user := suite.createTestUser("totaluser")

	// Create a show (1 contribution)
	suite.createShow(user.ID, "My Show")

	// Create a revision (1 contribution)
	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&adminm.Revision{
		EntityType: "artist", EntityID: 1, UserID: user.ID,
		FieldChanges: &fieldChanges,
	}).Error)

	// Create a tag vote (1 contribution)
	tag := &catalogm.Tag{Name: "rock", Slug: fmt.Sprintf("rock-%d", time.Now().UnixNano()), Category: "genre"}
	suite.Require().NoError(suite.db.Create(tag).Error)
	artist := &catalogm.Artist{Name: "Test Artist"}
	suite.Require().NoError(suite.db.Create(artist).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.TagVote{
		TagID: tag.ID, EntityType: "artist", EntityID: artist.ID, UserID: user.ID, Vote: 1,
	}).Error)

	// Create a report (1 contribution)
	suite.Require().NoError(suite.db.Create(&communitym.EntityReport{
		EntityType: "artist", EntityID: 1, ReportedBy: user.ID,
		ReportType: "inaccurate", Status: communitym.EntityReportStatusPending,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID, contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	// 1 show + 1 revision + 1 tag vote + 1 report = 4
	suite.Equal(int64(4), stats.TotalContributions)
}

// =============================================================================
// Group 4: GetContributionHistory
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_Empty() {
	user := suite.createTestUser("emptyhistory")

	entries, total, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "", contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(entries)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_ShowSubmissions() {
	user := suite.createTestUser("showhistory")
	suite.createShow(user.ID, "My Great Show")

	entries, total, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "", contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(entries, 1)
	suite.Equal("submit_show", entries[0].Action)
	suite.Equal("show", entries[0].EntityType)
	suite.Equal("submission", entries[0].Source)
	suite.Equal("My Great Show", entries[0].EntityName)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_AuditLogEntries() {
	user := suite.createTestUser("audithistory")
	suite.auditLog.LogAction(user.ID, "create_release", "release", 1, map[string]interface{}{
		"title": "New Album",
	})

	entries, total, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "", contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(entries, 1)
	suite.Equal("create_release", entries[0].Action)
	suite.Equal("release", entries[0].EntityType)
	suite.Equal("audit_log", entries[0].Source)
	// create_release publishes no metadata key, so a row carrying one arrives
	// with the field absent rather than with the stored document.
	suite.Nil(entries[0].Metadata)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_MergesSources() {
	user := suite.createTestUser("mergehistory")
	suite.createShow(user.ID, "Submitted Show")
	suite.createVenue(user.ID, "Submitted Venue")
	suite.auditLog.LogAction(user.ID, "create_release", "release", 1, nil)

	entries, total, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "", contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(entries, 3)

	// Verify both sources are represented
	sources := map[string]bool{}
	for _, e := range entries {
		sources[e.Source] = true
	}
	suite.True(sources["submission"])
	suite.True(sources["audit_log"])
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_Pagination() {
	user := suite.createTestUser("paginationhistory")
	for i := 0; i < 5; i++ {
		suite.createShow(user.ID, fmt.Sprintf("Show %d", i))
	}

	// Page 1
	page1, total, err := suite.profileService.GetContributionHistory(user.ID, 2, 0, "", contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(page1, 2)

	// Page 2
	page2, _, err := suite.profileService.GetContributionHistory(user.ID, 2, 2, "", contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Len(page2, 2)

	// Page 3
	page3, _, err := suite.profileService.GetContributionHistory(user.ID, 2, 4, "", contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Len(page3, 1)

	// No overlap
	suite.NotEqual(page1[0].ID, page2[0].ID)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_EntityTypeFilter() {
	user := suite.createTestUser("filterhistory")
	suite.createShow(user.ID, "A Show")
	suite.createVenue(user.ID, "A Venue")

	// Filter to shows only
	entries, total, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "show", contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(entries, 1)
	suite.Equal("show", entries[0].EntityType)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_LimitClamping() {
	user := suite.createTestUser("limithistory")

	// Limit > 100 should be clamped
	_, _, err := suite.profileService.GetContributionHistory(user.ID, 200, 0, "", contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)

	// Limit < 1 should default to 20
	_, _, err = suite.profileService.GetContributionHistory(user.ID, 0, 0, "", contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_VenueSubmissionEnriched() {
	user := suite.createTestUser("venueenrich")
	suite.createVenue(user.ID, "The Rebel Lounge")

	entries, _, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "", contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)
	suite.Equal("submit_venue", entries[0].Action)
	suite.Equal("venue", entries[0].EntityType)
	suite.Equal("The Rebel Lounge", entries[0].EntityName)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_OrderedByCreatedAtDesc() {
	user := suite.createTestUser("orderhistory")

	// Create entries with different timestamps
	show1 := suite.createShow(user.ID, "First Show")
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	show2 := suite.createShow(user.ID, "Second Show")

	entries, _, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "", contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().Len(entries, 2)
	// Most recent first
	suite.Equal(show2.ID, entries[0].EntityID)
	suite.Equal(show1.ID, entries[1].EntityID)
}

// TestGetContributionHistory_PendingEditEnriched_AllEntityTypes verifies
// that pending_entity_edits rows — which surface in the feed with the
// synthetic entity_type "<type>_edit" — resolve to a human-readable entity
// name for every supported type. Without enrichment, the activity-feed
// entity-name slot leaks the raw discriminator string ("artist_edit", etc.)
// directly into the UI.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_PendingEditEnriched_AllEntityTypes() {
	cases := []struct {
		name           string
		entityType     string // value stored on PendingEntityEdit (no _edit suffix)
		expectedName   string
		setup          func(submittedBy uint) uint // returns entity ID created
		wantAction     string
		wantEntityType string // the synthetic "<type>_edit" we expect on the response
	}{
		{
			name:         "artist",
			entityType:   adminm.PendingEditEntityArtist,
			expectedName: "Amyl and The Sniffers",
			setup: func(_ uint) uint {
				a := &catalogm.Artist{Name: "Amyl and The Sniffers"}
				suite.Require().NoError(suite.db.Create(a).Error)
				return a.ID
			},
			wantAction:     "submit_artist_edit",
			wantEntityType: "artist_edit",
		},
		{
			name:         "venue",
			entityType:   adminm.PendingEditEntityVenue,
			expectedName: "Valley Bar",
			// Bypass createVenue: setting submitted_by here would inject an
			// extra "submit_venue" row from the venueQuery UNION and stop
			// us from isolating the _edit enrichment path.
			setup: func(_ uint) uint {
				v := &catalogm.Venue{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
				suite.Require().NoError(suite.db.Create(v).Error)
				return v.ID
			},
			wantAction:     "submit_venue_edit",
			wantEntityType: "venue_edit",
		},
		{
			name:         "release",
			entityType:   adminm.PendingEditEntityRelease,
			expectedName: "Comfort to Me",
			setup: func(_ uint) uint {
				r := &catalogm.Release{Title: "Comfort to Me"}
				suite.Require().NoError(suite.db.Create(r).Error)
				return r.ID
			},
			wantAction:     "submit_release_edit",
			wantEntityType: "release_edit",
		},
		{
			name:         "label",
			entityType:   adminm.PendingEditEntityLabel,
			expectedName: "Rough Trade",
			setup: func(_ uint) uint {
				l := &catalogm.Label{Name: "Rough Trade"}
				suite.Require().NoError(suite.db.Create(l).Error)
				return l.ID
			},
			wantAction:     "submit_label_edit",
			wantEntityType: "label_edit",
		},
		{
			name:         "festival",
			entityType:   adminm.PendingEditEntityFestival,
			expectedName: "Zona Music Festival",
			setup: func(_ uint) uint {
				slug := fmt.Sprintf("zona-2026-%d", time.Now().UnixNano())
				f := &catalogm.Festival{
					Name:        "Zona Music Festival",
					Slug:        slug,
					SeriesSlug:  "zona",
					EditionYear: 2026,
					StartDate:   "2026-12-01",
					EndDate:     "2026-12-02",
				}
				suite.Require().NoError(suite.db.Create(f).Error)
				return f.ID
			},
			wantAction:     "submit_festival_edit",
			wantEntityType: "festival_edit",
		},
	}

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			user := suite.createTestUser(fmt.Sprintf("editenrich-%s-%d", tc.name, time.Now().UnixNano()))
			entityID := tc.setup(user.ID)

			changes := json.RawMessage(`[{"field":"name","old_value":"X","new_value":"Y"}]`)
			suite.Require().NoError(suite.db.Create(&adminm.PendingEntityEdit{
				EntityType:   tc.entityType,
				EntityID:     entityID,
				SubmittedBy:  user.ID,
				FieldChanges: &changes,
				Summary:      "Fix name",
				Status:       adminm.PendingEditStatusPending,
			}).Error)

			entries, _, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "", contracts.ShowViewer{UserID: user.ID})

			suite.Require().NoError(err)
			suite.Require().Len(entries, 1, "expected exactly one feed row for one suggest-edit")
			suite.Equal(tc.wantAction, entries[0].Action)
			suite.Equal(tc.wantEntityType, entries[0].EntityType)
			suite.Equal(entityID, entries[0].EntityID)
			suite.Equal(tc.expectedName, entries[0].EntityName,
				"entity_name must resolve to the underlying entity, not the raw '%s' discriminator",
				tc.wantEntityType,
			)
		})
	}
}

// =============================================================================
// Group 5: Privacy Settings
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdatePrivacySettings_Success() {
	user := suite.createTestUser("privacyuser")

	settings := contracts.PrivacySettings{
		Contributions:   contracts.PrivacyHidden,
		SavedShows:      contracts.PrivacyVisible,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyCountOnly,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyVisible,
	}

	result, err := suite.profileService.UpdatePrivacySettings(user.ID, settings)

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Equal(contracts.PrivacyHidden, result.Contributions)
	suite.Equal(contracts.PrivacyVisible, result.SavedShows)
	suite.Equal(contracts.PrivacyHidden, result.Following)
	suite.Equal(contracts.PrivacyCountOnly, result.Collections)
	suite.Equal(contracts.PrivacyHidden, result.LastActive)
	suite.Equal(contracts.PrivacyVisible, result.ProfileSections)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdatePrivacySettings_Persists() {
	user := suite.createTestUser("privacypersist")

	settings := contracts.PrivacySettings{
		Contributions:   contracts.PrivacyHidden,
		SavedShows:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	}

	_, err := suite.profileService.UpdatePrivacySettings(user.ID, settings)
	suite.Require().NoError(err)

	// Reload and verify
	profile, err := suite.profileService.GetOwnProfile(contracts.ShowViewer{UserID: user.ID})
	suite.Require().NoError(err)
	suite.Require().NotNil(profile.PrivacySettings)
	suite.Equal(contracts.PrivacyHidden, profile.PrivacySettings.Contributions)
	suite.Equal(contracts.PrivacyHidden, profile.PrivacySettings.LastActive)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdatePrivacySettings_InvalidLevel() {
	user := suite.createTestUser("privacyinvalid")

	settings := contracts.DefaultPrivacySettings()
	settings.Contributions = "invalid_level"

	result, err := suite.profileService.UpdatePrivacySettings(user.ID, settings)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "invalid privacy level")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdatePrivacySettings_CountOnlyBinaryField() {
	user := suite.createTestUser("privacybinary")

	settings := contracts.DefaultPrivacySettings()
	settings.LastActive = contracts.PrivacyCountOnly

	result, err := suite.profileService.UpdatePrivacySettings(user.ID, settings)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "only supports 'visible' or 'hidden'")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_PrivacyGating_ContributionsHidden() {
	user := suite.createTestUser("privgatecontrib")
	suite.createShow(user.ID, "Hidden Show")

	suite.setPrivacySettings(user.ID, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyHidden,
		SavedShows:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	})

	otherUser := suite.createTestUser("viewer1")
	profile, err := suite.profileService.GetPublicProfile("privgatecontrib", contracts.ShowViewer{UserID: otherUser.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Nil(profile.Stats, "Stats should be nil when contributions are hidden")
	suite.Nil(profile.StatsCount, "StatsCount should be nil when contributions are hidden")
	suite.Nil(profile.LastActive, "LastActive should be nil when hidden")
	suite.Empty(profile.Sections, "Sections should be empty when hidden")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_PrivacyGating_ContributionsCountOnly() {
	user := suite.createTestUser("privgatecountonly")
	suite.createShow(user.ID, "Counted Show")
	suite.createShow(user.ID, "Another Counted Show")

	suite.setPrivacySettings(user.ID, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyCountOnly,
		SavedShows:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyVisible,
		ProfileSections: contracts.PrivacyVisible,
	})

	otherUser := suite.createTestUser("viewer2")
	profile, err := suite.profileService.GetPublicProfile("privgatecountonly", contracts.ShowViewer{UserID: otherUser.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Nil(profile.Stats, "Full stats should be nil with count_only")
	suite.Require().NotNil(profile.StatsCount, "StatsCount should be present with count_only")
	suite.Equal(int64(2), *profile.StatsCount)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_OwnerSeesEverything() {
	user := suite.createTestUser("ownerseesall")
	suite.createShow(user.ID, "My Show")

	suite.setPrivacySettings(user.ID, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyHidden,
		SavedShows:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	})

	profile, err := suite.profileService.GetPublicProfile("ownerseesall", contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Require().NotNil(profile.Stats, "Owner always sees stats")
	suite.Require().NotNil(profile.PrivacySettings, "Owner sees privacy settings")
	suite.Require().NotNil(profile.LastActive, "Owner always sees last active")
}

// =============================================================================
// Group 6: User Tier
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_DefaultTier() {
	suite.createTestUser("tierdefault")

	profile, err := suite.profileService.GetPublicProfile("tierdefault", contracts.ShowViewer{})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal("new_user", profile.UserTier)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_CustomTier() {
	user := suite.createTestUser("tiercustom")
	err := suite.db.Model(user).Update("user_tier", "contributor").Error
	suite.Require().NoError(err)

	profile, err := suite.profileService.GetPublicProfile("tiercustom", contracts.ShowViewer{})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal("contributor", profile.UserTier)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnProfile_IncludesTier() {
	user := suite.createTestUser("tierown")
	err := suite.db.Model(user).Update("user_tier", "trusted_contributor").Error
	suite.Require().NoError(err)

	profile, err := suite.profileService.GetOwnProfile(contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal("trusted_contributor", profile.UserTier)
}

// =============================================================================
// Group 7: Profile Sections CRUD
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestCreateSection_Success() {
	user := suite.createTestUser("sectioncreate")

	section, err := suite.profileService.CreateSection(user.ID, "My Music", "I love punk rock", 0)

	suite.Require().NoError(err)
	suite.Require().NotNil(section)
	suite.Equal("My Music", section.Title)
	suite.Equal("I love punk rock", section.Content)
	suite.Equal(0, section.Position)
	suite.True(section.IsVisible)
	suite.NotZero(section.ID)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestCreateSection_MaxSections() {
	user := suite.createTestUser("sectionmax")

	_, err := suite.profileService.CreateSection(user.ID, "Section 1", "Content", 0)
	suite.Require().NoError(err)
	_, err = suite.profileService.CreateSection(user.ID, "Section 2", "Content", 1)
	suite.Require().NoError(err)
	_, err = suite.profileService.CreateSection(user.ID, "Section 3", "Content", 2)
	suite.Require().NoError(err)

	// Fourth should fail
	section, err := suite.profileService.CreateSection(user.ID, "Section 4", "Content", 0)

	suite.Error(err)
	suite.Nil(section)
	suite.Contains(err.Error(), "maximum 3 profile sections allowed")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestCreateSection_EmptyTitle() {
	user := suite.createTestUser("sectionempty")

	section, err := suite.profileService.CreateSection(user.ID, "", "Content", 0)

	suite.Error(err)
	suite.Nil(section)
	suite.Contains(err.Error(), "title must be between 1 and 255 characters")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestCreateSection_InvalidPosition() {
	user := suite.createTestUser("sectionpos")

	section, err := suite.profileService.CreateSection(user.ID, "Title", "Content", 5)

	suite.Error(err)
	suite.Nil(section)
	suite.Contains(err.Error(), "position must be between 0 and 2")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestCreateSection_NegativePosition() {
	user := suite.createTestUser("sectionneg")

	section, err := suite.profileService.CreateSection(user.ID, "Title", "Content", -1)

	suite.Error(err)
	suite.Nil(section)
	suite.Contains(err.Error(), "position must be between 0 and 2")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetUserSections_OnlyVisible() {
	user := suite.createTestUser("sectionvisible")

	s1, err := suite.profileService.CreateSection(user.ID, "Visible", "Content", 0)
	suite.Require().NoError(err)
	s2, err := suite.profileService.CreateSection(user.ID, "Hidden", "Content", 1)
	suite.Require().NoError(err)

	// Hide the second section
	_, err = suite.profileService.UpdateSection(user.ID, s2.ID, map[string]interface{}{"is_visible": false})
	suite.Require().NoError(err)

	sections, err := suite.profileService.GetUserSections(user.ID)

	suite.Require().NoError(err)
	suite.Len(sections, 1)
	suite.Equal(s1.ID, sections[0].ID)
	suite.Equal("Visible", sections[0].Title)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnSections_IncludesHidden() {
	user := suite.createTestUser("sectionown")

	_, err := suite.profileService.CreateSection(user.ID, "Visible", "Content", 0)
	suite.Require().NoError(err)
	s2, err := suite.profileService.CreateSection(user.ID, "Hidden", "Content", 1)
	suite.Require().NoError(err)

	// Hide the second section
	_, err = suite.profileService.UpdateSection(user.ID, s2.ID, map[string]interface{}{"is_visible": false})
	suite.Require().NoError(err)

	sections, err := suite.profileService.GetOwnSections(user.ID)

	suite.Require().NoError(err)
	suite.Len(sections, 2)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdateSection_Success() {
	user := suite.createTestUser("sectionupdate")

	section, err := suite.profileService.CreateSection(user.ID, "Original", "Original content", 0)
	suite.Require().NoError(err)

	updated, err := suite.profileService.UpdateSection(user.ID, section.ID, map[string]interface{}{
		"title":   "Updated Title",
		"content": "Updated content",
	})

	suite.Require().NoError(err)
	suite.Equal("Updated Title", updated.Title)
	suite.Equal("Updated content", updated.Content)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdateSection_NotFound() {
	user := suite.createTestUser("sectionnotfound")

	section, err := suite.profileService.UpdateSection(user.ID, 99999, map[string]interface{}{
		"title": "Nope",
	})

	suite.Nil(section)
	suite.assertProfileSectionNotFound(err)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdateSection_WrongOwner() {
	user1 := suite.createTestUser("sectionowner1")
	user2 := suite.createTestUser("sectionowner2")

	section, err := suite.profileService.CreateSection(user1.ID, "Mine", "Content", 0)
	suite.Require().NoError(err)

	// user2 tries to update user1's section
	result, err := suite.profileService.UpdateSection(user2.ID, section.ID, map[string]interface{}{
		"title": "Hijacked",
	})

	suite.Nil(result)
	suite.assertProfileSectionNotFound(err)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestDeleteSection_Success() {
	user := suite.createTestUser("sectiondelete")

	section, err := suite.profileService.CreateSection(user.ID, "Doomed", "Content", 0)
	suite.Require().NoError(err)

	err = suite.profileService.DeleteSection(user.ID, section.ID)

	suite.NoError(err)

	// Verify it's gone
	sections, err := suite.profileService.GetOwnSections(user.ID)
	suite.Require().NoError(err)
	suite.Empty(sections)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestDeleteSection_NotFound() {
	user := suite.createTestUser("sectiondelnotfound")

	err := suite.profileService.DeleteSection(user.ID, 99999)

	suite.assertProfileSectionNotFound(err)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestDeleteSection_WrongOwner() {
	user1 := suite.createTestUser("sectiondelowner1")
	user2 := suite.createTestUser("sectiondelowner2")

	section, err := suite.profileService.CreateSection(user1.ID, "Protected", "Content", 0)
	suite.Require().NoError(err)

	err = suite.profileService.DeleteSection(user2.ID, section.ID)

	suite.assertProfileSectionNotFound(err)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_IncludesSections() {
	user := suite.createTestUser("profilesections")

	_, err := suite.profileService.CreateSection(user.ID, "About Me", "I go to shows", 0)
	suite.Require().NoError(err)
	_, err = suite.profileService.CreateSection(user.ID, "Favorite Genres", "Punk, Indie", 1)
	suite.Require().NoError(err)

	profile, err := suite.profileService.GetPublicProfile("profilesections", contracts.ShowViewer{})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Len(profile.Sections, 2)
	suite.Equal("About Me", profile.Sections[0].Title)
	suite.Equal("Favorite Genres", profile.Sections[1].Title)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnProfile_IncludesAllSections() {
	user := suite.createTestUser("ownsections")

	_, err := suite.profileService.CreateSection(user.ID, "Visible", "Content", 0)
	suite.Require().NoError(err)
	s2, err := suite.profileService.CreateSection(user.ID, "Hidden", "Secret", 1)
	suite.Require().NoError(err)
	_, err = suite.profileService.UpdateSection(user.ID, s2.ID, map[string]interface{}{"is_visible": false})
	suite.Require().NoError(err)

	profile, err := suite.profileService.GetOwnProfile(contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Len(profile.Sections, 2, "Own profile should include hidden sections")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetSections_OrderedByPosition() {
	user := suite.createTestUser("sectionorder")

	_, err := suite.profileService.CreateSection(user.ID, "Third", "Content", 2)
	suite.Require().NoError(err)
	_, err = suite.profileService.CreateSection(user.ID, "First", "Content", 0)
	suite.Require().NoError(err)
	_, err = suite.profileService.CreateSection(user.ID, "Second", "Content", 1)
	suite.Require().NoError(err)

	sections, err := suite.profileService.GetUserSections(user.ID)

	suite.Require().NoError(err)
	suite.Require().Len(sections, 3)
	suite.Equal("First", sections[0].Title)
	suite.Equal("Second", sections[1].Title)
	suite.Equal("Third", sections[2].Title)
}

// =============================================================================
// Group 8: GetActivityHeatmap
// =============================================================================

func TestGetActivityHeatmap_NilDB(t *testing.T) {
	svc := &ContributorProfileService{db: nil}
	result, err := svc.GetActivityHeatmap(1, contracts.ShowViewer{UserID: 1})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_NoActivity() {
	user := suite.createTestUser("heatmap_empty")

	resp, err := suite.profileService.GetActivityHeatmap(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Empty(resp.Days)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_ShowSubmissions() {
	user := suite.createTestUser("heatmap_shows")
	suite.createShow(user.ID, "Show 1")
	suite.createShow(user.ID, "Show 2")

	resp, err := suite.profileService.GetActivityHeatmap(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(2, resp.Days[0].Count)
	// Date should be today
	suite.Equal(time.Now().UTC().Format("2006-01-02"), resp.Days[0].Date)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_VenueSubmissions() {
	user := suite.createTestUser("heatmap_venues")
	suite.createVenue(user.ID, "Venue 1")

	resp, err := suite.profileService.GetActivityHeatmap(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(1, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_AuditLogEntries() {
	user := suite.createTestUser("heatmap_audit")
	helper := &testAuditLogHelper{db: suite.db}
	helper.LogAction(user.ID, "create_release", "release", 1, nil)
	helper.LogAction(user.ID, "edit_artist", "artist", 1, nil)

	resp, err := suite.profileService.GetActivityHeatmap(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(2, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_Revisions() {
	user := suite.createTestUser("heatmap_revisions")
	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&adminm.Revision{
		EntityType: "artist", EntityID: 1, UserID: user.ID,
		FieldChanges: &fieldChanges,
	}).Error)

	resp, err := suite.profileService.GetActivityHeatmap(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(1, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_PendingEdits() {
	user := suite.createTestUser("heatmap_edits")
	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&adminm.PendingEntityEdit{
		EntityType:   "venue",
		EntityID:     1,
		SubmittedBy:  user.ID,
		Status:       adminm.PendingEditStatusPending,
		FieldChanges: &fieldChanges,
	}).Error)

	resp, err := suite.profileService.GetActivityHeatmap(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(1, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_MultipleTablesAggregated() {
	user := suite.createTestUser("heatmap_multi")

	// Create entries across multiple tables (all today)
	suite.createShow(user.ID, "Heatmap Show")
	suite.createVenue(user.ID, "Heatmap Venue")
	helper := &testAuditLogHelper{db: suite.db}
	helper.LogAction(user.ID, "edit_artist", "artist", 1, nil)
	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&adminm.Revision{
		EntityType: "artist", EntityID: 1, UserID: user.ID,
		FieldChanges: &fieldChanges,
	}).Error)
	suite.Require().NoError(suite.db.Create(&adminm.PendingEntityEdit{
		EntityType:   "venue",
		EntityID:     1,
		SubmittedBy:  user.ID,
		Status:       adminm.PendingEditStatusPending,
		FieldChanges: &fieldChanges,
	}).Error)

	resp, err := suite.profileService.GetActivityHeatmap(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	// 1 show + 1 venue + 1 audit + 1 revision + 1 pending edit = 5
	suite.Equal(5, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_DoesNotCountOtherUsers() {
	user1 := suite.createTestUser("heatmap_user1")
	user2 := suite.createTestUser("heatmap_user2")

	suite.createShow(user1.ID, "User 1 Show")
	suite.createShow(user2.ID, "User 2 Show")

	resp, err := suite.profileService.GetActivityHeatmap(user1.ID, contracts.ShowViewer{UserID: user1.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(1, resp.Days[0].Count, "Should only count user1's activity")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_OldDataExcluded() {
	user := suite.createTestUser("heatmap_old")

	// Create a show dated today (should be included)
	suite.createShow(user.ID, "Recent Show")

	// Directly insert a show with an old created_at (>365 days ago)
	oldDate := time.Now().UTC().AddDate(0, 0, -400)
	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO shows (title, submitted_by, status, event_date, created_at, updated_at) VALUES (?, ?, 'approved', ?, ?, ?)",
		"Old Show", user.ID, oldDate, oldDate, oldDate,
	).Error)

	resp, err := suite.profileService.GetActivityHeatmap(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	// Should only have 1 day (today's show), not the old one
	suite.Require().Len(resp.Days, 1)
	suite.Equal(1, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_DateFormat() {
	user := suite.createTestUser("heatmap_format")
	suite.createShow(user.ID, "Date Format Show")

	resp, err := suite.profileService.GetActivityHeatmap(user.ID, contracts.ShowViewer{UserID: user.ID})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	// Verify date format is YYYY-MM-DD
	_, err = time.Parse("2006-01-02", resp.Days[0].Date)
	suite.NoError(err, "Date should be in YYYY-MM-DD format")
}

// =============================================================================
// Group: Percentile Rankings
// =============================================================================

// createManyUsers creates n users with sequential usernames.
func (suite *ContributorProfileServiceIntegrationTestSuite) createManyUsers(prefix string, n int) []*authm.User {
	users := make([]*authm.User, n)
	for i := 0; i < n; i++ {
		users[i] = suite.createTestUser(fmt.Sprintf("%s%d", prefix, i))
	}
	return users
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_NilDB() {
	svc := &ContributorProfileService{db: nil}
	result, err := svc.GetPercentileRankings(1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
	suite.Nil(result)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_TooFewUsers() {
	// Only 3 users — should return nil
	for i := 0; i < 3; i++ {
		suite.createTestUser(fmt.Sprintf("fewuser%d", i))
	}

	result, err := suite.profileService.GetPercentileRankings(1)
	suite.NoError(err)
	suite.Nil(result, "Should return nil when fewer than 10 active users")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_NoContributions() {
	users := suite.createManyUsers("nocontrib", 10)

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	// With no contributions, user should be at 0th percentile for all dimensions
	for _, r := range result.Rankings {
		suite.Equal(int64(0), r.Value, "Dimension %s should have 0 value", r.Dimension)
		suite.Equal(0, r.Percentile, "Dimension %s should have 0 percentile", r.Dimension)
	}
	suite.Equal(0, result.OverallScore)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_WithShowContributions() {
	users := suite.createManyUsers("showperc", 10)

	// Give the first user 5 shows, second user 2 shows, rest 0
	for i := 0; i < 5; i++ {
		suite.createShow(users[0].ID, fmt.Sprintf("Show %d", i))
	}
	for i := 0; i < 2; i++ {
		suite.createShow(users[1].ID, fmt.Sprintf("Other Show %d", i))
	}

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	// Find shows_submitted dimension
	var showsRanking *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "shows_submitted" {
			showsRanking = &result.Rankings[i]
			break
		}
	}
	suite.Require().NotNil(showsRanking, "Should have shows_submitted dimension")
	suite.Equal(int64(5), showsRanking.Value)
	// User 0 has 5 shows; 9 users have less (8 with 0, 1 with 2) → 9/10 = 90%
	suite.Equal(90, showsRanking.Percentile)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_MultipleUsers_RelativeRanking() {
	users := suite.createManyUsers("relrank", 10)

	// User 0: 3 shows, user 1: 1 show, users 2-9: 0 shows
	for i := 0; i < 3; i++ {
		suite.createShow(users[0].ID, fmt.Sprintf("Show %d", i))
	}
	suite.createShow(users[1].ID, "Single Show")

	// Check user 0 (highest)
	result0, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result0)

	// Check user 1 (middle)
	result1, err := suite.profileService.GetPercentileRankings(users[1].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result1)

	// Check user 2 (no contributions)
	result2, err := suite.profileService.GetPercentileRankings(users[2].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result2)

	var p0, p1, p2 int
	for _, r := range result0.Rankings {
		if r.Dimension == "shows_submitted" {
			p0 = r.Percentile
		}
	}
	for _, r := range result1.Rankings {
		if r.Dimension == "shows_submitted" {
			p1 = r.Percentile
		}
	}
	for _, r := range result2.Rankings {
		if r.Dimension == "shows_submitted" {
			p2 = r.Percentile
		}
	}

	// User 0 > user 1 > user 2
	suite.Greater(p0, p1, "User with 3 shows should rank higher than user with 1")
	suite.Greater(p1, p2, "User with 1 show should rank higher than user with 0")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_VenuesDimension() {
	users := suite.createManyUsers("venueperc", 10)

	suite.createVenue(users[0].ID, "Venue A")
	suite.createVenue(users[0].ID, "Venue B")
	suite.createVenue(users[0].ID, "Venue C")

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	var venuesRanking *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "venues_submitted" {
			venuesRanking = &result.Rankings[i]
			break
		}
	}
	suite.Require().NotNil(venuesRanking)
	suite.Equal(int64(3), venuesRanking.Value)
	// 9 users with less → 9/10 = 90%
	suite.Equal(90, venuesRanking.Percentile)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_TagsDimension() {
	users := suite.createManyUsers("tagperc", 10)

	// Create a tag
	tag := &catalogm.Tag{
		Name:     "indie-rock",
		Slug:     "indie-rock",
		Category: catalogm.TagCategoryGenre,
	}
	suite.Require().NoError(suite.db.Create(tag).Error)

	// Create an artist to tag
	artist := &catalogm.Artist{Name: "Test Band"}
	suite.Require().NoError(suite.db.Create(artist).Error)

	// Apply tags by user 0
	for i := 0; i < 3; i++ {
		et := &catalogm.EntityTag{
			TagID:         tag.ID,
			EntityType:    "artist",
			EntityID:      artist.ID + uint(i), // different entity IDs to avoid unique constraint
			AddedByUserID: users[0].ID,
		}
		suite.Require().NoError(suite.db.Create(et).Error)
	}

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	var tagsRanking *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "tags_applied" {
			tagsRanking = &result.Rankings[i]
			break
		}
	}
	suite.Require().NotNil(tagsRanking)
	suite.Equal(int64(3), tagsRanking.Value)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_EditsDimension() {
	users := suite.createManyUsers("editperc", 10)

	// Create a revision for user 0
	rev := &adminm.Revision{
		EntityType:   "artist",
		EntityID:     1,
		UserID:       users[0].ID,
		FieldChanges: nil,
	}
	// field_changes must be non-nil JSONB
	emptyChanges := json.RawMessage(`[]`)
	rev.FieldChanges = &emptyChanges
	suite.Require().NoError(suite.db.Create(rev).Error)

	// Create an approved pending edit for user 0
	changes := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	pe := &adminm.PendingEntityEdit{
		EntityType:   "artist",
		EntityID:     1,
		SubmittedBy:  users[0].ID,
		FieldChanges: &changes,
		Summary:      "Test edit",
		Status:       adminm.PendingEditStatusApproved,
	}
	suite.Require().NoError(suite.db.Create(pe).Error)

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	var editsRanking *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "edits_approved" {
			editsRanking = &result.Rankings[i]
			break
		}
	}
	suite.Require().NotNil(editsRanking)
	suite.Equal(int64(2), editsRanking.Value, "Should count 1 revision + 1 approved edit")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_RequestsFulfilledDimension() {
	users := suite.createManyUsers("reqperc", 10)

	// Create a fulfilled request by user 0
	req := &communitym.Request{
		Title:       "Find artist info",
		EntityType:  "artist",
		RequesterID: users[1].ID,
		FulfillerID: &users[0].ID,
		Status:      communitym.RequestStatusFulfilled,
	}
	suite.Require().NoError(suite.db.Create(req).Error)

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	var reqRanking *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "requests_fulfilled" {
			reqRanking = &result.Rankings[i]
			break
		}
	}
	suite.Require().NotNil(reqRanking)
	suite.Equal(int64(1), reqRanking.Value)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_WeightedOverallScore() {
	users := suite.createManyUsers("overall", 10)

	// Give user 0 shows (weight 25) and venues (weight 15)
	for i := 0; i < 5; i++ {
		suite.createShow(users[0].ID, fmt.Sprintf("Show %d", i))
	}
	suite.createVenue(users[0].ID, "Venue 1")
	suite.createVenue(users[0].ID, "Venue 2")

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	// Shows: 90th percentile (weight 25), Venues: 90th (weight 15), others: 0 (weight 10+25+10=45)
	// Expected: (90*25 + 90*15 + 0*10 + 0*25 + 0*10) / (25+15+10+25+10) = 3600 / 85 = 42
	suite.Equal(42, result.OverallScore)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_AllDimensionsPresent() {
	users := suite.createManyUsers("alldims", 10)

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	suite.Require().Len(result.Rankings, 5, "Should have 5 ranking dimensions")

	expectedDimensions := map[string]bool{
		"shows_submitted":    false,
		"venues_submitted":   false,
		"tags_applied":       false,
		"edits_approved":     false,
		"requests_fulfilled": false,
	}
	for _, r := range result.Rankings {
		_, exists := expectedDimensions[r.Dimension]
		suite.True(exists, "Unexpected dimension: %s", r.Dimension)
		expectedDimensions[r.Dimension] = true
		suite.NotEmpty(r.Label, "Dimension %s should have a label", r.Dimension)
	}
	for dim, found := range expectedDimensions {
		suite.True(found, "Missing dimension: %s", dim)
	}
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_ExactlyTenUsers() {
	// Exactly 10 users should work (threshold is < 10 returns nil)
	suite.createManyUsers("exact", 10)

	result, err := suite.profileService.GetPercentileRankings(1) // any valid user
	suite.Require().NoError(err)
	suite.Require().NotNil(result, "Exactly 10 users should be enough for rankings")
}

// =============================================================================
// Contribution history: collection visibility
// =============================================================================

// createCollectionForHistory creates a collection owned by creatorID. GORM skips
// a false bool on Create, so private collections are created public and updated,
// which is the same pattern the tag suite uses.
func (suite *ContributorProfileServiceIntegrationTestSuite) createCollectionForHistory(
	creatorID uint, title, slug string, isPublic bool,
) *communitym.Collection {
	c := &communitym.Collection{
		Title:     title,
		Slug:      slug,
		CreatorID: creatorID,
		IsPublic:  true,
	}
	suite.Require().NoError(suite.db.Create(c).Error)
	if !isPublic {
		suite.Require().NoError(suite.db.Model(c).Update("is_public", false).Error)
		c.IsPublic = false
	}
	return c
}

func (suite *ContributorProfileServiceIntegrationTestSuite) createCollectionItemForHistory(
	collectionID, addedBy uint,
) *communitym.CollectionItem {
	item := &communitym.CollectionItem{
		CollectionID:  collectionID,
		EntityType:    "artist",
		EntityID:      1,
		AddedByUserID: addedBy,
		CreatedAt:     time.Now().UTC(),
	}
	suite.Require().NoError(suite.db.Create(item).Error)
	return item
}

// A PRIVATE COLLECTION LEAVES SOMEBODY ELSE'S CONTRIBUTION TIMELINE ENTIRELY:
// its id, its title and the slug the audit metadata carries.
//
// The timeline is anonymous-readable, so every collection audit row is a
// disclosure of the collection's identity to whoever loads the profile. The
// total is asserted alongside the page because a row dropped after the count is
// the same disclosure restated as arithmetic.
//
// Both id families are covered. create_collection names the collection, while
// add_collection_item names a collection_items row under the same entity_type,
// and a gate that read entity_id one way for both would judge one family against
// the other's table.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_HidesPrivateCollections() {
	actor := suite.createTestUser("collectionactor")
	stranger := suite.createTestUser("collectionstranger")

	openColl := suite.createCollectionForHistory(actor.ID, "Open Picks", "open-picks-history", true)
	shutColl := suite.createCollectionForHistory(actor.ID, "Shut Picks", "shut-picks-history", false)
	openItem := suite.createCollectionItemForHistory(openColl.ID, actor.ID)
	shutItem := suite.createCollectionItemForHistory(shutColl.ID, actor.ID)

	suite.auditLog.LogAction(actor.ID, "create_collection", "collection", openColl.ID,
		map[string]interface{}{"slug": openColl.Slug})
	suite.auditLog.LogAction(actor.ID, "create_collection", "collection", shutColl.ID,
		map[string]interface{}{"slug": shutColl.Slug})
	suite.auditLog.LogAction(actor.ID, "add_collection_item", "collection", openItem.ID,
		map[string]interface{}{"slug": openColl.Slug, "collection_id": openColl.ID})
	suite.auditLog.LogAction(actor.ID, "add_collection_item", "collection", shutItem.ID,
		map[string]interface{}{"slug": shutColl.Slug, "collection_id": shutColl.ID})

	strangerView := contracts.ShowViewer{UserID: stranger.ID}
	entries, total, err := suite.profileService.GetContributionHistory(actor.ID, 50, 0, "", strangerView)
	suite.Require().NoError(err)
	suite.EqualValues(len(entries), total, "the total must count the same rows the page contains")
	suite.Require().Len(entries, 2, "a stranger sees only the public collection's two rows")

	for _, e := range entries {
		suite.NotEqual(shutColl.ID, e.EntityID, "the private collection's id reached a stranger")
		suite.NotEqual(shutItem.ID, e.EntityID, "the private collection's item id reached a stranger")
		suite.NotEqual("Shut Picks", e.EntityName, "the private collection's title reached a stranger")
		suite.Nil(e.Metadata,
			"the collection family publishes no metadata key, so the private "+
				"collection's slug cannot reach a stranger through one")
	}

	// An ANONYMOUS caller is the same tier as a stranger here, and it is the tier
	// the route actually serves most often.
	anonEntries, anonTotal, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{})
	suite.Require().NoError(err)
	suite.EqualValues(len(anonEntries), anonTotal)
	suite.Len(anonEntries, 2)

	// An ADMIN gets the stranger's answer, because no collection read grants one.
	adminEntries, adminTotal, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID, IsAdmin: true})
	suite.Require().NoError(err)
	suite.EqualValues(len(adminEntries), adminTotal)
	suite.Len(adminEntries, 2, "an admin must not be served a private collection here")

	// The CREATOR sees all four, which is what makes the assertions above about
	// visibility rather than about the gate having broken the arm.
	ownEntries, ownTotal, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(ownEntries), ownTotal)
	suite.Require().Len(ownEntries, 4, "the creator sees their own private collection's rows")

	// The item rows carry NO resolved name in either direction: their entity_id
	// is a collection_items id, so looking it up in collections would resolve an
	// unrelated collection's title.
	for _, e := range ownEntries {
		if e.Action == "add_collection_item" {
			suite.Empty(e.EntityName,
				"an item row must not carry a name resolved from an unrelated collection")
		}
		if e.Action == "create_collection" && e.EntityID == shutColl.ID {
			suite.Equal("Shut Picks", e.EntityName, "the creator's own collection resolves its title")
		}
	}
}

// A REMOVED ITEM'S AUDIT ROW IS DECIDED BY THE PARENT ID ITS METADATA RECORDS.
//
// remove_collection_item stores the deleted item's id and collection_items are
// hard-deleted, so the metadata's collection_id is the only durable reference
// left. It is an id rather than the slug beside it because a rename frees the
// slug and a later collection can take that string, which would republish the
// original's rows.
//
// A row whose collection_id resolves to nothing is withheld from everyone,
// which is the same answer a private parent gets.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_RemovedItemRowsDecidedByMetadataParentID() {
	actor := suite.createTestUser("removalactor")
	stranger := suite.createTestUser("removalstranger")

	openColl := suite.createCollectionForHistory(actor.ID, "Removal Open", "removal-open-history", true)
	shutColl := suite.createCollectionForHistory(actor.ID, "Removal Shut", "removal-shut-history", false)
	// The removed item's id is what identifies each row in the response: the
	// timeline publishes no metadata key for this family, so the recorded slug
	// cannot be read back to tell the two rows apart.
	itemIDs := map[uint]uint{}
	for _, c := range []*communitym.Collection{openColl, shutColl} {
		item := suite.createCollectionItemForHistory(c.ID, actor.ID)
		itemID := item.ID
		itemIDs[c.ID] = itemID
		suite.Require().NoError(suite.db.Delete(&communitym.CollectionItem{}, itemID).Error)
		suite.auditLog.LogAction(actor.ID, "remove_collection_item", "collection", itemID,
			map[string]interface{}{"slug": c.Slug, "collection_id": c.ID})
	}
	// A row whose recorded parent id names no collection at all.
	suite.auditLog.LogAction(actor.ID, "remove_collection_item", "collection", 987654,
		map[string]interface{}{"slug": "a-slug-that-never-existed", "collection_id": 987654321})

	entries, total, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(entries), total)
	suite.Require().Len(entries, 1, "a stranger sees only the public parent's removal")
	suite.EqualValues(itemIDs[openColl.ID], entries[0].EntityID)
	suite.Nil(entries[0].Metadata, "the recorded slug decides the row, it is not published")

	own, ownTotal, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(own), ownTotal)
	suite.Len(own, 2, "the creator sees both real removals and neither the unresolvable one")
}

// AN AUDIT ACTION WITH NO DISPOSITION IS REFUSED, not judged against whichever
// table the last branch happens to name.
//
// contributionCollectionActions is hand-maintained against the four writers the
// map's own doc names, not against collection.go alone. A writer added without an
// entry here withholds its rows, which is recoverable and loud on a profile page; the
// alternative reads the row's entity_id as a collections id and publishes the
// parent slug of whatever collection shares that number.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_UndispositionedCollectionActionIsRefused() {
	actor := suite.createTestUser("undispositionedactor")
	openColl := suite.createCollectionForHistory(actor.ID, "Undispositioned", "undispositioned-history", true)

	// A public collection and its own creator: everything except the missing
	// disposition says this row should be served.
	suite.auditLog.LogAction(actor.ID, "reorder_collection_items", "collection", openColl.ID,
		map[string]interface{}{"slug": openColl.Slug})

	entries, total, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(entries), total)
	suite.Empty(entries, "an action with no entry in contributionCollectionActions must be refused")

	// The control: the same row with a dispositioned action is served.
	suite.auditLog.LogAction(actor.ID, "update_collection", "collection", openColl.ID, nil)
	entries, _, err = suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)
	suite.Equal("update_collection", entries[0].Action)
}

// A CLONE'S ROW SURVIVES; ITS SOURCE ATTRIBUTION DOES NOT, when the source is
// one the viewer may not see.
//
// CloneCollection creates the clone public whatever the source was, so the row
// passes the gate on its own id while its metadata still names the source.
// Removing the two keys keeps the public contribution and drops the private
// attribution.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_ScrubsPrivateCloneSource() {
	actor := suite.createTestUser("cloneactor")
	stranger := suite.createTestUser("clonestranger")

	source := suite.createCollectionForHistory(actor.ID, "Clone Source", "clone-source-history", false)
	clone := suite.createCollectionForHistory(actor.ID, "Clone Result", "clone-result-history", true)
	suite.auditLog.LogAction(actor.ID, "clone_collection", "collection", clone.ID,
		map[string]interface{}{"source_slug": source.Slug, "source_id": source.ID})

	entries, _, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID})
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1, "the clone is public, so the contribution stays")
	suite.Nil(entries[0].Metadata,
		"the two source keys are the only ones this action publishes, so scrubbing them "+
			"must leave the entry answering like one that carried no metadata")

	own, _, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.Require().Len(own, 1)
	suite.Equal(source.Slug, own[0].Metadata["source_slug"],
		"the creator keeps the attribution to their own private source")
}

// A CATALOG MERGE DOES NOT MOVE AN ENTITY-REQUEST ROW ONTO ANOTHER USER'S
// REQUEST.
//
// repointEntityRefs (services/catalog/entity_ref_repoint.go) rewrites
// audit_logs.entity_id keyed on (entity_type, entity_id) alone. Entity-request
// rows store the REQUESTED catalog type in entity_type, so merging the artist
// whose id equals a request's id rewrites that request's rows to the canonical
// artist's number. Deciding those rows by entity_id would then judge them
// against whichever request holds THAT number, which is somebody else's.
//
// The gate reads the metadata's request_id instead, which no merge statement
// touches. This replays the merge's UPDATE against a row whose rewritten
// entity_id names a stranger's request, and the row must still answer for the
// request that wrote it.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_EntityRequestRowsSurviveAMergeRepoint() {
	requester := suite.createTestUser("mergerequester")
	stranger := suite.createTestUser("mergestranger")

	mine := suite.createEntityRequestForHistory(requester.ID, "artist")
	// The request the rewritten entity_id will land on. A DIFFERENT user's, so
	// a gate reading entity_id would serve this row to them and withhold it
	// from its own author.
	theirs := suite.createEntityRequestForHistory(stranger.ID, "artist")

	suite.auditLog.LogAction(requester.ID, "queue_entity_request", "artist", mine.ID,
		map[string]interface{}{"request_id": mine.ID})

	// The merge's own statement, replayed verbatim over the same key.
	suite.Require().NoError(suite.db.Exec(
		"UPDATE audit_logs SET entity_id = ? WHERE entity_type = ? AND entity_id = ?",
		theirs.ID, "artist", mine.ID).Error)

	own, ownTotal, err := suite.profileService.GetContributionHistory(
		requester.ID, 50, 0, "", contracts.ShowViewer{UserID: requester.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(own), ownTotal)
	suite.Require().Len(own, 1, "the author keeps their own row after a merge rewrote its entity_id")
	suite.Equal("queue_entity_request", own[0].Action)

	other, otherTotal, err := suite.profileService.GetContributionHistory(
		requester.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(other), otherTotal)
	suite.Empty(other,
		"the user who owns the request the merge pointed at must not be served this row")
}

// A ROW THAT RECORDS NO USABLE request_id FALLS BACK TO entity_id, which is what
// keeps the rows written before the key was recorded on their own author's
// timeline. A stored 0 names no request and counts as absent, the same reading
// the collection arm gives its own sentinel.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_EntityRequestRowsFallBackToEntityID() {
	requester := suite.createTestUser("fallbackrequester")
	stranger := suite.createTestUser("fallbackstranger")
	request := suite.createEntityRequestForHistory(requester.ID, "artist")

	suite.auditLog.LogAction(requester.ID, "queue_entity_request", "artist", request.ID, nil)
	suite.auditLog.LogAction(requester.ID, "approve_entity_request", "artist", request.ID,
		map[string]interface{}{"request_id": 0})

	own, ownTotal, err := suite.profileService.GetContributionHistory(
		requester.ID, 50, 0, "", contracts.ShowViewer{UserID: requester.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(own), ownTotal)
	suite.Len(own, 2, "a missing key and a zero sentinel both fall back to entity_id")

	for _, viewer := range []contracts.ShowViewer{{}, {UserID: stranger.ID}} {
		refused, refusedTotal, err := suite.profileService.GetContributionHistory(
			requester.ID, 50, 0, "", viewer)
		suite.Require().NoError(err)
		suite.EqualValues(len(refused), refusedTotal)
		suite.Empty(refused, "the fallback is a reference, not a bypass")
	}
}

// A STALE FORK SLUG IS DROPPED EVEN WHEN THE SOURCE IS VISIBLE.
//
// clone_collection freezes the source's slug at clone time while the id beside
// it keeps naming the source. A rename regenerates the slug and frees the old
// string for another collection to claim, so publishing the frozen one names a
// collection this row's gate never looked at. The id survives; the stale slug
// does not.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_DropsAStaleCloneSourceSlug() {
	actor := suite.createTestUser("staleslugactor")
	stranger := suite.createTestUser("staleslugstranger")

	source := suite.createCollectionForHistory(actor.ID, "Stale Source", "stale-source-history", true)
	clone := suite.createCollectionForHistory(actor.ID, "Stale Clone", "stale-clone-history", true)
	suite.auditLog.LogAction(actor.ID, "clone_collection", "collection", clone.ID,
		map[string]interface{}{"source_slug": source.Slug, "source_id": source.ID})

	// The source is renamed, and a DIFFERENT collection takes the freed slug.
	suite.Require().NoError(suite.db.Model(&communitym.Collection{}).
		Where("id = ?", source.ID).Update("slug", "stale-source-history-renamed").Error)
	claimer := suite.createTestUser("staleslugclaimer")
	suite.createCollectionForHistory(claimer.ID, "Claimed", "stale-source-history", false)

	entries, _, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID})
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1, "the clone row is the only audited action here")
	cloneRow := entries[0]
	suite.Require().Equal(contributionCloneAction, cloneRow.Action)
	suite.Require().NotNil(cloneRow.Metadata, "the source is visible, so the id survives")
	suite.NotContains(cloneRow.Metadata, "source_slug",
		"the frozen slug now names another collection and must not be published")
	sourceID, ok := metadataUint(cloneRow.Metadata["source_id"])
	suite.Require().True(ok)
	suite.Equal(source.ID, sourceID, "the id still names the source it was recorded for")
}

// THE PROFILE'S COLLECTION COUNTS ARE NARROWED TOO, because they sit on the same
// public profile as the timeline that now filters the matching rows.
//
// A whole count differenced against a filtered listing is a count of the private
// collections this user contributes to, arrived at by subtraction. The owner's
// own numbers are unchanged, which is what makes this about the viewer rather
// than about the counts having broken.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_NarrowsCollectionCounts() {
	actor := suite.createTestUser("statsactor")
	stranger := suite.createTestUser("statsstranger")

	openColl := suite.createCollectionForHistory(actor.ID, "Stats Open", "stats-open-history", true)
	shutColl := suite.createCollectionForHistory(actor.ID, "Stats Shut", "stats-shut-history", false)
	suite.createCollectionItemForHistory(openColl.ID, actor.ID)
	suite.createCollectionItemForHistory(shutColl.ID, actor.ID)
	for _, c := range []*communitym.Collection{openColl, shutColl} {
		suite.Require().NoError(suite.db.Create(&communitym.CollectionSubscriber{
			CollectionID: c.ID,
			UserID:       actor.ID,
			CreatedAt:    time.Now().UTC(),
		}).Error)
	}

	strangerStats, err := suite.profileService.GetContributionStats(actor.ID, contracts.ShowViewer{UserID: stranger.ID})
	suite.Require().NoError(err)
	suite.EqualValues(1, strangerStats.CollectionItemsAdded,
		"a stranger must not count items added to a private collection")
	suite.EqualValues(1, strangerStats.CollectionSubscriptions,
		"nor subscriptions to one")

	ownStats, err := suite.profileService.GetContributionStats(actor.ID, contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.EqualValues(2, ownStats.CollectionItemsAdded)
	suite.EqualValues(2, ownStats.CollectionSubscriptions)
}

// A RENAMED PARENT DOES NOT CHANGE THE ANSWER, and a collection that takes the
// freed slug does not inherit the old one's rows.
//
// The item branch is decided against the metadata's collection_id, not the slug
// beside it, precisely because a rename regenerates the slug and frees the old
// string for anyone to claim with a public collection.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_AFreedSlugDoesNotRepublishItemRows() {
	actor := suite.createTestUser("slugactor")
	stranger := suite.createTestUser("slugstranger")
	claimer := suite.createTestUser("slugclaimer")

	shut := suite.createCollectionForHistory(actor.ID, "Freed Slug", "freed-slug-history", false)
	item := suite.createCollectionItemForHistory(shut.ID, actor.ID)
	suite.auditLog.LogAction(actor.ID, "add_collection_item", "collection", item.ID,
		map[string]interface{}{"slug": shut.Slug, "collection_id": shut.ID})

	// The private collection lets its slug go, and somebody else takes it with a
	// PUBLIC collection.
	suite.Require().NoError(suite.db.Model(shut).Update("slug", "freed-slug-history-renamed").Error)
	suite.createCollectionForHistory(claimer.ID, "Claimed", "freed-slug-history", true)

	entries, total, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(entries), total)
	suite.Empty(entries,
		"a public collection holding the freed slug must not republish the private parent's rows")

	// The creator still sees it, so the assertion above is about the slug rather
	// than about the row having been lost.
	own, _, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.Len(own, 1)
}

// THE COMMENT AND SUBSCRIPTION AUDIT FAMILIES ARE COLLECTION-TYPED TOO, and an
// action missing from the disposition map is dropped from every timeline
// including its own author's.
//
// These three are written by handlers outside community/collection.go with
// whatever entity type the caller named, so a collection produces a
// collection-typed row that the CASE has to have an answer for.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_CommentAndSubscriptionActionsSurvive() {
	actor := suite.createTestUser("crossactor")
	stranger := suite.createTestUser("crossstranger")

	openColl := suite.createCollectionForHistory(actor.ID, "Cross Open", "cross-open-history", true)
	shutColl := suite.createCollectionForHistory(actor.ID, "Cross Shut", "cross-shut-history", false)
	for _, action := range []string{"create_comment", "subscribe_comments", "unsubscribe_comments", "report_collection"} {
		suite.auditLog.LogAction(actor.ID, action, "collection", openColl.ID, nil)
		suite.auditLog.LogAction(actor.ID, action, "collection", shutColl.ID, nil)
	}

	entries, total, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(entries), total)
	suite.Len(entries, 4, "the public collection's rows survive for a stranger")

	own, ownTotal, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(own), ownTotal)
	suite.Len(own, 8, "the creator sees both collections' rows")
}

// A ROW WRITTEN BEFORE collection_id WAS RECORDED IS STILL LISTED.
//
// The writers record the parent's id now; every item row written before they did
// carries the slug alone. Its entity_id names a collection_items row that may
// since have been hard-deleted, so an id-only rule withholds it from EVERYONE,
// including its own author on a public collection, and from the total as well as
// the page. The slug arm carries exactly those rows and only those: it is
// selected by `collection_id IS NULL`, which no new row can satisfy.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_LegacyItemRowsWithNoParentIDAreStillListed() {
	actor := suite.createTestUser("legacyactor")
	stranger := suite.createTestUser("legacystranger")

	legacyItemIDs := map[uint]uint{}
	openColl := suite.createCollectionForHistory(actor.ID, "Legacy Open", "legacy-open-history", true)
	shutColl := suite.createCollectionForHistory(actor.ID, "Legacy Shut", "legacy-shut-history", false)

	// THE PRE-DEPLOY SHAPE: the slug and nothing else, on an item that has since
	// been removed. A row whose item still existed would pass on the item arm and
	// prove nothing about the legacy one.
	for _, c := range []*communitym.Collection{openColl, shutColl} {
		item := suite.createCollectionItemForHistory(c.ID, actor.ID)
		itemID := item.ID
		suite.Require().NoError(suite.db.Delete(&communitym.CollectionItem{}, itemID).Error)
		legacyItemIDs[c.ID] = itemID
		suite.auditLog.LogAction(actor.ID, "remove_collection_item", "collection", itemID,
			map[string]interface{}{"slug": c.Slug})
	}
	// A legacy row whose slug names no collection at all stays withheld, which is
	// the fail-closed answer every other arm gives.
	suite.auditLog.LogAction(actor.ID, "remove_collection_item", "collection", 987655,
		map[string]interface{}{"slug": "a-slug-that-never-existed"})

	entries, total, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(entries), total, "the total must count the same rows the page contains")
	suite.Require().Len(entries, 1,
		"a legacy row on a PUBLIC collection is still listed to a stranger")
	suite.EqualValues(legacyItemIDs[openColl.ID], entries[0].EntityID)
	suite.Nil(entries[0].Metadata,
		"a legacy row passes on its slug and still publishes none: a rename frees "+
			"the string for another collection to take")

	own, ownTotal, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(own), ownTotal)
	suite.Len(own, 2,
		"the author sees both real legacy rows and neither the unresolvable one")
}

// A ROW CARRYING THE ZERO SENTINEL IS STILL LISTED.
//
// Id 0 matches no collection, so a row stamped `"collection_id": 0` passes the
// parent-id arm no more than a row with no key at all does: its item is gone,
// the id resolves to nothing, and reading the key as PRESENT would leave the row
// passing no arm and withheld from everyone including its own author on a public
// collection. The gate reads the sentinel as absent and the row falls to the
// slug arm. Rows in this shape exist; the writers can no longer produce one.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_ZeroSentinelParentRowsAreStillListed() {
	actor := suite.createTestUser("zeroparentactor")
	stranger := suite.createTestUser("zeroparentstranger")

	openColl := suite.createCollectionForHistory(actor.ID, "Zero Parent Open", "zero-parent-open", true)
	shutColl := suite.createCollectionForHistory(actor.ID, "Zero Parent Shut", "zero-parent-shut", false)

	zeroItemIDs := map[uint]uint{}
	seedZeroRow := func(c *communitym.Collection) {
		item := suite.createCollectionItemForHistory(c.ID, actor.ID)
		itemID := item.ID
		zeroItemIDs[c.ID] = itemID
		suite.Require().NoError(suite.db.Delete(&communitym.CollectionItem{}, itemID).Error)
		suite.auditLog.LogAction(actor.ID, "remove_collection_item", "collection", itemID,
			map[string]interface{}{"slug": c.Slug, "collection_id": 0})
	}
	seedZeroRow(openColl)
	seedZeroRow(shutColl)

	entries, total, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(entries), total)
	suite.Require().Len(entries, 1, "the sentinel row on a PUBLIC collection is listed to a stranger")
	suite.EqualValues(zeroItemIDs[openColl.ID], entries[0].EntityID)
	suite.Nil(entries[0].Metadata)

	// AND THE PRIVATE ONE IS NOT. Falling to the slug arm is a fallback, not a
	// bypass: the arm still decides the named collection against the viewer.
	own, ownTotal, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(own), ownTotal)
	suite.Len(own, 2, "the creator sees both")
}

// THE HEATMAP'S UNDECIDED ARMS REFUSE A GATED DISCRIMINATOR.
//
// pending_entity_edits and entity_edit_audit_logs are counted without a per-row
// visibility test, on the evidence that their writers record catalog types with
// no read-time rule. entity_edit_audit_logs.entity_type has no allowlist behind
// it, so the arms exclude the gated discriminators outright rather than trusting
// that evidence: a row naming a show or a collection is not counted, whoever is
// asking, including the actor themselves.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_UndecidedArmsRefuseGatedTypes() {
	actor := suite.createTestUser("undecidedarmsactor")
	shutColl := suite.createCollectionForHistory(actor.ID, "Arms Shut", "arms-shut", false)

	dayTotal := func() int {
		heatmap, err := suite.profileService.GetActivityHeatmap(actor.ID, contracts.ShowViewer{UserID: actor.ID})
		suite.Require().NoError(err)
		total := 0
		for _, d := range heatmap.Days {
			total += d.Count
		}
		return total
	}
	before := dayTotal()

	// An UNGATED type in the same table is the control: it is counted, so a zero
	// delta below is the exclusion rather than an arm that counts nothing.
	suite.Require().NoError(suite.db.Create(&adminm.EntityEditAuditLog{
		ActorID: &actor.ID, EntityType: "artist", EntityID: 1, CreatedAt: time.Now().UTC(),
	}).Error)
	suite.Equal(before+1, dayTotal(), "an artist edit is counted, which is the control")

	// The gated pair, in both undecided tables.
	suite.Require().NoError(suite.db.Create(&adminm.EntityEditAuditLog{
		ActorID: &actor.ID, EntityType: "collection", EntityID: shutColl.ID, CreatedAt: time.Now().UTC(),
	}).Error)
	suite.Require().NoError(suite.db.Create(&adminm.EntityEditAuditLog{
		ActorID: &actor.ID, EntityType: "show", EntityID: 987654, CreatedAt: time.Now().UTC(),
	}).Error)
	suite.Require().NoError(suite.db.Exec(
		`INSERT INTO pending_entity_edits (entity_type, entity_id, submitted_by, field_changes, summary, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?::jsonb, ?, ?, NOW(), NOW())`,
		"collection", shutColl.ID, actor.ID, `{"title":"x"}`, "a gated pending edit",
		adminm.PendingEditStatusPending).Error)

	suite.Equal(before+1, dayTotal(),
		"a show- or collection-typed row in either undecided arm is not counted")
}

// THE HEATMAP AND THE TIMELINE ANSWER FOR THE SAME ROWS. Both routes are
// anonymous and both read audit_logs for the same actor, so a day the heatmap
// counts while the timeline withholds locates a private collection to the day it
// was touched, and differencing the two reports how many actions were taken on
// it.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_WithholdsPrivateCollectionDays() {
	actor := suite.createTestUser("heatmapactor")
	stranger := suite.createTestUser("heatmapstranger")

	openColl := suite.createCollectionForHistory(actor.ID, "Heatmap Open", "heatmap-open", true)
	shutColl := suite.createCollectionForHistory(actor.ID, "Heatmap Shut", "heatmap-shut", false)
	suite.auditLog.LogAction(actor.ID, "create_collection", "collection", openColl.ID,
		map[string]interface{}{"slug": openColl.Slug})
	suite.auditLog.LogAction(actor.ID, "create_collection", "collection", shutColl.ID,
		map[string]interface{}{"slug": shutColl.Slug})

	dayTotal := func(viewer contracts.ShowViewer) int {
		heatmap, err := suite.profileService.GetActivityHeatmap(actor.ID, viewer)
		suite.Require().NoError(err)
		total := 0
		for _, d := range heatmap.Days {
			total += d.Count
		}
		return total
	}

	strangerDays := dayTotal(contracts.ShowViewer{UserID: stranger.ID})
	anonDays := dayTotal(contracts.ShowViewer{})
	ownerDays := dayTotal(contracts.ShowViewer{UserID: actor.ID})

	suite.Equal(1, strangerDays, "a stranger counts only the public collection's action")
	suite.Equal(1, anonDays, "and so does an anonymous caller")
	suite.Equal(2, ownerDays, "the creator counts both, which is the control")

	// The TIMELINE agrees, which is the property that closes the subtraction.
	//
	// THE FIXTURE IS THE BOUND on this equality and cannot be removed from it:
	// the heatmap unions six sources and the timeline five, so the two totals are
	// comparable only because this actor has nothing in the sources they do not
	// share. The timeline side is restricted to audit rows to say which source is
	// under test; adding a show, venue, pending edit or revision for this actor
	// breaks the equality and the fixture, not the gate, is what to fix.
	entries, _, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID})
	suite.Require().NoError(err)
	auditEntries := 0
	for _, e := range entries {
		if e.Source != "audit_log" {
			suite.Failf("fixture drift",
				"this actor has a %s row, which only the heatmap counts; the equality below is no longer meaningful",
				e.Source)
		}
		auditEntries++
	}
	suite.Equal(auditEntries, strangerDays,
		"the heatmap and the timeline must count the same audit rows for the same viewer")
}

// THE AUDIT-SOURCED COUNTERS MOVE WITH THE VIEWER, and they agree with the
// timeline that lists the same rows.
//
// moderation_actions counts approve_show and reject_show, and a REJECTED show is
// gated by definition, so the whole count sat beside a filtered listing of its
// own rows: subtracting one from the other reported how many gated shows this
// moderator had touched. Both now read audit_logs through one condition.
//
// The tag-vote count is here for the same reason on a different table: tag_votes
// is polymorphic, so a vote on a gated show or a private collection was counted
// for everyone.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_NarrowsGatedAuditRows() {
	moderator := suite.createTestUser("gatedauditmod")
	submitter := suite.createTestUser("gatedauditsubmitter")

	open := suite.createShow(submitter.ID, "Open Show")
	gated := suite.createShow(submitter.ID, "Gated Show")
	suite.Require().NoError(
		suite.db.Model(&catalogm.Show{}).Where("id = ?", gated.ID).
			Update("status", catalogm.ShowStatusRejected).Error)

	suite.auditLog.LogAction(moderator.ID, "approve_show", "show", open.ID, nil)
	suite.auditLog.LogAction(moderator.ID, "reject_show", "show", gated.ID, nil)

	// One tag vote on each show, cast by the moderator.
	for i, showID := range []uint{open.ID, gated.ID} {
		tag := &catalogm.Tag{
			Name:     fmt.Sprintf("gated audit tag %d", i),
			Slug:     fmt.Sprintf("gated-audit-tag-%d-%d", i, showID),
			Category: "genre",
		}
		suite.Require().NoError(suite.db.Create(tag).Error)
		suite.Require().NoError(suite.db.Create(&catalogm.TagVote{
			TagID: tag.ID, EntityType: "show", EntityID: showID,
			UserID: moderator.ID, Vote: 1,
		}).Error)
	}

	anonymous, err := suite.profileService.GetContributionStats(moderator.ID, contracts.ShowViewer{})
	suite.Require().NoError(err)
	suite.Equal(int64(1), anonymous.ModerationActions,
		"the rejected show's moderation row is still counted for an anonymous reader")
	suite.Equal(int64(1), anonymous.TagVotesCast,
		"the tag vote on the rejected show is still counted for an anonymous reader")

	// The submitter sees their own gated show, so both of its rows come back.
	toSubmitter, err := suite.profileService.GetContributionStats(
		moderator.ID, contracts.ShowViewer{UserID: submitter.ID})
	suite.Require().NoError(err)
	suite.Equal(int64(2), toSubmitter.ModerationActions)
	suite.Equal(int64(2), toSubmitter.TagVotesCast)

	// An admin sees every show, so the counts are whole for them too. This arm is
	// what proves the narrowing is a VISIBILITY test rather than a filter that
	// drops non-approved rows for everybody.
	toAdmin, err := suite.profileService.GetContributionStats(
		moderator.ID, contracts.ShowViewer{UserID: moderator.ID, IsAdmin: true})
	suite.Require().NoError(err)
	suite.Equal(int64(2), toAdmin.ModerationActions)
	suite.Equal(int64(2), toAdmin.TagVotesCast)

	// AND THE COUNT IS A SUBSET OF THE LISTING it sits beside, for the same
	// viewer. That direction is the property: a row the timeline withholds must
	// not be counted, or the count reports the withheld row as arithmetic.
	//
	// The rows are matched BY ACTION rather than by comparing two totals. The
	// show-scoped timeline also carries this actor's own show submissions, which
	// no counter here reads, so a total-to-total equality would hold only for a
	// fixture whose moderator submitted nothing.
	//
	// The comparison is still like-for-like only while every moderation row this
	// actor has is show-typed, which is what the fixture seeds. Asserted rather
	// than assumed, so a later fixture that adds a venue or report action fails
	// with a sentence saying why instead of an unexplained off-by-one.
	var nonShowModeration int64
	suite.Require().NoError(suite.db.Model(&adminm.AuditLog{}).
		Where("actor_id = ?", moderator.ID).
		Where("action IN ?", moderationActionNameList()).
		Where("entity_type NOT IN ?", []string{"show", "show_edit"}).
		Count(&nonShowModeration).Error)
	suite.Require().Zero(nonShowModeration,
		"this fixture's moderator has a moderation row that is not show-typed, so the "+
			"show-scoped timeline below cannot list it and the counts are no longer "+
			"comparable; seed it show-typed or widen the timeline query")

	entries, total, err := suite.profileService.GetContributionHistory(
		moderator.ID, 50, 0, "show", contracts.ShowViewer{})
	suite.Require().NoError(err)
	suite.Require().Len(entries, int(total), "the timeline's page and total disagree")

	listedModeration := int64(0)
	for _, e := range entries {
		if moderationActionNames[e.Action] {
			listedModeration++
		}
	}
	suite.Equal(anonymous.ModerationActions, listedModeration,
		"the anonymous moderation count and the moderation rows the anonymous timeline "+
			"lists disagree, so one of them is counting a row the other withholds")
}

// tags_applied IS THE PUBLIC-TIER COUNT, on the value as well as the position.
//
// entity_tags is polymorphic, so a row can name a gated show or a private
// collection, and this route publishes the number as a VALUE for a NAMED user
// with no viewer to vary by. The leaderboard's tags dimension already reports
// the public-tier count for the same user, so a whole count here was that user's
// private collections and gated shows recoverable by subtracting one public
// number from another.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPercentileRankings_TagsAppliedIsPublicTier() {
	// The route answers nil below ten active users, so the cohort is seeded first.
	users := suite.createManyUsers("rankingcohort", 12)
	tagger := users[0]

	open := suite.createShow(tagger.ID, "Ranked Open Show")
	gated := suite.createShow(tagger.ID, "Ranked Gated Show")
	suite.Require().NoError(
		suite.db.Model(&catalogm.Show{}).Where("id = ?", gated.ID).
			Update("status", catalogm.ShowStatusRejected).Error)

	tag := &catalogm.Tag{Name: "ranked tag", Slug: "ranked-tag", Category: "genre"}
	suite.Require().NoError(suite.db.Create(tag).Error)
	for _, showID := range []uint{open.ID, gated.ID} {
		suite.Require().NoError(suite.db.Create(&catalogm.EntityTag{
			TagID: tag.ID, EntityType: "show", EntityID: showID, AddedByUserID: tagger.ID,
		}).Error)
	}

	// A PEER whose only tag is on the rejected show. This row is what separates
	// the two halves of the gate: the value comes from the subject's own count,
	// the POSITION comes from the cohort join, and without the predicate on that
	// join this peer counts 1 and stops being "a user with fewer than the
	// subject", moving the percentile. With it, the peer counts 0.
	// A second tag, because entity_tags is unique on (tag, entity): the peer has
	// to apply a different one to the same gated show.
	peer := users[1]
	peerTag := &catalogm.Tag{Name: "ranked peer tag", Slug: "ranked-peer-tag", Category: "genre"}
	suite.Require().NoError(suite.db.Create(peerTag).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.EntityTag{
		TagID: peerTag.ID, EntityType: "show", EntityID: gated.ID, AddedByUserID: peer.ID,
	}).Error)

	result, err := suite.profileService.GetPercentileRankings(tagger.ID)
	suite.Require().NoError(err)

	var tagsApplied *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "tags_applied" {
			tagsApplied = &result.Rankings[i]
		}
	}
	suite.Require().NotNil(tagsApplied, "tags_applied is missing from the rankings")
	suite.Equal(int64(1), tagsApplied.Value,
		"the tag on the rejected show is still counted, so subtracting the leaderboard's "+
			"public-tier count for this user reports it")

	// Eleven of the twelve users hold no visible tag, the peer included, so the
	// subject sits above all eleven.
	suite.Equal(11*100/12, tagsApplied.Percentile,
		"the cohort counts the peer's tag on the rejected show, so the position is "+
			"measured against a population counted differently from the subject")
}

// =============================================================================
// PSY-2015: the metadata allowlist and the entity-request id space
// =============================================================================

// createEntityRequestForHistory writes an entity_requests row, which is the
// table the entity-request arm decides its audit rows against.
func (suite *ContributorProfileServiceIntegrationTestSuite) createEntityRequestForHistory(
	requesterID uint, entityType string,
) *communitym.EntityRequest {
	payload := json.RawMessage(`{"name":"Requested Thing"}`)
	request := &communitym.EntityRequest{
		EntityType:    entityType,
		Payload:       &payload,
		RequesterID:   requesterID,
		SourceContext: communitym.EntityRequestSourceManual,
		DecisionState: communitym.EntityRequestStatePending,
	}
	suite.Require().NoError(suite.db.Create(request).Error)
	return request
}

// ONLY ALLOWLISTED KEYS REACH THE RESPONSE, and the default is none.
//
// The stored document is what the writers happened to record; the response is
// what contributionMetadataKeys says the action may publish. Three rows here
// carry the three shapes that matter: a moderation artifact, an id naming an
// entity the row's own gate never looked at, and an allowlisted key.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_ProjectsOnlyAllowlistedMetadata() {
	actor := suite.createTestUser("allowlistactor")
	gated := suite.createShow(actor.ID, "Gated Show")
	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).Where("id = ?", gated.ID).
		Update("status", "rejected").Error)
	collection := suite.createCollectionForHistory(actor.ID, "Allowlist Picks", "allowlist-picks", true)
	source := suite.createCollectionForHistory(actor.ID, "Allowlist Source", "allowlist-source", true)

	// An admin's prose about another user's submission.
	suite.auditLog.LogAction(actor.ID, "reject_show", "show", gated.ID, map[string]interface{}{
		"reason":   "spam, do not resubmit",
		"batch":    true,
		"category": "spam",
	})
	// A report row: entity_type "show_report" passes every entity-type arm
	// untouched, so nothing has decided the show_id it records.
	suite.auditLog.LogAction(actor.ID, "dismiss_report", "show_report", 4242, map[string]interface{}{
		"show_id": gated.ID,
		"notes":   "reporter is a repeat offender",
	})
	// The allowlisted control, plus two keys on the same row that are not:
	// entity_type and entity_id name the item's own subject, which no arm on
	// this row has looked at.
	item := suite.createCollectionItemForHistory(collection.ID, actor.ID)
	suite.auditLog.LogAction(actor.ID, "add_collection_item", "collection", item.ID,
		map[string]interface{}{
			"slug":          collection.Slug,
			"collection_id": collection.ID,
			"entity_type":   "show",
			"entity_id":     gated.ID,
		})
	suite.auditLog.LogAction(actor.ID, "clone_collection", "collection", collection.ID,
		map[string]interface{}{
			"source_slug": source.Slug,
			"source_id":   source.ID,
			// NOT ALLOWLISTED, on the one action that publishes anything. This
			// is what makes the assertion below about the per-key projection
			// rather than about the action's presence in the map.
			"source_creator_id": actor.ID,
		})

	// THE OWNER'S OWN VIEW, which is the widest tier this endpoint serves: the
	// allowlist does not vary by viewer, so what the owner sees is the ceiling.
	entries, _, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.Require().Len(entries, 5,
		"two moderation rows, the item row, the clone row and the show submission")

	byAction := map[string]*contracts.ContributionEntry{}
	for _, e := range entries {
		byAction[e.Action] = e
	}

	suite.Require().Contains(byAction, "reject_show")
	suite.Nil(byAction["reject_show"].Metadata,
		"an admin's rejection reason must not be published under the actor's username")

	suite.Require().Contains(byAction, "dismiss_report")
	suite.Nil(byAction["dismiss_report"].Metadata,
		"a moderation note, and the id of the show it names, must not be published")

	suite.Require().Contains(byAction, "add_collection_item")
	suite.Nil(byAction["add_collection_item"].Metadata,
		"the item's own subject is a gated show, its parent slug can be stale, and "+
			"the collection family publishes neither")

	// THE ONE PUBLISHED PAIR, so the assertions above are about the allowlist
	// rather than about the projection having dropped everything.
	suite.Require().Contains(byAction, "clone_collection")
	cloneMetadata := byAction["clone_collection"].Metadata
	suite.Require().NotNil(cloneMetadata, "the fork attribution is allowlisted")
	suite.Equal(source.Slug, cloneMetadata["source_slug"])
	// Read back through the SERVICE's own reader, so the test cannot decide a
	// JSON number differently from the scrub that gates this very key.
	sourceID, ok := metadataUint(cloneMetadata["source_id"])
	suite.Require().True(ok, "source_id did not read back as an id")
	suite.Equal(source.ID, sourceID)
	suite.NotContains(cloneMetadata, "source_creator_id",
		"an unlisted key on an action that publishes other keys must still be dropped")
}

// AN ACTION WITH NO ENTRY IN THE ALLOWLIST PUBLISHES NOTHING, which is the same
// answer an entry naming no key gives. The row itself is served either way.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_UndispositionedActionPublishesNoMetadata() {
	actor := suite.createTestUser("undispositionedmeta")

	suite.auditLog.LogAction(actor.ID, "an_action_nobody_dispositioned", "artist", 1,
		map[string]interface{}{"secret": "value"})

	entries, total, err := suite.profileService.GetContributionHistory(
		actor.ID, 50, 0, "", contracts.ShowViewer{UserID: actor.ID})
	suite.Require().NoError(err)
	suite.EqualValues(len(entries), total)
	suite.Require().Len(entries, 1, "the ROW is served; only its metadata is withheld")
	suite.Nil(entries[0].Metadata)
}

// ENTITY-REQUEST ROWS ARE DECIDED AGAINST entity_requests, NEVER AGAINST THE
// TABLE THEIR entity_type NAMES.
//
// The writers record the REQUESTED type in entity_type and the REQUEST's id in
// entity_id, so the show arm was testing a request id against shows.id: a
// request was withheld or served on the basis of whichever show happened to
// share its number.
//
// The rule: requester or admin. Four routes read entity_requests and each serves
// one of those two tiers: POST /entity-requests hands the row back to the
// requester who filed it, and the admin list, decide and fulfill routes serve an
// admin. Anonymous and stranger tiers get nothing anywhere, and this rule is
// those two tiers written as a predicate.
//
// Four tiers, and the total moves with the page in every one of them.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_EntityRequestRowsAreDecidedAgainstEntityRequests() {
	requester := suite.createTestUser("requestactor")
	stranger := suite.createTestUser("requeststranger")

	// A show whose id is made to COLLIDE with the request's, which is the number
	// the show arm was reading. Approved and public, so the id-space mistake
	// served this request row to every tier on the strength of an unrelated show.
	request := suite.createEntityRequestForHistory(requester.ID, "show")
	collidingShow := suite.createShow(requester.ID, "Unrelated Colliding Show")
	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).
		Where("id = ?", collidingShow.ID).Update("id", request.ID).Error)

	suite.auditLog.LogAction(requester.ID, "queue_entity_request", "show", request.ID,
		map[string]interface{}{"request_id": request.ID, "decision_state": "pending"})

	// The colliding show is still this actor's own approved submission, so every
	// tier sees that one row whatever it is told about the request.
	const submissionRows = 1

	for _, tier := range []struct {
		name      string
		viewer    contracts.ShowViewer
		seesRow   bool
		reasoning string
	}{
		{"anonymous", contracts.ShowViewer{}, false,
			"a pending request names unpublished content and no route serves it anonymously"},
		{"stranger", contracts.ShowViewer{UserID: stranger.ID}, false,
			"a signed-in stranger is the same tier as an anonymous reader here"},
		{"requester", contracts.ShowViewer{UserID: requester.ID}, true,
			"the requester filed the row and already holds its content"},
		{"admin", contracts.ShowViewer{UserID: stranger.ID, IsAdmin: true}, true,
			"GET /admin/entity-requests already serves an admin the whole row"},
	} {
		entries, total, err := suite.profileService.GetContributionHistory(
			requester.ID, 50, 0, "", tier.viewer)
		suite.Require().NoError(err, tier.name)
		suite.EqualValues(len(entries), total,
			"%s: the total must count the same rows the page contains", tier.name)

		var sawRequest bool
		for _, e := range entries {
			if e.Action == "queue_entity_request" {
				sawRequest = true
				suite.Empty(e.EntityName,
					"%s: a request id resolved a name out of the shows table", tier.name)
				suite.Nil(e.Metadata,
					"%s: the entity-request family publishes no metadata key", tier.name)
			}
		}
		suite.Equal(tier.seesRow, sawRequest, "%s: %s", tier.name, tier.reasoning)

		expected := submissionRows
		if tier.seesRow {
			expected++
		}
		suite.Len(entries, expected, "%s", tier.name)
	}
}

// A REQUEST THAT NO LONGER EXISTS IS WITHHELD FROM EVERYONE, including the
// actor who filed it and an admin. Absent and refused are one answer, which is
// what stops the pair from enumerating the request id space.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_DeletedEntityRequestRowsAreWithheld() {
	requester := suite.createTestUser("deletedrequestactor")
	request := suite.createEntityRequestForHistory(requester.ID, "artist")
	requestID := request.ID
	suite.Require().NoError(suite.db.Delete(&communitym.EntityRequest{}, requestID).Error)

	suite.auditLog.LogAction(requester.ID, "queue_entity_request", "artist", requestID,
		map[string]interface{}{"request_id": requestID})

	for _, viewer := range []contracts.ShowViewer{
		{},
		{UserID: requester.ID},
		{UserID: requester.ID, IsAdmin: true},
	} {
		entries, total, err := suite.profileService.GetContributionHistory(
			requester.ID, 50, 0, "", viewer)
		suite.Require().NoError(err)
		suite.EqualValues(len(entries), total)
		suite.Empty(entries, "a row naming no request must be withheld from every tier")
	}
}

// THE HEATMAP WITHHOLDS THE SAME DAYS THE TIMELINE WITHHOLDS.
//
// Both routes are anonymous and both read audit_logs for the same actor from one
// condition builder. A day counted here that the timeline does not list would
// locate a pending request to the day it was filed, and differencing the two
// reports how many were filed that day.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_WithholdsEntityRequestDays() {
	requester := suite.createTestUser("heatmaprequester")
	request := suite.createEntityRequestForHistory(requester.ID, "artist")
	suite.auditLog.LogAction(requester.ID, "queue_entity_request", "artist", request.ID,
		map[string]interface{}{"request_id": request.ID})

	anon, err := suite.profileService.GetActivityHeatmap(requester.ID, contracts.ShowViewer{})
	suite.Require().NoError(err)
	suite.Empty(anon.Days, "an anonymous reader must not be told the day a request was filed")

	own, err := suite.profileService.GetActivityHeatmap(
		requester.ID, contracts.ShowViewer{UserID: requester.ID})
	suite.Require().NoError(err)
	suite.Require().Len(own.Days, 1, "the requester's own day is counted")
	suite.Equal(1, own.Days[0].Count)
}
