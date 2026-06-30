package catalog

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// createReleaseTx is the shared "insert a release + its links in this tx" core,
// extracted so the interactive CreateRelease path and the importer's dedup path
// (FindOrCreateReleaseByReleaseGroupMBID, PSY-1281) build releases identically —
// one create funnel rather than two copies that drift (mirrors FindOrCreateArtistTx).
//
// apply, when non-nil, sets fields on the new release before insert (e.g. stamp the
// release-group MBID on the importer path); it must NOT change Title, since the slug
// keys off it. Runs entirely on the caller's tx (atomic with the caller's work); the
// slug's uniqueness is checked within that tx, so rows created earlier in the same tx
// are seen — a deliberate change from the pre-refactor CreateRelease (which checked on
// the base DB), matching the FindOrCreateArtistTx funnel.
func createReleaseTx(tx *gorm.DB, req *contracts.CreateReleaseRequest, apply func(*catalogm.Release)) (*catalogm.Release, error) {
	releaseType := catalogm.ReleaseType(req.ReleaseType)
	if releaseType == "" {
		releaseType = catalogm.ReleaseTypeLP
	}

	slug := uniqueReleaseSlugTx(tx, req.Title)
	release := &catalogm.Release{
		Title:       req.Title,
		Slug:        &slug,
		ReleaseType: releaseType,
		ReleaseYear: req.ReleaseYear,
		ReleaseDate: req.ReleaseDate,
		CoverArtURL: req.CoverArtURL,
		Description: req.Description,
	}
	if apply != nil {
		apply(release)
	}

	if err := tx.Create(release).Error; err != nil {
		return nil, fmt.Errorf("failed to create release: %w", err)
	}

	// Create artist_releases entries
	for i, artistEntry := range req.Artists {
		role := artistEntry.Role
		if role == "" {
			role = string(catalogm.ArtistReleaseRoleMain)
		}
		ar := &catalogm.ArtistRelease{
			ArtistID:  artistEntry.ArtistID,
			ReleaseID: release.ID,
			Role:      catalogm.ArtistReleaseRole(role),
			Position:  i,
		}
		if err := tx.Create(ar).Error; err != nil {
			return nil, fmt.Errorf("failed to create artist-release link: %w", err)
		}
	}

	// Create external links
	for _, linkEntry := range req.ExternalLinks {
		link := &catalogm.ReleaseExternalLink{
			ReleaseID: release.ID,
			Platform:  linkEntry.Platform,
			URL:       linkEntry.URL,
		}
		if err := tx.Create(link).Error; err != nil {
			return nil, fmt.Errorf("failed to create external link: %w", err)
		}
	}

	// PSY-1189: keep the artist Bandcamp embed fresh — if this release carries an
	// embeddable /album|/track Bandcamp link and a credited artist's embed is still
	// NULL, fill it (release_derived). Same tx, so a failure rolls the whole create
	// back. No-op when there's no Bandcamp link or no empty artist.
	if err := fillReleaseDerivedEmbedsForRelease(tx, release.ID); err != nil {
		return nil, err
	}

	// PSY-1247: prompt on-create cover-art enrichment via the transactional outbox.
	// Same tx as the release create (atomic), best-effort (never fails the create —
	// see enqueueImageEnrich).
	enqueueImageEnrich(tx, catalogm.ImageEnrichEntityRelease, release.ID)

	return release, nil
}

