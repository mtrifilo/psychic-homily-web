package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// dietLogLine is the shape of one sitemap_diet_omitted line, as the weekly
// revert check reads it.
type dietLogLine struct {
	Msg        string `json:"msg"`
	Rule       string `json:"rule"`
	Family     string `json:"family"`
	Shard      string `json:"shard"`
	Omitted    int    `json:"omitted"`
	Advertised int    `json:"advertised"`
}

// capturingDietLog returns a context whose request logger writes JSON into a
// buffer, and a reader for the diet lines it collected.
func capturingDietLog(t *testing.T) (context.Context, func() []dietLogLine) {
	t.Helper()
	var buf bytes.Buffer
	ctx := logger.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, nil)))
	return ctx, func() []dietLogLine {
		var lines []dietLogLine
		for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			if raw == "" {
				continue
			}
			var line dietLogLine
			if err := json.Unmarshal([]byte(raw), &line); err != nil {
				t.Fatalf("parse log line %q: %v", raw, err)
			}
			if line.Msg == "sitemap_diet_omitted" {
				lines = append(lines, line)
			}
		}
		buf.Reset()
		return lines
	}
}

func dietService(db *gorm.DB, diet sitemapDiet) *SitemapService {
	svc := NewSitemapService(db)
	svc.diet = diet
	return svc
}

// assertOmitsExactly checks that `on` is `off` minus the slugs in `omitted`,
// with every surviving entry unchanged and in the same order.
func assertOmitsExactly(t *testing.T, family string, off, on []contracts.SitemapEntry, omitted []string) {
	t.Helper()
	want := make([]contracts.SitemapEntry, 0, len(off))
	for _, e := range off {
		if !slices.Contains(omitted, e.Slug) {
			want = append(want, e)
		}
	}
	if len(want) != len(off)-len(omitted) {
		t.Fatalf("%s: rule off did not advertise every slug the test expects the rule to omit (off = %v, omitted = %v)",
			family, sitemapSlugsOf(off), omitted)
	}
	if len(on) != len(want) {
		t.Fatalf("%s: rule on = %v, want %v", family, sitemapSlugsOf(on), sitemapSlugsOf(want))
	}
	for i := range want {
		if on[i].Slug != want[i].Slug || !on[i].UpdatedAt.Equal(want[i].UpdatedAt) {
			t.Errorf("%s: rule on entry %d = %+v, want %+v", family, i, on[i], want[i])
		}
	}
}

func assertOneDietLine(t *testing.T, lines []dietLogLine, want dietLogLine) {
	t.Helper()
	if len(lines) != 1 {
		t.Fatalf("diet log lines = %+v, want exactly one", lines)
	}
	got := lines[0]
	got.Msg = ""
	if got != want {
		t.Errorf("diet log line = %+v, want %+v", got, want)
	}
}

