package pipeline

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/services/contracts"
)

// fakeMB is an in-memory mbSearcher. searchResults is returned verbatim from
// SearchArtistCandidates (so a test can plant a famous namesake AT THE TOP and
// the correct match buried below); relsByID maps an MB artist UUID to its
// url-rels so the exact-name-gated candidate fetches its links.
type fakeMB struct {
	searchResults []MBArtistResult
	relsByID      map[string][]MBURLRelation
	searchErr     error
}

func (f *fakeMB) SearchArtistCandidates(_ context.Context, _ string) ([]MBArtistResult, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchResults, nil
}

func (f *fakeMB) LookupArtistURLRelations(_ context.Context, mbid string) ([]MBURLRelation, error) {
	return f.relsByID[mbid], nil
}

// stubLiveness reports a fixed liveness for every URL so candidate assembly is
// deterministic and free of network I/O.
type stubLiveness struct{ live bool }

func (s stubLiveness) IsLive(context.Context, string) bool { return s.live }

func urlRel(t, resource string) MBURLRelation {
	r := MBURLRelation{Type: t}
	r.URL.Resource = resource
	return r
}

// newTestService wires the service with the fake MB client, a stub liveness
// checker, and an injected region set — no DB, no network.
func newTestService(mb mbSearcher, live bool, regions []showRegion) *DiscoverMusicService {
	return &DiscoverMusicService{
		mb:        mb,
		liveness:  stubLiveness{live: live},
		regionsFn: func(uint) ([]showRegion, error) { return regions, nil },
	}
}

// TestExactNameGate_RejectsFamousNamesake is the load-bearing precision test:
// "Club XCX" must NOT return Charli xcx's links even though MB scores Charli xcx
// far higher and returns it at the top.
func TestExactNameGate_RejectsFamousNamesake(t *testing.T) {
	mb := &fakeMB{
		searchResults: []MBArtistResult{
			// Famous namesake at the TOP with a perfect score.
			{ID: "charli-xcx-id", Name: "Charli xcx", Score: 100, Country: "GB"},
			// The actual local act — lower score, buried.
			{ID: "club-xcx-id", Name: "Club XCX", Score: 72, Country: "US"},
		},
		relsByID: map[string][]MBURLRelation{
			"charli-xcx-id": {urlRel("free streaming", "https://open.spotify.com/artist/charlixcx")},
			"club-xcx-id":   {urlRel("bandcamp", "https://clubxcx.bandcamp.com/")},
		},
	}
	svc := newTestService(mb, true, nil)

	res, err := svc.DiscoverMusic(context.Background(), 1, "Club XCX")
	if err != nil {
		t.Fatalf("DiscoverMusic returned error: %v", err)
	}

	for _, c := range res.Candidates {
		if c.MBArtistID == "charli-xcx-id" {
			t.Fatalf("exact-name gate leaked Charli xcx as a candidate for 'Club XCX': %+v", c)
		}
	}
	// And it must surface the correct buried match.
	if len(res.Candidates) != 1 {
		t.Fatalf("expected exactly 1 candidate (the real Club XCX), got %d: %+v", len(res.Candidates), res.Candidates)
	}
	if got := res.Candidates[0].URL; got != "https://clubxcx.bandcamp.com" {
		t.Fatalf("expected the real Club XCX bandcamp link (canonicalized, no trailing slash), got %q", got)
	}
}

