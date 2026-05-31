package explore

// Predicate parity (PSY-880) — the write-side referent validation in
// services/admin/featured_slot.go (validateReferent) and the read-side
// "publicly visible" filter in this package (resolveFeaturedBill /
// resolveFeaturedCollection) encode the SAME predicate: a bill is
// featurable iff its show is `approved`; a collection is featurable iff
// it is `is_public=true`. Nothing but a source comment kept them in sync
// before this suite.
//
// Silent-divergence risk (the bug PSY-850 closed, re-opened by drift): if
// one side's predicate changes and the other doesn't, the admin "save"
// succeeds but /explore hides the slot — the phantom-save UX returns,
// only the trigger condition is different.
//
// Each test below exercises BOTH sides against the same fixture so the
// suite fails if either side's predicate drifts along a dimension it
// already exercises — show status, collection visibility, or referent
// existence:
//   - write side: SetActiveSlot returns the expected rejection sentinel
//     (or accepts, for the valid cases);
//   - read side: GetFeatured excludes the referent (or surfaces it).
// The read side is checked by forcing a slot at the offending referent
// with a raw insert that bypasses write-side validation — otherwise an
// invalid referent could never reach the read path at all.
//
// Scope honesty: fixture-based parity can only catch drift along the
// dimensions the fixtures vary. A NEW predicate column added to just one
// side in a future PR (say a read-side-only is_deleted filter) is
// invisible here, because every fixture would share the same value for
// it. Against that class the in-source "change both sides in the same PR"
// discipline (see featured_slot.go validateReferent + explore.go
// resolveFeatured*) stays the primary guard; this suite backs the
// dimensions that exist today.
//
// These methods extend ExploreServiceIntegrationSuite (defined in
// explore_test.go), reusing its DB, both wired services, per-test
// cleanup, and createAdmin / createShow / createCollection helpers.

import (
	"errors"
	"fmt"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	adminsvc "psychic-homily-backend/internal/services/admin"
)

// insertActiveSlotRaw inserts an active featured_slots row directly,
// bypassing SetActiveSlot's referent validation. The three NOT-NULL
// columns without a default (slot_type, entity_id, created_by) are
// enough — active_from / created_at / updated_at default NOW() and
// active_until defaults NULL (active). Mirrors the raw-insert bypass in
// services/admin/featured_slot_test.go's partial-unique-index test.
func (s *ExploreServiceIntegrationSuite) insertActiveSlotRaw(slotType string, entityID, createdBy uint) {
	s.Require().NoError(s.db.Exec(
		"INSERT INTO featured_slots (slot_type, entity_id, created_by) VALUES (?, ?, ?)",
		slotType, entityID, createdBy,
	).Error)
}

// createShowWithStatus seeds a bare show row with the requested status.
// The suite's createShow always inserts an approved show with full
// venue/artist joins; the parity rejection cases only need a row whose
// status column drives the predicate, so this is the minimal variant.
func (s *ExploreServiceIntegrationSuite) createShowWithStatus(title string, status catalogm.ShowStatus) *catalogm.Show {
	city := "Phoenix"
	state := "AZ"
	slug := fmt.Sprintf("%s-%d", title, time.Now().UnixNano())
	show := &catalogm.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: time.Now().UTC().AddDate(0, 0, 7),
		City:      &city,
		State:     &state,
		Status:    status,
	}
	s.Require().NoError(s.db.Create(show).Error)
	return show
}

// ──────────────────────────────────────────────
// Rejection-path parity — write rejects ⇔ read excludes
// ──────────────────────────────────────────────

