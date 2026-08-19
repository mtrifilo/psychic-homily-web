package catalog

import (
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Scene completeness gap counts (PSY-1845) — run as part of the
// SceneServiceIntegrationTestSuite (real Postgres, all migrations).

// sceneWithTwoVenues seeds the minimum verified-venue count so GetSceneGaps
// clears its existence gate, and returns one of the rooms to hang bills on.
func (suite *SceneServiceIntegrationTestSuite) sceneWithTwoVenues() *catalogm.Venue {
	venue := suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	return venue
}

// setLink writes one social column directly. The link columns are not on the
// createArtist* helpers, and going through GORM's Updates keeps the empty-string
// case (which the gap predicate must treat as missing) reachable.
func (suite *SceneServiceIntegrationTestSuite) setLink(artist *catalogm.Artist, column, value string) {
	suite.Require().NoError(
		suite.db.Model(&catalogm.Artist{}).Where("id = ?", artist.ID).
			Update(column, value).Error,
	)
}

// The listen-link definition pin. "Listen link" is spotify | bandcamp |
// youtube | soundcloud — the streaming-discovery worklist's definition. The
// non-music social columns are each seeded here precisely so that widening the
// predicate to "any social link" (the shape services/admin's global
// artists_missing_links count uses) fails loudly instead of quietly reporting
// the gap as smaller than it is.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGaps_ListenLinkIsMusicPlatformsOnly() {
	suite.sceneWithTwoVenues()

	// Each music platform ALONE clears the gap.
	for i, col := range []string{"spotify", "bandcamp", "youtube", "soundcloud"} {
		a := suite.createArtist("Linked Band " + string(rune('A'+i)))
		suite.setLink(a, col, "https://example.com/"+col)
	}

	// Non-music socials do NOT clear it — these bands still have nothing to play.
	for i, col := range []string{"instagram", "facebook", "twitter", "website"} {
		a := suite.createArtist("Social Only " + string(rune('A'+i)))
		suite.setLink(a, col, "https://example.com/"+col)
	}

	// An empty string is not a link. A bare `IS NULL` predicate would read this
	// row as linked and understate the gap.
	blank := suite.createArtist("Blank String Band")
	suite.setLink(blank, "bandcamp", "")

	// Nothing at all.
	suite.createArtist("Bare Band")

	gaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().NoError(err)

	// 4 social-only + 1 empty-string + 1 bare = 6; the 4 linked bands are out.
	suite.Equal(6, gaps.ArtistsMissingListenLink)
}

// Roster scope pin: the listen-link gap counts bands BASED here, so it is
// always a subset of SceneStats.ArtistCount. A linkless band based elsewhere is
// not this scene's gap to close, even when it plays here constantly.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGaps_ListenLinkCountsBasedHereOnly() {
	venue := suite.sceneWithTwoVenues()
	user := suite.createUser()

	suite.createArtist("Local Linkless")

	touring := suite.createArtistIn("Touring Linkless", "Portland", "OR")
	suite.createApprovedShow("touring stop", venue.ID, touring.ID, user.ID, time.Now().UTC().AddDate(0, 0, -10))

	gaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().NoError(err)

	suite.Equal(1, gaps.ArtistsMissingListenLink)
}

// The location gap's population is "played the scene's rooms", which is a
// DIFFERENT set from the roster — that is the whole point of the number. A
// locationless band with no local bill is invisible to this scene, and a band
// that played ten times is one gap, not ten.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGaps_LocationCountsBillsDistinctly() {
	venue := suite.sceneWithTwoVenues()
	user := suite.createUser()
	now := time.Now().UTC()

	// Locationless, played here three times: exactly ONE gap.
	repeat := suite.createArtistIn("Repeat Ghost", "", "")
	suite.createApprovedShow("gig 1", venue.ID, repeat.ID, user.ID, now.AddDate(0, 0, -60))
	suite.createApprovedShow("gig 2", venue.ID, repeat.ID, user.ID, now.AddDate(0, 0, -30))
	suite.createApprovedShow("gig 3", venue.ID, repeat.ID, user.ID, now.AddDate(0, 0, 30))

	// Locationless but never played here — not this scene's gap.
	suite.createArtistIn("Absent Ghost", "", "")

	// Located band on the bills — nothing to fix.
	located := suite.createArtist("Located Band")
	suite.createApprovedShow("located gig", venue.ID, located.ID, user.ID, now.AddDate(0, 0, -5))

	gaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().NoError(err)

	suite.Equal(1, gaps.ArtistsOnBillsMissingLocation)
}

