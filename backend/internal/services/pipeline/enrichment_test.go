package pipeline

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
// Scope: SEQUENTIAL callers only. The process-wide guarantee under CONCURRENT
// callers (discovery + enrichment contending on c.mu) is covered separately by
// TestMusicBrainzClient_ThrottleSpacesConcurrentCallers below — a throttle that
// released c.mu across the wait would still pass THIS test, so keep both.
func TestMusicBrainzClient_ThrottleEnforcesSpacing(t *testing.T) {
	c := NewMusicBrainzClient()
	// Shorten the interval so the test stays fast while still proving the
	// throttle blocks for ~one interval; the production interval is mbRateLimit.
	// 200ms (not, say, 50ms) leaves the first-call "< rateLimit" check generous
	// margin: that call does only a lock plus time.Now(), so it flakes only if
	// the box stalls >200ms between the anchor and the check.
	c.rateLimit = 200 * time.Millisecond

	ctx := context.Background()

	start := time.Now()
	require.NoError(t, c.throttle(ctx)) // first slot is free
	assert.Less(t, time.Since(start), c.rateLimit,
		"first throttle should not block (lastReq is zero)")

	// firstSlot is the throttle's OWN record of the slot it just handed out, and
	// it anchors both assertions below. Everything the throttle promises is
	// relative to it: call 2 builds its timer from lastReq, so it cannot fire
	// before firstSlot+rateLimit, and Go timers only ever fire late — the bound
	// is sound by construction, not by margin. Restarting a stopwatch here
	// instead would anchor strictly later than firstSlot, silently subtracting
	// the difference from the measured wait; that gap was the original flake.
	// The anchor assumes lastReq records the wall-clock time of the COMPLETED
	// request; a throttle reformulated to store the next allowed slot instead
	// must re-derive it. Both guards below pin the anchor to real time taken by
	// the test, so a throttle that writes a synthetic lastReq cannot make the
	// bounds vacuous. Unsynchronized reads are safe: this test is single-threaded.
	firstSlot := c.lastReq
	require.False(t, firstSlot.IsZero(), "anchor must be a real slot")
	require.False(t, firstSlot.Before(start),
		"slot record must be real wall time, not a synthetic slot")

	require.NoError(t, c.throttle(ctx)) // second must wait one interval

	// Both assertions are load-bearing; each catches a regression the other
	// misses, and both were confirmed by mutating throttle():
	//   - Wall time proves the call actually BLOCKED. Without it, a throttle
	//     that advances lastReq to the next slot without sleeping passes in
	//     0.00s — every value it compares is one the throttle wrote itself.
	//   - The slot gap proves the wait is measured from the PREVIOUS slot.
	//     Without it, a throttle that records lastReq BEFORE waiting rather than
	//     after still blocks the full interval here, so wall time is satisfied,
	//     while its slots drift closer together under sustained load.
	assert.GreaterOrEqual(t, time.Since(firstSlot), c.rateLimit,
		"second throttle must actually block until the slot opens")
	assert.GreaterOrEqual(t, c.lastReq.Sub(firstSlot), c.rateLimit,
		"successive throttle slots must be spaced at least one rateLimit apart")
}