// TestExactNameGate_SurfacesBuriedCorrectMatchOverJunkTopHit asserts the gate
// RESCUES a correct match that MB ranks below a junk top-hit (PSY-1197: the real
// "Dylan Day" over "LaymGlitched").
func TestExactNameGate_SurfacesBuriedCorrectMatchOverJunkTopHit(t *testing.T) {
	mb := &fakeMB{
		searchResults: []MBArtistResult{
			{ID: "junk-id", Name: "LaymGlitched", Score: 100, Country: "US"},
			{ID: "dylan-day-id", Name: "Dylan Day", Score: 81, Country: "US"},
		},
		relsByID: map[string][]MBURLRelation{
			"junk-id":      {urlRel("bandcamp", "https://laymglitched.bandcamp.com/")},
			"dylan-day-id": {urlRel("bandcamp", "https://dylanday.bandcamp.com/")},
		},
	}
	svc := newTestService(mb, true, nil)

	res, err := svc.DiscoverMusic(context.Background(), 1, "Dylan Day")
	if err != nil {
		t.Fatalf("DiscoverMusic error: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].MBArtistID != "dylan-day-id" {
		t.Fatalf("expected only the buried-correct Dylan Day, got %+v", res.Candidates)
	}
}

// TestExactNameGate_AmpersandAndPunctuation confirms the normalizer folds "&"→
// "and" and strips punctuation so equivalent name spellings still match.
func TestExactNameGate_AmpersandAndPunctuation(t *testing.T) {
	mb := &fakeMB{
		searchResults: []MBArtistResult{
			{ID: "x", Name: "Florence + the Machine", Score: 90, Country: "US"},
			{ID: "y", Name: "Florence and The Machine", Score: 88, Country: "US"},
		},
		relsByID: map[string][]MBURLRelation{
			"y": {urlRel("bandcamp", "https://fatm.bandcamp.com/")},
		},
	}
	svc := newTestService(mb, true, nil)

	// Query with the "&" form; the "and" candidate must match.
	res, err := svc.DiscoverMusic(context.Background(), 1, "Florence & the Machine")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].MBArtistID != "y" {
		t.Fatalf("&→and normalization failed; got %+v", res.Candidates)
	}
}

// TestRegionTier_NeverDropsOnMismatch is the load-bearing recall test: an
// exact-name match whose MB region is foreign (Pond, MB-area Australia) is
// returned as "review", NOT dropped.
func TestRegionTier_NeverDropsOnMismatch_PondInMinneapolis(t *testing.T) {
	mb := &fakeMB{
		searchResults: []MBArtistResult{
			{
				ID:        "pond-au-id",
				Name:      "Pond",
				Score:     99,
				Country:   "AU",
				Area:      &MBArea{Name: "Australia", Type: "Country"},
				BeginArea: &MBArea{Name: "Perth", Type: "City"},
			},
		},
		relsByID: map[string][]MBURLRelation{
			"pond-au-id": {urlRel("bandcamp", "https://pondband.bandcamp.com/")},
		},
	}
	// Artist plays in Minneapolis, MN — a region the AU band can't match.
	svc := newTestService(mb, true, []showRegion{{City: "Minneapolis", State: "MN"}})

	res, err := svc.DiscoverMusic(context.Background(), 1, "Pond")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("region mismatch must NOT drop the candidate; got %d candidates", len(res.Candidates))
	}
	c := res.Candidates[0]
	if c.Confidence != contracts.MusicConfidenceReview {
		t.Fatalf("expected review tier for AU band in MN, got %q", c.Confidence)
	}
	if c.RegionMatch {
		t.Fatalf("region_match must be false for AU band in MN")
	}
	if c.Notes == "" {
		t.Fatalf("expected a touring-act/namesake note on a review-tier candidate")
	}
}

// TestRegionTier_HighOnStateMatch confirms a same-state MB origin yields "high".
func TestRegionTier_HighOnStateMatch(t *testing.T) {
	mb := &fakeMB{
		searchResults: []MBArtistResult{
			{
				ID:        "local-id",
				Name:      "Localband",
				Score:     95,
				Country:   "US",
				Area:      &MBArea{Name: "Minnesota", Type: "Subdivision"},
				BeginArea: &MBArea{Name: "Minneapolis", Type: "City"},
			},
		},
		relsByID: map[string][]MBURLRelation{
			"local-id": {urlRel("bandcamp", "https://localband.bandcamp.com/")},
		},
	}
	svc := newTestService(mb, true, []showRegion{{City: "Minneapolis", State: "MN"}})

	res, err := svc.DiscoverMusic(context.Background(), 1, "Localband")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].Confidence != contracts.MusicConfidenceHigh {
		t.Fatalf("expected high tier on MN state match, got %+v", res.Candidates)
	}
	if !res.Candidates[0].RegionMatch {
		t.Fatalf("region_match should be true on a state match")
	}
}

