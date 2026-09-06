package admin

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// The approve-side check is worth exactly as much as the lock it holds: it reads
// a value the same transaction is about to overwrite, so an unlocked read is a
// verdict a concurrent approval can invalidate between the check and the write.
// The submit side deliberately takes no lock, since it writes no entity row.
//
// Asserted on the emitted SQL, because the lock is a clause on a handle and
// nothing at the read itself says whether the caller attached one.
func TestOldValueReadsLockOnlyOnTheApprovePath(t *testing.T) {
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open dummy connection: %v", err)
	}
	var queries []string
	if err := db.Callback().Query().After("gorm:query").Register("capture_sql", func(tx *gorm.DB) {
		queries = append(queries, tx.Statement.SQL.String())
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}

	changes := []adminm.FieldChange{{Field: "name", OldValue: "", NewValue: "New Name"}}

	queries = nil
	if _, err := deriveOldValues(db, "artist", 7, changes); err != nil {
		t.Fatalf("deriveOldValues: %v", err)
	}
	assertLocking(t, "submit", queries, false)

	queries = nil
	if err := verifyOldValuesAtApprove(db, "artist", 7, changes); err != nil {
		t.Fatalf("verifyOldValuesAtApprove: %v", err)
	}
	assertLocking(t, "approve", queries, true)
}

func assertLocking(t *testing.T, path string, queries []string, want bool) {
	t.Helper()
	if len(queries) != 1 {
		t.Fatalf("%s path issued %d queries, want 1: %q", path, len(queries), queries)
	}
	if got := strings.Contains(queries[0], "FOR UPDATE"); got != want {
		t.Errorf("%s path FOR UPDATE = %v, want %v: %q", path, got, want, queries[0])
	}
}

// queueDescriptionEdit submits a description edit as its own user. The unique
// index admits one pending edit per submitter per entity, so two edits against
// one artist need two submitters.
func (s *PendingEditServiceIntegrationTestSuite) queueDescriptionEdit(artistID uint, from, to string) *contracts.PendingEditResponse {
	submitter := s.createTestUser()
	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artistID,
		UserID:     submitter.ID,
		Changes:    makeChanges("description", from, to),
		Summary:    "describe them",
	})
	s.Require().NoError(err)
	return resp
}

func (s *PendingEditServiceIntegrationTestSuite) artistDescription(artistID uint) string {
	var artist catalogm.Artist
	s.Require().NoError(s.db.First(&artist, artistID).Error)
	if artist.Description == nil {
		return ""
	}
	return *artist.Description
}

// The race the ticket names: two edits queued against one value, the first
// applied, the second refused rather than applied over a value its submitter
// never saw.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RefusesSupersededOldValue() {
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Two Submitters")
	s.Require().NoError(s.db.Model(artist).Update("description", "Original blurb.").Error)

	first := s.queueDescriptionEdit(artist.ID, "Original blurb.", "Mesa emo, formed 1993.")
	second := s.queueDescriptionEdit(artist.ID, "Original blurb.", "Tempe emo, formed 1994.")

	_, err := s.svc.ApprovePendingEdit(context.Background(), first.ID, reviewer.ID)
	s.Require().NoError(err)
	s.Equal("Mesa emo, formed 1993.", s.artistDescription(artist.ID))

	_, err = s.svc.ApprovePendingEdit(context.Background(), second.ID, reviewer.ID)
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditStaleValue, editErr.Code)
	s.Contains(editErr.Message, "description")
	s.Contains(editErr.Message, "since this edit was submitted",
		"the copy addresses the moderator, not the submitter")

	s.Equal("Mesa emo, formed 1993.", s.artistDescription(artist.ID),
		"the refused approval applies nothing")

	var stored adminm.PendingEntityEdit
	s.Require().NoError(s.db.First(&stored, second.ID).Error)
	s.Equal(adminm.PendingEditStatusPending, stored.Status, "a refused edit stays actionable")
	s.Nil(stored.ReviewedBy)
	s.Nil(stored.ReviewedAt)

	s.Equal("Original blurb.", s.storedChanges(second.ID)["description"].OldValue,
		"the recorded previous value is never re-stamped")
}

// The no-op variant of the same race: the second edit proposes exactly what the
// first applied, so applying it would change no column, but its recorded
// previous value is still one the entity no longer holds and Rollback would
// still restore it.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RefusesSupersededOldValueOnIdenticalNewValue() {
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Same Proposal")
	s.Require().NoError(s.db.Model(artist).Update("description", "Original blurb.").Error)

	first := s.queueDescriptionEdit(artist.ID, "Original blurb.", "Mesa emo, formed 1993.")
	second := s.queueDescriptionEdit(artist.ID, "Original blurb.", "Mesa emo, formed 1993.")

	_, err := s.svc.ApprovePendingEdit(context.Background(), first.ID, reviewer.ID)
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), second.ID, reviewer.ID)
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditStaleValue, editErr.Code)
}

