package catalog

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// AuditActionSetTagParent is the action name recorded when an admin sets a
// tag's parent (or clears it). Matches the fire-and-forget direct-GORM
// convention used by cleanup_service.go (PSY-308) and tag_merge.go (PSY-306).
const AuditActionSetTagParent = "set_tag_parent"

// maxHierarchyWalkDepth caps ancestor traversal so a malformed/legacy row
// (already-looped parent chain) can't spin forever. The cycle-detection
// logic rejects new cycles before they land, but we belt-and-suspenders
// the walk itself for operational safety.
const maxHierarchyWalkDepth = 64

// GetTagAncestors walks the parent chain for a tag, from its direct parent
// up to the root. Returns an empty slice (never nil) when the tag has no
// parent. The tag itself is NOT included in the result. Ordering is
// closest-ancestor-first (direct parent at index 0, root last).
//
// Bounded by maxHierarchyWalkDepth to protect against pre-existing looped
// data. Cycle-detection on writes is the real guard; this is a safety net.
func (s *TagService) GetTagAncestors(tagID uint) ([]*catalogm.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	ancestors := make([]*catalogm.Tag, 0, 4)
	seen := map[uint]struct{}{tagID: {}}

	var tag catalogm.Tag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrTagNotFound(tagID)
		}
		return nil, fmt.Errorf("failed to load tag: %w", err)
	}

	currentParentID := tag.ParentID
	for depth := 0; depth < maxHierarchyWalkDepth; depth++ {
		if currentParentID == nil {
			break
		}
		if _, loop := seen[*currentParentID]; loop {
			// Looped data on disk — stop walking and return what we have.
			break
		}

		var parent catalogm.Tag
		if err := s.db.First(&parent, *currentParentID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Orphaned parent_id pointing at a deleted row — stop walking.
				break
			}
			return nil, fmt.Errorf("failed to load ancestor: %w", err)
		}

		seen[parent.ID] = struct{}{}
		p := parent // copy so we can take &
		ancestors = append(ancestors, &p)
		currentParentID = parent.ParentID
	}

	return ancestors, nil
}

// GetTagChildren returns the direct children of a tag (one level down).
// Ordered by usage_count DESC then name ASC for stable rendering. Returns
// an empty slice (not nil) when the tag has no children.
func (s *TagService) GetTagChildren(tagID uint) ([]*catalogm.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var children []catalogm.Tag
	err := s.db.Where("parent_id = ?", tagID).
		Order("usage_count DESC, name ASC").
		Find(&children).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load children: %w", err)
	}

	out := make([]*catalogm.Tag, len(children))
	for i := range children {
		t := children[i]
		out[i] = &t
	}
	return out, nil
}

// GetGenreHierarchy returns all genre tags as a flat list with parent_id
// populated; the frontend builds the tree client-side. Flat shape keeps
// the query trivial (one indexed scan) and avoids a recursive CTE. Ordered
// so the UI can render consistently without client-side sorting.
func (s *TagService) GetGenreHierarchy() ([]*catalogm.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var tags []catalogm.Tag
	err := s.db.Where("category = ?", catalogm.TagCategoryGenre).
		Order("name ASC").
		Find(&tags).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load genre hierarchy: %w", err)
	}

	out := make([]*catalogm.Tag, len(tags))
	for i := range tags {
		t := tags[i]
		out[i] = &t
	}
	return out, nil
}

// SetTagParent sets or clears the parent of a tag. Passing parentID=nil
// clears the parent (makes the tag a root). Rejects:
//   - tag not found
//   - tag is not in the 'genre' category (hierarchy is genre-only)
//   - parentID equals tagID (direct self-parent)
//   - proposed parent is not found
//   - proposed parent is not in the 'genre' category
//   - proposed parent is a descendant of the tag (would create a cycle)
//
// Writes a fire-and-forget audit log entry on success, using the direct-GORM
// pattern from cleanup_service.go (PSY-308). Errors from the audit write are
// logged but never fail the parent operation.
func (s *TagService) SetTagParent(tagID uint, parentID *uint, actorUserID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var tag catalogm.Tag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrTagNotFound(tagID)
		}
		return fmt.Errorf("failed to load tag: %w", err)
	}

	if tag.Category != catalogm.TagCategoryGenre {
		return apperrors.ErrTagHierarchyNotGenre(tag.Name, tag.Category)
	}

	if err := s.validateTagParent(&tag, parentID); err != nil {
		return err
	}

	// Load parent name for the audit log BEFORE the update, so a race with
	// a concurrent rename still records the name we resolved against.
	var parentName string
	if parentID != nil {
		var p catalogm.Tag
		if err := s.db.First(&p, *parentID).Error; err == nil {
			parentName = p.Name
		}
	}

	// Use Select + Updates so GORM writes NULL when parentID is nil.
	// A plain Updates map would skip the nil entry and leave parent_id unchanged.
	if err := s.db.Model(&catalogm.Tag{}).
		Where("id = ?", tagID).
		Select("parent_id").
		Updates(map[string]interface{}{"parent_id": parentID}).Error; err != nil {
		return fmt.Errorf("failed to set tag parent: %w", err)
	}

	go s.writeSetParentAuditLog(actorUserID, tag.ID, tag.Name, parentID, parentName)
	return nil
}

