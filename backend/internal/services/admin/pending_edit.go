package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// PendingEditService handles business logic for generic pending entity edits.
//
// md is the shared utils.MarkdownRenderer (goldmark + bluemonday,
// comment-system allowlist) used to render the submitter's `summary` and the
// admin's `rejection_reason` on read (PSY-605). Sanitization is applied on
// every response so existing plain-text rows are also rendered safely — the
// sanitizer is the source of truth for XSS safety, not the input pipeline.
type PendingEditService struct {
	db              *gorm.DB
	revisionService contracts.RevisionServiceInterface
	emailService    contracts.EmailServiceInterface
	frontendURL     string
	// backendURL + jwtSecret mint the HMAC-signed edit-notifications
	// unsubscribe URL placed in the approval/rejection emails.
	backendURL string
	jwtSecret  string
	md         *utils.MarkdownRenderer
	// bandcampFiller resolves a newly-applied artist Bandcamp PROFILE root → an
	// embed (PSY-1190 fill-when-empty). Optional/nil-safe — when unset (older
	// tests), the approval applies the bandcamp change but skips embed resolution.
	// Wired in the service container (SetBandcampFiller).
	bandcampFiller contracts.BandcampProfileFillerInterface
}

// SetBandcampFiller wires the PSY-1190 profile→embed resolver used after a
// pending edit that sets an artist's social.bandcamp is approved. Optional — the
// approval flow is a no-op for embed resolution when this is nil.
func (s *PendingEditService) SetBandcampFiller(f contracts.BandcampProfileFillerInterface) {
	s.bandcampFiller = f
}

// NewPendingEditService creates a new PendingEditService.
func NewPendingEditService(database *gorm.DB, revisionService contracts.RevisionServiceInterface, emailService contracts.EmailServiceInterface, frontendURL, backendURL, jwtSecret string) *PendingEditService {
	if database == nil {
		database = db.GetDB()
	}
	return &PendingEditService{
		db:              database,
		revisionService: revisionService,
		emailService:    emailService,
		frontendURL:     frontendURL,
		backendURL:      backendURL,
		jwtSecret:       jwtSecret,
		md:              utils.NewMarkdownRenderer(),
	}
}

// renderMarkdown returns sanitized HTML for the given markdown source. Returns
// "" for empty input. Falls back to a freshly-constructed renderer when the
// service was built without one (older test paths or bare struct literals).
func (s *PendingEditService) renderMarkdown(src string) string {
	if src == "" {
		return ""
	}
	if s.md == nil {
		s.md = utils.NewMarkdownRenderer()
	}
	return s.md.Render(src)
}

// renderRejectionReason is a *string-aware wrapper around renderMarkdown for
// the nullable rejection_reason column. Returns "" when the pointer is nil or
// empty.
func (s *PendingEditService) renderRejectionReason(reason *string) string {
	if reason == nil {
		return ""
	}
	return s.renderMarkdown(*reason)
}

// CreatePendingEdit submits a new pending edit for an entity.
func (s *PendingEditService) CreatePendingEdit(req *contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("database not initialized"))
	}

	if !adminm.IsValidPendingEditEntityType(req.EntityType) {
		return nil, apperrors.ErrPendingEditInvalidEntityType(req.EntityType)
	}
	if len(req.Changes) == 0 {
		return nil, apperrors.ErrPendingEditInvalidRequest("no changes provided")
	}
	if req.Summary == "" {
		return nil, apperrors.ErrPendingEditInvalidRequest("summary is required")
	}
	// PSY-605: cap the markdown source at the same length comments and
	// collection descriptions use, so the rendered output is bounded and the
	// renderer's allocation profile stays consistent with the rest of the
	// markdown surfaces.
	if len(req.Summary) > contracts.MaxPendingEditSummaryLength {
		return nil, apperrors.ErrPendingEditInvalidRequest(fmt.Sprintf("summary exceeds maximum length of %d characters", contracts.MaxPendingEditSummaryLength))
	}

	// Verify the entity exists
	tableName := req.EntityType + "s"
	var count int64
	if err := s.db.Table(tableName).Where("id = ?", req.EntityID).Count(&count).Error; err != nil {
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to verify entity: %w", err))
	}
	if count == 0 {
		return nil, apperrors.ErrPendingEditEntityNotFound(req.EntityType, req.EntityID)
	}

	changesJSON, err := json.Marshal(req.Changes)
	if err != nil {
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to marshal changes: %w", err))
	}
	raw := json.RawMessage(changesJSON)

	edit := &adminm.PendingEntityEdit{
		EntityType:   req.EntityType,
		EntityID:     req.EntityID,
		SubmittedBy:  req.UserID,
		FieldChanges: &raw,
		Summary:      req.Summary,
		Status:       adminm.PendingEditStatusPending,
	}

	if err := s.db.Create(edit).Error; err != nil {
		// A unique-constraint violation means the submitter already has a
		// pending edit for this entity — a conflict, not an internal fault.
		if shared.IsDuplicateKey(err) {
			return nil, apperrors.ErrPendingEditDuplicate(err)
		}
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to create pending edit: %w", err))
	}

	// Reload with relationships
	return s.GetPendingEdit(edit.ID)
}

