package indexnow

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// runLease is how long a run's claim stays valid before another instance may
// treat the run as abandoned, and how long runCycle lets a run last. A run is
// one selection plus, per batch, at most two POSTs of requestTimeout and one
// retry backoff, about a minute, so it covers a backlog of over twenty batches.
const runLease = 30 * time.Minute

// ChangeSource selects the entries whose sitemap lastmod is later than since.
// catalog.SitemapService implements it with the sitemap's own definitions.
type ChangeSource interface {
	EntriesChangedSince(ctx context.Context, since time.Time) (*contracts.SitemapEntries, error)
}

// markStore supplies the high-water mark: an instant no later than the start
// of the last successful run. A store either keeps the mark itself, recording
// start on Advance, or derives it from the loop's run row, which the loop
// stamps only when runCycle reports no error; runRowMarks is the second kind,
// and its Advance writes nothing.
type markStore interface {
	// Begin returns the database time now, which a store that keeps the mark
	// records on Advance, and the current mark; hasMark is false when no run
	// has succeeded yet.
	Begin(ctx context.Context) (start, mark time.Time, hasMark bool, err error)
	// Advance is called once per successful run, with that run's start.
	Advance(ctx context.Context, start time.Time) error
}

// familyPaths maps each submitted sitemap family to the path its pages live
// under: the family's prefix in FAMILY_URL_PREFIXES in
// frontend/app/sitemap-shards.ts, which builds the sitemap's own URLs, plus a
// slash. TestFamilyPathsMatchTheFrontendSitemap pins the two together.
var familyPaths = []struct {
	family  string
	path    string
	entries func(*contracts.SitemapEntries) []contracts.SitemapEntry
}{
	{"shows", "/shows/", func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Shows }},
	{"artists", "/artists/", func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Artists }},
	{"venues", "/venues/", func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Venues }},
	{"releases", "/releases/", func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Releases }},
}

// runStats is what one run did, for its log line and its run row.
type runStats struct {
	selected  int
	submitted int
	batches   int
	status    int
	// firstRun is a run that found no mark: it records one and submits
	// nothing.
	firstRun bool
}

// Pinger submits changed public URLs to IndexNow on a schedule.
type Pinger struct {
	cfg       Config
	source    ChangeSource
	marks     markStore
	submitter *submitter
	interval  time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// New returns the pinger over database and source, configured from the
// environment and frontendURL, or nil and the reason it is off; see
// ConfigFromEnv.
func New(database *gorm.DB, source ChangeSource, frontendURL string) (*Pinger, string) {
	cfg, enabled, reason := ConfigFromEnv(frontendURL, os.Getenv)
	if !enabled {
		return nil, reason
	}
	return newPinger(cfg, source, runRowMarks{db: database}, slog.Default()), ""
}

func newPinger(cfg Config, source ChangeSource, marks markStore, logger *slog.Logger) *Pinger {
	return &Pinger{
		cfg:       cfg,
		source:    source,
		marks:     marks,
		submitter: newSubmitter(cfg.Endpoint, logger),
		interval:  defaultInterval,
		stopCh:    make(chan struct{}),
		logger:    logger,
	}
}

// Start begins the loop.
func (p *Pinger) Start(ctx context.Context) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		shared.RunScheduledLoop(ctx, p.loopConfig(), p.runCycle)
	}()
	p.logger.Info("indexnow pinger started",
		"interval", p.interval,
		"host", p.cfg.Host,
		"endpoint", p.cfg.Endpoint,
	)
}

// loopConfig is the pinger's schedule. Store is set explicitly because the
// interval is below the run-state persistence threshold, under which the loop
// keeps no row by default, and the row's last_success_at IS the high-water
// mark (runRowMarks): without a store the mark never moves, so every run would
// either find no mark and submit nothing or resubmit everything since a
// frozen one.
func (p *Pinger) loopConfig() shared.LoopConfig {
	return shared.LoopConfig{
		Name:     LoopName,
		Interval: p.interval,
		StopCh:   p.stopCh,
		Store:    shared.DefaultRunStore(),
		RunLease: runLease,
	}
}

// Stop stops the loop and waits for an in-flight run to return.
func (p *Pinger) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("indexnow pinger stopped")
}

// runCycle runs once, for at most runLease, and writes one log line and the
// run row. The row's count is the submitted URLs; a failed run's error text
// also carries the selected count, the batches tried and the last status.
// Reporting the run's error is what keeps the loop from stamping
// last_success_at, and so from advancing the mark, after a failed run.
func (p *Pinger) runCycle(ctx context.Context) {
	runCtx, cancel := context.WithTimeout(ctx, runLease)
	defer cancel()
	stats, err := p.run(runCtx)
	attrs := []any{
		"selected", stats.selected,
		"submitted", stats.submitted,
		"batches", stats.batches,
		"status", stats.status,
		"first_run", stats.firstRun,
	}
	switch {
	case err != nil:
		shared.LogCycleFailure(p.logger, "indexnow: run failed; mark not advanced", err, attrs...)
	case stats.firstRun:
		p.logger.Info("indexnow: first run recorded a mark and submitted nothing; pages changed before it reach search engines through the sitemap", attrs...)
	default:
		p.logger.Info("indexnow: run complete", attrs...)
	}
	if err != nil {
		err = fmt.Errorf("%w (selected %d, submitted %d, batches %d, last status %d)",
			err, stats.selected, stats.submitted, stats.batches, stats.status)
	}
	shared.ReportCycle(ctx, stats.submitted, err)
}

// run submits every URL whose lastmod is later than the mark, in batches of at
// most maxURLsPerRequest, and returns nil, calling Advance, only when every
// batch succeeded. A run that finds no mark submits nothing and succeeds, so
// the next run has one.
func (p *Pinger) run(ctx context.Context) (runStats, error) {
	var stats runStats
	start, mark, hasMark, err := p.marks.Begin(ctx)
	if err != nil {
		return stats, err
	}
	if !hasMark {
		stats.firstRun = true
		return stats, p.marks.Advance(ctx, start)
	}

	entries, err := p.source.EntriesChangedSince(ctx, mark)
	if err != nil {
		return stats, err
	}
	urls := p.urlsFor(entries)
	stats.selected = len(urls)

	for len(urls) > 0 {
		n := min(len(urls), maxURLsPerRequest)
		batch := urls[:n]
		urls = urls[n:]
		status, err := p.submitter.submit(ctx, submission{
			Host:        p.cfg.Host,
			Key:         p.cfg.Key,
			KeyLocation: p.cfg.KeyLocation(),
			URLList:     batch,
		})
		stats.batches++
		stats.status = status
		if err != nil {
			return stats, err
		}
		stats.submitted += len(batch)
	}
	return stats, p.marks.Advance(ctx, start)
}

// urlsFor builds the absolute URL of every entry, in familyPaths order.
// PathEscape leaves a generated slug, which is lowercase letters, digits and
// dashes, unchanged, so such a URL is byte-identical to the sitemap's; it only
// escapes a slug outside that alphabet.
func (p *Pinger) urlsFor(entries *contracts.SitemapEntries) []string {
	var urls []string
	for _, family := range familyPaths {
		for _, entry := range family.entries(entries) {
			urls = append(urls, p.cfg.SiteOrigin+family.path+url.PathEscape(entry.Slug))
		}
	}
	return urls
}
