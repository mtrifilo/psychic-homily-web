package catalog

import "psychic-homily-backend/internal/services/contracts"

// Genre-family rollup for the Atlas globe dot tint (PSY-1315). Raw genre tags are
// a flat folksonomy ("post-punk", "garage-punk", "coldwave"); there is no
// populated genre hierarchy to roll them up to, so this is a CURATED map from tag
// slug -> one of a small fixed set of families. The frontend paints each family a
// distinct colorblind-safe color and renders the legend
// (frontend/features/scenes/genreFamilies.ts owns family -> color + label).
//
// The family KEYS below are the cross-layer contract: they MUST stay in sync with
// GENRE_FAMILIES in that frontend file (a key emitted here but unknown there
// leaves the dot un-tinted with no legend entry). Ambiguous tags are resolved
// here by decree (goth -> Punk via the post-punk lineage; disco -> Electronic as
// dance; noise -> Jazz/Experimental while noise-rock -> Rock) — that decree is the
// whole point of a curated map over a folksonomy.
//
// A scene's dominant family is emitted only when one family holds a confident
// share of the scene's genre mass; otherwise the scene stays neutral (the globe's
// default orange). See dominantGenreFamily.

// Genre-family keys. Stable identifiers shared with the frontend; renaming one is
// a breaking change to that contract.
const (
	genreFamilyPunk        = "punk_hardcore"
	genreFamilyMetal       = "metal"
	genreFamilyRockIndie   = "rock_indie"
	genreFamilyElectronic  = "electronic"
	genreFamilyHipHop      = "hip_hop"
	genreFamilyFolkCountry = "folk_country"
	genreFamilyJazzExp     = "jazz_experimental"
	genreFamilyPopSoul     = "pop_soul"
)

// genreFamilyKeys is the fixed iteration order used to break ties deterministically
// when two families hold equal mass (map iteration is unordered). Order is not
// otherwise meaningful — the frontend owns family -> color assignment.
var genreFamilyKeys = []string{
	genreFamilyPunk, genreFamilyMetal, genreFamilyRockIndie, genreFamilyElectronic,
	genreFamilyHipHop, genreFamilyFolkCountry, genreFamilyJazzExp, genreFamilyPopSoul,
}