// GetPendingEdit returns a single pending edit by ID.
func (s *PendingEditService) GetPendingEdit(editID uint) (*contracts.PendingEditResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("database not initialized"))
	}

	var edit adminm.PendingEntityEdit
	err := s.db.Preload("Submitter").Preload("Reviewer").First(&edit, editID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to get pending edit: %w", err))
	}

	return s.toResponse(&edit), nil
}

// GetPendingEditsForEntity returns all pending edits for a specific entity.
func (s *PendingEditService) GetPendingEditsForEntity(entityType string, entityID uint) ([]contracts.PendingEditResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("database not initialized"))
	}

	var edits []adminm.PendingEntityEdit
	err := s.db.Where("entity_type = ? AND entity_id = ? AND status = ?", entityType, entityID, adminm.PendingEditStatusPending).
		Preload("Submitter").
		Order("created_at ASC").
		Find(&edits).Error
	if err != nil {
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to get pending edits for entity: %w", err))
	}

	return s.toResponses(edits), nil
}

// GetUserPendingEdits returns all pending edits submitted by a user.
func (s *PendingEditService) GetUserPendingEdits(userID uint, limit, offset int) ([]contracts.PendingEditResponse, int64, error) {
	if s.db == nil {
		return nil, 0, apperrors.ErrPendingEditInternal(fmt.Errorf("database not initialized"))
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var total int64
	s.db.Model(&adminm.PendingEntityEdit{}).Where("submitted_by = ?", userID).Count(&total)

	var edits []adminm.PendingEntityEdit
	err := s.db.Where("submitted_by = ?", userID).
		Preload("Submitter").
		Preload("Reviewer").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&edits).Error
	if err != nil {
		return nil, 0, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to get user pending edits: %w", err))
	}

	return s.toResponses(edits), total, nil
}

// ListPendingEdits returns pending edits for the admin review queue.
func (s *PendingEditService) ListPendingEdits(filters *contracts.PendingEditFilters) ([]contracts.PendingEditResponse, int64, error) {
	if s.db == nil {
		return nil, 0, apperrors.ErrPendingEditInternal(fmt.Errorf("database not initialized"))
	}

	limit := 20
	offset := 0
	if filters != nil {
		if filters.Limit > 0 && filters.Limit <= 100 {
			limit = filters.Limit
		}
		if filters.Offset > 0 {
			offset = filters.Offset
		}
	}

	query := s.db.Model(&adminm.PendingEntityEdit{})

	if filters != nil {
		if filters.Status != "" {
			query = query.Where("status = ?", filters.Status)
		}
		if filters.EntityType != "" {
			query = query.Where("entity_type = ?", filters.EntityType)
		}
	}

	var total int64
	query.Count(&total)

	var edits []adminm.PendingEntityEdit
	err := query.
		Preload("Submitter").
		Preload("Reviewer").
		Order("created_at ASC").
		Limit(limit).
		Offset(offset).
		Find(&edits).Error
	if err != nil {
		return nil, 0, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to list pending edits: %w", err))
	}

	return s.toResponses(edits), total, nil
}