// Partial locations are deliberately NOT counted: "no location" means city AND
// state both blank, matching the project's existing artists_missing_location
// definition. A band with a city but no state is unclaimable by the roster too,
// but it is a different gap with a different fix — folding it in here would
// silently redefine a published number.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGaps_LocationExcludesPartialLocations() {
	venue := suite.sceneWithTwoVenues()
	user := suite.createUser()
	now := time.Now().UTC()

	cityOnly := suite.createArtistIn("City Only", "Tucson", "")
	suite.createApprovedShow("city only gig", venue.ID, cityOnly.ID, user.ID, now.AddDate(0, 0, -3))

	stateOnly := suite.createArtistIn("State Only", "", "NM")
	suite.createApprovedShow("state only gig", venue.ID, stateOnly.ID, user.ID, now.AddDate(0, 0, -4))

	fullyBlank := suite.createArtistIn("Fully Blank", "", "")
	suite.createApprovedShow("blank gig", venue.ID, fullyBlank.ID, user.ID, now.AddDate(0, 0, -5))

	gaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().NoError(err)

	suite.Equal(1, gaps.ArtistsOnBillsMissingLocation)
}

// Only APPROVED bills count. An unreviewed submission is not yet a fact about
// the scene, so it must not manufacture a gap — the same status gate every
// other show-side scene surface applies.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGaps_LocationIgnoresPendingShows() {
	venue := suite.sceneWithTwoVenues()
	user := suite.createUser()
	now := time.Now().UTC()

	pendingOnly := suite.createArtistIn("Pending Ghost", "", "")
	suite.createPendingShow("unreviewed gig", venue.ID, pendingOnly.ID, user.ID, now.AddDate(0, 0, -2))

	gaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().NoError(err)

	suite.Equal(0, gaps.ArtistsOnBillsMissingLocation)
}

// A bill in another scene's room does not become this scene's gap.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGaps_LocationIgnoresOutOfScopeVenues() {
	suite.sceneWithTwoVenues()
	elsewhere := suite.createVerifiedVenue("Mississippi Studios", "Portland", "OR")
	user := suite.createUser()

	ghost := suite.createArtistIn("Portland Ghost", "", "")
	suite.createApprovedShow("portland gig", elsewhere.ID, ghost.ID, user.ID, time.Now().UTC().AddDate(0, 0, -7))

	gaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().NoError(err)

	suite.Equal(0, gaps.ArtistsOnBillsMissingLocation)
}

// A complete scene reports zeros — the frontend hides the line on 0/0, so this
// is the state that turns the prompt off, not an error or a null payload.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGaps_CompleteSceneReturnsZeros() {
	venue := suite.sceneWithTwoVenues()
	user := suite.createUser()

	local := suite.createArtist("Complete Band")
	suite.setLink(local, "bandcamp", "https://completeband.bandcamp.com")
	suite.createApprovedShow("complete gig", venue.ID, local.ID, user.ID, time.Now().UTC().AddDate(0, 0, -1))

	gaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().NoError(err)

	suite.Equal(0, gaps.ArtistsMissingListenLink)
	suite.Equal(0, gaps.ArtistsOnBillsMissingLocation)
	suite.Equal("Phoenix", gaps.City)
	suite.Equal("AZ", gaps.State)
	suite.Equal("phoenix-az", gaps.Slug)
}

