package imageenrich

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/mbadapter"
	"psychic-homily-backend/internal/services/pipeline"
)

// Enricher is the shared image-enrichment capability that BOTH triggers drive: the
// Phase-A sweep (its background ticker) and the PSY-1247 outbox poller (its
// on-create queue). It owns the actual provider lookups (runPhotoEnricher /
// runCoverEnricher), the no-result memo stamp (stampAttempted, used by the sweep),
// and — critically — the ONE process-wide MusicBrainz client (PSY-1208). The
// container constructs exactly one Enricher and injects it into both, so the
// single-throttle invariant lives in one place and neither trigger reaches into the
// other's internals (PSY-1266 — previously the poller held a *ImageEnrichmentSweep
// and called its unexported fields).
type Enricher struct {
	db           *gorm.DB
	mb           *pipeline.MusicBrainzClient // shared (PSY-1208) — do not replace with a fresh client
	discogsToken string
	logger       *slog.Logger

	// EnrichPhotos / EnrichCovers run the actual provider lookups for a bounded id
	// batch. They are fields so tests can substitute fakes and exercise the callers'
	// selection / queue / memo logic without real MusicBrainz/Wikidata/Commons/CAA
	// traffic. They default to runPhotoEnricher / runCoverEnricher.
	EnrichPhotos func(ctx context.Context, ids []uint) error
	EnrichCovers func(ctx context.Context, ids []uint) error
}

// NewEnricher constructs the shared enricher. mbClient MUST be the shared
// process-wide MusicBrainz client (PSY-1208) — passing a fresh one would double the
// MB request rate. discogsToken may be empty (CAA-only covers).
func NewEnricher(database *gorm.DB, mbClient *pipeline.MusicBrainzClient, discogsToken string) *Enricher {
	if database == nil {
		database = db.GetDB()
	}
	if mbClient == nil {
		mbClient = pipeline.NewMusicBrainzClient()
	}
	e := &Enricher{
		db:           database,
		mb:           mbClient,
		discogsToken: discogsToken,
		logger:       slog.Default(),
	}
	e.EnrichPhotos = e.runPhotoEnricher
	e.EnrichCovers = e.runCoverEnricher
	return e
}

// stampAttempted marks the batch as attempted now, so the no-result tail isn't
// re-queried until the re-attempt window elapses. Uses Table (not Model) so the
// bookkeeping write does not bump updated_at. (Used by the sweep; the outbox
// deliberately does not stamp — PSY-1265.)
func (e *Enricher) stampAttempted(ctx context.Context, table string, ids []uint) error {
	return e.db.WithContext(ctx).
		Table(table).
		Where("id IN ?", ids).
		Update("image_enrich_attempted_at", time.Now()).Error
}

// runPhotoEnricher resolves Commons photos for the given artist ids using the
// shared MB client + fresh Wikidata/Commons clients (closed after the batch).
func (e *Enricher) runPhotoEnricher(ctx context.Context, ids []uint) error {
	wd := catalog.NewWikidataClient()
	defer wd.Close()
	commons := catalog.NewCommonsClient()
	defer commons.Close()

	report, err := catalog.BackfillCommonsPhotos(ctx, e.db, mbadapter.NewArtistAdapter(e.mb), wd, commons, catalog.CommonsEnrichOptions{IDs: ids})
	if report != nil {
		e.logger.Info("image-enrich photos",
			"scanned", report.ArtistsScanned, "matched", report.ArtistsMatched,
			"skipped", report.ArtistsSkipped, "errors", report.ArtistErrors)
	}
	return err
}

// runCoverEnricher resolves CAA/Discogs covers for the given release ids using the
// shared MB client + a fresh CAA client (+ Discogs when a token is set).
func (e *Enricher) runCoverEnricher(ctx context.Context, ids []uint) error {
	caa := catalog.NewCoverArtArchiveClient()
	defer caa.Close()

	opts := catalog.CoverArtEnrichOptions{IDs: ids}
	var (
		report *catalog.CoverArtEnrichReport
		err    error
	)
	// Pass an untyped nil when no token — a typed (*catalog.DiscogsClient)(nil)
	// stored in the interface is non-nil and would panic on first call (mirrors the
	// cmd).
	if e.discogsToken != "" {
		discogs := catalog.NewDiscogsClient(e.discogsToken)
		defer discogs.Close()
		report, err = catalog.BackfillCoverArt(ctx, e.db, mbadapter.NewReleaseAdapter(e.mb), caa, discogs, opts)
	} else {
		report, err = catalog.BackfillCoverArt(ctx, e.db, mbadapter.NewReleaseAdapter(e.mb), caa, nil, opts)
	}
	if report != nil {
		e.logger.Info("image-enrich covers",
			"scanned", report.ReleasesScanned,
			"matched_caa", report.ReleasesMatchedCAA, "matched_discogs", report.ReleasesMatchedDiscogs,
			"skipped", report.ReleasesSkipped, "errors", report.ReleaseErrors)
	}
	return err
}