// updatedString returns the pending-edit's new value for key when it is present
// and a string, otherwise the fallback (the entity's current value). Used to
// build the effective post-edit location for re-geocoding.
func updatedString(updates map[string]interface{}, key, fallback string) string {
	if v, ok := updates[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

// ApprovePendingEdit approves a pending edit, applying changes to the entity
// and recording a revision.
func (s *PendingEditService) ApprovePendingEdit(editID uint, reviewerID uint) (*contracts.PendingEditResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("database not initialized"))
	}

	var edit adminm.PendingEntityEdit
	if err := s.db.First(&edit, editID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPendingEditNotFound()
		}
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to get pending edit: %w", err))
	}

	if edit.Status != adminm.PendingEditStatusPending {
		return nil, apperrors.ErrPendingEditNotPending(string(edit.Status))
	}

	// Parse field changes
	var changes []adminm.FieldChange
	if err := json.Unmarshal(*edit.FieldChanges, &changes); err != nil {
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to parse field changes: %w", err))
	}

	// PSY-572: per-entity allowlist gate. Defence in depth — even though the
	// suggest-edit handler validates field names at submission time, an
	// attacker (or a buggy/legacy code path) that manages to land a
	// pending_entity_edits row carrying a non-allowlisted column (e.g.
	// is_admin, password_hash, trust_tier) must not have it applied via
	// the untyped Updates() call below. If any rejected fields are present,
	// auto-mark the pending_edit 'rejected' with a clear reason and bail
	// before mutating the entity.
	_, rejectedFields := adminm.FilterAllowedFields(edit.EntityType, changes)
	if len(rejectedFields) > 0 {
		joined := strings.Join(rejectedFields, ", ")
		reason := fmt.Sprintf(
			"Rejected automatically: pending edit carries %d field(s) not allowed for %s entities (%s). "+
				"This usually indicates a corrupted submission — the contributor's UI does not expose these fields.",
			len(rejectedFields), edit.EntityType, joined,
		)
		slog.Default().Error("pending_edit_disallowed_fields",
			"edit_id", edit.ID,
			"entity_type", edit.EntityType,
			"entity_id", edit.EntityID,
			"submitted_by", edit.SubmittedBy,
			"reviewer_id", reviewerID,
			"rejected_fields", rejectedFields,
		)
		now := time.Now()
		if err := s.db.Model(&edit).Updates(map[string]interface{}{
			"status":           adminm.PendingEditStatusRejected,
			"reviewed_by":      reviewerID,
			"reviewed_at":      now,
			"rejection_reason": reason,
			"updated_at":       now,
		}).Error; err != nil {
			return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to auto-reject pending edit with disallowed fields: %w", err))
		}
		// Sentinel (NOT a PendingEditError): the approve handler maps this via
		// errors.Is to a 400 with the rejected field list. Keep it as-is.
		return nil, fmt.Errorf("%w: %s", adminm.ErrPendingEditDisallowedFields, joined)
	}

	// Build update map from new values
	updates := make(map[string]interface{})
	for _, c := range changes {
		updates[c.Field] = c.NewValue
	}
	updates["updated_at"] = time.Now()

	// PSY-985: a venue location edit through the contribution flow bypasses
	// VenueService, so re-geocode here too. Resolve the effective post-edit
	// location (changed value, else current) and write latitude/longitude/
	// timezone — nil on a miss → SQL NULL → legacy state->tz fallback. These
	// columns are system-derived (not in the contributor allowlist), so we set
	// them programmatically after the allowlist filter above.
	if edit.EntityType == "venue" {
		_, cityChanged := updates["city"]
		_, stateChanged := updates["state"]
		_, countryChanged := updates["country"]
		if cityChanged || stateChanged || countryChanged {
			var current catalogm.Venue
			if err := s.db.Select("city", "state", "country").First(&current, edit.EntityID).Error; err == nil {
				currentCountry := ""
				if current.Country != nil {
					currentCountry = *current.Country
				}
				lat, lng, tz := geo.LookupPointers(
					geo.Default(),
					updatedString(updates, "city", current.City),
					updatedString(updates, "state", current.State),
					updatedString(updates, "country", currentCountry),
				)
				updates["latitude"] = lat
				updates["longitude"] = lng
				updates["timezone"] = tz
				// metro is a sibling of the geocoding (PSY-1255 step B): keep it
				// fresh when a contribution edit relocates the venue.
				updates["metro"] = geo.MetroPointer(geo.Default(),
					updatedString(updates, "city", current.City),
					updatedString(updates, "state", current.State),
					updatedString(updates, "country", currentCountry))
			}
		}
	}

	// The closure returns typed errors directly: a vanished entity is a 422
	// (the edit can no longer be applied), everything else is a 500.
	err := s.db.Transaction(func(tx *gorm.DB) error {
		tableName := edit.EntityType + "s"
		result := tx.Table(tableName).Where("id = ?", edit.EntityID).Updates(updates)
		if result.Error != nil {
			return apperrors.ErrPendingEditInternal(fmt.Errorf("failed to apply changes: %w", result.Error))
		}
		if result.RowsAffected == 0 {
			return apperrors.ErrPendingEditEntityGone(edit.EntityType, edit.EntityID)
		}

		// Mark edit as approved
		now := time.Now()
		if err := tx.Model(&edit).Updates(map[string]interface{}{
			"status":      adminm.PendingEditStatusApproved,
			"reviewed_by": reviewerID,
			"reviewed_at": now,
			"updated_at":  now,
		}).Error; err != nil {
			return apperrors.ErrPendingEditInternal(fmt.Errorf("failed to update edit status: %w", err))
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// PSY-1190: an artist edit that sets social.bandcamp lands here via a direct
	// UPDATE (community suggestion OR trusted-tier inline edit auto-applied through
	// canEditDirectly), bypassing ArtistService.UpdateArtist and its profile→embed
	// resolver. Mirror that resolver here so a profile root set this way still
	// fills bandcamp_embed_url (profile_resolved, fill-when-empty; a manual value
	// is left untouched by the resolver's IS NULL guard). Runs after the approval
	// commits; the filler itself dispatches the network fetch off-thread.
	if s.bandcampFiller != nil && edit.EntityType == "artist" {
		if v, ok := updates["bandcamp"]; ok {
			if bc, ok := v.(string); ok && bc != "" {
				s.bandcampFiller.FillProfileResolvedEmbedFromBandcamp(edit.EntityID, bc)
			}
		}
	}

	// Record revision (fire-and-forget — don't fail the approval if this errors)
	if s.revisionService != nil {
		_ = s.revisionService.RecordRevision(edit.EntityType, edit.EntityID, edit.SubmittedBy, changes, edit.Summary)
	}

	// Send approval notification email (fire-and-forget)
	s.sendApprovalEmail(&edit)

	return s.GetPendingEdit(editID)
}

// RejectPendingEdit rejects a pending edit with a reason.
func (s *PendingEditService) RejectPendingEdit(editID uint, reviewerID uint, reason string) (*contracts.PendingEditResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("database not initialized"))
	}

	if reason == "" {
		return nil, apperrors.ErrPendingEditInvalidRequest("rejection reason is required")
	}
	// PSY-605: rejection_reason mirrors summary's markdown stack and limit so
	// the contributor-side render (PSY-600 surface, when it ships) is bounded.
	if len(reason) > contracts.MaxPendingEditSummaryLength {
		return nil, apperrors.ErrPendingEditInvalidRequest(fmt.Sprintf("rejection reason exceeds maximum length of %d characters", contracts.MaxPendingEditSummaryLength))
	}

	var edit adminm.PendingEntityEdit
	if err := s.db.First(&edit, editID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPendingEditNotFound()
		}
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to get pending edit: %w", err))
	}

	if edit.Status != adminm.PendingEditStatusPending {
		return nil, apperrors.ErrPendingEditNotPending(string(edit.Status))
	}

	now := time.Now()
	if err := s.db.Model(&edit).Updates(map[string]interface{}{
		"status":           adminm.PendingEditStatusRejected,
		"reviewed_by":      reviewerID,
		"reviewed_at":      now,
		"rejection_reason": reason,
		"updated_at":       now,
	}).Error; err != nil {
		return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to reject pending edit: %w", err))
	}

	// Send rejection notification email (fire-and-forget)
	s.sendRejectionEmail(&edit, reason)

	return s.GetPendingEdit(editID)
}

