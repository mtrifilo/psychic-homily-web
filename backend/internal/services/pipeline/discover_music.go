package pipeline

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// livenessConcurrency bounds the number of in-flight liveness probes. The
// probes are independent, SSRF-guarded, read-only HEAD/GET requests; running
// them concurrently collapses N sequential per-probe timeouts into ~one timeout
// of wall-clock, keeping the whole request comfortably inside the frontend's
// bound even when an artist has many exact-name MB matches.
const livenessConcurrency = 8

// mbSearcher is the slice of the MusicBrainz client the discovery flow needs.
// Declaring it as an interface lets the unit tests inject a fake MB client
// without any network I/O (PSY-1191) — the exact-name gate and region tier are
// the load-bearing logic and must be tested deterministically.
type mbSearcher interface {
	SearchArtistCandidates(name string) ([]MBArtistResult, error)
	LookupArtistURLRelations(mbid string) ([]MBURLRelation, error)
}

// DiscoverMusicService discovers candidate Bandcamp/Spotify links for a
// link-less artist via MusicBrainz. It is DISCOVERY-ONLY — it writes nothing.
// The admin reviews the returned candidates and picks one via the existing
// bandcamp/spotify update endpoints.
//
// The validated gating rules (PSY-1196 yield + PSY-1197 precision spikes):
//   - Exact-name gate (HARD): keep only MB candidates whose name
//     normalizes-equals the artist name. MB score is NEVER an identity signal.
//   - Region is a confidence TIER, never a gate: a region mismatch downgrades a
//     candidate to "review", it never drops it (touring acts legitimately play
//     far from their MB-tagged origin).
type DiscoverMusicService struct {
	db       *gorm.DB
	mb       mbSearcher
	liveness LivenessChecker
	// regionsFn resolves an artist's PH show regions. A field (defaulting to
	// artistShowRegions) so unit tests can inject a region set without a live DB
	// — the region-confidence tier is load-bearing logic that must be tested
	// deterministically against the exact-name-gated candidate flow.
	regionsFn func(artistID uint) ([]showRegion, error)
}

// NewDiscoverMusicService builds the service with the real MusicBrainz client
// and the SSRF-safe liveness checker. A nil database resolves to the process
// default so callers don't have to thread the global DB pointer.
func NewDiscoverMusicService(database *gorm.DB) *DiscoverMusicService {
	if database == nil {
		database = db.GetDB()
	}
	s := &DiscoverMusicService{
		db:       database,
		mb:       NewMusicBrainzClient(),
		liveness: NewSSRFSafeLivenessChecker(),
	}
	s.regionsFn = s.artistShowRegions
	return s
}

// DiscoverMusic runs the discovery flow for one artist and returns the candidate
// list. The artist's name is supplied by the caller (resolved from the ID) so
// this service stays free of artist-lookup concerns.
func (s *DiscoverMusicService) DiscoverMusic(artistID uint, artistName string) (*contracts.DiscoverMusicResult, error) {
	result := &contracts.DiscoverMusicResult{
		ArtistID:   artistID,
		Candidates: []contracts.MusicLinkCandidate{},
	}

	normName := normalizeArtistName(artistName)
	if normName == "" {
		// No usable name to match on — return an empty candidate set rather
		// than searching for the empty string.
		return result, nil
	}

	// Region signal: the set of (city, state) where this artist has played PH
	// shows. Empty is fine — every candidate then resolves to "review" with the
	// no-region note. A query failure is non-fatal: discovery still works, it
	// just can't compute the high-confidence tier, so all candidates degrade to
	// "review".
	regions, err := s.regionsFn(artistID)
	if err != nil {
		regions = nil
	}

	candidates, err := s.mb.SearchArtistCandidates(artistName)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz candidate search: %w", err)
	}

	// dedup keys the candidate list by (platform, url) so the same link surfaced
	// by multiple MB artists (or multiple relations on one artist) appears once.
	seen := make(map[string]struct{})

	for _, cand := range candidates {
		// EXACT-NAME GATE (hard requirement). NEVER take the top match or use
		// score for identity.
		if normalizeArtistName(cand.Name) != normName {
			continue
		}

		rels, relErr := s.mb.LookupArtistURLRelations(cand.ID)
		if relErr != nil {
			// One artist's lookup failing shouldn't sink the whole request —
			// skip it and keep going.
			continue
		}

		confidence, regionMatch := regionTier(cand, regions)
		notes := candidateNotes(cand, regionMatch)

		for _, rel := range rels {
			platform, normalizedURL, ok := classifyPlatformURL(rel.URL.Resource)
			if !ok {
				continue
			}
			dedupKey := platform + "|" + normalizedURL
			if _, dup := seen[dedupKey]; dup {
				continue
			}
			seen[dedupKey] = struct{}{}

			result.Candidates = append(result.Candidates, contracts.MusicLinkCandidate{
				Platform:     platform,
				URL:          normalizedURL,
				Source:       "musicbrainz",
				MBArtistID:   cand.ID,
				MBArtistName: cand.Name,
				Confidence:   confidence,
				RegionMatch:  regionMatch,
				// Live is filled in concurrently below — each probe has its own
				// timeout and an artist with several exact-name MB matches could
				// otherwise serialize N×timeout and trip the frontend's request
				// bound.
				Notes: notes,
			})
		}
	}

	s.fillLiveness(result.Candidates)

	// Stable order: high-confidence first, then platform, then URL — so the
	// admin sees the best matches at the top regardless of MB's relation order.
	sort.SliceStable(result.Candidates, func(i, j int) bool {
		a, b := result.Candidates[i], result.Candidates[j]
		if a.Confidence != b.Confidence {
			return a.Confidence == contracts.MusicConfidenceHigh
		}
		if a.Platform != b.Platform {
			return a.Platform < b.Platform
		}
		return a.URL < b.URL
	})

	return result, nil
}

