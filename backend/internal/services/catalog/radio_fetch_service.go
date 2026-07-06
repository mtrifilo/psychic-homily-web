package catalog

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"math/rand/v2"
	"net"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// Default radio fetch interval (6 hours)
const DefaultRadioFetchInterval = 6 * time.Hour

// Default affinity computation interval (24 hours)
const DefaultAffinityInterval = 24 * time.Hour

// Default re-matching interval (7 days)
const DefaultReMatchInterval = 7 * 24 * time.Hour

// Default show-discovery interval (24 hours). Provider rosters change slowly
// (WFMU adds shows monthly at most; KEXP/NTS slower), so once a day is plenty
// and the per-station DJ-index fetch cost is negligible.
const DefaultDiscoverInterval = 24 * time.Hour

// Default create-on-first window (90 days). PSY-1153: the discover run materializes a
// newly-discovered roster show only if it aired within this window, importing that
// window as the show's initial history. So RADIO_AUTO_BACKFILL_DAYS is now "how far
// back to look for a new show's first episode" (= the history it arrives with), not a
// separate post-discovery backfill job. 0 narrows it to a today-only create window
// (minimal; new shows still get created the day they air) — it does NOT disable the
// scheduled discover/fetch loops.
const DefaultAutoBackfillDays = 90

// Default post-air backfill sweep interval (1 hour). The backfill loop re-fetches
// aired episodes whose playlist is still incomplete (PSY-1154); an hourly sweep
// catches a playlist soon after it's published without hammering providers (the
// candidate set is small — only recently-aired incomplete episodes — and each run
// is per-station-lock + breaker + rate-limit gated). Env: RADIO_BACKFILL_INTERVAL_HOURS.
const DefaultBackfillInterval = 1 * time.Hour

// Default post-air backfill lookback (7 days). Only episodes that aired within this
// window are swept — it bounds the candidate set (and the one-time re-fetch burst at
// rollout) and reflects that providers only keep recent episodes listable for
// re-fetch. The per-episode attempt cap (RadioBackfillMaxAttempts) is the real
// give-up control; the lookback just bounds the scan.
//
// RADIO_BACKFILL_LOOKBACK_DAYS=0 disables the DEDICATED sweep (this loop's goroutine
// isn't started). It does NOT disable post-air healing entirely: the scheduled fetch
// loop still re-lists recently-aired episodes and importEpisode's re-fetch is
// state-driven, so an incomplete aired episode it re-lists is still healed/advanced.
// "Off" means "no proactive sweep," not "playlist_state is frozen."
const DefaultBackfillLookbackDays = 7

// Default janitor/reconcile interval (24 hours — nightly). The janitor reconciles
// show lifecycle (active↔dormant), corrects play_count drift, and sweeps backfill
// stragglers (PSY-1155). RADIO_JANITOR_INTERVAL_HOURS=0 disables the whole cycle.
const DefaultJanitorInterval = 24 * time.Hour

// Default dormancy window (30 days) for recency-semantics stations only: a show with
// no episode aired in this window is marked 'dormant' (inactive/historical, still
// browsable); a show that aired within it is 'active'. Schedule-authoritative
// (WFMU-family) stations ignore it — they reconcile by grid membership (PSY-1348).
// Owner decision (2026-06-21). Env: RADIO_JANITOR_DORMANT_DAYS.
const DefaultJanitorDormantDays = 30

// Default janitor backfill straggler lookback (30 days) — wider than the hourly
// post-air sweep (DefaultBackfillLookbackDays=7) so the nightly run catches aired
// incomplete episodes the hourly sweep missed (service downtime, late-created rows).
// Env: RADIO_JANITOR_BACKFILL_LOOKBACK_DAYS. NOTE: 0 is a today-only window (not a
// disable — the sweep still runs); to disable the whole janitor use
// RADIO_JANITOR_INTERVAL_HOURS=0.
const DefaultJanitorBackfillLookbackDays = 30

// Default WFMU schedule-scrape interval (7 days — weekly). The WFMU program grid
// (wfmu.org/table) only changes with the season, so a weekly scrape is ample and gentle
// on WFMU (PSY-1159). RADIO_SCHEDULE_INTERVAL_HOURS=0 (or negative) disables the cycle.
const DefaultScheduleInterval = 7 * 24 * time.Hour

// Default WFMU sub-stream schedule-scrape interval (daily). Unlike the seasonal
// /table grid, the sub-stream pass MUST run daily-or-faster: its source pages are
// rolling windows whose scrape-day group is partial, so each run trusts only the
// six full weekdays and preserves the scrape day — a weekly cadence would land on
// the SAME weekday every run and freeze that day's slots forever (PSY-1322
// adversarial finding). RADIO_SUBSTREAM_SCHEDULE_INTERVAL_HOURS=0 disables it.
const DefaultSubstreamScheduleInterval = 24 * time.Hour

// Default schedule-aware slot-fetch interval (PSY-1333). Bounds how long a
// schedule-bearing show's episode row can lag its scheduled start/end — the
// fix for whole broadcasts sitting invisible inside the (default 6h) station
// sweep gap. Each boundary triggers one scoped incremental fetch for that show
// (~2 per show per airing — see radio_slot_fetch.go for the honest cost
// breakdown); the interval is a latency knob, not a load knob, because work is
// driven by slot boundaries, not by ticks. RADIO_SLOT_FETCH_INTERVAL_MINUTES=0
// disables it.
const DefaultSlotFetchInterval = 10 * time.Minute

// Transient-retry policy (PSY-1142). Two tiers per the Google SRE retry-budget
// model + AWS Full-Jitter backoff (docs/research/radio-ingestion-best-practices-2026.md
// §2). Tier 1 (per-request): retry a transient error up to radioRetryMaxAttempts
// total with Full-Jitter exponential backoff. Tier 2 (per-client): a process-wide
// retry budget sheds retries once they exceed radioRetryBudgetRatio of requests over
// a rolling window, so a degraded provider fails fast instead of amplifying load.
const (
	// radioRetryMaxAttempts is the total number of attempts per request (1 initial +
	// up to N-1 retries). Google SRE's per-request cap is ≤3.
	radioRetryMaxAttempts = 3

	// radioRetryBackoffBase / radioRetryBackoffCap bound the Full-Jitter window: the
	// nth retry sleeps a random duration in [0, min(cap, base·2^n)). Jitter (vs fixed
	// or plain-exponential backoff) de-synchronizes retries across stations so they
	// don't thunder.
	radioRetryBackoffBase = 500 * time.Millisecond
	radioRetryBackoffCap  = 30 * time.Second

	// radioRetryBudgetWindow / radioRetryBudgetRatio are the per-client tier: at most
	// ~10% of requests over a rolling 2-min window may be retries (Google SRE; keeps
	// load growth ~1.1×). radioRetryBudgetMinReqs is a low-volume guard — below this
	// many requests in the window the budget is inactive (a handful of retries can't
	// form a storm, and a strict ratio would otherwise shed the first retry of every
	// low-volume cycle).
	//
	// Honest scope: at the radio loop's steady-state cadence (6h fetch / 24h discover,
	// stations processed sequentially behind a 1-rps-per-provider rate limiter) the
	// per-request attempt cap (tier 1) is the dominant control, and ≥minReqs requests
	// rarely land within a single 2-min window — so tier 2 engages essentially only on
	// the startup co-fire (both loops runImmediately) or if request volume grows. It is
	// a forward-looking safety net, not a per-cycle limiter. The events slice is bounded
	// by that same upstream provider rate limiter (≈1 noteRequest/sec/provider) — a
	// change that removes that throttle would need to revisit this.
	radioRetryBudgetWindow  = 2 * time.Minute
	radioRetryBudgetRatio   = 0.10
	radioRetryBudgetMinReqs = 10
)

