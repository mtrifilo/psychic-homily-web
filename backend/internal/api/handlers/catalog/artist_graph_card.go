package catalog

import (
	"context"
	"errors"
	"sort"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// ArtistGraphCardHandler serves the node-select summary card for graph
// surfaces (PSY-1345): the homepage scene-graph section and the /graph
// Observatory fetch it when the user clicks an artist node. It composes
// existing per-domain service methods into one small response so a node
// click costs one request, not four.
type ArtistGraphCardHandler struct {
	artistService contracts.ArtistServiceInterface
	relService    contracts.ArtistRelationshipServiceInterface
	radioService  contracts.RadioServiceInterface
}

// NewArtistGraphCardHandler creates a new handler.
func NewArtistGraphCardHandler(
	artistService contracts.ArtistServiceInterface,
	relService contracts.ArtistRelationshipServiceInterface,
	radioService contracts.RadioServiceInterface,
) *ArtistGraphCardHandler {
	return &ArtistGraphCardHandler{
		artistService: artistService,
		relService:    relService,
		radioService:  radioService,
	}
}

type GetArtistGraphCardRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID or slug" example:"42"`
	Timezone string `query:"timezone" required:"false" doc:"Timezone for the next-show date filter" example:"America/Phoenix"`
}

type GetArtistGraphCardResponse struct {
	Body contracts.ArtistGraphCard
}

// GetArtistGraphCardHandler handles GET /artists/{artist_id}/graph-card.
//
// Card degradation contract: only the artist identity is required. Every
// enrichment lookup (next show, labels, radio, connection counts) degrades
// to its empty shape on error — a node click on a graph must always yield a
// card, and a partial card beats a 500 (the section's canvas already proved
// the artist exists).
func (h *ArtistGraphCardHandler) GetArtistGraphCardHandler(ctx context.Context, req *GetArtistGraphCardRequest) (*GetArtistGraphCardResponse, error) {
	// ID-or-slug resolution, same convention as GetArtistHandler.
	var artist *contracts.ArtistDetailResponse
	var err error
	if id, parseErr := strconv.ParseUint(req.ArtistID, 10, 32); parseErr == nil {
		artist, err = h.artistService.GetArtist(uint(id))
	} else {
		artist, err = h.artistService.GetArtistBySlug(req.ArtistID)
	}
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch artist", err)
	}

	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}

	card := contracts.ArtistGraphCard{
		ID:     artist.ID,
		Name:   artist.Name,
		Slug:   artist.Slug,
		City:   artist.City,
		State:  artist.State,
		Labels: []contracts.ArtistGraphCardLabel{},
	}

	// Next upcoming show (limit 1, soonest first).
	shows, _, err := h.artistService.GetShowsForArtist(artist.ID, timezone, 1, "upcoming")
	if err != nil {
		logger.FromContext(ctx).Warn("graph-card: next-show lookup failed", "artist_id", artist.ID, "error", err)
	} else if len(shows) > 0 && shows[0] != nil {
		show := shows[0]
		next := &contracts.ArtistGraphCardShow{
			ID:        show.ID,
			EventDate: show.EventDate,
		}
		if show.Venue != nil {
			next.VenueName = show.Venue.Name
			next.VenueCity = show.Venue.City
			next.VenueState = show.Venue.State
			next.VenueTimezone = show.Venue.Timezone
		}
		card.NextShow = next
	}

	labels, err := h.artistService.GetLabelsForArtist(artist.ID)
	if err != nil {
		logger.FromContext(ctx).Warn("graph-card: labels lookup failed", "artist_id", artist.ID, "error", err)
	}
	for _, l := range labels {
		if l == nil {
			continue
		}
		card.Labels = append(card.Labels, contracts.ArtistGraphCardLabel{Name: l.Name, Slug: l.Slug})
	}

	// "As heard on": GetAsHeardOnForArtist returns one row per (station,
	// radio show) ordered by PER-ROW play count — first-encounter order
	// would rank a one-show station above a station whose larger total is
	// split across shows. Aggregate per station, then sort by station total.
	rows, err := h.radioService.GetAsHeardOnForArtist(artist.ID)
	if err != nil {
		logger.FromContext(ctx).Warn("graph-card: radio lookup failed", "artist_id", artist.ID, "error", err)
	} else if len(rows) > 0 {
		type stationTotal struct {
			name  string
			plays int
		}
		order := []uint{}
		totals := make(map[uint]*stationTotal, len(rows))
		total := 0
		for _, r := range rows {
			if r == nil {
				continue
			}
			total += r.PlayCount
			if st, ok := totals[r.StationID]; ok {
				st.plays += r.PlayCount
			} else {
				totals[r.StationID] = &stationTotal{name: r.StationName, plays: r.PlayCount}
				order = append(order, r.StationID)
			}
		}
		if total > 0 {
			sort.SliceStable(order, func(i, j int) bool {
				return totals[order[i]].plays > totals[order[j]].plays
			})
			radio := &contracts.ArtistGraphCardRadio{Stations: make([]string, 0, len(order)), PlayCount: total}
			for _, id := range order {
				radio.Stations = append(radio.Stations, totals[id].name)
			}
			card.Radio = radio
		}
	}

	counts, err := h.relService.CountRelationshipsByType(artist.ID)
	if err != nil {
		logger.FromContext(ctx).Warn("graph-card: relationship counts failed", "artist_id", artist.ID, "error", err)
	} else {
		card.Connections = contracts.ArtistGraphCardConnections{
			Bills:        counts[catalogm.RelationshipTypeSharedBills],
			Similar:      counts[catalogm.RelationshipTypeSimilar],
			Members:      counts[catalogm.RelationshipTypeMemberOf] + counts[catalogm.RelationshipTypeSideProject],
			Radio:        counts[catalogm.RelationshipTypeRadioCooccurrence],
			SharedLabels: counts[catalogm.RelationshipTypeSharedLabel],
		}
	}

	return &GetArtistGraphCardResponse{Body: card}, nil
}