// The existence gate: a place below the verified-venue threshold is not a
// scene, and must 404 here exactly as it does on GET /scenes/{slug} rather than
// publishing "complete" about a place that has no scene page.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGaps_BelowVenueThresholdIsNotFound() {
	suite.createVerifiedVenue("Lone Room", "Phoenix", "AZ")
	suite.createArtist("Linkless Local")

	gaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().Error(err)
	suite.Nil(gaps)

	// Typed, not just "an error": the handler maps ONLY *SceneError to a 404,
	// so a plain fmt.Errorf here would surface to readers as a 500.
	var sceneErr *apperrors.SceneError
	suite.Require().ErrorAs(err, &sceneErr)
	suite.Equal(apperrors.CodeSceneNotFound, sceneErr.Code)
}

// ---------------------------------------------------------------------------
// Direct-SQL cross-check (PSY-1845 acceptance)
// ---------------------------------------------------------------------------

// crossCheckGapsSQL recomputes both gap counts with hand-written SQL that
// shares NO code with the implementation — it keys on the geo dataset's CBSA
// for the scene rather than going through scopeFor/artistPredicate/
// venuePredicate, and spells "blank" with COALESCE + an equality test rather
// than the implementation's NULLIF + IS NULL.
//
// The point is that a bug in the shared predicate builders cannot hide by being
// present on both sides of the assertion.
func (suite *SceneServiceIntegrationTestSuite) crossCheckGapsSQL(city, state string) (int, int) {
	metro := seedMetro(city, state)
	suite.Require().NotNil(metro, "cross-check assumes a CBSA-keyed scene")

	var missingListenLink int64
	suite.Require().NoError(suite.db.Raw(`
		SELECT COUNT(*)
		FROM artists a
		WHERE a.metro = ?
		  AND COALESCE(TRIM(a.spotify), '')    = ''
		  AND COALESCE(TRIM(a.bandcamp), '')   = ''
		  AND COALESCE(TRIM(a.youtube), '')    = ''
		  AND COALESCE(TRIM(a.soundcloud), '') = ''
	`, *metro).Scan(&missingListenLink).Error)

	var missingLocation int64
	suite.Require().NoError(suite.db.Raw(`
		SELECT COUNT(DISTINCT sa.artist_id)
		FROM show_artists sa
		JOIN shows s        ON s.id = sa.show_id AND s.status = 'approved'
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v       ON v.id = sv.venue_id AND v.metro = ?
		JOIN artists a      ON a.id = sa.artist_id
		WHERE COALESCE(TRIM(a.city), '')  = ''
		  AND COALESCE(TRIM(a.state), '') = ''
	`, *metro).Scan(&missingLocation).Error)

	return int(missingListenLink), int(missingLocation)
}

