package pipeline

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/logger"
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
	SearchArtistCandidates(ctx context.Context, name string) ([]MBArtistResult, error)
	LookupArtistURLRelations(ctx context.Context, mbid string) ([]MBURLRelation, error)
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

// NewDiscoverMusicService builds the service with the given MusicBrainz client
// and the SSRF-safe liveness checker. A nil database resolves to the process
// default so callers don't have to thread the global DB pointer; a nil mbClient
// resolves to a freshly constructed client so standalone/test callers can pass
// nil.
//
// mbClient is the SHARED MusicBrainz client (PSY-1208): the server constructs
// ONE *MusicBrainzClient and passes the same instance here and to
// NewEnrichmentService, so a single mutex-serialized throttle enforces a true
// ~1 req/s across ALL MusicBrainz calls in the process (MB blocks for exceeding
// ~1 req/s/IP).
func NewDiscoverMusicService(database *gorm.DB, mbClient *MusicBrainzClient) *DiscoverMusicService {
	if database == nil {
		database = db.GetDB()
	}
	if mbClient == nil {
		mbClient = NewMusicBrainzClient()
	}
	s := &DiscoverMusicService{
		db:       database,
		mb:       mbClient,
		liveness: NewSSRFSafeLivenessChecker(),
	}
	s.regionsFn = s.artistShowRegions
	return s
}

// DiscoverMusic runs the discovery flow for one artist and returns the candidate
// list. The artist's name is supplied by the caller (resolved from the ID) so
// this service stays free of artist-lookup concerns. ctx bounds the whole
// operation: if the admin disconnects, the in-flight MB calls and liveness
// probes are cancelled instead of running to completion (and holding the shared
// MB rate-limit lock).
func (s *DiscoverMusicService) DiscoverMusic(ctx context.Context, artistID uint, artistName string) (*contracts.DiscoverMusicResult, error) {
	result := &contracts.DiscoverMusicResult{
		ArtistID:   artistID,
		Candidates: []contracts.MusicLinkCandidate{},
	}

	normName := NormalizeArtistName(artistName)
	if normName == "" {
		// No usable name to match on — return an empty candidate set rather
		// than searching for the empty string.
		return result, nil
	}

	// Region signal: the set of (city, state) where this artist has played PH
	// shows. Empty is fine — every candidate then resolves to "review" with the
	// no-region note. A query failure is non-fatal: discovery still works, it
	// just can't compute the high-confidence tier, so all candidates degrade to
	// "review". Logged at WARN so a persistent DB failure (every discovery
	// silently review-tier) is distinguishable from a genuine no-region artist.
	regions, err := s.regionsFn(artistID)
	if err != nil {
		logger.Default().Warn("discover_music_region_query_failed",
			"artist_id", artistID,
			"error", err.Error(),
		)
		regions = nil
	}

	candidates, err := s.mb.SearchArtistCandidates(ctx, artistName)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz candidate search: %w", err)
	}

	// seenAt maps a (platform, canonical-url) dedup key to the index of the
	// already-appended candidate for that link, so the same link surfaced by
	// multiple exact-name MB artists (or multiple relations on one artist)
	// appears once. On a collision the STRONGER candidate wins: MB returns
	// artists in score order, not confidence order, so a later high-tier
	// duplicate must be able to upgrade an earlier review-tier row — otherwise
	// the surviving row would carry whichever MB artist happened to come first,
	// not the best available confidence/region match.
	seenAt := make(map[string]int)

	for _, cand := range candidates {
		// Stop early if the request was cancelled — each kept candidate costs a
		// rate-limited (~1s) MB lookup, so there's no point starting another.
		if ctx.Err() != nil {
			break
		}

		// EXACT-NAME GATE (hard requirement). NEVER take the top match or use
		// score for identity.
		if NormalizeArtistName(cand.Name) != normName {
			continue
		}

		rels, relErr := s.mb.LookupArtistURLRelations(ctx, cand.ID)
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
			if idx, dup := seenAt[dedupKey]; dup {
				// Same link from another MB artist — upgrade the stored row if
				// this candidate is strictly stronger (high beats review). The
				// upgrade adopts this candidate's MB artist, region match, and
				// notes so the surviving row is internally consistent.
				if confidence == contracts.MusicConfidenceHigh &&
					result.Candidates[idx].Confidence != contracts.MusicConfidenceHigh {
					result.Candidates[idx].Confidence = confidence
					result.Candidates[idx].RegionMatch = regionMatch
					result.Candidates[idx].MBArtistID = cand.ID
					result.Candidates[idx].MBArtistName = cand.Name
					result.Candidates[idx].Notes = notes
				}
				continue
			}

			result.Candidates = append(result.Candidates, contracts.MusicLinkCandidate{
				Platform:     platform,
				URL:          normalizedURL,
				Source:       "musicbrainz",
				MBArtistID:   cand.ID,
				MBArtistName: cand.Name,
				Confidence:   confidence,
				RegionMatch:  regionMatch,
				Notes:        notes,
				// Live is left at its zero value here and filled in concurrently
				// by fillLiveness below (see that method for why probing inline
				// would serialize per-candidate timeouts).
			})
			seenAt[dedupKey] = len(result.Candidates) - 1
		}
	}

	s.fillLiveness(ctx, result.Candidates)

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
func (s *DiscoverMusicService) fillLiveness(ctx context.Context, candidates []contracts.MusicLinkCandidate) {
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
			candidates[idx].Live = s.liveness.IsLive(ctx, candidates[idx].URL)
		}(i)
	}
	wg.Wait()
}