// errorKind classifies a radio provider error for retry + breaker routing.
// kindTransient → retried (up to radioRetryMaxAttempts) by fetchStationWithRetry;
// never trips the breaker.
// kindPermanent → no retry; increments the persistent breaker counter and trips
// it at radioCircuitBreakerThreshold (see breakerTransition in radio_sync.go).
type errorKind int

const (
	kindPermanent errorKind = iota // default — string-only errors land here too
	kindTransient
)

// classifyError routes a radio provider error to transient vs permanent per
// PSY-887. Type-assertion-based, not string-matching:
//   - context.DeadlineExceeded                  → transient
//   - any error implementing net.Error.Timeout() → transient
//   - *net.OpError                              → transient (covers connection refused,
//     network unreachable, EOF on idle socket — all worth one retry)
//   - *RadioHTTPError                           → 429 transient, other non-OK permanent
//   - errors.Is(err, ErrTransient)              → transient (manual provider tag)
//   - errors.Is(err, ErrPermanent)              → permanent (manual provider tag)
//   - anything else (parse, schema, db, setup)  → permanent
//
// Operates on the WHOLE error chain via errors.As / errors.Is so wrapped
// errors (fmt.Errorf("...: %w", err)) classify correctly. A non-2xx HTTP
// response wrapped in five layers of fmt.Errorf still routes by status code.
func classifyError(err error) errorKind {
	if err == nil {
		return kindPermanent // caller shouldn't pass nil; default safely
	}

	// Context-level timeout — the surrounding ctx hit its deadline mid-request.
	if errors.Is(err, context.DeadlineExceeded) {
		return kindTransient
	}

	// HTTP-level classification via RadioHTTPError.Unwrap() chain. Has to come
	// before the net.Error check because http.Client errors can satisfy
	// net.Error too (via *url.Error.Timeout()) — but we want 429 routed
	// specifically, not lumped in with generic "network timeout".
	var httpErr *RadioHTTPError
	if errors.As(err, &httpErr) {
		// Unwrap() returns ErrTransient for 429, ErrPermanent otherwise.
		if errors.Is(httpErr, ErrTransient) {
			return kindTransient
		}
		return kindPermanent
	}

	// Generic net.Error.Timeout() catches both *url.Error (http.Client.Timeout)
	// and any other net-package error chain. The interface is the idiomatic
	// classifier; *url.Error implements it directly.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return kindTransient
	}

	// *net.OpError catches connection refused, network unreachable, EOF on
	// an idle socket, etc. — anything dial-level worth one retry.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return kindTransient
	}

	// Last resort: manual provider tags. If a provider wraps an error with
	// errors.Join(ErrTransient, ...) without RadioHTTPError, honor it.
	if errors.Is(err, ErrTransient) {
		return kindTransient
	}
	if errors.Is(err, ErrPermanent) {
		return kindPermanent
	}

	// Parse failures, schema mismatches, db setup errors — all permanent.
	return kindPermanent
}

// RadioFetchService is a background service that periodically:
//  1. Fetches new episodes from all active stations with playlist sources (every 6h)
//  2. Computes artist affinity from co-occurrence data (daily)
//  3. Re-matches unmatched plays against newly added artists (weekly)
//  4. Discovers newly-added shows on every active station (daily)
//
// It follows the same Start/Stop pattern as the other background ticker services.
type RadioFetchService struct {
	radioService   *RadioService
	discordService contracts.DiscordServiceInterface

	fetchInterval    time.Duration
	affinityInterval time.Duration
	rematchInterval  time.Duration
	discoverInterval time.Duration

	// autoBackfillDays: the create-on-first window (PSY-1153) — how far back the discover
	// run looks for a newly-discovered roster show's first episode, and the history it
	// imports when materializing the row. 0 → today-only create window (new shows still
	// created the day they air; not a disable). Admins can deepen an existing show's
	// history via POST /admin/radio-shows/{id}/backfill.
	autoBackfillDays int

	// backfillInterval / backfillLookbackDays drive the post-air backfill sweep
	// (PSY-1154): every backfillInterval, re-fetch playlists for aired episodes that
	// are still incomplete and aired within backfillLookbackDays. backfillLookbackDays
	// == 0 disables the loop (no goroutine started).
	backfillInterval     time.Duration
	backfillLookbackDays int

	// janitor* drive the nightly reconcile cycle (PSY-1155): lifecycle (active↔dormant
	// by grid membership on schedule-authoritative stations, by janitorDormantDays of
	// episode idle elsewhere — PSY-1348), play_count drift, and a wider-lookback
	// (janitorBackfillLookbackDays) backfill straggler sweep. janitorEnabled == false
	// (RADIO_JANITOR_INTERVAL_HOURS=0) disables the whole cycle (no goroutine started).
	janitorEnabled              bool
	janitorInterval             time.Duration
	janitorDormantDays          int
	janitorBackfillLookbackDays int

	// schedule* drive the WFMU program-schedule scrape (PSY-1159): every scheduleInterval,
	// scrape wfmu.org/table and write each 91.1 show's recurring slots to radio_shows.schedule
	// (the air-time source PSY-1152 stamps episode windows from). scheduleEnabled == false
	// (RADIO_SCHEDULE_INTERVAL_HOURS=0) disables the cycle (no goroutine started).
	scheduleEnabled  bool
	scheduleInterval time.Duration

	// substreamSchedule* drive the WFMU sub-stream schedule scrape (PSY-1322) —
	// a SEPARATE, faster ticker than the flagship grid scrape, because the
	// partial-today merge only converges when the excluded weekday rotates
	// (see DefaultSubstreamScheduleInterval).
	substreamScheduleEnabled  bool
	substreamScheduleInterval time.Duration

	// slotFetch* drive the schedule-aware slot fetch (PSY-1333): every
	// slotFetchInterval, single-show scoped fetches fire for shows whose stored
	// schedule had a slot start/end inside the window since the last tick — so
	// an episode row appears within minutes of its scheduled airing instead of
	// waiting out the (default 6h) station sweep. slotFetchEnabled == false
	// (RADIO_SLOT_FETCH_INTERVAL_MINUTES=0) disables the cycle (no goroutine
	// started). lastSlotFetchAt is the previous tick's window edge — touched
	// only by the single slot-fetch goroutine, so it needs no lock; a restart
	// zeroes it and the next tick falls back to a bounded cold-start lookback
	// (the station sweep remains the backstop for anything longer).
	slotFetchEnabled  bool
	slotFetchInterval time.Duration
	lastSlotFetchAt   time.Time

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger

	// The circuit breaker is no longer in-memory (PSY-1140): it lives in
	// radio_station_health.breaker_state and is owned end-to-end by RunStationSync
	// (read at the gate, written on the run's outcome via updateStationHealth), so
	// it survives restarts. The loops below just consult RunStationSyncResult.Skipped
	// for a breaker-open station.

	// retryBudget is the per-client (per-process) transient-retry budget (PSY-1142),
	// shared across the fetch + discover loops. Lazily initialized via budget() so
	// tests that build &RadioFetchService{...} directly still work.
	retryBudget *retryBudget
	budgetOnce  sync.Once

	// retryBackoffFn, when non-nil, overrides the Full-Jitter backoff delay — tests
	// set it to return 0 so the retry loop doesn't actually sleep. nil → fullJitterBackoff.
	retryBackoffFn func(retry int) time.Duration
}

