package catalog

import (
	"slices"
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

// The predicate is assembled by string concatenation, so a typo'd alias
// compiles fine and only fails at query time. Pin the shape here; the
// behavioral coverage lives in the charts and bill-composition integration
// suites.
func TestHeadlineSlotSQLBindsEveryColumnToTheGivenAlias(t *testing.T) {
	sql := headlineSlotSQL("sa1")

	for _, want := range []string{"sa1.show_id", "sa1.set_type", "sa1.position = 0"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("predicate does not reference %q:\n%s", want, sql)
		}
	}
	// The correlated subquery must scope itself to the bill; without its own
	// alias it would resolve set_type against the outer row and every bill
	// would read as curated.
	if !strings.Contains(sql, "curated_sa.show_id = sa1.show_id") {
		t.Fatalf("curated-bill subquery is not correlated to the outer row:\n%s", sql)
	}
	// show_id (correlation), set_type (curated branch), position (fallback).
	if got := strings.Count(sql, "sa1."); got != 3 {
		t.Fatalf("expected exactly 3 outer-alias references, got %d:\n%s", got, sql)
	}
}

// The vocabulary is the source of truth: renaming the neutral default or the
// headliner value must not leave a stale literal behind in the SQL.
func TestHeadlineSlotSQLDerivesValuesFromTheVocabulary(t *testing.T) {
	sql := headlineSlotSQL("sa")

	if !strings.Contains(sql, "'"+contracts.SetTypeHeadliner+"'") {
		t.Fatalf("predicate does not test for %q:\n%s", contracts.SetTypeHeadliner, sql)
	}
	if !strings.Contains(headlineSlotUnknownValues, "'"+contracts.SetTypePerformer+"'") {
		t.Fatalf("unknown-slot values do not include %q: %s", contracts.SetTypePerformer, headlineSlotUnknownValues)
	}
	// Every OTHER vocabulary value states a real slot and must therefore mark
	// a bill as curated rather than counting as unknown.
	for _, value := range contracts.SetTypeVocabulary() {
		if value == contracts.SetTypePerformer {
			continue
		}
		if strings.Contains(headlineSlotUnknownValues, "'"+value+"'") {
			t.Fatalf("%q states a real slot but is treated as unknown: %s", value, headlineSlotUnknownValues)
		}
	}
}

// Two concurrent creates of ONE bill listed in opposite orders must take the
// same advisory locks in the same order, or Postgres kills one with a 40P01.
// Sorting is the whole guarantee, and it is invisible at the call site, so it is
// pinned here.
func TestShowDedupLockKeysAreOrderIndependent(t *testing.T) {
	eventDate := time.Date(2027, 6, 1, 20, 0, 0, 0, time.UTC)
	venues := []contracts.CreateShowVenue{{Name: "Lock Room", City: "Phoenix", State: "AZ"}}
	headliner := contracts.SetTypeHeadliner

	forward := showDedupLockKeys(&contracts.CreateShowRequest{
		EventDate: eventDate,
		Venues:    venues,
		Artists: []contracts.CreateShowArtist{
			{Name: "Earth"},
			{Name: "Boris", SetType: &headliner},
		},
	}, eventDate)

	reversed := showDedupLockKeys(&contracts.CreateShowRequest{
		EventDate: eventDate,
		Venues:    venues,
		Artists: []contracts.CreateShowArtist{
			{Name: "Boris"},
			{Name: "Earth", SetType: &headliner},
		},
	}, eventDate)

	if len(forward) != 2 {
		t.Fatalf("both acts must be locked, got %d keys", len(forward))
	}
	if !slices.IsSorted(forward) {
		t.Errorf("keys must be acquired in sorted order, got %v", forward)
	}
	if !slices.Equal(forward, reversed) {
		t.Errorf("the same bill in either order must lock in the same order: %v vs %v", forward, reversed)
	}
}

// One act cannot take two locks: the probe compares names case-insensitively, so
// the keys must be deduplicated the same way.
func TestShowDedupLockKeysDeduplicateOneAct(t *testing.T) {
	eventDate := time.Date(2027, 6, 2, 20, 0, 0, 0, time.UTC)
	headliner := contracts.SetTypeHeadliner

	keys := showDedupLockKeys(&contracts.CreateShowRequest{
		EventDate: eventDate,
		Venues:    []contracts.CreateShowVenue{{Name: "Lock Room", City: "Phoenix", State: "AZ"}},
		Artists:   []contracts.CreateShowArtist{{Name: "earth", SetType: &headliner}},
	}, eventDate)

	if len(keys) != 1 {
		t.Errorf("one act at one venue is one lock, got %d: %v", len(keys), keys)
	}
}

// ResolveHeadlinerName is what four slug writers persist, so every branch of its
// ranking is pinned here rather than only through their integration suites.
func TestResolveHeadlinerName(t *testing.T) {
	cases := []struct {
		name string
		bill []HeadlineCandidate
		want string
	}{
		{
			name: "empty bill names nobody",
			bill: nil,
			want: "",
		},
		{
			name: "curated headliner outranks a lower position",
			bill: []HeadlineCandidate{
				{Name: "Opener", SetType: contracts.SetTypeOpener, Position: 0},
				{Name: "Headliner", SetType: contracts.SetTypeHeadliner, Position: 1},
			},
			want: "Headliner",
		},
		{
			name: "uncurated bill falls back to lowest position",
			bill: []HeadlineCandidate{
				{Name: "Second", SetType: contracts.SetTypePerformer, Position: 1},
				{Name: "First", SetType: contracts.SetTypePerformer, Position: 0},
			},
			want: "First",
		},
		{
			name: "a curated bill naming no headliner still names an act",
			bill: []HeadlineCandidate{
				{Name: "Top", SetType: contracts.SetTypePerformer, Position: 0},
				{Name: "Opener", SetType: contracts.SetTypeOpener, Position: 1},
			},
			want: "Top",
		},
		{
			name: "set_type is normalized, not compared raw",
			bill: []HeadlineCandidate{
				{Name: "Opener", SetType: "opener", Position: 0},
				{Name: "Headliner", SetType: "  HEADLINER  ", Position: 1},
			},
			want: "Headliner",
		},
		{
			name: "an unmappable label makes no claim",
			bill: []HeadlineCandidate{
				{Name: "Top", SetType: "", Position: 0},
				{Name: "Host", SetType: "host", Position: 1},
			},
			want: "Top",
		},
		{
			name: "two curated headliners tie on position",
			bill: []HeadlineCandidate{
				{Name: "Later", SetType: contracts.SetTypeHeadliner, Position: 3},
				{Name: "Earlier", SetType: contracts.SetTypeHeadliner, Position: 2},
			},
			want: "Earlier",
		},
		{
			name: "equal position and rank leave bill order in charge",
			bill: []HeadlineCandidate{
				{Name: "Listed First", SetType: contracts.SetTypePerformer, Position: 0},
				{Name: "Listed Second", SetType: contracts.SetTypePerformer, Position: 0},
			},
			want: "Listed First",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveHeadlinerName(tc.bill); got != tc.want {
				t.Errorf("ResolveHeadlinerName = %q, want %q", got, tc.want)
			}
		})
	}
}
