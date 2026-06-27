package catalog

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/services/shared"
)

// Ongoing image-enrichment sweep (PSY-1246) — the Phase-A "safety net" of the
// hybrid trigger model decided in PSY-1245. A slow background ticker runs the
// shipped fill-when-empty enrichers (BackfillCommonsPhotos for artist photos,
// BackfillCoverArt for release covers) over entities that still have no image, so
// coverage stays current as the catalog grows regardless of how an entity was
// added. (Phase B — prompt on-create enqueue — is PSY-1247.)
//
// It does NOT change what users see: enrichment only populates data; prod display
// stays gated on PSY-1242.
//
// Two design points carry the weight:
//
//   - SHARED MusicBrainz client (PSY-1208). Both enrichers hit MusicBrainz, which
//     blocks for exceeding ~1 req/s/IP. The sweep MUST reuse the one process-wide
//     *pipeline.MusicBrainzClient (injected) so all MB traffic stays under a single
//     mutex-serialized throttle — a second client would double the rate and trip
//     MB's sticky 503 penalty.
//   - No-result memo. Fill-when-empty keys only on an empty image column, so the
//     large imageless long tail (no provider match) would be re-queried every
//     cycle. The sweep stamps image_enrich_attempted_at on each batch and skips
//     rows attempted within the re-attempt window, so a bounded batch converges
//     instead of re-hammering the providers.

const (
	defaultImageEnrichSweepInterval = 24 * time.Hour
	defaultImageEnrichSweepBatch    = 50
	defaultImageEnrichReattempt     = 90 * 24 * time.Hour
)

// ImageEnrichmentSweep is a background ticker service (mirrors CleanupService /
// EnrichmentWorker) that fills missing artist photos + release covers.
type ImageEnrichmentSweep struct {
	db           *gorm.DB
	mb           *pipeline.MusicBrainzClient // shared (PSY-1208) — do not replace with a fresh client
	discogsToken string

	interval  time.Duration
	batch     int
	reattempt time.Duration

	// enrichPhotos / enrichCovers run the actual provider lookups for a bounded id
	// batch. They are fields so tests can substitute fakes and exercise the
	// memo/selection logic without real MusicBrainz/Wikidata/Commons traffic.
	enrichPhotos func(ctx context.Context, ids []uint) error
	enrichCovers func(ctx context.Context, ids []uint) error

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewImageEnrichmentSweep constructs the sweep. mbClient MUST be the shared
// process-wide MusicBrainz client (PSY-1208) — passing a fresh one would double
// the MB request rate. discogsToken may be empty (CAA-only covers).
func NewImageEnrichmentSweep(database *gorm.DB, mbClient *pipeline.MusicBrainzClient, discogsToken string) *ImageEnrichmentSweep {
	if database == nil {
		database = db.GetDB()
	}
	if mbClient == nil {
		mbClient = pipeline.NewMusicBrainzClient()
	}

	s := &ImageEnrichmentSweep{
		db:           database,
		mb:           mbClient,
		discogsToken: discogsToken,
		interval:     sweepEnvDuration("IMAGE_ENRICH_SWEEP_INTERVAL_HOURS", time.Hour, defaultImageEnrichSweepInterval),
		batch:        sweepEnvInt("IMAGE_ENRICH_SWEEP_BATCH", defaultImageEnrichSweepBatch),
		reattempt:    sweepEnvDuration("IMAGE_ENRICH_SWEEP_REATTEMPT_DAYS", 24*time.Hour, defaultImageEnrichReattempt),
		stopCh:       make(chan struct{}),
		logger:       slog.Default(),
	}
	s.enrichPhotos = s.runPhotoEnricher
	s.enrichCovers = s.runCoverEnricher
	return s
}

// Start begins the background sweep. No startup cycle (runImmediately=false): a
// server restart shouldn't kick off provider traffic; the first sweep fires one
// interval in. Mirrors EnrichmentWorker.
func (s *ImageEnrichmentSweep) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		shared.RunTickerLoop(ctx, "image_enrich_sweep", s.interval, s.stopCh, false, s.runCycle)
	}()
	s.logger.Info("image enrichment sweep started",
		"interval", s.interval, "batch", s.batch, "reattempt", s.reattempt)
}

