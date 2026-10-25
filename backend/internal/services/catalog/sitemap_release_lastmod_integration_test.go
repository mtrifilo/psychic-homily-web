package catalog

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// A release's lastmod is the later of its row's updated_at and its newest
// link's created_at, with the unlinked-release diet rule off and on.
func TestSitemapReleaseLastmodFollowsItsNewestLink(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	rowAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	firstLinkAt := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	newestLinkAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	editedAfterLinkAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	seedRelease := func(slug string, updatedAt time.Time, linkTimes ...time.Time) {
		t.Helper()
		release := &catalogm.Release{Title: slug, Slug: stringPtr(slug)}
		if err := td.DB.Create(release).Error; err != nil {
			t.Fatalf("seed release %s: %v", slug, err)
		}
		if err := td.DB.Exec("UPDATE releases SET updated_at = ? WHERE id = ?", updatedAt, release.ID).Error; err != nil {
			t.Fatalf("stamp release %s: %v", slug, err)
		}
		for _, at := range linkTimes {
			link := &catalogm.ReleaseExternalLink{ReleaseID: release.ID, Platform: "bandcamp", URL: "https://example.bandcamp.com/" + slug, CreatedAt: at}
			if err := td.DB.Create(link).Error; err != nil {
				t.Fatalf("seed link for %s: %v", slug, err)
			}
		}
	}
	seedRelease("linked-after-the-row", rowAt, firstLinkAt, newestLinkAt)
	seedRelease("edited-after-its-link", editedAfterLinkAt, firstLinkAt)
	seedRelease("never-linked", rowAt)

	want := map[string]time.Time{
		"linked-after-the-row":  newestLinkAt,
		"edited-after-its-link": editedAfterLinkAt,
		"never-linked":          rowAt,
	}
	assertLastmods := func(label string, entries []contracts.SitemapEntry, expected map[string]time.Time) {
		t.Helper()
		if len(entries) != len(expected) {
			t.Fatalf("%s: got %d release entries, want %d", label, len(entries), len(expected))
		}
		for _, e := range entries {
			at, ok := expected[e.Slug]
			if !ok {
				t.Errorf("%s: unexpected release %q", label, e.Slug)
				continue
			}
			if !e.UpdatedAt.Equal(at) {
				t.Errorf("%s: %s lastmod = %v, want %v", label, e.Slug, e.UpdatedAt.UTC(), at)
			}
		}
	}

	off, err := dietService(td.DB, sitemapDiet{}).Entries(context.Background(), "releases")
	if err != nil {
		t.Fatalf("Entries(releases), diet off: %v", err)
	}
	assertLastmods("diet off", off.Releases, want)

	on, err := dietService(td.DB, sitemapDiet{omitUnlinkedReleases: true}).Entries(context.Background(), "releases")
	if err != nil {
		t.Fatalf("Entries(releases), diet on: %v", err)
	}
	linkedOnly := map[string]time.Time{
		"linked-after-the-row":  newestLinkAt,
		"edited-after-its-link": editedAfterLinkAt,
	}
	assertLastmods("diet on", on.Releases, linkedOnly)

	// A sub-shard request computes the same lastmod as the whole family.
	var sharded []contracts.SitemapEntry
	for _, shard := range sitemapShardsByFamily["releases"] {
		got, err := dietService(td.DB, sitemapDiet{}).Entries(context.Background(), shard.id)
		if err != nil {
			t.Fatalf("Entries(%s): %v", shard.id, err)
		}
		sharded = append(sharded, got.Releases...)
	}
	assertLastmods("sub-shards", sharded, want)

	// Still one statement for the family.
	var queries int
	counting := td.DB.Session(&gorm.Session{Logger: queryCounter{Interface: td.DB.Logger, n: &queries}})
	if _, err := dietService(counting, sitemapDiet{}).Entries(context.Background(), "releases"); err != nil {
		t.Fatalf("Entries(releases), counted: %v", err)
	}
	if queries != 1 {
		t.Errorf("Entries(releases) issued %d queries, want 1", queries)
	}
}