// CancelPendingEdit allows the submitter to cancel their own pending edit.
func (s *PendingEditService) CancelPendingEdit(editID uint, userID uint) error {
	if s.db == nil {
		return apperrors.ErrPendingEditInternal(fmt.Errorf("database not initialized"))
	}

	var edit adminm.PendingEntityEdit
	if err := s.db.First(&edit, editID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrPendingEditNotFound()
		}
		return apperrors.ErrPendingEditInternal(fmt.Errorf("failed to get pending edit: %w", err))
	}

	if edit.SubmittedBy != userID {
		return apperrors.ErrPendingEditNotSubmitter()
	}

	if edit.Status != adminm.PendingEditStatusPending {
		return apperrors.ErrPendingEditNotPending(string(edit.Status))
	}

	if err := s.db.Delete(&edit).Error; err != nil {
		return apperrors.ErrPendingEditInternal(fmt.Errorf("failed to delete pending edit: %w", err))
	}
	return nil
}

// toResponse converts a PendingEntityEdit model to a response DTO.
//
// Summary and RejectionReason are rendered on read via the shared
// utils.MarkdownRenderer (goldmark + bluemonday, comment-system allowlist),
// matching the comment + collection-description shape (PSY-605). Raw markdown
// is preserved alongside HTML so contributors can re-populate the textarea
// without re-parsing HTML back to markdown.
func (s *PendingEditService) toResponse(edit *adminm.PendingEntityEdit) *contracts.PendingEditResponse {
	// Single combined lookup so name + slug come from the same row read.
	// Slug is non-nil for slug-addressed entity types — lets the
	// contributor /submissions view build /artists/:slug links instead of
	// dead /artists/:id links (PSY-600).
	entityName, entitySlug := resolveEntityNameAndSlug(s.db, edit.EntityType, edit.EntityID)
	resp := &contracts.PendingEditResponse{
		ID:                  edit.ID,
		EntityType:          edit.EntityType,
		EntityID:            edit.EntityID,
		EntityName:          entityName,
		EntitySlug:          entitySlug,
		SubmittedBy:         edit.SubmittedBy,
		Summary:             edit.Summary,
		SummaryHTML:         s.renderMarkdown(edit.Summary),
		Status:              edit.Status,
		ReviewedBy:          edit.ReviewedBy,
		ReviewedAt:          edit.ReviewedAt,
		RejectionReason:     edit.RejectionReason,
		RejectionReasonHTML: s.renderRejectionReason(edit.RejectionReason),
		CreatedAt:           edit.CreatedAt,
		UpdatedAt:           edit.UpdatedAt,
	}

	// Parse field changes
	if edit.FieldChanges != nil {
		var changes []adminm.FieldChange
		if err := json.Unmarshal(*edit.FieldChanges, &changes); err == nil {
			resp.FieldChanges = changes
		}
	}

	if edit.Submitter.ID != 0 {
		resp.SubmitterName = shared.ResolveUserName(&edit.Submitter)
		resp.SubmitterUsername = shared.ResolveUserUsername(&edit.Submitter)
	}

	if edit.Reviewer != nil && edit.Reviewer.ID != 0 {
		resp.ReviewerName = shared.ResolveUserName(edit.Reviewer)
		resp.ReviewerUsername = shared.ResolveUserUsername(edit.Reviewer)
	}

	return resp
}

