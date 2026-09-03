package catalog

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// boundsEqual reports whether two shard bounds name the same point. nil is the
// open bound and equals only itself; instants compare by Equal so a zone or
// monotonic-clock difference cannot make two identical instants look distinct.
func boundsEqual(a, b any) bool {
	at, aok := a.(time.Time)
	bt, bok := b.(time.Time)
	if aok || bok {
		return aok && bok && at.Equal(bt)
	}
	return a == b
}

// boundsAscend reports whether [from, before) is a non-empty range. Bounds of
// different types never ascend, which is itself the finding: one family's
// ranges must all be cut on one column.
func boundsAscend(from, before any) bool {
	switch f := from.(type) {
	case time.Time:
		b, ok := before.(time.Time)
		return ok && f.Before(b)
	case string:
		b, ok := before.(string)
		return ok && f < b
	}
	return false
}

// TestSitemapShardsPartitionTheirFamilies is the structural half of the
// sub-shard guard — no database, so it runs in short mode too.
//
// The bounds are a hand-maintained table, and the failure mode of getting them
// wrong is invisible: a gap drops every row inside it out of the sitemap with
// every shard still rendering a valid document, and an overlap announces the
// same URL from two shards. Neither shows up in a byte-size measurement, which
// is the only other thing anyone re-checks when re-tuning the cut points.
//
// A partition of a totally ordered domain into half-open ranges is total and
// disjoint exactly when it is contiguous and open at both outer ends, so those
// are what this asserts, per family.
func TestSitemapShardsPartitionTheirFamilies(t *testing.T) {
	if len(sitemapShardsByFamily) == 0 {
		t.Fatal("sitemapShardsByFamily is empty — no family is sub-sharded")
	}

	for family, shards := range sitemapShardsByFamily {
		if len(shards) < 2 {
			t.Errorf("family %q has %d shard(s) — sub-sharding needs at least 2", family, len(shards))
			continue
		}

		if shards[0].from != nil {
			t.Errorf("family %q: first shard %q has a lower bound — it must be open below, or values sorting under it belong to no shard",
				family, shards[0].id)
		}
		if last := shards[len(shards)-1]; last.before != nil {
			t.Errorf("family %q: last shard %q has an upper bound — it must be open above", family, last.id)
		}

		for i, shard := range shards {
			// The field must agree with the key it is filed under: everything
			// downstream reads `shard.family` rather than the group it came from,
			// so a mismatch would narrow one family's scope with another's bounds.
			if shard.family != family {
				t.Errorf("shard %q is filed under %q but names family %q", shard.id, family, shard.family)
			}
			// The id is the wire value AND the frontend's route segment, so a
			// stray separator or a missing family prefix breaks the sitemap index
			// rather than only reading oddly. It is also what makes ids unique
			// across families without a second check.
			if !strings.HasPrefix(shard.id, family+"-") {
				t.Errorf("shard id %q does not start with the family it slices (%q)", shard.id, family)
			}
			// One family, one cut column: mixed columns would make the ranges
			// incomparable and the partition argument meaningless.
			if shard.column != shards[0].column {
				t.Errorf("family %q: shard %q cuts on %q while %q cuts on %q — a family partitions on one column",
					family, shard.id, shard.column, shards[0].id, shards[0].column)
			}

			if i == 0 {
				continue
			}
			if prev := shards[i-1]; !boundsEqual(prev.before, shard.from) {
				t.Errorf("family %q: gap or overlap between %q (before %v) and %q (from %v): the ranges must be contiguous",
					family, prev.id, prev.before, shard.id, shard.from)
			}
			// Ordering is checked in Go, which is NOT the order the database
			// applies to a string bound (see the collation note on
			// releaseShards). That is fine for the cut points this table is meant
			// to hold — plain lowercase letters, where byte order and en_US.utf8
			// agree — and it catches the realistic typo of an inverted pair. It is
			// NOT authoritative for a cut point containing punctuation or digits:
			// such a bound could pass here and still select an empty range in
			// Postgres. If a cut point ever needs one, assert its placement in the
			// integration test, where the database decides, rather than
			// strengthening this line. Instant bounds have no such caveat.
			if shard.before != nil && !boundsAscend(shard.from, shard.before) {
				t.Errorf("family %q: shard %q has an empty or inverted range [%v, %v)", family, shard.id, shard.from, shard.before)
			}
		}
	}
}

// TestSitemapShardIDsAreUnique guards the one property the per-family checks
// cannot see: two families claiming one id would make the wire value ambiguous
// and hand the frontend a shard whose rows belong to the other family.
func TestSitemapShardIDsAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, shard := range sitemapShards {
		if prev, dup := seen[shard.id]; dup {
			t.Errorf("shard id %q is claimed by both %q and %q", shard.id, prev, shard.family)
		}
		seen[shard.id] = shard.family
	}
}

