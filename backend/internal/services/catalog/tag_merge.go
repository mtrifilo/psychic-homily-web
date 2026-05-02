package catalog

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// AuditActionMergeTags is the action name recorded when an admin merges tags.
const AuditActionMergeTags = "merge_tags"

// PreviewMergeTags computes what a MergeTags call would do, without mutating state.
// Returns (nil, err) on validation error; (preview, nil) on success.
func (s *TagService) PreviewMergeTags(sourceID, targetID uint) (*contracts.MergeTagsPreview, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	source, target, err := s.loadMergeTags(s.db, sourceID, targetID)
	if err != nil {
		return nil, err
	}

	preview := &contracts.MergeTagsPreview{
		SourceName: source.Name,
		TargetName: target.Name,
	}

	moved, skipped, err := countEntityTagMoves(s.db, source.ID, target.ID)
	if err != nil {
		return nil, err
	}
	preview.MovedEntityTags = moved
	preview.SkippedEntityTags = skipped

	movedVotes, skippedVotes, err := countVoteMoves(s.db, source.ID, target.ID)
	if err != nil {
		return nil, err
	}
	preview.MovedVotes = movedVotes
	preview.SkippedVotes = skippedVotes

	// Split moved votes by sign so the merge dialog can show
	// "N upvotes, M downvotes" (PSY-487). The split mirrors the same conflict
	// rules used by countVoteMoves — votes that would be skipped because the
	// target already has a vote from that user on that entity are excluded.
	movedUp, movedDown, err := countVoteMovesBySign(s.db, source.ID, target.ID)
	if err != nil {
		return nil, err
	}
	preview.MovedUpvotes = movedUp
	preview.MovedDownvotes = movedDown

	var aliasCount int64
	if err := s.db.Model(&catalogm.TagAlias{}).Where("tag_id = ?", source.ID).Count(&aliasCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count source aliases: %w", err)
	}
	preview.SourceAliasesCount = aliasCount

	return preview, nil
}

// countVoteMovesBySign breaks the MovedVotes total into upvotes/downvotes so
// the merge preview UI can display per-sign counters. Conflict semantics
// match countVoteMoves: a source vote conflicting with a target vote (same
// user + entity) is excluded from both buckets — it'll be dropped in favor of
// the target's existing vote when the merge runs.
func countVoteMovesBySign(db *gorm.DB, sourceID, targetID uint) (up, down int64, err error) {
	type signRow struct {
		Vote  int
		Count int64
	}
	var rows []signRow
	err = db.Raw(`
		SELECT src.vote AS vote, COUNT(*) AS count
		FROM tag_votes src
		WHERE src.tag_id = ?
		  AND NOT EXISTS (
			SELECT 1 FROM tag_votes tgt
			WHERE tgt.tag_id = ?
			  AND tgt.entity_type = src.entity_type
			  AND tgt.entity_id   = src.entity_id
			  AND tgt.user_id     = src.user_id
		  )
		GROUP BY src.vote
	`, sourceID, targetID).Scan(&rows).Error
	if err != nil {
		return 0, 0, fmt.Errorf("failed to split vote moves by sign: %w", err)
	}
	for _, r := range rows {
		if r.Vote == 1 {
			up = r.Count
		} else if r.Vote == -1 {
			down = r.Count
		}
	}
	return up, down, nil
}

