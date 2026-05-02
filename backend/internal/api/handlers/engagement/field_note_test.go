package engagement

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

func testFieldNoteHandler() *FieldNoteHandler {
	return NewFieldNoteHandler(nil, nil, nil)
}

func makeFieldNoteResponse(id uint, showID uint, userID uint) *contracts.CommentResponse {
	sd := contracts.FieldNoteStructuredData{
		IsVerifiedAttendee: true,
	}
	sdBytes, _ := json.Marshal(sd)
	raw := json.RawMessage(sdBytes)
	return &contracts.CommentResponse{
		ID:              id,
		EntityType:      "show",
		EntityID:        showID,
		Kind:            "field_note",
		UserID:          userID,
		Depth:           0,
		Body:            "Great show!",
		BodyHTML:        "<p>Great show!</p>",
		StructuredData:  &raw,
		Visibility:      "visible",
		ReplyPermission: "anyone",
		Ups:             0,
		Downs:           0,
		Score:           0,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

// ============================================================================
// Tests: CreateFieldNote
// ============================================================================

func TestCreateFieldNote_NoAuth(t *testing.T) {
	h := testFieldNoteHandler()
	_, err := h.CreateFieldNoteHandler(context.Background(), &CreateFieldNoteRequest{
		ShowID: "1",
	})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestCreateFieldNote_InvalidShowID(t *testing.T) {
	h := testFieldNoteHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})
	_, err := h.CreateFieldNoteHandler(ctx, &CreateFieldNoteRequest{
		ShowID: "abc",
	})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateFieldNote_EmptyBody(t *testing.T) {
	h := testFieldNoteHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})
	req := &CreateFieldNoteRequest{ShowID: "1"}
	req.Body.Body = "   "
	_, err := h.CreateFieldNoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateFieldNote_ShowNotFound(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		CreateFieldNoteFn: func(userID uint, req *contracts.CreateFieldNoteRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("show not found")
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})
	req := &CreateFieldNoteRequest{ShowID: "999"}
	req.Body.Body = "test note"
	_, err := h.CreateFieldNoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestCreateFieldNote_FutureShow(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		CreateFieldNoteFn: func(userID uint, req *contracts.CreateFieldNoteRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("field notes can only be added to past shows")
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})
	req := &CreateFieldNoteRequest{ShowID: "1"}
	req.Body.Body = "test note"
	_, err := h.CreateFieldNoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateFieldNote_SoundQualityInvalid(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		CreateFieldNoteFn: func(userID uint, req *contracts.CreateFieldNoteRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("sound_quality must be between 1 and 5")
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})
	req := &CreateFieldNoteRequest{ShowID: "1"}
	req.Body.Body = "test note"
	sq := 0
	req.Body.SoundQuality = &sq
	_, err := h.CreateFieldNoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateFieldNote_CrowdEnergyInvalid(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		CreateFieldNoteFn: func(userID uint, req *contracts.CreateFieldNoteRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("crowd_energy must be between 1 and 5")
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})
	req := &CreateFieldNoteRequest{ShowID: "1"}
	req.Body.Body = "test note"
	ce := 7
	req.Body.CrowdEnergy = &ce
	_, err := h.CreateFieldNoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateFieldNote_ArtistNotOnShow(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		CreateFieldNoteFn: func(userID uint, req *contracts.CreateFieldNoteRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("artist is not on this show's bill")
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})
	req := &CreateFieldNoteRequest{ShowID: "1"}
	req.Body.Body = "test note"
	aid := uint(99)
	req.Body.ShowArtistID = &aid
	_, err := h.CreateFieldNoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateFieldNote_RateLimited(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		CreateFieldNoteFn: func(userID uint, req *contracts.CreateFieldNoteRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("please wait 60 seconds between comments on the same entity")
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})
	req := &CreateFieldNoteRequest{ShowID: "1"}
	req.Body.Body = "test note"
	_, err := h.CreateFieldNoteHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 429)
}

func TestCreateFieldNote_Success(t *testing.T) {
	expected := makeFieldNoteResponse(1, 42, 10)
	mock := &testhelpers.MockFieldNoteService{
		CreateFieldNoteFn: func(userID uint, req *contracts.CreateFieldNoteRequest) (*contracts.CommentResponse, error) {
			if userID != 10 {
				t.Errorf("expected userID=10, got %d", userID)
			}
			if req.ShowID != 42 {
				t.Errorf("expected showID=42, got %d", req.ShowID)
			}
			if req.Body != "Great show!" {
				t.Errorf("expected body='Great show!', got '%s'", req.Body)
			}
			return expected, nil
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})
	req := &CreateFieldNoteRequest{ShowID: "42"}
	req.Body.Body = "Great show!"
	resp, err := h.CreateFieldNoteHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected ID=1, got %d", resp.Body.ID)
	}
	if resp.Body.Kind != "field_note" {
		t.Errorf("expected kind=field_note, got %s", resp.Body.Kind)
	}
}

