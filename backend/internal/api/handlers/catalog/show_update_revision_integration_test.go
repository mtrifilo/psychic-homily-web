package catalog

import (
	"fmt"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/admin"
)

// handlerWithRevisions is the suite's handler with a real revision writer, so
// a save's revision row and its updated_at can be read back together.
func (s *ShowHandlerIntegrationSuite) handlerWithRevisions() *ShowHandler {
	return NewShowHandler(
		s.deps.ShowService,
		s.deps.ShowService,
		s.deps.ShowService,
		s.deps.SavedShowService,
		s.deps.DiscordService,
		s.deps.ExtractionService,
		admin.NewRevisionService(s.deps.DB),
		testhelpers.AllShowsVisible(),
	)
}

func (s *ShowHandlerIntegrationSuite) showRevisionFields(showID uint) []string {
	var fields []string
	s.Require().NoError(s.deps.DB.Raw(
		`SELECT e->>'field' FROM revisions r, jsonb_array_elements(r.field_changes) e
		 WHERE r.entity_type = 'show' AND r.entity_id = ? ORDER BY r.id, e->>'field'`, showID,
	).Scan(&fields).Error)
	return fields
}

// Two identical saves leave updated_at alone; a save that changes music_at
// moves it and writes one revision naming music_at. The revision assertion
// confirms the changed path end to end; that an identical save writes no
// revision follows from revisiondiff.Compare finding no difference, which this
// test cannot order against the off-request revision writes.
func (s *ShowHandlerIntegrationSuite) TestUpdateShow_UnchangedSaveKeepsUpdatedAt() {
	handler := s.handlerWithRevisions()
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Restated Show")
	pinned := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s.Require().NoError(s.deps.DB.Exec("UPDATE shows SET updated_at = ? WHERE id = ?", pinned, show.ID).Error)
	storedUpdatedAt := func() time.Time {
		var row catalogm.Show
		s.Require().NoError(s.deps.DB.First(&row, show.ID).Error)
		return row.UpdatedAt
	}

	ctx := testhelpers.CtxWithUser(user)
	for i := 0; i < 2; i++ {
		req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
		req.Body.Title = &show.Title
		req.Body.EventDate = &show.EventDate
		req.Body.City = show.City
		req.Body.State = show.State
		_, err := handler.UpdateShowHandler(ctx, req)
		s.Require().NoError(err)
		s.True(pinned.Equal(storedUpdatedAt()), "identical save %d must not move updated_at", i+1)
	}

	music := show.EventDate.Add(time.Hour)
	changed := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	changed.Body.MusicAt = shared.NullableInputSet(music)
	_, err := handler.UpdateShowHandler(ctx, changed)
	s.Require().NoError(err)
	s.True(storedUpdatedAt().After(pinned), "a save that changes music_at must move updated_at")

	// The revision is written off the request goroutine.
	s.Eventually(func() bool { return len(s.showRevisionFields(show.ID)) > 0 }, 5*time.Second, 20*time.Millisecond)
	s.Equal([]string{"music_at"}, s.showRevisionFields(show.ID), "the music_at save writes one revision naming music_at")
}
