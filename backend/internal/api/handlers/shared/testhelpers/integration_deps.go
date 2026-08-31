package testhelpers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/community"
	"psychic-homily-backend/internal/services/engagement"
	exploresvc "psychic-homily-backend/internal/services/explore"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/notification"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/services/shared"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
)

// IntegrationDeps holds all services + DB for handler integration tests.
// Fields are exported so tests in any handler sub-package can reach them.
type IntegrationDeps struct {
	DB                        *gorm.DB
	TestDB                    *testutil.TestDatabase
	Ctx                       context.Context
	ShowService               *catalog.ShowService
	VenueService              *catalog.VenueService
	SavedShowService          *engagement.SavedShowService
	ShowReportService         *adminsvc.ShowReportService
	UserService               *usersvc.UserService
	AuditLogService           *adminsvc.AuditLogService
	ExploreService            *exploresvc.ExploreService
	DiscordService            *notification.DiscordService
	ExtractionService         *pipeline.ExtractionService
	APITokenService           *adminsvc.APITokenService
	DataSyncService           *adminsvc.DataSyncService
	AdminStatsService         *adminsvc.AdminStatsService
	DiscoveryService          *pipeline.DiscoveryService
	StreamingWorklistService  *pipeline.StreamingWorklistService
	ArtistService             *catalog.ArtistService
	FestivalService           *catalog.FestivalService
	LabelService              *catalog.LabelService
	ReleaseService            *catalog.ReleaseService
	CollectionService         *community.CollectionService
	RequestService            *community.RequestService
	TagService                *catalog.TagService
	ArtistRelationshipService *catalog.ArtistRelationshipService
	SceneService              *catalog.SceneService
}

// SetupIntegrationDeps spins up a Postgres test database and constructs the
// services every handler integration test reaches for. Mirrors what the
// production server's container.go does, scoped to what handler tests need.
func SetupIntegrationDeps(t *testing.T) *IntegrationDeps {
	t.Helper()

	testDB := testutil.SetupTestPostgres(t)
	db := testDB.DB

	emptyCfg := &config.Config{}

	deps := &IntegrationDeps{
		DB:                        db,
		TestDB:                    testDB,
		Ctx:                       context.Background(),
		ShowService:               catalog.NewShowService(db),
		// nil address geocoder: handler tests must never reach the public
		// Nominatim API (flaky CI + OSM usage-policy violation).
		VenueService:              catalog.NewVenueService(db).WithAddressGeocoder(nil),
		SavedShowService:          engagement.NewSavedShowService(db),
		ShowReportService:         adminsvc.NewShowReportService(db),
		UserService:               usersvc.NewUserService(db),
		AuditLogService:           adminsvc.NewAuditLogService(db),
		ExploreService:            exploresvc.NewExploreService(db),
		DiscordService:            notification.NewDiscordService(emptyCfg),
		ExtractionService:         pipeline.NewExtractionService(db, emptyCfg, catalog.NewArtistService(db), catalog.NewVenueService(db).WithAddressGeocoder(nil)),
		APITokenService:           adminsvc.NewAPITokenService(db),
		DataSyncService:           adminsvc.NewDataSyncService(db),
		AdminStatsService:         adminsvc.NewAdminStatsService(db),
		DiscoveryService:          pipeline.NewDiscoveryService(db, catalog.NewVenueService(db).WithAddressGeocoder(nil)),
		StreamingWorklistService:  pipeline.NewStreamingWorklistService(db),
		ArtistService:             catalog.NewArtistService(db),
		FestivalService:           catalog.NewFestivalService(db),
		LabelService:              catalog.NewLabelService(db),
		ReleaseService:            catalog.NewReleaseService(db),
		CollectionService:         community.NewCollectionService(db),
		RequestService:            community.NewRequestService(db),
		TagService:                catalog.NewTagService(db),
		ArtistRelationshipService: catalog.NewArtistRelationshipService(db),
		SceneService:              catalog.NewSceneService(db),
	}
	// PSY-354: collections gain polymorphic tagging via the tag service.
	// Wire the dependency so handler tests exercise the same code path
	// that production uses.
	deps.CollectionService.SetTagService(deps.TagService)

	return deps
}

