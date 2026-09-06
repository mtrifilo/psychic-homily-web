package catalog

import (
	"context"
	"strings"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
)

// seedCrossSurfaceScenes builds the corpus every scene surface is asserted over
// below: one healthy metro scene, both shapes of metro drift, the spelling
// collision that needs no drift at all, and a city too small to be a scene.
//
// The shapes are seeded together on purpose. Each one resolves through a
// different branch of the slug/scope resolution, and a corpus holding one at a
// time cannot catch a branch that answers for the wrong group.
func (suite *SceneServiceIntegrationTestSuite) seedCrossSurfaceScenes() {
	user := suite.createUser()
	band := suite.createArtist("Local Band")

	// Midweek of the CURRENT ISO week, at midday UTC. The week assertions read
	// the current week, and the sitemap's week window ends at the current
	// week's end, so a fixture anchored on "now plus a few days" covers them
	// only early in the week. Midday UTC on Wednesday and Thursday is inside
	// the same ISO week for every North American zone the corpus uses.
	nowUTC := time.Now().UTC()
	y, w := nowUTC.ISOWeek()
	midweek := ISOWeekStart(y, w, time.UTC).AddDate(0, 0, 2).Add(12 * time.Hour)

	shows := func(prefix string, rooms ...*catalogm.Venue) {
		for i := 0; i < sceneMinShows; i++ {
			suite.createApprovedShow(prefix, rooms[i%len(rooms)].ID, band.ID, user.ID, midweek.Add(time.Duration(i)*6*time.Hour))
		}
	}

	// A healthy metro scene, rolled up from its principal city and a member
	// city: the shape the geo dataset alone resolves correctly.
	phoenixA := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	mesaA := suite.createVerifiedVenue("Nile Theater", "Mesa", "AZ")
	suite.Require().NotNil(phoenixA.Metro, "the healthy fixture must carry the CBSA")
	shows("Metro", phoenixA, mesaA)

	// Drift against that same metro: rooms in the principal city with no
	// venues.metro. They group apart from the CBSA rooms and publish the same
	// slug, which the directory's collapse settles in the CBSA group's favour.
	driftedPhoenixA := suite.createVerifiedVenueNullMetro("Drifted Phoenix A", "Phoenix", "AZ")
	driftedPhoenixB := suite.createVerifiedVenueNullMetro("Drifted Phoenix B", "Phoenix", "AZ")
	shows("Drifted Phoenix", driftedPhoenixA, driftedPhoenixB)

	// Drift with nothing to collide with: every verified room of a CBSA-pinning
	// PRINCIPAL city carries a NULL metro, so the scene the directory lists is
	// the fallback group and the metro holds no rooms at all.
	flagstaffA := suite.createVerifiedVenueNullMetro("Orpheum Flagstaff", "Flagstaff", "AZ")
	flagstaffB := suite.createVerifiedVenueNullMetro("Green Room", "Flagstaff", "AZ")
	shows("Flagstaff", flagstaffA, flagstaffB)

	// The same drift in a metro MEMBER city, where the geo reading answers with
	// a principal city (Phoenix) the rooms are not in and the slug the
	// directory published (tempe-az) never names.
	tempeA := suite.createVerifiedVenueNullMetro("Yucca Tap Room", "Tempe", "AZ")
	tempeB := suite.createVerifiedVenueNullMetro("Marquee", "Tempe", "AZ")
	shows("Tempe", tempeA, tempeB)

	// PARTIAL drift, which is what an interrupted reconcile leaves: one room of
	// a city keeps its CBSA while the rest lose it. The metro group is below
	// the venue floor, so the drifted group is what the directory publishes,
	// and the room still carrying the CBSA belongs to its metro's group rather
	// than to this scene.
	driftedTucsonA := suite.createVerifiedVenueNullMetro("Drifted Tucson A", "Tucson", "AZ")
	driftedTucsonB := suite.createVerifiedVenueNullMetro("Drifted Tucson B", "Tucson", "AZ")
	pinnedTucson := suite.createVerifiedVenue("Club Congress", "Tucson", "AZ")
	suite.Require().NotNil(pinnedTucson.Metro, "the partial-drift fixture needs one room still carrying the CBSA")
	shows("Tucson", driftedTucsonA, driftedTucsonB)

	// Two spellings of one non-US city: two groups under one slug with no drift
	// involved, since the group key only lower/trims while the slug also maps
	// spaces to dashes (PSY-1981).
	spacedA := suite.createVerifiedVenue("Spaced A", "Saint Jerome", "QC")
	spacedB := suite.createVerifiedVenue("Spaced B", "Saint Jerome", "QC")
	suite.Require().Nil(spacedA.Metro, "a non-US city must not pin a CBSA")
	shows("Spaced", spacedA, spacedB)
	hyphenA := suite.createVerifiedVenue("Hyphen A", "Saint-Jerome", "QC")
	hyphenB := suite.createVerifiedVenue("Hyphen B", "Saint-Jerome", "QC")
	shows("Hyphen", hyphenA, hyphenB)

	// A collision the SHOW floor decides. The spelling with more rooms has no
	// shows, so the directory publishes the other one, and the spelling with
	// more rooms is the one a venue-count comparison alone would pick.
	suite.createVerifiedVenue("Quiet A", "Val Dor", "QC")
	suite.createVerifiedVenue("Quiet B", "Val Dor", "QC")
	quietC := suite.createVerifiedVenue("Quiet C", "Val Dor", "QC")
	suite.Require().Nil(quietC.Metro, "a non-US city must not pin a CBSA")
	bookedA := suite.createVerifiedVenue("Booked A", "Val-Dor", "QC")
	bookedB := suite.createVerifiedVenue("Booked B", "Val-Dor", "QC")
	shows("Val-Dor", bookedA, bookedB)

	// Below the venue floor, drifted as well: it must stay off every surface.
	sedona := suite.createVerifiedVenueNullMetro("Sound Bites", "Sedona", "AZ")
	shows("Sedona", sedona)

	// A spelling collision whose halves are one room each: the slug matches two
	// rooms and no group clears the floor, so no scene exists under it and the
	// page the slug resolves to holds one room.
	trois := suite.createVerifiedVenue("Trois A", "Trois Rivieres", "QC")
	suite.Require().Nil(trois.Metro, "a non-US city must not pin a CBSA")
	suite.createVerifiedVenue("Trois B", "Trois-Rivieres", "QC")
}

