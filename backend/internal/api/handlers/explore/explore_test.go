package explore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// Upcoming Shows
// =============================================================================

func TestExploreHandler_GetUpcomingShows_DelegatesToService(t *testing.T) {
	want := &contracts.ExploreUpcomingShowsResponse{
		Shows: []contracts.ExploreUpcomingShowItem{
			{ID: 1, Title: "Show A", HeadlinerName: "Artist A", EventDate: time.Now()},
		},
		Total:  1,
		Limit:  20,
		Offset: 0,
	}
	mock := &testhelpers.MockExploreService{
		GetUpcomingShowsFn: func(limit, offset int) (*contracts.ExploreUpcomingShowsResponse, error) {
			assert.Equal(t, 20, limit)
			assert.Equal(t, 0, offset)
			return want, nil
		},
	}
	handler := NewExploreHandler(mock)

	resp, err := handler.GetUpcomingShowsHandler(context.Background(), &GetUpcomingShowsRequest{Limit: 20, Offset: 0})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, *want, resp.Body)
}

func TestExploreHandler_GetUpcomingShows_PropagatesLimitOffset(t *testing.T) {
	mock := &testhelpers.MockExploreService{
		GetUpcomingShowsFn: func(limit, offset int) (*contracts.ExploreUpcomingShowsResponse, error) {
			assert.Equal(t, 5, limit)
			assert.Equal(t, 10, offset)
			return &contracts.ExploreUpcomingShowsResponse{Limit: limit, Offset: offset}, nil
		},
	}
	handler := NewExploreHandler(mock)

	_, err := handler.GetUpcomingShowsHandler(context.Background(), &GetUpcomingShowsRequest{Limit: 5, Offset: 10})
	require.NoError(t, err)
}

func TestExploreHandler_GetUpcomingShows_ServiceErrorBecomes500(t *testing.T) {
	mock := &testhelpers.MockExploreService{
		GetUpcomingShowsFn: func(int, int) (*contracts.ExploreUpcomingShowsResponse, error) {
			return nil, errors.New("db blew up")
		},
	}
	handler := NewExploreHandler(mock)

	resp, err := handler.GetUpcomingShowsHandler(context.Background(), &GetUpcomingShowsRequest{Limit: 20})
	require.Error(t, err)
	assert.Nil(t, resp)
}

// =============================================================================
// Featured
// =============================================================================

func TestExploreHandler_GetFeatured_NullableFieldsPassThrough(t *testing.T) {
	mock := &testhelpers.MockExploreService{
		GetFeaturedFn: func() (*contracts.ExploreFeaturedResponse, error) {
			return &contracts.ExploreFeaturedResponse{
				Bill:       nil,
				Collection: nil,
			}, nil
		},
	}
	handler := NewExploreHandler(mock)

	resp, err := handler.GetFeaturedHandler(context.Background(), &GetFeaturedRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Body.Bill)
	assert.Nil(t, resp.Body.Collection)
}

func TestExploreHandler_GetFeatured_PopulatedSlotsPassThrough(t *testing.T) {
	curatorNote := "smoke"
	mock := &testhelpers.MockExploreService{
		GetFeaturedFn: func() (*contracts.ExploreFeaturedResponse, error) {
			return &contracts.ExploreFeaturedResponse{
				Bill: &contracts.ExploreFeaturedBill{
					ID: 42, Title: "Bill", HeadlinerName: "HL", CuratorNote: &curatorNote,
				},
				Collection: &contracts.ExploreFeaturedCollection{
					ID: 100, Title: "Crate", Slug: "crate-slug",
				},
			}, nil
		},
	}
	handler := NewExploreHandler(mock)

	resp, err := handler.GetFeaturedHandler(context.Background(), &GetFeaturedRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp.Body.Bill)
	assert.Equal(t, uint(42), resp.Body.Bill.ID)
	require.NotNil(t, resp.Body.Collection)
	assert.Equal(t, "crate-slug", resp.Body.Collection.Slug)
}

func TestExploreHandler_GetFeatured_ServiceErrorBecomes500(t *testing.T) {
	mock := &testhelpers.MockExploreService{
		GetFeaturedFn: func() (*contracts.ExploreFeaturedResponse, error) {
			return nil, errors.New("fail")
		},
	}
	handler := NewExploreHandler(mock)

	resp, err := handler.GetFeaturedHandler(context.Background(), &GetFeaturedRequest{})
	require.Error(t, err)
	assert.Nil(t, resp)
}

// =============================================================================
// Shuffle Target
// =============================================================================

func TestExploreHandler_GetShuffleTarget_EmptyPoolPassesNilThrough(t *testing.T) {
	mock := &testhelpers.MockExploreService{
		GetShuffleTargetFn: func() (*contracts.ExploreShuffleTargetResponse, error) {
			return &contracts.ExploreShuffleTargetResponse{}, nil
		},
	}
	handler := NewExploreHandler(mock)

	resp, err := handler.GetShuffleTargetHandler(context.Background(), &GetShuffleTargetRequest{})
	require.NoError(t, err)
	assert.Nil(t, resp.Body.ArtistID)
	assert.Nil(t, resp.Body.ArtistName)
}

func TestExploreHandler_GetShuffleTarget_PopulatedPassThrough(t *testing.T) {
	id := uint(7)
	slug := "the-artist"
	name := "The Artist"
	mock := &testhelpers.MockExploreService{
		GetShuffleTargetFn: func() (*contracts.ExploreShuffleTargetResponse, error) {
			return &contracts.ExploreShuffleTargetResponse{
				ArtistID:   &id,
				ArtistSlug: &slug,
				ArtistName: &name,
			}, nil
		},
	}
	handler := NewExploreHandler(mock)

	resp, err := handler.GetShuffleTargetHandler(context.Background(), &GetShuffleTargetRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp.Body.ArtistID)
	assert.Equal(t, uint(7), *resp.Body.ArtistID)
	assert.Equal(t, "the-artist", *resp.Body.ArtistSlug)
	assert.Equal(t, "The Artist", *resp.Body.ArtistName)
}

func TestExploreHandler_GetShuffleTarget_ServiceErrorBecomes500(t *testing.T) {
	mock := &testhelpers.MockExploreService{
		GetShuffleTargetFn: func() (*contracts.ExploreShuffleTargetResponse, error) {
			return nil, errors.New("fail")
		},
	}
	handler := NewExploreHandler(mock)

	resp, err := handler.GetShuffleTargetHandler(context.Background(), &GetShuffleTargetRequest{})
	require.Error(t, err)
	assert.Nil(t, resp)
}
