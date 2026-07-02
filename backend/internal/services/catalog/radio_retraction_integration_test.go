package catalog

import (
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1286 retraction reconcile: a stored placeholder episode that an
// exhaustive-listing provider (WFMU) no longer publishes inside the fetch
// window is deleted; everything else — real episodes, boundary days,
// non-exhaustive providers, empty scrapes — is untouched. Runs against the
// same testcontainers Postgres as RadioSyncSuite (methods span files).

// exhaustiveMockProvider is mockPlaylistProvider plus the
// ExhaustiveEpisodeLister capability — the shape the reconcile trusts.
type exhaustiveMockProvider struct {
	mockPlaylistProvider
}

func (m *exhaustiveMockProvider) EpisodeListingIsExhaustive() bool { return true }

// seedReconcileEpisode seeds an episode with the retraction-relevant fields
// explicit (seedEpisodeAt leaves playlist_state to the DB default, which this
// suite's guards must not depend on).
func (s *RadioSyncSuite) seedReconcileEpisode(showID uint, ext, airDate, playlistState string, playCount int) catalogm.RadioEpisode {
	ep := catalogm.RadioEpisode{
		ShowID:        showID,
		ExternalID:    &ext,
		AirDate:       airDate,
		Status:        "aired",
		PlaylistState: playlistState,
		PlayCount:     playCount,
	}
	s.Require().NoError(s.db.Create(&ep).Error)
	return ep
}

func (s *RadioSyncSuite) episodeExists(id uint) bool {
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).Where("id = ?", id).Count(&n).Error)
	return n == 1
}

// The headline case, end-to-end through FetchNewEpisodes: an in-window
// trackless pending row absent from the exhaustive listing is deleted; rows
// that are present upstream, carry plays, or have a complete playlist survive.
func (s *RadioSyncSuite) TestRetractionReconcile_DeletesRetractedPlaceholderOnly() {
	st := s.seedStation(catalogm.PlaylistSourceWFMU)
	recent := time.Now().Add(-time.Hour)
	show := s.seedActiveShow(st.ID, "CK", &recent)

	day := func(offset int) string {
		return time.Now().UTC().AddDate(0, 0, offset).Format("2006-01-02")
	}

	// Absent from the listing, trackless, pending → the retracted stray.
	stray := s.seedReconcileEpisode(show.ID, "165820", day(-4), catalogm.RadioPlaylistStatePending, 0)
	// Absent from the listing but playlist already fetched complete → real.
	completed := s.seedReconcileEpisode(show.ID, "165700", day(-5), catalogm.RadioPlaylistStateComplete, 0)
	// Absent from the listing, pending, but carries a real play row → real.
	withPlays := s.seedReconcileEpisode(show.ID, "165710", day(-6), catalogm.RadioPlaylistStatePending, 1)
	s.Require().NoError(s.db.Create(&catalogm.RadioPlay{EpisodeID: withPlays.ID, Position: 1, ArtistName: "Artist"}).Error)
	// Present in the listing → obviously kept.
	kept := s.seedReconcileEpisode(show.ID, "165889", day(-3), catalogm.RadioPlaylistStatePending, 0)

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &exhaustiveMockProvider{mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "165889", ShowExternalID: "CK", AirDate: day(-3)}}, nil
			},
		}}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	_, err := s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err)

	s.False(s.episodeExists(stray.ID), "retracted trackless pending row must be deleted")
	s.True(s.episodeExists(completed.ID), "a complete-playlist row is real regardless of the listing")
	s.True(s.episodeExists(withPlays.ID), "a row with radio_plays is real regardless of the listing")
	s.True(s.episodeExists(kept.ID), "a row still in the listing is kept")
}