// TestEveryListedSceneResolvesOnItsDetailRoute is the cross-surface property:
// a slug the scenes directory publishes has a page, that page is built from the
// rooms the directory counted, and the existence probe the frontend proxy
// soft-404s on agrees that it is there.
//
// It asserts over whatever the directory returns rather than over a list of
// expected slugs, so a shape added to the corpus is covered by construction.
func (suite *SceneServiceIntegrationTestSuite) TestEveryListedSceneResolvesOnItsDetailRoute() {
	suite.seedCrossSurfaceScenes()
	existence := NewEntityExistenceService(suite.db, suite.sceneService)

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().NotEmpty(scenes, "the corpus must list scenes, or this property passes vacuously")

	slugs := map[string]int{}
	for _, listed := range scenes {
		slugs[listed.Slug]++
	}
	for slug, n := range slugs {
		suite.Equal(1, n, "one directory row per scene slug: %s", slug)
	}

	for _, listed := range scenes {
		city, state, err := suite.sceneService.ParseSceneSlug(listed.Slug)
		suite.Require().NoError(err, "the directory published %s", listed.Slug)
		suite.Equal(listed.Slug, buildSceneSlug(city, state),
			"%s resolves to an identity that names another page", listed.Slug)

		detail, err := suite.sceneService.GetSceneDetail(city, state)
		suite.Require().NoError(err, "the detail route 404s a slug the directory lists: %s", listed.Slug)
		suite.Equal(listed.Slug, detail.Slug)
		suite.Equal(listed.VenueCount, detail.Stats.VenueCount,
			"%s prints a room count the directory contradicts", listed.Slug)
		suite.Require().Len(detail.Venues, listed.VenueCount,
			"%s lists rooms its own count disagrees with", listed.Slug)

		exists, err := existence.Exists("scenes", listed.Slug)
		suite.Require().NoError(err)
		suite.True(exists, "the proxy existence gate soft-404s a scene the directory lists: %s", listed.Slug)

		week, err := suite.sceneService.GetSceneWeek(city, state, ISOWeekKey(time.Now().UTC()))
		suite.Require().NoError(err, "the week permalink 404s for a scene the directory lists: %s", listed.Slug)
		suite.Equal(listed.Slug, buildSceneSlug(week.City, week.State))

		// The seam under all of the above: the scope the page queries through
		// holds the rooms the directory counted.
		scope, err := suite.sceneService.scopeFor(listed.City, listed.State)
		suite.Require().NoError(err)
		count, err := suite.sceneService.verifiedVenueCount(scope)
		suite.Require().NoError(err)
		suite.Equal(int64(listed.VenueCount), count,
			"%s is scoped to a room set the directory did not count", listed.Slug)
	}

	suite.NotContains(slugs, "sedona-az", "a city below the venue floor is not a scene")
	suite.NotContains(slugs, "trois-rivieres-qc", "two spellings of one room each are not a scene")

	// The floor still gates: an identity whose own rooms are one, and whose
	// metro holds none, has no page. Addressed as the identity it is, since a
	// route reaches it through ParseSceneSlug, which canonicalizes sedona-az
	// onto the metro's principal city instead.
	_, err = suite.sceneService.GetSceneDetail("Sedona", "AZ")
	suite.Error(err, "the venue floor still gates the detail page")
}

