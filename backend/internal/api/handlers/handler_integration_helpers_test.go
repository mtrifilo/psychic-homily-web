package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// handlerIntegrationDeps holds all services + DB for handler integration tests
type handlerIntegrationDeps struct {
	db                    *gorm.DB
	container             testcontainers.Container
	ctx                   context.Context
	showService           *services.ShowService
	venueService          *services.VenueService
	savedShowService      *services.SavedShowService
	favoriteVenueService  *services.FavoriteVenueService
	showReportService     *services.ShowReportService
	userService           *services.UserService
	auditLogService       *services.AuditLogService
	discordService        *services.DiscordService
	musicDiscoveryService *services.MusicDiscoveryService
	extractionService     *services.ExtractionService
	apiTokenService       *services.APITokenService
	dataSyncService       *services.DataSyncService
	adminStatsService     *services.AdminStatsService
	discoveryService      *services.DiscoveryService
	artistService         *services.ArtistService
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

	// Run migrations - path from handlers/ -> api/ -> internal/ -> backend/ -> db/migrations/
	migrations := []string{
		"000001_create_initial_schema.up.sql",
		"000002_add_artist_search_indexes.up.sql",
		"000003_add_venue_search_indexes.up.sql",
		"000004_update_venue_constraints.up.sql",
		"000005_add_show_status.up.sql",
		"000006_add_user_saved_shows.up.sql",
		"000007_add_private_show_status.up.sql",
		"000008_add_pending_venue_edits.up.sql",
		"000009_add_bandcamp_embed_url.up.sql",
		"000010_add_scraper_source_fields.up.sql",
		"000011_add_webauthn_tables.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000013_add_slugs.up.sql",
		"000014_add_account_lockout.up.sql",
		"000015_add_user_favorite_venues.up.sql",
		"000018_add_show_reports.up.sql",
		"000020_add_show_status_flags.up.sql",
		"000021_add_api_tokens.up.sql",
		"000022_add_audit_logs.up.sql",
		"000023_rename_scraper_to_discovery.up.sql",
		"000026_add_duplicate_of_show_id.up.sql",
		"000028_change_event_date_to_timestamptz.up.sql",
		"000030_add_artist_reports.up.sql",
		"000031_add_user_terms_acceptance.up.sql",
		"000032_add_favorite_cities.up.sql",
	}

	migrationDir := filepath.Join("..", "..", "..", "db", "migrations")

	for _, m := range migrations {
		migrationSQL, err := os.ReadFile(filepath.Join(migrationDir, m))
		if err != nil {
			t.Fatalf("failed to read migration file %s: %v", m, err)
		}
		_, err = sqlDB.Exec(string(migrationSQL))
		if err != nil {
			t.Fatalf("failed to run migration %s: %v", m, err)
		}
	}

	// Run migration 000027 with CONCURRENTLY stripped
	migration27, err := os.ReadFile(filepath.Join(migrationDir, "000027_add_index_duplicate_of_show_id.up.sql"))
	if err != nil {
		t.Fatalf("failed to read migration 000027: %v", err)
	}
	sql27 := strings.ReplaceAll(string(migration27), "CONCURRENTLY ", "")
	_, err = sqlDB.Exec(sql27)
	if err != nil {
		t.Fatalf("failed to run migration 000027: %v", err)
	}

	// Construct services
	emptyCfg := &config.Config{}

	deps := &handlerIntegrationDeps{
		db:                    db,
		container:             container,
		ctx:                   ctx,
		showService:           services.NewShowService(db),
		venueService:          services.NewVenueService(db),
		savedShowService:      services.NewSavedShowService(db),
		favoriteVenueService:  services.NewFavoriteVenueService(db),
		showReportService:     services.NewShowReportService(db),
		userService:           services.NewUserService(db),
		auditLogService:       services.NewAuditLogService(db),
		discordService:        services.NewDiscordService(emptyCfg),
		musicDiscoveryService: services.NewMusicDiscoveryService(emptyCfg),
		extractionService:     services.NewExtractionService(db, emptyCfg),
		apiTokenService:       services.NewAPITokenService(db),
		dataSyncService:       services.NewDataSyncService(db),
		adminStatsService:     services.NewAdminStatsService(db),
		discoveryService:      services.NewDiscoveryService(db),
		artistService:         services.NewArtistService(db),
	}

	return deps
}

func cleanupTables(db *gorm.DB) {
	sqlDB, _ := db.DB()
	// Order respects FK constraints
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM artist_reports")
	_, _ = sqlDB.Exec("DELETE FROM show_reports")
	_, _ = sqlDB.Exec("DELETE FROM user_saved_shows")
	_, _ = sqlDB.Exec("DELETE FROM user_favorite_venues")
	_, _ = sqlDB.Exec("DELETE FROM pending_venue_edits")
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
