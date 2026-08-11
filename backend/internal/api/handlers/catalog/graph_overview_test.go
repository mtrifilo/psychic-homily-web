package catalog

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// GetGraphOverviewHandler Tests
// =============================================================================

func graphOverviewFixture() *contracts.GraphOverview {
	return &contracts.GraphOverview{
		Version:   contracts.GraphOverviewVersion,
		NodeCount: 2,
		EdgeCount: 1,
		Nodes: contracts.GraphOverviewNodes{
			ID:   []uint{1, 2},
			Name: []string{"One", "Two"},
		},
	}
}

func TestGetGraphOverview_Success(t *testing.T) {
	overview := graphOverviewFixture()
	h := NewGraphOverviewHandler(&testhelpers.MockGraphOverviewService{
		GetGraphOverviewFn: func() (*contracts.GraphOverview, string, error) {
			return overview, `"abc123"`, nil
		},
	})

	resp, err := h.GetGraphOverviewHandler(context.Background(), &GetGraphOverviewRequest{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if resp.Body != overview {
		t.Error("body is not the service's payload")
	}
	if resp.ETag != `"abc123"` {
		t.Errorf("ETag = %q, want the service's tag", resp.ETag)
	}
	if resp.CacheControl != graphOverviewCacheControl {
		t.Errorf("Cache-Control = %q, want %q", resp.CacheControl, graphOverviewCacheControl)
	}
}

func TestGetGraphOverview_MatchingETagIsAnswered304WithNoBody(t *testing.T) {
	h := NewGraphOverviewHandler(&testhelpers.MockGraphOverviewService{
		GetGraphOverviewFn: func() (*contracts.GraphOverview, string, error) {
			return graphOverviewFixture(), `"abc123"`, nil
		},
	})

	resp, err := h.GetGraphOverviewHandler(context.Background(), &GetGraphOverviewRequest{
		IfNoneMatch: `"abc123"`,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusNotModified {
		t.Errorf("status = %d, want 304", resp.Status)
	}
	if resp.Body != nil {
		t.Error("a 304 must not carry the payload")
	}
	if resp.ETag != `"abc123"` {
		t.Errorf("ETag = %q; a 304 must still carry the tag", resp.ETag)
	}
	if resp.CacheControl != graphOverviewCacheControl {
		t.Error("a 304 must still carry the caching policy")
	}
}

func TestGetGraphOverview_StaleETagGetsTheFullPayload(t *testing.T) {
	h := NewGraphOverviewHandler(&testhelpers.MockGraphOverviewService{
		GetGraphOverviewFn: func() (*contracts.GraphOverview, string, error) {
			return graphOverviewFixture(), `"new"`, nil
		},
	})

	resp, err := h.GetGraphOverviewHandler(context.Background(), &GetGraphOverviewRequest{
		IfNoneMatch: `"old"`,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusOK || resp.Body == nil {
		t.Errorf("status = %d, body nil = %v; want 200 with a body", resp.Status, resp.Body == nil)
	}
}

func TestGetGraphOverview_NotBuiltYetIs503(t *testing.T) {
	// A nil payload with a nil error is "the nightly job has not run here yet".
	// It must not become an empty map, which a client would cache as the truth.
	h := NewGraphOverviewHandler(&testhelpers.MockGraphOverviewService{
		GetGraphOverviewFn: func() (*contracts.GraphOverview, string, error) {
			return nil, "", nil
		},
	})

	_, err := h.GetGraphOverviewHandler(context.Background(), &GetGraphOverviewRequest{})

	testhelpers.AssertHumaError(t, err, http.StatusServiceUnavailable)
}

func TestGetGraphOverview_ServiceErrorIs500(t *testing.T) {
	h := NewGraphOverviewHandler(&testhelpers.MockGraphOverviewService{
		GetGraphOverviewFn: func() (*contracts.GraphOverview, string, error) {
			return nil, "", fmt.Errorf("boom")
		},
	})

	_, err := h.GetGraphOverviewHandler(context.Background(), &GetGraphOverviewRequest{})

	testhelpers.AssertHumaError(t, err, http.StatusInternalServerError)
}

// The If-None-Match parsing itself lives in handlers/shared and is tested
// there; these two cases only pin that this handler routes through it.

// =============================================================================
// GetGraphStartingPointsHandler Tests (PSY-1749)
// =============================================================================

func TestGetGraphStartingPoints_PassesThroughRankedOrder(t *testing.T) {
	ranked := []contracts.GraphStartingPoint{
		{ArtistID: 9, ArtistName: "Most Central", ArtistSlug: "most-central"},
		{ArtistID: 4, ArtistName: "Next", ArtistSlug: "next"},
	}
	h := NewGraphOverviewHandler(&testhelpers.MockGraphOverviewService{
		GetGraphStartingPointsFn: func() ([]contracts.GraphStartingPoint, error) {
			return ranked, nil
		},
	})

	resp, err := h.GetGraphStartingPointsHandler(context.Background(), &GetGraphStartingPointsRequest{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Artists) != 2 || resp.Body.Artists[0].ArtistID != 9 {
		t.Errorf("artists = %+v, want the service's list in its ranked order", resp.Body.Artists)
	}
	if resp.CacheControl != graphStartingPointsCacheControl {
		t.Errorf("Cache-Control = %q, want %q", resp.CacheControl, graphStartingPointsCacheControl)
	}
}

// The feature's failure posture, pinned: "nothing to suggest" is the ordinary
// state of a catalog before its first nightly build, and the client answers it
// with a random artist. A 503 here — the MAP's answer to the same emptiness —
// would turn that ordinary state into an error path.
func TestGetGraphStartingPoints_NothingToSuggestIsAnEmptyList(t *testing.T) {
	h := NewGraphOverviewHandler(&testhelpers.MockGraphOverviewService{
		GetGraphStartingPointsFn: func() ([]contracts.GraphStartingPoint, error) {
			return nil, nil
		},
	})

	resp, err := h.GetGraphStartingPointsHandler(context.Background(), &GetGraphStartingPointsRequest{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Artists == nil {
		t.Error("artists is nil; it must marshal as [] rather than null")
	}
	if len(resp.Body.Artists) != 0 {
		t.Errorf("artists = %+v, want empty", resp.Body.Artists)
	}
}

func TestGetGraphStartingPoints_ServiceErrorIs500(t *testing.T) {
	h := NewGraphOverviewHandler(&testhelpers.MockGraphOverviewService{
		GetGraphStartingPointsFn: func() ([]contracts.GraphStartingPoint, error) {
			return nil, fmt.Errorf("boom")
		},
	})

	_, err := h.GetGraphStartingPointsHandler(context.Background(), &GetGraphStartingPointsRequest{})

	testhelpers.AssertHumaError(t, err, http.StatusInternalServerError)
}
