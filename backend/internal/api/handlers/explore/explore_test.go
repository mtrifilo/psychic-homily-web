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
		GetUpcomingShowsFn: func(limit, offset int, cities []contracts.CityStateFilter) (*contracts.ExploreUpcomingShowsResponse, error) {
			assert.Equal(t, 20, limit)
			assert.Equal(t, 0, offset)
			assert.Empty(t, cities, "no cities param ⇒ nil filter")
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
		GetUpcomingShowsFn: func(limit, offset int, _ []contracts.CityStateFilter) (*contracts.ExploreUpcomingShowsResponse, error) {
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
		GetUpcomingShowsFn: func(int, int, []contracts.CityStateFilter) (*contracts.ExploreUpcomingShowsResponse, error) {
			return nil, errors.New("db blew up")
		},
	}
	handler := NewExploreHandler(mock)

	resp, err := handler.GetUpcomingShowsHandler(context.Background(), &GetUpcomingShowsRequest{Limit: 20})
	require.Error(t, err)
	assert.Nil(t, resp)
}

func TestExploreHandler_GetUpcomingShows_ParsesCitiesParam(t *testing.T) {
	var got []contracts.CityStateFilter
	mock := &testhelpers.MockExploreService{
		GetUpcomingShowsFn: func(_, _ int, cities []contracts.CityStateFilter) (*contracts.ExploreUpcomingShowsResponse, error) {
			got = cities
			return &contracts.ExploreUpcomingShowsResponse{}, nil
		},
	}
	handler := NewExploreHandler(mock)

	_, err := handler.GetUpcomingShowsHandler(context.Background(),
		&GetUpcomingShowsRequest{Limit: 20, Cities: " Phoenix , AZ |Omaha,NE| malformed |"})
	require.NoError(t, err)
	require.Equal(t, []contracts.CityStateFilter{
		{City: "Phoenix", State: "AZ"},
		{City: "Omaha", State: "NE"},
	}, got, "trims whitespace, parses pairs, skips malformed segments")
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
