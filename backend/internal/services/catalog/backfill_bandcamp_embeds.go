package catalog

import (
	"fmt"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Backfill of artists.bandcamp_embed_url from catalogued release Bandcamp links
// (PSY-1188).
//
// Many artists have only a Bandcamp PROFILE ROOT in social.bandcamp and an empty
// bandcamp_embed_url, so the artist page falls back to a plain text link — even
// when the artist HAS a release whose Bandcamp external link is a perfectly
// embeddable album/track URL. This pass derives the artist embed from its
// releases (see ArtistService.DeriveBandcampEmbedForArtist + the selection rule
// in selectBandcampEmbedFromReleases) and, on apply, stamps the provenance
// "release_derived" so the PSY-1189 keep-fresh hook can later refresh/clean up
// the auto-derived ones without touching human-curated embeds.
//
// FILL-WHEN-EMPTY (load-bearing invariant): the pass only considers artists
// where bandcamp_embed_url IS NULL, and never overwrites a non-null value. An
// artist whose embed was set manually is out of scope by construction.
//
// Dry-run by default: the report describes exactly what an apply run would
// change without writing anything. Idempotent: a second apply run reports zero
// fills (the now-non-null rows are excluded by the IS NULL gate).

// BandcampEmbedBackfillOptions configures a backfill run. Verbosity of the
// printed report is a CLI concern (the cmd's --verbose flag), so it is not part
// of the service options.
type BandcampEmbedBackfillOptions struct {
	DryRun bool
}

// BandcampEmbedFill records the planned/applied change for a single artist.
type BandcampEmbedFill struct {
	ArtistID uint
	Name     string
	EmbedURL string // the derived album/track URL the apply run would/did write
}

// BandcampEmbedBackfillReport is the structured outcome of a backfill run.
type BandcampEmbedBackfillReport struct {
	// ArtistsScanned is the number of artists with a NULL bandcamp_embed_url
	// that were considered (artists with an embed already set are excluded by
	// the query, not counted here — see Left).
	ArtistsScanned int
	// Filled is artists for which a release-derived embed was found (and, on
	// apply, written).
	Filled int
	// SkippedNoLink is NULL-embed artists with no embeddable release Bandcamp
	// link — left untouched.
	SkippedNoLink int
	// Left is artists whose bandcamp_embed_url was already set (non-null) and so
	// were never candidates. Reported for transparency; the fill-when-empty
	// invariant means these are never written.
	Left int

	Fills  []BandcampEmbedFill
	Errors []string
}

// BackfillArtistBandcampEmbeds derives a Bandcamp embed URL for every artist
// whose bandcamp_embed_url IS NULL from its catalogued release Bandcamp links.
// With opts.DryRun the report describes exactly what an apply run would change
// without writing anything; the same derivation runs in both modes so the
// dry-run is faithful.
//
// On apply, each filled artist gets bandcamp_embed_url set AND
// bandcamp_embed_source = "release_derived", in a single scoped UPDATE that
// re-asserts the IS NULL guard so a concurrent manual write can't be clobbered.
func BackfillArtistBandcampEmbeds(database *gorm.DB, opts BandcampEmbedBackfillOptions) (*BandcampEmbedBackfillReport, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	report := &BandcampEmbedBackfillReport{}
	svc := &ArtistService{db: database}

	// Count artists with an embed already set, for the transparency "Left" line.
	var alreadySet int64
	if err := database.Model(&catalogm.Artist{}).
		Where("bandcamp_embed_url IS NOT NULL").
		Count(&alreadySet).Error; err != nil {
		return nil, fmt.Errorf("failed to count artists with embed set: %w", err)
	}
	report.Left = int(alreadySet)

	// Candidate artists: only those with a NULL embed (the fill-when-empty gate).
	// id + name are all we need; the derivation re-queries releases per artist.
	type artistRow struct {
		ID   uint
		Name string
	}
	var candidates []artistRow
	if err := database.Model(&catalogm.Artist{}).
		Select("id", "name").
		Where("bandcamp_embed_url IS NULL").
		Order("id ASC").
		Find(&candidates).Error; err != nil {
		return nil, fmt.Errorf("failed to list candidate artists: %w", err)
	}
	report.ArtistsScanned = len(candidates)

	for _, a := range candidates {
		embed, err := svc.DeriveBandcampEmbedForArtist(a.ID)
		if err != nil {
			report.Errors = append(report.Errors,
				fmt.Sprintf("artist %d %q: derive failed: %v", a.ID, a.Name, err))
			continue
		}
		if embed == nil {
			report.SkippedNoLink++
			continue
		}

		fill := BandcampEmbedFill{ArtistID: a.ID, Name: a.Name, EmbedURL: *embed}

		if !opts.DryRun {
			// Apply: set the embed + stamp release_derived provenance. The
			// `bandcamp_embed_url IS NULL` guard is re-asserted so a manual
			// write that landed between the scan and now is NOT clobbered (the
			// invariant holds even under concurrency). RowsAffected == 0 means
			// it was filled in the meantime — a benign skip, not an error, so
			// it is NOT counted as a fill.
			res := database.Model(&catalogm.Artist{}).
				Where("id = ? AND bandcamp_embed_url IS NULL", a.ID).
				Updates(map[string]interface{}{
					"bandcamp_embed_url":    *embed,
					"bandcamp_embed_source": catalogm.BandcampEmbedSourceReleaseDerived,
				})
			if res.Error != nil {
				report.Errors = append(report.Errors,
					fmt.Sprintf("artist %d %q: update failed: %v", a.ID, a.Name, res.Error))
				continue
			}
			if res.RowsAffected == 0 {
				// Raced by a manual write — count as already-set, not filled.
				report.Left++
				continue
			}
		}

		// Recorded only when the change is real: every dry-run candidate, and
		// every confirmed apply write.
		report.Filled++
		report.Fills = append(report.Fills, fill)
	}

	return report, nil
}