// NewRadioFetchService creates a new radio fetch background service.
// Env vars:
//   - RADIO_FETCH_INTERVAL_HOURS (default 6)
//   - RADIO_AFFINITY_INTERVAL_HOURS (default 24)
//   - RADIO_REMATCH_INTERVAL_HOURS (default 168, i.e. 7 days)
//   - RADIO_DISCOVER_INTERVAL_HOURS (default 24)
//   - RADIO_AUTO_BACKFILL_DAYS (default 90; create-on-first window — how far back to
//     look for a new show's first episode + the history it arrives with; 0 = today-only)
//   - RADIO_BACKFILL_INTERVAL_HOURS (default 1; post-air playlist backfill sweep)
//   - RADIO_BACKFILL_LOOKBACK_DAYS (default 7; 0 disables the post-air backfill loop)
//   - RADIO_JANITOR_INTERVAL_HOURS (default 24; 0 disables the nightly janitor cycle)
//   - RADIO_JANITOR_DORMANT_DAYS (default 30; active↔dormant idle threshold)
//   - RADIO_JANITOR_BACKFILL_LOOKBACK_DAYS (default 30; janitor straggler sweep window)
//   - RADIO_SLOT_FETCH_INTERVAL_MINUTES (default 10; 0 disables the PSY-1333
//     schedule-aware slot fetch)
func NewRadioFetchService(
	radioService *RadioService,
	discordService contracts.DiscordServiceInterface,
) *RadioFetchService {
	// Loop intervals (hours; must be > 0, else the default). PSY-1270: see env.go.
	fetchInterval := envPositiveHours("RADIO_FETCH_INTERVAL_HOURS", DefaultRadioFetchInterval)
	affinityInterval := envPositiveHours("RADIO_AFFINITY_INTERVAL_HOURS", DefaultAffinityInterval)
	rematchInterval := envPositiveHours("RADIO_REMATCH_INTERVAL_HOURS", DefaultReMatchInterval)
	discoverInterval := envPositiveHours("RADIO_DISCOVER_INTERVAL_HOURS", DefaultDiscoverInterval)
	backfillInterval := envPositiveHours("RADIO_BACKFILL_INTERVAL_HOURS", DefaultBackfillInterval)

	// Backfill lookbacks (days; 0 explicitly disables, so >= 0; a negative/unparseable
	// value falls back to the default).
	autoBackfillDays := envNonNegativeInt("RADIO_AUTO_BACKFILL_DAYS", DefaultAutoBackfillDays)
	backfillLookbackDays := envNonNegativeInt("RADIO_BACKFILL_LOOKBACK_DAYS", DefaultBackfillLookbackDays)
	janitorBackfillLookbackDays := envNonNegativeInt("RADIO_JANITOR_BACKFILL_LOOKBACK_DAYS", DefaultJanitorBackfillLookbackDays)

	// Dormancy window (days; must be > 0, else the default).
	janitorDormantDays := envPositiveInt("RADIO_JANITOR_DORMANT_DAYS", DefaultJanitorDormantDays)

	// Janitor cycle (PSY-1155). RADIO_JANITOR_INTERVAL_HOURS=0 (or negative) disables
	// the whole nightly cycle; otherwise it sets the interval (default 24h). The
	// enable/disable-on-0 semantics don't fit the envPositiveHours helper, so this one
	// stays bespoke.
	janitorEnabled := true
	janitorInterval := DefaultJanitorInterval
	if envVal := os.Getenv("RADIO_JANITOR_INTERVAL_HOURS"); envVal != "" {
		if hours, err := strconv.Atoi(envVal); err == nil {
			if hours <= 0 {
				janitorEnabled = false
			} else {
				janitorInterval = time.Duration(hours) * time.Hour
			}
		}
	}

	// WFMU schedule scrape (PSY-1159). RADIO_SCHEDULE_INTERVAL_HOURS=0 (or negative)
	// disables the cycle; otherwise it sets the interval (default 7 days). Same
	// enable/disable-on-0 semantics as the janitor cycle, so it stays bespoke too.
	scheduleEnabled := true
	scheduleInterval := DefaultScheduleInterval
	if envVal := os.Getenv("RADIO_SCHEDULE_INTERVAL_HOURS"); envVal != "" {
		if hours, err := strconv.Atoi(envVal); err == nil {
			if hours <= 0 {
				scheduleEnabled = false
			} else {
				scheduleInterval = time.Duration(hours) * time.Hour
			}
		}
	}

	// WFMU sub-stream schedule scrape (PSY-1322). Same enable/disable-on-0
	// semantics; deliberately its own knob — see DefaultSubstreamScheduleInterval
	// for why this cadence must stay daily-or-faster.
	substreamScheduleEnabled := true
	substreamScheduleInterval := DefaultSubstreamScheduleInterval
	if envVal := os.Getenv("RADIO_SUBSTREAM_SCHEDULE_INTERVAL_HOURS"); envVal != "" {
		if hours, err := strconv.Atoi(envVal); err == nil {
			if hours <= 0 {
				substreamScheduleEnabled = false
			} else {
				substreamScheduleInterval = time.Duration(hours) * time.Hour
			}
		}
	}

	// Schedule-aware slot fetch (PSY-1333). Same enable/disable-on-0 semantics;
	// minutes, not hours — the whole point is a boundary-to-visible latency of
	// ~one interval, and an hour floor would defeat it.
	slotFetchEnabled := true
	slotFetchInterval := DefaultSlotFetchInterval
	if envVal := os.Getenv("RADIO_SLOT_FETCH_INTERVAL_MINUTES"); envVal != "" {
		if minutes, err := strconv.Atoi(envVal); err == nil {
			if minutes <= 0 {
				slotFetchEnabled = false
			} else {
				slotFetchInterval = time.Duration(minutes) * time.Minute
			}
		}
	}

	return &RadioFetchService{
		radioService:                radioService,
		discordService:              discordService,
		fetchInterval:               fetchInterval,
		affinityInterval:            affinityInterval,
		rematchInterval:             rematchInterval,
		discoverInterval:            discoverInterval,
		autoBackfillDays:            autoBackfillDays,
		backfillInterval:            backfillInterval,
		backfillLookbackDays:        backfillLookbackDays,
		janitorEnabled:              janitorEnabled,
		janitorInterval:             janitorInterval,
		janitorDormantDays:          janitorDormantDays,
		janitorBackfillLookbackDays: janitorBackfillLookbackDays,
		scheduleEnabled:             scheduleEnabled,
		scheduleInterval:            scheduleInterval,
		substreamScheduleEnabled:    substreamScheduleEnabled,
		substreamScheduleInterval:   substreamScheduleInterval,
		slotFetchEnabled:            slotFetchEnabled,
		slotFetchInterval:           slotFetchInterval,
		stopCh:                      make(chan struct{}),
		logger:                      slog.Default(),
	}
}

// Start begins the background radio fetch service.
func (s *RadioFetchService) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.runFetchLoop(ctx)
	s.wg.Add(1)
	go s.runAffinityLoop(ctx)
	s.wg.Add(1)
	go s.runReMatchLoop(ctx)
	s.wg.Add(1)
	go s.runDiscoverLoop(ctx)

	// Post-air playlist backfill (PSY-1154). Skipped entirely when disabled
	// (RADIO_BACKFILL_LOOKBACK_DAYS=0) so no goroutine spins on an empty sweep.
	if s.backfillLookbackDays > 0 {
		s.wg.Add(1)
		go s.runBackfillLoop(ctx)
	}

	// Nightly janitor/reconcile (PSY-1155). Skipped when disabled
	// (RADIO_JANITOR_INTERVAL_HOURS=0).
	if s.janitorEnabled {
		s.wg.Add(1)
		go s.runJanitorLoop(ctx)
	}

	// WFMU schedule scrape (PSY-1159). Skipped when disabled
	// (RADIO_SCHEDULE_INTERVAL_HOURS=0).
	if s.scheduleEnabled {
		s.wg.Add(1)
		go s.runScheduleLoop(ctx)
	}

	// WFMU sub-stream schedule scrape (PSY-1322). Skipped when disabled
	// (RADIO_SUBSTREAM_SCHEDULE_INTERVAL_HOURS=0).
	if s.substreamScheduleEnabled {
		s.wg.Add(1)
		go s.runSubstreamScheduleLoop(ctx)
	}

	// Schedule-aware slot fetch (PSY-1333). Skipped when disabled
	// (RADIO_SLOT_FETCH_INTERVAL_MINUTES=0).
	if s.slotFetchEnabled {
		s.wg.Add(1)
		go s.runSlotFetchLoop(ctx)
	}

	s.logger.Info("radio fetch service started",
		"fetch_interval_hours", s.fetchInterval.Hours(),
		"fetch_lookback_floor_days", resolveFetchLookbackFloorDays(),
		"affinity_interval_hours", s.affinityInterval.Hours(),
		"rematch_interval_hours", s.rematchInterval.Hours(),
		"discover_interval_hours", s.discoverInterval.Hours(),
		"backfill_interval_hours", s.backfillInterval.Hours(),
		"backfill_lookback_days", s.backfillLookbackDays,
		"janitor_enabled", s.janitorEnabled,
		"janitor_interval_hours", s.janitorInterval.Hours(),
		"janitor_dormant_days", s.janitorDormantDays,
		"schedule_enabled", s.scheduleEnabled,
		"schedule_interval_hours", s.scheduleInterval.Hours(),
		"substream_schedule_enabled", s.substreamScheduleEnabled,
		"substream_schedule_interval_hours", s.substreamScheduleInterval.Hours(),
		"slot_fetch_enabled", s.slotFetchEnabled,
		"slot_fetch_interval_minutes", s.slotFetchInterval.Minutes(),
	)
}