// fillLiveness probes each candidate's URL concurrently (bounded by
// livenessConcurrency) and writes the result back into candidates[i].Live. Each
// goroutine writes a DISJOINT index, so no lock is needed on the slice; the
// WaitGroup is the only synchronization. Probes are independent and read-only.
func (s *DiscoverMusicService) fillLiveness(candidates []contracts.MusicLinkCandidate) {
	if len(candidates) == 0 {
		return
	}
	sem := make(chan struct{}, livenessConcurrency)
	var wg sync.WaitGroup
	for i := range candidates {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			candidates[idx].Live = s.liveness.IsLive(candidates[idx].URL)
		}(i)
	}
	wg.Wait()
}

// normalizeArtistName lowercases, expands "&" to "and", and strips every
// non-alphanumeric character. Two names are the SAME identity iff their
// normalized forms are byte-equal. This is the exact-name gate's comparison key
// (PSY-1197): it folds "Club XCX" and "Club X.C.X" together but keeps "Club XCX"
// distinct from "Charli xcx" (→ "charlixcx" vs "clubxcx"), killing the
// famous-namesake false class at ~zero yield loss.
func normalizeArtistName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	lower = strings.ReplaceAll(lower, "&", "and")
	var b strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// showRegion is one (city, state) where an artist has played a PH show.
type showRegion struct {
	City  string
	State string
}

// artistShowRegions returns the distinct (city, state) pairs where the artist
// has an APPROVED PH show, via show_artists → shows → show_venues → venues.
// The denormalized show_artists.venue_id is nullable (PSY-576), so the safe join
// goes through show_venues, scoped to approved shows.
func (s *DiscoverMusicService) artistShowRegions(artistID uint) ([]showRegion, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var regions []showRegion
	err := s.db.Table("show_artists").
		Joins("JOIN shows ON shows.id = show_artists.show_id").
		Joins("JOIN show_venues ON show_venues.show_id = shows.id").
		Joins("JOIN venues ON venues.id = show_venues.venue_id").
		Where("show_artists.artist_id = ? AND shows.status = ?", artistID, catalogm.ShowStatusApproved).
		Where("venues.state <> '' AND venues.city <> ''").
		Distinct().
		Select("venues.city AS city, venues.state AS state").
		Scan(&regions).Error
	if err != nil {
		return nil, err
	}
	return regions, nil
}

// regionTier compares an MB candidate's geography to the artist's PH show
// regions and returns the confidence tier plus whether a region match was found.
//
// Region is a TIER, never a gate. A match (same metro/state) → "high"; anything
// else (state mismatch, non-US, or no PH region to compare) → "review". A
// "review" candidate is ALWAYS returned — a touring act legitimately plays far
// from its MB-tagged origin (Pond plays Minneapolis but MB-tags Perth), so
// dropping on region would kill correct matches.
func regionTier(cand MBArtistResult, regions []showRegion) (confidence string, regionMatch bool) {
	if len(regions) == 0 {
		return contracts.MusicConfidenceReview, false
	}
	// Non-US MB origin → can't align with a US show region; review tier. Check
	// BOTH the top-level country code AND the area/begin-area, because MB
	// frequently leaves `country` empty while still tagging a foreign `area`
	// (e.g. country:"" + area:"United Kingdom"). A bare empty country must NOT be
	// treated as US.
	if candidateIsNonUS(cand) {
		return contracts.MusicConfidenceReview, false
	}

	mbStates := candidateStateAbbrevs(cand)
	mbCities := candidateCityNames(cand)

	for _, r := range regions {
		st := strings.ToUpper(strings.TrimSpace(r.State))
		city := strings.ToLower(strings.TrimSpace(r.City))
		if st != "" {
			if _, ok := mbStates[st]; ok {
				return contracts.MusicConfidenceHigh, true
			}
		}
		// A city match is only trustworthy when the candidate is also anchored
		// to a US state (mbStates non-empty) — a bare MB city name ("London")
		// carries no state, so without that anchor it would false-match a
		// same-named US city in a different state ("London, KY"). With no US
		// state signal at all, the city match stays at review tier.
		if city != "" && len(mbStates) > 0 {
			if _, ok := mbCities[city]; ok {
				return contracts.MusicConfidenceHigh, true
			}
		}
	}
	return contracts.MusicConfidenceReview, false
}

