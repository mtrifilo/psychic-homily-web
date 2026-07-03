package catalog

import (
	"log/slog"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1155 janitor: lifecycle reconcile (active↔dormant), play_count reconcile, and
// the end-to-end nightly cycle. Runs against the same testcontainers Postgres as
// RadioSyncSuite (methods span files).

func (s *RadioSyncSuite) reloadShow(id uint) catalogm.RadioShow {
	var sh catalogm.RadioShow
	s.Require().NoError(s.db.First(&sh, id).Error)
	return sh
}

// seedShowWithState seeds a show with an explicit lifecycle_state (seedShowFor leaves
// it at the DB default 'active').
func (s *RadioSyncSuite) seedShowWithState(stationID uint, name, slug, ext, state string) catalogm.RadioShow {
	show := catalogm.RadioShow{StationID: stationID, Name: name, Slug: slug, ExternalID: &ext, LifecycleState: state}
	s.Require().NoError(s.db.Create(&show).Error)
	return show
}

// seedEpisodeAt seeds a minimal episode on a given air date (no window needed for the
// lifecycle/play_count reconciles, which key off air_date / radio_plays).
func (s *RadioSyncSuite) seedEpisodeAt(showID uint, ext, airDate string) catalogm.RadioEpisode {
	ep := catalogm.RadioEpisode{ShowID: showID, ExternalID: &ext, AirDate: airDate}
	s.Require().NoError(s.db.Create(&ep).Error)
	return ep
}

// ReconcileShowLifecycle promotes returning shows, demotes idle/empty ones, and never
// touches a 'retired' (manual-only) show.
func (s *RadioSyncSuite) TestReconcileShowLifecycle_PromotesAndDemotes() {
	now := time.Now()
	recent := now.AddDate(0, 0, -5).Format("2006-01-02") // within 30d
	old := now.AddDate(0, 0, -45).Format("2006-01-02")   // beyond 30d
	st := s.seedBackfillStation()

	activeRecent := s.seedShowFor(st.ID, "Active Recent", "active-recent", "ext-ar") // defaults active
	s.seedEpisodeAt(activeRecent.ID, "ar-1", recent)

	activeOld := s.seedShowFor(st.ID, "Active Old", "active-old", "ext-ao")
	s.seedEpisodeAt(activeOld.ID, "ao-1", old)

	activeEmpty := s.seedShowFor(st.ID, "Active Empty", "active-empty", "ext-ae") // no episodes at all

	dormantReturning := s.seedShowWithState(st.ID, "Dormant Returning", "dormant-returning", "ext-dr", catalogm.RadioLifecycleDormant)
	s.seedEpisodeAt(dormantReturning.ID, "dr-1", recent)

	dormantStale := s.seedShowWithState(st.ID, "Dormant Stale", "dormant-stale", "ext-ds", catalogm.RadioLifecycleDormant)
	s.seedEpisodeAt(dormantStale.ID, "ds-1", old)

	retired := s.seedShowWithState(st.ID, "Retired", "retired-show", "ext-rt", catalogm.RadioLifecycleRetired)
	s.seedEpisodeAt(retired.ID, "rt-1", recent) // recent, but retired is manual-only → untouched

	promoted, demoted, err := s.svc.ReconcileShowLifecycle(30*24*time.Hour, now)
	s.Require().NoError(err)
	s.Equal(1, promoted, "only the returning dormant show is promoted")
	s.Equal(2, demoted, "the idle + empty active shows are demoted")

	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(activeRecent.ID).LifecycleState)
	s.Equal(catalogm.RadioLifecycleDormant, s.reloadShow(activeOld.ID).LifecycleState)
	s.Equal(catalogm.RadioLifecycleDormant, s.reloadShow(activeEmpty.ID).LifecycleState, "zero-episode show goes dormant")
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(dormantReturning.ID).LifecycleState)
	s.Equal(catalogm.RadioLifecycleDormant, s.reloadShow(dormantStale.ID).LifecycleState)
	s.Equal(catalogm.RadioLifecycleRetired, s.reloadShow(retired.ID).LifecycleState, "janitor never auto-touches retired")
}

