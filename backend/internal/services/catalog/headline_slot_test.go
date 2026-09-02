package catalog

import (
	"strings"
	"testing"

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

// ResolveHeadlinerName is what three slug writers persist, so every branch of
// its ranking is pinned here rather than only through their integration suites.
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