// Stop gracefully stops the sweep.
func (s *ImageEnrichmentSweep) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("image enrichment sweep stopped")
}

// RunSweepNow runs one cycle immediately (tests / manual trigger).
func (s *ImageEnrichmentSweep) RunSweepNow(ctx context.Context) {
	s.runCycle(ctx)
}

// runCycle sweeps photos then covers sequentially, so the two share the MB
// throttle without overlapping.
func (s *ImageEnrichmentSweep) runCycle(ctx context.Context) {
	s.sweep(ctx, "artists", s.enrichPhotos)
	if ctx.Err() != nil {
		return
	}
	s.sweep(ctx, "releases", s.enrichCovers)
}

// sweep selects a bounded, memo-filtered batch of image-less entities from
// `table`, stamps their attempt timestamp, then runs `enrich` over those ids.
//
// Stamp-before-enrich is deliberate: a row is marked attempted even if the enrich
// step errors, so a poison row can't wedge the sweep (it waits one re-attempt
// window before a retry). Hard infra failures are rare and the window retries
// them.
func (s *ImageEnrichmentSweep) sweep(ctx context.Context, table string, enrich func(context.Context, []uint) error) {
	ids, err := s.selectBatch(ctx, table)
	if err != nil {
		s.logger.Error("image-enrich sweep: select failed", "table", table, "error", err)
		return
	}
	if len(ids) == 0 {
		return
	}

	if err := s.stampAttempted(ctx, table, ids); err != nil {
		s.logger.Error("image-enrich sweep: stamp failed", "table", table, "error", err)
		return
	}

	if err := enrich(ctx, ids); err != nil {
		s.logger.Error("image-enrich sweep: enrich failed", "table", table, "count", len(ids), "error", err)
		return
	}
	s.logger.Info("image-enrich sweep batch done", "table", table, "count", len(ids))
}

// imageColumn is the reference column whose emptiness marks an entity as needing
// enrichment, per table.
func imageColumn(table string) string {
	if table == "releases" {
		return "cover_art_url"
	}
	return "image_url"
}

// selectBatch returns up to `batch` entity ids that have no image and were not
// attempted within the re-attempt window, oldest-attempt first (NULLs — never
// tried — first) so brand-new and stalest rows are picked before recently
// re-checked ones.
func (s *ImageEnrichmentSweep) selectBatch(ctx context.Context, table string) ([]uint, error) {
	cutoff := time.Now().Add(-s.reattempt)
	col := imageColumn(table)
	var ids []uint
	err := s.db.WithContext(ctx).
		Table(table).
		// Explicit parens keep the OR-group local: this is the one query where an
		// OR clause is AND-combined with a second OR clause, so the grouping is
		// load-bearing (don't rely on GORM's per-Where auto-parenthesization alone).
		Where("("+col+" IS NULL OR "+col+" = '')").
		Where("image_enrich_attempted_at IS NULL OR image_enrich_attempted_at < ?", cutoff).
		Order("image_enrich_attempted_at ASC NULLS FIRST").
		Order("id ASC").
		Limit(s.batch).
		Pluck("id", &ids).Error
	return ids, err
}

// stampAttempted marks the batch as attempted now, so the no-result tail isn't
// re-queried until the re-attempt window elapses. Uses Table (not Model) so the
// bookkeeping write does not bump updated_at.
func (s *ImageEnrichmentSweep) stampAttempted(ctx context.Context, table string, ids []uint) error {
	return s.db.WithContext(ctx).
		Table(table).
		Where("id IN ?", ids).
		Update("image_enrich_attempted_at", time.Now()).Error
}

// runPhotoEnricher resolves Commons photos for the given artist ids using the
// shared MB client + fresh Wikidata/Commons clients (closed after the batch).
func (s *ImageEnrichmentSweep) runPhotoEnricher(ctx context.Context, ids []uint) error {
	wd := NewWikidataClient()
	defer wd.Close()
	commons := NewCommonsClient()
	defer commons.Close()

	report, err := BackfillCommonsPhotos(ctx, s.db, mbArtistEnrichAdapter{client: s.mb}, wd, commons, CommonsEnrichOptions{IDs: ids})
	if report != nil {
		s.logger.Info("image-enrich sweep photos",
			"scanned", report.ArtistsScanned, "matched", report.ArtistsMatched,
			"skipped", report.ArtistsSkipped, "errors", report.ArtistErrors)
	}
	return err
}

