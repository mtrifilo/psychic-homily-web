package engagement

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestFieldNote_NilDB(t *testing.T) {
	svc := NewCommentService(nil)

	t.Run("CreateFieldNote_NilDB", func(t *testing.T) {
		_, err := svc.CreateFieldNote(1, &contracts.CreateFieldNoteRequest{
			ShowID: 1,
			Body:   "test",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("ListFieldNotesForShow_NilDB", func(t *testing.T) {
		_, err := svc.ListFieldNotesForShow(1, 20, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})
}

func TestFieldNote_BodyValidation(t *testing.T) {
	svc := &CommentService{db: &gorm.DB{}}

	t.Run("CreateFieldNote_EmptyBody", func(t *testing.T) {
		_, err := svc.CreateFieldNote(1, &contracts.CreateFieldNoteRequest{
			ShowID: 1,
			Body:   "",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "field note body is required")
	})

	t.Run("CreateFieldNote_WhitespaceBody", func(t *testing.T) {
		_, err := svc.CreateFieldNote(1, &contracts.CreateFieldNoteRequest{
			ShowID: 1,
			Body:   "   \n\t  ",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "field note body is required")
	})

	t.Run("CreateFieldNote_TooLongBody", func(t *testing.T) {
		longBody := make([]byte, models.MaxCommentBodyLength+1)
		for i := range longBody {
			longBody[i] = 'a'
		}
		_, err := svc.CreateFieldNote(1, &contracts.CreateFieldNoteRequest{
			ShowID: 1,
			Body:   string(longBody),
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum length")
	})
}

func TestFieldNote_RatingValidation(t *testing.T) {
	svc := &CommentService{db: &gorm.DB{}}

	t.Run("SoundQuality_Zero", func(t *testing.T) {
		val := 0
		_, err := svc.CreateFieldNote(1, &contracts.CreateFieldNoteRequest{
			ShowID:       1,
			Body:         "test note",
			SoundQuality: &val,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sound_quality must be between 1 and 5")
	})

	t.Run("SoundQuality_Six", func(t *testing.T) {
		val := 6
		_, err := svc.CreateFieldNote(1, &contracts.CreateFieldNoteRequest{
			ShowID:       1,
			Body:         "test note",
			SoundQuality: &val,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sound_quality must be between 1 and 5")
	})

	t.Run("SoundQuality_Negative", func(t *testing.T) {
		val := -1
		_, err := svc.CreateFieldNote(1, &contracts.CreateFieldNoteRequest{
			ShowID:       1,
			Body:         "test note",
			SoundQuality: &val,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sound_quality must be between 1 and 5")
	})

	t.Run("CrowdEnergy_Zero", func(t *testing.T) {
		val := 0
		_, err := svc.CreateFieldNote(1, &contracts.CreateFieldNoteRequest{
			ShowID:      1,
			Body:        "test note",
			CrowdEnergy: &val,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "crowd_energy must be between 1 and 5")
	})

	t.Run("CrowdEnergy_Six", func(t *testing.T) {
		val := 6
		_, err := svc.CreateFieldNote(1, &contracts.CreateFieldNoteRequest{
			ShowID:      1,
			Body:        "test note",
			CrowdEnergy: &val,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "crowd_energy must be between 1 and 5")
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type FieldNoteIntegrationTestSuite struct {
	suite.Suite
	testDB         *testutil.TestDatabase
	db             *gorm.DB
	commentService *CommentService
}

func (suite *FieldNoteIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.commentService = NewCommentService(suite.testDB.DB)
}

func (suite *FieldNoteIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *FieldNoteIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM comment_subscriptions")
	_, _ = sqlDB.Exec("DELETE FROM comment_votes")
	_, _ = sqlDB.Exec("DELETE FROM comment_edits")
	_, _ = sqlDB.Exec("DELETE FROM comments")
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestFieldNoteIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(FieldNoteIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *FieldNoteIntegrationTestSuite) createTestUser() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("fn-user-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("fnuser%d", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
		UserTier:      "contributor", // contributor tier auto-publishes comments
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *FieldNoteIntegrationTestSuite) createTestNewUser() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("fn-newuser-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("fnnewuser%d", time.Now().UnixNano())),
		FirstName:     stringPtr("New"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
		UserTier:      "new_user",
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *FieldNoteIntegrationTestSuite) createPastShow(title string, daysAgo int) uint {
	slug := fmt.Sprintf("%s-%d", title, time.Now().UnixNano())
	show := &models.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: time.Now().Add(-time.Duration(daysAgo) * 24 * time.Hour),
		Status:    models.ShowStatusApproved,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show.ID
}

func (suite *FieldNoteIntegrationTestSuite) createFutureShow(title string) uint {
	slug := fmt.Sprintf("%s-%d", title, time.Now().UnixNano())
	show := &models.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: time.Now().Add(7 * 24 * time.Hour),
		Status:    models.ShowStatusApproved,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show.ID
}

func (suite *FieldNoteIntegrationTestSuite) createTestArtist(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	artist := &models.Artist{
		Name: name,
		Slug: &slug,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist.ID
}

func (suite *FieldNoteIntegrationTestSuite) addArtistToShow(showID, artistID uint, position int) {
	sa := &models.ShowArtist{
		ShowID:   showID,
		ArtistID: artistID,
		Position: position,
		SetType:  "performer",
	}
	err := suite.db.Create(sa).Error
	suite.Require().NoError(err)
}

func (suite *FieldNoteIntegrationTestSuite) markGoing(userID, showID uint, createdAt time.Time) {
	bookmark := &models.UserBookmark{
		UserID:     userID,
		EntityType: models.BookmarkEntityShow,
		EntityID:   showID,
		Action:     models.BookmarkActionGoing,
		CreatedAt:  createdAt,
	}
	err := suite.db.Create(bookmark).Error
	suite.Require().NoError(err)
}

// insertFieldNote creates a field note directly in the DB, bypassing rate limiting.
func (suite *FieldNoteIntegrationTestSuite) insertFieldNote(userID, showID uint, body string, sd *contracts.FieldNoteStructuredData) *models.Comment {
	svc := suite.commentService
	bodyHTML := svc.renderMarkdown(body)
	comment := &models.Comment{
		EntityType:      models.CommentEntityShow,
		EntityID:        showID,
		Kind:            models.CommentKindFieldNote,
		UserID:          userID,
		Body:            body,
		BodyHTML:        bodyHTML,
		Visibility:      models.CommentVisibilityVisible,
		ReplyPermission: models.ReplyPermissionAnyone,
	}
	if sd != nil {
		sdJSON, err := json.Marshal(sd)
		suite.Require().NoError(err)
		raw := json.RawMessage(sdJSON)
		comment.StructuredData = &raw
	}
	err := suite.db.Create(comment).Error
	suite.Require().NoError(err)
	return comment
}

// =============================================================================
// Group 1: CreateFieldNote — Basic CRUD
// =============================================================================

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_AllFields() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Test Show", 3)
	artistID := suite.createTestArtist("Opener Band")
	suite.addArtistToShow(showID, artistID, 0)

	soundQuality := 4
	crowdEnergy := 5
	songPosition := 2
	notableMoments := "Surprise cover of Ziggy Stardust"

	fieldNote, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID:         showID,
		Body:           "Incredible show tonight. **Best I've seen all year.**",
		ShowArtistID:   &artistID,
		SongPosition:   &songPosition,
		SoundQuality:   &soundQuality,
		CrowdEnergy:    &crowdEnergy,
		NotableMoments: &notableMoments,
		SetlistSpoiler: true,
	})
	suite.Require().NoError(err)
	suite.NotZero(fieldNote.ID)
	suite.Equal("show", fieldNote.EntityType)
	suite.Equal(showID, fieldNote.EntityID)
	suite.Equal("field_note", fieldNote.Kind)
	suite.Equal(user.ID, fieldNote.UserID)
	suite.Contains(fieldNote.BodyHTML, "<strong>")
	suite.Equal("visible", fieldNote.Visibility)
	suite.Equal(0, fieldNote.Depth)
	suite.Nil(fieldNote.ParentID)
	suite.Nil(fieldNote.RootID)

	// Verify structured data
	suite.NotNil(fieldNote.StructuredData)
	var sd contracts.FieldNoteStructuredData
	err = json.Unmarshal(*fieldNote.StructuredData, &sd)
	suite.Require().NoError(err)
	suite.Equal(&artistID, sd.ShowArtistID)
	suite.Equal(&songPosition, sd.SongPosition)
	suite.Equal(&soundQuality, sd.SoundQuality)
	suite.Equal(&crowdEnergy, sd.CrowdEnergy)
	suite.Equal(&notableMoments, sd.NotableMoments)
	suite.True(sd.SetlistSpoiler)
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_MinimalFields() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Minimal Show", 1)

	fieldNote, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: showID,
		Body:   "Just a brief note about the show.",
	})
	suite.Require().NoError(err)
	suite.NotZero(fieldNote.ID)
	suite.Equal("field_note", fieldNote.Kind)
	suite.Equal("show", fieldNote.EntityType)

	// Structured data should still be present but with nil optional fields
	suite.NotNil(fieldNote.StructuredData)
	var sd contracts.FieldNoteStructuredData
	err = json.Unmarshal(*fieldNote.StructuredData, &sd)
	suite.Require().NoError(err)
	suite.Nil(sd.ShowArtistID)
	suite.Nil(sd.SongPosition)
	suite.Nil(sd.SoundQuality)
	suite.Nil(sd.CrowdEnergy)
	suite.Nil(sd.NotableMoments)
	suite.False(sd.SetlistSpoiler)
	suite.False(sd.IsVerifiedAttendee)
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_MarkdownRendered() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Markdown Show", 2)

	fieldNote, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: showID,
		Body:   "**bold** and *italic*",
	})
	suite.Require().NoError(err)
	suite.Contains(fieldNote.BodyHTML, "<strong>bold</strong>")
	suite.Contains(fieldNote.BodyHTML, "<em>italic</em>")
}

// =============================================================================
// Group 2: CreateFieldNote — Validation
// =============================================================================

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_FutureShow_Rejected() {
	user := suite.createTestUser()
	showID := suite.createFutureShow("Future Show")

	_, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: showID,
		Body:   "Can't post field notes for future shows.",
	})
	suite.Error(err)
	suite.Contains(err.Error(), "field notes can only be added to past shows")
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_ShowNotFound() {
	user := suite.createTestUser()

	_, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: 999999,
		Body:   "No such show.",
	})
	suite.Error(err)
	suite.Contains(err.Error(), "show not found")
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_ArtistNotOnShow() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Artist Show", 2)
	unrelatedArtistID := suite.createTestArtist("Unrelated Artist")

	_, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID:       showID,
		Body:         "About this artist...",
		ShowArtistID: &unrelatedArtistID,
	})
	suite.Error(err)
	suite.Contains(err.Error(), "artist is not on this show's bill")
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_SoundQualityInvalid() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Sound Show", 2)

	zero := 0
	_, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID:       showID,
		Body:         "Bad sound quality value",
		SoundQuality: &zero,
	})
	suite.Error(err)
	suite.Contains(err.Error(), "sound_quality must be between 1 and 5")

	six := 6
	_, err = suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID:       showID,
		Body:         "Bad sound quality value",
		SoundQuality: &six,
	})
	suite.Error(err)
	suite.Contains(err.Error(), "sound_quality must be between 1 and 5")
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_CrowdEnergyInvalid() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Energy Show", 2)

	zero := 0
	_, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID:      showID,
		Body:        "Bad crowd energy value",
		CrowdEnergy: &zero,
	})
	suite.Error(err)
	suite.Contains(err.Error(), "crowd_energy must be between 1 and 5")

	six := 6
	_, err = suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID:      showID,
		Body:        "Bad crowd energy value",
		CrowdEnergy: &six,
	})
	suite.Error(err)
	suite.Contains(err.Error(), "crowd_energy must be between 1 and 5")
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_ValidSoundQualityRange() {
	user := suite.createTestUser()

	for _, val := range []int{1, 2, 3, 4, 5} {
		showID := suite.createPastShow(fmt.Sprintf("SQ-%d", val), 2)
		sq := val
		fieldNote, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
			ShowID:       showID,
			Body:         fmt.Sprintf("Sound quality %d", val),
			SoundQuality: &sq,
		})
		suite.Require().NoError(err, "sound_quality=%d should be valid", val)
		suite.NotZero(fieldNote.ID)
	}
}

// =============================================================================
// Group 3: Verified Attendee Computation
// =============================================================================

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_VerifiedAttendee_GoingBeforeShow() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Past Show", 7)

	// Mark going 10 days ago (before the show that was 7 days ago)
	suite.markGoing(user.ID, showID, time.Now().Add(-10*24*time.Hour))

	fieldNote, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: showID,
		Body:   "I was there!",
	})
	suite.Require().NoError(err)

	var sd contracts.FieldNoteStructuredData
	err = json.Unmarshal(*fieldNote.StructuredData, &sd)
	suite.Require().NoError(err)
	suite.True(sd.IsVerifiedAttendee, "user who marked going before the show should be verified")
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_NotVerified_GoingAfterShow() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Past Show After", 7)

	// Mark going 2 days ago (AFTER the show that was 7 days ago)
	suite.markGoing(user.ID, showID, time.Now().Add(-2*24*time.Hour))

	fieldNote, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: showID,
		Body:   "I was definitely there (but marked it late).",
	})
	suite.Require().NoError(err)

	var sd contracts.FieldNoteStructuredData
	err = json.Unmarshal(*fieldNote.StructuredData, &sd)
	suite.Require().NoError(err)
	suite.False(sd.IsVerifiedAttendee, "user who marked going after the show should NOT be verified")
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_NotVerified_NoGoingRecord() {
	user := suite.createTestUser()
	showID := suite.createPastShow("No Going Show", 5)

	fieldNote, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: showID,
		Body:   "I was there but never marked going.",
	})
	suite.Require().NoError(err)

	var sd contracts.FieldNoteStructuredData
	err = json.Unmarshal(*fieldNote.StructuredData, &sd)
	suite.Require().NoError(err)
	suite.False(sd.IsVerifiedAttendee, "user with no going record should NOT be verified")
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_NotVerified_InterestedOnly() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Interested Show", 5)

	// Mark interested (not going) before the show
	bookmark := &models.UserBookmark{
		UserID:     user.ID,
		EntityType: models.BookmarkEntityShow,
		EntityID:   showID,
		Action:     models.BookmarkActionInterested,
		CreatedAt:  time.Now().Add(-10 * 24 * time.Hour),
	}
	err := suite.db.Create(bookmark).Error
	suite.Require().NoError(err)

	fieldNote, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: showID,
		Body:   "Interested only, not going.",
	})
	suite.Require().NoError(err)

	var sd contracts.FieldNoteStructuredData
	err = json.Unmarshal(*fieldNote.StructuredData, &sd)
	suite.Require().NoError(err)
	suite.False(sd.IsVerifiedAttendee, "user who only marked interested (not going) should NOT be verified")
}