// Stop gracefully stops the radio fetch service.
func (s *RadioFetchService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("radio fetch service stopped")
}

// runFetchLoop runs the periodic station fetch cycle. Runs once on startup.
func (s *RadioFetchService) runFetchLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_fetch", s.fetchInterval, s.stopCh, true, func(_ context.Context) {
		s.runFetchCycle()
	})
}

// runBackfillLoop runs the periodic post-air playlist backfill sweep (PSY-1154).
// runImmediately=false: unlike fetch/discover there is no "see output now" payoff in
// firing it at boot, and skipping the startup tick avoids piling a third sweep onto
// the fetch+discover co-fire (which both run immediately).
func (s *RadioFetchService) runBackfillLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_backfill", s.backfillInterval, s.stopCh, false, func(_ context.Context) {
		s.runBackfillCycle()
	})
}

// runJanitorLoop runs the nightly janitor/reconcile cycle (PSY-1155).
// runImmediately=false: it's a maintenance sweep with no urgency at boot, and the
// lifecycle + play_count reconciles + the straggler backfill are heavier than a
// normal tick — skipping the startup fire avoids piling them onto the fetch+discover
// co-fire. Admins can force a run via RunJanitorCycleNow.
func (s *RadioFetchService) runJanitorLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_janitor", s.janitorInterval, s.stopCh, false, func(_ context.Context) {
		s.runJanitorCycle()
	})
}

// runScheduleLoop runs the periodic WFMU schedule scrape (PSY-1159).
// runImmediately=true: the schedule is the air-time source for episode windowing, so a
// fresh deploy should populate it without waiting a full (weekly) interval — and it's a
// single cheap GET of one page, not a per-station sweep.
func (s *RadioFetchService) runScheduleLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_schedule", s.scheduleInterval, s.stopCh, true, func(_ context.Context) {
		s.runScheduleCycle()
	})
}

// runScheduleCycle scrapes wfmu.org/table and writes the parsed recurring slots onto the
// WFMU 91.1 shows (matched by external_id). WFMU-only: the /table grid is the 91.1
// schedule. Every failure is logged and returns — never fatal; the ticker loop continues.
func (s *RadioFetchService) runScheduleCycle() {
	start := time.Now()
	provider, err := s.radioService.getProvider(catalogm.PlaylistSourceWFMU)
	if err != nil {
		s.logger.Error("radio schedule: get WFMU provider failed", "error", err)
		return
	}
	defer closeProvider(provider)

	sd, ok := provider.(scheduleDiscoverer)
	if !ok {
		s.logger.Error("radio schedule: WFMU provider does not support schedule discovery")
		return
	}
	entries, skipped, err := sd.DiscoverSchedule()
	if err != nil {
		s.logger.Error("radio schedule: scrape failed", "error", err)
		return
	}
	matched, unmatched, cleared, err := s.radioService.ApplyWFMUSchedule(entries)
	if err != nil {
		s.logger.Error("radio schedule: apply failed", "error", err)
		return
	}
	s.logger.Info("radio schedule cycle complete",
		"shows_parsed", len(entries),
		"cells_skipped", skipped,
		"schedules_written", matched,
		"unmatched_codes", unmatched,
		"schedules_cleared", cleared,
		"duration", time.Since(start),
	)
}

// runSlotFetchLoop runs the schedule-aware slot fetch (PSY-1333).
// runImmediately=false: the boot co-fire already runs a FULL station sweep,
// which supersedes anything a slot tick would fetch; the first tick one
// interval later starts the steady state (its cold-start lookback covers the
// boot window).
func (s *RadioFetchService) runSlotFetchLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_slot_fetch", s.slotFetchInterval, s.stopCh, false, func(_ context.Context) {
		s.runSlotFetchCycle()
	})
}

// runSlotFetchCycle fires a single-show scoped fetch for every show whose
// stored schedule had a slot start or end inside (lastTick, now] — the
// PSY-1333 freshness fix: the row for a show appears within ~one interval of
// its scheduled airing instead of waiting out the 6h station sweep. Each
// scoped run routes through RunStationSync (per-station lock, breaker,
// run row with show_id) with Trigger=Scheduled; a lock-contended or
// breaker-skipped show is simply logged — the interval sweep remains the
// backstop for anything a tick misses. The window edge advances to `now`
// unconditionally: a failed scoped fetch must not be re-fired every tick
// (its next boundary, the post-air backfill sweep, and the station sweep
// all still cover it). Known debt: the admin sync-run feed has no filter
// for the scoped rows this writes (PSY-1343).
func (s *RadioFetchService) runSlotFetchCycle() {
	now := time.Now()
	from := s.lastSlotFetchAt
	if from.IsZero() {
		// Cold start (fresh boot): look back two intervals so a quick restart
		// doesn't drop a boundary that crossed mid-deploy. Anything older is the
		// station sweep's job — an unbounded lookback would re-fetch every show
		// that aired since the last shutdown.
		from = now.Add(-2 * s.slotFetchInterval)
	}
	s.lastSlotFetchAt = now

	due, err := s.radioService.ShowsWithSlotBoundariesIn(from, now)
	if err != nil {
		s.logger.Error("radio slot fetch: listing due shows failed", "error", err)
		return
	}
	if len(due) == 0 {
		return
	}

	var fetched, skipped, failed int
	for stationID, showIDs := range due {
		for _, showID := range showIDs {
			id := showID
			res, err := s.radioService.RunStationSync(context.Background(), stationID, RunStationSyncOpts{
				Mode:    catalogm.RadioSyncRunTypeFetch,
				Trigger: catalogm.RadioSyncRunTriggerScheduled,
				ShowID:  &id,
			})
			switch {
			case err != nil:
				failed++
				s.logger.Warn("radio slot fetch: scoped fetch failed",
					"station_id", stationID, "show_id", showID, "error", err)
			case res.LockContended || res.Skipped:
				skipped++
			default:
				fetched++
			}
		}
	}
	s.logger.Info("radio slot fetch cycle complete",
		"window_start", from.UTC().Format(time.RFC3339),
		"shows_fetched", fetched, "shows_skipped", skipped, "shows_failed", failed,
		"duration", time.Since(now))
}

// runSubstreamScheduleLoop runs the periodic WFMU sub-stream schedule scrape
// (PSY-1322) on its OWN daily ticker — not the weekly flagship cycle, whose
// fixed weekday would freeze the partial-today exclusion on one day forever.
// runImmediately=true for the same reason the flagship loop uses it: a fresh
// deploy should populate windows without waiting a full interval.
func (s *RadioFetchService) runSubstreamScheduleLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_substream_schedule", s.substreamScheduleInterval, s.stopCh, true, func(_ context.Context) {
		s.runSubstreamScheduleCycle()
	})
}

