package engagement

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// ============================================================================
// Focused interfaces for dependency injection
// ============================================================================

// FieldNoteWriter defines the write operations for field notes.
type FieldNoteWriter interface {
	CreateFieldNote(userID uint, req *contracts.CreateFieldNoteRequest) (*contracts.CommentResponse, error)
}

// FieldNoteReader defines the read operations for field notes.
type FieldNoteReader interface {
	ListFieldNotesForShow(showID uint, limit, offset int) (*contracts.CommentListResponse, error)
	// ListFieldNotesForVenue rolls up the notes written about shows held at a
	// venue (PSY-1590) — venues own no field notes of their own.
	ListFieldNotesForVenue(venueID uint, limit, offset int) (*contracts.VenueFieldNoteListResponse, error)
}

// FieldNoteHandler handles field note API requests.
type FieldNoteHandler struct {
	writer          FieldNoteWriter
	reader          FieldNoteReader
	auditLogService contracts.AuditLogServiceInterface
}

// NewFieldNoteHandler creates a new FieldNoteHandler.
func NewFieldNoteHandler(writer FieldNoteWriter, reader FieldNoteReader, auditLogService contracts.AuditLogServiceInterface) *FieldNoteHandler {
	return &FieldNoteHandler{
		writer:          writer,
		reader:          reader,
		auditLogService: auditLogService,
	}
}

// ============================================================================
// Create Field Note (protected)
// ============================================================================

// CreateFieldNoteRequest represents the Huma request for creating a field note.
type CreateFieldNoteRequest struct {
	ShowID string `path:"show_id" doc:"Show ID" example:"42"`
	Body   struct {
		Body           string  `json:"body" doc:"Field note body (Markdown)" example:"The sound was incredible tonight."`
		ShowArtistID   *uint   `json:"show_artist_id,omitempty" required:"false" doc:"Artist ID on the show bill" example:"5"`
		SongPosition   *int    `json:"song_position,omitempty" required:"false" doc:"Position in the setlist (1-based)" example:"3"`
		SoundQuality   *int    `json:"sound_quality,omitempty" required:"false" doc:"Sound quality rating 1-5" example:"4"`
		CrowdEnergy    *int    `json:"crowd_energy,omitempty" required:"false" doc:"Crowd energy rating 1-5" example:"5"`
		NotableMoments *string `json:"notable_moments,omitempty" required:"false" doc:"Notable moments description" example:"Surprise cover of Ziggy Stardust"`
		SetlistSpoiler bool    `json:"setlist_spoiler" required:"false" doc:"Whether this note contains setlist spoilers" example:"false"`
	}
}

// CreateFieldNoteResponse represents the response for creating a field note.
type CreateFieldNoteResponse struct {
	Body *contracts.CommentResponse
}

// CreateFieldNoteHandler handles POST /shows/{show_id}/field-notes
func (h *FieldNoteHandler) CreateFieldNoteHandler(ctx context.Context, req *CreateFieldNoteRequest) (*CreateFieldNoteResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	if strings.TrimSpace(req.Body.Body) == "" {
		return nil, huma.Error400BadRequest("Field note body is required")
	}

	serviceReq := &contracts.CreateFieldNoteRequest{
		ShowID:         uint(showID),
		Body:           req.Body.Body,
		ShowArtistID:   req.Body.ShowArtistID,
		SongPosition:   req.Body.SongPosition,
		SoundQuality:   req.Body.SoundQuality,
		CrowdEnergy:    req.Body.CrowdEnergy,
		NotableMoments: req.Body.NotableMoments,
		SetlistSpoiler: req.Body.SetlistSpoiler,
	}

	fieldNote, err := h.writer.CreateFieldNote(user.ID, serviceReq)
	if err != nil {
		// Field-note creation can fail with either a field-note-specific
		// error (show / past-show / artist-on-bill gates) or a shared
		// CommentError (body validation, sound_quality / crowd_energy,
		// rate limits, user-not-found).
		if mapped := shared.MapFieldNoteError(err); mapped != nil {
			return nil, mapped
		}
		if mapped := shared.MapCommentError(err); mapped != nil {
			return nil, mapped
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create field note (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "create_field_note", "show", fieldNote.ID, map[string]interface{}{
				"show_id": uint(showID),
			})
		})
	}

	return &CreateFieldNoteResponse{Body: fieldNote}, nil
}