func TestSitemapDietPastShowsOmitByVenueLocalAge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("load Phoenix: %v", err)
	}
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("load Berlin: %v", err)
	}
	phoenixRoom := &catalogm.Venue{Name: "Diet Phoenix Room", Slug: strPtr("diet-phoenix-room"), City: "Phoenix", State: "AZ", Timezone: strPtr("America/Phoenix")}
	berlinRoom := &catalogm.Venue{Name: "Diet Berlin Room", Slug: strPtr("diet-berlin-room"), City: "Berlin", Country: strPtr("Germany"), Timezone: strPtr("Europe/Berlin")}
	for _, v := range []*catalogm.Venue{phoenixRoom, berlinRoom} {
		if err := td.DB.Create(v).Error; err != nil {
			t.Fatalf("seed venue %s: %v", v.Name, err)
		}
	}

	localDate := func(loc *time.Location) string { return time.Now().In(loc).Format(time.DateOnly) }
	phoenixToday, berlinToday := localDate(phoenix), localDate(berlin)
	// daysBack days before the venue-local today, at hh:mm on that venue's clock.
	at := func(loc *time.Location, daysBack, hh, mm int) time.Time {
		y, m, d := time.Now().In(loc).Date()
		return time.Date(y, m, d-daysBack, hh, mm, 0, 0, loc)
	}

	const maxAge = 30
	seed := []struct {
		slug   string
		when   time.Time
		venue  *catalogm.Venue
		status catalogm.ShowStatus
		noSlug bool
	}{
		// The venue-local day boundary: day N-30 stays to its last minute, day
		// N-31 goes from its first.
		{slug: "diet-phx-d30-2330", when: at(phoenix, maxAge, 23, 30), venue: phoenixRoom},
		{slug: "diet-phx-d31-0030", when: at(phoenix, maxAge+1, 0, 30), venue: phoenixRoom},
		// 06:30 UTC on day N-30: a UTC-date rule would keep it.
		{slug: "diet-phx-d31-2330", when: at(phoenix, maxAge+1, 23, 30), venue: phoenixRoom},
		// The evening before day N-30 in UTC: a UTC-date rule would drop it.
		{slug: "diet-ber-d30-0030", when: at(berlin, maxAge, 0, 30), venue: berlinRoom},
		{slug: "diet-phx-old", when: at(phoenix, 400, 20, 0), venue: phoenixRoom},
		{slug: "diet-phx-recent", when: at(phoenix, 3, 20, 0), venue: phoenixRoom},
		{slug: "diet-phx-upcoming", when: at(phoenix, -10, 20, 0), venue: phoenixRoom},
		// Old, but never advertised, so never counted as omitted.
		{slug: "diet-phx-old-pending", when: at(phoenix, 400, 20, 0), venue: phoenixRoom, status: catalogm.ShowStatusPending},
		{slug: "diet-phx-old-unslugged", when: at(phoenix, 400, 20, 0), venue: phoenixRoom, noSlug: true},
	}
	for _, s := range seed {
		status := s.status
		if status == "" {
			status = catalogm.ShowStatusApproved
		}
		show := &catalogm.Show{Title: "Diet " + s.slug, EventDate: s.when.UTC(), Status: status}
		if !s.noSlug {
			show.Slug = strPtr(s.slug)
		}
		if err := td.DB.Create(show).Error; err != nil {
			t.Fatalf("seed show %s: %v", s.slug, err)
		}
		if err := td.DB.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: s.venue.ID}).Error; err != nil {
			t.Fatalf("seed show_venue %s: %v", s.slug, err)
		}
	}
	wantOmitted := []string{"diet-phx-d31-0030", "diet-phx-d31-2330", "diet-phx-old"}

	ctx, dietLines := capturingDietLog(t)

	off, err := dietService(td.DB, sitemapDiet{}).Entries(ctx, "shows")
	if err != nil {
		t.Fatalf("Entries(shows) rule off: %v", err)
	}
	if lines := dietLines(); len(lines) != 0 {
		t.Errorf("rule off logged %+v, want no diet line", lines)
	}

	svc := dietService(td.DB, sitemapDiet{pastShowMaxAgeDays: maxAge})
	on, err := svc.Entries(ctx, "shows")
	if err != nil {
		t.Fatalf("Entries(shows) rule on: %v", err)
	}
	if localDate(phoenix) != phoenixToday || localDate(berlin) != berlinToday {
		t.Skip("a venue-local date rolled over while the test ran; the seeded ages moved under it")
	}
	assertOmitsExactly(t, "shows", off.Shows, on.Shows, wantOmitted)
	assertOneDietLine(t, dietLines(), dietLogLine{
		Rule: sitemapPastShowMaxAgeDaysEnv, Family: "shows", Shard: "",
		Omitted: len(wantOmitted), Advertised: len(on.Shows),
	})

	// The buckets partition the rule the way they partition the family: their
	// union is the whole-family answer, and their logged counts sum to its count.
	var union []string
	omittedSum := 0
	for _, shard := range sitemapShardsByFamily["shows"] {
		got, err := svc.Entries(ctx, shard.id)
		if err != nil {
			t.Fatalf("Entries(%s): %v", shard.id, err)
		}
		union = append(union, sitemapSlugsOf(got.Shows)...)
		lines := dietLines()
		if len(lines) != 1 || lines[0].Shard != shard.id || lines[0].Advertised != len(got.Shows) {
			t.Fatalf("Entries(%s) logged %+v, want one line naming the shard and its advertised count %d", shard.id, lines, len(got.Shows))
		}
		omittedSum += lines[0].Omitted
	}
	slices.Sort(union)
	if want := sitemapSlugsOf(on.Shows); !slices.Equal(union, want) {
		t.Errorf("union of show buckets with the rule on = %v, want %v", union, want)
	}
	if omittedSum != len(wantOmitted) {
		t.Errorf("show buckets logged %d omitted in total, want %d", omittedSum, len(wantOmitted))
	}

	// No other family moves: the venue's year archive still carries the old show.
	venueYears, err := svc.Entries(ctx, "venue_years")
	if err != nil {
		t.Fatalf("Entries(venue_years): %v", err)
	}
	oldYear := at(phoenix, 400, 20, 0).Format("2006")
	if !slices.Contains(sitemapSlugsOf(venueYears.VenueYears), "diet-phoenix-room/shows/"+oldYear) {
		t.Errorf("venue_years = %v, want the %s archive still announced with the rule on", sitemapSlugsOf(venueYears.VenueYears), oldYear)
	}
}

