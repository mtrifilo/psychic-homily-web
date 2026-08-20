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
