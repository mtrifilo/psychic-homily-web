package catalog

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
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
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetArtistGraph_TypesParsing(t *testing.T) {
	var capturedTypes []string
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			GetArtistGraphFn: func(artistID uint, types []string, userID uint) (*contracts.ArtistGraph, error) {
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

// PSY-363: handler accepts festival_cobill as a valid type filter and
// passes it through to the service unchanged.
func TestGetArtistGraph_FestivalCobillTypePassedThrough(t *testing.T) {
	var capturedTypes []string
	mockGraph := &contracts.ArtistGraph{
		Center: contracts.ArtistGraphNode{ID: 1, Name: "C"},
		Links: []contracts.ArtistGraphLink{
			{
				SourceID: 1,
				TargetID: 2,
				Type:     "festival_cobill",
				Score:    0.4,
				Detail: map[string]interface{}{
					"festival_names":   "Coachella",
					"count":            1,
					"most_recent_year": 2025,
				},
			},
		},
	}
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			GetArtistGraphFn: func(artistID uint, types []string, userID uint) (*contracts.ArtistGraph, error) {
				capturedTypes = types
				return mockGraph, nil
			},
		},
		nil,
	)
	req := &GetArtistGraphRequest{ArtistID: "1", Types: "festival_cobill"}

	resp, err := h.GetArtistGraphHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedTypes) != 1 || capturedTypes[0] != "festival_cobill" {
		t.Errorf("expected types [festival_cobill], got %v", capturedTypes)
	}
	if resp == nil || len(resp.Body.Links) != 1 {
		t.Fatalf("expected 1 link in response, got %v", resp)
	}
	if resp.Body.Links[0].Type != "festival_cobill" {
		t.Errorf("expected link type festival_cobill, got %q", resp.Body.Links[0].Type)
	}
}

// PSY-363: festival_cobill works alongside other types in a multi-value filter.
func TestGetArtistGraph_FestivalCobillCombinedWithOtherTypes(t *testing.T) {
	var capturedTypes []string
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			GetArtistGraphFn: func(artistID uint, types []string, userID uint) (*contracts.ArtistGraph, error) {
				capturedTypes = types
				return &contracts.ArtistGraph{}, nil
			},
		},
		nil,
	)
	req := &GetArtistGraphRequest{ArtistID: "1", Types: "similar,festival_cobill,shared_bills"}

	_, err := h.GetArtistGraphHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"similar", "festival_cobill", "shared_bills"}
	if len(capturedTypes) != len(want) {
		t.Fatalf("expected %v, got %v", want, capturedTypes)
	}
	for i, v := range capturedTypes {
		if v != want[i] {
			t.Errorf("at %d: expected %q, got %q", i, want[i], v)
		}
	}
}

// --- GetRelatedArtistsHandler ---

