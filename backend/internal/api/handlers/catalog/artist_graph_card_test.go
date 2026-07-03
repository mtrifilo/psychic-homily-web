package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	pherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1345: the graph-card handler composes four service lookups. The tests
// pin (1) id-vs-slug resolution, (2) the assembled happy-path shape incl.
// station de-dup + play-count summing and the member_of+side_project fold,
// (3) 404 on unknown artist, and (4) the degradation contract — enrichment
// failures yield empty shapes, never an error.

func graphCardMocks() (*testhelpers.MockArtistService, *testhelpers.MockArtistRelationshipService, *testhelpers.MockRadioService) {
	city := "Providence"
	state := "RI"
	artist := &contracts.ArtistDetailResponse{
		ID: 7, Slug: "lightning-bolt", Name: "Lightning Bolt", City: &city, State: &state,
	}
	artistSvc := &testhelpers.MockArtistService{
		GetArtistFn: func(id uint) (*contracts.ArtistDetailResponse, error) {
			if id != 7 {
				return nil, pherrors.ErrArtistNotFound(id)
			}
			return artist, nil
		},
		GetArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
			if slug != "lightning-bolt" {
				return nil, pherrors.ErrArtistNotFound(0)
			}
			return artist, nil
		},
		GetShowsForArtistFn: func(id uint, tz string, limit int, filter string) ([]*contracts.ArtistShowResponse, int64, error) {
			return []*contracts.ArtistShowResponse{{
				ID:        99,
				EventDate: time.Date(2026, 6, 12, 20, 0, 0, 0, time.UTC),
				Venue:     &contracts.ArtistShowVenueResponse{Name: "Trunk Space", City: "Phoenix", State: "AZ"},
			}}, 1, nil
		},
		GetLabelsForArtistFn: func(id uint) ([]*contracts.ArtistLabelResponse, error) {
			return []*contracts.ArtistLabelResponse{{ID: 3, Name: "Thrill Jockey", Slug: "thrill-jockey"}}, nil
		},
	}
	relSvc := &testhelpers.MockArtistRelationshipService{
		CountRelationshipsByTypeFn: func(id uint) (map[string]int, error) {
			return map[string]int{
				"shared_bills":       7,
				"similar":            4,
				"member_of":          1,
				"side_project":       1,
				"radio_cooccurrence": 5,
				"shared_label":       3,
			}, nil
		},
	}
	radioSvc := &testhelpers.MockRadioService{
		GetAsHeardOnForArtistFn: func(id uint) ([]*contracts.RadioAsHeardOnResponse, error) {
			// Per-row (station, radio show) ordering: KEXP's single show
			// outranks either WFMU row, but WFMU's TOTAL (15+14) wins —
			// pins the per-station aggregation before ordering.
			return []*contracts.RadioAsHeardOnResponse{
				{StationID: 2, StationName: "KEXP", PlayCount: 20},
				{StationID: 1, StationName: "WFMU", PlayCount: 15},
				{StationID: 1, StationName: "WFMU", PlayCount: 14},
			}, nil
		},
	}
	return artistSvc, relSvc, radioSvc
}

func TestGetArtistGraphCardHandler_AssemblesCard(t *testing.T) {
	artistSvc, relSvc, radioSvc := graphCardMocks()
	h := NewArtistGraphCardHandler(artistSvc, relSvc, radioSvc)

	resp, err := h.GetArtistGraphCardHandler(context.Background(), &GetArtistGraphCardRequest{ArtistID: "7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	card := resp.Body
	if card.Name != "Lightning Bolt" || card.Slug != "lightning-bolt" {
		t.Errorf("identity mismatch: %+v", card)
	}
	if card.NextShow == nil || card.NextShow.VenueName != "Trunk Space" || card.NextShow.VenueCity != "Phoenix" {
		t.Errorf("next show mismatch: %+v", card.NextShow)
	}
	if len(card.Labels) != 1 || card.Labels[0].Name != "Thrill Jockey" {
		t.Errorf("labels mismatch: %+v", card.Labels)
	}
	if card.Radio == nil {
		t.Fatalf("expected radio block")
	}
	if card.Radio.PlayCount != 49 {
		t.Errorf("play count: want 49 (summed across rows), got %d", card.Radio.PlayCount)
	}
	if len(card.Radio.Stations) != 2 || card.Radio.Stations[0] != "WFMU" || card.Radio.Stations[1] != "KEXP" {
		t.Errorf("stations must order by per-station TOTAL (WFMU 29 > KEXP 20): %v", card.Radio.Stations)
	}
	want := contracts.ArtistGraphCardConnections{Bills: 7, Similar: 4, Members: 2, Radio: 5, SharedLabels: 3}
	if card.Connections != want {
		t.Errorf("connections: want %+v, got %+v", want, card.Connections)
	}
}

func TestGetArtistGraphCardHandler_ResolvesSlug(t *testing.T) {
	artistSvc, relSvc, radioSvc := graphCardMocks()
	h := NewArtistGraphCardHandler(artistSvc, relSvc, radioSvc)

	resp, err := h.GetArtistGraphCardHandler(context.Background(), &GetArtistGraphCardRequest{ArtistID: "lightning-bolt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 7 {
		t.Errorf("slug resolution failed: %+v", resp.Body)
	}
}

func TestGetArtistGraphCardHandler_NotFound(t *testing.T) {
	artistSvc, relSvc, radioSvc := graphCardMocks()
	h := NewArtistGraphCardHandler(artistSvc, relSvc, radioSvc)

	_, err := h.GetArtistGraphCardHandler(context.Background(), &GetArtistGraphCardRequest{ArtistID: "999"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetArtistGraphCardHandler_DegradesOnEnrichmentFailure(t *testing.T) {
	artistSvc, relSvc, radioSvc := graphCardMocks()
	artistSvc.GetShowsForArtistFn = func(uint, string, int, string) ([]*contracts.ArtistShowResponse, int64, error) {
		return nil, 0, fmt.Errorf("shows query broke")
	}
	artistSvc.GetLabelsForArtistFn = func(uint) ([]*contracts.ArtistLabelResponse, error) {
		return nil, fmt.Errorf("labels query broke")
	}
	radioSvc.GetAsHeardOnForArtistFn = func(uint) ([]*contracts.RadioAsHeardOnResponse, error) {
		return nil, fmt.Errorf("radio query broke")
	}
	relSvc.CountRelationshipsByTypeFn = func(uint) (map[string]int, error) {
		return nil, fmt.Errorf("counts query broke")
	}
	h := NewArtistGraphCardHandler(artistSvc, relSvc, radioSvc)

	resp, err := h.GetArtistGraphCardHandler(context.Background(), &GetArtistGraphCardRequest{ArtistID: "7"})
	if err != nil {
		t.Fatalf("degradation contract violated — enrichment failure must not error: %v", err)
	}
	card := resp.Body
	if card.Name != "Lightning Bolt" {
		t.Errorf("identity must survive: %+v", card)
	}
	if card.NextShow != nil || card.Radio != nil || len(card.Labels) != 0 {
		t.Errorf("enrichment failures must yield empty shapes: %+v", card)
	}
	if card.Connections != (contracts.ArtistGraphCardConnections{}) {
		t.Errorf("counts failure must yield zero connections: %+v", card.Connections)
	}
}
