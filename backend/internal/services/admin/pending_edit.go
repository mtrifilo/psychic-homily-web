package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
	"psychic-homily-backend/internal/utils/urlguard"
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

	// Read the entity and take its CURRENT value for every field this edit
	// names, in place of whatever the submitter claimed the previous value was.
	// See the OLD-VALUE CONTRACT in pending_edit_old_value.go: the stored
	// old_value reaches a column verbatim through Rollback, so it may not be
	// contributor input. This also stands in for the entity-exists check, since
	// a missing entity is what the read reports.
	changes, err := deriveOldValues(s.db, req.EntityType, req.EntityID, req.Changes)
	if err != nil {
		return nil, err
	}

	changesJSON, err := json.Marshal(changes)
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

// NarrowNumericUpdates rewrites every registered whole-number field in an
// update map from the shape JSONB hands back (float64) to the shape the column
// actually is (*int), leaving a nil as a typed nil so it lands as SQL NULL.
//
// This is a TYPE fix, not a policy one, and it is deliberately separate from the
// range check below. The layers underneath accept a float or a numeric string
// for an integer column without complaint and quietly truncate (see
// utils.WholeNumber), so any path that feeds an untyped Updates() from stored
// JSON needs this or it writes a value nobody chose. Both such paths call it:
// ApprovePendingEdit, and RevisionService.Rollback.
//
// Rollback deliberately gets narrowing WITHOUT the range check. It restores a
// value this system previously stored, and history can legitimately contain
// values that predate a bound, so re-litigating the range there would break
// undo for exactly the rows most likely to need it.
//
// A value that is not a whole number at all is returned as an error rather than
// written, because there is no faithful narrowing of "banana" to an integer.
//
// LEGACY ENCODING: for a field whose registry entry sets LegacyTextEncoding, a
// stored value that is a plain decimal-integer STRING is parsed rather than
// refused. That asymmetry with the submit-side gate is deliberate.
// founded_year and release_year joined the registry in PSY-1703, long after the
// edit drawer began submitting them as text, so any year edit accepted before
// then wrote a STRING into pending_entity_edits.changes and, once approved, into
// revisions.field_changes. How many such rows exist was not measured; that the
// old drawer could only produce that shape is a fact about the code. The submit
// gate refuses that shape at the door so nothing NEW is written that way (one
// encoding per edit, per PSY-1694). This function is on the other side of the
// pipeline: it type-corrects what the system has already stored. Refusing those
// rows would strand pending edits that are perfectly readable and, worse, break
// Rollback for exactly the history most likely to need undoing. Rollback already
// declines to re-litigate a stored value against a bound for that reason;
// declining to re-litigate its encoding follows from it.
//
// "1985" has exactly one integer meaning, so parsing it invents nothing, and the
// corruption class the registry exists for -- fractions, bools, objects,
// out-of-int values -- is still refused. The approve path still range checks
// afterwards.
//
// The flag rather than blanket string tolerance, because capacity has no such
// history: its registry entry and its numeric drawer control shipped together,
// so a capacity string can only be a corrupt row, and it stays an error here.
func NarrowNumericUpdates(updates map[string]interface{}) error {
	// One construction, not one per field: NumericEditFieldBounds builds a fresh
	// map on every call.
	registry := contracts.NumericEditFieldBounds()
	for field := range registry {
		if err := narrowNumericUpdate(updates, field, registry); err != nil {
			return err
		}
	}
	return nil
}

