package auth

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Signed-in home layout: which sections the home page renders and in what
// order. Stored in user_preferences.home_layout as a nullable JSONB document;
// NULL means the shipped default layout, a state a grid of boolean columns
// could not represent (see the migration for why).
//
// This package, not services/user, is the home for the shape and the rules
// because contracts imports models/auth, so the service that writes the
// document, the handler that rejects a bad one and the interface between them
// can all name the same type.

// HomeLayoutVersion is the only document version this server accepts. A newer
// client's document may place meaning in fields this build does not model, so
// storing it after a partial read would silently discard them.
const HomeLayoutVersion = 1

// Section ids. These are a STORAGE CONTRACT shared with the frontend, so they
// are renamed only alongside a migration that rewrites stored documents.
const (
	HomeSectionSavedShows     = "saved_shows"
	HomeSectionNearbyShows    = "nearby_shows"
	HomeSectionCommunityStats = "community_stats"
	HomeSectionCityGraph      = "city_graph"
	HomeSectionRadioShows     = "radio_shows"
)

// knownHomeSections is the whitelist, in the order the sections are intended
// to ship in. The backend is its single home: an id the server does not know
// can never be rendered, so accepting one would store a section the user can
// neither see nor remove.
//
// A document may OMIT ids, and Validate accepts that on the CONTRACT that a
// client appends any known section the document leaves out, in this order.
// That is what lets a new section ship without rewriting stored rows. The
// consuming frontend is not built yet (PSY-2104), so nothing enforces its half
// of that contract today.
//
// These ids are unverified against a renderer: no component keyed to them
// exists yet. Renaming one after the first production write costs a data
// migration that rewrites stored documents, so they should be confirmed
// against the real sections before that write happens.
var knownHomeSections = []string{
	HomeSectionSavedShows,
	HomeSectionNearbyShows,
	HomeSectionCommunityStats,
	HomeSectionCityGraph,
	HomeSectionRadioShows,
}

// ErrInvalidHomeLayout marks every rejection below, so one errors.Is at the
// transport boundary separates "the client sent a bad document" (422) from
// "the write failed" (500). The wrapped text says which rule was broken.
var ErrInvalidHomeLayout = errors.New("invalid home layout")

// homeSectionVocabularyCSV renders the whitelist in the CSV form the enum tag
// on HomeLayoutSection.ID must spell literally. Test-only: struct tags must be
// constant literals, so the tag cannot be built from this slice, and
// TestHomeSectionEnumTagMatchesVocabulary is the join that keeps them in step.
func homeSectionVocabularyCSV() string {
	return strings.Join(knownHomeSections, ",")
}

// HomeLayoutSection is one section's placement and visibility. Position in the
// enclosing slice is the placement; a hidden section keeps its slot.
type HomeLayoutSection struct {
	ID      string `json:"id" enum:"saved_shows,nearby_shows,community_stats,city_graph,radio_shows" doc:"Section id"`
	Visible bool   `json:"visible" doc:"Whether the section renders"`
}

// HomeLayout is the whole stored document. Replace semantics: a write carries
// every section the user wants placed, and the stored value is exactly what
// the next read returns.
type HomeLayout struct {
	Version  int                 `json:"version" doc:"Document version; must be 1"`
	Sections []HomeLayoutSection `json:"sections" maxItems:"5" doc:"Sections in render order; a hidden section keeps its slot"`
}

// Validate reports whether the document may be stored. Every rejection wraps
// ErrInvalidHomeLayout.
//
// The enum tag on HomeLayoutSection.ID rejects an unknown id at the transport
// layer before this runs. These rules are still the authority: they are what a
// non-HTTP caller of the service gets, and they cover the duplicate and
// version rules no schema tag can express.
func (l *HomeLayout) Validate() error {
	if l == nil {
		return fmt.Errorf("%w: document is missing", ErrInvalidHomeLayout)
	}
	if l.Version != HomeLayoutVersion {
		return fmt.Errorf("%w: unsupported version %d, expected %d",
			ErrInvalidHomeLayout, l.Version, HomeLayoutVersion)
	}
	// A document cannot name more sections than exist, since duplicates are
	// rejected below. Checked up front so the error names the whole-document
	// problem rather than whichever duplicate happens to come first. This is a
	// correctness rule, NOT a cost bound: over HTTP the body is already decoded
	// and schema-validated before Validate runs. maxItems on the field is what
	// states the cap to clients; TestHomeSectionMaxItemsMatchesVocabulary pins
	// the two together.
	if len(l.Sections) > len(knownHomeSections) {
		return fmt.Errorf("%w: %d sections exceeds the %d known sections",
			ErrInvalidHomeLayout, len(l.Sections), len(knownHomeSections))
	}

	seen := make(map[string]struct{}, len(l.Sections))
	for _, section := range l.Sections {
		// Exact match: no trimming, no case folding. The ids are generated by
		// the client, so a value needing repair is a client defect, and
		// repairing it here would let two spellings of one section look valid.
		if !slices.Contains(knownHomeSections, section.ID) {
			return fmt.Errorf("%w: unknown section id %q", ErrInvalidHomeLayout, section.ID)
		}
		if _, duplicate := seen[section.ID]; duplicate {
			return fmt.Errorf("%w: duplicate section id %q", ErrInvalidHomeLayout, section.ID)
		}
		seen[section.ID] = struct{}{}
	}
	return nil
}