// The refusal reports the entity's current value per field, which is what lets a
// client re-seed the form it composed the edit in.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_StaleRefusalCarriesCurrentValue() {
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Reports Current")
	s.Require().NoError(s.db.Model(artist).Update("description", "Original blurb.").Error)

	first := s.queueDescriptionEdit(artist.ID, "Original blurb.", "Mesa emo, formed 1993.")
	second := s.queueDescriptionEdit(artist.ID, "Original blurb.", "Tempe emo, formed 1994.")
	_, err := s.svc.ApprovePendingEdit(context.Background(), first.ID, reviewer.ID)
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), second.ID, reviewer.ID)
	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Require().Len(editErr.StaleFields, 1)
	s.Equal("description", editErr.StaleFields[0].Field)
	s.Equal("Mesa emo, formed 1993.", editErr.StaleFields[0].Current)
}

// The whole edit is refused, not the stale field alone: every field of one
// submission was composed against one reading of the entity, so applying the
// fields that still match would record a revision the submitter never proposed.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_StaleFieldRefusesTheWholeEdit() {
	submitter := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Partly Stale")
	s.Require().NoError(s.db.Model(artist).Update("description", "Original blurb.").Error)

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     submitter.ID,
		Changes: []adminm.FieldChange{
			{Field: "description", OldValue: "Original blurb.", NewValue: "A better blurb."},
			{Field: "city", OldValue: "", NewValue: "Tempe"},
		},
		Summary: "blurb and city",
	})
	s.Require().NoError(err)

	// A direct admin write moves only the description.
	s.Require().NoError(s.db.Model(artist).Update("description", "An admin got there first.").Error)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditStaleValue, editErr.Code)
	s.Contains(editErr.Message, "description")
	s.NotContains(editErr.Message, "city", "only the field that moved is named")

	var after catalogm.Artist
	s.Require().NoError(s.db.First(&after, artist.ID).Error)
	s.Equal("An admin got there first.", *after.Description)
	s.Empty(after.City, "the field that still matched is not applied either")
}

// The ordinary case still approves: nothing moved between submission and
// approval, so the gate is invisible.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_UnchangedEntityStillApplies() {
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Undisturbed")
	s.Require().NoError(s.db.Model(artist).Update("description", "Original blurb.").Error)

	created := s.queueDescriptionEdit(artist.ID, "Original blurb.", "Mesa emo, formed 1993.")

	resp, err := s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)
	s.Equal(adminm.PendingEditStatusApproved, adminm.PendingEditStatus(resp.Status))
	s.Equal("Mesa emo, formed 1993.", s.artistDescription(artist.ID))
}

// A withheld column derives as its withheld view on both sides, so a queued
// venue-address edit stays approvable however the column moves underneath it.
// Comparing the column instead would refuse every such edit and publish the
// value in the refusal.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_WithheldAddressIsNotStale() {
	submitter := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("House Show")
	s.Require().NoError(s.db.Model(venue).Update("address", "123 Real St").Error)

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     submitter.ID,
		Changes:    []adminm.FieldChange{{Field: "address", OldValue: nil, NewValue: "456 New St"}},
		Summary:    "they moved",
	})
	s.Require().NoError(err)

	s.Require().NoError(s.db.Model(venue).Update("address", "789 Somewhere Else Rd").Error)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err, "an unreadable column cannot produce a stale-value refusal")

	var after catalogm.Venue
	s.Require().NoError(s.db.First(&after, venue.ID).Error)
	s.Require().NotNil(after.Address)
	s.Equal("456 New St", *after.Address)
}

// An entity deleted between submission and approval is the approve path's
// ENTITY_GONE (422), not the submit path's 404, even though the read that
// reports it now happens before the update.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_DeletedEntityIsGoneNotNotFound() {
	submitter := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Will Vanish")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     submitter.ID,
		Changes:    makeChanges("name", "Will Vanish", "New Name"),
		Summary:    "rename",
	})
	s.Require().NoError(err)
	s.Require().NoError(s.db.Delete(artist).Error)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditEntityGone, editErr.Code)
}

// The same race with the two approvals dispatched together. Exactly one may
// apply.
//
// The property, not the lock: whether the two transactions actually interleave
// is not under this test's control, so what pins the lock is
// TestOldValueReadsLockOnlyOnTheApprovePath, which reads the emitted SQL.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_ConcurrentApprovalsApplyOnce() {
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Concurrent")
	s.Require().NoError(s.db.Model(artist).Update("description", "Original blurb.").Error)

	first := s.queueDescriptionEdit(artist.ID, "Original blurb.", "Mesa emo, formed 1993.")
	second := s.queueDescriptionEdit(artist.ID, "Original blurb.", "Tempe emo, formed 1994.")

	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, editID := range []uint{first.ID, second.ID} {
		wg.Add(1)
		go func(slot int, id uint) {
			defer wg.Done()
			<-start
			_, err := s.svc.ApprovePendingEdit(context.Background(), id, reviewer.ID)
			errs[slot] = err
		}(i, editID)
	}
	close(start)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		s.FailNow("concurrent approvals did not finish; the row lock is deadlocked")
	}

	applied := 0
	for _, err := range errs {
		if err == nil {
			applied++
			continue
		}
		var editErr *apperrors.PendingEditError
		s.Require().ErrorAs(err, &editErr)
		s.Equal(apperrors.CodePendingEditStaleValue, editErr.Code)
	}
	s.Equal(1, applied, "exactly one of two edits queued against the same value may apply")

	description := s.artistDescription(artist.ID)
	s.Contains([]string{"Mesa emo, formed 1993.", "Tempe emo, formed 1994."}, description)
}