// =============================================================================
// Group 4: ListFieldNotesForShow
// =============================================================================

func (suite *FieldNoteIntegrationTestSuite) TestListFieldNotesForShow_Basic() {
	user := suite.createTestUser()
	showID := suite.createPastShow("List Show", 2)

	suite.insertFieldNote(user.ID, showID, "First note", nil)
	suite.insertFieldNote(user.ID, showID, "Second note", nil)

	result, err := suite.commentService.ListFieldNotesForShow(showID, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
	suite.Len(result.Comments, 2)
	suite.False(result.HasMore)
}

func (suite *FieldNoteIntegrationTestSuite) TestListFieldNotesForShow_SortedBySongPosition() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Sorted Show", 2)

	// Insert field notes with varying song positions and no position (NULL)
	pos3 := 3
	pos1 := 1
	suite.insertFieldNote(user.ID, showID, "Song at position 3", &contracts.FieldNoteStructuredData{SongPosition: &pos3})
	suite.insertFieldNote(user.ID, showID, "Show-wide note (no position)", nil)
	suite.insertFieldNote(user.ID, showID, "Song at position 1", &contracts.FieldNoteStructuredData{SongPosition: &pos1})

	result, err := suite.commentService.ListFieldNotesForShow(showID, 20, 0)
	suite.Require().NoError(err)
	suite.Len(result.Comments, 3)

	// NULLs first (show-wide notes), then position 1, then position 3
	suite.Equal("Show-wide note (no position)", result.Comments[0].Body)
	suite.Equal("Song at position 1", result.Comments[1].Body)
	suite.Equal("Song at position 3", result.Comments[2].Body)
}