// uniqueReleaseSlugTx generates a slug unique among releases, scoped to tx.
func uniqueReleaseSlugTx(tx *gorm.DB, title string) string {
	base := utils.GenerateArtistSlug(title)
	return utils.GenerateUniqueSlug(base, func(candidate string) bool {
		var count int64
		tx.Model(&catalogm.Release{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})
}

// FindOrCreateReleaseByReleaseGroupMBID is the exact, artist-anchored release dedup
// path for the discography importer (PSY-1282) — keyed on the release-GROUP MBID,
// the release subsystem's first exact dedup key (PSY-1281; mirrors the artist MBID
// keystone, PSY-1249). The interactive CreateRelease keeps its no-dedup behavior;
// the importer calls this instead.
//
// Scope note: the step-2 title-match reconciliation policy lives in this keystone
// deliberately — so it is unit-testable independent of the MusicBrainz fetch layer —
// even though the importer (PSY-1282) is its only intended caller. Any other future
// caller inherits the title-merge behavior, so add new callers knowingly.
//
// Resolution order (each step is homonym-safe: an uncertain identity is NEVER
// merged — a missed merge is a recoverable duplicate, a wrong merge corrupts two
// releases):
//
//  1. A release already carrying this RG-MBID -> return it, created=false. The
//     partial-unique index guarantees at most one (re-import / concurrent-import
//     idempotency).
//  2. Else an artist-anchored EXACT-title release whose RG-MBID is still NULL ->
//     stamp it (fill-when-empty) and return created=false. "Artist-anchored" =
//     shares >=1 credited artist with req.Artists; "exact" = case-insensitive,
//     trimmed Title. EXACTLY ONE such match only — zero or ambiguous (>=2) matches
//     fall through to create.
//  3. Else create a new release (createReleaseTx) with the RG-MBID stamped.
//
// tx is the caller's transaction or a base *gorm.DB. Returns the release, whether it
// was newly created, and any error.
func FindOrCreateReleaseByReleaseGroupMBID(tx *gorm.DB, releaseGroupMBID string, req *contracts.CreateReleaseRequest) (*catalogm.Release, bool, error) {
	// Validate at the boundary of trust (Code Complete): a malformed MBID would
	// poison the dedup key, and a blank title can't generate a slug.
	if !utils.IsValidMBID(releaseGroupMBID) {
		return nil, false, fmt.Errorf("invalid release-group MBID %q", releaseGroupMBID)
	}
	if req == nil || strings.TrimSpace(req.Title) == "" {
		return nil, false, errors.New("release title is required")
	}

	// Step 1: exact dedup on the RG-MBID.
	var existing catalogm.Release
	err := tx.Where("musicbrainz_release_group_id = ?", releaseGroupMBID).First(&existing).Error
	switch {
	case err == nil:
		return &existing, false, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		// fall through
	default:
		return nil, false, fmt.Errorf("find release by RG-MBID %q: %w", releaseGroupMBID, err)
	}

	// Step 2: artist-anchored exact-title fill-when-empty.
	if matched, ok, ferr := fillReleaseGroupMBIDOnTitleMatch(tx, releaseGroupMBID, req); ferr != nil {
		return nil, false, ferr
	} else if ok {
		return matched, false, nil
	}

	// Step 3: create, conflict-safe against the partial-unique RG-MBID index.
	//
	// The step-1 SELECT and this INSERT are not atomic, so a concurrent import can
	// stamp this RG-MBID in between; the index then trips our INSERT. The nested
	// tx.Transaction is a SAVEPOINT (or a standalone BEGIN/COMMIT on a base *gorm.DB)
	// that CONTAINS the failed INSERT — Postgres aborts the whole tx on any failed
	// statement, so without it the re-select below and the caller's COMMIT would fail
	// on the poisoned tx (see pattern_gorm_translate_error / FindOrCreateArtistTx).
	var release *catalogm.Release
	createErr := tx.Transaction(func(itx *gorm.DB) error {
		var cerr error
		release, cerr = createReleaseTx(itx, req, func(r *catalogm.Release) {
			r.MusicBrainzReleaseGroupID = &releaseGroupMBID
		})
		return cerr
	})
	if createErr != nil {
		if shared.IsDuplicateKey(createErr) {
			// The collision is the RG-MBID index (the race this guards): re-select the
			// winner and return it as not-created so concurrent importers converge. A
			// slug-only collision (GenerateUniqueSlug suffixes, so vanishingly rare)
			// finds nothing and falls through to the error.
			var winner catalogm.Release
			if ferr := tx.Where("musicbrainz_release_group_id = ?", releaseGroupMBID).First(&winner).Error; ferr == nil {
				return &winner, false, nil
			}
		}
		return nil, false, fmt.Errorf("create release %q: %w", req.Title, createErr)
	}
	return release, true, nil
}

// fillReleaseGroupMBIDOnTitleMatch implements step 2 above: stamp releaseGroupMBID
// onto a pre-existing, artist-anchored, exact-title release whose RG-MBID is still
// NULL.
//
// Return contract: the bool is "handled" (a usable release is being returned for the
// caller to surface as created=false), NOT "created". It is true on a unique fill and
// on a concurrent-conflict convergence; false (with a nil release) means "not handled
// — caller should create", returned when there is no artist anchor, no title match, an
// AMBIGUOUS (>=2) match (never merge an uncertain identity), or the fill lost a
// concurrent race.
func fillReleaseGroupMBIDOnTitleMatch(tx *gorm.DB, releaseGroupMBID string, req *contracts.CreateReleaseRequest) (*catalogm.Release, bool, error) {
	// Anchor on the credited artists. With no artist to anchor on we cannot
	// distinguish same-title-different-artist releases, so we never title-match.
	artistIDs := make([]uint, 0, len(req.Artists))
	for _, a := range req.Artists {
		if a.ArtistID != 0 {
			artistIDs = append(artistIDs, a.ArtistID)
		}
	}
	if len(artistIDs) == 0 {
		return nil, false, nil
	}

	var candidates []catalogm.Release
	err := tx.
		Model(&catalogm.Release{}).
		Joins("JOIN artist_releases ar ON ar.release_id = releases.id").
		Where("releases.musicbrainz_release_group_id IS NULL").
		Where("LOWER(TRIM(releases.title)) = LOWER(TRIM(?))", req.Title).
		Where("ar.artist_id IN ?", artistIDs).
		Group("releases.id"). // a release sharing 2 anchored artists must not count twice
		Find(&candidates).Error
	if err != nil {
		return nil, false, fmt.Errorf("title-match lookup for RG-MBID %q: %w", releaseGroupMBID, err)
	}
	if len(candidates) != 1 {
		// Zero matches, or ambiguous: let the caller create a fresh release.
		return nil, false, nil
	}

	matched := &candidates[0]

	// The stamp must survive two concurrent hazards under READ COMMITTED (prod's
	// isolation), since the SELECT above and this write are not atomic:
	//
	//   - A different importer stamps a DIFFERENT RG-MBID onto this same row first.
	//     The candidate SELECT filtered on IS NULL, but a bare PK update would clobber
	//     that fresh value (the partial-unique index can't catch a different value on
	//     the same row). So we repeat `IS NULL` in the WHERE and treat RowsAffected==0
	//     as "lost the race" -> fall through to create our own release.
	//   - A different importer stamps the SAME RG-MBID onto ANOTHER row first. Our
	//     write then trips uq_releases_musicbrainz_release_group_id. The update MUST be
	//     wrapped in a nested tx.Transaction (SAVEPOINT) — exactly like step 3 — so the
	//     failed statement doesn't poison the caller's transaction (Postgres aborts the
	//     whole tx on any failed statement; without this the recovery re-SELECT below,
	//     and the importer's other in-flight work, would die with SQLSTATE 25P02).
	var rowsAffected int64
	updErr := tx.Transaction(func(itx *gorm.DB) error {
		res := itx.Model(&catalogm.Release{}).
			Where("id = ? AND musicbrainz_release_group_id IS NULL", matched.ID).
			Update("musicbrainz_release_group_id", releaseGroupMBID)
		rowsAffected = res.RowsAffected
		return res.Error
	})
	if updErr != nil {
		if shared.IsDuplicateKey(updErr) {
			// Same RG-MBID was concurrently claimed on another row; converge on that
			// winner (re-SELECT runs on the caller's still-clean tx — the SAVEPOINT
			// rolled the failed update back).
			var winner catalogm.Release
			if rerr := tx.Where("musicbrainz_release_group_id = ?", releaseGroupMBID).First(&winner).Error; rerr == nil {
				return &winner, true, nil
			}
		}
		return nil, false, fmt.Errorf("stamp RG-MBID on title match: %w", updErr)
	}
	if rowsAffected == 0 {
		// The row got a different RG-MBID between our SELECT and this update, so it is
		// no longer ours to fill — let the caller create a fresh release (itself
		// conflict-safe on the RG-MBID index).
		return nil, false, nil
	}
	matched.MusicBrainzReleaseGroupID = &releaseGroupMBID
	return matched, true, nil
}