// TestRegionTier_NoRegionIsReview confirms that with no PH region to compare,
// every candidate degrades to review (never high, never dropped).
func TestRegionTier_NoRegionIsReview(t *testing.T) {
	mb := &fakeMB{
		searchResults: []MBArtistResult{
			{ID: "id", Name: "Band", Score: 95, Country: "US", Area: &MBArea{Name: "California", Type: "Subdivision"}},
		},
		relsByID: map[string][]MBURLRelation{
			"id": {urlRel("bandcamp", "https://band.bandcamp.com/")},
		},
	}
	svc := newTestService(mb, true, nil) // no regions

	res, _ := svc.DiscoverMusic(context.Background(), 1, "Band")
	if len(res.Candidates) != 1 || res.Candidates[0].Confidence != contracts.MusicConfidenceReview {
		t.Fatalf("no-region must be review tier, got %+v", res.Candidates)
	}
}

// TestDiscoverMusic_DedupsAndExtractsBothPlatforms confirms multi-link
// extraction (bandcamp + spotify from one artist) plus de-dup of a repeated URL.
func TestDiscoverMusic_DedupsAndExtractsBothPlatforms(t *testing.T) {
	mb := &fakeMB{
		searchResults: []MBArtistResult{
			{ID: "id", Name: "Band", Score: 95, Country: "US", Area: &MBArea{Name: "Texas", Type: "Subdivision"}},
		},
		relsByID: map[string][]MBURLRelation{
			"id": {
				urlRel("bandcamp", "https://band.bandcamp.com/"),
				urlRel("free streaming", "https://open.spotify.com/artist/abc123"),
				urlRel("streaming", "https://open.spotify.com/artist/abc123"), // dup
				urlRel("free streaming", "https://www.deezer.com/artist/999"), // ignored platform
				urlRel("youtube", "https://youtube.com/band"),                 // ignored
			},
		},
	}
	svc := newTestService(mb, true, []showRegion{{City: "Austin", State: "TX"}})

	res, _ := svc.DiscoverMusic(context.Background(), 1, "Band")
	if len(res.Candidates) != 2 {
		t.Fatalf("expected 2 candidates (bandcamp + 1 deduped spotify), got %d: %+v", len(res.Candidates), res.Candidates)
	}
	platforms := map[string]bool{}
	for _, c := range res.Candidates {
		platforms[c.Platform] = true
	}
	if !platforms[contracts.MusicPlatformBandcamp] || !platforms[contracts.MusicPlatformSpotify] {
		t.Fatalf("expected both bandcamp and spotify, got %+v", platforms)
	}
}

// TestDiscoverMusic_SearchErrorPropagates confirms an MB search failure is a
// hard error (the handler maps it to 502).
func TestDiscoverMusic_SearchErrorPropagates(t *testing.T) {
	mb := &fakeMB{searchErr: errors.New("mb down")}
	svc := newTestService(mb, true, nil)
	if _, err := svc.DiscoverMusic(context.Background(), 1, "Band"); err == nil {
		t.Fatalf("expected error when MB search fails")
	}
}

// TestDiscoverMusic_EmptyNameSkips confirms an empty/whitespace name returns an
// empty result without searching for the empty string.
func TestDiscoverMusic_EmptyNameSkips(t *testing.T) {
	mb := &fakeMB{searchErr: errors.New("should not be called")}
	svc := newTestService(mb, true, nil)
	res, err := svc.DiscoverMusic(context.Background(), 1, "   ")
	if err != nil {
		t.Fatalf("empty name should not error, got %v", err)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("empty name should yield no candidates")
	}
}

// TestRegionTier_CrossStateCityIsReview guards the city-match accuracy fix: a
// bare MB city name ("London", no US-state anchor) must NOT false-match a
// same-named US city in a different state ("London, KY") as high confidence.
func TestRegionTier_CrossStateCityIsReview(t *testing.T) {
	cand := MBArtistResult{
		Name:    "Some Band",
		Country: "GB",
		Area:    &MBArea{Name: "United Kingdom", Type: "Country"},
		// MB City begin-area with no US state context.
		BeginArea: &MBArea{Name: "London", Type: "City"},
	}
	conf, match := regionTier(cand, []showRegion{{City: "London", State: "KY"}})
	if conf != contracts.MusicConfidenceReview || match {
		t.Fatalf("bare foreign city must not high-match a same-named US city; got conf=%q match=%v", conf, match)
	}
}

