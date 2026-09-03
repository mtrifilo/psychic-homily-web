package user

import (
	"reflect"
	"sort"
	"testing"

	"psychic-homily-backend/internal/services/contracts"
)

// Every counter GetContributionStats publishes must have a decided position on
// the show and collection visibility rules.
//
// A counter is a listing restated as one number, and the public profile is
// anonymous. A whole count sitting beside a filtered listing of the same rows
// reports the withheld ones by subtraction, which is the disclosure the
// timeline's gate exists to prevent, expressed as arithmetic instead of as rows.
//
// The gates live inside one long function, which makes them forgettable the way
// the route gates were: a counter added next year gets no gate, no failing test,
// and the leak is found by the next audit. This file is the structural answer.
// It enumerates the FIELDS of contracts.ContributionStats by reflection and
// fails unless each one appears below with a recorded disposition.
//
// It pins the INVENTORY, not the behaviour. That a narrowed counter actually
// moves with the viewer is contributor_profile_test.go's job.

// statsDisposition records why a counter is safe.
type statsDisposition int

const (
	// statsNarrowed: the query carries a visibility condition from
	// services/shared or from contributionVisibilitySQL, so the number counts
	// only rows the viewer may see.
	statsNarrowed statsDisposition = iota
	// statsUngated: the counter reads a table with no read-time visibility rule
	// on any entity it can name, so there is nothing to withhold.
	statsUngated
	// statsOpen: the counter can name a gated entity and is NOT narrowed. The
	// entry carries the reason, and the reason is a product question rather than
	// a missing spelling. Adding one of these is a disclosure, not a default.
	statsOpen
	// statsDerived: the field is computed from the others in this struct, so it
	// inherits their dispositions and has no query of its own.
	statsDerived
)

// String names a disposition so a failure reads as a name rather than as an
// integer whose meaning depends on the declaration order above.
func (d statsDisposition) String() string {
	switch d {
	case statsNarrowed:
		return "statsNarrowed"
	case statsUngated:
		return "statsUngated"
	case statsOpen:
		return "statsOpen"
	case statsDerived:
		return "statsDerived"
	}
	return "unknown statsDisposition"
}

// contributionStatsDispositions is the whole inventory, keyed by the struct
// field name. A field missing here fails the test, and adding one is a claim
// about it.
var contributionStatsDispositions = map[string]statsDisposition{
	// Sourced from audit_logs through the timeline's own condition, so these
	// four count exactly the rows GET /users/{username}/contributions lists for
	// the same actor. moderation_actions is the one that needed it:
	// approve_show and reject_show name shows, and a rejected show is gated by
	// definition.
	"ModerationActions": statsNarrowed,
	"ReleasesCreated":   statsNarrowed,
	"LabelsCreated":     statsNarrowed,
	"FestivalsCreated":  statsNarrowed,

	// Narrowed against their own tables' rules. tag_votes is polymorphic and can
	// name a gated show or a private collection, so it takes the registry-backed
	// condition; catalogm.TagEntityTypes and the registry hold the same seven
	// types, so nothing is dropped for being unregistered.
	"ShowsSubmitted":          statsNarrowed,
	"RevisionsMade":           statsNarrowed,
	"CollectionItemsAdded":    statsNarrowed,
	"CollectionSubscriptions": statsNarrowed,
	"TagVotesCast":            statsNarrowed,

	// These name no entity with a read-time rule. Venues and artists are always
	// visible; pending_entity_edits is held to adminm.ValidPendingEditEntityTypes
	// and entity_edit_audit_logs is written with artist, release, label, festival
	// and scene, so neither admits show or collection; relationship votes are
	// artist-to-artist and request votes name a community request; and
	// validFollowEntityTypes (services/engagement/follow.go) has no show or
	// collection key, which is what makes the two social counts safe.
	"VenuesSubmitted":       statsUngated,
	"VenueEditsSubmitted":   statsUngated,
	"ArtistsEdited":         statsUngated,
	"PendingEditsSubmitted": statsUngated,
	"RelationshipVotesCast": statsUngated,
	"RequestVotesCast":      statsUngated,
	"FollowersCount":        statsUngated,
	"FollowingCount":        statsUngated,

	// OPEN, and named rather than hidden. Both read the three report TABLES
	// rather than audit_logs, so the timeline's condition does not reach them,
	// and entity_reports carries a polymorphic entity_type whose vocabulary is
	// wider than the visibility registry's: a fail-closed gate there would drop
	// the count for every type nobody has dispositioned, silently. What a
	// report's existence may disclose, and to whom, is a product question.
	// show_reports in particular counts a report the submitter filed on their
	// own withdrawn show.
	"ReportsFiled":    statsOpen,
	"ReportsResolved": statsOpen,

	// Computed from the fields above.
	"ApprovalRate":       statsDerived,
	"TotalContributions": statsDerived,
}

func TestEveryContributionStatHasADisposition(t *testing.T) {
	fields := reflect.TypeOf(contracts.ContributionStats{})

	var undecided []string
	for i := range fields.NumField() {
		name := fields.Field(i).Name
		if _, ok := contributionStatsDispositions[name]; !ok {
			undecided = append(undecided, name)
		}
	}

	if len(undecided) > 0 {
		sort.Strings(undecided)
		t.Errorf("%d contribution counter(s) have no recorded position on the show and "+
			"collection visibility rules:\n  %v\n\nA counter is a listing restated as one "+
			"number, and this profile is anonymous, so a whole count beside a filtered "+
			"listing of the same rows publishes the withheld ones by subtraction. Add each "+
			"to contributionStatsDispositions with the disposition that is TRUE of it, and "+
			"if it is statsOpen, say in the entry why.",
			len(undecided), undecided)
	}
}

// A stale entry is a claim about nothing, and it hides the removal of a counter
// a reader believes is still dispositioned.
func TestContributionStatsDispositionsHasNoStaleEntries(t *testing.T) {
	fields := reflect.TypeOf(contracts.ContributionStats{})

	known := map[string]bool{}
	for i := range fields.NumField() {
		known[fields.Field(i).Name] = true
	}

	var stale []string
	for name := range contributionStatsDispositions {
		if !known[name] {
			stale = append(stale, name)
		}
	}

	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d entr(ies) in contributionStatsDispositions name fields that are not on "+
			"contracts.ContributionStats:\n  %v", len(stale), stale)
	}
}
