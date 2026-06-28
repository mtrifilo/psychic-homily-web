package pipeline

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

func TestEnrichmentService_InvalidEnrichmentType(t *testing.T) {
	// Use a non-nil DB pointer to pass the nil check, but won't actually use it
	svc := &EnrichmentService{db: &gorm.DB{}}
	err := svc.QueueShowForEnrichment(1, "invalid_type")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid enrichment type")
}

func TestEnrichmentService_ValidEnrichmentTypes(t *testing.T) {
	// Verify that the validation switch accepts all valid types.
	// We can't call QueueShowForEnrichment with a zero-value gorm.DB because
	// GORM panics, so we just verify the type constants are defined properly.
	validTypes := []string{
		adminm.EnrichmentTypeArtistMatch,
		adminm.EnrichmentTypeMusicBrainz,
		adminm.EnrichmentTypeAPICrossRef,
		adminm.EnrichmentTypeAll,
	}
	assert.Equal(t, "artist_match", validTypes[0])
	assert.Equal(t, "musicbrainz", validTypes[1])
	assert.Equal(t, "api_crossref", validTypes[2])
	assert.Equal(t, "all", validTypes[3])
}

// TestMBIDToStamp exercises the PSY-1249 valid-UUID + fill-when-empty + exact-name
// gate in isolation (the load-bearing decision; the surrounding GORM write is covered
// by the integration suite). A wrong/malformed ID landing on an artist is the core
// risk, so the validation, never-overwrite, and name-rejection cases matter most.
//
// NOTE the helper is a self-contained gate by design; some cases below exercise an
// input the production caller (enrichMusicBrainz → SearchArtist, which pre-filters by
// EqualFold) can't actually produce. They pin the helper's contract for the
// un-pre-filtered reuse path (the raw SearchArtistCandidates list), not a reachable
// enrichMusicBrainz state — each such case says so.
func TestMBIDToStamp(t *testing.T) {
	const validMBID = "65f4f0c5-ef9e-490c-aee3-909e7ae6b2ab"
	sp := func(s string) *string { return &s }

	tests := []struct {
		name   string
		artist catalogm.Artist
		result *MBLookupResult
		want   string
	}{
		{
			name:   "exact-name match on an empty artist stamps the MBID",
			artist: catalogm.Artist{Name: "Snail Mail"},
			result: &MBLookupResult{MBID: validMBID, Name: "Snail Mail"},
			want:   validMBID,
		},
		{
			name:   "case/punctuation-insensitive exact match still stamps",
			artist: catalogm.Artist{Name: "Godspeed You Black Emperor"},
			result: &MBLookupResult{MBID: validMBID, Name: "Godspeed You! Black Emperor"},
			want:   validMBID,
		},
		{
			// Helper-level defensive gate: a non-matching name is rejected. This exact
			// input is UNREACHABLE via enrichMusicBrainz (SearchArtist already discards
			// non-EqualFold names); it verifies the helper stays correct if reused from
			// an un-pre-filtered path.
			name:   "name mismatch is rejected (helper-level defense; SearchArtist pre-filters in prod)",
			artist: catalogm.Artist{Name: "Crush"},
			result: &MBLookupResult{MBID: validMBID, Name: "Crush the Korean Rapper"},
			want:   "",
		},
		{
			// Punctuation-only names both normalize to "" — the empty-name guard must
			// reject rather than treat two different names as equal. (Mirrors
			// matchMBLocation's want=="" guard so the identity gates can't drift.)
			name:   "punctuation-only names that both normalize to empty are rejected",
			artist: catalogm.Artist{Name: "!!!"},
			result: &MBLookupResult{MBID: validMBID, Name: "+/-"},
			want:   "",
		},
		{
			name:   "an already-set MBID is never overwritten",
			artist: catalogm.Artist{Name: "Snail Mail", MusicBrainzArtistID: sp("11111111-2222-3333-4444-555555555555")},
			result: &MBLookupResult{MBID: validMBID, Name: "Snail Mail"},
			want:   "",
		},
		{
			name:   "a blank existing MBID counts as empty and is filled",
			artist: catalogm.Artist{Name: "Snail Mail", MusicBrainzArtistID: sp("")},
			result: &MBLookupResult{MBID: validMBID, Name: "Snail Mail"},
			want:   validMBID,
		},
		{
			name:   "a nil result stamps nothing",
			artist: catalogm.Artist{Name: "Snail Mail"},
			result: nil,
			want:   "",
		},
		{
			name:   "an empty MBID on the result stamps nothing",
			artist: catalogm.Artist{Name: "Snail Mail"},
			result: &MBLookupResult{MBID: "", Name: "Snail Mail"},
			want:   "",
		},
		{
			// Trust-boundary: a malformed (non-UUID) id from the MB API is declined,
			// so it never enters the VARCHAR(36) identity column.
			name:   "a malformed (non-UUID) MBID is rejected",
			artist: catalogm.Artist{Name: "Snail Mail"},
			result: &MBLookupResult{MBID: "not-a-uuid", Name: "Snail Mail"},
			want:   "",
		},
		{
			// An oversized id would otherwise raise "value too long for VARCHAR(36)"
			// and abort the whole provenance Updates — rejected up front instead.
			name:   "an oversized MBID is rejected",
			artist: catalogm.Artist{Name: "Snail Mail"},
			result: &MBLookupResult{MBID: validMBID + "-trailing-garbage", Name: "Snail Mail"},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mbidToStamp(tt.artist, tt.result))
		})
	}
}

