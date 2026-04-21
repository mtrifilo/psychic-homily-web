package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/notification"
	"psychic-homily-backend/internal/services/pipeline"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
)

// handlerIntegrationDeps holds all services + DB for handler integration tests
type handlerIntegrationDeps struct {
	db                    *gorm.DB
	testDB                *testutil.TestDatabase
	ctx                   context.Context
	showService           *catalog.ShowService
	venueService          *catalog.VenueService
	savedShowService      *engagement.SavedShowService
	favoriteVenueService  *engagement.FavoriteVenueService
	showReportService     *adminsvc.ShowReportService
	userService           *usersvc.UserService
	auditLogService       *adminsvc.AuditLogService
	discordService        *notification.DiscordService
	musicDiscoveryService *pipeline.MusicDiscoveryService
	extractionService     *pipeline.ExtractionService
	apiTokenService       *adminsvc.APITokenService
	dataSyncService       *adminsvc.DataSyncService
	adminStatsService     *adminsvc.AdminStatsService
	discoveryService      *pipeline.DiscoveryService
	artistService         *catalog.ArtistService
	festivalService       *catalog.FestivalService
	labelService          *catalog.LabelService
	releaseService        *catalog.ReleaseService
	collectionService     *services.CollectionService
	requestService        *services.RequestService
	tagService            *catalog.TagService
}

func setupHandlerIntegrationDeps(t *testing.T) *handlerIntegrationDeps {
	t.Helper()

	testDB := testutil.SetupTestPostgres(t)
	db := testDB.DB

	// Construct services
	emptyCfg := &config.Config{}

	deps := &handlerIntegrationDeps{
		db:                    db,
		testDB:                testDB,
		ctx:                   context.Background(),
		showService:           catalog.NewShowService(db),
		venueService:          catalog.NewVenueService(db),
		savedShowService:      engagement.NewSavedShowService(db),
		favoriteVenueService:  engagement.NewFavoriteVenueService(db),
		showReportService:     adminsvc.NewShowReportService(db),
		userService:           usersvc.NewUserService(db),
		auditLogService:       adminsvc.NewAuditLogService(db),
		discordService:        notification.NewDiscordService(emptyCfg),
		musicDiscoveryService: pipeline.NewMusicDiscoveryService(emptyCfg),
		extractionService:     pipeline.NewExtractionService(db, emptyCfg, catalog.NewArtistService(db), catalog.NewVenueService(db)),
		apiTokenService:       adminsvc.NewAPITokenService(db),
		dataSyncService:       adminsvc.NewDataSyncService(db),
		adminStatsService:     adminsvc.NewAdminStatsService(db),
		discoveryService:      pipeline.NewDiscoveryService(db, catalog.NewVenueService(db)),
		artistService:         catalog.NewArtistService(db),
		festivalService:       catalog.NewFestivalService(db),
		labelService:          catalog.NewLabelService(db),
		releaseService:        catalog.NewReleaseService(db),
		collectionService:     services.NewCollectionService(db),
		requestService:        services.NewRequestService(db),
		tagService:            catalog.NewTagService(db),
	}

	return deps
}

func cleanupTables(db *gorm.DB) {
	sqlDB, _ := db.DB()
	// Order respects FK constraints
	_, _ = sqlDB.Exec("DELETE FROM request_votes")
	_, _ = sqlDB.Exec("DELETE FROM requests")
	_, _ = sqlDB.Exec("DELETE FROM collection_subscribers")
	_, _ = sqlDB.Exec("DELETE FROM collection_items")
	_, _ = sqlDB.Exec("DELETE FROM collections")
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM artist_reports")
	_, _ = sqlDB.Exec("DELETE FROM show_reports")
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM festival_venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM release_labels")
	_, _ = sqlDB.Exec("DELETE FROM artist_labels")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM release_external_links")
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM api_tokens")
	_, _ = sqlDB.Exec("DELETE FROM webauthn_credentials")
	_, _ = sqlDB.Exec("DELETE FROM webauthn_challenges")
	_, _ = sqlDB.Exec("DELETE FROM oauth_accounts")
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func createTestUser(db *gorm.DB) *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	db.Create(user)
	return user
}

func createAdminUser(db *gorm.DB) *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("admin-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Admin"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		IsAdmin:       true,
		EmailVerified: true,
	}
	db.Create(user)
	return user
}

func createVerifiedVenue(db *gorm.DB, name, city, state string) *models.Venue {
	venue := &models.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Verified: true,
	}
	db.Create(venue)
	return venue
}

func createUnverifiedVenue(db *gorm.DB, name, city, state string) *models.Venue {
	venue := &models.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Verified: false,
	}
	db.Create(venue)
	return venue
}

func createArtist(db *gorm.DB, name string) *models.Artist {
	artist := &models.Artist{
		Name: name,
	}
	db.Create(artist)
	return artist
}

func createApprovedShow(db *gorm.DB, userID uint, title string) *models.Show {
	show := &models.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	db.Create(show)

	// Create venue and artist associations
	venue := createVerifiedVenue(db, title+" Venue", "Phoenix", "AZ")
	artist := createArtist(db, title+" Artist")

	db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	return show
}

func createPendingShow(db *gorm.DB, userID uint, title string) *models.Show {
	show := &models.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusPending,
		SubmittedBy: &userID,
	}
	db.Create(show)

	venue := createVerifiedVenue(db, title+" Venue", "Phoenix", "AZ")
	artist := createArtist(db, title+" Artist")

	db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	return show
}

func createFutureApprovedShow(db *gorm.DB, userID uint, title string, daysFromNow int) *models.Show {
	show := &models.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, daysFromNow),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	db.Create(show)

	venue := createVerifiedVenue(db, title+" Venue", "Phoenix", "AZ")
	artist := createArtist(db, title+" Artist")

	db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	return show
}

func createPastApprovedShow(db *gorm.DB, userID uint, title string, daysAgo int) *models.Show {
	show := &models.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, -daysAgo),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	db.Create(show)

	venue := createVerifiedVenue(db, title+" Venue", "Phoenix", "AZ")
	artist := createArtist(db, title+" Artist")

	db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	return show
}

// stringPtr is already defined in auth_test.go as strPtr - add alias for consistency
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func uintPtr(u uint) *uint {
	return &u
}

func float64Ptr(f float64) *float64 {
	return &f
}
