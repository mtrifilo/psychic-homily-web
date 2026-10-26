package indexnow

import (
	"context"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// runLease is how long a run's claim stays valid before another instance may
// treat the run as abandoned. A run is one selection plus, per batch, at most
// two POSTs of requestTimeout and one retry backoff, about a minute, so the
// lease covers a backlog of over twenty batches.
const runLease = 30 * time.Minute

// ChangeSource selects the entries whose sitemap lastmod is later than since.
// catalog.SitemapService implements it with the sitemap's own definitions.
type ChangeSource interface {
	EntriesChangedSince(ctx context.Context, since time.Time) (*contracts.SitemapEntries, error)
}

// markStore persists the high-water mark: the database time at which the last
// successful run began.
type markStore interface {
	// Begin returns the database time now, which becomes the mark if this run
	// succeeds, and the current mark; hasMark is false when no run has
	// succeeded yet.
	Begin(ctx context.Context) (start, mark time.Time, hasMark bool, err error)
	// Advance stores start as the mark.
	Advance(ctx context.Context, start time.Time) error
}

// familyPaths maps each submitted family to the path its pages live under,
// the same prefixes the frontend's sitemap builds each family's URLs with.
var familyPaths = []struct {
	path    string
	entries func(*contracts.SitemapEntries) []contracts.SitemapEntry
}{
	{"/shows/", func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Shows }},
	{"/artists/", func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Artists }},
	{"/venues/", func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Venues }},
	{"/releases/", func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Releases }},
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

// Start begins the loop. Its interval is below the run-state persistence
// threshold, so the loop passes the default run store explicitly to keep a
// background_service_runs row.
func (p *Pinger) Start(ctx context.Context) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		shared.RunScheduledLoop(ctx, shared.LoopConfig{
			Name:     LoopName,
			Interval: p.interval,
			StopCh:   p.stopCh,
			Store:    shared.DefaultRunStore(),
			RunLease: runLease,
		}, p.runCycle)
	}()
	p.logger.Info("indexnow pinger started",
		"interval", p.interval,
		"host", p.cfg.Host,
		"endpoint", p.cfg.Endpoint,
	)
}

// Stop stops the loop and waits for an in-flight run to return.
func (p *Pinger) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("indexnow pinger stopped")
}

// runCycle runs once and writes one log line and the run row's count.
func (p *Pinger) runCycle(ctx context.Context) {
	stats, err := p.run(ctx)
	attrs := []any{
		"selected", stats.selected,
		"submitted", stats.submitted,
		"batches", stats.batches,
		"status", stats.status,
		"first_run", stats.firstRun,
	}
	if err != nil {
		shared.LogCycleFailure(p.logger, "indexnow: run failed; mark not advanced", err, attrs...)
	} else {
		p.logger.Info("indexnow: run complete", attrs...)
	}
	shared.ReportCycle(ctx, stats.submitted, err)
}

// run submits every URL whose lastmod is later than the mark, in batches of at
// most maxURLsPerRequest, and advances the mark to this run's start only when
// every batch succeeded. A run that finds no mark submits nothing and records
// one.
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
func (p *Pinger) urlsFor(entries *contracts.SitemapEntries) []string {
	var urls []string
	for _, family := range familyPaths {
		for _, entry := range family.entries(entries) {
			urls = append(urls, p.cfg.SiteOrigin+family.path+url.PathEscape(entry.Slug))
		}
	}
	return urls
}