// narrowNumericUpdate applies NarrowNumericUpdates' rule to ONE field, so a
// caller that must judge fields independently (Rollback, which skips a refused
// field rather than refusing the whole revision) shares the rule rather than a
// second copy of it. A field with no registry entry is left alone.
//
// registry is passed in rather than looked up, because NumericEditFieldBounds
// builds a fresh map per call and a per-field caller would otherwise rebuild it
// once per field.
func narrowNumericUpdate(updates map[string]interface{}, field string, registry map[string]contracts.NumericEditBounds) error {
	bounds, registered := registry[field]
	if !registered {
		return nil
	}
	raw, present := updates[field]
	if !present {
		return nil
	}
	if raw == nil {
		updates[field] = (*int)(nil)
		return nil
	}
	if legacy, isString := raw.(string); isString && bounds.LegacyTextEncoding {
		// TrimSpace because Postgres accepts ' 1985'::int, so a padded string
		// is a value this column really would have taken back when the field
		// was unregistered. Atoi and nothing looser: it refuses "1985.0",
		// "1e3" and "1985 approx" outright rather than reading a prefix.
		n, err := strconv.Atoi(strings.TrimSpace(legacy))
		if err != nil {
			return apperrors.ErrPendingEditInvalidRequest(
				fmt.Sprintf("%s must be a whole number", field))
		}
		updates[field] = &n
		return nil
	}
	n, ok := utils.WholeNumber(raw)
	if !ok {
		return apperrors.ErrPendingEditInvalidRequest(
			fmt.Sprintf("%s must be a whole number", field))
	}
	updates[field] = &n
	return nil
}

// checkNumericUpdateBounds rejects a registered whole-number field whose value
// falls outside the range its column accepts. Runs on the APPROVE path only:
// that is where new contributor input is applied.
//
// The range is re-checked here even though the suggest-edit handler already
// rejected out-of-range values at submit time. This is the same defence-in-depth
// posture as FilterAllowedFields directly above: rows can reach
// pending_entity_edits from outside that handler, and this is the last point
// before an untyped Updates() that can still tell a real value from garbage.
//
// A bad value returns an invalid-request error (422) rather than an internal
// one, because the actionable fact is the value, not a fault in the server. The
// pending row is left PENDING rather than auto-rejected, unlike the
// disallowed-fields gate above which writes a rejection reason: a disallowed
// COLUMN is unambiguously corrupt and nobody should have to look at it, while a
// bad value is something an admin can read, judge, and reject with a real
// reason. RejectPendingEdit does not run this, so such a row is always
// clearable. Both dispositions are deliberate.
//
// Reads the same contracts.NumericEditFieldBounds registry the submit-side
// validator does, so the two cannot drift into disagreeing.
func checkNumericUpdateBounds(updates map[string]interface{}) error {
	for field, bounds := range contracts.NumericEditFieldBounds() {
		raw, present := updates[field]
		if !present {
			continue
		}
		narrowed, isPtr := raw.(*int)
		if !isPtr || narrowed == nil {
			continue // absent, or the clear gesture
		}
		if *narrowed < bounds.Min || *narrowed > bounds.Max {
			return apperrors.ErrPendingEditInvalidRequest(fmt.Sprintf(
				"%s must be between %d and %d", field, bounds.Min, bounds.Max))
		}
	}
	return nil
}

// fetchedURLFields maps a pending-edit field whose stored value is later
// FETCHED server-side to the user-facing label used in the refusal message. The
// canonical registry is urlFieldSpecs in internal/api/handlers/shared (the
// entries marked `fetched`); this is its counterpart on the apply side.
//
// To be precise about why it is a copy: nothing in the compiler stops a service
// from importing internal/api/handlers/shared (no cycle exists in either
// direction). It is a LAYERING rule, and one this codebase keeps everywhere
// else: no package under internal/services imports a handler package in
// production code. Handler-shaped concerns come with handler-shaped errors, and
// validateFetchHost over there returns a huma 422 that means nothing to a
// service.
//
// The better factoring, if this list ever grows past one entry, is to move the
// field-to-label registry AND its dispatch down into internal/utils/urlguard
// (a true leaf, already imported by both layers) and let each caller wrap the
// returned error in its own error type. That is left alone here because it
// would rewrite PSY-1675's urlFieldSpecs, whose displayName is also consumed by
// the scheme and length checks that have no business moving.
//
// TestFetchedURLFieldsMatchHandlerRegistry is the tripwire for the two drifting
// apart: mark another field `fetched` there without adding it here and the
// approve path silently stops guarding it.
var fetchedURLFields = map[string]string{
	"image_url": "Image URL",
}