func TestSitemapDietUnlinkedReleasesAreOmitted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	releases := map[string]*catalogm.Release{}
	for _, slug := range []string{"diet-rel-spotify", "diet-rel-other-platform", "diet-rel-unlinked"} {
		r := &catalogm.Release{Title: "Diet " + slug, Slug: strPtr(slug)}
		if err := td.DB.Create(r).Error; err != nil {
			t.Fatalf("seed release %s: %v", slug, err)
		}
		releases[slug] = r
	}
	if err := td.DB.Create(&catalogm.Release{Title: "Diet unslugged unlinked"}).Error; err != nil {
		t.Fatalf("seed unslugged release: %v", err)
	}
	for slug, platform := range map[string]string{"diet-rel-spotify": "spotify", "diet-rel-other-platform": "discogs"} {
		link := &catalogm.ReleaseExternalLink{ReleaseID: releases[slug].ID, Platform: platform, URL: "https://example.com/" + slug}
		if err := td.DB.Create(link).Error; err != nil {
			t.Fatalf("seed link %s: %v", slug, err)
		}
	}

	ctx, dietLines := capturingDietLog(t)
	off, err := dietService(td.DB, sitemapDiet{}).Entries(ctx, "releases")
	if err != nil {
		t.Fatalf("Entries(releases) rule off: %v", err)
	}
	if lines := dietLines(); len(lines) != 0 {
		t.Errorf("rule off logged %+v, want no diet line", lines)
	}
	on, err := dietService(td.DB, sitemapDiet{omitUnlinkedReleases: true}).Entries(ctx, "releases")
	if err != nil {
		t.Fatalf("Entries(releases) rule on: %v", err)
	}
	assertOmitsExactly(t, "releases", off.Releases, on.Releases, []string{"diet-rel-unlinked"})
	assertOneDietLine(t, dietLines(), dietLogLine{
		Rule: sitemapOmitUnlinkedReleasesEnv, Family: "releases", Shard: "",
		Omitted: 1, Advertised: 2,
	})
}