// PSY-1172: setting 'retired' through the admin UpdateShow write path is sticky across a
// janitor run. The show is seeded active with only a stale episode — exactly the shape
// the janitor would demote to dormant — so a passing run proves the manual retire holds
// end-to-end (write path + janitor exclusion), not just for a directly-seeded retired row.
func (s *RadioSyncSuite) TestUpdateShow_ManualRetire_SurvivesJanitor() {
	now := time.Now()
	stale := now.AddDate(0, 0, -45).Format("2006-01-02") // beyond the 30d window
	st := s.seedBackfillStation()

	show := s.seedShowFor(st.ID, "Ended Forever", "ended-forever", "ext-ef") // defaults active
	s.seedEpisodeAt(show.ID, "ef-1", stale)                                  // stale → janitor would demote to dormant

	retired := catalogm.RadioLifecycleRetired
	resp, err := s.svc.UpdateShow(show.ID, &contracts.UpdateRadioShowRequest{LifecycleState: &retired})
	s.Require().NoError(err)
	s.Equal(catalogm.RadioLifecycleRetired, resp.LifecycleState, "UpdateShow sets retired")

	// Assert only on this show's state, not the global promoted/demoted counts — those
	// are run-wide side effects sensitive to seeded/leftover rows (the suite wipes in
	// TearDownTest, not before each test).
	_, _, err = s.svc.ReconcileShowLifecycle(30*24*time.Hour, now)
	s.Require().NoError(err)
	s.Equal(catalogm.RadioLifecycleRetired, s.reloadShow(show.ID).LifecycleState, "janitor leaves a manually-set retired untouched (not demoted to dormant)")
}

// PSY-1172 + PSY-1153: the OTHER auto-reconcile path — real-time reactivation on episode
// import — must also leave a manually-retired show alone. reactivateShowIfDormant only
// flips 'dormant' → 'active', so a new episode landing on a retired show never resurrects
// it (the WHERE guard scopes to 'dormant').
func (s *RadioSyncSuite) TestReactivateShowIfDormant_LeavesRetiredAlone() {
	now := time.Now()
	st := s.seedBackfillStation()
	show := s.seedShowWithState(st.ID, "Retired On Import", "retired-on-import", "ext-roi", catalogm.RadioLifecycleRetired)

	s.svc.reactivateShowIfDormant(show.ID, now)

	s.Equal(catalogm.RadioLifecycleRetired, s.reloadShow(show.ID).LifecycleState, "import reactivation must not resurrect a retired show")
}

// ReconcilePlayCounts corrects drifted denormalized counts (over- and under-count)
// against the real radio_plays row count, and leaves already-correct rows untouched.
func (s *RadioSyncSuite) TestReconcilePlayCounts_CorrectsDrift() {
	now := time.Now()
	today := now.Format("2006-01-02")
	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "PC Show", "pc-show", "ext-pc")

	// play_count=99 but 3 actual plays → corrected to 3.
	drift := s.seedEpisodeAt(show.ID, "pc-drift", today)
	s.Require().NoError(s.db.Model(&drift).Update("play_count", 99).Error)
	for i := 1; i <= 3; i++ {
		s.Require().NoError(s.db.Create(&catalogm.RadioPlay{EpisodeID: drift.ID, Position: i, ArtistName: "A"}).Error)
	}

	// play_count=5 but 0 plays → corrected to 0.
	phantom := s.seedEpisodeAt(show.ID, "pc-phantom", today)
	s.Require().NoError(s.db.Model(&phantom).Update("play_count", 5).Error)

	// play_count=2 with 2 plays → already correct, untouched (not counted).
	correct := s.seedEpisodeAt(show.ID, "pc-correct", today)
	s.Require().NoError(s.db.Model(&correct).Update("play_count", 2).Error)
	for i := 1; i <= 2; i++ {
		s.Require().NoError(s.db.Create(&catalogm.RadioPlay{EpisodeID: correct.ID, Position: i, ArtistName: "B"}).Error)
	}

	corrected, err := s.svc.ReconcilePlayCounts()
	s.Require().NoError(err)
	s.Equal(2, corrected, "two drifted episodes corrected; the already-correct one untouched")
	s.Equal(3, s.reloadEpisode(drift.ID).PlayCount)
	s.Equal(0, s.reloadEpisode(phantom.ID).PlayCount)
	s.Equal(2, s.reloadEpisode(correct.ID).PlayCount)
}