// NormalizeArtistName lowercases, expands "&" to "and", and strips every
// non-alphanumeric character. Two names are the SAME identity iff their
// normalized forms are byte-equal. This is the exact-name gate's comparison key
// (PSY-1197): it folds "Club XCX" and "Club X.C.X" together but keeps "Club XCX"
// distinct from "Charli xcx" (→ "charlixcx" vs "clubxcx"), killing the
// famous-namesake false class at ~zero yield loss.
func NormalizeArtistName(name string) string {
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

// candidateNotes builds the human-readable note shown next to a candidate: MB's
// own disambiguation string verbatim when present (it often reads "rock band
// from Perth, Australia" / "instrumental hip-hop artist", which helps the admin
// tell same-name acts apart), plus a touring-act/namesake caveat when the region
// didn't match.
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

// ClassifyPlatformURL host-anchors a MusicBrainz relation URL and, if it is a
// Bandcamp artist subdomain/apex or an open.spotify.com artist page, returns the
// platform tag and a canonicalized URL. Exported for MBID-keyed link backfill
// (PSY-1279) and other url-rel consumers outside this package.
func ClassifyPlatformURL(rawURL string) (platform, normalized string, ok bool) {
	return classifyPlatformURL(rawURL)
}

// classifyPlatformURL host-anchors a MusicBrainz relation URL and, if it is a
// Bandcamp artist subdomain/apex or an open.spotify.com artist page, returns the
// platform tag and a CANONICALIZED URL. The host check is the identity signal —
// NOT the MB relation `type` string, which varies ("free streaming",
// "streaming", "bandcamp"). A substring check would be bypassable; this parses
// and anchors on the parsed host (pattern: ssrf host anchor) via the shared
// isAllowedPlatformHost allowlist.
//
// Canonicalization makes the returned URL a stable dedup key (MB stores the same
// link under cosmetic variants — trailing slash, scheme, tracking query, host
// case): lowercase the host, force https, drop userinfo/fragment/query, and trim
// a trailing slash. Userinfo is dropped both as URL hygiene (it would otherwise
// be shown to the admin and persisted) and so credentials can't ride along.
func classifyPlatformURL(rawURL string) (platform, normalized string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", false
	}
	host := strings.ToLower(u.Hostname())
	if !isAllowedPlatformHost(host) {
		return "", "", false
	}

	// isAllowedPlatformHost has already confirmed host is open.spotify.com or a
	// bandcamp host; the only remaining discrimination is spotify (which also
	// requires an /artist/ path) vs bandcamp.
	if host == "open.spotify.com" {
		// Only artist pages are useful links; album/track/playlist URLs are not
		// what the artist's spotify field stores.
		if !strings.Contains(u.Path, "/artist/") {
			return "", "", false
		}
		return contracts.MusicPlatformSpotify, canonicalPlatformURL(host, u.Path), true
	}
	// bandcamp.com / *.bandcamp.com
	return contracts.MusicPlatformBandcamp, canonicalPlatformURL(host, u.Path), true
}

// ClassifyReleasePlatformURL host-anchors a MusicBrainz relation URL and, if it
// is a playable RELEASE unit — a Bandcamp /album/ or /track/ page, or a Spotify
// /album/ or /track/ page — returns the platform tag and the canonicalized URL.
// The artist-flavored classifyPlatformURL is the wrong gate for releases: it
// accepts bare Bandcamp profiles (not embeddable as a release) and REJECTS
// Spotify album URLs (it requires an /artist/ path). Host anchoring, not the MB
// relation `type` string, remains the identity check — same rationale and
// canonical form as classifyPlatformURL.
func ClassifyReleasePlatformURL(rawURL string) (platform, normalized string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", false
	}
	host := strings.ToLower(u.Hostname())
	if !isAllowedPlatformHost(host) {
		return "", "", false
	}
	// The /album|/track path is the canonical embeddable unit on BOTH platforms
	// (pattern: bandcamp embed provenance; frontend findBandcampEmbedUrl +
	// parseSpotifyEmbed). Artist/profile/playlist URLs do not identify this
	// release.
	if !strings.Contains(u.Path, "/album/") && !strings.Contains(u.Path, "/track/") {
		return "", "", false
	}
	if host == "open.spotify.com" {
		return contracts.MusicPlatformSpotify, canonicalPlatformURL(host, u.Path), true
	}
	return contracts.MusicPlatformBandcamp, canonicalPlatformURL(host, u.Path), true
}

// SamePlatformArtistURL reports whether two URLs are the same Spotify-artist or
// Bandcamp link once reduced to canonical form (https, lowercased host, trimmed
// path; userinfo/query/fragment dropped). It returns false unless BOTH are
// recognized platform URLs of the same platform with the same canonical value.
//
// It lets a caller confirm a MusicBrainz candidate shares an artist's own known
// streaming link — a strong identity match — before trusting that candidate's
// location (PSY-1255 homonym guard). Only Spotify/Bandcamp are compared (the
// hosts classifyPlatformURL recognizes); other links can't anchor identity here.
func SamePlatformArtistURL(a, b string) bool {
	pa, na, oka := classifyPlatformURL(a)
	if !oka {
		return false
	}
	pb, nb, okb := classifyPlatformURL(b)
	if !okb {
		return false
	}
	return pa == pb && na == nb
}

// canonicalPlatformURL builds the stable, hygienic form used for both the dedup
// key and the value returned to the admin: always https, lowercased host (passed
// in already-lowercased), the path with a single trailing slash trimmed, and NO
// userinfo, query, or fragment. The host is a verified bandcamp/spotify host, so
// forcing https is safe (both serve https) and removes the http/https dedup
// split. The bare apex/profile ("https://artist.bandcamp.com") and a trailing-
// slash variant collapse to one key.
func canonicalPlatformURL(host, path string) string {
	p := strings.TrimRight(path, "/")
	return "https://" + host + p
}