// A DENSE scene (many bands, many bills, gaps of both kinds) and a SPARSE one
// (two rooms, one band, nothing to fix) in the same database, each cross-checked
// against independent SQL — the acceptance check for these counts.
//
// Both scenes are populated at once deliberately: it is the case where a
// missing scope filter would leak one city's gaps into the other's numbers, and
// a single-scene fixture cannot see that class of bug at all.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGaps_MatchesDirectSQLForDenseAndSparseScenes() {
	user := suite.createUser()
	now := time.Now().UTC()

	// ── Dense scene: Phoenix ──
	phx := suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")

	for i := 0; i < 7; i++ {
		suite.createArtist("PHX Linkless " + string(rune('A'+i)))
	}
	for i := 0; i < 3; i++ {
		a := suite.createArtist("PHX Linked " + string(rune('A'+i)))
		suite.setLink(a, "bandcamp", "https://phx.bandcamp.com/")
	}
	for i := 0; i < 4; i++ {
		ghost := suite.createArtistIn("PHX Ghost "+string(rune('A'+i)), "", "")
		suite.createApprovedShow("phx gig "+string(rune('A'+i)), phx.ID, ghost.ID, user.ID, now.AddDate(0, 0, -i-1))
	}

	// ── Sparse scene: Tucson (its own CBSA, so a genuinely separate scene) ──
	tus := suite.createVerifiedVenue("Club Congress", "Tucson", "AZ")
	suite.createVerifiedVenue("191 Toole", "Tucson", "AZ")

	tucsonBand := suite.createArtistIn("Tucson Complete", "Tucson", "AZ")
	suite.setLink(tucsonBand, "spotify", "https://open.spotify.com/artist/tucson")
	suite.createApprovedShow("tucson gig", tus.ID, tucsonBand.ID, user.ID, now.AddDate(0, 0, -2))

	// ── Dense ──
	denseGaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().NoError(err)
	denseLinkSQL, denseLocSQL := suite.crossCheckGapsSQL("Phoenix", "AZ")

	suite.Equal(denseLinkSQL, denseGaps.ArtistsMissingListenLink, "dense listen-link gap must match direct SQL")
	suite.Equal(denseLocSQL, denseGaps.ArtistsOnBillsMissingLocation, "dense location gap must match direct SQL")
	// Pin the absolute numbers too, so a cross-check that drifts in BOTH
	// spellings at once still fails.
	suite.Equal(7, denseGaps.ArtistsMissingListenLink)
	suite.Equal(4, denseGaps.ArtistsOnBillsMissingLocation)

	// ── Sparse ──
	sparseGaps, err := suite.sceneService.GetSceneGaps("Tucson", "AZ")
	suite.Require().NoError(err)
	sparseLinkSQL, sparseLocSQL := suite.crossCheckGapsSQL("Tucson", "AZ")

	suite.Equal(sparseLinkSQL, sparseGaps.ArtistsMissingListenLink, "sparse listen-link gap must match direct SQL")
	suite.Equal(sparseLocSQL, sparseGaps.ArtistsOnBillsMissingLocation, "sparse location gap must match direct SQL")
	suite.Equal(0, sparseGaps.ArtistsMissingListenLink)
	suite.Equal(0, sparseGaps.ArtistsOnBillsMissingLocation)
}

// The no-CBSA fallback branch, which every other test in this file misses:
// Phoenix and Tucson both resolve to metros, so they all exercise the ONE-arg
// venue predicate. The fallback branch binds TWO args, and the bills query
// splices them ahead of its own status placeholder — a positional-binding slip
// would land here and nowhere else, either erroring outright or counting one
// city's gaps against another. Same reasoning as
// TestGetSceneDetail_VenuesOnNoCBSAFallbackScene, applied to both gap counts.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGaps_NoCBSAFallbackScene() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Club One", "Faketown", "ZZ")
	suite.createVerifiedVenue("Club Two", "Faketown", "ZZ")
	suite.Require().Nil(v1.Metro, "a no-CBSA place has a NULL metro")

	// Based in Faketown, no listen link: gap 1.
	suite.createArtistIn("Faketown Linkless", "Faketown", "ZZ")
	// Based in Faketown WITH a link: not a gap.
	linked := suite.createArtistIn("Faketown Linked", "Faketown", "ZZ")
	suite.setLink(linked, "bandcamp", "https://faketown.bandcamp.com")
	// Based in another no-CBSA city — must NOT leak across the (city, state) key.
	suite.createArtistIn("Othertown Linkless", "Othertown", "ZZ")

	// Locationless band on a Faketown bill: gap 2.
	ghost := suite.createArtistIn("Faketown Ghost", "", "")
	suite.createApprovedShow("faketown gig", v1.ID, ghost.ID, user.ID, time.Now().UTC().AddDate(0, 0, -3))

	// Locationless band on ANOTHER fallback city's bill: must not leak in.
	elsewhere := suite.createVerifiedVenue("Club One", "Othertown", "ZZ")
	otherGhost := suite.createArtistIn("Othertown Ghost", "", "")
	suite.createApprovedShow("othertown gig", elsewhere.ID, otherGhost.ID, user.ID, time.Now().UTC().AddDate(0, 0, -4))

	gaps, err := suite.sceneService.GetSceneGaps("Faketown", "ZZ")
	suite.Require().NoError(err)

	suite.Equal(1, gaps.ArtistsMissingListenLink, "Othertown's linkless band is a different scene")
	suite.Equal(1, gaps.ArtistsOnBillsMissingLocation, "Othertown's bill is a different scene")
	suite.Equal("faketown-zz", gaps.Slug)
}