// End-to-end nightly cycle via RunJanitorCycleNow: demotes an idle show, corrects a
// play_count drift, and the demoted state is visible through the show API
// (lifecycle_state exposure).
func (s *RadioSyncSuite) TestJanitorCycle_EndToEnd() {
	now := time.Now()
	old := now.AddDate(0, 0, -45).Format("2006-01-02")
	today := now.Format("2006-01-02")
	st := s.seedBackfillStation()

	// (a) lifecycle: an active show with only an old episode → demoted to dormant.
	idle := s.seedShowFor(st.ID, "Idle Show", "idle-show", "ext-idle")
	s.seedEpisodeAt(idle.ID, "idle-1", old)

	// (b) play_count: an episode with a wrong count.
	pcShow := s.seedShowFor(st.ID, "PC2 Show", "pc2-show", "ext-pc2")
	pcEp := s.seedEpisodeAt(pcShow.ID, "pc2-1", today)
	s.Require().NoError(s.db.Model(&pcEp).Update("play_count", 42).Error)
	s.Require().NoError(s.db.Create(&catalogm.RadioPlay{EpisodeID: pcEp.ID, Position: 1, ArtistName: "Z"}).Error)

	// The janitor's backfill-sweep step routes through getProvider; a stub returning
	// no episodes keeps it a clean no-op (this test asserts lifecycle + play_count).
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	fetchSvc := &RadioFetchService{
		radioService:                s.svc,
		stopCh:                      make(chan struct{}),
		logger:                      slog.Default(),
		janitorDormantDays:          30,
		janitorBackfillLookbackDays: 30,
	}
	fetchSvc.RunJanitorCycleNow()

	s.Equal(catalogm.RadioLifecycleDormant, s.reloadShow(idle.ID).LifecycleState, "janitor demotes the idle show")
	s.Equal(1, s.reloadEpisode(pcEp.ID).PlayCount, "janitor corrects play_count drift")

	// lifecycle_state is surfaced through the show API (backend-only deliverable).
	resp, err := s.svc.GetShow(idle.ID)
	s.Require().NoError(err)
	s.Equal(catalogm.RadioLifecycleDormant, resp.LifecycleState)
}

// setGridSchedule stamps a minimal valid schedule JSONB on a show. On a
// scrape-capable station this makes the station schedule-authoritative (PSY-1348).
func (s *RadioSyncSuite) setGridSchedule(showID uint) {
	raw := `{"timezone":"America/New_York","slots":[{"day_of_week":1,"start":"06:00","end":"09:00"}]}`
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", showID).
		Update("schedule", raw).Error)
}