// MergeTags moves all entity_tags and tag_votes from source to target, creates
// an alias from source.name → target, re-points existing aliases of source to
// target, and deletes the source tag. Runs in a single transaction. Returns
// zeroed result on error.
//
// Rules:
//   - Entity-tag conflicts: if target is already applied to the same entity,
//     the source row is deleted (not double-applied).
//   - Vote conflicts: existing target vote wins; source vote is dropped.
//   - Aliases on source are re-pointed to target. If an alias name collides
//     with an alias already on the target, the existing target alias wins.
//   - source.name becomes an alias on target. Collisions abort the merge.
//   - is_official carries forward: if either source or target is official, the
//     result is official (union semantics — safe default that never loses the
//     official designation).
//
// actorUserID is used only for the post-commit audit log (fire-and-forget).
func (s *TagService) MergeTags(sourceID, targetID uint, actorUserID uint) (*contracts.MergeTagsResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if sourceID == 0 || targetID == 0 {
		return nil, apperrors.ErrTagMergeInvalid("source and target IDs are required")
	}
	if sourceID == targetID {
		return nil, apperrors.ErrTagMergeInvalid("cannot merge a tag into itself")
	}

	var (
		result      contracts.MergeTagsResult
		sourceName  string
		targetName  string
		mergedTagID uint
	)

	err := s.db.Transaction(func(tx *gorm.DB) error {
		source, target, err := s.loadMergeTags(tx, sourceID, targetID)
		if err != nil {
			return err
		}
		sourceName = source.Name
		targetName = target.Name
		mergedTagID = target.ID

		// Reject merge when source.name is already an alias on a different
		// canonical tag — the DB unique index on LOWER(alias) will also reject
		// this, but checking here surfaces a clean error instead of a 500.
		var existingAlias catalogm.TagAlias
		err = tx.Where("LOWER(alias) = LOWER(?)", source.Name).First(&existingAlias).Error
		if err == nil && existingAlias.TagID != target.ID {
			return apperrors.ErrTagMergeAliasConflict(source.Name, existingAlias.TagID)
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return fmt.Errorf("failed to check alias collision: %w", err)
		}

		// 1. Entity tags.
		moved, skipped, err := moveEntityTags(tx, source.ID, target.ID)
		if err != nil {
			return err
		}
		result.MovedEntityTags = moved
		result.SkippedEntityTags = skipped

		// 2. Votes.
		movedVotes, skippedVotes, err := moveVotes(tx, source.ID, target.ID)
		if err != nil {
			return err
		}
		result.MovedVotes = movedVotes
		result.SkippedVotes = skippedVotes

		// 3. Aliases: re-point source's aliases to target. If an alias name
		// already exists on the target, drop the source copy (target's alias wins).
		movedAliases, err := moveAliases(tx, source.ID, target.ID)
		if err != nil {
			return err
		}
		result.MovedAliases = movedAliases

		// 4. Create alias: source.name → target. Skip if target already has this alias.
		aliasExists := existingAlias.TagID == target.ID && strings.EqualFold(existingAlias.Alias, source.Name)
		if !aliasExists {
			// Also check that source.name doesn't collide with target.name (edge case).
			if !strings.EqualFold(source.Name, target.Name) {
				if err := tx.Create(&catalogm.TagAlias{TagID: target.ID, Alias: source.Name}).Error; err != nil {
					return fmt.Errorf("failed to create alias: %w", err)
				}
				result.AliasCreated = true
			}
		}

		// 5. Delete source tag. FK cascades would normally zap child rows, but
		// we've already moved everything off source; this is the final cleanup.
		if err := tx.Delete(&catalogm.Tag{}, source.ID).Error; err != nil {
			return fmt.Errorf("failed to delete source tag: %w", err)
		}

		// 6. Recompute target.usage_count from actual entity_tags — cheaper and
		// more reliable than arithmetic over movedEntityTags, which could drift
		// if anything else touches the rows concurrently.
		var count int64
		if err := tx.Model(&catalogm.EntityTag{}).Where("tag_id = ?", target.ID).Count(&count).Error; err != nil {
			return fmt.Errorf("failed to recount usage: %w", err)
		}
		if err := tx.Model(&catalogm.Tag{}).Where("id = ?", target.ID).Update("usage_count", count).Error; err != nil {
			return fmt.Errorf("failed to update usage count: %w", err)
		}

		// 7. Carry official designation forward: if either was official, the
		// result is official. Only update when we need to flip false → true on target.
		if source.IsOfficial && !target.IsOfficial {
			if err := tx.Model(&catalogm.Tag{}).Where("id = ?", target.ID).Update("is_official", true).Error; err != nil {
				return fmt.Errorf("failed to carry official flag: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Fire-and-forget audit log after the transaction commits, matching the
	// convention used in neighboring admin services. Errors inside LogAction
	// are logged but never bubble up.
	go s.writeMergeAuditLog(actorUserID, sourceID, mergedTagID, sourceName, targetName, &result)

	return &result, nil
}

// loadMergeTags resolves source and target, and rejects circular merges.
func (s *TagService) loadMergeTags(db *gorm.DB, sourceID, targetID uint) (*catalogm.Tag, *catalogm.Tag, error) {
	var source catalogm.Tag
	if err := db.First(&source, sourceID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, apperrors.ErrTagNotFound(sourceID)
		}
		return nil, nil, fmt.Errorf("failed to load source: %w", err)
	}
	var target catalogm.Tag
	if err := db.First(&target, targetID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, apperrors.ErrTagNotFound(targetID)
		}
		return nil, nil, fmt.Errorf("failed to load target: %w", err)
	}

	// Guard against circular merge: if source is already an alias of target,
	// or target is already an alias of source, the relationship is ambiguous.
	var aliasOfTarget catalogm.TagAlias
	err := db.Where("tag_id = ? AND LOWER(alias) = LOWER(?)", target.ID, source.Name).First(&aliasOfTarget).Error
	if err == nil {
		return nil, nil, apperrors.ErrTagMergeInvalid(
			fmt.Sprintf("'%s' is already an alias of '%s'", source.Name, target.Name),
		)
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, nil, fmt.Errorf("failed to check existing alias relationship: %w", err)
	}

	return &source, &target, nil
}

// moveEntityTags moves source's entity_tag rows to target. Rows where target
// already has the same (entity_type, entity_id) are deleted instead (no
// double-application).
func moveEntityTags(db *gorm.DB, sourceID, targetID uint) (moved, skipped int64, err error) {
	// Delete source rows whose (entity_type, entity_id) already exist on target.
	del := db.Exec(`
		DELETE FROM entity_tags
		WHERE tag_id = ?
		  AND EXISTS (
			SELECT 1 FROM entity_tags et2
			WHERE et2.tag_id = ?
			  AND et2.entity_type = entity_tags.entity_type
			  AND et2.entity_id   = entity_tags.entity_id
		  )
	`, sourceID, targetID)
	if del.Error != nil {
		return 0, 0, fmt.Errorf("failed to drop conflicting entity_tags: %w", del.Error)
	}
	skipped = del.RowsAffected

	// Remaining source rows: re-point to target.
	upd := db.Model(&catalogm.EntityTag{}).Where("tag_id = ?", sourceID).Update("tag_id", targetID)
	if upd.Error != nil {
		return 0, 0, fmt.Errorf("failed to move entity_tags: %w", upd.Error)
	}
	moved = upd.RowsAffected

	return moved, skipped, nil
}

// countEntityTagMoves is the preview-only counterpart to moveEntityTags.
func countEntityTagMoves(db *gorm.DB, sourceID, targetID uint) (moved, skipped int64, err error) {
	var total int64
	if err := db.Model(&catalogm.EntityTag{}).Where("tag_id = ?", sourceID).Count(&total).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to count source entity_tags: %w", err)
	}
	var conflicts int64
	err = db.Raw(`
		SELECT COUNT(*) FROM entity_tags src
		WHERE src.tag_id = ?
		  AND EXISTS (
			SELECT 1 FROM entity_tags tgt
			WHERE tgt.tag_id = ?
			  AND tgt.entity_type = src.entity_type
			  AND tgt.entity_id   = src.entity_id
		  )
	`, sourceID, targetID).Scan(&conflicts).Error
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count entity_tag conflicts: %w", err)
	}
	return total - conflicts, conflicts, nil
}

// moveVotes moves source's tag_vote rows to target. Rows where target already
// has a vote for the same (entity_type, entity_id, user_id) are deleted — the
// target's existing vote wins.
func moveVotes(db *gorm.DB, sourceID, targetID uint) (moved, skipped int64, err error) {
	del := db.Exec(`
		DELETE FROM tag_votes
		WHERE tag_id = ?
		  AND EXISTS (
			SELECT 1 FROM tag_votes tv2
			WHERE tv2.tag_id = ?
			  AND tv2.entity_type = tag_votes.entity_type
			  AND tv2.entity_id   = tag_votes.entity_id
			  AND tv2.user_id     = tag_votes.user_id
		  )
	`, sourceID, targetID)
	if del.Error != nil {
		return 0, 0, fmt.Errorf("failed to drop conflicting tag_votes: %w", del.Error)
	}
	skipped = del.RowsAffected

	upd := db.Model(&catalogm.TagVote{}).Where("tag_id = ?", sourceID).Update("tag_id", targetID)
	if upd.Error != nil {
		return 0, 0, fmt.Errorf("failed to move tag_votes: %w", upd.Error)
	}
	moved = upd.RowsAffected

	return moved, skipped, nil
}

// countVoteMoves is the preview-only counterpart to moveVotes.
func countVoteMoves(db *gorm.DB, sourceID, targetID uint) (moved, skipped int64, err error) {
	var total int64
	if err := db.Model(&catalogm.TagVote{}).Where("tag_id = ?", sourceID).Count(&total).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to count source votes: %w", err)
	}
	var conflicts int64
	err = db.Raw(`
		SELECT COUNT(*) FROM tag_votes src
		WHERE src.tag_id = ?
		  AND EXISTS (
			SELECT 1 FROM tag_votes tgt
			WHERE tgt.tag_id = ?
			  AND tgt.entity_type = src.entity_type
			  AND tgt.entity_id   = src.entity_id
			  AND tgt.user_id     = src.user_id
		  )
	`, sourceID, targetID).Scan(&conflicts).Error
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count vote conflicts: %w", err)
	}
	return total - conflicts, conflicts, nil
}

