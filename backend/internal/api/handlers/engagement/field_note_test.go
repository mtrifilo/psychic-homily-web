package engagement

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
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
		SetlistSpoiler: true,
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
			return nil, apperrors.ErrFieldNoteShowNotFound()
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
			return nil, apperrors.ErrFieldNoteShowFuture()
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
			return nil, apperrors.ErrCommentFieldValidation("sound_quality must be between 1 and 5")
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
			return nil, apperrors.ErrCommentFieldValidation("crowd_energy must be between 1 and 5")
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
			return nil, apperrors.ErrFieldNoteArtistNotOnBill()
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
			return nil, apperrors.ErrCommentRateLimitedEntity()
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

// ============================================================================
// Tests: ListVenueFieldNotes (PSY-1590 venue rollup)
// ============================================================================

func makeVenueFieldNote(id uint, showID uint, title string) *contracts.VenueFieldNote {
	return &contracts.VenueFieldNote{
		CommentResponse: *makeFieldNoteResponse(id, showID, 10),
		ShowTitle:       title,
		ShowSlug:        "a-show-slug",
		ShowDate:        time.Date(2024, 6, 14, 3, 0, 0, 0, time.UTC),
	}
}

// The venue rollup is numeric-id only by design (see ListVenueFieldNotesRequest).
func TestListVenueFieldNotes_InvalidVenueID(t *testing.T) {
	h := testFieldNoteHandler()
	_, err := h.ListVenueFieldNotesHandler(context.Background(), &ListVenueFieldNotesRequest{
		VenueID: "the-rebel-lounge",
	})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestListVenueFieldNotes_Success(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		ListFieldNotesForVenueFn: func(venueID uint, limit, offset int) (*contracts.VenueFieldNoteListResponse, error) {
			if venueID != 42 {
				t.Errorf("expected venueID=42, got %d", venueID)
			}
			return &contracts.VenueFieldNoteListResponse{
				Notes:   []*contracts.VenueFieldNote{makeVenueFieldNote(1, 7, "Doom Night")},
				Total:   3,
				HasMore: true,
			}, nil
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	resp, err := h.ListVenueFieldNotesHandler(context.Background(), &ListVenueFieldNotesRequest{
		VenueID: "42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Total spans the whole venue, not the page — the teaser states it beside a
	// single quoted note.
	if resp.Body.Total != 3 {
		t.Errorf("expected total=3, got %d", resp.Body.Total)
	}
	if len(resp.Body.Notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(resp.Body.Notes))
	}
	// The show identity has to survive the handler: without it the teaser
	// cannot say which night the note is about.
	if resp.Body.Notes[0].ShowTitle != "Doom Night" {
		t.Errorf("expected show title to survive, got %q", resp.Body.Notes[0].ShowTitle)
	}
	if resp.Body.Notes[0].ShowDate.IsZero() {
		t.Error("expected show date to survive the handler")
	}
}

func TestListVenueFieldNotes_DefaultAndCappedLimit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		limit    int
		expected int
	}{
		{"unset defaults to 25", 0, 25},
		{"negative defaults to 25", -5, 25},
		{"oversized caps at 100", 500, 100},
		{"in-range passes through", 3, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &testhelpers.MockFieldNoteService{
				ListFieldNotesForVenueFn: func(venueID uint, limit, offset int) (*contracts.VenueFieldNoteListResponse, error) {
					if limit != tc.expected {
						t.Errorf("expected limit=%d, got %d", tc.expected, limit)
					}
					return &contracts.VenueFieldNoteListResponse{
						Notes: []*contracts.VenueFieldNote{},
					}, nil
				},
			}
			h := NewFieldNoteHandler(mock, mock, nil)
			if _, err := h.ListVenueFieldNotesHandler(context.Background(), &ListVenueFieldNotesRequest{
				VenueID: "42",
				Limit:   tc.limit,
			}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// A venue with no notes is the COMMON case, not an error — the panel renders no
// section at all rather than an empty box.
func TestListVenueFieldNotes_EmptyIsNotAnError(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		ListFieldNotesForVenueFn: func(venueID uint, limit, offset int) (*contracts.VenueFieldNoteListResponse, error) {
			return &contracts.VenueFieldNoteListResponse{
				Notes: []*contracts.VenueFieldNote{},
			}, nil
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	resp, err := h.ListVenueFieldNotesHandler(context.Background(), &ListVenueFieldNotesRequest{
		VenueID: "42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 0 || len(resp.Body.Notes) != 0 {
		t.Errorf("expected an empty rollup, got total=%d notes=%d", resp.Body.Total, len(resp.Body.Notes))
	}
}

func TestListVenueFieldNotes_ServerError(t *testing.T) {
	mock := &testhelpers.MockFieldNoteService{
		ListFieldNotesForVenueFn: func(venueID uint, limit, offset int) (*contracts.VenueFieldNoteListResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := NewFieldNoteHandler(mock, mock, nil)
	_, err := h.ListVenueFieldNotesHandler(context.Background(), &ListVenueFieldNotesRequest{
		VenueID: "42",
	})
	testhelpers.AssertHumaError(t, err, 500)
}