// runSubstreamScheduleCycle scrapes each WFMU sub-stream's rolling-week
// schedule page (PSY-1322). Per-station failures are logged and the loop
// continues — one broken page never blocks the siblings; an unseeded station
// (dev environments) logs quietly, not as an error. Iteration order is fixed
// for deterministic logs.
func (s *RadioFetchService) runSubstreamScheduleCycle() {
	provider, err := s.radioService.getProvider(catalogm.PlaylistSourceWFMU)
	if err != nil {
		s.logger.Error("radio substream schedule: get WFMU provider failed", "error", err)
		return
	}
	defer closeProvider(provider)

	sd, ok := provider.(substreamScheduleDiscoverer)
	if !ok {
		s.logger.Error("radio substream schedule: WFMU provider does not support sub-stream schedule discovery")
		return
	}
	for _, stationSlug := range slices.Sorted(maps.Keys(wfmuSubstreamSchedulePages)) {
		pagePath := wfmuSubstreamSchedulePages[stationSlug]
		start := time.Now()
		matched, unmatched, cleared, skipped, err := s.radioService.applySubstreamScheduleForStation(sd, stationSlug, pagePath)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logger.Info("radio substream schedule: station not seeded, skipped", "station", stationSlug)
			continue
		}
		if err != nil {
			s.logger.Error("radio substream schedule: cycle failed", "station", stationSlug, "error", err)
			continue
		}
		s.logger.Info("radio substream schedule cycle complete",
			"station", stationSlug,
			"rows_skipped", skipped,
			"schedules_written", matched,
			"unmatched_codes", unmatched,
			"schedules_cleared", cleared,
			"duration", time.Since(start),
		)
	}
}

// runAffinityLoop runs the periodic affinity computation.
// No startup cycle — the first fetch cycle is allowed to complete first.
func (s *RadioFetchService) runAffinityLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_affinity", s.affinityInterval, s.stopCh, false, func(_ context.Context) {
		s.runAffinityCycle()
	})
}

// runReMatchLoop runs the periodic re-matching of unmatched plays.
// No startup cycle — the first fetch cycle is allowed to complete first.
func (s *RadioFetchService) runReMatchLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_rematch", s.rematchInterval, s.stopCh, false, func(cycleCtx context.Context) {
		s.runReMatchCycle(cycleCtx)
	})
}

// runDiscoverLoop runs the periodic show-discovery cycle. Runs immediately on
// startup so operators see output without waiting a full interval.
func (s *RadioFetchService) runDiscoverLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_discover", s.discoverInterval, s.stopCh, true, func(_ context.Context) {
		s.runDiscoverCycle()
	})
}

// fetchStationWithRetry calls the run op and retries a TRANSIENT error per the
// two-tier policy (PSY-1142): up to radioRetryMaxAttempts total attempts with
// Full-Jitter backoff (tier 1), each retry gated by the per-client retry budget
// (tier 2). Permanent errors fail immediately; an error that turns permanent on a
// retry stops the loop. The breaker counter is the caller's responsibility (persisted
// by RunStationSync's updateStationHealth); this helper only owns the retry decision
// so the fetch + discover loops can share it. The stopCh-aware sleep unwinds a
// mid-backoff service shutdown promptly.
func (s *RadioFetchService) fetchStationWithRetry(
	stationID uint,
	stationName string,
	op string, // "fetch" or "discover" — for log clarity
	call func() (any, error),
) (any, error) {
	s.budget().noteRequest()

	result, err := call()
	if err == nil {
		return result, nil
	}
	if classifyError(err) != kindTransient {
		return nil, err
	}

	for retry := 1; retry < radioRetryMaxAttempts; retry++ {
		if !s.budget().allowRetry() {
			s.logger.Warn("transient station error; retry shed by per-client budget",
				"station_id", stationID, "station_name", stationName, "op", op, "error", err)
			return nil, err
		}

		delay := s.retryDelay(retry - 1) // retry 1 → exponent 0
		s.logger.Warn("transient station error, retrying after jittered backoff",
			"station_id", stationID, "station_name", stationName, "op", op,
			"retry", retry, "backoff", delay, "error", err)
		select {
		case <-time.After(delay):
		case <-s.stopCh:
			return nil, err // shutdown mid-backoff: surface the original transient error
		}

		result, err = call()
		if err == nil {
			s.logger.Info("station recovered after transient retry",
				"station_id", stationID, "station_name", stationName, "op", op, "retry", retry)
			return result, nil
		}
		if classifyError(err) != kindTransient {
			return nil, err // turned permanent — stop retrying
		}
	}
	// Exhausted attempts; surface the last (still-transient) error so the caller's
	// log reflects the post-retry state.
	return nil, err
}

// retryDelay returns the backoff for the given 0-based retry exponent, using the
// test seam when set, else Full-Jitter.
func (s *RadioFetchService) retryDelay(exp int) time.Duration {
	if s.retryBackoffFn != nil {
		return s.retryBackoffFn(exp)
	}
	return fullJitterBackoff(exp)
}

// fullJitterBackoff returns a random duration in [0, min(cap, base·2^exp)) — AWS
// "Full Jitter", the recommended de-synchronizing backoff. math/rand/v2's global
// source is fine for jitter (no determinism needed).
func fullJitterBackoff(exp int) time.Duration {
	ceiling := radioRetryBackoffCap
	// In practice exp is only 0..1 (radioRetryMaxAttempts=3). The guards keep this
	// safe if a caller ever passes a large exponent: base (5e8 ns) << exp overflows
	// int64 around exp 35 (wrapping to ≤0), and the < 62 rail avoids an out-of-range
	// shift; either way `scaled > 0` carries the real check and ceiling stays at the
	// 30s cap, so rand.Int64N never sees a non-positive bound.
	if exp >= 0 && exp < 62 {
		if scaled := radioRetryBackoffBase << exp; scaled > 0 && scaled < ceiling {
			ceiling = scaled
		}
	}
	return time.Duration(rand.Int64N(int64(ceiling)))
}

// budget returns the per-client retry budget, lazily initialized so direct-literal
// test constructions work without wiring it up.
func (s *RadioFetchService) budget() *retryBudget {
	s.budgetOnce.Do(func() {
		if s.retryBudget == nil {
			s.retryBudget = newRetryBudget()
		}
	})
	return s.retryBudget
}

// ───────────────────────────── per-client retry budget (PSY-1142) ─────────────────────────────

// retryBudget caps the transient-retry RATIO (retries / requests) over a rolling
// window — the per-client tier of the Google SRE retry budget. Below minRequests
// requests in the window it is inactive (too little volume to storm). now is
// injectable for deterministic window tests.
type retryBudget struct {
	mu          sync.Mutex
	window      time.Duration
	ratio       float64
	minRequests int
	now         func() time.Time
	events      []budgetEvent
}

// budgetEvent is one request or retry, timestamped for the rolling window.
type budgetEvent struct {
	at    time.Time
	retry bool // false = an original request, true = a retry
}

func newRetryBudget() *retryBudget {
	return &retryBudget{
		window:      radioRetryBudgetWindow,
		ratio:       radioRetryBudgetRatio,
		minRequests: radioRetryBudgetMinReqs,
		now:         time.Now,
	}
}

// noteRequest records one original (non-retry) request. It is called per
// fetchStationWithRetry invocation, before the op runs — so a no-op run (breaker-open
// Skipped / LockContended, no provider hit) is still counted. That inflates the ratio
// denominator slightly, which only makes the budget MORE lenient (errs toward allowing
// retries, never over-shedding); keeping the helper op-agnostic is worth that.
func (b *retryBudget) noteRequest() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune()
	b.events = append(b.events, budgetEvent{at: b.now()})
}

