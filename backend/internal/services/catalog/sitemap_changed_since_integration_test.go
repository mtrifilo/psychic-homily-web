package catalog

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// changedSinceFixture seeds a catalogue whose every row was last changed an
// hour before mark, then changes some rows after it through the paths that
// production uses, and returns mark.
type changedSinceFixture struct {
	db    *gorm.DB
	shows *ShowService
	user  *authm.User
}

func (f changedSinceFixture) createShow(t *testing.T, title, venue, artist string) *contracts.ShowResponse {
	t.Helper()
	resp, err := f.shows.CreateShow(&contracts.CreateShowRequest{
		Title:             title,
		EventDate:         time.Date(2026, 12, 15, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: venue, City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: artist, IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &f.user.ID,
		SubmitterIsAdmin:  true,
	})
	if err != nil {
		t.Fatalf("create show %s: %v", title, err)
	}
	return resp
}

func (f changedSinceFixture) createRelease(t *testing.T, slug string) *catalogm.Release {
	t.Helper()
	release := &catalogm.Release{Title: slug, Slug: stringPtr(slug)}
	if err := f.db.Create(release).Error; err != nil {
		t.Fatalf("create release %s: %v", slug, err)
	}
	return release
}

func (f changedSinceFixture) linkRelease(t *testing.T, release *catalogm.Release, at time.Time) {
	t.Helper()
	link := &catalogm.ReleaseExternalLink{
		ReleaseID: release.ID,
		Platform:  "bandcamp",
		URL:       fmt.Sprintf("https://example.bandcamp.com/album/%d-%d", release.ID, at.UnixNano()),
		CreatedAt: at,
	}
	if err := f.db.Create(link).Error; err != nil {
		t.Fatalf("link release %d: %v", release.ID, err)
	}
}

// backdate moves every row of the four families, and every release link, to
// at, so only what the test changes afterwards is later than the mark.
func (f changedSinceFixture) backdate(t *testing.T, at time.Time) {
	t.Helper()
	for _, stmt := range []string{
		"UPDATE shows SET updated_at = ?",
		"UPDATE artists SET updated_at = ?",
		"UPDATE venues SET updated_at = ?",
		"UPDATE releases SET updated_at = ?",
		"UPDATE release_external_links SET created_at = ?",
	} {
		if err := f.db.Exec(stmt, at).Error; err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func newChangedSinceFixture(t *testing.T, db *gorm.DB) changedSinceFixture {
	t.Helper()
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("changed-since-%d@test.com", time.Now().UnixNano())),
		IsActive:      true,
		EmailVerified: true,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return changedSinceFixture{db: db, shows: NewShowService(db), user: user}
}

func slugsOf(entries []contracts.SitemapEntry) []string {
	slugs := make([]string, 0, len(entries))
	for _, e := range entries {
		slugs = append(slugs, e.Slug)
	}
	slices.Sort(slugs)
	return slugs
}

func sortedSlugs(slugs ...string) []string {
	out := slices.Clone(slugs)
	slices.Sort(out)
	return out
}

// A show whose bill changes after the mark is returned with the artist that
// change created; an untouched show, a pending show and an artist that kept
// its bill are not. A release is returned when its newest link is after the
// mark, a venue when its row is.
func TestSitemapEntriesChangedSinceFollowsEachFamilyLastmod(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()
	f := newChangedSinceFixture(t, td.DB)

	rebilled := f.createShow(t, "Rebilled Show", "Rebilled Room", "Kept Headliner")
	untouched := f.createShow(t, "Untouched Show", "Untouched Room", "Untouched Headliner")
	pending := f.createShow(t, "Pending Show", "Pending Room", "Pending Headliner")
	if err := td.DB.Model(&catalogm.Show{}).Where("id = ?", pending.ID).
		Update("status", catalogm.ShowStatusPending).Error; err != nil {
		t.Fatalf("mark show pending: %v", err)
	}
	linkedLater := f.createRelease(t, "linked-after-the-mark")
	linkedBefore := f.createRelease(t, "linked-before-the-mark")
	f.linkRelease(t, linkedBefore, time.Now())

	mark := time.Now().Add(-time.Minute)
	f.backdate(t, mark.Add(-time.Hour))

	// After the mark: a bill change, a new release link, a venue edit, and an
	// edit to the pending show.
	kept := rebilled.Artists[0]
	if _, _, err := f.shows.UpdateShowWithRelations(rebilled.ID, nil, nil, []contracts.CreateShowArtist{
		{ID: &kept.ID, Name: kept.Name, IsHeadliner: boolPtr(true)},
		{Name: "Fresh Opener", IsHeadliner: boolPtr(false)},
	}, true); err != nil {
		t.Fatalf("rebill show: %v", err)
	}
	f.linkRelease(t, linkedLater, time.Now())
	if err := td.DB.Exec("UPDATE venues SET updated_at = NOW() WHERE id = ?", untouched.Venues[0].ID).Error; err != nil {
		t.Fatalf("edit venue: %v", err)
	}
	if err := td.DB.Exec("UPDATE shows SET updated_at = NOW() WHERE id = ?", pending.ID).Error; err != nil {
		t.Fatalf("edit pending show: %v", err)
	}

	svc := dietService(td.DB, sitemapDiet{})
	got, err := svc.EntriesChangedSince(context.Background(), mark)
	if err != nil {
		t.Fatalf("EntriesChangedSince: %v", err)
	}

	var rebilledSlug, freshOpenerSlug, untouchedVenueSlug string
	if err := td.DB.Raw("SELECT slug FROM shows WHERE id = ?", rebilled.ID).Scan(&rebilledSlug).Error; err != nil {
		t.Fatalf("read show slug: %v", err)
	}
	if err := td.DB.Raw("SELECT slug FROM artists WHERE name = ?", "Fresh Opener").Scan(&freshOpenerSlug).Error; err != nil {
		t.Fatalf("read artist slug: %v", err)
	}
	if err := td.DB.Raw("SELECT slug FROM venues WHERE id = ?", untouched.Venues[0].ID).Scan(&untouchedVenueSlug).Error; err != nil {
		t.Fatalf("read venue slug: %v", err)
	}

	if want := sortedSlugs(rebilledSlug); !slices.Equal(slugsOf(got.Shows), want) {
		t.Errorf("shows = %v, want %v", slugsOf(got.Shows), want)
	}
	if want := sortedSlugs(freshOpenerSlug); !slices.Equal(slugsOf(got.Artists), want) {
		t.Errorf("artists = %v, want %v", slugsOf(got.Artists), want)
	}
	if want := sortedSlugs(untouchedVenueSlug); !slices.Equal(slugsOf(got.Venues), want) {
		t.Errorf("venues = %v, want %v", slugsOf(got.Venues), want)
	}
	if want := sortedSlugs("linked-after-the-mark"); !slices.Equal(slugsOf(got.Releases), want) {
		t.Errorf("releases = %v, want %v", slugsOf(got.Releases), want)
	}
	for name, family := range map[string][]contracts.SitemapEntry{
		"venue_years": got.VenueYears, "shows_months": got.ShowsMonths, "venue_cities": got.VenueCities,
		"scenes": got.Scenes, "scene_weeks": got.SceneWeeks, "scene_periods": got.ScenePeriods,
		"labels": got.Labels, "festivals": got.Festivals, "tags": got.Tags,
	} {
		if family == nil || len(family) != 0 {
			t.Errorf("%s = %v, want an empty slice", name, family)
		}
	}
}

// For each family, the changed set is exactly the sitemap's own entries whose
// lastmod is later than the mark, with every diet rule off and on, and a
// changed-since call writes no diet log line.
func TestSitemapEntriesChangedSinceIsTheSitemapFilteredByLastmod(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()
	f := newChangedSinceFixture(t, td.DB)

	old := f.createShow(t, "Old Show", "Old Room", "Old Headliner")
	f.createShow(t, "Recent Show", "Recent Room", "Recent Headliner")
	linked := f.createRelease(t, "recent-linked")
	f.linkRelease(t, linked, time.Now())
	f.createRelease(t, "recent-unlinked")
	if err := td.DB.Create(&catalogm.Artist{Name: "Recent Idle", Slug: stringPtr("recent-idle")}).Error; err != nil {
		t.Fatalf("create idle artist: %v", err)
	}

	mark := time.Now().Add(-time.Minute)
	f.backdate(t, mark.Add(-time.Hour))
	for _, stmt := range []string{
		"UPDATE shows SET updated_at = NOW() WHERE title = 'Recent Show'",
		"UPDATE artists SET updated_at = NOW() WHERE name IN ('Recent Headliner', 'Recent Idle')",
		"UPDATE venues SET updated_at = NOW() WHERE name = 'Recent Room'",
		"UPDATE releases SET updated_at = NOW() WHERE slug IN ('recent-linked', 'recent-unlinked')",
		// Two years in the past, so the past-show rule omits it once it is on.
		"UPDATE shows SET event_date = NOW() - INTERVAL '730 days', updated_at = NOW() WHERE id = " + fmt.Sprint(old.ID),
	} {
		if err := td.DB.Exec(stmt).Error; err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	for _, diet := range []sitemapDiet{
		{},
		{pastShowMaxAgeDays: 365, omitUnlinkedReleases: true, omitIdleArtists: true},
	} {
		t.Run(fmt.Sprintf("diet %+v", diet), func(t *testing.T) {
			svc := dietService(td.DB, diet)
			full, err := svc.Entries(context.Background(), "")
			if err != nil {
				t.Fatalf("Entries: %v", err)
			}
			ctx, dietLines := capturingDietLog(t)
			changed, err := svc.EntriesChangedSince(ctx, mark)
			if err != nil {
				t.Fatalf("EntriesChangedSince: %v", err)
			}
			if lines := dietLines(); len(lines) != 0 {
				t.Errorf("changed-since call logged %d sitemap_diet_omitted lines, want 0", len(lines))
			}
			after := func(entries []contracts.SitemapEntry) []string {
				var kept []contracts.SitemapEntry
				for _, e := range entries {
					if e.UpdatedAt.After(mark) {
						kept = append(kept, e)
					}
				}
				return slugsOf(kept)
			}
			for _, family := range []struct {
				name          string
				full, changed []contracts.SitemapEntry
			}{
				{"shows", full.Shows, changed.Shows},
				{"artists", full.Artists, changed.Artists},
				{"venues", full.Venues, changed.Venues},
				{"releases", full.Releases, changed.Releases},
			} {
				want := after(family.full)
				if len(want) == 0 {
					t.Errorf("%s: fixture has no changed entry to compare", family.name)
				}
				if got := slugsOf(family.changed); !slices.Equal(got, want) {
					t.Errorf("%s: changed = %v, sitemap entries after the mark = %v", family.name, got, want)
				}
			}
		})
	}
}