// genreFamilyBySlug maps a genre tag slug to its family. Grounded in the actual
// catalog genre tags (PSY-1315 dev-DB audit) plus common variants likely to
// appear as the catalog grows (metal/jazz/hip-hop families have few or no rows
// today but exist for when they do). An unmapped slug still counts toward a
// scene's TOTAL genre mass but toward NO family, so a scene dominated by unmapped
// genres stays neutral — correct, since it isn't one of the fixed families.
var genreFamilyBySlug = map[string]string{
	// Punk & Hardcore — the catalog's core. goth/gothic ride the post-punk lineage.
	"punk": genreFamilyPunk, "garage-punk": genreFamilyPunk, "hardcore-punk": genreFamilyPunk,
	"hardcore": genreFamilyPunk, "post-punk": genreFamilyPunk, "powerviolence": genreFamilyPunk,
	"screamo": genreFamilyPunk, "emo": genreFamilyPunk, "egg-punk": genreFamilyPunk,
	"d-beat": genreFamilyPunk, "crust": genreFamilyPunk, "oi": genreFamilyPunk,
	"goth": genreFamilyPunk, "gothic": genreFamilyPunk, "cowpunk": genreFamilyPunk,

	// Metal (no catalog rows yet; kept for growth).
	"metal": genreFamilyMetal, "doom": genreFamilyMetal, "doom-metal": genreFamilyMetal,
	"black-metal": genreFamilyMetal, "death-metal": genreFamilyMetal, "sludge": genreFamilyMetal,
	"grindcore": genreFamilyMetal, "metalcore": genreFamilyMetal, "thrash": genreFamilyMetal,
	"stoner-metal": genreFamilyMetal, "stoner-rock": genreFamilyMetal,

	// Rock & Indie — garage/psych/shoegaze/dream-pop/noise-rock live here.
	"rock": genreFamilyRockIndie, "indie-rock": genreFamilyRockIndie, "noise-rock": genreFamilyRockIndie,
	"garage-rock": genreFamilyRockIndie, "garage": genreFamilyRockIndie, "art-rock": genreFamilyRockIndie,
	"grunge": genreFamilyRockIndie, "rock-roll": genreFamilyRockIndie, "rocknroll": genreFamilyRockIndie,
	"psychedelic": genreFamilyRockIndie, "psych": genreFamilyRockIndie, "shoegaze": genreFamilyRockIndie,
	"dream-pop": genreFamilyRockIndie, "alternative": genreFamilyRockIndie, "indie": genreFamilyRockIndie,
	"indie-pop": genreFamilyRockIndie, "math-rock": genreFamilyRockIndie, "post-rock": genreFamilyRockIndie,

	// Electronic — synth/wave/industrial cluster; disco filed here as dance.
	"electronic": genreFamilyElectronic, "industrial": genreFamilyElectronic, "coldwave": genreFamilyElectronic,
	"darkwave": genreFamilyElectronic, "new-wave": genreFamilyElectronic, "synth-pop": genreFamilyElectronic,
	"synthpop": genreFamilyElectronic, "synth": genreFamilyElectronic, "synthwave": genreFamilyElectronic,
	"ambient": genreFamilyElectronic, "techno": genreFamilyElectronic, "house": genreFamilyElectronic,
	"idm": genreFamilyElectronic, "ebm": genreFamilyElectronic, "disco": genreFamilyElectronic,

	// Hip-Hop & Rap.
	"hip-hop": genreFamilyHipHop, "rap": genreFamilyHipHop, "trap": genreFamilyHipHop,
	"boom-bap": genreFamilyHipHop,

	// Folk & Country — roots/americana/singer-songwriter; dream-folk/dark-folk here.
	"folk": genreFamilyFolkCountry, "dream-folk": genreFamilyFolkCountry, "dark-folk": genreFamilyFolkCountry,
	"americana": genreFamilyFolkCountry, "country": genreFamilyFolkCountry, "country-rock": genreFamilyFolkCountry,
	"roots": genreFamilyFolkCountry, "singer-songwriter": genreFamilyFolkCountry, "bluegrass": genreFamilyFolkCountry,
	"alt-country": genreFamilyFolkCountry,

	// Jazz & Experimental — noise (not noise-rock), drone, avant, field recording.
	"experimental": genreFamilyJazzExp, "avant-garde": genreFamilyJazzExp, "drone": genreFamilyJazzExp,
	"noise": genreFamilyJazzExp, "field-recording": genreFamilyJazzExp, "cinematic": genreFamilyJazzExp,
	"soundtracks": genreFamilyJazzExp, "jazz": genreFamilyJazzExp, "free-jazz": genreFamilyJazzExp,
	"no-wave": genreFamilyJazzExp, "musique-concrete": genreFamilyJazzExp,

	// Pop, R&B & Soul.
	"pop": genreFamilyPopSoul, "rb": genreFamilyPopSoul, "rnb": genreFamilyPopSoul,
	"r-and-b": genreFamilyPopSoul, "funk": genreFamilyPopSoul, "soul": genreFamilyPopSoul,
	"gospel": genreFamilyPopSoul, "blues": genreFamilyPopSoul,

	// Deliberately unmapped -> Other/neutral: world/regional tags outside the eight
	// western-rooted families (corridos, cumbia, lukthung). They count toward total
	// mass, so a scene must be dominated by a mapped family to tint.
}

// dominantGenreFamilyMinShare: a family must hold at least this share of a scene's
// genre mass to tint the dot (PSY-1315, user decision "confident dominance"). Below
// it the scene is genuinely mixed and stays neutral (the globe's default orange).
const dominantGenreFamilyMinShare = 0.40

// dominantGenreFamilyMinTagged: a scene needs at least this many tagged-artist
// units before a dominant family is trusted, so a 1-2 artist scene can't read as
// "100% punk". Deliberately lower than sceneGenreMinTaggedArtists (the detailed
// distribution endpoint's 30-artist bar) — the dot tint is a coarse discovery
// signal, not a published stat. Tunable.
const dominantGenreFamilyMinTagged = 5

// dominantGenreFamily returns the family key that confidently dominates a scene's
// genre distribution, or "" when none does. counts is the scene's per-genre-slug
// distinct-artist counts (the same mass GetSceneGenreDistribution/diversity use).
// A family dominates when it holds >= dominantGenreFamilyMinShare of the TOTAL
// genre mass (every genre, mapped or not, is in the denominator) AND the scene has
// at least dominantGenreFamilyMinTagged tagged-artist units. Ties break by
// genreFamilyKeys order so the result is deterministic.
func dominantGenreFamily(counts []contracts.GenreCount) string {
	total := 0
	byFamily := make(map[string]int, len(genreFamilyKeys))
	for _, c := range counts {
		if c.Count <= 0 {
			continue
		}
		total += c.Count
		if fam, ok := genreFamilyBySlug[c.Slug]; ok {
			byFamily[fam] += c.Count
		}
	}
	if total < dominantGenreFamilyMinTagged {
		return ""
	}

	bestFam, bestMass := "", 0
	for _, fam := range genreFamilyKeys {
		if mass := byFamily[fam]; mass > bestMass {
			bestFam, bestMass = fam, mass
		}
	}
	if bestFam == "" {
		return ""
	}
	if float64(bestMass) >= dominantGenreFamilyMinShare*float64(total) {
		return bestFam
	}
	return ""
}
