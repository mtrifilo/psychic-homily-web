package catalog

import (
	"slices"
	"strings"
	"testing"
)

// TestSitemapShardsBucketTheirFamilies is the structural half of the sub-shard
// guard, with no database, so it runs in short mode too.
//
// The failure it exists to catch is invisible at runtime: a family whose shards
// do not cover every residue drops every row with the missing residue out of
// the sitemap while each shard still renders a valid document, and a residue
// served twice announces the same URL from two documents. Neither shows up in a
// byte-size measurement, which is the only other thing anyone re-checks.
//
// A partition of the non-negative integers by `id % N` is total and disjoint
// exactly when every residue in [0, N) is served once, so that is what this
// asserts, per family, together with the two fields the resolution step in
// Entries trusts: the family a shard names, and the id that names it.
func TestSitemapShardsBucketTheirFamilies(t *testing.T) {
	if len(sitemapShardsByFamily) == 0 {
		t.Fatal("sitemapShardsByFamily is empty, so no family is sub-sharded")
	}

	for family, shards := range sitemapShardsByFamily {
		if len(shards) < 2 {
			t.Errorf("family %q has %d shard(s), and sub-sharding needs at least 2", family, len(shards))
			continue
		}
		if !slices.Contains(sitemapFamilies, family) {
			t.Errorf("family %q is sub-sharded but is not a sitemap family, so the flatten drops its ids and they 422 forever", family)
		}

		served := make([]string, len(shards))
		for _, shard := range shards {
			// The field must agree with the key it is filed under: everything
			// downstream reads `shard.family` rather than the group it came
			// from, so a mismatch would bucket another family's rows.
			if shard.family != family {
				t.Errorf("shard %q is filed under %q but names family %q", shard.id, family, shard.family)
			}
			// One family, one modulus. A shard carrying a different N would
			// overlap its siblings and leave residues unserved at the same time.
			if shard.buckets != len(shards) {
				t.Errorf("shard %q declares %d buckets while family %q holds %d shards",
					shard.id, shard.buckets, family, len(shards))
				continue
			}
			if shard.bucket < 0 || shard.bucket >= shard.buckets {
				t.Errorf("shard %q has residue %d outside [0, %d)", shard.id, shard.bucket, shard.buckets)
				continue
			}
			if prev := served[shard.bucket]; prev != "" {
				t.Errorf("residue %d of family %q is served by both %q and %q, which announce the same rows",
					shard.bucket, family, prev, shard.id)
				continue
			}
			served[shard.bucket] = shard.id
			// The id is the wire value AND the frontend's route segment, so it
			// has to name the residue it actually selects: an id saying b3 over
			// a shard selecting 4 would be invisible to every other check here.
			if want := sitemapShardID(family, shard.bucket); shard.id != want {
				t.Errorf("shard selecting residue %d of %q is named %q, want %q", shard.bucket, family, shard.id, want)
			}
			if !strings.HasPrefix(shard.id, family+"-") {
				t.Errorf("shard id %q does not start with the family it buckets (%q)", shard.id, family)
			}
		}

		for residue, id := range served {
			if id == "" {
				t.Errorf("family %q serves no shard for residue %d, so every row whose id is %d modulo %d leaves the sitemap",
					family, residue, residue, len(shards))
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
// would silently receive one bucket of it.
func TestSitemapShardByIDRejectsAFamilyName(t *testing.T) {
	for _, id := range []string{"releases", "shows", "artists", "", "releases-", "shows-", "shows-b", "shows-b99"} {
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
		t.Errorf("sitemapShards holds %d ids, sitemapShardsByFamily holds %d, so a family was dropped by the flatten",
			len(got), countShards())
	}
}

func countShards() int {
	n := 0
	for _, shards := range sitemapShardsByFamily {
		n += len(shards)
	}
	return n
}

// TestSitemapShardsPerFamilyIsTheOnlyBucketCount pins the generated table
// against the table it was generated from. A family whose declared count and
// shard count disagree would serve a partition of a modulus nobody wrote down.
func TestSitemapShardsPerFamilyIsTheOnlyBucketCount(t *testing.T) {
	if len(sitemapShardsPerFamily) != len(sitemapShardsByFamily) {
		t.Errorf("sitemapShardsPerFamily declares %d families, sitemapShardsByFamily holds %d",
			len(sitemapShardsPerFamily), len(sitemapShardsByFamily))
	}
	for family, buckets := range sitemapShardsPerFamily {
		if buckets < 2 {
			t.Errorf("family %q declares %d buckets, and sub-sharding needs at least 2", family, buckets)
		}
		if got := len(sitemapShardsByFamily[family]); got != buckets {
			t.Errorf("family %q declares %d buckets but holds %d shards", family, buckets, got)
		}
	}
}
