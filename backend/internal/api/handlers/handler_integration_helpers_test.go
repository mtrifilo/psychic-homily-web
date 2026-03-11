package handlers

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/testutil"
)

// handlerIntegrationDeps holds all services + DB for handler integration tests
type handlerIntegrationDeps struct {
	db                    *gorm.DB
	container             testcontainers.Container
	ctx                   context.Context
	showService           *services.ShowService
	venueService          *services.VenueService
	savedShowService      *engagement.SavedShowService
	favoriteVenueService  *engagement.FavoriteVenueService
	showReportService     *services.ShowReportService
	userService           *services.UserService
	auditLogService       *services.AuditLogService
	discordService        *services.DiscordService
	musicDiscoveryService *pipeline.MusicDiscoveryService
	extractionService     *pipeline.ExtractionService
	apiTokenService       *services.APITokenService
	dataSyncService       *services.DataSyncService
	adminStatsService     *services.AdminStatsService
	discoveryService      *pipeline.DiscoveryService
	artistService         *services.ArtistService
	festivalService       *services.FestivalService
	labelService          *services.LabelService
	releaseService        *services.ReleaseService
}

func setupHandlerIntegrationDeps(t *testing.T) *handlerIntegrationDeps {
	t.Helper()

	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "test_db",
				"POSTGRES_USER":     "test_user",
				"POSTGRES_PASSWORD": "test_password",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get port: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}

	testutil.RunAllMigrations(t, sqlDB, filepath.Join("..", "..", "..", "db", "migrations"))

	// Construct services
	emptyCfg := &config.Config{}

	deps := &handlerIntegrationDeps{
		db:                    db,
		container:             container,
		ctx:                   ctx,
		showService:           services.NewShowService(db),
		venueService:          services.NewVenueService(db),
		savedShowService:      engagement.NewSavedShowService(db),
		favoriteVenueService:  engagement.NewFavoriteVenueService(db),
		showReportService:     services.NewShowReportService(db),
		userService:           services.NewUserService(db),
		auditLogService:       services.NewAuditLogService(db),
		discordService:        services.NewDiscordService(emptyCfg),
		musicDiscoveryService: pipeline.NewMusicDiscoveryService(emptyCfg),
		extractionService:     pipeline.NewExtractionService(db, emptyCfg, services.NewArtistService(db), services.NewVenueService(db)),
		apiTokenService:       services.NewAPITokenService(db),
		dataSyncService:       services.NewDataSyncService(db),
		adminStatsService:     services.NewAdminStatsService(db),
		discoveryService:      pipeline.NewDiscoveryService(db, services.NewVenueService(db)),
		artistService:         services.NewArtistService(db),
		festivalService:       services.NewFestivalService(db),
		labelService:          services.NewLabelService(db),
		releaseService:        services.NewReleaseService(db),
	}

	return deps
}

func cleanupTables(db *gorm.DB) {
	sqlDB, _ := db.DB()
	// Order respects FK constraints
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM artist_reports")
	_, _ = sqlDB.Exec("DELETE FROM show_reports")
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM pending_venue_edits")
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