func (suite *FieldNoteIntegrationTestSuite) TestListFieldNotesForShow_OnlyFieldNotes() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Mixed Show", 2)

	// Insert a regular comment (not a field note)
	svc := suite.commentService
	bodyHTML := svc.renderMarkdown("Regular comment")
	regularComment := &models.Comment{
		EntityType:      models.CommentEntityShow,
		EntityID:        showID,
		Kind:            models.CommentKindComment,
		UserID:          user.ID,
		Body:            "Regular comment",
		BodyHTML:        bodyHTML,
		Visibility:      models.CommentVisibilityVisible,
		ReplyPermission: models.ReplyPermissionAnyone,
	}
	err := suite.db.Create(regularComment).Error
	suite.Require().NoError(err)

	// Insert a field note
	suite.insertFieldNote(user.ID, showID, "This is a field note", nil)

	result, err := suite.commentService.ListFieldNotesForShow(showID, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.Total)
	suite.Len(result.Comments, 1)
	suite.Equal("field_note", result.Comments[0].Kind)
	suite.Equal("This is a field note", result.Comments[0].Body)
}

func (suite *FieldNoteIntegrationTestSuite) TestListFieldNotesForShow_ExcludesHiddenNotes() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Hidden Show", 2)

	// Insert a visible field note
	suite.insertFieldNote(user.ID, showID, "Visible note", nil)

	// Insert a hidden field note directly
	svc := suite.commentService
	bodyHTML := svc.renderMarkdown("Hidden note")
	hidden := &models.Comment{
		EntityType:      models.CommentEntityShow,
		EntityID:        showID,
		Kind:            models.CommentKindFieldNote,
		UserID:          user.ID,
		Body:            "Hidden note",
		BodyHTML:        bodyHTML,
		Visibility:      models.CommentVisibilityHiddenByMod,
		ReplyPermission: models.ReplyPermissionAnyone,
	}
	err := suite.db.Create(hidden).Error
	suite.Require().NoError(err)

	result, err := suite.commentService.ListFieldNotesForShow(showID, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.Total)
	suite.Len(result.Comments, 1)
	suite.Equal("Visible note", result.Comments[0].Body)
}

