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

func TestGraphOverviewETagMatches(t *testing.T) {
	tests := []struct {
		name        string
		ifNoneMatch string
		etag        string
		want        bool
	}{
		{"exact", `"abc"`, `"abc"`, true},
		{"empty header", "", `"abc"`, false},
		{"empty etag", `"abc"`, "", false},
		{"wildcard", "*", `"abc"`, true},
		{"list containing the tag", `"x", "abc" , "y"`, `"abc"`, true},
		{"list without the tag", `"x", "y"`, `"abc"`, false},
		// RFC 9110 §13.1.2: If-None-Match uses WEAK comparison, so a tag an
		// intermediary weakened still matches.
		{"weakened by the client", `W/"abc"`, `"abc"`, true},
		{"weak on both sides", `W/"abc"`, `W/"abc"`, true},
		{"different tag", `"abcd"`, `"abc"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := graphOverviewETagMatches(tt.ifNoneMatch, tt.etag); got != tt.want {
				t.Errorf("graphOverviewETagMatches(%q, %q) = %v, want %v", tt.ifNoneMatch, tt.etag, got, tt.want)
			}
		})
	}
}