// validateTagParent is the shared cycle-detection + category-guard helper.
// It is called by SetTagParent and by UpdateTag when the caller supplies a
// parent_id, so both entry points enforce the same rules.
//
// Pass tag = the tag being mutated. parentID = the proposed new parent
// (nil means "clearing" and always passes). validateTagParent does NOT
// verify tag.Category == genre — that's the caller's responsibility.
// Callers routing through UpdateTag want to preserve the historical behavior
// of not rejecting hierarchy writes on non-genre tags that already have a
// parent_id — but they SHOULD reject setting a new one. See UpdateTag.
func (s *TagService) validateTagParent(tag *catalogm.Tag, parentID *uint) error {
	if parentID == nil {
		return nil
	}
	if *parentID == tag.ID {
		return apperrors.ErrTagHierarchyCycle("cannot set a tag as its own parent")
	}

	var parent catalogm.Tag
	if err := s.db.First(&parent, *parentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrTagNotFound(*parentID)
		}
		return fmt.Errorf("failed to load proposed parent: %w", err)
	}
	if parent.Category != catalogm.TagCategoryGenre {
		return apperrors.ErrTagHierarchyNotGenre(parent.Name, parent.Category)
	}

	// Walk the proposed parent's ancestor chain: if we ever see tag.ID,
	// setting this parent would create a cycle. Walking ancestors (not
	// descendants) is O(depth) regardless of subtree size.
	seen := map[uint]struct{}{parent.ID: {}}
	cursor := parent.ParentID
	for depth := 0; depth < maxHierarchyWalkDepth; depth++ {
		if cursor == nil {
			return nil
		}
		if *cursor == tag.ID {
			return apperrors.ErrTagHierarchyCycle(
				fmt.Sprintf("'%s' is an ancestor of '%s'", tag.Name, parent.Name),
			)
		}
		if _, loop := seen[*cursor]; loop {
			// Pre-existing loop in the data; bail so we don't hang.
			return nil
		}
		seen[*cursor] = struct{}{}

		var anc catalogm.Tag
		if err := s.db.First(&anc, *cursor).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return fmt.Errorf("failed to walk ancestors: %w", err)
		}
		cursor = anc.ParentID
	}
	return nil
}

// writeSetParentAuditLog records a parent-set (or parent-clear) in the audit
// log via direct GORM, matching the pattern in cleanup_service.go (PSY-308).
// Errors are logged but never bubble up — fire-and-forget.
func (s *TagService) writeSetParentAuditLog(actorID, tagID uint, tagName string, parentID *uint, parentName string) {
	if s.db == nil {
		return
	}

	metadata := map[string]interface{}{
		"tag_id":   tagID,
		"tag_name": tagName,
	}
	if parentID != nil {
		metadata["parent_id"] = *parentID
		metadata["parent_name"] = parentName
	} else {
		metadata["parent_id"] = nil
		metadata["parent_name"] = nil
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		slog.Default().Error("failed to marshal set_tag_parent audit metadata", "error", err)
		return
	}

	raw := json.RawMessage(metadataJSON)
	var actor *uint
	if actorID > 0 {
		actor = &actorID
	}

	entry := adminm.AuditLog{
		ActorID:    actor,
		Action:     AuditActionSetTagParent,
		EntityType: "tag",
		EntityID:   tagID,
		Metadata:   &raw,
		CreatedAt:  time.Now().UTC(),
	}

	if err := s.db.Create(&entry).Error; err != nil {
		slog.Default().Error("failed to write set_tag_parent audit log", "error", err)
	}
}