// allowRetry reports whether a retry is within budget; when it returns true it also
// records the retry (so the ratio reflects retries granted). Below minRequests the
// budget is inactive — the per-request attempt cap is then the only limit.
func (b *retryBudget) allowRetry() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune()
	var requests, retries int
	for _, e := range b.events {
		if e.retry {
			retries++
		} else {
			requests++
		}
	}
	if requests >= b.minRequests && float64(retries) >= b.ratio*float64(requests) {
		return false
	}
	b.events = append(b.events, budgetEvent{at: b.now(), retry: true})
	return true
}

// prune drops events older than the window, compacting in place so the backing array
// stays bounded to one window's events (the loops are low-rate).
func (b *retryBudget) prune() {
	cutoff := b.now().Add(-b.window)
	live := b.events[:0]
	for _, e := range b.events {
		if !e.at.Before(cutoff) {
			live = append(live, e)
		}
	}
	b.events = live
}

// runFetchCycle fetches new episodes from all active stations sequentially.
func (s *RadioFetchService) runFetchCycle() {
	cycleStart := time.Now()
	s.logger.Info("starting radio fetch cycle")

	stations, err := s.radioService.GetActiveStationsWithPlaylistSource()
	if err != nil {
		s.logger.Error("failed to list active stations", "error", err)
		return
	}

	if len(stations) == 0 {
		s.logger.Info("no active stations with playlist source found")
		return
	}

	var (
		totalProcessed int
		totalEpisodes  int
		totalPlays     int
		totalMatched   int
		totalFailed    int
		totalTransient int
		totalSkipped   int // breaker-open or lock-contended no-ops (no fetch happened)
	)

	// Process stations sequentially to respect per-provider rate limits. The
	// breaker is no longer pre-checked here (PSY-1140): RunStationSync consults the
	// persistent breaker itself and returns Skipped for an open station (writing a
	// skipped run row — better observability than the old silent in-memory skip).
	for _, station := range stations {
		totalProcessed++
		s.logger.Info("fetching station",
			"station_id", station.ID,
			"station_name", station.Name,
		)

		// Route through the unified orchestrator (PSY-1134): the run is recorded in
		// radio_sync_runs, and as of PSY-1140 RunStationSync OWNS the persistent
		// breaker end-to-end (reads the gate, writes the outcome via
		// updateStationHealth) — it is now the sole, authoritative breaker. The
		// returned hard error still feeds the two-tier transient retry below
		// (fetchStationWithRetry: Full-Jitter backoff + retry budget, PSY-1142).
		raw, err := s.fetchStationWithRetry(station.ID, station.Name, "fetch",
			func() (any, error) {
				return s.radioService.RunStationSync(context.Background(), station.ID, RunStationSyncOpts{
					Mode:    catalogm.RadioSyncRunTypeFetch,
					Trigger: catalogm.RadioSyncRunTriggerScheduled,
				})
			},
		)
		if err != nil {
			totalFailed++
			// The persistent breaker counter is owned by RunStationSync's
			// updateStationHealth; here we classify only to label the log + tally
			// post-retry transients (the count drives no breaker decision now).
			kind := classifyError(err)
			if kind == kindTransient {
				totalTransient++
			}
			s.logger.Error("station fetch failed",
				"station_id", station.ID,
				"station_name", station.Name,
				"error_kind", errorKindName(kind),
				"error", err,
			)
			continue
		}

		result := raw.(*RunStationSyncResult)
		// A no-op run — the per-station lock was held by another in-flight sync, or
		// the persistent breaker is open (a skipped run row was written) — produced
		// no Import payload, so skip the result handling below. The persistent
		// breaker counter is reset by RunStationSync's own success path, not here.
		if result.LockContended || result.Skipped {
			totalSkipped++
			reason := "breaker_open"
			if result.LockContended {
				reason = "sync_already_running"
			}
			s.logger.Info("station fetch skipped",
				"station_id", station.ID, "station_name", station.Name, "reason", reason)
			continue
		}

		if imp := result.Import; imp != nil {
			totalEpisodes += imp.EpisodesImported
			totalPlays += imp.PlaysImported
			totalMatched += imp.PlaysMatched

			s.logger.Info("station fetch complete",
				"station_id", station.ID,
				"station_name", station.Name,
				"run_id", result.RunID,
				"episodes_imported", imp.EpisodesImported,
				"plays_imported", imp.PlaysImported,
				"plays_matched", imp.PlaysMatched,
			)

			if len(imp.Errors) > 0 {
				s.logger.Warn("station fetch had errors",
					"station_id", station.ID,
					"station_name", station.Name,
					"error_count", len(imp.Errors),
				)
			}
		}
	}

	cycleDuration := time.Since(cycleStart)
	s.logger.Info("radio fetch cycle complete",
		"stations_processed", totalProcessed,
		// PSY-1140: stations the breaker skipped (or that another run held the lock
		// for) — counted in stations_processed but did NOT fetch. Each also wrote a
		// skipped run row.
		"stations_skipped", totalSkipped,
		"episodes_imported", totalEpisodes,
		"plays_imported", totalPlays,
		"plays_matched", totalMatched,
		"failures", totalFailed,
		// PSY-887: counts stations whose error STILL classified as transient
		// after the in-cycle retry — NOT total retries fired. A station that
		// recovered on retry is not counted (success path resets the counter).
		"transient_failures_after_retry", totalTransient,
		"duration", cycleDuration,
	)
}

// errorKindName returns a stable log key for an errorKind classification.
func errorKindName(k errorKind) string {
	if k == kindTransient {
		return "transient"
	}
	return "permanent"
}

// runAffinityCycle computes the artist affinity table and syncs to artist relationships.
func (s *RadioFetchService) runAffinityCycle() {
	start := time.Now()
	s.logger.Info("starting affinity computation")

	if err := s.radioService.ComputeAffinity(); err != nil {
		s.logger.Error("affinity computation failed", "error", err)
		return
	}

	s.logger.Info("affinity computation complete", "duration", time.Since(start))

	// PSY-1261: compute the disparity-filter backbone significance over the freshly-recomputed
	// graph and store it on radio_artist_affinity. Non-fatal — the backbone is additive metadata
	// (no endpoint reads it yet), so a failure here must not block the relationship sync below.
	if err := s.radioService.ComputeBackboneSignificance(); err != nil {
		s.logger.Error("backbone significance computation failed", "error", err)
	}

	// Sync affinity data to artist_relationships as radio_cooccurrence type
	syncStart := time.Now()
	s.logger.Info("starting affinity-to-relationship sync")

	syncResult, err := s.radioService.SyncAffinityToRelationships()
	if err != nil {
		s.logger.Error("affinity-to-relationship sync failed", "error", err)
		return
	}

	s.logger.Info("affinity-to-relationship sync complete",
		"created", syncResult.Created,
		"updated", syncResult.Updated,
		"deleted", syncResult.Deleted,
		"failed", syncResult.Failed,
		"duration", time.Since(syncStart),
	)

	// PSY-1262: rebuild the Leiden community partition over the freshly-synced
	// relationship graph. Non-fatal like the backbone step — on failure the
	// previous partition stays live (the swap is transactional).
	if _, err := s.radioService.ComputeArtistCommunities(); err != nil {
		s.logger.Error("artist community computation failed", "error", err)
	}
}

// runReMatchCycle re-matches unmatched plays against current artists.
func (s *RadioFetchService) runReMatchCycle(ctx context.Context) {
	start := time.Now()
	s.logger.Info("starting re-match of unmatched plays")

	cycleCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if s.stopCh != nil {
		go func() {
			select {
			case <-s.stopCh:
				cancel()
			case <-cycleCtx.Done():
			}
		}()
	}

	result, err := s.radioService.ReMatchUnmatchedChunked(cycleCtx, defaultReMatchNamePageSize, UnmatchedArtistNameFilter{}, 0)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			s.logger.Info("re-match abandoned on shutdown",
				"names_processed", result.NamesProcessed,
				"matched", result.Matched,
				"duration", time.Since(start),
			)
			return
		}
		s.logger.Error("re-match failed", "error", err)
		return
	}

	s.logger.Info("re-match complete",
		"names_processed", result.NamesProcessed,
		"total", result.Total,
		"matched", result.Matched,
		"unmatched", result.Unmatched,
		"duration", time.Since(start),
	)
}

