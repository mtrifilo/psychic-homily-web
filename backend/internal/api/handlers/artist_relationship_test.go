package handlers

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

func testArtistRelationshipHandler() *ArtistRelationshipHandler {
	return NewArtistRelationshipHandler(nil, nil)
}

// --- GetArtistGraphHandler ---

func TestGetArtistGraph_InvalidID(t *testing.T) {
	h := testArtistRelationshipHandler()
	req := &GetArtistGraphRequest{ArtistID: "abc"}

	_, err := h.GetArtistGraphHandler(context.Background(), req)
	assertHumaError(t, err, 400)
}

func TestGetArtistGraph_TypesParsing(t *testing.T) {
	var capturedTypes []string
	h := NewArtistRelationshipHandler(
		&mockArtistRelationshipService{
			getArtistGraphFn: func(artistID uint, types []string, userID uint) (*contracts.ArtistGraph, error) {
				capturedTypes = types
				return &contracts.ArtistGraph{}, nil
			},
		},
		nil,
	)
	req := &GetArtistGraphRequest{ArtistID: "1", Types: "similar,shared_bills"}

	_, err := h.GetArtistGraphHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedTypes) != 2 || capturedTypes[0] != "similar" || capturedTypes[1] != "shared_bills" {
		t.Errorf("expected types [similar shared_bills], got %v", capturedTypes)
	}
}

// --- GetRelatedArtistsHandler ---

func TestGetRelatedArtists_InvalidID(t *testing.T) {
	h := testArtistRelationshipHandler()
	req := &GetRelatedArtistsRequest{ArtistID: "abc"}

	_, err := h.GetRelatedArtistsHandler(context.Background(), req)
	assertHumaError(t, err, 400)
}

// --- CreateRelationshipHandler ---

func TestCreateRelationship_NoAuth(t *testing.T) {
	h := testArtistRelationshipHandler()
	req := &CreateRelationshipRequest{}
	req.Body.SourceArtistID = 1
	req.Body.TargetArtistID = 2
	req.Body.Type = "similar"

	_, err := h.CreateRelationshipHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestCreateRelationship_MissingSourceID(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &CreateRelationshipRequest{}
	req.Body.TargetArtistID = 2
	req.Body.Type = "similar"

	_, err := h.CreateRelationshipHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestCreateRelationship_MissingType(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &CreateRelationshipRequest{}
	req.Body.SourceArtistID = 1
	req.Body.TargetArtistID = 2

	_, err := h.CreateRelationshipHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- VoteHandler ---

func TestVote_NoAuth(t *testing.T) {
	h := testArtistRelationshipHandler()
	req := &VoteRelationshipRequest{SourceID: "1", TargetID: "2"}
	req.Body.Type = "similar"
	req.Body.IsUpvote = true

	_, err := h.VoteHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestVote_InvalidSourceID(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &VoteRelationshipRequest{SourceID: "abc", TargetID: "2"}
	req.Body.Type = "similar"

	_, err := h.VoteHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestVote_InvalidTargetID(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &VoteRelationshipRequest{SourceID: "1", TargetID: "abc"}
	req.Body.Type = "similar"

	_, err := h.VoteHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestVote_MissingType(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &VoteRelationshipRequest{SourceID: "1", TargetID: "2"}

	_, err := h.VoteHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- RemoveVoteHandler ---

func TestRemoveVote_NoAuth(t *testing.T) {
	h := testArtistRelationshipHandler()
	req := &RemoveRelationshipVoteRequest{SourceID: "1", TargetID: "2", Type: "similar"}

	_, err := h.RemoveVoteHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestRemoveVote_MissingType(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &RemoveRelationshipVoteRequest{SourceID: "1", TargetID: "2"}

	_, err := h.RemoveVoteHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- DeleteRelationshipHandler ---

func TestDeleteRelationship_NoAuth(t *testing.T) {
	h := testArtistRelationshipHandler()
	req := &DeleteRelationshipRequest{SourceID: "1", TargetID: "2", Type: "similar"}

	_, err := h.DeleteRelationshipHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestDeleteRelationship_NonAdmin(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &DeleteRelationshipRequest{SourceID: "1", TargetID: "2", Type: "similar"}

	_, err := h.DeleteRelationshipHandler(ctx, req)
	assertHumaError(t, err, 403)
}

// --- splitAndTrim ---

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"similar,shared_bills", []string{"similar", "shared_bills"}},
		{"similar, shared_bills , side_project", []string{"similar", "shared_bills", "side_project"}},
		{"", []string{}},
		{"similar", []string{"similar"}},
		{" , , ", []string{}},
	}

	for _, tt := range tests {
		result := splitAndTrim(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("splitAndTrim(%q) = %v, want %v", tt.input, result, tt.expected)
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("splitAndTrim(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
			}
		}
	}
}
