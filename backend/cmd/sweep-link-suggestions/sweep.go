package main

import (
	"context"
	"fmt"
	"log"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// linklessArtist is the minimal projection of a sweep target: the id + name are
// all DiscoverMusic needs (it resolves regions from the id, searches MB on the
// name).
type linklessArtist struct {
	ID   uint
	Name string
}

// SweepReport is the tally a sweep run produces. ArtistsScanned is the count of
// link-less artists processed; SuggestionsFound is the total candidate count
// across all of them (what a live run WOULD upsert); SuggestionsWritten /
// SuggestionsSkipped split the candidates of SUCCESSFULLY-upserted artists into
// rows newly inserted vs rows skipped because the (artist, platform, url) already
// existed (already queued/reviewed). SuggestionsWritten being 0 on a re-run is
// what makes idempotency observable. ArtistsWithCandidates / ArtistsNoCandidates
// split the scan by yield. Errors collects per-artist discovery/upsert failures
// (non-fatal — one artist failing never aborts the sweep); a failed artist's
// candidates count toward SuggestionsFound but toward NEITHER Written nor
// Skipped, so the two write-side counters stay honest about what actually
// happened.
type SweepReport struct {
	ArtistsScanned        int
	ArtistsWithCandidates int
	ArtistsNoCandidates   int
	SuggestionsFound      int
	SuggestionsWritten    int
	SuggestionsSkipped    int
	Errors                []string
}

// discoverer is the slice of DiscoverMusicService the sweep needs. An interface
// so the sweep logic can be unit-tested without a live MusicBrainz client (the
// real service IS the production discoverer; the cmd wires exactly one of them).
type discoverer interface {
	DiscoverMusic(ctx context.Context, artistID uint, artistName string) (*contracts.DiscoverMusicResult, error)
}

// upserter is the slice of LinkSuggestionService the sweep needs: the write-side
// primitive that owns the artist_link_suggestions ON CONFLICT mechanics. An
// interface keeps the sweep loosely coupled and unit-testable.
type upserter interface {
	UpsertSuggestions(artistID uint, candidates []contracts.MusicLinkCandidate) (int, error)
}

// RunSweep walks every link-less artist (bandcamp_embed_url IS NULL AND spotify
// IS NULL) STRICTLY SEQUENTIALLY through the ONE provided discoverer, and (when
// dryRun is false) upserts each discovered candidate into artist_link_suggestions
// as a pending row via the suggestion store's UpsertSuggestions.
//
// SEQUENTIAL BY DESIGN (PSY-1206 / PSY-1208): there is NO goroutine pool over
// artists. The discoverer wraps ONE shared MusicBrainzClient whose mutex
// serializes a ~1 req/s throttle; running artists in parallel would let multiple
// in-flight MB requests defeat that throttle and risk an MB IP block. The
// service's internal liveness probes are already bounded (8 concurrent per
// artist); sequential artists keep total MB pressure at ~1 req/s. Do NOT add
// parallelism here.
//
// IDEMPOTENT / RESUMABLE: UpsertSuggestions is ON CONFLICT (artist_id, platform,
// url) DO NOTHING, so a re-run inserts only genuinely new candidates and NEVER
// resurrects an already-reviewed (accepted/rejected) row — the conflict target is
// the row's identity, not its status. The target query also drops any artist that
// has since gained a link. A run can be interrupted and re-run safely.
//
// Resumability is for CORRECTNESS, not cost: a re-run still re-discovers EVERY
// link-less artist via MusicBrainz (only the DB insert is conflict-skipped, not
// the rate-limited MB lookup), so resuming near the end of the backlog re-spends
// most of the MB traffic. This is deliberate — re-discovery lets a re-sweep pick
// up links MB has newly published for an artist — and acceptable because the run
// is a one-shot ops task, not a hot path. (An "AND NOT EXISTS suggestion" filter
// would make resume cheap but would stop re-discovering artists whose prior
// candidates were all rejected; that tradeoff is an open ops decision, see PR.)
//
// ctx bounds the whole sweep: cancelling it stops the loop (checked before AND
// after each artist's discovery) and cancels any in-flight MB/liveness work for
// the current artist via the shared context.
func RunSweep(ctx context.Context, db *gorm.DB, disc discoverer, store upserter, dryRun bool) (*SweepReport, error) {
	report := &SweepReport{}

	artists, err := linklessArtists(db)
	if err != nil {
		return nil, fmt.Errorf("query link-less artists: %w", err)
	}

	for _, a := range artists {
		if ctx.Err() != nil {
			// Cancelled (signal / timeout): stop cleanly and return what we have.
			report.Errors = append(report.Errors,
				fmt.Sprintf("sweep cancelled after %d artists: %v", report.ArtistsScanned, ctx.Err()))
			break
		}
		report.ArtistsScanned++

		result, err := disc.DiscoverMusic(ctx, a.ID, a.Name)
		if err != nil {
			// One artist's MB discovery failing must not sink the whole sweep —
			// record it and move on. The run stays resumable: a re-run retries
			// this artist.
			report.Errors = append(report.Errors,
				fmt.Sprintf("artist %d %q: discover: %v", a.ID, a.Name, err))
			continue
		}

		// Re-check cancellation AFTER discovery: a SIGINT that lands DURING
		// DiscoverMusic makes it return a PARTIAL candidate set with a nil error
		// (it breaks its own loop on ctx cancellation). Upserting that partial set
		// is harmless for the idempotent queue, but skipping it keeps the
		// "stops cleanly between artists" contract literally true — the in-flight
		// artist is left entirely for the resuming run.
		if ctx.Err() != nil {
			report.Errors = append(report.Errors,
				fmt.Sprintf("sweep cancelled during artist %d %q: %v", a.ID, a.Name, ctx.Err()))
			break
		}

		if len(result.Candidates) == 0 {
			report.ArtistsNoCandidates++
			continue
		}
		report.ArtistsWithCandidates++
		report.SuggestionsFound += len(result.Candidates)

		if dryRun {
			// Heartbeat so a multi-hour run (~1 req/s over the backlog) is visibly
			// progressing rather than indistinguishable from a hang.
			log.Printf("[%d/%d] artist %d %q: %d candidate(s) (dry-run)",
				report.ArtistsScanned, len(artists), a.ID, a.Name, len(result.Candidates))
			continue
		}

		written, err := store.UpsertSuggestions(a.ID, result.Candidates)
		if err != nil {
			report.Errors = append(report.Errors,
				fmt.Sprintf("artist %d %q: upsert: %v", a.ID, a.Name, err))
			continue
		}
		report.SuggestionsWritten += written
		// Only count the skipped (already-present) candidates of an artist whose
		// upsert SUCCEEDED — a failed upsert's candidates were neither written nor
		// "already present" (see SweepReport.Errors).
		report.SuggestionsSkipped += len(result.Candidates) - written
		log.Printf("[%d/%d] artist %d %q: %d candidate(s), %d written",
			report.ArtistsScanned, len(artists), a.ID, a.Name, len(result.Candidates), written)
	}

	return report, nil
}

// linklessArtists returns the (id, name) of every artist with NO music-platform
// link — exactly the bulk-backfill target set (PSY-1206): bandcamp_embed_url IS
// NULL AND spotify IS NULL. Ordered by id so a run's progress (and an
// interrupted-then-resumed run) is deterministic.
//
// CONVENTION NOTE: the cmd↔service split here is the dedup-shows shape (the cmd
// owns the loop + this target query; the service owns the per-item mutation,
// LinkSuggestionService.UpsertSuggestions). The OTHER artist-backfill cmd,
// backfill-artist-bandcamp-embeds, pushes its whole query+loop INTO a service
// (catalog.BackfillArtistBandcampEmbeds) — so the repo has both shapes and this
// one deliberately follows dedup-shows. The factor that tips it: the loop body is
// a network call (MusicBrainz) bounded by a shared throttle + signal cancellation,
// which is orchestration the cmd already owns; pushing it into a service buys no
// reuse (no other caller wants a multi-hour MB sweep) at the cost of threading
// ctx/throttle/signal plumbing through the service layer.
func linklessArtists(db *gorm.DB) ([]linklessArtist, error) {
	var out []linklessArtist
	err := db.Model(&catalogm.Artist{}).
		Where("bandcamp_embed_url IS NULL AND spotify IS NULL").
		Order("id").
		Select("id, name").
		Scan(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}