// candidateIsNonUS reports whether the MB candidate's geography indicates a
// non-US origin. True when the top-level country is a non-empty non-US code, OR
// when an area/begin-area resolves to a recognizable non-US country. An empty
// country with only a US-state ("Subdivision") or US "City" area is treated as
// US (the common US-band shape). An area that is neither a known US state nor a
// known US-aligned form, but IS a country-type area whose name isn't "United
// States", marks the candidate non-US.
func candidateIsNonUS(cand MBArtistResult) bool {
	if cand.Country != "" && strings.ToUpper(cand.Country) != "US" {
		return true
	}
	for _, a := range []*MBArea{cand.Area, cand.BeginArea} {
		if a == nil || a.Name == "" {
			continue
		}
		// A US state name resolves via the abbrev map → US-aligned, not foreign.
		if _, ok := utils.StateNameToAbbrev(a.Name); ok {
			continue
		}
		// A country-type area that isn't the United States is a foreign signal.
		if strings.EqualFold(a.Type, "Country") && !strings.EqualFold(a.Name, "United States") {
			return true
		}
	}
	return false
}

// candidateStateAbbrevs returns the set of US state abbreviations implied by the
// MB candidate's area / begin-area. MB tags US states as "Subdivision" areas
// whose name is the full state name (e.g. "Minnesota"); we map those to the
// two-letter abbreviation used in venues.state.
func candidateStateAbbrevs(cand MBArtistResult) map[string]struct{} {
	out := make(map[string]struct{})
	for _, a := range []*MBArea{cand.Area, cand.BeginArea} {
		if a == nil || a.Name == "" {
			continue
		}
		if abbr, ok := utils.StateNameToAbbrev(a.Name); ok {
			out[abbr] = struct{}{}
		}
	}
	return out
}

// candidateCityNames returns the lowercased city names from the MB candidate's
// area / begin-area (MB tags origin cities as "City" areas).
func candidateCityNames(cand MBArtistResult) map[string]struct{} {
	out := make(map[string]struct{})
	for _, a := range []*MBArea{cand.Area, cand.BeginArea} {
		if a == nil || a.Name == "" {
			continue
		}
		if strings.EqualFold(a.Type, "City") {
			out[strings.ToLower(a.Name)] = struct{}{}
		}
	}
	return out
}

// candidateNotes builds the human-readable note shown next to a candidate.
// Collaboration/partial-name disambiguations carry through MB's own
// disambiguation comment; a region mismatch gets the touring-act caveat.
func candidateNotes(cand MBArtistResult, regionMatch bool) string {
	var parts []string
	if d := strings.TrimSpace(cand.Disambiguation); d != "" {
		parts = append(parts, d)
	}
	if !regionMatch {
		parts = append(parts, "possible touring act or namesake — verify before linking")
	}
	return strings.Join(parts, "; ")
}

// classifyPlatformURL host-anchors a MusicBrainz relation URL and, if it is a
// Bandcamp artist subdomain or an open.spotify.com artist page, returns the
// platform tag and the normalized URL. The host check is the identity signal —
// NOT the MB relation `type` string, which varies ("free streaming",
// "streaming", "bandcamp"). A substring check would be bypassable; this parses
// and anchors on the resolved host (pattern: ssrf host anchor).
func classifyPlatformURL(rawURL string) (platform, normalized string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", false
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "bandcamp.com" || strings.HasSuffix(host, ".bandcamp.com"):
		return contracts.MusicPlatformBandcamp, u.String(), true
	case host == "open.spotify.com":
		// Only artist pages are useful links; album/track/playlist URLs are not
		// what the artist's spotify field stores.
		if strings.Contains(u.Path, "/artist/") {
			return contracts.MusicPlatformSpotify, u.String(), true
		}
		return "", "", false
	default:
		return "", "", false
	}
}
