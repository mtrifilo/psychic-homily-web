package catalog

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/utils"
)

// VenueSlugBackfillOptions configures a BackfillVenueSlugs run.
type VenueSlugBackfillOptions struct {
	// DryRun computes and reports every change without writing (the CLI default).
	DryRun bool
}

// VenueSlugChange records one venue whose slug shows the corruption signature
// and the canonical slug it will be (or was) rewritten to.
type VenueSlugChange struct {
	VenueID uint
	Name    string
	City    string
	State   string
	OldSlug string // "" when the stored slug was NULL or empty
	NewSlug string
	Applied bool // true only when a live run committed the change
}

// VenueSlugBackfillReport summarizes a BackfillVenueSlugs run.
type VenueSlugBackfillReport struct {
	Scanned   int
	Changed   int // planned (dry-run) or applied (live) changes
	Unchanged int
	Changes   []VenueSlugChange
	Errors    []string
}

// hasLocationTail reports whether slug ends with the venue's canonical location
// tail (e.g. "phoenix-az"), optionally followed by a single "-<digits>"
// uniqueness suffix (GenerateUniqueSlug / prior migration dedup, e.g.
// "...-phoenix-az-2"). Every slug GenerateVenueSlug produces ends with this
// tail, so its presence marks a well-formed slug — including one left
// deliberately stale by a rename.
func hasLocationTail(slug, tail string) bool {
	if slug == tail {
		return true
	}
	marker := "-" + tail
	idx := strings.LastIndex(slug, marker)
	if idx < 0 {
		return false
	}
	rest := slug[idx+len(marker):]
	if rest == "" {
		return true
	}
	if len(rest) < 2 || rest[0] != '-' {
		return false
	}
	for _, r := range rest[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// slugLooksCorrupt reports whether a stored slug shows the PSY-1385 corruption
// signature: it is empty, or it is missing the mandatory "-{city}-{state}"
// location tail that GenerateVenueSlug always appends (the historical bug
// dropped the first character of every word, including the city, and dropped
// the state entirely — e.g. "alley-ar-hoenix" for Valley Bar in Phoenix, AZ).
//
// A well-formed slug is left untouched even when it diverges from the venue's
// CURRENT name: UpdateVenue never regenerates the slug on rename/relocate
// (venue.go), so a renamed venue's slug is intentionally stable and has no
// redirect. Rewriting it would 404 every bookmarked/shared link. Detecting the
// corruption signature — not mere divergence from the current name — is what
// keeps this backfill from clobbering those deliberately-stable slugs.
func slugLooksCorrupt(slug, city, state string) bool {
	if slug == "" {
		return true
	}
	tail := utils.GenerateSlug(city, state)
	if tail == "" {
		// No location signal to check against (both city and state empty). Fail
		// safe: leave the slug alone rather than guess it is corrupt.
		return false
	}
	return !hasLocationTail(slug, tail)
}

// venueSlugTarget computes the canonical, collision-safe slug for a venue whose
// stored slug shows the corruption signature, and whether it differs from the
// current slug. `slugTaken` reports whether a candidate slug is already used by a
// DIFFERENT venue (or already claimed earlier in the same run). Kept pure (no
// DB) so the decision logic is unit-testable; BackfillVenueSlugs supplies the
// DB-backed slugTaken closure.
func venueSlugTarget(name, city, state, currentSlug string, slugTaken func(candidate string) bool) (target string, needsUpdate bool) {
	if !slugLooksCorrupt(currentSlug, city, state) {
		return currentSlug, false
	}
	expected := utils.GenerateVenueSlug(name, city, state)
	if expected == "" {
		// GenerateVenueSlug only blanks when name+city+state are all empty
		// (forbidden by the NOT NULL columns). Fail safe: never blank a slug.
		return currentSlug, false
	}
	target = utils.GenerateUniqueSlug(expected, slugTaken)
	return target, target != currentSlug
}

// BackfillVenueSlugs repairs venues whose slug shows the PSY-1385 corruption
// signature (empty, or missing the "-{city}-{state}" location tail), rewriting
// them to the canonical, collision-safe utils.GenerateVenueSlug output.
//
// It runs in two phases. Phase 1 (reads only, identical for dry-run and live)
// computes the full plan, reserving each proposed slug in an in-run "claimed"
// set so two venues that canonicalize to the same slug preview the SAME "-2"
// resolution a live run would apply — the dry-run report is exactly what a live
// run writes. Phase 2 (live only) applies the whole plan inside a single
// transaction, so a mid-run failure rolls back rather than leaving venues
// half-rewritten.
//
// It is idempotent: a rewritten slug carries the location tail, so a second run
// sees no corruption signature and reports zero changes. There is no
// slug-redirect mechanism and the corrupted slugs have no legitimate external
// references, so the old slugs simply 404 after the rewrite; internal links
// regenerate from venue.slug.
func BackfillVenueSlugs(database *gorm.DB, opts VenueSlugBackfillOptions) (*VenueSlugBackfillReport, error) {
	var venues []catalogm.Venue
	if err := database.Order("id").Find(&venues).Error; err != nil {
		return nil, fmt.Errorf("load venues: %w", err)
	}

	report := &VenueSlugBackfillReport{Scanned: len(venues)}
	claimed := make(map[string]bool) // slugs reserved earlier in THIS run
	var plan []VenueSlugChange

	// Phase 1 — compute the plan (reads only).
	for i := range venues {
		v := &venues[i]

		current := ""
		if v.Slug != nil {
			current = *v.Slug
		}

		var probeErr error
		target, needsUpdate := venueSlugTarget(v.Name, v.City, v.State, current, func(candidate string) bool {
			if claimed[candidate] {
				return true
			}
			var count int64
			if err := database.Model(&catalogm.Venue{}).
				Where("slug = ? AND id <> ?", candidate, v.ID).
				Count(&count).Error; err != nil {
				// Fail safe: a probe we couldn't run must not be read as "free",
				// which would propose a possibly-taken slug and hit the unique index.
				probeErr = err
				return true
			}
			return count > 0
		})
		if probeErr != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("venue %d (%q): probe slug uniqueness: %v", v.ID, v.Name, probeErr))
			continue
		}
		if !needsUpdate {
			report.Unchanged++
			continue
		}

		claimed[target] = true
		plan = append(plan, VenueSlugChange{
			VenueID: v.ID,
			Name:    v.Name,
			City:    v.City,
			State:   v.State,
			OldSlug: current,
			NewSlug: target,
		})
	}

	// Dry-run: report the plan, write nothing.
	if opts.DryRun {
		report.Changed = len(plan)
		report.Changes = plan
		return report, nil
	}

	// Phase 2 — apply the whole plan atomically.
	if err := database.Transaction(func(tx *gorm.DB) error {
		for _, c := range plan {
			if err := tx.Model(&catalogm.Venue{}).
				Where("id = ?", c.VenueID).
				Update("slug", c.NewSlug).Error; err != nil {
				return fmt.Errorf("venue %d (%q): update slug: %w", c.VenueID, c.Name, err)
			}
		}
		return nil
	}); err != nil {
		// Rolled back — no changes were applied; surface the error and report
		// zero applied changes so the operator doesn't think a partial run stuck.
		report.Errors = append(report.Errors, err.Error())
		return report, nil
	}

	for _, c := range plan {
		c.Applied = true
		report.Changes = append(report.Changes, c)
	}
	report.Changed = len(plan)
	return report, nil
}
