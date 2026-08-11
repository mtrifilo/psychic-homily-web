package catalog

import (
	"strings"
	"testing"
)

// TestReleaseShardsPartitionTheFamily is the structural half of the releases
// sub-shard guard (PSY-1763) — no database, so it runs in short mode too.
//
// The bounds are a hand-maintained table, and the failure mode of getting them
// wrong is invisible: a gap drops every slug inside it out of the sitemap with
// every shard still rendering a valid document, and an overlap announces the
// same URL from two shards. Neither shows up in a byte-size measurement, which
// is the only other thing anyone re-checks when re-tuning the cut points.
//
// A partition of a totally ordered domain into half-open ranges is total and
// disjoint exactly when it is contiguous and open at both outer ends, so those
// are what this asserts.
func TestReleaseShardsPartitionTheFamily(t *testing.T) {
	if len(releaseShards) < 2 {
		t.Fatalf("releaseShards has %d entries — sub-sharding needs at least 2", len(releaseShards))
	}

	if releaseShards[0].from != "" {
		t.Errorf("first shard %q has lower bound %q — it must be open below, or slugs sorting under it (digits, hyphens) belong to no shard",
			releaseShards[0].id, releaseShards[0].from)
	}
	last := releaseShards[len(releaseShards)-1]
	if last.before != "" {
		t.Errorf("last shard %q has upper bound %q — it must be open above", last.id, last.before)
	}

	seen := map[string]bool{}
	for i, shard := range releaseShards {
		if seen[shard.id] {
			t.Errorf("duplicate shard id %q", shard.id)
		}
		seen[shard.id] = true

		// The id is the wire value AND the frontend's route segment, so a stray
		// separator or a missing family prefix breaks the sitemap index rather
		// than only reading oddly.
		if !strings.HasPrefix(shard.id, "releases-") {
			t.Errorf("shard id %q does not start with the family it slices", shard.id)
		}

		if i == 0 {
			continue
		}
		if prev := releaseShards[i-1]; prev.before != shard.from {
			t.Errorf("gap or overlap between %q (before %q) and %q (from %q): the ranges must be contiguous",
				prev.id, prev.before, shard.id, shard.from)
		}
		if shard.before != "" && shard.from >= shard.before {
			t.Errorf("shard %q has an empty or inverted range [%q, %q)", shard.id, shard.from, shard.before)
		}
	}
}

// TestReleaseShardByIDRejectsAFamilyName guards the resolution step: `releases`
// itself must NOT resolve to a shard, or a caller asking for the whole family
// would silently receive one range of it.
func TestReleaseShardByIDRejectsAFamilyName(t *testing.T) {
	for _, id := range []string{"releases", "artists", "", "releases-"} {
		if shard := releaseShardByID(id); shard != nil {
			t.Errorf("releaseShardByID(%q) = %q, want nil", id, shard.id)
		}
	}
	for _, shard := range releaseShards {
		if got := releaseShardByID(shard.id); got == nil || got.id != shard.id {
			t.Errorf("releaseShardByID(%q) did not resolve to its own shard", shard.id)
		}
	}
}