func TestGetRelatedArtists_InvalidID(t *testing.T) {
	h := testArtistRelationshipHandler()
	req := &GetRelatedArtistsRequest{ArtistID: "abc"}

	_, err := h.GetRelatedArtistsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// --- CreateRelationshipHandler ---

func TestCreateRelationship_NoAuth(t *testing.T) {
	h := testArtistRelationshipHandler()
	req := &CreateRelationshipRequest{}
	req.Body.SourceArtistID = 1
	req.Body.TargetArtistID = 2
	req.Body.Type = "similar"

	_, err := h.CreateRelationshipHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestCreateRelationship_MissingSourceID(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &CreateRelationshipRequest{}
	req.Body.TargetArtistID = 2
	req.Body.Type = "similar"

	_, err := h.CreateRelationshipHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestCreateRelationship_MissingType(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &CreateRelationshipRequest{}
	req.Body.SourceArtistID = 1
	req.Body.TargetArtistID = 2

	_, err := h.CreateRelationshipHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// --- VoteHandler ---

func TestVote_NoAuth(t *testing.T) {
	h := testArtistRelationshipHandler()
	req := &VoteRelationshipRequest{SourceID: "1", TargetID: "2"}
	req.Body.Type = "similar"
	req.Body.IsUpvote = true

	_, err := h.VoteHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestVote_InvalidSourceID(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteRelationshipRequest{SourceID: "abc", TargetID: "2"}
	req.Body.Type = "similar"

	_, err := h.VoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestVote_InvalidTargetID(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteRelationshipRequest{SourceID: "1", TargetID: "abc"}
	req.Body.Type = "similar"

	_, err := h.VoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestVote_MissingType(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteRelationshipRequest{SourceID: "1", TargetID: "2"}

	_, err := h.VoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// --- RemoveVoteHandler ---

func TestRemoveVote_NoAuth(t *testing.T) {
	h := testArtistRelationshipHandler()
	req := &RemoveRelationshipVoteRequest{SourceID: "1", TargetID: "2", Type: "similar"}

	_, err := h.RemoveVoteHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestRemoveVote_MissingType(t *testing.T) {
	h := testArtistRelationshipHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &RemoveRelationshipVoteRequest{SourceID: "1", TargetID: "2"}

	_, err := h.RemoveVoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
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

// --- GetArtistBillCompositionHandler ---

func TestGetArtistBillComposition_InvalidID(t *testing.T) {
	h := testArtistRelationshipHandler()
	_, err := h.GetArtistBillCompositionHandler(context.Background(), &GetArtistBillCompositionRequest{ArtistID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetArtistBillComposition_NegativeMonths(t *testing.T) {
	h := testArtistRelationshipHandler()
	_, err := h.GetArtistBillCompositionHandler(context.Background(), &GetArtistBillCompositionRequest{ArtistID: "1", Months: -1})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestGetArtistBillComposition_Success(t *testing.T) {
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			GetArtistBillCompositionFn: func(artistID uint, months int) (*contracts.ArtistBillComposition, error) {
				if artistID != 1 || months != 12 {
					t.Errorf("unexpected params artistID=%d months=%d", artistID, months)
				}
				bc := &contracts.ArtistBillComposition{}
				bc.Stats.TotalShows = 5
				bc.TimeFilterMonths = months
				return bc, nil
			},
		},
		nil,
	)
	resp, err := h.GetArtistBillCompositionHandler(context.Background(), &GetArtistBillCompositionRequest{ArtistID: "1", Months: 12})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Stats.TotalShows != 5 {
		t.Errorf("expected total shows=5, got %d", resp.Body.Stats.TotalShows)
	}
}

func TestGetArtistBillComposition_NotFound(t *testing.T) {
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			GetArtistBillCompositionFn: func(_ uint, _ int) (*contracts.ArtistBillComposition, error) {
				// Handler maps an "artist not found"-prefixed error to 404.
				return nil, fmt.Errorf("artist not found: 99")
			},
		},
		nil,
	)
	_, err := h.GetArtistBillCompositionHandler(context.Background(), &GetArtistBillCompositionRequest{ArtistID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetArtistBillComposition_ServiceError(t *testing.T) {
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			GetArtistBillCompositionFn: func(_ uint, _ int) (*contracts.ArtistBillComposition, error) {
				return nil, fmt.Errorf("db error")
			},
		},
		nil,
	)
	_, err := h.GetArtistBillCompositionHandler(context.Background(), &GetArtistBillCompositionRequest{ArtistID: "1"})
	testhelpers.AssertHumaError(t, err, 500)
}

// --- DeleteRelationshipHandler (admin) ---

func TestDeleteRelationship_Success(t *testing.T) {
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			DeleteRelationshipFn: func(sourceID, targetID uint, relType string) error {
				if sourceID != 1 || targetID != 2 || relType != "similar" {
					t.Errorf("unexpected params source=%d target=%d type=%q", sourceID, targetID, relType)
				}
				return nil
			},
		},
		nil,
	)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	_, err := h.DeleteRelationshipHandler(ctx, &DeleteRelationshipRequest{SourceID: "1", TargetID: "2", Type: "similar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteRelationship_ServiceError(t *testing.T) {
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			DeleteRelationshipFn: func(_, _ uint, _ string) error {
				return fmt.Errorf("db error")
			},
		},
		nil,
	)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	_, err := h.DeleteRelationshipHandler(ctx, &DeleteRelationshipRequest{SourceID: "1", TargetID: "2", Type: "similar"})
	testhelpers.AssertHumaError(t, err, 500)
}

// --- DeriveRelationshipsHandler (admin) ---

func TestDeriveRelationships_Success(t *testing.T) {
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			DeriveSharedBillsFn: func(minShows int) (int64, error) {
				if minShows != 2 {
					t.Errorf("expected minShows=2, got %d", minShows)
				}
				return 7, nil
			},
			DeriveSharedLabelsFn: func(minLabels int) (int64, error) {
				if minLabels != 1 {
					t.Errorf("expected minLabels=1, got %d", minLabels)
				}
				return 3, nil
			},
		},
		nil,
	)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	resp, err := h.DeriveRelationshipsHandler(ctx, &DeriveRelationshipsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || resp.Body.SharedBillsUpserted != 7 || resp.Body.SharedLabelsUpserted != 3 {
		t.Errorf("unexpected body: %+v", resp.Body)
	}
}

func TestDeriveRelationships_SharedBillsError(t *testing.T) {
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			DeriveSharedBillsFn: func(_ int) (int64, error) {
				return 0, fmt.Errorf("db error")
			},
		},
		nil,
	)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	_, err := h.DeriveRelationshipsHandler(ctx, &DeriveRelationshipsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestDeriveRelationships_SharedLabelsError(t *testing.T) {
	h := NewArtistRelationshipHandler(
		&testhelpers.MockArtistRelationshipService{
			DeriveSharedBillsFn: func(_ int) (int64, error) {
				return 5, nil
			},
			DeriveSharedLabelsFn: func(_ int) (int64, error) {
				return 0, fmt.Errorf("db error")
			},
		},
		nil,
	)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	_, err := h.DeriveRelationshipsHandler(ctx, &DeriveRelationshipsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}