// runDiscoverCycle calls DiscoverStationShows for every active station with a
// playlist source. The cycle is idempotent — existing shows are no-ops; only
// brand-new rows fire the per-station Discord notification.
func (s *RadioFetchService) runDiscoverCycle() {
	cycleStart := time.Now()
	s.logger.Info("starting radio discover cycle")

	stations, err := s.radioService.GetActiveStationsWithPlaylistSource()
	if err != nil {
		s.logger.Error("failed to list active stations for discover", "error", err)
		return
	}

	if len(stations) == 0 {
		s.logger.Info("no active stations with playlist source found for discover")
		return
	}

	var (
		totalProcessed  int
		totalDiscovered int
		totalNew        int
		totalCreated    int // shows materialized via create-on-first (PSY-1153)
		totalFailed     int
		totalTransient  int
		totalSkipped    int // breaker-open or lock-contended no-ops (no discover happened)
	)

discoverLoop:
	for _, station := range stations {
		// Bail between stations on shutdown. PSY-1153 made each discover heavy (inline
		// create-on-first import under the per-station lock), so — like runBackfillSweep —
		// the cycle must stop promptly rather than open a fresh per-station run for every
		// remaining station; the in-flight station's run is cancelled by its watcher.
		// (Labeled break: a bare `break` in a select exits only the select.)
		select {
		case <-s.stopCh:
			s.logger.Info("radio discover cycle: abandoned on shutdown", "processed", totalProcessed)
			break discoverLoop
		default:
		}

		// Breaker pre-check removed (PSY-1140) — RunStationSync handles the
		// persistent breaker and returns Skipped for an open station, same as
		// runFetchCycle.
		totalProcessed++
		s.logger.Info("discovering shows for station",
			"station_id", station.ID,
			"station_name", station.Name,
		)

		// Route through the unified orchestrator (PSY-1134). PSY-1153: the discover run
		// now ALSO create-on-first-imports newly-discovered aired shows (executeSyncMode),
		// so it carries the create window (autoBackfillDays = how far back to look for a
		// new show's first episode, i.e. the history it arrives with) and gets shutdown
		// cancellation so Stop() doesn't block on a long create-on-first import. Still
		// wrapped in the transient-retry helper like the other paths.
		until := time.Now()
		since := until.AddDate(0, 0, -s.autoBackfillDays)
		raw, err := s.fetchStationWithRetry(station.ID, station.Name, "discover",
			func() (any, error) {
				return s.runStationSyncWithShutdownCancel(station.ID, RunStationSyncOpts{
					Mode:        catalogm.RadioSyncRunTypeDiscover,
					Trigger:     catalogm.RadioSyncRunTriggerScheduled,
					WindowStart: &since,
					WindowEnd:   &until,
				})
			},
		)
		if err != nil {
			totalFailed++
			kind := classifyError(err) // log label + transient tally only (see runFetchCycle)
			if kind == kindTransient {
				totalTransient++
			}
			s.logger.Error("station discover failed",
				"station_id", station.ID,
				"station_name", station.Name,
				"error_kind", errorKindName(kind),
				"error", err,
			)
			continue
		}

		result := raw.(*RunStationSyncResult)
		// See runFetchCycle: a no-op (lock contended or breaker open) carries no
		// Discover payload, so skip the result handling below.
		if result.LockContended || result.Skipped {
			totalSkipped++
			reason := "breaker_open"
			if result.LockContended {
				reason = "sync_already_running"
			}
			s.logger.Info("station discover skipped",
				"station_id", station.ID, "station_name", station.Name, "reason", reason)
			continue
		}

		disc := result.Discover
		if disc == nil {
			continue
		}

		totalDiscovered += disc.ShowsDiscovered
		totalNew += disc.ShowsNew
		totalCreated += len(disc.CreatedShowNames)

		s.logger.Info("station discover complete",
			"station_id", station.ID,
			"station_name", station.Name,
			"run_id", result.RunID,
			"shows_discovered", disc.ShowsDiscovered,
			"shows_new", disc.ShowsNew, // roster candidates (not yet persisted)
			"shows_created", len(disc.CreatedShowNames), // create-on-first materialized
			"error_count", len(disc.Errors),
		)

		// PSY-1153: create-on-first ran inside the discover run above, so a row exists
		// only for shows that actually aired in the window. Notify on those real
		// creations (replaces the old fire-before-a-row-exists discover ping).
		if len(disc.CreatedShowNames) > 0 && s.discordService != nil {
			s.discordService.NotifyNewRadioShows(station.Name, disc.CreatedShowNames)
		}
	}

	s.logger.Info("radio discover cycle complete",
		"stations_processed", totalProcessed,
		"stations_skipped", totalSkipped, // breaker/lock no-ops, counted in processed (PSY-1140)
		"shows_discovered", totalDiscovered,
		"shows_new", totalNew,
		"shows_created", totalCreated, // create-on-first materialized (PSY-1153)
		"failures", totalFailed,
		// PSY-887: same semantics as runFetchCycle — see fetch-cycle log note.
		"transient_failures_after_retry", totalTransient,
		"duration", time.Since(cycleStart),
	)
}

// runStationSyncWithShutdownCancel runs RunStationSync with a watcher that cancels the
// run on service shutdown (s.stopCh) so Stop() never blocks on a long import. The
// watcher learns the run id via OnRunOpened and flips the run to cancelled on stopCh;
// the executor's cancel checks (isSyncRunCancelled) then unwind it within ~one episode/
// show. A caller-supplied OnRunOpened is chained. The watcher is fully joined before
// returning, so no DB-touching goroutine outlives the WaitGroup barrier in Stop().
// Shared by the auto-backfill (PSY-1135) and the create-on-first discover run (PSY-1153,
// now heavy enough to need shutdown cancellation).
func (s *RadioFetchService) runStationSyncWithShutdownCancel(stationID uint, opts RunStationSyncOpts) (*RunStationSyncResult, error) {
	runIDCh := make(chan uint, 1)
	done := make(chan struct{})
	watcherExited := make(chan struct{})
	shared.GoSafe(context.Background(), "radio_sync_shutdown_cancel", func() {
		defer close(watcherExited)
		select {
		case runID := <-runIDCh:
			select {
			case <-s.stopCh:
				_ = s.radioService.CancelSyncRun(runID)
			case <-done:
			}
		case <-done:
		}
	})

	callerOnOpen := opts.OnRunOpened
	opts.OnRunOpened = func(id uint) {
		runIDCh <- id
		if callerOnOpen != nil {
			callerOnOpen(id)
		}
	}

	res, err := s.radioService.RunStationSync(context.Background(), stationID, opts)
	close(done)
	<-watcherExited
	return res, err
}

// runAutoBackfillShow runs one show's auto-backfill (RunStationSync backfill) with
// shutdown cancellation. Returns the run result, or nil on a pre-open failure (logged).
func (s *RadioFetchService) runAutoBackfillShow(stationID, showID uint, since, until time.Time) *RunStationSyncResult {
	res, err := s.runStationSyncWithShutdownCancel(stationID, RunStationSyncOpts{
		Mode:        catalogm.RadioSyncRunTypeBackfill,
		Trigger:     catalogm.RadioSyncRunTriggerAutoBackfill,
		ShowID:      &showID,
		WindowStart: &since,
		WindowEnd:   &until,
	})
	if res == nil {
		s.logger.Warn("auto_backfill_open_failed", "show_id", showID, "error", err)
		return nil
	}
	if err != nil && res.Status == catalogm.RadioSyncRunStatusFailed {
		s.logger.Warn("auto_backfill_show_failed", "show_id", showID, "error", err)
	}
	return res
}