// TestRegionTier_EmptyCountryForeignAreaIsReview guards the non-US fix: MB often
// leaves the top-level country empty while tagging a foreign Country area — that
// must still resolve to review, not slip through as US.
func TestRegionTier_EmptyCountryForeignAreaIsReview(t *testing.T) {
	cand := MBArtistResult{
		Name:    "Some Band",
		Country: "", // empty — the old guard skipped on this
		Area:    &MBArea{Name: "United Kingdom", Type: "Country"},
	}
	conf, match := regionTier(cand, []showRegion{{City: "London", State: "KY"}})
	if conf != contracts.MusicConfidenceReview || match {
		t.Fatalf("empty-country + foreign area must be review; got conf=%q match=%v", conf, match)
	}
}

// TestRegionTier_USCityWithStateAnchorIsHigh confirms a US band whose City
// begin-area is anchored by a US-state area still earns high on a city match.
func TestRegionTier_USCityWithStateAnchorIsHigh(t *testing.T) {
	cand := MBArtistResult{
		Name:      "Localband",
		Country:   "US",
		Area:      &MBArea{Name: "Minnesota", Type: "Subdivision"}, // US state anchor
		BeginArea: &MBArea{Name: "Minneapolis", Type: "City"},
	}
	// Region carries a city the show DB might store without a matching state row;
	// the state anchor makes the city match trustworthy.
	conf, match := regionTier(cand, []showRegion{{City: "Minneapolis", State: "MN"}})
	if conf != contracts.MusicConfidenceHigh || !match {
		t.Fatalf("US band with state anchor should be high on city/state match; got conf=%q match=%v", conf, match)
	}
}

func TestNormalizeArtistName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Club XCX", "clubxcx"},
		{"Charli xcx", "charlixcx"},
		{"Florence & the Machine", "florenceandthemachine"},
		{"Florence and The Machine", "florenceandthemachine"},
		{"  Spaced  Out  ", "spacedout"},
		{"Mötley Crüe", "mtleycre"}, // diacritics are non-[a-z0-9] → stripped
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeArtistName(c.in); got != c.want {
			t.Errorf("NormalizeArtistName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClassifyPlatformURL(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		wantPlat string
		wantOK   bool
	}{
		{"bandcamp subdomain", "https://artist.bandcamp.com/", contracts.MusicPlatformBandcamp, true},
		{"bandcamp apex", "https://bandcamp.com/artist", contracts.MusicPlatformBandcamp, true},
		{"spotify artist", "https://open.spotify.com/artist/abc", contracts.MusicPlatformSpotify, true},
		{"spotify album rejected", "https://open.spotify.com/album/abc", "", false},
		{"spotify wrong host rejected", "https://open.spotify.com.evil.test/artist/abc", "", false},
		{"ssrf substring bypass rejected", "http://169.254.169.254/?x=open.spotify.com/artist/abc", "", false},
		{"bandcamp substring bypass rejected", "http://169.254.169.254/album/x?bandcamp.com", "", false},
		{"deezer ignored", "https://www.deezer.com/artist/1", "", false},
		{"javascript scheme rejected", "javascript:alert(1)//bandcamp.com", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plat, _, ok := classifyPlatformURL(c.url)
			if ok != c.wantOK || plat != c.wantPlat {
				t.Errorf("classifyPlatformURL(%q) = (%q, %v), want (%q, %v)", c.url, plat, ok, c.wantPlat, c.wantOK)
			}
		})
	}
}

// TestClassifyPlatformURL_Canonicalizes confirms cosmetic variants of the same
// link canonicalize to one form (so they dedup) and that userinfo/query/fragment
// are stripped from the value returned to the admin.
func TestClassifyPlatformURL_Canonicalizes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://artist.bandcamp.com/", "https://artist.bandcamp.com"},
		{"http://artist.bandcamp.com", "https://artist.bandcamp.com"},                              // scheme forced to https
		{"https://ARTIST.Bandcamp.com/", "https://artist.bandcamp.com"},                            // host lowercased
		{"https://open.spotify.com/artist/abc?si=track123", "https://open.spotify.com/artist/abc"}, // tracking query dropped
		{"https://open.spotify.com/artist/abc#frag", "https://open.spotify.com/artist/abc"},        // fragment dropped
		{"https://user:pass@artist.bandcamp.com/album/x", "https://artist.bandcamp.com/album/x"},   // userinfo dropped
	}
	for _, c := range cases {
		_, got, ok := classifyPlatformURL(c.in)
		if !ok || got != c.want {
			t.Errorf("classifyPlatformURL(%q) normalized = %q (ok=%v), want %q", c.in, got, ok, c.want)
		}
	}
}