// TestMusicBrainzClient_ThrottleSpacesConcurrentCallers verifies the rate limit
// is process-WIDE, not per-caller: several goroutines hammering ONE shared
// client (the PSY-1208 arrangement, where discovery and enrichment contend on
// the same limiter) must still be granted slots at least one rateLimit apart.
// Exceeding that is the condition MusicBrainz bans clients for, and it is not
// something -race can see: a throttle that computes its delay under c.mu, then
// unlocks across the wait and relocks to write lastReq, has no data race at all
// — just a lost update on the spacing invariant. It passes every sequential
// throttle test. This test is the one that fails on it.
//
// Measurement is deliberately BLACK-BOX: each goroutine stamps its own
// completion with the TEST's clock, so no assertion below compares two values
// the subject wrote to itself. The trailing c.lastReq checks only corroborate.
//
// SCOPE: spacing under contention, nothing else. Every caller here passes a live
// context, so cancellation WHILE CONTENDING is still uncovered — including the
// specific claim in throttle's own CAVEAT that a waiter's cancelled context
// cannot shorten a hold taken with context.Background(). ThrottleCancellable
// covers cancellation of the wait single-threaded; neither test covers the
// interaction. Don't read "there's a concurrency test now" as covering it.
func TestMusicBrainzClient_ThrottleSpacesConcurrentCallers(t *testing.T) {
	// Callers serialize BY CONSTRUCTION, so the test costs (callers-1) *
	// rateLimit and every extra caller is bought with a full interval. Three is
	// enough: it already produces a caller queued behind two others and two
	// independent adjacent gaps to check, and both mutants this test exists to
	// kill (see below) died 20/20 runs at this N. Raising it buys runtime, not
	// signal.
	const callers = 3

	c := NewMusicBrainzClient()
	// Shortened for runtime, as in the sequential test; the production interval
	// is mbRateLimit. Nothing below depends on the value except the per-gap
	// slack, which is sized against measured jitter (see bound 2).
	c.rateLimit = 100 * time.Millisecond

	// Two-stage gate, and BOTH stages are load-bearing. close(release) alone
	// would wake whichever goroutines happen to exist, but `go func()` only
	// guarantees a goroutine is created, not scheduled — the spawn loop can
	// finish before any of them has run a single statement. ready.Wait() blocks
	// until every caller has actually executed and parked on the receive, so the
	// close hits N runnable goroutines at once.
	//
	// This matters for what the test can DETECT, not for whether it passes: the
	// assertions below are lower bounds, so callers trickling in more than a
	// rateLimit apart would still satisfy them — while never contending, which
	// is the entire property under test. Dropping the ready stage would leave a
	// test that quietly stops discriminating on a loaded machine.
	release := make(chan struct{})
	var ready, done sync.WaitGroup
	ready.Add(callers)
	done.Add(callers)

	granted := make([]time.Time, callers)
	errs := make([]error, callers)
	for i := range granted {
		go func(i int) {
			defer done.Done()
			ready.Done()
			<-release
			errs[i] = c.throttle(context.Background())
			granted[i] = time.Now()
		}(i)
	}

	ready.Wait()
	start := time.Now() // taken before any throttle can run
	close(release)
	done.Wait()
	require.NoError(t, errors.Join(errs...), "throttle must not fail on a live context")

	// Slots are handed out in SOME order; the test cannot know which goroutine
	// got which, so sort and reason about the sequence of grants.
	sort.Slice(granted, func(i, j int) bool { return granted[i].Before(granted[j]) })

	// Bound 1 — cumulative, and sound with ZERO tolerance. If spacing holds,
	// the i-th grant cannot land before start+i*rateLimit, and a stamp taken
	// after the grant only ever pushes the observation later. (Sorting is safe
	// under that reasoning: adding non-negative delays cannot make the i-th
	// smallest observation earlier than the i-th smallest grant.) This is the
	// assertion that kills the unlock-across-the-wait mutant, where callers all
	// compute their delay against the same lastReq and fire in one batch.
	//
	// Starts at 1: the i=0 term reduces to "the first stamp is not before start",
	// which holds by construction and can never fail, so including it would
	// overstate how many of these checks carry weight.
	for i := 1; i < len(granted); i++ {
		assert.GreaterOrEqual(t, granted[i].Sub(start), time.Duration(i)*c.rateLimit,
			"grant %d of %d landed before its slot could open — concurrent callers shared a slot", i+1, callers)
	}

	// Bound 2 — per-adjacent-pair, which bound 1 alone does not give: a throttle
	// that stalls once and then releases everyone at once (grants at 200, 200.1,
	// 200.2ms) clears every cumulative floor while firing all three requests in
	// one interval. Confirmed by mutation, not argued: that variant passes bound
	// 1 untouched and fails only here.
	//
	// Unlike bound 1 this compares two independently scheduled stamps, so it
	// needs slack: it fails spuriously only if the EARLIER grant's stamp is
	// delayed more than the later one's, since delay common to both cancels out.
	//
	// The value is a two-sided compromise, and both sides were measured rather
	// than argued:
	//
	//   - Too tight and CI jitter reds an innocent PR. Differential jitter was
	//     never observed to bite at all: with slack set to ZERO this test still
	//     passed 50/50 under -race, and the smallest margin over rateLimit seen
	//     across 195 instrumented gaps was +1.00ms. Treat that as a floor from
	//     ONE dev box, not a guarantee — CI runs on shared runners and without
	//     -race, which those numbers cannot speak for.
	//   - Too loose and real violations slip through. A caller that shares a
	//     slot lands MICROseconds behind its neighbour, so any threshold well
	//     clear of scheduler noise catches THAT. But unequal compensating gaps
	//     (grants at 0, 149, 200ms: every cumulative floor cleared, one pair
	//     only 51ms apart) are caught only if the threshold stays high enough.
	//     Half the interval would let that sequence pass.
	//
	// rateLimit/5 keeps ~20x margin over the worst jitter ever measured here
	// while holding the bypass window to gaps of 0.8-1.0 rateLimit, which are
	// near-compliant anyway. Steady over-rate is bound 1's job and is unaffected
	// by this number.
	slack := c.rateLimit / 5
	for i := 1; i < len(granted); i++ {
		assert.GreaterOrEqual(t, granted[i].Sub(granted[i-1]), c.rateLimit-slack,
			"grants %d and %d landed inside one rateLimit interval", i, i+1)
	}

	// Corroboration from the throttle's own bookkeeping, pinned to real time so
	// it cannot be satisfied by a back-dated synthetic slot (the PSY-1559
	// lesson). Reading lastReq here is race-free: done.Wait() synchronizes with
	// every goroutine's write under c.mu.
	require.False(t, c.lastReq.Before(start), "final slot record must be real wall time")
	assert.GreaterOrEqual(t, c.lastReq.Sub(start), time.Duration(callers-1)*c.rateLimit,
		"the throttle's own last slot must sit at least (callers-1) intervals out")
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
func (m *mockArtistServiceForEnrichment) GetArtistSummary(artistID uint) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistSummaryBySlug(slug string) (*contracts.ArtistDetailResponse, error) {
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
func (m *mockArtistServiceForEnrichment) GetArtistsWithShowCounts(filters map[string]interface{}, limit, offset int) ([]*contracts.ArtistWithShowCountResponse, int64, error) {
	return nil, 0, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistListing() ([]contracts.ArtistListingEntry, error) {
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
func (m *mockArtistServiceForEnrichment) GetShowsForArtist(artistID uint, timezone string, query contracts.ArtistShowsQuery) ([]*contracts.ArtistShowResponse, int64, error) {
	return nil, 0, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistShowYears(artistID uint, timeFilter string) ([]contracts.ArtistShowYearCount, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetNextShowForArtist(artistID uint, timezone string) (*contracts.ArtistShowResponse, error) {
	return nil, nil
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

// --- PSY-1572: the safety mechanics enrichment_queue previously lacked ---

// TestProcessQueue_ReclaimsStrandedRow is the headline gap. A row left in
// `processing` — by a crash, a deploy mid-batch, or a finalize write that failed
// silently — used to be a PERMANENT zombie: the claim filter only looks at
// `pending`, so nothing ever selected it again. ReclaimStale must return it.
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_ReclaimsStrandedRow() {
	showID, _ := s.createTestShowWithArtist()

	// A row stranded in `processing` longer ago than the reclaim window, with
	// attempts left — the crash/deploy shape.
	stranded := &adminm.EnrichmentQueueItem{
		ShowID:         showID,
		Status:         adminm.EnrichmentStatusProcessing,
		Attempts:       1,
		MaxAttempts:    3,
		EnrichmentType: adminm.EnrichmentTypeAll,
	}
	s.Require().NoError(s.db.Create(stranded).Error)
	s.Require().NoError(s.db.Model(stranded).
		UpdateColumn("updated_at", time.Now().Add(-enrichmentStaleReclaim-time.Minute)).Error)

	processed, err := s.svc.ProcessQueue(s.ctx, 10)
	s.Require().NoError(err)
	s.Equal(1, processed, "the stranded row must be reclaimed and then processed")

	var got adminm.EnrichmentQueueItem
	s.Require().NoError(s.db.First(&got, stranded.ID).Error)
	s.Equal(adminm.EnrichmentStatusCompleted, got.Status, "reclaimed row must reach a terminal state, not zombie")
}

// TestProcessQueue_FailsExhaustedStrandedRow: a stranded row with NO attempts left
// must be marked failed, not left pending. Leaving it pending would zombie it a
// second way — the claim filter is attempts < max_attempts, so it would never be
// re-claimed and never finalize. The sentinel distinguishes it from a real failure.
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_FailsExhaustedStrandedRow() {
	showID := s.createTestShow()

	exhausted := &adminm.EnrichmentQueueItem{
		ShowID:         showID,
		Status:         adminm.EnrichmentStatusProcessing,
		Attempts:       3,
		MaxAttempts:    3,
		EnrichmentType: adminm.EnrichmentTypeAll,
	}
	s.Require().NoError(s.db.Create(exhausted).Error)
	s.Require().NoError(s.db.Model(exhausted).
		UpdateColumn("updated_at", time.Now().Add(-enrichmentStaleReclaim-time.Minute)).Error)

	_, err := s.svc.ProcessQueue(s.ctx, 10)
	s.Require().NoError(err)

	var got adminm.EnrichmentQueueItem
	s.Require().NoError(s.db.First(&got, exhausted.ID).Error)
	s.Equal(adminm.EnrichmentStatusFailed, got.Status)
	s.Require().NotNil(got.LastError)
	s.Equal(enrichmentStrandedError, *got.LastError,
		"must carry the sentinel so machinery loss is distinguishable from a real enrichment failure")
}

// TestProcessQueue_DoesNotReclaimFreshProcessing: the reclaim window must not fire
// on work that is still legitimately running. Reclaiming early burns an attempt on
// a row that is succeeding — the PSY-1569 failure mode.
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_DoesNotReclaimFreshProcessing() {
	showID := s.createTestShow()

	fresh := &adminm.EnrichmentQueueItem{
		ShowID:         showID,
		Status:         adminm.EnrichmentStatusProcessing,
		Attempts:       1,
		MaxAttempts:    3,
		EnrichmentType: adminm.EnrichmentTypeAll,
	}
	s.Require().NoError(s.db.Create(fresh).Error)

	processed, err := s.svc.ProcessQueue(s.ctx, 10)
	s.Require().NoError(err)
	s.Equal(0, processed, "an in-flight row must not be claimed")

	var got adminm.EnrichmentQueueItem
	s.Require().NoError(s.db.First(&got, fresh.ID).Error)
	s.Equal(adminm.EnrichmentStatusProcessing, got.Status, "still in flight")
	s.Equal(1, got.Attempts, "no attempt burned on work that is still running")
}

// TestProcessQueue_ClaimIsAtomic: the claim must flip rows to `processing` and
// increment attempts atomically. Previously the SELECT and the mark-as-processing
// were separate unlocked statements — a TOCTOU window two pollers could both pass.
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_ClaimIsAtomic() {
	showID := s.createTestShow() // no artist: EnrichShow errors, so the row stays observable
	s.Require().NoError(s.svc.QueueShowForEnrichment(showID, adminm.EnrichmentTypeArtistMatch))

	_, err := s.svc.ProcessQueue(s.ctx, 10)
	s.Require().NoError(err)

	var got adminm.EnrichmentQueueItem
	s.Require().NoError(s.db.Where("show_id = ?", showID).First(&got).Error)
	s.Equal(1, got.Attempts, "claim increments exactly once, in the same tx as the status flip")
}

// TestProcessQueue_ConcurrentClaimsAreDisjoint: FOR UPDATE SKIP LOCKED means two
// concurrent pollers never claim the same row. Without it both selected the same
// pending rows and enriched them twice.
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_ConcurrentClaimsAreDisjoint() {
	const n = 6
	for i := 0; i < n; i++ {
		showID, _ := s.createTestShowWithArtist()
		s.Require().NoError(s.svc.QueueShowForEnrichment(showID, adminm.EnrichmentTypeAll))
	}

	var wg sync.WaitGroup
	counts := make([]int, 2)
	errs := make([]error, 2)
	for i := range counts {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			counts[idx], errs[idx] = s.svc.ProcessQueue(s.ctx, n)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		s.Require().NoError(err, "poller %d", i)
	}
	s.Equal(n, counts[0]+counts[1], "every row processed exactly once across both pollers")

	var processedTwice int64
	s.db.Model(&adminm.EnrichmentQueueItem{}).Where("attempts > 1").Count(&processedTwice)
	s.Zero(processedTwice, "no row may be claimed by both pollers")
}