// TestSitemapSceneEntriesResolveOnTheDetailRoute holds the same property for
// the third surface. The sitemap publishes scene URLs to crawlers from its own
// grouping, so a URL it announces that the detail route refuses is a first
// party link into a 404.
func (suite *SceneServiceIntegrationTestSuite) TestSitemapSceneEntriesResolveOnTheDetailRoute() {
	suite.seedCrossSurfaceScenes()

	entries, err := NewSitemapService(suite.db).Entries(context.Background(), "")
	suite.Require().NoError(err)
	suite.Require().NotEmpty(entries.Scenes, "the corpus must announce scenes, or this property passes vacuously")

	for _, entry := range entries.Scenes {
		city, state, err := suite.sceneService.ParseSceneSlug(entry.Slug)
		suite.Require().NoError(err, "the sitemap announces %s", entry.Slug)
		detail, err := suite.sceneService.GetSceneDetail(city, state)
		suite.Require().NoError(err, "the sitemap announces a scene the detail route 404s: %s", entry.Slug)
		suite.Equal(entry.Slug, detail.Slug)
	}

	// Every week permalink names a scene root the same document announces, and
	// every week it names is served by the page that root points at.
	roots := map[string]bool{}
	for _, entry := range entries.Scenes {
		roots[entry.Slug] = true
	}
	suite.Require().NotEmpty(entries.SceneWeeks,
		"the corpus must announce week permalinks, or the loop below passes vacuously")
	announced := map[string]bool{}
	for _, entry := range entries.SceneWeeks {
		slug, weekKey, ok := strings.Cut(entry.Slug, "/")
		suite.Require().True(ok, "a week entry names a scene and a week: %s", entry.Slug)
		suite.True(roots[slug], "the weeks document names a scene the roots document omits: %s", slug)

		city, state, err := suite.sceneService.ParseSceneSlug(slug)
		suite.Require().NoError(err)
		week, err := suite.sceneService.GetSceneWeek(city, state, weekKey)
		suite.Require().NoError(err, "the sitemap announces a week permalink that 404s: %s", entry.Slug)
		suite.NotZero(week.ShowCount, "the sitemap announces an empty week: %s", entry.Slug)
		announced[entry.Slug] = true
	}

	// The other direction, which is what makes the week projection's scope
	// load-bearing: a scene whose page serves shows this week must have its
	// week permalink announced. A week scoped to rooms the scene does not hold
	// finds no shows and quietly announces nothing.
	weekKey := ISOWeekKey(time.Now().UTC())
	for _, entry := range entries.Scenes {
		city, state, err := suite.sceneService.ParseSceneSlug(entry.Slug)
		suite.Require().NoError(err)
		week, err := suite.sceneService.GetSceneWeek(city, state, weekKey)
		suite.Require().NoError(err)
		if week.ShowCount == 0 {
			continue
		}
		suite.True(announced[entry.Slug+"/"+weekKey],
			"%s serves %d shows this week and the sitemap announces no week permalink for it",
			entry.Slug, week.ShowCount)
	}
}

