package engagement

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
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
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "body is required") || strings.Contains(err.Error(), "exceeds maximum length") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "field notes can only be added to past shows") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "sound_quality must be") || strings.Contains(err.Error(), "crowd_energy must be") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "artist is not on this show") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "please wait") || strings.Contains(err.Error(), "hourly comment limit") {
			return nil, rateLimited429(err)
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create field note (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_field_note", "show", fieldNote.ID, map[string]interface{}{
				"show_id": uint(showID),
			})
		}()
	}

	return &CreateFieldNoteResponse{Body: fieldNote}, nil
}

// ============================================================================
// List Field Notes (public/optional auth)
// ============================================================================

// ListFieldNotesRequest represents the Huma request for listing field notes on a show.
type ListFieldNotesRequest struct {
	ShowID string `path:"show_id" doc:"Show ID" example:"42"`
	Limit  int    `query:"limit" required:"false" doc:"Page size (default 25, max 100)" example:"25"`
	Offset int    `query:"offset" required:"false" doc:"Pagination offset" example:"0"`
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