// runBackfillCycle is the post-air playlist backfill sweep (PSY-1154). It finds shows
// with aired episodes whose playlist is still incomplete (within the lookback window),
// then re-fetches each show's playlists through RunStationSync(backfill) — reusing
// runAutoBackfillShow, so each sweep is traced in radio_sync_runs and honors the
// per-station lock, persistent breaker, and shutdown cancellation. Shows are processed
// SEQUENTIALLY so a roster burst stays within the per-provider 1-req/sec rate limit, and
// the loop bails between shows on shutdown.
func (s *RadioFetchService) runBackfillCycle() {
	cycleStart := time.Now()
	lookback := time.Duration(s.backfillLookbackDays) * 24 * time.Hour
	r := s.runBackfillSweep(lookback)
	s.logger.Info("radio backfill cycle complete",
		"lookback_days", s.backfillLookbackDays,
		"shows_processed", r.processed,
		"shows_completed", r.completed,
		"shows_skipped", r.skipped,
		"episodes_imported", r.episodes,
		"plays_matched", r.plays,
		"duration", time.Since(cycleStart),
	)
}

// backfillSweepResult tallies one backfill sweep.
type backfillSweepResult struct {
	processed, completed, skipped, episodes, plays int
}

// runBackfillSweep re-fetches playlists for aired incomplete episodes within lookback,
// per show, through RunStationSync(backfill) (reusing runAutoBackfillShow — traced in
// radio_sync_runs, per-station-lock + breaker + shutdown-cancel honored). Shows are
// processed SEQUENTIALLY to respect the per-provider rate limit, bailing between shows
// on shutdown. Shared by the hourly post-air sweep (PSY-1154) and the nightly janitor's
// wider straggler sweep (PSY-1155), which differ only in `lookback`.
func (s *RadioFetchService) runBackfillSweep(lookback time.Duration) backfillSweepResult {
	var r backfillSweepResult

	candidates, err := s.radioService.ListBackfillCandidates(lookback, catalogm.RadioBackfillMaxAttempts, time.Now())
	if err != nil {
		s.logger.Error("radio backfill: listing candidates failed", "error", err)
		return r
	}
	if len(candidates) == 0 {
		return r
	}

	for _, c := range candidates {
		// Stop cleanly between shows on shutdown.
		select {
		case <-s.stopCh:
			s.logger.Info("radio backfill sweep abandoned on shutdown", "processed", r.processed)
			return r
		default:
		}

		r.processed++
		res := s.runAutoBackfillShow(c.StationID, c.ShowID, c.Since, c.Until)
		if res == nil {
			continue // pre-open failure, already logged
		}
		if res.LockContended || res.Skipped {
			r.skipped++
			continue // a scheduled run holds the lock, or the breaker is open — retry next cycle
		}
		if res.Status == catalogm.RadioSyncRunStatusCancelled {
			s.logger.Info("radio backfill sweep abandoned on shutdown", "processed", r.processed)
			return r
		}
		if imp := res.Import; imp != nil {
			r.completed++
			r.episodes += imp.EpisodesImported
			r.plays += imp.PlaysMatched
		}
	}

	return r
}

// runJanitorCycle is the nightly reconcile (PSY-1155). Three independent steps, each
// guarded so one failure doesn't abort the others:
//  1. lifecycle reconcile — active↔dormant by grid membership on schedule-authoritative
//     stations, by episode idle elsewhere (PSY-1348; the active/historical split);
//  2. play_count reconcile — correct denormalized counts against radio_plays;
//  3. backfill straggler sweep — a wider-lookback pass for aired incomplete episodes the
//     hourly post-air sweep missed.
//
// The fast DB reconciles (1, 2) run before the slow provider-HTTP sweep (3), so a
// shutdown mid-sweep still leaves the reconciles done.
func (s *RadioFetchService) runJanitorCycle() {
	now := time.Now()

	promoted, demoted, err := s.radioService.ReconcileShowLifecycle(
		time.Duration(s.janitorDormantDays)*24*time.Hour, now)
	if err != nil {
		s.logger.Error("radio janitor: lifecycle reconcile failed", "error", err)
	}

	pcCorrected, err := s.radioService.ReconcilePlayCounts()
	if err != nil {
		s.logger.Error("radio janitor: play_count reconcile failed", "error", err)
	}

	sweep := s.runBackfillSweep(time.Duration(s.janitorBackfillLookbackDays) * 24 * time.Hour)

	// Escalate stations stuck in a sustained total-fetch outage (PSY-1269). A healthy
	// run advances last_playlist_fetch_at, so a watermark stale beyond the threshold
	// means the station has imported nothing for that long. Scale the threshold to the
	// configured fetch cadence (3× the interval) so a widened RADIO_FETCH_INTERVAL_HOURS
	// doesn't false-escalate healthy stations; the const is the floor.
	outageThreshold := 3 * s.fetchInterval
	if outageThreshold < radioFetchOutageEscalationThreshold {
		outageThreshold = radioFetchOutageEscalationThreshold
	}
	outagesEscalated, err := s.radioService.EscalateStaleFetchOutages(outageThreshold, now)
	if err != nil {
		s.logger.Error("radio janitor: fetch-outage escalation failed", "error", err)
	}

	// Escalate single shows stuck in a consecutive-fetch-failure streak on an
	// otherwise-healthy station (PSY-1274) — the per-show alerting gap the station
	// watermark can't see. The healthy-station guard MUST be on the same clock as the
	// streak (threshold × the fetch interval, unfloored): the 18h-floored station
	// threshold would let a short total-station outage on a fast interval trip every
	// sibling's streak before the station reads as unhealthy (see the function doc).
	streakWindow := time.Duration(radioShowFetchFailureEscalationThreshold) * s.fetchInterval
	if streakWindow <= 0 { // defensive: an unset interval must not turn the guard into "skip everything"
		streakWindow = radioFetchOutageEscalationThreshold
	}
	showOutagesEscalated, err := s.radioService.EscalateShowFetchFailureStreaks(
		radioShowFetchFailureEscalationThreshold, streakWindow, now)
	if err != nil {
		s.logger.Error("radio janitor: show fetch-streak escalation failed", "error", err)
	}

	s.logger.Info("radio janitor cycle complete",
		"dormant_days", s.janitorDormantDays,
		"shows_promoted", promoted,
		"shows_demoted", demoted,
		"play_counts_corrected", pcCorrected,
		"backfill_shows_processed", sweep.processed,
		"backfill_shows_completed", sweep.completed,
		"fetch_outages_escalated", outagesEscalated,
		"show_fetch_outages_escalated", showOutagesEscalated,
		"duration", time.Since(now),
	)
}

// RunFetchCycleNow triggers an immediate fetch cycle (useful for testing/admin).
func (s *RadioFetchService) RunFetchCycleNow() {
	s.runFetchCycle()
}

// RunBackfillCycleNow triggers an immediate post-air backfill sweep (useful for
// testing/admin).
func (s *RadioFetchService) RunBackfillCycleNow() {
	s.runBackfillCycle()
}

// RunJanitorCycleNow triggers an immediate janitor/reconcile cycle (useful for
// testing/admin).
func (s *RadioFetchService) RunJanitorCycleNow() {
	s.runJanitorCycle()
}

// RunAffinityCycleNow triggers an immediate affinity computation (useful for testing/admin).
func (s *RadioFetchService) RunAffinityCycleNow() {
	s.runAffinityCycle()
}

// RunReMatchCycleNow triggers an immediate re-match cycle (useful for testing/admin).
func (s *RadioFetchService) RunReMatchCycleNow() {
	s.runReMatchCycle(context.Background())
}

// RunDiscoverCycleNow triggers an immediate discover cycle (useful for testing/admin).
func (s *RadioFetchService) RunDiscoverCycleNow() {
	s.runDiscoverCycle()
}