func TestMusicBrainzClient_NewClient(t *testing.T) {
	client := NewMusicBrainzClient()
	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Equal(t, mbRateLimit, client.rateLimit)
	assert.Equal(t, mbMinScore, client.minScore)
}

// TestMusicBrainzClient_SharedAcrossServices is the PSY-1208 repro: when the
// SAME *MusicBrainzClient is injected into both DiscoverMusicService and
// EnrichmentService, both services hold pointer-identical instances, so a single
// mutex-serialized throttle covers ALL MusicBrainz calls in the process. Before
// PSY-1208 each constructor called NewMusicBrainzClient() independently, giving
// two throttles that could combine for ~2 req/s and trip MB's ~1 req/s/IP block.
//
// This test enforces the CONSTRUCTOR-LEVEL contract that container.go relies on
// — that passing one client to both constructors yields one shared client. The
// container's own wiring (NewServiceContainer constructs ONE mbClient and passes
// that same variable to both, container.go) is obvious by construction and not
// re-asserted here, since a full container test would need a live DB + config.
func TestMusicBrainzClient_SharedAcrossServices(t *testing.T) {
	shared := NewMusicBrainzClient()

	// Non-nil DB pointer satisfies the constructors without a live DB — neither
	// constructor touches the DB, and we only inspect the MB client field.
	stubDB := &gorm.DB{}
	discover := NewDiscoverMusicService(stubDB, shared)
	enrich := NewEnrichmentService(stubDB, nil, "", shared)

	// DiscoverMusicService.mb is typed as the mbSearcher interface (PSY-1191);
	// assert it holds the same concrete *MusicBrainzClient we injected.
	discoverMB, ok := discover.mb.(*MusicBrainzClient)
	assert.True(t, ok, "discover.mb should be the concrete *MusicBrainzClient")
	assert.Same(t, shared, discoverMB, "discovery must hold the injected shared client")
	assert.Same(t, shared, enrich.mbClient, "enrichment must hold the injected shared client")
	assert.Same(t, discoverMB, enrich.mbClient,
		"discovery + enrichment must share ONE MusicBrainz client (pointer identity)")
}

// TestMusicBrainzClient_DefaultsWhenNotInjected verifies the standalone/test
// fallback: passing a nil client still yields a working, non-nil throttle so
// existing callers keep working.
func TestMusicBrainzClient_DefaultsWhenNotInjected(t *testing.T) {
	stubDB := &gorm.DB{}

	discover := NewDiscoverMusicService(stubDB, nil)
	discoverMB, ok := discover.mb.(*MusicBrainzClient)
	assert.True(t, ok)
	assert.NotNil(t, discoverMB, "nil client must default-construct")

	enrich := NewEnrichmentService(stubDB, nil, "", nil)
	assert.NotNil(t, enrich.mbClient, "nil client must default-construct")

	// Two default-constructed services get DISTINCT clients (the pre-PSY-1208
	// behavior preserved for standalone callers that don't opt into sharing).
	assert.NotSame(t, discoverMB, enrich.mbClient)
}

