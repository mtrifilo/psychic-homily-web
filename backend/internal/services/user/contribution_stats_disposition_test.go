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

// moderationActionNames is the set of audit actions that feed ModerationActions,
// derived from the dispatch map rather than written out, so the timeline-subset
// assertion in contributor_profile_test.go cannot drift from what is counted.
var moderationActionNames = func() map[string]bool {
	// One struct, so the selectors are compared by the ADDRESS they return
	// within it: two selectors naming the same field yield the same pointer.
	stats := &contracts.ContributionStats{}
	names := map[string]bool{}
	for action, counter := range contributionStatActions {
		if counter(stats) == &stats.ModerationActions {
			names[action] = true
		}
	}
	return names
}()

// moderationActionNameList is moderationActionNames as a sorted slice, for a
// query's IN clause.
func moderationActionNameList() []string {
	names := make([]string, 0, len(moderationActionNames))
	for action := range moderationActionNames {
		names = append(names, action)
	}
	sort.Strings(names)
	return names
}

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
	// statsDerived: the field is computed from the others in this struct and runs
	// NO QUERY OF ITS OWN, so it inherits their dispositions. A field that reads
	// like a ratio of two others but issues its own queries is not this; it takes
	// the disposition its queries earn.
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

// contributionStatDispositions is the whole inventory, keyed by the struct
// field name. A field missing here fails the test, and adding one is a claim
// about it.
var contributionStatDispositions = map[string]statsDisposition{
	// Sourced from audit_logs through the timeline's own condition, so a row
	// counted here is a row that timeline lists for the same actor. The reverse
	// does not hold: the counters read 17 named actions and the timeline reads
	// every row, so these are a subset of it, which is the direction that
	// matters. moderation_actions is the one that needed the gate: approve_show
	// and reject_show name shows, and a rejected show is gated by definition.
	//
	// The last three are ALSO fed by a second query, the entity_edit_audit_logs
	// group-by. That table's entity_type is a free column, so the second arm
	// excludes the gated discriminators outright rather than relying on its
	// writers; without that, one label here would be covering one gated query and
	// one open one.
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

	// ApprovalRate is not derived from the fields above despite reading like a
	// ratio of them: it runs two queries of its own over pending_entity_edits,
	// which ValidPendingEditEntityTypes holds to artist, venue, festival, release
	// and label. Recorded UNGATED rather than derived, so that extending approval
	// rate to show submissions, the obvious next step since approve_show and
	// reject_show already exist, has to come back here.
	"ApprovalRate": statsUngated,

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

	// The one field genuinely computed from the others: a sum, with no query.
	"TotalContributions": statsDerived,
}

func TestEveryContributionStatHasADisposition(t *testing.T) {
	fields := reflect.TypeOf(contracts.ContributionStats{})

	var undecided []string
	for i := range fields.NumField() {
		name := fields.Field(i).Name
		if _, ok := contributionStatDispositions[name]; !ok {
			undecided = append(undecided, name)
		}
	}

	if len(undecided) > 0 {
		sort.Strings(undecided)
		t.Errorf("%d contribution counter(s) have no recorded position on the show and "+
			"collection visibility rules:\n  %v\n\nA counter is a listing restated as one "+
			"number, and this profile is anonymous, so a whole count beside a filtered "+
			"listing of the same rows publishes the withheld ones by subtraction. Add each "+
			"to contributionStatDispositions with the disposition that is TRUE of it, and "+
			"if it is statsOpen, say in the entry why.",
			len(undecided), undecided)
	}
}

// statsOpenCounters is the set of counters that CAN name a gated entity and are
// deliberately not narrowed. Every one of them is a live disclosure.
//
// Pinned as a set rather than left to the map above, so adding an open counter
// is a two-line change a reviewer sees rather than one word in a table. The
// direction that matters is growth: narrowing one of these is a fix and only
// needs this list shortened.
var statsOpenCounters = map[string]bool{
	"ReportsFiled":    true,
	"ReportsResolved": true,
}

func TestNoNewOpenContributionStat(t *testing.T) {
	for name, disposition := range contributionStatDispositions {
		if disposition == statsOpen && !statsOpenCounters[name] {
			t.Errorf("%s is recorded %s and is not in statsOpenCounters: a counter that can "+
				"name a gated entity and is not narrowed publishes the withheld rows by "+
				"subtraction on an anonymous profile. Narrow it, or add it here with the "+
				"reason in its map entry.", name, disposition)
		}
		if disposition != statsOpen && statsOpenCounters[name] {
			t.Errorf("%s is listed in statsOpenCounters but is recorded %s, so remove it from "+
				"the list, the disclosure is closed", name, disposition)
		}
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
	for name := range contributionStatDispositions {
		if !known[name] {
			stale = append(stale, name)
		}
	}

	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d entr(ies) in contributionStatDispositions name fields that are not on "+
			"contracts.ContributionStats:\n  %v", len(stale), stale)
	}
}