// toResponses converts a slice of models to response DTOs.
func (s *PendingEditService) toResponses(edits []adminm.PendingEntityEdit) []contracts.PendingEditResponse {
	responses := make([]contracts.PendingEditResponse, len(edits))
	for i := range edits {
		responses[i] = *s.toResponse(&edits[i])
	}
	return responses
}

// sendApprovalEmail looks up the submitter and entity, then sends an approval notification.
// Fire-and-forget: errors are logged but never fail the parent operation.
func (s *PendingEditService) sendApprovalEmail(edit *adminm.PendingEntityEdit) {
	if s.emailService == nil || !s.emailService.IsConfigured() {
		return
	}

	// Look up submitter
	var user authm.User
	if err := s.db.First(&user, edit.SubmittedBy).Error; err != nil {
		log.Printf("sendApprovalEmail: failed to look up submitter %d: %v", edit.SubmittedBy, err)
		return
	}
	if user.Email == nil || *user.Email == "" {
		return
	}

	if !s.editNotificationsEnabled(user.ID) {
		return
	}

	entityName, entityURL := s.resolveEntityInfo(edit.EntityType, edit.EntityID)
	username := shared.ResolveUserName(&user)
	unsubURL := engagement.GenerateScopedUnsubscribeURL(s.backendURL, user.ID, engagement.UnsubscribeScopeEditNotifications, s.jwtSecret)

	if err := s.emailService.SendEditApprovedEmail(*user.Email, username, edit.EntityType, entityName, entityURL, unsubURL); err != nil {
		log.Printf("sendApprovalEmail: failed to send email to %s: %v", *user.Email, err)
	}
}