// CleanupTables truncates all tables touched by handler integration tests
// in dependency order. Call from TearDownTest.
func CleanupTables(db *gorm.DB) {
	sqlDB, _ := db.DB()
	// Order respects FK constraints
	_, _ = sqlDB.Exec("DELETE FROM request_votes")
	_, _ = sqlDB.Exec("DELETE FROM requests")
	// PSY-354: clear polymorphic tag links + votes before removing the
	// underlying entities they reference (entity_tags has no FK to the
	// polymorphic entity, but tag_votes references tags + users; clearing
	// these here lets entity_tags rows from prior tests not leak into
	// later tests that share the same database).
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM collection_likes")
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
	// PSY-366: artist_relationships before artists — FK has no ON DELETE
	// CASCADE (migration 000052).
	_, _ = sqlDB.Exec("DELETE FROM artist_relationships")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM api_tokens")
	_, _ = sqlDB.Exec("DELETE FROM webauthn_credentials")
	_, _ = sqlDB.Exec("DELETE FROM webauthn_challenges")
	_, _ = sqlDB.Exec("DELETE FROM oauth_accounts")
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	// PSY-354: clear the polymorphic tag corpus too — tags / tag_aliases
	// can leak between integration test cases via the LOWER(name) unique
	// index ("post-punk" reused across tests would collide). Tags's FK to
	// users is ON DELETE SET NULL, so order vs `users` is fine, but we
	// drop tags before users so tag cleanup doesn't see orphaned rows.
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

// CreateTestUser inserts a normal (non-admin) verified user with a unique
// email and returns it.
func CreateTestUser(db *gorm.DB) *authm.User {
	user := &authm.User{
		Email:         StringPtr(fmt.Sprintf("user-%d@test.com", time.Now().UnixNano())),
		FirstName:     StringPtr("Test"),
		LastName:      StringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	db.Create(user)
	return user
}

// CreateUserWithTier inserts a normal (non-admin) verified user at the given
// user tier (e.g. "new_user", "trusted_contributor", "local_ambassador") and
// returns it. Used by handlers that gate behavior on UserTier.
func CreateUserWithTier(db *gorm.DB, tier string) *authm.User {
	user := &authm.User{
		Email:         StringPtr(fmt.Sprintf("user-%s-%d@test.com", tier, time.Now().UnixNano())),
		FirstName:     StringPtr("Tier"),
		LastName:      StringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
		UserTier:      tier,
	}
	db.Create(user)
	return user
}

// CreateAdminUser inserts an admin user with a unique email and returns it.
func CreateAdminUser(db *gorm.DB) *authm.User {
	user := &authm.User{
		Email:         StringPtr(fmt.Sprintf("admin-%d@test.com", time.Now().UnixNano())),
		FirstName:     StringPtr("Admin"),
		LastName:      StringPtr("User"),
		IsActive:      true,
		IsAdmin:       true,
		EmailVerified: true,
	}
	db.Create(user)
	return user
}

// MetroFor resolves a (city, state) to its CBSA code for fixtures, mirroring the
// production venue/artist write paths that denormalize metro via the geocoder
// (PSY-1255 step C). Since PSY-1747 it mirrors them through the very function
// they call, so a change to the production derivation moves the fixtures with it
// instead of silently disagreeing. nil for a non-US / no-CBSA place.
//
// The country is pinned to "US" here — unlike the write paths, which read the
// entity's own column — because these fixtures exist to produce a CBSA and CBSAs
// are US-only.
func MetroFor(city, state string) *string {
	return shared.DeriveMetro(geo.Default(), shared.Location{City: city, State: state, Country: "US"})
}

// CreateVerifiedVenue inserts a verified venue and returns it.
func CreateVerifiedVenue(db *gorm.DB, name, city, state string) *catalogm.Venue {
	venue := &catalogm.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Metro:    MetroFor(city, state),
		Verified: true,
	}
	db.Create(venue)
	return venue
}

// CreateUnverifiedVenue inserts an unverified venue and returns it.
func CreateUnverifiedVenue(db *gorm.DB, name, city, state string) *catalogm.Venue {
	venue := &catalogm.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Metro:    MetroFor(city, state),
		Verified: false,
	}
	db.Create(venue)
	return venue
}

// CreateArtist inserts an artist by name and returns it.
func CreateArtist(db *gorm.DB, name string) *catalogm.Artist {
	artist := &catalogm.Artist{
		Name: name,
	}
	db.Create(artist)
	return artist
}

