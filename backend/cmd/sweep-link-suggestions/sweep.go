package main

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
// across all of them (what a live run WOULD upsert); SuggestionsWritten is the
// count of rows actually INSERTED (post-ON-CONFLICT, so it excludes
// already-present rows — this is what makes idempotency observable: a second run
// reports 0 written). ArtistsWithCandidates / ArtistsNoCandidates split the scan
// by yield. Errors collects per-artist discovery/upsert failures (non-fatal —
// one artist failing never aborts the sweep).
type SweepReport struct {
	ArtistsScanned        int
	ArtistsWithCandidates int
	ArtistsNoCandidates   int
	SuggestionsFound      int
	SuggestionsWritten    int
	Errors                []string
}

// discoverer is the slice of DiscoverMusicService the sweep needs. An interface
// so the sweep logic can be unit-tested without a live MusicBrainz client (the
// real service IS the production discoverer; the cmd wires exactly one of them).
type discoverer interface {
	DiscoverMusic(ctx context.Context, artistID uint, artistName string) (*contracts.DiscoverMusicResult, error)
}

// RunSweep walks every link-less artist (bandcamp_embed_url IS NULL AND spotify
// IS NULL) STRICTLY SEQUENTIALLY through the ONE provided discoverer, and (when
// dryRun is false) upserts each discovered candidate into artist_link_suggestions
// as a pending row.
//
// SEQUENTIAL BY DESIGN (PSY-1206 / PSY-1208): there is NO goroutine pool over
// artists. The discoverer wraps ONE shared MusicBrainzClient whose mutex
// serializes a ~1 req/s throttle; running artists in parallel would let multiple
// in-flight MB requests defeat that throttle and risk an MB IP block. The
// service's internal liveness probes are already bounded (8 concurrent per
// artist); sequential artists keep total MB pressure at ~1 req/s. Do NOT add
// parallelism here.
//
// IDEMPOTENT / RESUMABLE: the upsert is ON CONFLICT (artist_id, platform, url) DO
// NOTHING, so a re-run inserts only genuinely new candidates and NEVER resurrects
// an already-reviewed (accepted/rejected) row — the conflict target is the row's
// identity, not its status. The target query also drops any artist that has since
// gained a link. A run can be interrupted and re-run safely.
//
// ctx bounds the whole sweep: cancelling it stops the loop between artists and
// cancels any in-flight MB/liveness work for the current artist.
func RunSweep(ctx context.Context, db *gorm.DB, disc discoverer, dryRun bool) (*SweepReport, error) {
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

		if len(result.Candidates) == 0 {
			report.ArtistsNoCandidates++
			continue
		}
		report.ArtistsWithCandidates++
		report.SuggestionsFound += len(result.Candidates)

		if dryRun {
			continue
		}

		written, err := upsertSuggestions(db, a.ID, result.Candidates)
		if err != nil {
			report.Errors = append(report.Errors,
				fmt.Sprintf("artist %d %q: upsert: %v", a.ID, a.Name, err))
			continue
		}
		report.SuggestionsWritten += written
	}

	return report, nil
}

// linklessArtists returns the (id, name) of every artist with NO music-platform
// link — exactly the bulk-backfill target set (PSY-1206): bandcamp_embed_url IS
// NULL AND spotify IS NULL. Ordered by id so a run's progress (and an
// interrupted-then-resumed run) is deterministic.
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

// upsertSuggestions inserts each candidate as a pending artist_link_suggestions
// row, skipping any (artist_id, platform, url) that already exists. Returns the
// number of rows actually inserted (RowsAffected), which a caller uses to report
// idempotency: a re-discovered candidate contributes 0.
//
// ON CONFLICT DO NOTHING (not DO UPDATE) is deliberate: the unique key IS the
// row identity, so a conflict means this exact candidate was already queued —
// possibly already accepted or rejected by a human. DO NOTHING leaves that
// reviewed row untouched (never flips it back to pending), which is the whole
// point of a resumable sweep.
func upsertSuggestions(db *gorm.DB, artistID uint, candidates []contracts.MusicLinkCandidate) (int, error) {
	rows := make([]catalogm.ArtistLinkSuggestion, 0, len(candidates))
	for _, c := range candidates {
		rows = append(rows, catalogm.ArtistLinkSuggestion{
			ArtistID:     artistID,
			Platform:     c.Platform,
			URL:          c.URL,
			Source:       c.Source,
			MBArtistID:   nilIfEmpty(c.MBArtistID),
			MBArtistName: nilIfEmpty(c.MBArtistName),
			Confidence:   c.Confidence,
			RegionMatch:  c.RegionMatch,
			Live:         c.Live,
			Notes:        nilIfEmpty(c.Notes),
			Status:       catalogm.LinkSuggestionStatusPending,
		})
	}

	res := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "artist_id"}, {Name: "platform"}, {Name: "url"}},
		DoNothing: true,
	}).Create(&rows)
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

// nilIfEmpty maps the contract's value-type "" (its zero value for an absent
// optional field) to a nil *string so the column stores SQL NULL rather than an
// empty string — matching the nullable mb_artist_id / mb_artist_name / notes
// columns.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