func (s *ExploreServiceIntegrationSuite) TestPredicateParity_BillNotApproved() {
	admin := s.createAdmin("parity-bill-pending")
	pending := s.createShowWithStatus("parity-pending-bill", catalogm.ShowStatusPending)

	// Write side: a non-approved bill must be rejected up front.
	_, err := s.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, pending.ID, nil, admin.ID)
	s.Require().Error(err)
	s.True(errors.Is(err, adminsvc.ErrFeaturedSlotReferentNotApproved),
		"write side must reject a non-approved bill, got %v", err)

	// Read side: force a slot at the same referent, GetFeatured must hide it.
	s.insertActiveSlotRaw(adminm.FeaturedSlotTypeBill, pending.ID, admin.ID)
	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err)
	s.Nil(resp.Bill, "read side must exclude a non-approved bill")
}

func (s *ExploreServiceIntegrationSuite) TestPredicateParity_BillMissingReferent() {
	admin := s.createAdmin("parity-bill-missing")
	const missingID = uint(999999) // entity_id has no FK — safe to point at nothing

	_, err := s.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, missingID, nil, admin.ID)
	s.Require().Error(err)
	s.True(errors.Is(err, adminsvc.ErrFeaturedSlotReferentNotFound),
		"write side must reject a missing bill referent, got %v", err)

	s.insertActiveSlotRaw(adminm.FeaturedSlotTypeBill, missingID, admin.ID)
	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err)
	s.Nil(resp.Bill, "read side must exclude a missing bill referent")
}

func (s *ExploreServiceIntegrationSuite) TestPredicateParity_CollectionNotPublic() {
	admin := s.createAdmin("parity-coll-private")
	private := s.createCollection(admin.ID, "parity-private-coll", false)

	_, err := s.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeCollection, private.ID, nil, admin.ID)
	s.Require().Error(err)
	s.True(errors.Is(err, adminsvc.ErrFeaturedSlotReferentNotPublic),
		"write side must reject a private collection, got %v", err)

	s.insertActiveSlotRaw(adminm.FeaturedSlotTypeCollection, private.ID, admin.ID)
	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err)
	s.Nil(resp.Collection, "read side must exclude a private collection")
}

func (s *ExploreServiceIntegrationSuite) TestPredicateParity_CollectionMissingReferent() {
	admin := s.createAdmin("parity-coll-missing")
	const missingID = uint(999999)

	_, err := s.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeCollection, missingID, nil, admin.ID)
	s.Require().Error(err)
	s.True(errors.Is(err, adminsvc.ErrFeaturedSlotReferentNotFound),
		"write side must reject a missing collection referent, got %v", err)

	s.insertActiveSlotRaw(adminm.FeaturedSlotTypeCollection, missingID, admin.ID)
	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err)
	s.Nil(resp.Collection, "read side must exclude a missing collection referent")
}

// ──────────────────────────────────────────────
// Acceptance-path parity — write accepts ⇔ read surfaces
// ──────────────────────────────────────────────
//
// The valid cases guard the opposite drift: a write side that grows
// STRICTER than the read side (rejects something /explore would happily
// show) is just as much a divergence as the reverse.

func (s *ExploreServiceIntegrationSuite) TestPredicateParity_BillApprovedVisibleOnBothSides() {
	admin := s.createAdmin("parity-bill-approved")
	show, _, _ := s.createShow("parity-approved-bill", 14)

	_, err := s.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, nil, admin.ID)
	s.Require().NoError(err, "write side must accept an approved bill")

	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err)
	s.Require().NotNil(resp.Bill, "read side must surface an approved bill the write side accepted")
	s.Equal(show.ID, resp.Bill.ID)
}

func (s *ExploreServiceIntegrationSuite) TestPredicateParity_CollectionPublicVisibleOnBothSides() {
	admin := s.createAdmin("parity-coll-public")
	coll := s.createCollection(admin.ID, "parity-public-coll", true)

	_, err := s.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeCollection, coll.ID, nil, admin.ID)
	s.Require().NoError(err, "write side must accept a public collection")

	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err)
	s.Require().NotNil(resp.Collection, "read side must surface a public collection the write side accepted")
	s.Equal(coll.ID, resp.Collection.ID)
}