func (suite *FieldNoteIntegrationTestSuite) TestListFieldNotesForShow_Pagination() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Paginated Show", 2)

	// Insert 5 field notes
	for i := 0; i < 5; i++ {
		suite.insertFieldNote(user.ID, showID, fmt.Sprintf("Note %d", i), nil)
	}

	// Page 1: limit 2, offset 0
	result, err := suite.commentService.ListFieldNotesForShow(showID, 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), result.Total)
	suite.Len(result.Comments, 2)
	suite.True(result.HasMore)

	// Page 3: limit 2, offset 4 (should have 1 item)
	result, err = suite.commentService.ListFieldNotesForShow(showID, 2, 4)
	suite.Require().NoError(err)
	suite.Equal(int64(5), result.Total)
	suite.Len(result.Comments, 1)
	suite.False(result.HasMore)
}

func (suite *FieldNoteIntegrationTestSuite) TestListFieldNotesForShow_EmptyShow() {
	showID := suite.createPastShow("Empty Show", 2)

	result, err := suite.commentService.ListFieldNotesForShow(showID, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(0), result.Total)
	suite.Len(result.Comments, 0)
	suite.False(result.HasMore)
}

func (suite *FieldNoteIntegrationTestSuite) TestListFieldNotesForShow_StructuredDataPreserved() {
	user := suite.createTestUser()
	showID := suite.createPastShow("SD Show", 2)

	sq := 4
	ce := 5
	moments := "Epic guitar solo"
	suite.insertFieldNote(user.ID, showID, "Note with SD", &contracts.FieldNoteStructuredData{
		SoundQuality:       &sq,
		CrowdEnergy:        &ce,
		NotableMoments:     &moments,
		SetlistSpoiler:     true,
		IsVerifiedAttendee: true,
	})

	result, err := suite.commentService.ListFieldNotesForShow(showID, 20, 0)
	suite.Require().NoError(err)
	suite.Require().Len(result.Comments, 1)
	suite.NotNil(result.Comments[0].StructuredData)

	var sd contracts.FieldNoteStructuredData
	err = json.Unmarshal(*result.Comments[0].StructuredData, &sd)
	suite.Require().NoError(err)
	suite.Equal(&sq, sd.SoundQuality)
	suite.Equal(&ce, sd.CrowdEnergy)
	suite.Require().NotNil(sd.NotableMoments)
	suite.Equal(moments, *sd.NotableMoments)
	suite.True(sd.SetlistSpoiler)
	suite.True(sd.IsVerifiedAttendee)
}