// shapedURLFields is the apply-side counterpart of the `shape` rules in
// urlFieldSpecs: the URL fields whose stored value must take a particular FORM,
// beyond being a URL. It is a copy for the same layering reason fetchedURLFields
// is, and it carries the rule itself rather than only a label because the rule
// lives in internal/utils, which both layers may import.
//
// TestShapedURLFieldsMatchHandlerRegistry is the tripwire for the two drifting
// apart: add a `shape` rule there without adding it here and the approve path
// silently stops guarding that field.
var shapedURLFields = map[string]struct {
	displayName string
	validate    func(value, fieldName string) error
}{
	utils.BandcampEmbedURLField: {
		displayName: utils.BandcampEmbedURLLabel,
		validate:    utils.ValidateBandcampEmbedURL,
	},
}

// rollbackURLFields is every field a rollback may write whose value is a URL
// somebody will click, with the label a refusal names it by.
//
// It exists because Rollback is the one write path that takes its value from
// FieldChange.OldValue, and for every row written before deriveOldValues shipped
// that value is contributor input nothing validated: the submit handler checked
// NewValue only, and approve copies the pair verbatim into
// revisions.field_changes. So every forward gate (scheme, social host anchor,
// release shape) was bypassed by going backwards, for every field in the table.
// A contributor pairs a real Spotify NewValue with
// `https://spotify-verify.evil.test/` as the OldValue, an admin later presses
// rollback, and SocialLinks renders that host under the Spotify glyph.
//
// Deriving old_value at submit time closes that path for NEW rows only. These
// rules are what stands between a value already in the table and the column it
// names, so they are defence in depth over the same value rather than a
// duplicate of the derivation.
//
// It is keyed on FIELD NAME, so it covers artist, venue, label and festival
// alike: the allowlists share these names.
//
// It covers every URL field the shared registry knows, which is a SUPERSET of
// what today's *AllowedEditFields maps expose: cover_image_url, for instance,
// belongs to collections and is not editable through this pipeline at all. A
// superset on purpose: an entry costs one map lookup on a field that never
// appears, and a field added to an allowlist later is guarded on arrival rather
// than on someone remembering this file.
//
// Only the platform fields carry a host anchor; the rest get the scheme rule,
// which is still the difference between restoring a link and restoring
// "javascript:..." into a rendered attribute.
//
// image_url is HERE for its scheme rule but its host is NOT resolved: the SSRF
// guard needs a context.Context the function reading this map does not take.
// Rollback runs revalidateFetchedURLField for that half, so the two gates
// together reproduce the forward contract and neither alone does.
var rollbackURLFields = map[string]string{
	"instagram":       "Instagram URL",
	"facebook":        "Facebook URL",
	"twitter":         "Twitter URL",
	"youtube":         "YouTube URL",
	"spotify":         "Spotify URL",
	"soundcloud":      "SoundCloud URL",
	"bandcamp":        "Bandcamp URL",
	"website":         "Website URL",
	"ticket_url":      "Ticket URL",
	"cover_art_url":   "Cover art URL",
	"cover_image_url": "Cover image URL",
	"flyer_url":       "Flyer URL",
	"image_url":       "Image URL",
}

// validateRollbackURLField re-runs the forward paths' URL rules over ONE value a
// rollback is about to write, and reports it if it must not go live. A field
// with no registry entry carries no URL rule and passes.
//
// WHY THIS IS NOT PARANOIA: see rollbackURLFields. On any row that predates the
// submit-time derivation, OldValue is unvalidated contributor input that reaches
// the column through an admin's undo button.
//
// Per FIELD, because Rollback drops the fields it refuses rather than refusing
// the revision: a revision records every field of one contributor edit, so
// refusing it whole let one planted OldValue block the undo of every honest
// field recorded beside it.
//
// `website` is host-unrestricted by design (it is the any-host escape hatch), so
// for that field this is the scheme check alone, as it is for the image and
// flyer fields, which have no platform to anchor to.
//
// image_url gets its SCHEME rule here but not its host guard: that resolves DNS
// and needs a context.Context this function does not take. Rollback runs
// revalidateFetchedURLField over the same field for that half, so the two
// together reproduce the forward contract; neither alone does.
func validateRollbackURLField(updates map[string]interface{}, field string) error {
	displayName, guarded := rollbackURLFields[field]
	if !guarded {
		return nil
	}
	value, present, err := updateStringValue(updates, field, displayName)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	if err := utils.ValidateHTTPURL(value, displayName); err != nil {
		return err
	}
	return utils.ValidateSocialHost(field, displayName, value)
}