// PSY-1348: on a schedule-authoritative station (≥1 scrape-maintained schedule)
// lifecycle is grid-driven — on-grid promotes regardless of recency, off-grid demotes
// regardless of recency, schedule_locked shows keep their admin-set state, retired
// stays untouched — while a recency-semantics station in the same run keeps the old
// episode-recency behavior, even when one of its shows carries an admin-locked
// schedule.
func (s *RadioSyncSuite) TestReconcileShowLifecycle_ScheduleAuthoritativeStation() {
	now := time.Now()
	recent := now.AddDate(0, 0, -5).Format("2006-01-02")
	old := now.AddDate(0, 0, -45).Format("2006-01-02")

	grid := s.seedWFMUStation("grid-station")      // authoritative via the schedules below
	rec := s.seedBackfillStation()                            // KEXP source → recency semantics

	// On-grid, dormant, only a stale episode → promoted (grid overrides recency).
	scheduledDormant := s.seedShowWithState(grid.ID, "Scheduled Dormant", "sched-dormant", "ext-sd", catalogm.RadioLifecycleDormant)
	s.setGridSchedule(scheduledDormant.ID)
	s.seedEpisodeAt(scheduledDormant.ID, "sd-1", old)

	// On-grid, active, zero episodes → stays active (a scheduled show needs no episode).
	scheduledActive := s.seedShowFor(grid.ID, "Scheduled Active", "sched-active", "ext-sa")
	s.setGridSchedule(scheduledActive.ID)

	// Fill-in: off-grid, active, aired RECENTLY → demoted anyway (the 122-active bug).
	fillIn := s.seedShowFor(grid.ID, "Fill In", "fill-in-show", "ext-fi")
	s.seedEpisodeAt(fillIn.ID, "fi-1", recent)

	// Admin-locked shows keep their admin-set lifecycle in BOTH directions: a locked
	// dormant show is not promoted, a locked active off-grid show is not demoted.
	lockedDormant := s.seedShowWithState(grid.ID, "Locked Dormant", "locked-dormant", "ext-ld", catalogm.RadioLifecycleDormant)
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", lockedDormant.ID).
		Update("schedule_locked", true).Error)
	lockedActive := s.seedShowFor(grid.ID, "Locked Active", "locked-active", "ext-la")
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", lockedActive.ID).
		Update("schedule_locked", true).Error)

	// Off-grid but code-less (NULL or empty external_id) → exempt from the grid
	// demote (the scrape matches by code, so the grid is not authoritative for them).
	noCode := catalogm.RadioShow{StationID: grid.ID, Name: "No Code", Slug: "no-code-show"}
	s.Require().NoError(s.db.Create(&noCode).Error)
	emptyCode := s.seedShowFor(grid.ID, "Empty Code", "empty-code-show", "")

	// Retired + on-grid → untouched (retired is manual-only, both directions).
	retiredGrid := s.seedShowWithState(grid.ID, "Retired Grid", "retired-grid", "ext-rg", catalogm.RadioLifecycleRetired)
	s.setGridSchedule(retiredGrid.ID)

	// Recency station: recent episode keeps it active — the grid rule must not leak
	// onto stations with no schedule source.
	recShow := s.seedShowFor(rec.ID, "Recency Recent", "recency-recent", "ext-rr")
	s.seedEpisodeAt(recShow.ID, "rr-1", recent)

	// An admin schedule on the recency station — even UNLOCKED (the documented
	// "keep it scrape-managed" UpdateShow path) — must NOT flip the station to grid
	// semantics: authoritativeness is hard-gated on the scrape-capable provider, not
	// inferred from schedule data alone. The schedule-less sibling with recent
	// episodes (recShow above) staying active is the proof. The curated show itself
	// still lives under recency semantics, so it needs a recent episode too.
	recLocked := s.seedShowFor(rec.ID, "Recency Curated", "recency-curated", "ext-rc")
	s.setGridSchedule(recLocked.ID) // deliberately NOT locked
	s.seedEpisodeAt(recLocked.ID, "rc-1", recent)

	_, _, err := s.svc.ReconcileShowLifecycle(30*24*time.Hour, now)
	s.Require().NoError(err)

	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(scheduledDormant.ID).LifecycleState, "on-grid dormant show promoted despite stale episodes")
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(scheduledActive.ID).LifecycleState, "on-grid zero-episode show stays active")
	s.Equal(catalogm.RadioLifecycleDormant, s.reloadShow(fillIn.ID).LifecycleState, "off-grid fill-in demoted despite a recent episode")
	s.Equal(catalogm.RadioLifecycleDormant, s.reloadShow(lockedDormant.ID).LifecycleState, "locked show keeps admin-set dormant")
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(lockedActive.ID).LifecycleState, "locked off-grid show keeps admin-set active")
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(noCode.ID).LifecycleState, "NULL-code show exempt from the grid demote")
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(emptyCode.ID).LifecycleState, "empty-code show exempt from the grid demote")
	s.Equal(catalogm.RadioLifecycleRetired, s.reloadShow(retiredGrid.ID).LifecycleState, "retired untouched by the grid rule")
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(recShow.ID).LifecycleState, "recency station unaffected by the grid rule")
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(recLocked.ID).LifecycleState, "an admin schedule (even unlocked) does not flip a recency station to grid semantics")
}