func TestSitemapDietIdleArtistsAreOmitted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	artists := map[string]*catalogm.Artist{}
	for _, slug := range []string{"diet-art-billed", "diet-art-pending-bill", "diet-art-released", "diet-art-idle"} {
		a := &catalogm.Artist{Name: "Diet " + slug, Slug: strPtr(slug)}
		if err := td.DB.Create(a).Error; err != nil {
			t.Fatalf("seed artist %s: %v", slug, err)
		}
		artists[slug] = a
	}
	if err := td.DB.Create(&catalogm.Artist{Name: "Diet unslugged idle"}).Error; err != nil {
		t.Fatalf("seed unslugged artist: %v", err)
	}
	for slug, status := range map[string]catalogm.ShowStatus{
		"diet-art-billed":       catalogm.ShowStatusApproved,
		"diet-art-pending-bill": catalogm.ShowStatusPending,
	} {
		show := &catalogm.Show{Title: "Diet bill " + slug, EventDate: time.Now().AddDate(0, 0, 5), Status: status}
		if err := td.DB.Create(show).Error; err != nil {
			t.Fatalf("seed show for %s: %v", slug, err)
		}
		if err := td.DB.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artists[slug].ID}).Error; err != nil {
			t.Fatalf("seed show_artist for %s: %v", slug, err)
		}
	}
	release := &catalogm.Release{Title: "Diet artist release", Slug: strPtr("diet-artist-release")}
	if err := td.DB.Create(release).Error; err != nil {
		t.Fatalf("seed release: %v", err)
	}
	if err := td.DB.Create(&catalogm.ArtistRelease{ArtistID: artists["diet-art-released"].ID, ReleaseID: release.ID, Role: "main"}).Error; err != nil {
		t.Fatalf("seed artist_release: %v", err)
	}

	ctx, dietLines := capturingDietLog(t)
	off, err := dietService(td.DB, sitemapDiet{}).Entries(ctx, "artists")
	if err != nil {
		t.Fatalf("Entries(artists) rule off: %v", err)
	}
	if lines := dietLines(); len(lines) != 0 {
		t.Errorf("rule off logged %+v, want no diet line", lines)
	}
	on, err := dietService(td.DB, sitemapDiet{omitIdleArtists: true}).Entries(ctx, "artists")
	if err != nil {
		t.Fatalf("Entries(artists) rule on: %v", err)
	}
	assertOmitsExactly(t, "artists", off.Artists, on.Artists, []string{"diet-art-idle"})
	assertOneDietLine(t, dietLines(), dietLogLine{
		Rule: sitemapOmitIdleArtistsEnv, Family: "artists", Shard: "",
		Omitted: 1, Advertised: 3,
	})
}

// TestSitemapDietKeepsOneQueryPerFamily holds the diet rules to the cheap-scan
// property TestSitemapEntriesIssuesOneQueryPerSimpleFamily pins with them off:
// a rule is a column of the family's one statement, never a second query.
func TestSitemapDietKeepsOneQueryPerFamily(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	if err := td.DB.Create(&catalogm.Artist{Name: "Diet counted", Slug: strPtr("diet-counted")}).Error; err != nil {
		t.Fatalf("seed artist: %v", err)
	}
	if err := td.DB.Create(&catalogm.Release{Title: "Diet counted", Slug: strPtr("diet-counted")}).Error; err != nil {
		t.Fatalf("seed release: %v", err)
	}
	show := &catalogm.Show{Title: "Diet counted", Slug: strPtr("diet-counted"), EventDate: time.Now().AddDate(0, 0, -90), Status: catalogm.ShowStatusApproved}
	if err := td.DB.Create(show).Error; err != nil {
		t.Fatalf("seed show: %v", err)
	}

	allOn := sitemapDiet{pastShowMaxAgeDays: 30, omitUnlinkedReleases: true, omitIdleArtists: true}
	for _, family := range []string{"shows", "artists", "releases", "shows-b0", "artists-b0", "releases-b0"} {
		var queries int
		counting := td.DB.Session(&gorm.Session{Logger: queryCounter{Interface: td.DB.Logger, n: &queries}})
		if _, err := dietService(counting, allOn).Entries(context.Background(), family); err != nil {
			t.Fatalf("Entries(%s): %v", family, err)
		}
		if queries != 1 {
			t.Errorf("Entries(%s) with every diet rule on issued %d queries, want 1", family, queries)
		}
	}
}