// Boundary days are out of reconcile reach: the `since` day itself (the parser
// can exclude it when `since` is mid-day) and anything newer than UTC-today-1
// (the provider's local today can trail UTC). Exercised via a direct call so
// the bounds are pinned against a known `since`/`now`.
func (s *RadioSyncSuite) TestRetractionReconcile_BoundaryDaysUntouched() {
	st := s.seedStation(catalogm.PlaylistSourceWFMU)
	show := s.seedActiveShow(st.ID, "CK", nil)

	now := time.Now().UTC()
	since := now.AddDate(0, 0, -7)
	day := func(offset int) string { return now.AddDate(0, 0, offset).Format("2006-01-02") }

	onSinceDay := s.seedReconcileEpisode(show.ID, "200001", day(-7), catalogm.RadioPlaylistStatePending, 0)
	inWindow := s.seedReconcileEpisode(show.ID, "200002", day(-4), catalogm.RadioPlaylistStatePending, 0)
	yesterday := s.seedReconcileEpisode(show.ID, "200003", day(-1), catalogm.RadioPlaylistStatePending, 0)
	today := s.seedReconcileEpisode(show.ID, "200004", day(0), catalogm.RadioPlaylistStatePending, 0)

	provider := &exhaustiveMockProvider{}
	upstream := []RadioEpisodeImport{{ExternalID: "unrelated", ShowExternalID: "CK", AirDate: day(-2)}}
	deleted := s.svc.reconcileRetractedEpisodes(show.ID, provider, upstream, since, now)

	s.Equal(1, deleted)
	s.True(s.episodeExists(onSinceDay.ID), "the since-boundary day is outside reconcile reach")
	s.False(s.episodeExists(inWindow.ID), "a mid-window absent placeholder is deleted")
	s.True(s.episodeExists(yesterday.ID), "yesterday can still be the provider's local today — untouched")
	s.True(s.episodeExists(today.ID), "same-day churn is never reconciled")
}

// A provider without the ExhaustiveEpisodeLister capability never authorizes
// deletion — absence from a paged/filtered listing means nothing.
func (s *RadioSyncSuite) TestRetractionReconcile_NonExhaustiveProviderIsNoop() {
	st := s.seedStation(catalogm.PlaylistSourceWFMU)
	show := s.seedActiveShow(st.ID, "CK", nil)
	stray := s.seedReconcileEpisode(show.ID, "300001", time.Now().UTC().AddDate(0, 0, -4).Format("2006-01-02"), catalogm.RadioPlaylistStatePending, 0)

	now := time.Now().UTC()
	upstream := []RadioEpisodeImport{{ExternalID: "unrelated", ShowExternalID: "CK", AirDate: now.AddDate(0, 0, -2).Format("2006-01-02")}}
	deleted := s.svc.reconcileRetractedEpisodes(show.ID, &mockPlaylistProvider{}, upstream, now.AddDate(0, 0, -7), now)

	s.Equal(0, deleted)
	s.True(s.episodeExists(stray.ID))
}

// An empty listing skips the reconcile: a parser broken by a page-layout
// change returns nothing, and that must not read as "everything retracted".
func (s *RadioSyncSuite) TestRetractionReconcile_EmptyListingSkips() {
	st := s.seedStation(catalogm.PlaylistSourceWFMU)
	show := s.seedActiveShow(st.ID, "CK", nil)
	stray := s.seedReconcileEpisode(show.ID, "400001", time.Now().UTC().AddDate(0, 0, -4).Format("2006-01-02"), catalogm.RadioPlaylistStatePending, 0)

	now := time.Now().UTC()
	deleted := s.svc.reconcileRetractedEpisodes(show.ID, &exhaustiveMockProvider{}, nil, now.AddDate(0, 0, -7), now)

	s.Equal(0, deleted)
	s.True(s.episodeExists(stray.ID))
}

// The 'unavailable' state is deletable too: a retracted playlist 404s its
// post-air playlist fetches to exhaustion, landing exactly there.
func (s *RadioSyncSuite) TestRetractionReconcile_UnavailableStateDeletable() {
	st := s.seedStation(catalogm.PlaylistSourceWFMU)
	show := s.seedActiveShow(st.ID, "CK", nil)
	stray := s.seedReconcileEpisode(show.ID, "500001", time.Now().UTC().AddDate(0, 0, -4).Format("2006-01-02"), catalogm.RadioPlaylistStateUnavailable, 0)

	now := time.Now().UTC()
	upstream := []RadioEpisodeImport{{ExternalID: "unrelated", ShowExternalID: "CK", AirDate: now.AddDate(0, 0, -2).Format("2006-01-02")}}
	deleted := s.svc.reconcileRetractedEpisodes(show.ID, &exhaustiveMockProvider{}, upstream, now.AddDate(0, 0, -7), now)

	s.Equal(1, deleted)
	s.False(s.episodeExists(stray.ID))
}