// revalidateFetchedURLs re-runs the SSRF host guard over the values an approval
// is about to apply, and reports one that must not go live.
//
// Submission already checks these (shared.ValidateFieldChangeValue), so this is
// deliberately a SECOND run of the same policy rather than a new one. It exists
// because the queue outlives the guard: a row written before PSY-1675 shipped,
// or through any write path that missed it, carries an unvetted value that
// approval would otherwise apply verbatim. Approval is where the value goes
// live, so approval is where it has to hold.
//
// Failing is the right outcome, not silently dropping the field: an admin who
// is told which host was refused can reject the edit with a reason, whereas a
// silent drop would approve an edit that did not do what either party read.
//
// It takes the built `updates` map, NOT the raw change slice, for two reasons.
// The value checked is then literally the value the untyped Updates() writes,
// so "validated equals applied" is structural rather than an argument about two
// loops staying in step. And driving the loop from fetchedURLFields caps the
// work at one DNS lookup per guarded field however many entries the row
// carries: this function's whole premise is that it cannot trust how the row
// got here, so it must not inherit the submit path's fan-out bounds
// (maxPendingEditChanges and the per-field dedup live in the handler, and a row
// that skipped the handler never met them).
//
// A present-but-non-string value is refused rather than skipped. It cannot be
// an SSRF vector (JSON yields float64/bool/map/slice/nil, none of which spell a
// URL, and pgx's extended protocol errors rather than coercing one into the
// varchar column), but skipping it would leave the row to 500 at the driver on
// every approve attempt and sit pending forever. A 422 is actionable. An empty
// string still passes: that is the clear-the-field gesture.
func revalidateFetchedURLs(ctx context.Context, updates map[string]interface{}) error {
	for field := range fetchedURLFields {
		if err := revalidateFetchedURLField(ctx, updates, field); err != nil {
			return err
		}
	}
	return nil
}

// revalidateFetchedURLField applies revalidateFetchedURLs' rule to ONE field, so
// the per-field caller shares the rule rather than a second copy of it. A field
// with no registry entry is never fetched server-side and passes.
func revalidateFetchedURLField(ctx context.Context, updates map[string]interface{}, field string) error {
	displayName, guarded := fetchedURLFields[field]
	if !guarded {
		return nil
	}
	value, present, err := updateStringValue(updates, field, displayName)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	return urlguard.Default.Validate(ctx, value, displayName)
}

// updateStringValue reads one field out of the map an approval is about to
// apply, and answers whether there is a string worth validating.
//
// It exists so each apply-side gate holds only its own RULE. What the map can
// contain is one contract, not one per gate: absent or nil means the approval
// does not touch the field, an empty string is the clear-the-field gesture, and
// a present-but-non-string value is refused rather than skipped.
//
// Refusing the non-string is the load-bearing part. It cannot be an attack (JSON
// yields float64/bool/map/slice/nil, none of which spell a URL, and pgx errors
// rather than coercing one into the column), but skipping it would leave the row
// to 500 at the driver on every approve attempt and sit pending forever. A 422
// is actionable.
func updateStringValue(updates map[string]interface{}, field, displayName string) (value string, present bool, err error) {
	raw, ok := updates[field]
	if !ok || raw == nil {
		return "", false, nil
	}
	s, isString := raw.(string)
	if !isString {
		return "", false, fmt.Errorf("%s must be a string", displayName)
	}
	if s == "" {
		return "", false, nil
	}
	return s, true, nil
}

