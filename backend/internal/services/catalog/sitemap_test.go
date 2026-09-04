package catalog

import (
	"slices"
	"testing"
)

// TestSitemapShardsPerFamilyIsWellFormed checks the two things the shard table
// is not built from, and so can get wrong.
//
// Everything else about a bucket, its id, its residue, its modulus and its
// family, is set by bucketShards from these same two values, so asserting it
// would be asserting the generator against itself. What the generator cannot
// make true is that the family it is asked to shard is a real one, since a key
// outside sitemapFamilies is dropped by flattenSitemapShards and its ids then
// 422 forever, and that a family is cut into more than one bucket.
func TestSitemapShardsPerFamilyIsWellFormed(t *testing.T) {
	if len(sitemapShardsPerFamily) == 0 {
		t.Fatal("sitemapShardsPerFamily is empty, so no family is sub-sharded")
	}
	for family, buckets := range sitemapShardsPerFamily {
		if !slices.Contains(sitemapFamilies, family) {
			t.Errorf("family %q is sub-sharded but is not a sitemap family, so the flatten drops its ids and they 422 forever", family)
		}
		if buckets < 2 {
			t.Errorf("family %q declares %d buckets, and sub-sharding needs at least 2", family, buckets)
		}
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
