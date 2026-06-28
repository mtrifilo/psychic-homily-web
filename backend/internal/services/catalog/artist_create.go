package catalog

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// FindOrCreateArtistTx is the single write path for "find-or-create an artist by
// name". Every create-by-name site funnels through it — admin create, show inline
// artists, discovery import, the data-sync show + single-artist imports,
// festival-entry, and seed — so name dedup and unique-slug generation live in
// exactly one place rather than being copy-pasted and drifting (the prior copies
// differed in slug timing). The image-enrichment outbox enqueue (PSY-1247) lives
// here too, so it covers every create path at once. No magic: callers invoke it
// explicitly.
//
// tx is the caller's transaction (or the base *gorm.DB — it works on either).
// apply, when non-nil, sets fields on a NEWLY created artist before insert; it is
// NOT called when an existing artist is found, and must NOT change Name (dedup +
// slug key off the name argument). Returns created=false for an existing match,
// whose slug is backfilled if missing (absorbing the prior data-sync special case).
func FindOrCreateArtistTx(tx *gorm.DB, name string, apply func(*catalogm.Artist)) (*catalogm.Artist, bool, error) {
	// Validate at the boundary of trust (Code Complete): callers' empty-name guards
	// vary (some check == "", some don't), so reject blank/whitespace names once here.
	if strings.TrimSpace(name) == "" {
		return nil, false, errors.New("artist name is required")
	}

	var artist catalogm.Artist
	err := tx.Where("LOWER(name) = LOWER(?)", name).First(&artist).Error
	switch {
	case err == nil:
		// Existing artist: backfill a missing slug, then return as not-created.
		if artist.Slug == nil || *artist.Slug == "" {
			slug := uniqueArtistSlugTx(tx, artist.Name)
			if uerr := tx.Model(&artist).Update("slug", slug).Error; uerr != nil {
				return nil, false, fmt.Errorf("backfill artist slug for %q: %w", name, uerr)
			}
			artist.Slug = &slug
		}
		return &artist, false, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		// fall through to create
	default:
		return nil, false, fmt.Errorf("find artist %q: %w", name, err)
	}

	artist = catalogm.Artist{Name: name}
	if apply != nil {
		apply(&artist)
	}
	slug := uniqueArtistSlugTx(tx, artist.Name)
	artist.Slug = &slug

	// Conflict-safe create (PSY-1256). The SELECT above and this INSERT are not
	// atomic, so a concurrent create of a case-variant of the same name can commit in
	// between and our INSERT then trips the case-insensitive unique index
	// (artists_lower_name_uniq) — the funnel dedups on LOWER(name), so this is the
	// race the index now closes (exact-case dups were already blocked by the old
	// artists_name_key, dropped by this migration).
	//
	// The INSERT runs in a nested tx.Transaction: a SAVEPOINT when the caller already
	// holds a transaction (show import, discovery), or a standalone BEGIN/COMMIT on a
	// base *gorm.DB (admin create, data-sync, seed). Either way it CONTAINS a failed
	// INSERT — Postgres aborts the whole transaction on any failed statement, so
	// without this the recovery SELECT below (and the caller's eventual COMMIT) would
	// fail on the poisoned tx. On a collision we re-select and return the winner as
	// created=false, so concurrent creators converge instead of erroring.
	createErr := tx.Transaction(func(itx *gorm.DB) error {
		return itx.Create(&artist).Error
	})
	if createErr != nil {
		if shared.IsDuplicateKey(createErr) {
			// The collision is on the name index (the case this guards) or, rarely,
			// the slug index (a concurrent same-slug race). Post-TranslateError the
			// two are indistinguishable (see pattern_gorm_translate_error), so we
			// re-select by name: a name collision returns the winner; a slug-only
			// collision finds nothing and falls through to the error.
			var existing catalogm.Artist
			if ferr := tx.Where("LOWER(name) = LOWER(?)", name).First(&existing).Error; ferr == nil {
				return &existing, false, nil
			}
		}
		return nil, false, fmt.Errorf("create artist %q: %w", name, createErr)
	}
	// PSY-1247: prompt on-create image enrichment. Enqueue ONLY on the created
	// path — a found artist is already covered by its own create-time enqueue (or,
	// for pre-funnel rows, by the Phase-A sweep), so re-enqueuing every time a show
	// references it would be churn. Best-effort: never fails the create (and no-ops
	// when the feature is disabled). Atomicity depends on whether the caller passes
	// a tx — see enqueueImageEnrich.
	enqueueImageEnrich(tx, catalogm.ImageEnrichEntityArtist, artist.ID)
	return &artist, true, nil
}

// uniqueArtistSlugTx generates a slug unique among artists, scoped to tx.
func uniqueArtistSlugTx(tx *gorm.DB, name string) string {
	base := utils.GenerateArtistSlug(name)
	return utils.GenerateUniqueSlug(base, func(candidate string) bool {
		var count int64
		tx.Model(&catalogm.Artist{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})
}