// writeMergeAuditLog records a tag merge in the audit log.
// Fire-and-forget — errors are logged but never fail the parent operation.
func (s *TagService) writeMergeAuditLog(actorID, sourceID, targetID uint, sourceName, targetName string, result *contracts.MergeTagsResult) {
	if s.db == nil {
		return
	}

	metadata := map[string]interface{}{
		"source_tag_id":       sourceID,
		"source_tag_name":     sourceName,
		"target_tag_name":     targetName,
		"moved_entity_tags":   result.MovedEntityTags,
		"moved_votes":         result.MovedVotes,
		"skipped_entity_tags": result.SkippedEntityTags,
		"skipped_votes":       result.SkippedVotes,
		"moved_aliases":       result.MovedAliases,
		"alias_created":       result.AliasCreated,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		slog.Default().Error("failed to marshal merge audit metadata", "error", err)
		return
	}

	raw := json.RawMessage(metadataJSON)
	var actor *uint
	if actorID > 0 {
		actor = &actorID
	}

	auditLog := adminm.AuditLog{
		ActorID:    actor,
		Action:     AuditActionMergeTags,
		EntityType: "tag",
		EntityID:   targetID,
		Metadata:   &raw,
		CreatedAt:  time.Now().UTC(),
	}

	if err := s.db.Create(&auditLog).Error; err != nil {
		slog.Default().Error("failed to write merge audit log", "error", err)
	}
}

// moveAliases re-points aliases from source to target. Aliases whose name
// already exists on target (case-insensitive) are dropped — target's alias wins.
func moveAliases(db *gorm.DB, sourceID, targetID uint) (int64, error) {
	del := db.Exec(`
		DELETE FROM tag_aliases
		WHERE tag_id = ?
		  AND EXISTS (
			SELECT 1 FROM tag_aliases ta2
			WHERE ta2.tag_id = ?
			  AND LOWER(ta2.alias) = LOWER(tag_aliases.alias)
		  )
	`, sourceID, targetID)
	if del.Error != nil {
		return 0, fmt.Errorf("failed to drop conflicting aliases: %w", del.Error)
	}

	upd := db.Model(&catalogm.TagAlias{}).Where("tag_id = ?", sourceID).Update("tag_id", targetID)
	if upd.Error != nil {
		return 0, fmt.Errorf("failed to move aliases: %w", upd.Error)
	}
	return upd.RowsAffected, nil
}
