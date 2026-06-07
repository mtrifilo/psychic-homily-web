package catalog

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// ListTagEntitiesHandler
// ============================================================================

func TestListTagEntities_ByID(t *testing.T) {
	mock := &testhelpers.MockTagService{
		GetTagFn: func(tagID uint) (*catalogm.Tag, error) {
			return &catalogm.Tag{ID: tagID, Name: "punk", Slug: "punk"}, nil
		},
		GetTagEntitiesFn: func(tagID uint, entityType string, limit, offset int) ([]contracts.TaggedEntityItem, int64, error) {
			if tagID != 3 {
				t.Errorf("expected tagID=3, got %d", tagID)
			}
			return []contracts.TaggedEntityItem{{EntityType: "artist", EntityID: 1, Name: "Band"}}, 1, nil
		},
	}
	h := NewTagHandler(mock, nil)
	resp, err := h.ListTagEntitiesHandler(context.Background(), &ListTagEntitiesRequest{TagID: "3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 || len(resp.Body.Entities) != 1 {
		t.Errorf("unexpected body: %+v", resp.Body)
	}
}

func TestListTagEntities_BySlug(t *testing.T) {
	mock := &testhelpers.MockTagService{
		GetTagBySlugFn: func(slug string) (*catalogm.Tag, error) {
			return &catalogm.Tag{ID: 9, Name: "post-punk", Slug: slug}, nil
		},
		GetTagEntitiesFn: func(tagID uint, _ string, _, _ int) ([]contracts.TaggedEntityItem, int64, error) {
			if tagID != 9 {
				t.Errorf("expected resolved tagID=9, got %d", tagID)
			}
			return nil, 0, nil
		},
	}
	h := NewTagHandler(mock, nil)
	resp, err := h.ListTagEntitiesHandler(context.Background(), &ListTagEntitiesRequest{TagID: "post-punk"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 0 {
		t.Errorf("expected total=0, got %d", resp.Body.Total)
	}
}

func TestListTagEntities_TagNotFound(t *testing.T) {
	// resolveTag returns nil when both ID and slug lookups miss → 404.
	mock := &testhelpers.MockTagService{
		GetTagBySlugFn: func(_ string) (*catalogm.Tag, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewTagHandler(mock, nil)
	_, err := h.ListTagEntitiesHandler(context.Background(), &ListTagEntitiesRequest{TagID: "ghost"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestListTagEntities_ServiceError(t *testing.T) {
	mock := &testhelpers.MockTagService{
		GetTagFn: func(tagID uint) (*catalogm.Tag, error) {
			return &catalogm.Tag{ID: tagID}, nil
		},
		GetTagEntitiesFn: func(_ uint, _ string, _, _ int) ([]contracts.TaggedEntityItem, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewTagHandler(mock, nil)
	_, err := h.ListTagEntitiesHandler(context.Background(), &ListTagEntitiesRequest{TagID: "3"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetTagIntersectionHandler (PSY-995)
// ============================================================================

func TestGetTagIntersection_FewerThanTwoTags(t *testing.T) {
	h := NewTagHandler(&testhelpers.MockTagService{}, nil)
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{Tags: "shoegaze"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetTagIntersection_DuplicateCollapsesBelowTwo(t *testing.T) {
	// "shoegaze,shoegaze" dedupes to one distinct slug → still < 2 → 400.
	h := NewTagHandler(&testhelpers.MockTagService{}, nil)
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{Tags: "shoegaze,shoegaze"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetTagIntersection_UnknownTag(t *testing.T) {
	mock := &testhelpers.MockTagService{
		GetTagBySlugFn: func(slug string) (*catalogm.Tag, error) {
			if slug == "shoegaze" {
				return &catalogm.Tag{ID: 1, Slug: slug}, nil
			}
			return nil, nil // ghost slug → unknown
		},
	}
	h := NewTagHandler(mock, nil)
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{Tags: "shoegaze,ghost"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetTagIntersection_PreviewLimitClampedToMax(t *testing.T) {
	var gotLimit int
	mock := &testhelpers.MockTagService{
		GetTagBySlugFn: func(slug string) (*catalogm.Tag, error) {
			return &catalogm.Tag{ID: 1, Slug: slug}, nil
		},
		IntersectEntitiesByTagsFn: func(_ []string, _ bool, previewLimit int) (*contracts.TagIntersectionResponse, error) {
			gotLimit = previewLimit
			return &contracts.TagIntersectionResponse{TagMatch: "all"}, nil
		},
	}
	h := NewTagHandler(mock, nil)
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{
		Tags:         "shoegaze,ambient",
		PreviewLimit: 999,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != intersectionPreviewLimitMax {
		t.Errorf("expected preview_limit clamped to %d, got %d", intersectionPreviewLimitMax, gotLimit)
	}
}

func TestGetTagIntersection_DefaultPreviewLimitAndMatch(t *testing.T) {
	var gotLimit int
	var gotMatchAny bool
	mock := &testhelpers.MockTagService{
		GetTagBySlugFn: func(slug string) (*catalogm.Tag, error) {
			return &catalogm.Tag{ID: 1, Slug: slug}, nil
		},
		IntersectEntitiesByTagsFn: func(_ []string, matchAny bool, previewLimit int) (*contracts.TagIntersectionResponse, error) {
			gotLimit = previewLimit
			gotMatchAny = matchAny
			return &contracts.TagIntersectionResponse{TagMatch: "all"}, nil
		},
	}
	h := NewTagHandler(mock, nil)
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{Tags: "shoegaze,ambient"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != intersectionPreviewLimitDefault {
		t.Errorf("expected default preview_limit %d, got %d", intersectionPreviewLimitDefault, gotLimit)
	}
	if gotMatchAny {
		t.Errorf("expected AND (matchAny=false) by default")
	}
}

func TestGetTagIntersection_AnyMatch(t *testing.T) {
	var gotMatchAny bool
	mock := &testhelpers.MockTagService{
		GetTagBySlugFn: func(slug string) (*catalogm.Tag, error) {
			return &catalogm.Tag{ID: 1, Slug: slug}, nil
		},
		IntersectEntitiesByTagsFn: func(_ []string, matchAny bool, _ int) (*contracts.TagIntersectionResponse, error) {
			gotMatchAny = matchAny
			return &contracts.TagIntersectionResponse{TagMatch: "any"}, nil
		},
	}
	h := NewTagHandler(mock, nil)
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{Tags: "shoegaze,ambient", TagMatch: "any"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotMatchAny {
		t.Errorf("expected matchAny=true for tag_match=any")
	}
}

func TestGetTagIntersection_ServiceError(t *testing.T) {
	mock := &testhelpers.MockTagService{
		GetTagBySlugFn: func(slug string) (*catalogm.Tag, error) {
			return &catalogm.Tag{ID: 1, Slug: slug}, nil
		},
		IntersectEntitiesByTagsFn: func(_ []string, _ bool, _ int) (*contracts.TagIntersectionResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewTagHandler(mock, nil)
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{Tags: "shoegaze,ambient"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetGenreHierarchyHandler
// ============================================================================

func TestGetGenreHierarchy_Success(t *testing.T) {
	parentID := uint(1)
	mock := &testhelpers.MockTagService{
		GetGenreHierarchyFn: func() ([]*catalogm.Tag, error) {
			return []*catalogm.Tag{
				{ID: 1, Name: "rock", Slug: "rock", IsOfficial: true},
				{ID: 2, Name: "punk", Slug: "punk", ParentID: &parentID, UsageCount: 12},
			}, nil
		},
	}
	h := NewTagHandler(mock, nil)
	resp, err := h.GetGenreHierarchyHandler(context.Background(), &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(resp.Body.Tags))
	}
	if resp.Body.Tags[1].ParentID == nil || *resp.Body.Tags[1].ParentID != 1 {
		t.Errorf("expected punk parent_id=1, got %+v", resp.Body.Tags[1])
	}
}

func TestGetGenreHierarchy_ServiceError(t *testing.T) {
	mock := &testhelpers.MockTagService{
		GetGenreHierarchyFn: func() ([]*catalogm.Tag, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewTagHandler(mock, nil)
	_, err := h.GetGenreHierarchyHandler(context.Background(), &struct{}{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// SetTagParentHandler
// ============================================================================

func TestSetTagParent_Success(t *testing.T) {
	newParent := uint(1)
	mock := &testhelpers.MockTagService{
		SetTagParentFn: func(tagID uint, parentID *uint, actorUserID uint) error {
			if tagID != 2 || parentID == nil || *parentID != 1 || actorUserID != 7 {
				t.Errorf("unexpected params tagID=%d parentID=%v actor=%d", tagID, parentID, actorUserID)
			}
			return nil
		},
	}
	h := NewTagHandler(mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	req := &SetTagParentRequest{TagID: "2"}
	req.Body.ParentID = &newParent

	_, err := h.SetTagParentHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetTagParent_InvalidID(t *testing.T) {
	h := NewTagHandler(&testhelpers.MockTagService{}, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	_, err := h.SetTagParentHandler(ctx, &SetTagParentRequest{TagID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestSetTagParent_CycleMapsToTagError(t *testing.T) {
	// A hierarchy-cycle TagError flows through shared.MapTagError → 422.
	mock := &testhelpers.MockTagService{
		SetTagParentFn: func(_ uint, _ *uint, _ uint) error {
			return apperrors.ErrTagHierarchyCycle("parent is a descendant")
		},
	}
	h := NewTagHandler(mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	_, err := h.SetTagParentHandler(ctx, &SetTagParentRequest{TagID: "2"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestSetTagParent_ServiceError(t *testing.T) {
	// A non-TagError falls through to a generic 500.
	mock := &testhelpers.MockTagService{
		SetTagParentFn: func(_ uint, _ *uint, _ uint) error {
			return fmt.Errorf("db error")
		},
	}
	h := NewTagHandler(mock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	_, err := h.SetTagParentHandler(ctx, &SetTagParentRequest{TagID: "2"})
	testhelpers.AssertHumaError(t, err, 500)
}