// runCoverEnricher resolves CAA/Discogs covers for the given release ids using
// the shared MB client + a fresh CAA client (+ Discogs when a token is set).
func (s *ImageEnrichmentSweep) runCoverEnricher(ctx context.Context, ids []uint) error {
	caa := NewCoverArtArchiveClient()
	defer caa.Close()

	opts := CoverArtEnrichOptions{IDs: ids}
	var (
		report *CoverArtEnrichReport
		err    error
	)
	// Pass an untyped nil when no token — a typed (*DiscogsClient)(nil) stored in
	// the interface is non-nil and would panic on first call (mirrors the cmd).
	if s.discogsToken != "" {
		discogs := NewDiscogsClient(s.discogsToken)
		defer discogs.Close()
		report, err = BackfillCoverArt(ctx, s.db, mbReleaseEnrichAdapter{client: s.mb}, caa, discogs, opts)
	} else {
		report, err = BackfillCoverArt(ctx, s.db, mbReleaseEnrichAdapter{client: s.mb}, caa, nil, opts)
	}
	if report != nil {
		s.logger.Info("image-enrich sweep covers",
			"scanned", report.ReleasesScanned,
			"matched_caa", report.ReleasesMatchedCAA, "matched_discogs", report.ReleasesMatchedDiscogs,
			"skipped", report.ReleasesSkipped, "errors", report.ReleaseErrors)
	}
	return err
}

// --- MusicBrainz adapters -------------------------------------------------
// These mirror the cmd-local adapters in cmd/backfill-{commons-photos,cover-art}
// (kept there for the standalone CLIs). Consolidating all three into one shared
// helper is tracked as PSY-1248; for now the sweep carries its own so this change
// does not touch the shipped backfill commands.

type mbArtistEnrichAdapter struct {
	client *pipeline.MusicBrainzClient
}

func (a mbArtistEnrichAdapter) SearchArtistCandidates(ctx context.Context, name string) ([]MBArtistCandidate, error) {
	raw, err := a.client.SearchArtistCandidates(ctx, name)
	if err != nil {
		return nil, err
	}
	out := make([]MBArtistCandidate, 0, len(raw))
	for _, r := range raw {
		out = append(out, MBArtistCandidate{MBID: r.ID, Name: r.Name})
	}
	return out, nil
}

func (a mbArtistEnrichAdapter) LookupArtistURLs(ctx context.Context, mbid string) ([]string, error) {
	rels, err := a.client.LookupArtistURLRelations(ctx, mbid)
	if err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(rels))
	for _, r := range rels {
		if r.URL.Resource != "" {
			urls = append(urls, r.URL.Resource)
		}
	}
	return urls, nil
}

type mbReleaseEnrichAdapter struct {
	client *pipeline.MusicBrainzClient
}

func (a mbReleaseEnrichAdapter) SearchReleaseGroups(ctx context.Context, artist, title string, limit int) ([]MBReleaseGroupCandidate, error) {
	raw, err := a.client.SearchReleaseGroups(ctx, artist, title, limit)
	if err != nil {
		return nil, err
	}
	out := make([]MBReleaseGroupCandidate, 0, len(raw))
	for _, rg := range raw {
		out = append(out, MBReleaseGroupCandidate{
			MBID:             rg.ID,
			Title:            rg.Title,
			ArtistNames:      flattenMBArtistNames(rg.ArtistCredit),
			FirstReleaseDate: rg.FirstReleaseDate,
		})
	}
	return out, nil
}

func flattenMBArtistNames(credits []pipeline.MBArtistCredit) []string {
	names := make([]string, 0, len(credits)*2)
	for _, ac := range credits {
		if ac.Name != "" {
			names = append(names, ac.Name)
		}
		if ac.Artist.Name != "" && ac.Artist.Name != ac.Name {
			names = append(names, ac.Artist.Name)
		}
	}
	return names
}

// --- env helpers ----------------------------------------------------------

func sweepEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func sweepEnvDuration(key string, unit, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * unit
		}
	}
	return def
}
