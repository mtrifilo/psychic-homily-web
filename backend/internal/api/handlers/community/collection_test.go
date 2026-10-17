package community

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// UpdateItemHandler
// ============================================================================

func TestUpdateItem_NoAuth(t *testing.T) {
	h := NewCollectionHandler(&testhelpers.MockCollectionService{}, nil, testhelpers.AllShowsVisible())
	_, err := h.UpdateItemHandler(context.Background(), &UpdateItemHandlerRequest{Slug: "mix", ItemID: "1"})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUpdateItem_InvalidItemID(t *testing.T) {
	h := NewCollectionHandler(&testhelpers.MockCollectionService{}, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.UpdateItemHandler(ctx, &UpdateItemHandlerRequest{Slug: "mix", ItemID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateItem_Success(t *testing.T) {
	notes := "great record"
	mock := &testhelpers.MockCollectionService{
		UpdateItemFn: func(slug string, itemID, userID uint, isAdmin bool, req *contracts.UpdateCollectionItemRequest) (*contracts.CollectionItemResponse, uint, error) {
			if slug != "mix" || itemID != 5 || userID != 1 {
				t.Errorf("unexpected params slug=%q itemID=%d userID=%d", slug, itemID, userID)
			}
			return &contracts.CollectionItemResponse{ID: 5, Notes: req.Notes}, 42, nil
		},
	}
	h := NewCollectionHandler(mock, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UpdateItemHandlerRequest{Slug: "mix", ItemID: "5"}
	req.Body.Notes = &notes

	resp, err := h.UpdateItemHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 5 {
		t.Errorf("expected item ID=5, got %d", resp.Body.ID)
	}
}

func TestUpdateItem_NotFoundMapsThroughCollectionError(t *testing.T) {
	// A CollectionError flows through shared.MapCollectionError → 404.
	mock := &testhelpers.MockCollectionService{
		UpdateItemFn: func(_ string, _, _ uint, _ bool, _ *contracts.UpdateCollectionItemRequest) (*contracts.CollectionItemResponse, uint, error) {
			return nil, 0, apperrors.ErrCollectionItemNotFound(5)
		},
	}
	h := NewCollectionHandler(mock, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.UpdateItemHandler(ctx, &UpdateItemHandlerRequest{Slug: "mix", ItemID: "5"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUpdateItem_ServiceError(t *testing.T) {
	// A non-CollectionError falls through to a generic 500.
	mock := &testhelpers.MockCollectionService{
		UpdateItemFn: func(_ string, _, _ uint, _ bool, _ *contracts.UpdateCollectionItemRequest) (*contracts.CollectionItemResponse, uint, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewCollectionHandler(mock, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.UpdateItemHandler(ctx, &UpdateItemHandlerRequest{Slug: "mix", ItemID: "5"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetEntityCollectionsHandler
// ============================================================================

func TestGetEntityCollections_Success(t *testing.T) {
	mock := &testhelpers.MockCollectionService{
		GetEntityCollectionsFn: func(entityType string, entityID, viewerID uint, limit int) ([]*contracts.CollectionListResponse, error) {
			if entityType != "artist" || entityID != 42 {
				t.Errorf("unexpected params type=%q id=%d", entityType, entityID)
			}
			return []*contracts.CollectionListResponse{{Slug: "mix"}}, nil
		},
	}
	h := NewCollectionHandler(mock, nil, testhelpers.AllShowsVisible())
	resp, err := h.GetEntityCollectionsHandler(context.Background(), &GetEntityCollectionsHandlerRequest{EntityType: "artist", EntityID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Collections) != 1 {
		t.Errorf("expected 1 collection, got %d", len(resp.Body.Collections))
	}
}

func TestGetEntityCollections_InvalidEntityID(t *testing.T) {
	h := NewCollectionHandler(&testhelpers.MockCollectionService{}, nil, testhelpers.AllShowsVisible())
	_, err := h.GetEntityCollectionsHandler(context.Background(), &GetEntityCollectionsHandlerRequest{EntityType: "artist", EntityID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetEntityCollections_InvalidEntityType(t *testing.T) {
	h := NewCollectionHandler(&testhelpers.MockCollectionService{}, nil, testhelpers.AllShowsVisible())
	_, err := h.GetEntityCollectionsHandler(context.Background(), &GetEntityCollectionsHandlerRequest{EntityType: "comment", EntityID: "1"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestGetEntityCollections_ServiceError(t *testing.T) {
	mock := &testhelpers.MockCollectionService{
		GetEntityCollectionsFn: func(_ string, _, _ uint, _ int) ([]*contracts.CollectionListResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewCollectionHandler(mock, nil, testhelpers.AllShowsVisible())
	_, err := h.GetEntityCollectionsHandler(context.Background(), &GetEntityCollectionsHandlerRequest{EntityType: "artist", EntityID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetUserPublicCollectionsHandler
// ============================================================================

func TestGetUserPublicCollections_Success(t *testing.T) {
	mock := &testhelpers.MockCollectionService{
		GetUserPublicCollectionsByUsernameFn: func(username string, _ contracts.ShowViewer, limit, offset int) ([]*contracts.CollectionListResponse, int64, error) {
			if username != "johndoe" {
				t.Errorf("unexpected username=%q", username)
			}
			// Default limit is applied when limit<=0.
			if limit != 20 {
				t.Errorf("expected default limit=20, got %d", limit)
			}
			return []*contracts.CollectionListResponse{{Slug: "mix"}}, 1, nil
		},
	}
	h := NewCollectionHandler(mock, nil, testhelpers.AllShowsVisible())
	resp, err := h.GetUserPublicCollectionsHandler(context.Background(), &GetUserPublicCollectionsHandlerRequest{Username: "johndoe", Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 || len(resp.Body.Collections) != 1 {
		t.Errorf("unexpected body: %+v", resp.Body)
	}
}

func TestGetUserPublicCollections_ServiceError(t *testing.T) {
	mock := &testhelpers.MockCollectionService{
		GetUserPublicCollectionsByUsernameFn: func(_ string, _ contracts.ShowViewer, _, _ int) ([]*contracts.CollectionListResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewCollectionHandler(mock, nil, testhelpers.AllShowsVisible())
	_, err := h.GetUserPublicCollectionsHandler(context.Background(), &GetUserPublicCollectionsHandlerRequest{Username: "johndoe"})
	testhelpers.AssertHumaError(t, err, 500)
}

// The handler passes the optional session's viewer to the service, which owns
// the private-profile gate: an anonymous request is the zero viewer.
func TestGetUserPublicCollections_PassesSessionViewer(t *testing.T) {
	cases := map[string]struct {
		ctx  context.Context
		want contracts.ShowViewer
	}{
		"anonymous": {ctx: context.Background(), want: contracts.ShowViewer{}},
		"signed-in": {ctx: testhelpers.CtxWithUser(&authm.User{ID: 7}), want: contracts.ShowViewer{UserID: 7}},
		"admin":     {ctx: testhelpers.CtxWithUser(&authm.User{ID: 9, IsAdmin: true}), want: contracts.ShowViewer{UserID: 9, IsAdmin: true}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var got contracts.ShowViewer
			mock := &testhelpers.MockCollectionService{
				GetUserPublicCollectionsByUsernameFn: func(_ string, viewer contracts.ShowViewer, _, _ int) ([]*contracts.CollectionListResponse, int64, error) {
					got = viewer
					return []*contracts.CollectionListResponse{}, 0, nil
				},
			}
			h := NewCollectionHandler(mock, nil, testhelpers.AllShowsVisible())
			if _, err := h.GetUserPublicCollectionsHandler(tc.ctx, &GetUserPublicCollectionsHandlerRequest{Username: "johndoe"}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("service viewer = %+v, want %+v", got, tc.want)
			}
		})
	}
}