// TestSceneSlugOrderByAgreesWithTheGroupMinima pins the one comparison Go and
// Postgres both perform: sceneGroupOutranks compares two groups' MIN(city)
// BYTE-WISE, while ParseSceneSlug's literal fallback takes `ORDER BY city, state
// LIMIT 1` under the database's collation.
//
// The two answer different populations. Groups above the venue floor are
// resolved in Go; the SQL is what resolves the ones below it, where no group
// publishes the slug. A collation that sorts punctuation the other way makes
// them name different spellings, and this fails rather than going quiet, which
// is what the corpus's below-floor collision is seeded for.
func (suite *SceneServiceIntegrationTestSuite) TestSceneSlugOrderByAgreesWithTheGroupMinima() {
	suite.seedCrossSurfaceScenes()

	for _, slug := range []string{"trois-rivieres-qc", "val-dor-qc", "saint-jerome-qc"} {
		var sqlPick struct {
			City  string
			State string
		}
		suite.Require().NoError(suite.db.Raw(`
			SELECT city, state
			FROM venues
			WHERE verified = true
			  AND `+sceneSlugExprSQL+` = ?
			ORDER BY city, state
			LIMIT 1
		`, slug).Scan(&sqlPick).Error)
		suite.Require().NotEmpty(sqlPick.City, "the fixture must hold rooms under %s", slug)

		var goPick string
		suite.Require().NoError(suite.db.Raw(`
			SELECT MIN(city)
			FROM venues
			WHERE verified = true
			  AND `+sceneSlugExprSQL+` = ?
		`, slug).Scan(&goPick).Error)

		var cities []string
		suite.Require().NoError(suite.db.Raw(`
			SELECT DISTINCT city
			FROM venues
			WHERE verified = true
			  AND `+sceneSlugExprSQL+` = ?
		`, slug).Scan(&cities).Error)
		byteWise := cities[0]
		for _, c := range cities[1:] {
			if c < byteWise {
				byteWise = c
			}
		}

		suite.Equal(sqlPick.City, byteWise,
			"ORDER BY and a byte-wise comparison pick different spellings of %s", slug)
		suite.Equal(goPick, byteWise, "MIN(city) and a byte-wise comparison disagree for %s", slug)
	}
}

// TestExistenceProbeAgreesWithTheDetailRoute covers the slugs the directory
// does NOT publish, where the probe and the page can still disagree. The probe
// is what the frontend proxy soft-404s on, so a probe that says no about a page
// that renders hides it, and one that says yes about a page that 404s sends a
// reader to an error.
//
// Both directions are seeded. A metro member slug (sedona-az, mesa-az) has no
// group of its own and canonicalizes onto its metro's page; sedona-az is the
// one that bites, because the metro it rolls up to holds its rooms in a drifted
// fallback group. A spelling collision below the floor (trois-rivieres-qc)
// matches two rooms by slug and one by scope.
func (suite *SceneServiceIntegrationTestSuite) TestExistenceProbeAgreesWithTheDetailRoute() {
	suite.seedCrossSurfaceScenes()
	existence := NewEntityExistenceService(suite.db, suite.sceneService)

	agrees := func(slug string) bool {
		city, state, err := suite.sceneService.ParseSceneSlug(slug)
		suite.Require().NoError(err)
		_, detailErr := suite.sceneService.GetSceneDetail(city, state)
		exists, err := existence.Exists("scenes", slug)
		suite.Require().NoError(err)
		suite.Equal(detailErr == nil, exists, "the proxy gate and the page disagree about %s", slug)
		return exists
	}

	for _, slug := range []string{"sedona-az", "mesa-az"} {
		city, state := parseSceneSlugParts(slug)
		_, pins := geo.Default().ResolveMetro(city, state, usCountry)
		suite.Require().True(pins, "%s is only a member slug if it pins a CBSA", slug)
		suite.True(agrees(slug), "%s canonicalizes onto a scene this corpus holds", slug)
	}

	suite.False(agrees("trois-rivieres-qc"),
		"two spellings of one room each are not a scene under either spelling")
}
