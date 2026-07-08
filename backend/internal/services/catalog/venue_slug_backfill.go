package catalog

import (
	"fmt"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/utils"
)

// VenueSlugBackfillOptions configures a BackfillVenueSlugs run.
type VenueSlugBackfillOptions struct {
	// DryRun computes and reports every change without writing (the CLI default).
	DryRun bool
}

// VenueSlugChange records one venue whose stored slug did not match the
// canonical GenerateVenueSlug output.
type VenueSlugChange struct {
	VenueID uint
	Name    string
	City    string
	State   string
	OldSlug string // "" when the stored slug was NULL or empty
	NewSlug string
	Applied bool // true only when a live run wrote the change
}

// VenueSlugBackfillReport summarizes a BackfillVenueSlugs run.
type VenueSlugBackfillReport struct {
	Scanned   int
	Changed   int // planned (dry-run) or applied (live) changes
	Unchanged int
	Changes   []VenueSlugChange
	Errors    []string
}

// venueSlugTarget computes the canonical, collision-safe slug for a venue and
// whether it differs from the current stored slug. `slugTaken` reports whether a
// candidate slug is already used by a DIFFERENT venue. Kept pure (no DB) so the
// decision logic is unit-testable; BackfillVenueSlugs supplies the DB-backed
// slugTaken closure.
//
// Guarantees:
//   - never proposes an empty slug — GenerateVenueSlug only returns "" when
//     name+city+state are all empty (forbidden by the NOT NULL columns), but we
//     fail safe and leave such a row untouched rather than blanking it;
//   - idempotent — a venue whose stored slug already equals the canonical
//     collision-safe form (including a legitimate "-2" uniqueness suffix) reports
//     needsUpdate=false and proposes no redundant write.
func venueSlugTarget(name, city, state, currentSlug string, slugTaken func(candidate string) bool) (target string, needsUpdate bool) {
	expected := utils.GenerateVenueSlug(name, city, state)
	if expected == "" {
		return "", false
	}
	// Fast path: the common canonical case needs no uniqueness probe.
	if currentSlug == expected {
		return expected, false
	}
	target = utils.GenerateUniqueSlug(expected, slugTaken)
	return target, target != currentSlug
}

// BackfillVenueSlugs finds venues whose stored slug does not match the canonical
// utils.GenerateVenueSlug(name, city, state) output and rewrites them to the
// correct, collision-safe slug (PSY-1385).
//
// Historical seed data left a handful of venues with corrupted slugs — the first
// character of each word dropped, the state suffix missing, or the slug empty
// (e.g. "alley-ar-hoenix" for Valley Bar in Phoenix, AZ). Every current
// create/update path already uses GenerateVenueSlug, so this is a one-shot
// cleanup and is idempotent: a second run reports zero changes.
//
// There is no slug-redirect mechanism in the system and the corrupted slugs have
// no legitimate external references, so old slugs simply 404 after the rewrite;
// internal links regenerate from venue.slug. In live mode changes are written
// sequentially, so two venues whose canonical slug collides resolve
// deterministically (the second gets a "-2" suffix from GenerateUniqueSlug).
func BackfillVenueSlugs(database *gorm.DB, opts VenueSlugBackfillOptions) (*VenueSlugBackfillReport, error) {
	var venues []catalogm.Venue
	if err := database.Order("id").Find(&venues).Error; err != nil {
		return nil, fmt.Errorf("load venues: %w", err)
	}

	report := &VenueSlugBackfillReport{}
	for i := range venues {
		v := &venues[i]
		report.Scanned++

		current := ""
		if v.Slug != nil {
			current = *v.Slug
		}

		target, needsUpdate := venueSlugTarget(v.Name, v.City, v.State, current, func(candidate string) bool {
			var count int64
			database.Model(&catalogm.Venue{}).
				Where("slug = ? AND id <> ?", candidate, v.ID).
				Count(&count)
			return count > 0
		})
		if !needsUpdate {
			report.Unchanged++
			continue
		}

		change := VenueSlugChange{
			VenueID: v.ID,
			Name:    v.Name,
			City:    v.City,
			State:   v.State,
			OldSlug: current,
			NewSlug: target,
		}

		if opts.DryRun {
			report.Changed++
			report.Changes = append(report.Changes, change)
			continue
		}

		if err := database.Model(&catalogm.Venue{}).
			Where("id = ?", v.ID).
			Update("slug", target).Error; err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("venue %d (%q): update slug: %v", v.ID, v.Name, err))
			report.Changes = append(report.Changes, change) // Applied stays false
			continue
		}

		change.Applied = true
		report.Changed++
		report.Changes = append(report.Changes, change)
	}

	return report, nil
}
