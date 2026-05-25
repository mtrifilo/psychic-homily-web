package pipeline

import (
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

// ──────────────────────────────────────────────
// Unit tests — error paths + happy path with mock service
// ──────────────────────────────────────────────

func testStreamingWorklistHandler() *StreamingWorklistHandler {
	return NewStreamingWorklistHandler(nil, nil)
}

func streamingWorklistHandlerWith(svc contracts.StreamingWorklistServiceInterface) *StreamingWorklistHandler {
	return NewStreamingWorklistHandler(svc, nil)
}

func TestGetStreamingWorklistHandler_Success(t *testing.T) {
	h := streamingWorklistHandlerWith(&testhelpers.MockStreamingWorklistService{
		ListStreamingWorklistFn: func(status string, limit, offset int) (*contracts.StreamingWorklistResult, error) {
			if limit != 50 {
				t.Errorf("expected default limit=50, got %d", limit)
			}
			return &contracts.StreamingWorklistResult{
				Entries: []contracts.StreamingWorklistEntry{
					{ArtistID: 1, ArtistName: "Test", StreamingDiscoveryStatus: "unreviewed", SoonestEventDate: time.Now().Add(48 * time.Hour)},
				},
				Total: 1,
			}, nil
		},
	})

	resp, err := h.GetStreamingWorklistHandler(adminCtx(), &GetStreamingWorklistRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(resp.Body.Entries))
	}
}

func TestGetStreamingWorklistHandler_InvalidStatus(t *testing.T) {
	h := streamingWorklistHandlerWith(&testhelpers.MockStreamingWorklistService{
		ListStreamingWorklistFn: func(status string, limit, offset int) (*contracts.StreamingWorklistResult, error) {
			return nil, fmt.Errorf("%w: status %q is not a non-terminal worklist status", contracts.ErrInvalidStreamingStatusTransition, status)
		},
	})

	_, err := h.GetStreamingWorklistHandler(adminCtx(), &GetStreamingWorklistRequest{Status: "linked"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetStreamingWorklistHandler_ServiceError(t *testing.T) {
	h := streamingWorklistHandlerWith(&testhelpers.MockStreamingWorklistService{
		ListStreamingWorklistFn: func(_ string, _, _ int) (*contracts.StreamingWorklistResult, error) {
			return nil, fmt.Errorf("db blew up")
		},
	})

	_, err := h.GetStreamingWorklistHandler(adminCtx(), &GetStreamingWorklistRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestUpdateStreamingDiscoveryStatusHandler_InvalidArtistID(t *testing.T) {
	h := testStreamingWorklistHandler()
	req := &UpdateStreamingStatusRequest{ArtistID: "not-a-number"}
	req.Body.Status = "linked"

	_, err := h.UpdateStreamingDiscoveryStatusHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateStreamingDiscoveryStatusHandler_EmptyStatus(t *testing.T) {
	h := testStreamingWorklistHandler()
	req := &UpdateStreamingStatusRequest{ArtistID: "1"}
	// Body.Status zero-value

	_, err := h.UpdateStreamingDiscoveryStatusHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateStreamingDiscoveryStatusHandler_InvalidTransition(t *testing.T) {
	h := streamingWorklistHandlerWith(&testhelpers.MockStreamingWorklistService{
		UpdateStreamingDiscoveryStatusFn: func(_ contracts.UpdateStreamingDiscoveryStatusInput) (*contracts.StreamingDiscoveryArtistResponse, error) {
			return nil, fmt.Errorf("%w: linked → skipped is not allowed", contracts.ErrInvalidStreamingStatusTransition)
		},
	})

	req := &UpdateStreamingStatusRequest{ArtistID: "42"}
	req.Body.Status = "skipped"

	_, err := h.UpdateStreamingDiscoveryStatusHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateStreamingDiscoveryStatusHandler_NotFound(t *testing.T) {
	h := streamingWorklistHandlerWith(&testhelpers.MockStreamingWorklistService{
		UpdateStreamingDiscoveryStatusFn: func(_ contracts.UpdateStreamingDiscoveryStatusInput) (*contracts.StreamingDiscoveryArtistResponse, error) {
			return nil, contracts.ErrStreamingArtistNotFound
		},
	})

	req := &UpdateStreamingStatusRequest{ArtistID: "9999"}
	req.Body.Status = "linked"

	_, err := h.UpdateStreamingDiscoveryStatusHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUpdateStreamingDiscoveryStatusHandler_Success(t *testing.T) {
	var captured contracts.UpdateStreamingDiscoveryStatusInput
	h := streamingWorklistHandlerWith(&testhelpers.MockStreamingWorklistService{
		UpdateStreamingDiscoveryStatusFn: func(input contracts.UpdateStreamingDiscoveryStatusInput) (*contracts.StreamingDiscoveryArtistResponse, error) {
			captured = input
			return &contracts.StreamingDiscoveryArtistResponse{
				ID:                       input.ArtistID,
				Name:                     "Mock Artist",
				StreamingDiscoveryStatus: input.Status,
				StreamingDiscoveryReason: input.Reason,
				UpdatedAt:                time.Now(),
			}, nil
		},
	})

	reason := "AI search returned no matches"
	req := &UpdateStreamingStatusRequest{ArtistID: "7"}
	req.Body.Status = "no_links_found"
	req.Body.Reason = &reason

	resp, err := h.UpdateStreamingDiscoveryStatusHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.ArtistID != 7 {
		t.Errorf("expected captured artistID=7, got %d", captured.ArtistID)
	}
	if captured.Status != "no_links_found" {
		t.Errorf("expected captured status=no_links_found, got %q", captured.Status)
	}
	if captured.Reason == nil || *captured.Reason != reason {
		t.Errorf("expected captured reason=%q, got %v", reason, captured.Reason)
	}
	if resp.Body.StreamingDiscoveryStatus != "no_links_found" {
		t.Errorf("expected response status=no_links_found, got %q", resp.Body.StreamingDiscoveryStatus)
	}
}