// TestMusicBrainzClient_ThrottleEnforcesSpacing verifies the rate limit still
// enforces ~1.1s spacing between successive requests on a single client (the
// shared instance after PSY-1208). throttle() is exercised directly so the test
// needs no network I/O: the first call returns immediately (zero-value lastReq),
// the second must block until at least one rateLimit interval has elapsed.
func TestMusicBrainzClient_ThrottleEnforcesSpacing(t *testing.T) {
	c := NewMusicBrainzClient()
	// Shorten the interval so the test stays fast while still proving the
	// throttle blocks for ~one interval; the production interval is mbRateLimit.
	// 200ms (not, say, 50ms) gives the first-call lower-bound assertion below
	// generous margin: the first throttle does only a lock + time.Now(), so the
	// "< rateLimit" check would only flake if a loaded/GC-starved CI box stalled
	// that for >200ms — far less likely than a tighter window.
	c.rateLimit = 200 * time.Millisecond

	ctx := context.Background()

	start := time.Now()
	assert.NoError(t, c.throttle(ctx)) // first slot is free
	firstElapsed := time.Since(start)
	assert.Less(t, firstElapsed, c.rateLimit,
		"first throttle should not block (lastReq is zero)")

	start = time.Now()
	assert.NoError(t, c.throttle(ctx)) // second must wait one interval
	secondElapsed := time.Since(start)
	// secondElapsed >= rateLimit is the robust direction: time.Timer can only
	// fire late, never early, so this lower bound holds regardless of jitter.
	assert.GreaterOrEqual(t, secondElapsed, c.rateLimit,
		"second throttle must block for at least one rateLimit interval")
}

// TestMusicBrainzClient_ThrottleCancellable verifies the throttle aborts the
// per-call rate-limit WAIT on a cancelled context instead of holding the lock
// for the full interval — the PSY-1191 cancellable-discovery behavior the
// shared client must preserve. NOTE: this covers the cancellation of the wait
// itself, NOT contention on c.mu.Lock(). With one shared client (PSY-1208) a
// concurrent discovery call can still block up to ~one interval acquiring the
// lock behind an in-flight enrichment throttle — that bounded wait is the
// intended cost of a true ~1 req/s process-wide limit, documented in the PR.
func TestMusicBrainzClient_ThrottleCancellable(t *testing.T) {
	c := NewMusicBrainzClient()
	c.rateLimit = time.Hour // make the wait effectively unbounded

	// Prime lastReq so the next throttle would block for ~rateLimit.
	assert.NoError(t, c.throttle(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	start := time.Now()
	err := c.throttle(ctx)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), time.Second,
		"cancelled throttle must return promptly, not wait the interval")
}