// TestDiscoverMusic_DedupUpgradesToHigherTier is the adversarial-review HIGH fix:
// when two exact-name MB artists expose the SAME link, the surviving deduped row
// must carry the BEST available confidence — even when MB returns the review-tier
// artist first (score order != confidence order).
func TestDiscoverMusic_DedupUpgradesToHigherTier(t *testing.T) {
	const sharedLink = "https://shared.bandcamp.com/"
	mb := &fakeMB{
		searchResults: []MBArtistResult{
			// Review-tier artist returned FIRST (foreign, higher score).
			{ID: "review-id", Name: "Band", Score: 99, Country: "AU", Area: &MBArea{Name: "Australia", Type: "Country"}},
			// High-tier artist returned SECOND (US, same state as the show).
			{ID: "high-id", Name: "Band", Score: 80, Country: "US", Area: &MBArea{Name: "Texas", Type: "Subdivision"}},
		},
		relsByID: map[string][]MBURLRelation{
			"review-id": {urlRel("bandcamp", sharedLink)},
			"high-id":   {urlRel("bandcamp", sharedLink)},
		},
	}
	svc := newTestService(mb, true, []showRegion{{City: "Austin", State: "TX"}})

	res, err := svc.DiscoverMusic(context.Background(), 1, "Band")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("the shared link must dedup to one candidate, got %d", len(res.Candidates))
	}
	c := res.Candidates[0]
	if c.Confidence != contracts.MusicConfidenceHigh || !c.RegionMatch {
		t.Fatalf("deduped row must adopt the HIGH tier from the second MB artist, got conf=%q match=%v (mbid=%s)", c.Confidence, c.RegionMatch, c.MBArtistID)
	}
	if c.MBArtistID != "high-id" {
		t.Fatalf("deduped row should adopt the high-tier MB artist, got %s", c.MBArtistID)
	}
}

// TestDiscoverMusic_DedupCosmeticVariants confirms trailing-slash / scheme /
// tracking-query variants of the same link from one artist collapse to one row.
func TestDiscoverMusic_DedupCosmeticVariants(t *testing.T) {
	mb := &fakeMB{
		searchResults: []MBArtistResult{
			{ID: "id", Name: "Band", Score: 95, Country: "US", Area: &MBArea{Name: "Texas", Type: "Subdivision"}},
		},
		relsByID: map[string][]MBURLRelation{
			"id": {
				urlRel("free streaming", "https://open.spotify.com/artist/abc"),
				urlRel("streaming", "https://open.spotify.com/artist/abc?si=xyz"), // tracking variant
				urlRel("streaming", "http://open.spotify.com/artist/abc/"),        // scheme + slash variant
			},
		},
	}
	svc := newTestService(mb, true, []showRegion{{City: "Austin", State: "TX"}})

	res, _ := svc.DiscoverMusic(context.Background(), 1, "Band")
	if len(res.Candidates) != 1 {
		t.Fatalf("cosmetic spotify variants must dedup to 1 candidate, got %d: %+v", len(res.Candidates), res.Candidates)
	}
}

func TestIsAllowedPlatformHost(t *testing.T) {
	allowed := []string{"bandcamp.com", "artist.bandcamp.com", "ARTIST.BANDCAMP.COM", "open.spotify.com"}
	denied := []string{"evil.test", "open.spotify.com.evil.test", "bandcamp.com.evil.test", "spotify.com", "accounts.spotify.com", ""}
	for _, h := range allowed {
		if !isAllowedPlatformHost(h) {
			t.Errorf("isAllowedPlatformHost(%q) = false, want true", h)
		}
	}
	for _, h := range denied {
		if isAllowedPlatformHost(h) {
			t.Errorf("isAllowedPlatformHost(%q) = true, want false", h)
		}
	}
}