// TestSitemapShardByIDRejectsAFamilyName guards the resolution step: a family
// name must NOT resolve to a shard, or a caller asking for the whole family
// would silently receive one slice of it.
func TestSitemapShardByIDRejectsAFamilyName(t *testing.T) {
	for _, id := range []string{"releases", "shows", "artists", "", "releases-", "shows-", "shows-2026"} {
		if shard := sitemapShardByID(id); shard != nil {
			t.Errorf("sitemapShardByID(%q) = %q, want nil", id, shard.id)
		}
	}
	for _, shard := range sitemapShards {
		if got := sitemapShardByID(shard.id); got == nil || got.id != shard.id {
			t.Errorf("sitemapShardByID(%q) did not resolve to its own shard", shard.id)
		}
	}
}

// TestSitemapShardsFlattenInFamilyOrder pins the ordering the wire enum is
// built from. Ranging sitemapShardsByFamily directly would reorder
// SitemapFamilyValues() on every run, so the enum literal on
// GetSitemapEntriesRequest could never be kept equal to it.
func TestSitemapShardsFlattenInFamilyOrder(t *testing.T) {
	want := []string{}
	for _, family := range sitemapFamilies {
		for _, shard := range sitemapShardsByFamily[family] {
			want = append(want, shard.id)
		}
	}

	got := make([]string, 0, len(sitemapShards))
	for _, shard := range sitemapShards {
		got = append(got, shard.id)
	}

	if !slices.Equal(got, want) {
		t.Errorf("sitemapShards ids = %v, want %v", got, want)
	}
	// A family sub-sharded but absent from sitemapFamilies would be dropped by
	// the flatten and never reach the enum, so its ids would 422 forever.
	if len(got) != countShards() {
		t.Errorf("sitemapShards holds %d ids, sitemapShardsByFamily holds %d — a family was dropped by the flatten", len(got), countShards())
	}
}

func countShards() int {
	n := 0
	for _, shards := range sitemapShardsByFamily {
		n += len(shards)
	}
	return n
}

// TestShowShardYearsAreContiguous pins the assumption showShards' totality
// argument rests on. The head shard ends where the first enumerated year
// begins and the tail starts where the last one ends, so a skipped year in
// between would be a gap no other check can see: every month shard would still
// be contiguous with its neighbour in the slice, while the shows dated inside
// the missing year belonged to no shard at all.
func TestShowShardYearsAreContiguous(t *testing.T) {
	if len(showShardYears) == 0 {
		t.Fatal("showShardYears is empty — the shows family would be served entirely by its open shards")
	}
	for i := 1; i < len(showShardYears); i++ {
		if showShardYears[i] != showShardYears[i-1]+1 {
			t.Errorf("showShardYears jumps from %d to %d — every year in the span must be enumerated",
				showShardYears[i-1], showShardYears[i])
		}
	}
}

// TestShowShardYearsStayAheadOfTheCalendar is the only automated warning that
// the enumerated span is running out, and it is deliberately time-dependent.
//
// Nothing else fires until it is too late: every other test here passes for any
// span, and the data-cache budget gate does not warn — it fails the production
// build at 80% of the cap, which is the incident this sharding exists to end.
// Shows are ingested on a rolling forward horizon, so the open tail shard starts
// filling as soon as dates in the year after the span are announced and crosses
// the gate a few months later.
//
// Requiring the span to reach NEXT year buys roughly twelve months of notice: it
// goes red on 1 January of the last enumerated year, long before anything is
// dated past the span. Fixing it is the four-edit procedure on showShardYears.
func TestShowShardYearsStayAheadOfTheCalendar(t *testing.T) {
	last := showShardYears[len(showShardYears)-1]
	if want := time.Now().UTC().Year() + 1; last < want {
		t.Errorf("showShardYears ends at %d, but shows dated %d and later already fall in the open tail shard. "+
			"Append years through %d — see the four-edit procedure on showShardYears.", last, last+1, want)
	}
}

// TestShowShardsAreMonthlyAndUTC pins the two facts the ids promise: each
// enumerated shard covers exactly one UTC calendar month, and its id names that
// month. A shard whose id said 2026-09 while its range covered October would be
// invisible to every other check here.
func TestShowShardsAreMonthlyAndUTC(t *testing.T) {
	for _, shard := range sitemapShardsByFamily["shows"] {
		if shard.from == nil || shard.before == nil {
			continue // the open head and tail shards
		}
		from, ok := shard.from.(time.Time)
		if !ok {
			t.Errorf("shard %q has a non-instant lower bound %v", shard.id, shard.from)
			continue
		}
		before, ok := shard.before.(time.Time)
		if !ok {
			t.Errorf("shard %q has a non-instant upper bound %v", shard.id, shard.before)
			continue
		}
		if from.Location() != time.UTC || before.Location() != time.UTC {
			t.Errorf("shard %q has bounds outside UTC (%v, %v)", shard.id, from.Location(), before.Location())
		}
		if want := from.AddDate(0, 1, 0); !before.Equal(want) {
			t.Errorf("shard %q spans [%v, %v), which is not one calendar month", shard.id, from, before)
		}
		if want := "shows-" + from.Format("2006-01"); shard.id != want {
			t.Errorf("shard %q covers %v, so its id must be %q", shard.id, from.Format("2006-01"), want)
		}
	}
}