// Two fields moving at once: the message names both, sorted, and the refusal
// reports a current value for each. The plural copy and the ordering have no
// other coverage.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_ReportsEveryStaleField() {
	submitter := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Two Fields")
	s.Require().NoError(s.db.Model(artist).Updates(map[string]interface{}{
		"description": "Original blurb.",
		"website":     "https://old.example.org",
	}).Error)

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     submitter.ID,
		Changes: []adminm.FieldChange{
			{Field: "website", OldValue: "https://old.example.org", NewValue: "https://new.example.org"},
			{Field: "description", OldValue: "Original blurb.", NewValue: "A better blurb."},
		},
		Summary: "site and blurb",
	})
	s.Require().NoError(err)

	s.Require().NoError(s.db.Model(artist).Updates(map[string]interface{}{
		"description": "Somebody else's blurb.",
		"website":     "https://somewhere.else.example.org",
	}).Error)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Contains(editErr.Message, "These fields changed", "two stale fields take the plural copy")
	s.Contains(editErr.Message, "description, website", "the message lists them sorted")
	s.Contains(editErr.Message, "different values")

	current := map[string]interface{}{}
	for _, f := range editErr.StaleFields {
		current[f.Field] = f.Current
	}
	s.Equal(map[string]interface{}{
		"description": "Somebody else's blurb.",
		"website":     "https://somewhere.else.example.org",
	}, current)
}

// A numeric column is the one place the recorded claim and the derived value
// reach the comparison in different encodings: the stored old_value round-trips
// through JSONB as a float64 while the column yields an int. An ordinary
// capacity edit must approve, and a superseded one must refuse.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_ComparesNumericOldValue() {
	submitter := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Numeric Approve")
	s.Require().NoError(s.db.Model(venue).Update("capacity", 550).Error)

	queue := func(userID uint) uint {
		resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
			EntityType: "venue",
			EntityID:   venue.ID,
			UserID:     userID,
			Changes:    []adminm.FieldChange{{Field: "capacity", OldValue: float64(550), NewValue: float64(600)}},
			Summary:    "bigger room",
		})
		s.Require().NoError(err)
		return resp.ID
	}
	first := queue(submitter.ID)
	second := queue(s.createTestUser().ID)

	_, err := s.svc.ApprovePendingEdit(context.Background(), first, reviewer.ID)
	s.Require().NoError(err, "a numeric claim that still matches must not read as stale")

	var applied catalogm.Venue
	s.Require().NoError(s.db.First(&applied, venue.ID).Error)
	s.Require().NotNil(applied.Capacity)
	s.Equal(600, *applied.Capacity)

	_, err = s.svc.ApprovePendingEdit(context.Background(), second, reviewer.ID)
	s.Require().Error(err)
	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditStaleValue, editErr.Code)
}

// Verifying a venue publishes its address, which moves the DERIVED value for a
// queued address edit from the withheld blank to the column. The gate cannot
// tell that from the column having moved, so the edit becomes unapprovable and
// the moderator has to reject it.
//
// That is the conservative answer rather than the desirable one, and it is
// pinned here so a change to it is deliberate: applying the edit would record a
// previous value of "" against a column that held a street address, and Rollback
// writes OldValue back verbatim.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_VerifyingVenueStrandsQueuedAddressEdit() {
	submitter := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Becomes Verified")
	s.Require().NoError(s.db.Model(venue).Update("address", "123 Real St").Error)

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     submitter.ID,
		Changes:    []adminm.FieldChange{{Field: "address", OldValue: nil, NewValue: "456 New St"}},
		Summary:    "they moved",
	})
	s.Require().NoError(err)
	s.Equal("", s.storedChanges(created.ID)["address"].OldValue)

	s.Require().NoError(s.db.Model(venue).Update("verified", true).Error)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditStaleValue, editErr.Code)

	var after catalogm.Venue
	s.Require().NoError(s.db.First(&after, venue.ID).Error)
	s.Require().NotNil(after.Address)
	s.Equal("123 Real St", *after.Address)
}

// A row that predates the derivation carries an unvetted previous value, and the
// gate refuses it for the same reason it refuses a superseded one: nothing
// distinguishes the two, and applying either records a previous value nobody
// observed.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RefusesLegacyPlantedOldValue() {
	submitter := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Legacy Row")
	s.Require().NoError(s.db.Model(artist).Update("description", "Real blurb.").Error)

	edit := s.insertCorruptPendingEdit("artist", artist.ID, submitter.ID,
		makeChanges("description", "https://evil.test/planted", "A new blurb."), "legacy row")

	_, err := s.svc.ApprovePendingEdit(context.Background(), edit.ID, reviewer.ID)
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditStaleValue, editErr.Code)
	s.Equal("Real blurb.", s.artistDescription(artist.ID))
}
