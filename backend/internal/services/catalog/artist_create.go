package catalog

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/utils"
)

// FindOrCreateArtistTx is the single write path for "find-or-create an artist by
// name". Every create-by-name site — admin create, show inline artists, discovery
// import, data-sync import — funnels through it, so name dedup, unique-slug
// generation, and (PSY-1247) image-enrichment enqueue live in exactly one place
// rather than being copy-pasted and drifting (the four prior copies differed in
// slug timing). No magic: callers invoke it explicitly.
//
// tx is the caller's transaction (or the base *gorm.DB — it works on either).
// apply, when non-nil, sets fields on a NEWLY created artist before insert; it is
// NOT called when an existing artist is found. Returns created=false for an
// existing match, whose slug is backfilled if missing (absorbing the prior
// data-sync special case).
func FindOrCreateArtistTx(tx *gorm.DB, name string, apply func(*catalogm.Artist)) (*catalogm.Artist, bool, error) {
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
	if cerr := tx.Create(&artist).Error; cerr != nil {
		return nil, false, fmt.Errorf("create artist %q: %w", name, cerr)
	}
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