// PSY-1348: the ingest-time reactivation path must agree with the janitor's grid rule —
// a fill-in's new episode on a schedule-authoritative station must NOT promote it
// (otherwise ingest and the nightly janitor flap the show active↔dormant daily).
func (s *RadioSyncSuite) TestReactivateShowIfDormant_RespectsGridRule() {
	now := time.Now()
	grid := s.seedWFMUStation("grid-station-2")
	rec := s.seedBackfillStation() // KEXP source → recency semantics

	// Make the grid station authoritative + an on-grid dormant show to promote.
	scheduled := s.seedShowWithState(grid.ID, "Sched React", "sched-react", "ext-sr", catalogm.RadioLifecycleDormant)
	s.setGridSchedule(scheduled.ID)

	fillIn := s.seedShowWithState(grid.ID, "FillIn React", "fillin-react", "ext-fr", catalogm.RadioLifecycleDormant)

	// Locked show on the authoritative station: admin-set dormant survives ingest.
	locked := s.seedShowWithState(grid.ID, "Locked React", "locked-react", "ext-lkr", catalogm.RadioLifecycleDormant)
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", locked.ID).
		Update("schedule_locked", true).Error)

	// Code-less dormant show on the authoritative station: the grid can't speak for
	// it (mirrors the grid-demote exemption), so ingest still reactivates it.
	noCode := catalogm.RadioShow{StationID: grid.ID, Name: "NoCode React",
		Slug: "nocode-react", LifecycleState: catalogm.RadioLifecycleDormant}
	s.Require().NoError(s.db.Create(&noCode).Error)

	recShow := s.seedShowWithState(rec.ID, "Recency React", "recency-react", "ext-rcr", catalogm.RadioLifecycleDormant)

	s.svc.reactivateShowIfDormant(scheduled.ID, now)
	s.svc.reactivateShowIfDormant(fillIn.ID, now)
	s.svc.reactivateShowIfDormant(locked.ID, now)
	s.svc.reactivateShowIfDormant(noCode.ID, now)
	s.svc.reactivateShowIfDormant(recShow.ID, now)

	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(scheduled.ID).LifecycleState, "on-grid dormant show reactivates on ingest")
	s.Equal(catalogm.RadioLifecycleDormant, s.reloadShow(fillIn.ID).LifecycleState, "off-grid fill-in must NOT reactivate on ingest")
	s.Equal(catalogm.RadioLifecycleDormant, s.reloadShow(locked.ID).LifecycleState, "locked show keeps admin-set dormant on ingest")
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(noCode.ID).LifecycleState, "code-less dormant show still reactivates on ingest")
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(recShow.ID).LifecycleState, "recency-station show still reactivates on ingest")
}

// PSY-1348: create-on-first must also follow station semantics — a brand-new show on a
// schedule-authoritative station is created DORMANT (most such creations are fill-ins;
// a show genuinely added to the grid promotes at the janitor run after the next
// scrape), while a recency-station show is created active (it exists only because its
// first episode is being ingested).
func (s *RadioSyncSuite) TestUpsertRadioShow_CreateLifecycleFollowsStationSemantics() {
	grid := s.seedWFMUStation("grid-station-3")
	anchor := s.seedShowFor(grid.ID, "Grid Anchor", "grid-anchor", "ext-ga")
	s.setGridSchedule(anchor.ID) // makes the station authoritative

	rec := s.seedBackfillStation() // KEXP source → recency semantics

	gridID, created, err := s.svc.upsertRadioShow(grid.ID, RadioShowImport{Name: "New Fill In", ExternalID: "ext-nfi"})
	s.Require().NoError(err)
	s.Require().True(created)
	s.Equal(catalogm.RadioLifecycleDormant, s.reloadShow(gridID).LifecycleState, "new show on an authoritative station created dormant")

	recID, created, err := s.svc.upsertRadioShow(rec.ID, RadioShowImport{Name: "New Recency Show", ExternalID: "ext-nrs"})
	s.Require().NoError(err)
	s.Require().True(created)
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(recID).LifecycleState, "new show on a recency station created active")
}