// normalizeBlankShapedURLs turns the clear-the-field gesture into the value the
// column must actually hold for it: NULL, not "".
//
// AFTER the gate, never before, and the ordering is the whole correctness
// argument. ValidateBandcampEmbedURL refuses a whitespace-only value and passes
// only the empty string; normalizing first would turn "   " into nil and the
// gate would then skip it, silently accepting an input the rule says to refuse.
// Validate what arrived, then normalize what survived.
//
// Why NULL matters: a blank-but-not-null row is invisible to every
// `bandcamp_embed_url IS NULL` gate: the profile resolver, the release-derived
// fill, cmd/backfill-artist-bandcamp-embeds, cmd/sweep-link-suggestions, so the
// artist can never be repaired by any automated path again, while rendering
// exactly the same as NULL.
//
// Driven by shapedURLFields but delegating per field, so a future shape field
// whose blank is NOT a clear gesture does not silently inherit this.
func normalizeBlankShapedURLs(updates map[string]interface{}) {
	if raw, ok := updates[utils.BandcampEmbedURLField]; ok {
		updates[utils.BandcampEmbedURLField] = utils.BlankBandcampEmbedToNil(raw)
	}
}

// revalidateShapedURLs re-runs the FORM rules over the values an approval is
// about to apply, and reports one that must not go live.
//
// Same argument as revalidateFetchedURLs above, for a different class of rule
// and a different harm. Submission checks these too
// (shared.ValidateFieldChangeValue), so this is a SECOND run of one policy
// rather than a new one; it exists because the queue outlives the guard. A row
// written before a rule shipped, or through a path that missed it, carries an
// unvetted value that approval would otherwise apply verbatim.
//
// The harm it blocks is not a broken embed. bandcamp_embed_url renders as an
// OUTBOUND link labelled "Listen to <artist> on Bandcamp" wherever the embed
// resolve comes back empty, so an arbitrary host approved into that column is a
// phishing destination wearing a name readers trust.
//
// Reads the built `updates` map for the same reason the fetched-URL gate does:
// the value checked is then literally the value the untyped Updates() writes.
func revalidateShapedURLs(updates map[string]interface{}) error {
	for field := range shapedURLFields {
		if err := revalidateShapedURLField(updates, field); err != nil {
			return err
		}
	}
	return nil
}

// revalidateShapedURLField applies revalidateShapedURLs' rule to ONE field, so
// the per-field caller shares the rule rather than a second copy of it. A field
// with no registry entry carries no FORM rule and passes.
func revalidateShapedURLField(updates map[string]interface{}, field string) error {
	rule, shaped := shapedURLFields[field]
	if !shaped {
		return nil
	}
	value, present, err := updateStringValue(updates, field, rule.displayName)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	return rule.validate(value, rule.displayName)
}