func TestCreateFieldNote_PassesAllFields(t *testing.T) {
	sq := 4
	ce := 5
	sp := 2
	nm := "Epic solo"
	aid := uint(7)
	mock := &testhelpers.MockFieldNoteService{
		CreateFieldNoteFn: func(userID uint, req *contracts.CreateFieldNoteRequest) (*contracts.CommentResponse, error) {
			if req.ShowArtistID == nil || *req.ShowArtistID != 7 {
				t.Errorf("expected show_artist_id=7")
			}
			if req.SoundQuality == nil || *req.SoundQuality != 4 {
				t.Errorf("expected sound_quality=4")
			}
			if req.CrowdEnergy == nil || *req.CrowdEnergy != 5 {
				t.Errorf("expected crowd_energy=5")
			}
			if req.SongPosition == nil || *req.SongPosition != 2 {
				t.Errorf("expected song_position=2")
			}
			if req.NotableMoments == nil || *req.NotableMoments != "Epic solo" {
				t.Errorf("expected notable_moments='Epic solo'")
			}
			if !req.SetlistSpoiler {
				t.Errorf("expected setlist_spoiler=true")
			}
			return makeFieldNoteResponse(1, 42, 10), nil
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})
	req := &CreateFieldNoteRequest{ShowID: "42"}
	req.Body.Body = "note"
	req.Body.ShowArtistID = &aid
	req.Body.SoundQuality = &sq
	req.Body.CrowdEnergy = &ce
	req.Body.SongPosition = &sp
	req.Body.NotableMoments = &nm
	req.Body.SetlistSpoiler = true
	_, err := h.CreateFieldNoteHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================================
// Tests: ListFieldNotes
// ============================================================================

func TestListFieldNotes_InvalidShowID(t *testing.T) {
	h := testFieldNoteHandler()
	_, err := h.ListFieldNotesHandler(context.Background(), &ListFieldNotesRequest{
		ShowID: "abc",
	})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestListFieldNotes_DefaultPagination(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		ListFieldNotesForShowFn: func(showID uint, limit, offset int) (*contracts.CommentListResponse, error) {
			if showID != 42 {
				t.Errorf("expected showID=42, got %d", showID)
			}
			if limit != 25 {
				t.Errorf("expected default limit=25, got %d", limit)
			}
			if offset != 0 {
				t.Errorf("expected default offset=0, got %d", offset)
			}
			return &contracts.CommentListResponse{
				Comments: []*contracts.CommentResponse{},
				Total:    0,
				HasMore:  false,
			}, nil
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	resp, err := h.ListFieldNotesHandler(context.Background(), &ListFieldNotesRequest{
		ShowID: "42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 0 {
		t.Errorf("expected total=0")
	}
}

func TestListFieldNotes_LimitCapped(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		ListFieldNotesForShowFn: func(showID uint, limit, offset int) (*contracts.CommentListResponse, error) {
			if limit != 100 {
				t.Errorf("expected limit capped at 100, got %d", limit)
			}
			return &contracts.CommentListResponse{
				Comments: []*contracts.CommentResponse{},
				Total:    0,
				HasMore:  false,
			}, nil
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	_, err := h.ListFieldNotesHandler(context.Background(), &ListFieldNotesRequest{
		ShowID: "42",
		Limit:  500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListFieldNotes_Success(t *testing.T) {
	notes := []*contracts.CommentResponse{makeFieldNoteResponse(1, 42, 10), makeFieldNoteResponse(2, 42, 11)}
	mock := &testhelpers.MockFieldNoteService{
		ListFieldNotesForShowFn: func(showID uint, limit, offset int) (*contracts.CommentListResponse, error) {
			return &contracts.CommentListResponse{
				Comments: notes,
				Total:    2,
				HasMore:  false,
			}, nil
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	resp, err := h.ListFieldNotesHandler(context.Background(), &ListFieldNotesRequest{
		ShowID: "42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Body.Total)
	}
	if len(resp.Body.Comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(resp.Body.Comments))
	}
}

func TestListFieldNotes_ServerError(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		ListFieldNotesForShowFn: func(showID uint, limit, offset int) (*contracts.CommentListResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	_, err := h.ListFieldNotesHandler(context.Background(), &ListFieldNotesRequest{
		ShowID: "42",
	})
	testhelpers.AssertHumaError(t, err, 500)
}