// =============================================================================
// Group 5: Trust Tier / Visibility
// =============================================================================

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_NewUser_PendingReview() {
	user := suite.createTestNewUser()
	showID := suite.createPastShow("New User Show", 2)

	fieldNote, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: showID,
		Body:   "New user field note",
	})
	suite.Require().NoError(err)
	suite.Equal("pending_review", fieldNote.Visibility)
}

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_Contributor_Visible() {
	user := suite.createTestUser() // contributor tier
	showID := suite.createPastShow("Contributor Show", 2)

	fieldNote, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: showID,
		Body:   "Contributor field note",
	})
	suite.Require().NoError(err)
	suite.Equal("visible", fieldNote.Visibility)
}

// =============================================================================
// Group 6: Auto-Subscribe
// =============================================================================

func (suite *FieldNoteIntegrationTestSuite) TestCreateFieldNote_AutoSubscribes() {
	user := suite.createTestUser()
	showID := suite.createPastShow("Subscribe Show", 2)

	_, err := suite.commentService.CreateFieldNote(user.ID, &contracts.CreateFieldNoteRequest{
		ShowID: showID,
		Body:   "Should auto-subscribe",
	})
	suite.Require().NoError(err)

	// Verify the subscription was created
	var sub models.CommentSubscription
	err = suite.db.Where("user_id = ? AND entity_type = ? AND entity_id = ?",
		user.ID, "show", showID).First(&sub).Error
	suite.Require().NoError(err)
	suite.Equal(user.ID, sub.UserID)
}