// CreateCollection inserts a collection with the given visibility and returns
// it, with is_public read back out of the database.
//
// Beside CreateApprovedShow / CreatePendingShow because collections are the
// OTHER entity type with a read-time visibility rule (PSY-1987), and a privacy
// fixture is exactly the thing that must not be re-derived per test package.
//
// TWO WRITES, and both are load-bearing. GORM omits a false boolean on Create
// because false is the zero value, so `IsPublic: false` alone leaves the column
// at its DEFAULT of true — the same gotcha CreateCollection in
// services/community works around. A fixture that silently stayed public makes
// every privacy assertion built on it pass for the wrong reason, so the flag is
// written through a map and then read back; a fixture that did not take is a
// loud failure here rather than a quiet pass three assertions later.
func CreateCollection(t *testing.T, db *gorm.DB, creatorID uint, title, slug string, isPublic bool) *communitym.Collection {
	t.Helper()
	collection := &communitym.Collection{
		Title:     title,
		Slug:      slug,
		CreatorID: creatorID,
		IsPublic:  isPublic,
	}
	if err := db.Create(collection).Error; err != nil {
		t.Fatalf("create collection %q: %v", slug, err)
	}
	SetCollectionPublic(t, db, collection.ID, isPublic)
	collection.IsPublic = isPublic
	return collection
}

// SetCollectionPublic writes is_public through a map update and reads it back.
//
// Separate from CreateCollection because the privacy tests flip a collection
// mid-test — that transition is the repro — and the read-back matters just as
// much there.
func SetCollectionPublic(t *testing.T, db *gorm.DB, collectionID uint, isPublic bool) {
	t.Helper()
	if err := db.Model(&communitym.Collection{}).Where("id = ?", collectionID).
		Update("is_public", isPublic).Error; err != nil {
		t.Fatalf("set is_public on collection %d: %v", collectionID, err)
	}
	var got bool
	if err := db.Model(&communitym.Collection{}).Where("id = ?", collectionID).
		Select("is_public").Scan(&got).Error; err != nil {
		t.Fatalf("read back is_public for collection %d: %v", collectionID, err)
	}
	if got != isPublic {
		t.Fatalf("collection %d has is_public=%v, want %v — the fixture did not take",
			collectionID, got, isPublic)
	}
}

// CreateApprovedShow inserts a show in the approved state, with a venue +
// artist association, dated 7 days from now. Returns the show.
func CreateApprovedShow(db *gorm.DB, userID uint, title string) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        StringPtr("Phoenix"),
		State:       StringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	db.Create(show)

	// Create venue and artist associations
	venue := CreateVerifiedVenue(db, title+" Venue", "Phoenix", "AZ")
	artist := CreateArtist(db, title+" Artist")

	db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	return show
}

// CreatePendingShow inserts a show in the pending state, with a venue +
// artist association, dated 7 days from now. Returns the show.
func CreatePendingShow(db *gorm.DB, userID uint, title string) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        StringPtr("Phoenix"),
		State:       StringPtr("AZ"),
		Status:      catalogm.ShowStatusPending,
		SubmittedBy: &userID,
	}
	db.Create(show)

	venue := CreateVerifiedVenue(db, title+" Venue", "Phoenix", "AZ")
	artist := CreateArtist(db, title+" Artist")

	db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	return show
}

// CreateFutureApprovedShow inserts an approved show daysFromNow into the
// future, with a venue + artist association.
func CreateFutureApprovedShow(db *gorm.DB, userID uint, title string, daysFromNow int) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, daysFromNow),
		City:        StringPtr("Phoenix"),
		State:       StringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	db.Create(show)

	venue := CreateVerifiedVenue(db, title+" Venue", "Phoenix", "AZ")
	artist := CreateArtist(db, title+" Artist")

	db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	return show
}

// CreatePastApprovedShow inserts an approved show daysAgo days in the past,
// with a venue + artist association.
func CreatePastApprovedShow(db *gorm.DB, userID uint, title string, daysAgo int) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, -daysAgo),
		City:        StringPtr("Phoenix"),
		State:       StringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	db.Create(show)

	venue := CreateVerifiedVenue(db, title+" Venue", "Phoenix", "AZ")
	artist := CreateArtist(db, title+" Artist")

	db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	return show
}

// StringPtr returns a pointer to s.
func StringPtr(s string) *string { return &s }

// BoolPtr returns a pointer to b.
func BoolPtr(b bool) *bool { return &b }
