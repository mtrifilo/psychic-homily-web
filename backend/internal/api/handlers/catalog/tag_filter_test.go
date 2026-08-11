package catalog

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// A browse list's `tags=` slug list is capped because every slug becomes a
// bound parameter in the tag subquery, so an uncapped list lets one
// unauthenticated GET choose its own cost — and past Postgres'
// 65,535-parameter limit, turn into a 500 rather than merely a slow query.
func TestCapBrowseTagSlugs(t *testing.T) {
	slugs := make([]string, 0, maxBrowseTagSlugs*3)
	for i := 0; i < maxBrowseTagSlugs*3; i++ {
		slugs = append(slugs, fmt.Sprintf("tag-%02d", i))
	}

	tf := capBrowseTagSlugs(parseTagFilter(strings.Join(slugs, ","), "all"))

	if len(tf.TagSlugs) != maxBrowseTagSlugs {
		t.Fatalf("expected the slug list capped at %d, got %d", maxBrowseTagSlugs, len(tf.TagSlugs))
	}
	// The kept ones are the FIRST ones, so truncation is at least predictable
	// from the caller's own ordering rather than arbitrary.
	for i, got := range tf.TagSlugs {
		if want := slugs[i]; got != want {
			t.Errorf("slug %d = %q, want %q", i, got, want)
		}
	}
}

// A list under the cap must pass through untouched — the cap is a ceiling for
// abuse, not a page size, and every real facet selection sits well below it.
func TestCapBrowseTagSlugsLeavesShortListsAlone(t *testing.T) {
	tf := capBrowseTagSlugs(parseTagFilter("post-punk,shoegaze,emo", "any"))

	if len(tf.TagSlugs) != 3 {
		t.Fatalf("expected 3 slugs, got %d (%v)", len(tf.TagSlugs), tf.TagSlugs)
	}
	if !tf.MatchAny {
		t.Error("tag_match=any must still select OR semantics")
	}
}

// parseTagFilter itself must NOT truncate. GetTagIntersectionHandler bounds the
// same input by REJECTING it, and a truncating parse would make that 400
// unreachable — a regression caught by TestGetTagIntersection_TooManyTags when
// the cap was briefly placed in the shared parse.
func TestParseTagFilterDoesNotTruncate(t *testing.T) {
	slugs := make([]string, 0, maxBrowseTagSlugs+5)
	for i := 0; i < maxBrowseTagSlugs+5; i++ {
		slugs = append(slugs, fmt.Sprintf("tag-%02d", i))
	}

	tf := parseTagFilter(strings.Join(slugs, ","), "all")

	if len(tf.TagSlugs) != len(slugs) {
		t.Fatalf("parseTagFilter must pass every slug through so callers can choose to reject: got %d, want %d", len(tf.TagSlugs), len(slugs))
	}
}

// The cap has to survive the trip through a real handler, not just the helper:
// GET /artists is where the fan-out was measured, and it is the endpoint whose
// `total` binds the same slug list a second time.
func TestListArtists_CapsTagSlugsReachingTheService(t *testing.T) {
	slugs := make([]string, 0, maxBrowseTagSlugs*2)
	for i := 0; i < maxBrowseTagSlugs*2; i++ {
		slugs = append(slugs, fmt.Sprintf("tag-%02d", i))
	}

	var got catalog.TagFilter
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(filters map[string]interface{}, _, _ int) ([]*contracts.ArtistWithShowCountResponse, int64, error) {
			tf, ok := filters["tag_filter"].(catalog.TagFilter)
			if !ok {
				t.Fatalf("expected a catalog.TagFilter in the filters map, got %T", filters["tag_filter"])
			}
			got = tf
			return nil, 0, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil, nil)

	if _, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{
		Tags: strings.Join(slugs, ","),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.TagSlugs) != maxBrowseTagSlugs {
		t.Fatalf("expected the service to receive %d slugs, got %d", maxBrowseTagSlugs, len(got.TagSlugs))
	}
}