// sendRejectionEmail looks up the submitter and entity, then sends a rejection notification.
// Fire-and-forget: errors are logged but never fail the parent operation.
func (s *PendingEditService) sendRejectionEmail(edit *adminm.PendingEntityEdit, reason string) {
	if s.emailService == nil || !s.emailService.IsConfigured() {
		return
	}

	// Look up submitter
	var user authm.User
	if err := s.db.First(&user, edit.SubmittedBy).Error; err != nil {
		log.Printf("sendRejectionEmail: failed to look up submitter %d: %v", edit.SubmittedBy, err)
		return
	}
	if user.Email == nil || *user.Email == "" {
		return
	}

	if !s.editNotificationsEnabled(user.ID) {
		return
	}

	entityName, _ := s.resolveEntityInfo(edit.EntityType, edit.EntityID)
	username := shared.ResolveUserName(&user)
	unsubURL := engagement.GenerateScopedUnsubscribeURL(s.backendURL, user.ID, engagement.UnsubscribeScopeEditNotifications, s.jwtSecret)

	if err := s.emailService.SendEditRejectedEmail(*user.Email, username, edit.EntityType, entityName, reason, unsubURL); err != nil {
		log.Printf("sendRejectionEmail: failed to send email to %s: %v", *user.Email, err)
	}
}

// editNotificationsEnabled reports whether the user wants edit-review emails.
// Defaults to TRUE (opt-OUT): a missing preferences row or a read error means
// the user hasn't opted out, so we send. Only an explicit FALSE suppresses.
func (s *PendingEditService) editNotificationsEnabled(userID uint) bool {
	var prefs authm.UserPreferences
	if err := s.db.Select("notify_on_edit_notifications").
		Where("user_id = ?", userID).First(&prefs).Error; err != nil {
		return true
	}
	return prefs.NotifyOnEditNotifications
}

// resolveEntityInfo looks up an entity's name and builds its frontend URL.
func (s *PendingEditService) resolveEntityInfo(entityType string, entityID uint) (name string, url string) {
	name = fmt.Sprintf("%s #%d", entityType, entityID)
	url = s.frontendURL

	switch entityType {
	case "artist":
		var artist struct {
			Name string
			Slug *string
		}
		if err := s.db.Table("artists").Select("name, slug").Where("id = ?", entityID).Scan(&artist).Error; err == nil {
			name = artist.Name
			if artist.Slug != nil && *artist.Slug != "" {
				url = fmt.Sprintf("%s/artists/%s", s.frontendURL, *artist.Slug)
			}
		}
	case "venue":
		var venue struct {
			Name string
			Slug *string
		}
		if err := s.db.Table("venues").Select("name, slug").Where("id = ?", entityID).Scan(&venue).Error; err == nil {
			name = venue.Name
			if venue.Slug != nil && *venue.Slug != "" {
				url = fmt.Sprintf("%s/venues/%s", s.frontendURL, *venue.Slug)
			}
		}
	case "festival":
		var festival struct {
			Name string
			Slug string
		}
		if err := s.db.Table("festivals").Select("name, slug").Where("id = ?", entityID).Scan(&festival).Error; err == nil {
			name = festival.Name
			if festival.Slug != "" {
				url = fmt.Sprintf("%s/festivals/%s", s.frontendURL, festival.Slug)
			}
		}
	case "release":
		var release struct {
			Title string
			Slug  *string
		}
		if err := s.db.Table("releases").Select("title, slug").Where("id = ?", entityID).Scan(&release).Error; err == nil {
			name = release.Title
			if release.Slug != nil && *release.Slug != "" {
				url = fmt.Sprintf("%s/releases/%s", s.frontendURL, *release.Slug)
			}
		}
	case "label":
		var label struct {
			Name string
			Slug *string
		}
		if err := s.db.Table("labels").Select("name, slug").Where("id = ?", entityID).Scan(&label).Error; err == nil {
			name = label.Name
			if label.Slug != nil && *label.Slug != "" {
				url = fmt.Sprintf("%s/labels/%s", s.frontendURL, *label.Slug)
			}
		}
	}

	return name, url
}