// ============================================================================
// List Field Notes (public/optional auth)
// ============================================================================

// ListFieldNotesRequest represents the Huma request for listing field notes on a show.
type ListFieldNotesRequest struct {
	ShowID string `path:"show_id" doc:"Show ID" example:"42"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Page size (default 25, max 100)" example:"25"`
	Offset int    `query:"offset" required:"false" minimum:"0" doc:"Pagination offset" example:"0"`
}

// ListFieldNotesResponse represents the response for listing field notes.
type ListFieldNotesResponse struct {
	Body *contracts.CommentListResponse
}

// ListFieldNotesHandler handles GET /shows/{show_id}/field-notes
func (h *FieldNoteHandler) ListFieldNotesHandler(ctx context.Context, req *ListFieldNotesRequest) (*ListFieldNotesResponse, error) {
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	result, err := h.reader.ListFieldNotesForShow(uint(showID), limit, offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch field notes")
	}

	return &ListFieldNotesResponse{Body: result}, nil
}

// ============================================================================
// List Venue Field Notes (public)
// ============================================================================

// ListVenueFieldNotesRequest represents the Huma request for the venue rollup.
//
// VenueID is NUMERIC ONLY, unlike the id-or-slug `/venues/{venue_id}/shows`
// family. This handler lives in the engagement package and resolving a slug
// would mean wiring the catalog venue service into it purely for a lookup its
// only caller (the Atlas venue panel, which holds the numeric id already) never
// needs. `/venues/{venue_id}/confirm` narrows the same way for its own reason.
// The path PARAMETER still has to be named `venue_id` to match its siblings:
// chi keys its routing tree on path shape, not parameter name, so a `{slug}`
// here would silently rename the parameter for whichever registration lost.
type ListVenueFieldNotesRequest struct {
	VenueID string `path:"venue_id" doc:"Venue ID (numeric only)" example:"42"`
	Limit   int    `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Page size (default 25, max 100)" example:"1"`
	Offset  int    `query:"offset" required:"false" minimum:"0" maximum:"10000" doc:"Pagination offset (max 10000)" example:"0"`
}

// maxVenueFieldNoteOffset bounds the one knob on this anonymous route that a
// caller could otherwise drive without limit: the count/page queries evaluate
// the joins, the JSONB predicate and the sort before an offset discards rows,
// so an absurd offset is pure attacker-driven cost. Clamped (not rejected) to
// match how the limit bound behaves; real note counts per venue sit orders of
// magnitude below this.
const maxVenueFieldNoteOffset = 10000

// ListVenueFieldNotesResponse represents the response for the venue rollup.
type ListVenueFieldNotesResponse struct {
	Body *contracts.VenueFieldNoteListResponse
}

// ListVenueFieldNotesHandler handles GET /venues/{venue_id}/field-notes.
//
// The notes are rolled up from the shows held at the venue — see
// contracts.VenueFieldNote for why a venue-scoped field note cannot exist.
func (h *FieldNoteHandler) ListVenueFieldNotesHandler(ctx context.Context, req *ListVenueFieldNotesRequest) (*ListVenueFieldNotesResponse, error) {
	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > maxVenueFieldNoteOffset {
		offset = maxVenueFieldNoteOffset
	}

	result, err := h.reader.ListFieldNotesForVenue(uint(venueID), limit, offset)
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			// Match the sibling /venues/{venue_id} reads: an unknown or
			// merged-away venue is a 404, never an empty 200 a stale Atlas
			// pin would mistake for "no notes yet".
			return nil, huma.Error404NotFound("Venue not found")
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to fetch venue field notes (request_id: %s)", requestID),
		)
	}

	return &ListVenueFieldNotesResponse{Body: result}, nil
}