func TestSeatGeekClient_NotConfigured(t *testing.T) {
	client := NewSeatGeekClient("")
	assert.False(t, client.IsConfigured())

	result, err := client.SearchEvent("Test Venue", time.Now())
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestSeatGeekClient_Configured(t *testing.T) {
	client := NewSeatGeekClient("test_client_id")
	assert.True(t, client.IsConfigured())
}

func TestEnrichmentWorker_NewWorker(t *testing.T) {
	svc := &EnrichmentService{}
	worker := NewEnrichmentWorker(svc)
	assert.NotNil(t, worker)
	assert.Equal(t, DefaultEnrichmentInterval, worker.interval)
	assert.Equal(t, DefaultEnrichmentBatchSize, worker.batchSize)
}

// =============================================================================
// Mock ArtistService for enrichment tests
// =============================================================================

type mockArtistServiceForEnrichment struct {
	searchArtistsFn func(query string) ([]*contracts.ArtistDetailResponse, error)
}

func (m *mockArtistServiceForEnrichment) CreateArtist(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtist(artistID uint) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistByName(name string) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistBySlug(slug string) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtists(filters map[string]interface{}) ([]*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistsWithShowCounts(filters map[string]interface{}) ([]*contracts.ArtistWithShowCountResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) UpdateArtist(artistID uint, req *contracts.UpdateArtistRequest) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) DeleteArtist(artistID uint) error { return nil }
func (m *mockArtistServiceForEnrichment) SearchArtists(query string) ([]*contracts.ArtistDetailResponse, error) {
	if m.searchArtistsFn != nil {
		return m.searchArtistsFn(query)
	}
	return []*contracts.ArtistDetailResponse{}, nil
}
func (m *mockArtistServiceForEnrichment) GetShowsForArtist(artistID uint, timezone string, limit int, timeFilter string) ([]*contracts.ArtistShowResponse, int64, error) {
	return nil, 0, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistCities() ([]*contracts.ArtistCityResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetLabelsForArtist(artistID uint) ([]*contracts.ArtistLabelResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) AddArtistAlias(artistID uint, alias string) (*contracts.ArtistAliasResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) RemoveArtistAlias(aliasID uint) error { return nil }
func (m *mockArtistServiceForEnrichment) GetArtistAliases(artistID uint) ([]*contracts.ArtistAliasResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) MergeArtists(canonicalID, mergeFromID uint) (*contracts.MergeArtistResult, error) {
	return nil, nil
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type EnrichmentIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	svc       *EnrichmentService
	ctx       context.Context
}

func (s *EnrichmentIntegrationTestSuite) SetupSuite() {
	s.ctx = context.Background()

	container, err := testcontainers.GenericContainer(s.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "test_db",
				"POSTGRES_USER":     "test_user",
				"POSTGRES_PASSWORD": "test_password",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	s.Require().NoError(err)
	s.container = container

	host, _ := container.Host(s.ctx)
	port, _ := container.MappedPort(s.ctx, "5432")

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable", host, port.Port())
	// Match production: TranslateError so duplicate-key checks behave the same
	// in tests as in production.
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
	s.Require().NoError(err)
	s.db = db

	sqlDB, err := db.DB()
	s.Require().NoError(err)

	migrationDir, _ := filepath.Abs("../../../db/migrations")
	testutil.RunAllMigrations(s.T(), sqlDB, migrationDir)

	mockArtist := &mockArtistServiceForEnrichment{}
	s.svc = NewEnrichmentService(db, mockArtist, "", nil)
}

func (s *EnrichmentIntegrationTestSuite) TearDownSuite() {
	if s.container != nil {
		//nolint:errcheck // test teardown best-effort; container is going away
		s.container.Terminate(s.ctx)
	}
}

func (s *EnrichmentIntegrationTestSuite) SetupTest() {
	// Clean tables between tests
	s.db.Exec("DELETE FROM enrichment_queue")
	s.db.Exec("DELETE FROM show_artists")
	s.db.Exec("DELETE FROM show_venues")
	s.db.Exec("DELETE FROM shows")
	s.db.Exec("DELETE FROM artists")
	s.db.Exec("DELETE FROM venues")
}

func (s *EnrichmentIntegrationTestSuite) createTestShow() uint {
	show := catalogm.Show{
		Title:     "Test Show",
		EventDate: time.Now().Add(24 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
		Source:    catalogm.ShowSourceDiscovery,
	}
	s.Require().NoError(s.db.Create(&show).Error)
	return show.ID
}

func (s *EnrichmentIntegrationTestSuite) createTestShowWithArtist() (uint, uint) {
	artist := catalogm.Artist{Name: fmt.Sprintf("Test Artist %d-%d", time.Now().UnixNano(), rand.Intn(1000000))}
	s.Require().NoError(s.db.Create(&artist).Error)

	venue := catalogm.Venue{Name: fmt.Sprintf("Test Venue %d-%d", time.Now().UnixNano(), rand.Intn(1000000)), City: "Phoenix", State: "AZ"}
	s.Require().NoError(s.db.Create(&venue).Error)

	show := catalogm.Show{
		Title:     "Test Show with Artist",
		EventDate: time.Now().Add(24 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
		Source:    catalogm.ShowSourceDiscovery,
	}
	s.Require().NoError(s.db.Create(&show).Error)

	showArtist := catalogm.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, SetType: "headliner"}
	s.Require().NoError(s.db.Create(&showArtist).Error)

	showVenue := catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}
	s.Require().NoError(s.db.Create(&showVenue).Error)

	return show.ID, artist.ID
}

// Test: QueueShowForEnrichment
func (s *EnrichmentIntegrationTestSuite) TestQueueShowForEnrichment() {
	showID := s.createTestShow()

	err := s.svc.QueueShowForEnrichment(showID, adminm.EnrichmentTypeAll)
	s.Require().NoError(err)

	// Verify item was created
	var item adminm.EnrichmentQueueItem
	err = s.db.Where("show_id = ?", showID).First(&item).Error
	s.Require().NoError(err)
	s.Equal(adminm.EnrichmentStatusPending, item.Status)
	s.Equal(adminm.EnrichmentTypeAll, item.EnrichmentType)
	s.Equal(0, item.Attempts)
	s.Equal(3, item.MaxAttempts)
}

// Test: ProcessQueue - empty queue
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_EmptyQueue() {
	processed, err := s.svc.ProcessQueue(s.ctx, 10)
	s.Require().NoError(err)
	s.Equal(0, processed)
}

// Test: ProcessQueue - processes pending items
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_ProcessesPending() {
	showID, _ := s.createTestShowWithArtist()

	// Queue the show
	err := s.svc.QueueShowForEnrichment(showID, adminm.EnrichmentTypeAll)
	s.Require().NoError(err)

	// Process the queue
	processed, err := s.svc.ProcessQueue(s.ctx, 10)
	s.Require().NoError(err)
	s.Equal(1, processed)

	// Verify item was completed
	var item adminm.EnrichmentQueueItem
	err = s.db.Where("show_id = ?", showID).First(&item).Error
	s.Require().NoError(err)
	s.Equal(adminm.EnrichmentStatusCompleted, item.Status)
	s.Equal(1, item.Attempts)
	s.NotNil(item.CompletedAt)
	s.NotNil(item.Results)
}

// Test: ProcessQueue - respects batch size
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_RespectsBatchSize() {
	// Create 3 shows and queue them
	for i := 0; i < 3; i++ {
		showID, _ := s.createTestShowWithArtist()
		s.Require().NoError(s.svc.QueueShowForEnrichment(showID, adminm.EnrichmentTypeAll))
	}

	// Process only 2
	processed, err := s.svc.ProcessQueue(s.ctx, 2)
	s.Require().NoError(err)
	s.Equal(2, processed)

	// Verify 1 still pending
	var pendingCount int64
	s.db.Model(&adminm.EnrichmentQueueItem{}).
		Where("status = ?", adminm.EnrichmentStatusPending).
		Count(&pendingCount)
	s.Equal(int64(1), pendingCount)
}

// Test: ProcessQueue - skips items at max attempts
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_SkipsMaxAttempts() {
	showID := s.createTestShow()

	// Create a queue item already at max attempts
	item := &adminm.EnrichmentQueueItem{
		ShowID:         showID,
		Status:         adminm.EnrichmentStatusPending,
		Attempts:       3,
		MaxAttempts:    3,
		EnrichmentType: adminm.EnrichmentTypeAll,
	}
	s.Require().NoError(s.db.Create(item).Error)

	// Process — should not pick up this item
	processed, err := s.svc.ProcessQueue(s.ctx, 10)
	s.Require().NoError(err)
	s.Equal(0, processed)
}

// Test: EnrichShow - show not found
func (s *EnrichmentIntegrationTestSuite) TestEnrichShow_ShowNotFound() {
	_, err := s.svc.EnrichShow(s.ctx, 999999)
	s.Error(err)
	s.Contains(err.Error(), "show not found")
}

// Test: EnrichShow - successful enrichment
func (s *EnrichmentIntegrationTestSuite) TestEnrichShow_Success() {
	showID, _ := s.createTestShowWithArtist()

	result, err := s.svc.EnrichShow(s.ctx, showID)
	s.Require().NoError(err)
	s.NotNil(result)
	s.Equal(showID, result.ShowID)
	s.Contains(result.CompletedSteps, "artist_match")
	s.Contains(result.CompletedSteps, "musicbrainz")
	s.Contains(result.CompletedSteps, "api_crossref")
}

// Test: EnrichShow - context cancellation
func (s *EnrichmentIntegrationTestSuite) TestEnrichShow_ContextCancellation() {
	showID, _ := s.createTestShowWithArtist()

	ctx, cancel := context.WithCancel(s.ctx)
	cancel() // Cancel immediately

	result, err := s.svc.EnrichShow(ctx, showID)
	// Should still return partial result (at least artist_match step)
	s.NoError(err)
	s.NotNil(result)
	s.Equal(showID, result.ShowID)
}

// Test: GetQueueStats
func (s *EnrichmentIntegrationTestSuite) TestGetQueueStats() {
	// Create some items in different states
	showID1 := s.createTestShow()
	showID2 := s.createTestShow()
	showID3 := s.createTestShow()

	s.db.Create(&adminm.EnrichmentQueueItem{
		ShowID: showID1, Status: adminm.EnrichmentStatusPending, EnrichmentType: adminm.EnrichmentTypeAll,
	})
	s.db.Create(&adminm.EnrichmentQueueItem{
		ShowID: showID2, Status: adminm.EnrichmentStatusProcessing, EnrichmentType: adminm.EnrichmentTypeAll,
	})
	now := time.Now()
	s.db.Create(&adminm.EnrichmentQueueItem{
		ShowID: showID3, Status: adminm.EnrichmentStatusCompleted, EnrichmentType: adminm.EnrichmentTypeAll,
		CompletedAt: &now,
	})

	stats, err := s.svc.GetQueueStats()
	s.Require().NoError(err)
	s.Equal(int64(1), stats.Pending)
	s.Equal(int64(1), stats.Processing)
	s.Equal(int64(1), stats.CompletedToday)
}

// Test: SeatGeek enrichment skipped when not configured
func (s *EnrichmentIntegrationTestSuite) TestEnrichShow_SeatGeekSkippedWhenNotConfigured() {
	showID, _ := s.createTestShowWithArtist()

	result, err := s.svc.EnrichShow(s.ctx, showID)
	s.Require().NoError(err)
	s.NotNil(result.SeatGeek)
	s.False(result.SeatGeek.Found) // SeatGeek not configured, so no results
}

func TestEnrichmentIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}
	suite.Run(t, new(EnrichmentIntegrationTestSuite))
}
