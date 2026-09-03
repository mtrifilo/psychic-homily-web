package catalog

import (
	"context"
	"fmt"
	"strings"
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
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
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
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
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
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
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
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	_, err := h.ListTagEntitiesHandler(context.Background(), &ListTagEntitiesRequest{TagID: "3"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetTagIntersectionHandler (PSY-995)
// ============================================================================

func TestGetTagIntersection_NoTags(t *testing.T) {
	// Empty/whitespace tags param resolves to zero distinct slugs → below the
	// minimum of 1 → 400 before the service is touched.
	h := NewTagHandler(&testhelpers.MockTagService{}, nil, testhelpers.AllShowsVisible())
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{Tags: " , "})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetTagIntersection_SingleTagAllowed(t *testing.T) {
	// PSY-993: a single tag is a valid degenerate intersection (the tag's own
	// matches). The rebuilt /tags/{slug} detail page renders its grouped
	// sections from this endpoint with one tag, so single-tag must pass through
	// to the service (not 400). "shoegaze,shoegaze" dedupes to one distinct slug
	// — still valid.
	var gotSlugs []string
	mock := &testhelpers.MockTagService{
		IntersectEntitiesByTagsFn: func(slugs []string, _ bool, _ int) (*contracts.TagIntersectionResponse, error) {
			gotSlugs = slugs
			return &contracts.TagIntersectionResponse{TagMatch: "all"}, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{Tags: "shoegaze,shoegaze"})
	if err != nil {
		t.Fatalf("unexpected error for single-tag intersection: %v", err)
	}
	if len(gotSlugs) != 1 || gotSlugs[0] != "shoegaze" {
		t.Errorf("expected single resolved slug [shoegaze], got %v", gotSlugs)
	}
}

func TestGetTagIntersection_TooManyTags(t *testing.T) {
	// More than intersectionMaxTags (10) distinct slugs → 400 before the service
	// is touched, bounding fan-out on this public endpoint.
	h := NewTagHandler(&testhelpers.MockTagService{}, nil, testhelpers.AllShowsVisible())
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{
		Tags: "t1,t2,t3,t4,t5,t6,t7,t8,t9,t10,t11",
	})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetTagIntersection_UnknownTag(t *testing.T) {
	// The service resolves + validates slugs in one batched query and returns a
	// typed UnknownTagSlugError for a ghost slug; the handler maps it to 400.
	mock := &testhelpers.MockTagService{
		IntersectEntitiesByTagsFn: func(_ []string, _ bool, _ int) (*contracts.TagIntersectionResponse, error) {
			return nil, &contracts.UnknownTagSlugError{Slug: "ghost"}
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{Tags: "shoegaze,ghost"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetTagIntersection_PreviewLimitClampedToMax(t *testing.T) {
	var gotLimit int
	mock := &testhelpers.MockTagService{
		IntersectEntitiesByTagsFn: func(_ []string, _ bool, previewLimit int) (*contracts.TagIntersectionResponse, error) {
			gotLimit = previewLimit
			return &contracts.TagIntersectionResponse{TagMatch: "all"}, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{
		Tags:         "shoegaze,ambient",
		PreviewLimit: 999,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != contracts.MaxIntersectionPreviewLimit {
		t.Errorf("expected preview_limit clamped to %d, got %d", contracts.MaxIntersectionPreviewLimit, gotLimit)
	}
}

func TestGetTagIntersection_DefaultPreviewLimitAndMatch(t *testing.T) {
	var gotLimit int
	var gotMatchAny bool
	mock := &testhelpers.MockTagService{
		IntersectEntitiesByTagsFn: func(_ []string, matchAny bool, previewLimit int) (*contracts.TagIntersectionResponse, error) {
			gotLimit = previewLimit
			gotMatchAny = matchAny
			return &contracts.TagIntersectionResponse{TagMatch: "all"}, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	_, err := h.GetTagIntersectionHandler(context.Background(), &GetTagIntersectionRequest{Tags: "shoegaze,ambient"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != contracts.DefaultIntersectionPreviewLimit {
		t.Errorf("expected default preview_limit %d, got %d", contracts.DefaultIntersectionPreviewLimit, gotLimit)
	}
	if gotMatchAny {
		t.Errorf("expected AND (matchAny=false) by default")
	}
}

func TestGetTagIntersection_AnyMatch(t *testing.T) {
	var gotMatchAny bool
	mock := &testhelpers.MockTagService{
		IntersectEntitiesByTagsFn: func(_ []string, matchAny bool, _ int) (*contracts.TagIntersectionResponse, error) {
			gotMatchAny = matchAny
			return &contracts.TagIntersectionResponse{TagMatch: "any"}, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
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
		IntersectEntitiesByTagsFn: func(_ []string, _ bool, _ int) (*contracts.TagIntersectionResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
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
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
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
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
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
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	req := &SetTagParentRequest{TagID: "2"}
	req.Body.ParentID = &newParent

	_, err := h.SetTagParentHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetTagParent_InvalidID(t *testing.T) {
	h := NewTagHandler(&testhelpers.MockTagService{}, nil, testhelpers.AllShowsVisible())
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
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
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
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	_, err := h.SetTagParentHandler(ctx, &SetTagParentRequest{TagID: "2"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// The per-entity tag WRITE gate
// ============================================================================

// A caller who may not see the entity may not write tags onto it.
//
// The harm the gate removes is not only an oracle: a tag added to a private
// collection appears on its owner's page, counts toward the per-collection cap,
// and is attributed to whoever added it. All four write registrations on this
// path family are covered, because a write route left out of a sweep is how a
// family gets missed.
//
// deniesEverything stands in for a gate that refuses; the service mocks below
// fail the test if they are reached at all, which is the assertion that matters:
// the refusal happens BEFORE the write.
func TestTagWriteRoutes_RefuseAnEntityTheCallerCannotSee(t *testing.T) {
	deniesEverything := &testhelpers.MockShowVisibility{
		ShowVisibleToFn:       func(uint, contracts.ShowViewer) bool { return false },
		CollectionVisibleToFn: func(uint, contracts.ShowViewer) bool { return false },
	}
	reached := func(t *testing.T) func() {
		return func() { t.Error("the tag service was reached for an entity the caller cannot see") }
	}

	t.Run("add", func(t *testing.T) {
		mock := &testhelpers.MockTagService{
			AddTagToEntityFn: func(uint, string, string, uint, uint, string) (*catalogm.EntityTag, error) {
				reached(t)()
				return nil, nil
			},
		}
		h := NewTagHandler(mock, nil, deniesEverything)
		ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
		req := &AddTagToEntityRequest{EntityType: "collection", EntityID: "7"}
		req.Body.TagID = 3
		_, err := h.AddTagToEntityHandler(ctx, req)
		testhelpers.AssertHumaError(t, err, 404)
	})

	t.Run("remove", func(t *testing.T) {
		mock := &testhelpers.MockTagService{
			RemoveTagFromEntityFn: func(uint, string, uint) error {
				reached(t)()
				return nil
			},
		}
		h := NewTagHandler(mock, nil, deniesEverything)
		ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
		_, err := h.RemoveTagFromEntityHandler(ctx, &RemoveTagFromEntityRequest{
			EntityType: "collection", EntityID: "7", TagID: "3",
		})
		testhelpers.AssertHumaError(t, err, 404)
	})

	t.Run("vote", func(t *testing.T) {
		mock := &testhelpers.MockTagService{
			VoteOnTagFn: func(uint, string, uint, uint, bool) error {
				reached(t)()
				return nil
			},
		}
		h := NewTagHandler(mock, nil, deniesEverything)
		ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
		req := &VoteTagRequest{EntityType: "collection", EntityID: "7", TagID: "3"}
		req.Body.IsUpvote = true
		_, err := h.VoteTagHandler(ctx, req)
		testhelpers.AssertHumaError(t, err, 404)
	})

	t.Run("remove vote", func(t *testing.T) {
		mock := &testhelpers.MockTagService{
			RemoveTagVoteFn: func(uint, string, uint, uint) error {
				reached(t)()
				return nil
			},
		}
		h := NewTagHandler(mock, nil, deniesEverything)
		ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
		_, err := h.RemoveTagVoteHandler(ctx, &RemoveTagVoteRequest{
			EntityType: "collection", EntityID: "7", TagID: "3",
		})
		testhelpers.AssertHumaError(t, err, 404)
	})
}

// AN UNREGISTERED ENTITY TYPE IS REFUSED TOO, on the same terms, because the
// gate's registry is what decides which types have a rule and a type nobody
// dispositioned is not visible. The permissive gate here is what makes this
// about the registry rather than about the checker.
func TestTagWriteRoutes_RefuseAnUnregisteredEntityType(t *testing.T) {
	mock := &testhelpers.MockTagService{
		AddTagToEntityFn: func(uint, string, string, uint, uint, string) (*catalogm.EntityTag, error) {
			t.Error("the tag service was reached for an unregistered entity type")
			return nil, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &AddTagToEntityRequest{EntityType: "radio_show", EntityID: "7"}
	req.Body.TagName = "punk"
	_, err := h.AddTagToEntityHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

// The control. A gate that simply broke these routes would pass every assertion
// above, so a visible entity must still be writable.
func TestTagWriteRoutes_AllowAVisibleEntity(t *testing.T) {
	called := 0
	mock := &testhelpers.MockTagService{
		AddTagToEntityFn: func(uint, string, string, uint, uint, string) (*catalogm.EntityTag, error) {
			called++
			return &catalogm.EntityTag{}, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &AddTagToEntityRequest{EntityType: "collection", EntityID: "7"}
	req.Body.TagID = 3
	if _, err := h.AddTagToEntityHandler(ctx, req); err != nil {
		t.Fatalf("a visible collection refused a tag write: %v", err)
	}
	if called != 1 {
		t.Errorf("the tag service was called %d times, want 1", called)
	}
}

// THE SHOW HALF of the same gate, and the refusal's SHAPE.
//
// The four write routes above are exercised against a collection, which proves
// the gate is called but not that it answers the SHOW rule: an entity type the
// checker ignored would pass every one of those subtests.
//
// The acceptance criterion is the last assertion. A write against a gated show
// has to be indistinguishable from a write against a show that never existed, or
// the write route is an existence oracle over a dense id space, which is what
// the tag family was.
func TestTagWriteRoutes_RefuseAGatedShowExactlyLikeAMissingOne(t *testing.T) {
	const submitterID = uint(2)
	const gatedShowID = uint(7)
	const neverUsedShowID = uint(99999999)

	// The real show rule, plus the one fact that outranks it: NO caller sees a
	// show that is not there, an admin included. Without that arm the never-used
	// id would be granted to the admin and the pair below would not be a pair.
	showRule := &testhelpers.MockShowVisibility{
		ShowVisibleToFn: func(showID uint, viewer contracts.ShowViewer) bool {
			if showID == neverUsedShowID {
				return false
			}
			return viewer.IsAdmin || viewer.UserID == submitterID
		},
		CollectionVisibleToFn: func(uint, contracts.ShowViewer) bool { return false },
	}

	addTag := func(user *authm.User, showID uint) (int, error) {
		reached := 0
		mock := &testhelpers.MockTagService{
			AddTagToEntityFn: func(uint, string, string, uint, uint, string) (*catalogm.EntityTag, error) {
				reached++
				return &catalogm.EntityTag{}, nil
			},
		}
		h := NewTagHandler(mock, nil, showRule)
		req := &AddTagToEntityRequest{EntityType: "show", EntityID: fmt.Sprint(showID)}
		req.Body.TagID = 3
		_, err := h.AddTagToEntityHandler(testhelpers.CtxWithUser(user), req)
		return reached, err
	}

	stranger := &authm.User{ID: 3}
	admin := &authm.User{ID: 6, IsAdmin: true}

	for _, tier := range []struct {
		name   string
		user   *authm.User
		showID uint
		want   int
	}{
		{"an authenticated stranger", stranger, gatedShowID, 404},
		{"the show's submitter", &authm.User{ID: submitterID}, gatedShowID, 0},
		{"an admin", admin, gatedShowID, 0},
		{"an admin, on a show id nobody has used", admin, neverUsedShowID, 404},
	} {
		t.Run(tier.name, func(t *testing.T) {
			reached, err := addTag(tier.user, tier.showID)
			if tier.want == 0 {
				if err != nil {
					t.Fatalf("a caller entitled to the show was refused: %v", err)
				}
				if reached != 1 {
					t.Errorf("the tag service ran %d times for a granted caller, want 1", reached)
				}
				return
			}
			testhelpers.AssertHumaError(t, err, tier.want)
			if reached != 0 {
				t.Errorf("the tag service ran %d times for a refused caller, want 0", reached)
			}
		})
	}

	// The pair that has to be one answer.
	//
	// The detail ECHOES the id the caller sent, which is not a disclosure: they
	// chose it. The normalizer substitutes each request's own id back out so what
	// is compared is everything else; comparing raw would pass a refusal that named
	// the show's TITLE for the gated id, which is the shape this guards against.
	_, gatedErr := addTag(stranger, gatedShowID)
	_, missingErr := addTag(stranger, neverUsedShowID)
	testhelpers.AssertSameRefusal(t, gatedErr, missingErr, func(detail string) string {
		detail = strings.ReplaceAll(detail, fmt.Sprint(gatedShowID), "{id}")
		return strings.ReplaceAll(detail, fmt.Sprint(neverUsedShowID), "{id}")
	})
}