// ApprovePendingEdit approves a pending edit, applying changes to the entity
// and recording a revision.
//
// ctx bounds the DNS lookups the SSRF host guard performs below; a disconnected
// admin cancels them.
func (s *PendingEditService) ApprovePendingEdit(ctx context.Context, editID uint, reviewerID uint) (*contracts.PendingEditResponse, error) {
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

	// Numeric columns first, before any entity-specific handling: JSONB hands
	// every number back as float64, and the driver would take that for an
	// integer column and silently truncate it. Narrowing is the type fix; the
	// bounds check is the policy one, and only new contributor input gets it.
	if err := NarrowNumericUpdates(updates); err != nil {
		return nil, err
	}
	if err := checkNumericUpdateBounds(updates); err != nil {
		return nil, err
	}

	// PSY-1692: re-classify the URL fields that get fetched server-side, reading
	// the very map the untyped Updates() below writes.
	//
	// Placement (after the allowlist gate and the map build, before the
	// transaction) is load bearing in three ways:
	//
	//  1. It leaves the row ACTIONABLE. Unlike entity_requests, whose Decide is an
	//     atomic pending→approved claim (so PSY-1675 had to check pre-claim in the
	//     handler or strand the row), this function has no claim: the status flip
	//     happens inside the transaction below. Returning here touches nothing, so
	//     the edit stays 'pending' with reviewed_by NULL and the admin can still
	//     reject it with a reason, or the contributor can cancel and resubmit.
	//  2. It runs after the PSY-572 disallowed-fields auto-rejection, so that
	//     gate's behaviour for corrupted submissions is unchanged and still wins.
	//  3. It reads `updates`, so no later edit to how the map is built can leave a
	//     checked value and a written value out of step. The branches below only
	//     add system-derived columns (geocode, metro, timezone), never a URL.
	//
	// In the service rather than in the approve handler so it covers BOTH callers
	// (the admin approve endpoint and the trusted-tier auto-approve inside
	// suggestEdit) at the one point where the value actually goes live.
	//
	// A cancelled admin request must not disable the classification: the writes
	// below take no context, so the row would still go live. urlguard detaches
	// from caller cancellation itself (hostResolvesPublic), which is why the
	// request ctx can be passed straight through here.
	if err := revalidateFetchedURLs(ctx, updates); err != nil {
		slog.Default().Warn("pending_edit_blocked_unsafe_url",
			"edit_id", edit.ID,
			"entity_type", edit.EntityType,
			"entity_id", edit.EntityID,
			"submitted_by", edit.SubmittedBy,
			"reviewer_id", reviewerID,
			"error", err.Error(),
		)
		return nil, apperrors.ErrPendingEditInvalidRequest(fmt.Sprintf(
			"cannot approve: %s. Reject this edit and ask the contributor to resubmit with a public image URL.",
			err,
		))
	}

	// PSY-1966. Kept a separate call rather than folded into the block above
	// because the two answer different questions (where a host POINTS vs what
	// shape a URL has) and only one of them resolves DNS.
	if err := revalidateShapedURLs(updates); err != nil {
		slog.Default().Warn("pending_edit_blocked_url_shape",
			"edit_id", edit.ID,
			"entity_type", edit.EntityType,
			"entity_id", edit.EntityID,
			"submitted_by", edit.SubmittedBy,
			"reviewer_id", reviewerID,
			"error", err.Error(),
		)
		return nil, apperrors.ErrPendingEditInvalidRequest(fmt.Sprintf("cannot approve: %s", err))
	}

	normalizeBlankShapedURLs(updates)

	// PSY-985: a venue location edit through the contribution flow bypasses
	// VenueService, so the system-derived columns have to be maintained here too.
	// They are not in the contributor allowlist, so they are set programmatically
	// AFTER the allowlist filter above.
	if edit.EntityType == "venue" {
		// The contribution flow bypasses VenueService, so the empty-to-NULL
		// normalization that path applies to age_policy has to be repeated here
		// or it simply does not happen for the flow that produces most values.
		// Without it a contributor submitting "  " lands a present-but-blank
		// policy (the exact state the column is meant to distinguish from NULL),
		// and " 21+ " becomes a second bucket beside "21+".
		if raw, ok := updates["age_policy"]; ok {
			if s, isString := raw.(string); isString {
				updates["age_policy"] = utils.NilIfBlank(s)
			}
		}
		_, addressChanged := updates["address"]
		_, zipcodeChanged := updates["zipcode"]
		// Street-level geocode (PSY-1536): the contribution path does not call
		// Nominatim inline, so when any component of the address key changes,
		// clear the street fields rather than leave coordinates that belong to
		// the OLD address. The API's freshness gate (streetGeocodeFresh) would
		// hide the stale values anyway; clearing keeps the row honest.
		// Re-resolution happens on the next daily street-geocode sweep
		// (catalog.StreetGeocodeSweep) — the cleared geocoded_address key no
		// longer matches, which is exactly what the reconciler queries for.
		if addressChanged || zipcodeChanged || shared.LocationTouched(updates) {
			updates["street_latitude"] = (*float64)(nil)
			updates["street_longitude"] = (*float64)(nil)
			updates["geocode_precision"] = (*string)(nil)
			updates["geocoded_address"] = (*string)(nil)
		}
	}

	// The system-derived location columns, for every entity type that has them:
	// a venue's coordinates and timezone, an artist's or festival's metro. Shared
	// with RevisionService.Rollback, which needs the identical re-derivation —
	// see applyDerivedLocation for why the two must not have separate copies.
	// Runs after the venue block above so the street-geocode clearing, which is
	// venue-only and NOT shared with rollback, keeps its position.
	applyDerivedLocation(s.db, edit.EntityType, edit.EntityID, updates)

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
